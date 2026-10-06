// gate_test.go covers the strict cwd anchor gate (checkCwdAnchorGate) and its path comparator (samePath) as pure path-math tables — no git spawning, no fixture trees — so this file stays untagged.
// It also pins ResolveWithAnchor and ResolveWorktree as permanently ungated at each of the gate's own rejection triples, so a later "consistency" change cannot quietly gate either bypass and break clone/gitkit.
// Every test here is parallel: none touches process-global state.

package lyxcwd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCheckCwdAnchorGate covers the (cwd, anchorRel, worktreePath) triple space: exact match resolves, a subdirectory errors, a parent errors, and a sibling errors.
// At every triple the gate rejects, it also pins ResolveWithAnchor and ResolveWorktree as permanently ungated: buildLocation with applyGate=false, the shared body both entry points route through with git-spawning already done, must still succeed with the anchorRel as given.
// That guards clone, whose freshly-cloned worktree root sits above a non-"." subpath anchor, and gitkit's synthetic-hub anchor injection, so a later "consistency" change cannot quietly gate either bypass.
// buildLocation is exercised directly rather than the two exported entry points, so the file stays untagged with no git spawned; they are one-line wrappers over this same applyGate=false path.
//
//testtiming:keep pins the gate's accept and reject verdicts per triple as pure path math, which the resolution scenario covering its blocks only reaches through a real checkout
func TestCheckCwdAnchorGate(t *testing.T) {
	t.Parallel()

	worktreePath := filepath.Join("home", "user", "repo")
	hubPath := filepath.Dir(worktreePath)

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
		{
			name:      "worktree root above a subpath anchor errors",
			cwd:       worktreePath,
			anchorRel: "backend",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkCwdAnchorGate(tt.cwd, tt.anchorRel, worktreePath)
			if tt.wantErr && !errors.Is(err, ErrCwdOutsideAnchor) {
				t.Errorf("checkCwdAnchorGate(%q, %q, %q) = %v; want wrapped ErrCwdOutsideAnchor", tt.cwd, tt.anchorRel, worktreePath, err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("checkCwdAnchorGate(%q, %q, %q) = %v; want nil", tt.cwd, tt.anchorRel, worktreePath, err)
			}
			if !tt.wantErr {
				return
			}

			loc, err := buildLocation(tt.cwd, worktreePath, hubPath, tt.anchorRel, false)
			if err != nil {
				t.Fatalf("buildLocation(%q, ..., applyGate=false) error = %v; want nil", tt.cwd, err)
			}
			if loc.AnchorRel != tt.anchorRel {
				t.Errorf("buildLocation(%q, ...).AnchorRel = %q; want %q", tt.cwd, loc.AnchorRel, tt.anchorRel)
			}
		})
	}
}

// TestSamePath covers path-normalization edge cases: trailing separator, "."/".."
// segments, mixed separators, a symlinked path resolving to its target, and a case-differing path that must match on Windows and must not on Linux.
//
//testtiming:keep pins samePath's normalization rows directly, which the gate test covering its blocks only reaches through exact-match triples
func TestSamePath(t *testing.T) {
	t.Parallel()

	repo := filepath.Join("home", "user", "repo")
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"trailing separator", repo, repo + string(filepath.Separator), true},
		{"dot and dotdot segments", repo, filepath.Join("home", "user", "other", "..", "repo", "."), true},
		{"mixed separators", repo, "home/user/repo", true},
		// Case-insensitive on Windows only.
		{"case-differing path", filepath.Join("home", "user", "Repo"), filepath.Join("home", "user", "repo"), runtime.GOOS == "windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := samePath(tt.a, tt.b); got != tt.want {
				t.Errorf("samePath(%q, %q) = %v; want %v (GOOS=%s)", tt.a, tt.b, got, tt.want, runtime.GOOS)
			}
		})
	}

	t.Run("symlinked path resolves to its target", func(t *testing.T) {
		t.Parallel()

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
}
