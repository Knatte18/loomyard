// crashresume.go resumes an interrupted run's session when a later start of its role re-runs the same work.
// A session a crash, a reboot or a killed pane ended had not finished its work, so the start that re-runs it resumes it.
// A deliberate restart sets such a run aside, so the start runs a fresh session.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// interruptedResumePreamble opens the prompt of a start that resumes an interrupted session.
// The session already holds its first prompt and its own progress,
// so the preamble tells it to continue rather than start over.
const interruptedResumePreamble = "lym resumed this session after it ended before its task was done, by a crash, a reboot or a killed pane.\n" +
	"The files it wrote are still in the worktree, and its work so far is in this session.\n" +
	"Below is its task as lym renders it now. Read it again, check the state on disk, and continue from where the session stopped instead of starting over.\n\n"

// resumableAfterInterruption reports whether rs is a run whose session a later start of its role may resume at now.
// Its session ended before its work did:
// the run died, lym stopped it as interrupted, or it reads running while its waiter is gone and no relaunch holds it.
// A run set aside or with no session is not.
// Neither is one that finished, timed out, ended on API errors or was stopped on purpose.
func resumableAfterInterruption(rs RunState, now time.Time, bound time.Duration) bool {
	if rs.NoResume || rs.SessionID == "" {
		return false
	}
	switch rs.Outcome {
	case string(OutcomeDied):
		return true
	case runOutcomeStopped:
		return rs.Interrupted
	case runOutcomeRunning:
		return !runIsLive(rs, now, bound)
	}
	return false
}

// interruptedResume is a start's spec turned into a resume of an interrupted session.
// It carries what the resume check reported.
type interruptedResume struct {
	spec Spec
	// warning is the resume check's warning, empty when the check confirmed the session.
	warning string
	// copied is whether the check copied the session's transcript into the account the resume runs under.
	copied bool
}

// resumeInterrupted returns spec as a resume of pred's session, and whether the start resumes it.
// pred is the newest run of spec's role, and found is false when there is none.
// It resumes a spec that names its output files and no session of its own, on an engine that can resume a session.
// It does so when pred declares the same output files and is resumable after its interruption.
// The resumed spec's prompt opens with interruptedResumePreamble.
// It takes no skill-load turn, since the session already holds the skills its first launch loaded.
// A resume check can refuse, as for a session with no transcript on this machine or one a live process holds.
// The refusal is logged, and the start runs a fresh session.
func (r *Runner) resumeInterrupted(spec Spec, engine Engine, pred predecessor, found bool) (interruptedResume, bool) {
	if !found || spec.Unawaited || spec.ResumeSessionID != "" || !outputFilesSetEqual(pred.state.OutputFiles, spec.OutputFiles) {
		return interruptedResume{}, false
	}
	if !resumableAfterInterruption(pred.state, r.clock.Now(), relaunchBound(r.cfg)) {
		return interruptedResume{}, false
	}
	if _, ok := engine.(SessionResumer); !ok {
		return interruptedResume{}, false
	}
	resumed := spec
	resumed.ResumeSessionID = pred.state.SessionID
	resumed.SkipSkillLoadTurn = true
	resumed.Prompt = interruptedResumePreamble + spec.Prompt
	warning, copied, err := r.checkResume(resumed, engine)
	if err != nil {
		logger.Warn("shuttle: the interrupted session cannot be resumed, so the start runs a fresh one", "runDir", pred.runDir, "sessionID", pred.state.SessionID, "role", pred.state.RoleKey, "error", err)
		return interruptedResume{}, false
	}
	logger.Info("shuttle: resuming an interrupted session", "runDir", pred.runDir, "sessionID", pred.state.SessionID, "role", pred.state.RoleKey)
	return interruptedResume{spec: resumed, warning: warning, copied: copied}, true
}

// setAside marks the run in runDir so that no later start resumes its session.
// A run directory that is gone has nothing to mark.
// A failed write is logged, since it can only let a later start resume the session.
func setAside(runDir string) {
	if _, err := os.Stat(runDir); errors.Is(err, os.ErrNotExist) {
		return
	}
	if err := updateRunState(runDir, markNoResume); err != nil {
		logger.Warn("shuttle: could not set the run aside, so a later start may resume its session", "runDir", runDir, "error", err)
	}
}

// markNoResume is the record change that sets a run aside.
// It leaves a record that is absent or already set aside as it is.
func markNoResume(rs RunState, found bool) (RunState, error) {
	if !found || rs.NoResume {
		return rs, errLeaveRunRecord
	}
	rs.NoResume = true
	return rs, nil
}

// RetireInterruptedRuns sets aside every run under the runner's run-directory root.
// It takes the runs whose session a later start of their role would resume.
// A deliberate restart of the work, such as a goto, calls it, so the starts that re-run that work run fresh sessions.
// It returns how many runs it set aside.
// A record that does not decode is skipped.
// A failed write does not stop the scan, and every failure is returned joined once the scan ends.
func (r *Runner) RetireInterruptedRuns() (int, error) {
	if r.toldErr != nil {
		return 0, r.toldErr
	}
	root := runDirRoot(r.cfg, r.anchorPath)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("shuttle: retire interrupted runs: read run dir root: %w", err)
	}
	now, bound := r.clock.Now(), relaunchBound(r.cfg)
	retired := 0
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found || !resumableAfterInterruption(rs, now, bound) {
			continue
		}
		if err := updateRunState(runDir, markNoResume); err != nil {
			errs = append(errs, fmt.Errorf("shuttle: retire interrupted run %s: %w", runDir, err))
			continue
		}
		retired++
	}
	return retired, errors.Join(errs...)
}
