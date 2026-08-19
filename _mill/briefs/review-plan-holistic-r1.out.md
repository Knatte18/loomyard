MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Claude Code agent harness; exact point-version not self-verifiable)
reviewed_file: plan/
date: 2026-08-19
```

## Findings

### [BLOCKING:design] landingshed.Deps has no way to obtain a Shuttle for the resolver
**Location:** Batch 4, card 20 (`internal/landingshed/deps.go`), consumed by cards 23/24.
**Issue:** Batch 3 says `mergeresolve.Deps` requires a non-nil `Fabric` **and** a non-nil `Shuttle` (the `Run(shuttleengine.Spec) (shuttleengine.Result, error)` seam) — `New` rejects a nil `Shuttle` (03-mergeresolve-engine.md card 12). Batch 4's scope text says landingshed "depends on batch 3 for the resolver both producers drive," and cards 23/24 say Publish/Finalize each "run the resolver's merge-in." But card 20's enumerated `Deps` fields (`WorktreeRoot, TaskBranch, ParentBranch, WebsterDir, StencilsDir, ScratchDir, OriginURL, PushSkipped, PushBranch, OpenFabric, OpenParentFabric, Registry, Config`) carry no `Shuttle` value and no pre-built `*mergeresolve.Resolver` — verified: `Shuttle`/`Resolver`/`mergeresolve.New` are never mentioned anywhere in 04-landingshed-producers.md. `OpenFabric`/`OpenParentFabric` supply only `*fabricengine.Fabric` (the `MergeSurface` half), never a shuttle runner. Confirmed against the established sibling pattern (`shedadapters.NewSingleLLMProducer(name, specs, shuttle Shuttle, now)`, `internal/shedadapters/singlellm.go`): every existing shuttle-driving constructor takes its `Shuttle` as an explicit told value. Without an equivalent field here, `NewPublish`/`NewFinalize` cannot construct or receive a working resolver, so neither producer can spawn the conflict-resolution session cards 23/24 describe.
**Fix:** Add a told `Shuttle` field (or a pre-built `*mergeresolve.Resolver`/`mergeresolve.Deps`-shaped value) to `landingshed.Deps` in card 20, and state in cards 23/24 where/when `mergeresolve.New` is called (construction time vs. lazily in `Call`, mirroring the `OpenFabric` laziness rule already stated for the Fabric half). Card 34's `testDeps` extension will also need this field populated for `New(testDeps(t))` to keep succeeding once rows 12/13 are real.

### [NIT:scope] shed.md link-count claim off by one (self-correcting, but worth flagging)
**Location:** Batch 6, card 38 (`manifest/designs/shed.md`).
**Issue:** Card 38 claims "four links" in shed.md pointing at `landing.md` (status banner, worked-example line, task-bundling paragraph, Related entry). Grep for `](...landing.md...)` in shed.md finds only three actual markdown links (lines ~62, ~298, ~309); the "status banner" reference is a bare backtick/code-span filename mention (`` `manifest/designs/landing.md` ``), not `[text](target)` syntax, so it is prose per CONSTRAINTS.md's own Markdown Link Integrity distinction, not a link the guard enforces.
**Fix:** Correct the count to "three links plus one prose mention" so the implementer doesn't try to force the status-banner prose into `[...]()` link syntax it never had; the card's own re-grep instruction will catch this but the miscount should not ride along uncorrected.

## Verdict

REQUEST_CHANGES
Card 20's told-value `Deps` omits the `Shuttle` seam cards 23/24 require to drive the resolver; must be added before batch 4 is buildable.
MILL_REVIEW_END
