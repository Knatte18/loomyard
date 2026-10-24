// beamfork.go decides where Worktree-Create forks a task's pair from: the prime for a task outside any beam, the task's beam for a member of one, and the refusals that stop a member's create while its beam cannot take it.

package battencli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// beamForkParent returns the beam slug's pair forks from over the local board, "" meaning the prime.
// A beam goes through beamForkPrecondition first, which stops the create with a precondition when the beam cannot take the member.
func beamForkParent(prime *lyxcwd.Location, slug string, branchOnOrigin func(beam string) (bool, error)) (string, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return "", err
	}
	beam := boardengine.ParentBeam(tasks, slug)
	if beam == "" {
		return "", nil
	}
	var beamEntry boardengine.Task
	beamFound := false
	for _, task := range tasks {
		if task.Slug == beam {
			beamEntry, beamFound = task, true
			break
		}
	}
	pairPresent, err := taskWorktreePresent(prime, beam)
	if err != nil {
		return "", err
	}
	if err := beamForkPrecondition(slug, beam, beamEntry, beamFound, pairPresent, func() (bool, error) { return branchOnOrigin(beam) }); err != nil {
		return "", err
	}
	return beam, nil
}

// beamForkPrecondition stops the create of slug's pair with a *battenshed.PreconditionError when its beam cannot take the member.
// A beam whose pair is absent here names the beam and the command that brings it: the beam's create when branchOnOrigin reports its branch off origin, its adopt when on origin.
// A beam whose board entry beamEntry is run-held by its own landing names the landing and the cancel of slug that lets the landing go on.
// An error from branchOnOrigin is returned as is.
func beamForkPrecondition(slug, beam string, beamEntry boardengine.Task, beamFound, pairPresent bool, branchOnOrigin func() (bool, error)) error {
	if !pairPresent {
		onOrigin, err := branchOnOrigin()
		if err != nil {
			return err
		}
		command := "lyx beam create " + beam
		if onOrigin {
			command = "lyx adopt " + beam
		}
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, whose pair is not on this machine", slug, beam),
			WayForward: fmt.Sprintf("run %q, then %q", command, "lyx batten run "+slug),
		}
	}
	if beamFound && boardengine.IsRunStatus(beamEntry.Status) {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, which its own landing holds (%s)", slug, beam, *beamEntry.Status),
			WayForward: fmt.Sprintf("read the landing with %q; it stops on this open member, so run %q to end this member's run and let the landing go on", "lyx batten status "+beam, "lyx batten cancel "+slug),
		}
	}
	return nil
}
