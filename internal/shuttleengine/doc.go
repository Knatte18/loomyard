// Package shuttleengine runs one LLM agent as an interactive session and
// returns its result. It is the unit review and loom call once per spawn:
// "run this producer / handler / progress-judge, give me back its output
// files." shuttle owns which provider (via an engine), the prompt envelope,
// and what "done" means. It does not own panes, layout, or tmux mechanics —
// it asks mux for a strand and drives the LLM in it.
//
// Every agent runs as an interactive tmux session, never headless
// `claude -p` — an economic constraint (subscription coverage), not a
// technical one. This is why the whole proc -> mux -> shuttle stack exists
// instead of a plain headless exec.
//
// shuttle runs a provider through an engine: a per-LLM adapter that knows
// how to launch and drive its provider as a tmux session — construct the
// launch command, inject the prompt, recognize the completion edge, locate
// the output. A Claude engine now; Gemini etc. later. The
// verdict/output contract is provider-invariant, which is what makes
// engines swappable: shuttleengine defines the Engine interface and its
// value types, and never imports a concrete engine implementation (the
// provider-seam import rule, enforced by seam_enforcement_test.go in a
// later batch) — concrete engines import shuttleengine, not the reverse.
//
// The only channel in and out of a shuttle run is files: the prompt is
// handed to the provider as the launch argument (never typed into a live
// pane), and the agent writes its structured result to a file the caller
// reads. This package (the foundation batch) provides the pure, hermetic
// building blocks the rest of shuttleengine is built from: the config
// module (shuttle.yaml), the run Spec and its validation, the run
// directory / run.json state and its age-guarded orphan sweep, and the
// Windows-to-POSIX path helper the engine layer needs for hook commands.
// Nothing here calls tmux or claude, or knows about either.
package shuttleengine
