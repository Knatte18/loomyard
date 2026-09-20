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
// Every constructor in this package is a plain filepath.Join onto a told *lyxcwd.Location's
// AnchorPath(), per the Cwd Resolution Invariant: the package resolves no cwd of its own and spawns
// no git.
package shedrun
