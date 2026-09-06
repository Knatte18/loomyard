// classify_test.go table-tests classifyRef's five-rule shape classification directly, pinning both
// the documented cases and the documented shedrecipe.lookup misclassification-as-path.

package planparser

import "testing"

func TestClassifyRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want refKind
	}{
		{
			name: "nested path with slash",
			raw:  "internal/boardcli/list.go",
			want: refKindPath,
		},
		{
			name: "bare filename with lowercase extension",
			raw:  "list.go",
			want: refKindPath,
		},
		{
			name: "worktree-root escape contains a slash",
			raw:  "//cmd/lyx/main.go",
			want: refKindPath,
		},
		{
			name: "package-qualified symbol",
			raw:  "shedrecipe.Lookup",
			want: refKindSymbol,
		},
		{
			name: "bare symbol with no dot classifies as an extensionless bare filename",
			raw:  "Lookup",
			want: refKindPath,
		},
		{
			name: "bare symbol-shaped filename with no extension",
			raw:  "Makefile",
			want: refKindPath,
		},
		{
			name: "documented misclassification: lowercase final segment reads as a path",
			raw:  "shedrecipe.lookup",
			want: refKindPath,
		},
		{
			name: "member glyph",
			raw:  "internal/boardcli#RowJSON",
			want: refKindGlyph,
		},
		{
			name: "unit self glyph",
			raw:  "internal/boardcli#",
			want: refKindGlyph,
		},
		{
			name: "file self glyph",
			raw:  "internal/boardcli/list.go#",
			want: refKindGlyph,
		},
		{
			name: "glyph shape wins over the slash rule despite the unit containing a slash",
			raw:  "internal/boardcli#Owner.Name",
			want: refKindGlyph,
		},
		{
			name: "plan: handle",
			raw:  "plan:approve",
			want: refKindHandle,
		},
		{
			name: "plan: handle prefix wins over every other rule",
			raw:  "plan:internal/foo#Bar",
			want: refKindHandle,
		},
		{
			name: "symbol with a mixed-case final segment",
			raw:  "pkg.Symbol",
			want: refKindSymbol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyRef(tt.raw)
			if got != tt.want {
				t.Errorf("classifyRef(%q) = %v; want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestIsPathRef_IsGlyphRef_IsHandleRef covers the three convenience wrappers directly, one
// representative case each, so a future refactor of classifyRef's return value cannot silently
// break one wrapper while the table above still passes.
func TestIsPathRef_IsGlyphRef_IsHandleRef(t *testing.T) {
	t.Parallel()

	if !isPathRef("list.go") {
		t.Errorf("isPathRef(%q) = false; want true", "list.go")
	}
	if isPathRef("internal/boardcli#RowJSON") {
		t.Errorf("isPathRef(%q) = true; want false", "internal/boardcli#RowJSON")
	}

	if !isGlyphRef("internal/boardcli#RowJSON") {
		t.Errorf("isGlyphRef(%q) = false; want true", "internal/boardcli#RowJSON")
	}
	if isGlyphRef("list.go") {
		t.Errorf("isGlyphRef(%q) = true; want false", "list.go")
	}

	if !isHandleRef("plan:approve") {
		t.Errorf("isHandleRef(%q) = false; want true", "plan:approve")
	}
	if isHandleRef("list.go") {
		t.Errorf("isHandleRef(%q) = true; want false", "list.go")
	}
}
