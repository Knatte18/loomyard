// Package fabrickit builds made-up fabriccore.Snapshot values for tests of fabric's rules and of what runs them.
// A snapshot holds fabric's own state only, each part as a value in fabric's vocabulary.
// Its parts are refs, worktrees, anchors, the records store, pending teardowns, outcomes and read failures.
// The kit stands in for no other module's data.
//
// It imports only the standard library, internal/fabriclayout and internal/fabriccore.
package fabrickit
