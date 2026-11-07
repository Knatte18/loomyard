package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSubstitute covers which occurrences of loom the substitution tables rename and which they leave.
func TestSubstitute(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"IdentifierAfterPackage", "shedrun.RecipeLoom", "shedrun.RecipeTapestry"},
		{"IdentifierCamelStart", "NameLoomPreflight", "NameTapestryPreflight"},
		{"IdentifierCamelEnd", "loomPreflightProducer", "tapestryPreflightProducer"},
		{"RowName", "running · Loom-Preflight", "running · Tapestry-Preflight"},
		{"FileName", "loom-recipe.yaml and loompreflight_test.go", "tapestry-recipe.yaml and tapestrypreflight_test.go"},
		{"Phrase", "a loom run's status", "a tapestry run's status"},
		{"LongestPhraseWins", "both loom runs", "both tapestry runs"},
		{"CapitalisedPhrase", "Loom and darn alike", "Tapestry and darn alike"},
		{"VerbKeepsItsName", "`lym loom run <run-id>`", "`lym loom run <run-id>`"},
		{"GeneralLoomStays", "loom.yaml, internal/loomcli, lym loom status and the LOOM program", "loom.yaml, internal/loomcli, lym loom status and the LOOM program"},
		{"PackageNameStays", "loomrecipe.New", "loomrecipe.New"},
		{"NoTokenInsideAWord", "heirloom run", "heirloom run"},
		{"PhraseFollowedByAWordStays", "loom running", "loom running"},
		{"RecipeValueInASeed", `{"recipe":"loom","driver":"go"}`, `{"recipe":"tapestry","driver":"go"}`},
		{"RecipeValueInAFlag", "lym shed seed self --recipe loom --driver go", "lym shed seed self --recipe tapestry --driver go"},
		{"RecipeValueInAnArgument", `"seed", "self", "--recipe", "loom", "--driver"`, `"seed", "self", "--recipe", "tapestry", "--driver"`},
		{"RecipeValueInATableCell", "| `base` | loom |  |", "| `base` | tapestry |  |"},
		{"ValueKeyCarriesAnIdentifier", `{"loom", RecipeLoom, false}`, `{"tapestry", RecipeTapestry, false}`},
		{"ConfigModuleStays", `LoadConfig(dirs, "loom") and {"loom": {"code_max_bounces": 1}}`, `LoadConfig(dirs, "loom") and {"loom": {"code_max_bounces": 1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := substitute(tt.in, substitutionKeys("")); got != tt.want {
				t.Errorf("substitute(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}

	t.Run("PathRuleAppliesInItsFileAloneAndOnce", func(t *testing.T) {
		t.Parallel()
		list := "\t\t\t\"LoomPreflight\",\n\t\t\t\"MultiLLM\",\n\t\t\t\"Stub\",\n\t\t\t\"WorktreeCreate\",\n"
		sorted := "\t\t\t\"MultiLLM\",\n\t\t\t\"Stub\",\n\t\t\t\"TapestryPreflight\",\n\t\t\t\"WorktreeCreate\",\n"
		keys := substitutionKeys("internal/shedrecipe/registry_test.go")
		if got := substitute(list, keys); got != sorted {
			t.Errorf("substitute(registry list) = %q; want %q", got, sorted)
		}
		if got := substitute(sorted, keys); got != sorted {
			t.Errorf("substitute(sorted list) = %q; want it unchanged", got)
		}
		if got, want := substitute(list, substitutionKeys("other.go")), "\t\t\t\"TapestryPreflight\",\n\t\t\t\"MultiLLM\",\n\t\t\t\"Stub\",\n\t\t\t\"WorktreeCreate\",\n"; got != want {
			t.Errorf("substitute(list elsewhere) = %q; want %q", got, want)
		}
	})
}

// TestRenameTree runs the moves and substitutions over a small tree:
// a listed file moves, a file outside the excluded paths is rewritten, and a Go file is formatted after its rename.
// An excluded one is left, and a second run changes nothing.
func TestRenameTree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	root := t.TempDir()
	write := func(rel, text string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	write("contracts/recipes/loom-recipe.yaml", "# the loom recipe\n")
	write("internal/x/x.go", "package x\n\nconst (\n\tRecipeLoom = 1\n\tOther      = 2\n)\n")
	write("docs/research/report.md", "a loom run\n")

	moved, err := applyMoves(root, moves, false)
	if err != nil {
		t.Fatalf("applyMoves() error = %v", err)
	}
	if want := []move{moves[0]}; !slices.Equal(moved, want) {
		t.Errorf("applyMoves() = %v; want %v", moved, want)
	}
	changed, err := substituteTree(root, false)
	if err != nil {
		t.Fatalf("substituteTree() error = %v", err)
	}
	if want := []string{"contracts/recipes/tapestry-recipe.yaml", "internal/x/x.go"}; !slices.Equal(changed, want) {
		t.Errorf("substituteTree() = %v; want %v", changed, want)
	}
	if got, want := read("contracts/recipes/tapestry-recipe.yaml"), "# the tapestry recipe\n"; got != want {
		t.Errorf("moved recipe = %q; want %q", got, want)
	}
	if got, want := read("internal/x/x.go"), "package x\n\nconst (\n\tRecipeTapestry = 1\n\tOther          = 2\n)\n"; got != want {
		t.Errorf("x.go = %q; want %q", got, want)
	}
	if got, want := read("docs/research/report.md"), "a loom run\n"; got != want {
		t.Errorf("excluded report = %q; want %q", got, want)
	}

	moved, err = applyMoves(root, moves, false)
	if err != nil || len(moved) != 0 {
		t.Errorf("second applyMoves() = %v, %v; want nothing moved", moved, err)
	}
	changed, err = substituteTree(root, false)
	if err != nil || len(changed) != 0 {
		t.Errorf("second substituteTree() = %v, %v; want nothing changed", changed, err)
	}
}
