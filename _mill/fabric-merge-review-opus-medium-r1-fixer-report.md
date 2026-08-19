# `fabric merge` — fixer report (round `opus-medium-r1`)

Companion to `_mill/fabric-merge-review-opus-medium-r1.md`.
Every finding recorded in that review is fixed here unless the deferred section below says
otherwise, with a per-finding commit on branch `fabric-merge-crucible-hardening`. Nothing is pushed.

Gate commands used throughout (tags named explicitly, per the campaign rule):

```sh
go build ./...
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...
go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...   # tag: none (hermetic)
go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...   # tag: integration
./deploy-dev   # re-run before EVERY live re-verification
```

All new regression tests live in `internal/fabricengine/mergecrucible_integration_test.go`
(`//go:build integration`), following the existing `*_integration_test.go` pattern and the hubforge
Fabric-Fixture invariant. They reuse `newMergePairFixture` (`mergein_integration_test.go`) and
`openFreshFabric` (`mergein_recovery_integration_test.go`) rather than growing a parallel fixture
set.

---

## F2 — BLOCKING — refuse a merge while either checkout has a detached HEAD

**Commit** `a20dc2ca`.

**Implemented**

- `internal/gitrepo/merge.go` — new `(*Repo).HeadDetached() (bool, error)`. `CurrentBranch()` could
  not serve: it collapses detachment into an error indistinguishable from a genuine read failure.
  Like `ResolveSHA` it is a go-git read of on-disk state, so it stays off the gitrepo Client
  Boundary Invariant's pinned CLI list, and its godoc says so.
- `internal/fabricengine/mergeerrors.go` — new closed-set member
  `mergeReasonDetachedHead = "checkout is not on a branch"` (side-free, path-free, order-free).
- `internal/fabricengine/mergeguards.go` — new `detachedHeadReason(f)`, evaluating **both** sides
  unconditionally before combining, exactly as `pairDirtyReason` does, so the aggregated reason
  never reveals which side was detached.
- `internal/fabricengine/merge.go` — wired into both `MergeIn`'s and `Merge`'s guard aggregation,
  immediately after the dirty guard, still inside the strictly read-only guard stage.
- `internal/gitrepo/doc.go` — `HeadDetached` added to the admitted merge-surface list.
- `internal/fabricengine/doc.go` — new "Both checkouts must be on a branch" paragraph in
  "# The merge surface", naming why the asymmetry (orphan on one side, final-and-unabortable on the
  other) makes this a precondition and not a curiosity, and pointing at
  `internal/websterengine/integration.go` as the production driver of `CheckoutDetached`.
- `internal/fabricengine/mergevocab_test.go` — pinned closed-set list extended in the same commit,
  per the guards decision's same-commit rule.

**Deliberately NOT changed**: `MergeContinue`/`MergeAbort` are left unguarded against detachment.
They are the recovery half of the quartet; refusing there would strand a pair that got detached
mid-resolution, which is the opposite of what the guard is for.

**Test** `TestMergeCrucible_DetachedHeadRefused` — table-driven over `WarpDetached` / `WeftDetached`,
asserting the sole guard reason, both HEADs unchanged, and `MergeInProgress() == false` (a guard
refusal must write no record).

**False-green proof**: replaced the `reasons = append(reasons, detachedReasons...)` wiring in
`merge.go` with a discard (`_, err = detachedHeadReason(f)`), re-ran the test — both subtests failed
at the intended assertion:

```
mergecrucible_integration_test.go:67: MergeIn(feature) error = <nil> (<nil>); want *fabricengine.MergeGuardError
```

Restored the file from a byte-copy backup, confirmed the diff was empty, re-ran green.

**Gates**: `go build` / `go vet` green; hermetic `go test` green (fabricengine, fabriccli, gitrepo,
cmd/lyx); `-tags integration` green (fabricengine 23.7 s, fabriccli 2.2 s, gitrepo 1.5 s).

**Live re-verification** (after `./deploy-dev`, fresh hub `v_detach`):

```
$ git checkout --detach HEAD && lyx fabric merge-in task1
{"error":"fabricengine: merge preconditions failed: checkout is not on a branch",
 "mutations":[],"ok":false,"partial":false}
warp HEAD unchanged (4083d46 == main)
$ git checkout main && lyx fabric merge-in task1
{"already_up_to_date":false,"committed":true, …}     # still works on a branch
```

---

## F1 — BLOCKING — `MergeContinue` must refuse an attempt that never reached both sides

**Commit** `51426758`.

**Implemented**

- `internal/fabricengine/mergeerrors.go` — new closed-set member
  `mergeReasonAttemptIncomplete = "merge attempt did not reach both sides"`.
- `internal/fabricengine/mergelifecycle.go` — new `mergeAttemptIncompleteReason(st)`, true when
  either recorded outcome is empty (the shape a crash before the first `MergeStart`, or between the
  two, leaves behind — `merge.go` persists a side's outcome only once that side's `MergeStart` has
  returned). `MergeContinue` now aggregates it with `mergeReasonUnresolvedConflicts` into a single
  `*MergeGuardError`, so which precondition failed never discloses evaluation order, and refuses
  **before** the write lock and before anything lands.
- `concludeMergeSides`' godoc now states the precondition explicitly: every caller must have
  established both recorded outcomes are non-empty. `MergeIn`/`Merge` satisfy it by construction;
  `MergeContinue` enforces it.
- `internal/fabricengine/doc.go` — new "Not every crash is continuable, and the record says which"
  paragraph, which also supplies the qualification the existing crash-recovery paragraph needed.
- `internal/fabricengine/mergevocab_test.go` — pinned closed-set list extended, same commit.

**Why refuse rather than resume the un-started side**: teaching `MergeContinue` to run the missing
`MergeStart` itself would let a *new* conflict appear during a continue, which the verb's contract
does not admit, and would make `--continue` a mutation entry point rather than a conclude. Refusal
plus `MergeAbort` covers the whole window with no new failure mode, and `MergeAbort` always works
because it restores from the recorded pre-merge SHAs rather than from how far the attempt got.

**Test** `TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides` — reconstructs the crash
state byte-for-byte (real `git merge --no-commit` on the warp side, record saved with an empty weft
outcome via `SaveMergeStateForTest`), resumes through a **fresh** `Fabric` handle, and asserts the
sole guard reason, both HEADs unchanged, and then that `MergeAbort` still recovers the same record
and clears it. The fixture commits a divergent commit on each target branch first, so the
reconstructed attempt stages rather than fast-forwards — the staged shape is the crash window under
test, and the first draft of this test was wrong precisely because it fast-forwarded.

**False-green proof**: replaced the aggregation line with a discard
(`_ = mergeAttemptIncompleteReason(st)`), re-ran — the test failed at the intended assertion with
the original defect verbatim:

```
MergeContinue on an attempt that never reached both sides
  error = fabricengine: merge conclude did not finish; run MergeContinue again
  (*fabricengine.ErrMergeIncomplete); want *fabricengine.MergeGuardError
```

Restored from backup, `diff -q` reported the files identical, re-ran green.

**Gates**: `go build` / `go vet` green; hermetic `go test` green; `-tags integration` green
(fabricengine 40.1 s, fabriccli 3.0 s, gitrepo 1.9 s).

**Live re-verification** (after `./deploy-dev`, fresh hub `v_f1`, crash state reconstructed by hand):

```
$ lyx fabric merge --continue
{"error":"fabricengine: merge preconditions failed: merge attempt did not reach both sides",
 "mutations":[],"ok":false,"partial":false}
warp HEAD 210f4b3 — unchanged, nothing landed
$ lyx fabric merge --abort
{"committed":false,"mutations":[worktree_reset ×2],"ok":true,"partial":false}
warp 210f4b3, weft 640016e, worktrees clean, record gone
```

---

## F3 — MEDIUM — derive `Committed`/`AlreadyUpToDate` from what the merge actually did

**Commit** `6c92de71`.

**Implemented**

- `internal/fabricengine/mergestate.go` — two derivations on the record itself:
  `(*mergeState).landedConcludeCommit()` (either conclude-SHA field non-empty) and
  `(*mergeState).bothSidesAlreadyUpToDate()` (both recorded outcomes `up_to_date`).
- `internal/fabricengine/merge.go` — both verbs' both-sides-clean return now reads both flags off the
  record instead of hardcoding `Committed: true`. `MergeResult`'s godoc states the derivation.
- `internal/fabricengine/mergelifecycle.go` — `MergeContinue` likewise.
- `internal/fabricengine/doc.go` — new "What the result flags mean" paragraph, naming both cases
  that used to lie.

`Committed` deliberately reads the record rather than "did *this call* commit", so a resumed
`MergeContinue` whose sides both landed on an earlier call still reports true — the pair does carry
the commits. `Merge`'s *pre-lock* early return keeps its own `AlreadyUpToDate: true`; that one is
honest, and it is the post-lock path that was wrong.

**Four pre-existing integration tests were pinning the defect** and failed once it was fixed. All
four asserted `Committed == true` on paths that fast-forward:

| test | repair |
| --- | --- |
| `TestMergeIn_BothSidesClean` | fixture now commits a divergent commit on each target branch, so the merge genuinely needs a conclude-commit. The test claimed to cover the conclude path and never did; the comment now says the divergence is load-bearing and why. |
| `TestMergeIn_OneSideAlreadyUpToDate_OtherMerges` | asserts the honest fast-forward outcome (`Committed` false, `AlreadyUpToDate` false) plus a new assertion that the merging side's HEAD really moved. |
| `TestMergeIn_Freshness_LocalBehindRemote` | asserts `Committed` false; the merged-content check was always the real assertion. |
| `TestMergeIn_Freshness_SourceOnlyRemote` | same. |

**Test** `TestMergeCrucible_ResultFlagsDescribeWhatHappened` — two subtests:
`FastForwardBothSidesFabricatesNoCommit` (flags false, but `feature.txt` present, so the flags say
"no commit" and not "no merge") and `SecondCallReportsAlreadyUpToDateNotCommitted`, which is the
sequential control the interleaved loser must now match.

**False-green proof**: restored the hardcoded `MergeResult{Committed: true, Conflicts: …}` return in
`merge.go`, re-ran — failed at the intended assertion
(`Committed = true; want false — both sides fast-forwarded…`). Restored, `diff -q` identical,
re-ran green.

**Gates**: build / vet green; hermetic green; `-tags integration` green (fabricengine 37.0 s).

**Live re-verification** — the concurrency scenario re-driven post-fix on two fresh hubs plus a
sequential control, all after `./deploy-dev`:

| run | winner | loser |
| --- | --- | --- |
| hub `pf1` interleaved | `{committed:true, …}` | `{"already_up_to_date":true,"committed":false,"mutations":[]}` |
| hub `pf2` interleaved | `{committed:true, …}` | `{"already_up_to_date":true,"committed":false,"mutations":[]}` |
| hub `pfc` sequential control | `{committed:true, …}` | `{"already_up_to_date":true,"committed":false,"mutations":[]}` |

The interleaved loser now reports byte-identically to the sequential control, which is the property
the finding asked for.

---

## F4 — MEDIUM — pin `--ff` in `MergeStart`

**Commit** `536384b2`.

**Implemented**

- `internal/gitrepo/merge.go` — the non-squash form is now
  `git merge --ff --no-commit <ref>`, with a godoc paragraph explaining that `--ff` is git's default
  only until an operator sets `merge.ff`, and that pinning it is the same posture `MergeConclude`
  takes with `--no-edit`. The squash form needs no pin (`merge.ff` does not apply to `--squash`).
- `internal/gitrepo/doc.go` — the merge-surface paragraph now states both pins and why.

**Test** `TestMergeStart_HostileMergeFFConfig` in `internal/gitrepo/merge_integration_test.go` —
table-driven over both hostile directions: `merge.ff=only` must still produce `MergeStaged` on a
real merge, and `merge.ff=false` must still produce `MergeFastForwarded` on a fast-forward.

**False-green proof**: dropped `--ff` from the invocation, re-ran — both subtests failed, the first
with the exact production symptom (`git merge --no-commit feature: exit 128: … Diverging branches
can't be fast-forwarded`), the second with `outcome = 0; want 2`. Restored, `diff -q` identical,
re-ran green.

**Gates**: build / vet green; hermetic green; `-tags integration` green.

**Live re-verification** (fresh hub `v_ff`, `merge.ff = only` **and** a blocking
`core.editor = sh -c "sleep 600"` in `GIT_CONFIG_GLOBAL`, run under `timeout 60`): the merge
concluded normally on both sides with real merge commits, no hang, rc 0.

---

## F5 + F6 — LOW — `Remove` refuses a pair some other merge is consuming; `doc.go` states the real guarded set

**Commit** `7fe3da72`.

**Implemented (F5)**

- `internal/fabricengine/mergestate.go` — new `mergeSourceInFlight(l, warpBranch)`. It globs every
  pair's record — the prime pair's `<weft repo>/.git/fabric-merge.json` and every linked pair's
  `.git/worktrees/*/fabric-merge.json` — and reads each through `state.ReadJSON` under its `.lock`,
  exactly as `loadMergeState` does, never a raw `os.ReadFile` that could observe a torn record
  another process is mid-write on. A hub whose weft repo root will not resolve reports false rather
  than blocking, matching `mergeBlocksMutation`'s own already-half-broken-pair rule.
- `internal/fabricengine/remove.go` — consulted before any teardown, alongside (not instead of) the
  existing same-pair guard. The two answer genuinely different questions, and the code comment says
  so. `force` does not override it, per the existing gate rule.

**Implemented (F6)** — `internal/fabricengine/doc.go`'s sibling-refusal paragraph replaced. It now
names the four guarded verbs, both subjects `Remove` guards, and the stated reason each remaining
mutating entry point is safe unguarded (push family, `Cleanup`, `Prune`, `Reconcile`, junction verbs,
`Add`, `RebuildIndex`, `RecordCorrespondence`, `ResetHard`), plus the one knowing exception —
`CheckoutDetached`/`RestoreBranch`, whose harmful direction F2's precondition closes.

**Test** `TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming` — the prime pair is left
mid-merge on the source pair's branches (warp-side conflict only; a weft-root conflict would be
unmappable and self-abort before any record survives, which the first draft of this test hit), then
asserts `Remove` refuses with and without `force`, that neither the source worktree nor the
`<slug>-weft` branch was touched, and that after `MergeAbort` the same `Remove` succeeds — proving
the guard closes a window rather than blocking the pair forever.

**False-green proof**: replaced the refusal with a bare error-check on the same call, re-ran — failed
with `error = <nil> (<nil>); want *ErrMergeInProgress`. Restored, `diff -q` identical, re-ran green.

**Gates**: build / vet green; hermetic green; `-tags integration` green (fabricengine 42.7 s).

**Live re-verification** (fresh hub `v_f5`): `lyx fabric remove task1` and `remove task1 --force`
both refused with `a merge is in progress…` while the prime was mid-merge; the `task1` worktree and
the `task1-weft` branch both survived; after `merge --abort`, `remove task1` succeeded.

---

## F7 + F8 + F9 — NIT — empty-never-nil `Conflicts`, reject `-m` with `--abort`, document the conflict-vs-error discriminator

**Commit** `9edf37eb`.

**Implemented**

- **F7** `internal/fabricengine/mergelifecycle.go` — `MergeContinue` and `MergeAbort` now return the
  `mergeNoConflicts` sentinel on their success paths, honouring the contract `MergeResult`'s godoc
  and `merge.go:23` already stated. `MergeAbort`'s godoc updated.
- **F8** `internal/fabriccli/merge_verbs.go` — the pre-flight rejects `-m` alongside `--abort`, in
  the same carve-out that already rejects `--squash`, with a comment saying why silently ignoring a
  flag is worse than refusing it. `-m` stays meaningful for `--continue`. The file header notes the
  pre-flight's job.
- **F9** `internal/fabricengine/doc.go` — new "A conflict result is not a failure, and a script tells
  them apart by the envelope" paragraph, plus the empty-never-nil sentence folded into the
  conflict-reporting paragraph; both CLI `Long` texts now state the discrimination rule and `-m`'s
  scope.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — the suite carried **no** merge scenarios at all. Added
  **F18** (merge-in → conflict envelope → sibling refusals → `--continue`, then the same merge
  `--abort`ed, including the `committed`-on-a-fast-forward check) and **F19** (the precondition
  matrix — dirty, detached HEAD, non-fabric-managed sources, foreign plain-git state including the
  `--squash` shape with no `MERGE_HEAD` — plus hostile `merge.ff`/`core.editor` and the flag
  pre-flight). `F0`'s verb count corrected 16 → 18. `go test ./tools/...` green.

**Tests**
- `TestMergeCrucible_ConflictsIsEmptyNeverNil` — asserts on the marshalled JSON, since `null` vs `[]`
  is the property a consumer actually sees.
- `TestRunCLI_MergeRejectsFlagsItWouldOtherwiseIgnore` — table-driven over `-m`+`--abort` and
  `--squash`+`--abort`, driven through `RunCLIIn` against a real hub so the check is proven to sit
  ahead of the engine call.
- `TestRunCLI_MergeContinueAcceptsMessage` — pins the other half of the rule: `-m` still names the
  conclude-commit for `--continue`.

**False-green proofs**
- F7: reverted both returns to bare `MergeResult{…}` — failed with
  `MergeContinue.Conflicts is nil` and the marshalled envelope showing `"conflicts":null`. Restored,
  identical, green.
- F8: removed the pre-flight arm — failed with
  `error = "fabricengine: no merge in progress"; want "usage: -m cannot be combined with --abort"`
  (i.e. the flag was reaching the engine and being ignored). Restored, identical, green.

**Live re-verification** (fresh hub `v_nits`): `merge --abort -m hi` and `merge --abort --squash`
both rejected as usage errors; `merge --continue -m "crucible chosen message"` produced a
conclude-commit with that exact subject; `merge-in --help` carries the `conflicts` discriminator
sentence.

---

## Determinism proof for the new tests

Per the campaign rule, proven by repetition under load rather than by a single pass. Eight busy-loop
CPU hogs were started, then:

```sh
go test -tags integration -count=10 -run 'TestMergeCrucible_' ./internal/fabricengine/            # ok 17.9s
go test -tags integration -count=10 -run 'TestRunCLI_Merge(RejectsFlags|ContinueAccepts)' ./internal/fabriccli/   # ok 4.0s
go test -tags integration -count=10 -run 'TestMergeStart_HostileMergeFFConfig' ./internal/gitrepo/ # ok 0.5s
```

All green, zero flakes. No new test sleeps or assumes synchrony: every one waits on an actual state
transition (a returned result, a resolved SHA, a git ref) rather than on elapsed time.

Also, per the prompt's stress requirement:
`go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — green.

## Final gate run (tag named on each)

```
go build ./...                                                          BUILD_OK
go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...   VET_OK
go vet ./...                                                            clean
go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...   tag: none      all ok
go test -count=1 ./...                                                  tag: none      all ok
go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...
                                                                        tag: integration
                                                                        fabricengine 39.9s ok, fabriccli 4.1s ok, gitrepo 2.2s ok
```

`gofmt -l` reports only `internal/lyxcwd/docslink_test.go` and `internal/shell/posix.go`, both
pre-existing and untouched by this round (`git diff --name-only HEAD` does not list them).

## Final live regression sweep (deployed binary `lyx @ 9edf37eb`)

One fresh hub per destructive scenario, all after `./deploy-dev`:

| # | scenario | result |
| --- | --- | --- |
| 1 | `merge-in` conflict | conflict envelope, sorted unified paths |
| 2 | sibling `commit` mid-merge | `ErrMergeInProgress` |
| 3 | `remove` of the merge source | `ErrMergeInProgress` (**was** a silent success) |
| 4 | `merge --continue` | both sides concluded, SHA-labelled subject |
| 5 | `merge --abort` | both HEADs restored exactly, both worktrees clean |
| 6 | unmappable weft-root conflict | `ErrUnmergeableState`, both sides restored |
| 7 | `merge --squash` | `Squashed commit of the following:` on both sides |
| 8 | foreign plain-git merge state | `ErrForeignMergeState`, state left untouched |

## Deferred — nothing

Every finding F1-F9 is fixed and committed. Nothing was marked
NOT-FIXED-THIS-ROUND, and nothing was left for an operator decision.

Two items are carried as *recorded non-findings* rather than deferred work, both stated in the review
report:

- **Windows path behaviour** in `weftPathVisible`/`unifyConflictPaths` — explicitly out of scope for
  this campaign, and not executable on this Linux host. Not fixed blind.
- **A sibling verb slipping through the deliberately-unlocked guard window** — reasoned about,
  never reproduced. Per the campaign rule that is not a finding, so there is nothing to fix.

## Changed files

| file | why |
| --- | --- |
| `internal/gitrepo/merge.go` | `HeadDetached` (F2); `--ff` pin (F4) |
| `internal/gitrepo/doc.go` | merge-surface list + the two pinned behaviours (F2, F4) |
| `internal/gitrepo/merge_integration_test.go` | `TestMergeStart_HostileMergeFFConfig` (F4) |
| `internal/fabricengine/mergeerrors.go` | two new closed-set reasons (F1, F2) |
| `internal/fabricengine/mergeguards.go` | `detachedHeadReason` (F2) |
| `internal/fabricengine/merge.go` | guard wiring (F2); result-flag derivation (F3) |
| `internal/fabricengine/mergelifecycle.go` | `mergeAttemptIncompleteReason` + refusal (F1); result flags (F3); `mergeNoConflicts` (F7) |
| `internal/fabricengine/mergestate.go` | `landedConcludeCommit`/`bothSidesAlreadyUpToDate` (F3); `mergeSourceInFlight` (F5) |
| `internal/fabricengine/remove.go` | source-in-flight refusal (F5) |
| `internal/fabricengine/doc.go` | five paragraphs (F1, F2, F3, F6, F9) |
| `internal/fabricengine/mergevocab_test.go` | pinned closed-set list (F1, F2) |
| `internal/fabricengine/mergecrucible_integration_test.go` | **new** — F1, F2, F3, F5, F7 regressions |
| `internal/fabricengine/mergein_integration_test.go` | two tests that pinned F3's defect |
| `internal/fabricengine/mergein_recovery_integration_test.go` | two tests that pinned F3's defect |
| `internal/fabriccli/merge_verbs.go` | `-m`+`--abort` pre-flight (F8); `Long` text (F8, F9) |
| `internal/fabriccli/merge_cli_integration_test.go` | F8 regressions |
| `tools/sandbox/SANDBOX-FABRIC-SUITE.md` | scenarios F18, F19; F0 verb count (F9) |

No change to `manifest/roadmap.md` (correct — this is hardening, per `CLAUDE.md`), and none to
`docs/overview.md` or `CONSTRAINTS.md`: the module table did not move and no new cross-cutting
invariant was introduced. The two new guard reasons live inside the merge surface's own existing
closed set, whose same-commit rule is already recorded in `mergeerrors.go` and enforced by
`mergevocab_test.go`.

## Teardown

Every scratch hub was built under the session scratchpad, outside this worktree, and all are removed.
No background processes survive the round. `git status` in the worktree shows only
`_mill/fabric-merge-review-HANDOFF.md`, which is the orchestrator's file and was never staged —
every commit in this round named its paths explicitly, and no `git add -A` or `git add .` was used.
Nothing was pushed.
