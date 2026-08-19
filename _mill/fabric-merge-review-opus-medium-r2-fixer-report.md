# `fabric merge` — fixer report, round `opus-medium-r2`

Companion to `_mill/fabric-merge-review-opus-medium-r2.md`.
Five findings recorded (R1, R2, R3, R5, plus R4 which was withdrawn on evidence during the review rather than carried into Job 2). **All five are closed. Nothing is deferred.**

## What was implemented

### R1 (BLOCKING) — `MergeStart` misclassified an empty-result merge as `AlreadyUpToDate` — commit `cb3ffe1e`

`internal/gitrepo/merge.go` — the success-path classification now probes `MergeHeadPresent()` and classifies a live `MERGE_HEAD` as `MergeStaged`, ahead of the HEAD-moved test:

```go
case staged || mergeHeadPresent:
	return MergeStaged, nil
```

A real, non-fast-forward merge whose result tree equals HEAD's own tree stages nothing and moves no HEAD, so the previous two-signal classifier called it `MergeAlreadyUpToDate` while git had written `MERGE_HEAD`. `git merge --squash` writes no `MERGE_HEAD`, so the probe is vacuous there and a squash with an empty result keeps reporting `AlreadyUpToDate` — which is the honest answer, and which is also what makes residual 1's test route survive this fix.

Docs in the same commit: `internal/gitrepo/merge.go`'s `MergeStart` doc comment, `internal/gitrepo/doc.go`'s Client Boundary paragraph, and a new "**"Already up to date" means git had nothing to do**" paragraph in `internal/fabricengine/doc.go`'s merge surface.

### R2 (BLOCKING) — `MergeAbort` discarded a conclude-commit that had already landed — commit `e30253f9`

- `internal/fabricengine/mergeerrors.go` — new closed-set member `mergeReasonConcludeLanded = "merge conclude already landed"`.
- `internal/fabricengine/mergeguards.go` — new `concludeLandedReason(f, st)` and its per-side predicate `sideConcludeMayHaveLanded`, both sides evaluated unconditionally per the aggregation rule.
- `internal/fabricengine/mergelifecycle.go` — `MergeAbort` evaluates the precondition after loading the record and **before** taking the lock or touching either checkout.

The predicate is deliberately wider than the recorded conclude SHA:

```go
if committed != "" { return true, nil }
if outcome != mergeOutcomeStaged && outcome != mergeOutcomeConflicted { return false, nil }
head, err := repo.CurrentSHA()
return head != start, nil
```

The record learns a side's conclude SHA only after `git commit`, the `CurrentSHA` read, **and** the record re-save have all succeeded, so a failure at either of the last two leaves a landed commit the record never mentions — four of the seventeen dangerous sites in the review's enumeration. Reading HEAD closes all of them, and the failure direction over-refuses rather than destroying.

Recovery is `MergeContinue`, which skips a side whose committed SHA is recorded and is therefore idempotent. This is the exact mirror of round 1's F1, and `doc.go` now says so, including the plain-git last resort when the underlying git failure cannot be fixed.

Docs and vocabulary in the same commit: `mergevocab_test.go` + `mergeerrors_test.go` pin-lists (the closed set's same-commit rule), `doc.go`'s lifecycle-quartet paragraph and a new "**And not every crash is abortable**" paragraph, and `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s new F20.

### R3 (NIT) — dangling `mergeReasonNoMergeInProgress` — commit `f81d84df`

Deleted from the closed set and from all three test pin-lists in the same commit. The const block now states the membership rule that keeps it deleted: the set is for **aggregatable preconditions**, and a terminal standalone disposition belongs in the typed error surface — which is where `*ErrNoMergeInProgress` already was, carrying the same words and doing the actual work.

### Residual 1 — `bothSidesAlreadyUpToDate` had zero coverage — commit `a5b1f117`

`TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord` in `internal/fabricengine/mergecrucible_integration_test.go`.

**No production test seam was needed.** The prompt allowed one; the route I found is deterministic, single-process, sleep-free and goroutine-free: `Merge(source, {Squash: true})` where the source is not an ancestor of HEAD on either side — so the pre-lock probe's `IsAncestor` is false and it cannot early-return with its hardcoded `AlreadyUpToDate: true` — but where the squash result tree equals HEAD's tree, so both post-lock `MergeStart` outcomes come back `up_to_date`. The test asserts the non-ancestor precondition with `t.Fatal`, so the fixture cannot silently degrade into the pre-lock case.

### R5 (LOW, doc-only) — the `CheckoutDetached`/`RestoreBranch` justification — commit `6bdabe9c`

`doc.go` claimed the attached-HEAD precondition closes the harmful direction. It does not — that precondition stops a merge *starting* while detached and says nothing about detaching a checkout that is already mid-merge. The doc now states what actually holds (git refuses `checkout --detach` while unmerged entries exist, closing the long window) and names the narrow resolved-but-not-concluded window as an accepted hazard belonging to the caller. The conclusion — these stay unguarded — is unchanged.

## Deferred

**Nothing.** No finding was left unfixed, and nothing was marked NOT-FIXED-THIS-ROUND: none of the five needed a design decision I could not make or a capability I do not have.

## False-green proof for every new test

Each proof mutated production code to reintroduce the exact defect, watched the test fail **at the intended assertion**, then restored and confirmed an empty diff.

| # | test | sabotage | observed failure | restored |
|---|---|---|---|---|
| 1 | `TestMergeStart_EmptyResultTree_ClassifiedStagedNotAlreadyUpToDate` (gitrepo, `-tags integration`) | `case staged \|\| mergeHeadPresent:` → `case staged:` | `MergeStart(feature, false) outcome = 3; want 0` — i.e. `MergeAlreadyUpToDate` where `MergeStaged` is required. The squash subtest correctly stayed green, proving the fix does not over-reach. | `git diff` empty |
| 2 | `TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned` (fabricengine, `-tags integration`) | same sabotage | Seven assertions fired, reproducing every harm the review documented: `MERGE_HEAD is live in the warp checkout…`, the same for weft, `AlreadyUpToDate = true`, `Committed = false`, both HEADs unchanged, and `MergeAbort() error = … (*fabricengine.ErrForeignMergeState); want *ErrNoMergeInProgress`. | `git diff` empty |
| 3a | `TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded` (fabricengine, `-tags integration`) | guard disabled (`if false && len(reasons) > 0`) | **both** arms fail: `MergeAbort() error = <nil> (<nil>); want *fabricengine.MergeGuardError` | `git diff` empty |
| 3b | same test | guard kept, but the predicate narrowed to the recorded SHA alone (the HEAD-moved clause short-circuited to `false`) | **only** the `InvisibleConcludeTheRecordNeverLearnedAbout` arm fails; `RecordedConcludeSHA` still passes. This is the companion direction: it proves the second clause is load-bearing and the two arms are not testing the same thing. | `git diff` empty |
| 4 | `TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord` (fabricengine, `-tags integration`) | `bothSidesAlreadyUpToDate()` hardwired to `return false` — the orchestrator's own residual-1 measurement | `Merge(feature, squash).AlreadyUpToDate = false; want true`. **Decisive**: under that sabotage this is the ONLY failing test in the entire `-tags integration` `fabricengine` tier (`--- FAIL` appears exactly once across a full 45 s run), and the pre-existing lookalike `TestMergeCrucible_ResultFlagsDescribeWhatHappened` still passes — which is the proof gap itself, now inverted. | `git diff` empty |
| 5 | R3's deletion (`TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree`) | re-added `"no merge in progress"` to the pinned `want` list without a matching constant | `closed guard-reason set has 8 members; want exactly 9 -- update this test's pinned list in the same commit as any change to the set` | `git diff` empty |

## Determinism proof for the new tests

Git and filesystem state at the edges are asynchronous, so the new tests were proven under repetition and load rather than run once. No test sleeps or assumes synchrony; every one waits on real state (`CurrentSHA`, `MergeInProgress`, an on-disk record read) rather than on elapsed time.

```
go test -tags integration -count=15 -timeout 20m -run '<the three new fabricengine tests>' \
        ./internal/fabricengine/...                                    -> ok 15.815s

# six concurrent processes, each -count=5, across both packages (30 runs of each test under
# self-inflicted load)
for i in 1..6: go test -tags integration -count=5 -run '<four new tests>' \
        ./internal/fabricengine/... ./internal/gitrepo/...             -> 6/6 exit 0, 12/12 ok
```

Zero flakes.

## Exact test commands, with tags and results

**Baseline, before any edit** (recorded in the review report; repeated here for the before/after pair):

```
go build ./...                                                                     clean
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...  clean
go test -count=1 <the four packages>                        ok / ok / ok / ok
go test -tags integration -count=1 -timeout 30m <three pkgs> ok 43.814s / 4.340s / 2.371s
```

**Final, after every fix:**

```
go build ./...                                              clean
go vet ./...                                                clean  (whole repo, not just the merge surface)

go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... \
        ./internal/gitrepo/... ./cmd/lyx/...
  ok internal/fabricengine 0.516s   ok internal/fabriccli 0.007s
  ok internal/gitrepo 0.005s        ok cmd/lyx 1.429s

go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... \
        ./internal/fabriccli/... ./internal/gitrepo/...
  ok internal/fabricengine 44.544s  ok internal/fabriccli 3.652s  ok internal/gitrepo 1.908s

go test -count=1 ./...                                      whole repo, zero non-ok packages
```

The whole-repo hermetic sweep is there because R1 changes a `gitrepo` primitive other modules also call; nothing outside the merge surface moved.

The sandbox coverage guard (`TestSandboxCoverage_AllModulesCoveredOrExcluded`, `cmd/lyx`) stayed green across the `SANDBOX-FABRIC-SUITE.md` edit, in the same commit as the behaviour it documents.

## Live re-drive against the redeployed binary

`./deploy-dev` was re-run after the last source change (`Deployed lyx @ 6bdabe9c`), and every Job-1 scenario was re-driven on **freshly built hubs** — never a hub a previous destructive scenario had already mangled.

| scenario | before | after |
|---|---|---|
| R1, `merge-in` of an empty-result merge (`hub1f`) | `already_up_to_date:true, committed:false`, live `MERGE_HEAD` in both checkouts, record deleted, all three merge verbs then refusing with `ErrForeignMergeState` | `already_up_to_date:false, committed:true` with four honest mutations, **no `MERGE_HEAD` on either side**, both HEADs advanced, both worktrees clean, and `merge --abort` correctly reporting `no merge in progress` |
| R2, abort after a half-landed conclude (`hub2f`) | `merge --abort` → `ok:true`, warp conclude commit **discarded**, record deleted | `merge --abort` → `merge preconditions failed: merge conclude already landed`, **zero mutations**, warp conclude commit intact, record retained |
| R2 recovery (`hub2f`) | n/a | hook removed, `merge --continue` → `ok:true, committed:true` with a single `merge_committed` mutation for the weft side only (warp correctly skipped), record deleted, both sides carrying their merge commit |
| Residual 1's squash route (`hub3f`) | `already_up_to_date:true` | unchanged — `already_up_to_date:true, committed:false`, no `MERGE_HEAD`, no record, no HEAD movement. R1's fix correctly does **not** reclassify squash. |
| Round 1's deferred sibling race (`hubAf`, `hubBf`) | 0/75 | 0/75 again (25 interleaved + 25 sequential control on one hub, 25 interleaved on an independent second hub) |
| Ordinary-flow regression under hostile git config (`hubR2`) | n/a | `core.editor="sleep 600"`, `merge.ff=only`, `commit.template=/nonexistent` on both sides. A real conflicted `merge-in` returns the `conflicts` envelope; `merge --abort` restores both sides; a resolved `merge --continue` lands `committed:true`; a genuine second `merge-in` reports `already_up_to_date:true`; `merge --abort` with nothing live reports `no merge in progress`. **Every command returned well inside a 120 s timeout — no hang.** |

## Changed files

Production:

- `internal/gitrepo/merge.go` — R1's classifier probe and its doc comment
- `internal/gitrepo/doc.go` — R1
- `internal/fabricengine/mergeerrors.go` — R2's new reason, R3's deletion, the membership rule
- `internal/fabricengine/mergeguards.go` — R2's `concludeLandedReason` / `sideConcludeMayHaveLanded`, file header
- `internal/fabricengine/mergelifecycle.go` — R2's guard in `MergeAbort` and its doc comment
- `internal/fabricengine/doc.go` — R1, R2, R5

Tests:

- `internal/gitrepo/merge_integration_test.go` — R1 (both directions)
- `internal/fabricengine/mergecrucible_integration_test.go` — R1 at the pair level, R2 (two arms), residual 1, and three new helpers
- `internal/fabricengine/mergevocab_test.go`, `internal/fabricengine/mergeerrors_test.go` — closed-set pin-lists, moved in the same commits as the set

Docs:

- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — new scenario F20 covering both live behaviours

Reports:

- `_mill/fabric-merge-review-opus-medium-r2.md`, `_mill/fabric-merge-review-opus-medium-r2-fixer-report.md`

`manifest/roadmap.md` was **not** touched, per `CLAUDE.md`. `CONSTRAINTS.md` was not touched: nothing here introduces a new cross-cutting invariant — R1 and R2 are corrections inside surfaces `doc.go` already owns.

## Commits, in order

```
aa7fcd49  fabric: review notes — open r2 review report
7e251055  fabric: review notes — findings R1, R2, R3, R4
75b8b6b8  fabric: review notes — r2 review COMPLETE (residual-2 enumeration, residual-1 route, race non-repro, R4 withdrawn, R5 added)
cb3ffe1e  fabric: fix R1 — a merge whose result tree equals HEAD is staged, not already-up-to-date
f81d84df  fabric: fix R3 — delete the dangling mergeReasonNoMergeInProgress from the closed guard-reason set
e30253f9  fabric: fix R2 — MergeAbort refuses an attempt whose conclude already landed
a5b1f117  fabric: close residual 1 — a test that genuinely reaches bothSidesAlreadyUpToDate
6bdabe9c  fabric: fix R5 — state what actually closes the CheckoutDetached exemption
```

Nothing was pushed.

## Teardown

Every hub, fixture repo and temp repo this round created was removed: `hub1`, `hub2`, `hub3`, `hubA`, `hubB`, `hub1f`, `hub2f`, `hub3f`, `hubAf`, `hubBf`, `hubR`, `hubR2`, `exp1`, `exp2a`, `exp2b`, `exp2c`. Only the driving scripts and the parallel-run logs remain in the scratch directory.

No stray `git` processes. The one running `lyx` process belongs to a different worktree (`reed-shuttle-crucible-hardening`) and is not mine.

`git status` in this worktree shows exactly one modified file — `_mill/fabric-merge-review-HANDOFF.md`, the orchestrator's own running note. I did not create, read, edit or stage it; every `git add` this round named its paths explicitly, and none named that file.

## Merge readiness

**READY.** Both BLOCKING defects are fixed, each pinned by a sabotage-proven integration test and re-driven live end to end on a fresh hub against the redeployed binary. Residual 1's proof gap is closed by a test that is the only thing in the tier to fail under the orchestrator's own sabotage. Residuals 2 and 3 are discharged — the enumeration is in the review report as a full 45-row table, and its one dangerous class is exactly what R2's guard closes. Every gate is green at both tiers, including a whole-repo sweep.
