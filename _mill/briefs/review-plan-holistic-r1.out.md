MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Batch 6 cards 20/21 split breaks per-card compilation
**Location:** 06-compose-webster.md, cards 20 and 21
**Issue:** Card 20 edits `beginbatch.go`/`recoverbatch.go`/`runlevel.go` to pass a new trailing `notePath` argument into `RenderForkPrompt`/`RenderRecoveryPrompt`/`RenderIntegrationPrompt`/`RenderMasterPrompt` (its own text says "card 21 adds the parameters"), but card 21 — the NEXT card — is what actually widens those four signatures in `render.go`. Verified against the current `render.go` (lines 145/177/210/264) and the three call sites (`beginbatch.go:311`, `recoverbatch.go:154`, `runlevel.go:548`, `runlevel.go:564`): after card 20's own commit lands, the call sites pass one more argument than the functions accept, so the tree does not build. This violates the plan-card contract's own rule 1 in `contracts/stencils/loom/loom-template-plan.md` ("Builds on its own ... the project compiles ... immediately after the card's commit"), and is inconsistent with batch 5's card 17, which lands `burlerengine.New`'s new parameter and all three of its call sites together in one card for exactly this reason.
**Fix:** Merge cards 20 and 21 into one card (signature widening + all four call sites + Deps fields together), or reorder so the `render.go` signature change lands first and the call-site edits land after.

### [BLOCKING:scope] Card 22 never targets the file the `loomCLI` struct is declared in
**Location:** 07-wiring.md, card 22
**Issue:** Card 22 requires "Store `frictionDir` on the `loomCLI` receiver beside the existing `c.cfg`/`c.runner` fields," which later cards read (`run.go` in card 23, `drive.go` in card 24). Verified against `internal/loomcli/cli.go`: `type loomCLI struct { ... cfg loomengine.Config ... runner *shuttleengine.Runner ... }` is declared there, in that file alone — a new field can only be added by editing it. Card 22's own `Edits:` list names only `internal/loomcli/wiring.go`, and no other card in batch 7 (22–26) lists `internal/loomcli/cli.go` under `Creates`/`Edits` either. The parallel change for webster (`internal/webstercli/cli.go` gaining `frictionDir`) correctly lists `cli.go` under card 25's `Edits:`, which throws the loomcli omission into relief.
**Fix:** Add `internal/loomcli/cli.go` to card 22's `Edits:` list and its `Context:`, with a requirement line adding the new `frictionDir string` field to the `loomCLI` struct.

## Verdict
REQUEST_CHANGES
Two BLOCKING sequencing/scope defects in batches 6 and 7; the rest of the plan is unusually well source-grounded.
MILL_REVIEW_END
