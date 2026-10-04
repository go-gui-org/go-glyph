.PHONY: docs docs-serve check test-race coverage vet lint lint-bin nocgo \
	cross-build build-modules prepush

# Repo-local bin for the pinned linter. The pinned VERSION itself lives in
# tools/lint/go.mod -- see the $(LINT_BIN) rule below. `make lint` and CI
# both build from that file, so a local pass and a CI pass run one version.
LINT_DIR = $(CURDIR)/.bin
LINT_BIN = $(LINT_DIR)/golangci-lint

# golangci-lint honours go.work the same way the toolchain does. This repo
# ships no workspace file, but a developer may add one to point at a
# sibling checkout; the gate should still validate what CI builds.
LINT := GOWORK=off $(LINT_BIN)

## docs: generate docs/api/API.md via gomarkdoc
docs:
	go generate ./...

## docs-serve: browse package docs locally via pkgsite
docs-serve:
	pkgsite -open .

## check: run tests, vet, and lint
check: $(LINT_BIN)
	go test ./...
	go vet ./...
	$(LINT) run ./...

## test-race: run tests with the Go race detector enabled
test-race:
	go test -race ./...

## coverage: run tests with coverage profile and open HTML report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

## vet: go vet the root module. Mirrors the CI test job's vet step.
vet:
	go vet ./...

## lint-bin: build the pinned golangci-lint into .bin/. It rebuilds only
## when tools/lint/go.mod or go.sum change. GOWORK=off keeps a local
## go.work out of the build. GOOS/GOARCH/CGO_ENABLED are cleared so a
## caller that sets them to pick a lint target does not cross-compile
## the linter itself into a binary this host cannot run.
$(LINT_BIN): tools/lint/go.mod tools/lint/go.sum
	GOWORK=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS= GOBIN=$(LINT_DIR) \
	  go -C tools/lint install \
	  github.com/golangci/golangci-lint/v2/cmd/golangci-lint

lint-bin: $(LINT_BIN)

## lint: lint the packages CI lints. Note the scope is narrower than the
## `check` target's ./... — CI passes this explicit list, and this target
## exists to reproduce CI exactly. Run `make check` for the wider sweep.
lint: $(LINT_BIN)
	$(LINT) run . ./accessibility ./backend/gpu ./ime

## nocgo: the cgo-free gate CI enforces on the root package. The root is
## pure Go — only backend/ and the examples pull in C — and CI asserts that
## on both linux and android, so a stray cgo import must fail here.
nocgo:
	CGO_ENABLED=0 go build .
	CGO_ENABLED=0 go vet .
	CGO_ENABLED=0 go test .

## cross-build: the cross-target compile gates CI runs, minus the ones
## needing a C toolchain. js/wasm is CI's whole test-wasm job; android is
## the CGO_ENABLED=0 half of test-android. The cgo halves of the android
## and ios jobs need an NDK / Xcode sysroot and stay CI-only.
cross-build:
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go build ./...
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go vet ./...
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build .
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go vet .

## build-modules: the separate modules CI compiles. Each has its own
## go.mod, so `go build ./...` from the root never reaches them and they
## bit-rot silently when the root API changes.
build-modules:
	( cd backend/ebitengine && go build ./... )
	( cd examples/demo && go build ./... )
	( cd examples/demo_gpu && go build ./... )
	( cd examples/showcase_gpu && go build ./... )

## prepush: recommended full local validation before pushing (issue
## go-gui#314). Approximates the CI matrix from one host: race tests, vet,
## lint, the cgo-free root gate, the js/wasm and android cross-builds, and
## the separate-module builds. Aborts on the first failing target.
##
## Omissions vs CI, by design:
##   - the OS matrix (CI runs the suite on linux, windows and macOS)
##   - the cgo halves of the ios and android jobs, which need an Xcode
##     iphonesimulator sysroot or an Android NDK clang
##   - the coverage artifact upload; CI enforces no threshold on it, so
##     there is nothing to gate locally
prepush: test-race vet lint nocgo cross-build build-modules
