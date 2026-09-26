package profiler

import (
	"context"
	"time"
)

// QueryObserver is an optional consumer interface for telemetry and metrics.
type QueryObserver interface {
	OnQuery(ctx context.Context, query string, duration time.Duration, err error)
}

// QueryObserverFunc allows using a standard function as a QueryObserver.
type QueryObserverFunc func(ctx context.Context, query string, duration time.Duration, err error)

// OnQuery implements QueryObserver for QueryObserverFunc.
func (f QueryObserverFunc) OnQuery(ctx context.Context, query string, duration time.Duration, err error) {
	f(ctx, query, duration, err)
}
