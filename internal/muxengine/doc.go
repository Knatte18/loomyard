// doc.go carries the package-level godoc comment for muxengine. It holds no
// code — its only job is documenting the package's role and contract in one
// place a reader finds first.

// Package muxengine is the domain kernel for lyx's psmux window manager: the
// psmux subprocess overlay, strand bookkeeping, persisted state, config, and
// (in the operations layer) the lifecycle verbs that compose them. It is the
// "dumb carrier" for its caller's strand data — muxengine stores every field
// a caller writes into a strand and reads none of them semantically. There is
// deliberately no domain `type` field on a strand: `cmd`/`resumeCmd` are
// opaque strings muxengine never parses or branches on, and `--role`/`--round`
// are formatting-only inputs consumed once, at add-time, to fill the
// strand-name template — they are never persisted or read back.
//
// muxengine imports internal/muxengine/render (the pure display-vocabulary
// leaf) and maps its own persisted records down to render.Strand when
// computing a layout; render never imports muxengine, so the import graph
// stays acyclic (muxcli -> muxengine -> render).
//
// One additional invariant this package enforces: exactly one named psmux
// server per hub. The server name is derived deterministically from the hub
// path (ServerName), so every worktree under the same hub locates and shares
// the same psmux server rather than each spawning its own.
package muxengine
