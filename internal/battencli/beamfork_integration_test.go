// beamfork_integration_test.go drives the Worktree-Create row over a hubforge hub holding a beam and its tasks.
// It covers where each task's pair forks from, and the start guard's dirt check.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedBeamBoard seeds h's board with the beam project b, holding tasks m, armed, late, crashed and left.
// It also seeds the beam project inner under b, and the grouping project g holding task grouped.
// The beam project orphan sits under the beam far, which has no pair here.
// The task left is one whose local branch the caller leaves behind.
func seedBeamBoard(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	entries := []map[string]any{
		{"slug": "b", "kind": "project"},
		{"slug": "g", "kind": "project"},
		{"slug": "m", "kind": "task", "project": "b"},
		{"slug": "armed", "kind": "task", "project": "b"},
		{"slug": "left", "kind": "task", "project": "b"},
		{"slug": "late", "kind": "task", "project": "b"},
		{"slug": "crashed", "kind": "task", "project": "b"},
		{"slug": "inner", "kind": "project", "project": "b"},
		{"slug": "far", "kind": "project"},
		{"slug": "orphan", "kind": "project", "project": "far"},
		{"slug": "grouped", "kind": "task", "project": "g"},
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
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b", "inner", "far", "orphan"}, Beam: true}); err != nil {
		t.Fatalf("flag the beam projects: %v", err)
	}
}

// TestWorktreeCreate_ForksFromTheBeam asserts the create row forks a beam member's pair from the beam.
// The fork succeeds while the prime holds tracked dirt.
// A task of a grouping project forks from the prime.
// A run armed from the beam's own location forks the member from the beam,
// and a local branch left from a removed pair is refused with ErrBranchExists, since batten never adopts one.
// A beam project forks from its parent beam unmarked.
// When that beam's pair is absent here, the project is refused, naming its parent's create.
// A member whose beam holds once its pair is made loses the pair and is refused naming the two steps that progress.
// A member that resumes over a complete pair while its beam holds does the same.
func TestWorktreeCreate_ForksFromTheBeam(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	seedBeamBoard(t, h)
	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(h.Location))
	if err != nil {
		t.Fatalf("load the fabric config: %v", err)
	}
	mustRunGit(t, h.PrimeWorktree(), "branch", cfg.BranchPrefix+"left")

	readme := filepath.Join(h.PrimeWorktree(), "README")
	clean, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, append(clean, "dirt\n"...), 0o644); err != nil {
		t.Fatal(err)
	}

	parentOf := func(t *testing.T, slug string) string {
		t.Helper()
		origin, found, err := fabricengine.ReadOriginFor(h.Location, slug)
		if err != nil || !found {
			t.Fatalf("read the origin record of %q: found=%v err=%v", slug, found, err)
		}
		return origin.ParentWorktree
	}

	t.Run("MemberForksFromTheBeamWhileThePrimeIsDirty", func(t *testing.T) {
		c := wireForHub(t, h, "m", nil)
		if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
			t.Fatalf("CreateWorktree(m): %v", err)
		}
		if got := parentOf(t, "m"); got != "b" {
			t.Errorf("origin of m names parent worktree %q; want b", got)
		}
	})

	t.Run("RunArmedFromTheBeamStillForksTheMemberFromIt", func(t *testing.T) {
		prime, err := ResolveBattenPrime(resolvePair(t, h, "b"))
		if err != nil {
			t.Fatalf("ResolveBattenPrime(b): %v", err)
		}
		c := &battenCLI{location: prime}
		if err := c.wire(prime, "armed"); err != nil {
			t.Fatalf("wire(armed): %v", err)
		}
		if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
			t.Fatalf("CreateWorktree(armed): %v", err)
		}
		if got := parentOf(t, "armed"); got != "b" {
			t.Errorf("origin of armed names parent worktree %q; want b", got)
		}
	})

	t.Run("LocalBranchLeftFromARemovedPairIsRefused", func(t *testing.T) {
		c := wireForHub(t, h, "left", nil)
		err := c.env.CreateWorktree(context.Background(), holdNothing)
		var exists *fabricengine.ErrBranchExists
		if !errors.As(err, &exists) {
			t.Fatalf("CreateWorktree(left) = %v; want ErrBranchExists", err)
		}
	})

	t.Run("BeamProjectForksFromItsParentBeamUnmarked", func(t *testing.T) {
		c := wireForHub(t, h, "inner", nil)
		if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
			t.Fatalf("CreateWorktree(inner): %v", err)
		}
		origin, found, err := fabricengine.ReadOriginFor(h.Location, "inner")
		if err != nil || !found {
			t.Fatalf("read the origin record of inner: found=%v err=%v", found, err)
		}
		if origin.ParentWorktree != "b" || origin.Beam {
			t.Errorf("origin of inner = parent %q, beam %v; want parent b and no beam mark, since Beam-Up marks it", origin.ParentWorktree, origin.Beam)
		}
	})

	t.Run("BeamProjectWhoseParentBeamHasNoPairHereIsRefusedNamingItsCreate", func(t *testing.T) {
		c := wireForHub(t, h, "orphan", nil)
		err := c.env.CreateWorktree(context.Background(), holdNothing)
		if err == nil || !strings.Contains(err.Error(), "lym beam create far") {
			t.Fatalf("CreateWorktree(orphan) = %v; want a refusal naming `lym beam create far`", err)
		}
	})

	heldAtParentAlign := func(string) (string, bool, error) { return "running · Parent-Align", true, nil }
	wantHeldRefusal := func(t *testing.T, member string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("CreateWorktree(%s) = nil; want the member-hold refusal", member)
		}
		for _, want := range []string{"lym batten cancel " + member, "lym batten run " + member, "lym batten status b", "lym board upsert"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("CreateWorktree(%s) = %q; want it to carry %q", member, err.Error(), want)
			}
		}
		if _, statErr := os.Stat(fabricengine.WorktreePath(h.Location, member)); !os.IsNotExist(statErr) {
			t.Errorf("the pair of %s is still there after the refusal (stat error %v); want it removed", member, statErr)
		}
	}

	t.Run("MemberWhoseBeamHoldsAfterItsPairIsMadeLosesThePair", func(t *testing.T) {
		c := wireForHub(t, h, "late", nil)
		c.beamHold = heldAtParentAlign
		holds := &countingHold{}
		wantHeldRefusal(t, "late", c.env.CreateWorktree(context.Background(), holds.hold))
		// One hold for Add, and one for the removal of the held member's pair.
		if holds.takes != 2 || holds.releases != 2 {
			t.Errorf("prime lock takes/releases = %d/%d; want 2/2", holds.takes, holds.releases)
		}
	})

	t.Run("MemberResumedOverACompletePairWhileItsBeamHoldsLosesThePair", func(t *testing.T) {
		c := wireForHub(t, h, "crashed", nil)
		if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
			t.Fatalf("CreateWorktree(crashed): %v", err)
		}
		c.beamHold = heldAtParentAlign
		holds := &countingHold{}
		wantHeldRefusal(t, "crashed", c.env.CreateWorktree(context.Background(), holds.hold))
		// The complete pair skips Add, so the removal takes the only hold.
		if holds.takes != 1 || holds.releases != 1 {
			t.Errorf("prime lock takes/releases = %d/%d; want 1/1", holds.takes, holds.releases)
		}
	})

	t.Run("TaskOfAGroupingProjectForksFromThePrime", func(t *testing.T) {
		if err := os.WriteFile(readme, clean, 0o644); err != nil {
			t.Fatal(err)
		}
		c := wireForHub(t, h, "grouped", nil)
		if err := c.env.CreateWorktree(context.Background(), holdNothing); err != nil {
			t.Fatalf("CreateWorktree(grouped): %v", err)
		}
		if got := parentOf(t, "grouped"); got != primeName {
			t.Errorf("origin of grouped names parent worktree %q; want the prime %q", got, primeName)
		}
	})

	t.Run("MemberOfABeamFlaggedReadyIsRefused", func(t *testing.T) {
		beam := resolvePair(t, h, "b")
		record := pawlengine.ReadyRecord{Slug: "b", SetAt: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
		if err := pawlengine.WriteReady(loomengine.BeamReadyPath(beam), loomengine.BeamReadyLockPath(beam), record); err != nil {
			t.Fatalf("WriteReady(b): %v", err)
		}
		c := wireForHub(t, h, "left", nil)
		err := c.env.CreateWorktree(context.Background(), holdNothing)
		if err == nil || !strings.Contains(err.Error(), "lym batten cancel left --remove") {
			t.Fatalf("CreateWorktree(left) = %v; want a refusal naming `lym batten cancel left --remove`", err)
		}
	})
}
