//go:build tier0

// dobby_test.go pins the pieces of dobby's hub wiring that need no hub: the run-process registration of the dobby, the error text read from what a pass printed and the machine-named landings seam.

package beamcli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/runproc"
)

func TestRegisterDobby(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		geom  dobbyengine.Geometry
		owner string
	}{
		{"BeamRegistersUnderItsSlug", dobbyengine.Geometry{Slug: "payments"}, "payments"},
		{"PrimeRegistersUnderTheHubOwner", dobbyengine.Geometry{}, runproc.HubOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			if err := registerDobby(dir, tt.geom); err != nil {
				t.Fatalf("registerDobby() = %v; want nil", err)
			}

			records, err := runproc.List(dir, tt.owner)
			if err != nil {
				t.Fatalf("List(%q) = _, %v; want nil", tt.owner, err)
			}
			if len(records) != 1 || records[0].Role != runproc.RoleDobby {
				t.Errorf("List(%q) = %+v; want one record with role %q", tt.owner, records, runproc.RoleDobby)
			}
		})
	}
}

func TestPassErrorText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		printed string
		waitErr error
		want    string
	}{
		{"LastEnvelopeErrorWins", "noise\n{\"ok\":false,\"error\":\"first\"}\n{\"ok\":false,\"error\":\"pre-run refused\"}\n", nil, "pre-run refused"},
		{"EnvelopeWithoutAnErrorFallsBackToTheOutput", "{\"ok\":true}\n", nil, "{\"ok\":true}"},
		{"NoOutputFallsBackToTheWaitError", "", errors.New("exit status 3"), "exit status 3"},
		{"NothingAtAllIsEmpty", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := passErrorText([]byte(tt.printed), tt.waitErr); got != tt.want {
				t.Errorf("passErrorText() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestMachineLandings(t *testing.T) {
	t.Parallel()

	t.Run("PassesTheNameToTheRead", func(t *testing.T) {
		t.Parallel()
		var readFor string
		landings := machineLandings(
			func() (string, error) { return "m1", nil },
			func(machine string) ([]string, error) {
				readFor = machine
				return []string{"a", "b"}, nil
			},
		)

		got, err := landings()

		if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) || readFor != "m1" {
			t.Errorf("landings() = %v, %v with read for %q; want [a b], nil for m1", got, err, readFor)
		}
	})

	t.Run("FailingNameReadNamesTheWayForwardAndReadsNothing", func(t *testing.T) {
		t.Parallel()
		var read bool
		landings := machineLandings(
			func() (string, error) { return "", errors.New("hostname unreadable") },
			func(string) ([]string, error) {
				read = true
				return nil, nil
			},
		)

		_, err := landings()

		if err == nil || !strings.Contains(err.Error(), "hostname unreadable") || !strings.Contains(err.Error(), "give the machine a hostname") {
			t.Errorf("landings() error = %v; want one naming the read's error and giving the machine a hostname", err)
		}
		if read {
			t.Error("landings() called read; want it never called after a failed name read")
		}
	})
}
