// Package reedcli is the reed CLI, the cobra layer over the tmux window manager whose domain lives in `internal/reedengine`'s package doc.
//
// The per-hub watchdog daemon (`lyx reed watchdog`) runs a discovery loop over the hub's tmux socket.
// Its cadence starts at five seconds, doubles up to sixty across cycles that find the session set unchanged, and returns to five seconds when the set changes.
// The loop also wakes on the hub-level discover signal, a file that a cold session boot touches: a consumed signal runs a cycle at once, but never sooner than five seconds after the previous cycle, so a process touching the file in a loop costs one cycle per five seconds.
// When the signal file cannot be watched the loop logs that once and keeps its backed-off cadence.
// Idle exit and orphan reap each count three consecutive cycles, so at the backed-off cadence they take up to three minutes.
package reedcli
