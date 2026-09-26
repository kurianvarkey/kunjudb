.PHONY: test bench fmt lint

# Run all tests
test:
	go test -v ./...

# Run benchmarks to ensure no performance regressions in the scanner
bench:
	go test -run=^$$ -bench=. -benchmem ./tests

# Format code
fmt:
	go fmt ./...

# Check for common Go mistakes
lint:
	go vet ./...
	staticcheck ./...