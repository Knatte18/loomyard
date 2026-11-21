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
// Only under ModeProduction does a write record the running binary as the writer,
// so a write from any other mode lets the next production binary refresh the copy once.
// WritesDue runs the same classification without writing or logging.
// The caller tells the drift warning its build-ancestry input through Source, because stencilstore runs no git.
//
// A process can outlive the binary that wrote the board copies it reads:
// a production refresh by a newer binary rewrites them under a running driver.
// The embedding package registers its defaults through RegisterShipped.
// The caller registers its writer ordering through RegisterOrdering, normally the same function as Source's Older.
// After that, a read of a board copy that a newer binary wrote is a NewerBoardCopyError.
// A copy whose body equals its shipped default is read as it stands.
// The error names the restart of the run's driver on the current binary as the way forward.
// An edited copy is refused the same as an untouched one, and a refusal never writes the board.
// Without a registered ordering, newer is ordered by the writer's and the running binary's commit times.
//
// Pins hashes each installed board copy's body, so a caller can record what a run may render when it starts.
// ChangedPins reports the pinned stencils whose board copy changed or is gone since.
//
// # Engine blocks
//
// An engine block is an include whose text differs by the engine a stencil is rendered for.
// A stencil includes it as the bare {{template "engine-<block>"}}.
// Each engine keeps its own copy at `engine/<engine>/<block>.md` under the stencils directory.
// A copy is read without its file's final newline, so a block can stand inside a line as well as on lines of its own.
// Every caller that renders a stencil for a launch reads its blocks for that launch's engine.
// It does so through ReadSet or ReadForEngine.
// An engine with no copy of an included block is a MissingEngineBlockError naming the block, the engine and the file,
// so a caller can tell that refusal apart from any other read error.
package stencilstore
