// normalize_test.go covers normalizeCardPath's root:/// resolution rule, including malformed forms it does not reject.
// It also covers normalizeCard's classifier-gated application of that rule across the card's path lists and pairs.
// It also checks that those slices keep nil and empty non-nil distinct.

package planparser

import (
	"reflect"
	"testing"

	"github.com/Knatte18/quarry/glyph"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestNormalizeCardPath(t *testing.T) {
	tiergate.Require(t, 0)
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
			raw:  "//cmd/lym/main.go",
			want: "cmd/lym/main.go",
		},
		{
			name: "// always worktree-root-relative, root absent",
			root: "",
			raw:  "//cmd/lym/main.go",
			want: "cmd/lym/main.go",
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
			// The join onto a set root must keep the leading "/" marker that card-path-malformed keys on.
			// Without that marker, the check would be silently disabled for every plan that sets a root.
			name: "malformed single-/ prefix survives a set root, not absorbed into it",
			root: "internal/boardcli",
			raw:  "/etc/passwd",
			want: "/etc/passwd",
		},
		{
			// An empty entry must stay empty under a set root.
			// Joining it onto root would yield the root directory and leave the validator's "empty entry" branch unreachable.
			name: "empty entry survives a set root, not resolved to the root directory",
			root: "internal/boardcli",
			raw:  "",
			want: "",
		},
		{
			// A ".." gets no carve-out: it is resolved against root.
			// Only one that climbs past the worktree root survives as a leading "..".
			// Such a leading ".." is what card-path-malformed keys on.
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

// TestNormalizeCard covers normalizeCard's root join across a whole card.
// Both endpoints of every Rename Pairs entry and every TargetGroups entry's Refs and Pairs are normalized.
// Targets and Pairs agree afterwards.
// The shape classifier runs before an entry is touched.
// A symbol, a glyph and a `plan:` handle pass through byte-identical;
// a path is root-joined, and a "//" escape resolves against the worktree root.
// The nil-versus-empty-non-nil distinction on Targets, Uses and every group's Refs survives.
// The test compares each card whole.
// A nil-versus-empty-non-nil slice, an untouched RenameRaw bullet or a stray field fails it.
//
//testtiming:keep pins normalizeCard's whole-card root join, nil-versus-empty preservation and classifier gate, which its covering tests do not assert
func TestNormalizeCard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		card Card
		want Card
	}{
		{
			name: "both endpoints of a rename pair, a malformed bullet untouched",
			card: Card{
				Targets:   []string{"list.go"},
				Pairs:     []MovePair{{Old: "old.go", New: "//cmd/lym/new.go"}},
				RenameRaw: []string{"a malformed bullet, left as-is"},
			},
			want: Card{
				Targets:   []string{"internal/boardcli/list.go"},
				Pairs:     []MovePair{{Old: "internal/boardcli/old.go", New: "cmd/lym/new.go"}},
				RenameRaw: []string{"a malformed bullet, left as-is"},
			},
		},
		{
			name: "nil slices stay nil and an empty non-nil group slice stays non-nil",
			card: Card{TargetGroups: []TargetGroup{
				{Type: CardTypeCustom, Refs: nil},
				{Type: CardTypeEdit, Refs: []string{}},
			}},
			want: Card{TargetGroups: []TargetGroup{
				{Type: CardTypeCustom, Refs: nil},
				{Type: CardTypeEdit, Refs: []string{}},
			}},
		},
		{
			name: "target groups and the flat Targets agree, a symbol passing through verbatim",
			card: Card{
				Targets: []string{"list.go", "//cmd/lym/main.go", "boardcli.RowJSON"},
				TargetGroups: []TargetGroup{
					{Type: CardTypeEdit, Refs: []string{"list.go", "//cmd/lym/main.go"}},
					{Type: CardTypeCreate, Refs: []string{"boardcli.RowJSON"}},
				},
			},
			want: Card{
				Targets: []string{"internal/boardcli/list.go", "cmd/lym/main.go", "boardcli.RowJSON"},
				TargetGroups: []TargetGroup{
					{Type: CardTypeEdit, Refs: []string{"internal/boardcli/list.go", "cmd/lym/main.go"}},
					{Type: CardTypeCreate, Refs: []string{"boardcli.RowJSON"}},
				},
			},
		},
		{
			// The group-scoped path-missing check stats this value,
			// so an un-normalized group pair would make it stat the unprefixed path.
			name: "a rename group's own pairs are root-joined",
			card: Card{TargetGroups: []TargetGroup{{
				Type:  CardTypeRename,
				Pairs: []MovePair{{Old: "old.go", New: "//cmd/lym/new.go"}},
			}}},
			want: Card{TargetGroups: []TargetGroup{{
				Type:  CardTypeRename,
				Pairs: []MovePair{{Old: "internal/boardcli/old.go", New: "cmd/lym/new.go"}},
			}}},
		},
		{
			name: "only a path is root-joined, in Targets and in Uses",
			card: Card{
				Targets: []string{"boardcli.newListCmd", "list.go", "internal/boardcli#RowJSON", "plan:internal/boardcli#RowJSON", "//cmd/lym/main.go"},
				Uses:    []string{"boardcli.RowJSON", "helpers.go"},
			},
			want: Card{
				Targets: []string{"boardcli.newListCmd", "internal/boardcli/list.go", "internal/boardcli#RowJSON", "plan:internal/boardcli#RowJSON", "cmd/lym/main.go"},
				Uses:    []string{"boardcli.RowJSON", "internal/boardcli/helpers.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			card := tt.card
			normalizeCard(&card, "internal/boardcli")
			if !reflect.DeepEqual(card, tt.want) {
				t.Errorf("normalizeCard() = %#v; want %#v", card, tt.want)
			}
		})
	}
}

// pipeline normalizes card against root, then canonicalizes it under cardKey.
// It records the surface lexemes into surface.
func pipeline(card *Card, root, cardKey string, surface map[string]map[string][]string) {
	normalizeCard(card, root)
	canonicalizeCard(card, cardKey, glyph.Go, surface)
}

// TestCanonicalizeCard_Target covers how canonicalizeCard spells one Targets entry.
// An extension-carrying path canonicalizes into its glyph string.
// So does a slash-free extensionless path, which names a repository-root file:
// a bare token would wedge every glyph-backed layer downstream.
// A slashed extensionless path survives untouched, leaving a directory target to classify.
// A glyph-shaped ref, copied verbatim from a quarry answer, is never root:-joined or touched.
// That includes a bare-filename surface glyph such as "focus.go#".
// Under a non-"." root:, that glyph names the repository-root file.
// A plain path and its own file self glyph land on the identical canonical string.
func TestCanonicalizeCard_Target(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		root string
		raw  string
		want string
	}{
		{name: "extension-carrying path canonicalizes", root: "internal/boardcli", raw: "list.go", want: "internal/boardcli/list.go#"},
		{name: "file self glyph agrees with its plain path", root: "internal/boardcli", raw: "internal/boardcli/list.go#", want: "internal/boardcli/list.go#"},
		{name: "bare extensionless filename under a non-\".\" root root-joins into a slashed path", root: "internal/boardcli", raw: "Makefile", want: "internal/boardcli/Makefile"},
		{name: "bare extensionless filename under an empty root canonicalizes to its self glyph", root: "", raw: "Makefile", want: "Makefile#"},
		{name: "worktree-root-escaped extensionless filename canonicalizes", root: "internal/boardcli", raw: "//LICENSE", want: "LICENSE#"},
		{name: "slashed extensionless directory path survives untouched", root: "", raw: "internal/foo", want: "internal/foo"},
		{name: "member glyph is never root-joined", root: "internal/boardcli", raw: "internal/boardcli#RowJSON", want: "internal/boardcli#RowJSON"},
		{name: "unit self glyph is never root-joined", root: "internal/boardcli", raw: "internal/boardcli#", want: "internal/boardcli#"},
		{name: "bare-filename surface glyph names the repository-root file", root: "internal/boardcli", raw: "focus.go#", want: "focus.go#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			card := Card{Targets: []string{tt.raw}}
			pipeline(&card, tt.root, "1-a", make(map[string]map[string][]string))
			if card.Targets[0] != tt.want {
				t.Errorf("card.Targets[0] = %q; want %q", card.Targets[0], tt.want)
			}
		})
	}
}

// TestCanonicalizeCard_SurfaceRefs covers the pre-canonicalization surface lexemes that canonicalizeCard records.
// They are keyed by the owning card and by the canonical string.
// That lets RewriteRefs restore the exact byte form a card's file carried.
// A repository-root filename is recorded under its new canonical string.
// Two cards that spell one canonical string differently each keep their own entry.
// One card that spells one canonical ref two ways across two fields keeps one deduplicated lexeme.
// Recording only the last spelling would make RewriteRefs rewrite one bullet and leave the other stale.
func TestCanonicalizeCard_SurfaceRefs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const canonical = "internal/boardcli/list.go#"
	type keyedCard struct {
		key  string
		root string
		card Card
	}
	tests := []struct {
		name  string
		cards []keyedCard
		want  map[string]map[string][]string
	}{
		{
			name:  "repository-root filename",
			cards: []keyedCard{{key: "1-a", root: "", card: Card{Targets: []string{"LICENSE"}}}},
			want:  map[string]map[string][]string{"1-a": {"LICENSE#": {"LICENSE"}}},
		},
		{
			name: "two cards spell one canonical string differently",
			cards: []keyedCard{
				{key: "1-a", root: "internal/boardcli", card: Card{Targets: []string{"list.go"}}},
				{key: "2-b", root: "unrelated/root", card: Card{Targets: []string{"//internal/boardcli/list.go"}}},
			},
			want: map[string]map[string][]string{
				"1-a": {canonical: {"internal/boardcli/list.go"}},
				"2-b": {canonical: {"internal/boardcli/list.go"}},
			},
		},
		{
			name: "one card spells one canonical ref two ways",
			cards: []keyedCard{{key: "1-a", root: "internal/boardcli", card: Card{
				Targets: []string{"list.go"},
				Uses:    []string{"//internal/boardcli/list.go"},
			}}},
			want: map[string]map[string][]string{"1-a": {canonical: {"internal/boardcli/list.go"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			surface := make(map[string]map[string][]string)
			for _, kc := range tt.cards {
				card := kc.card
				pipeline(&card, kc.root, kc.key, surface)
			}
			if !reflect.DeepEqual(surface, tt.want) {
				t.Errorf("surface = %v; want %v", surface, tt.want)
			}
		})
	}
}
