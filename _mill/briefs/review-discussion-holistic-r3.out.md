MILL_REVIEW_BEGIN
# Review: Reconsider the collapsed strand strip default size

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model as exposed to this harness
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:consistency] 40-row clamp threshold contradicts its own arithmetic
**Section:** Decisions → `strip-default-six`
**Issue:** "five-deep on a 40-row one" does not follow from `stackHeights` as read in `render/height.go`: with a 40-row window the stack box is 38 (header 1 + divider 1), five strips plus the active pane use 5 dividers → usable 33, `stripDemand` 30, active pane 3 — positive, so nothing clamps; the first clamping depth at 40 rows is six strips (38 − 7·6 < 0). The 30-row ("four-deep") and 50-row ("seven-deep") figures both check out under the same strip-count reading, so the middle figure is the outlier.
**Fix:** Recompute and restate the 40-row threshold (six strips under the same counting the other two use), and state explicitly whether "N-deep" counts strips or total strands, since that rationale is what the template comment will carry.

### [BLOCKING:consistency] Integration run: required to land, or not required?
**Section:** Constraints vs Testing
**Issue:** Constraints says "no live tmux may be required to verify this change", while Testing says `attachgeometry_integration_test.go` "must still be RUN" as "the only check that `6` actually lands unclamped on a real multiplexer" and that `go test -tags integration ./...` must pass. A plan writer cannot tell whether landing is gated on a tmux-equipped machine or whether the tagged test's self-skip satisfies the requirement.
**Fix:** State one disposition — either the integration run is a landing gate (and the Constraints line is narrowed to "the untagged tier stays tmux-free"), or it is best-effort and the skip is acceptable.

### [NIT:scope] `lock_test.go` missing from the must-not-move enumeration
**Section:** Technical context → "Test values that must NOT move"
**Issue:** `internal/reedengine/lock_test.go:60,98` also sets `CollapsedStripRows: 2`; the list names only `render/rules_test.go`, `pins_test.go`, `height_test.go`, and `apply_test.go`.
**Fix:** Add `lock_test.go` to the same no-edit enumeration so the "surgical edit" check covers every `CollapsedStripRows: 2` site.

### [NIT:consistency] Overstated claim about sibling knob comments
**Section:** Decisions → `rationale-lives-in-the-template-comment`
**Issue:** "Every other reed knob — `mouse`, `watchdog`, `debug_log`, `width`, `height` — carries its rationale and its adoption caveat" is not what the templates show: only `mouse` and `watchdog` carry the "already-materialized reed.yaml keeps whatever value it holds" caveat, `debug_log` carries a different one ("existing hubs must run lyx config reconcile to adopt this key"), and `width`/`height` carry none. Scope §L26 states this correctly.
**Fix:** Align the rationale's wording with Scope's — rationale in every knob's comment, adoption caveat in `mouse`/`watchdog` only.

## Verdict

REQUEST_CHANGES
One rationale figure is arithmetically wrong; integration-run gating is stated two contradictory ways.
MILL_REVIEW_END
