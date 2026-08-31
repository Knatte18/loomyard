MILL_REVIEW_BEGIN
# Review: Reconsider the collapsed strand strip default size

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (self-reported; exact build unverifiable from inside the session)
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:scope] Integration test consuming the default is unenumerated
**Section:** Technical context ("Test assertions that pin the default" / "Test values that must NOT move") + Testing
**Issue:** `internal/reedengine/attachgeometry_integration_test.go` asserts live tmux pane height against `e.cfg.CollapsedStripRows` (lines 248-249, 367-368) with a fixture that loads the real template (`newIntegrationEngine` → `seedReedConfig` → `LoadConfig`, `mouse_boot_integration_test.go:35-40`), so it exercises the new `6` against real geometry at 100x30 and 100x90 — yet it appears in neither the "must move" nor the "must NOT move" list, and Testing asserts `config_test.go` is "the only place behaviour is asserted" and that no live tmux is involved.
**Fix:** Give that file an explicit disposition (value-agnostic, no edit) and state whether the integration tier must be run to confirm the new default still lands unclamped at 100x30.

### [NIT:design] Six-row cost analysis omits the clamping case
**Section:** Decisions › strip-default-six
**Issue:** Both worked examples are chosen so `clampToFit` does not fire; at `6`, four strips on a 30-row client give `usable = 24`, `stripDemand = 24`, active-pane natural height `0` (`render/height.go:55-80`), where `3` would leave 12 — so `6` does introduce clamping at a depth/height `3` did not, unstated.
**Fix:** Name the depth×window threshold at which `6` begins to clamp and state it as accepted degradation, cross-referencing `clamp-path-unchanged`.

### [NIT:decision] "Documented" no-migration has no named artefact
**Section:** Decisions › no-value-migration vs Scope
**Issue:** The decision says existing hubs keep `3` and "the change is documented", but Scope's template edit is described only as "the readability rationale", while every other reed knob's comment (`mouse`, `watchdog` in both templates) also carries the "an already-materialized reed.yaml keeps whatever value it holds" adoption caveat.
**Fix:** State that the new `collapsed_strip_rows` comment carries the reconcile adoption caveat alongside the rationale, matching the sibling knobs.

## Verdict

REQUEST_CHANGES
One live-tmux test consuming the changed default has no stated disposition.
MILL_REVIEW_END
