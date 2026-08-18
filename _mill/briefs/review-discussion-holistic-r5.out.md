MILL_REVIEW_BEGIN
# Review: the standalone CLI path

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus (self-assessed; runtime reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] Byte-identity list omits webster's changed classification
**Section:** "hub mode is byte-identical" + mode-trigger part 2 scope amendment
**Issue:** The enumeration claims exactly three deviations and that a fourth is "a bug in this plan", but the `webstercli` repoint changes webster's own observable behaviour in a wired-worktree subdirectory (today: silent standalone via `HubPresent` false; after: refusal) — verified at `internal/webstercli/cli.go:127`, which currently discards the `Resolve` error class entirely.
**Fix:** Name webster's reclassification as a fourth, deliberate deviation in that decision, so the completeness claim stays true.

### [NIT:scope] "one line" understates the `webstercli/cli.go` edit
**Section:** Technical context, files table
**Issue:** `ResolveMode` returns `(loc, mode, error)`, so `resolvePersistentPreRun` gains a refuse branch (surface verbatim + `clihelp.Abort`) beyond the probe swap, and `cli.go:4`/`:103` plus `wiring.go`'s doc comments name `HubPresent`/`hubPresent` and go stale.
**Fix:** Reword the row to "probe swap + refuse branch + doc-comment repoint" so the plan writer does not size it as a token substitution.

### [NIT:consistency] "the design forbids importing fabricengine into either CLI"
**Section:** mode-trigger part 1, final Rejected bullet
**Issue:** Stated without qualifier, yet both CLIs must keep the import in hub mode — `internal/burlercli/cli.go:16,107` (`fabricengine.StencilsDir`) and `internal/perchcli/run.go:301,327,334,344` (`StencilsDir`, `EnvSyncOptions`, `ScopedPathspec`, `Open`).
**Fix:** Scope the sentence to "importing it to reach `Ready`", so no implementer reads it as a ban on the retained hub-mode uses.

### [NIT:decision] perch `pause`'s envelope has no stated disposition
**Section:** "operator visibility — the run envelopes name the directories"
**Issue:** The decision covers "both run verbs" only; `internal/perchcli/pause.go:110` emits its own success envelope and, in standalone, writes under `<state>` after the same `wireStandalone` (including the stencils `Reconcile`), so whether it gains `mode`/`stateDir`/`stencilsDir` is unstated.
**Fix:** State the disposition explicitly — `pause` already reports an absolute `pauseFile`, so "deliberately unchanged" is a fine answer, but it should be written down.

## Verdict

APPROVE
Decisions are complete and source-grounded; four recorded NITs, none blocking plan writing.
MILL_REVIEW_END
