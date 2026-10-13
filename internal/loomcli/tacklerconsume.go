// tacklerconsume.go consumes a tackler's resume verdict when the Go driver re-launches a run on it:
// it checks the run against the attempt under the status lock, marks the attempt applied and performs the verdict's goto.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// consumeTacklerAttempt applies the tackler attempt held in attemptDir to the run base addresses.
// It reads the attempt record and the call's validated envelope.json, never the agent's verdict.yaml.
// Before taking any lock it refuses an attempt whose envelope is missing or unparseable, whose verdict is not a resume or carries a cause, and one already applied.
// It then calls shedengine.Relaunch with the envelope's goto target and an Admit that refuses, under the status lock, unless the run is in the state the tackler left:
// its stop key is the attempt's and the pause conditions equal those the attempt recorded at spawn.
// Admit writes the applied mark before the goto is performed, so the mark and the move land under one hold of the status lock.
// A resume with no goto relaunches with an empty target, which leaves the status unchanged.
// Every refusal names the attempt directory and the way forward: the driver acts on the refused stop as a new stop, or reports it in the stop report and the run's parent decides.
func consumeTacklerAttempt(attemptDir string, base shedengine.RelaunchRequest) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("loom: tackler attempt %s: "+format+"; way forward: act on the refused stop as a new stop, or report it in the stop report and the run's parent decides", append([]any{attemptDir}, args...)...)
	}

	attempt, err := loomengine.ReadTacklerAttempt(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	envelope, found, err := loomengine.ReadTacklerEnvelope(loomengine.TacklerEnvelopePath(attemptDir))
	switch {
	case err != nil:
		return refuse("unusable: %v", err)
	case !found:
		return refuse("unusable: the tackler call left no envelope")
	case envelope.Verdict != loomengine.TacklerVerdictResume || envelope.Cause != "":
		return refuse("unusable: the validated verdict is %q with cause %q, not a resume", envelope.Verdict, envelope.Cause)
	}
	applied, err := loomengine.TacklerApplied(attemptDir)
	if err != nil {
		return refuse("unusable: %v", err)
	}
	if applied {
		return refuse("already applied")
	}

	request := base
	request.Target = envelope.Goto
	request.Admit = func(current shedengine.Status) error {
		if loomengine.StopKeyOf(current) != attempt.StopKey {
			return refuse("the run moved since the tackler spawned: its stop key is no longer the attempt's")
		}
		if current.PauseBefore != attempt.PauseBefore || current.PauseAfter != attempt.PauseAfter {
			return refuse("the run was paused since the tackler spawned")
		}
		return loomengine.MarkTacklerApplied(attemptDir)
	}
	_, err = shedengine.Relaunch(request)
	return err
}
