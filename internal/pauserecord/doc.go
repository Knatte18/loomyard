// Package pauserecord owns the pause record: one JSON file that every reader of a pause consults.
//
// The record holds the live entries and the recently resumed ones.
// An entry covers the whole hub (`ScopeHub`) or one pair's slug, and there is at most one live entry per scope.
// A hub entry covers every slug, and a slug entry covers that slug only.
//
// Update is the only writer: it takes a write lock beside the file, runs the caller's mutation, drops resumed entries older than `Retention` and replaces the file atomically.
// Read, ReadCoverage and the Record methods are the readers.
// A missing file reads as the empty record, so a hub that never paused holds nothing.
//
// Every reader is told a Geometry and derives no path itself; only `internal/hubgeom` and `internal/standalonegeom` construct it.
// A reader that cannot decode the record logs it and reads uncovered, so a corrupt file never freezes the hub.
//
// Excluded measures how much of a time span the covered intervals took, so overlapping pauses are subtracted once.
// CoveredError is the refusal every held entry point returns, and `ResumeLine` is the one line a resumed session is given.
//
// SleepWatch detects machine sleep: each reading pairs a wall time with an awake clock, and the wall delta in excess of the awake delta, past `SleepTolerance`, is a sleep ending at that reading.
// AwakeSince returns the time since an activity net of the union of the sleeps and pauses after it, so every idle clock measures awake, unpaused time.
// `MonotonicAwake` is the default awake clock; it stops during suspend on Linux and macOS and keeps running on Windows.
//
// WaitUncovered is the wait every held entry point shares: it returns once no live entry covers the slug, polling from one second and doubling to five while the covering entry is unchanged.
// It writes one line when the wait starts and has no deadline of its own.
// A caller whose run holds a parent lock is exempt from slug entries, so only a hub entry holds it.
//
// Network is the process gate that holds network calls: the CLI root installs a Gate once per process, and a call waits at entry while a pause covers the process, then writes a marker under `NetDir` that its returned function removes.
// ListNetOps lists the markers of live processes, which are the operations in flight.
// With no gate installed, as in every test binary, Network does nothing.
// The gate only waits and marks; it fails no call.
//
// The package imports the standard library and `internal/state` only.
// Update goes through `internal/state`, so a key of the file that the record types do not declare survives a rewrite by an older binary.
package pauserecord
