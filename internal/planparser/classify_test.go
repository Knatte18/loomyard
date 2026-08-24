// classify_test.go table-tests classifyRef's three-rule shape classification directly, pinning
// both the documented cases and the documented shedrecipe.lookup misclassification-as-path.

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
			name: "bare symbol with no dot",
			raw:  "Lookup",
			want: refKindSymbol,
		},
		{
			name: "bare symbol-shaped filename with no extension",
			raw:  "Makefile",
			want: refKindSymbol,
		},
		{
			name: "documented misclassification: lowercase final segment reads as a path",
			raw:  "shedrecipe.lookup",
			want: refKindPath,
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
