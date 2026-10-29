default:
    @just --list

# Build binary to bin/lserve
build:
    @mkdir -p bin
    go build -o bin/lserve ./cmd/lserve

# Format Go source code
fmt:
    go fmt ./...

# Vet Go source code
vet:
    go vet ./...

# Run unit tests
test *args:
    go test -short ./... {{args}}

# Run checks (gofmt check, vet, test)
check:
    test -z "$(gofmt -l .)"
    go vet ./...
    go test -short ./...

# Local CI verification gate
ci: check build
