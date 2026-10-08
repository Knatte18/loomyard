package planparser_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestNestedModules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, dir := range []string{
		".",
		"nested",
		".hidden/x",
		"_skipped/x",
		"vendor/x",
		"pkg/testdata/fixture",
		"a",
	} {
		full := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "go.mod"), []byte("module m\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plan := func(cards ...planparser.Card) *planparser.Plan {
		return &planparser.Plan{Language: "go", Cards: cards}
	}
	card := func(number int, groups ...planparser.TargetGroup) planparser.Card {
		return planparser.Card{Number: number, TargetGroups: groups}
	}
	create := func(refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: refs}
	}
	remove := func(refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: refs}
	}

	onDisk := []string{"a", "nested", "pkg/testdata/fixture"}
	tests := []struct {
		name    string
		plan    *planparser.Plan
		through int
		want    []string
	}{
		{
			name: "disk set skips dot, underscore and vendor directories, walks testdata and ignores the root",
			plan: plan(),
			want: onDisk,
		},
		{
			name:    "Create on a card at or below through counts",
			plan:    plan(card(2, create("x/go.mod"))),
			through: 2,
			want:    []string{"a", "nested", "pkg/testdata/fixture", "x"},
		},
		{
			name:    "Create on a later card does not count",
			plan:    plan(card(3, create("x/go.mod"))),
			through: 2,
			want:    onDisk,
		},
		{
			name:    "through 0 is the disk set alone",
			plan:    plan(card(1, create("x/go.mod"))),
			through: 0,
			want:    onDisk,
		},
		{
			name:    "Create of a self glyph counts",
			plan:    plan(card(1, create("x/go.mod#"))),
			through: 1,
			want:    []string{"a", "nested", "pkg/testdata/fixture", "x"},
		},
		{
			name:    "Create of the root go.mod is not a nested module",
			plan:    plan(card(1, create("go.mod"))),
			through: 1,
			want:    onDisk,
		},
		{
			name:    "Delete removes an on-disk module",
			plan:    plan(card(1, remove("nested/go.mod"))),
			through: 1,
			want:    []string{"a", "pkg/testdata/fixture"},
		},
		{
			name: "a later card's Create wins over an earlier Delete",
			plan: plan(
				card(2, create("nested/go.mod")),
				card(1, remove("nested/go.mod")),
			),
			through: 2,
			want:    onDisk,
		},
		{
			name: "Rename of go.mod moves the module",
			plan: plan(card(1, planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Pairs: []planparser.MovePair{{Old: "a/go.mod", New: "b/go.mod"}},
			})),
			through: 1,
			want:    []string{"b", "nested", "pkg/testdata/fixture"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := planparser.NestedModules(tt.plan, tt.through, root)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NestedModules = %v, want %v", got, tt.want)
			}
		})
	}

	modules := []string{"a", "a/b", "ab"}
	moduleOf := []struct {
		dir, want string
	}{
		{"a/b/c", "a/b"},
		{"a/b", "a/b"},
		{"a/c", "a"},
		{"a/bc", "a"},
		{"abc", "."},
		{".", "."},
		{"other", "."},
	}
	for _, tt := range moduleOf {
		if got := planparser.ModuleOf(modules, tt.dir); got != tt.want {
			t.Errorf("ModuleOf(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}
