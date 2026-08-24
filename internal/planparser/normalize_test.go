// normalize_test.go covers normalizeCardPath's three-case root:/// resolution rule directly: the
// "//" worktree-root escape, a set root's join, the degenerate "root: ."
// case,
// and the malformed forms (a single-"/" prefix, a ".."
// escape) that normalizeCardPath resolves but deliberately does not reject — that is Validate's
// card-path-malformed check, not this package's job. It also covers normalizeCard's
// classifier-gated application of that rule across Targets, Uses, and both endpoints of every
// Rename Pairs entry, and the nil-vs-empty-non-nil preservation those slices must keep.

package planparser

import "testing"

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
// distinction on both Targets and Uses.
func TestNormalizeCard_NilSliceStaysNil(t *testing.T) {
	t.Parallel()

	card := Card{}
	normalizeCard(&card, "internal/boardcli")

	if card.Targets != nil {
		t.Errorf("card.Targets = %v; want nil", card.Targets)
	}
	if card.Uses != nil {
		t.Errorf("card.Uses = %v; want nil", card.Uses)
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
