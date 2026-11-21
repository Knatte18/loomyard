// Package shedrun is the sole declarer of the "shed" run-directory path segment.
// It also declares the run-id vocabulary, including the reserved literal "self".
// It declares the seed.json contract too.
// That file records a run's chosen recipe, driver, and parameters at seed time.
//
// The recipe names a seed may hold are the closed vocabulary RecipeNames returns.
//
// "Seed" here names one thing only: the run-identity artefact this package reads and writes via ReadSeed and WriteSeed.
// It is not the status file; this package does not touch a status file.
//
// The literal "self" is an alias for the worktree's own slug:
// ResolveRunID maps it to the told location's WorktreeName, and a run's directory is named by that slug.
//
// WriteSeed stamps `started_at`, the RFC 3339 UTC time the run was seeded, when the seed has none.
// It keeps a given stamp, so a re-written agreeing seed keeps its original stamp.
//
// A seed decodes with any key this binary does not know, and an agreeing WriteSeed leaves the file as it is.
// A run's parent is resolved at use from the pair's origin record alone; see internal/agentname's package doc.
//
// The package also declares the paths and record formats of a run's durable and ephemeral files.
// These cover the loop's files, the arbiter, the Go driver and its tackler and the settle records.
// They also cover the batten-watched marker, the stencil pins, the wake and row-notice records and the usage directory.
// Each has its own doc comment.
// Every record is written through internal/state and read tolerantly, so an unknown key is kept and never refused.
//
// Every constructor that returns an absolute path joins onto a told *lymcwd.Location's AnchorPath().
// This follows the Cwd Resolution Invariant.
// The *Rel constructors and TreeRunFileRels return relative paths, and no constructor resolves a cwd or spawns git.
package shedrun
