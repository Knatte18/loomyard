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
			name:   "external test package glyph maps to its package directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a_test#TestRun"}}},
			want:   []planparser.TargetDir{{Dir: "internal/a", NamesGo: true}},
		},
		{
			name:   "package and its external test package merge into one directory",
			groups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/a#Run", "internal/a_test#TestRun"}}},
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

func TestRefCovers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		cover    string
		target   string
		want     bool
	}{
		{name: "package self glyph covers its external test package under go", language: "go", cover: "x/pkg#", target: "x/pkg_test#", want: true},
		{name: "package self glyph does not cover a _test directory under csharp", language: "csharp", cover: "x/pkg#", target: "x/pkg_test#"},
		{name: "file cover does not cover a C# member, which maps to no scope", language: "csharp", cover: "src/Foo.cs", target: "Glyphchain.Sharp#Renderer.Title"},
		{name: "member covers the same member through a handle", cover: "internal/a#Foo", target: "plan:internal/a#Foo", want: true},
		{name: "member against another member of the package", cover: "internal/a#Foo", target: "internal/a#Bar"},
		{name: "member against the file self glyph of its file", cover: "internal/a#Foo", target: "internal/a/a.go#"},
		{name: "file self glyph covers the plain path of the file", cover: "internal/a/a.go#", target: "internal/a/a.go", want: true},
		{name: "file self glyph covers a member glyph in that file unit", cover: "internal/a/a.go#", target: "internal/a/a.go#Run", want: true},
		{name: "file self glyph covers a package-unit member glyph", cover: "internal/a/a.go#", target: "internal/a#Foo", want: true},
		{name: "file self glyph covers a package-unit handle", cover: "internal/a/a.go#", target: "plan:internal/a#Foo", want: true},
		{name: "file self glyph covers the package self glyph of its directory", cover: "internal/a/a.go#", target: "internal/a#", want: true},
		{name: "file against another file of the same directory", cover: "internal/a/a.go#", target: "internal/a/b.go#"},
		{name: "file against the plain directory path of its directory", cover: "internal/a/a.go#", target: "internal/a"},
		{name: "test file covers the external test package member", cover: "internal/a/a_test.go#", target: "internal/a_test#TestRun", want: true},
		{name: "directory path covers a file glyph under a subdirectory", cover: "internal/a", target: "internal/a/sub/s.go#", want: true},
		{name: "directory path covers a package-unit member glyph of that directory", cover: "internal/a", target: "internal/a#Foo", want: true},
		{name: "directory path against a file under a sibling sharing the prefix", cover: "internal/a", target: "internal/ab/x.go#"},
		{name: "package self glyph covers a member glyph in its file", cover: "internal/a#", target: "internal/a/a.go#Run", want: true},
		{name: "root directory covers every mapped target", cover: ".", target: "internal/a/a.go#Run", want: true},
		{name: "unmappable handle as cover", cover: "plan:nohash", target: "internal/a#Foo"},
		{name: "unmappable handle as target", cover: "internal/a", target: "plan:nohash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := &planparser.Plan{Language: "go"}
			if tt.language != "" {
				plan.Language = tt.language
			}
			if got := planparser.RefCovers(plan, tt.cover, tt.target); got != tt.want {
				t.Errorf("RefCovers(%q, %q) = %v; want %v", tt.cover, tt.target, got, tt.want)
			}
		})
	}
}

func TestRefMember(t *testing.T) {
	t.Parallel()

	goPlan := &planparser.Plan{Language: "go"}
	tests := []struct {
		name   string
		plan   *planparser.Plan
		ref    string
		want   string
		wantOK bool
	}{
		{name: "package-unit member glyph", plan: goPlan, ref: "internal/a#Foo", want: "internal/a#Foo", wantOK: true},
		{name: "file-unit member glyph", plan: goPlan, ref: "internal/a/a.go#Run", want: "internal/a/a.go#Run", wantOK: true},
		{name: "member handle", plan: goPlan, ref: "plan:internal/a#Foo", want: "internal/a#Foo", wantOK: true},
		{name: "package self glyph", plan: goPlan, ref: "internal/a#"},
		{name: "file self glyph", plan: goPlan, ref: "internal/a/a.go#"},
		{name: "directory path", plan: goPlan, ref: "internal/a"},
		{name: "unmappable handle", plan: goPlan, ref: "plan:nohash"},
		{name: "non-Go plan", plan: &planparser.Plan{Language: "none"}, ref: "internal/a#Foo"},
		{name: "nil plan reads as the default language", plan: nil, ref: "internal/a#Foo", want: "internal/a#Foo", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := planparser.RefMember(tt.plan, tt.ref)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("RefMember(%q) = %q, %v; want %q, %v", tt.ref, got, ok, tt.want, tt.wantOK)
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

func TestTargetHolds(t *testing.T) {
	t.Parallel()

	goPlan := &planparser.Plan{Language: "go"}
	nonePlan := &planparser.Plan{Language: "none"}
	tests := []struct {
		name   string
		plan   *planparser.Plan
		target string
		member string
		file   string
		want   bool
	}{
		{name: "member glyph itself", plan: goPlan, target: "internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "handle whose body is the member", plan: goPlan, target: "plan:internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "containing type", plan: goPlan, target: "internal/a#T", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "member target against its owning type", plan: goPlan, target: "internal/a#T.M", member: "internal/a#T", file: "internal/a/a.go"},
		{name: "method sibling", plan: goPlan, target: "internal/a#T.N", member: "internal/a#T.M", file: "internal/a/a.go"},
		{name: "unit self glyph", plan: goPlan, target: "internal/a#", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "external test member against its directory's unit self glyph", plan: goPlan, target: "internal/a#", member: "internal/a_test#TestX", file: "internal/a/a_test.go", want: true},
		{name: "file as a path", plan: goPlan, target: "internal/a/a.go", file: "internal/a/a.go", want: true},
		{name: "file as a self glyph", plan: goPlan, target: "internal/a/a.go#", file: "internal/a/a.go", want: true},
		{name: "file target against a found member in that file", plan: goPlan, target: "internal/a/a.go", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "handle whose body is the file's self glyph", plan: goPlan, target: "plan:internal/a/a.go#", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "another file", plan: goPlan, target: "internal/a/b.go#", file: "internal/a/a.go"},
		{name: "nil plan reads as the default language", target: "internal/a#T", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "language none holds a file path", plan: nonePlan, target: "internal/a/a.go", member: "internal/a#T.M", file: "internal/a/a.go", want: true},
		{name: "language none holds no member", plan: nonePlan, target: "internal/a#T.M", member: "internal/a#T.M", file: "internal/a/a.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := planparser.TargetHolds(tt.plan, tt.target, tt.member, tt.file); got != tt.want {
				t.Errorf("TargetHolds(%q, %q, %q) = %v; want %v", tt.target, tt.member, tt.file, got, tt.want)
			}
		})
	}
}
