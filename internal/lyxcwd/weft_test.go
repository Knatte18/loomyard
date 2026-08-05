// weft_test.go covers the weft geometry methods on Location and verifies the
// host↔weft junction pairing for the AnchorRel "." and subpath cases.

package lyxcwd_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestWeftGeometryMethods covers the eight weft geometry methods with both
// AnchorRel "." (root) and subpath cases, verifying AnchorRel-mirroring and junction pairing.
func TestWeftGeometryMethods(t *testing.T) {
	tests := []struct {
		name    string
		hub     string
		slug    string
		relPath string
		// Expected results for the remaining seven methods (computed in the
		// test); WeftRepoRoot's replacement lives in fabricengine now, and its
		// property is held by that package's own retargeted test callers.
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
			slug:                 "x",
			relPath:              ".",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "_lyx"),
			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "_raddle"),
			wantHostLyxLink:      filepath.Join("/h", "x", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              "sub",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "sub", "_lyx"),
			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "sub", "_raddle"),
			wantHostLyxLink:      filepath.Join("/h", "x", "sub", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub/dir case",
			hub:                  "/h",
			slug:                 "y",
			relPath:              "sub/dir",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "sub/dir", "_raddle"),
			wantHostLyxLink:      filepath.Join("/h", "y", "sub/dir", "_lyx"),
			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub/dir", "_lyx"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: "feat",
				AnchorRel:    tt.relPath,
			}

			// Test WeftWorktree()
			if got := loc.WeftWorktree(); got != tt.wantWeftWorktree {
				t.Errorf("WeftWorktree() = %q; want %q", got, tt.wantWeftWorktree)
			}

			// Test WeftWorktreePath(slug)
			if got := loc.WeftWorktreePath(tt.slug); got != tt.wantWeftWorktreePath {
				t.Errorf("WeftWorktreePath(%q) = %q; want %q", tt.slug, got, tt.wantWeftWorktreePath)
			}

			// Test WeftLyxDir()
			if got := loc.WeftLyxDir(); got != tt.wantWeftLyxDir {
				t.Errorf("WeftLyxDir() = %q; want %q", got, tt.wantWeftLyxDir)
			}

			// Test WeftLyxDirFor(slug)
			if got := loc.WeftLyxDirFor(tt.slug); got != tt.wantWeftLyxDirFor {
				t.Errorf("WeftLyxDirFor(%q) = %q; want %q", tt.slug, got, tt.wantWeftLyxDirFor)
			}

			// Test WeftRaddleDir()
			if got := loc.WeftRaddleDir(); got != tt.wantWeftRaddleDir {
				t.Errorf("WeftRaddleDir() = %q; want %q", got, tt.wantWeftRaddleDir)
			}

			// Test HostLyxLink(slug)
			if got := loc.HostLyxLink(tt.slug); got != tt.wantHostLyxLink {
				t.Errorf("HostLyxLink(%q) = %q; want %q", tt.slug, got, tt.wantHostLyxLink)
			}

			// Test HostLyxLinkHere()
			if got := loc.HostLyxLinkHere(); got != tt.wantHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere() = %q; want %q", got, tt.wantHostLyxLinkHere)
			}

			// Verify junction pairing: HostLyxLink(slug) and WeftLyxDirFor(slug) are
			// siblings differing only by the -weft suffix on the worktree dir
			hostWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(loc.WeftWorktreePath(tt.slug))

			// The junction pair should differ only by -weft in the worktree name
			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("WeftWorktreePath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}

			// Verify HostLyxLinkHere is based on WorktreePath+AnchorRel (documented intent).
			hostLyxLinkHereVal := loc.HostLyxLinkHere()
			expectedHostLyxLinkHere := filepath.Join(loc.WorktreePath(), loc.AnchorRel, "_lyx")
			if hostLyxLinkHereVal != expectedHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere() = %q; want %q", hostLyxLinkHereVal, expectedHostLyxLinkHere)
			}
		})
	}
}

// TestLyxDirEqualsHostLyxLinkHere verifies that, under the strict cwd gate,
// LyxDir() (AnchorPath-anchored) and HostLyxLinkHere() (WorktreePath+AnchorRel-
// anchored) always resolve to the same _lyx directory: AnchorPath() is defined
// as filepath.Join(WorktreePath(), AnchorRel), so the two are the same path by
// construction, at both the root and a nested AnchorRel.
func TestLyxDirEqualsHostLyxLinkHere(t *testing.T) {
	atRoot := &lyxcwd.Location{
		HubPath:      "/h",
		WorktreeName: "feat",
		AnchorRel:    ".",
	}
	if atRoot.HostLyxLinkHere() != atRoot.LyxDir() {
		t.Errorf("HostLyxLinkHere() = %q; want it to equal LyxDir() = %q at AnchorRel \".\"",
			atRoot.HostLyxLinkHere(), atRoot.LyxDir())
	}

	atSub := &lyxcwd.Location{
		HubPath:      "/h",
		WorktreeName: "feat",
		AnchorRel:    "sub",
	}
	if atSub.HostLyxLinkHere() != atSub.LyxDir() {
		t.Errorf("HostLyxLinkHere() = %q; want it to equal LyxDir() = %q at a nested AnchorRel",
			atSub.HostLyxLinkHere(), atSub.LyxDir())
	}
}

// TestHostJunctions verifies that HostJunctions(slug, names) returns one record per name
// in names, in names's own input order, with Link/Target correctly composed from the
// Location's WorktreePath/WeftWorktreePath and AnchorRel, at AnchorRel == "." and at a nested
// AnchorRel, for an empty names slice, a 3-name slice, and a reversed 2-name slice — and that
// no entry's Name equals _raddle for the default two-name pathspec.
func TestHostJunctions(t *testing.T) {
	tests := []struct {
		name    string
		hub     string
		slug    string
		relPath string
		names   []string
	}{
		{
			name:    "prime-derived layout, root case",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{"_lyx", "_pattern"},
		},
		{
			name:    "non-prime worktree layout, root case",
			hub:     "/h",
			slug:    "other",
			relPath: ".",
			names:   []string{"_lyx", "_pattern"},
		},
		{
			name:    "subpath case",
			hub:     "/h",
			slug:    "feat",
			relPath: "sub",
			names:   []string{"_lyx", "_pattern"},
		},
		{
			name:    "empty names yields zero records",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{},
		},
		{
			name:    "3-name slice yields three records in input order",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{"_lyx", "_pattern", "_extra"},
		},
		{
			name:    "reversed 2-name slice preserves given order, no forced sort",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{"_pattern", "_lyx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: tt.slug,
				AnchorRel:    tt.relPath,
			}

			junctions := loc.HostJunctions(tt.slug, tt.names)

			// Verify count matches the input names slice exactly, including the
			// empty-slice case (zero records).
			if len(junctions) != len(tt.names) {
				t.Fatalf("HostJunctions(%q, %v) returned %d entries; want %d", tt.slug, tt.names, len(junctions), len(tt.names))
			}

			// Verify each record, in the given names order (config order is
			// authoritative — no forced sort).
			for i, wantName := range tt.names {
				got := junctions[i]
				if got.Name != wantName {
					t.Errorf("HostJunctions(%q, %v)[%d].Name = %q; want %q", tt.slug, tt.names, i, got.Name, wantName)
				}
				wantLink := filepath.Join(filepath.Join(loc.HubPath, tt.slug), loc.AnchorRel, wantName)
				if got.Link != wantLink {
					t.Errorf("HostJunctions(%q, %v)[%d].Link = %q; want %q", tt.slug, tt.names, i, got.Link, wantLink)
				}
				wantTarget := filepath.Join(loc.WeftWorktreePath(tt.slug), loc.AnchorRel, wantName)
				if got.Target != wantTarget {
					t.Errorf("HostJunctions(%q, %v)[%d].Target = %q; want %q", tt.slug, tt.names, i, got.Target, wantTarget)
				}
			}
		})
	}

	// Sub-test: scope guard — verify no junction name is _raddle for the default pathspec.
	t.Run("no_raddle_names", func(t *testing.T) {
		loc := &lyxcwd.Location{
			HubPath:      "/h",
			WorktreeName: "main",
			AnchorRel:    ".",
		}

		junctions := loc.HostJunctions("slug", []string{"_lyx", "_pattern"})
		for _, j := range junctions {
			if j.Name == "_raddle" {
				t.Errorf("HostJunctions found _raddle entry (forbidden by design)")
			}
		}
	})
}
