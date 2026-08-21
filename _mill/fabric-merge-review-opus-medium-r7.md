# fabric merge surface — independent review (round 7, `opus-medium-r7`)

Clean-room round 7 of the fabric merge-surface crucible campaign.
Findings below were formed without reading any prior `_mill/fabric-merge-review-*` material.

## Scope reviewed

- `internal/fabricengine/merge.go`, `mergelifecycle.go`, `mergeerrors.go`, `mergeguards.go`, `mergestate.go`, `mergestage.go`, `mergepaths.go`
- `internal/gitrepo/merge.go`
- `internal/fabriccli/merge_verbs.go`, `envelope.go`, `weft_verbs.go`
- `internal/fabricengine/doc.go` "# The merge surface" (lines 846–1069)
- All `merge*_test.go` / `merge*_integration_test.go` siblings

## What was tested

### Baseline gates (all green, before any edit)

- `go build ./...` — OK
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — OK (exit 0)
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — OK
  (fabricengine 31.8s, fabriccli 2.6s, gitrepo 1.7s)
- `./deploy-dev` — dev binary at `.dev-bin/lyx`

### Live substrate — real bare warp/weft pair, hub driven by the dev binary

Hub built by hand under the scratch directory (never inside the worktree), `GIT_CONFIG_GLOBAL` with
`[init] defaultBranch = main` exported before the first `git init`:
`git init --bare warp.git weft.git` → seed+push warp `main` → `lyx fabric clone <weft-bare> <warp-bare>` →
`lyx fabric add task-a`.

1. **Two-sided conflicting MergeIn.** Divergent `src/app.txt` (warp side) and `_lyx/raddle/notes.md`
   (weft side, written through the junction and landed with `lyx fabric push`) on both `main` and
   `task-a`. `lyx fabric merge-in task-a` reported
   `{"conflicts":["_lyx/raddle/notes.md","src/app.txt"], ... "ok":false,"partial":false}` with two
   `merge_staged` mutations. Record on disk carried both starts, both resolved sources, and
   `conflicted` on both outcomes. Live `MERGE_HEAD` on both sides matched the recorded sources exactly.
   → unified conflict list, record shape, and `merge_staged` recording all correct.
2. **MergeAbort from the two-sided conflicted state.** Both HEADs restored to the recorded starts,
   both `MERGE_HEAD`s gone, both `git status --porcelain` empty, record deleted, two `worktree_reset`
   mutations. A second `--abort` and a `--continue` both returned `fabricengine: no merge in progress`
   (not foreign state). A fresh `merge-in task-a` afterwards reproduced the same conflicts rather than
   being refused as foreign — so the abort really does clear git-level merge state, not just fabric's.
3. **Operator-route conflict resolution (see F1).** Resolved BOTH conflicted paths through the single
   visible worktree, then staged as an operator would: `git add src/app.txt` succeeded;
   `git add _lyx/raddle/notes.md` failed with
   `fatal: pathspec '_lyx/raddle/notes.md' is beyond a symbolic link` (exit 128). `merge --continue`
   then refused with `unresolved conflicts remain`. There is no CLI surface for `MergeStageResolved`.
4. **MergeContinue after staging the weft side with raw `git -C <weft> add`.** Concluded with
   `committed:true` and two `merge_committed` mutations. `rev-list --parents -n1 HEAD` on both sides
   showed exactly two parents in exactly the recorded (start, source) order — the shape
   `sideConcludeAlreadyLanded` demands. Record deleted, resolved content correct on both sides.

### Sabotage sweep

Harness: apply exactly one source mutation, `go build ./...` (a build break is discarded, not counted
as proof), then `go test -tags integration -count=1 ./internal/fabricengine/... ./internal/fabriccli/...
./internal/gitrepo/...`, then restore. A PASS after sabotage means the mechanism is unguarded.

Round 6's own new mechanisms (the named re-examination target):

| # | sabotage | result |
|---|---|---|
| S-a | `len(parents) != 2` → `len(parents) < 2` | caught — `TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted` |
| S-b | drop `parents[0] != start` | caught — `TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted` |
| S-c | drop `parents[1] != sourceSHA` | caught — `TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted` |
| S-f | drop the squash refusal in adoption | caught — `TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted` |
| S-g | drop the live-`MERGE_HEAD` refusal in adoption | caught — `TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead` |
| S-h | drop `MergeStageResolved`'s foreign-state guard (F7) | caught — `TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging` |
| S-i | drop `finalizeMergeResult`'s `Conflicts` nil-safety (F8) | caught — `TestMergeCrucible_ConflictsIsEmptyNeverNil` (4 subtests) |
| S-e | `filepath.ToSlash` → blanket `strings.ReplaceAll(…, "\\", "/")` (F6) | caught — `…/single_anchor_segment_containing_a_backslash_is_not_split` |
| **S-d** | **DELETE `filepath.ToSlash` entirely (identity)** | **PASS — UNGUARDED → F2** |

Each of S-a/S-b/S-c was caught by a *different* test, so round 6's three parentage clauses are
independently pinned and each test fails for its own reason. Round 6's work is sound except S-d.

Wider surface:

| # | sabotage | result |
|---|---|---|
| S-1 | `MergeStart` drop the `--ff` pin | caught — `TestMergeStart_HostileMergeFFConfig` |
| S-2b | `MergeStart` drop the `mergeHeadPresent` classification arm | caught — 2 tests, both packages |
| S-3 | `ConflictedFiles` drop `-z` | caught — 7 tests |
| **S-4** | **`StageResolved` `add -A --` → `add --`** | **PASS — UNGUARDED → F3** |
| S-5 | `MergeFFOnly` → `reset --hard` | caught — `TestMergeFFOnly_FailsLoudlyOnDivergedPair` |
| S-6 | `detachedHeadReason` drop the weft half | caught — `…/WeftDetached` |
| S-7 | `pairDirtyReason` drop the weft half | caught — 2 tests |
| **S-8** | **`foreignMergeStatePresent` drop BOTH weft probes** | **PASS — UNGUARDED → F4** |
| **S-10** | **`foreignMergeStatePresent` drop the warp conflicted-index probe** | **PASS — UNGUARDED → F4** |
| **S-11** | **`foreignMergeStatePresent` drop the warp `MERGE_HEAD` probe** | **PASS — UNGUARDED → F4** |
| **S-12** | **`syncedToUpstreamReason` drop the weft half** | **PASS — UNGUARDED → F5** |
| **S-13** | **`sideNotSyncedToUpstream` drop the behind-passes clause** | **PASS — UNGUARDED → F5** |
| S-14 | `syncSideBeforeMerge` drop the whole FF advance | caught — 2 tests |
| S-9 | `sideConcludeMayHaveLanded` drop the HEAD-moved clause | caught — 2 tests |

### Live: is `Merge`'s not-synced guard reachable at all? (→ F5)

`git add`-probe first (`git version 2.53.0`): a delete/modify conflict resolved by deletion stages
with **plain** `git add -- f.txt`, exit 0, index clean afterwards — so `StageResolved`'s godoc
rationale for `-A` is false on any modern git (→ F3).

Then, on the live hub: created pairs `target` and `task-b`; both sides of `target` carry a real
`@{u}` (`origin/target`, `origin/target-weft`). A separate clone pushed a commit to `origin/target`;
`target` then made a local commit **without fetching**. State at call time:

```
target HEAD:          47828cdf…      (local commit)
target origin/target: 81b823c7…      (stale, pre-fetch)
real remote tip:      c815ec1a…
```

`lyx fabric merge task-b` returned:

```
{"already_up_to_date":false,"committed":true,"mutations":[…],"ok":true,"partial":false}   exit=0
```

and afterwards `git rev-list --left-right --count 'HEAD...@{u}'` = **`3	1`** — genuinely diverged,
with `origin/target` now advanced to `c815ec1a…` because this very call fetched it. The guard that
exists to refuse exactly this never fired.

### Live: linked-worktree merge record, and weft-side-only foreign state

- Created pairs `target`, `task-b`, `task-c`, `fx`, `fy` on the live hub.
- A `merge-in` run from the **linked** pair `target` wrote its record to
  `warp-weft/.git/worktrees/target-weft/fabric-merge.json` — the linked shape, not the prime shape.
  With that as the ONLY record on disk, `lyx fabric remove task-c` refused
  (`a merge is in progress; run MergeContinue or MergeAbort first`), so `mergeSourceInFlight`'s
  linked-worktree glob is correct in production. No test covers it (S-28b) → F7.
- Weft-side-ONLY foreign state, built properly (first attempt degraded to a clean fast-forward and
  proved nothing — asserted the precondition instead of assuming it): `fx` and `fy` given conflicting
  `_lyx/raddle/clash.md`, then a raw `git merge fy-weft` **inside `fx-weft` only**. Asserted state:
  `fx-weft` MERGE_HEAD live + `_lyx/raddle/clash.md` unmerged; `fx` warp MERGE_HEAD none, conflicted
  none, `status --porcelain` empty. Then `lyx fabric merge-in fy` and `lyx fabric merge --abort` BOTH
  returned `git merge state exists that fabric did not start…`, and `fx-weft`'s MERGE_HEAD was
  unchanged afterwards. Production behaviour correct; no test creates this state (S-8) → F4.

## Findings

Eight findings: 1 behavioral defect (F5), 1 operability gap (F1), 5 proof-quality/doc gaps, 1 NIT
hardening. Counts by severity: MEDIUM 5, LOW 2, NIT 1. No BLOCKING.

### F5 — `Merge`'s not-synced guard is defeated by evaluation order and merges over a genuine divergence — MEDIUM, CONFIRMED

`internal/fabricengine/merge.go:352` (guard call), `internal/fabricengine/mergeguards.go:184-237`
(`syncedToUpstreamReason` / `sideNotSyncedToUpstream`), `internal/fabricengine/merge.go:519-561`
(`syncSideBeforeMerge`).

`Merge` evaluates `syncedToUpstreamReason` in its guard stage, at merge.go:352. That predicate reads
`@{u}` through `upstreamSHAAt`, i.e. the **remote-tracking ref as it stands before this call fetches
anything** — the call's first fetch happens later, inside `resolveMergeSources` (merge.go:358), and a
second one inside `syncSideBeforeMerge` (merge.go:397-402). So the guard's verdict is computed from
stale knowledge, and the call then goes on to acquire the very knowledge that would have changed it
and does nothing with it.

`syncSideBeforeMerge` is where the fresh knowledge lands, and it discards the divergence case
silently: after fetching and re-resolving it computes `behind` (merge.go:548) and, when `behind` is
false, `return nil` — which lumps "ahead of upstream" (fine) together with "genuinely diverged from
upstream" (the exact state `mergeReasonNotSynced` exists to refuse).

Failure scenario, reproduced live end-to-end against real bare repos:

1. Pair `target` has upstreams on both sides (`origin/target`, `origin/target-weft`).
2. Someone else pushes to `origin/target`. `target` has not fetched.
3. `target` makes one local commit. It is now genuinely diverged, but its `origin/target` still
   points at the old tip.
4. `lyx fabric merge task-b` → `{"ok":true,"committed":true,…}`, exit 0.
5. Afterwards `git rev-list --left-right --count 'HEAD...@{u}'` = `3	1`, and `origin/target` has
   advanced to the real remote tip **because this call fetched it**.

The documented contract (`mergeReasonNotSynced`, and doc.go's Merge narrative) says such a target
refuses. It does not. `TestMerge_DivergedTargetRefuses` passes only because its fixture hand-runs
`git fetch` first and says so in a comment (`merge_target_integration_test.go:332-334`,
"the guard stage never fetches on its own") — the test pre-arranges the single condition under which
the guard works, so it proves the predicate, not the verb.

Sabotage corroboration: **S-13** — deleting the behind-passes clause from `sideNotSyncedToUpstream`
leaves the whole suite green, because no test ever reaches that clause; and **S-12** — deleting the
weft half of `syncedToUpstreamReason` also leaves the suite green.

Suggested fix: make `syncSideBeforeMerge` refuse on the post-fetch divergence it already detects —
classify the three cases explicitly (equal → no-op; behind → `MergeFFOnly` + record; ahead → no-op;
neither → `*MergeGuardError{mergeReasonNotSynced}`) instead of collapsing ahead and diverged into one
`return nil`. Keep the pre-lock guard as the cheap fast path. Add an integration test for the
unfetched-divergence route (the one the live drive above walks), plus tests for the weft-side
divergence and the behind-passes clause so S-12/S-13 are closed.

### F1 — the CLI merge lifecycle cannot be completed by an operator when a conflict lands on the weft side — MEDIUM, CONFIRMED

`internal/fabriccli/merge_verbs.go:53-71` and `:87-106` (the two Long texts),
`internal/fabricengine/mergestage.go:50` (`MergeStageResolved`, no CLI surface),
`internal/fabricengine/doc.go:983-996, 1052-1057`.

`merge-in`'s help tells the operator that a conflicting merge is "concluded with
`lyx fabric merge --continue` … once every conflict is resolved", and `MergeResult.Conflicts` hands
them a unified, worktree-relative path list to resolve. `MergeContinue`'s guard is an **index** probe
(`ConflictedFiles()`), which no amount of content editing clears — the path must be staged.

For a warp-side path the operator stages it with `git add`. For a weft-side path — anything under a
wired junction name, i.e. every `_lyx/…` conflict — they cannot, because the path is only reachable
through the junction and git refuses to stage through it. Reproduced live:

```
$ git add _lyx/raddle/notes.md
fatal: pathspec '_lyx/raddle/notes.md' is beyond a symbolic link      (exit 128)
$ lyx fabric merge --continue
{"error":"fabricengine: merge preconditions failed: unresolved conflicts remain", …}
```

The engine has exactly the verb needed — `MergeStageResolved`, which partitions unified paths onto
whichever side's index lists them unmerged — but it is registered on no cobra command. Its only
caller is `internal/mergeresolve` (the LLM-driven resolver). So the operator-facing lifecycle the CLI
help documents is completable only by reaching into the weft checkout with raw git, which is precisely
what the Hub Containment / Fabric-illusion posture says an operator never has to do, and which the
help text never mentions. I finished my own live merge that way and it worked — but a user following
the shipped help has no way to discover it.

Suggested fix: register the existing engine verb as `lyx fabric merge-stage <path>...` in
`addMergeVerbs`, joining the weft-verb family exactly as `merge`/`merge-in` do, with the same
`setMergeExit`-style envelope; then correct `merge-in`'s and `merge`'s Long text and doc.go's
conflict-reporting paragraph to name the real three-step operator route (resolve → `merge-stage` →
`merge --continue`). Add CLI tests covering a weft-side path, a warp-side path, and the
not-conflicted-on-either-side error.

### F2 — round 6's Windows path fix is unguarded against its own deletion on every host lyx builds on — MEDIUM, CONFIRMED

`internal/fabricengine/mergepaths.go:55` (`slashAnchorRel := filepath.ToSlash(anchorRel)`),
`internal/fabricengine/mergepaths_test.go:43-67`.

`filepath.ToSlash` is the identity function whenever `os.PathSeparator == '/'`, so on Linux and macOS
**no test can distinguish the call from its absence**. Proven, not inferred (S-d): replacing the line
with `slashAnchorRel := anchorRel` leaves the entire hermetic + integration suite green. The one row
that does bite (S-e, `single anchor segment containing a backslash is not split`) discriminates
`ToSlash` from a blanket `strings.ReplaceAll` — a different wrong implementation — and says nothing
about the conversion being present at all.

So round 6's F6 fix is a real fix with a guard that cannot fail on any host this campaign has, two
instalments and seven rounds in. A later refactor deleting the call (it reads like decoration) would
reintroduce the exact defect — every `_lyx` conflict under a multi-segment anchor on Windows
misclassified as unmappable, self-aborting the whole merge with `*ErrUnmergeableState` — and CI would
stay green.

This is also the honest answer to the round's "state plainly whether you can do anything more than
round 6 already did about Windows". Executing on real Windows: no, no host exists. But the separator
logic itself is pure and can be made **host-independent** rather than host-dependent, and then it is
fully exercisable here.

Suggested fix: give `weftPathVisible` a separator-parameterised inner form
(`weftPathVisibleSep(weftPath, anchorRel, wiredNames, sep rune)`) doing the conversion explicitly, with
`weftPathVisible` calling it at `os.PathSeparator`. A table test then drives `sep = '\\'` and asserts
that `apps\backend` maps a `apps/backend/_lyx/…` conflict, and drives `sep = '/'` and asserts that
`weird\name` is NOT split — two rows that fail on Linux both if the conversion is deleted and if it is
made a blanket replace. That converts the never-executed Windows clause into one this host really runs.

### F4 — `foreignMergeStatePresent`'s weft half and its two probe kinds are all unproven — MEDIUM, CONFIRMED (proof-quality)

`internal/fabricengine/mergestate.go:240-259`.

Three independent sabotages of this one four-probe expression each leave the whole suite green:

- **S-8** — drop BOTH weft probes (`weftMergeHead`, `weftConflicted`): PASS.
- **S-10** — drop the warp conflicted-index probe: PASS.
- **S-11** — drop the warp `MERGE_HEAD` probe: PASS.

The function's own godoc claims the state is "checked on both warp and weft" and that "All four probes
are evaluated unconditionally … so no evaluation-order timing difference ever leaks which side (if
either) carries the state". The tests back neither half of that. Every existing foreign-state fixture
builds a **conflicted warp** merge, which sets `warpMergeHead` and `warpConflicted` simultaneously, so
either one alone carries the assertion and the weft side is never populated at all.

The two probe kinds are genuinely different states, not redundant spellings: a foreign merge that is
resolved-but-not-concluded has a live `MERGE_HEAD` and an EMPTY unmerged set, and only the
`MERGE_HEAD` probe sees it. And weft-side-only foreign state is the more likely one in this system —
the weft checkout is the hidden side an operator is told not to touch, so state appearing there is
exactly the state nobody is watching.

Live-verified the production code is correct (see "What was tested"): weft-only foreign state made
both `merge-in` and `merge --abort` return `*ErrForeignMergeState` and left the state untouched. This
is a proof gap, not a live defect — but it is the gap that would let a refactor silently drop the weft
half.

Suggested fix: an integration test matrix over the four foreign-state shapes — warp-conflicted,
warp-`MERGE_HEAD`-only (resolved, unconcluded), weft-conflicted, weft-`MERGE_HEAD`-only — each
asserting `*ErrForeignMergeState` from a mutating merge verb and that the foreign state is byte-identical
afterwards, with the precondition (`MERGE_HEAD` present / unmerged set empty or not) asserted with
`t.Fatal` so a degraded fixture cannot pass vacuously.

### F7 — `mergeSourceInFlight`'s linked-worktree scan — the common real-world case — is unproven — MEDIUM, CONFIRMED (proof-quality)

`internal/fabricengine/mergestate.go:218` (the `.git/worktrees/*/` glob).

**S-28b** — point the linked-record glob at a filename that cannot exist: suite green. **S-31** —
point the PRIME record path at a filename that cannot exist: caught by
`TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming`. So the single test covering this
guard exercises only the prime pair's record location.

That is the wrong one to cover in isolation. A merge run from any pair other than the prime writes its
record to `<weft repo>/.git/worktrees/<weft worktree name>/fabric-merge.json`, and task pairs are
where merges normally run — the prime pair is the exception. Confirmed on the live hub: `merge-in` run
from the linked pair `target` put its record under `warp-weft/.git/worktrees/target-weft/`, and with
that as the only record on disk `lyx fabric remove task-c` correctly refused. Production is right; the
proof covers the rarer half.

Suggested fix: extend the existing test (or add a sibling) so the in-flight record lives in a LINKED
pair's weft gitdir, asserting the record's on-disk location with `t.Fatal` before asserting the
refusal, so the glob is what carries the assertion.

### F3 — `gitrepo.StageResolved`'s stated reason for `git add -A` is false on any modern git, and the flag is unpinned — LOW, CONFIRMED

`internal/gitrepo/merge.go:169-185`.

The godoc says: "It runs `git add -A -- <paths>`, the -A form rather than the plain form
StageAndCommit uses, because a delete/modify conflict is legitimately resolved by the file being gone:
the plain `add --` form errors on a missing pathspec, while -A stages the removal."

The second half is checkably false. Probed directly on this host (`git version 2.53.0`): a
modify/delete conflict resolved by deleting the file stages with plain `git add -- f.txt`, exit 0,
and `diff --diff-filter=U` is empty afterwards. Git made `git add <pathspec>` stage removals in 2.0
(2014); the documented rationale describes pre-2.0 git.

Corroborated by sabotage **S-4**: rewriting `add -A --` to `add --` leaves the suite green, including
`TestMergeStageResolved_DeleteModifyConflictResolvedByDeletion` — which passes for the wrong reason,
since on this git both forms behave identically.

The `-A` itself is not wrong and should stay: it pins behaviour rather than inheriting a
version-dependent default, exactly the posture `MergeStart`'s `--ff` and `MergeConclude`'s `--no-edit`
comments already argue for. What has to change is the reason given, which a reader can check and find
false — the specific failure mode this campaign exists to catch.

Suggested fix: restate the rationale as a version-pin (plain `add <pathspec>` only acquired
removal-staging in git 2.0, so `-A` states what fabric depends on instead of inheriting it), and say
plainly that the two forms are indistinguishable on a modern git so no test can separate them — rather
than leaving a claim that reads as though a test could.

### F6 — `bothSidesAlreadyUpToDate`'s weft conjunct is unproven — LOW, CONFIRMED (proof-quality)

`internal/fabricengine/mergestate.go:99-101`.

**S-24** — rewrite `st.WarpOutcome == up_to_date && st.WeftOutcome == up_to_date` to test the warp
outcome alone: suite green. No test covers a record with warp `up_to_date` and weft NOT `up_to_date`
asserting that `AlreadyUpToDate` is false.

That combination is not exotic — it is exactly what `TestMergeIn_OneSideAlreadyUpToDate_OtherMerges`
is named for, and doc.go's "What the result flags mean" paragraph makes `AlreadyUpToDate` a promise
about **both** sides ("whether the attempt found both sides already carrying the resolved source").
With the conjunct half-dropped, a merge that really moved the weft side would report
`already_up_to_date: true` to every consumer of the envelope.

Suggested fix: a direct unit test over `bothSidesAlreadyUpToDate` (or an assertion added to the
one-side-up-to-date integration test) covering all four outcome combinations, so each conjunct is
independently load-bearing.

### F8 — `resolveMergeSources` discards the weft side's found-ness, so an unresolvable weft source can reach `git merge ""` — NIT, PLAUSIBLE

`internal/fabricengine/mergeguards.go:67`
(`weftSHA, _ := pickMergeSourceSHA(f.weft, weftLocalSHA, weftLocalErr == nil, weftRemoteSHA, weftRemoteErr == nil)`).

The warp side checks `warpFound` and appends `mergeReasonSourceNotFound` when the source resolved on
neither local nor remote (mergeguards.go:53-56). The weft side deliberately drops the same boolean and
relies instead on a *different* predicate, `weftManaged := weftBranchExists(l, weftBranch) ||
weftRemoteErr == nil`. Those two are not the same test: `weftBranchExists` is a raw
`git rev-parse --verify refs/heads/<branch>` at the weft repo root, while `pickMergeSourceSHA`'s local
arm is a go-git `ResolveRevision` in the weft worktree. When the first succeeds and the second does
not, `weftManaged` is true, no reason is appended, and `weftSHA` is the empty string — which is then
handed to `f.weft.MergeStart("")`, i.e. `git merge --ff --no-commit ""`.

The blast radius is contained (the git error routes into `selfAbortMergeAttempt`, which resets both
sides and deletes the record), so this is a confusing-error finding rather than a corruption one, and
I could not construct a route that makes the two probes disagree on real git — hence PLAUSIBLE, not
CONFIRMED. But the asymmetry is free to remove and the guard set is meant to be exhaustive.

Suggested fix: keep `weftManaged` as the fabric-managed check it is, and additionally append
`mergeReasonSourceNotFound` when the weft pick reports `found == false`, so no verb can ever hand an
empty ref to `MergeStart`. Both reasons aggregate, which is the file's existing rule.

## Deferred items — re-evaluated

- **Windows path behaviour.** I cannot execute on real Windows either; no host exists. But I CAN do
  more than round 6 did, and F2 is that: the separator logic is pure and can be made host-independent
  and driven with an explicit separator on this host, which turns the never-executed clause into an
  executed one. Fixed under F2 rather than carried forward again.
- **Round 6's own new mechanisms.** Re-sabotaged individually (S-a/b/c/e/f/g/h/i). Every one is caught
  by its own distinct test and fails for the right reason, EXCEPT the `filepath.ToSlash` conversion
  (S-d) → F2. Round 6's behavioral fixes are sound.
- **Four states where `MergeContinue` gets stuck** (first instalment round 2, rows 27/28/30/31): not
  touched by anything in this round; re-confirmed unchanged.
- **The post-record error-return class's per-site adjudication:** not required this round; not
  attempted.
