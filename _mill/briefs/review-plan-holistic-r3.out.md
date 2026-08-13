MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-13
```

## Findings

### [BLOCKING:design] Card 36 deletes a load-bearing fabric.yaml override, not a template duplicate
**Location:** batch 6 (stuck packages), card 36 (`internal/loomengine/preflight_integration_test.go`)
**Issue:** Card 36 instructs "Delete `seedRepoWideFabricConfig` outright" with no replacement, on the premise that `fabriccli.CloneAndWire`'s default `fabric.yaml` already covers it. But `seedRepoWideFabricConfig` here writes `pathspec: _extra` (verified: the file's own comment says this is deliberate — `RepoWiredNames` must agree with `setupPreflightFixture`'s `WireJunctions(..., []string{"_lyx", DotLyxDirName, "_extra"})` call), whereas `fabricengine`'s registered default template (`template.yaml`) ships `pathspec: ""`. Deleting the helper with no replacement leaves `_extra` unrecognized as a wired name post-clone, breaking `TestPreflight_MissingOptionalJunctionIsAJunctionFault` and the "Extra" junction-corruption sub-tests (lines ~448-484, ~499-515), which depend on `_extra` being classified as a real (missing/corrupt) optional junction rather than an unrecognized path. This is a genuine override, not a template match — outcome 3 of the overview's own three-way `SeedConfig` triage, exactly as batch 4 card 27 correctly resolves the analogous `seedRepoWideFabricConfig` helper in `perchcli`.
**Fix:** Replace the deletion with `hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")`, mirroring card 27's resolution, instead of relying on `CloneAndWire`'s default.

### [NIT:scope] Card 27's seedRepoWideFabricConfig call-site count is off by one
**Location:** batch 4, card 27 (`internal/perchcli/run_integration_test.go`)
**Issue:** Card 27 says "the `seedRepoWideFabricConfig` helper, five call sites across `internal/perchcli/run_integration_test.go`" — grep confirms only 4 call sites (lines 69, 128, 195, 278).
**Fix:** Correct "five" to "four" in the card text; the replace-all-calls instruction itself is unaffected.

### [NIT:scope] checkResolved call count in card 35 / Batch Tests conflates text hits with call sites
**Location:** batch 6, card 35 and the batch's "## Batch Tests" section (`internal/loomengine/preflight_integration_test.go`)
**Issue:** Both places state `checkResolved(` is called "twenty-six times." Grep for actual call expressions (`checkResolved\(`) finds 12; the count of 26 is the raw substring count of the bare word `checkResolved` (call sites plus `t.Fatalf("checkResolved: %v", err)` message literals). Card 35's own retarget instruction ("every `checkResolved(` call") is correctly scoped and unaffected, but the stated count overstates the shim's real call-site load by more than 2x.
**Fix:** Correct "twenty-six" to "twelve" (or state it as "26 textual mentions, 12 call sites") in both locations.

## Verdict

REQUEST_CHANGES
Card 36's blanket deletion of a genuine fabric.yaml override breaks two loomengine test cases; two other cards misstate call-site counts.
MILL_REVIEW_END
