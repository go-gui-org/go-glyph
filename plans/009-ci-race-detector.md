# Plan 009: CI runs the race detector, and a test shapes one shared font from two Contexts at once

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 682109e..HEAD -- .github/workflows/ci.yml Makefile` If any
> in-scope file changed since this plan was written, compare the "Current state"
> excerpts against the live code before proceeding; on a mismatch, treat it as a
> STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: 015 (shared-face shaping race fix). Must be DONE first.
- **Category**: tests
- **Planned at**: commit `6d6a018`, 2026-11-13; refreshed at `682109e`
- **Revised**: 2026-11-13. The first run of this plan found 4 data races in
  shaping. Plan 015 fixes them and adds the test
  `TestConcurrentContextsShareFaces` (in
  `concurrent_contexts_puregoft_test.go`). This plan now only checks that the
  test exists and passes, then adds the CI job. Do not create or edit the test
  file.

## Why this matters

go-glyph keeps process-wide shared state: a parsed-face LRU (`faceCache`), a
coverage cache, a background idle sweeper and pooled HarfBuzz fonts. Every
`Context` in the process shares them. `make prepush` runs `go test -race`
locally, but CI never does, so a data race introduced in a PR from a machine
that skipped `prepush` reaches `main` unnoticed. Separately, no test shapes text
through the same shared face from two `Context`s on two goroutines. That is the
pattern a multi-window app uses (go-gui creates one `TextSystem` per window).
After this plan, CI has a `test-race` job, and a test proves concurrent shaping
across Contexts is race-free, or shows that it is not.

## Current state

- `.github/workflows/ci.yml` — jobs `test`, `lint`, `test-wasm`, `test-ios`,
  `test-android`, `test-macos`, `test-windows`. None passes `-race`. The `test`
  job (lines 22-91) is the exemplar to copy for setup:

```yaml
test:
  runs-on: ubuntu-latest

  steps:
    - name: Checkout
      uses: actions/checkout@v6

    - name: Set up Go
      uses: actions/setup-go@v6
      with:
        go-version-file: go.mod
        cache-dependency-path: |
          go.sum
          examples/demo/go.sum
          examples/demo_gpu/go.sum

          examples/showcase_gpu/go.sum

    - name: Install native dependencies
      run: |
        sudo apt-get update
        sudo apt-get install -y \
          libgl1-mesa-dev \
          libsdl2-dev \
          fonts-noto-cjk \
          fonts-noto-color-emoji \
          pkg-config

    - name: Test root module
      env:
        CGO_ENABLED: "1"
      run: go test -coverprofile=coverage.out ./...
```

- `Makefile:50-51` — the local target:

```make
test-race:
	go test -race ./...
```

- `freetype_types_puregoft.go`: every Context shares one cached face per font
  file. Plan 015 (merged as #173, commit `682109e`) fixed the shaping races on
  it. This plan does not touch that file.

- `TextSystem` is documented "Not safe for concurrent use" (`glyph.go:23-24`).
  That covers one `TextSystem`. Two separate `Context`s on two goroutines are
  the supported multi-window pattern, and that is what the new test exercises.

- Test exemplar for creating a Context: `bench_terminal_frame_test.go:26-30`:

```go
	ctx, err := NewContext(1.0)
	if err != nil {
		b.Fatal(err)
	}
	defer ctx.Free()
```

## Commands you will need

| Purpose   | Command                                                       | Expected on success     |
| --------- | ------------------------------------------------------------- | ----------------------- |
| New test  | `go test -race -run TestConcurrentContextsShare -count=3 .`   | `ok`, no `DATA RACE`    |
| All, race | `make test-race`                                              | all `ok`                |
| YAML      | `ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml")'` | exit 0 (if ruby exists) |
| Full gate | `make prepush`                                                | exit 0                  |

## Scope

**In scope**:

- `.github/workflows/ci.yml` — add one job, `test-race`

`concurrent_contexts_puregoft_test.go` already exists (plan 015). It is out of
scope here: run it, do not edit it.

**Out of scope**:

- Any change to non-test Go code. If the new test finds a race, do NOT fix it in
  this plan. See the STOP conditions.
- The other CI jobs. Do not add `-race` to the macOS or Windows jobs. macOS
  already needs `GOMEMLIMIT=2GiB -p 1` to avoid running out of memory, and race
  mode uses 5–10× more memory.
- `Makefile`.

## Git workflow

- Branch: `advisor/009-ci-race-detector`
- One commit, e.g.
  `ci: run the race detector; test concurrent shaping across Contexts`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Confirm the concurrent-shaping test from plan 015

Do not write a test. Plan 015 added `TestConcurrentContextsShareFaces` in
`concurrent_contexts_puregoft_test.go`. It runs 4 Contexts on 4 goroutines over
mixed scripts at two sizes.

**Verify**:
`grep -n 'func TestConcurrentContextsShareFaces' concurrent_contexts_puregoft_test.go`
prints 1 line. If it prints nothing, plan 015 has not landed: STOP.
`go test -race -run TestConcurrentContextsShareFaces -count=3 .` → `ok`, and no
`WARNING: DATA RACE` in the output. If a race is reported, go to the STOP
conditions.

### Step 2: CI job

Add a `test-race` job to `.github/workflows/ci.yml`, after the `test` job. Copy
the `test` job's `runs-on`, Checkout, Set up Go (with
`cache-dependency-path: go.sum` only) and "Install native dependencies" steps.
Then add one step:

```yaml
# The race detector needs cgo. Root module only: the examples and
# backend/ebitengine are build-only in CI, and backend tests need a
# display. Kept out of the macOS/Windows jobs because race mode costs
# 5-10x memory and macOS already runs with GOMEMLIMIT and -p 1.
- name: Test root module with the race detector
  env:
    CGO_ENABLED: "1"
  run: go test -race ./...
```

Match the file's indentation (2 spaces) and the comment style of the existing
jobs.

**Verify**: if `ruby` exists,
`ruby -ryaml -e 'puts YAML.load_file(".github/workflows/ci.yml")["jobs"].keys'`
lists `test-race`. If not, use
`python3 -c 'import yaml,sys;print(list(yaml.safe_load(open(".github/workflows/ci.yml"))["jobs"]))'`.
If neither works, say so in the report.

### Step 3: Gate

**Verify**: `make test-race` → all `ok`. `make prepush` → exit 0.

## Test plan

- `TestConcurrentContextsShareFaces`: 4 goroutines × 50 layouts over mixed
  scripts and two sizes. It must pass under `-race -count=3`, and widths must
  match the sequential result.
- CI: the new `test-race` job runs the whole root suite under `-race` on Linux.
  The reviewer confirms it on the PR.

## Done criteria

- [ ] `go test -race -run TestConcurrentContextsShareFaces -count=3 .` → `ok`,
      and `grep -c 'DATA RACE'` on its output prints `0`
- [ ] `grep -n 'go test -race' .github/workflows/ci.yml` prints 1 line
- [ ] `make prepush` exits 0
- [ ] Only `.github/workflows/ci.yml` changed (`git status`, plus
      `plans/README.md`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- The race detector reports a race in Step 1 or Step 3. Do not fix it. Report:
  the two stack traces (first 30 lines of each), and which shared object they
  touch (for example `cachedFace.face`, `hbPool`, `coverageCache`). Commit
  nothing. The advisor will write a separate fix plan. This is a valid outcome
  of this plan, not a failure.
- `make test-race` takes over 10 minutes locally. Report the time. CI may need a
  `-timeout` flag or a narrower package list.
- The test is flaky (it passes and fails across `-count=3` runs without a race
  report). Report the failing assertion.

## Maintenance notes

- A future change to the face cache, `hbPool` or the coverage cache must keep
  `TestConcurrentContextsShareFaces` passing under `-race`.
- If the CI race job grows too slow, run it with `-short` and mark the heavy
  sweeps `testing.Short()`-skippable, instead of dropping the job.
