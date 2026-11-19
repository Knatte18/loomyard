// landed_test.go pins the landed-commit file's place and its round trip.

package shedrun

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLandedCommit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	l := syntheticLocation(t)

	if want := filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, "shed", "slug", "landed-commit"); LandedCommitFile(l, "slug") != want {
		t.Errorf("LandedCommitFile() = %q; want %q", LandedCommitFile(l, "slug"), want)
	}
	if got, err := ReadLandedCommit(l, "slug"); err != nil || got != (LandedCommit{}) {
		t.Errorf("ReadLandedCommit() before a write = %+v, %v; want the zero LandedCommit and nil", got, err)
	}
	for _, landed := range []LandedCommit{{Commit: "abc123", EmptyDiff: true}, {Commit: "def456"}} {
		if err := WriteLandedCommit(l, "slug", landed); err != nil {
			t.Fatalf("WriteLandedCommit(%+v) = %v; want nil", landed, err)
		}
		if got, err := ReadLandedCommit(l, "slug"); err != nil || got != landed {
			t.Errorf("ReadLandedCommit() = %+v, %v; want %+v and nil", got, err, landed)
		}
	}

	// A file holding the commit alone reads as a landing whose diff was not empty.
	if err := os.WriteFile(LandedCommitFile(l, "slug"), []byte("0a1b2c\n"), 0o644); err != nil {
		t.Fatalf("write a commit-only landed-commit file: %v", err)
	}
	if got, err := ReadLandedCommit(l, "slug"); err != nil || got != (LandedCommit{Commit: "0a1b2c"}) {
		t.Errorf("ReadLandedCommit() of a commit-only file = %+v, %v; want {Commit:0a1b2c} and nil", got, err)
	}
}

// TestRecordLandedCommit pins that a landing recorded again for the same commit keeps the first record's flag.
// A different commit takes its own flag.
func TestRecordLandedCommit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		first  LandedCommit
		second LandedCommit
		want   LandedCommit
	}{
		{"SameCommitKeepsANotEmptyFlag", LandedCommit{Commit: "abc123"}, LandedCommit{Commit: "abc123", EmptyDiff: true}, LandedCommit{Commit: "abc123"}},
		{"SameCommitKeepsAnEmptyFlag", LandedCommit{Commit: "abc123", EmptyDiff: true}, LandedCommit{Commit: "abc123"}, LandedCommit{Commit: "abc123", EmptyDiff: true}},
		{"ANewCommitTakesItsOwnFlag", LandedCommit{Commit: "abc123"}, LandedCommit{Commit: "def456", EmptyDiff: true}, LandedCommit{Commit: "def456", EmptyDiff: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := syntheticLocation(t)

			for _, landed := range []LandedCommit{tt.first, tt.second} {
				if err := RecordLandedCommit(l, "slug", landed); err != nil {
					t.Fatalf("RecordLandedCommit(%+v) = %v; want nil", landed, err)
				}
			}
			if got, err := ReadLandedCommit(l, "slug"); err != nil || got != tt.want {
				t.Errorf("ReadLandedCommit() = %+v, %v; want %+v and nil", got, err, tt.want)
			}
		})
	}
}
