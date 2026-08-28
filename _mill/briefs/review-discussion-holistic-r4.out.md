MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:consistency] `up` does not reach an install statement
**Section:** `hook-install-points-are-named-statements` (and `hook-mechanism-is-a-pure-tmux-resize-pane`, which says "which every `up`/`add`/`resume` does")
**Issue:** A fresh `up` clears every pane binding on boot (`lifecycle.go:666-670`, `clearAllPaneBindings` + `HeaderPaneID = ""`), so the subsequent `reconcileApplyPersistLocked` → `applyLayoutLocked` hits `!anyPlacedStrand` and installs nothing; the claim is only true for an `up` on an already-live session with bound strands, so the stated uncovered case ("never held two live panes") is the wrong characterisation of the gap — the real first installer is the first `add`.
**Fix:** Restate the coverage claim as "the first `add`/`resume` that places a strand", and re-derive the uncovered-window sentence (and the `doc.go` bullet derived from it) from that.

### [BLOCKING:design] Zero pins leaves a stale array with no clear
**Section:** `hook-body-is-one-array-entry-per-pin` / Testing (`internal/reedengine` untagged: "zero pins (nothing issued at all, not an empty hook body)")
**Issue:** The refresh is clear-then-rebuild, but issuing nothing on a zero-pin apply skips the `set-hook -u` too, so a previously installed strip pin survives against a pane that `render` has since placed as a full pane — reachable at the install statement when `st.HeaderPaneID` is not in the present set (`planLayout` blanks it) and no strip remains, after which every resize keeps clamping that live pane to the old strip budget.
**Fix:** State a disposition for the zero-pin case — either always issue the `set-hook -u` clear at the install statement, or record why the stale array is preferred there as it is for the two guard-skip states.

## Verdict

REQUEST_CHANGES
Two source-contradicted claims: `up` installs no hook, and zero pins skips the clear.
MILL_REVIEW_END
