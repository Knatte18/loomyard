// Package hubreconcile reconciles a hub's config once after a binary change, under a hub lock.
// It commits everything it wrote.
// Its one entry point is Ensure; a start verb calls it, through EnsureOrWarn, before loading any module config.
//
// The package is told the hub's geometry and derives none (PATTERN-cwd-resolution), so it never imports internal/lymcwd.
//
// # Stamp and lock
//
// The build stamp, `.lym/config/build-stamp.json` in the Bolt checkout, holds a key for the running binary.
// A stamp naming the running key makes Ensure return without a lock or git;
// an absent or unreadable stamp counts as stale, as does a per-worktree module whose repo default is absent.
// A lock still held after Options' wait returns *LockTimeoutError and touches nothing.
//
// # The walk
//
// The walk pulls the board, reconciles and commits the board's config, then reconciles each code worktree the hub lists.
// It commits each worktree's applied files in that worktree.
// The board's push goes to the detached `lym board sync`, so no start verb waits on a push.
// A failed render of heddle targets or of the overview is logged and never fails the walk.
//
// The walk removes no key the running binary's template lacks, so an older binary never strips a newer one's settings.
// `lym config reconcile --apply` is the one way to remove such a key; a local override file is pruned of every one.
// Every non-seed-only base file stays sparse over the template (PATTERN-config-layers).
// A seed-only file follows its template changes and keeps every value the operator set.
//
// A failed step restores that step's config files to their prior bytes, so a retry rewrites them.
// Steps that already committed keep their commits.
//
// # Skips
//
// An incomplete pair, a worktree whose directory is gone and a mid-merge worktree are skipped.
// A mid-merge skip leaves the stamp absent, so the next start verb retries.
// The hub-wide write is skipped, leaving the stamp absent, when the board cannot be brought up to date.
// A board with no upstream is never skipped.
// A hub-wide commit that landed locally writes the stamp whether or not its push has landed.
//
// # Reinterpretations
//
// A binary that changes what a base-file value means registers a configreg.Reinterpretation.
// The walk applies each once per hub, guarded by the board marker `_lym/config/reinterpreted-<id>`.
// Once the commit landed, a `[reconcile notice]` line goes to the hub's orch through the InstallNotifier notifier.
// A machine-local override file is not rewritten, since the marker is shared by every machine of the hub.
//
// # The pair call
//
// Options.Pair reconciles that one pair unconditionally.
// It uses the same lock and mid-merge refusal, reads and writes no stamp, and commits only that pair's files.
//
// # Errors
//
// *WorktreeError names the worktree, the file and the cause, and ends with the way forward.
// EnsureOrWarn logs one Warn for a failure and lets the caller go on with the config as it was.
// The pair call and the clone's walk keep Ensure and its error.
package hubreconcile
