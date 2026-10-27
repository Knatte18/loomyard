// owner.go holds batten's owner checks: the seams that read and write a task's owner machine, the start check, the first-act claim of the prime, the shared owner pre-flight of a run about to be driven, and the owner keys of the status envelope.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// ownerSeams carries what the owner checks need from the hub's board and this machine.
// The zero value reads as no owner check, which only a receiver that calls no arm has.
type ownerSeams struct {
	// Machine returns this machine's name.
	Machine func() (string, error)
	// Pull brings the hub's Bolt level with origin.
	// An unreachable origin returns an error wrapping boardengine.ErrOwnerOriginUnreachable, and a Bolt that cannot be brought level returns an error naming its detail and way forward.
	Pull func() error
	// Read reads the owners the local Bolt records.
	Read func() (boardengine.Owners, error)
	// ParentOf returns the slug of the pair a slug was forked from, "" for the prime.
	ParentOf func(slug string) (string, error)
	// Set records owners and a claim in one Bolt commit and pushes it.
	Set func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
}

// hubOwnerSeams builds the owner seams over the Bolt of the hub location sits in.
// A hub with no board directory has no owners, so it gets the zero seams.
func hubOwnerSeams(location *lymcwd.Location) ownerSeams {
	boardDir := fabricengine.BoardDir(location.HubPath)
	if _, err := os.Stat(boardDir); errors.Is(err, fs.ErrNotExist) {
		return ownerSeams{}
	}
	parentOf := func(slug string) (string, error) { return hubgeom.ParentOf(location, slug) }
	return ownerSeams{
		Machine:  machinename.Name,
		Pull:     func() error { return pullBolt(boardDir) },
		Read:     func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) },
		ParentOf: parentOf,
		Set: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
			if req.ParentOf == nil {
				req.ParentOf = parentOf
			}
			return boardengine.SetOwners(boardDir, req)
		},
	}
}

// pullBolt brings the Bolt at boardDir level with its upstream through a pull-first write of nothing.
// BOARD_SKIP_GIT and BOARD_SKIP_PUSH skip the pull, as they skip the owner write's.
func pullBolt(boardDir string) error {
	skip := boardengine.ApplySkipEnv(boardengine.Config{})
	if skip.SkipGit || skip.SkipPush {
		return nil
	}
	nothing := fabricengine.BoltWrite{Message: "board: pull", Write: func() ([]string, error) { return nil, nil }}
	result, err := fabricengine.NewBolt(boardDir).PullThenCommitWritten([]fabricengine.BoltWrite{nothing}, fabricengine.NewMutations(filepath.Dir(boardDir)))
	if err != nil {
		return err
	}
	switch result.Skipped {
	case "":
		return nil
	case fabricengine.BoltSkipFetchFailed:
		return fmt.Errorf("%w: %s", boardengine.ErrOwnerOriginUnreachable, result.SkipDetail)
	default:
		return fmt.Errorf("the board could not be brought up to date: %s", result.SkipDetail)
	}
}

// OwnedElsewhereError is the refusal of a run another machine owns, which a pass exits on with its own code.
type OwnedElsewhereError struct {
	// Slug is the task whose run is owned elsewhere.
	Slug string
	// Owner is the machine that owns it.
	Owner string
	// Subject is the beam or `root` that `lym adopt` takes to move it.
	Subject string
	// text replaces the standard refusal text when set.
	text string
}

// Error returns the refusal text, naming the owner and the way forward.
func (e *OwnedElsewhereError) Error() string {
	if e.text != "" {
		return e.text
	}
	return fmt.Sprintf(
		"battencli: %q is owned by machine %q, so this machine does not start, resume or land it; way forward: run `lym adopt %s` from this machine's prime, which takes it over and resumes its run, or records the owner when no run exists here, after which `lym batten run %s` starts it",
		e.Slug, e.Owner, e.Subject, e.Slug,
	)
}

// ownedElsewhereFor is the one refusal for a run another machine owns, naming the subject `lym adopt` takes to move it, the slug when subject is empty.
func ownedElsewhereFor(slug, owner, subject string) error {
	return &OwnedElsewhereError{Slug: slug, Owner: owner, Subject: subjectOrSlug(subject, slug)}
}

// subjectOrSlug returns the adopt subject, the slug when the owner reader left it empty.
func subjectOrSlug(subject, slug string) string {
	if subject == "" {
		return slug
	}
	return subject
}

// thisMachine returns this machine's name, refusing when it cannot be read or is empty.
func (c *battenCLI) thisMachine() (string, error) {
	machine, err := c.owner.Machine()
	if err == nil && machine == "" {
		err = errors.New("the hostname is empty")
	}
	if err != nil {
		return "", fmt.Errorf("battencli: this machine's name is unknown, so ownership cannot be judged: %w; fix the hostname, then re-run lym batten run %s", err, c.slug)
	}
	return machine, nil
}

// refuseStartOwnedElsewhere is the check of a run about to start: it refuses when the slug's task is owned by another machine.
// It pulls the Bolt, which needs origin, claims the prime when the Bolt records no prime owner, and then reads the slug's effective owner from the Bolt it just pulled.
// A slug with no board entry has no owner and passes, and a receiver with no owner seams passes unchecked.
func (c *battenCLI) refuseStartOwnedElsewhere() error {
	if c.owner.Machine == nil {
		return nil
	}
	machine, err := c.thisMachine()
	if err != nil {
		return err
	}
	if err := c.owner.Pull(); err != nil {
		return c.startRefusal(err)
	}
	// A cancel that began while the pull ran claims nothing for the prime.
	if err := c.refuseCancelMarker(); err != nil {
		return err
	}
	if err := c.claimPrimeOwner(); err != nil {
		return c.startRefusal(err)
	}
	owners, err := c.owner.Read()
	if err != nil {
		return fmt.Errorf("battencli: cannot read the owner of %q from the board: %w", c.slug, err)
	}
	owner, err := owners.Effective(c.slug, c.owner.ParentOf)
	if err != nil {
		return fmt.Errorf("battencli: cannot resolve the owner of %q: %w", c.slug, err)
	}
	if boardengine.OwnedElsewhere(owner, machine) {
		subject, err := owners.AdoptSubject(c.slug, c.owner.ParentOf)
		if err != nil {
			return fmt.Errorf("battencli: cannot resolve what adopt takes for %q: %w", c.slug, err)
		}
		return ownedElsewhereFor(c.slug, owner, subject)
	}
	return nil
}

// startRefusal words the refusal of a start whose pull or prime claim failed with err.
func (c *battenCLI) startRefusal(err error) error {
	if errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
		return fmt.Errorf("battencli: a start records this machine as the task's owner, which needs origin (%w); re-run `lym batten run %s` once origin answers", err, c.slug)
	}
	return fmt.Errorf("battencli: cannot start %q: %w", c.slug, err)
}

// claimPrimeOwner records this machine as the hub's prime owner when the local Bolt records none.
// A write the replay lost leaves the other machine's claim standing, which the next read takes.
func (c *battenCLI) claimPrimeOwner() error {
	machine, err := c.thisMachine()
	if err != nil {
		return err
	}
	owners, err := c.owner.Read()
	if err != nil {
		return fmt.Errorf("read the prime owner: %w", err)
	}
	if owners.Prime != "" {
		return nil
	}
	if _, err := c.owner.Set(boardengine.OwnerRequest{Machine: machine, Prime: true}); err != nil {
		return fmt.Errorf("record this machine as the prime owner: %w", err)
	}
	return nil
}

// claimBoardEntry records this machine as slug's owner together with the Worktree-Create run claim, in one Bolt commit.
// An absent entry writes nothing, and a receiver with no owner seams writes no claim.
// An entry another machine owns is never taken, so the claim of a start cannot move a run.
func (c *battenCLI) claimBoardEntry(slug string) error {
	if c.owner.Set == nil {
		return nil
	}
	machine, err := c.thisMachine()
	if err != nil {
		return err
	}
	claim := boardengine.RunStatus(string(shedengine.StateRunning), battenrecipe.NameWorktreeCreate)
	write, err := c.owner.Set(boardengine.OwnerRequest{Machine: machine, Slugs: []string{slug}, Claim: claim})
	if err != nil {
		return fmt.Errorf("battencli: claim the board entry of %q: %w", slug, err)
	}
	if owner, lost := write.Lost[slug]; lost {
		owners, err := c.owner.Read()
		if err != nil {
			return fmt.Errorf("battencli: cannot read the owner of %q from the board: %w", slug, err)
		}
		subject, err := owners.AdoptSubject(slug, c.owner.ParentOf)
		if err != nil {
			return fmt.Errorf("battencli: cannot resolve what adopt takes for %q: %w", slug, err)
		}
		return ownedElsewhereFor(slug, owner, subject)
	}
	return nil
}

// ownerPreflight is the owner check of a run about to be driven.
// With no status file, or a status still at Worktree-Create whose pair this machine does not hold, it is the start check;
// otherwise it is the resume reconcile, which also judges the owner.
// A receiver with no owner seams runs the resume reconcile for a found status and checks nothing for an absent one.
func (c *battenCLI) ownerPreflight(found bool, status shedengine.Status) error {
	if c.owner.Machine == nil {
		if !found {
			return nil
		}
		return c.reconcileResume()
	}
	if !found {
		return c.refuseStartOwnedElsewhere()
	}
	if status.CurrentProducer == battenrecipe.NameWorktreeCreate {
		present, err := taskWorktreePresent(c.location, c.slug)
		if err != nil {
			return err
		}
		if !present {
			return c.refuseStartOwnedElsewhere()
		}
	}
	return c.reconcileResume()
}

// preflightRunOwner reads the run's status file and runs ownerPreflight over it.
func (c *battenCLI) preflightRunOwner() error {
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		return errors.New("battencli: decode status file " + c.shedPaths.StatusPath + ": " + err.Error())
	}
	return c.ownerPreflight(found, status)
}

// refuseResumeUnreachable judges a resume whose reconcile could not reach origin against the local Bolt.
// A task the local board shows owned elsewhere refuses; a task owned here, run-held with no owner or unjudgeable resumes with the Warn the caller logged, and no owner write.
func (c *battenCLI) refuseResumeUnreachable() error {
	if c.owner.Machine == nil {
		return nil
	}
	machine, err := c.thisMachine()
	if err != nil {
		return err
	}
	owners, err := c.owner.Read()
	if err != nil {
		logger.Warn("battencli: local board unreadable, resuming without judging the owner", "slug", c.slug, "cause", err)
		return nil
	}
	owner, err := owners.Effective(c.slug, c.owner.ParentOf)
	if err != nil {
		logger.Warn("battencli: local owner unresolvable, resuming without judging the owner", "slug", c.slug, "cause", err)
		return nil
	}
	if boardengine.OwnedElsewhere(owner, machine) {
		subject, err := owners.AdoptSubject(c.slug, c.owner.ParentOf)
		if err != nil {
			logger.Warn("battencli: local adopt subject unresolvable, naming the slug", "slug", c.slug, "cause", err)
			subject = c.slug
		}
		return &OwnedElsewhereError{
			Slug:    c.slug,
			Owner:   owner,
			Subject: subject,
			text:    fmt.Sprintf("battencli: the local board shows %q owned by %q; re-run `lym batten run %s` once origin answers, or run `lym adopt %s` from this machine's prime to take it over", c.slug, owner, c.slug, subject),
		}
	}
	return nil
}

// ownerExtras returns the status keys naming the slug's effective owner on the local Bolt: owner_machine and owned_here.
// A receiver with no owner seams and an unreadable board add nothing; the latter logs one Warn, so a status read never fails on the board.
func (c *battenCLI) ownerExtras() map[string]any {
	if c.owner.Machine == nil {
		return nil
	}
	machine, err := c.owner.Machine()
	if err != nil {
		logger.Warn("battencli: machine name unreadable, status omits the owner", "slug", c.slug, "cause", err)
		return nil
	}
	owners, err := c.owner.Read()
	if err != nil {
		logger.Warn("battencli: local board unreadable, status omits the owner", "slug", c.slug, "cause", err)
		return nil
	}
	owner, err := owners.Effective(c.slug, c.owner.ParentOf)
	if err != nil {
		logger.Warn("battencli: owner unresolvable, status omits the owner", "slug", c.slug, "cause", err)
		return nil
	}
	return map[string]any{
		"owner_machine": owner,
		"owned_here":    !boardengine.OwnedElsewhere(owner, machine),
	}
}
