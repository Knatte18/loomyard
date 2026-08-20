# Orchestrator review — discussion.md

Reviewed against `main` (`dce9fde8`, unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

Dense discussion, dozens of concrete file/line citations across `internal/loomengine`, `internal/loomshed`, `internal/shedengine`, `internal/shedadapters`, `internal/preflight`, and several docs. Spot-checked every one with a line number or quoted text — all correct, including several that are easy to get wrong:

| Claim | Status |
|---|---|
| `preflight.go:78-159` is `runCheck4` | Correct, exact function boundaries |
| `status.go:23-27` is loom's `Status{Slug, Parent, StartSha}` | Correct |
| `loomshed.go:68` `Deps.Preflight shedengine.ShedProducer` | Correct |
| `hardener.md:16,95` "Hardener's own Preflight" (both occurrences) | Correct |
| `coherence.go:41,91` hardcoded `"Preflight"` literals | Correct |
| `coherence.go:84-89` fresh-start/tolerated-entry rationale | Correct, exact prose |
| `loomshed.go:90-96` escalate-only posture for Preflight/Batchifier/Publish/Finalize | Correct |
| `loomshed.go:98-102` Plan-Sweep-not-a-row rationale | Correct |
| `loomshed.go:114-121` Publish/Finalize built by reference from `landingshed` | Correct |
| `loomshed.go:15-17` constant-not-literal rule | Correct |
| `loomshed.go:23-36` the 12 name constants | Correct, exact order |
| `run.go:110` "producer list has changed" hard-error | Correct |
| `run.go:187-200` `Stuck`/`OnStuck: ""` → `RunBlocked` | Correct |
| `run.go:223-244` `Done` persist-then-call-next sequencing | Correct |
| `seed.go:57` `CurrentProducer: NamePreflight` | Correct |
| `loom-status-spec.md:33,39` fresh-seed shape | Correct |
| `preflight.go:114-116` `MkdirAll` guard, `128-134` TOCTOU guard | Correct |
| `internal/loomshed/preflight.go:44-65` existing row-1 wrapper's `Call` | Correct |
| `shedadapters/ctx.go:14-30` `entryErr`/`cancelErr(ctx, name, engine)` | Correct, 3-arg shape confirmed |
| `internal/loomshed/ctx.go` "same idea without the engine label" | Correct — confirmed 2-arg (`ctx, name`) |
| `loomshed_test.go:144` "the thirteen rows" | Correct, exact wording |
| `loomshed_test.go:27-40` `wantProducerTable` | Correct |
| `smoke_test.go:21` "eight of its thirteen rows with stub producers" | Correct wording, and correctly flagged as wrong against present-day `stub.go:12` |
| `stub.go:12` "backs five rows of loom's 12-row producer list" | Correct — confirms the doubly-wrong claim (eight vs. five, thirteen vs. twelve) |
| `docs/overview.md:237` "loom's own 13-row producer list" | Correct |
| `CONSTRAINTS.md:64` tier-3 bullet naming `loomengine.Preflight` | Correct, exact text |
| `docs/benchmarks/running-tests.md:13` Tier 1 definition | Correct |
| `landingshed/doc.go:40-43` Fabric Vocabulary owner-set model | Correct |
| `sequence_test.go:15-18,20-32,33-35` row numbering / `wantSequenceOrder` | Correct |
| `manifest/roadmap.md:66-71` — the exact Planned item text, "13 rows to 14" | Correct, verbatim |
| All 16 named test functions in the `preflight_integration_test.go` → `preflight/preflight_integration_test.go` migration table | Correct — every name on both sides exists exactly as written, including the two correctly-flagged orphans (`TestPreflight_ConfigLoadFailed`, `TestPreflight_MissingOptionalJunctionIsAJunctionFault`) that have no counterpart and must be migrated, not dropped |

No inaccurate citation found anywhere in this discussion — a longer and denser doc than the `shedengine-segments-bounce-budget` one reviewed just before it, with a clean result.

## Design read

**The row-count reconciliation (`row-count-reconciliation` decision) is the standout piece of this discussion.** The roadmap item's own wording ("13 rows to 14") is wrong for the code (`loomshed.New` builds 12 today, going to 13) and right only for `loom.md`'s design table (13 going to 14, because that table carries the unbuilt `Plan-Sweep` row the code doesn't). Catching that the two counts are legitimately different — rather than either taking the roadmap's numbers literally or silently picking one — and then separately catching that `stub.go` and `smoke_test.go` already disagree with each other *today* (five stubbed rows vs. "eight of thirteen," both wrong now, both counted correctly by this task) is exactly the kind of cross-file consistency check that's easy to skip and expensive to skip.

**`check3BlocksSeed`'s removal is well-justified**, not just asserted: the reasoning (`Shed` never advances past a `Stuck` row 1, so row 2 provably never runs when tiers 1–3 failed) is a real proof from `run.go`'s own control flow, not a convenience claim.

**The coherence-rule rewrite (`coherence-rules-after-the-split`) is correctly derived from `Run`'s exact persist-then-call ordering**, not from an assumption about when things happen — the "at the instant row 2 runs, the file already reads `current_producer: Loom-Preflight` with a `Preflight` `Done` entry" claim is a direct, verified consequence of `run.go:223-244`.

One soft ambiguity, not a blocker: `preflightshed`'s own context-check helpers are told to model on `shedadapters/ctx.go`'s two-function shape, with `internal/loomshed/ctx.go` cited as "the same idea without the engine label" — but the discussion doesn't pin which of the two shapes (3-arg with engine label, or 2-arg without) `preflightshed` itself should end up with. Leaving that to the planner is fine; flag it so it doesn't get decided by accident.

The self-flagged "Known risk the planner must confirm" (whether `loomshed`'s Tier-1 fixtures write a status file coherent enough for a *real* row 2 to pass, since today row 2 doesn't exist and row 1 is faked) is the right thing to carry forward explicitly rather than discover mid-implementation — good that it's named rather than left implicit.

## Verdict

Sound. Nothing here should block moving to Plan. The ctx-helper-shape ambiguity is worth a one-line pin during planning, not a discussion round of its own.
