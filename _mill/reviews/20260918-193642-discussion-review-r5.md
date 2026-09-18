MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
duration_s: 173.5
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-assessment; harness reports "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:decision] Third AddStrand caller undispositioned
**Demoted-from:** BLOCKING
**Section:** Scope (In/Out), Technical context (`shuttleengine/reed.go:19`)
**Issue:** `internal/shuttleengine/run.go:252` (`Runner.Start`) calls `AddStrand` on every Go-launched agent spawn — loom, webster, burler, and the standalone detached runner — so the self-heal changes behaviour for a third entrypoint the discussion only ever mentions as a signature-compatibility note ("no seam breaks"); a shuttle run in a cold worktree now spawns a tmux server instead of failing fast.
**Fix:** State the disposition explicitly (intended, since a spawn into a fresh worktree is the headline case) and say whether the standalone/detached runner path is included, so a plan writer does not narrow the change to the two CLI verbs.

### [NIT:consistency] Superseded "Status() → Up() swap" line
**Demoted-from:** BLOCKING
**Section:** Technical context, `internal/reedcli/attach.go:55` bullet
**Issue:** The bullet still reads "the `Status()` → `Up()` swap", contradicting Scope line 28 and the `attach-preflight-keeps-its-status-call` decision, which keep `Status()` and add `EnsureSession()` ahead of it.
**Fix:** Rewrite the bullet as "`EnsureSession()` inserted ahead of the retained `Status()` call" and keep the header-comment note.

### [NIT:scope] reed API design doc missing from the doc list
**Demoted-from:** BLOCKING
**Section:** Scope → doc updates
**Issue:** `manifest/designs/reed-fabric-standalone-api.md` pins `*Engine` at exactly **17** exported methods, enumerated by name (`:124`, restated `:130`, `:169`); adding `EnsureSession()` makes that 18, and the doc is absent from the same-commit doc list.
**Fix:** Add that file to the doc-update list with the count and the method enumeration named as the specific edit.

### [NIT:scope] Zero-pane husk test has no stated construction
**Section:** Testing → "Zero-pane husk is still repaired"
**Issue:** No existing test or fixture in the repo builds a zero-pane session (`lifecycle.go:229-247` reaches it only via a layout-reaped pane), and `listPanes` errors rather than returning empty if tmux cannot list, so the recipe may not be constructible as written.
**Fix:** Name the construction method (or mark the test as best-effort, since the repair path itself is pre-existing and not changed by this task).

## Verdict

APPROVE
Seam design is sound; three gaps — shuttle caller disposition, a stale line, a missing doc.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
