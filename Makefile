.PHONY: docs docs-serve check test-race coverage

## docs: generate docs/api/API.md via gomarkdoc
docs:
	go generate ./...

## docs-serve: browse package docs locally via pkgsite
docs-serve:
	pkgsite -open .

## check: run tests, vet, and lint (requires golangci-lint)
check:
	go test ./...
	go vet ./...
	golangci-lint run ./...

## test-race: run tests with the Go race detector enabled
test-race:
	go test -race ./...

## coverage: run tests with coverage profile and open HTML report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
