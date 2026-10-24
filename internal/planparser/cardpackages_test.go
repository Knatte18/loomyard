//go:build tier0

// cardpackages_test.go covers CardPackages: the Go packages a set of cards' targets live in, grouped per module.

package planparser_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestCardPackages(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	for name, content := range map[string]string{
		"tools/go.mod":          "module example.com/tools\n",
		"internal/disk/disk.go": "package disk\n",
		"stencils/x.md":         "x\n",
	} {
		path := filepath.Join(worktree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plan := &planparser.Plan{Language: "go"}
	card := func(number int, groups ...planparser.TargetGroup) planparser.Card {
		return planparser.Card{Number: number, TargetGroups: groups}
	}
	edit := func(refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: planparser.CardTypeEdit, Refs: refs}
	}
	deleted := func(refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: refs}
	}

	tests := []struct {
		name  string
		cards []planparser.Card
		want  []planparser.ModulePackages
	}{
		{
			name:  "a root-module package",
			cards: []planparser.Card{card(1, edit("internal/a/a.go"))},
			want:  []planparser.ModulePackages{{Dir: ".", Packages: []string{"./internal/a"}}},
		},
		{
			name:  "a nested module's package is module-relative and follows the root module",
			cards: []planparser.Card{card(1, edit("tools/x/x.go", "internal/a/a.go"))},
			want: []planparser.ModulePackages{
				{Dir: ".", Packages: []string{"./internal/a"}},
				{Dir: "tools", Packages: []string{"./x"}},
			},
		},
		{
			name:  "a Delete-only directory and a stencil-only directory drop out",
			cards: []planparser.Card{card(1, deleted("internal/old/old.go"), edit("stencils/x.md"))},
			want:  nil,
		},
		{
			name:  "a directory holding a Go file on disk is kept without naming Go",
			cards: []planparser.Card{card(1, edit("internal/disk"))},
			want:  []planparser.ModulePackages{{Dir: ".", Packages: []string{"./internal/disk"}}},
		},
		{
			name:  "a repeated directory is listed once",
			cards: []planparser.Card{card(1, edit("internal/a/a.go")), card(2, edit("internal/a/b.go", "internal/b/b.go"))},
			want:  []planparser.ModulePackages{{Dir: ".", Packages: []string{"./internal/a", "./internal/b"}}},
		},
		{
			name:  "no cards",
			cards: nil,
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := planparser.CardPackages(plan, tt.cards, worktree); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CardPackages() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
