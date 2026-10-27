// beamfork.go decides where Worktree-Create forks a task's pair from: the prime for a task outside any beam, the task's beam for a member of one, and the refusals that stop a member's create while its beam cannot take it.

package battencli

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// beamForkParent returns the beam slug's pair forks from over the local board, "" meaning the prime.
// A beam goes through beamForkPrecondition first, which stops the create with a precondition when the beam cannot take the member.
func beamForkParent(prime *lymcwd.Location, slug string, branchOnOrigin func(beam string) (bool, error)) (string, error) {
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
// A beam whose board entry beamEntry is held by its own landing run, from Parent-Align on, names the landing and the cancel of slug that lets the landing go on.
// An error from branchOnOrigin is returned as is.
func beamForkPrecondition(slug, beam string, beamEntry boardengine.Task, beamFound, pairPresent bool, branchOnOrigin func() (bool, error)) error {
	if !pairPresent {
		onOrigin, err := branchOnOrigin()
		if err != nil {
			return err
		}
		command := "lym beam create " + beam
		if onOrigin {
			command = "lym adopt " + beam
		}
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, whose pair is not on this machine", slug, beam),
			WayForward: fmt.Sprintf("run %q, then %q", command, "lym batten run "+slug),
		}
	}
	if beamFound && boardengine.RunHolds(beamEntry) {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, which its own landing holds (%s)", slug, beam, *beamEntry.Status),
			WayForward: fmt.Sprintf("read the landing with %q; it stops on this open member, so run %q to end this member's run and let the landing go on", "lym batten status "+beam, "lym batten cancel "+slug),
		}
	}
	return nil
}

// memberHoldRefusal is the refusal of a member whose beam's own run holds the beam's entry once the member's pair was made, and whose pair was removed.
// It names the two steps that progress from there, since the member keeps its claim and stays an open member that stops the beam's landing:
// the member's cancel from the beam's parent worktree, which ends it and releases its claim, then moving its project off the beam, since a draft member still blocks the landing and the beam's next land request lands it;
// or, once the beam's run is back at Beam-Await, the member's run, which resumes the halted member.
func memberHoldRefusal(slug, beam, status string) error {
	return &battenshed.PreconditionError{
		Unmet: fmt.Sprintf("%s belongs to beam %s, which its own landing holds (%s), so the pair of %s was removed", slug, beam, status, slug),
		WayForward: fmt.Sprintf("run %q in the beam's parent worktree to end %s and release its claim, then move its project off the beam with `lym board upsert`, since a draft member still blocks the landing, after which the beam's next land request lands it; or, once %q shows the beam's run back at Beam-Await, run %q, which resumes the halted member",
			"lym batten cancel "+slug, slug, "lym batten status "+beam, "lym batten run "+slug),
	}
}

// readBeamHold reads beam's board entry over the hub of prime: its status, and whether its run holds the entry.
// A beam with no entry reads as not held.
func readBeamHold(prime *lymcwd.Location, beam string) (status string, held bool, err error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return "", false, err
	}
	for _, task := range tasks {
		if task.Slug != beam {
			continue
		}
		if task.Status != nil {
			status = *task.Status
		}
		return status, boardengine.RunHolds(task), nil
	}
	return "", false, nil
}

// refuseHeldMember reads slug's beam again once slug's pair is made, or found complete, and removes the pair and refuses with memberHoldRefusal when the beam's run holds its entry.
// The beam's accept may write its hold between the fork precondition's read and the pair's making, and a member that crashed after the pair was made is judged here when it is resumed; either way this read or the beam's own re-check sees the other.
// The member keeps its claim, so the removal settles none.
// A slug outside a beam is never refused.
func (c *battenCLI) refuseHeldMember(ctx context.Context, prime *lymcwd.Location, slug string) error {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return err
	}
	beam := boardengine.ParentBeam(tasks, slug)
	if beam == "" {
		return nil
	}
	read := c.beamHold
	if read == nil {
		read = func(beam string) (string, bool, error) { return readBeamHold(prime, beam) }
	}
	status, held, err := read(beam)
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	td, err := pairteardown.New(prime)
	if err != nil {
		return err
	}
	res, err := td.Run(ctx, pairteardown.Request{Slug: slug, Remote: true, LeaveClaim: true})
	if err := finishRemoval(res.Removal, err, slug); err != nil {
		return err
	}
	return memberHoldRefusal(slug, beam, status)
}

// anchorFromSeed reads the anchor the first pass of slug recorded in the run's seed, empty when it recorded none.
func anchorFromSeed(prime *lymcwd.Location, slug string) (string, error) {
	seed, found, err := shedrun.ReadSeed(prime, slug)
	if err != nil || !found {
		return "", err
	}
	return seed.Params[anchorParam], nil
}
