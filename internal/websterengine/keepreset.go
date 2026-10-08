// keepreset.go holds KeepResetRefusal, the error of a standalone reset that git's keep form refused.
// The CLI alone calls it: webster's engine runs no mutating git, so the keep-reset itself is made by the caller.

package websterengine

import (
	"fmt"
	"slices"
	"strings"
)

// KeepResetRefusal is the error of a standalone reset to plan's commit that git's keep form refused with cause.
// overwritten lists the uncommitted paths the move would overwrite, which cause names in git's words.
// The way forward gives one step per path, `git checkout -- <path>` for a path the run itself wrote (plan.OwnPaths) and commit or restore for any other, then re-running the reset.
// Keep leaves HEAD and every file unchanged when it refuses.
func KeepResetRefusal(plan ResetPlan, cause error, overwritten []string) error {
	var steps []string
	for _, path := range overwritten {
		if slices.Contains(plan.OwnPaths, path) {
			steps = append(steps, fmt.Sprintf("run `git checkout -- %s` to drop the run's own change", path))
		} else {
			steps = append(steps, fmt.Sprintf("commit %s, or restore it yourself if it is not worth keeping", path))
		}
	}
	if len(steps) == 0 {
		steps = append(steps, "commit or restore the uncommitted paths git names")
	}
	verb := "lyx webster reset --to " + string(plan.Target)
	if plan.Target == ResetToReportHead || plan.Target == ResetToBatchStart {
		verb += " --batch NN"
	}
	steps = append(steps, "re-run `"+verb+"`")
	return fmt.Errorf("webster: reset --to %s refused: git's keep form would overwrite uncommitted changes (%s); %s",
		plan.Target, strings.TrimSpace(cause.Error()), wayForwardSteps(steps...))
}
