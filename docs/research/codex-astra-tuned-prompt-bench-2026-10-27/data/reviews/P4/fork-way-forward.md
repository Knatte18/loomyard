- Card 3 owner-payload refusal → `lyx adopt <slug>`; progresses for an existing task.
- Card 3 unreachable owner write → retry when origin answers; progresses.
- Card 3 dirty/diverged Bolt or rejected push → reported branch repair, then retry; progresses.
- Card 3 deferred push → `lyx board sync`, then retry; progresses after the lock holder releases.
- Card 8 foreign-owner start/resume/claim → adopt from the prime; progresses.
- Card 8 unreachable start or owner-writing resume → retry after connectivity returns; progresses.
- Card 9 foreign-owner shed resume → adopt from the prime; progresses.
- Cards 10–11 owner pause → adopt, lift the labelled pause and resume; progresses.
- Card 12 parent-only, run-only and combined ownership halts → the corresponding adopt forms; progresses subject to the explicitly documented local-commit/live-pair choices.
- Card 12 past-merge recovery after a transient parent-push failure → re-step Finalize and reach the parent push; **does not reliably progress**, finding below.
- Card 12 adopter-first repair → remove the duplicate squash, then remove the pair while leaving its owner pause; the proposed repair hook misses an applicable re-step path, covered by the same finding.
- Card 13 failed/unwired orch owner pause → rerun `lyx orch start`; **does not progress with the prescribed installation order**, finding below.
- Card 16 local-commit/live-pair stop → `--keep-local`, or `--drop-local` for a non-live pair; progresses.
- Card 16 leftover local branch → remove the leftover branch and retry adopt; progresses.
- Card 16 failed push/lost ownership → repair/retry adopt; progresses.
- Card 16 standing ordinary pause → `lyx resume <slug>` or hub-wide `lyx resume`; progresses.

**Finding: past-merge retry still traverses guards that prevent the promised push and repair**

- Severity: MEDIUM
- Class: decision
- Location: `_lyx/plan/12-finalize-owner-checks.md`, Intent paragraphs for `Finalize.Call`, the past-merge exemption and `adopterFirstRepair`.
- Summary: Skipping the new owner checks alone does not make an already-merged Finalize re-step reach its push.
- Evidence: `internal/landingshed/finalize.go:199` calls `holdParent` before the step 1b3 replacement; `internal/landingshed/parentguard.go:123` fetches parent tips, and `internal/fabricengine/parenttips.go:46` returns a fetch failure. Thus an unreachable origin on the re-step returns before the proposed past-merge exemption and push. Separately, after the adopter has landed first, the old parent and origin contain distinct squashes: `internal/fabricengine/merge.go:389` invokes `syncedToUpstreamReason`, whose divergence predicate is in `internal/fabricengine/mergeguards.go:190`, before the later push. Card 12 leaves that parent-side Merge call in place and attaches the duplicate-squash repair only to `pushParent`.
- Consequence: The D6/D7 recovery promised for the transient re-step can stop before `pushParent`; the adopter-first case can lack its required reset/rebase/removal instructions.
- Proposed direction: Specify the complete past-merge continuation through the existing parent-tip and merge guards, with reaching tests for unreachable origin and adopter-first divergence during an in-process re-step.

**Finding: rerunning orch start cannot repair its unwired owner pause**

- Severity: MEDIUM
- Class: consistency
- Location: `_lyx/plan/13-wake-and-orch-owner-pause.md`, Intent paragraphs for `InstallOwnerPause`, `Command` and the unwired-pause warning.
- Summary: Copying the installed pause into `orchCLI` during command construction happens before the root pre-run installs it.
- Evidence: `cmd/lyx/main.go:119` declares the root `PersistentPreRunE`; `cmd/lyx/main.go:147` constructs the subcommand tree, including `orchcli.Command()` at line 161. Card 13 explicitly installs the pause in the former and copies it into the receiver in the latter.
- Consequence: A fresh production process captures nil, skips the owner pause and advises rerunning `lyx orch start`; every fresh rerun repeats the same construction order.
- Proposed direction: Resolve the installed callback at execution time, or install it before constructing `orchcli.Command`; cover the actual initialization order rather than only receivers with manually populated fake callbacks.

Tool-call record:

1. Read board CLI/sync, batten parent-lock wiring and tender lift/resume implementations; these recovery callees were absent from the inherited definition excerpts.
2. Searched shed halt persistence, merge helpers and command construction; needed to establish how owner halt reasons survive into resume and identify later merge guards.
3. Read parent-side Merge, fixture cloning and removal entry-point references; needed the continuation after the new owner checks and the removal route.
4. Read `mergeInStep`, `parentGuard`, fabric remove and teardown sequencing; these were the next recovery-path callees beyond the manifest.
5. Read `ReadParentTips`, divergence guards, teardown quiet checks and the network gate body; needed exact behavior for unreachable-origin and adopter-first retries.
6. Read `checkRunStatus` and searched remaining resume/removal edges; needed to check the statuses accepted by adopt’s delegated resume and existing recovery boundaries.
