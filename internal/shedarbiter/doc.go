// Package shedarbiter is the arbiter call every shed-driven recipe's Go driver uses.
// It handles a stop the driver's rule table and tackler could not settle.
// It spawns a cold, stronger-model agent through shuttle.
// It waits for the agent's verdict file and returns the validated verdict.
//
// It is a told-geometry package: every path, clock, shuttle and reader reaches it through Deps,
// and it imports no cwd resolver and no CLI package.
// The records an attempt leaves (attempt, marker, verdict, envelope) stay in loomengine.
// The call writes and reads them there.
//
// # The call
//
// A call spawns at most Settings.MaxSpawns agents per run.
// The count comes from the attempt records on disk, so no restart resets it.
// A call moves no run itself.
// A call for a stop an earlier attempt already arbitrated spawns nothing:
// it returns that attempt's verdict, or waits on the attempt's live arbiter.
// A call ends as a hand-on naming a cause in three cases:
// a verdict file does not read; a run times out or dies; a resume's precondition fails.
// A run that ends on an API error ends as a hand-on with cause `api-error`,
// and the hand-on's question names the error, so the stop reaches the run's parent at once rather than at the deadline.
// Every refusal ends with Deps.HandBackWayForward, which names the verb that shows the stop.
//
// # The fix
//
// A driver that sets Deps.Worktree lets its arbiter make one fix of its own:
// it edits the task's code worktree and commits the edit on the task branch before it writes its verdict.
// The plan, the decision record and PATTERN entries still change only through the recipe's repair verbs.
// The arbiter must leave every change committed, on the branch and on top of the HEAD it found;
// otherwise a resume is handed on with cause `precondition`.
// A resume after commits needs a goto target.
// The run's own rows, and their gates, then check the fix before the run goes on.
// A hand-on's question names the commits for the run's parent to keep or revert.
// A driver that leaves Deps.Worktree nil keeps its arbiter to a verdict, and its prompt says so.
//
// # The prompt
//
// Prompt fills the arbiter stencil over one stop's paths and the description of the recipe whose run it judges,
// so the stencil itself names no recipe.
//
// # The resume
//
// ConsumeAttempt applies a resume verdict when the driver re-launches the loop naming its attempt:
// it checks the run against the attempt under the status lock, marks the attempt applied and performs the goto.
package shedarbiter
