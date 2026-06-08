# Design: asynchronous git sync

**Status:** Accepted — implementing (2026-06-08).

## Problem

Today every wiki write runs the full git round-trip synchronously inside
`writeOp`: `pull → mutate → render → save → commit → push`. The process does not
return until the push to GitHub completes, so a single write takes **~4.4 s**
(see [benchmarks.md](benchmarks.md)). A burst of small updates (e.g. a thread
making many `set-phase` status changes) serialises into many multi-second waits.

GitHub backup is a hard requirement, so push **must** happen. The question is
only whether the calling process has to *wait* for it.

## Key insight

All `mhgo` processes on one machine share the **same `tasks.json`**. A reader
loads that file; a writer swaps it atomically under the swap lock. So one process
sees another's change **immediately through the file** — no git involved. Git
push/pull matters **only** for replicating to another machine / GitHub backup.

Therefore remote sync is inherently a background concern, and the write path can
drop git entirely. The filesystem *is* the request queue: by writing `tasks.json`,
a change has already "asked" to be synced; a background pusher drains it.

## Goals / non-goals

**Goals**
- A wiki write returns in ~10 ms (file I/O only), never waiting on the network.
- Every change is pushed to GitHub, automatically, within seconds.
- A burst of N writes collapses into ~1 push, not N.
- No new always-on daemon and no IPC beyond the filesystem + file locks.

**Non-goals**
- Synchronous "confirmed on GitHub before the command returns" (that is the
  status quo; this design trades it for "on GitHub within seconds").
- Cross-machine real-time consistency (each machine pulls on its own sync).

## Design overview

Split the two halves that `writeOp` currently fuses:

1. **Write path (synchronous, fast):** `lock → load → mutate → render → save`.
   No git. Returns immediately. The save leaves `tasks.json` (and the rendered
   files) dirty in the git working tree — that *is* the "needs sync" signal.
2. **Pusher (asynchronous, slow):** a detached background process does
   `git add -A → commit → pull --rebase → push`. Spawned by the write, but the
   write does **not** wait for it.

```
mhgo wiki upsert ...                 (one-shot process)
  lock → load → mutate → render → save        ~10 ms
  spawn detached `mhgo wiki sync`              (does not wait)
  exit                                         ← command returns here

mhgo wiki sync                       (detached background process)
  acquire push-lock (single pusher)
  loop until git is clean & fully pushed:
    [under write-lock]  git add -A ; git commit -m "wiki sync"
    [lock-free]         git pull --rebase ; git push        ~4 s
  release push-lock ; exit
```

## Locking model

Three independent locks, each held only as long as needed:

| Lock | Held by | Duration | Purpose |
|------|---------|----------|---------|
| `tasks.json.swaplock` | reader (shared) / writer + pusher commit (exclusive) | microseconds | fence reads against the atomic rename (existing) |
| `tasks.json.lock` (write-lock) | writer during mutate; pusher during `add`+`commit` | ~10–300 ms | serialise file-state changes and snapshot for commit |
| `tasks.json.push.lock` | the pusher, whole loop | seconds | guarantee a **single** active pusher |

The crucial change: the write-lock is now held only for the **file mutation** and
for the pusher's **commit** — never across the network push. The pusher takes the
write-lock briefly to `git add`+`commit` a consistent snapshot, releases it, then
does `pull --rebase`+`push` lock-free. Writers are therefore blocked at most for a
commit, not for a ~4 s push.

## Coalescing & the wakeup guarantee

We want a burst of writes to collapse into ~1 push, while guaranteeing no change
is ever left unpushed.

- **Single pusher:** the `push.lock` ensures only one pusher does network work at
  a time. The "dirty" signal is git's own state — uncommitted changes
  (`git status --porcelain`) or unpushed commits (`git rev-list @{u}..HEAD`).
- **Coalescing:** because `git push` sends *all* commits ahead of the remote, one
  push covers many commits. The pusher loops: commit + pull + push, then
  re-checks git state; if new changes arrived during the push it loops again,
  otherwise it exits. So 10 rapid writes → ~1–2 actual pushes.
- **Wakeup guarantee:** every write spawns its *own* pusher. Even if all currently
  running/queued pushers miss a change, that write's own pusher will eventually
  acquire `push.lock`, observe the dirty git state, and push it. Pushers that find
  nothing to do exit instantly. So the worst case for a burst is a handful of
  short-lived pusher processes, only one of which does network work.

Trade-off: a burst spawns several pusher processes (each ~30 ms to start) that
mostly no-op. Bursts are small in practice, and the work is off the hot path. If
this ever matters, a "pusher-alive" flag can suppress redundant spawns — deferred.

## Failure & recovery

- **Offline:** the detached push fails, but the commit stays in the local repo.
  `git push` is cumulative, so the *next* successful push (from any later write)
  sends all pending commits at once. The system is **self-healing** as long as
  writes keep happening while online.
- **Residual gap:** the only change that can stay un-pushed is the *last* write
  before the machine goes offline/idle forever. It is safe on local disk, just not
  yet on GitHub; the next write while online heals it. We accept this narrow edge
  — there is no periodic safety-net sync.
- **Pusher crash:** leaves dirty git state; the next write spawns a fresh pusher
  that picks it up. Locks are OS file locks (gofrs/flock), released automatically
  if the process dies.
- **Cross-machine:** the pusher's `pull --rebase` before push folds in another
  machine's commits; a rebase conflict aborts and the push retries next loop
  (existing `CommitPush` behaviour).

## Commit granularity

The pusher commits whatever is on disk, so a burst of writes becomes one
`"wiki sync"` commit rather than one commit per change. For a backup of a task
tracker this is fine, and it keeps writes at ~10 ms.

If per-change history is wanted instead, the **write** can commit locally (under
the write-lock) and the pusher only pulls+pushes. That preserves `"wiki: <slug>"`
messages at the cost of ~300 ms per write (git subprocess spawns). Default:
batched commits by the pusher; revisit if granular history is needed.

## Test impact

Writes become pure file operations, so unit/concurrency tests keep using
`WIKI_SKIP_GIT=1` to mean "do not spawn the pusher" — they already run git-free
and fast. The pusher is exercised by the integration suite against the dummy wiki.

## Decisions

1. **Pusher entrypoint:** an `mhgo wiki sync` subcommand that runs the loop to
   completion (reuses the binary via `os.Executable()`). It is an ordinary
   foreground command — also runnable by hand to force a sync. The *write path*
   launches it **detached** (Windows `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`,
   no inherited stdio, no `Wait`) so the write returns immediately.
2. **Granularity:** batched pusher commits (one `wiki sync` commit per drained
   burst). Per-write local commits are not used.
3. **No periodic safety-net** — option 1 is self-healing; we accept the narrow
   last-write-then-offline edge.
