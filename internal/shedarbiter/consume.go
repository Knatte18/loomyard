// consume.go consumes an arbiter's resume verdict when the driver re-launches the loop naming its attempt:
// it checks the run against the attempt under the status lock, marks the attempt applied and performs the verdict's goto.

package shedarbiter

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ConsumeAttempt applies the arbiter attempt held in attemptDir to the run base addresses.
// It reads the attempt record and the call's validated envelope.json, never the agent's verdict.yaml.
// Before taking any lock it refuses an attempt whose envelope is missing or unparseable, whose verdict is not a resume or carries a cause, and one already applied.
// It then calls shedengine.Relaunch with the envelope's goto target and an Admit that refuses, under the status lock, unless the run is in the state the arbiter left:
// its stop key is the attempt's and the pause conditions equal those the attempt recorded at spawn,
// or the run already stands at the verdict's goto target, in which case no second goto is performed.
// A pause_requested left by an older binary does not refuse: Relaunch carries it through and the shed honours it at the next producer boundary.
// Admit writes the applied mark before the goto is performed, so the mark and the move land under one hold of the status lock.
// Every refusal names the verdict path and the hand-on way forward: the driver's stop report quotes the verdict and the run's parent decides.
func ConsumeAttempt(attemptDir string, base shedengine.RelaunchRequest) error {
	verdictPath := loomengine.ArbiterVerdictPath(attemptDir)
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("loom: arbiter attempt %s: "+format+"; way forward: report the stop to the run's parent in the stop report, quoting the verdict at %s, and the parent decides", append(append([]any{attemptDir}, args...), verdictPath)...)
	}

	attempt, err := loomengine.ReadArbiterAttempt(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	envelope, found, err := loomengine.ReadArbiterEnvelope(loomengine.ArbiterEnvelopePath(attemptDir))
	switch {
	case err != nil:
		return refuse("unusable: %v", err)
	case !found:
		return refuse("unusable: the arbiter verb left no envelope")
	case envelope.Verdict != loomengine.ArbiterVerdictResume || envelope.Cause != "":
		return refuse("unusable: the validated verdict is %q with cause %q, not a resume", envelope.Verdict, envelope.Cause)
	}
	applied, err := loomengine.ArbiterApplied(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	if applied {
		return refuse("already applied")
	}

	request := base
	request.Target = envelope.Goto
	request.Admit = func(current shedengine.Status) (bool, error) {
		inEffect, reason := sheddriver.AdmitRelaunch(current, attempt.StopKey, attempt.PauseBefore, attempt.PauseAfter, request.Target, "arbiter")
		if reason != "" {
			return false, refuse("%s", reason)
		}
		return inEffect, loomengine.MarkArbiterApplied(attemptDir)
	}
	_, err = shedengine.Relaunch(request)
	return err
}
