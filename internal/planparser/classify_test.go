// classify_test.go table-tests the shape classification of classifyRef directly.
// It pins the documented cases and the known misclassification of shedrecipe.lookup as a path.

package planparser

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestClassifyRef pins classifyRef and the exported IsHandleRef over every ref shape.
//
//testtiming:keep pins classifyRef's five-rule shape table and IsHandleRef's agreement with it, which its covering tests reach only after canonicalization
func TestClassifyRef(t *testing.T) {
	tiergate.Require(t, 0)
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
			raw:  "//cmd/lym/main.go",
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
			if gotHandle := IsHandleRef(tt.raw); gotHandle != (tt.want == refKindHandle) {
				t.Errorf("IsHandleRef(%q) = %v; want %v", tt.raw, gotHandle, tt.want == refKindHandle)
			}
		})
	}
}
