// Package shuttleengine runs one LLM agent as an interactive session and returns its result.
// It is the unit review and loom call once per spawn: "run this producer / handler /
// progress-judge, give me back its output files."
// shuttle owns which provider (via an engine), the prompt envelope, and what "done" means.
// It does not own panes, layout, or tmux mechanics — it asks reed for a strand and drives the LLM
// in it.
//
// Every agent runs as an interactive tmux session, never headless `claude -p` — an economic
// constraint (subscription coverage), not a technical one.
// This is why the whole proc -> reed -> shuttle stack exists instead of a plain headless exec.
//
// shuttle runs a provider through an engine: a per-LLM adapter that knows how to launch and drive
// its provider as a tmux session — construct the launch command, inject the prompt, recognize the
// completion edge, locate the output.
// A Claude engine now;
// Gemini etc. later.
// The verdict/output contract is provider-invariant, which is what makes engines swappable:
// shuttleengine defines the Engine interface and its value types, and never imports a concrete
// engine implementation (the provider-seam import rule, enforced by seam_enforcement_test.go in a
// later batch) — concrete engines import shuttleengine, not the reverse.
//
// shuttle is told its anchor path and worktree root as plain strings, at Runner construction, and
// derives neither — internal/lyxcwd is consequently absent from the package's production imports.
//
// The only channel in and out of a shuttle run is files: the prompt is handed to the provider as
// the launch argument (never typed into a live pane),
// and the agent writes its structured result to a file the caller reads.
// The package is two halves.
// The pure, hermetic half derives nothing and spawns nothing: the config module (shuttle.yaml), the
// run Spec and its validation, the run directory / run.json state and its age-guarded orphan sweep,
// and the Windows-to-POSIX path helper the engine layer needs for hook commands.
// The run-loop half — Runner/Run in run.go and Wait in wait.go — drives a LIVE agent through the
// ReedOps seam: it registers and removes strands, polls a real pane's capture through the engine's
// Startup classifier, plays key choreography into that pane for Interrupt/Send/Inject, and reads
// reed's own persisted state during the orphan sweep.
// What stays true of BOTH halves is the provider boundary, not hermeticity: nothing here names a
// tmux command or a Claude specific — panes are reed's vocabulary, reached only through ReedOps,
// and provider grammar is the concrete Engine's, reached only through the Engine interface.
package shuttleengine
