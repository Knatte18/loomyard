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
