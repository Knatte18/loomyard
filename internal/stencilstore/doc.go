// doc.go carries the package doc comment for stencilstore.

// Package stencilstore owns the entire stencil lifecycle -- seeding, hash-stamping, edit detection,
// reading, and validation -- against a caller-supplied absolute stencils directory.
// It never derives that directory's geometry itself: baseDir always arrives fully resolved from the
// caller (fabricengine.StencilsDir), which is what keeps this package's tests hermetic against a
// bare t.TempDir() and keeps it free of the _board/_lyx literals those packages own.
// A Mode sets how a pass treats an untouched board copy whose shipped default has changed.
// ModeProduction, chosen by ModeFor only for a production-stamped binary with a clean VCS stamp, refreshes it forward only:
// when the copy records no writer, or when the caller's Older ordering reports the recorded writer older than the running binary.
// ModeDev and ModeUnstamped (the zero Mode) seed absent files but warn instead of refreshing, and a forced refresh (ForceRefresh) overrides every mode.
// A write records the running binary as the writer only under ModeProduction, as `build=<revision>` and `time=<RFC 3339>` keys after the hash in the banner's stamp line (ApplyWriter, ParseWriter), so a write from any other mode lets the next production binary refresh the copy once.
// WritesDue runs the same classification without writing or logging, so a caller can tell whether a pass has anything to commit.
// A pass logs one line per notice case, each listing the stencils it left untouched and naming its remedy once, and one line for the stencils whose board copy drifted from the worktree source.
// The drift line lists each differing stencil with its class and logs at Info only when every differing stencil is in the behind class.
// Reconcile runs once per process, so each of those lines appears once per process.
// The drift warning's build-ancestry input arrives the same way, told by the caller through Source, because stencilstore runs no git.
package stencilstore
