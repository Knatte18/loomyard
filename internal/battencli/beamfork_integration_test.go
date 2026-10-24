//go:build tier1

// beamfork_integration_test.go drives the Worktree-Create row over a hubforge hub with a beam and a board of its members: where each task's pair forks from, and the start guard's dirt check.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// seedBeamBoard seeds h's board with a beam project b holding tasks m and armed, a grouping project g holding task grouped, and a task left, whose local branch the caller leaves behind.
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
		{"slug": "grouped", "kind": "task", "project": "g"},
	}
	for _, entry := range entries {
		entry["title"] = entry["slug"]
		entry["labels"] = []string{"bug"}
		if _, err := board.UpsertTask(entry); err != nil {
			t.Fatalf("seed board entry %v: %v", entry["slug"], err)
		}
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("flag b as a beam: %v", err)
	}
}

// TestWorktreeCreate_ForksFromTheBeam asserts the create row forks a beam member's pair from the beam while the prime holds tracked dirt, and a task of a grouping project from the prime.
// A run armed from the beam's own location forks the member from the beam, and a local branch left from a removed pair is refused with ErrBranchExists, since batten never adopts one.
func TestWorktreeCreate_ForksFromTheBeam(t *testing.T) {
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
		if err := c.env.CreateWorktree(context.Background()); err != nil {
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
		if err := c.env.CreateWorktree(context.Background()); err != nil {
			t.Fatalf("CreateWorktree(armed): %v", err)
		}
		if got := parentOf(t, "armed"); got != "b" {
			t.Errorf("origin of armed names parent worktree %q; want b", got)
		}
	})

	t.Run("LocalBranchLeftFromARemovedPairIsRefused", func(t *testing.T) {
		c := wireForHub(t, h, "left", nil)
		err := c.env.CreateWorktree(context.Background())
		var exists *fabricengine.ErrBranchExists
		if !errors.As(err, &exists) {
			t.Fatalf("CreateWorktree(left) = %v; want ErrBranchExists", err)
		}
	})

	t.Run("TaskOfAGroupingProjectForksFromThePrime", func(t *testing.T) {
		if err := os.WriteFile(readme, clean, 0o644); err != nil {
			t.Fatal(err)
		}
		c := wireForHub(t, h, "grouped", nil)
		if err := c.env.CreateWorktree(context.Background()); err != nil {
			t.Fatalf("CreateWorktree(grouped): %v", err)
		}
		if got := parentOf(t, "grouped"); got != primeName {
			t.Errorf("origin of grouped names parent worktree %q; want the prime %q", got, primeName)
		}
	})
}
