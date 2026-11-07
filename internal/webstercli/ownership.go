// ownership.go provides ownerlessRunWarnings, which the bracket verbs call to warn when no run owns their state.
// The bracket verbs are begin, await, record and recover-batch.
// A bracket verb is Master's own call and legitimately runs only under a live run holding run.lock.
// A Master that keeps driving verbs after its run process died is a zombie.
// A timeout exit leaves the pane alive, so its mutations have no run-level owner and no run-exit backstop.
// That is worth surfacing, never worth refusing.
// Refusing would break the sandbox suite's sanctioned manual driving, so the check only warns.
package webstercli

import "github.com/Knatte18/loomyard/internal/websterengine"

// ownerlessRunWarnings returns warnings with a notice appended when no live run holds scratchDir's run.lock.
// A probe error is folded into the warnings rather than failing the verb.
// The probe is advisory, and a filesystem hiccup reading a lock file must never block a bracket verb.
func ownerlessRunWarnings(scratchDir string, warnings []string) []string {
	active, err := websterengine.RunActive(scratchDir)
	if err != nil {
		return append(warnings, "could not determine whether a live `lym webster run` owns this state: "+err.Error())
	}
	if !active {
		return append(warnings, "no live `lym webster run` owns this state (run.lock is free) -- this verb's mutations have no run-level owner or exit backstop; resume with `lym webster run`")
	}
	return warnings
}
