MILL_REVIEW_BEGIN
# Review: Seeded driver choice: ly-drive strand as the child's driver — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-20
```

## Findings

No findings. The implementation was checked batch by batch against every card's `Requirements:` and against the Driver Choice Single-Site Invariant, the mechanism-ships-before-the-gate-is-lifted ordering, and the other Shared Decisions.

Specific points verified in depth, not merely spot-checked:

- Batch 1 (`shuttleengine`): `Spec.NameOverride` is forwarded verbatim into `AddSpec` and left untouched by `validate`; `Run.RunDir()` matches the shape of `StrandGUID()`. Both are covered by the load-bearing tests the plan calls out.
- Batch 2 (`loomengine`): `Driver` config key validated only when non-empty, template comment placed after the friction pair, `ResolveDriver` resolves through the registry (not a raw copy) with the empty-string off/default arm handled before `Parse`.
- Batch 3: `loomcli.BootstrapVerb`/`battencli.BootstrapVerb` declared as specified; `shedcli`'s table sync meta-test and the both-arms-present guard are present; the mid-batch `wiring_test.go` nine-key fix is applied.
- Batch 4 (the core branch): `mustSpawnDriver`'s conjunction and its two mixed-row tests, `resolveDriverStrandAction`, `driverReportPath`'s two-seams composer with the frozen-clock/random-suffix test, `driverPrompt`/`AutonomousDriveStepCap`, `driverSpec`'s full field-by-field pin, the `driverStarter`/`driverPaneProbe` seams wired in `wiring.go` over the *same* runner/reed instances (no second construction), `awaitDriverPane`'s bounded poll, `seedAndCommitBootstrap`'s read-through-existing-seed fix (card 14) with both call sites updated in the same commit, the driver branch in `start.go` reading the seed's driver from the already-returned value (no second `ReadSeed`), all six bootstrap-lock-release failure sites individually tested, and the never-reaches-the-handshake assertion. The Driver Choice Single-Site Invariant is recorded in `CONSTRAINTS.md` beside the other shed invariants, with its AST-scan tripwire in `bootstrap_test.go` correctly scoped to the `Driver` field selector (not the constants) and correctly excluding `internal/shedrun`.
- Batch 5: `ValidateDriver`'s two-arm switch and message naming both values; `shedcli`'s `writeSeed` capability gate ordered driver-vocabulary-first, capability-second, driven by a test-local table rather than a name comparison; batten's own `--driver` still refuses `llm` while `--child-driver` accepts it, with the four-way matrix test present.
- Batch 6: the skill's `## Autonomous driver` section states all four changes and no more; `AutonomousDriveStepCap` is pinned against the skill's fixed phrase via an anchored (not file-wide) regex match in `drivercap_test.go`.
- Batch 7: the smoke test drives three successive bootstraps and asserts the strand count at each (1, 1, 1) including the corpse-then-relaunch case; the pre-existing `smoke_test.go`/`smoke_bootstrapwiring_test.go` breakage from the earlier `LoomStatusFile`-family rename is fixed (confirmed no stale references remain). The integration test asserts the launch returns before the stub's settle delay elapses, and that the report lands under the ephemeral scratch directory.
- Batch 8: `start.go`'s help text describes the seed-driven choice per-path; `manifest/designs/seeded-shed.md` records the as-built driver section with both accepted residuals stated; `manifest/roadmap.md` moves the item to Done with a working link; `tools/sandbox/SANDBOX-CORE-SUITE.md` records the disposition in prose only, and `cmd/lyx/sandbox_coverage_test.go` is untouched as required.

Cross-batch contracts hold: `loomengine.DriverSettings`/`ResolveDriver` (batch 2) is consumed correctly by `driverSpec` (batch 4); `loomcli.BootstrapVerb`/`battencli.BootstrapVerb` (batch 3) are consumed by `shedcli`'s table and by batch 5's gates with no literal re-spelling anywhere; `shuttleengine.Spec.NameOverride`/`Run.RunDir` (batch 1) are consumed by `driverSpec`/`driverlaunch.go` (batch 4); `AutonomousDriveStepCap` (batch 4) is consumed by the skill and by batch 6's drift test. No out-of-plan files were found — every source file in the manifest maps onto a batch's `Context:`/`Edits:`/`Creates:` list. No global-utility duplication beyond the already-documented, deliberate small mirrors (`battencli.battenDriver` / `shedcli.resolveSeedDriver`), each with its own comment explaining why it isn't shared (import-cycle avoidance, per-module capability declaration).

## Verdict

APPROVE
Implementation matches the approved plan faithfully across all eight batches, with no constraint violations found.
MILL_REVIEW_END
