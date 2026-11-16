package shedworktree_test

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedworktree"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestArbiterSeam_Changes covers the arbiter seam's read of the uncommitted paths over a real checkout.
// It lists an untracked file, a modified tracked file and a removed tracked file.
// Each key is a slash-separated path relative to the worktree.
// Each file is hashed from its place under the worktree, and a removed file reads as deleted.
func TestArbiterSeam_Changes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hash := func(content string) string {
		sum := sha256.Sum256([]byte(content))
		return hex.EncodeToString(sum[:])
	}
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	worktree := hub.PrimeWorktree()
	gitkit.CommitFile(t, worktree, "kept.go", "kept", "add kept")
	gitkit.CommitFile(t, worktree, "edited.go", "before", "add edited")
	gitkit.CommitFile(t, worktree, "removed.go", "removed", "add removed")

	seam := shedworktree.ArbiterSeam(hub.Location)
	got, err := seam.Changes()
	if err != nil || len(got) != 0 {
		t.Fatalf("Changes() over a clean worktree = %v, %v; want none, nil", got, err)
	}

	if err := os.WriteFile(filepath.Join(worktree, "edited.go"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(worktree, "removed.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(worktree, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "sub", "new.go"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"edited.go":  hash("after"),
		"removed.go": "deleted",
		"sub/new.go": hash("new"),
	}
	got, err = seam.Changes()
	if err != nil || !maps.Equal(got, want) {
		t.Errorf("Changes() = %v, %v; want %v, nil", got, err, want)
	}
}
