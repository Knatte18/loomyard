# `fabric merge` — independent review (round `opus-medium-r1`)

Reviewer: crucible round agent, clean-room, round 1 of the `fabric merge` campaign.
Surface under review: the merge primitive as shipped by `a2bf44e2`.

> Sections below are appended as work proceeds (log-as-you-go).
> The executive summary and the final severity ordering are written last.

## What was tested

(appended per command/scenario)

## Findings

(appended as formed)

### Gates (baseline, committed tree)

- `go build ./...` — **green**.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — **green** (no output).
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — **green** (tag: none / hermetic).
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — **green** (tag: `integration`; fabricengine 23.8 s, fabriccli 2.4 s, gitrepo 1.6 s).
- `./deploy-dev` — deployed `lyx @ 125757f4` to `.dev-bin/lyx`; all live driving below runs that binary.

### Live substrate

Scratch hubs are built outside the worktree, under the session scratchpad, by
`mkhub.sh` (bare warp remote with a seeded `main`, empty bare weft remote, then
`lyx fabric clone <weft-bare> <warp-bare>`) and `mkscen.sh <name> <mode>` which adds a
`task1` pair and creates a `clean` / `conflict` / `weftonly` divergence.
Each destructive scenario gets its own freshly built hub. `GIT_CONFIG_GLOBAL` is
pinned per hub so the host's git config never leaks in.

### Scenario L1 — plain two-sided `merge` that fast-forwards both sides (hub `h1`)

`lyx fabric merge task1` after both branches advanced ahead of `main`/`main-weft`:

```
{"already_up_to_date":false,"committed":true,
 "mutations":[{"kind":"merge_staged","target":"warp-bare","detail":"d140036…"},
              {"kind":"merge_staged","target":"warp-bare-weft","detail":"e824beb…"}],
 "ok":true,"partial":false}
```

Both sides fast-forwarded (warp log shows `task1: warp f1` as HEAD, no merge commit).
Observations: `committed: true` although no conclude-commit landed on either side, and
no `merge_committed` entry appears. `KindMergeStaged`'s godoc does say it covers the
fast-forward case, so the mutation kind is intentional — the `committed` flag is not
documented anywhere and is what a script would read. See finding **F6**.

### Scenario L2 — `merge-in` conflict → sibling refusals → `--continue` (hub `s_conflict`)

`lyx fabric merge-in task1` with a conflicting `shared.txt` on warp and
`_lyx/shared.txt` on weft:

```
{"conflicts":["_lyx/shared.txt","shared.txt"],
 "error":"merge produced conflicts; resolve them, then run \"lyx fabric merge --continue\"",
 "mutations":[{"kind":"merge_staged",…}×2],"ok":false,"partial":false}   exit 1
```

Path unification is correct (weft `_lyx/…` is a wired name at anchor `.`, so it maps by
identity). `fabric-merge.json` in the weft gitdir records
`warp_outcome=conflicted, weft_outcome=conflicted`. Both checkouts show `AA` for the
conflicted path — record and git agree.

Sibling verbs while the record exists (all run from the prime warp worktree):

| verb | result |
| --- | --- |
| `fabric commit` | refused, `ErrMergeInProgress` |
| `fabric pull` | refused, `ErrMergeInProgress` |
| `fabric push` | refused, `ErrMergeInProgress` (commit half) |
| `fabric sync` | refused, `ErrMergeInProgress` |
| `fabric checkout task1` | refused, `ErrMergeInProgress` |
| `fabric remove task1` | **SUCCEEDED** — removed the merge's source pair and deleted branch `task1-weft` mid-merge (finding **F5**) |
| `fabric add task2` | refused, but with the unrelated `source worktree has uncommitted changes` message |
| `fabric cleanup --apply` | ran unguarded (no-op here) |
| `fabric prune --apply` | ran unguarded (no-op here) |
| `fabric reconcile` | ran unguarded (no-op here) |

Resolving both sides and running `lyx fabric merge --continue` concluded correctly:
two `merge_committed` entries, a merge commit on each side, record deleted.
Weft merge-commit subject is `Merge commit '<sha>' into main-weft` — SHA-labelled, no
branch name leaked, matching `doc.go`'s claim.

### Scenario L3 — crash between the warp and weft `MergeStart` (hub `s_crash`) — CONFIRMED DEFECT

State reconstructed exactly as a kill in that window leaves it: `git merge --no-commit
<task1-sha>` run in the warp checkout, weft untouched, and `fabric-merge.json` written
with `warp_outcome:"staged"`, `weft_outcome:""` (this is byte-for-byte what
`merge.go:141-148` persists before it calls the weft `MergeStart`).

From a fresh process:

```
$ lyx fabric merge --continue
{"error":"fabricengine: merge conclude did not finish; run MergeContinue again",
 "mutations":[{"kind":"merge_committed","target":"warp-bare","detail":"21835d5…"}],
 "ok":false,"partial":true}
```

- The warp merge commit **landed**.
- The weft conclude then failed (`git commit --no-edit` on a clean tree), so the pair is
  now out of correspondence and no correspondence entry was recorded.
- Every subsequent `--continue` fails identically with zero mutations — the instruction
  in the error message ("run MergeContinue again") can never succeed.
- Only `--abort` recovers; it correctly reset both sides to `warp_start`/`weft_start`
  and deleted the record.

See finding **F1**.

### Scenario L4 — unmappable weft conflict path (hub `s_unmap`)

A conflict on `outside.txt` at the weft repo root — outside every wired junction name — with a
clean warp-side change. `lyx fabric merge-in task1` returned `ErrUnmergeableState`, and both
sides were restored: warp HEAD and weft HEAD identical to their pre-merge values, `git status`
clean on warp, no `MERGE_HEAD` on either side, `fabric-merge.json` deleted.
**Sabotage/reach proof:** `*ErrUnmergeableState` is produced at exactly one site
(`merge.go:187`, the `unmappable` arm of `unifyConflictPaths`); observing that error is proof the
arm executed. The mutation record also carries the two `worktree_reset` entries only that arm's
`resetMergeSides` call can append.

### Scenario L5 — squash (hubs `s_squash`, `s_sqconf`)

- Clean squash: both sides squash-committed (`Squashed commit of the following:`), four mutation
  entries (2 × `merge_staged`, 2 × `merge_committed`), no `SQUASH_MSG` left behind.
- Conflicted squash: self-aborted to `ErrMergeInRequired`, both HEADs restored, worktrees clean,
  **no leftover `SQUASH_MSG` / `MERGE_MSG`** in either gitdir, record deleted.
- Contamination probe: running `merge-in` + `--continue` immediately after the aborted squash
  produced the correct `Merge commit '<sha>'` message on both sides — no `SQUASH_MSG` bleed.

### Scenario L6 — foreign git merge state (hubs `s_foreign`, `s_foreign2`)

| plain-git state left in the warp checkout | `MERGE_HEAD`? | fabric's answer |
| --- | --- | --- |
| conflicted `git merge` | yes | `ErrForeignMergeState` from `merge-in`, `merge`, `--continue` and `--abort` — foreign state left untouched |
| conflicted `git merge --squash` | **no** | `ErrForeignMergeState` (caught by the unmerged-index-entry half of the probe) |
| clean `git merge --squash`, staged | no | not "foreign", but refused by the dirty guard: `merge preconditions failed: worktree dirty` |
| detached HEAD after a completed `git rebase --onto` | no | **NOT refused — the merge ran (finding F2)** |

### Scenario L7 — merge onto a detached warp HEAD (hub `s_detach`) — CONFIRMED DEFECT

`git checkout --detach` in the warp checkout, then `lyx fabric merge-in task1`:

```
{"already_up_to_date":false,"committed":true,
 "mutations":[merge_staged×2, merge_committed×2],"ok":true,"partial":false}
warp HEAD: 84a4c518…   main: 8fde06ed…   branch: HEAD
weft HEAD: 7a818723…   main-weft: 7a818723…
```

The warp merge commit `84a4c518` is reachable from **nothing** — `main` never moved — while the
weft merge commit is permanently on `main-weft`. Correspondence was recorded pairing an orphan
warp SHA with a live weft SHA, and the record was deleted, so `MergeAbort` is no longer
available. The next `git checkout <branch>` in the warp checkout silently discards the warp half
of a merge whose weft half has already landed. See finding **F2**.
`internal/websterengine/integration.go:133` drives `Fabric.CheckoutDetached`, so this window is
reachable in production, not only by hand.

### Scenario L8 — source-ref resolution edge cases (hub `s_src`)

All from `lyx fabric merge-in <arg>`; none reached git with an attacker-shaped ref:

| arg | result |
| --- | --- |
| `plain-branch` (warp-only branch, no `-weft` counterpart) | `source branch is not fabric-managed` |
| `v1` (tag) | `source branch is not fabric-managed` |
| a raw 40-hex SHA | `source branch is not fabric-managed` |
| `HEAD` | `source branch is not fabric-managed` |
| `nope` | `source branch is not fabric-managed; source branch not found` |
| `task1/../task1` | `source branch is not fabric-managed; source branch not found` |
| `refs/heads/task1` | `source branch is not fabric-managed` |
| `origin/task1` | `source branch is not fabric-managed` |
| `-x` (after `--`) | `source branch is not fabric-managed; source branch not found` |
| `--squash-ish` | cobra: `unknown flag` |

The not-fabric-managed guard is the effective gate for everything that is not a
`<branch>`/`<branch>-weft` pair, and `MergeStart`'s leading-`-` pre-check is never even reached.
No defect found here.

### Scenario L9 — hostile git config (hubs `s_hostile`, `s_hostile2`, `s_hostile3`)

`GIT_CONFIG_GLOBAL` loaded with `core.editor = sh -c 'sleep 600'`, `commit.template =
/nonexistent/template.txt`, `commit.verbose = true`, `merge.ff = only`, `pull.rebase = true`,
and (third hub) `commit.gpgsign = true` with no key. Every command run under `timeout 60`.

- **No hang anywhere.** `MergeConclude("")`'s `git commit --no-edit` concluded a conflicted
  `merge-in` in well under a second with the blocking editor configured and a missing
  `commit.template`. The `--no-edit` contract holds. (rc 124 was never observed.)
- `commit.gpgsign = true` with no key: the conclude fails, `ErrMergeIncomplete` is returned with
  an **empty** mutation record and `partial:false` (honest — nothing landed), the record is
  retained, and `--abort` recovers both sides cleanly.
- **`merge.ff = only` breaks every non-fast-forward merge**: `git merge --no-commit <sha>` exits
  128 with `fatal: Not possible to fast-forward, aborting.`, which `MergeStart` classifies as a
  genuine error, so `selfAbortMergeAttempt` resets both sides and the verb fails. See finding
  **F4**.

### Scenario L10 — concurrency (hubs `r1`, `r2` interleaved; `rc1` sequential control)

Two `lyx fabric merge-in task1` processes started simultaneously against the same pair, on two
independent hubs, plus a strictly sequential control of the identical command pair.

| run | winner | loser |
| --- | --- | --- |
| hub `r1` (interleaved) | `committed:true`, 4 mutation entries | `{"already_up_to_date":false,"committed":true,"mutations":[]}` |
| hub `r2` (interleaved) | `committed:true`, 4 mutation entries | `{"already_up_to_date":false,"committed":true,"mutations":[]}` |
| hub `rc1` (sequential control) | `committed:true`, 4 mutation entries | `{"already_up_to_date":true,"committed":false,"mutations":[]}` |

Final on-disk state was correct and identical in all three runs (both HEADs merged, worktrees
clean, no record, warp still on `main`) — **no corruption**. The defect is the report: the loser
claims `committed:true, already_up_to_date:false` for a call that committed nothing, where the
sequential control of the same two commands honestly reports `already_up_to_date:true,
committed:false`. Reproduced on two independent hubs with a sequential control. See finding **F3**.

Also driven: `merge --continue` racing `merge --abort` on a conflicted pair, two independent hubs.
Abort won both times; continue failed with `ErrMergeIncomplete` and zero mutations; both sides
ended reset to their pre-merge SHAs with the record deleted and clean worktrees. No defect.

Not reproduced, therefore **not reported as a finding**: a sibling verb slipping through its
unlocked guard window between `mergeRecordExists()` and `saveMergeState`. I reasoned about it but
never made it happen, and the campaign rule says that is not a finding.

### Scenario L11 — sibling verbs against a pair that is genuinely mid-merge (hub `s_pair`)

`lyx fabric merge-in main` run **inside** the `task1` pair, leaving a conflicted merge whose
record lives at `warp-bare-weft/.git/worktrees/task1-weft/fabric-merge.json` (per-pair, correctly
isolated from the prime pair's record).

From the prime warp worktree: `remove task1` correctly refused with `ErrMergeInProgress`;
`checkout task1` failed with git's own "already used by worktree" (the merge guard is scoped to
the *current* pair, which is correct); `cleanup --apply`, `prune --apply`, `reconcile` and
`add task9` all ran unguarded (see the enumeration table).
From inside the mid-merge pair: `commit`, `pull`, `push`, `sync` all refused with
`ErrMergeInProgress`. The pair stayed `AA shared.txt` throughout.

### Scenario L12 — CLI arity, flag combinations, exit codes

| invocation | result |
| --- | --- |
| `merge --continue --abort` | cobra mutually-exclusive error |
| `merge --continue x` | `usage: … (--continue \| --abort) takes no positional arguments` |
| `merge` | `usage: lyx fabric merge <branch> [--squash] [-m <message>]` |
| `merge a b` | same usage error |
| `merge-in` | `accepts 1 arg(s), received 0` |
| `merge --squash --abort` | `usage: --squash cannot be combined with --continue or --abort` |
| `merge --abort -m msg` | **`-m` silently accepted and ignored** (finding **F8**) |
| conflicted `merge-in` | exit **1**, `ok:false`, `conflicts:[…]` |
| `merge --abort` with no merge | exit **1**, `ok:false`, no `conflicts` key |

A conflicted merge and a hard error are **not** distinguishable by exit status; only the presence
of the `conflicts` key separates them. That is a deliberate consequence of the shared envelope,
but it is documented nowhere. See finding **F9**.

## Enumerated class — every mutating `fabricengine` entry point vs. a live merge record

**Enumeration method (reproducible):**

```sh
FILES=$(ls internal/fabricengine/*.go | grep -v _test.go)
grep -h "^func (f \*Fabric) [A-Z]\|^func (t \*Topology) [A-Z]\|^func [A-Z][A-Za-z]*(" $FILES | wc -l   # 80
grep -rn "mergeBlocksMutation\|ErrMergeInProgress" internal/ --include=*.go | grep -v _test.go        # guard sites
```

80 exported entry points in `fabricengine`. Of those, 27 mutate on-disk or git state; the other
53 are constructors, path/name derivations, config loads and read-only probes
(`List`, `Status`, `Diff`, `Healthy`, `Ready`, `Clean`, `MergeInProgress`, `WeftSHAForWarpSHA`,
`RepoWiredNames`, `WeftBranchName`, …) and cannot corrupt or be corrupted by a merge.
The delta versus a naive `grep -c mergeBlocksMutation` (which finds 2 sites, in `checkout.go` and
`remove.go`) is that `commit.go` and `pull.go` call `f.mergeRecordExists()` directly rather than
the helper, and `commit.go` additionally arms on `foreignMergeStatePresent()`; the guarded set is
therefore 4 verbs across 5 refusal sites, not 2.

| # | mutating entry point | guarded? | adjudication |
| --- | --- | --- | --- |
| 1 | `Fabric.MergeIn` | self (`mergeReasonAlreadyInProgress` + foreign) | correct |
| 2 | `Fabric.Merge` | self (same) | correct |
| 3 | `Fabric.MergeContinue` | record-driven | correct — but see **F1** |
| 4 | `Fabric.MergeAbort` | record-driven | correct |
| 5 | `Fabric.Commit` | **yes** (record + foreign) | correct; driven in L2/L11 |
| 6 | `Fabric.Pull` | **yes** (record) | correct; driven in L2/L11 |
| 7 | `Topology.Checkout` | **yes** (record, current pair) | correct; driven in L2/L11 |
| 8 | `Topology.Remove` | **yes** (record, pair being removed) | correct for the pair being removed; **does not cover the pair that is the merge's SOURCE — F5** |
| 9 | `Topology.Add` | no | safe — creates a *new* pair from a branch tip, never touches the mid-merge pair's index or HEAD. Driven from prime mid-merge: refused incidentally by the source-worktree-dirty check; driven from prime while the *pair* is mid-merge (L11): succeeded, correctly. |
| 10 | `Topology.Cleanup` | no | safe by construction — a weft branch that is checked out at any worktree is unconditionally protected (`cleanup.go` file header), and a mid-merge pair has both worktrees materialized. Driven in L2/L11: no-op. |
| 11 | `Topology.Prune` | no | safe — only acts on a pair whose warp worktree directory is already gone, or a weft worktree with no warp sibling. A mid-merge pair has both. Driven in L2/L11: no-op. |
| 12 | `Topology.Reconcile` | no | safe — repairs junction links and creates missing weft worktrees; touches no index, HEAD or ref of a materialized pair. Driven in L2/L11: `already_healthy`. |
| 13 | `CloneHub` | no | safe — different hub; cannot address an existing pair. |
| 14 | `Fabric.PushWeft` | no | **deliberate** (spec) — pushes the committed branch tip, which an uncommitted merge has not moved. |
| 15 | `PushWarpAt` | no | deliberate (spec) — same reason. |
| 16 | `CoalescePushBothAt` | no | deliberate (spec) — same reason. |
| 17 | `SpawnDetachedPush` | no | deliberate (spec) — spawns the push half only. |
| 18 | `Fabric.RebuildIndex` | no | safe — rewrites the correspondence cache only, which `doc.go` already pins as explicitly rebuildable. No git mutation. |
| 19 | `Fabric.RecordCorrespondence` | no | safe, and *required* to stay unguarded: the merge verbs call it themselves while their own record is still live. |
| 20 | `Fabric.ResetHard` | no | incidentally safe — `force:false` plus `dirtyScopeTracked()` refuse against a conflicted or staged merge worktree, which is dirty by definition. |
| 21 | `Fabric.CheckoutDetached` | no | **hazard, accepted this round**: it would abandon a merge in progress. It is a raw primitive, not a verb, driven only by `internal/websterengine/integration.go`. F2's new detached-HEAD merge guard closes the harmful direction (a merge *starting* while detached); guarding the primitive itself belongs to webster's own surface, not the merge primitive's. |
| 22 | `Fabric.RestoreBranch` | no | same as #21, paired with it. |
| 23 | `WireJunctions` / `WireJunctionsWith` | no | safe — filesystem links only, no git state. |
| 24 | `UnwireJunctions` | no | safe — filesystem links only. |
| 25 | `Unwire` | no | safe — filesystem links only. |
| 26 | `InstallPostCheckoutHook` | no | safe — writes a hook file. |
| 27 | `CommitSeededStencils` | no | safe — commits into the board worktree, a different pair from any task pair, and the prime pair's own board branch is not a merge subject. |

Total mutating entry points: **27**. Guarded: **4** (rows 5-8). Self-guarding: **4** (rows 1-4).
Deliberately unguarded per the spec: **4** (rows 14-17). Adjudicated safe: **13**.
Accepted hazards outside this surface: **2** (rows 21-22).
Genuine gap found: **1** (row 8 — finding **F5**).

## Scope assessment — plan vs. shipped

The six plan batches recovered from `3b800bc8` are all present in the tree:

| batch | shipped |
| --- | --- |
| 01 gitrepo merge primitives | `internal/gitrepo/merge.go` — `MergeStart` (4-way classification), `MergeConclude`, `ConflictedFiles`, `MergeHeadPresent`, `MergeFFOnly`, `ResolveSHA`. Complete. |
| 02 merge state, errors, mapping | `mergestate.go`, `mergeerrors.go` (7-member closed reason set + 6 typed errors), `mergepaths.go`. Complete. |
| 03 mergein + lifecycle | `merge.go` `MergeIn`, `mergelifecycle.go` quartet. Complete, with the F1 recovery gap. |
| 04 merge target verb | `merge.go` `Merge` + `syncSideBeforeMerge`, squash, `ErrMergeInRequired`. Complete. |
| 05 sibling guards + vocabulary | 4 guarded verbs + `mergevocab_test.go`. Complete as specified; `doc.go` overstates it (**F6**). |
| 06 CLI + docs | `merge_verbs.go`, `envelope.go` `errConflictsWithRecord`, `doc.go` "# The merge surface". Complete. |

No silently-dropped requirement and no shipped-beyond-scope surface found. The deferred
two-sided reset-to-SHA verb for a *landed* merge is correctly recorded as deferred and is
explicitly out of scope for this campaign.

## Findings

### F1 — BLOCKING — CONFIRMED — `MergeContinue` lands half a merge, then dead-ends forever, when the recorded attempt never reached one side

`internal/fabricengine/mergelifecycle.go:26` (`concludeMergeSides`), reached from
`mergelifecycle.go:87` (`MergeContinue`); the window is created at
`internal/fabricengine/merge.go:141-156` and `merge.go:305-320`.

**Scenario** (driven, L3): a crash between the warp `MergeStart` and the weft `MergeStart` leaves
`fabric-merge.json` with `warp_outcome:"staged"`, `weft_outcome:""` — exactly what `merge.go`
persists in that window. A resumed `MergeContinue`:
1. concludes and **commits** the warp side,
2. then calls `MergeConclude("")` on the weft side, which was never started, so `git commit
   --no-edit` fails on a clean tree,
3. returns `ErrMergeIncomplete` — *"run MergeContinue again"* — which can never succeed, since
   the weft side will still be clean on every retry,
4. leaves the pair out of correspondence with a warp merge commit and no weft counterpart.

Only `--abort` recovers. This violates the prompt's invariant that `MergeContinue` be idempotent
across a resumed run: the first call produces an irreversible commit and every later call is a
no-op failure.

**Fix**: an empty recorded outcome means "the attempt never reached this side"; `MergeContinue`
must detect that **before** landing anything and refuse the whole call with a reason directing the
operator at `--abort`, so the pair is never left half-concluded.

### F2 — BLOCKING — CONFIRMED — no merge verb requires the checkouts to be on a branch

`internal/fabricengine/merge.go:78-96` and `merge.go:245-268` (the two guard aggregations);
`internal/fabricengine/mergeguards.go` has no detached-HEAD predicate at all.

**Scenario** (driven, L7): with the warp checkout on a detached HEAD, `lyx fabric merge-in task1`
reports full success, lands a warp merge commit reachable from no ref, lands the weft merge commit
permanently on `main-weft`, records correspondence between the orphan warp SHA and the live weft
SHA, and deletes the record — so `MergeAbort` is gone. The next branch checkout in the warp
checkout silently discards the warp half of an already-half-landed merge. `fabricengine` itself
exposes `CheckoutDetached`, driven by `internal/websterengine/integration.go:133`, so this is a
reachable production window, not only a hand-made one.

**Fix**: add a `mergeReasonDetachedHead` member to the closed reason set and a
`detachedHeadReason(f)` guard evaluating **both** sides unconditionally (side-free, aggregated,
same shape as `pairDirtyReason`), wired into both `MergeIn` and `Merge`. Needs a
`gitrepo.Repo.HeadDetached()` probe, since `CurrentBranch()` collapses detachment into an error.

### F3 — MEDIUM — CONFIRMED — `MergeResult.Committed` / `AlreadyUpToDate` do not describe what happened

`internal/fabricengine/merge.go:205`, `merge.go:400` (`return MergeResult{Committed: true, …}`),
`internal/fabricengine/mergelifecycle.go:146` (`MergeContinue`'s own `Committed: true`).

Both verbs hardcode `Committed: true` on the both-sides-clean path regardless of whether
`concludeMergeSides` actually landed anything, and `AlreadyUpToDate` is decided by a probe taken
**before** the write lock is acquired.

**Scenarios** (both driven):
- L1: a merge that fast-forwarded both sides reports `committed:true` with no `merge_committed`
  entry anywhere in the record — no commit was created.
- L10: the loser of two concurrent `merge-in` calls reports
  `{"already_up_to_date":false,"committed":true,"mutations":[]}` for a call that did nothing at
  all, where the strictly sequential control of the identical command pair honestly reports
  `{"already_up_to_date":true,"committed":false}`. Reproduced on two independent hubs.

**Fix**: derive both fields from the post-lock reality — `AlreadyUpToDate` when both `MergeStart`
outcomes are `MergeAlreadyUpToDate`, `Committed` only when a conclude commit actually landed on at
least one side.

### F4 — MEDIUM — CONFIRMED — an operator's `merge.ff = only` breaks every non-fast-forward fabric merge

`internal/gitrepo/merge.go:57` — `r.runChecked("merge", "--no-commit", ref)`.

**Scenario** (driven, L9): with `merge.ff = only` in the git config, `git merge --no-commit <sha>`
exits 128 with `fatal: Not possible to fast-forward, aborting.`. `MergeStart` finds no conflicted
files, so it classifies a genuine error, `selfAbortMergeAttempt` resets both sides, and the verb
fails with a raw git-hint blob in its message. Every merge into a target that has moved is
unusable until the operator finds and changes the config.

**Fix**: pass `--ff` explicitly in the non-squash form, exactly the same posture `MergeConclude`
already takes with `--no-edit` — pin the behaviour fabric depends on rather than inheriting the
operator's config. (`--squash` is already immune; `MergeFFOnly` intentionally wants `--ff-only`.)

### F5 — LOW — CONFIRMED — `Remove` can delete the pair that is a live merge's source

`internal/fabricengine/remove.go:65` — the guard asks only whether *the pair being removed* has a
record.

**Scenario** (driven, L2): with the prime pair mid-merge on `merge-in task1`,
`lyx fabric remove task1` succeeded, removing both `task1` worktrees and deleting branch
`task1-weft`. The in-flight merge still concluded correctly afterwards (verified), so harm is
bounded — but if the operator aborts instead, the weft-side source work survives only on
`origin/task1-weft`, and the operator was given no warning that the branch they deleted was the
subject of a merge in progress.

**Fix**: before deleting, check whether any pair in the hub holds a merge record whose `Source`
names this slug, and refuse with `ErrMergeInProgress` if so.

### F6 — LOW — CONFIRMED — `doc.go` overstates the guarded sibling set

`internal/fabricengine/doc.go:884`: *"Every other mutating fabric verb refuses with
`*ErrMergeInProgress` while a merge record exists"*. Only `Commit`, `Pull`, `Topology.Checkout`
and `Topology.Remove` do; 13 further mutating entry points are adjudicated safe and 4 are
deliberately unguarded. A reader takes the sentence as an invariant it is not.

**Fix**: name the exact guarded set and say in one clause why the rest need no guard.

### F7 — NIT — CONFIRMED — `MergeContinue`/`MergeAbort` break the "`Conflicts` is empty, never nil" contract

`internal/fabricengine/mergelifecycle.go:146` and `:189` return `MergeResult{Committed: true}` /
`MergeResult{}`, leaving `Conflicts` nil. `merge.go:23` declares `mergeNoConflicts` — *"the
empty-never-nil `Conflicts` value every `MergeResult` that carries no conflicts returns, so a
caller's JSON never sees a `null` conflicts field"* — and `MergeResult`'s own godoc repeats it.
A Go consumer marshalling a `MergeContinue` result gets `"conflicts": null`; the CLI happens to
hide it because it only emits the key when `len(res.Conflicts) > 0`.

**Fix**: return `mergeNoConflicts` on both success paths.

### F8 — NIT — CONFIRMED — `merge --abort -m <msg>` silently accepts and ignores `-m`

`internal/fabriccli/merge_verbs.go:118` rejects `--squash` alongside `--continue`/`--abort` but
says nothing about `-m`. `-m` is meaningful for `--continue` (it is `MergeContinue`'s message
override) and meaningless for `--abort`, which discards `message` without a word.

**Fix**: extend the pre-flight to reject `-m` with `--abort`, matching `--squash`'s treatment.

### F9 — NIT — docs — a conflicted merge is not distinguishable from a hard error by exit status

`internal/fabricengine/doc.go` "# The merge surface" and
`internal/fabriccli/merge_verbs.go`'s `Long` text. Both a conflicted `merge-in` and a genuine
failure exit 1 with `ok:false`; only the presence of the `conflicts` key separates them, and
nothing says so. A script author will get this wrong.

**Fix**: state the discrimination rule (`ok:false` + a `conflicts` array = a conflict result, not
a failure) in `doc.go`'s conflict-reporting paragraph and in the `merge-in` `Long` text.

## Docs & operability

- `doc.go`'s crash-recovery claim — *"there is no window where the record and the checkouts can
  drift silently out of reach of the quartet"* — is true only because `MergeAbort` still works;
  `MergeContinue` demonstrably *can* drift out of reach (**F1**). The paragraph needs the
  qualification once F1's guard exists.
- `doc.go` says nothing about the checkouts needing to be on a branch (**F2**).
- `MergeResult.Committed`'s meaning appears in the struct godoc only and is contradicted in
  practice (**F3**).
- The guarded-sibling sentence is an overstatement (**F6**).
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` carries no merge scenarios at all; the detached-HEAD and
  crash-resume behaviours F1/F2 cover are exactly what a live suite should exercise.

## What could NOT be verified, and why

- **Windows path behaviour** in `weftPathVisible`/`unifyConflictPaths` (backslash separators,
  case-insensitive prefix matching). Explicitly out of scope; this host is Linux. Not fixed blind.
- **A sibling verb slipping through the unlocked guard window.** Reasoned about; never reproduced
  across repeated attempts, so per the campaign rule it is not recorded as a finding.
