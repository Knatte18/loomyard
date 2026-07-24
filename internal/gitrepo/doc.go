// Package gitrepo provides a typed Repo over a single local git checkout,
// built on top of internal/gitexec's raw command runner. It exposes the small
// set of semantic operations (current SHA, stage+commit, changed-files-since,
// SHA existence, push, snapshot tracking) that every consumer of a git-backed
// repo (fabric, raddle, codeintel, webster) would otherwise reimplement by
// parsing raw git stdout itself.
//
// # Relationship to internal/gitexec
//
// internal/gitexec is deliberately minimal: one function, RunGit(args
// []string, cwd string) (stdout, stderr string, exitCode int, err error),
// that shells out to git and returns raw output. gitrepo is the next layer
// up — it never calls exec.Command itself; every method goes through a
// single unexported run helper that wraps gitexec.RunGit with the Repo's
// path, so gitexec stays a zero-dependency leaf and gitrepo is one of its
// many consumers (not merged into it — gitexec has ~80 call-sites across
// packages, some lower in the layering than gitrepo, e.g. hubgeometry).
//
// # The Repo API
//
// New(path) wraps an existing checkout with no validation and no I/O — it
// cannot fail, and it does not create, clone, or otherwise manage repo
// topology; that is fabric's job, built directly on gitexec. From there:
//
//   - CurrentSHA, StageAndCommit, ChangedFilesSince, and SHAExists are the
//     core read/write primitives.
//   - Push and PushCoalesced are the push surface (see below).
//   - SnapshotSHA and SetSnapshotSHA are the snapshot-tracking surface (see
//     below).
//
// Caller-supplied SHA arguments (SHAExists, ChangedFilesSince,
// SetSnapshotSHA) are validated as plain hex object names before ever
// reaching git, so an option-shaped string (a value with a leading '-') can
// never be parsed as a git flag; invalid SHAs surface as ErrInvalidSHA, or
// as false from SHAExists per its bool-swallowing posture.
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
// diff-since-SHA, current-SHA, push, and snapshot/correspondence tracking.
// Rebase, interactive staging, cherry-pick, and conflict resolution are
// explicitly not supported — a human can always use plain git directly in
// the working tree, since it's an ordinary git repo underneath. fabric
// layers a further, separate set of topology operations — clone, worktree
// add/remove, checkout, branch naming — on top of gitrepo; those are
// fabric-specific, not part of gitrepo itself.
//
// # Push surface
//
// Push and PushCoalesced are both push-only — committing is always the
// caller's separate, prior StageAndCommit call, so a wildcard `add -A` never
// enters gitrepo. Every push goes through `git -c push.autoSetupRemote=true
// push`, so a checkout's very first push (no upstream configured yet) still
// succeeds and establishes tracking instead of failing outright — matching
// hasUnpushed's no-upstream-means-unpushed contract. Push runs a single git
// push and transparently recovers from exactly one non-fast-forward-style
// rejection (stderr containing "non-fast-forward", "rejected", or "fetch
// first" — the full trigger set board's sync.go:pushUnpushed matches) via
// one `pull --rebase` before retrying; the rebase-retry path requires a
// worktree clean of tracked-file changes, which StageAndCommit's caller is
// responsible for by having already committed. A push that recovered via the
// rebase rewrote the local commits it replayed: any SHA captured before the
// push (StageAndCommit's return value in particular) may afterwards name an
// off-history commit that SHAExists still reports true for via the reflog —
// callers re-read CurrentSHA after a successful push before recording a SHA
// anywhere, SetSnapshotSHA included. PushCoalesced adds
// cross-process coalescing on top: it
// acquires a single-pusher lock file, .gitrepo-push.lock, in the repo's
// worktree root before checking whether anything is actually unpushed, so a
// burst of concurrent callers collapses into as few pushes as possible — a
// caller that finds nothing unpushed once it acquires the lock returns
// immediately instead of pushing again. This is the coalescing engine behind
// board's sync.go push-loop replacement.
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
package gitrepo
