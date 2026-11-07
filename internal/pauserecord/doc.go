// Package pauserecord owns the pause record: one JSON file that every reader of a pause consults.
//
// The record holds the live entries and the recently resumed ones.
// An entry covers the whole hub (`ScopeHub`) or one pair's slug, and there is at most one live entry per scope.
// A hub entry covers every slug, and a slug entry covers that slug only.
//
// Update is the only writer.
// It takes a write lock beside the file and runs the caller's mutation.
// It drops resumed entries older than `Retention` and replaces the file atomically.
// Read, ReadCoverage and the Record methods are the readers.
// A missing file reads as the empty record, so a hub that never paused holds nothing.
//
// Every reader is told a Geometry and derives no path itself;
// only `internal/hubgeom` and `internal/standalonegeom` construct it.
// A reader that cannot decode the record logs it and reads uncovered, so a corrupt file never freezes the hub.
//
// `OwnerCaller` is the fixed `Caller` of every pause entry that holds a run owned by another machine,
// so its lifter and a running loop tell those entries apart from any other.
// `Coverage.OwnerHeld` is set when a live entry of the slug's own scope carries it.
// It is set whichever entry is the oldest covering one.
// `HoldCaller` is the fixed `Caller` of every pause entry that holds a run or a released task `lym adopt` took over.
// Such an entry holds until its subject's adopt check is green.
// `Coverage.Held` is set when a live entry of the slug's own scope carries it.
// A dobby's owner-pause lift and the hibernate wake judge only `OwnerCaller`.
// Neither treats a hold-caller entry as theirs.
// LatestResumed finds the resumed entry covering a slug that resumed last,
// so a reader that was away can tell a pause it never saw restarted what it measures.
// ClaimSession records one session in the oldest live entry covering a slug.
// That entry's resume gives the session its line.
// An uncovered slug and a session a live entry already owns record nothing.
//
// Excluded measures how much of a time span the covered intervals took, so overlapping pauses are subtracted once.
// CoveredError is the refusal every held entry point returns.
// `ResumeLine` is the one line a resumed session is given.
//
// SleepWatch detects machine sleep: each reading pairs a wall time with an awake clock.
// The wall delta in excess of the awake delta, past `SleepTolerance`, is a sleep ending at that reading.
// AwakeSince returns the time since an activity net of the union of the sleeps and pauses after it,
// so every idle clock measures awake, unpaused time.
// `MonotonicAwake` is the default awake clock; it stops during suspend on Linux and macOS and keeps running on Windows.
//
// WaitUncovered is the wait every held entry point shares.
// It returns once no live entry covers the slug.
// It polls from one second and doubles to five while the covering entry is unchanged.
// It writes one line when the wait starts and has no deadline of its own.
// A caller whose run holds a parent lock is exempt from slug entries, so only a hub entry holds it.
//
// Network is the process gate that holds network calls.
// The CLI root installs a Gate once per process, and a call waits at entry while a pause covers the process.
// It then writes a marker under `NetDir` that its returned function removes.
// ListNetOps lists the markers of live processes, which are the operations in flight.
// An operation named remote-refs marks a lock hold.
// The hold lasts from before the lock is taken until its release, and it covers the git calls and retries made under it.
// Network returns the time it held the call, zero unless the wait slept,
// so a caller can move its own deadline later by that time.
// Network takes only the operation name and no context.
// With no gate installed, as in every test binary, Network does nothing and reports zero.
// The gate only waits and marks; it fails no call.
// While a call waits, it keeps a `WaitingMarker` (pid, start time and slug) under `WaitingDir`.
// `WaitingDir` is a sibling of `NetDir` that `ListNetOps` never reads.
// The marker is removed as soon as the wait returns.
// `ListWaiting` lists the markers of live processes and removes the rest.
// The marker records only what the gate waits on, and no gate decision reads it.
// Admit passes the gate as Network does and admits the process until its release runs.
// While an admission stands, every network call of the process passes at once and stays marked in flight.
// It does so whichever goroutine or repository makes the call.
// A caller takes it before a lock other processes wait on, so a pause never holds the lock's holder.
// SetGateExemptAll sets `Gate.ExemptAll` on the installed gate, so its waiter lets every live entry pass.
// It serves the one process an explicit operator verb runs in.
// It does nothing with no gate installed, and the returned function restores the previous value.
// The exemption never reaches a child, which installs its own gate.
// GateExemptsAll reports whether the exemption is set, so a parent can hand it to a child in the child's own input.
// Only a detached push or board-sync child installs a gate with `Defer` set.
// When `GateCovered` reads a live entry covering the process, such a gate hands off before any wait.
// It hands off to the function `SetGateDeferral` stored, which is the child's exit and does not return.
// With none stored, or one that returns, the gate waits as before, so it still fails no call.
// `GateCovered` applies the exemptions the gate's waits apply,
// and it reads false with no gate installed or an unreadable record.
// Only a push child spawned under its slug's parent lock installs a gate with `OwnerEntryExempt` set.
// Such a gate lets a live entry of its own slug whose `Caller` is `OwnerCaller` pass.
// A hub entry, and an operator's entry of its own slug, still hold it.
// SetGateExempt names one live entry, by scope and pause time, that the installed gate lets pass.
// It serves the process that owns that entry.
// Any other entry still holds the call, and the returned function restores the previous exemption.
//
// The package imports the standard library and `internal/state` only.
// Update goes through `internal/state`,
// so a key of the file that the record types do not declare survives a rewrite by an older binary.
package pauserecord
