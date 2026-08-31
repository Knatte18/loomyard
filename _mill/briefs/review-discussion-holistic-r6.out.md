MILL_REVIEW_BEGIN
# Review: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Cross-client control cannot hit when fully covered
**Section:** Testing, cross-client trigger (sizing rule) vs `root-cause-model`
**Issue:** The model says dots are tmux padding a client region *not covered* by the window (client larger than window), yet the cross-client scenario is required to size the observed client so it is **fully covered** — under the model's own mechanism that configuration has no uncovered region and the control scenario, which must hit or fail the run, plausibly cannot reproduce anything; the discussion states a disposition for "treatment cannot be cleared" but none for "control does not reproduce when sized fully covered".
**Fix:** State the mechanism by which a fully-covered client shows stale dots at all in the cross-client case, and give an explicit disposition for a cross-client control that fails to reproduce under the mandated sizing (e.g. accept the uncovered sizing for the control, or drop the pair to documentation-only).

### [BLOCKING:consistency] Candidate 2's body has no builder home or test shape
**Section:** `repaint-body-composition` / Testing, `internal/reedengine` unit tests
**Issue:** `repaint-body-composition` specifies only candidate 1 ("for candidate 1 the body needs four things", wrapped in `tmuxQuoteValue` and prefixed `run-shell -b`), while candidate 2 is a bare `refresh-client` with no `run-shell` — but Testing asserts unconditionally that the repaint body builder has "correct `-b`, correct `tmuxQuoteValue` escaping", which candidate 2 would fail by construction, and no file/home or `""`-on-Windows wrapper is named for it.
**Fix:** Say where candidate 2's body builder lives, whether it is quoted at all, and make the Testing assertions conditional on the selected candidate.

### [NIT:design] New `internal/shell` member signature left non-committal
**Section:** `repaint-body-composition`, fourth bullet
**Issue:** The new primitive is specified as "along the lines of `ForEachLine(command, body string) string`" — the one genuinely new public surface in the task is left to the plan writer's interpretation, including whether the body receives the line via a named variable and how that variable is spelled per dialect.
**Fix:** Pin the exact method name, parameter list, and how the per-line value is exposed to the body in each dialect.

## Verdict

REQUEST_CHANGES
Cross-client control sizing contradicts the root-cause model; candidate 2's body is unspecified.
MILL_REVIEW_END
