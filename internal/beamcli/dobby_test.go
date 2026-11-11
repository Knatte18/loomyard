// dobby_test.go pins the dobby's hub wiring that needs no hub, and the recipe count-to-duration fallback.

package beamcli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRegisterDobby(t *testing.T) {
	tiergate.Require(t, 0)
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

func TestTestMapRunning(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// owner and role name the one live record registered; an empty role registers none.
		owner, role string
		want        bool
	}{
		{"NoRecordReadsAsNotRunning", runproc.HubOwner, "", false},
		{"AHubRecordOfAnotherRoleReadsAsNotRunning", runproc.HubOwner, runproc.RoleSlackRelay, false},
		{"ATestMapRecordUnderARunsSlugReadsAsNotRunning", "payments", runproc.RoleTestMap, false},
		{"ALiveTestMapRecordUnderTheHubOwnerReadsAsRunning", runproc.HubOwner, runproc.RoleTestMap, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.role != "" {
				if err := runproc.Register(dir, tt.owner, runproc.SelfRecord(tt.role)); err != nil {
					t.Fatalf("Register() = %v; want nil", err)
				}
			}

			got, err := testMapRunning(dir)
			if err != nil || got != tt.want {
				t.Errorf("testMapRunning() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

func TestPassErrorText(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("PassesTheCurrentAndFormerNamesToTheRead", func(t *testing.T) {
		t.Parallel()
		var readFor []string
		landings := machineLandings(
			func() (string, []string, error) { return "m1", []string{"m0"}, nil },
			func(machines []string) ([]string, error) {
				readFor = machines
				return []string{"a", "b"}, nil
			},
		)

		got, err := landings()

		if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) || !reflect.DeepEqual(readFor, []string{"m1", "m0"}) {
			t.Errorf("landings() = %v, %v with read for %q; want [a b], nil for [m1 m0]", got, err, readFor)
		}
	})

	t.Run("FailingNameReadKeepsItsWayForwardAndReadsNothing", func(t *testing.T) {
		t.Parallel()
		var read bool
		landings := machineLandings(
			func() (string, []string, error) {
				return "", nil, errors.New("name unreadable; way forward: run `lym machine name <name>`")
			},
			func([]string) ([]string, error) {
				read = true
				return nil, nil
			},
		)

		_, err := landings()

		if err == nil || !strings.Contains(err.Error(), "name unreadable") || !strings.Contains(err.Error(), "lym machine name") || strings.Contains(err.Error(), "hostname") {
			t.Errorf("landings() error = %v; want one keeping the read's error and way forward and adding no hostname fix", err)
		}
		if read {
			t.Error("landings() called read; want it never called after a failed name read")
		}
	})
}

func TestHoldReexec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, held := range []bool{true, false} {
		checks := 0
		check := holdReexec(func() bool { return held }, func(func(), func(error)) { checks++ })

		check(nil, nil)

		if want := map[bool]int{true: 0, false: 1}[held]; checks != want {
			t.Errorf("held %v: the binary check ran %d times; want %d", held, checks, want)
		}
	}
}

func TestDurationFromCount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const fallback = 30 * time.Minute
	tests := []struct {
		name  string
		value any
		want  time.Duration
	}{
		{"PositiveInt", 5, 5 * time.Minute},
		{"PositiveInt64", int64(7), 7 * time.Minute},
		{"PositiveFloat64", 1.5, 90 * time.Second},
		{"ZeroFallsBack", 0, fallback},
		{"NegativeFallsBack", -3, fallback},
		{"ZeroFloatFallsBack", 0.0, fallback},
		{"AbsentFallsBack", nil, fallback},
		{"NonNumberFallsBack", "5", fallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := durationFromCount(tt.value, time.Minute, fallback); got != tt.want {
				t.Errorf("durationFromCount(%v) = %v; want %v", tt.value, got, tt.want)
			}
		})
	}
}
