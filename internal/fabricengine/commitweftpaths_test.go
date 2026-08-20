// commitweftpaths_test.go — unit tests for weftCommitPathspec and CommitWeftPaths' git-free early
// return paths. Every test here builds inputs by hand and spawns no git, per the Test Tier Purity
// Invariant.

package fabricengine

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestWeftCommitPathspec covers anchorRel == "." (entries returned with no added segment) and
// anchorRel == "backend" (each entry prefixed), asserting the result is positive-only with no
// entry beginning with a ":(exclude)" marker.
func TestWeftCommitPathspec(t *testing.T) {
	tests := []struct {
		name      string
		anchorRel string
		relPaths  []string
		want      []string
	}{
		{
			name:      "root anchor",
			anchorRel: ".",
			relPaths:  []string{"_lyx/fabric/origin.json"},
			want:      []string{filepath.Join(".", "_lyx/fabric/origin.json")},
		},
		{
			name:      "subpath anchor",
			anchorRel: "backend",
			relPaths:  []string{"_lyx/fabric/origin.json", "_lyx/other.json"},
			want: []string{
				filepath.Join("backend", "_lyx/fabric/origin.json"),
				filepath.Join("backend", "_lyx/other.json"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftCommitPathspec(tt.anchorRel, tt.relPaths)
			if len(got) != len(tt.want) {
				t.Fatalf("weftCommitPathspec(%q, %v) = %v; want %v", tt.anchorRel, tt.relPaths, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("weftCommitPathspec(%q, %v)[%d] = %q; want %q", tt.anchorRel, tt.relPaths, i, got[i], tt.want[i])
				}
				if strings.HasPrefix(got[i], ":(exclude)") {
					t.Errorf("weftCommitPathspec(%q, %v)[%d] = %q; want positive-only, no exclude marker", tt.anchorRel, tt.relPaths, i, got[i])
				}
			}
		})
	}
}

// TestCommitWeftPaths_SkipGit asserts CommitWeftPaths returns ("", false, nil) for
// SyncOptions{SkipGit: true} against a path that does not exist, proving no lock was taken and no
// directory created, and that the passed recorder's snapshot stays empty.
func TestCommitWeftPaths_SkipGit(t *testing.T) {
	weftPath := filepath.Join(t.TempDir(), "does-not-exist")
	rec := NewMutations("")

	sha, committed, err := CommitWeftPaths(rec, weftPath, ".", []string{"_lyx/fabric/origin.json"}, "msg", SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("CommitWeftPaths() error = %v; want nil", err)
	}
	if sha != "" || committed {
		t.Errorf("CommitWeftPaths() = (%q, %v); want (\"\", false)", sha, committed)
	}
	if got := rec.Snapshot().Len(); got != 0 {
		t.Errorf("recorder snapshot has %d entries; want 0 — the SkipGit early return must claim no commit", got)
	}
}

// TestCommitWeftPaths_EmptyPaths asserts CommitWeftPaths returns ("", false, nil) for an empty
// relPaths slice against a path that does not exist, proving no lock was taken and no directory
// created, and that the passed recorder's snapshot stays empty.
func TestCommitWeftPaths_EmptyPaths(t *testing.T) {
	weftPath := filepath.Join(t.TempDir(), "does-not-exist")
	rec := NewMutations("")

	sha, committed, err := CommitWeftPaths(rec, weftPath, ".", nil, "msg", SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeftPaths() error = %v; want nil", err)
	}
	if sha != "" || committed {
		t.Errorf("CommitWeftPaths() = (%q, %v); want (\"\", false)", sha, committed)
	}
	if got := rec.Snapshot().Len(); got != 0 {
		t.Errorf("recorder snapshot has %d entries; want 0 — the empty-relPaths early return must claim no commit", got)
	}
}
