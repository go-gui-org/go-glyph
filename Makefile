.PHONY: docs docs-serve doc-tools api-doc check test-race coverage vet lint lint-cross lint-bin nocgo \
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

DOC_BIN_GOMARKDOC = $(LINT_DIR)/gomarkdoc
DOC_BIN_PKGSITE   = $(LINT_DIR)/pkgsite

## doc-tools: build the pinned gomarkdoc and pkgsite into .bin/ from
## tools/docs/go.mod. Same pattern as the linter: the versions live in
## that module so they stay out of go-glyph's published go.mod.
$(DOC_BIN_GOMARKDOC) $(DOC_BIN_PKGSITE): tools/docs/go.mod tools/docs/go.sum
	GOWORK=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS= GOBIN=$(LINT_DIR) \
	  go -C tools/docs install \
	  github.com/princjef/gomarkdoc/cmd/gomarkdoc \
	  golang.org/x/pkgsite/cmd/pkgsite

doc-tools: $(DOC_BIN_GOMARKDOC) $(DOC_BIN_PKGSITE)

## docs: run go generate (API doc and emoji table; the emoji step needs
## network access)
docs: doc-tools
	GOMARKDOC=$(DOC_BIN_GOMARKDOC) go generate ./...

## api-doc: regenerate docs/api/API.md only (no network, unlike `docs`,
## whose go generate also refreshes the emoji table from unicode.org).
api-doc: $(DOC_BIN_GOMARKDOC)
	GOMARKDOC=$(DOC_BIN_GOMARKDOC) bash scripts/gen-docs.sh

## docs-serve: browse package docs locally via pkgsite
docs-serve: $(DOC_BIN_PKGSITE)
	$(DOC_BIN_PKGSITE) -open .

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

## lint-cross: lint the targets the host never compiles. Files are swapped
## by build tag (*_wasm.go, discover_windows.go, ...), and golangci-lint
## only sees files that build for its GOOS, so `make lint` on one OS leaves
## the others unlinted. backend/gpu needs cgo and iOS needs an Xcode
## sysroot, so neither is here.
lint-cross: $(LINT_BIN)
	GOOS=js GOARCH=wasm CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(LINT) run . ./accessibility ./ime

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
## lint, lint-cross (lints every cgo-free target), the cgo-free root gate,
## the js/wasm and android cross-builds, and the separate-module builds.
## Aborts on the first failing target.
##
## Omissions vs CI, by design:
##   - the OS matrix (CI runs the suite on linux, windows and macOS)
##   - the cgo halves of the ios and android jobs, which need an Xcode
##     iphonesimulator sysroot or an Android NDK clang
##   - the coverage artifact upload; CI enforces no threshold on it, so
##     there is nothing to gate locally
prepush: test-race vet lint lint-cross nocgo cross-build build-modules
