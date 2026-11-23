# Why pushes of the weft repository's `main` race on one machine

Read from code, the hub's traces and the board checkout's git history, on 2026-11-23 for the window 09:15 to 09:25 (+01:00).
No run was timed and no trial was made.
Paths below are relative to the code repository unless they start with `_bolt/`, `loomyard/` or `loomyard-weft/`, which are hub directories.

## Cause

Weft `main` is the board branch.
Its only checkout is the Bolt worktree `_bolt` (a linked worktree of `loomyard-weft`), so every push of it starts in `fabricengine.Bolt`.
Three different locks guard the Bolt's pushes, and the first push attempt of most paths takes none:

- `lym board sync` holds `board.push.lock` (`fabricengine.BoardPushLockFile`) through `Bolt.Sync` and `coalescePush`.
- `pullFirst` and `retryBoltPush` hold `lym-push.lock` in the Bolt's git directory (`gitrepo.Repo.PushLockPath`).
- `Bolt.PushRecorded`, which every synchronous pusher calls, takes no lock for its first attempt:
  `pushClassified` calls `gitrepo.Repo.PushRebaseFree`, which documents that it takes no push-serialising lock.

So the absorbing lock only serialises `lym board sync` processes against each other.
A board write that pushes itself (the owner claim a `batten` run makes, a status write, a config commit) pushes in parallel with a detached sync and with other claims.
Two parallel pushes of one ref make GitHub refuse the loser with `cannot lock ref ... is at X but expected Y`.
The loser's retry then runs the identical push again, and it loses again while the other pushers keep moving the ref.
After 4 attempts it gives up and reports `origin is unreachable`, though the commit it wanted to push was already on origin.

## The incident

The refused push is the owner claim of `godocreflow-worsens-findings`, run by `lym batten step` (pid 3432585, trace `d61eff96`) from `loomyard/`:

| Time | Event |
|---|---|
| 09:22:27.995 | The claim commits `d359b72` on `_bolt` `main`, whose parent is `550dd1f`. |
| 09:22:39.9 | Attempt 1 refused: origin is at `678ac33`, expected `550dd1f`. |
| 09:22:49.9 | Attempt 2 refused: origin is at `a31061a`, expected `678ac33`. |
| 09:22:54.99 | Attempt 3 refused: origin is at `f4a24e2`, expected `a31061a`. |
| 09:23:09.97 | Attempt 4 refused: origin is at `9f73b6e`, expected `f4a24e2`. The push gives up, and the step is stuck in `Worktree-Create`. |

All five SHAs are in `_bolt`'s local history, in this order: `550dd1f`, `d359b72`, `678ac33`, `a31061a`, `f4a24e2`, `9f73b6e`.
`d359b72` is an ancestor of `678ac33`, so the claim was on origin from the first refusal on: another process's push had carried it.
The refusal was spurious.

The checkout made eight commits in the minute from 09:22:00 (two owner writes and six `board sync` commits), so several processes were committing and pushing the one ref.
Trace `eeb880f5` (a detached `lym board sync --deferrable`, pid 3310157) logged the same refusal at 09:22:34 with the same expected value `550dd1f` and the same actual value `678ac33`.
Two pushers started from `550dd1f` and a third landed `678ac33` first.
Other syncs refused on `ref=main` at 09:20:27, 09:21:31, 09:21:44, 09:24:11 and 09:25:14.

Each attempt is slow.
The warnings are 10.0 s, 5.1 s and 15.0 s apart while the backoff sleeps are 1, 2 and 4 s plus up to half as jitter, so a push takes about 2 to 11 s.
That is the window in which another push can move the ref, and a push that lasts seconds loses it often.

## Push sites of weft `main`

Every site reaches `pushClassified` with the Bolt side, through `Bolt.PushRecorded` or `Bolt.Push`.
The lock column is what the site holds when the first push attempt runs.

| Call site | Caller | Lock at the first attempt | Checkout |
|---|---|---|---|
| `boardengine.Sync` (`internal/boardengine/sync.go`) | detached `lym board sync` that every board write, `tendercli` and `hubreconcile` spawn | `board.push.lock` | `_bolt` |
| `pushOwnerCommit` (`internal/boardengine/ownerset.go`) | `SetOwners`, `commitBoardWriteAndPush`, `Board.SetStatusPushed`; the `batten` claim and the landing's done mark | none | `_bolt` |
| `newHubCommit` and the reconcile apply (`internal/configcli/hubwide.go`) | `lym config` edit and reconcile | none | `_bolt` |
| `internal/fabriccli/shortname.go`, `internal/fabriccli/fabric.go` | shortname and hub-config verbs | none | `_bolt` |
| `internal/fabricengine/clone.go` | `lym fabric clone` seed | none | `_bolt` |
| `retryBoltPush` (`internal/fabricengine/pushclassified.go`) | any of the above after a plain rejection | `lym-push.lock`, taken after a try on the board write lock | `_bolt` |

No other site pushes weft `main`.
The records worktrees push their own `<slug>-weft` and `main-weft` branches under `fabric.push.lock` of each worktree's `.weft/` directory, and the code repository is another remote.
`pullFirst` (`PullThenCommitWritten`, `Pull`) takes `lym-push.lock` but does not push.

The lock order today is the board write lock, then `lym-push.lock` in `pullFirst`, and `board.push.lock`, then the board write lock in `Sync`'s `commitDirty`.
The two chains use different push locks, so they cannot deadlock now.
Unifying the locks without changing the order would create that deadlock.

## Questions

### Which paths ran between 09:15 and 09:25

- The `batten` claim above (`pushOwnerCommit`, no lock).
- Detached `lym board sync --deferrable` processes (`Sync`, `board.push.lock`), spawned by board writes such as `batten run pr-findings-open-fabric-nil` at 09:20:04.
- Records pushes of task `-weft` branches (`lym fabric push`, traces `357b4590`, `75db8bd0`, `f54c2434`, `24559e33`), each under its own worktree's `fabric.push.lock`.
- The records push of the prime `loomyard-weft` (`main-weft`) by `batten run`, under `loomyard-weft/.weft/fabric.push.lock`.

### Whether another machine wrote the board

No evidence of one.
Every origin tip a refusal names is a commit in this checkout's history.
The checkout's `main` reflog for the window holds only `commit` entries (67), with no pull, rebase or reset, so nothing foreign was merged in.
Every owner commit in the window names `neve`.
A foreign push the checkout had not fetched would show in a refusal as an `is at` SHA it lacks, and none does.

### The start refused with `push lock busy`

This is a different lock and a different ref.
It is `loomyard-weft/.weft/fabric.push.lock`, the lock of the prime's records worktree (`main-weft`), and `StatusPushLockWait` (30 s) bounds the wait.
`batten run pr-findings-open-fabric-nil` logged it at 09:20:35, held by pid 3180975 for 15 s.
Dobby's catch-up for `amend-plan-loses-webster-batches` hit it again, held by pid 3180973 for 1 s.
Each run's catch-up and each status transition pushes the prime's records, so these pushes queue on one lock whose holder spends seconds per GitHub push.
`pushOwedForCatchUp` (`internal/battencli/catchup.go`) fails the catch-up on the first expired wait, while the landing push rides the same error out (`pushRidingOutBusyLock`, 30 s, 1 min and 2 min pauses).

### Why the retry gives up

`gitrepo.PushWithRetry` reruns the identical `git push <url> refs/heads/main:refs/heads/main` after 1, 2 and 4 s (each plus up to half as jitter), four attempts in all.
It treats a bare `(failed)` and a `cannot lock ref` refusal as retryable and nothing else.
Each run re-resolves the local `main`, so it pushes whatever tip the checkout has by then, and git advertises origin's current value, so no stale commit is pushed.
It neither fetches nor rebases, because neither is needed: the local branch already contains origin's tip.
It also never asks whether its own commit already landed.
After the fourth refusal it returns `ErrPushRetriesExhausted`.
`classifyFailure` records that as the `transport` class, and `pushOwnerCommit` maps it to `ErrOwnerOriginUnreachable`, which reads `origin is unreachable`.
`battencli` then fails the claim and the step is stuck.

## Fix for each path

Candidate note: one lock for every push of weft `main` on a machine, and a landed check after a refused push.

1. **One lock inside the Bolt's push.**
   `Bolt.PushRecorded` takes `board.push.lock` (with the pause gate `coalescePush` uses) around the whole push, its retries included.
   Every site in the table above then pushes under it, with no edit to the callers.
   - `Bolt.Sync` stops holding `board.push.lock` across its loop: the step commits under the board write lock first, then pushes through the locking `PushRecorded`.
     Concurrent syncs still absorb each other: the second finds nothing unpushed once the first has moved the tracking ref.
   - `pullFirst` takes `board.push.lock` in place of `lym-push.lock`, so one lock guards the Bolt.
     `retryBoltPush` runs inside `PushRecorded`, which already holds that lock, so it takes none.
     The order is the board write lock, then `board.push.lock`, on every path.
   - A test starts a claim and a sync at once against a fake remote that refuses a second concurrent update, and expects both to land with one push each.
2. **A refusal is no failure when the commit is already on origin.**
   After `ErrPushRetriesExhausted` or any rejection, the Bolt side fetches, and when `HasUnpushed` is then false it records the push as landed.
   This alone would have let the claim at 09:22 succeed, and it covers a push from a second machine.
3. **Name the exhausted refusal for what it is.**
   `classifyFailure` maps `ErrPushRetriesExhausted` to `transport`, and the owner write turns that into `origin is unreachable`.
   Record it under its own class, with the refusal line, so the way forward does not send the operator looking at the network.
4. **Records push lock wait (a separate path).**
   `pushOwedForCatchUp` rides out `ErrPushLockBusy` the way the landing push does, instead of failing the catch-up after 30 s.
   Making that lock machine-wide (one lock for the board and every records push to `loomyard-weft`) is a larger change that this report does not size.
   The traces show `(failed)` refusals on three `-weft` branches within two seconds at 09:21:21 to 09:21:23, each landing on the second attempt, so GitHub may refuse concurrent updates of one repository across refs.
   That reading is not established, and fix 1 does not depend on it.
