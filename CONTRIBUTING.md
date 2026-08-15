# Contributing to Go-Glyph

## Prerequisites

- Go 1.26+
- [golangci-lint](https://golangci-lint.run/)
- SDL2 (for GPU examples only; not needed for the `glyph` package itself)

## Build and Test

Run the full local validation gate before pushing a branch:

```bash
make prepush
```

`make prepush` approximates the CI matrix from one host: race-enabled tests,
`go vet`, lint at the version CI pins, the cgo-free root-package gate, the
js/wasm and android cross-builds, and the separate-module builds. It aborts on
the first failing target.

Individual targets for a tighter loop while iterating:

```bash
make test-race      # tests with the race detector
make vet            # static analysis
make lint           # lint the packages CI lints, at the pinned version
make nocgo          # CGO_ENABLED=0 build/vet/test of the root package
make cross-build    # js/wasm and android compile gates
make build-modules  # backend/ebitengine and the example modules
make check          # tests + vet + a wider golangci-lint run ./...
make coverage       # coverage profile and HTML report
```

Two scope notes:

- `make lint` runs the explicit package list CI passes
  (`. ./accessibility ./backend/gpu ./ime`). `make check` lints `./...`, which
  is wider than CI. `prepush` uses the CI-faithful one.
- `make build-modules` exists because `backend/ebitengine` and the example
  programs have their own `go.mod` files. A root `go build ./...` never reaches
  them, so they bit-rot silently when the root API changes.

Gate targets run with `GOWORK=off` so they resolve the versions in `go.mod`.
This repo ships no workspace file, but golangci-lint honours one if you add it,
and CI never sees it.

### CI-only validation

- The OS matrix — CI runs the suite on Linux, Windows and macOS.
- The cgo halves of the iOS and Android jobs, which need an Xcode
  `iphonesimulator` sysroot or an Android NDK clang. `make cross-build` covers
  the CGO-free android half; `make nocgo` covers the cgo-free gate.
- The coverage artifact upload. CI enforces no threshold on it, so there is
  nothing to gate locally.

## Coding Conventions

- All code must pass `gofmt` and `golangci-lint run ./...` with zero issues
  before committing.

## Submitting Changes

1. Fork the repository and create a feature branch.
2. Make focused, single-purpose commits.
3. Add or update tests for any changed behavior.
4. Run the full check suite before pushing:
   ```bash
   make prepush
   ```
5. Open a pull request against `main`.

## License

Contributions are accepted under the [MIT License](LICENSE).
