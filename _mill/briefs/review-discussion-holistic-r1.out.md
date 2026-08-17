MILL_REVIEW_BEGIN
# Review: shuttleengine + reedengine + tokenvocab told-geometry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [BLOCKING:consistency] `AnchorRoot`/`anchorRoot` collides with the root-vs-cwd rule
**Section:** § Decisions → reedengine.Geometry / shuttleengine two strings; § Constraints (Cwd Resolution Invariant)
**Issue:** The invariant says "`root` always means the git worktree/repo root … never name a parameter, field, or local variable `root` for a value that is actually `cwd`", and `AnchorPath()` is exactly the value cwd is gated to equal; `manifest/designs/fabric-unified-view.md:61-63` repeats the rule ("never a stand-in name for whatever base path a module joins its own subdirectory onto") — and the same design doc's T4 spells the identical concept `anchorPath` (line 330-331) while T3 spells it `anchorRoot`. The discussion cites the Cwd Resolution Invariant only for the "no `os.Getwd` in `hubgeom`" half and never states a disposition on the name.
**Fix:** State a disposition — either justify `AnchorRoot`/`anchorRoot` against the naming rule explicitly, or pin `AnchorPath`/`anchorPath` (and align `WorktreeRoot` accordingly) before the field name is frozen into a struct T5-T8 extend.

### [BLOCKING:scope] Blast-radius enumeration covered `reedengine.New`, not `NewRunner`
**Section:** § Scope → In (test-file bullet); § Out (websterengine bullet)
**Issue:** The enumeration method found `reedengine.New`/`HubLogsDir` call sites but missed out-of-package `shuttleengine.NewRunner` sites: `internal/shuttlecli/cli_test.go:237`, `internal/websterengine/recoverbatch_test.go:189`, `internal/webstercli/verbs_test.go:227`. That also falsifies the stated boundary "this task touches `websterengine` in exactly two files" — `recoverbatch_test.go` is a third.
**Fix:** Re-run the enumeration symbol-by-symbol over all four changed exported symbols (`NewRunner`, `FindRun`, `New`, `HubLogsDir`) and restate the `websterengine` boundary as production-files-only.

### [NIT:consistency] Design-doc row 64 goes stale under the no-edit rule
**Section:** § Decisions → "Design doc is not rewritten, but T6/T7 get a breadcrumb"
**Issue:** `manifest/designs/producers-standalone.md:64` attributes `HubLogsDir` to `internal/reedengine/lifecycle.go`; the move to `fabricengine` makes that row false, and the decision forbids editing any line but the two T6/T7 pointers.
**Fix:** Either add row 64 to the permitted pointer edits or name it alongside the already-acknowledged "Known cost" as a second recorded staleness.

## Verdict

REQUEST_CHANGES
Naming disposition and one enumeration gap need resolving; the rest is sound.
MILL_REVIEW_END
