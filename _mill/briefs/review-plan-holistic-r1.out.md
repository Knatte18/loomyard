MILL_REVIEW_BEGIN
# Review: Master Builder: new, parallel fork-based implementation module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-17
```

## Findings

### [MAJOR] BeginBatch forward-references RestartChain (later card)
**Location:** 05-bracket-verbs, card 19 (step 3) vs card 21
**Issue:** Card 19 `BeginBatch` calls webster `RestartChain`, but that function is created in card 21 (later in the same batch); card 19's per-card commit will not compile (undefined symbol), violating card self-containment/no-forward-deps.
**Fix:** Reorder so card 21 (chain.go / RestartChain) lands before card 19, or fold RestartChain creation into card 19's Creates.

### [MINOR] Card 38 authors suite content specified in card 40 (cross-batch)
**Location:** 08-webstercli-registration card 38; 09-sandbox-and-docs card 40
**Issue:** Card 38 (batch 8) must write both W1/W2 scenarios "specified in card 40's requirements" (batch 9), but neither card 40 nor discussion.md is in card 38's `Context:`; ownership of the scenario prose is split and back-referenced to a downstream batch.
**Fix:** Have card 38 write only the minimal `**Covers:** webster`-tagged stub the coverage guard needs, and let card 40 author the full scenarios; or copy the full W1/W2 spec into card 38 and add its sources to `Context:`.

### [NIT] Exported builderengine helpers referenced without their source in Context
**Location:** card 25 (`builderengine.FirstFreeArchivePath`, runlevel.go), card 29 (`builderengine.RemoveStrandIfLive`, spawn.go)
**Issue:** Requirements name these helpers but the files defining them are not in the card's `Context:` (signatures are pinned in card 5, so impact is low).
**Fix:** Add `internal/builderengine/runlevel.go` to card 25's Context and `internal/builderengine/spawn.go` to card 29's Context.

### [NIT] Run-exit cross-check reads pointer Result.ForkAudit without nil note
**Location:** 07-run-level, card 30
**Issue:** `Result.ForkAudit` is `*ForkAudit`; card 30 runs `CheckParent`/`CheckFork` over it without stating the nil guard (populated only when `ForkSubagents` set — true for the Master, but the contract should be explicit).
**Fix:** State that a nil `Result.ForkAudit` on a `done` fork-authorized run is itself a hard error (audit could not complete), matching the fail-loud posture.

## Verdict

REQUEST_CHANGES
Sound, source-accurate plan; fix one within-batch ordering compile dep and tighten two Context/ownership gaps.
MILL_REVIEW_END
