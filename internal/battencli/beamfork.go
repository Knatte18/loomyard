// beamfork.go decides where Worktree-Create forks a task's pair from:
// the prime for a task outside any beam, and the task's beam for a member of one.
// It also holds the refusals that stop a member's create while its beam cannot take it.

package battencli

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// beamForkParent returns the slug of the beam that slug's pair forks from, or "" for the prime.
// It returns a *battenshed.PreconditionError when that beam cannot take slug.
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
	// The ready record sits in the beam's pair, so it is read only once the pair is present.
	ready := func() (bool, error) {
		beamLocation, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, beam))
		if err != nil {
			return false, err
		}
		return pawlengine.ReadyFor(loomengine.BeamReadyPath(beamLocation), loomengine.BeamReadyLockPath(beamLocation), beam)
	}
	if err := beamForkPrecondition(slug, beam, beamEntry, beamFound, pairPresent, func() (bool, error) { return branchOnOrigin(beam) }, ready); err != nil {
		return "", err
	}
	return beam, nil
}

// beamForkPrecondition stops the create of the pair of slug when beam cannot take slug.
// The stop is a *battenshed.PreconditionError that names the way forward.
// When the beam's pair is not on this machine, the way forward names the command that brings it:
// `lym adopt` when the beam's branch is on origin, otherwise `lym beam create`.
// When the beam's own landing run holds its board entry, the way forward names the landing.
// The hold counts from Finalize on, not from Parent-Align.
// The way forward also names the cancel of slug, which lets the landing go on.
// A beam whose ready record reads as flagged takes no member:
// the record is final, so the member could not land into the beam.
// The way forward names the cancel that removes slug.
// A ready read that fails is also refused, naming the error, a retry of the run, and the cancel that removes slug.
// An error from branchOnOrigin is returned as is.
func beamForkPrecondition(slug, beam string, beamEntry boardengine.Task, beamFound, pairPresent bool, branchOnOrigin func() (bool, error), ready func() (bool, error)) error {
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
	cancelRemove := "lym batten cancel " + slug + " --remove"
	flagged, err := ready()
	if err != nil {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, whose ready record does not read: %v", slug, beam, err),
			WayForward: fmt.Sprintf("fix the read and run %q again; when the beam is flagged ready to land, run %q instead", "lym batten run "+slug, cancelRemove),
		}
	}
	if flagged {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("%s belongs to beam %s, which is flagged ready to land, so no member starts in it", slug, beam),
			WayForward: fmt.Sprintf("ready is final and %s cannot land into this beam; run %q to end %s and remove its pair", slug, cancelRemove, slug),
		}
	}
	return nil
}

// memberHoldRefusal returns the refusal of a member whose beam's run holds the beam's entry.
// It applies once the member's pair is removed.
// The member keeps its claim and stays an open member, so it still stops the beam's landing.
// The refusal names two ways forward.
// One is the member's cancel from the beam's parent worktree, which ends it and releases its claim.
// Then the project moves off the beam with `lym board upsert`, since a draft member still blocks the landing.
// After that, the beam's next land request lands the member.
// The other, once the beam's run is back at Beam-Await, is the member's run, which resumes the halted member.
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

// refuseHeldMember refuses slug when its beam's run holds the beam's entry, after removing slug's pair.
// It runs once slug's pair is made or found complete.
// The beam's accept may write its hold between the fork precondition's read and the pair's making,
// and a member that crashed after its pair was made is judged here when it is resumed.
// Either way this read or the beam's own re-check sees the other.
// The member keeps its claim, so the removal settles none.
// The removal runs under holdPrime, since it writes the hub's worktree registry.
// A slug outside a beam is never refused.
func (c *battenCLI) refuseHeldMember(ctx context.Context, prime *lymcwd.Location, slug string, holdPrime battenshed.HoldPrimeLock) error {
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
	release, err := holdPrime()
	if err != nil {
		return err
	}
	defer release()
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
