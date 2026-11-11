// underadopt_integration_test.go pins that UnderAdoptWatch judges a hub's adopt record against the hub's board.
// It covers the prime and a beam with no owner of its own, whose watcher reads the prime's foreign owner.

package orchcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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

// TestUnderAdoptWatch_CoversABeamWithNoOwnerOfItsOwnWhileARootAdoptIsUnfinished pins the watcher of beam b.
// b records no owner, so the watcher reads the prime's owner, another machine.
// The accessor the watcher builds must then read b covered by a root adopt, so the watcher stops nothing.
func TestUnderAdoptWatch_CoversABeamWithNoOwnerOfItsOwnWhileARootAdoptIsUnfinished(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const foreign = "other-machine"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	board := `{"version":1,"prime_owner":"` + foreign + `","entries":[` +
		`{"id":0,"slug":"b","title":"B","kind":"project","labels":["bug"],"beam":true}]}`
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}

	owner, elsewhere, err := beamOwnerElsewhere(beam)
	if err != nil || owner != foreign || !elsewhere {
		t.Fatalf("beamOwnerElsewhere(b) = %q, %v, %v; want the prime's owner %q, elsewhere", owner, elsewhere, err, foreign)
	}
	watch, slug, err := (&orchCLI{location: beam}).underAdoptWatch()
	if err != nil {
		t.Fatalf("underAdoptWatch() = %v; want nil", err)
	}
	if slug != "b" {
		t.Errorf("underAdoptWatch() slug = %q; want %q", slug, "b")
	}
	if watch.Covers(slug) {
		t.Error("Covers(b) with no adopt record = true; want false")
	}
	if err := adoptrecord.Write(h.Path, adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: []string{"root"}}); err != nil {
		t.Fatalf("adoptrecord.Write() = %v; want nil", err)
	}
	if !watch.Covers(slug) {
		t.Error("Covers(b) under a running root adopt = false; want true")
	}
}
