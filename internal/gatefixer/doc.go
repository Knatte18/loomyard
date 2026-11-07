// Package gatefixer owns the life of one cold fixer strand:
// the agent a failed gate hands its mechanical failures to before the gate is run again.
//
// # Roles
//
// A fixer strand runs under one of two reed roles:
// RoleGateFixer for the callers of this package and RoleVerifyFixer for webster's verify-gate fixer.
// Roles lists them and IsFixerRole reports whether a role, a numbered form included, is one of them.
//
// # Record and store
//
// A Record is what a caller keeps of one spawned fixer.
// It holds the fixer's strand, session, engine and spawn time, the shuttle run directory, the report and reply paths,
// and the events file with the offset past the skill-load turns.
// It is live while its status is empty, and terminal with StatusDone or StatusDead.
// The caller owns where records live and reaches them through a Store.
//
// # Spawn, wait and stop
//
// SpawnOrAttach attaches to the live record a Store holds.
// Otherwise it spawns a strand whose only output file is the reply file.
// It registers the strand through Deps.Register and saves its record before it returns.
// A spawn that cannot be recorded or registered stops the strand again, so no fixer outlives its call unrecorded.
// Await polls the strand until it is done or dead.
// The strand is done on its reply file.
// It is dead when its newest turn ended without one, its timeout passed or its strand is gone.
// Every terminal result stops the strand.
// Await never marks a record terminal; the caller does.
// Registrar builds the Deps.Register seam that registers a fixer's pane process with runproc.
// runproc's end verbs then stop it with the rest of its run.
//
// # Prompt
//
// RenderPrompt joins one shared cold frame ahead of one caller body stencil, reading both through stencilstore.
// It fills them from the report and reply paths it is told and the body's own values.
// It also fills the parent, edit, PATTERN and friction directives.
//
// # Row gates
//
// A Fixer is the spawn-and-wait seam callers drive, and NewStrandFixer is its production form over real strands.
// WrapRowGate wraps a shuttle gate spec.
// A failure of one named entry at the writer's idle boundary then goes to a fixer,
// and the entry is judged again over the fixer's finished tree.
// The wrap keeps its records in a rowStore at `<visit dir>/<entry>/records.json` under the row visit's directory.
// Each fixer's report and reply sit beside it as `report-<n>.md` and `reply-<n>.md`.
// At most one fixer runs per failed evaluation, and at most the entry's attempts plus one per row visit.
// A failure judged over a fixer the same evaluation waited on goes to the writer.
// The entry's final closure and every other entry only wait for a live fixer,
// so every entry is judged over the fixer's finished tree.
//
// # Judging
//
// A fixer never judges its gate: the caller runs the gate again and that run decides.
// This package imports nothing of any caller, so every gate caller can import it.
package gatefixer
