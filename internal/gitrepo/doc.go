// Package gitrepo provides a typed Repo over a single local git checkout.
// It exposes the semantic operations a consumer of a git-backed repo needs,
// so no consumer parses raw git stdout itself.
// It is not a general-purpose git wrapper:
// rebase as a caller operation, interactive staging, conflict resolution and branch management are not supported.
// gitrepo reports conflicts; resolving them is the caller's job.
//
// # Two backends
//
// Reads of local objects and refs go through go-git (gogit.go) and spawn no git.
// Anything that authenticates to a remote or mutates the working tree runs the git CLI through internal/gitexec.
// PATTERN-gitrepo-client-boundary states the enforced split and the review obligation of a new CLI call.
// gitexec stays a low-level package that imports no gitrepo or feature package.
//
// The CLI-bound methods stay so because go-git:
//
//   - performs no CRLF conversion, so a file it commits is permanently "modified" to CLI git under core.autocrlf=true;
//   - deletes untracked and gitignored files on a hard reset, which would destroy .lym/, .scratch/ and build output;
//   - moves HEAD before its dirty check on Checkout and Pull, so a rejection leaves a half-applied state;
//   - never invokes a git credential helper and ships no rebase.
//
// goGit never calls git.PlainOpen or sets DetectDotGit, and keeps KeepDescriptors false:
//
//   - PlainOpen on a linked worktree returns no error but reports every ref in the shared common dir as absent.
//   - DetectDotGit walks up to an ancestor and can silently open the wrong repository.
//   - KeepDescriptors true locks packfiles and blocks `git worktree remove` on Windows.
//   - extensions.worktreeConfig makes go-git refuse to open; that typed error is wrapped with %w, never swallowed.
//
// An object that exists only in a packfile written after the handle indexed reads as object-not-found.
// readGoGit reindexes and reruns the read once, because the triggers of a proactive reindex cannot be enumerated.
//
// Some outcome classification matches git's untranslated (C/English locale) message text.
// Examples are Push's rebase-retry triggers and the "no rebase in progress" abort check.
// Under a translated locale those matches fail loudly, as a hard push error, never as a silent wrong success.
//
// # The Repo API
//
// New(path) wraps an existing checkout with no validation and no I/O, so it cannot fail.
// It does not create, clone, or manage repo topology.
//
//   - Fetch refreshes remote-tracking refs without merging, so it never moves the local branch.
//     It prunes the tracking ref of every branch the remote deleted, and never a local tag.
//   - HoldRemoteRefs takes the remote-refs lock, a file in the common git dir that every worktree shares.
//     Fetch, FetchWithArgs, Pull and DropRemoteTracking run under it and wait while it is held.
//     Push methods transfer outside the lock and take it only to write the tracking ref or upstream afterwards.
//     A caller that reads origin and then judges the tracking refs holds the lock across both.
//     It passes the process's pause gate before taking the lock, so a pause never holds a lock others wait on.
//   - UpdateRemoteBranchLeased and DeleteRemoteBranchLeased act only while the remote branch sits at the expected SHA;
//     a moved branch is ErrLeaseRejected, distinct from any other push failure.
//   - RefNamespace maps local branch and tag names to the ref names they take on the remote.
//     Its zero value is the identity namespace; gitrepo holds no other layout and follows the remote's refspecs.
//   - A push whose recovery rebase failed and was aborted satisfies errors.Is(err, ErrPullRebaseFailed).
//
// Caller-supplied SHA arguments are validated as plain hex object names before they reach git.
// An option-shaped value such as "--hard" is never parsed as a git flag.
// An invalid SHA surfaces as ErrInvalidSHA, or as false from SHAExists.
//
// # Commits and the never-sweep norm
//
// An automated commit must never absorb content nobody asked it to commit.
// StageAndCommit takes an explicit file list and never wildcard-stages.
// It enforces the norm structurally: its pathspec cannot commit an unlisted path, whatever races it.
// StageAllAndCommit is the opt-in wildcard exception.
// CommitEmpty(msg) records a commit whose only content is msg, for a caller that wants a commit regardless.
// It has no pathspec, so it refuses instead: a dirty index returns ErrIndexNotEmpty without committing.
// That guarantee is narrower: an index write landing between the pre-check and the commit is swept in.
// Serializing writes against that window is the caller's discipline.
// CommitPathAllowEmpty(msg, relPath) scopes the commit to one path and leaves other staged paths staged.
// It never returns ErrIndexNotEmpty.
//
// # SHAExists
//
// A human always has plain git in the working tree,
// so a rebase, amend or force-push can invalidate a stored SHA under any consumer.
// gitrepo is not rebase-aware; SHAExists is a cheap existence check.
// Code that reads a stored SHA calls it first and treats a missing SHA as staleness: it rebuilds or re-syncs.
//
// # Push
//
// Push and PushCoalesced are push-only: committing is the caller's separate, prior step, and push never stages anything.
// Every push names its branch explicitly.
// The checked-out branch goes to the same-named branch of its push remote, its upstream remote or origin.
// A branch with no upstream takes that remote branch as its upstream once the push succeeds.
// Push recovers from one non-fast-forward rejection with a `pull --rebase` and a retry.
// That recovery requires a worktree clean of tracked-file changes.
// A push that recovered rewrote the local commits it replayed,
// so a SHA captured before it, such as StageAndCommit's return value, may name an off-history commit.
// Callers re-read CurrentSHA after a successful push before recording a SHA anywhere.
//
// Push, PushCoalesced and PushRebaseFree also retry a push the remote refuses on the pushed branch.
// The retry uses no rebase or force and runs inside whatever lock the caller holds.
// A push that exhausts its attempts returns an error wrapping ErrPushRetriesExhausted and the last push error.
// It never returns ErrPushRejected.
// A rejection's error carries git's rejection line and the push's `remote:` lines,
// so a hook's own refusal text reaches the caller.
// PushCoalesced takes a single-pusher lock file in the checkout's own git dir before it checks for unpushed work,
// so a burst of concurrent callers collapses into as few pushes as possible.
//
// # Pull and reset
//
// Pull is fast-forward-only by contract.
// A diverged local branch is an error and never a merge commit; recovering from divergence is caller policy.
// A non-zero exit names the repo path and git's exit code without git's raw "fatal:" text.
//
// ResetHard points HEAD, the index and the working tree at a caller-supplied SHA.
// It discards local commits and uncommitted changes past it.
// ResetKeep suits a checkout holding uncommitted work.
// It changes nothing and returns git's refusal when an uncommitted change sits in a path the move would rewrite.
package gitrepo
