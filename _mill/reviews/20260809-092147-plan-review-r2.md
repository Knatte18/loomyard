MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (per harness: "Sonnet 5" / model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [BLOCKING:scope] Card 8 Context omits warpbinding.go despite naming its constant
**Location:** batch 3 (03-cli-surface.md), Card 8 **Issue:** Requirements instructs naming `fabricengine.WarpBindingFileName` in the rewritten `Long` text, but that constant is declared in `internal/fabricengine/warpbinding.go`, which is not in Card 8's `Context:` (only `internal/fabriccli/clone.go`, `internal/fabricengine/clone.go`, `internal/weftname/weftname.go`, `CONSTRAINTS.md`). Verified: `clone.go` never declares or references this constant itself. **Fix:** Add `internal/fabricengine/warpbinding.go` to Card 8's `Context:`.

### [BLOCKING:scope] Card 12 needs export_test.go but never lists it
**Location:** batch 4 (04-clone-integration-tests.md), Card 12, `TestCloneHub_HubExistsCheckPrecedesProbeInTwoArgForm` **Issue:** Requirements says to add the probe-prefix constant to the package's `export_test.go` seam "if it is unexported" — but Card 3 (batch 2) declares `warpProbeDirPrefix` lower-case/unexported, so this is not conditional, it will always apply. `internal/fabricengine/export_test.go` (confirmed to exist, currently re-exporting `NewPairedForTest`/`WarpForTest`/`WeftForTest`) is absent from Card 12's `Context:`, `Edits:`, and `Creates:`. **Fix:** Add `internal/fabricengine/export_test.go` to Card 12's `Edits:` and `Context:`, and state the re-export explicitly (e.g. `WarpProbeDirPrefixForTest`) rather than "if... add it there."

### [NIT:consistency] Card 21 misses an adjacent stale `.fabric-anchor` reference
**Location:** batch 6 (06-docs-and-sandbox.md), Card 21 vs. `tools/sandbox/SANDBOX-FABRIC-SUITE.md` **Issue:** F5's Watch line (line 189) still names the pre-rename `.fabric-anchor` marker ("confirm the repo-wide records survive unwire — `.fabric-anchor` and `<BoardDir>/_lyx/config/fabric.yaml` are untouched"), the exact staleness Card 19 does a drive-by fix for in `CONSTRAINTS.md` in the same batch. **Fix:** Extend Card 21 (or Card 19's drive-by-fix note) to also correct this occurrence to `.lyx-anchor`.

### [NIT:consistency] Card 21's "Watch" claim for S6 doesn't match the file
**Location:** batch 6 (06-docs-and-sandbox.md), Card 21 vs. `tools/sandbox/SANDBOX-CORE-SUITE.md` S6 **Issue:** Card 21 states the subpath-anchored-clone scenario "spells the full command warp-first in both its Goal and its Watch lines," but only the Goal line (205) spells URLs; the Watch line (213) never names argument order at all (`` `lyx fabric clone --subpath <sub>` ``, no URLs). Harmless (nothing to flip), but the premise is inaccurate. **Fix:** Correct Card 21 to say only the Goal line needs flipping.

### [NIT:design] Batch 4's depends-on: [3] looks unnecessarily conservative
**Location:** 00-overview.md Batch Index; batch 4 (04-clone-integration-tests.md) **Issue:** Batch 4's cards (11, 12) test `internal/fabricengine.CloneHub` directly via `CloneOptions`, delivered by batch 2 (Card 4); nothing in batch 4's Context/Edits touches `internal/fabriccli` (batch 3's sole target), and there's no file overlap between batches 3 and 4. `depends-on: [2]` would be equally correct and would let batches 3 and 4 schedule independently. **Fix:** Either change to `depends-on: [2]` or add a one-line rationale for why 3 must precede 4.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (Cards 8, 12) are blocking; the remainder are minor doc/DAG polish.
MILL_REVIEW_END
