// Package driftcli builds the `lym drift` group over internal/driftengine.
// The verbs are `check`, `amend`, `capture`, `defer`, `status` and the hidden `capture-child`.
// The engine holds every rule and is handed paths and seams; this package resolves the worktree and builds them.
//
// Every verb resolves its worktree with lymcwd.Resolve and runs only inside the worktree of a run with Drift rows:
// a tapestry or darn run, or a sampler run whose Drift rows are on,
// so the module is hub-only and internal/cliwire does not serve it.
// `check` carries the role audience, since any session of the run's worktree runs it;
// `capture`, `amend`, `defer` and `status` carry the operator audience, and `capture-child` the internal one.
// The envelope carries a verb's result under its own name,
// and a refusal goes through output.Err with the engine's message, which names its way forward.
//
// # Wiring
//
// The run status is read under its lock from the self run, beside whether the capture lock is held.
// HEAD, the branch point and the tracked dirty paths come from the worktree's fabric:
// the branch point is the merge base of HEAD with the parent branch the origin record names.
// The gate slots come from hubgeom.GateSlots,
// and the capture child is registered under the run-process directory with the worktree name as owner.
// The engine is told the worktree root lymcwd resolves, which a command step's relative dir is joined onto.
// It is also told the anchored directory, where the capture child starts and a step with no dir runs.
//
// # Capture
//
// `capture` checks the main capture's precondition.
// It then starts `<this binary> drift capture-child` detached in the worktree's anchored directory with breakaway.
// Where Windows refuses breakaway, the start falls back to a plain detached start.
// The call returns once the child's capture record carries the child's pid.
// A record that already reads failed then fails the call with driftengine.FailedCaptureError.
// So a capture that ended before the call returned exits non-zero with its stated cause.
// An exit 0 with no baseline file is among those.
// Otherwise the result carries the pid and the record's state.
// The child holds the capture's named job and runs the capture under the capture lock.
// It re-checks the precondition under that lock and releases the job as its last act.
// A capture that ends failed is logged by the child as an error envelope carrying driftengine.FailedCaptureError.
// RunCapture's OnEnd writes it under the capture lock, and the child exits non-zero.
// A child whose cwd is outside a git repository, or is not the anchored directory, fails the same way on a fresh start.
// It marks its error envelope with driftengine.LastingFaultKey.
// It writes nothing to its log once the capture has returned;
// the Drift-Await row may commit the log once the lock is free.
// Under go test the start returns an error and spawns nothing.
//
// # Check
//
// `check` runs driftengine.Check over the worktree.
// It prints, under `check`, whether the drift gate would pass and the skip.
// It also prints each fault, each unexpected cell with its case, identity, old and new value, and the report's findings.
// Its seams read the run's recipe row order from the recipe the run's seed names.
// They take the current capture in the gate slot the process's own gate holds when there is one.
// They run the compare with no skip list.
// A run not past Drift-Expect prints that no expectation is sealed yet and captures nothing.
// It writes no tracked record, only the scratch capture directory, and decides nothing: Drift-Verify repeats the check.
//
// # Amend and defer
//
// `amend <slug>` and `defer [<slug>]` act on a run from any worktree of the hub:
// openSlugWorktree resolves the slug's worktree as fabricengine.WorktreePath names it, at the cwd's own anchor.
// It refuses a slug that names none, naming `lym board list`.
// `defer` without a slug acts on the cwd's own worktree.
// Both run checkParentCaller first, as `lym loom decision add` runs its own caller check:
// the caller's strand name, from agentname.StrandNameEnv, passes when it is empty, as in the operator's own shell;
// it passes when it equals the run's parent orch, resolved by orchcli.NearestLiveParent over the target worktree;
// it is refused, naming the parent and the operator's shell, when it is another session;
// a parent that cannot be resolved refuses a named caller, naming the error.
// Bound: the check rests on the strand name and on the shell guard that refuses setting lym's strand variables.
// `amend` takes exactly one of `--entries <file>`, `--restore` or `--discard`, and calls driftengine.Amend.
// It refuses a file it cannot read, naming its path.
// `defer` calls driftengine.Defer.
// Both are handed the seams of checkDeps, which include the records commit,
// so each writes its record through the records guard.
//
// # Status
//
// `status` prints the scope record, the capture record, and whether the capture lock and the compare marker are held.
// It also prints the diff mode, which is the scope record's or, while none is recorded, drift.yaml's.
// Under `lines` it also prints `compare_ignored: true`, since the compare command never runs under that mode.
package driftcli
