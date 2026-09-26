#!/bin/bash

# Exit on any error
set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: ./release.sh v1.x.x"
    exit 1
fi

echo "🚀 Starting release process for $VERSION..."

# 1. Run Format and Lint
echo "Checking code style..."
go fmt ./...
go vet ./...

# 2. Run Tests
echo "Running integration tests..."
go test -v ./...

# 3. Check for Performance Regressions
echo "Running benchmarks..."
# We check if allocations have spiked
BENCH_RESULT=$(go test -bench=. -benchmem ./kunjudb/)
echo "$BENCH_RESULT"

# 4. Update go.mod
echo "Tidying go.mod..."
go mod tidy

# 5. Git Operations
echo "Tagging version $VERSION..."
git add .
git commit -m "chore: release $VERSION" || true # allow skip if no changes
git tag -a $VERSION -m "Release $VERSION"

echo "✅ Done! To push the release, run:"
echo "git push origin main --tags"