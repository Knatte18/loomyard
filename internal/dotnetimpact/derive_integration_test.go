//go:build tier1

// derive_integration_test.go drives DeriveDiff over real git repositories holding the shared C# tree.

package dotnetimpact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

func TestDeriveDiff(t *testing.T) {
	t.Parallel()
	const unknownBase = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name string
		// edit changes the repository after its base commit.
		edit func(t *testing.T, worktree string)
		// base overrides the base commit; "" takes the repository's first commit, and noBase takes none.
		base string
		want Scope
	}{
		{
			name: "ChangedProductionType_SelectsTestClass",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "cs/Lib/Circle.cs", "namespace Lib; public class Circle { int Radius; }")
			},
			want: circleScope(),
		},
		{
			name: "RenamedType_SeedsFromBaseVersion",
			edit: func(t *testing.T, worktree string) {
				gitkit.Git(t, worktree, "mv", "cs/Lib/Circle.cs", "cs/Lib/Round.cs")
				writeFile(t, worktree, "cs/Lib/Round.cs", "namespace Lib; public class Round { }")
			},
			want: circleScope(),
		},
		{
			name: "DeletedFile_SeedsFromBaseVersion",
			edit: func(t *testing.T, worktree string) {
				if err := os.Remove(filepath.Join(worktree, "cs/Lib/Circle.cs")); err != nil {
					t.Fatalf("Remove failed: %v", err)
				}
			},
			want: circleScope(),
		},
		{
			name: "UntrackedCsFile_CountsAsChanged",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "cs/Lib.Tests/ExtraTests.cs", "[TestFixture] class ExtraTests { [Test] void M() { } }")
			},
			want: Scope{Projects: []Project{{Path: "Lib.Tests/Lib.Tests.csproj", Terms: []string{"FullyQualifiedName~ExtraTests"}}}},
		},
		{
			name: "InertOnlyDiff_EmptyScope",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "docs/readme.md", "changed")
			},
			want: Scope{},
		},
		{
			name: "EmptyDiff_EmptyScope",
			edit: func(*testing.T, string) {},
			want: Scope{},
		},
		{
			name: "EmptyBase_Whole",
			edit: func(*testing.T, string) {},
			base: noBase,
			want: Scope{Whole: true, Fallback: "no base commit to diff from"},
		},
		{
			name: "NonAncestorBase_Whole",
			edit: func(*testing.T, string) {},
			base: unknownBase,
			want: Scope{Whole: true, Fallback: "base " + unknownBase + " is not an ancestor of HEAD"},
		},
		{
			name: "CsprojChange_Whole",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "cs/Lib/Lib.csproj", "<Project Sdk=\"x\" />")
			},
			want: Scope{Whole: true, Fallback: "cs/Lib/Lib.csproj is neither inert nor a .cs file"},
		},
		{
			name: "CsFileOutsideDir_Whole",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "Outside.cs", "class Outside { }")
			},
			want: Scope{Whole: true, Fallback: "Outside.cs lies outside cs"},
		},
		{
			name: "TypelessCsFile_Whole",
			edit: func(t *testing.T, worktree string) {
				writeFile(t, worktree, "cs/GlobalUsings.cs", "global using System;")
			},
			want: Scope{Whole: true, Fallback: "cs/GlobalUsings.cs declares no type"},
		},
		{
			name: "RenameOfNonCsFileToCs_Whole",
			edit: func(t *testing.T, worktree string) {
				gitkit.Git(t, worktree, "mv", "notes.txt", "cs/Lib/Notes.cs")
			},
			want: Scope{Whole: true, Fallback: "notes.txt is neither inert nor a .cs file"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := plankit.Repo(t, csTree())
			gitkit.Git(t, worktree, "init", "-q")
			gitkit.Git(t, worktree, "add", "-A")
			gitkit.Git(t, worktree, "commit", "-q", "-m", "base")
			base := gitkit.Git(t, worktree, "rev-parse", "HEAD")
			tt.edit(t, worktree)
			switch tt.base {
			case "":
			case noBase:
				base = ""
			default:
				base = tt.base
			}

			got, err := DeriveDiff(context.Background(), worktree, treeDir, base, isMarkdown)
			if err != nil {
				t.Fatalf("DeriveDiff() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("DeriveDiff() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// noBase marks a row that diffs from the empty base.
const noBase = "<none>"

func writeFile(t *testing.T, worktree, rel, content string) {
	t.Helper()
	plankit.WriteTree(t, worktree, map[string]string{rel: content})
}
