//go:build tier0

// treesitterlinkfindings_test.go pins the pure half of the tree-sitter link guard, which treesitterlink_test.go holds, over an in-memory import graph.

package main

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestTreeSitterLinkFindings(t *testing.T) {
	t.Parallel()

	deps := map[string][]string{
		"github.com/tree-sitter/go-tree-sitter":                                                          {"fmt"},
		"github.com/Knatte18/loomyard/internal/index":                                                    {"github.com/tree-sitter/go-tree-sitter"},
		"github.com/Knatte18/loomyard/internal/seam":                                                     {"fmt"},
		"github.com/Knatte18/loomyard/internal/user":                                                     {"github.com/Knatte18/loomyard/internal/index"},
		"github.com/Knatte18/loomyard/internal/clean":                                                    {"github.com/Knatte18/loomyard/internal/seam"},
		"github.com/Knatte18/loomyard/internal/index.test":                                               {"github.com/Knatte18/loomyard/internal/index [github.com/Knatte18/loomyard/internal/index.test]"},
		"github.com/Knatte18/loomyard/internal/index [github.com/Knatte18/loomyard/internal/index.test]": {"github.com/tree-sitter/go-tree-sitter"},
		"github.com/Knatte18/loomyard/internal/user.test":                                                {"github.com/Knatte18/loomyard/internal/user"},
		"github.com/Knatte18/loomyard/internal/clean.test":                                               {"github.com/Knatte18/loomyard/internal/clean", "github.com/Knatte18/loomyard/internal/seam"},
		"github.com/Knatte18/loomyard/internal/seam.test":                                                {"github.com/Knatte18/loomyard/internal/seam"},
	}
	indexEntry := scankit.Entry{Key: "internal/index", Why: "calls the index"}
	seamEntry := scankit.Entry{Key: "internal/seam", Why: "was cut from the index"}

	tests := []struct {
		name    string
		allowed []scankit.Entry
		want    []string
	}{
		{
			name:    "a package outside the list fails with its chain",
			allowed: []scankit.Entry{indexEntry},
			want: []string{
				"internal/user: its test binary links tree-sitter through github.com/Knatte18/loomyard/internal/user.test -> github.com/Knatte18/loomyard/internal/user -> github.com/Knatte18/loomyard/internal/index -> github.com/tree-sitter/go-tree-sitter; cut the import at its seam, or allow the package in treeSitterAllowedLinks with the remaining link stated",
			},
		},
		{
			name:    "an allowed package that no longer links fails as stale",
			allowed: []scankit.Entry{indexEntry, {Key: "internal/user", Why: "reaches the index"}, seamEntry},
			want:    []string{"internal/seam: allowed in treeSitterAllowedLinks, but its test binary no longer links tree-sitter; remove the entry"},
		},
		{
			name:    "every linked package allowed and no stale entry passes",
			allowed: []scankit.Entry{indexEntry, {Key: "internal/user", Why: "reaches the index"}},
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := treeSitterLinkFindings(treeSitterLinkedTestPackages(deps), tt.allowed)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("treeSitterLinkFindings() = %q; want %q", got, tt.want)
			}
		})
	}
}
