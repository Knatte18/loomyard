// entries_block_test.go covers the closed table of block gate checks: a check's lookup, its gate and the refusal of a name the table lacks.

package shedrecipe

import (
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

	t.Run("the table lists its names sorted", func(t *testing.T) {
		t.Parallel()
		if got, want := BlockGateNames(), []string{BlockGateAlways}; !reflect.DeepEqual(got, want) {
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
