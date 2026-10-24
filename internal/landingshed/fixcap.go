// fixcap.go implements FixCap, the judge of a merge fixer's change against the size and kind caps that bound what an unreviewed fix can let through.

package landingshed

import (
	"fmt"
	"path"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// FixCap bounds one fixer change: its size, and the kinds of path and line it may touch.
type FixCap struct {
	// MaxLines is the added plus deleted lines a change may hold.
	MaxLines int
	// IsTestFile reports whether a slash-separated repository-relative path is a test file of the run's language.
	IsTestFile func(path string) bool
}

// buildConfigNames are the base names of gate and build configuration files.
var buildConfigNames = []string{"go.mod", "go.sum", "go.work", "Makefile", "gate.yaml"}

// buildConfigExtensions are the extensions of gate and build configuration files.
var buildConfigExtensions = []string{".csproj", ".sln", ".props", ".targets"}

// testSkipMarkers are the fragments that make an added line skip a test.
var testSkipMarkers = []string{"t.Skip", "[Ignore", "Skip ="}

// buildConstraintPrefixes are the openings of an added line that adds a build constraint.
var buildConstraintPrefixes = []string{"//go:build", "// +build", "#if"}

// Exceeded returns the reason change is over the caps, or "" when it is within them.
// A change is over when its lines exceed MaxLines; when, whatever its size, it touches a test file, a testdata path or a gate or build configuration file; or when an added line skips a test or adds a build constraint.
// The reason names the first offending path or line.
func (c FixCap) Exceeded(change fabricengine.LineChange) string {
	if change.Lines > c.MaxLines {
		return fmt.Sprintf("the change holds %d lines, over the cap of %d", change.Lines, c.MaxLines)
	}
	for _, p := range change.Paths {
		if c.IsTestFile(p) {
			return fmt.Sprintf("the change touches the test file %s", p)
		}
		if hasPathSegment(p, "testdata") {
			return fmt.Sprintf("the change touches the testdata file %s", p)
		}
		if isBuildConfigPath(p) {
			return fmt.Sprintf("the change touches the gate or build configuration file %s", p)
		}
	}
	for _, line := range change.AddedLines {
		trimmed := strings.TrimSpace(line)
		for _, marker := range testSkipMarkers {
			if strings.Contains(trimmed, marker) {
				return fmt.Sprintf("the change adds a line that skips a test: %s", trimmed)
			}
		}
		for _, prefix := range buildConstraintPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				return fmt.Sprintf("the change adds a build constraint: %s", trimmed)
			}
		}
	}
	return ""
}

// hasPathSegment reports whether any slash-separated segment of p is segment.
func hasPathSegment(p, segment string) bool {
	for _, s := range strings.Split(p, "/") {
		if s == segment {
			return true
		}
	}
	return false
}

// isBuildConfigPath reports whether p is a gate or build configuration file, or lies under .github.
func isBuildConfigPath(p string) bool {
	if hasPathSegment(p, ".github") {
		return true
	}
	base := path.Base(p)
	for _, name := range buildConfigNames {
		if base == name {
			return true
		}
	}
	if strings.HasPrefix(base, "Directory.Build.") {
		return true
	}
	ext := path.Ext(base)
	for _, e := range buildConfigExtensions {
		if ext == e {
			return true
		}
	}
	return false
}
