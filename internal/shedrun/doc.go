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
// The seed carries an optional, write-once parent: the full agent name of the session that spawned the run.
// `lyx batten run` records the caller's own LYX_STRAND_NAME when it arms the seed, batten's Seed-Child copies that parent into the child run's seed,
// and `lyx loom start` records its caller the same way.
// A later write never replaces a recorded parent, and a seed written before the field existed reads back with it empty.
// Reed tells the parent to every strand in the worktree as LYX_PARENT; see the Agent Name Invariant in CONSTRAINTS.md.
//
// RunsRootRel names the anchor-relative run-records root, because fabricengine's Add drops everything under it from a freshly forked pair.
//
// Every constructor in this package is a plain filepath.Join onto a told *lyxcwd.Location's
// AnchorPath(), per the Cwd Resolution Invariant: the package resolves no cwd of its own and spawns
// no git.
package shedrun
