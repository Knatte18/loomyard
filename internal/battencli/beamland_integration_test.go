//go:build tier1

// beamland_integration_test.go drives a beam's landing prerequisites over a hubforge hub: the start guard and Worktree-Create's skip for beams made the two ways, the unreadable-pair refusal, a sub-beam under an open beam, and the landing refusal over a beam with members.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

func TestBeamLanding_ReachesWorktreeCreateSkip(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}

	// made stands for a beam made by create's steps.
	// prom stands for a task made a beam by promote's steps.
	// gone stands for a beam whose origin record is lost.
	// c stands for a sub-beam of the beam outer, which holds the grouping project g and its task t.
	for _, slug := range []string{"made", "outer", "gone"} {
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true, Beam: true})
	}
	hubforge.AddPairWith(t, h, "c", fabricengine.AddOptions{SkipPush: true, Beam: true, Parent: "outer"})
	hubforge.AddPairWith(t, h, "prom", fabricengine.AddOptions{SkipPush: true})
	rec := fabricengine.NewMutations(h.Path)
	if _, err := fabricengine.MarkBeam(rec, h.Location, "prom"); err != nil {
		t.Fatalf("mark prom: %v", err)
	}
	if _, err := fabricengine.ResetRecordsToParent(rec, h.Location, "prom"); err != nil {
		t.Fatalf("reset the records of prom: %v", err)
	}
	if err := os.Remove(fabricengine.OriginRecordPathFor(h.Location, "gone")); err != nil {
		t.Fatalf("lose the origin record of gone: %v", err)
	}

	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	entries := []map[string]any{
		{"slug": "made", "kind": "project"},
		{"slug": "outer", "kind": "project"},
		{"slug": "gone", "kind": "project"},
		{"slug": "c", "kind": "project", "project": "outer"},
		{"slug": "g", "kind": "project", "project": "outer"},
		{"slug": "t", "kind": "task", "project": "g"},
		{"slug": "prom", "kind": "task"},
	}
	for _, entry := range entries {
		entry["title"] = entry["slug"]
		entry["labels"] = []string{"bug"}
		if _, err := board.UpsertTask(entry); err != nil {
			t.Fatalf("seed board entry %v: %v", entry["slug"], err)
		}
		// A new task is a draft, which a fresh run refuses.
		if entry["kind"] == "task" {
			if err := board.SetStatus(entry["slug"].(string), nil); err != nil {
				t.Fatalf("release board task %v: %v", entry["slug"], err)
			}
		}
	}
	for _, request := range []boardengine.OwnerRequest{
		{Machine: machine, Slugs: []string{"made", "outer", "gone", "c"}, Beam: true},
		{Machine: machine, Slugs: []string{"prom"}, Beam: true, ToProject: true},
	} {
		if _, err := boardengine.SetOwners(h.BoardDir(), request); err != nil {
			t.Fatalf("record the beams %v: %v", request.Slugs, err)
		}
	}

	readme := filepath.Join(h.PrimeWorktree(), "README")
	clean, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, append(clean, "dirt\n"...), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, slug := range []string{"made", "prom", "c"} {
		t.Run(slug+"PassesTheGuardWithThePrimeDirtyAndReachesTheSkip", func(t *testing.T) {
			if err := hubStartGuard(h.Location, slug).refuse(slug, "run", shedengine.Status{}, false, false); err != nil {
				t.Fatalf("start guard for %s = %v; want it passed", slug, err)
			}
			c := wireForHub(t, h, slug, nil)
			if err := c.env.CreateWorktree(context.Background()); err != nil {
				t.Fatalf("CreateWorktree(%s) = %v; want the skip over the complete pair", slug, err)
			}
		})
	}

	t.Run("BeamWhoseOriginRecordIsLostIsRefusedNamingReconcile", func(t *testing.T) {
		err := hubStartGuard(h.Location, "gone").refuse("gone", "run", shedengine.Status{}, false, false)
		if err == nil || !strings.Contains(err.Error(), "lyx fabric reconcile") || strings.Contains(err.Error(), "cancel") {
			t.Fatalf("start guard for gone = %v; want a refusal naming `lyx fabric reconcile` and no cancel", err)
		}
	})

	t.Run("BeamWithMembersIsRefusedNamingEachOpenEntry", func(t *testing.T) {
		err := BeamLandRefusal(h.Location, "outer")
		if err == nil {
			t.Fatal("BeamLandRefusal(outer) = nil; want a refusal")
		}
		for _, want := range []string{"the open entry t ", "the open entry c ", "the pair c "} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("BeamLandRefusal(outer) = %q; want it to carry %q", err.Error(), want)
			}
		}
	})
}
