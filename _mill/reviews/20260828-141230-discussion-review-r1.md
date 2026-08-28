# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:consistency] New reap logging is "discovered" but never becomes a Decision, a Scope item, or a test

**Demoted-from:** BLOCKING
**Section:** Constraints → "Discovered during discussion" (bottom bullet: "logger calls in reed carry `"socket"` and `"session"` keys; a new reap log line should follow that shape"), cross-referenced against Constraints → Live-Substrate Spawn Observability, and against Scope → In.

**Issue:** `reconcileLocked`'s `kill-pane` loop in `internal/reedengine/reconcile.go` currently has zero `logger` calls — confirmed by inspecting the file: the only `logger.*` call in this pair of files is `spawn.go:215`'s unrelated `clearConflictingPaneBindings` warning. This task substantially widens when and how often that kill-pane loop fires (the whole point of `reap-gate-accepts-the-header-as-survival-anchor` is to make the untracked reap fire on the zero-strand precondition M16/M22 share, which today it never does). The discussion itself notices this is worth a log line ("Discovered during discussion" footnote), but that observation never graduates into an actual Decision, never appears in the Scope → In file list (`reconcile.go` is listed there only for the gate change itself), and Testing has no case asserting a log line is emitted on reap. A footnote that says "should follow that shape" with nothing committing it to the plan is exactly the kind of item mill-plan will silently drop, since Scope → In is what plan-writing actually reads for its file list.

**Suggested fix:** Either (a) add a Decision that explicitly adds `logger.Info`/`logger.Debug` (per the Spawn Observability invariant's lifecycle-vs-probe split — this is a lifecycle reap, so `Info`) around the untracked-pane kill-pane calls in `reconcileLocked`, carrying `"socket"`/`"session"` and the killed pane ids, and list it in Scope → In; or (b) add an explicit Decision that kill-pane is teardown-of-a-pane rather than "starts a real OS process" and is therefore out of the Spawn Observability invariant's scope, with the reasoning spelled out the way every other Decision in this document is. Either resolves the dangling footnote — leaving it unresolved does not.

## Verdict

APPROVE
Every other technical claim in this discussion checked out exactly against source (line numbers, ordering, comments, and all seven cited CONSTRAINTS.md invariant names verified verbatim) and the three-decision structure genuinely addresses M16 and M22 as distinct root causes rather than conflating them, but the one BLOCKING item above must be resolved into an actual Decision/Scope commitment before this is plan-ready.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
