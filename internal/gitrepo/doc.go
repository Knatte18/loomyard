// Package gitrepo provides a typed Repo over a single local git checkout,
// split across two backends: go-git for local object and ref reads, and
// internal/gitexec's raw command runner for anything that authenticates to a
// remote or mutates the working tree. It exposes the small set of semantic
// operations (current SHA, stage+commit, changed-files-since, SHA existence,
// push, pull, hard reset, snapshot tracking) that every consumer of a
// git-backed repo (fabric, raddle, codeintel, webster) would otherwise
// reimplement by parsing raw git stdout itself.
//
// # Relationship to internal/gitexec — the two-backend boundary
//
// internal/gitexec is deliberately minimal: one function, RunGit(args
// []string, cwd string) (stdout, stderr string, exitCode int, err error),
// that shells out to git and returns raw output. gitrepo used to route every
// method through it via a single unexported run helper; it no longer does.
// The read surface — CurrentSHA, SHAExists, ChangedFilesSince, CurrentBranch,
// remoteName, isStrictDescendant, SnapshotSHA's ref read, and
// SetSnapshotSHA's two inline local reads — resolves state entirely through
// go-git's own object and ref access (see gogit.go), bypassing run and
// gitexec.RunGit completely. Everything that authenticates to a remote or
// mutates the working tree stays CLI-bound through run: StageAndCommit,
// StageAllAndCommit, Push, PushCoalesced, Pull, ResetHard, CheckoutDetached,
// RestoreBranch, SetSnapshotSHA's push, SnapshotSHA's fetch, and hasUnpushed
// (measured and reverted from a go-git ancestry walk; see hasUnpushed's own
// godoc in push.go for the reversal criterion). See
// CONSTRAINTS.md's gitrepo Client Boundary Invariant for the enforced,
// exhaustive version of this split and the review obligation any new CLI
// call inside this package carries. gitexec itself stays a zero-dependency
// leaf regardless of which side of the boundary a gitrepo method is on — it
// has roughly eighty call-sites across packages, some lower in the layering
// than gitrepo (e.g. hubgeometry), and gitrepo remains one of its many
// consumers, not merged into it.
//
// Some outcome classification still matches git's own untranslated
// (C/English locale) message text — but less of it than before this
// package's read surface migrated. CurrentSHA's unborn-HEAD detection is now
// typed (plumbing.ErrReferenceNotFound, surfaced as ErrNoCommits) and no
// longer depends on git's locale at all. The recoverable push-rejection
// triggers behind Push's rebase-retry and SetSnapshotSHA's adopt-on-conflict,
// and the benign "no rebase in progress" abort check, stay CLI-bound and
// therefore still match git's untranslated stderr text: a git with
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
//   - CurrentSHA, StageAndCommit, StageAllAndCommit, ChangedFilesSince, and
//     SHAExists are the core read/write primitives.
//   - Push and PushCoalesced are the push surface (see below).
//   - Pull is the fast-forward-only pull surface (see below).
//   - ResetHard is the SHA-validated hard-reset surface (see below).
//   - SnapshotSHA and SetSnapshotSHA are the snapshot-tracking surface (see
//     below).
//   - CurrentBranch, CheckoutDetached, and RestoreBranch are the in-place
//     bisect exception (see Scope boundaries below).
//
// Caller-supplied SHA arguments (SHAExists, ChangedFilesSince,
// SetSnapshotSHA, ResetHard) are validated as plain hex object names before
// ever reaching git, so an option-shaped string (a value with a leading '-',
// e.g. "--hard") can never be parsed as a git flag; invalid SHAs surface as
// ErrInvalidSHA, or as false from SHAExists per its bool-swallowing posture.
//
// # The self-correcting snapshot pattern
//
// SnapshotSHA/SetSnapshotSHA is the one pattern every consumer of gitrepo
// (fabric's coordination, raddle's staleness tracking, codeintel's
// per-language notification) reuses: a consumer only calls SetSnapshotSHA
// after confirmed success. If a downstream step fails partway, the stored
// SHA is not advanced, so the next attempt naturally recomputes the diff
// from the old SHA and catches everything missed — including from earlier
// failed attempts. No separate crash-recovery logic is needed; correctness
// falls out of the "advance state only on confirmed success" rule.
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
// reset, snapshot/correspondence tracking, and the CurrentBranch/CheckoutDetached/RestoreBranch
// trio below. StageAllAndCommit is a separate wildcard-stage variant provided as board's
// opt-in exception, not a relaxation of the explicit-list default — fabric, raddle, and
// codeintel keep using explicit-list StageAndCommit (called via
// fabricengine.CommitWeftAt on board's behalf, not boardengine calling
// gitrepo directly). Rebase, interactive
// staging, cherry-pick, conflict resolution, and general-purpose branch/checkout management are
// explicitly not supported — a human can always use plain git directly in
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
// push before recording a SHA anywhere, SetSnapshotSHA included.
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
// ResetHard is the primitive fabric's RevertWithWeft history-recovery flow
// builds on: it points HEAD, the index, and the working tree at a
// caller-supplied SHA exactly, via `git reset --hard`, discarding any local
// commits or uncommitted changes past that point. The SHA-shape validation
// every caller-supplied SHA argument goes through (see above) guarantees an
// option-shaped string can never reach `git reset` as a flag instead of a
// target commit — ResetHard rejects it as ErrInvalidSHA before any git
// spawn, exactly like ChangedFilesSince. Non-zero-exit errors follow Pull's
// no-stderr-leak style: the repo path and git's exit code, never raw stderr.
//
// # Snapshot remote model
//
// SnapshotSHA/SetSnapshotSHA store each key's value under
// refs/loomyard/snapshot/<key>, pushed to the repo's remote so state is
// shared across clones rather than confined to one worktree. SnapshotSHA
// performs a best-effort fetch of the whole snapshot namespace before
// reading; a fetch failure degrades to the last-known local ref rather than
// surfacing as an error, since a slightly-stale snapshot at worst
// reprocesses already-done work. SetSnapshotSHA writes are fast-forward-only
// with adopt-on-conflict: a rejected push normally means another clone
// already advanced the key past this value, so SetSnapshotSHA fetches and
// adopts the remote's value into the local ref and returns nil rather than
// an error — a key advances along a single monotonically-forward line, so a
// rejection usually means someone else processed further and their SHA is
// the correct one to take. The one exception is transient contention — a
// remote-side creation race that rejects the loser regardless of ancestry
// ("reference already exists"), or a lost ref-lock race under concurrent
// writers; when the adopted value turns out to be a strict ancestor of the
// value being set, SetSnapshotSHA re-advances and retries the push, looping
// (bounded) until it lands or the remote genuinely moves past it, so a
// strictly-newer value is not silently dropped by transient contention — not
// even under three or more concurrent writers, where a single retry could
// itself lose the race.
//
// The snapshot remote is resolved from the current branch's tracking
// configuration, falling back to the conventional "origin" name. In a repo
// that violates that assumption (its only remote named something else, no
// branch tracking configured) the two surfaces degrade asymmetrically:
// SnapshotSHA's best-effort fetch fails silently every call, so reads report
// only the local ref — ("", nil) forever if nothing was ever set locally,
// indistinguishable from "no snapshot" — while SetSnapshotSHA fails loudly
// on its push. The loud write-path failure is what keeps the silence
// acceptable: a misconfigured consumer cannot run long without its first
// write surfacing the problem.
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
//     outright and a go-git fetch would fail on every call — the latter more
//     dangerous, since SnapshotSHA's fetch already swallows failures by
//     design, so snapshot refs would silently stop being shared across
//     clones. Push, PushCoalesced, SetSnapshotSHA's push, and SnapshotSHA's
//     fetch all stay CLI-bound for this reason. Push's rebase-retry is
//     doubly CLI-bound regardless: go-git ships no rebase implementation at
//     all.
//
// From the second probe (measuring the linked-worktree topology every real
// checkout in this codebase uses — this checkout's own .git is a file,
// fabricengine creates host and weft as linked worktrees, and
// internal/fslink reaches them through junctions):
//
//   - git.PlainOpenWithOptions with EnableDotGitCommonDir: true is mandatory.
//     Against a linked worktree, the obvious call — git.PlainOpen, which
//     defaults EnableDotGitCommonDir to false — returns NO error and hands
//     back a handle that cannot read HEAD, cannot read any object, and
//     reports every existing refs/loomyard/snapshot/* key as absent,
//     forever, with no error anywhere. This is why goGit (gogit.go) opens
//     with the option explicitly set rather than the convenience call.
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
