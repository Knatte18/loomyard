//go:build tier0

// derive_test.go holds the hand-built package graph the pure core's tests share and covers the changed-path parser.

package impactset

import (
	"strings"
	"testing"
)

// testGraph is a small module.
// Package b imports a, c imports b, d is unrelated, and tmuxonly reaches a only through a test import.
// The command package cmd/lym imports b, and e builds the binary in its tests and reaches nothing else, so it joins only for a change to cmd/lym itself.
func testGraph() moduleGraph {
	const m = "m/"
	return newModuleGraph([]pkg{
		{ImportPath: m + "internal/a", Dir: "internal/a"},
		{ImportPath: m + "internal/b", Dir: "internal/b", Imports: []string{m + "internal/a"}},
		{ImportPath: m + "internal/c", Dir: "internal/c", Imports: []string{m + "internal/b"}},
		{ImportPath: m + "internal/d", Dir: "internal/d"},
		{ImportPath: m + "internal/tmuxonly", Dir: "internal/tmuxonly", TestImports: []string{m + "internal/a"}},
		{ImportPath: m + "cmd/lym", Dir: "cmd/lym", Imports: []string{m + "internal/b"}},
		{ImportPath: m + "internal/testkit/lymbin", Dir: "internal/testkit/lymbin"},
		{ImportPath: m + "internal/e", Dir: "internal/e", TestImports: []string{m + "internal/testkit/lymbin"}},
	})
}

func TestParseChangedPaths(t *testing.T) {
	t.Parallel()

	got := parseChangedPaths("M\x00a.go\x00D\x00b.go\x00R100\x00old.go\x00new.go\x00A\x00c.go\x00")
	want := "a.go b.go old.go new.go c.go"
	if strings.Join(got, " ") != want {
		t.Errorf("parseChangedPaths() = %v; want %s", got, want)
	}
}
