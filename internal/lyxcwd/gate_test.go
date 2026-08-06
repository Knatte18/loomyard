// gate_test.go covers the strict cwd anchor gate (checkCwdAnchorGate) and its path comparator
// (samePath) as pure path-math tables — no git spawning, no fixture trees — so this file stays
// untagged.
// It also pins ResolveWithAnchor and ResolveWorktree as permanently ungated at each of the gate's
// own rejection triples, so a later "consistency" change cannot quietly gate either bypass and
// break clone/lyxtest.

package lyxcwd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCheckCwdAnchorGate covers the (cwd, anchorRel, worktreePath) triple space: exact match
// resolves, a subdirectory errors, a parent errors, and a sibling errors.
func TestCheckCwdAnchorGate(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo")

	tests := []struct {
		name      string
		cwd       string
		anchorRel string
		wantErr   bool
	}{
		{
			name:      "exact match at root resolves",
			cwd:       worktreePath,
			anchorRel: ".",
			wantErr:   false,
		},
		{
			name:      "exact match at subpath anchor resolves",
			cwd:       filepath.Join(worktreePath, "backend"),
			anchorRel: "backend",
			wantErr:   false,
		},
		{
			name:      "subdirectory of an unanchored root errors",
			cwd:       filepath.Join(worktreePath, "sub"),
			anchorRel: ".",
			wantErr:   true,
		},
		{
			name:      "parent of the worktree root errors",
			cwd:       filepath.Dir(worktreePath),
			anchorRel: ".",
			wantErr:   true,
		},
		{
			name:      "sibling directory of a subpath anchor errors",
			cwd:       filepath.Join(worktreePath, "frontend"),
			anchorRel: "backend",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCwdAnchorGate(tt.cwd, tt.anchorRel, worktreePath)
			if tt.wantErr && !errors.Is(err, ErrCwdOutsideAnchor) {
				t.Errorf("checkCwdAnchorGate(%q, %q, %q) = %v; want wrapped ErrCwdOutsideAnchor", tt.cwd, tt.anchorRel, worktreePath, err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("checkCwdAnchorGate(%q, %q, %q) = %v; want nil", tt.cwd, tt.anchorRel, worktreePath, err)
			}
		})
	}
}

// TestSamePath covers path-normalization edge cases: trailing separator, "."/".."
// segments, mixed separators, a symlinked path resolving to its target, and a case-differing path
// that must match on Windows and must not on Linux.
func TestSamePath(t *testing.T) {
	t.Run("trailing separator", func(t *testing.T) {
		a := filepath.Join("home", "user", "repo")
		b := a + string(filepath.Separator)
		if !samePath(a, b) {
			t.Errorf("samePath(%q, %q) = false; want true", a, b)
		}
	})

	t.Run("dot and dotdot segments", func(t *testing.T) {
		a := filepath.Join("home", "user", "repo")
		b := filepath.Join("home", "user", "other", "..", "repo", ".")
		if !samePath(a, b) {
			t.Errorf("samePath(%q, %q) = false; want true", a, b)
		}
	})

	t.Run("mixed separators", func(t *testing.T) {
		a := filepath.Join("home", "user", "repo")
		b := "home/user/repo"
		if !samePath(a, b) {
			t.Errorf("samePath(%q, %q) = false; want true", a, b)
		}
	})

	t.Run("symlinked path resolves to its target", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		link := filepath.Join(tmpDir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlink not supported in this environment: %v", err)
		}

		if !samePath(target, link) {
			t.Errorf("samePath(%q, %q) = false; want true (link resolves to target)", target, link)
		}
	})

	t.Run("case-differing path", func(t *testing.T) {
		a := filepath.Join("home", "user", "Repo")
		b := filepath.Join("home", "user", "repo")

		got := samePath(a, b)
		want := runtime.GOOS == "windows"
		if got != want {
			t.Errorf("samePath(%q, %q) = %v; want %v (GOOS=%s)", a, b, got, want, runtime.GOOS)
		}
	})
}

// TestUngatedEntryPoints_AtGateRejectionTriples pins ResolveWithAnchor and ResolveWorktree as
// permanently ungated: at a (cwd, anchorRel, worktreePath) triple that makes checkCwdAnchorGate
// return ErrCwdOutsideAnchor, buildLocation with applyGate=false — the shared body both entry
// points route through with git-spawning already done — must still succeed with no error.
// This guards clone, whose freshly-cloned worktree root sits above a non-"."
// subpath anchor,
// and lyxtest's synthetic-hub anchor injection.
// This test exercises buildLocation directly rather than the two exported entry points, so the file
// stays untagged with no git spawned;
// ResolveWithAnchor and ResolveWorktree are themselves one-line wrappers over this same
// applyGate=false path (see lyxcwd.go), so pinning it here pins both.
func TestUngatedEntryPoints_AtGateRejectionTriples(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	hubPath := filepath.Dir(worktreeRoot)

	tests := []struct {
		name string
		cwd  string
	}{
		{"cwd equals worktree root, anchor is a subpath", worktreeRoot},
		{"cwd is a sibling of the subpath anchor", filepath.Join(worktreeRoot, "frontend")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sanity: this triple must actually be rejected by the gate, so the
			// ungated assertion below is meaningful rather than vacuous.
			if err := checkCwdAnchorGate(tt.cwd, "backend", worktreeRoot); !errors.Is(err, ErrCwdOutsideAnchor) {
				t.Fatalf("checkCwdAnchorGate(%q, %q, %q) = %v; want wrapped ErrCwdOutsideAnchor (test setup invalid)", tt.cwd, "backend", worktreeRoot, err)
			}

			loc, err := buildLocation(tt.cwd, worktreeRoot, hubPath, "backend", false)
			if err != nil {
				t.Fatalf("buildLocation(%q, ..., applyGate=false) error = %v; want nil", tt.cwd, err)
			}
			if loc.AnchorRel != "backend" {
				t.Errorf("buildLocation(%q, ...).AnchorRel = %q; want %q", tt.cwd, loc.AnchorRel, "backend")
			}
		})
	}
}
