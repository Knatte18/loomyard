// doc.go carries the package doc comment for stencilstore.

// Package stencilstore owns the entire stencil lifecycle against a caller-supplied absolute stencils directory:
// seeding, hash-stamping, edit detection, reading, and validation.
// It never derives that directory's geometry itself: baseDir always arrives fully resolved from the caller.
// That keeps this package's tests hermetic against a bare t.TempDir().
// It also keeps the package free of the Bolt checkout and _lym literals that the geometry packages own.
//
// A Mode sets how a pass treats an untouched board copy whose shipped default has changed.
// ModeFor chooses ModeProduction only for a production-stamped binary with a clean VCS stamp.
// ModeProduction refreshes a copy only in two cases:
// it records no writer, or the caller's Older ordering reports the recorded writer older than the running binary.
// ModeDev and ModeUnstamped (the zero Mode) seed absent files but warn instead of refreshing,
// and a forced refresh (ForceRefresh) overrides every mode.
// Only under ModeProduction does a write record the running binary as the writer.
// The record is the `build=<revision>` and `time=<RFC 3339>` keys after the hash in the banner's stamp line.
// ApplyWriter and ParseWriter write and read them.
// A write from any other mode therefore lets the next production binary refresh the copy once.
// WritesDue runs the same classification without writing or logging.
// A caller can tell from it whether a pass has anything to commit.
//
// A pass logs one line per notice case.
// Each lists the stencils it left untouched and names its remedy once.
// A further line covers the stencils whose board copy drifted from the worktree source.
// The drift line lists each differing stencil with its class.
// It logs at Info only when every differing stencil is in the behind class.
// Reconcile runs once per process, so each of those lines appears once per process.
// The caller tells the drift warning its build-ancestry input through Source, because stencilstore runs no git.
//
// # Engine blocks
//
// An engine block is an include whose text differs by the engine a stencil is rendered for.
// A stencil includes it as the bare {{template "engine-<block>"}}.
// Each engine keeps its own copy at `engine/<engine>/<block>.md` under the stencils directory.
// The copy is registered under the name EngineBlockName returns.
// ReadSet is the one resolver.
// It reads a stencil with every block it includes, an engine block from the copy of the engine it is told.
// Every caller that renders a stencil for a seat therefore reads its blocks for that seat's engine.
// An engine with no copy of an included block is a MissingEngineBlockError naming the block, the engine and the file,
// so a caller can tell that refusal apart from any other read error.
package stencilstore
