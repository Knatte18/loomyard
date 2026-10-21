// targets_test.go drives the pure core of DeriveTargets over a hand-built package graph and changed-path list.

package impactset

import (
	"reflect"
	"testing"
)

func TestTargetsFor(t *testing.T) {
	t.Parallel()

	guards := map[string][]string{"internal/d": {"TestGuardD", "TestGuardD2"}, "internal/a": {"TestGuardA"}}
	root := func(packages ...string) []ModuleTargets {
		return []ModuleTargets{{Dir: ".", Packages: packages}}
	}

	tests := []struct {
		name    string
		changed []string
		guards  map[string][]string
		want    Targets
	}{
		{
			name:    "one changed package yields it and its reverse dependencies, tmux and llm test imports included",
			changed: []string{"internal/a/a.go"},
			guards:  guards,
			want: Targets{
				Modules: root("./cmd/lyx", "./internal/a", "./internal/b", "./internal/c", "./internal/e", "./internal/tmuxonly"),
				Guards:  map[string][]string{"internal/d": {"TestGuardD", "TestGuardD2"}},
			},
		},
		{
			name:    "a change to a dependency of cmd/lyx pulls in every package that builds the binary",
			changed: []string{"internal/b/b.go"},
			want:    Targets{Modules: root("./cmd/lyx", "./internal/b", "./internal/c", "./internal/e"), Guards: map[string][]string{}},
		},
		{
			name:    "a change outside cmd/lyx's dependencies leaves the binary-building packages out and keeps a test-less package",
			changed: []string{"internal/d/d.go"},
			want:    Targets{Modules: root("./internal/d"), Guards: map[string][]string{}},
		},
		{
			name:    "a rename between package directories puts both in the set",
			changed: []string{"internal/c/x.go", "internal/d/x.go"},
			want:    Targets{Modules: root("./internal/c", "./internal/d"), Guards: map[string][]string{}},
		},
		{
			name:    "a Markdown-only diff outside any package yields no module and every guard",
			changed: []string{"docs/overview.md"},
			guards:  guards,
			want: Targets{Guards: map[string][]string{
				"internal/a": {"TestGuardA"},
				"internal/d": {"TestGuardD", "TestGuardD2"},
			}},
		},
		{
			name:    "a guard name held by two outside packages is listed under each, once",
			changed: []string{"docs/overview.md"},
			guards:  map[string][]string{"internal/a": {"TestGuardX"}, "internal/d": {"TestGuardX"}},
			want:    Targets{Guards: map[string][]string{"internal/a": {"TestGuardX"}, "internal/d": {"TestGuardX"}}},
		},
		{
			name:    "a non-Go file maps to its nearest enclosing package",
			changed: []string{"internal/d/testdata/x.yaml"},
			want:    Targets{Modules: root("./internal/d"), Guards: map[string][]string{}},
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

			if got := targetsFor(testGraph(), tt.changed, tt.guards); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("targetsFor() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
