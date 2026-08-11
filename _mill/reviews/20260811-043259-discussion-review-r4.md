MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Claude Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:consistency] Acceptance command misses the Tier-2 evidence
**Demoted-from:** BLOCKING
**Section:** Testing → "Acceptance: `go build ./...` and `go test ./...` pass."
**Issue:** Both files carrying this task's decisive evidence are `//go:build integration` — `internal/websterengine/runlevel_test.go:1` (the 15 `TestRun_*` + `newRunFixture` `Batcher:` injection, and the nil-guard test that would live beside it) and `internal/webstercli/verbs_test.go:1` (TDD candidate 4, the relocated `PersistentPreRunE` gate pair) — so plain `go test ./...` neither runs nor even compiles them; a broken `RunDeps` literal or a stale `websterengine.ConfigTemplate()` `strings.Replace` would land green.
**Fix:** State the tagged run explicitly in Acceptance (`go test -tags integration ./...` alongside the untagged run), and mark TDD candidates 4 and the nil-guard scenario as Tier 2 so the plan writer does not expect them under the default tier.

### [NIT:consistency] Nil-guard placement wording contradicts itself
**Section:** Decisions → runlevel-call-site, "Nil contract"
**Issue:** The paragraph says the guard returns "before any batching happens" but then that it "belongs immediately beside the existing zero-batch refusal" — that refusal is `runlevel.go:336`, *after* `active.Batch(plan.Cards)` at `:331`, so the literal reading places the nil check after the very call that panics.
**Fix:** Say the guard precedes the `deps.Batcher.Batch(plan.Cards)` call (replacing the deleted `Select` block at `:327`), and drop or reword "beside the zero-batch refusal" to a same-shape-of-refusal analogy only.

## Verdict

APPROVE
Acceptance criteria omit the integration tier where this task's decisive tests live.
MILL_REVIEW_END
