//go:build tier0

// landed_test.go pins the landed-commit file's place and its round trip.

package shedrun

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

func TestLandedCommit(t *testing.T) {
	t.Parallel()
	l := syntheticLocation(t)

	if want := filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, "shed", "slug", "landed-commit"); LandedCommitFile(l, "slug") != want {
		t.Errorf("LandedCommitFile() = %q; want %q", LandedCommitFile(l, "slug"), want)
	}
	if got, err := ReadLandedCommit(l, "slug"); err != nil || got != "" {
		t.Errorf("ReadLandedCommit() before a write = %q, %v; want empty and nil", got, err)
	}
	for _, commit := range []string{"abc123", "def456"} {
		if err := WriteLandedCommit(l, "slug", commit); err != nil {
			t.Fatalf("WriteLandedCommit(%q) = %v; want nil", commit, err)
		}
		if got, err := ReadLandedCommit(l, "slug"); err != nil || got != commit {
			t.Errorf("ReadLandedCommit() = %q, %v; want %q and nil", got, err, commit)
		}
	}
}
