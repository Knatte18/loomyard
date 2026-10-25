// doc.go carries the package godoc for shedarbiter: the arbiter's place in a shed run's stop chain and the seams its callers fill.

// Package shedarbiter is the arbiter call every shed-driven recipe's Go driver uses.
// At a stop the driver's rule table and tackler could not settle, it spawns a cold, stronger-model agent through shuttle, waits for its verdict file and returns the validated verdict.
//
// It is a told-geometry package: every path, clock, shuttle and reader reaches it through Deps, and it imports no cwd resolver and no CLI package.
// A recipe's driver builds Deps once and either settles a stop through NewCall and Call.Settle, or hands Seam to the shed driver.
// The records an attempt leaves (attempt, marker, verdict, envelope) stay in loomengine, and the call writes and reads them there.
//
// # The call
//
// A call spawns at most Settings.MaxSpawns agents per run, counted from the attempt records on disk, so no restart resets the count, and it moves no run itself.
// A call for a stop an earlier attempt already arbitrated spawns nothing: it returns that attempt's verdict, or waits on the attempt's live arbiter.
// A verdict file that does not read, a run that timed out or died, and a resume whose precondition fails each end as a hand-on naming a cause.
// Every refusal ends with Deps.HandBackWayForward, which names the verb that shows the stop.
//
// # The prompt
//
// Prompt fills the arbiter stencil over one stop's paths and the description of the recipe whose run it judges, so the stencil itself names no recipe.
// Spec and VerdictGate compose the launch and the re-prompt gate that keeps an unreadable verdict from ending the run.
//
// # The resume
//
// ConsumeAttempt applies a resume verdict when the driver re-launches the loop naming its attempt: it checks the run against the attempt under the status lock, marks the attempt applied and performs the goto.
package shedarbiter
