// registry_test.go pins the machine registry's comparison and the adopt holds' reader.

package boardengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadsAsHere(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	registry := boardengine.Registry{
		"id-a": {Name: "osl46", Former: []string{"neve"}},
		"id-b": {Name: "box", Former: []string{"osl46-old", "shared"}},
		"id-c": {Name: "shared"},
	}
	registered := machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}
	unregistered := machinename.Identity{Name: "laptop", Former: []string{"oldlaptop"}}
	tests := []struct {
		name            string
		recorded        string
		here            machinename.Identity
		wantReadsAsHere bool
	}{
		{"OwnCurrentName", "osl46", registered, true},
		{"OwnFormerName", "neve", registered, true},
		{"AnotherMachinesCurrentName", "box", registered, false},
		{"AnotherMachinesFormerName", "osl46-old", registered, false},
		{"UnregisteredNameEqualToHere", "laptop", unregistered, true},
		{"UnregisteredFormerNameOfHere", "oldlaptop", unregistered, true},
		{"UnregisteredNameUnequalToHere", "stranger", unregistered, false},
		{"UnregisteredMachineReadsARegisteredCurrentName", "osl46", unregistered, false},
		{"EmptyName", "", registered, false},
		{"NameCurrentForOneEntryAndFormerForAnotherResolvesToTheCurrentHolder", "shared", machinename.Identity{ID: "id-c", Name: "shared"}, true},
		{"NameCurrentForOneEntryAndFormerForAnotherIsNotTheFormerHolders", "shared", machinename.Identity{ID: "id-b", Name: "box"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := registry.ReadsAsHere(tt.recorded, tt.here); got != tt.wantReadsAsHere {
				t.Errorf("ReadsAsHere(%q, %+v) = %t; want %t", tt.recorded, tt.here, got, tt.wantReadsAsHere)
			}
		})
	}
}

func TestHoldOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	adopt := boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: "osl46"}
	adoptFix := boardengine.Hold{Kind: boardengine.HoldKindAdoptFix, Machine: "osl46", FixTask: "adopt-fix-beam"}
	tests := []struct {
		name     string
		holds    []boardengine.Hold
		slug     string
		wantHold boardengine.Hold
		wantHeld bool
	}{
		{"NoHold", nil, "task", boardengine.Hold{}, false},
		{"AdoptHoldHoldsEverySlug", []boardengine.Hold{adopt}, "task", adopt, true},
		{"AdoptFixHoldHoldsAnotherSlug", []boardengine.Hold{adoptFix}, "task", adoptFix, true},
		{"AdoptFixHoldDoesNotHoldItsFixTask", []boardengine.Hold{adoptFix}, "adopt-fix-beam", boardengine.Hold{}, false},
		{"AnAdoptHoldAfterAnAdoptFixHoldStillHoldsTheFixTask", []boardengine.Hold{adoptFix, adopt}, "adopt-fix-beam", adopt, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, held := boardengine.Owners{Holds: tt.holds}.HoldOf(tt.slug)
			if held != tt.wantHeld || got.Kind != tt.wantHold.Kind || got.FixTask != tt.wantHold.FixTask {
				t.Errorf("HoldOf(%q) = %+v, %t; want %+v, %t", tt.slug, got, held, tt.wantHold, tt.wantHeld)
			}
		})
	}
}
