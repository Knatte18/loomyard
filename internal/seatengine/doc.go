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
// A table may hold no advisors: the chair then runs alone, and seat-directive-chair tells it to decide each question itself.
// Every seat has at least one output and no two seats share an output path, and every advisor output is among the chair's inputs.
// The role prefix and every seat's formed role pass the agent-name role grammar.
// Every seat's stencil is readable from the told stencils directory, as is every block it includes (stencil.IncludeNames over the bytes read), and an included block declares no include of its own.
// A table or seat value neither collides with a name in ReservedMarkers nor is empty or whitespace-only.
// A table value named in Table.Optional may be empty and then renders as nothing in the stencil and in every block it includes; an Optional name may not be a reserved marker, and a seat value is never exempt.
// Prompts composes every seat's prompt from a table without starting anything, so a table builder can test what its seats will read.
// The list ReservedMarkers returns is declared once in this package; the table's collision check and the seats' value maps read that declaration, and a caller that builds a table reaches it through Table.Validate.
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
// # Running a table fresh
//
// Engine.Run validates the table, forms every seat's strand name from the geometry's shortname and slug and the seat's role, and starts the advisors in table order and the chair last.
// Each seat's prompt is its stencil filled, with the blocks it includes, from a value map holding the table's values, the seat's own and every reserved marker.
// The chair starts with the table's gate and is the only interactive seat; an advisor starts ungated, keeps its pane and holds a turn end quietly.
// A started seat whose strand took a different name than expected, which is reed numbering a name an earlier strand still holds, is stopped and reported as a start error naming the holder and the way forward.
// An advisor that fails to start is recorded on its SeatResult and named in the chair's failed advisors, and the step goes on without it.
// A chair that fails to start stops every started advisor and returns the error, which wraps shuttleengine.ErrNotStarted when the provider never came up.
//
// # The notice sender
//
// Each started advisor is waited on from the moment it starts.
// An advisor that reaches done queues nothing, since it stays reachable and the chair reads its file.
// An advisor that dies, times out or whose wait errors queues one line for the chair.
// One goroutine per chair types the queued lines through the chair's handle, one at a time, so two lines are never interleaved.
// It retries a busy or unlanded send after the engine's notice interval until the line lands, the chair's outputs all exist or the sender is stopped, and logs a line that never lands rather than failing the step.
//
// # The stop rule
//
// Run waits on the chair, stops the sender, then stops every started advisor, a kept done one included, before it returns.
// A chair whose wait errored carries no terminal outcome and may still be running, so it is stopped as well.
// A stop that fails is an error wrapping ErrSeatNotStopped whose message names the way forward; it is never retried and the seat is not waited for.
// The result carries the chair's outcome, gate and NotStarted as shuttle reported them: the engine judges nothing about finishing itself (PATTERN-completion-signal).
//
// # Probe before archive
//
// Engine.Probe reports which seats have a live run, the chair probed with the table's gate and each advisor ungated, and archives and starts nothing.
// A terminal advisor whose strand is still up, a done one kept by its pane or a timed-out one, is removed by the probe itself, keeping its record's outcome and its output file, and answers not found.
// With the chair live, Probe returns its handle and every advisor found.
// With the chair not live, Probe stops every live advisor and returns an empty LiveTable, so a caller archives stale outputs and starts fresh only once no seat is live.
// The engine never archives: the caller owns that step, between Probe and Run.
//
// # Resume
//
// Engine.Resume attaches to a live chair and the advisors the probe found, and joins them as a fresh run does; an advisor the probe did not find is never restarted.
// The chair is told of each such advisor once: one whose outputs all exist is finished and the line says its output stands and the seat no longer answers, any other has ended and the line says so as a mid-step death does.
// A notice sent before the interruption may land again, which costs the chair nothing.
//
// # Imports
//
// The package imports shuttleengine, stencil, stencilstore, agentname, segmentcolor, logger, the directive renderers and the standard library.
package seatengine
