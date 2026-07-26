// normalize_test.go covers normalizeCardPath's three-case root:/// resolution
// rule directly: the "//" worktree-root escape, a set root's join, the degenerate
// "root: ." case, and the malformed forms (a single-"/" prefix, a ".." escape)
// that normalizeCardPath resolves but deliberately does not reject — that is
// Validate's card-path-malformed check, not this package's job.

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

// TestNormalizeCard_MovesBothEndpoints covers normalizeCard applying
// normalizeCardPath to both sides of every Moves: pair, and leaving MovesRaw
// (malformed bullets) untouched since they never resolved into a MovePair.
func TestNormalizeCard_MovesBothEndpoints(t *testing.T) {
	t.Parallel()

	card := Card{
		EditsFiles: []string{"list.go"},
		Moves: []MovePair{
			{Old: "old.go", New: "//cmd/lyx/new.go"},
		},
		MovesRaw: []string{"a malformed bullet, left as-is"},
	}
	normalizeCard(&card, "internal/boardcli")

	if want := []string{"internal/boardcli/list.go"}; card.EditsFiles[0] != want[0] {
		t.Errorf("card.EditsFiles = %v; want %v", card.EditsFiles, want)
	}
	wantMoves := []MovePair{{Old: "internal/boardcli/old.go", New: "cmd/lyx/new.go"}}
	if card.Moves[0] != wantMoves[0] {
		t.Errorf("card.Moves = %+v; want %+v (crossing the root boundary)", card.Moves, wantMoves)
	}
	if card.MovesRaw[0] != "a malformed bullet, left as-is" {
		t.Errorf("card.MovesRaw = %v; want it untouched", card.MovesRaw)
	}
}

// TestNormalizeCard_NilSliceStaysNil proves normalizeCard preserves the
// nil-vs-empty-non-nil distinction: a field whose label was never seen (nil
// slice) stays nil after normalization, never silently promoted to an empty
// non-nil slice.
func TestNormalizeCard_NilSliceStaysNil(t *testing.T) {
	t.Parallel()

	card := Card{}
	normalizeCard(&card, "internal/boardcli")

	if card.ContextFiles != nil {
		t.Errorf("card.ContextFiles = %v; want nil", card.ContextFiles)
	}
	if card.EditsFiles != nil {
		t.Errorf("card.EditsFiles = %v; want nil", card.EditsFiles)
	}
}
