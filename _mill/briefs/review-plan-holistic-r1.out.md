MILL_REVIEW_BEGIN
# Review: Decide tmux mouse-mode default for lyx mux — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-15
```

## Findings

### [NIT] Card 3 Context omits mouse.go it calls mouseOption from
**Location:** Batch mouse-default / Card 3
**Issue:** Requirements call `mouseOption(e.cfg.Mouse)` but `internal/muxengine/mouse.go` is in neither `Context:` nor `Edits:`; the file is created in Card 1 of the same batch and its full `(string, error)` signature is spelled out inline, so cold-start risk is low but the entry is formally missing.
**Fix:** Add `internal/muxengine/mouse.go` to Card 3's `Context:`.

### [NIT] Card 2 references mouseOption without listing mouse.go
**Location:** Batch mouse-default / Card 2
**Issue:** The `Mouse` field doc comment text names `mouseOption` (mouse.go), which is not in Card 2's `Context:`/`Edits:`; it is a doc-comment reference, not a call, so functionally harmless.
**Fix:** Add `internal/muxengine/mouse.go` to Card 2's `Context:` for consistency.

### [NIT] Card 5 cites a non-existent contract-test option-read precedent
**Location:** Batch mouse-default / Card 5
**Issue:** Card says read mouse back "using the same raw-psmux command style the contract test uses for its option assertions," but `contract_integration_test.go` never reads an option back (it sets `remain-on-exit` but only asserts pane/session state); there is no `show-options` precedent to mirror.
**Fix:** Point at the test's general `mux.output(...)` read style (e.g. its list-sessions/display-message reads) rather than a nonexistent option-assertion precedent.

### [NIT] Integration test (Card 5) runs in no automated verify gate
**Location:** Batch mouse-default / Card 5 + overview Decision integration-test-gating
**Issue:** The `//go:build integration` test is excluded from batch `verify: go test ./internal/muxengine/`; it only runs via a manual `-tags integration` invocation. This is deliberate, documented in the Batch Tests section, and matches the pre-existing `contract_integration_test.go` convention — acceptable, noted for transparency.
**Fix:** None required; optionally record the `-tags integration` command in the top-level `verify:` note so the integration lane is discoverable.

## Verdict

APPROVE
Plan is faithful to its decisions and source; only minor context/wording NITs remain.
MILL_REVIEW_END
