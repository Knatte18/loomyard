// Package driftcli builds the `lym drift` group over internal/driftengine: `scope`, `accept`, `withdraw`, `compare`, `capture [--side]`, `defer`, `waive`, `status` and the hidden `capture-child`.
// The engine holds every rule and is handed paths and seams; this package resolves the worktree and builds them.
//
// Every verb resolves its worktree with lymcwd.Resolve and runs only inside a tapestry or darn run's worktree, so the module is hub-only and internal/cliwire does not serve it.
// `scope`, `accept`, `withdraw` and `compare` carry the role audience, since the Drift agents run them; `capture`, `defer`, `waive` and `status` carry the operator audience, and `capture-child` the internal one.
// `capture --side` is the Drift-Verify agent's form of an operator command, which the verify stencil names directly.
// The envelope carries a verb's result under its own name, and a refusal goes through output.Err with the engine's message, which names its way forward.
//
// # Wiring
//
// The run status is read under its lock from the self run, and the agent-session mark from the run counter; together they tell the Drift-Verify agent session from a caller at a halt.
// HEAD, the branch point and the tracked dirty paths come from the worktree's fabric: the branch point is the merge base of HEAD with the parent branch the origin record names.
// The gate slots come from hubgeom.GateSlots, and the capture child is registered under the run-process directory with the worktree name as owner.
// The engine is told the worktree root lymcwd resolves, which a command step's relative dir is joined onto, and the anchored directory, where the capture child starts and a step with no dir runs.
//
// # Capture
//
// `capture` checks the main capture's precondition, then starts `<this binary> drift capture-child` detached in the worktree's anchored directory with breakaway, falling back to a plain detached start where Windows refuses breakaway, and returns once the child's capture record carries the child's pid.
// A record that already reads failed then fails the call with driftengine.FailedCaptureError, so a capture that ended before the call returned, an exit 0 with no baseline file among them, exits non-zero with its stated cause; otherwise the result carries the pid and the record's state.
// The child holds the capture's named job, runs the capture under the capture lock, re-checks the precondition under that lock and releases the job as its last act.
// A capture that ends failed is written to the child's log as an error envelope carrying driftengine.FailedCaptureError, through RunCapture's OnEnd under the capture lock, and the child exits non-zero.
// A child whose cwd is outside a git repository or is not the anchored directory marks its error envelope with driftengine.LastingFaultKey, since a fresh start runs in the same directory and fails the same way.
// It writes nothing to its log once the capture has returned, because the Drift-Await row may commit the log once the lock is free.
// Under go test the start returns an error and spawns nothing.
//
// # Runs
//
// `compare` and `capture --side` each take one run of the agent session's budget from the run counter after the declared step is known, so a verb that fails on an undeclared step spends none.
// A spent budget is refused to the agent session with the instruction to write verify-report.md and end its turn, and to a caller at a halt with the instruction to resume the run for a fresh agent.
//
// # Accept
//
// `accept` takes the accept lock first and refuses while another accept of the worktree holds it, so two accepts never share the accepting directory.
// It then runs driftengine.CheckAccept, so a refused acceptance, a test name with no profile file under the baseline directory among them, captures nothing.
// It then runs the acceptance capture of the current code with the scope's skipped tests left out, in a gate slot and outside the session's budget, and a capture that does not pass refuses the acceptance with its reason;
// it reads the run status again after the capture, which can last minutes, and driftengine.Accept then keeps the test's output from that capture as the accepted one and decides from that status who the acceptance is by.
// The lock is held until Accept returns.
package driftcli
