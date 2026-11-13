# Plan 007: Doc-generation tools live in their own module, pinned, and out of every consumer's module graph

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 7234b0b..HEAD -- go.mod go.sum tools/ scripts/gen-docs.sh Makefile AGENTS.md docs/api/API.md`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dependencies / DX
- **Planned at**: commit `7234b0b`, 2026-11-13

## Why this matters

go-glyph is a published library (`github.com/go-gui-org/go-glyph`). Its root
`go.mod` lists two **documentation tools** as direct requirements:
`github.com/princjef/gomarkdoc` and `golang.org/x/pkgsite`. They are there only
because `tools/tools.go` imports them under a `tools` build tag. Together they
pull about 55 `// indirect` modules into the root module (go-git v5.3.0, viper,
cobra, logrus, x/crypto, …). Every consumer's module graph then includes them.
Dependency scanners flag the old go-git versions in consumers, and release notes
waste bullets on tool bumps ("Bump … x/pkgsite" in v1.26.5). The pin also does
nothing: `scripts/gen-docs.sh` runs whatever `gomarkdoc` is on `PATH`, not the
pinned version. The repo already solved this for the linter with a separate
`tools/lint` module. After this plan, doc tools are pinned in
`tools/docs/go.mod`, built into `.bin/` like the linter, and the root `go.mod`
lists only real library dependencies. The generated `docs/api/API.md` also stops
including example programs and the internal emoji generator.

## Current state

`go.mod` (root), lines 5-12:

```
require (
	github.com/go-text/typesetting v0.3.5
	github.com/princjef/gomarkdoc v1.1.0
	github.com/rivo/uniseg v0.4.7
	golang.org/x/image v0.48.0
	golang.org/x/pkgsite v0.6.0
	golang.org/x/text v0.44.0
)
```

followed by a second `require (...)` block of about 55 `// indirect` lines.

`tools/tools.go`:

```go
//go:build tools

// Package tools pins documentation tooling dependencies so
// `go mod tidy` keeps them in go.sum.
package tools

import (
	_ "github.com/princjef/gomarkdoc/cmd/gomarkdoc"
	_ "golang.org/x/pkgsite/cmd/pkgsite"
)
```

`scripts/gen-docs.sh` (called by `//go:generate bash scripts/gen-docs.sh` in
`generate.go:1`):

```bash
gomarkdoc \
  --output "$OUT_DIR/API.md" \
  ./...
```

Today `docs/api/API.md` has these top-level sections (`grep -n '^# '`): `glyph`,
`accessibility`, `ime`, `ebitengine`, `gpu`, `demo`, `demo\_gpu`,
`showcase\_gpu`, `showcase\_sections`, `genemoji`. The last five are example
programs and an internal generator, not public API.

**Exemplar to copy** — the linter module. `tools/lint/go.mod`:

```
// Separate module so the linter's dependency tree stays out of the public
// go-glyph module and out of every module that imports it. The pinned
// golangci-lint version lives here and nowhere else: `make lint` and CI
// both build the linter from this file.
module github.com/go-gui-org/go-glyph/tools/lint

go 1.26.0

tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint
```

`Makefile` builds it into the git-ignored `.bin/`:

```make
LINT_DIR = $(CURDIR)/.bin
LINT_BIN = $(LINT_DIR)/golangci-lint
...
$(LINT_BIN): tools/lint/go.mod tools/lint/go.sum
	GOWORK=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS= GOBIN=$(LINT_DIR) \
	  go -C tools/lint install \
	  github.com/golangci/golangci-lint/v2/cmd/golangci-lint
```

and the doc targets:

```make
## docs: generate docs/api/API.md via gomarkdoc
docs:
	go generate ./...

## docs-serve: browse package docs locally via pkgsite
docs-serve:
	pkgsite -open .
```

**Warning:** `go generate ./...` also runs `go run ./internal/genemoji`
(`generate.go:2`), which **downloads Unicode emoji data over the network** and
rewrites `emoji_table.go`. Do not run `go generate ./...` in this plan. Run
`bash scripts/gen-docs.sh` (or the new Make target) instead.

Docs that mention the tools: `AGENTS.md:31` (`pkgsite -open .`), `AGENTS.md:134`
(`tools/` — Tool dependency tracking (for gomarkdoc)), `AGENTS.md:144`,
`AGENTS.md:173` (lists gomarkdoc as a key dependency).

## Commands you will need

| Purpose          | Command                                               | Expected on success                       |
| ---------------- | ----------------------------------------------------- | ----------------------------------------- |
| Root deps        | see Step 5 (a `go list -m all` piped to `grep -c -E`) | `0` after the change                      |
| Tidy check       | `go mod tidy -diff`                                   | no output, exit 0                         |
| Build doc tool   | `make doc-tools`                                      | `.bin/gomarkdoc` and `.bin/pkgsite` exist |
| Generate API doc | `make api-doc`                                        | exit 0, `docs/api/API.md` rewritten       |
| Tests            | `go test ./...`                                       | all `ok`                                  |
| Lint             | `make lint`                                           | `0 issues.`                               |
| Full gate        | `make prepush`                                        | exit 0                                    |

## Scope

**In scope**:

- `tools/docs/go.mod`, `tools/docs/go.sum` (create)
- `tools/tools.go` (delete)
- root `go.mod`, `go.sum` (only via `go mod tidy`)
- `scripts/gen-docs.sh`
- `Makefile` — new `doc-tools` and `api-doc` targets. `docs` and `docs-serve`
  use the pinned binaries.
- `docs/api/API.md` (regenerated)
- `AGENTS.md` — only the lines that mention gomarkdoc, pkgsite and `tools/`
- `CHANGELOG.md` — one bullet under `## [Unreleased]` → `### Changed`

**Out of scope**:

- `emoji_table.go` and `internal/genemoji`. Do not regenerate them.
- `tools/lint/*`.
- `.github/workflows/ci.yml`. CI does not run doc generation today. A
  drift-check job is a possible follow-up, not part of this plan.
- Other stale content in `AGENTS.md` (for example its "go.mod Gotcha" section).
  A separate docs plan covers it.
- Example modules' `go.mod` files.

## Git workflow

- Branch: `advisor/007-doc-tools-module`
- One commit, e.g. `build: move gomarkdoc and pkgsite into a tools/docs module`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Record the baseline

Run and save the output for your summary:

- `go list -m all | wc -l` (number of modules in the root graph)
- `go list -m all | grep -c -E 'gomarkdoc|pkgsite|go-git'` (expect > 0)
- `grep -n '^# ' docs/api/API.md`

**Verify**: commands run, numbers recorded.

### Step 2: Create `tools/docs`

1. Create `tools/docs/go.mod` with the same header comment as
   `tools/lint/go.mod`, reworded for doc tools:
   `module github.com/go-gui-org/go-glyph/tools/docs`, `go 1.26.0`.
2. From inside `tools/docs`, run:
   `GOWORK=off go -C tools/docs get -tool github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0`
   and
   `GOWORK=off go -C tools/docs get -tool golang.org/x/pkgsite/cmd/pkgsite@v0.6.0`
   These are the same versions the root pins today, so docs output does not
   change because of a tool bump.
3. `GOWORK=off go -C tools/docs mod tidy`.

**Verify**: `grep -n '^tool ' tools/docs/go.mod` → 2 lines (gomarkdoc, pkgsite).
`GOWORK=off go -C tools/docs build ./...` may print "no packages" (the module
has none), which is fine. `GOWORK=off go -C tools/docs list tool` lists both
tools.

### Step 3: Makefile targets

Add, next to the lint variables and following their style and comments:

```make
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

## api-doc: regenerate docs/api/API.md only (no network, unlike `docs`,
## whose go generate also refreshes the emoji table from unicode.org).
api-doc: $(DOC_BIN_GOMARKDOC)
	GOMARKDOC=$(DOC_BIN_GOMARKDOC) bash scripts/gen-docs.sh
```

Change `docs-serve` to `$(DOC_BIN_PKGSITE) -open .` with the prerequisite
`$(DOC_BIN_PKGSITE)`. Make `docs` depend on `doc-tools` and export
`GOMARKDOC=$(DOC_BIN_GOMARKDOC)` for its `go generate ./...`. Add `doc-tools`
and `api-doc` to `.PHONY`.

**Verify**: `make doc-tools` → exit 0, and `ls .bin/gomarkdoc .bin/pkgsite`
lists both. `git status --short .bin` prints nothing (it is git-ignored).

### Step 4: Script uses the pinned binary and only public packages

Edit `scripts/gen-docs.sh`:

- `GOMARKDOC="${GOMARKDOC:-$REPO_ROOT/.bin/gomarkdoc}"`. If that file is not
  executable, print `gen-docs: $GOMARKDOC missing; run 'make doc-tools'` to
  stderr and `exit 1`.
- Replace `./...` with the explicit public package list:
  `. ./accessibility ./ime ./backend/ebitengine ./backend/gpu`. Add a comment
  that examples and `internal/` are deliberately excluded.

Run `make api-doc`.

**Verify**: `grep -n '^# ' docs/api/API.md` → exactly `glyph`, `accessibility`,
`ime`, `ebitengine`, `gpu`. Then `git diff --stat docs/api/API.md` shows only
deletions, or almost only deletions. Open the diff: the removed lines must all
belong to the `demo`, `demo\_gpu`, `showcase\_gpu`, `showcase\_sections` and
`genemoji` sections.

### Step 5: Drop the tools from the root module

1. `git rm tools/tools.go`
2. `GOWORK=off go mod tidy`

**Verify**:

- `grep -nE 'gomarkdoc|pkgsite' go.mod` → nothing
- `go list -m all | grep -c -E 'gomarkdoc|pkgsite|go-git'` → `0`
- `go list -m all | wc -l` → much smaller than the Step 1 baseline (record both
  numbers)
- `go mod tidy -diff` → no output
- `go build ./... && go test ./...` → all `ok`

### Step 6: Docs and changelog

- `AGENTS.md`: change `pkgsite -open .` to `make docs-serve`. Change the
  `tools/` line to say `tools/lint` and `tools/docs` are separate modules that
  pin the linter and the doc generators. Line 144: say `gen-docs.sh` uses
  `.bin/gomarkdoc` (built by `make doc-tools`). Line 173: remove gomarkdoc from
  the key library dependencies, or move it under a "Tooling (separate modules)"
  note. Run `prettier --prose-wrap always --print-width 80 --write AGENTS.md` if
  prettier is installed. If it is not, keep lines at 80 columns or fewer by
  hand.
- `CHANGELOG.md` under `## [Unreleased]` → `### Changed`: "Doc tools (gomarkdoc,
  pkgsite) moved to a separate `tools/docs` module. go-glyph's `go.mod` no
  longer brings about 55 tool-only modules (go-git, viper, cobra, …) into
  consumers' module graphs. `docs/api/API.md` no longer documents example
  programs."

**Verify**: `make prepush` → exit 0.

## Test plan

No Go behavior changes, so there are no new unit tests. The verification is the
module-graph check (Step 5), the regenerated API doc (Step 4), and the full
`make prepush` gate.

## Done criteria

- [ ] `test -f tools/tools.go` fails (file deleted)
- [ ] `grep -c '^tool ' tools/docs/go.mod` prints `2`
- [ ] `grep -nE 'gomarkdoc|pkgsite' go.mod` prints nothing
- [ ] `go list -m all | grep -c -E 'gomarkdoc|pkgsite|go-git'` prints `0`
- [ ] `grep -n '^# ' docs/api/API.md` prints exactly 5 headings
- [ ] `git diff --stat 7234b0b -- emoji_table.go` prints nothing
- [ ] `make prepush` exits 0
- [ ] `plans/README.md` status row updated

## STOP conditions

- `go list -m all` still shows go-git after Step 5. Some other root dependency
  pulls it in. Report `go mod why -m github.com/go-git/go-git/v5`.
- `go mod tidy` removes or changes any non-indirect requirement other than
  gomarkdoc and pkgsite.
- The regenerated `API.md` differs inside the `glyph`, `accessibility`, `ime`,
  `ebitengine` or `gpu` sections. That means the gomarkdoc build or flags differ
  from what generated the committed file. Report the first differing hunk and do
  not commit the regenerated doc.
- `gomarkdoc` cannot document `./backend/ebitengine` (a separate module) by
  path. Report the error. Do not drop the section silently.

## Maintenance notes

- Tool upgrades now happen in `tools/docs` (`go -C tools/docs get -tool …@vX`).
  They never touch the library's `go.mod`, and they need no consumer-facing
  release note.
- `.bin/` is shared by the linter and doc tools. It is git-ignored.
- Follow-up (not done here): a CI job that runs `make api-doc` and
  `git diff --exit-code docs/api/API.md`, so the generated docs cannot drift.
  Pinning the tool first is what makes that check reliable.
