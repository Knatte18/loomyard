// owner_test.go covers batten's owner checks over fake owner seams: the start check, and the owner keys of the status envelope.
// fakeOwners is the fake the resume, step and window tests share.

package battencli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
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

// TestRefuseStartOwnedElsewhere covers the owner check of a start: an owner elsewhere refuses, an owner here, an absent entry and a receiver with no seams pass, a pull that fails refuses with its way forward, an unreadable hostname refuses, and a hub with no prime owner has this machine claim it first.
func TestRefuseStartOwnedElsewhere(t *testing.T) {
	t.Parallel()

	const slug = "some-slug"
	tests := []struct {
		name       string
		fake       fakeOwners
		wantErrors []string
		wantPulls  int
		wantSets   []string
	}{
		{
			name:       "EntryOwnedElsewhereRefuses",
			fake:       fakeOwners{machine: "here", owners: ownedBy("here", "there", false)},
			wantErrors: []string{`"there"`, "lyx adopt " + slug},
			wantPulls:  1,
		},
		{
			name:      "EntryOwnedHerePasses",
			fake:      fakeOwners{machine: "here", owners: ownedBy("here", "here", false)},
			wantPulls: 1,
		},
		{
			name:      "AbsentEntryPasses",
			fake:      fakeOwners{machine: "here", owners: boardengine.Owners{Prime: "there"}},
			wantPulls: 1,
		},
		{
			name:       "UnreachablePullRefusesNamingTheRerun",
			fake:       fakeOwners{machine: "here", pullErr: fmt.Errorf("%w: fetch failed", boardengine.ErrOwnerOriginUnreachable)},
			wantErrors: []string{"needs origin", "lyx batten run " + slug, "once origin answers"},
			wantPulls:  1,
		},
		{
			name:       "DivergedPullRefusesNamingItsDetail",
			fake:       fakeOwners{machine: "here", pullErr: errors.New("the board could not be brought up to date: diverged; run lyx board sync")},
			wantErrors: []string{"diverged; run lyx board sync"},
			wantPulls:  1,
		},
		{
			name:       "EmptyMachineNameRefusesNamingTheHostnameCause",
			fake:       fakeOwners{owners: ownedBy("here", "here", false)},
			wantErrors: []string{"the hostname is empty", "fix the hostname"},
		},
		{
			name:      "HubWithNoPrimeOwnerClaimsItForThisMachine",
			fake:      fakeOwners{machine: "here", owners: ownedBy("", "", false)},
			wantPulls: 1,
			wantSets:  []string{"prime"},
		},
		{
			name:       "LostPrimeClaimReadsTheOtherMachinesClaim",
			fake:       fakeOwners{machine: "here", owners: ownedBy("", "", false), primeLostTo: "there"},
			wantErrors: []string{`"there"`, "lyx adopt " + slug},
			wantPulls:  1,
			wantSets:   []string{"prime"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := tt.fake
			c := &battenCLI{slug: slug, owner: fake.seams()}

			err := c.refuseStartOwnedElsewhere()

			if len(tt.wantErrors) == 0 && err != nil {
				t.Fatalf("refuseStartOwnedElsewhere() = %v; want nil", err)
			}
			if len(tt.wantErrors) > 0 && err == nil {
				t.Fatalf("refuseStartOwnedElsewhere() = nil; want a refusal carrying %q", tt.wantErrors)
			}
			for _, want := range tt.wantErrors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refuseStartOwnedElsewhere() = %q; want it to carry %q", err.Error(), want)
				}
			}
			if fake.pulls != tt.wantPulls {
				t.Errorf("pulls = %d; want %d", fake.pulls, tt.wantPulls)
			}
			if strings.Join(fake.sets, ",") != strings.Join(tt.wantSets, ",") {
				t.Errorf("owner writes = %v; want %v", fake.sets, tt.wantSets)
			}
		})
	}

	t.Run("ReceiverWithNoSeamsPasses", func(t *testing.T) {
		t.Parallel()

		if err := (&battenCLI{slug: slug}).refuseStartOwnedElsewhere(); err != nil {
			t.Errorf("refuseStartOwnedElsewhere() = %v; want nil for a receiver with no owner seams", err)
		}
	})
}

// TestBattenStatus_CarriesTheOwner asserts the status envelope names the slug's effective owner as owner_machine and whether this machine holds it as owned_here, and adds neither for an unreadable board or a receiver with no owner seams.
func TestBattenStatus_CarriesTheOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fake      *fakeOwners
		wantOwner any
		wantHere  any
	}{
		{name: "OwnedHere", fake: &fakeOwners{machine: "here", owners: ownedBy("here", "here", true)}, wantOwner: "here", wantHere: true},
		{name: "OwnedElsewhere", fake: &fakeOwners{machine: "here", owners: ownedBy("here", "there", true)}, wantOwner: "there", wantHere: false},
		{name: "UnreadableBoardAddsNeither", fake: &fakeOwners{machine: "here", readErr: errors.New("board.json is corrupt")}},
		{name: "NoSeamsAddsNeither"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			if tt.fake != nil {
				c.owner = tt.fake.seams()
			}

			extras, err := c.battenStatusExtras(shedengine.Status{State: shedengine.StateRunning})

			if err != nil {
				t.Fatalf("battenStatusExtras() error = %v; want nil", err)
			}
			if got := extras["owner_machine"]; got != tt.wantOwner {
				t.Errorf("owner_machine = %v; want %v", got, tt.wantOwner)
			}
			if got := extras["owned_here"]; got != tt.wantHere {
				t.Errorf("owned_here = %v; want %v", got, tt.wantHere)
			}
		})
	}
}
