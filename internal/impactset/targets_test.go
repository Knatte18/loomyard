//go:build tier0

// targets_test.go drives the pure core of DeriveTargets over a hand-built package graph and changed-path list.

package impactset

import (
	"reflect"
	"testing"
)

func TestTargetsFor(t *testing.T) {
	t.Parallel()

	guardsD := []GuardTest{{Name: "TestGuardD", Tier: 0}, {Name: "TestGuardD2", Tier: 1}}
	guards := map[string][]GuardTest{"internal/d": guardsD, "internal/a": {{Name: "TestGuardA", Tier: 1}}}
	root := func(packages ...string) []ModuleTargets {
		return []ModuleTargets{{Dir: ".", Packages: packages}}
	}

	binaryTests := []string{"internal/e"}
	graphWithoutLyxbin := moduleGraph{byImportPath: map[string]pkg{}, byDir: map[string]pkg{}}
	for importPath, p := range testGraph().byImportPath {
		if p.Dir != lyxbinDir {
			graphWithoutLyxbin.byImportPath[importPath] = p
			graphWithoutLyxbin.byDir[p.Dir] = p
		}
	}

	tests := []struct {
		name    string
		graph   *moduleGraph
		changed []string
		guards  map[string][]GuardTest
		want    Targets
	}{
		{
			name:    "one changed package yields it and its reverse dependencies, tmux and llm test imports included",
			changed: []string{"internal/a/a.go"},
			guards:  guards,
			want: Targets{
				Modules:     root("./cmd/lyx", "./internal/a", "./internal/b", "./internal/c", "./internal/tmuxonly"),
				Guards:      map[string][]GuardTest{"internal/d": guardsD},
				BinaryTests: binaryTests,
			},
		},
		{
			name:    "a change to a dependency of cmd/lyx leaves out a package that reaches it only through the binary",
			changed: []string{"internal/b/b.go"},
			want:    Targets{Modules: root("./cmd/lyx", "./internal/b", "./internal/c"), Guards: map[string][]GuardTest{}, BinaryTests: binaryTests},
		},
		{
			name:    "a change to the cmd/lyx main package pulls in every package that builds the binary",
			changed: []string{"cmd/lyx/main.go"},
			want:    Targets{Modules: root("./cmd/lyx", "./internal/e"), Guards: map[string][]GuardTest{}, BinaryTests: binaryTests},
		},
		{
			name:    "a change to the cmd/lyx main package over a module without the binary builder adds nothing",
			graph:   &graphWithoutLyxbin,
			changed: []string{"cmd/lyx/main.go"},
			want:    Targets{Modules: root("./cmd/lyx"), Guards: map[string][]GuardTest{}},
		},
		{
			name:    "a change outside cmd/lyx's dependencies leaves the binary-building packages out and keeps a test-less package",
			changed: []string{"internal/d/d.go"},
			want:    Targets{Modules: root("./internal/d"), Guards: map[string][]GuardTest{}, BinaryTests: binaryTests},
		},
		{
			name:    "a rename between package directories puts both in the set",
			changed: []string{"internal/c/x.go", "internal/d/x.go"},
			want:    Targets{Modules: root("./internal/c", "./internal/d"), Guards: map[string][]GuardTest{}, BinaryTests: binaryTests},
		},
		{
			name:    "a Markdown-only diff outside any package yields no module and every guard",
			changed: []string{"docs/overview.md"},
			guards:  guards,
			want: Targets{
				Guards: map[string][]GuardTest{
					"internal/a": {{Name: "TestGuardA", Tier: 1}},
					"internal/d": guardsD,
				},
				BinaryTests: binaryTests,
			},
		},
		{
			name:    "a guard name held by two outside packages is listed under each, once",
			changed: []string{"docs/overview.md"},
			guards:  map[string][]GuardTest{"internal/a": {{Name: "TestGuardX", Tier: 0}}, "internal/d": {{Name: "TestGuardX", Tier: 1}}},
			want:    Targets{Guards: map[string][]GuardTest{"internal/a": {{Name: "TestGuardX", Tier: 0}}, "internal/d": {{Name: "TestGuardX", Tier: 1}}}, BinaryTests: binaryTests},
		},
		{
			name:    "a non-Go file maps to its nearest enclosing package",
			changed: []string{"internal/d/testdata/x.yaml"},
			want:    Targets{Modules: root("./internal/d"), Guards: map[string][]GuardTest{}, BinaryTests: binaryTests},
		},
		{
			name:    "a file outside any package that is not Markdown falls back",
			changed: []string{"contracts/recipe.yaml"},
			want:    Targets{Whole: true, Fallback: "contracts/recipe.yaml is neither a Markdown file nor under a Go package directory"},
		},
		{
			name:    "a deleted Go file whose directory is no longer a package falls back",
			changed: []string{"internal/gone/gone.go"},
			want:    Targets{Whole: true, Fallback: "internal/gone/gone.go is neither a Markdown file nor under a Go package directory"},
		},
		{
			name:    "a go.mod change falls back",
			changed: []string{"internal/a/a.go", "go.mod"},
			want:    Targets{Whole: true, Fallback: "go.mod changed"},
		},
		{
			name:    "a go.sum change falls back",
			changed: []string{"go.sum"},
			want:    Targets{Whole: true, Fallback: "go.sum changed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			graph := testGraph()
			if tt.graph != nil {
				graph = *tt.graph
			}
			if got := targetsFor(graph, tt.changed, tt.guards); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("targetsFor() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
