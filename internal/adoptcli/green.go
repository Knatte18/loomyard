// green.go is the green end of the adopt check.
// It removes the subject's fix task; it clears no hold and starts no run.

package adoptcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// greenDeps holds the seams of one subject's green end:
// the subject, its worktree name (the prime's for `root`) and its project (none for `root`).
// Rerun is the command that checks the hold's subjects again, which the end's way forward lines name.
// Tasks reads the board's entries.
// FrontDeps are the seams a front task's removal reads.
type greenDeps struct {
	Subject   string
	Rerun     string
	Worktree  string
	Project   string
	Tasks     func() ([]boardengine.Task, error)
	FrontDeps fronttask.Deps
}

// greenEnd runs the green end of the subject's adopt check.
// It removes the subject's fix task and returns a line when the removal fails, naming its way forward.
// It clears no hold and starts no run: the hold check does both once every subject is green.
func greenEnd(deps greenDeps) []string {
	return removeFixTask(deps)
}

// removeFixTask removes the subject's adopt-check front task, in whatever status it stands not done.
// It returns a line when the removal fails.
// The removal ends a running task's run first,
// and the line carries the failing step's error with the command that finishes it by hand.
func removeFixTask(deps greenDeps) []string {
	tasks, err := deps.Tasks()
	if err != nil {
		return []string{fmt.Sprintf("the board was not read (%v), so the fix task of %s was not looked for; way forward: re-run %s", err, deps.Subject, deps.Rerun)}
	}
	task, found := fronttask.FrontOf(tasks, deps.Project, fronttask.ActionAdoptCheck)
	if !found {
		return nil
	}
	if err := fronttask.Remove(deps.FrontDeps, task.Slug); err != nil {
		removal := err.(*fronttask.RemoveError)
		return []string{fmt.Sprintf("the fix task %s of %s was not removed: the %s step failed (%v); way forward: %s", task.Slug, deps.Subject, removal.Step, removal.Err, removal.HandRemoval())}
	}
	return nil
}

// hubGreenDeps returns the greenDeps for the green end of subject, bound to the hub of prime, whose re-run is rerun.
func hubGreenDeps(prime *lymcwd.Location, subject, rerun string) (greenDeps, error) {
	frontDeps, err := fronttask.HubDeps(prime)
	if err != nil {
		return greenDeps{}, err
	}
	boardDir := fabriclayout.BoardDir(prime.HubPath)
	worktree, project := prime.WorktreeName, ""
	if subject != boardengine.RootSubject {
		worktree, project = subject, subject
	}
	return greenDeps{
		Subject:   subject,
		Rerun:     rerun,
		Worktree:  worktree,
		Project:   project,
		Tasks:     func() ([]boardengine.Task, error) { return boardengine.ReadTasks(boardDir) },
		FrontDeps: frontDeps,
	}, nil
}
