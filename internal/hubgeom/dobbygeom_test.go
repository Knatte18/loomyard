//go:build tier0

// dobbygeom_test.go pins the paths DobbyGeometry tells dobby for the prime and for a beam, over a temp layout with no git.

package hubgeom

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// dobbyHub lays out a hub of the prime "some-repo" and the beams "a-beam" and "other-beam" in a temp directory, with no git.
// The hub holds no board marker, so a beam's own anchor comes only from the marker in its records sibling.
func dobbyHub(t *testing.T) (hub string, prime *lyxcwd.Location) {
	t.Helper()

	hub = filepath.Join(t.TempDir(), "some-hub-LYXHUB")
	prime = &lyxcwd.Location{RepoName: "some-repo", HubPath: hub, WorktreeName: "some-repo", AnchorRel: "sub"}
	for name, marker := range map[string]string{"some-repo": "", "a-beam": "sub", "other-beam": "other"} {
		if err := os.MkdirAll(filepath.Join(hub, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if marker == "" {
			continue
		}
		sibling := fabricengine.RecordsWorktreePath(prime, name)
		if err := os.MkdirAll(sibling, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sibling, lyxcwd.AnchorFileName), []byte(marker+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return hub, prime
}

// standMove writes a move record on the records worktree of the worktree named worktree and returns its path.
func standMove(t *testing.T, prime *lyxcwd.Location, worktree string) string {
	t.Helper()

	records := fabricengine.RecordsWorktreePath(prime, worktree)
	if err := os.MkdirAll(fabricengine.RecordsLockDirPath(records), 0o755); err != nil {
		t.Fatal(err)
	}
	record := fabricengine.MoveRecordPath(records)
	if err := os.WriteFile(record, []byte(`{"a":".","b":"sub","kind":"move","pid":4242}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return record
}

// TestDobbyGeometry asserts the lock, state and marker paths lie under the named worktree's `.lyx/beam/` directory at its own anchor, the slug is empty for the prime, the batten runs directory lies under the prime, and a worktree with no directory is an error naming it.
// It also asserts PawlPaths puts the pawl run's three files in that same `.lyx/beam/` directory at the beam's own anchor, a beam anchored under a sub-directory included.
// It also asserts the move records that stop a dobby: the prime's alone for the prime and the beam's own and then the prime's for a beam, and that DobbyStart reads a standing record the same way.
func TestDobbyGeometry(t *testing.T) {
	t.Parallel()

	hub, prime := dobbyHub(t)

	tests := []struct {
		worktree   string
		wantSlug   string
		wantAnchor string
	}{
		{"some-repo", "", "sub"},
		{"a-beam", "a-beam", "sub"},
		{"other-beam", "other-beam", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.worktree, func(t *testing.T) {
			t.Parallel()

			got, err := DobbyGeometry(prime, tt.worktree)
			if err != nil {
				t.Fatalf("DobbyGeometry(%q) = %v; want nil", tt.worktree, err)
			}

			root := filepath.Join(hub, tt.worktree)
			beamDir := filepath.Join(root, tt.wantAnchor, lyxdirs.DotLyxDirName, lyxdirs.BeamDirName)
			if got.HubRoot != hub || got.BoardDir != fabricengine.BoardDir(hub) || got.PrimeAnchor != prime.AnchorPath() {
				t.Errorf("DobbyGeometry(%q) hub paths = %q, %q, %q; want %q, %q, %q", tt.worktree, got.HubRoot, got.BoardDir, got.PrimeAnchor, hub, fabricengine.BoardDir(hub), prime.AnchorPath())
			}
			if got.WorktreeRoot != root || got.WorktreeName != tt.worktree || got.Slug != tt.wantSlug {
				t.Errorf("DobbyGeometry(%q) worktree = %q, %q, slug %q; want %q, %q, slug %q", tt.worktree, got.WorktreeRoot, got.WorktreeName, got.Slug, root, tt.worktree, tt.wantSlug)
			}
			for name, path := range map[string]string{"LockPath": got.LockPath, "StatePath": got.StatePath, "MarkerDir": got.MarkerDir} {
				if filepath.Dir(path) != beamDir {
					t.Errorf("DobbyGeometry(%q).%s = %q; want it directly under %q", tt.worktree, name, path, beamDir)
				}
			}
			if tt.wantSlug != "" {
				beam, err := lyxcwd.NamedWorktree(hub, tt.worktree)
				if err != nil {
					t.Fatalf("NamedWorktree(%q) = %v; want nil", tt.worktree, err)
				}
				paths := PawlPaths(beam)
				for name, path := range map[string]string{"LandRequest": paths.LandRequest, "DobbyStopMarker": paths.DobbyStopMarker, "AwaitMarker": paths.AwaitMarker} {
					if filepath.Dir(path) != beamDir {
						t.Errorf("PawlPaths(%q).%s = %q; want it directly under the dobby's directory %q", tt.worktree, name, path, beamDir)
					}
				}
			}
			if want := shedrun.RunsRoot(prime); got.PrimeRunsDir != want {
				t.Errorf("DobbyGeometry(%q).PrimeRunsDir = %q; want %q, under the prime", tt.worktree, got.PrimeRunsDir, want)
			}
			if got.LyxExecutable == "" || !filepath.IsAbs(got.LyxExecutable) {
				t.Errorf("DobbyGeometry(%q).LyxExecutable = %q; want an absolute path", tt.worktree, got.LyxExecutable)
			}
			wantRecords := []string{fabricengine.MoveRecordPath(fabricengine.RecordsWorktreePath(prime, prime.WorktreeName))}
			if tt.worktree != prime.WorktreeName {
				wantRecords = append([]string{fabricengine.MoveRecordPath(fabricengine.RecordsWorktreePath(prime, tt.worktree))}, wantRecords...)
			}
			if !slices.Equal(got.MoveRecords, wantRecords) {
				t.Errorf("DobbyGeometry(%q).MoveRecords = %v; want %v, the worktree's own record and then the prime's", tt.worktree, got.MoveRecords, wantRecords)
			}
		})
	}

	t.Run("a standing move record reads for the worktree it is on, and the prime's for every beam", func(t *testing.T) {
		t.Parallel()

		_, prime := dobbyHub(t)
		start := DobbyStart(prime)
		standing := func(worktree string) (string, bool) {
			t.Helper()
			wayForward, stands, err := start.MoveInProgress(worktree)
			if err != nil {
				t.Fatalf("MoveInProgress(%q) = %v; want nil", worktree, err)
			}
			return wayForward, stands
		}

		for _, worktree := range []string{"some-repo", "a-beam", "other-beam"} {
			if _, stands := standing(worktree); stands {
				t.Errorf("MoveInProgress(%q) stands with no record; want it not", worktree)
			}
		}

		primeRecord := standMove(t, prime, "some-repo")
		for _, worktree := range []string{"some-repo", "a-beam", "other-beam"} {
			wayForward, stands := standing(worktree)
			if !stands || !strings.Contains(wayForward, "lyx fabric anchor some-repo --set sub") {
				t.Errorf("MoveInProgress(%q) = (%q, %v) with the prime's record; want it standing, naming the prime's re-run", worktree, wayForward, stands)
			}
		}

		if err := os.Remove(primeRecord); err != nil {
			t.Fatal(err)
		}
		standMove(t, prime, "a-beam")
		for worktree, want := range map[string]bool{"some-repo": false, "a-beam": true, "other-beam": false} {
			if _, stands := standing(worktree); stands != want {
				t.Errorf("MoveInProgress(%q) standing = %v with the beam's record; want %v", worktree, stands, want)
			}
		}
	})

	t.Run("a worktree with no directory is an error naming it", func(t *testing.T) {
		t.Parallel()

		if _, err := DobbyGeometry(prime, "no-such-beam"); err == nil || !strings.Contains(err.Error(), `"no-such-beam"`) {
			t.Errorf("DobbyGeometry(no-such-beam) = %v; want an error naming the worktree", err)
		}
	})
}
