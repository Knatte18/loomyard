// version.go defines the two pinned per-binary minimum multiplexer versions (native tmux and its Windows port — psmux — have independent, differently-shaped `-V` output, so one constant cannot compare both) plus the pure `-V` parsers and comparator the capability probe (probe.go) builds on.
// Every function here is build-tag-free and host-testable: it transforms strings, never touches an OS primitive.

package reedengine

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Pinned minimum multiplexer versions: minTmuxVersionWindows for psmux,
// minTmuxVersion for native tmux.
var (
	minTmuxVersionWindows = [3]int{3, 3, 3} // Windows tmux port / psmux (3-component version)
	minTmuxVersion        = [3]int{3, 3, 0} // native POSIX tmux (2-component version)
)

// tmuxVersionLinePattern extracts the major.minor pair from tmux's -V output.
var tmuxVersionLinePattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// tmuxVersionLinePatternWindows extracts major.minor.patch from psmux's -V output.
var tmuxVersionLinePatternWindows = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// parseTmuxVersion parses tmux's -V output into [major, minor, 0].
func parseTmuxVersion(out string) ([3]int, error) {
	trimmed := strings.TrimSpace(out)
	m := tmuxVersionLinePattern.FindStringSubmatch(trimmed)
	if m == nil {
		return [3]int{}, fmt.Errorf("parse tmux version: unrecognized output %q", trimmed)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse tmux version: %w", err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse tmux version: %w", err)
	}
	return [3]int{major, minor, 0}, nil
}

// parseTmuxVersionWindows parses psmux's -V output into [major, minor, patch].
func parseTmuxVersionWindows(out string) ([3]int, error) {
	trimmed := strings.TrimSpace(out)
	m := tmuxVersionLinePatternWindows.FindStringSubmatch(trimmed)
	if m == nil {
		return [3]int{}, fmt.Errorf("parse tmux version: unrecognized output %q", trimmed)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse tmux version: %w", err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse tmux version: %w", err)
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse tmux version: %w", err)
	}
	return [3]int{major, minor, patch}, nil
}

// versionAtLeast reports whether got >= min under lexicographic comparison.
func versionAtLeast(got, min [3]int) bool {
	for i := range got {
		if got[i] != min[i] {
			return got[i] > min[i]
		}
	}
	return true
}

// minMultiplexerVersion returns this GOOS's pinned minimum version:
// minTmuxVersionWindows on Windows, minTmuxVersion elsewhere.
func minMultiplexerVersion() [3]int {
	if runtime.GOOS == "windows" {
		return minTmuxVersionWindows
	}
	return minTmuxVersion
}

// parseMultiplexerVersion parses -V output using the GOOS-specific parser.
func parseMultiplexerVersion(out string) ([3]int, error) {
	if runtime.GOOS == "windows" {
		return parseTmuxVersionWindows(out)
	}
	return parseTmuxVersion(out)
}
