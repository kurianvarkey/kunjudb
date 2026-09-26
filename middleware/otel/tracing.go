package otel

import (
	"context"
	"database/sql"

	"github.com/kurianvarkey/kunjudb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type tracingExecutor struct {
	inner  kunjudb.SqlExecutor
	tracer trace.Tracer
}

// NewTracingMiddleware returns a kunjudb.Middleware that traces database queries using OpenTelemetry.
func NewTracingMiddleware(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
	return &tracingExecutor{
		inner:  next,
		tracer: otel.Tracer("kunjudb-orm"),
	}
}

func (te *tracingExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := te.tracer.Start(ctx, "DB Query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.statement", query),
			attribute.String("db.system", "postgresql"),
		),
	)
	defer span.End()

	rows, err := te.inner.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
	}

	return rows, err
}

func (te *tracingExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, span := te.tracer.Start(ctx, "DB Exec",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.statement", query),
			attribute.String("db.system", "postgresql"),
		),
	)
	defer span.End()

	res, err := te.inner.ExecContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
	}

	return res, err
}
