// underadopt_integration_test.go pins that UnderAdoptWatch judges a hub's adopt record against the hub's board.

package orchcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestUnderAdoptWatch_CoversThePrimeWhileARootAdoptIsUnfinished(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	watch := UnderAdoptWatch(h.Location)

	if watch.Covers("") {
		t.Error("Covers(prime) with no adopt record = true; want false")
	}
	if err := adoptrecord.Write(h.Path, adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: []string{"root"}}); err != nil {
		t.Fatalf("adoptrecord.Write() = %v; want nil", err)
	}
	if !watch.Covers("") {
		t.Error("Covers(prime) under a running root adopt = false; want true")
	}
	if err := adoptrecord.Write(h.Path, adoptrecord.Record{State: adoptrecord.StateDone, Subjects: []string{"root"}}); err != nil {
		t.Fatalf("adoptrecord.Write() = %v; want nil", err)
	}
	if watch.Covers("") {
		t.Error("Covers(prime) with the root adopt done = true; want false")
	}
}
