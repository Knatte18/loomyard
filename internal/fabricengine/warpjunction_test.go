//go:build tier0

// warpjunction_test.go covers the warp-side junction primitives relocated from internal/lymcwd in
// this batch — CodeLymLink, WarpLymLinkHere, WarpJunctions and WarpJunctionsHere, plus the
// WarpJunction record shape — over synthetic *lymcwd.Location literals rather than real fixtures,
// the same table shapes lymcwd's own tests used.
// Every _extra row asserts against the generic config-driven junction join
// (filepath.Join(WorktreePath(l, slug), l.AnchorRel, "_extra")) rather than a
// pattern-specific accessor, so this file survives card 35's deletion of those accessors unchanged.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestCodeLymLinkMethods covers CodeLymLink(l, slug) and WarpLymLinkHere(l) with both AnchorRel "."
// (root) and subpath cases, verifying AnchorRel-mirroring and junction pairing against the
// weft-sibling worktree.
//
//testtiming:keep CodeLymLink and WarpLymLinkHere mirroring AnchorRel and pairing with the weft sibling; coverage of its blocks by other tests does not show an assertion of this
func TestCodeLymLinkMethods(t *testing.T) {
	tests := []struct {
		name                string
		hub                 string
		slug                string
		relPath             string
		wantCodeLymLink     string
		wantWarpLymLinkHere string
	}{
		{
			name:                "/h /h/main feat . case",
			hub:                 "/h",
			slug:                "x",
			relPath:             ".",
			wantCodeLymLink:     filepath.Join("/h", "x", "_lym"),
			wantWarpLymLinkHere: filepath.Join("/h", "feat", "_lym"),
		},
		{
			name:                "/h /h/main feat sub case",
			hub:                 "/h",
			slug:                "x",
			relPath:             "sub",
			wantCodeLymLink:     filepath.Join("/h", "x", "sub", "_lym"),
			wantWarpLymLinkHere: filepath.Join("/h", "feat", "sub", "_lym"),
		},
		{
			name:                "/h /h/main feat sub/dir case",
			hub:                 "/h",
			slug:                "y",
			relPath:             "sub/dir",
			wantCodeLymLink:     filepath.Join("/h", "y", "sub/dir", "_lym"),
			wantWarpLymLinkHere: filepath.Join("/h", "feat", "sub/dir", "_lym"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: "feat",
				AnchorRel:    tt.relPath,
			}

			if got := CodeLymLink(loc, tt.slug); got != tt.wantCodeLymLink {
				t.Errorf("CodeLymLink(l, %q) = %q; want %q", tt.slug, got, tt.wantCodeLymLink)
			}

			if got := WarpLymLinkHere(loc); got != tt.wantWarpLymLinkHere {
				t.Errorf("WarpLymLinkHere(l) = %q; want %q", got, tt.wantWarpLymLinkHere)
			}

			// Verify junction pairing: CodeLymLink(l, slug) and the weft
			// sibling's _lym directory are siblings differing only by the
			// -weft suffix on the worktree dir.
			warpWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(weftname.SiblingPath(loc.HubPath, tt.slug))

			if warpWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, warpWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("weftname.SiblingPath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}

			// Verify WarpLymLinkHere is based on WorktreePath+AnchorRel (documented intent).
			warpLymLinkHereVal := WarpLymLinkHere(loc)
			expectedWarpLymLinkHere := filepath.Join(loc.WorktreePath(), loc.AnchorRel, "_lym")
			if warpLymLinkHereVal != expectedWarpLymLinkHere {
				t.Errorf("WarpLymLinkHere(l) = %q; want %q", warpLymLinkHereVal, expectedWarpLymLinkHere)
			}
		})
	}
}

// TestWarpJunctions verifies that WarpJunctions(l, slug, names) returns one record per name in names, in names's own input order, with Link/Target correctly composed from l's WorktreePath/weft-sibling path and AnchorRel, at AnchorRel == "."
// and at a nested AnchorRel, for an empty names slice, a 3-name slice, and a reversed 2-name slice — and that WarpJunctions never augments the caller-supplied names with an unsupplied hub-structural entry of its own.
// WarpJunctions itself takes names as a plain slice and does no sourcing of its own, so the two-name set here is passed literally, exactly as callers pass it today;
// the batch's actual change is upstream, in how junctionNames/WiredNames build that slice — `_lym` now arrives structurally (structuralCommittedDirs) rather than from a config `pathspec` entry, but WarpJunctions itself is unaware of that distinction.
// The _extra row asserts against the generic join, not a pattern-specific accessor.
//
//testtiming:keep WarpJunctions returning one record per name in input order with the Link and Target composed from the worktree, weft sibling and anchor; coverage of its blocks by other tests does not show an assertion of this
func TestWarpJunctions(t *testing.T) {
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
			names:   []string{"_lym", "_extra"},
		},
		{
			name:    "non-prime worktree layout, root case",
			hub:     "/h",
			slug:    "other",
			relPath: ".",
			names:   []string{"_lym", "_extra"},
		},
		{
			name:    "subpath case",
			hub:     "/h",
			slug:    "feat",
			relPath: "sub",
			names:   []string{"_lym", "_extra"},
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
			names:   []string{"_lym", "_extra", "_other"},
		},
		{
			name:    "reversed 2-name slice preserves given order, no forced sort",
			hub:     "/h",
			slug:    "feat",
			relPath: ".",
			names:   []string{"_extra", "_lym"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: tt.slug,
				AnchorRel:    tt.relPath,
			}

			junctions := WarpJunctions(loc, tt.slug, tt.names)

			if len(junctions) != len(tt.names) {
				t.Fatalf("WarpJunctions(l, %q, %v) returned %d entries; want %d", tt.slug, tt.names, len(junctions), len(tt.names))
			}

			for i, wantName := range tt.names {
				got := junctions[i]
				if got.Name != wantName {
					t.Errorf("WarpJunctions(l, %q, %v)[%d].Name = %q; want %q", tt.slug, tt.names, i, got.Name, wantName)
				}
				wantLink := filepath.Join(WorktreePath(loc, tt.slug), loc.AnchorRel, wantName)
				if got.Link != wantLink {
					t.Errorf("WarpJunctions(l, %q, %v)[%d].Link = %q; want %q", tt.slug, tt.names, i, got.Link, wantLink)
				}
				wantTarget := filepath.Join(RecordsWorktreePath(loc, tt.slug), loc.AnchorRel, wantName)
				if got.Target != wantTarget {
					t.Errorf("WarpJunctions(l, %q, %v)[%d].Target = %q; want %q", tt.slug, tt.names, i, got.Target, wantTarget)
				}
			}
		})
	}

	// Sub-test: scope guard — WarpJunctions takes names as a plain slice and returns one
	// record per entry in that slice's own order, doing no HubReservedNames/filterHubReserved
	// filtering of its own; TestFilterHubReserved and TestIsReservedHubName in
	// junctionnames_test.go own the reservation property. This asserts the narrower, still-true
	// claim: WarpJunctions returns exactly the caller-supplied names and never augments them
	// with a hub-structural entry of its own, using "_board" (still reserved via
	// HubReservedNames() after this batch) as the name that must NOT appear unless the caller
	// actually supplied it.
	t.Run("no_unsupplied_hub_structural_names", func(t *testing.T) {
		loc := &lymcwd.Location{
			HubPath:      "/h",
			WorktreeName: "main",
			AnchorRel:    ".",
		}

		junctions := WarpJunctions(loc, "slug", []string{"_lym", "_extra"})
		for _, j := range junctions {
			if j.Name == "_board" {
				t.Errorf("WarpJunctions augmented the caller-supplied names with an unsupplied %q entry", "_board")
			}
		}
	})
}

// TestWarpJunctionsHere verifies the Here-anchored, slug-free junction-detection accessor: it must
// return the expected Name/Link/Target for both RelPath == "."
// and a nested RelPath,
// and it must agree entry-for-entry with WarpJunctions(l, slug, names) when l's slug and current
// worktree coincide — the precondition every health-check call site relies on.
//
//testtiming:keep the slug-free junction accessor returning the expected Name, Link and Target at a root and nested anchor and agreeing with WarpJunctions; coverage of its blocks by other tests does not show an assertion of this
func TestWarpJunctionsHere(t *testing.T) {
	t.Run("at root", func(t *testing.T) {
		loc := &lymcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		junctions := WarpJunctionsHere(loc, []string{"_lym", "_extra"})
		if len(junctions) != 2 {
			t.Fatalf("WarpJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lymJunction := junctions[0]
		wantLymLink := WarpLymLinkHere(loc)
		wantLymTarget := filepath.Join(RecordsWorktree(loc), loc.AnchorRel, "_lym")
		if lymJunction.Name != "_lym" {
			t.Errorf("WarpJunctionsHere()[0].Name = %q; want %q", lymJunction.Name, "_lym")
		}
		if lymJunction.Link != wantLymLink {
			t.Errorf("WarpJunctionsHere()[0].Link = %q; want %q", lymJunction.Link, wantLymLink)
		}
		if lymJunction.Target != wantLymTarget {
			t.Errorf("WarpJunctionsHere()[0].Target = %q; want %q", lymJunction.Target, wantLymTarget)
		}

		extraJunction := junctions[1]
		wantExtraLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, "_extra")
		wantExtraTarget := filepath.Join(RecordsWorktree(loc), loc.AnchorRel, "_extra")
		if extraJunction.Name != "_extra" {
			t.Errorf("WarpJunctionsHere()[1].Name = %q; want %q", extraJunction.Name, "_extra")
		}
		if extraJunction.Link != wantExtraLink {
			t.Errorf("WarpJunctionsHere()[1].Link = %q; want %q", extraJunction.Link, wantExtraLink)
		}
		if extraJunction.Target != wantExtraTarget {
			t.Errorf("WarpJunctionsHere()[1].Target = %q; want %q", extraJunction.Target, wantExtraTarget)
		}
	})

	t.Run("at nested subpath", func(t *testing.T) {
		loc := &lymcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: filepath.Join("services", "api")}

		junctions := WarpJunctionsHere(loc, []string{"_lym", "_extra"})
		if len(junctions) != 2 {
			t.Fatalf("WarpJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lymJunction := junctions[0]
		wantLymLink := WarpLymLinkHere(loc)
		wantLymTarget := filepath.Join(RecordsWorktree(loc), loc.AnchorRel, "_lym")
		if lymJunction.Link != wantLymLink {
			t.Errorf("WarpJunctionsHere()[0].Link = %q; want %q", lymJunction.Link, wantLymLink)
		}
		if lymJunction.Target != wantLymTarget {
			t.Errorf("WarpJunctionsHere()[0].Target = %q; want %q", lymJunction.Target, wantLymTarget)
		}

		extraJunction := junctions[1]
		wantExtraLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, "_extra")
		wantExtraTarget := filepath.Join(RecordsWorktree(loc), loc.AnchorRel, "_extra")
		if extraJunction.Link != wantExtraLink {
			t.Errorf("WarpJunctionsHere()[1].Link = %q; want %q", extraJunction.Link, wantExtraLink)
		}
		if extraJunction.Target != wantExtraTarget {
			t.Errorf("WarpJunctionsHere()[1].Target = %q; want %q", extraJunction.Target, wantExtraTarget)
		}
	})

	t.Run("agrees with WarpJunctions when slug matches current worktree", func(t *testing.T) {
		loc := &lymcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		// The current worktree's own base name is the slug that makes
		// WarpJunctions(l, slug, names) resolve to the same warp worktree
		// WarpJunctionsHere(l, names) is already anchored at.
		slug := filepath.Base(loc.WorktreePath())
		names := []string{"_lym", "_extra"}

		here := WarpJunctionsHere(loc, names)
		bySlug := WarpJunctions(loc, slug, names)

		if len(here) != len(bySlug) {
			t.Fatalf("WarpJunctionsHere() returned %d junctions; WarpJunctions(%q) returned %d", len(here), slug, len(bySlug))
		}
		for i := range here {
			if here[i] != bySlug[i] {
				t.Errorf("WarpJunctionsHere()[%d] = %+v; WarpJunctions(%q)[%d] = %+v", i, here[i], slug, i, bySlug[i])
			}
		}
	})

	t.Run("names ordering regressions", func(t *testing.T) {
		loc := &lymcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		regressionTests := []struct {
			name  string
			names []string
		}{
			{name: "empty names yields zero records", names: []string{}},
			{name: "3-name slice yields three records in input order", names: []string{"_lym", "_extra", "_other"}},
			{name: "reversed 2-name slice preserves given order, no forced sort", names: []string{"_extra", "_lym"}},
		}

		for _, rt := range regressionTests {
			t.Run(rt.name, func(t *testing.T) {
				junctions := WarpJunctionsHere(loc, rt.names)
				if len(junctions) != len(rt.names) {
					t.Fatalf("WarpJunctionsHere(%v) returned %d entries; want %d", rt.names, len(junctions), len(rt.names))
				}
				for i, wantName := range rt.names {
					got := junctions[i]
					if got.Name != wantName {
						t.Errorf("WarpJunctionsHere(%v)[%d].Name = %q; want %q", rt.names, i, got.Name, wantName)
					}
					wantLink := filepath.Join(loc.WorktreePath(), loc.AnchorRel, wantName)
					if got.Link != wantLink {
						t.Errorf("WarpJunctionsHere(%v)[%d].Link = %q; want %q", rt.names, i, got.Link, wantLink)
					}
					wantTarget := filepath.Join(RecordsWorktree(loc), loc.AnchorRel, wantName)
					if got.Target != wantTarget {
						t.Errorf("WarpJunctionsHere(%v)[%d].Target = %q; want %q", rt.names, i, got.Target, wantTarget)
					}
				}
			})
		}
	})
}
