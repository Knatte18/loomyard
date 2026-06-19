// weft_test.go covers the weft geometry methods on Layout and verifies the
// host↔weft junction pairing for the RelPath "." and subpath cases.

package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestWeftGeometryMethods covers the eight weft geometry methods with both
// RelPath "." (root) and subpath cases, verifying RelPath-mirroring and junction pairing.
func TestWeftGeometryMethods(t *testing.T) {
	tests := []struct {
		name    string
		hub     string
		prime   string
		slug    string
		relPath string
		// Expected results for all eight methods (computed in the test)
		wantWeftRepoRoot     string
		wantWeftWorktree     string
		wantWeftWorktreePath string
		wantWeftLyxDir       string
		wantWeftLyxDirFor    string
		wantWeftCodeguideDir string
		wantHostLyxLink      string
		wantHostLyxLinkHere  string
	}{
		{
			name:                 "/h /h/main feat . case",
			hub:                  "/h",
			prime:                "/h/main",
			slug:                 "x",
			relPath:              ".",
			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "_lyx"),
			wantWeftCodeguideDir: filepath.Join("/h", "feat-weft", "_codeguide"),
			wantHostLyxLink:      filepath.Join("/h", "x", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub case",
			hub:                  "/h",
			prime:                "/h/main",
			slug:                 "x",
			relPath:              "sub",
			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "sub", "_lyx"),
			wantWeftCodeguideDir: filepath.Join("/h", "feat-weft", "sub", "_codeguide"),
			wantHostLyxLink:      filepath.Join("/h", "x", "sub", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub/dir case",
			hub:                  "/h",
			prime:                "/h/main",
			slug:                 "y",
			relPath:              "sub/dir",
			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
			wantWeftCodeguideDir: filepath.Join("/h", "feat-weft", "sub/dir", "_codeguide"),
			wantHostLyxLink:      filepath.Join("/h", "y", "sub/dir", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub/dir", "_lyx"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := &paths.Layout{
				Cwd:          filepath.Join(tt.hub, "feat", tt.relPath),
				WorktreeRoot: filepath.Join(tt.hub, "feat"),
				Hub:          tt.hub,
				RelPath:      tt.relPath,
				Prime:        tt.prime,
			}

			// Test WeftRepoRoot()
			if got := layout.WeftRepoRoot(); got != tt.wantWeftRepoRoot {
				t.Errorf("WeftRepoRoot() = %q; want %q", got, tt.wantWeftRepoRoot)
			}

			// Test WeftWorktree()
			if got := layout.WeftWorktree(); got != tt.wantWeftWorktree {
				t.Errorf("WeftWorktree() = %q; want %q", got, tt.wantWeftWorktree)
			}

			// Test WeftWorktreePath(slug)
			if got := layout.WeftWorktreePath(tt.slug); got != tt.wantWeftWorktreePath {
				t.Errorf("WeftWorktreePath(%q) = %q; want %q", tt.slug, got, tt.wantWeftWorktreePath)
			}

			// Test WeftLyxDir()
			if got := layout.WeftLyxDir(); got != tt.wantWeftLyxDir {
				t.Errorf("WeftLyxDir() = %q; want %q", got, tt.wantWeftLyxDir)
			}

			// Test WeftLyxDirFor(slug)
			if got := layout.WeftLyxDirFor(tt.slug); got != tt.wantWeftLyxDirFor {
				t.Errorf("WeftLyxDirFor(%q) = %q; want %q", tt.slug, got, tt.wantWeftLyxDirFor)
			}

			// Test WeftCodeguideDir()
			if got := layout.WeftCodeguideDir(); got != tt.wantWeftCodeguideDir {
				t.Errorf("WeftCodeguideDir() = %q; want %q", got, tt.wantWeftCodeguideDir)
			}

			// Test HostLyxLink(slug)
			if got := layout.HostLyxLink(tt.slug); got != tt.wantHostLyxLink {
				t.Errorf("HostLyxLink(%q) = %q; want %q", tt.slug, got, tt.wantHostLyxLink)
			}

			// Test HostLyxLinkHere()
			if got := layout.HostLyxLinkHere(); got != tt.wantHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere() = %q; want %q", got, tt.wantHostLyxLinkHere)
			}

			// Verify junction pairing: HostLyxLink(slug) and WeftLyxDirFor(slug) are
			// siblings differing only by the -weft suffix on the worktree dir
			hostLink := layout.HostLyxLink(tt.slug)
			weftTarget := layout.WeftLyxDirFor(tt.slug)
			hostWtName := filepath.Base(layout.WorktreePath(tt.slug))
			weftWtName := filepath.Base(layout.WeftWorktreePath(tt.slug))

			// The junction pair should differ only by -weft in the worktree name
			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("WeftWorktreePath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}

			// Verify HostLyxLinkHere and LyxDir differ (WorktreeRoot+RelPath vs Cwd+RelPath)
			if layout.RelPath != "." {
				lyxDir := layout.LyxDir()
				hostLyxLinkHere := layout.HostLyxLinkHere()
				// They should both be _lyx but with different bases
				if lyxDir == hostLyxLinkHere {
					t.Errorf("LyxDir() = HostLyxLinkHere() = %q; want them to differ", lyxDir)
				}
			}
		})
	}
}

// TestWeftGeometryAtMainWorktree verifies that WeftRepoRoot and WeftWorktree are equal
// when resolving at the main worktree.
func TestWeftGeometryAtMainWorktree(t *testing.T) {
	hub := "/h"
	main := "/h/main"
	layout := &paths.Layout{
		Cwd:          main,
		WorktreeRoot: main,
		Hub:          hub,
		RelPath:      ".",
		Prime:        main,
	}

	weftRepoRoot := layout.WeftRepoRoot()
	weftWorktree := layout.WeftWorktree()

	if weftRepoRoot != weftWorktree {
		t.Errorf("At main: WeftRepoRoot() = %q, WeftWorktree() = %q; want equal", weftRepoRoot, weftWorktree)
	}
}
