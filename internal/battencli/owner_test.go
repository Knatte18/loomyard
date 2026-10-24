//go:build tier0

// owner_test.go covers batten's owner checks over fake owner seams: the start check, and the owner keys of the status envelope.

package battencli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestRefuseStartOwnedElsewhere covers the owner check of a start: an owner elsewhere refuses, so does an unspawned member of a beam whose beam another machine owns, an owner here, an absent entry and a receiver with no seams pass, a pull that fails refuses with its way forward, an unreadable hostname refuses, and a hub with no prime owner has this machine claim it first.
func TestRefuseStartOwnedElsewhere(t *testing.T) {
	t.Parallel()

	const slug = "some-slug"
	tests := []struct {
		name       string
		fake       fakeOwners
		wantErrors []string
		wantPulls  int
		wantSets   []string
		// markerAfterPull makes the cancel marker read absent before the pull and present once it ran.
		markerAfterPull bool
		// parent is the slug the owner seams' ParentOf answers for every entry, "" keeping the fake's prime.
		parent string
	}{
		{
			name: "UnspawnedBeamMemberReadsItsBeamsOwnerElsewhere",
			fake: fakeOwners{machine: "here", owners: boardengine.Owners{
				Prime: "here",
				Entries: map[string]boardengine.OwnerEntry{
					slug: {IsTask: true},
					"b":  {Owner: "there", IsProject: true, Beam: true},
				},
			}},
			parent:     "b",
			wantErrors: []string{`"there"`, "lyx adopt " + slug},
			wantPulls:  1,
		},
		{
			name:            "CancelMarkerAppearingDuringThePullRefusesBeforeTheClaim",
			fake:            fakeOwners{machine: "here", owners: ownedBy("", "here", false)},
			wantErrors:      []string{"a cancel of some-slug is under way", "way forward"},
			wantPulls:       1,
			markerAfterPull: true,
		},
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
			seams := fake.seams()
			if tt.parent != "" {
				seams.ParentOf = func(string) (string, error) { return tt.parent, nil }
			}
			c := &battenCLI{slug: slug, owner: seams}
			if tt.markerAfterPull {
				c.guard.Marker = func() (cancelMarkerReading, error) {
					return cancelMarkerReading{Present: fake.pulls > 0, WriterAlive: true, Marker: cancelMarker{PID: 4242}}, nil
				}
			}

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
