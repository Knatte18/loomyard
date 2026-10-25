//go:build tier0

// dobby_test.go pins the pieces of dobby's hub wiring that need no hub: the run-process registration of the dobby and the error text read from what a pass printed.

package beamcli

import (
	"errors"
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
