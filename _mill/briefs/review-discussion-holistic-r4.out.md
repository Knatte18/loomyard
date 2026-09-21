MILL_REVIEW_BEGIN
# Review: Spawned agent panes resolve the spawning lyx binary

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-21
```

## Findings

### [BLOCKING:design] Up/EnsureSession disposition unstated
**Section:** `one-dialect-per-pane-enforced-at-the-op-boundary`
**Issue:** `Up()` (`lifecycle.go:520`) and `EnsureSession()` (`:558`) create panes on `e.cfg.Shell` — `new-session … e.cfg.Shell` (`lifecycle.go:337`) and Selvage's split (`selvagepane.go:235`) — yet the refusal binds only `AddStrand`/`UpdateStrand`/`Resume`, and neither the still-working list (`Down`, `Status`, `AttachArgv`, transport) nor the refusal list names them; the r2 Q&A claim "no pane is ever created on an un-modelled shell" is therefore false as written.
**Fix:** State explicitly whether `Up`/`EnsureSession` refuse or are deliberately exempt, and correct the "no pane is ever created" claim to the strand-pane scope it actually holds for.

### [BLOCKING:design] "Beside validateAnchor" contradicts the stated ordering
**Section:** `one-dialect-per-pane-enforced-at-the-op-boundary` ("Where it fires")
**Issue:** The two cited anchors are not the same place — `validateIfAbsent` runs at `AddStrand`'s op boundary (`strand.go:390`, before `ensureSessionLocked` and `loadOrInitStateLocked`), while `validateAnchor` runs *inside* `addStrandLocked`/`updateStrandLocked` (`strand.go:244`, `:286`), after a possible session boot and after state load; `UpdateStrand` (`:496`) has no op-boundary validator at all.
**Fix:** Name the exact insertion point per verb (first statement inside each `withOpLock` closure, before `ensureSessionLocked`/`requireSessionLocked`/`ensureServerAndSessionLocked`), so the promise "before any state is loaded, any pane is reaped, or any strand record is appended" is implementable as written.

### [BLOCKING:consistency] r2 Q&A entry still carries the retired quoting answer
**Section:** Q&A log (the r2 spaced-shell-path entry)
**Issue:** That entry still answers "quote it through the dialect's `Quote` at the strand split" and still asserts `new-session` takes command-plus-arguments rather than one `sh -c` string — both retired by the r3 entries and by Scope/`strand-panes-run-the-configured-shell`, and the second is a claim the artefact itself now calls factually wrong; the r1 entry got an explicit **Superseded** marker, this one did not.
**Fix:** Add the same explicit superseded marker (or rewrite the answer) so no reader of the Q&A log alone implements quoting.

### [NIT:consistency] Wrong line cite for splitPaneBelowLocked
**Section:** Scope/Out, `strand-panes-run-the-configured-shell`, `chokepoint-is-the-strand-launch-only`
**Issue:** `splitPaneBelowLocked` is declared at `internal/reedengine/selvagepane.go:342`; `:352` (cited three times) is inside its error return.
**Fix:** Repoint the three citations at `:342`.

## Verdict

REQUEST_CHANGES
Refusal's verb set and insertion point are underspecified; one Q&A entry is stale.
MILL_REVIEW_END
