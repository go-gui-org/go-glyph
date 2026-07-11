# Contributing to Go-Glyph

## Prerequisites

- Go 1.26+
- [golangci-lint](https://golangci-lint.run/)
- SDL2 (for GPU examples only; not needed for the `glyph` package itself)

## Build and Test

```bash
go build ./...                  # build all packages
go test ./...                   # run all tests
go vet ./...                    # static analysis
golangci-lint run ./...         # full lint
```

## Coding Conventions

- All code must pass `gofmt` and `golangci-lint run ./...` with zero issues
  before committing.

## Submitting Changes

1. Fork the repository and create a feature branch.
2. Make focused, single-purpose commits.
3. Add or update tests for any changed behavior.
4. Run the full check suite before pushing:
   ```bash
   go test ./... && go vet ./... && golangci-lint run ./...
   ```
5. Open a pull request against `main`.

## License

Contributions are accepted under the [MIT License](LICENSE).
