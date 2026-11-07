// tacklerconsume.go consumes a tackler's resume verdict when the Go driver re-launches a run on it:
// it checks the run against the attempt under the status lock, marks the attempt applied and performs the verdict's goto.

package sheddriver

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ConsumeTacklerAttempt applies the tackler attempt held in attemptDir to the run base addresses.
// It reads the attempt record and the call's validated envelope.json, never the agent's verdict.yaml.
// Before taking any lock it refuses an attempt that is unusable or already applied.
// An attempt is unusable when its record or envelope is missing or unparseable.
// It is also unusable when its verdict is not a resume or carries a cause.
// It then calls shedengine.Relaunch with the envelope's goto target.
// Under the status lock, its Admit refuses unless the run is in the state the tackler left.
// That state has the attempt's stop key and pause conditions equal to those recorded at spawn.
// A run that already stands at the verdict's goto target is admitted as in effect, with no second goto performed.
// Admit writes the applied mark before the goto is performed.
// The mark and the move therefore land under one hold of the status lock.
// A resume with no goto relaunches with an empty target, which leaves the status unchanged.
// Every refusal names the attempt directory and the way forward.
// The driver acts on the refused stop as a new stop, or reports it in the stop report for the run's parent to decide.
func ConsumeTacklerAttempt(attemptDir string, base shedengine.RelaunchRequest) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("sheddriver: tackler attempt %s: "+format+"; way forward: act on the refused stop as a new stop, or report it in the stop report and the run's parent decides", append([]any{attemptDir}, args...)...)
	}

	attempt, err := ReadTacklerAttempt(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	envelope, found, err := ReadTacklerEnvelope(TacklerEnvelopePath(attemptDir))
	switch {
	case err != nil:
		return refuse("unusable: %v", err)
	case !found:
		return refuse("unusable: the tackler call left no envelope")
	case envelope.Verdict != TacklerVerdictResume || envelope.Cause != "":
		return refuse("unusable: the validated verdict is %q with cause %q, not a resume", envelope.Verdict, envelope.Cause)
	}
	applied, err := TacklerApplied(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	if applied {
		return refuse("already applied")
	}

	request := base
	request.Target = envelope.Goto
	request.Admit = func(current shedengine.Status) (bool, error) {
		inEffect, reason := AdmitRelaunch(current, attempt.StopKey, attempt.PauseBefore, attempt.PauseAfter, request.Target, "tackler")
		if reason != "" {
			return false, refuse("%s", reason)
		}
		return inEffect, MarkTacklerApplied(attemptDir)
	}
	_, err = shedengine.Relaunch(request)
	return err
}
