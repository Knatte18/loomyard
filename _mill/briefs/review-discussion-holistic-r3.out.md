MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-reported ID "claude-opus-5"); cannot verify independently
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:consistency] Q&A Windows answer contradicts Scope premise
**Section:** Q&A log ("Does the new hook entry ship on Windows?") vs. Scope/Out + Technical context ("Windows — read this carefully")
**Issue:** The Q&A says the array "is already never installed there ... the entry inherits that rule with no new reasoning", which is exactly the wrong summary the other two sections spend a paragraph each refuting (`installResizePinsLocked` at `internal/reedengine/windowsize.go:303` has no `runtime.GOOS` gate; only `resizeSignalHookCommand` returns `""` on Windows).
**Fix:** Rewrite the Q&A answer to match the body — the entry inherits nothing and needs its own `""`-returning builder.

### [BLOCKING:design] Watchdog-off reconciliation rests on a false call-site claim
**Section:** `repaint-is-independent-of-watchdog` ("Reconciliation with the unconditional unset")
**Issue:** It claims `lifecycle.go`'s boot path and `AttachArgv` "both call `installResizePinsLocked` a few statements later in the same locked closure"; `lifecycle.go:430` calls `pinGeometryOptionsLocked()` and then returns — the only install sites are `attach.go:144` and `apply.go:235`, and `apply.go` returns at line 232 under `opts.SkipFocus` *before* line 235, which is the watchdog re-apply's own mode.
**Fix:** Re-derive the reconciliation from the real call sites and state the true residual window for `watchdog: off` (boot leaves the array cleared until an attach or a non-SkipFocus apply).

### [BLOCKING:design] Control scenario can be silently rebuilt away
**Section:** Testing → "The negative control"
**Issue:** The control rewrites the `window-resized` array from the test to reproduce reed's pre-task array, but every `AttachArgv` pre-flight rebuilds that array from scratch (`attach.go:144`); in the cross-client scenario the second attach necessarily happens after setup, so the repaint entry is re-installed and the control asserts against reed's *post*-fix array.
**Fix:** Pin the rewrite's position in each scenario's sequence relative to every attach, and state how the control proves the array it fired against was the one it wrote.

### [BLOCKING:design] Client-list target left as two alternatives
**Section:** `repaint-body-composition` (third bullet) and Technical context → "The tmux seam"
**Issue:** The target is given as "`exactSessionWindowTarget(e.SessionName())` / `exactSessionTarget`" with no choice; the two differ (`=name:` vs `=name`, `overlay.go:93-101`) and `list-clients -t` takes a session target, so a plan writer picking the window form may get a rejected or differently-scoped query.
**Fix:** Name one form for `list-clients` (hook body and attach-time warning both) and say why.

### [NIT:design] Dot-run threshold is self-adjusting
**Section:** Testing → "The assertion predicate"
**Issue:** "20 consecutive `.`" is qualified by "raise it if a clean run trips it", leaving the predicate's floor decided at implementation time rather than in the discussion.
**Fix:** State the floor as fixed, with the clean-capture check as a one-time validation rather than a licence to retune.

## Verdict

REQUEST_CHANGES
One self-contradiction, one false code premise, and two unresolved choices block plan writing.
MILL_REVIEW_END
