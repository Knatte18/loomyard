MILL_REVIEW_BEGIN
# Review: gitrepo: generic, repo-agnostic git primitives

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-24
```

## Findings

### [GAP] Rebase-retry assumes a clean worktree gitrepo no longer guarantees
**Section:** Decisions › Push surface / StageAndCommit
**Issue:** Board's rebase-retry works because `commitDirty` `add -A`s first, leaving a clean tree before `pull --rebase`; gitrepo stages only explicit files and "leaves an unlisted dirty file uncommitted", so a modified *tracked* file makes the retry's `pull --rebase` abort ("cannot pull with rebase: unstaged changes") and `Push`/`PushCoalesced` fails instead of recovering.
**Fix:** State the precondition (caller must have a clean tree, or dirty tracked files are the caller's responsibility) or specify a stash/guard, so "fully replace board's sync.go" holds without `add -A`.

### [GAP] Two-clone fixture cannot exercise the single-pusher lock blocking
**Section:** Testing › PushCoalesced (push-only)
**Issue:** The single-pusher lock is an `internal/lock` flock on a file in the worktree root, so it only serialises processes sharing one worktree; the bullet says use "two clones sharing a bare remote" to test that "a second concurrent PushCoalesced blocks on the single-pusher lock", but two clones have two worktree roots → two lock files → no blocking (that path only tests rebase-retry).
**Fix:** Split the scenarios — lock-blocking/coalescing = two processes on one clone; cross-clone conflict/rebase-retry = two clones — and name each fixture accordingly.

### [GAP] SnapshotSHA read-fetch failure behaviour unspecified
**Section:** Decisions › Snapshot remote sync / No remote
**Issue:** `SnapshotSHA` "fetches the snapshot refs before reading", but the No-remote decision enumerates only the write paths (`Push`, `PushCoalesced`, `SetSnapshotSHA`); it is unstated whether a transient fetch failure (offline, remote configured) makes every snapshot read error, or falls back to the local ref.
**Fix:** Decide and record: does a failed pre-read fetch error out, or does `SnapshotSHA` degrade to the last-known local ref value.

### [NOTE] Push-coalescing lock file name is not pinned
**Section:** Decisions › Lock ownership
**Issue:** The lock lives in the worktree root and gitrepo does not gitignore it, so its name is user-visible in the consumer's `git status`, yet no name is specified for a generic library (board's is the domain-specific `tasks.json.push.lock`).
**Fix:** Pin a repo-agnostic lock filename constant so consumers know what to gitignore.

### [NOTE] "Board auto-gitignore dropped as redundant" overstates
**Section:** Decisions › Lock ownership
**Issue:** Board's `ensureLockfilesIgnored` ignores `*.lock`, `*.swaplock`, and the render-manifest sidecar — only the lock-file part becomes redundant under explicit-only staging; board still needs to ignore the render manifest / swaplock.
**Fix:** Scope the claim to the push-lock file; note the board rewrite keeps its own gitignore for the non-lock sidecars.

## Verdict

GAPS_FOUND
Three resolvable gaps around rebase-on-dirty-tree, the coalescing-lock test fixture, and snapshot read-fetch failure.
MILL_REVIEW_END