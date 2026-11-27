// warpjunction_test.go covers the warp-side junction primitives and the WarpJunction record shape.
// It builds synthetic *lymcwd.Location literals rather than real fixtures.
// Every _extra row asserts against the generic config-driven junction join.
// That join is filepath.Join(WorktreePath(l, slug), l.AnchorRel, "_extra"), not a pattern-specific accessor.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWarpLymLinkHere covers WarpLymLinkHere(l) with AnchorRel "." and subpath cases.
// It verifies AnchorRel mirroring.
//
//testtiming:keep WarpLymLinkHere mirroring AnchorRel; coverage of its blocks by other tests does not show an assertion of this
func TestWarpLymLinkHere(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name                string
		hub                 string
		relPath             string
		wantWarpLymLinkHere string
	}{
		{
			name:                "/h /h/main feat . case",
			hub:                 "/h",
			relPath:             ".",
			wantWarpLymLinkHere: filepath.Join("/h", "feat", "_lym"),
		},
		{
			name:                "/h /h/main feat sub case",
			hub:                 "/h",
			relPath:             "sub",
			wantWarpLymLinkHere: filepath.Join("/h", "feat", "sub", "_lym"),
		},
		{
			name:                "/h /h/main feat sub/dir case",
			hub:                 "/h",
			relPath:             "sub/dir",
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

			if got := WarpLymLinkHere(loc); got != tt.wantWarpLymLinkHere {
				t.Errorf("WarpLymLinkHere(l) = %q; want %q", got, tt.wantWarpLymLinkHere)
			}
		})
	}
}

// TestWarpJunctions verifies that WarpJunctions(l, slug, names) returns one record per name, in the order given.
// Link and Target are composed from the worktree, the weft sibling path and AnchorRel.
// The cases cover AnchorRel "." and a nested AnchorRel; an empty names slice; a 3-name slice; a reversed 2-name slice.
// WarpJunctions takes names as a plain slice and does no sourcing, so the two-name set is passed literally.
// The _extra row asserts against the generic join, not a pattern-specific accessor.
//
//testtiming:keep WarpJunctions returning one record per name in input order with the Link and Target composed from the worktree, weft sibling and anchor; coverage of its blocks by other tests does not show an assertion of this
func TestWarpJunctions(t *testing.T) {
	tiergate.Require(t, 0)
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
}

// TestWarpJunctionsHere checks that WarpJunctionsHere agrees with WarpJunctions(l, slug, names), entry for entry.
// That agreement holds when the slug is the current worktree's, and health-check call sites rely on it.
//
//testtiming:keep the slug-free junction accessor agreeing with WarpJunctions; coverage of its blocks by other tests does not show an assertion of this
func TestWarpJunctionsHere(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("agrees with WarpJunctions when slug matches current worktree", func(t *testing.T) {
		loc := &lymcwd.Location{HubPath: "/h", WorktreeName: "feat", AnchorRel: "."}

		// The slug is the current worktree's base name.
		// With it, WarpJunctions(l, slug, names) resolves to the warp worktree that WarpJunctionsHere(l, names) is anchored at.
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
}
