# fabric `weft-is-never-merged` diff — fixer report (round `opus-high-r1`)

Companion to `_mill/fabric-review-opus-high-r1.md`.
Every finding recorded in that review was fixed in this round — 13 of 13, all severities including NIT.
Nothing was deferred, and nothing was classified LARGE / NOT-FIXED-THIS-ROUND.

## What was implemented

Eleven commits, one per fix, on `fabric-crucible-hardening`. Nothing pushed.

| Commit | Finding(s) | What changed |
| --- | --- | --- |
| `df185c8b` | F5 (LOW) | `merge.go` file header no longer describes `MergeIn` as merging into "the current pair's own warp **and weft** checkouts" |
| `84687354` | F6 (LOW) | `ErrWarpDirty` no longer promises the weft was fast-forwarded — the arm became non-fatal in the same diff |
| `79947900` | F1 (MEDIUM) | `PushAnchored`'s doc no longer asserts an `errors.Is(err, gitrepo.ErrPushRejected)` discrimination that exists in no caller |
| `694029c6` | F3 (MEDIUM) | `doc.go`: the weft conflict list is not "permanently empty" — `MergeContinue` populates it from a foreign weft conflict |
| `d16b8adc` | F4 (MEDIUM) | `doc.go`: the weft has not lost ALL power to block a merge — two weft-reading refusals survive on purpose |
| `769ec82d` | F10 (NIT) | One `saveMergeState` at `MergeIn`/`Merge` start, not two; `WeftOutcome` moves into the struct literal. **+1 integration test** |
| `4c00a8e3` | F9 (LOW) | **+1 integration test**: the narrowed weft guards driven in combination, not one at a time |
| `c974f8ae` | F2 (MEDIUM), F11 (NIT), F13 (NIT) | `commitStatusFailureDisposition`: a commit failure a re-probe explains as a live merge takes the skip disposition instead of halting the run. **+2 unit tests**, `loom.md` updated |
| `6045f34c` | F12 (NIT) | `PushResult`'s doc states the invariant instead of a roll-call that had gone stale twice |
| `79366900` | F7 (LOW) | **+1 integration test**: `cleanup`'s reserved `--force` changes no verdict |
| `0135044f` | F8 (LOW) | **+1 integration test file (5 tests)**: the CommitStatus seam driven against a real fabric pair |

### The one behaviour change: F2

Everything else in this round is documentation or test coverage. F2 is the only production-logic change, and it is deliberately narrow.

`newCommitStatusSeam` probed `MergeStateActive` with no lock held and then committed. Nothing fabric can hold serialises against an operator running plain git inside the weft checkout — which the Fabric Git Invariant's own carve-out permits — so a `MERGE_HEAD` can become live inside that window. When it did, the path-scoped commit failed on git's own `fatal: cannot do a partial commit during a merge`, the commit-hard-errors disposition fired, and the **whole loom run died** — in precisely the situation skip-while-mid-merge exists to absorb.

The fix re-probes on a commit failure and takes the skip when a merge is live now, or when the re-probe itself is unreadable. Every other commit failure keeps the hard-error disposition unchanged: a git fault on the run's own bookkeeping with no merge to explain it is still real infrastructure breakage.

**What the fix deliberately does not close, and why.** `gitrepo.StageAndCommit` runs `git add` before `git commit`, so a commit that loses the race has already staged the status file into the operator's merge index, and their own conclude carries it. Closing that would mean holding a lock the operator does not take — or moving the mid-merge probe inside `CommitWeftPaths`, which is outside this diff's review scope and changes behaviour for three other callers. The residual is stated in `commitStatusFailureDisposition`'s doc comment and in `manifest/designs/loom.md` rather than papered over.

### Behaviour explicitly NOT changed, and why

Three findings looked like they might want a code change and correctly did not get one:

- **F1**: the seam warns and continues on every push error. That is what the SPEC's `commit-hard-errors-push-warns` decision asked for — an offline laptop must not kill an autonomous run — so the doc comment was wrong, not the code.
- **F3/F4**: `MergeContinue` still refuses on a weft conflicted index, and `foreignMergeStatePresent` still refuses on weft merge state. Removing either would let a legacy-record resume (`WeftOutcome: "staged"` from a pre-change binary, which `concludeMergeSides`' retained weft arm still handles) conclude a weft merge over unresolved conflicts. Docs were corrected; behaviour was left alone.
- **F7**: `Topology.Cleanup`'s `force` parameter and the `--force` flag both stay. Removing either churns the CLI help tree and the verb table for no behavioural gain; the actual gap was that nothing pinned the reserved-ness, so a test now does.

## Documentation updated in the same change as the fix it documents

- `internal/fabricengine/doc.go` — the "merge surface" and merge-precondition sections (F3, F4).
- `internal/fabricengine/merge.go`, `pull.go`, `pushanchored.go`, `weftgit.go` — file-header and symbol doc comments (F5, F6, F1, F12).
- `internal/loomcli/wiring.go` — the seam's disposition enumeration and the new failure-disposition helper (F2, F11).
- `manifest/designs/loom.md` — the per-transition seam paragraph: the probe-failure skip, the unlocked-probe second half, and the `git add` residual (F2, F13).

No change was needed to `manifest/designs/fabric-unified-view.md`, `manifest/designs/shed.md`, `docs/overview.md`, or `CONSTRAINTS.md`: no invariant moved, no module table changed, and `shed.md`'s seam paragraph was already accurate (verified live — see below).
`manifest/roadmap.md` was not touched, per the round's own rule.

## Tests added

| Test | File | Pins |
| --- | --- | --- |
| `TestWeftGuards_EveryRecordThisBinaryWritesIsResumable` | `internal/fabricengine/weftguards_integration_test.go` | Every record this binary persists carries a non-empty `WeftOutcome`, so `mergeAttemptIncompleteReason` can never fire on a fabric-written record (F10) |
| `TestWeftGuards_DirtyAndDetachedWeftTogetherStillMerges` | same | The narrowed guards in combination — dirty **and** detached weft, clean warp — still merge, weft byte-identical (F9) |
| `TestCleanup_ForceIsReservedAndChangesNoVerdict` | `internal/fabricengine/reconcile_stale_registration_test.go` | Full dry-run verdict set is identical for `force=false` and `force=true`; apply with force deletes exactly the orphan (F7) |
| `TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip` | `internal/loomcli/wiring_commitstatus_test.go` | A commit failure a live merge explains resolves as the skip, with exactly two probes and no push (F2) |
| `TestNewCommitStatusSeam_CommitFailsAndReProbeFails_TakesTheSkip` | same | An unreadable re-probe resolves the same way the unreadable pre-commit probe does (F2) |
| `TestCommitStatusSeam_Real_OrdinaryPathCommitsAndPushes` | `internal/loomcli/wiring_commitstatus_integration_test.go` (new) | Real weft commit under the transition's own message, touching the status path, nothing left unpushed (F8) |
| `TestCommitStatusSeam_Real_MidMergeSkipsWithoutTouchingTheMerge` | same | The skip commits nothing, stages nothing into the operator's index, leaves `MERGE_HEAD` intact (F8) |
| `TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting` | same | The lost race, deterministically, with a real commit and a real re-probe (F2, F8) |
| `TestCommitStatusSeam_Real_RejectedPushWarnsAndTheCommitStays` | same | A genuinely diverged weft remote warns; the local commit stays and stays unpushed (F8) |
| `TestCommitStatusSeam_Real_UnreachableRemoteWarnsToo` | same | A non-rejection push error warns identically — the property F1's doc got wrong (F8) |

### Sabotage proof

The F2 regression test was proven to actually catch the bug rather than merely pass beside it. Reverting the fix to the pre-fix `return err` and re-running:

```
--- FAIL: TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting (0.17s)
    seam(...) error = gitrepo: git commit: git commit -m "loom: Discussion-Write -> running"
    -- _lyx/loom/status.json: exit 128: fatal: cannot do a partial commit during a merge.;
    want nil
```

Restoring the fix returns the package to `ok`.

## Verification — exact commands and results

All from `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, after the last fix landed.

### Hermetic

```sh
go build ./...                                                        # clean
go vet ./...                                                          # clean, whole module
go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/loomcli/... ./internal/shedengine/... \
        ./internal/landingshed/... ./internal/loomrecipe/... ./cmd/lyx/...
#   ok fabricengine 0.411s | fabriccli 0.004s | loomcli 0.075s | shedengine 0.074s
#   ok landingshed 0.037s | loomrecipe 0.226s | cmd/lyx 0.866s
go test ./...                                                         # task-wide: zero non-ok lines
```

### Live integration

```sh
go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/landingshed/... ./internal/loomcli/... -count=1
#   ok fabricengine 37.292s | fabriccli 3.494s | landingshed 0.475s | loomcli 0.745s
```

### Concurrency amplifier (diagnostic, not a merge gate)

Compiled once, three concurrent copies of the fabricengine integration binary run with cwd set to the package directory (the source-scanning invariant tests read their own `.go` files, so a foreign cwd fails them spuriously — my first attempt made exactly that mistake and is recorded here so the next round does not repeat it):

```sh
go test -tags integration -c -o <scratch>/bin/fabricengine.test ./internal/fabricengine
# 3 concurrent copies, cwd = internal/fabricengine
#   copy 1 exit=0 | copy 2 exit=0 | copy 3 exit=0 | no data race or panic markers
```

Three concurrent copies of the new loomcli real-substrate tests likewise: `exit=0` each, no markers.

### Live driving — real `lyx` binary rebuilt from the fixed source

`./deploy-dev` is refused by this environment's command classifier, so the fixed source was rebuilt to a scratchpad binary (`go build -o <scratch>/bin/lyx ./cmd/lyx`) and every review-round scenario was re-driven against it, foreground, one command at a time.

| Scenario | Result after the fixes |
| --- | --- |
| Weft dirty + detached, warp-only `merge-in` | Unchanged: `ok:true`, warp advanced, weft HEAD/dirt/detachment all preserved |
| Warp conflict → weft commits mid-attempt → `merge --abort` | Unchanged: warp reset alone, in-flight weft commit survives, no `fabric-merge.json` residue |
| `cleanup` dry / `--apply` / `--apply --force` | Unchanged: orphan deleted under `--apply` alone; primary and unmanaged protected; `--force` inert |
| Weft conflicted index + `merge --continue` | Unchanged: `unresolved: ["_lyx/loom/status.json"]` — the behaviour F3 documents rather than removes |
| `Pull` with a locally diverged weft | Unchanged: `weft_pulled:false` inside `ok:true`, warp advanced, local weft commit preserved |
| **TOCTOU: `MERGE_HEAD` planted inside the probe→commit window** | **Fixed.** Was: `RUN outcome="" err=…cannot do a partial commit during a merge`. Now: `RUN outcome="done"`, logging `WARN skip(commit failed, merge went live after probe)` and then skipping the remaining transitions normally, with the operator's `MERGE_HEAD` intact |

The TOCTOU re-verification used the same instrumented harness as the review (a 3 s window between probe and commit, a background saboteur running `git merge --no-commit --no-ff` in the weft at t=1.0 s), with the fix's disposition mirrored into it.

### Independently re-confirmed while fixing

- `shed.md`'s ordering claim ("the status-file write has already happened and is durable" when the seam errors) holds: a real commit failure from a planted `index.lock` halted the run with `status.json` already on disk carrying the correct transition and history.
- `landingshed`'s parent-weft-byte-identical assertion holds under the full integration suite.

## Changed files

Production:

- `internal/fabricengine/merge.go` (F5, F10)
- `internal/fabricengine/pull.go` (F6)
- `internal/fabricengine/pushanchored.go` (F1)
- `internal/fabricengine/doc.go` (F3, F4)
- `internal/fabricengine/weftgit.go` (F12)
- `internal/loomcli/wiring.go` (F2, F11)

Tests:

- `internal/fabricengine/weftguards_integration_test.go` (F9, F10)
- `internal/fabricengine/reconcile_stale_registration_test.go` (F7)
- `internal/loomcli/wiring_commitstatus_test.go` (F2)
- `internal/loomcli/wiring_commitstatus_integration_test.go` — new (F2, F8)

Docs:

- `manifest/designs/loom.md` (F2, F13)

Reports:

- `_mill/fabric-review-opus-high-r1.md`
- `_mill/fabric-review-opus-high-r1-fixer-report.md`

## Deferred

Nothing.

Two things are worth naming as **known, disclosed residuals** rather than deferrals, because both are now documented in the code and neither is fixable within this diff's scope:

1. **The `git add` staging in F2's lost race.** Bounded to one extra staged path in a merge the operator is already resolving by hand. Closing it needs the mid-merge probe inside `CommitWeftPaths` under the weft write lock — a change to a file outside this diff, affecting three other callers — and even that cannot fully close a window against an actor who takes no lock.
2. **`sideRecordedMergeGone`'s squash exemption.** Pre-existing, documented in place, untouched by `ab99f531`, and explicitly out of this round's scope.

## Merge readiness

**Ready to merge.** The merge bar — correctness in the normal single-instance flow — was met before this round and is unchanged by it; the round found no BLOCKING defect. What it changed is that the diff's own documentation now survives contact with the substrate it describes, one real run-killing interaction is absorbed instead of fatal, and the three properties that were only pinned against stubs are pinned against real git.
