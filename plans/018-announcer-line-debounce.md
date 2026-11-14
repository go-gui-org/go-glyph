# Plan 018: A line change that arrives during the debounce is announced later, not lost

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the next
> step. If anything in the "STOP conditions" section occurs, stop and report —
> do not improvise. When done, update the status row for this plan in
> `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**:
> `git diff --stat 4154f31..HEAD -- accessibility/announcer.go accessibility/accessibility_test.go`
> If any in-scope file changed since this plan was written, compare the "Current
> state" excerpts against the live code before proceeding; on a mismatch, treat
> it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug (accessibility)
- **Planned at**: commit `4154f31`, 2026-11-14

## Why this matters

`Announcer` tells a screen reader what the caret did. It drops any announcement
that comes within 150 ms of the previous one. `AnnounceLineNumber` records the
new line **before** that check. So when a line change arrives during the
debounce (for example, Down arrow right after a character was announced), the
message is dropped but the line is still marked as announced. The next call for
the same line returns `""`. The user never hears "line N" for that line.

After this plan, `lastLine` changes only when "line N" is actually posted. A
debounced line change is announced on the next call after the debounce.

No go-gui code calls `AnnounceLineNumber` today, so the impact is low. The fix
is 2 lines.

## Current state

`accessibility/announcer.go:108-119`:

```go
// AnnounceLineNumber announces "line N" on line change.
func (a *Announcer) AnnounceLineNumber(line int) string {
	if line == a.lastLine {
		return ""
	}
	a.lastLine = line
	if !a.shouldAnnounce() {
		return ""
	}
	msg := fmt.Sprintf("line %d", line)
	a.post(msg)
	return msg
}
```

`shouldAnnounce` (same file, near the end) reads `time.Now()`. It returns
`false` when less than `debounceMs` (150) has passed since
`lastAnnouncementTime`. Otherwise it sets `lastAnnouncementTime` to now and
returns `true`.

The test pattern, `accessibility/accessibility_test.go:85` and later:

```go
func TestAnnouncerLineNumber(t *testing.T) {
	a := NewAnnouncer()
	got := a.AnnounceLineNumber(5)
	...
	a.lastAnnouncementTime = 0
	got = a.AnnounceLineNumber(5)
```

Tests are in package `accessibility`, so they can set `lastAnnouncementTime`
directly. Setting it to `time.Now().UnixMilli()` forces the next call to be
debounced. Setting it to `0` clears the debounce.

## Commands you will need

| Purpose   | Command                                                                      | Expected on success |
| --------- | ---------------------------------------------------------------------------- | ------------------- |
| New test  | `go test -run TestAnnouncerLineNumberAfterDebounce -count=1 ./accessibility` | `ok`                |
| Package   | `go test -count=1 ./accessibility`                                           | `ok`                |
| Full gate | `make prepush`                                                               | exit 0              |

## Scope

**In scope**:

- `accessibility/announcer.go`: `AnnounceLineNumber` only.
- `accessibility/accessibility_test.go`: add one test.
- `CHANGELOG.md`: one bullet under `## [Unreleased]` → `### Fixed`.

**Out of scope**:

- The debounce for the other `Announce*` methods. They drop the message and keep
  no state, which is the intended behavior.
- `AnnounceDeadKeyResult`, which skips the debounce on purpose (the composed
  character must always be read).
- Adding a clock interface. Setting `lastAnnouncementTime` in the test is
  enough.

## Git workflow

- Branch: `advisor/018-announcer-line-debounce`
- One commit, e.g.
  `fix(accessibility): announce a line change that arrived during the debounce`
- Do NOT push or open a PR unless instructed.

## Steps

### Step 1: Write the failing test

In `accessibility/accessibility_test.go`, after `TestAnnouncerLineNumber`, add:

```go
// A line change that arrives during the debounce is dropped, but it must not be
// marked as announced. The next call for the same line, after the debounce,
// announces it.
func TestAnnouncerLineNumberAfterDebounce(t *testing.T) {
	a := NewAnnouncer()
	a.lastAnnouncementTime = time.Now().UnixMilli() // inside the debounce
	if got := a.AnnounceLineNumber(3); got != "" {
		t.Fatalf("debounced call = %q, want empty", got)
	}
	a.lastAnnouncementTime = 0 // debounce over
	if got := a.AnnounceLineNumber(3); got != "line 3" {
		t.Errorf("after debounce = %q, want %q", got, "line 3")
	}
	a.lastAnnouncementTime = 0
	if got := a.AnnounceLineNumber(3); got != "" {
		t.Errorf("same line again = %q, want empty", got)
	}
}
```

Add `"time"` to the test file's imports if it is not there.

**Verify**:
`go test -run TestAnnouncerLineNumberAfterDebounce -count=1 ./accessibility` →
`FAIL`, with `after debounce = "", want "line 3"`.

### Step 2: Record the line only when it is announced

In `AnnounceLineNumber`, move `a.lastLine = line` to after the `shouldAnnounce`
check:

```go
// AnnounceLineNumber announces "line N" on line change. The line is
// recorded only when the message is posted, so a change that arrives
// during the debounce is announced on a later call.
func (a *Announcer) AnnounceLineNumber(line int) string {
	if line == a.lastLine || !a.shouldAnnounce() {
		return ""
	}
	a.lastLine = line
	msg := fmt.Sprintf("line %d", line)
	a.post(msg)
	return msg
}
```

The `line == a.lastLine` check must stay first. `shouldAnnounce` sets
`lastAnnouncementTime`, so calling it for an unchanged line would start a new
debounce window for nothing. `||` short-circuits, so it is not called then.

**Verify**: `go test -count=1 ./accessibility` → `ok`.

### Step 3: Changelog and gate

Add under `## [Unreleased]` → `### Fixed` (match the nearby bullets, wrap at 80
columns):

"**Screen readers hear a line change that came during the debounce.**
`Announcer.AnnounceLineNumber` marked a line as announced even when the debounce
dropped the message, so that line was never read. It now marks the line only
when it posts the message."

**Verify**: `make prepush` → exit 0.

## Test plan

- `TestAnnouncerLineNumberAfterDebounce` (new). It fails before Step 2 and
  passes after. It also checks that an unchanged line is still not repeated.
- `TestAnnouncerLineNumber` (existing) keeps passing.

## Done criteria

- [ ] `go test -run 'TestAnnouncerLineNumber' -count=1 -v ./accessibility 2>&1 | grep -c '^--- PASS'`
      prints `2`
- [ ] `go test -count=1 ./accessibility` → `ok`
- [ ] `make prepush` exits 0
- [ ] `git status` shows changes only in `accessibility/announcer.go`,
      `accessibility/accessibility_test.go`, `CHANGELOG.md` (and
      `plans/README.md`)

## STOP conditions

- Step 1's test passes before the fix. Report the output.
- `TestAnnouncerLineNumber` fails after Step 2. Its expectations may depend on
  the old order. Report the failure. Do not change that test.

## Maintenance notes

- Any new `Announce*` method that keeps "last announced" state must update that
  state only after `shouldAnnounce` returns `true`.
