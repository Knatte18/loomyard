// entries_block_test.go covers the closed table of block gate checks: a check's lookup, its gate and the refusal of a name the table lacks.

package shedrecipe

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBlockGates(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("always holds with an empty reason", func(t *testing.T) {
		t.Parallel()
		check, err := LookupBlockGate(BlockGateAlways)
		if err != nil {
			t.Fatalf("LookupBlockGate(%q) error = %v; want nil", BlockGateAlways, err)
		}
		gate, err := check(Env{})
		if err != nil {
			t.Fatalf("always check error = %v; want nil", err)
		}
		if holds, reason, err := gate(); !holds || reason != "" || err != nil {
			t.Errorf("always gate() = %v, %q, %v; want true, empty reason, nil", holds, reason, err)
		}
	})

	t.Run("undiscussed holds unless the seam seeded the stored record", func(t *testing.T) {
		t.Parallel()
		check, err := LookupBlockGate(BlockGateUndiscussed)
		if err != nil {
			t.Fatalf("LookupBlockGate(%q) error = %v; want nil", BlockGateUndiscussed, err)
		}
		seamErr := errors.New("board unreadable")
		tests := []struct {
			name       string
			seeded     bool
			err        error
			wantHolds  bool
			wantReason string
			wantErr    error
		}{
			{"an entry not flagged discussed enters the block", false, nil, true, "", nil},
			{"a discussed entry's seeded record passes over the block", true, nil, false, undiscussedSkipReason, nil},
			{"a seam error is the gate's", false, seamErr, false, "", seamErr},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				gate, err := check(Env{SeedDiscussedRecord: func() (bool, error) { return tt.seeded, tt.err }})
				if err != nil {
					t.Fatalf("undiscussed check error = %v; want nil", err)
				}
				holds, reason, err := gate()
				if holds != tt.wantHolds || reason != tt.wantReason || !errors.Is(err, tt.wantErr) {
					t.Errorf("undiscussed gate() = %v, %q, %v; want %v, %q, %v", holds, reason, err, tt.wantHolds, tt.wantReason, tt.wantErr)
				}
			})
		}
	})

	t.Run("undiscussed refuses an Env with no SeedDiscussedRecord seam", func(t *testing.T) {
		t.Parallel()
		check, err := LookupBlockGate(BlockGateUndiscussed)
		if err != nil {
			t.Fatalf("LookupBlockGate(%q) error = %v; want nil", BlockGateUndiscussed, err)
		}
		if _, err := check(Env{}); err == nil || !strings.Contains(err.Error(), "Env.SeedDiscussedRecord must not be nil") {
			t.Errorf("undiscussed check(Env{}) error = %v; want it to name Env.SeedDiscussedRecord", err)
		}
	})

	t.Run("the table lists its names sorted", func(t *testing.T) {
		t.Parallel()
		if got, want := BlockGateNames(), []string{BlockGateAlways, BlockGateUndiscussed}; !reflect.DeepEqual(got, want) {
			t.Errorf("BlockGateNames() = %v; want %v", got, want)
		}
	})

	t.Run("an unknown name is refused with the way forward", func(t *testing.T) {
		t.Parallel()
		_, err := LookupBlockGate("sometimes")
		if err == nil {
			t.Fatal("LookupBlockGate(sometimes) error = nil; want a refusal")
		}
		for _, want := range []string{`unknown block gate "sometimes"`, "way forward: name one of: " + BlockGateAlways} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LookupBlockGate(sometimes) error = %v; want it to contain %q", err, want)
			}
		}
	})
}
