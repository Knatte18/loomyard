package planparser_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestCardTargetDirs(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	tests := []struct {
		name   string
		groups []planparser.TargetGroup
		uses   []string
		want   []planparser.TargetDir
	}{
		{
			name:   "path target maps to its directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "non-Go path names no Go",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeProsa, Refs: []string{"docs/overview.md"}}},
			want:   []planparser.TargetDir{{Dir: "docs"}},
		},
		{
			name:   "directory path maps to itself",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a"}},
		},
		{
			name:   "file self glyph maps to its directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"internal/a/a.go#"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "member glyph in a file unit maps to the unit's directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#Run"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "package glyph maps to the package directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a#Run"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "glyph of a non-Go file names no Go",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeProsa, Refs: []string{"contracts/stencils/x.md#"}}},
			want:   []planparser.TargetDir{{Dir: "contracts/stencils"}},
		},
		{
			name:   "handle maps through its glyph",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"plan:internal/a/a.go#Run"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name: "Rename contributes the New side alone",
			groups: []planparser.TargetGroup{{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"internal/old/o.go#Old", "plan:internal/new/n.go#New"},
				Pairs: []planparser.MovePair{{Old: "internal/old/o.go#Old", New: "plan:internal/new/n.go#New"}},
			}},
			want: []planparser.TargetDir{{Dir: "internal/new", NamesGo: true}},
		},
		{
			name:   "worktree root file maps to dot",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"main.go"}}},
			want:   []planparser.TargetDir{{Dir: ".", NamesGo: true}},
		},
		{
			name: "Delete-only directory is flagged",
			groups: []planparser.TargetGroup{
				{Type: planparser.CardTypeDelete, Refs: []string{"internal/gone/g.go#"}},
				{Type: planparser.CardTypeEdit, Refs: []string{"internal/kept/k.go#"}},
			},
			want: []planparser.TargetDir{
				{Dir: "internal/gone", DeleteOnly: true},
				{Dir: "internal/kept", NamesGo: true},
			},
		},
		{
			name: "a non-Delete target clears DeleteOnly",
			groups: []planparser.TargetGroup{
				{Type: planparser.CardTypeDelete, Refs: []string{"internal/a/old.go#"}},
				{Type: planparser.CardTypeCreate, Refs: []string{"internal/a/new.go#"}},
			},
			want: []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "Uses are excluded",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a/a.go#"}}},
			uses:   []string{"internal/b/b.go#Read"},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			card := planparser.Card{Number: 1, Slug: "a", TargetGroups: tt.groups, Uses: tt.uses}
			got := planparser.CardTargetDirs(plan, card)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CardTargetDirs() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestRefFile(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	tests := []struct {
		name     string
		ref      string
		want     string
		wantFile bool
	}{
		{name: "go path is itself", ref: "internal/a/a.go", want: "internal/a/a.go", wantFile: true},
		{name: "file-unit glyph maps to its file", ref: "internal/a/a.go#Run", want: "internal/a/a.go", wantFile: true},
		{name: "package-unit glyph has no file", ref: "internal/a#Run"},
		{name: "handle maps through its glyph", ref: "plan:internal/a/a.go#Run", want: "internal/a/a.go", wantFile: true},
		{name: "directory path has no file", ref: "internal/a"},
		{name: "empty ref has no file", ref: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := planparser.RefFile(plan, tt.ref)
			if got != tt.want || ok != tt.wantFile {
				t.Errorf("RefFile(%q) = %q, %v; want %q, %v", tt.ref, got, ok, tt.want, tt.wantFile)
			}
		})
	}
}
