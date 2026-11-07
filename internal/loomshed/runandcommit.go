// runandcommit.go implements runAndCommit, the run-a-session-then-commit step that the write producers share.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// runAndCommit calls session once and commits the outputs it produced, returning name in every error it raises.
// The session's triple is returned as is when its error is non-nil or its pointer's Path is empty.
// Then no artifact exists to commit.
// Otherwise commit runs for Done and for a gate-failed Stuck alike, so a halted run leaves its work committed,
// and the triple is returned with a nil error.
// A nil commit is refused by name rather than dereferenced;
// a commit error is returned, never made Stuck, because re-running the session cannot fix a git or filesystem fault.
func runAndCommit(ctx context.Context, name string, session shedengine.ShedProducer, commit func() error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	outcome, pointer, err := session.Call(ctx)
	if err != nil || pointer.Path == "" {
		return outcome, pointer, err
	}

	if commit == nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: no commit seam wired; the produced output would never be committed", name)
	}
	if err := commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w; way forward: the fault is transient, re-step the %s row", name, err, name)
	}

	return outcome, pointer, nil
}
