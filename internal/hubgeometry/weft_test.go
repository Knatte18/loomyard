// weft_test.go covers the weft geometry methods on Layout and verifies the
// host↔weft junction pairing for the RelPath "." and subpath cases.

package hubgeometry_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
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
		wantWeftRaddleDir    string
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
			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "_lyx"),
			wantWeftLyxDirFor: filepath.Join("/h", "x-weft", "_lyx"),
			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "_raddle"),
			wantHostLyxLink:   filepath.Join("/h", "x", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "_lyx"),
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
			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "sub", "_lyx"),
			wantWeftLyxDirFor: filepath.Join("/h", "x-weft", "sub", "_lyx"),
			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "sub", "_raddle"),
			wantHostLyxLink:   filepath.Join("/h", "x", "sub", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub", "_lyx"),
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
			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
			wantWeftLyxDirFor: filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "sub/dir", "_raddle"),
			wantHostLyxLink:   filepath.Join("/h", "y", "sub/dir", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub/dir", "_lyx"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := &hubgeometry.Layout{
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

			// Test WeftRaddleDir()
			if got := layout.WeftRaddleDir(); got != tt.wantWeftRaddleDir {
				t.Errorf("WeftRaddleDir() = %q; want %q", got, tt.wantWeftRaddleDir)
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
			hostWtName := filepath.Base(layout.WorktreePath(tt.slug))
			weftWtName := filepath.Base(layout.WeftWorktreePath(tt.slug))

			// The junction pair should differ only by -weft in the worktree name
			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("WeftWorktreePath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}

			// Verify HostLyxLinkHere is based on WorktreeRoot, not Cwd (documented intent).
			// When Cwd != WorktreeRoot, they differ; when Cwd == WorktreeRoot (RelPath == "."),
			// they are equal by construction.
			hostLyxLinkHereVal := layout.HostLyxLinkHere()
			expectedHostLyxLinkHere := filepath.Join(layout.WorktreeRoot, layout.RelPath, "_lyx")
			if hostLyxLinkHereVal != expectedHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere() = %q; want %q", hostLyxLinkHereVal, expectedHostLyxLinkHere)
			}
		})
	}
}

// TestHostLyxLinkHereDivergesFromLyxDir verifies the documented intent that
// HostLyxLinkHere() is anchored on WorktreeRoot+RelPath while LyxDir() is
// anchored on Cwd, so the two diverge whenever Cwd != WorktreeRoot+RelPath and
// coincide when they are equal.
func TestHostLyxLinkHereDivergesFromLyxDir(t *testing.T) {
	// Equal case: Cwd == WorktreeRoot and RelPath == "." -> both resolve to the
	// same _lyx directory.
	atRoot := &hubgeometry.Layout{
		Cwd:          filepath.Join("/h", "feat"),
		WorktreeRoot: filepath.Join("/h", "feat"),
		Hub:          "/h",
		RelPath:      ".",
		Prime:        "/h/main",
	}
	if atRoot.HostLyxLinkHere() != atRoot.LyxDir() {
		t.Errorf("HostLyxLinkHere() = %q; want it to equal LyxDir() = %q when Cwd == WorktreeRoot",
			atRoot.HostLyxLinkHere(), atRoot.LyxDir())
	}

	// Divergent case: Cwd points at the worktree root but RelPath is a real
	// subdir, so LyxDir() (Cwd-anchored) and HostLyxLinkHere() (WorktreeRoot+
	// RelPath-anchored) must differ.
	atSub := &hubgeometry.Layout{
		Cwd:          filepath.Join("/h", "feat"),
		WorktreeRoot: filepath.Join("/h", "feat"),
		Hub:          "/h",
		RelPath:      "sub",
		Prime:        "/h/main",
	}
	if atSub.HostLyxLinkHere() == atSub.LyxDir() {
		t.Errorf("HostLyxLinkHere() = %q; want it to differ from LyxDir() = %q when Cwd != WorktreeRoot+RelPath",
			atSub.HostLyxLinkHere(), atSub.LyxDir())
	}
}

// TestWeftGeometryAtMainWorktree verifies that WeftRepoRoot and WeftWorktree are equal
// when resolving at the main worktree.
func TestWeftGeometryAtMainWorktree(t *testing.T) {
	hub := "/h"
	main := "/h/main"
	layout := &hubgeometry.Layout{
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

// TestHostJunctions verifies that HostJunctions(slug) returns exactly one entry with
// the correct Name, Link, and Target fields, and that no entry's Name equals _raddle.
func TestHostJunctions(t *testing.T) {
	tests := []struct {
		name    string
		hub     string
		prime   string
		slug    string
		relPath string
		// Expected junction values
		wantJunctionCount int
		wantName          string
	}{
		{
			name:              "prime-derived layout, root case",
			hub:               "/h",
			prime:             "/h/main",
			slug:              "feat",
			relPath:           ".",
			wantJunctionCount: 1,
			wantName:          "_lyx",
		},
		{
			name:              "non-prime worktree layout, root case",
			hub:               "/h",
			prime:             "/h/main",
			slug:              "other",
			relPath:           ".",
			wantJunctionCount: 1,
			wantName:          "_lyx",
		},
		{
			name:              "subpath case",
			hub:               "/h",
			prime:             "/h/main",
			slug:              "feat",
			relPath:           "sub",
			wantJunctionCount: 1,
			wantName:          "_lyx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := &hubgeometry.Layout{
				Cwd:          filepath.Join(tt.hub, tt.slug, tt.relPath),
				WorktreeRoot: filepath.Join(tt.hub, tt.slug),
				Hub:          tt.hub,
				RelPath:      tt.relPath,
				Prime:        tt.prime,
			}

			junctions := layout.HostJunctions(tt.slug)

			// Verify count
			if len(junctions) != tt.wantJunctionCount {
				t.Errorf("HostJunctions(%q) returned %d entries; want %d", tt.slug, len(junctions), tt.wantJunctionCount)
			}

			// Verify the single entry
			if len(junctions) > 0 {
				j := junctions[0]

				if j.Name != tt.wantName {
					t.Errorf("HostJunctions(%q)[0].Name = %q; want %q", tt.slug, j.Name, tt.wantName)
				}

				// Verify Link matches HostLyxLink(slug)
				wantLink := layout.HostLyxLink(tt.slug)
				if j.Link != wantLink {
					t.Errorf("HostJunctions(%q)[0].Link = %q; want %q", tt.slug, j.Link, wantLink)
				}

				// Verify Target matches WeftLyxDirFor(slug)
				wantTarget := layout.WeftLyxDirFor(tt.slug)
				if j.Target != wantTarget {
					t.Errorf("HostJunctions(%q)[0].Target = %q; want %q", tt.slug, j.Target, wantTarget)
				}
			}
		})
	}

	// Sub-test: scope guard — verify no junction name is _raddle
	t.Run("no_raddle_names", func(t *testing.T) {
		layout := &hubgeometry.Layout{
			Cwd:          filepath.Join("/h", "main"),
			WorktreeRoot: filepath.Join("/h", "main"),
			Hub:          "/h",
			RelPath:      ".",
			Prime:        filepath.Join("/h", "main"),
		}

		junctions := layout.HostJunctions("slug")
		for _, j := range junctions {
			if j.Name == "_raddle" {
				t.Errorf("HostJunctions found _raddle entry (forbidden by design)")
			}
		}
	})
}
