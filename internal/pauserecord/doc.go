// Package pauserecord owns the pause record: one JSON file that every reader of a pause consults.
//
// The record holds the live entries and the recently resumed ones.
// An entry covers the whole hub (`ScopeHub`) or one pair's slug, with at most one live entry per scope.
// A hub entry covers every slug, and a slug entry covers that slug only.
//
// Update is the only writer; it takes a write lock and replaces the file atomically.
// Read, ReadCoverage and the Record methods are the readers.
// A missing file reads as the empty record.
// A reader that cannot decode the record logs it and reads uncovered, so a corrupt file never freezes the hub.
// Every reader is told a Geometry and derives no path itself.
//
// `BoltHoldCaller` is the `Caller` of the slug entry a machine writes for one of its runs while a Bolt hold stands.
// Such an entry names no resume, since the machine's dobby lifts it once the hold clears.
//
// SleepWatch detects machine sleep, and AwakeSince measures time net of sleeps and pauses,
// so every idle clock measures awake, unpaused time.
//
// WaitUncovered is the wait every held entry point shares; it has no deadline of its own.
// A caller whose run holds a parent lock is exempt from slug entries, so only a hub entry holds it.
//
// Network is the process gate that holds network calls while a pause covers the process.
// The CLI root installs a Gate once per process.
// The gate only waits and marks; it fails no call.
// With no gate installed, as in every test binary, Network does nothing and reports zero.
// Admit takes an admission before a lock other processes wait on, so a pause never holds the lock's holder.
// The exemptions (`Gate.ExemptAll`, SetGateExemptAll, SetGateExempt) never reach a child, which installs its own gate.
//
// The package imports the standard library and `internal/state` only.
// Update goes through `internal/state`,
// so a key of the file that the record types do not declare survives a rewrite by an older binary.
package pauserecord
