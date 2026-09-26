package profiler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kurianvarkey/kunjudb"
)

// Config defines settings for slow query tracking, telemetry, and index advisory.
type Config struct {
	// SlowThreshold defines the execution time above which a query is considered slow.
	// Defaults to 200ms if <= 0.
	SlowThreshold time.Duration

	// EnableIndexAdvisor, when true and attached to a PostgreSQL database, runs EXPLAIN
	// on slow SELECT queries to identify sequential scans and recommend missing indexes.
	EnableIndexAdvisor bool

	// Logger is the structured logger used to log slow queries. Defaults to slog.Default().
	Logger *slog.Logger

	// Observer is an optional telemetry observer hook (e.g., for Prometheus or OpenTelemetry).
	Observer QueryObserver
}

type profilerMiddleware struct {
	inner kunjudb.SqlExecutor
	rawDB *sql.DB
	cfg   Config
}

// New creates a kunjudb.Middleware that profiles queries, invokes optional observers,
// logs slow queries using structured slog, and optionally recommends indexes for slow PostgreSQL queries.
func New(rawDB *sql.DB, cfg Config) kunjudb.Middleware {
	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = 200 * time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return func(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
		return &profilerMiddleware{
			inner: next,
			rawDB: rawDB,
			cfg:   cfg,
		}
	}
}

func (p *profilerMiddleware) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := p.inner.QueryContext(ctx, query, args...)
	p.handleProfiling(ctx, query, args, time.Since(start), err)
	return rows, err
}

func (p *profilerMiddleware) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := p.inner.ExecContext(ctx, query, args...)
	p.handleProfiling(ctx, query, args, time.Since(start), err)
	return res, err
}

func (p *profilerMiddleware) handleProfiling(ctx context.Context, query string, args []any, dur time.Duration, err error) {
	if p.cfg.Observer != nil {
		p.cfg.Observer.OnQuery(ctx, query, dur, err)
	}

	if dur < p.cfg.SlowThreshold {
		return
	}

	attrs := []any{
		slog.Duration("duration", dur),
		slog.Duration("threshold", p.cfg.SlowThreshold),
		slog.String("query", query),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	if p.cfg.EnableIndexAdvisor && p.rawDB != nil && isSelectQuery(query) {
		if advice := p.inspectPlanForIndex(ctx, query, args); advice != "" {
			attrs = append(attrs, slog.String("index_recommendation", advice))
		}
	}

	p.cfg.Logger.WarnContext(ctx, "🚨 Slow Database Query Detected", attrs...)
}

func isSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if len(trimmed) < 6 {
		return false
	}
	prefix := strings.ToUpper(trimmed[:6])
	return prefix == "SELECT" || strings.HasPrefix(strings.ToUpper(trimmed), "WITH")
}

func (p *profilerMiddleware) inspectPlanForIndex(ctx context.Context, query string, args []any) string {
	explainCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	var planJSON string
	row := p.rawDB.QueryRowContext(explainCtx, "EXPLAIN (FORMAT JSON) "+query, args...)
	if err := row.Scan(&planJSON); err != nil {
		return ""
	}

	var root []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(planJSON), &root); err != nil || len(root) == 0 {
		return ""
	}

	return findSeqScanRecommendation(root[0].Plan)
}

func findSeqScanRecommendation(plan map[string]any) string {
	nodeType, _ := plan["Node Type"].(string)
	relationName, _ := plan["Relation Name"].(string)
	filter, _ := plan["Filter"].(string)

	if nodeType == "Seq Scan" && relationName != "" && filter != "" {
		return fmt.Sprintf("Sequential scan detected on table '%s' (filter: %s). Consider adding an index on table '%s'.",
			relationName, filter, relationName)
	}

	if children, ok := plan["Plans"].([]any); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]any); ok {
				if rec := findSeqScanRecommendation(childMap); rec != "" {
					return rec
				}
			}
		}
	}
	return ""
}
