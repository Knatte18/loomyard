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
// A seed written when it carried a top-level `parent` key still decodes; the key is discarded and never re-encoded.
// A run's parent is resolved at use from the pair's origin record alone; see PATTERN-agent-name.
//
// The ephemeral run-directory contents include the loop's files, all under StepsDir:
// the loop lock, pid file and log, the envelope the waiter prints and the per-loop-id slot it is retired into, and each step's trace copy and stderr file.
// LoopJobName names the loop's Windows job object from a hash of the steps directory, so it is unique across hubs.
//
// RunsRootRel names the anchor-relative run-records root, because fabricengine's Add drops everything under it from a freshly forked pair.
//
// Every constructor in this package is a plain filepath.Join onto a told *lyxcwd.Location's
// AnchorPath(), per the Cwd Resolution Invariant: the package resolves no cwd of its own and spawns
// no git.
package shedrun
