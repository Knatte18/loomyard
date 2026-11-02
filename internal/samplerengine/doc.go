// Package samplerengine owns the sampler recipe's hub-wide configuration.
//
// `sampler.yaml` is a hub-wide config module with two keys: `agent`, the model-spec of the sampler agent's interactive session, and `code_review`, whether the Code-Review perch runs on a sampler run.
// Neither key is a guard, so a task's `run_settings` or a profile block may set `sampler.code_review` in the local layer.
// LoadConfig loads the file strictly and validates the `agent` model-spec at load time.
// A hub with no `sampler.yaml` loads the template's values.
//
// Record is the Sampler-Session row's state in one never-tracked JSON file in the run's scratch directory: the session the row last recorded, and the done flag `lym loom sampler-done` writes.
// The flag is a DoneStamp, the entry stamp EntryStamp counted when it was written, so a flag from an earlier entry into the row is ignored.
// EntryStamp counts how often a run's history shows it entering the row from another row; a halt, a failure with no history entry and a resume leave it unchanged, and every return by a goto from another row raises it.
// ReadRecord and UpdateRecord read and write the file under its lock and keep any key they do not declare.
//
// SessionSpec builds the spec the sampler agent's interactive session starts under, from a SessionInputs the caller tells it, the resolved Config and a model registry.
// The session is unawaited, publishes nothing, and opens on the `sampler-template-agent` stencil, which carries paths and, in the PATTERN directive, the PATTERN overview: the agent reads the decision record as the task's brief and works with the operator in the task worktree.
// It renders the interactive parent directive, the edit directive and, from the task worktree's PATTERN overview, the implementer PATTERN directive, because the agent writes and commits code; it refuses an empty required field by name and an agent model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package samplerengine
