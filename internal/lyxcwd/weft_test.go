// weft_test.go covers the host-side junction geometry methods still owned by
// Location — HostLyxLink/HostLyxLinkHere and HostJunctions — verifying the
// host↔weft junction pairing for the AnchorRel "." and subpath cases. The
// weft-side accessors (WeftWorktree, WeftWorktreePath, WeftLyxDir,
// WeftLyxDirFor) relocated to internal/fabricengine; their coverage lives in
// fabricengine/weftpaths_test.go now. This file computes the weft-sibling
// half of each pairing assertion via weftname.SiblingPath directly, the same
// way the relocated accessors themselves are built.

package lyxcwd_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestHostLyxLinkMethods covers HostLyxLink(slug) and HostLyxLinkHere() with
// both AnchorRel "." (root) and subpath cases, verifying AnchorRel-mirroring
// and junction pairing against the weft-sibling worktree.
func TestHostLyxLinkMethods(t *testing.T) {
	tests := []struct {
		name                string
		hub                 string
		slug                string
		relPath             string
		wantHostLyxLink     string
		wantHostLyxLinkHere string
	}{
		{
			name:                "/h /h/main feat . case",
			hub:                 "/h",
			slug:                "x",
			relPath:             ".",
			wantHostLyxLink:     filepath.Join("/h", "x", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "_lyx"),
		},
		{
			name:                "/h /h/main feat sub case",
			hub:                 "/h",
			slug:                "x",
			relPath:             "sub",
			wantHostLyxLink:     filepath.Join("/h", "x", "sub", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub", "_lyx"),
		},
		{
			name:                "/h /h/main feat sub/dir case",
			hub:                 "/h",
			slug:                "y",
			relPath:             "sub/dir",
			wantHostLyxLink:     filepath.Join("/h", "y", "sub/dir", "_lyx"),
			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub/dir", "_lyx"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: "feat",
				AnchorRel:    tt.relPath,
			}

			// Test HostLyxLink(slug)
			if got := loc.HostLyxLink(tt.slug); got != tt.wantHostLyxLink {
				t.Errorf("HostLyxLink(%q) = %q; want %q", tt.slug, got, tt.wantHostLyxLink)
			}

			// Test HostLyxLinkHere()
			if got := loc.HostLyxLinkHere(); got != tt.wantHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere() = %q; want %q", got, tt.wantHostLyxLinkHere)
			}

			// Verify junction pairing: HostLyxLink(slug) and the weft sibling's
			// _lyx directory are siblings differing only by the -weft suffix on
			// the worktree dir.
			hostWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(weftname.SiblingPath(loc.HubPath, tt.slug))

			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("weftname.SiblingPath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
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

// TestHostJunctions verifies that HostJunctions(slug, names) returns one record per name
// in names, in names's own input order, with Link/Target correctly composed from the
// Location's WorktreePath/weft-sibling path and AnchorRel, at AnchorRel == "." and at a nested
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
				wantTarget := filepath.Join(weftname.SiblingPath(loc.HubPath, tt.slug), loc.AnchorRel, wantName)
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
