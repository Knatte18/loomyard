// arbiterconsume.go consumes an arbiter's resume verdict when the driver re-launches the loop naming its attempt:
// it checks the run against the attempt under the status lock, marks the attempt applied and performs the verdict's goto.

package loomcli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// consumeArbiterAttempt applies the arbiter attempt held in attemptDir to the run base addresses.
// It reads the attempt record and the verb's validated envelope.json, never the agent's verdict.yaml.
// Before taking any lock it refuses an attempt whose envelope is missing or unparseable, whose verdict is not a resume or carries a cause, and one already applied.
// It then calls shedengine.Relaunch with the envelope's goto target and an Admit that refuses, under the status lock, unless the run is in the state the arbiter left:
// its stop key is the attempt's and the pause conditions equal those the attempt recorded at spawn,
// or the run already stands at the verdict's goto target, in which case no second goto is performed.
// A pause_requested left by an older binary does not refuse: Relaunch carries it through and the shed honours it at the next producer boundary.
// Admit writes the applied mark before the goto is performed, so the mark and the move land under one hold of the status lock.
// Every refusal names the verdict path and the hand-on way forward: the driver's stop report quotes the verdict and the run's parent decides.
func consumeArbiterAttempt(attemptDir string, base shedengine.RelaunchRequest) error {
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
		inEffect, reason := admitRelaunch(current, attempt.StopKey, attempt.PauseBefore, attempt.PauseAfter, request.Target, "arbiter")
		if reason != "" {
			return false, refuse("%s", reason)
		}
		return inEffect, loomengine.MarkArbiterApplied(attemptDir)
	}
	_, err = shedengine.Relaunch(request)
	return err
}

// admitRelaunch decides whether a re-launch on a recorded attempt may proceed over the status held under the lock.
// current is the status now, key and pauseBefore/pauseAfter are the stop key and pause conditions the attempt recorded at spawn, target is the verdict's goto row, and actor names the spawner in refusal texts.
// An unmoved run (same stop key) is admitted unless its pause conditions differ from the recorded ones.
// A moved run is admitted as inEffect when it stands paused at target having had only goto entries since the spawn, the last naming target;
// there only a pause condition present now that the spawn did not record refuses.
// Any other move returns a refusal reason, and an empty reason with inEffect false is an ordinary admit.
func admitRelaunch(current shedengine.Status, key sheddriver.StopKey, pauseBefore, pauseAfter, target, actor string) (inEffect bool, refusal string) {
	paused := fmt.Sprintf("the run was paused since the %s spawned", actor)
	if sheddriver.StopKeyOf(current) == key {
		if current.PauseBefore != pauseBefore || current.PauseAfter != pauseAfter {
			return false, paused
		}
		return false, ""
	}
	if !verdictInEffect(current, key, target) {
		return false, fmt.Sprintf("the run moved since the %s spawned: its stop key is no longer the attempt's", actor)
	}
	if (current.PauseBefore != "" && current.PauseBefore != pauseBefore) || (current.PauseAfter != "" && current.PauseAfter != pauseAfter) {
		return false, paused
	}
	return true, ""
}

// verdictInEffect reports whether current stands paused at target with only goto entries added since the stop key was taken, the last naming target.
func verdictInEffect(current shedengine.Status, key sheddriver.StopKey, target string) bool {
	if target == "" || current.CurrentProducer != target || current.State != shedengine.StatePaused || len(current.History) <= key.HistoryLength {
		return false
	}
	added := current.History[key.HistoryLength:]
	for _, entry := range added {
		if entry.Outcome != shedengine.OutcomeGoto {
			return false
		}
	}
	return added[len(added)-1].Producer == target
}

// loomConsumeAttempt is loom's ConsumeAttempt hook: it parses attempt as a positive attempt number of the run and consumes that attempt's directory.
func (c *loomCLI) loomConsumeAttempt(_ context.Context, attempt string) error {
	number, err := strconv.Atoi(attempt)
	if err != nil || number < 1 {
		return fmt.Errorf("loom: arbiter attempt %q is not a positive integer; way forward: report the stop to the run's parent in the stop report, and the parent decides", attempt)
	}
	return consumeArbiterAttempt(shedrun.ArbiterAttemptDir(c.location, c.runID, number), shedengine.RelaunchRequest{
		StatusPath:              c.shedPaths.StatusPath,
		LockPath:                c.shedPaths.LockPath,
		StatusLockPath:          c.shedPaths.StatusLockPath,
		Producers:               c.routing.Producers,
		RunID:                   shedrun.ResolveRunID(c.location, c.runID),
		MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
	})
}
