// Package gitrepo provides a typed Repo over a single local git checkout.
// It splits across two backends: go-git for local object and ref reads;
// internal/gitexec's command runner serves anything that authenticates to a remote or mutates the working tree.
// It exposes the semantic operations a consumer of a git-backed repo needs,
// so no consumer parses raw git stdout itself.
//
// # Relationship to internal/gitexec — the two-backend boundary
//
// internal/gitexec exposes RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error).
// RunGit is the raw form: it shells out to git and returns its exit code for the caller to classify.
// Run(args []string, cwd string) (string, error) is the checked form.
// It returns a non-zero exit as *GitError.
// gitrepo wraps them as the unexported run and runChecked.
// The read methods resolve state through go-git's own object and ref access (see gogit.go) and use neither.
// Pull and Fetch sit on the raw run, because a non-zero exit is a failure there too.
// The raw form lets them keep git's `fatal:` text out of their messages, a rule a test enforces.
// GitDir and CommonDir are the one other local read:
// they resolve the checkout's `.git` entry through internal/dotgit once per handle, spawning nothing.
// The worktree root is not among them; that query belongs to internal/lymcwd alone.
// Every other CLI-bound method sits on runChecked.
// BlameLines stays CLI-bound:
// go-git's blame walks the whole history of the file per call and has no first-parent mode.
// HasUnpushed stays CLI-bound.
// PATTERN-gitrepo-client-boundary states the enforced, exhaustive version of this split.
// It also states the review obligation that a new CLI call inside this package carries.
// gitexec stays a low-level package that imports no gitrepo or feature package.
// gitrepo is one of its consumers, not merged into it.
//
// Some outcome classification matches git's untranslated (C/English locale) message text.
// CurrentSHA detects an unborn HEAD by type: plumbing.ErrReferenceNotFound surfaces as ErrNoCommits.
// That detection does not depend on the locale.
// Push's rebase-retry triggers and the "no rebase in progress" abort check match git's untranslated stderr.
// Under a non-English locale with translation catalogs installed, those matches fail.
// Every affected path then degrades loudly: a hard push error instead of a recovery, never a silent wrong success.
// Pinning the subprocess locale is a repo-wide gitexec decision, not this package's.
//
// # The Repo API
//
// New(path) wraps an existing checkout with no validation and no I/O, so it cannot fail.
// It does not create, clone, or otherwise manage repo topology; fabric does that, directly on gitexec.
// From there:
//
//   - CurrentSHA, StageAndCommit, CommitEmpty, CommitPathAllowEmpty, ChangedFilesSince and SHAExists are the core primitives.
//   - Push and PushCoalesced are the push surface (see below).
//   - Pull is the fast-forward-only pull surface (see below).
//     Fetch refreshes remote-tracking refs without merging, so it never moves the local branch.
//   - HoldRemoteRefs takes the repository's remote-refs lock, a file in the common git dir that every worktree shares.
//     Fetch, FetchWithArgs, Pull and DropRemoteTracking run git under it and wait while it is held.
//     DropRemoteTracking deletes a branch's remote-tracking ref and upstream setting.
//     A fetch and a push both write the shared remote-tracking refs;
//     two concurrent writers of one ref make the loser fail.
//     Every push method sends to the remote's push URL outside the lock.
//     Git therefore writes no tracking ref during the transfer.
//     After a successful transfer it takes the lock to move or delete the branch's tracking ref.
//     For a first push of the checked-out branch it also sets the upstream under the lock.
//     A caller that reads origin and then judges the tracking refs holds the lock across both.
//     No tracking-ref write then lands between the read and the verdict.
//     HoldRemoteRefs passes the process's pause gate before it takes the lock and stays admitted until the release,
//     so a pause never holds a lock holder that other processes wait on.
//   - IsAncestor reports whether a SHA is an ancestor of a ref via `git merge-base --is-ancestor`.
//     It maps git's tri-state exit code directly instead of folding every non-zero exit into failure.
//     CommitsNotIn lists the commits reachable from a tip and not from a base, newest first, via `git rev-list`.
//     MergeBase returns two commits' best common ancestor, read through go-git.
//   - UpdateRemoteBranchLeased moves a remote branch to a SHA, backwards included.
//     It does so only while the remote branch still sits at the SHA the caller read.
//     DeleteRemoteBranchLeased deletes it under the same lease.
//     A moved branch is ErrLeaseRejected in both, distinct from any other push failure.
//   - ResetHard is the SHA-validated hard-reset surface (see below).
//   - ResetKeep moves HEAD and the index to a SHA and carries uncommitted changes across.
//     It refuses, changing nothing, when an uncommitted change sits in a path the move would rewrite.
//   - CurrentBranch reads the branch HEAD points to, and it errors on a detached HEAD.
//   - UpstreamSHA resolves the current branch's configured upstream to its remote-tracking SHA as last fetched.
//     It returns ErrNoUpstream when there is none.
//   - CommitDetail returns one commit's full message and the paths its tree changes against its first parent.
//     It reads through go-git.
//   - TagRefs lists the tags under a name prefix, each peeled to its commit and sorted by the commit's date.
//     BranchRefs lists every local branch resolved to its tip commit, sorted by name.
//     It errors naming a branch whose tip is no commit.
//     Log walks the commits reachable from an arbitrary tip, first-parent or over every parent.
//     A date, a path prefix and a limit bound the walk.
//     SubdirsAtRevision lists the directories directly in one directory as of a revision.
//     TagRefs, BranchRefs, Log and SubdirsAtRevision are go-git local reads.
//     PreImageLines lists, per path, the lines of a commit's first parent that the commit removes or replaces.
//     It also reads through go-git.
//     BlameLines names the first-parent commit that brought each requested line of a file, via `git blame`.
//   - TrackedRootEntries returns the sorted, deduplicated first path segment of every index entry.
//     A root entry staged and never committed therefore counts as tracked.
//     It reads the index through go-git.
//   - CherryPick replays one commit onto HEAD via a plain `git cherry-pick`.
//     It skips a commit whose change the branch already carries and refuses a commit that is empty from the start.
//     CherryPickAbort abandons one in progress, and it succeeds when none is.
//   - A push whose recovery rebase failed and was aborted satisfies errors.Is(err, ErrPullRebaseFailed).
//     That tells a replayable conflict from every other push failure.
//
// Caller-supplied SHA arguments are validated as plain hex object names before they reach git.
// An option-shaped string, such as a value with a leading '-' like "--hard", is never parsed as a git flag.
// An invalid SHA surfaces as ErrInvalidSHA, or as false from SHAExists, which swallows errors into its bool.
//
// # CommitEmpty — the empty-commit primitive
//
// CommitEmpty(msg) records a commit whose only content is msg.
// Its tree is identical to its parent's, so it carries no file changes.
// StageAndCommit cannot produce that shape.
// It returns its no-op signal on an empty file list,
// and its `diff --cached --quiet` gate returns the same no-op when the listed files' staged content matches HEAD.
// An empty commit has no content to key either check on.
// CommitEmpty serves a caller that wants a commit to land regardless, with the meaning carried in msg.
//
// CommitEmpty instead pre-checks the index itself, branching on whether HEAD is born or unborn.
// On a born HEAD, an unscoped `git diff --cached --quiet` reports through its exit code.
// On an unborn HEAD there is no HEAD tree to compare against, so the pre-check runs `git ls-files --cached`.
// It treats any listed path as something staged.
// This avoids an empty-tree object constant, which depends on the hash algorithm.
//
// CommitPathAllowEmpty(msg, relPath) serves a caller that wants a commit to land regardless.
// The caller also has a path to scope it to.
// It runs `git commit --allow-empty -- <relPath>`, so the commit holds the working tree's copy of relPath alone.
// A relPath with no change against HEAD lands an empty commit.
// Another path already staged stays staged and out of the commit.
// It needs no pre-check and never returns ErrIndexNotEmpty.
// git refuses a relPath that matches no tracked or present file, and the error comes back wrapped.
//
// The package's never-sweep norm is that an automated commit must never absorb content nobody asked it to commit.
// StageAndCommit enforces it structurally through pathspec scoping:
// its `commit ... -- <files>` cannot commit an unlisted path, whatever races it.
// CommitEmpty has no pathspec to scope with, so it approximates the norm by refusing:
// an index found dirty at the pre-check returns ErrIndexNotEmpty (checkable with errors.Is) without committing.
// The guarantee is narrower, because the pre-check and the commit are two separate git spawns.
// An index write landing between them is still swept into the commit.
// The package does not serialize callers' writes against that window; that is the caller's discipline.
//
// # SHAExists — history-rewrite safety
//
// gitrepo is not a general-purpose git wrapper (see Scope boundaries below).
// A human always has plain git in the working tree.
// A rebase, amend or force-push can therefore invalidate a stored SHA under any consumer.
// Making gitrepo rebase-aware is open-ended: it needs reflog tracking and remapping of every stored reference.
// SHAExists is instead a cheap existence check.
// Code that reads a stored SHA calls it first and treats a missing SHA as any other staleness signal:
// it forces a rebuild or re-sync instead of trusting a reference that may no longer be valid.
// This applies the "advance state only on confirmed success" principle to a ground truth that moved under the consumer.
//
// # Scope boundaries — not a general-purpose git wrapper
//
// gitrepo covers only the operations its consumers need programmatically.
// StageAndCommit takes an explicit file list and never wildcard-stages.
// StageAllAndCommit is the separate wildcard-stage variant, an opt-in exception for board.
// It does not relax the explicit-list default, which fabric and raddle keep using.
// Merge start and conclude and conflicted-path enumeration are admitted for fabric's two-sided merge coordination.
// They are MergeStart, MergeConclude, ConflictedFiles, MergeHeadPresent, HeadDetached, MergeFFOnly and ResolveSHA.
// Two behaviours they depend on are pinned on the command line instead of inherited from the caller's git config:
// MergeStart passes --ff so merge.ff=only/false cannot change the outcome it classifies.
// MergeConclude passes --no-edit so core.editor can never hang a non-interactive caller.
// MergeStart's four-way classification reads a live MERGE_HEAD as MergeStaged before it reads HEAD movement.
// A real merge whose result tree equals HEAD's tree stages nothing and moves no HEAD, yet leaves a merge to conclude;
// reading only the index diff and HEAD would report it as MergeAlreadyUpToDate and strand it.
// Rebase as a caller operation, interactive staging and conflict *resolution* are not supported.
// General branch and checkout management is not supported either.
// gitrepo reports conflicts; resolving them is the caller's job.
// fabric layers its own topology operations on top of gitrepo.
// They are clone, worktree add and remove, general checkout and branch naming.
//
// # Push surface
//
// Push and PushCoalesced are push-only: committing is the caller's separate, prior step, and push never stages anything.
// Every push names its branch explicitly.
// The checked-out branch goes to the same-named branch of its push remote, which is its upstream remote or origin.
// A branch with no upstream takes that remote branch as its upstream once the push succeeds.
// This matches git's push.autoSetupRemote.
// A checkout's first push therefore establishes tracking instead of failing.
// A checkout with no upstream configured always counts as having something to push, so the first push is attempted.
// Push runs one git push.
// It recovers from exactly one non-fast-forward-style rejection.
// Such a rejection has a stderr containing "non-fast-forward", "rejected" or "fetch first".
// Push recovers with one `pull --rebase` and then retries.
// The rebase-retry requires a worktree clean of tracked-file changes, which the caller's prior commit call ensures.
// A push that recovered through the rebase rewrote the local commits it replayed.
// A SHA captured before the push may afterwards name an off-history commit.
// StageAndCommit's return value is such a SHA.
// SHAExists still reports true for it through the reflog.
// Callers re-read CurrentSHA after a successful push before recording a SHA anywhere.
// Push, PushCoalesced and PushRebaseFree also retry a push the remote refuses on the pushed branch.
// One retryable form is `! [remote rejected] <ref> -> <ref> (failed)`.
// A remote reason containing `cannot lock ref` is retryable too.
// A ref given in full `refs/heads/` or `refs/tags/` form also matches the short form git prints it in.
// A push is tried up to four times in total, with a doubling backoff from one second.
// The retries run inside whatever lock the caller holds and neither rebase nor force.
// A retryable refusal never triggers the rebase-retry.
// A push that exhausts the attempts returns an error wrapping ErrPushRetriesExhausted and the last push error.
// It never returns ErrPushRejected.
// PushWithRetry exposes the same loop for a caller that runs its own push, on the backoff delays the caller passes;
// PushWithRetryReasons adds reasons the caller names as retryable beside those two forms.
// PushCoalesced adds cross-process coalescing.
// It first acquires a single-pusher lock file (Repo.PushLockPath).
// The file lives in the checkout's own git dir, outside the tracked tree.
// It then checks whether anything is unpushed.
// A burst of concurrent callers therefore collapses into as few pushes as possible.
// A caller that finds nothing unpushed once it holds the lock returns immediately.
//
// # Pull surface
//
// Pull runs `git pull --ff-only` and is fast-forward-only by contract.
// A diverged local branch, one with commits its upstream lacks, is an error and never a merge commit.
// Recovering from divergence (rebase, reset, manual merge) is caller policy; gitrepo does not attempt it.
// A non-zero exit from Pull names the repo path and git's exit code without embedding git's raw stderr,
// so a diverged-branch or no-remote failure never leaks git's own "fatal:"-prefixed message text.
//
// # ResetHard surface
//
// ResetHard is the primitive for coordinated history recovery.
// It points HEAD, the index and the working tree at a caller-supplied SHA exactly, via `git reset --hard`.
// It discards any local commits or uncommitted changes past that point.
// The SHA-shape validation (see above) keeps an option-shaped string from reaching `git reset` as a flag:
// ResetHard rejects it as ErrInvalidSHA before any git spawn.
//
// ResetKeep resets via `git reset --keep`, for a caller whose checkout holds uncommitted work to preserve.
// It validates the SHA the same way, moves HEAD and the index, and leaves every uncommitted change in place.
// When an uncommitted change sits in a path the move would rewrite, it changes nothing.
// It then returns git's refusal, wrapped as the checked *gitexec.GitError.
// ResetKeep mutates the working tree, so it stays CLI-bound.
//
// # Evidence for the two-backend boundary
//
// The boundary above rests on two probes run on Windows against go-git/go-git/v5 v5.19.1.
// Their reports are not kept in the repository, so this section is their durable record.
// It is exhaustive rather than illustrative.
// A reader who sees only "commits are CLI-bound" will try to migrate them again;
// one who sees only "use go-git" will write PlainOpen and ship a silently broken handle.
//
// From the first probe, which measured the commit methods and their hazards:
//
//   - go-git performs no CRLF conversion.
//     With core.autocrlf=true and a CRLF-line-ending source file, the CLI stored one blob.
//     go-git stored a different one, which differs in bytes and in object name.
//     A file committed by go-git is thereafter permanently "modified" to CLI git,
//     which converts working-tree CRLF to LF and compares against the stored CRLF blob.
//     This is why StageAndCommit and StageAllAndCommit stay CLI-bound:
//     autocrlf=true is the Git-for-Windows system default,
//     and editors that default to CRLF on Windows routinely write the overlay files these methods commit.
//   - go-git's hard reset deletes untracked and gitignored files.
//     Worktree.Reset(HardReset) removes every one of them with no error, where the CLI leaves them in place.
//     In a lym checkout that would silently destroy .lym/, .scratch/ and build output.
//     That is a data-loss path inside ResetHard, which fabric's history-recovery flow builds on.
//     ResetHard stays CLI-bound.
//   - Checkout and Pull are non-atomic: go-git moves HEAD before the dirty check that can reject the operation.
//     A rejection leaves HEAD already moved to the target commit while the working tree still holds the old content.
//     git status then reports a mixed modified state, where the CLI aborts having moved nothing.
//     Pull also failed in repeated trials with a spurious unstaged-changes rejection on a stock Windows checkout.
//     That happened with core.autocrlf=true and never with autocrlf=false, and HEAD had already moved in every failure.
//     Pull stays CLI-bound.
//   - go-git never invokes a git credential helper.
//     It accepts an explicit transport.AuthMethod but has no mechanism to discover one from the environment.
//     Against an HTTPS remote that Git Credential Manager serves, a go-git push would fail outright.
//     A go-git fetch would fail on every call there.
//     Push, PushCoalesced and Pull all stay CLI-bound for this reason.
//     Push's rebase-retry is also CLI-bound because go-git ships no rebase implementation.
//
// Every real checkout uses a linked-worktree topology.
// A linked worktree's .git is a file.
// fabricengine creates the repo checkout and its fabric mirror as linked worktrees.
// internal/fslink reaches them through junctions.
// From the second probe, which measured that topology:
//
//   - git.PlainOpenWithOptions with EnableDotGitCommonDir: true is mandatory.
//     Against a linked worktree, git.PlainOpen defaults EnableDotGitCommonDir to false and returns no error.
//     The handle it returns cannot read HEAD or any object.
//     It reports every ref stored in the shared common dir as absent, with no error anywhere.
//     This is why goGit (gogit.go) opens with the option explicitly set rather than the convenience call.
//   - DetectDotGit is banned.
//     Given a path that is not itself a repository, it walks up the directory tree.
//     It silently opens an ancestor repository.
//     In the probe it escaped a fixture directory and opened the loomyard checkout.
//     gitrepo's callers always pass a worktree root, so DetectDotGit buys nothing.
//     It risks opening the wrong repository.
//   - KeepDescriptors must stay false, its default.
//     Under the default, an open go-git handle does not block `git worktree remove` on Windows.
//     That holds for a warmed handle held open mid-iteration too.
//     Forcing KeepDescriptors true does lock common-dir packfiles against it.
//     The default lets goGit cache a handle for the Repo's whole lifetime.
//     No explicit close is needed before a fabric topology mutation.
//   - Packfile-only objects need a reactive remedy, not a re-open.
//     A cached *git.Repository sees the CLI's ref writes and loose-object writes made later in the same process.
//     The one exception is an object that exists only in a packfile written after the handle indexed.
//     It reads as object-not-found though Head() returns its SHA, because ObjectStorage builds its index only once.
//     storer.Reindex() repairs this fully, so the fix is not a re-open.
//     The package's policy is reactive, and it retries the whole operation.
//     Every go-git read runs through readGoGit in gogit.go.
//     readGoGit reruns the read once after a reindex when the read fails with object-not-found,
//     and the pack-directory fingerprint differs from the snapshot taken before the read began.
//     Wrapping the whole read, not each object lookup, covers a Log walk, a tree diff or Worktree.Status().
//     Those reach objects that no per-lookup wrapper sees.
//     The comparison is against that snapshot, not only the last stored fingerprint.
//     A caller that lost a race to a concurrent reindex therefore still reruns.
//     The read holds the shared lock.
//     Only the fingerprint check, the reindex and the index rebuild that follows it take the exclusive lock,
//     so readers do not serialize.
//     The rebuild is forced there because go-git rebuilds its index unsynchronized on the next read.
//     An absent object reindexes at most once while the pack set is unchanged.
//     The policy is reactive because the trigger set for a proactive one cannot be enumerated reliably.
//     `gc --auto` fires after ordinary commits and fetches alike.
//     A hand-maintained list of operations that might repack goes stale when git's own auto-gc heuristics change.
//   - extensions.worktreeConfig makes go-git refuse to open a repo.
//     The refusal is a typed error that errors.Is matches against go-git's sentinel.
//     No lym code path sets the extension.
//     A worktree that enables it later must surface as a clear typed error from gitrepo.
//     It must never surface as a confusing downstream nil.
//     This is why goGit wraps the error with %w rather than swallowing it.
package gitrepo
