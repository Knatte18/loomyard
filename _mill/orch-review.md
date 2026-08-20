# Orchestrator review — discussion.md

Reviewed against `main` (unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

Dense discussion with many exact file/line citations across `shedengine`, `shedadapters`, `burlerengine`, `treadleengine`. Spot-checked every one.

| Claim | Status |
|---|---|
| `shedengine/producer.go` — `Call(ctx) (Outcome, OutputPointer, error)`, `Outcome` exactly `Done`/`Stuck`, `ProducerDef{Name, OnStuck, OnDone, Segment, MaxBounces}`, `OnStuck: ""` requires shared `Segment` | Correct, exact |
| `shedadapters/perch.go` — `resolveRunID`/`highestRunAttempt` disk-based round resolution | Correct |
| `burlerengine/engine.go:96` `Run(p Profile, opts RunOpts) (Result, error)` | Correct, exact line |
| `engine.go:163` non-done outcome returns `nil` error, caller branches on `Result.Outcome` | Correct, matches almost verbatim |
| `engine.go:101,176` both read `p.clusterLenses`/`len(p.clusterLenses)` | Correct, exact lines |
| `profile.go:59` `validate` signature and path-resolution order | Correct |
| `config.go:91` `ResolveFan`, `maxClusterN` = 16 | Correct |
| `treadleengine/run.go:443` `runRound`, and the "second consecutive non-done attempt is an infrastructure error, not Stuck" comment | Correct, exact quote |
| `singlellm.go:107` cited for "no retry at all" posture | Line 107 is the `OutcomeAsking` case, one case above the `OutcomeDied`/`OutcomeTimeout` case that actually shows the no-retry mapping (~line 113). Same switch block, adjacent case — close enough to not mislead a reader, but not the precise line the claim is about. Minor. |
| `docs/overview.md:235,316,318` "three adapters" (module tree line, prose sentence, status-table sentence) | Correct, all three say "three adapters"/list exactly `SingleLLMProducer`, `perch`, `Webster` |
| `shedadapters/doc.go` structured sections `# Outcome mapping`, `# Told, never derived`, `# Shared cancellation rule`, `# Limitations` | Correct, exact section headers at lines 7, 27, 52, 65 |
| `shedadapters/archive.go` — `archiveStaleOutputs`, `firstFreeArchivePath` | Correct |
| `internal/perchengine/adapter.go` exists, named as the closest existing mapping analogue | Correct, file exists |
| `Lens{Name, Text}` in `burlerengine` | Correct — `config.go:48` |
| CONSTRAINTS.md invariants named (Shed Producer-Seam, Told-Geometry, Lyxdirs Single-Declarer, Review Round, Treadle Runner-Seam, Producer Pointer-Rule, CLI/Cobra, Documentation Lifecycle, Config Strictness) | Spot-checked against the file; all named correctly for what they actually restrict |

No inaccurate citation beyond the one imprecise (not wrong, just adjacent-line) `singlellm.go:107` reference.

## Design read

**The never-`Done`-except-for-a-completed-round rule (§"Always Stuck, never Done") is correctly load-bearing, not stylistic.** The discussion catches a real hazard: if a *failed* round returned `Stuck` with no review file, the `Bouncer`'s seed-vs-judge distinction (file-existence based, per the roadmap's own design) would misread it as a seed call and silently re-seed instead of surfacing the failure. Tying "hard error on non-done" directly to that downstream consumer's parsing logic, rather than just asserting a policy, is the right level of justification.

**The retry-scope decision (one deterministic retry, no ported triage) is well-argued and correctly scoped by actually reading `treadleengine.Engine.runRound`** rather than assuming the machinery would be trivially reusable — the discussion states plainly that the retry/triage/artifact-naming logic lives in `treadleengine`, unexported, and chooses to reimplement a minimal slice rather than extract-and-share, with the double-payment argument (`treadle` is itself heading to zero consumers) as the reason not to invest in extraction. Sound.

**`ClusterExclude` filtering inside `validate`, after `ResolveFan`, is the correct insertion point** — the discussion verifies (`engine.go:101,176`) that `clusterLenses` is the single value both prompt composition and the exact-N fork audit consume, so filtering anywhere else risks the audit demanding forks for lenses the prompt never named. The fail-loud-on-full-exclusion choice is consistent with `burlerengine`'s existing `ErrClusterForksMissing` posture, correctly cited as precedent rather than invented fresh.

**The focus-file contract living in `shedadapters` (not `burlerengine`, not deferred to the `Bouncer` task) is the right ownership call** — `burlerengine` is domain-agnostic and knows nothing about `Shed`, and deferring the format would ship this task's reader against an undefined shape. Fail-safe parsing (degrade to "no directive" on any malformed input, never fail loud) is argued consistently with the same rationale `treadleengine`'s own `PreRoundTargeting` doc uses — the file's author is an LLM, and losing one directive costs only trimming, not correctness.

**The cancellation exception (a completed round survives cancellation even though its `Shed` verdict is `Stuck`, never `Done`) is a real edge case correctly flagged as needing explicit doc treatment** — the shared package rule's current wording enumerates `Done` outcomes, and this producer's own success case is `Stuck`. The discussion is right that this needs to be stated explicitly rather than left for a reader to infer.

No open decision looks wrong. The **Out** scope list is disciplined — it explicitly excludes the `Bouncer` itself, any `loomshed` wiring, retiring `perch`/`treadleengine`, porting treadle's triage/ledger/milestone machinery, any `shedengine` change, any prompt/rubric content, and any new CLI verb — each one a real temptation this task could have scope-crept into, each one correctly deferred to its own named roadmap item.

## Verdict

Sound. Nothing here should block moving to Plan. The one citation to fix if convenient: `singlellm.go:107` should point at the `OutcomeDied`/`OutcomeTimeout` case a few lines below (~113), not the `OutcomeAsking` case — not worth a discussion round on its own.
