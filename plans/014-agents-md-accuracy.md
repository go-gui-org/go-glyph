# Plan 014: AGENTS.md describes the repo as it is (no replace directive, no cgo requirement, no purego)

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 6d6a018..HEAD -- AGENTS.md` If
> AGENTS.md changed since this plan was written, compare the "Current state"
> excerpts against the live file before proceeding; on a mismatch, treat it as a
> STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none. If plans 009 (CI race job) or 010 (`make lint-cross`)
  have landed, also describe them, as noted in Step 2.
- **Category**: docs
- **Planned at**: commit `6d6a018`, 2026-11-13

## Why this matters

`AGENTS.md` is the file coding agents read first. It tells them the root module
needs `CGO_ENABLED=1`, that `go.mod` has a `replace` directive to a fork of
go-text/typesetting, and that `ebitengine/purego` is a key dependency. None of
this is true. An agent that believes it adds cgo, preserves a `replace` that
does not exist, or reasons about the wrong font stack. After this plan, every
factual claim in `AGENTS.md` matches the repo, and the file is formatted like
the repo's other Markdown.

## Current state

Facts verified at `6d6a018`:

- Root `go.mod` has no `replace`. Its direct requirements are
  `github.com/go-text/typesetting v0.3.5`, `github.com/rivo/uniseg`,
  `golang.org/x/image` and `golang.org/x/text`.
- The root package builds and tests with `CGO_ENABLED=0`. The `Makefile` `nocgo`
  target and the CI step "Assert Linux library builds and tests without cgo"
  enforce this. cgo is needed only by `backend/gpu` (OpenGL/X11), the iOS build
  and some examples.
- `github.com/ebitengine/purego` appears only as an `// indirect` requirement of
  `backend/ebitengine/go.mod` and `examples/demo/go.mod` (through Ebiten). The
  root module does not use it.
- `*_puregoft.go` files (for example `context_puregoft.go`, tag
  `linux || darwin || windows`) are the native text stack built on the pure-Go
  `go-text/typesetting` library. They do not use FreeType.
- The full local gate is `make prepush` (`Makefile:118`: race tests, vet, lint,
  the cgo-free root gate, cross-builds, separate-module builds).
- CI (`.github/workflows/ci.yml`) has jobs: `test` (ubuntu), `lint` (ubuntu),
  `test-wasm` (ubuntu), `test-ios` (macos, simulator build and vet with
  `-tags ios`), `test-android` (ubuntu), `test-windows` (windows), `test-macos`
  (macos).

Stale lines in `AGENTS.md` (172 lines, most of them longer than 80 columns):

- Line 6: `# Build all packages (native; requires CGO_ENABLED=1)`
- Lines 43-45, the whole section:
  ```
  ## go.mod Gotcha

  The root `go.mod` has a `replace` directive redirecting `github.com/go-text/typesetting` to a forked `github.com/go-gui-org/typesetting`. Any dependency changes must respect this. New dependencies in the `go-gui-org` namespace should be aware of the existing fork.
  ```
- Line 151: ``- `*_puregoft.go` — Pure Go FreeType implementation``
- Line 162 (section "## CI"): "GitHub Actions tests on: Ubuntu (Linux + WASM +
  Android cross-compile), macOS, Windows. ..." It leaves out the lint and iOS
  jobs.
- Lines 168-169 (section "## Key Dependencies"):
  ```
  - `github.com/go-text/typesetting` (replaced with `github.com/go-gui-org/typesetting`) — text shaping
  - `github.com/ebitengine/purego` — C library loading without cgo
  ```
- The "Essential Commands" block (lines 5-41) does not mention `make prepush`.

Formatting convention: Markdown in this repo is formatted with
`prettier --prose-wrap always --print-width 80` (see `CHANGELOG.md` and
`plans/*.md`).

## Commands you will need

| Purpose | Command                                                                                     | Expected on success                          |
| ------- | ------------------------------------------------------------------------------------------- | -------------------------------------------- |
| Format  | `prettier --prose-wrap always --print-width 80 --write AGENTS.md`                           | exit 0                                       |
| Check   | `prettier --prose-wrap always --print-width 80 --check AGENTS.md`                           | "All matched files use Prettier code style!" |
| Stale   | `grep -nE 'replace directive\|forked\|purego\|CGO_ENABLED=1\)\|Pure Go FreeType' AGENTS.md` | nothing                                      |
| Gate    | `make prepush`                                                                              | exit 0                                       |

## Scope

**In scope**:

- `AGENTS.md` only

**Out of scope**:

- `README.md`, `CLAUDE.md`, `docs/architecture.txt`, `docs/plans/*`. Some of
  these also describe the old Pango/cgo design. That is a separate follow-up.
  Note it in the report. Do not edit them.
- The "Separate Modules" section's `replace` text. It is correct: the separate
  modules do have `replace github.com/go-gui-org/go-glyph => ../..`.
- Any Go code, `Makefile` or CI file.

## Git workflow

- Branch: `advisor/014-agents-md-accuracy`
- One commit, e.g. `docs(agents): drop stale replace, cgo and purego claims`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Fix the stale claims

Edit `AGENTS.md`. Use the simple-english skill for any new sentences, if it is
available.

1. Line 6: change the comment to
   `# Build all packages (the root package is pure Go; CGO_ENABLED=0 works)`.
2. Delete the whole "## go.mod Gotcha" section (heading and paragraph).
3. In "Essential Commands", add near the top:
   ```
   # Full local gate before pushing (race tests, vet, lint, cgo-free root,
   # cross-builds, separate modules)
   make prepush
   ```
4. Line 151: change to
   ``- `*_puregoft.go` — native text stack on the pure-Go go-text/typesetting library (no FreeType)``.
5. "## Key Dependencies": change the typesetting line to
   ``- `github.com/go-text/typesetting` — text shaping and font parsing``.
   Delete the `purego` line. Add
   ``- `golang.org/x/text` — Unicode text processing`` if `golang.org/x/text` is
   still a direct requirement in root `go.mod`.
6. "## CI": rewrite the first sentence to list the real jobs from "Current
   state". If `.github/workflows/ci.yml` now has a `test-race` job (plan 009) or
   runs `make lint-cross` (plan 010), mention them too. Check with
   `grep -n 'test-race:\|lint-cross' .github/workflows/ci.yml`.

**Verify**: the "Stale" grep from the commands table → no output.
`grep -n 'make prepush' AGENTS.md` → at least 1 line.

### Step 2: Check every remaining command and path

For each shell command inside a fenced code block in `AGENTS.md`, confirm that
it exists. Every `make <target>` must appear in `Makefile`
(`grep -n '^<target>:' Makefile`). For each file path mentioned (for example
`context_puregoft.go`, `renderer_common.go`, `internal/genemoji/`), confirm with
`ls` that it exists. List any that do not exist in the report, and fix them only
if the fix is clear from the repo. Otherwise leave them and report.

**Verify**: in the report, a list of each `make` target and path checked, with
✓/✗.

### Step 3: Format

Run `prettier --prose-wrap always --print-width 80 --write AGENTS.md`. Prettier
reflows the whole file. That is expected and wanted here. Then read
`git diff --stat AGENTS.md` and spot-check the code blocks: Prettier must not
change the content of fenced blocks.

**Verify**: the "Check" command → passes. `make prepush` → exit 0 (docs only,
but it is the repo gate).

## Test plan

Docs only, so there are no Go tests. Verification is the stale-claims grep, the
command and path audit in Step 2, and the Prettier check.

## Done criteria

- [ ] `grep -nE 'replace directive|forked|purego([^f]|$)|CGO_ENABLED=1\)|Pure Go FreeType' AGENTS.md`
      prints nothing (`purego([^f]|$)` skips the `*_puregoft.go` file names that
      Step 2 keeps)
- Note (reconcile 2026-11-14): #180 renamed `AGENTS.md` to `CLAUDE.md` with the
  same content. Run these checks against `CLAUDE.md` now.
- [ ] `grep -c 'go.mod Gotcha' AGENTS.md` prints `0`
- [ ] `grep -n 'make prepush' AGENTS.md` prints at least 1 line
- [ ] `prettier --prose-wrap always --print-width 80 --check AGENTS.md` passes
- [ ] Only `AGENTS.md` changed (`git status`, plus `plans/README.md`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Root `go.mod` does have a `replace` directive, or the root package no longer
  builds with `CGO_ENABLED=0`. The facts changed, so report them and do not
  delete the sections.
- Prettier is not installed. Report it, keep every edited line at 80 columns or
  fewer by hand, and skip the whole-file reflow.

## Maintenance notes

- When a `Makefile` target, CI job or direct dependency changes, update
  `AGENTS.md` in the same PR. Agents trust it.
- Follow-up (not here): `docs/architecture.txt` and `docs/plans/*` describe the
  removed Pango/cgo path. Either update them or mark them historical.
