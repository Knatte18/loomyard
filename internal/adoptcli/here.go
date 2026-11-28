// here.go resolves `lym adopt --here` to the adopt it runs, or to the envelope it ends with when it runs none.
// The call's subject is the worktree it runs in: `root` in the prime, the beam in a beam.

package adoptcli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// The outcomes of a `lym adopt --here` call that runs no adopt.
const (
	// outcomeOwnedHere ends a call whose subject no other machine owns.
	outcomeOwnedHere = "owned_here"
	// outcomeNoSubject ends a call from a worktree that is neither the prime nor an open beam project.
	outcomeNoSubject = "no_subject"
)

// hereRequest resolves a `lym adopt --here` call to the request it runs, or to the envelope fields it ends with.
// call carries the call's flags, and a subject, `root` or --all in it is refused.
// An unfinished record that takes the call's worktree up gives the request:
// the record's own, with the call's flags merged in, so the call follows or resumes it whoever the owner reads as.
// Otherwise Bolt is pulled and the subject's owner is read from it.
// A pull that cannot bring Bolt up to date reads the local Bolt, and its detail becomes a warning of the end.
// A subject no other machine owns ends `owned_here`, and any other gives the request that adopts the subject alone.
// A worktree that is neither the prime nor an open beam project ends `no_subject`.
func hereRequest(deps callerDeps, call request) (request, map[string]any, error) {
	if len(call.Slugs) > 0 || call.Root || call.All {
		return request{}, nil, errors.New("lym adopt: --here adopts the subject of the worktree it runs in, so it takes no subject, `root` or --all: drop them, or drop --here")
	}
	steps := deps.Steps
	owners, err := steps.ReadOwners()
	if err != nil {
		return request{}, nil, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	subject, slug, found := hereSubject(owners, steps.PrimeName, deps.Worktree)
	if !found {
		return request{}, map[string]any{"outcome": outcomeNoSubject, "worktree": deps.Worktree}, nil
	}

	record, recorded, err := deps.ReadRecord()
	if err != nil {
		return request{}, nil, err
	}
	if recorded {
		takesUp, err := record.TakesUp(owners, slug, steps.ParentOf)
		if err != nil {
			return request{}, nil, fmt.Errorf("lym adopt: judge whether the unfinished adopt of %v takes up %s: %w", record.Subjects, subject, err)
		}
		if takesUp {
			merged, _ := mergeRequest(record.Request, call.recorded())
			return requestOf(merged), nil, nil
		}
	}

	skipped, err := deps.PullBolt()
	if err != nil {
		return request{}, nil, fmt.Errorf("lym adopt: pull Bolt to read the owner of %s: %w", subject, err)
	}
	if owners, err = steps.ReadOwners(); err != nil {
		return request{}, nil, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	here, err := steps.Machine()
	if err != nil {
		return request{}, nil, fmt.Errorf("lym adopt: read this machine's name: %w", err)
	}
	owner := owners.Prime
	if slug != "" {
		if owner, err = owners.Effective(slug, steps.ParentOf); err != nil {
			return request{}, nil, fmt.Errorf("lym adopt: read the owner of %s: %w", subject, err)
		}
	}
	if !boardengine.OwnedElsewhere(owner, here) {
		fields := map[string]any{"outcome": outcomeOwnedHere, "subject": subject}
		if skipped != "" {
			fields["warnings"] = []string{"Bolt was not brought up to date, so the owner was read from the local Bolt: " + skipped}
		}
		return request{}, fields, nil
	}
	req := call
	if slug == "" {
		req.Root = true
	} else {
		req.Slugs = []string{slug}
	}
	return req, nil, nil
}

// hereSubject returns the subject of the worktree named worktree and its slug, empty for the prime named primeName.
// It reports false for a worktree that is neither the prime nor an open beam project.
func hereSubject(owners boardengine.Owners, primeName, worktree string) (subject, slug string, found bool) {
	if worktree == primeName {
		return boardengine.RootSubject, "", true
	}
	entry := owners.Entries[worktree]
	if !entry.IsProject || !entry.Beam || entry.Done {
		return "", "", false
	}
	return worktree, worktree, true
}
