// Package seatengine runs one step as a table of seats: a chair and its advisors, each a shuttle run in its own strand.
// The chair's turn end, outputs and gate decide the step as a single agent's would; the advisors' outputs are the chair's inputs,
// and the advisors talk to the chair over Claude Code's session message.
// The seat-running logic lives here, below internal/shedadapters, whose MultiLLM producer maps the chair's result onto the Shed contract.
//
// # The seat model
//
// A Table holds the ordered Seats and what they share: the role prefix, the loom segment, the chair's gate, the timeout, the chair's interactivity and the marker values every seat's stencil takes.
// A Seat names its stencil, its model choice, the files it reads (Inputs) and writes (Outputs), its skills and its own marker values.
// The chair seat is named RoleChair; the advisors are named AdvisorName(1), AdvisorName(2) and so on, in table order.
// A seat's strand role is SeatRole(prefix, name), so a spec never spells a role as a literal.
//
// # The table rules
//
// Table.Validate applies every rule as a loud error naming the seat.
// The table has exactly one chair, and its other seats are advisors named from 1 contiguously in table order.
// Every seat has at least one output and no two seats share an output path, and every advisor output is among the chair's inputs.
// The role prefix and every seat's formed role pass the agent-name role grammar.
// Every seat's stencil is readable from the told stencils directory, as is every block it includes (stencil.IncludeNames over the bytes read), and an included block declares no include of its own.
// A table or seat value neither collides with a name in ReservedMarkers nor is empty or whitespace-only.
// ReservedMarkers is declared once in this package; the table's collision check, the seats' value maps and the recipe entry that builds a table all read it.
//
// # Told geometry
//
// The package is told the absolute paths it works on through Geometry and derives none of its own, so it imports neither internal/lyxcwd nor a geometry constructor and runs in a directory that is not a git repository (PATTERN-told-geometry).
//
// # The shuttle seam
//
// Shuttle is the seam the engine starts and probes its seats through, handle-shaped because the engine holds several live runs at once and sends into the chair's while it waits:
// StartGated and ProbeGated return a Handle, the seat's unwaited run.
// RunnerShuttle adapts a shuttleengine.Runner to it, and *shuttleengine.Run satisfies Handle.
//
// # Imports
//
// The package imports shuttleengine, stencil, stencilstore, agentname, segmentcolor and the standard library.
package seatengine
