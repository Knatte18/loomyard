// doc.go carries the package-level doc comment for internal/kwinplace.

// Package kwinplace places a terminal window on a KDE Plasma desktop.
// It owns the KWin and kscreen boundary and the placement flow.
//
// It reaches KWin only by loading a generated script through qdbus6.
// A script returns nothing to its caller, so each script logs marker lines carrying a run id, ending with an end marker,
// and the package reads them from the user journal of the plasma-kwin_wayland.service unit.
//
// A window is placed only when the environment of the process that opens it passes the pre-launch checks.
// OpenWindow is the opener a caller uses, and it runs the checks first.
// A window that passes them opens in a process of its own; one that fails a check opens as it would without placement.
//
// Every script run holds a per-user script lock from before its load until its script has left KWin,
// so no two runs of this package overlap.
// A run records its plugin name beside the lock, and the next one unloads what a killed run left behind.
// The journal is the only channel back.
// A run reads the unit's lines from just before its load until its end marker shows.
//
// Where each worktree's window belongs is remembered in one places file per hub, a never-tracked cache.
// Every write is one locked read-modify-write, so concurrent placements keep each other's entries.
// An absent or undecodable file reads as empty, and the next write replaces an undecodable one.
//
// The planner is pure.
// From a snapshot, the remembered places and the mode, it chooses the desktop, the grid and the window order.
// A window goes back to its home desktop; one without a home is counted from its parent window's desktop.
// A home that is no longer a desktop is forgotten once the window is placed,
// and the window takes the desktop it lands on as its home unless it was placed on a stand-in anchor.
// A full desktop is never a target, and only the target desktop's windows are rearranged.
//
// A placement records its window, waits for it to appear, and then waits for its parent window's place.
// It then takes the placement lock, kwinplace.lock beside the script lock.
// Under that lock it queries, plans, applies and records.
// A process holding the placement lock takes the script lock inside it for each run, never the other way round.
// Both waits happen outside the placement lock, so a placement never holds it while waiting on another.
// Every way a placement ends settles its window's entry, unless a newer window of the worktree has taken it.
// All but a cancel run the caller's command afterwards.
//
// A placement runs in a detached process of its own, which Spawn starts,
// so the verb that opened the window returns at once.
package kwinplace
