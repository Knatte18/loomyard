# fabric merge surface — independent review (round 9, tag `opus-high-r9`)

Reviewer: fresh clean-room pass, Opus/high.
Worktree: `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch `fabric-merge-crucible-round4`.
Scope: the merge lifecycle quintet (`MergeIn`/`Merge`/`MergeContinue`/`MergeAbort`/`MergeInProgress`) plus `MergeStageResolved`, `internal/gitrepo/merge.go`, and the `lyx fabric merge*` CLI surface.

## Status

REVIEW COMPLETE — four findings (1 MEDIUM, 2 LOW, 1 NIT), all CONFIRMED against the real substrate. Job 2 (fixes) begins only after this file is committed.

No production or test file in this worktree was touched during Job 1: every sabotage probe ran in an isolated `tar`-clone of the tree under the scratch directory.

## What was tested

### Hermetic gates (baseline, before any edit)

- `go build ./...` — OK.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (35.6s / 3.0s / 1.7s).

### Sabotage re-proofs of round 8's own new mechanisms

Sabotage was performed in an ISOLATED COPY of the tree (`scratchpad/r9/sabotage`, `tar`-cloned without `.git`), so no production or test file in this worktree was touched during Job 1.

| # | Sabotage | Test(s) that must fail | Result |
|---|---|---|---|
| S1 | `recheckMergePreconditionsUnderLock`: delete the foreign-state re-check | `TestMergeIn_ForeignStateAppearingWhileWaitingForLock_Refuses`, `TestMerge_…` | FAIL (both) — guard holds |
| S2 | `recheckMergePreconditionsUnderLock`: delete the dirtiness re-check | `TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt`, `TestMerge_…` | FAIL (both) — guard holds |
| S3 | `recheckMergePreconditionsUnderLock`: delete the record re-check | `TestMergeIn_RecordAppearingWhileWaitingForLock_…`, `TestMerge_…` | FAIL (both) — guard holds |
| S4 | `MergeAbort`: move lock acquisition back below the record read + conclude-landed guard | `TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting` | FAIL — guard holds |
| S5 | `errConflictsWithRecord`: drop the `merge-stage` step from the runtime conflict message | `TestErrConflictsWithRecord_ReservedKeysAreAlwaysTheHelperOwnValues` | FAIL — guard holds |
| S6 | `MergeStageResolved`: delete the foreign-state refusal | `TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging` | FAIL — guard holds |
| S7 | `MergeIn`: delete the WEFT-side start re-read under the lock | `TestMergeIn_StartsAreReReadUnderLock` | FAIL on `WeftStart` — guard holds |
| S8 | CLI merge-stage: `uniquePreservingOrder(args)` → `args` | `TestRunCLI_MergeStageEchoesEachPathOnce` | FAIL — guard holds |
| S9 | `MergeContinue`: move lock acquisition back below the record read + guards | `TestMergeContinue_RecordRetiredWhileWaitingForLock_ReportsNoMergeInProgress` | FAIL — guard holds |

No round-8 test survived sabotage of the mechanism it claims to guard.

### Live substrate (real bare warp/weft pair, `lyx fabric clone`, dev binary redeployed from this source)

Hub recipe: `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` before the first `git init`; bare `warp.git` + `weft.git`; seeded warp `main`; `lyx fabric clone <weft-bare> <warp-bare>` from an empty parent; `lyx fabric add feat` for the source pair.

- **S1 live — happy path.** conflicting divergence on BOTH sides (`shared.txt` warp, `_lyx/raddle-note.md` weft) → `merge-in feat` returns `conflicts:["_lyx/raddle-note.md","shared.txt"]`, `partial:false`, two `merge_staged` entries. Plain `git add _lyx/raddle-note.md` in the visible worktree refuses (`beyond a symbolic link`) exactly as the help claims. `merge-stage` + `merge --continue` → `committed:true`, both sides carry a two-parent merge commit with the recorded source as parent 2.
- **S2 live — partial staging.** Stage only the warp path, then `merge --continue` → `merge preconditions failed: unresolved conflicts remain`. See finding F1.
- **S3 live — `merge --abort`.** Restores both HEADs exactly, clean status both sides, record gone, `merge --abort`/`--continue` afterwards both answer `no merge in progress`.
- **S4 live — `merge` over a conflicting source.** Self-aborts: `ErrMergeInRequired`, `partial:true`, two `merge_staged` + two `worktree_reset` entries, both HEADs restored byte-exact, no `MERGE_HEAD` on either side, record gone. See finding F2 for the message.
- **S5 live — guard set.** warp-only branch → `source branch is not fabric-managed`; nonexistent branch → `source branch is not fabric-managed; source branch not found` (see F3); detached weft HEAD → `checkout is not on a branch`; dirty warp → `worktree dirty`.
- **S6 live — the post-fetch not-synced re-decision (round 6's fix).** Side clone pushes to `origin/main`; local `main` gains its own commit and is never fetched, so pre-fetch `rev-list --left-right --count HEAD...@{u}` = `1 0` (guard-stage sees "ahead", passes). `lyx fabric merge feat` refuses with `branch not synced to upstream`; post-call count is `1 1`, worktree clean, no record. The re-decision still holds.
- **S7 live — sibling refusals mid-merge.** `pull`, `checkout`, `remove` all return `a merge is in progress; run MergeContinue or MergeAbort first`; a second `merge-in` returns `merge already in progress; worktree dirty`.
- **S8 live — hand-landed conclude adoption.** Resolve + `merge-stage` both sides, then plain `git commit --no-edit` in warp only (record's `warp_committed` still empty). `merge --continue` adopts the hand-landed warp commit (`merge_committed` naming it), concludes weft, reports `committed:true`, deletes the record.
- **S9 live — unmappable weft conflict.** A conflicting weft file at the weft ROOT (`rootnote.txt`, outside every wired name) → `merge produced conflicts outside the fabric-managed tree; operator intervention required`, `partial:true`, both sides reset byte-exact, record gone. The offending path IS visible to the operator: `level=WARN … weft_conflicts=[rootnote.txt]` is emitted at default verbosity, not only under `-vv`.

### Live substrate, continued — the adversarial pass

- **S10 live — `concludeMergeSides` commits a merge that is not this merge's own.** See F1. CONFIRMED end to end on lab2.
- **Lint gate.** `golangci-lint run` and `golangci-lint run --build-tags integration` over `./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`: the merge surface itself is clean. The only hits are outside it — `internal/gitrepo/push.go:123` (`errcheck` on `defer l.Release()`), three `errcheck` hits in `internal/gitrepo/push_test.go`, and one `unused` in `internal/fabricengine/livestate_verbs_test.go`. All are in files outside this round's scope and are left alone.
- **SPEC conformance.** Every "In" bullet of the original SPEC (`git show 3b800bc8:_mill/discussion.md`) is delivered: the gitrepo primitives, the five fabricengine verbs, the merge-state record, the gated two-sided reset inside `destroy.go`, the sibling refusals, unified side-free conflict reporting, two-sided indivisibility, aggregated deterministic guards, `Commit`'s own refusal, `RecordCorrespondence`, both CLI verbs with envelope/help-tree tests, and the pinned-list and doc updates. Two deviations are reasoned and documented, not silent drops: `Cleanup` carries no refusal (the doc argues it cannot touch a checked-out weft branch and a mid-merge pair is materialized by definition), and `merge-stage` was added beyond the SPEC because the SPEC's own resolve flow was uncompletable for a weft-side conflict without it. No over-reach found.

## Findings

Four findings. Severity affects reporting only — all four are fixed in Job 2.

---

### F1 — MEDIUM — CONFIRMED — `concludeMergeSides` commits whatever git-level merge is live on a side, with no evidence it is THIS merge's own

**Where:** `internal/fabricengine/mergelifecycle.go:40-93` (`concludeMergeSides`), reached from `MergeContinue` (`mergelifecycle.go:227`).

**The asymmetry.** The ADOPT arm (`sideConcludeAlreadyLanded`, `mergelifecycle.go:147`) was hardened over two prior rounds to demand exact, discriminating evidence before it will CLAIM a landed commit as this merge's conclude: HEAD moved off the recorded start, no live `MERGE_HEAD`, not a squash, a recorded source SHA, and exactly two parents in exactly the recorded order. Its own godoc spells out why a looser reading was a real defect ("a silent false success").

The COMMIT arm, three lines below it, demands nothing at all. When `sideConcludeAlreadyLanded` reports `landed=false`, `concludeMergeSides` calls `MergeConclude("")` — `git commit --no-edit` — over whatever git-level merge state happens to be live in that checkout, then writes the resulting SHA into the record as this merge's conclude, appends `KindMergeCommitted` naming it, records correspondence, and deletes the record. Nothing checks that the live `MERGE_HEAD` is the source SHA the record says this merge is merging.

**Reproduction (live, lab2, dev binary built from this source).** Warp merges cleanly (real non-fast-forward, `warp_outcome: staged`, `MERGE_HEAD` = recorded `warp_source`), weft conflicts:

```
lyx fabric merge-in feat
  -> conflicts:["_lyx/note.md"]
  record: verb=merge-in source=feat warp_start=9cf1032 warp_source=5100110
          warp_outcome=staged weft_outcome=conflicted
  warp MERGE_HEAD: 5100110  (== warp_source)
```

The operator then does plain git in the warp checkout — permitted verbatim by the Fabric Git Invariant ("a human or any tool outside LYX keeps ordinary git in their warp worktree, untouched"), and the same premise round 8's F3 and the adoption defect were both graded on:

```
git merge --abort                       # warp HEAD back to 9cf1032, MERGE_HEAD gone
git checkout -b other 9cf1032; ...commit...; git checkout main
git merge --no-commit --no-ff other     # MERGE_HEAD = 87cc498, staged, no conflicts
```

They resolve the weft conflict, `lyx fabric merge-stage _lyx/note.md`, and:

```
lyx fabric merge --continue
{"already_up_to_date":false,"committed":true,
 "mutations":[{"kind":"merge_committed","target":"warp","detail":"aaafd5d..."},
              {"kind":"merge_committed","target":"warp-weft","detail":"9b48711..."}],
 "ok":true,"partial":false}
```

**Observed end state:**

```
warp HEAD:    aaafd5d Merge branch 'other'
warp parents: aaafd5d 9cf1032(start) 87cc498(other)      <- second parent is NOT warp_source
feat-only.txt present in warp?   NO
other-only.txt present in warp?  YES
is feat merged into main?        NO
is feat-weft merged into main-weft?  YES
record present?                  no
```

fabric reported `ok:true, committed:true`, named a `merge_committed` mutation for a commit that merges an unrelated branch, recorded correspondence between mismatched halves, and deleted the record. The pair is now permanently non-corresponding — `feat-weft` is merged on the weft side and `feat` is not merged on the warp side — and no fabric verb can notice, because the only evidence (the record) is gone.

**Why the existing suite misses it.** `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F20 already drives the near-identical fixture (warp merges cleanly, weft conflicts, `git merge --abort` in warp) for the octopus case — but there the operator *commits* the decoy merge, so HEAD moves and `MERGE_HEAD` is gone, which routes into the adoption arm and is correctly refused. Leaving `MERGE_HEAD` **live** routes into the commit arm instead, which has no check. Same for the "unrelated commit" case: it commits, so it adopts-and-refuses. The whole hardened evidence chain is bypassed simply by not committing.

The same shape is reachable without any adversarial intent, via the doc's own documented last-resort recovery: `doc.go` tells the operator to `git reset --hard <warp_start>` and then `git merge <warp_source>` by hand. An operator who merges the *branch name* rather than the recorded SHA (the branch tip having since moved) lands a merge of a different commit, and fabric commits and claims it with no complaint.

**Severity.** MEDIUM, graded consistently with round 8's F3 and the earlier adoption defect, which share this exact threat model. Its consequence class is the severest in the surface: a silent false success that destroys its own evidence.

**Suggested fix.** Give the commit arm the same shape of evidence the adopt arm demands, as a new aggregatable precondition on `MergeContinue`:

- `internal/gitrepo/merge.go`: expose `MERGE_HEAD`'s SHA, not merely its presence (a go-git read if `ResolveSHA("MERGE_HEAD")` resolves — keeping it off the Client Boundary pinned CLI list, matching `HeadDetached`/`CommitParents`/`ResolveSHA` — otherwise a `runChecked` method added to the pinned list and to `CONSTRAINTS.md` in the same commit).
- `internal/fabricengine/mergeguards.go`: a per-side predicate reporting "this side no longer carries the recorded merge", true only when the side is genuinely about to be concluded (recorded conclude SHA empty, outcome `staged`/`conflicted`) and is not the adoption shape, and either has no live `MERGE_HEAD` or has one that is not the recorded per-side source SHA.
- A new closed-set reason (side-free, path-free, order-free) aggregated into `MergeContinue`'s existing reason set.
- Exempt a squash record and an empty recorded source SHA, exactly as the adoption arm does and for the same stated reason: `git merge --squash` writes no `MERGE_HEAD`, so there is no evidence to demand and refusing every squash `--continue` would be wrong. That residual must be stated in the doc rather than papered over.
- Recovery after the refusal stays available: `MergeAbort` still passes its own `concludeLandedReason` in this state and resets both sides from the recorded pre-merge SHAs.

---

### F2 — LOW — CONFIRMED — `MergeContinue`'s unresolved-conflicts refusal names no path, and no shipped surface can re-list the remaining ones

**Where:** `internal/fabricengine/mergelifecycle.go:265-272`; `internal/fabricengine/mergeerrors.go:34` (`mergeReasonUnresolvedConflicts`); `internal/fabriccli/merge_verbs.go:33-46` (`setMergeExit`).

**Scenario (live, lab1).** `merge-in feat` reports `conflicts:["_lyx/raddle-note.md","shared.txt"]`. The operator resolves and stages only `shared.txt`, then:

```
lyx fabric merge --continue
{"error":"fabricengine: merge preconditions failed: unresolved conflicts remain","mutations":[],"ok":false,"partial":false}
```

Every shipped surface then fails to tell them which path is still unresolved:

```
lyx fabric status  -> {"changes":[{"path":"shared.txt","side":"warp"},{"path":"_lyx/raddle-note.md","side":"weft"}]}
                      (an ordinary change row, not marked unmerged; in a real hub it is one row among many)
git status in the visible worktree -> "M  shared.txt"   (the weft conflict is NOT visible at all)
git status in the weft checkout    -> "UU _lyx/raddle-note.md"
```

`merge-in` cannot be re-run to re-print the list (`merge already in progress`). So the only route back to the list is raw git inside the weft checkout — the one place the Fabric illusion says the operator never has to look, and precisely the argument a prior round used to justify shipping `merge-stage` at all.

The SPEC does record "surfacing merge-in-progress state in `lyx fabric status` output" as an explicit out-of-scope follow-up, so the *status-verb* half of this is a known deferral, not a regression. What is inside this round's surface is narrower and unaddressed: the refusal itself carries the information (`MergeContinue` has just read both sides' `ConflictedFiles()`) and throws it away.

**Suggested fix.** Have `MergeContinue` return the still-unresolved unified paths on the guard-refusal result (the same `unifyConflictPaths` mapping `MergeIn` uses), and have the CLI surface them on the failure envelope under a key that is NOT `conflicts` — the `conflicts` key is the documented discriminator between "a conflict result" and "a hard failure" (`merge-in`'s own `Long`), and must stay exclusive to the former.

---

### F3 — LOW — CONFIRMED — three runtime remedies name engine methods no shipped CLI surface offers

**Where:** `internal/fabricengine/mergeerrors.go:97`, `:125`, `:145`.

```
ErrMergeInRequired: "...; run MergeIn in the source branch's worktree first, then retry"
ErrMergeIncomplete: "...; run MergeContinue again"
ErrMergeInProgress: "...; run MergeContinue or MergeAbort first"
```

Observed verbatim at the CLI (live, lab1):

```
lyx fabric merge feat   -> {"error":"fabricengine: merge produced conflicts and was aborted; run MergeIn in the source branch's worktree first, then retry", ...}
lyx fabric pull         -> {"error":"fabricengine: a merge is in progress; run MergeContinue or MergeAbort first", ...}
```

There is no `MergeIn`, `MergeContinue` or `MergeAbort` in the shipped CLI; the spellings are `lyx fabric merge-in`, `lyx fabric merge --continue`, `lyx fabric merge --abort`. This is the same family as round 8's F1 — the CLI help was fixed to name the runnable verb (`merge_verbs.go`'s `Long` maps every one of them: "`--continue` (the engine's MergeContinue)"), while the runtime string an operator or agent actually reads was not.

That these strings are read as *operator instructions*, not as Go-API prose, is already the project's own position: `commit.go:118-121` argues that `*ErrMergeInProgress`'s "run MergeContinue or MergeAbort first" "would misdirect the operator into two more refusals" and therefore hands back `*ErrForeignMergeState` instead — whose own message already names what the operator runs ("conclude or abort it with plain git"). `errConflictsWithRecord` settles the same question the same way, naming `"lyx fabric merge-stage <path>..."` verbatim.

`internal/landingshed/finalize.go:138` catches `*ErrMergeInRequired` by type and surfaces the text into a stuck message a human reads, so the CLI envelope is not the only operator-facing exit.

**Suggested fix.** Reword all three to name the runnable verb, in `errConflictsWithRecord`'s established style (double-quoted CLI spelling). Update `mergeerrors_test.go`, `merge_cli_integration_test.go:225`, the two in-code comments that quote them (`commit.go`, `mergesiblings_integration_test.go`), and the four `tools/sandbox/SANDBOX-FABRIC-SUITE.md` quotations, all in the same commit. Historical `_mill/` reports are records of what past rounds saw and are not touched.

---

### F4 — NIT — CONFIRMED — `resolveMergeSources`' godoc states a disjointness rule the shipped code does not follow

**Where:** `internal/fabricengine/mergeguards.go:42-45`.

> Those two reasons stay disjoint on purpose: an unmanaged source reports ONLY mergeReasonNotFabricManaged, never that reason plus source-not-found […]

Live (lab1):

```
lyx fabric merge-in warponly        -> "source branch is not fabric-managed"                          (claim holds)
lyx fabric merge-in no-such-branch  -> "source branch is not fabric-managed; source branch not found"  (claim false)
```

The code appends `mergeReasonSourceNotFound` from the warp arm unconditionally on `!warpFound` (`mergeguards.go:72-74`) and only gates the weft arm's copy on `weftManaged`. A source that resolves nowhere is unmanaged *and* not found, so both fire — and `TestRunCLI_MergeNonexistentBranchReportsAggregatedGuardError` (`internal/fabriccli/merge_cli_integration_test.go:242`) pins exactly that dual-reason output as intended, as does `SANDBOX-FABRIC-SUITE.md`'s F19 ("a source that exists nowhere -> that plus `source branch not found`").

So the behaviour is deliberate and pinned in two places; the paragraph immediately above it in the same comment describes it correctly; only the "stay disjoint" sentence over-claims. It is exactly the doc-accuracy failure mode this round is asked to hunt — a claim a reader can check and find false — and reading it as true would invite "fixing" a pinned behaviour.

**Suggested fix.** Correct the sentence to state the actual rule: the *weft* arm's `source-not-found` is gated on `weftManaged` so an unmanaged-but-resolvable source reports one reason rather than two, while a source resolvable nowhere reports both, because both facts are independently true and the operator needs to know the branch does not exist at all. Documentation only — no behaviour change, and the two pinning tests must keep passing untouched.

---

## Deferred items — re-evaluated

- **Windows path behaviour (`weftPathVisible`/`unifyConflictPaths`).** Nothing further is possible here beyond what round 6 already did. No Windows host exists in this campaign; `filepath.ToSlash` is the identity function wherever `os.PathSeparator == '/'`, so the one remaining atom — the `os.PathSeparator` argument at `weftPathVisible`'s entry point (`mergepaths.go:63`) — is indistinguishable from a hardcoded `'/'` at runtime on this host and is pinned by source inspection only. The separator-explicit inner function is already driven with `'\\'` by a POSIX test, so both wrong implementations (dropping the conversion, and blanket-replacing every backslash) do fail. Stated plainly: I cannot execute this headlessly on this host, and neither could any prior round.
- **Round 8's own new mechanisms.** Re-sabotaged, nine ways (table above). Every one of the four lock-race tests and the merge-stage message/dedup tests fails when the mechanism it claims to guard is removed. No regression, no green-under-sabotage test found.
- **Four states where `MergeContinue` gets stuck** (first instalment round 2 rows 27/28/30/31). Re-confirmed unchanged; not touched. F1's fix adds no new stuck state — `MergeAbort` remains available in the state F1's new refusal produces, which was verified by tracing `concludeLandedReason` against that exact record shape.
- **Post-record error-return class per-site adjudication.** Not required this round; not touched.
- **N-way concurrent amplifier.** Not required this round; not run.

