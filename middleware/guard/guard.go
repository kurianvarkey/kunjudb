package guard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kurianvarkey/kunjudb"
)

var (
	ErrDangerousSQL = errors.New("security: dangerous SQL command blocked")

	defaultKeywords = []string{
		`DROP\s+TABLE`,
		`TRUNCATE`,
		`ALTER\s+TABLE`,
		`DROP\s+DATABASE`,
	}
	errFormat = "🚨 SECURITY ALERT: Blocked query: %s\n"
)

type sqlGuard struct {
	inner   kunjudb.SqlExecutor
	blocked *regexp.Regexp
}

// New returns a kunjudb.Middleware that blocks queries matching the provided regex keywords.
func New(keywords ...string) kunjudb.Middleware {
	finalKeywords := defaultKeywords
	if len(keywords) > 0 {
		finalKeywords = keywords
	}

	pattern := "(?i)(" + strings.Join(finalKeywords, "|") + ")"
	rgx := regexp.MustCompile(pattern)

	return func(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
		return &sqlGuard{
			inner:   next,
			blocked: rgx,
		}
	}
}

func (g *sqlGuard) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !g.isSafe(query) {
		fmt.Printf(errFormat, query)
		return nil, ErrDangerousSQL
	}
	return g.inner.ExecContext(ctx, query, args...)
}

func (g *sqlGuard) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if !g.isSafe(query) {
		fmt.Printf(errFormat, query)
		return nil, ErrDangerousSQL
	}
	return g.inner.QueryContext(ctx, query, args...)
}

func (g *sqlGuard) isSafe(query string) bool {
	return !g.blocked.MatchString(query)
}

type SecurityProfile int

const (
	ProfileDevelopment SecurityProfile = iota
	ProfileReadOnly
	ProfileStrict
)

// Profile returns a pre-configured SQL firewall middleware based on the environment.
func Profile(profile SecurityProfile) kunjudb.Middleware {
	switch profile {
	case ProfileStrict:
		return New("DROP", "TRUNCATE", "ALTER", "GRANT", "REVOKE")
	case ProfileReadOnly:
		return New("INSERT", "UPDATE", "DELETE", "DROP", "TRUNCATE", "ALTER")
	case ProfileDevelopment:
		return New("DROP DATABASE")
	default:
		return New()
	}
}
