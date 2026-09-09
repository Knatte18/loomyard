// normalize_test.go covers normalizeCardPath's three-case root:/// resolution rule directly: the
// "//" worktree-root escape, a set root's join, the degenerate "root: ."
// case,
// and the malformed forms (a single-"/" prefix, a ".."
// escape) that normalizeCardPath resolves but deliberately does not reject — that is Validate's
// card-path-malformed check, not this package's job. It also covers normalizeCard's
// classifier-gated application of that rule across Targets, Uses, both endpoints of every Rename
// Pairs entry, and every TargetGroups entry's own Refs/Pairs, and the nil-vs-empty-non-nil
// preservation those slices must keep.

package planparser

import (
	"slices"
	"testing"

	"github.com/Knatte18/quarry/glyph"
)

func TestNormalizeCardPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		root string
		raw  string
		want string
	}{
		{
			name: "// always worktree-root-relative, root set",
			root: "internal/boardcli",
			raw:  "//cmd/lyx/main.go",
			want: "cmd/lyx/main.go",
		},
		{
			name: "// always worktree-root-relative, root absent",
			root: "",
			raw:  "//cmd/lyx/main.go",
			want: "cmd/lyx/main.go",
		},
		{
			name: "root set joins root/path",
			root: "internal/boardcli",
			raw:  "list.go",
			want: "internal/boardcli/list.go",
		},
		{
			name: "root absent stores the path unchanged",
			root: "",
			raw:  "internal/boardcli/list.go",
			want: "internal/boardcli/list.go",
		},
		{
			name: `root: "." joins to raw unchanged, not an unclean "./raw"`,
			root: ".",
			raw:  "list.go",
			want: "list.go",
		},
		{
			name: "malformed single-/ prefix is left in place, not rejected",
			root: "",
			raw:  "/etc/passwd",
			want: "/etc/passwd",
		},
		{
			name: "malformed leading .. escape is left in place, not rejected",
			root: "",
			raw:  "../secret.go",
			want: "../secret.go",
		},
		{
			// R6-5: joining onto root collapsed the doubled separator and erased the leading-"/"
			// marker card-path-malformed keys on, so the malformed check was silently disabled for
			// every plan that set a root.
			name: "malformed single-/ prefix survives a set root, not absorbed into it",
			root: "internal/boardcli",
			raw:  "/etc/passwd",
			want: "/etc/passwd",
		},
		{
			// R6-5: an empty entry became path.Clean("internal/boardcli/") == the root directory,
			// making the validator's own "empty entry" branch unreachable under a set root.
			name: "empty entry survives a set root, not resolved to the root directory",
			root: "internal/boardcli",
			raw:  "",
			want: "",
		},
		{
			// A ".." needs no carve-out of its own: it is resolved against root, and only one that
			// climbs PAST the worktree root survives as a leading "..", which is exactly what
			// card-path-malformed keys on.
			name: "a .. that climbs past the worktree root survives a set root as an escape",
			root: "internal/boardcli",
			raw:  "../../../secret.go",
			want: "../secret.go",
		},
		{
			name: "harmless internal .. collapses away, not an escape",
			root: "internal",
			raw:  "boardcli/../boardengine/rows.go",
			want: "internal/boardengine/rows.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeCardPath(tt.root, tt.raw)
			if got != tt.want {
				t.Errorf("normalizeCardPath(%q, %q) = %q; want %q", tt.root, tt.raw, got, tt.want)
			}
		})
	}
}

// TestNormalizeCard_PairsBothEndpoints covers normalizeCard applying normalizeCardPath to both
// sides of every Rename Pairs entry.
func TestNormalizeCard_PairsBothEndpoints(t *testing.T) {
	t.Parallel()

	card := Card{
		Targets: []string{"list.go"},
		Pairs: []MovePair{
			{Old: "old.go", New: "//cmd/lyx/new.go"},
		},
		RenameRaw: []string{"a malformed bullet, left as-is"},
	}
	normalizeCard(&card, "internal/boardcli")

	if want := "internal/boardcli/list.go"; card.Targets[0] != want {
		t.Errorf("card.Targets[0] = %q; want %q", card.Targets[0], want)
	}
	wantPair := MovePair{Old: "internal/boardcli/old.go", New: "cmd/lyx/new.go"}
	if card.Pairs[0] != wantPair {
		t.Errorf("card.Pairs[0] = %+v; want %+v (crossing the root boundary)", card.Pairs[0], wantPair)
	}
	if card.RenameRaw[0] != "a malformed bullet, left as-is" {
		t.Errorf("card.RenameRaw = %v; want it untouched", card.RenameRaw)
	}
}

// TestNormalizeCard_NilSliceStaysNil proves normalizeCard preserves the nil-vs-empty-non-nil
// distinction on Targets, Uses, and every TargetGroups entry's own Refs.
func TestNormalizeCard_NilSliceStaysNil(t *testing.T) {
	t.Parallel()

	card := Card{
		TargetGroups: []TargetGroup{
			{Type: CardTypeCustom, Refs: nil},
			{Type: CardTypeEdit, Refs: []string{}},
		},
	}
	normalizeCard(&card, "internal/boardcli")

	if card.Targets != nil {
		t.Errorf("card.Targets = %v; want nil", card.Targets)
	}
	if card.Uses != nil {
		t.Errorf("card.Uses = %v; want nil", card.Uses)
	}
	if card.TargetGroups[0].Refs != nil {
		t.Errorf("card.TargetGroups[0].Refs = %v; want nil", card.TargetGroups[0].Refs)
	}
	if card.TargetGroups[1].Refs == nil {
		t.Errorf("card.TargetGroups[1].Refs = nil; want a non-nil, zero-length slice")
	}
	if len(card.TargetGroups[1].Refs) != 0 {
		t.Errorf("card.TargetGroups[1].Refs = %v; want empty", card.TargetGroups[1].Refs)
	}
}

// TestNormalizeCard_TargetGroupsPostCondition proves normalizeCard's stated post-condition under a
// non-empty root: with a "//"-escaped entry and a symbol-shaped entry in a group: Card.Targets
// equals the concatenation of TargetGroups[*].Refs in body order, Card.Pairs equals the
// concatenation of TargetGroups[*].Pairs in body order, and the symbol-shaped entry passes through
// verbatim on both sides.
func TestNormalizeCard_TargetGroupsPostCondition(t *testing.T) {
	t.Parallel()

	card := Card{
		Targets: []string{"list.go", "//cmd/lyx/main.go", "boardcli.RowJSON"},
		TargetGroups: []TargetGroup{
			{Type: CardTypeEdit, Refs: []string{"list.go", "//cmd/lyx/main.go"}},
			{Type: CardTypeCreate, Refs: []string{"boardcli.RowJSON"}},
		},
	}
	normalizeCard(&card, "internal/boardcli")

	wantGroup0Refs := []string{"internal/boardcli/list.go", "cmd/lyx/main.go"}
	if !slices.Equal(card.TargetGroups[0].Refs, wantGroup0Refs) {
		t.Errorf("card.TargetGroups[0].Refs = %v; want %v", card.TargetGroups[0].Refs, wantGroup0Refs)
	}
	wantGroup1Refs := []string{"boardcli.RowJSON"}
	if !slices.Equal(card.TargetGroups[1].Refs, wantGroup1Refs) {
		t.Errorf("card.TargetGroups[1].Refs = %v; want %v (symbol unmodified)", card.TargetGroups[1].Refs, wantGroup1Refs)
	}

	var wantTargets []string
	for _, g := range card.TargetGroups {
		wantTargets = append(wantTargets, g.Refs...)
	}
	if !slices.Equal(card.Targets, wantTargets) {
		t.Errorf("card.Targets = %v; want %v (concatenation of TargetGroups[*].Refs, body order)", card.Targets, wantTargets)
	}
}

// TestNormalizeCard_RenameGroupPairsRootJoined proves a Rename group's own Pairs.Old is
// root-joined after normalizeCard, since that is the value the group-scoped path-missing check
// stats — an un-normalized group pair would make it stat the unprefixed path.
func TestNormalizeCard_RenameGroupPairsRootJoined(t *testing.T) {
	t.Parallel()

	card := Card{
		TargetGroups: []TargetGroup{
			{
				Type: CardTypeRename,
				Pairs: []MovePair{
					{Old: "old.go", New: "//cmd/lyx/new.go"},
				},
			},
		},
	}
	normalizeCard(&card, "internal/boardcli")

	wantPair := MovePair{Old: "internal/boardcli/old.go", New: "cmd/lyx/new.go"}
	if card.TargetGroups[0].Pairs[0] != wantPair {
		t.Errorf("card.TargetGroups[0].Pairs[0] = %+v; want %+v", card.TargetGroups[0].Pairs[0], wantPair)
	}
}

// TestNormalizeCard_ClassifierGate proves normalizeCard consults the shape classifier before
// touching an entry: a symbol entry in Targets and a symbol entry in Uses each pass through
// byte-identical, while a path entry in the same list is root-joined — the single sharpest
// regression this migration can introduce.
func TestNormalizeCard_ClassifierGate(t *testing.T) {
	t.Parallel()

	card := Card{
		Targets: []string{"boardcli.newListCmd", "list.go"},
		Uses:    []string{"boardcli.RowJSON", "helpers.go"},
	}
	normalizeCard(&card, "internal/boardcli")

	if want := "boardcli.newListCmd"; card.Targets[0] != want {
		t.Errorf("card.Targets[0] (symbol) = %q; want %q unmodified", card.Targets[0], want)
	}
	if want := "internal/boardcli/list.go"; card.Targets[1] != want {
		t.Errorf("card.Targets[1] (path) = %q; want %q root-joined", card.Targets[1], want)
	}
	if want := "boardcli.RowJSON"; card.Uses[0] != want {
		t.Errorf("card.Uses[0] (symbol) = %q; want %q unmodified", card.Uses[0], want)
	}
	if want := "internal/boardcli/helpers.go"; card.Uses[1] != want {
		t.Errorf("card.Uses[1] (path) = %q; want %q root-joined", card.Uses[1], want)
	}
}

// TestNormalizeCard_PairsAndTargetsAgree proves a Rename card's Pairs and its projected Targets
// normalize to the same strings on both endpoints, so the two representations cannot drift apart.
func TestNormalizeCard_PairsAndTargetsAgree(t *testing.T) {
	t.Parallel()

	card := Card{
		Targets: []string{"old.go", "//cmd/lyx/new.go"},
		Pairs: []MovePair{
			{Old: "old.go", New: "//cmd/lyx/new.go"},
		},
	}
	normalizeCard(&card, "internal/boardcli")

	if card.Targets[0] != card.Pairs[0].Old {
		t.Errorf("card.Targets[0] = %q; card.Pairs[0].Old = %q; want them equal", card.Targets[0], card.Pairs[0].Old)
	}
	if card.Targets[1] != card.Pairs[0].New {
		t.Errorf("card.Targets[1] = %q; card.Pairs[0].New = %q; want them equal", card.Targets[1], card.Pairs[0].New)
	}
}

// TestNormalizeRefIfPath_GateAcrossAllKinds pins gateNormalizePath, the single root:-join gate this
// migration's own doc comment calls its highest-risk site: under a non-"." root, a path-shaped raw
// is root:-joined, and every other shape -- a bare symbol, a glyph, a plan: handle -- passes through
// byte-identical, plus the "//" worktree-root escape resolving to its worktree-root-relative form
// regardless of root.
func TestNormalizeRefIfPath_GateAcrossAllKinds(t *testing.T) {
	t.Parallel()

	const root = "internal/boardcli"

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "path is root-joined", raw: "list.go", want: "internal/boardcli/list.go"},
		{name: "bare symbol passes through untouched", raw: "boardcli.RowJSON", want: "boardcli.RowJSON"},
		{name: "glyph passes through untouched", raw: "internal/boardcli#RowJSON", want: "internal/boardcli#RowJSON"},
		{name: "plan: handle passes through untouched", raw: "plan:internal/boardcli#RowJSON", want: "plan:internal/boardcli#RowJSON"},
		{name: "\"//\" worktree-root escape resolves relative to the worktree root, not root", raw: "//cmd/lyx/main.go", want: "cmd/lyx/main.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeRefIfPath(root, tt.raw)
			if got != tt.want {
				t.Errorf("normalizeRefIfPath(%q, %q) = %q; want %q", root, tt.raw, got, tt.want)
			}
		})
	}
}

// pipeline runs normalizeCard then canonicalizeCard on card, in the exact order ParsePlan runs
// them, and returns the surface map canonicalizeCard populated.
func pipeline(card *Card, root, cardKey string) map[string]map[string][]string {
	normalizeCard(card, root)
	surface := make(map[string]map[string][]string)
	canonicalizeCard(card, cardKey, glyph.Go, surface)
	return surface
}

// TestCanonicalizeCard_ExtensionGate proves canonicalizablePath's two halves: an
// extension-carrying path-shaped ref is canonicalized into its glyph string, a SLASH-FREE
// extensionless one is canonicalized too — that is classifyRef rule 4's repository-root filename,
// and leaving it a bare token is what wedged every glyph-backed layer downstream (R9-1) — while a
// SLASHED extensionless one survives untouched, leaving directory-target something to classify.
func TestCanonicalizeCard_ExtensionGate(t *testing.T) {
	t.Parallel()

	t.Run("extension-carrying path canonicalizes", func(t *testing.T) {
		t.Parallel()
		card := Card{Targets: []string{"list.go"}}
		pipeline(&card, "internal/boardcli", "1-a")
		if want := "internal/boardcli/list.go#"; card.Targets[0] != want {
			t.Errorf("card.Targets[0] = %q; want %q", card.Targets[0], want)
		}
	})

	t.Run("bare extensionless filename under a non-\".\" root: root-joins into a slashed path and does not canonicalize", func(t *testing.T) {
		t.Parallel()
		card := Card{Targets: []string{"Makefile"}}
		pipeline(&card, "internal/boardcli", "1-a")
		if want := "internal/boardcli/Makefile"; card.Targets[0] != want {
			t.Errorf("card.Targets[0] = %q; want %q (root-joined, and slashed-extensionless so not canonicalized)", card.Targets[0], want)
		}
	})

	t.Run("bare extensionless filename under an empty root: canonicalizes to its self glyph", func(t *testing.T) {
		t.Parallel()
		card := Card{Targets: []string{"Makefile"}}
		pipeline(&card, "", "1-a")
		if want := "Makefile#"; card.Targets[0] != want {
			t.Errorf("card.Targets[0] = %q; want %q (rule 4's repository-root filename must reach the glyph layers as a glyph)", card.Targets[0], want)
		}
	})

	t.Run("worktree-root-escaped extensionless filename canonicalizes under a non-\".\" root:", func(t *testing.T) {
		t.Parallel()
		card := Card{Targets: []string{"//LICENSE"}}
		pipeline(&card, "internal/boardcli", "1-a")
		if want := "LICENSE#"; card.Targets[0] != want {
			t.Errorf("card.Targets[0] = %q; want %q", card.Targets[0], want)
		}
	})

	t.Run("slashed extensionless directory path survives canonicalization untouched", func(t *testing.T) {
		t.Parallel()
		card := Card{Targets: []string{"internal/foo"}}
		pipeline(&card, "", "1-a")
		if want := "internal/foo"; card.Targets[0] != want {
			t.Errorf("card.Targets[0] = %q; want %q (untouched, so directory-target still has something to classify)", card.Targets[0], want)
		}
	})
}

// TestCanonicalizeCard_RootFilenameSurfaceRefRecorded proves the surface lexeme of a rule-4
// repository-root filename is recorded under its new canonical string, so RewriteRefs can still
// restore the exact byte-form the card's own file carried.
func TestCanonicalizeCard_RootFilenameSurfaceRefRecorded(t *testing.T) {
	t.Parallel()

	card := Card{Targets: []string{"LICENSE"}}
	surface := make(map[string]map[string][]string)
	normalizeCard(&card, "")
	canonicalizeCard(&card, "1-a", glyph.Go, surface)

	got := surface["1-a"]["LICENSE#"]
	if len(got) != 1 || got[0] != "LICENSE" {
		t.Fatalf("surface[1-a][LICENSE#] = %#v; want [\"LICENSE\"]", got)
	}
}

// TestCanonicalizeCard_GlyphNeverRootJoined proves a glyph-shaped ref copied verbatim from a
// quarry answer is never touched by canonicalizeCard and never root:-joined by the preceding
// normalizeCard: it is already a complete repository-relative string, and prefixing root: onto
// one would corrupt it. This includes the documented consequence that a bare-filename surface
// glyph such as "focus.go#" under a non-"." root: names the repository-root file and is left
// alone.
func TestCanonicalizeCard_GlyphNeverRootJoined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "member glyph", raw: "internal/boardcli#RowJSON"},
		{name: "unit self glyph", raw: "internal/boardcli#"},
		{name: "file self glyph", raw: "internal/boardcli/list.go#"},
		{name: "bare-filename surface glyph", raw: "focus.go#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			card := Card{Targets: []string{tt.raw}}
			pipeline(&card, "internal/boardcli", "1-a")
			if card.Targets[0] != tt.raw {
				t.Errorf("card.Targets[0] = %q; want %q unmodified", card.Targets[0], tt.raw)
			}
		})
	}
}

// TestCanonicalizeCard_PlainPathAndFileSelfGlyphAgree proves a plain path and its own file self
// glyph land on the identical canonical model string.
func TestCanonicalizeCard_PlainPathAndFileSelfGlyphAgree(t *testing.T) {
	t.Parallel()

	plainPathCard := Card{Targets: []string{"list.go"}}
	pipeline(&plainPathCard, "internal/boardcli", "1-a")

	glyphCard := Card{Targets: []string{"internal/boardcli/list.go#"}}
	pipeline(&glyphCard, "internal/boardcli", "2-b")

	if plainPathCard.Targets[0] != glyphCard.Targets[0] {
		t.Errorf("plain path canonical = %q; file self glyph canonical = %q; want them equal", plainPathCard.Targets[0], glyphCard.Targets[0])
	}
}

// TestCanonicalizeCard_SurfaceRefs proves canonicalizeCard records the pre-canonicalization
// surface lexeme under the owning card's own key, canonical string second, and that two cards
// spelling one canonical string differently each keep their own entry.
func TestCanonicalizeCard_SurfaceRefs(t *testing.T) {
	t.Parallel()

	cardA := Card{Targets: []string{"list.go"}}
	surface := make(map[string]map[string][]string)
	normalizeCard(&cardA, "internal/boardcli")
	canonicalizeCard(&cardA, "1-a", glyph.Go, surface)

	cardB := Card{Targets: []string{"//internal/boardcli/list.go"}}
	normalizeCard(&cardB, "unrelated/root")
	canonicalizeCard(&cardB, "2-b", glyph.Go, surface)

	const canonical = "internal/boardcli/list.go#"
	for _, cardKey := range []string{"1-a", "2-b"} {
		got := surface[cardKey][canonical]
		if len(got) != 1 || got[0] != "internal/boardcli/list.go" {
			t.Errorf(`surface[%q][%q] = %v; want exactly ["internal/boardcli/list.go"]`, cardKey, canonical, got)
		}
	}
}

// TestCanonicalizeCard_SurfaceRefsKeepsEveryLexemeOnOneCard is R6-13's regression test: one card
// may spell one canonical ref two ways across two of its own fields, and recording only the last
// left RewriteRefs rewriting one bullet and leaving the other stale — a half-rewritten card.
func TestCanonicalizeCard_SurfaceRefsKeepsEveryLexemeOnOneCard(t *testing.T) {
	t.Parallel()

	card := Card{
		Targets: []string{"list.go"},
		Uses:    []string{"//internal/boardcli/list.go"},
	}
	surface := make(map[string]map[string][]string)
	normalizeCard(&card, "internal/boardcli")
	canonicalizeCard(&card, "1-a", glyph.Go, surface)

	const canonical = "internal/boardcli/list.go#"
	got := surface["1-a"][canonical]
	if len(got) != 1 {
		t.Fatalf(`surface["1-a"][%q] = %v; want one deduplicated lexeme (both spellings normalize to the same path)`, canonical, got)
	}
}
