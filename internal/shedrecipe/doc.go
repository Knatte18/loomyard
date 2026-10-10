// Package shedrecipe owns the engine registry: the name to shedengine.ShedProducer-constructor
// mapping a future recipe loader resolves each row's Engine field against.
// It is imported by nobody in the four producer-hosting packages it imports (internal/shedengine,
// internal/shedadapters, internal/websterengine, internal/landingshed), so the dependency runs one
// way only.
//
// This package holds PATTERN-told-geometry in its precise form: an
// engine is handed the absolute paths it operates on and derives none of its own, so it runs
// identically inside a lyx hub and in a bare directory that is not a git repository.
// Every root this package touches is told, none is derived, and the package's only path
// construction is joining a told root with a recipe-relative value.
//
// Env is the told bundle: the caller-filled roots and injected seams every entry may read from.
// Config is the portable per-row half: the recipe row's own static, already-decoded configuration,
// which never contains an absolute path and which this package never learns the file format of.
//
// # The MultiLLM row
//
// The MultiLLM entry builds a seatengine.Table from its row: a role prefix, a loom segment, an ordered list of seats, and optional gates, timeout, interactivity and table tokens.
// Each seat names its stencil, its model spec and its worktree-relative inputs and outputs, and optionally its skills and values.
// A seat's model spec is parsed and resolved against Env.Models at construction, so an unknown alias fails before any call.
// Table.Validate runs at construction against Env.StencilsDir, so an unreadable stencil, an unresolvable include and a reserved or blank value fail there too.
// The producer drives Env.Seats, the seat runner the caller wires over its shuttle runner; the entry reads no other seam.
//
// # The DiscussionSeats row
//
// The DiscussionSeats entry runs a chair and its advisors on a MultiLLM producer behind the discussion commit decorator.
// Its table is told rather than built from Config: Env.DiscussionTable is a closure the caller supplies, evaluated once per call, because building the table needs a location this package may not import.
// The row's "gates" key is stamped onto each table the closure returns, so the gate guards the chair whatever table the closure builds.
// When the gates hold an enabled "parent-review" entry, the producer's fresh-spawn preparation opens the next review round before any stale output is archived and before any seat starts; a resumed live chair continues the latest round.
package shedrecipe
