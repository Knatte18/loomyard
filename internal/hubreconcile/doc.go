// Package hubreconcile reconciles a hub's config once after a binary change, under a hub lock.
// It commits everything it wrote.
// Its one entry point is Ensure; a start verb calls it, through EnsureOrWarn, before loading any module config.
//
// The package is told the hub's geometry and derives none (PATTERN-cwd-resolution), so it never imports internal/lymcwd.
//
// # Stamp, lock and key
//
// The build stamp is the Bolt checkout's .lym/config/build-stamp.json.
// The hub lock is its .lym/config/reconcile.lock.
// Both are in the never-tracked mirror of the hub-wide config dir.
// The stamp holds a build key:
// the hex SHA-256 of the binary's VCS revision, its modified flag and the config registry's fingerprint.
// The revision and the flag are also in readable form for the log.
// A stamp naming the running key makes Ensure return after one file read.
// It also costs one stat per per-worktree module and one for the board marker.
// It takes no lock and runs no git; an absent or unreadable stamp counts as stale.
// A stamp naming the running key also counts as stale when a per-worktree module's repo default is absent.
// The check runs in the Bolt checkout.
// A default deleted after a walk is therefore reseeded by the next start verb.
// It counts as stale as well while the board marker `_lym/config/base-sparse` in the Bolt checkout is absent.
// A board not yet stripped to sparse base files is therefore walked.
// A marker removed by hand with a board commit makes the next start verb's walk strip the base files again.
// The lock wait defaults to DefaultLockWait and is an Options field;
// a lock still held when it runs out returns *LockTimeoutError and touches nothing.
// The holder's own per-worktree commits wait on each pair's write lock as a plain commit does, unbounded.
//
// # The walk
//
// Under the lock the stamp is read again, and a fresh one returns nil.
// The walk pulls the board first.
// It reconciles the board's config and commits it on the board under a subject carrying the binary label.
// It seeds every absent per-worktree repo default from the prime or the template.
// It then reconciles the prime's sparse layers.
// It lists the hub's code worktrees again and reconciles each pair not yet walked.
// It repeats until a listing finds no new pair.
// A pair registered while the walk ran is therefore walked before the stamp is written.
// The board commit carries the per-worktree repo defaults beside the hub-wide files.
// Each worktree's applied files are committed in that worktree.
// The board's push goes to the detached `lym board sync` every board write spawns.
// That sync carries the board commit and any an earlier run left unpushed.
// The walk spawns it and returns at once.
// No start verb waits on a push to GitHub, and the build stamp never waits on one either.
//
// # Keys the template lacks
//
// The walk removes no key the running binary's template lacks unless a retired key of its module covers it.
// Such a key, as a newer binary writes, stays in the board's base files and in each worktree file.
// It is carried in the result as preserved and logged when the walk writes the file,
// so a start verb run by an older binary never strips a newer one's settings from the hub.
// `lym config reconcile --apply`, with `--layer default` for a board file, is the one way to remove it.
// A local override file is pruned of every such key, since its load refuses one.
//
// A seed-only file follows its template: the walk runs its module's migration.
// It then carries each template change into the file against the template base kept beside it on the board.
// That base is `<module>.template-base`, and the walk commits it with the file.
// Every value the operator set is kept.
// A key the file holds as the base does takes the template's new value, gained, changed or dropped.
// A key the operator set, added or removed stays; a file with no base yet gains only the keys it lacks.
// Once the board commit landed, one `[reconcile notice]` line goes to the hub's orch.
// It names what each file took and each operator value the template holds otherwise.
// A pair the module leaves for the operator to settle is logged in one Warn per module.
// The Warn names `lym config <module>` as the way forward.
//
// # The base strip
//
// Every non-seed-only base file on the board is sparse over the template.
// While the board marker is absent, the walk strips each present base file once.
// It strips the units equal to the template or to a former template value.
// It writes a base left with no unit as the header-only file.
// It writes the marker only when every registered module's base file exists, after the files.
// It commits files and marker in the one board commit.
// A pass that leaves a base absent withholds the marker and the next walk strips again.
// A failed hub-wide step restores the board's files to their prior bytes and removes a marker the pass wrote.
// Steps that committed before it keep their commits.
// With the marker present the walk strips no base, so a unit later set equal to the template stays as a pin.
//
// A base file is a hub-wide module's hub file, or a per-worktree module's repo default.
// A seed-only module's board file is excepted.
// A seed-only module's board file takes each template change on every walk, against the template base kept beside it.
// It keeps every value the operator set.
// Every write to a base file, by any package, holds only the units the hub tuned (PATTERN-config-layers).
// It never fills in a template value.
// Only this walk's one-time strip drops a unit the template holds, behind the board marker.
// lenses' template-equal entries are the one exception:
// a per-name entry equal to the template is dropped on every reconcile.
//
// # The sparse migration
//
// A worktree without the sparse marker `_lym/config/sparse` holds full config files.
// Once every per-worktree module's repo default exists, the walk strips each such file.
// It strips the keys that equal what the layers below it resolve to, and the keys that equal a former template value.
// Every key the file held then resolves unchanged, bar a former value, which resolves to the new template.
// A file left empty is deleted, and the marker is written after the files.
// The files, deletions and marker go into the worktree's one reconcile commit.
// The log line of a changed module lists the keys it stripped and the keys it inherited.
// An inherited key is one of the repo default's keys that the file lacked and whose value differs from the template's.
// The operator can pin it with `lym config <module> --set`.
// A worktree carrying the marker is never stripped, and a pair forked from a migrated worktree inherits the marker.
// A pair forked before the migration is stripped by the next walk.
// So is a pair skipped as being created or mid-merge, or left by a walk that died partway.
// A failed or merge-interrupted walk restores the marker with the files.
// A migrated pair may meet the landing warning for a changed tracked config file once, in the commit that strips it.
// `lym config reconcile` reports the strip as pending and never performs it.
// Worktrees that committed before a failure keep their commits.
// The failing worktree's, or the board's, config files are restored to their prior bytes.
// A retry therefore rewrites and commits them instead of finding them already current.
// A worktree commit that landed before a later step of it failed keeps the written files.
// Recording the commit's correspondence is such a later step.
// A config file that exists but cannot be read fails that worktree before anything in it is written.
//
// # Skips
//
// A pair that is not complete is skipped before anything is opened or written.
// That covers a pair still being created and one a killed creation left partway.
// That skip leaves the stamp alone: a pair being created is reconciled by its creator's single-pair call,
// and a partial pair never keeps every later start verb walking.
// A worktree whose directory is gone is skipped as removed.
// A worktree that is mid-merge is skipped as well.
// It is not written when the merge state is seen up front.
// A merge that begins between the write and the commit restores the files to their prior bytes.
// A mid-merge skip leaves the stamp absent, so the next start verb retries.
//
// The hub-wide write is skipped, with nothing written, when the board cannot be brought up to date.
// That happens when the fetch failed or an uncommitted change blocks it.
// It also happens when the board's own commits cannot be replayed onto a moved upstream.
// That skip leaves the stamp absent as well, on every rerun until the board is brought up to date.
// A hub-wide commit that landed locally writes the stamp whether or not its push has landed.
// A push origin refuses reaches the orch as a failed-push record, not as an absent stamp.
// A rerun whose unpushed hub-wide commit sits over a moved upstream is cleared by the board's pull-first replay.
// The replay moves the commit onto the moved upstream, so the hub-wide write and the stamp follow.
// Only a conflict the replay cannot settle, an upstream that changed the same config file, meets the divergence skip.
// It repeats its Warn on every rerun.
// The Warn names the conflicting paths and the two ways to settle them.
// A board push that lands the commit clears it too.
// A board with no upstream is never skipped: it is written and committed with no pull.
// Its detached sync fails on its own, so a board with no reachable remote is not re-walked on every start verb.
//
// # Reinterpretations
//
// A binary that changes what a value in a module's hub base file means registers a configreg.Reinterpretation.
// In the board write, after the hub-wide reconcile, the walk applies each reinterpretation.
// It skips one whose board marker `_lym/config/reinterpreted-<id>` is present.
// It hands the module's base file to the rewrite with the prime's root and anchored directory.
// It writes the file when the rewrite changed it, and writes the marker after it.
// Each reinterpretation therefore applies once per hub, and a later binary never applies it again.
// An absent base file is only marked.
// The file and the marker join the board commit, and a failed write restores both with the board's other config files.
// Once the commit landed, one `[reconcile notice]` line names each rewritten and each flagged value.
// It goes to the hub's orch through the notifier the process installed with InstallNotifier, and to the log at Warn.
// With none installed, or no orch recorded, the log alone holds it.
// A walk runs before a start verb loads any config.
// A reinterpreted file is therefore carried across before the new binary runs a command from it.
// A machine-local override file is not rewritten.
// The board marker is shared by every machine of the hub, and the override is not.
//
// # The pair call
//
// Options.Pair reconciles that one pair unconditionally, whatever the stamp says.
// It uses the same lock and the same mid-merge refusal.
// It reads and writes no stamp, and only that pair's config files are committed.
//
// # Errors
//
// *WorktreeError names the worktree, the file and the cause of a reconcile or commit failure.
// It ends with the way forward:
// `lym config <module> --layer default` from the prime worktree for a board file.
// For a worktree file it is `lym config <module>`, and for the board marker only the file and the re-run.
// *LockTimeoutError names the lock.
// EnsureOrWarn is Ensure for the stamp-gated reads, the start verbs and the Preflight row.
// A failure logs one Warn naming the error and the way forward, and the caller goes on with the config as it was.
// The failing step's files are restored and no stamp is written, so the next start verb retries.
// Steps that committed before the failure keep their commits.
// The pair call and the clone's walk keep Ensure and its error.
//
// # Accepted gaps
//
// A fabric add whose fork read the prime before a walk committed it keeps the prime's older config.
// This happens when its pair became enumerable only after the walk's last listing.
// Preflight's batcher.yaml load, which refuses a retired key, backs that up.
// Two risks are accepted: a downgrade flip-flops the stamp between two binaries,
// and a lym-authored records commit in a running pair can land beside a driver's own work.
package hubreconcile
