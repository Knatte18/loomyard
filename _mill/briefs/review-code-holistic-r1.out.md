MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-25
```

## Findings

None. This is a thorough, high-fidelity implementation across all five batches.

## Verification notes

- **Batch 1** (`internal/shuttleengine/spec.go`, `wait.go`, `rundir.go`, `run.go`, `doc.go`): `Spec.AwaitOperator` is placed immediately after `Interactive` with the full required doc comment (orthogonality rationale, accepted failure mode). `Wait`'s poll branch correctly drops `OutcomeAsking` only when `AwaitOperator` is true, logs via `logger.Info`, and leaves every other exit (`OutcomeDone`, liveness branch, deadline) untouched. `RunState.Outcome`/`runOutcomeRunning` match the three-writable-states contract exactly; `Start` seeds `"running"`, `finalize` writes the classification for every terminal outcome **before** the fork-audit block (verified at `wait.go:411-424`, ahead of `AuditForks` at 425), with the failed-write path degrading to `logger.Warn` only. Tests cover the full defect-A matrix (`wait_test.go`) and the `Start`-seeds-`"running"` assertion (`run_test.go:160-184`).
- **Batch 2** (`attach.go`, `run.go`, `wait.go`): `Runner.Attach` implements the three-phase collect/disposition/combine algorithm precisely as specified in `attach-lives-in-shuttleengine`/`candidate-evaluation-order`/`mechanism-failures-do-not-attach-and-do-not-blindly-respawn`/`leftover-run-dir-from-a-completed-run` — zero-candidates short-circuit before any reed read, `reedengine.LoadState` read directly (never via `Status()`) for the absent/unreadable gate, `strand.Live`+`Outcome` dispositioning with the dead-pane/binding-cleared split, leftover-then-age tie-breaker, and error-dominates-then-multiplicity-then-single-match-then-none combine order. Run reconstruction sets `offset:0`, `attached:true`, a fresh `now+Timeout` deadline, and the caller's own normalized spec — matching `attach-reconstructs-the-run-explicitly` field-by-field. `run.go`'s `clock`/`attached` supporting edits are surgical and correctly wired. `attach_test.go` is exceptionally thorough (22 top-level tests covering every enumerated case in the batch's requirements, including the `reed.CallLog` proof that `Status()` is never consulted for the absent-state question).
- **Batch 3** (`shedadapters/singlellm.go`, four fakes): `Shuttle` interface widened by `Attach` with the required doc rationale; all four implementors (`shedadapters`, `shedrecipe`, `shedbuild`, `loomrecipe` fixtures) updated consistently, three minimal always-not-found and `loomrecipe`'s scriptable per the stated batch split. `Call`'s reordering (`entryErr` → spec → relative-path reject → probe → map-or-archive-then-run) matches `probe-before-archive` exactly, and `mapOutcome` is a single shared switch reached from both paths — no duplication found anywhere in the package (`outputFilesSetEqual`/`allOutputFilesExist` are also each declared exactly once).
- **Batch 4** (`loomengine/template.yaml`, `config.go`, `discussion.go`, `loomcli/wiring.go`): `discussion_interactive` lands in the template and `Config` together (`DiscussionInteractive bool`, tag `discussion_interactive`), placed after `DiscussionTimeoutMin` in both. `DiscussionSpec` sets `AwaitOperator: !autonomous` beside `Interactive: !autonomous` with the required orthogonality comment. `wire()`'s `!loomCfg.DiscussionInteractive` replaces the literal `true`, its comment is genuinely rewritten (not edited-around), and `PlanSpec`'s sibling comment is untouched. `wiring_test.go`'s two-case table and `config_test.go`'s malformed-spec fixtures (all three) correctly carry the new key.
- **Batch 5** (`loomrecipe` regression pair, docs): `fakeLoomShuttle.Attach` is scriptable with `attachFound`/`attachRole`/`attachResult`/`attachErr`/`attachCalls`, defaulting to not-found. `TestResume_DiscussionValidateBounceRespawnsDiscussionWrite` and `TestResume_LiveMatchingRunAttachesInsteadOfRespawning` are the exact regression pair the discussion calls for, with correct assertions on `discussionRunCalls`, `attachCalls`, and (for the attach case) that the original files were never archived. `manifest/designs/loom.md`'s crash-recovery section is rewritten as required — heading text preserved (anchor intact), two-part resume rule, ladder steps 2/3 named concretely, interactive-mode trap replaced with its resolution, accepted-residual window stated plainly, Graceful-pause cross-reference updated. `shed.md`'s restatement and `docs/overview.md`'s key list/reconcile-`--apply`/two-modes sentence both updated. `manifest/roadmap.md` moves the item to Done with the correct anchor link and adjusts the now-singular "real LLM producers" preamble.

No cross-batch contract mismatches, no out-of-plan files, no constraint violations (Shuttle Provider-Seam, Told-Geometry, Lyxdirs Single-Declarer, Config Strictness, Shed Producer-Seam all hold on inspection), and no global utility duplication found.

## Verdict

APPROVE
Implementation matches the plan and every discussion decision precisely across all five batches; no findings.
MILL_REVIEW_END
