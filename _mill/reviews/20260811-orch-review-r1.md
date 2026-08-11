MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: APPROVE
reviewer_model: claude-sonnet-5
reviewer_self_id: Claude Sonnet 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Scope of this pass

Re-derived every checkable citation in the document against the live tree in this worktree rather than trusting the doc's own "already verified" framing: all twelve verb entry-point line numbers (`remove.go:41`, `add.go:37,222`, `checkout.go:38,189`, `prune.go:77,38`, `cleanup.go:100,65`, `unwire.go:56`, `junction.go:368`, `reconcile.go:150,329`, `commit.go:107`, `weftgit.go:269`, `spawn.go:89`, `coalesce.go:86`, `pull.go:171`), all thirteen `destroy.go` executor/minter/refusal-type line numbers, `weft_verbs.go:183,196,265`'s bare `output.Ok(out, map[string]any{})` calls, `fabrictest/manifest.go:121,289` and `verbs.go:102,195`, the stale `fabric-crucible-followups.md:419` consumer claim (confirmed genuinely stale — `CommitWeftAt`/`PushWeftAt` do not exist; `boardengine/sync.go:38` uses `fabricengine.NewBolt` instead, exactly as the doc says), the full 8-token `CONSTRAINTS.md` banned-bypass list, `PartialCommitError`/`PartialPullError`'s existence, `output.Ok`/`output.Err`'s current signatures, and every cited `CONSTRAINTS.md` invariant name plus its enforcement-test file. Every one checked out exactly as cited — no off-by-N errors found anywhere in this document, which is unusual given its density (this campaign's other discussion docs each turned up 1-4 citation-accuracy NITs).

One finding, non-blocking.

## Findings

### [NIT:clarity] "The exported Check set" doesn't name which package exports it

**Section:** Technical context, "The refusal type"
**Issue:** The paragraph opens by citing `destructiveRefusal` and its fields at `destroy.go:62` (package `fabricengine`), then in the very next sentence says "The exported `Check` set has exactly three live members — `CheckContainment`, `CheckOwnership`, `CheckDirtiness`." Read in isolation, this implies those three constants live in `internal/fabricengine` alongside `destroy.go`. They don't: `fabricengine`'s own check enum (`checkContainment`/`checkOwnership`/`checkDirtiness`/`checkForce`, `destroy.go:35-40`) is entirely unexported. The exported `Check` mirror (`CheckContainment`/`CheckOwnership`/`CheckDirtiness`, `Check` as a `string` type) lives in a different package, `internal/fabricengine/fabrictest/refusal.go:19-30`, built by the prior slice for `RefusedByGate`. The paragraph's closing sentence ("This is documented in `fabrictest/doc.go`") retroactively points the reader at the right package, but a reader stopping mid-paragraph — or grepping `internal/fabricengine` for `CheckContainment` while implementing — will not find it there.
**Fix:** Name the package explicitly: "The exported `Check` set fabrictest mirrors this with (`fabrictest/refusal.go`) has exactly three live members..." or similar, so the sentence doesn't read as describing `fabricengine`'s own (unexported) enum.

## Verdict

APPROVE
Every line-numbered and quoted citation checked — verb entry points, gate executors, the stale consumer claim, the CONSTRAINTS.md invariant list — reverified accurate against the live tree. One NIT: a package-attribution ambiguity in the exported-Check-set sentence, immaterial to any decision.
MILL_REVIEW_END
