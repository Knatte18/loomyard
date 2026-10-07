// derive_test.go drives the pure core of Derive over a hand-built package graph and changed-path list.

package impactset

import (
	"fmt"
	"strings"
	"testing"
)

// testGraph is a small module.
// Package b imports a, c imports b, d is unrelated, and tmuxonly reaches a only through a test import.
// The command package cmd/lyx imports b, and e builds the binary in its tests.
func testGraph() moduleGraph {
	const m = "m/"
	return newModuleGraph([]pkg{
		{ImportPath: m + "internal/a", Dir: "internal/a"},
		{ImportPath: m + "internal/b", Dir: "internal/b", Imports: []string{m + "internal/a"}},
		{ImportPath: m + "internal/c", Dir: "internal/c", Imports: []string{m + "internal/b"}},
		{ImportPath: m + "internal/d", Dir: "internal/d"},
		{ImportPath: m + "internal/tmuxonly", Dir: "internal/tmuxonly", TestImports: []string{m + "internal/a"}},
		{ImportPath: m + "cmd/lyx", Dir: "cmd/lyx", Imports: []string{m + "internal/b"}},
		{ImportPath: m + "internal/testkit/lyxbin", Dir: "internal/testkit/lyxbin"},
		{ImportPath: m + "internal/e", Dir: "internal/e", TestImports: []string{m + "internal/testkit/lyxbin"}},
	})
}

// gateCommand renders the expected command: build, vet per tag, untagged, integration, then the trailing steps.
func gateCommand(packages string, trailing ...string) string {
	steps := []string{
		"go build ./...",
		"go vet -tags integration " + packages,
		"go vet -tags tmux " + packages,
		"go vet -tags llm " + packages,
		"go test " + packages,
		"go test -tags integration " + packages,
	}
	return strings.Join(append(steps, trailing...), " && ")
}

func TestCommandFor(t *testing.T) {
	t.Parallel()

	guards := map[string][]string{"internal/d": {"TestGuardD", "TestGuardD2"}, "internal/a": {"TestGuardA"}}

	tests := []struct {
		name         string
		graph        moduleGraph
		changed      []string
		guards       map[string][]string
		wantCommand  string
		wantFallback string
	}{
		{
			name:    "one changed package yields it and its reverse dependencies, tmux and llm test imports included",
			graph:   testGraph(),
			changed: []string{"internal/a/a.go"},
			guards:  guards,
			wantCommand: gateCommand("./cmd/lyx ./internal/a ./internal/b ./internal/c ./internal/e ./internal/tmuxonly",
				"go test -tags integration -run '^(TestGuardD|TestGuardD2)$' ./internal/d"),
		},
		{
			name:        "a change to a dependency of cmd/lyx pulls in every package that builds the binary",
			graph:       testGraph(),
			changed:     []string{"internal/b/b.go"},
			wantCommand: gateCommand("./cmd/lyx ./internal/b ./internal/c ./internal/e"),
		},
		{
			name:        "a change outside cmd/lyx's dependencies leaves the binary-building packages out",
			graph:       testGraph(),
			changed:     []string{"internal/d/d.go"},
			wantCommand: gateCommand("./internal/d"),
		},
		{
			name:        "a rename between package directories puts both in the set",
			graph:       testGraph(),
			changed:     []string{"internal/c/x.go", "internal/d/x.go"},
			wantCommand: gateCommand("./internal/c ./internal/d"),
		},
		{
			name:        "a Markdown-only diff outside any package yields the build and the guards",
			graph:       testGraph(),
			changed:     []string{"docs/overview.md"},
			guards:      guards,
			wantCommand: "go build ./... && go test -tags integration -run '^(TestGuardA)$' ./internal/a && go test -tags integration -run '^(TestGuardD|TestGuardD2)$' ./internal/d",
		},
		{
			name:        "a non-Go file maps to its nearest enclosing package",
			graph:       testGraph(),
			changed:     []string{"internal/d/testdata/x.yaml"},
			wantCommand: gateCommand("./internal/d"),
		},
		{
			name:         "a file outside any package that is not Markdown falls back",
			graph:        testGraph(),
			changed:      []string{"contracts/recipe.yaml"},
			wantFallback: "contracts/recipe.yaml is neither a Markdown file nor under a Go package directory",
		},
		{
			name:         "a deleted Go file whose directory is no longer a package falls back",
			graph:        testGraph(),
			changed:      []string{"internal/gone/gone.go"},
			wantFallback: "internal/gone/gone.go is neither a Markdown file nor under a Go package directory",
		},
		{
			name:         "a go.mod change falls back",
			graph:        testGraph(),
			changed:      []string{"internal/a/a.go", "go.mod"},
			wantFallback: "go.mod changed",
		},
		{
			name:         "a go.sum change falls back",
			graph:        testGraph(),
			changed:      []string{"go.sum"},
			wantFallback: "go.sum changed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, fallback := commandFor(tt.graph, tt.changed, tt.guards)
			if command != tt.wantCommand || fallback != tt.wantFallback {
				t.Errorf("commandFor() = (%q, %q); want (%q, %q)", command, fallback, tt.wantCommand, tt.wantFallback)
			}
		})
	}
}

func TestCommandFor_OverLongCommandFallsBack(t *testing.T) {
	t.Parallel()

	pkgs := []pkg{{ImportPath: "m/internal/root", Dir: "internal/root"}}
	for i := 0; i < 400; i++ {
		dir := fmt.Sprintf("internal/dependent%03d", i)
		pkgs = append(pkgs, pkg{ImportPath: "m/" + dir, Dir: dir, Imports: []string{"m/internal/root"}})
	}

	command, fallback := commandFor(newModuleGraph(pkgs), []string{"internal/root/root.go"}, nil)
	if command != "" || !strings.Contains(fallback, "over the 8000 limit") {
		t.Errorf("commandFor() = (%q, %q); want a fallback naming the 8000 limit", command, fallback)
	}
}

func TestParseChangedPaths(t *testing.T) {
	t.Parallel()

	got := parseChangedPaths("M\x00a.go\x00D\x00b.go\x00R100\x00old.go\x00new.go\x00A\x00c.go\x00")
	want := "a.go b.go old.go new.go c.go"
	if strings.Join(got, " ") != want {
		t.Errorf("parseChangedPaths() = %v; want %s", got, want)
	}
}
