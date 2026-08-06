// ownership.go provides ownerlessRunWarnings, the shared helper every
// state-mutating bracket verb (begin/await/record/recover-batch) uses to
// append a warning when no live `lyx webster run` owns the state it is about
// to mutate. A bracket verb is Master's own call and legitimately runs only
// under a live run holding run.lock; a Master that keeps driving verbs after
// its run process died (the shuttle-asking exit leaves the pane alive) is a
// zombie whose mutations have no run-level owner and no run-exit backstop --
// worth surfacing, never worth refusing (refusing would break the sandbox
// suite's sanctioned manual driving), so this is warning-only.
package webstercli

import "github.com/Knatte18/loomyard/internal/websterengine"

// ownerlessRunWarnings returns warnings with a zombie-run notice appended
// when no live run holds websterDir's run.lock. A probe error is folded into
// the warnings too rather than failing the verb: the probe is advisory, and
// a filesystem hiccup reading a lock file must never block a bracket verb.
func ownerlessRunWarnings(websterDir string, warnings []string) []string {
	active, err := websterengine.RunActive(websterDir)
	if err != nil {
		return append(warnings, "could not determine whether a live `lyx webster run` owns this state: "+err.Error())
	}
	if !active {
		return append(warnings, "no live `lyx webster run` owns this state (run.lock is free) -- this verb's mutations have no run-level owner or exit backstop; resume with `lyx webster run`")
	}
	return warnings
}
