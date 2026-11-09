// codereads_integration_test.go covers Fabric.CodeDirAt and Fabric.CodeFileAt over a wired hub's code checkout.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCodeReadsAtCommit reads a commit's directories and files.
// The working tree holds an edit and an untracked file meanwhile.
// Its subtests share the hub's one commit and run in sequence, so none is parallel.
func TestCodeReadsAtCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	root := h.PrimeWorktree()
	gitkit.CommitFile(t, root, "pkg/sub/c.go", "package sub\n", "add c")
	gitkit.CommitFile(t, root, "pkg/b.txt", "b\n", "add b")
	head := gitkit.CommitFile(t, root, "pkg/a.go", "package pkg\n", "add a")
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "new.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("CodeDirAtListsTheCommitsFiles", func(t *testing.T) {
		tests := []struct {
			dir  string
			want []string
		}{
			{"pkg", []string{"a.go", "b.txt"}},
			{"pkg/sub", []string{"c.go"}},
			{"absent", nil},
		}
		for _, tt := range tests {
			got, err := f.CodeDirAt(head, tt.dir)
			if err != nil {
				t.Fatalf("CodeDirAt(%q) error = %v", tt.dir, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("CodeDirAt(%q) = %q; want %q", tt.dir, got, tt.want)
			}
		}
	})

	t.Run("CodeFileAtReadsTheCommit", func(t *testing.T) {
		tests := []struct {
			rel       string
			wantFound bool
			want      string
		}{
			{"pkg/a.go", true, "package pkg\n"},
			{"pkg/new.go", false, ""},
			{"pkg/absent.go", false, ""},
		}
		for _, tt := range tests {
			data, found, err := f.CodeFileAt(head, tt.rel)
			if err != nil {
				t.Fatalf("CodeFileAt(%q) error = %v", tt.rel, err)
			}
			if found != tt.wantFound || string(data) != tt.want {
				t.Errorf("CodeFileAt(%q) = (%q, %v); want (%q, %v)", tt.rel, data, found, tt.want, tt.wantFound)
			}
		}
	})
}
