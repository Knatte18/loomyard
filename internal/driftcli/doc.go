// Package driftcli builds the `lym drift` group over internal/driftengine: `scope`, `accept`, `withdraw`, `compare`, `capture [--side]`, `defer`, `waive`, `status` and the hidden `capture-child`.
// The engine holds every rule and is handed paths and seams; this package resolves the worktree and builds them.
//
// Every verb resolves its worktree with lymcwd.Resolve and runs only inside a loom or darn run's worktree, so the module is hub-only and internal/cliwire does not serve it.
// `scope`, `accept`, `withdraw` and `compare` carry the role audience, since the Drift agents run them; `capture`, `defer`, `waive` and `status` carry the operator audience, and `capture-child` the internal one.
// `capture --side` is the Drift-Verify agent's form of an operator command, which the verify stencil names directly.
// The envelope carries a verb's result under its own name, and a refusal goes through output.Err with the engine's message, which names its way forward.
//
// # Wiring
//
// The run status is read under its lock from the self run, the recipe from the run's seed, and the agent-session mark from the run counter, which together tell the Drift-Verify agent session from a caller at a halt.
// HEAD, the branch point and the tracked dirty paths come from the worktree's fabric: the branch point is the merge base of HEAD with the parent branch the origin record names.
// The gate slots come from hubgeom.GateSlots, and the capture child is registered under the run-process directory with the worktree name as owner.
//
// # Capture
//
// `capture` checks the main capture's precondition, then starts `<this binary> drift capture-child` detached with breakaway, falling back to a plain detached start where Windows refuses breakaway, and returns once the child's capture record reads running.
// The child holds the capture's named job, runs the capture under the capture lock, re-checks the precondition under that lock and releases the job as its last act.
// It writes nothing to its log once the capture has returned, because the Drift-Await row may commit the log once the lock is free.
// Under go test the start returns an error and spawns nothing.
//
// # Runs
//
// `compare` and `capture --side` each take one run of the agent session's budget from the run counter after the declared step is known, so a verb that fails on an undeclared step spends none.
// A spent budget is refused to the agent session with the instruction to write verify-report.md and end its turn, and to a caller at a halt with the instruction to resume the run for a fresh agent.
package driftcli
