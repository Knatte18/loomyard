// Package shedrun is the sole declarer of the "shed" run-directory path segment, of the run-id
// vocabulary (including the reserved literal "self"), and of the seed.json contract: the file that
// records a run's chosen recipe, driver, and parameters at seed time.
//
// "Seed" here names one thing only -- the run-identity artefact this package reads and writes via
// ReadSeed/WriteSeed. It is never internal/loomengine's CheckSeed sense (the initial status.json for
// a fresh run) and never shedverbs.KindUnseeded's sense (also a status file); a run addressed by this
// package's seed.json is a different concern from either of those and this package does not touch
// either of them.
//
// The literal "self" is an alias for the worktree's own slug: ResolveRunID maps it to the told
// location's WorktreeName, and a run's directory is named by that slug.
// When _lyx/shed/<slug>/ is absent and _lyx/shed/self/ exists, both spellings join the legacy "self"
// directory, so a run started before the rename keeps working with no on-disk migration.
//
// WriteSeed stamps `started_at`, the RFC 3339 UTC time the run was seeded, when the seed has none and keeps a given one;
// a re-written agreeing seed keeps its original stamp, and a seed written before the field existed has none.
//
// A seed decodes with any key this binary does not know, the retired `parent` key included, and an agreeing WriteSeed leaves the file as it is.
// A run's parent is resolved at use from the pair's origin record alone; see PATTERN-agent-name.
//
// The ephemeral run-directory contents include the loop's files, all under StepsDir:
// the loop lock, pid file and log, the envelope the waiter prints and the per-loop-id slot it is retired into, and each step's trace copy and stderr file.
// LoopJobName names the loop's Windows job object from a hash of the steps directory, so it is unique across hubs.
//
// The arbiter's paths are declared here too.
// ArbiterDir is the durable directory of a run's arbiter attempts, and ArbiterRel its anchor-relative form.
// ArbiterAttemptDir is one attempt's directory (attempt-<N>, 1-based).
// ArbiterMarker is the ephemeral in-flight marker under ScratchDir.
// AlignmentDir is the durable directory of a run's alignment attempts at one point, and AlignmentRel the anchor-relative form of the directory holding every point's.
//
// The Go driver and its tackler have paths here too.
// DriverLock is the ephemeral Go-driver lock under ScratchDir, distinct from the run lock and the status lock.
// TacklerDir is the durable directory of a run's tackler attempts, TacklerRel its anchor-relative form and TacklerAttemptDir one attempt's directory (attempt-<N>, 1-based).
// TacklerMarker is the ephemeral in-flight marker under ScratchDir.
// SettleRecords is the Go driver's settle-record file under a run's directory, beside its drive reports.
//
// The batten-watched marker's format is declared here too.
// BattenWatchedContent renders the pid on the first line and, when known, the prime's batten scratch directory and the Run-Shed row's name on the next two.
// ParseBattenWatched reads it back, and a one-line marker parses with an empty scratch directory and row.
//
// RunsRootRel names the anchor-relative run-records root, because fabricengine's Add drops everything under it from a freshly forked pair.
// RunsRoot and ScratchRoot are its absolute durable and ephemeral forms, the directories every run directory and scratch directory sit under.
// RunDirRel is a run directory's anchor-relative form, the pathspec of a records commit that deletes the whole directory.
//
// TreeRunFileRels names a run's seed.json and status.json as slash-separated paths relative to a git tree, with no alias and no legacy fallback.
// ParseSeed decodes seed bytes a caller read elsewhere, as ReadSeed does.
//
// Every constructor that returns an absolute path joins onto a told *lyxcwd.Location's AnchorPath(), per the Cwd Resolution Invariant;
// the *Rel constructors and TreeRunFileRels return relative paths, and no constructor resolves a cwd or spawns git.
package shedrun
