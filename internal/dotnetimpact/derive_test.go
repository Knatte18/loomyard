//go:build tier0

// derive_test.go covers DeriveCards over fixture trees on disk.

package dotnetimpact

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

func TestDeriveCards(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		extra map[string]string
		// lock names a file, relative to the worktree, that the row makes unreadable.
		lock  string
		seeds []planparser.CSharpSeed
		// want is the expected Scope; "{worktree}" in its Fallback stands for the worktree path.
		want Scope
	}{
		{
			name:  "MemberGlyphSeed_SelectsOwnersTestClass",
			seeds: []planparser.CSharpSeed{{Ref: "Circle.Area", Types: []string{"Circle", "Area"}}},
			want:  circleScope(),
		},
		{
			name:  "FileSeed_SelectsTestClass",
			seeds: []planparser.CSharpSeed{{Ref: "cs/Lib/Circle.cs", File: "cs/Lib/Circle.cs"}},
			want:  circleScope(),
		},
		{
			name: "InertSeedDroppedBesideRealOne",
			seeds: []planparser.CSharpSeed{
				{Ref: "docs/readme.md", File: "docs/readme.md"},
				{Ref: "cs/Lib/Circle.cs", File: "cs/Lib/Circle.cs"},
			},
			want: circleScope(),
		},
		{
			name:  "AllInertSeeds_EmptyScope",
			seeds: []planparser.CSharpSeed{{Ref: "docs/readme.md", File: "docs/readme.md"}},
			want:  Scope{},
		},
		{
			name: "BinAndObjAreNotScanned",
			extra: map[string]string{
				"cs/Lib.Tests/obj/Gen.cs":       "[TestFixture] class GenTests { [Test] void M() { new Circle(); } }",
				"cs/Lib.Tests/bin/Other.csproj": "<Project />",
			},
			seeds: []planparser.CSharpSeed{{Ref: "Circle", Types: []string{"Circle"}}},
			want:  circleScope(),
		},
		{
			name:  "NonInertJsonFile_Whole",
			seeds: []planparser.CSharpSeed{{Ref: "cs/appsettings.json", File: "cs/appsettings.json"}},
			want:  Scope{Whole: true, Fallback: "cs/appsettings.json is neither inert nor a .cs file"},
		},
		{
			name:  "DeletedCsFile_Whole",
			seeds: []planparser.CSharpSeed{{Ref: "cs/Lib/Gone.cs", File: "cs/Lib/Gone.cs"}},
			want:  Scope{Whole: true, Fallback: "cs/Lib/Gone.cs is absent from the working tree and names no type"},
		},
		{
			name:  "CsFileOutsideDir_Whole",
			seeds: []planparser.CSharpSeed{{Ref: "Outside.cs", File: "Outside.cs"}},
			want:  Scope{Whole: true, Fallback: "Outside.cs lies outside cs"},
		},
		{
			name:  "EmptySeed_Whole",
			seeds: []planparser.CSharpSeed{{Ref: "cs/Lib"}},
			want:  Scope{Whole: true, Fallback: "cs/Lib names neither a file nor a type"},
		},
		{
			name:  "UnreadableCsFile_Whole",
			extra: map[string]string{"cs/Lib/Locked.cs": "class Locked { }"},
			lock:  "cs/Lib/Locked.cs",
			seeds: []planparser.CSharpSeed{{Ref: "cs/Lib/Circle.cs", File: "cs/Lib/Circle.cs"}},
			want:  Scope{Whole: true, Fallback: "scan failed: open {worktree}/cs/Lib/Locked.cs: permission denied"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.lock != "" && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("a mode-0 file stays readable on Windows and to root")
			}
			files := csTree()
			for rel, content := range tt.extra {
				files[rel] = content
			}
			worktree := plankit.Repo(t, files)
			if tt.lock != "" {
				if err := os.Chmod(filepath.Join(worktree, filepath.FromSlash(tt.lock)), 0); err != nil {
					t.Fatalf("Chmod(%s) failed: %v", tt.lock, err)
				}
			}
			tt.want.Fallback = strings.ReplaceAll(tt.want.Fallback, "{worktree}", filepath.ToSlash(worktree))

			got, err := DeriveCards(context.Background(), worktree, treeDir, tt.seeds, isMarkdown)
			if err != nil {
				t.Fatalf("DeriveCards() error = %v; want nil", err)
			}
			got.Fallback = filepath.ToSlash(got.Fallback)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("DeriveCards() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
