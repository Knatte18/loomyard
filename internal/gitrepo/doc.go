// Package gitrepo provides a typed Repo over a single local git checkout,
// split across two backends: go-git for local object and ref reads, and
// internal/gitexec's raw command runner for anything that authenticates to a
// remote or mutates the working tree. It exposes the small set of semantic
// operations (current SHA, stage+commit, changed-files-since, SHA existence,
// push, pull, hard reset) that every consumer of a git-backed repo (fabric,
// raddle, scout, webster) would otherwise reimplement by parsing raw git
// stdout itself.
//
// # Relationship to internal/gitexec — the two-backend boundary
//
// internal/gitexec exposes a two-function pair: RunGit(args []string, cwd
// string) (stdout, stderr string, exitCode int, err error), the raw form
// that shells out to git and returns its exit code for the caller to
// classify, and Run(args []string, cwd string) (string, error), the checked
// form that treats a non-zero exit as failure and returns it as *GitError.
// gitrepo used to route every CLI-bound method through a single unexported
// run helper over RunGit; it now has a matching pair, run and runChecked.
// The read surface — CurrentSHA, SHAExists, ChangedFilesSince, and
// CurrentBranch — resolves state entirely through go-git's own object and
// ref access (see gogit.go), bypassing both of them completely. Of the
// CLI-bound methods, only Pull and Fetch sit on the raw run — a non-zero
// exit is a failure at both sites too, but the package's test-enforced
// no-`fatal:`-leak surface forbids folding git's stderr into their
// messages, which is what run's raw form lets them keep working around.
// Every other CLI-bound method — StageAndCommit, StageAllAndCommit, Push,
// PushCoalesced, ResetHard, CheckoutDetached, RestoreBranch, IsAncestor, and
// HasUnpushed (measured and reverted from a go-git ancestry walk; see
// HasUnpushed's own godoc in push.go for the reversal criterion) — sits on
// runChecked. See CONSTRAINTS.md's gitrepo Client Boundary Invariant for the
// enforced, exhaustive version of this split and the review obligation any
// new CLI call inside this package carries. gitexec itself stays a
// zero-dependency leaf regardless of which side of the boundary a gitrepo
// method is on — it has roughly seventy non-test call sites across
// gitrepo, fabricengine, fabriccli, lyxcwd, and websterengine, and gitrepo
// remains one of its many consumers, not merged into it.
//
// Some outcome classification still matches git's own untranslated
// (C/English locale) message text — but less of it than before this
// package's read surface migrated. CurrentSHA's unborn-HEAD detection is now
// typed (plumbing.ErrReferenceNotFound, surfaced as ErrNoCommits) and no
// longer depends on git's locale at all. The recoverable push-rejection
// triggers behind Push's rebase-retry, and the benign "no rebase in
// progress" abort check, stay CLI-bound and therefore still match git's
// untranslated stderr text: a git with
// translation catalogs installed, running under a non-English locale,
// defeats those matches, and every affected path then degrades loudly — a
// hard push error instead of a recovery — never a silent wrong success.
// Pinning the subprocess locale would be a gitexec-level, repo-wide
// decision, deliberately not taken here.
//
// # The Repo API
//
// New(path) wraps an existing checkout with no validation and no I/O — it
// cannot fail, and it does not create, clone, or otherwise manage repo
// topology; that is fabric's job, built directly on gitexec. From there:
//
//   - CurrentSHA, StageAndCommit, CommitEmpty, StageAllAndCommit,
//     ChangedFilesSince, and SHAExists are the core read/write primitives.
//   - Push and PushCoalesced are the push surface (see below).
//   - Pull is the fast-forward-only pull surface (see below); Fetch is its
//     fetch-without-merge sibling, refreshing remote-tracking refs without
//     ever moving the local branch.
//   - IsAncestor is the ancestry/reachability primitive: it answers "is sha
//     an ancestor of ref" via `git merge-base --is-ancestor`, mapping git's
//     tri-state exit code directly rather than folding any non-zero exit
//     into failure.
//   - ResetHard is the SHA-validated hard-reset surface (see below).
//   - CurrentBranch, CheckoutDetached, and RestoreBranch are the in-place
//     bisect exception (see Scope boundaries below).
//
// Caller-supplied SHA arguments (SHAExists, ChangedFilesSince, ResetHard) are
// validated as plain hex object names before ever reaching git, so an
// option-shaped string (a value with a leading '-', e.g. "--hard") can never
// be parsed as a git flag; invalid SHAs surface as ErrInvalidSHA, or as false
// from SHAExists per its bool-swallowing posture.
//
// # CommitEmpty — the empty-commit primitive
//
// CommitEmpty(msg) records a commit whose only content is msg itself — a
// commit whose tree is identical to its parent's, carrying no file changes at
// all. Neither existing primitive can be coaxed into that shape: StageAndCommit
// returns its documented no-op signal, by design, on an empty file list, and
// its `diff --cached --quiet` gate returns that same no-op the moment the
// listed files' staged content matches HEAD — both are "commit this content,
// or signal that there was none" primitives, and an empty commit has no
// content to key either check on. CommitEmpty exists for the caller that
// wants a commit to land regardless, with meaning carried entirely in msg.
//
// Because it has no content to check, CommitEmpty instead runs a pre-check on
// the index itself, branching on whether HEAD is born or unborn. On a born
// HEAD, an unscoped `git diff --cached --quiet` reports via exit code exactly
// like StageAndCommit's own pathspec-scoped check does. On an unborn HEAD
// there is no HEAD tree for `diff --cached` to compare against, so the
// pre-check runs `git ls-files --cached` instead and treats any listed path
// as "something is staged" — this avoids needing an empty-tree object
// constant, which is hash-algorithm-dependent and would be a latent
// SHA-256 bug the moment a repo's hash algorithm changes.
//
// This package's never-sweep norm — an automated commit must never absorb
// content nobody asked it to commit — is what StageAndCommit enforces
// structurally, through pathspec scoping: its `commit ... -- <files>` cannot
// commit an unlisted path no matter what races it. CommitEmpty has no
// pathspec to scope an empty commit with, so it can only approximate that
// norm by refusing: finding the index dirty at the pre-check returns
// ErrIndexNotEmpty (checkable via errors.Is) without committing. That leaves
// an honest, narrower guarantee than StageAndCommit's — CommitEmpty is
// check-then-commit across two separate git spawns, so an index write landing
// in the milliseconds between the pre-check and the commit is still swept
// into it, a window StageAndCommit's pathspec scoping closes structurally and
// CommitEmpty cannot. This package makes no claim that CommitEmpty's callers
// serialize their writes against that window; that discipline lives with the
// caller.
//
// # SHAExists — history-rewrite safety
//
// gitrepo is not a general-purpose git wrapper (see Scope boundaries below)
// — a human always has plain git available in the working tree. That means
// rebase/amend/force-push can invalidate a stored SHA reference out from
// under any consumer. Rather than making gitrepo "rebase-aware"
// (open-ended: reflog tracking, remapping every stored reference), SHAExists
// is a cheap existence check. Any code reading a stored SHA should call it
// first and treat a missing SHA as any other staleness signal — force a
// rebuild/re-sync rather than trusting a reference that may no longer be
// valid. This extends the "advance state only on confirmed success"
// principle to also cover "the ground truth moved out from under us," not
// just "we lost track ourselves."
//
// # Scope boundaries — deliberately not a general-purpose git wrapper
//
// gitrepo covers only the operations its consumers actually need
// programmatically: stage+commit (explicit file list, never wildcard-stage),
// diff-since-SHA, current-SHA, push, fast-forward pull, SHA-validated hard
// reset, and the CurrentBranch/CheckoutDetached/RestoreBranch
// trio below. StageAllAndCommit is a separate wildcard-stage variant provided as board's
// opt-in exception, not a relaxation of the explicit-list default — fabric, raddle, and
// scout keep using explicit-list StageAndCommit (called via fabricengine's own
// board-facing commit wrapper on board's behalf, not boardengine calling
// gitrepo directly). Merge start/conclude and conflicted-path enumeration
// (MergeStart, MergeConclude, ConflictedFiles, MergeHeadPresent, HeadDetached,
// MergeFFOnly, ResolveSHA) are admitted, used by fabric's two-sided merge
// coordination. The two behaviours these depend on are pinned on the command
// line rather than inherited from the caller's git config: MergeStart passes
// --ff so merge.ff=only/false cannot change the outcome it classifies, and
// MergeConclude passes --no-edit so core.editor can never hang a
// non-interactive caller.
// Rebase, interactive staging, cherry-pick, conflict *resolution*, and
// general-purpose branch/checkout management stay explicitly not supported —
// gitrepo reports conflicts, but resolving them is the caller's job. A human
// can always use plain git directly in
// the working tree, since it's an ordinary git repo underneath. fabric
// layers a further, separate set of topology operations — clone, worktree
// add/remove, general checkout, branch naming — on top of gitrepo; those
// remain fabric-specific, not part of gitrepo itself. The one admitted
// exception is CurrentBranch/CheckoutDetached/RestoreBranch, used solely by
// webster's integration bisect to check a candidate SHA out in place in its
// single worktree, run the plan's verify step, and restore HEAD to the
// branch it started on. That is a sequential, post-run, read-only-ish
// inspection cycle over the one worktree gitrepo already has a handle
// on — not a topology operation (no clone, no worktree add/remove, no new
// branch), and never run concurrently with fabric's parallel topology work —
// so it stays inside gitrepo rather than becoming a second, overlapping
// checkout surface in fabric.
//
// # Push surface
//
// Push and PushCoalesced are both push-only — committing is always the
// caller's separate, prior step, whether via StageAndCommit's explicit list
// or board's StageAllAndCommit wildcard escape hatch; push itself never
// stages anything. Every push goes through `git -c push.autoSetupRemote=true
// push`, so a checkout's very first push (no upstream configured yet) still
// succeeds and establishes tracking instead of failing outright — a
// checkout with no upstream configured is always treated as having
// something to push, so the first push is attempted rather than skipped.
// Push runs a single git push and transparently recovers from exactly one
// non-fast-forward-style rejection (stderr containing "non-fast-forward",
// "rejected", or "fetch first") via one `pull --rebase` before retrying; the
// rebase-retry path requires a worktree clean of tracked-file changes,
// which the caller's prior commit call is responsible for. A push that
// recovered via the rebase rewrote the local commits it replayed: any SHA
// captured before the push (StageAndCommit's return value in particular)
// may afterwards name an off-history commit that SHAExists still reports
// true for via the reflog — callers re-read CurrentSHA after a successful
// push before recording that SHA anywhere.
// PushCoalesced adds cross-process coalescing on top: it acquires a
// single-pusher lock file, .gitrepo-push.lock, in the repo's
// worktree root before checking whether anything is actually unpushed, so a
// burst of concurrent callers collapses into as few pushes as possible — a
// caller that finds nothing unpushed once it acquires the lock returns
// immediately instead of pushing again.
//
// # Pull surface
//
// Pull runs `git pull --ff-only` and is fast-forward-only by contract: a
// diverged local branch — one with commits its upstream lacks — is an error,
// never a merge commit. Recovering from divergence (rebase, reset, manual
// merge) stays a caller policy; gitrepo does not attempt it. Unlike the rest
// of the package's error style, a non-zero exit from Pull names the repo
// path and git's exit code without embedding git's raw stderr, so a
// diverged-branch or no-remote failure never leaks git's own
// "fatal:"-prefixed message text.
//
// # ResetHard surface
//
// ResetHard is the primitive fabric's coordinated history-recovery flows
// build on: it points HEAD, the index, and the working tree at a
// caller-supplied SHA exactly, via `git reset --hard`, discarding any local
// commits or uncommitted changes past that point. The SHA-shape validation
// every caller-supplied SHA argument goes through (see above) guarantees an
// option-shaped string can never reach `git reset` as a flag instead of a
// target commit — ResetHard rejects it as ErrInvalidSHA before any git
// spawn, exactly like ChangedFilesSince.
//
// # Evidence for the two-backend boundary
//
// The boundary above was reached empirically, by two background probes run
// against this go-git version (go-git/go-git/v5 v5.19.1) before any read
// migrated. Both probe reports lived under .scratch/, which is gitignored
// and disappears with the worktree that produced it — this section is their
// only durable record, so it is exhaustive rather than illustrative. A
// future reader who sees only "commits are CLI-bound" will try to migrate
// them again; one who sees only "use go-git" will write PlainOpen and ship a
// silently broken handle.
//
// From the first probe (measuring the commit methods and three hazards):
//
//   - go-git performs no CRLF conversion at all. Against identical fixtures
//     with core.autocrlf=true and a CRLF-line-ending source file, the CLI
//     stored one blob and go-git stored a different one — different bytes,
//     different object name. A file committed by go-git is thereafter
//     permanently "modified" to CLI git, which converts working-tree CRLF to
//     LF and compares against the stored CRLF blob. This is why
//     StageAndCommit and StageAllAndCommit stay CLI-bound: autocrlf=true is
//     the Git-for-Windows system default, and the overlay files these
//     methods commit are routinely written by editors that default to CRLF
//     on Windows.
//   - go-git's hard reset deletes untracked and gitignored files. Against
//     identical fixtures, the CLI leaves untracked and gitignored files in
//     place; go-git's Worktree.Reset(HardReset) removes every one of them
//     with no error. In a lyx checkout that would silently destroy .lyx/,
//     .scratch/, and build output — a data-loss path inside ResetHard, which
//     fabric's history-recovery flow builds on. ResetHard stays CLI-bound.
//   - Checkout and Pull are non-atomic: go-git moves HEAD before the dirty
//     check that can reject the operation. Observed: an unstaged-changes
//     rejection with HEAD already moved to the target commit, the working
//     tree still holding the old content, and git status reporting a mixed
//     modified state — the CLI aborts having moved nothing. Pull additionally
//     failed 25 of 40 trials with a spurious unstaged-changes rejection on a
//     stock Windows checkout with core.autocrlf=true (0 of 40 with
//     autocrlf=false), HEAD already moved in all 25 failures. Pull and
//     CheckoutDetached stay CLI-bound; RestoreBranch stays paired with
//     CheckoutDetached for the same reason even though it was not itself the
//     failing half.
//   - go-git never invokes a git credential helper. Its only "credential"
//     occurrences are doc comments on an Auth option field; it accepts an
//     explicit transport.AuthMethod but has no mechanism to discover one from
//     the environment. Against a real HTTPS remote resolved through Git
//     Credential Manager (this repo's own remote), a go-git push would fail
//     outright and a go-git fetch would fail on every call. Push,
//     PushCoalesced, and Pull all stay CLI-bound for this reason. Push's
//     rebase-retry is doubly CLI-bound regardless: go-git ships no rebase
//     implementation at all.
//
// From the second probe (measuring the linked-worktree topology every real
// checkout in this codebase uses — this checkout's own .git is a file,
// fabricengine creates the repo checkout and its fabric mirror as linked worktrees, and
// internal/fslink reaches them through junctions):
//
//   - git.PlainOpenWithOptions with EnableDotGitCommonDir: true is mandatory.
//     Against a linked worktree, the obvious call — git.PlainOpen, which
//     defaults EnableDotGitCommonDir to false — returns NO error and hands
//     back a handle that cannot read HEAD, cannot read any object, and
//     reports every ref stored in the shared common dir as absent, forever,
//     with no error anywhere. This is why goGit (gogit.go) opens with the
//     option explicitly set rather than the convenience call.
//   - DetectDotGit is banned outright. Set true and pointed at a path that is
//     not itself a repository, it walks up the directory tree and silently
//     opens whatever ancestor repository it finds — proven, in the probe, to
//     escape a fixture directory and open this very loomyard checkout
//     instead. gitrepo's callers always pass a worktree root, so DetectDotGit
//     buys nothing and risks operating on the wrong repository entirely.
//   - KeepDescriptors must stay false (its default). Measured, not assumed:
//     an open go-git handle does not block `git worktree remove` on Windows
//     under the default (code 0 even with a warmed handle held open
//     mid-iteration), but forcing KeepDescriptors true does lock common-dir
//     packfiles against it. This is what lets goGit cache a handle for the
//     Repo's whole lifetime with no explicit close needed before a fabric
//     topology mutation.
//   - Packfile-only objects need a reactive remedy, not a re-open. A cached
//     *git.Repository does see the CLI's ref writes and loose-object writes
//     made in the same process afterwards — the one measured exception is an
//     object that exists only in a packfile written after the handle indexed,
//     which reads as object-not-found even though the same handle's Head()
//     returns that very SHA, because ObjectStorage's index is built once and
//     never refreshed. storer.Reindex() fully repairs this, so the fix is not
//     a re-open. The policy this package implements is reactive rather than
//     proactive — reindex once and retry only when a pack-directory
//     fingerprint has changed since the last index build (see
//     lookupObjectRetrying in gogit.go) — because the trigger set for a
//     proactive policy cannot be enumerated reliably: the probe observed
//     `gc --auto` firing after ordinary commits and fetches alike, so any
//     hand-maintained list of "operations that might repack" goes stale the
//     moment git's own auto-gc heuristics change.
//   - extensions.worktreeConfig makes go-git refuse to open a repo outright,
//     with its own typed error (checkable via errors.Is against go-git's
//     sentinel, see goGit's doc). Not set on this repo today and no lyx code
//     path sets it, but a worktree created in a way that enables it later
//     must surface as a clear typed error from gitrepo, never a confusing
//     downstream nil — this is why goGit wraps rather than swallows it.
//
// Gate (c) of the original spike's rubric — "works on Windows 11" — carried
// forward as a Win11-pending marker on every one of gitnativepoc's MIGRATE
// verdicts (see manifest/roadmap.md's retired git-native-library entry for
// that history). This task closes it for every migrated method: both probes
// above ran on Windows, and the package's Tier-2 integration suite —
// internal/gitrepo's own tests plus the parity harness comparing every
// migrated method against the real git CLI — was run as part of this task's
// whole-repo verification. That is a record, not a promise to check later;
// no Win11-pending marker is carried forward from here.
package gitrepo
