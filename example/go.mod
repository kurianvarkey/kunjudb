module github.com/kurianvarkey/kunjudb-example

go 1.27.1

require (
	github.com/kurianvarkey/kunjudb v0.0.0
	github.com/lib/pq v1.10.9
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
)

replace github.com/kurianvarkey/kunjudb => ../
