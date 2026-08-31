MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
duration_s: 146.8
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] Call-site map overstates when an attach installs
**Demoted-from:** BLOCKING
**Section:** `repaint-is-independent-of-watchdog` (and the same claim in `## Constraints` → Discovered during discussion)
**Issue:** The map says the array is "(re)established … by an attach", and that with `watchdog: off` it is empty "from boot until the first attach or the first non-`SkipFocus` apply" — but `internal/reedengine/attach.go:69-75` returns bare *before taking the lock* on non-positive `cols`/`rows` (no-TTY), and every degrade inside the closure (`errAttachChainSuppressed` at lines 89-124, state-load or `listPanes` error) returns before the `installResizePinsLocked` call at line 144; meanwhile `pinGeometryOptionsLocked` (line 87) does run first and, with `watchdog: off`, issues an unconditional `set-hook -u` (`windowsize.go:145-153`), so a degrading attach under `watchdog: off` re-empties the array every time rather than filling it once.
**Fix:** Restate the map as "an attach whose chain succeeds, or a non-`SkipFocus` apply", and say explicitly that under `watchdog: off` a no-TTY or chain-suppressed attach clears without rebuilding — since this text is designated to be copied into `doc.go` verbatim.

### [NIT:consistency] Windows assertion placed on a pure function
**Section:** Testing → `internal/reedengine` pure unit tests, first bullet
**Issue:** `resizePinHookArgvs` (`windowsize.go:224`) takes command *strings* and holds no `runtime.GOOS` branch, so "the entry is absent on the Windows path" cannot be asserted there — only the empty-body case can; the GOOS branch lives in the engine builder, as `resizeSignalHookCommand` (`windowsize.go:258`) and its GOOS-skipped test (`windowsize_test.go:619`) already show.
**Fix:** Split the assertion: empty-body-emits-no-entry on the pure function, `""`-on-Windows on the engine wrapper's test in `windowsize_test.go`.

### [NIT:design] Measurement gate ordering vs. candidate 1's prerequisites
**Section:** Testing → Measurement gate ("Before the repaint entry is implemented…")
**Issue:** Candidate 1 cannot be measured before it exists — it requires the new `internal/shell` line-iterating primitive plus the body builder — so "measure, then decide whether to ship" has an unstated mechanism.
**Fix:** State that the gate is run by writing the candidate body into the array directly from the smoke scenario (the same `set-hook` rewrite technique the control already uses), with the production builder added only after a candidate is accepted.

## Verdict

APPROVE
One inaccurate call-site claim destined for `doc.go`; everything else verified against source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
