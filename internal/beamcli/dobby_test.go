// dobby_test.go pins the dobby's hub wiring that needs no hub, and the recipe count-to-duration fallback.

package beamcli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLandingsPast(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	line := []fabricengine.LineCommit{
		{SHA: "aaaaaaa1111", Message: "Add widgets\n\nBody."},
		{SHA: "bbbbbbb2222", Message: "beam: bookkeeping", RecordsOnly: true},
		{SHA: "ccccccc3333", Message: "Add gadgets"},
	}
	tests := []struct {
		name string
		from string
		want []string
	}{
		{"CommitsAfterFromAreListedOldestFirstWithoutRecordsOnlyOnes", "aaaaaaa1111", []string{"ccccccc Add gadgets"}},
		{"FromAtTheHeadListsNone", "ccccccc3333", nil},
		{"FromNotOnTheLineListsTheWholeLine", "elsewhere", []string{"aaaaaaa Add widgets", "ccccccc Add gadgets"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := landingsPast(line, tt.from); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("landingsPast(line, %q) = %q; want %q", tt.from, got, tt.want)
			}
		})
	}
}

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
