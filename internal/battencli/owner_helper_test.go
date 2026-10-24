// owner_helper_test.go holds fakeOwners, the fake the start, resume, step and window tests of every tier share, and the owners fixture built for it.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/boardengine"
)

// fakeOwners is a working in-memory stand-in for the hub's board: it serves the owners it holds and records every pull and every owner write.
type fakeOwners struct {
	// machine is the name Machine returns; machineErr, when set, replaces it.
	machine    string
	machineErr error
	// pullErr is what Pull returns.
	pullErr error
	// owners is what Read returns, and a prime write lands in its Prime.
	owners  boardengine.Owners
	readErr error
	// setErr fails every write; primeSetErr fails only the prime write.
	setErr      error
	primeSetErr error
	// lost is what a slug write reports as standing instead of this machine.
	lost map[string]string
	// primeLostTo, when set, makes a prime write lose to that machine's claim, which then stands.
	primeLostTo string

	pulls int
	// sets lists the writes in order, as "prime" and "slug:<slug>".
	sets []string
	// requests holds the owner requests in the order of sets.
	requests []boardengine.OwnerRequest
}

// seams returns the owner seams over the fake; its ParentOf reads every parent as the prime.
func (f *fakeOwners) seams() ownerSeams {
	return ownerSeams{
		Machine:  func() (string, error) { return f.machine, f.machineErr },
		Pull:     func() error { f.pulls++; return f.pullErr },
		Read:     func() (boardengine.Owners, error) { return f.owners, f.readErr },
		ParentOf: func(string) (string, error) { return "", nil },
		Set:      f.set,
	}
}

func (f *fakeOwners) set(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
	f.requests = append(f.requests, req)
	if req.Prime {
		f.sets = append(f.sets, "prime")
		if f.primeSetErr != nil {
			return boardengine.OwnerWrite{}, f.primeSetErr
		}
		if f.primeLostTo != "" {
			f.owners.Prime = f.primeLostTo
			return boardengine.OwnerWrite{Lost: map[string]string{boardengine.OwnerWritePrime: f.primeLostTo}}, nil
		}
		f.owners.Prime = req.Machine
	}
	for _, slug := range req.Slugs {
		f.sets = append(f.sets, "slug:"+slug)
	}
	if f.setErr != nil {
		return boardengine.OwnerWrite{}, f.setErr
	}
	return boardengine.OwnerWrite{Lost: f.lost}, nil
}

// ownedBy returns the owners of a hub whose prime this machine owns and that holds one task, some-slug, with the given owner and run state.
func ownedBy(prime, owner string, runHeld bool) boardengine.Owners {
	return boardengine.Owners{
		Prime:   prime,
		Entries: map[string]boardengine.OwnerEntry{"some-slug": {Owner: owner, IsTask: true, RunHeld: runHeld}},
	}
}
