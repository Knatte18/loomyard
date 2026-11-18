// registrysetter_integration_test.go covers RegisterMachine over a real board repository and a bare origin.
// It adds a second clone for the clashing registration.

package boardengine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRegisterMachine(t *testing.T) {
	tiergate.Require(t, 1)
	// Not parallel at the top: registering the replayer is process-global, as in TestSetOwners.
	fabricengine.RegisterBoltReplayer(FabricReplay{})

	t.Run("RegistrationIsOnOriginWhenItReturnsAndARepeatWritesNothing", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		reg := Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46", Former: []string{"neve"}}}

		wrote, err := RegisterMachine(f.boardDir, reg)
		if err != nil || !wrote {
			t.Fatalf("RegisterMachine() = %t, %v; want true, nil", wrote, err)
		}
		want := Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}}
		if got := f.originOwners(t).Registry; !reflect.DeepEqual(got, want) {
			t.Errorf("origin registry = %v; want %v", got, want)
		}

		head := gitkit.RevParse(t, f.bare, "main")
		wrote, err = RegisterMachine(f.boardDir, reg)
		if err != nil || wrote {
			t.Fatalf("RegisterMachine() again = %t, %v; want false, nil", wrote, err)
		}
		if got := gitkit.RevParse(t, f.bare, "main"); got != head {
			t.Errorf("origin head = %s after a repeat; want it to stay %s", got, head)
		}
	})

	t.Run("AClashingRegistrationFromAnotherCloneIsRefusedAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		if _, err := RegisterMachine(f.boardDir, Registration{Identity: machinename.Identity{ID: "id-a", Name: "osl46"}}); err != nil {
			t.Fatalf("RegisterMachine() first registration error = %v; want nil", err)
		}
		head := gitkit.RevParse(t, f.bare, "main")

		wrote, err := RegisterMachine(f.otherDir, Registration{Identity: machinename.Identity{ID: "id-b", Name: "osl46"}})
		var refusal *RegistrationError
		if !errors.As(err, &refusal) || wrote {
			t.Fatalf("RegisterMachine() = %t, %v; want false and a *RegistrationError", wrote, err)
		}
		if refusal.Machine != "osl46" {
			t.Errorf("refusal machine = %q; want osl46", refusal.Machine)
		}
		if got := gitkit.RevParse(t, f.bare, "main"); got != head {
			t.Errorf("origin head = %s after a refusal; want it to stay %s", got, head)
		}
	})

	t.Run("ARenameMovesTheOldNameToFormerOnOrigin", func(t *testing.T) {
		t.Parallel()
		f := newOwnerFixture(t, "neve")
		// The machine file of a renamed machine lists the old name among its names, as `lym machine name` writes it.
		for _, identity := range []machinename.Identity{
			{ID: "id-a", Name: "neve"},
			{ID: "id-a", Name: "osl46", Former: []string{"neve"}},
		} {
			if _, err := RegisterMachine(f.boardDir, Registration{Identity: identity}); err != nil {
				t.Fatalf("RegisterMachine(%s) error = %v; want nil", identity.Name, err)
			}
		}
		want := Registry{"id-a": {Name: "osl46", Former: []string{"neve"}}}
		if got := f.originOwners(t).Registry; !reflect.DeepEqual(got, want) {
			t.Errorf("origin registry = %v; want %v", got, want)
		}
	})
}
