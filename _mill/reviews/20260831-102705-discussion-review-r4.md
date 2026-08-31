MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
duration_s: 120.2
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (runtime reports claude-opus-5); exact build not self-verifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] Q&A contradicts the list-clients target form
**Demoted-from:** BLOCKING
**Section:** Q&A log (target-form entry vs. hook-body entry) **Issue:** The hook-body Q&A answers `exactSessionWindowTarget`, while `repaint-body-composition` and the dedicated target-form Q&A both mandate `exactSessionTarget` (bare `=<name>`) for `list-clients`, with a stated reason. **Fix:** Correct the hook-body Q&A answer to `exactSessionTarget` so one form is stated at every site.

### [NIT:consistency] "Rebuild on both call paths" is superseded
**Demoted-from:** BLOCKING
**Section:** Constraints → Discovered during discussion; Q&A (watchdog gating) **Issue:** Both state the `watchdog: off` array clear "is immediately followed by a rebuild on both call paths", which `repaint-is-independent-of-watchdog` explicitly refutes and the source confirms — `lifecycle.go:430` calls `pinGeometryOptionsLocked()` and returns with no install, and `apply.go:232-235` returns on `SkipFocus` before `installResizePinsLocked`. **Fix:** Replace both statements with the decision's actual residual (empty array from boot until the first attach or first focusing apply).

### [BLOCKING:design] No-candidate branch leaves treatment scenarios undefined
**Section:** Testing → Measurement gate **Issue:** If neither candidate clears either trigger, no repaint entry ships, yet the treatment scenarios assert the artifact is *absent*; only the cross-client trigger has a stated inversion rule, and it is scoped to the fully-covered sizing case, not to this branch. **Fix:** State the disposition of both triggers' treatment scenarios in the no-candidate branch (inverted to assert the documented residual, skipped, or not landed).

### [BLOCKING:design] Hook self-retrigger risk for candidate 1 unaddressed
**Section:** `repaint-mechanism` / `repaint-body-composition` **Issue:** Candidate 1 fires `refresh-client` against *every* client from inside `window-resized` under `window-size latest`, which keys on the most-recently-used client — the discussion states elsewhere that anything making a client current resizes the window, yet gives no stop condition, no bounded-retrigger reasoning, and no measurement criterion beyond "did it clear the artifact". **Fix:** State whether a server-issued `refresh-client` can move MRU/window size, and make "no repeated hook fire / no resize storm" an explicit acceptance criterion of the measurement gate.

### [NIT:consistency] Scope states the repaint entry unconditionally
**Section:** Scope → In (third bullet) **Issue:** The bullet promises "one additional entry in the … `window-resized` array" as a deliverable, while the measurement gate permits landing with no entry at all. **Fix:** Hedge the scope bullet to match the measurement gate's conditional outcome.

### [NIT:consistency] Builder file placement doesn't match source
**Section:** `repaint-body-composition` **Issue:** The `""`-returning builder is placed in `watchdog.go` "modelled on `resizeSignalHookCommand`", but that function lives in `windowsize.go:258`; only `resizeHookCommand`/`tmuxQuoteValue` are in `watchdog.go`. **Fix:** Name both files, or state which one the new builder joins and which test file follows it.

### [NIT:design] Warning cardinality with several differing clients
**Section:** `attach-time-multi-client-warning` / Testing **Issue:** Testing pins "exactly one warning" for one differing client; the behaviour for N differing clients (one line each vs. one aggregate) is unstated. **Fix:** State the cardinality rule.

## Verdict

REQUEST_CHANGES
Two superseded statements, an undefined no-fix test branch, and an unexamined hook-retrigger risk.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
