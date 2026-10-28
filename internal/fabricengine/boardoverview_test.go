//go:build tier0

// boardoverview_test.go is table-driven, pure-logic (Tier 0) coverage for isBoardOverview's file-name rule.

package fabricengine

import "testing"

func TestIsBoardOverview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"Readme", "README.md", true},
		{"ProjectFile", "project-x.md", true},
		{"ProjectFileWithDashedSlug", "project-a-b.md", true},
		{"EmptySlug", "project-.md", false},
		{"SlugWithSlash", "project-a/b.md", false},
		{"ProjectFileInSubdirectory", "dir/project-x.md", false},
		{"NotMarkdown", "project-x.json", false},
		{"EntryStore", "board.json", false},
		{"DesignFile", "design-x.md", false},
		{"NestedReadme", "dir/README.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBoardOverview(tt.path); got != tt.want {
				t.Errorf("isBoardOverview(%q) = %v; want %v", tt.path, got, tt.want)
			}
		})
	}
}
