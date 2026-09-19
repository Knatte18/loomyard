MILL_REVIEW_BEGIN
# Review: reed: born-as-strand for loom start's operator attach

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-assessed; brief names "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Cold-bootstrap focus promise fails in a VS Code session
**Section:** `display-below-parent-focused-no-shrink` / `focus-on-re-attach-follows-the-active-pane`
**Issue:** `focusTarget` (`internal/reedengine/render/focus.go`) returns the bottom-most strand carrying `Display.Focus` *before* falling back to bottom-most-overall, and `internal/vscode/config.go` line 101 adds the `claude` strand with `--focus`, so in any worktree opened through the folderOpen chain — the entry path this document itself calls the one used in practice — a persisted `Focus: true` strand always wins and the operator never lands in their own pane, even on a first `loom start`.
**Fix:** State the disposition for a session that already holds a `Focus: true` strand — accept it as a third case alongside cold/warm, or change the focus decision — and adjust the Tier 1 `orderStack`/`focusTarget` test matrix, which today pins only a `[loom-status, operator]` stack with no `Focus` flag set anywhere.

### [BLOCKING:consistency] "testing.Testing() in every Go test" is false for loomcli smoke
**Section:** Testing, the `--no-attach` bullet
**Issue:** The claim that no Go test can observe a real watchdog spawn rests on `suppressWatchdogSpawn == testing.Testing()`, but `internal/loomcli`'s smoke tier drives a **built** binary out-of-process (`buildLyxBinary` → `go build ./cmd/lyx`, then `runLoomCLINoFatal(exe, worktree, …, "loom", "start")` at ten sites in `smoke_test.go`), where `testing.Testing()` is false — so the newly added spawn fires live in those runs, detached, with `cmd.Dir` pinned to a throwaway fixture hub the test then tears down.
**Fix:** Say what the smoke tier does about the real daemon (env gate, teardown reap, or accepted as-is with the reason), and correct the premise sentence so the plan writer does not size the test plan on a false "unobservable" claim.

### [NIT:decision] Operator strand's pinned name literal is never stated
**Section:** `idempotent-via-ifabsent`, Scope "In"
**Issue:** The name is only ever "a pinned constant beside `statusStrandDisplayName`"; its literal value is never given, yet it is the `IfAbsent` match key, is operator-visible, and must stay stable across versions.
**Fix:** Name the constant and its string value, as `statusStrandDisplayName = "loom-status"` already is in `bootstrap.go`.

### [NIT:design] Watchdog spawn sits under the held bootstrap lock, unaddressed
**Section:** `watchdog-parity`, Decision (gating)
**Issue:** The chosen call site (immediately after `c.ensureStatusStrand()`) is inside the region where `bootstrapLock` is still held — the document argues the lock-release point is load-bearing for the operator strand but says nothing about holding it across `MkdirAll` + `os.Executable` + a detached `Start`.
**Fix:** One sentence stating the lock is deliberately held across the best-effort spawn and why that is acceptable.

## Verdict

REQUEST_CHANGES
Focus promise breaks under the VS Code entry path; a testing premise is contradicted by source.
MILL_REVIEW_END
