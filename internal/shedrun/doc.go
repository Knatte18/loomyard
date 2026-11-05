// Package shedrun is the sole declarer of the "shed" run-directory path segment, of the run-id
// vocabulary (including the reserved literal "self"), and of the seed.json contract: the file that
// records a run's chosen recipe, driver, and parameters at seed time.
//
// The recipe names a seed may hold are batten, darn, motif, pawl, raddle, sampler and tapestry (RecipeNames).
// Motif, the recipe that makes a sketch, the decision record, runs alone and is also the tapestry recipe's Motif block.
//
// "Seed" here names one thing only -- the run-identity artefact this package reads and writes via
// ReadSeed/WriteSeed. It is never shedverbs.KindUnseeded's sense (a status file); a run addressed by this
// package's seed.json is a different concern and this package does not touch a status file.
//
// The literal "self" is an alias for the worktree's own slug: ResolveRunID maps it to the told
// location's WorktreeName, and a run's directory is named by that slug.
// When _lym/shed/<slug>/ is absent and _lym/shed/self/ exists, both spellings join the legacy "self"
// directory, so a run started before the rename keeps working with no on-disk migration.
//
// WriteSeed stamps `started_at`, the RFC 3339 UTC time the run was seeded, when the seed has none and keeps a given one;
// a re-written agreeing seed keeps its original stamp, and a seed written before the field existed has none.
//
// A seed decodes with any key this binary does not know, the retired `parent` key included, and an agreeing WriteSeed leaves the file as it is.
// A run's parent is resolved at use from the pair's origin record alone; see internal/agentname's package doc.
//
// The ephemeral run-directory contents include the loop's files, all under StepsDir:
// the loop lock, pid file and log, the envelope the waiter prints and the per-loop-id slot it is retired into, and each step's trace copy and stderr file.
// LoopJobName names the loop's Windows job object from a hash of the steps directory, so it is unique across hubs.
//
// The arbiter's paths are declared here too.
// ArbiterDir is the durable directory of a run's arbiter attempts, and ArbiterRel its anchor-relative form.
// ArbiterAttemptDir is one attempt's directory (attempt-<N>, 1-based).
// ArbiterMarker is the ephemeral in-flight marker under ScratchDir.
//
// The Go driver and its tackler have paths here too.
// DriverLock is the ephemeral Go-driver lock under ScratchDir, distinct from the run lock and the status lock.
// TacklerDir is the durable directory of a run's tackler attempts, TacklerRel its anchor-relative form and TacklerAttemptDir one attempt's directory (attempt-<N>, 1-based).
// TacklerMarker is the ephemeral in-flight marker under ScratchDir.
// DriverRecordsRel lists the anchor-relative drive-reports, arbiter and tackler directories that hold a file, the driver records every recipe's commit of a run's records stages.
// ChildRecipeFile is the durable record under RunDir of the recipe a batten run seeded its task worktree's run with, which WriteChildRecipe writes and ReadChildRecipe reads, empty when none is recorded;
// it outlives the task worktree's own seed, so a done run still tells which recipe its child ran.
// LandedCommitFile is the ephemeral file under ScratchDir holding the commit a run's landing put on its parent branch, which WriteLandedCommit writes and ReadLandedCommit reads, empty when none is recorded.
// ParentNoticeFile is the ephemeral file under ScratchDir holding the key of the parent-branch notice a run's pre-push check last posted.
// CancelMarker is the ephemeral marker `lym batten cancel` writes, and CancelMarkerLock the lock whose holder is its live writer.
// Both sit directly under ScratchRoot beside the per-slug scratch directories, never inside RunDir or ScratchDir, so a slug's records removal leaves them.
// Batten's verbs read the marker.
// SettleRecords is the Go driver's settle-record file under a run's directory, beside its drive reports.
// The settle-record format is declared here too: SettleRecord, StopKey, the settler and action names, and ParseSettleRecords, the tolerant decoder that skips a line that does not decode and reports it by line number.
// The Go driver appends and reads the file.
//
// The batten-watched marker's format is declared here too.
// BattenWatchedContent renders the pid on the first line and, when known, the prime's batten scratch directory and the Run-Shed row's name on the next two.
// ParseBattenWatched reads it back, and a one-line marker parses with an empty scratch directory and row.
//
// WakeRecord is the ephemeral wake record under ScratchDir, which WriteWakeStamp writes after a row completes a wake and ReadWakeStamp reads back by row, found only for the row that wrote it.
// The Go driver reads it to tell a loop that lived through a wake from one that kept dying.
//
// RunsRootRel names the anchor-relative run-records root, so a caller that needs the whole run-records tree need not name the shed segment itself.
// RunsRoot and ScratchRoot are its absolute durable and ephemeral forms, the directories every run directory and scratch directory sit under.
// RunDirRel is a run directory's anchor-relative form, the pathspec of a records commit that deletes the whole directory.
//
// TreeRunFileRels names a run's seed.json and status.json as slash-separated paths relative to a git tree, with no alias and no legacy fallback.
// ParseSeed decodes seed bytes a caller read elsewhere, as ReadSeed does.
//
// The usage directory is declared here too.
// UsageDirRel names the anchor-relative durable directory _lym/usage, and UsageDir its absolute form.
// It holds three kinds of record file: session-<session id>.json, a SessionUsageRecord per agent session that WriteSessionUsage writes and a later write of the same id replaces;
// run.json, a RunUsageRecord holding the run's recipe, profile and stored-record flag, which WriteRunUsage writes;
// and landed.json, a LandedSizeRecord holding the landed diff's size, which WriteLandedSize writes.
// ParseRunUsage is the one reader of the directory's files, given as a name-to-bytes map.
// Every record is written through internal/state and read tolerantly, so an unknown key is kept and never refused.
//
// Every constructor that returns an absolute path joins onto a told *lymcwd.Location's AnchorPath(), per the Cwd Resolution Invariant;
// the *Rel constructors and TreeRunFileRels return relative paths, and no constructor resolves a cwd or spawns git.
package shedrun
