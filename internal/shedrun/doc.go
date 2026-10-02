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
// The seed's optional `parent` field is legacy: no code path writes it any more, and a seed written before that change still decodes.
// Only `internal/hubgeom`'s parent resolver reads it, as a fallback when the pair's origin record names no parent worktree.
// A run's parent is otherwise resolved at use from that origin record; see the Agent Name Invariant in CONSTRAINTS.md.
//
// RunsRootRel names the anchor-relative run-records root, because fabricengine's Add drops everything under it from a freshly forked pair.
//
// Every constructor in this package is a plain filepath.Join onto a told *lyxcwd.Location's
// AnchorPath(), per the Cwd Resolution Invariant: the package resolves no cwd of its own and spawns
// no git.
package shedrun
