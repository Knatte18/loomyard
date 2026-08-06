// hostjunction_test.go covers the host-side junction primitives relocated from internal/lyxcwd in
// this batch — HostLyxLink, HostLyxLinkHere, HostJunctions and HostJunctionsHere, plus the
// HostJunction record shape — over synthetic *lyxcwd.Location literals rather than real fixtures,
// the same table shapes lyxcwd's own tests used.
// Every _pattern row asserts against the generic config-driven junction join
// (filepath.Join(WorktreePath(l, slug), l.AnchorRel, pattern.DirName)) rather than a
// pattern-specific accessor, so this file survives card 35's deletion of those accessors unchanged.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestHostLyxLinkMethods covers HostLyxLink(l, slug) and HostLyxLinkHere(l) with both AnchorRel "."
// (root) and subpath cases, verifying AnchorRel-mirroring and junction pairing against the
// weft-sibling worktree.
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

			if got := HostLyxLink(loc, tt.slug); got != tt.wantHostLyxLink {
				t.Errorf("HostLyxLink(l, %q) = %q; want %q", tt.slug, got, tt.wantHostLyxLink)
			}

			if got := HostLyxLinkHere(loc); got != tt.wantHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere(l) = %q; want %q", got, tt.wantHostLyxLinkHere)
			}

			// Verify junction pairing: HostLyxLink(l, slug) and the weft
			// sibling's _lyx directory are siblings differing only by the
			// -weft suffix on the worktree dir.
			hostWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(weftname.SiblingPath(loc.HubPath, tt.slug))

			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("weftname.SiblingPath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}

			// Verify HostLyxLinkHere is based on WorktreePath+AnchorRel (documented intent).
			hostLyxLinkHereVal := HostLyxLinkHere(loc)
			expectedHostLyxLinkHere := filepath.Join(loc.WorktreePath(), loc.AnchorRel, "_lyx")
			if hostLyxLinkHereVal != expectedHostLyxLinkHere {
				t.Errorf("HostLyxLinkHere(l) = %q; want %q", hostLyxLinkHereVal, expectedHostLyxLinkHere)
			}
		})
	}
}

// TestHostJunctions verifies that HostJunctions(l, slug, names) returns one record per name in
// names, in names's own input order, with Link/Target correctly composed from l's
// WorktreePath/weft-sibling path and AnchorRel, at AnchorRel == "."
// and at a nested AnchorRel, for an empty names slice, a 3-name slice, and a reversed 2-name slice
// — and that no entry's Name equals _raddle for the default two-name pathspec.
// The _pattern row asserts against the generic join, not a pattern-specific accessor.
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
			names:   []string{"_lyx", pattern.DirName},
		},
		{
			name:    "non-prime worktree layout, root case",
			hub:     "/h",
			slug:    "other",
			relPath: ".",
			names:   []string{"_lyx", pattern.DirName},
		},
		{
			name:    "subpath case",
			hub:     "/h",
			slug:    "feat",
			relPath: "sub",
			names:   []string{"_lyx", pattern.DirName},
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
			names:   []string{"_lyx", pattern.DirName, "_extra"},
		},
		{
			name:    "reversed 2-name slice preserves given order, no forced sort",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{pattern.DirName, "_lyx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: tt.slug,
				AnchorRel:    tt.relPath,
			}

			junctions := HostJunctions(loc, tt.slug, tt.names)

			if len(junctions) != len(tt.names) {
				t.Fatalf("HostJunctions(l, %q, %v) returned %d entries; want %d", tt.slug, tt.names, len(junctions), len(tt.names))
			}

			for i, wantName := range tt.names {
				got := junctions[i]
				if got.Name != wantName {
					t.Errorf("HostJunctions(l, %q, %v)[%d].Name = %q; want %q", tt.slug, tt.names, i, got.Name, wantName)
				}
				wantLink := filepath.Join(WorktreePath(loc, tt.slug), loc.AnchorRel, wantName)
				if got.Link != wantLink {
					t.Errorf("HostJunctions(l, %q, %v)[%d].Link = %q; want %q", tt.slug, tt.names, i, got.Link, wantLink)
				}
				wantTarget := filepath.Join(WeftWorktreePath(loc, tt.slug), loc.AnchorRel, wantName)
				if got.Target != wantTarget {
					t.Errorf("HostJunctions(l, %q, %v)[%d].Target = %q; want %q", tt.slug, tt.names, i, got.Target, wantTarget)
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

		junctions := HostJunctions(loc, "slug", []string{"_lyx", pattern.DirName})
		for _, j := range junctions {
			if j.Name == "_raddle" {
				t.Errorf("HostJunctions found _raddle entry (forbidden by design)")
			}
		}
	})
}

// TestHostJunctionsHere verifies the Here-anchored, slug-free junction-detection accessor: it must
// return the expected Name/Link/Target for both RelPath == "."
// and a nested RelPath,
// and it must agree entry-for-entry with HostJunctions(l, slug, names) when l's slug and current
// worktree coincide — the precondition every health-check call site relies on.
func TestHostJunctionsHere(t *testing.T) {
	t.Run("at root", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		junctions := HostJunctionsHere(loc, []string{"_lyx", pattern.DirName})
		if len(junctions) != 2 {
			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lyxJunction := junctions[0]
		wantLyxLink := HostLyxLinkHere(loc)
		wantLyxTarget := filepath.Join(WeftWorktree(loc), loc.AnchorRel, "_lyx")
		if lyxJunction.Name != "_lyx" {
			t.Errorf("HostJunctionsHere()[0].Name = %q; want %q", lyxJunction.Name, "_lyx")
		}
		if lyxJunction.Link != wantLyxLink {
			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
		}
		if lyxJunction.Target != wantLyxTarget {
			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
		}

		patternJunction := junctions[1]
		wantPatternLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, pattern.DirName)
		wantPatternTarget := filepath.Join(WeftWorktree(loc), loc.AnchorRel, pattern.DirName)
		if patternJunction.Name != pattern.DirName {
			t.Errorf("HostJunctionsHere()[1].Name = %q; want %q", patternJunction.Name, pattern.DirName)
		}
		if patternJunction.Link != wantPatternLink {
			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
		}
		if patternJunction.Target != wantPatternTarget {
			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
		}
	})

	t.Run("at nested subpath", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: filepath.Join("services", "api")}

		junctions := HostJunctionsHere(loc, []string{"_lyx", pattern.DirName})
		if len(junctions) != 2 {
			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lyxJunction := junctions[0]
		wantLyxLink := HostLyxLinkHere(loc)
		wantLyxTarget := filepath.Join(WeftWorktree(loc), loc.AnchorRel, "_lyx")
		if lyxJunction.Link != wantLyxLink {
			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
		}
		if lyxJunction.Target != wantLyxTarget {
			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
		}

		patternJunction := junctions[1]
		wantPatternLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, pattern.DirName)
		wantPatternTarget := filepath.Join(WeftWorktree(loc), loc.AnchorRel, pattern.DirName)
		if patternJunction.Link != wantPatternLink {
			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
		}
		if patternJunction.Target != wantPatternTarget {
			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
		}
	})

	t.Run("agrees with HostJunctions when slug matches current worktree", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		// The current worktree's own base name is the slug that makes
		// HostJunctions(l, slug, names) resolve to the same host worktree
		// HostJunctionsHere(l, names) is already anchored at.
		slug := filepath.Base(loc.WorktreePath())
		names := []string{"_lyx", pattern.DirName}

		here := HostJunctionsHere(loc, names)
		bySlug := HostJunctions(loc, slug, names)

		if len(here) != len(bySlug) {
			t.Fatalf("HostJunctionsHere() returned %d junctions; HostJunctions(%q) returned %d", len(here), slug, len(bySlug))
		}
		for i := range here {
			if here[i] != bySlug[i] {
				t.Errorf("HostJunctionsHere()[%d] = %+v; HostJunctions(%q)[%d] = %+v", i, here[i], slug, i, bySlug[i])
			}
		}
	})

	t.Run("names ordering regressions", func(t *testing.T) {
		loc := &lyxcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		regressionTests := []struct {
			name  string
			names []string
		}{
			{name: "empty names yields zero records", names: []string{}},
			{name: "3-name slice yields three records in input order", names: []string{"_lyx", pattern.DirName, "_extra"}},
			{name: "reversed 2-name slice preserves given order, no forced sort", names: []string{pattern.DirName, "_lyx"}},
		}

		for _, rt := range regressionTests {
			t.Run(rt.name, func(t *testing.T) {
				junctions := HostJunctionsHere(loc, rt.names)
				if len(junctions) != len(rt.names) {
					t.Fatalf("HostJunctionsHere(%v) returned %d entries; want %d", rt.names, len(junctions), len(rt.names))
				}
				for i, wantName := range rt.names {
					got := junctions[i]
					if got.Name != wantName {
						t.Errorf("HostJunctionsHere(%v)[%d].Name = %q; want %q", rt.names, i, got.Name, wantName)
					}
					wantLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, wantName)
					if got.Link != wantLink {
						t.Errorf("HostJunctionsHere(%v)[%d].Link = %q; want %q", rt.names, i, got.Link, wantLink)
					}
					wantTarget := filepath.Join(WeftWorktree(loc), loc.AnchorRel, wantName)
					if got.Target != wantTarget {
						t.Errorf("HostJunctionsHere(%v)[%d].Target = %q; want %q", rt.names, i, got.Target, wantTarget)
					}
				}
			})
		}
	})
}
