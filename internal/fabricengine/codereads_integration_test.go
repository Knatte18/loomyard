// codereads_integration_test.go covers the code reads at a commit over a wired hub's code checkout.
// They are Fabric.CodeDirAt, Fabric.CodeFileAt and Fabric.ExportCodeTreeAt.
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

	t.Run("ExportCodeTreeAtWritesTheCommitsFiles", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "base")
		if err := f.ExportCodeTreeAt(head, dest); err != nil {
			t.Fatalf("ExportCodeTreeAt() error = %v", err)
		}
		tests := []struct {
			rel       string
			wantFound bool
			want      string
		}{
			{"pkg/a.go", true, "package pkg\n"},
			{"pkg/sub/c.go", true, "package sub\n"},
			{"pkg/new.go", false, ""},
		}
		for _, tt := range tests {
			data, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(tt.rel)))
			found := err == nil
			if found != tt.wantFound || string(data) != tt.want {
				t.Errorf("exported %s = (%q, found %v); want (%q, found %v)", tt.rel, data, found, tt.want, tt.wantFound)
			}
		}
		if err := f.ExportCodeTreeAt("not-a-sha", t.TempDir()); err == nil {
			t.Error("ExportCodeTreeAt(not-a-sha) error = nil; want the invalid name refused")
		}
	})
}
