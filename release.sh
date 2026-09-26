#!/usr/bin/env bash

# Exit on any error, undefined variable, or pipeline failure
set -euo pipefail

VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
    echo "❌ Error: Version argument is required."
    echo "Usage: ./release.sh v1.0.0"
    exit 1
fi

# Ensure version starts with 'v' followed by semantic versioning
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    echo "❌ Error: Version '$VERSION' must follow semantic versioning format: vX.Y.Z (e.g. v1.0.0, v1.0.1-rc1)"
    exit 1
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
echo "🚀 Starting release process for $VERSION on branch '$CURRENT_BRANCH'..."

# 1. Format code
echo "🧹 Formatting Go code..."
go fmt ./...
(cd example && go fmt ./...)

# 2. Static Analysis & Linting
echo "🔍 Running go vet..."
go vet ./...
(cd example && go vet ./...)

# 3. Run All Unit & Integration Tests
echo "🧪 Running tests across all packages..."
go test -v -count=1 ./...

# 4. Verify Example App Builds Cleanly
echo "📦 Verifying example application builds..."
(cd example && go build -o /dev/null .)

# 5. Run Benchmarks to Detect Performance Regressions
echo "⚡ Running benchmarks to verify zero-allocation hot paths..."
go test -run=^$ -bench=. -benchmem ./tests

# 6. Tidy Go Modules
echo "📄 Tidying go.mod dependencies..."
go mod tidy
(cd example && go mod tidy)

# 7. Git Commit & Tag
echo "🏷️ Creating git tag for $VERSION..."
git add .
if ! git diff --cached --quiet; then
    git commit -m "chore: prepare release $VERSION"
fi

if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "⚠️ Warning: Tag '$VERSION' already exists locally. Deleting and recreating..."
    git tag -d "$VERSION"
fi

git tag -a "$VERSION" -m "Release $VERSION"

echo ""
echo "🎉 Release $VERSION successfully prepared and tagged locally!"
echo ""
echo "To publish this release to GitHub / remote, run:"
echo "    git push origin $CURRENT_BRANCH --tags"