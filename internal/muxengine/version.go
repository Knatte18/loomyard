// version.go defines the two pinned per-binary minimum multiplexer versions
// (psmux and tmux have independent, differently-shaped `-V` output, so one
// constant cannot compare both) plus the pure `-V` parsers and comparator
// the capability probe (probe.go) builds on. Every function here is
// build-tag-free and host-testable: it transforms strings, never touches an
// OS primitive.

package muxengine

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Pinned minimum multiplexer versions. minPsmuxVersion is set one patch
// below the version the on-box dev psmux binary (psmux 3.3.4) reports, so
// the pin acts as a drift canary — it fails loud on a real regression
// without bricking `mux up` on the current dev box. minTmuxVersion is
// pinned per the batch discussion at 3.3, the first tmux release this
// engine's overlay was verified against.
var (
	minPsmuxVersion = [3]int{3, 3, 3}
	minTmuxVersion  = [3]int{3, 3, 0}
)

// tmuxVersionLinePattern extracts the leading major.minor pair from tmux's
// `-V` output. tmux's own version scheme has no stable patch component —
// releases are named "tmux X.Y", sometimes with a trailing letter suffix
// ("tmux 3.3a") or an unreleased "next-X.Y" prefix — so only major.minor is
// pulled out; patch is always reported as 0.
var tmuxVersionLinePattern = regexp.MustCompile(`(\d+)\.(\d+)`)

// psmuxVersionLinePattern extracts the major.minor.patch triple from
// psmux's `-V` output ("psmux X.Y.Z"), which — unlike tmux — always reports
// a full three-component version.
var psmuxVersionLinePattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// parseTmuxVersion parses tmux's `-V` output shape ("tmux X.Y", tolerating
// a "next-X.Y" prefix or a trailing letter suffix like "3.3a") into
// [major, minor, patch], with patch always 0 since tmux's own scheme
// carries none. It returns an error when no major.minor pair can be found
// anywhere in out.
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

// parsePsmuxVersion parses psmux's `-V` output shape ("psmux X.Y.Z") into
// [major, minor, patch]. It returns an error when no major.minor.patch
// triple can be found anywhere in out.
func parsePsmuxVersion(out string) ([3]int, error) {
	trimmed := strings.TrimSpace(out)
	m := psmuxVersionLinePattern.FindStringSubmatch(trimmed)
	if m == nil {
		return [3]int{}, fmt.Errorf("parse psmux version: unrecognized output %q", trimmed)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse psmux version: %w", err)
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse psmux version: %w", err)
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return [3]int{}, fmt.Errorf("parse psmux version: %w", err)
	}
	return [3]int{major, minor, patch}, nil
}

// versionAtLeast reports whether got is greater than or equal to min under
// lexicographic [major, minor, patch] comparison — the ordering both
// pinned floors (minPsmuxVersion, minTmuxVersion) are checked against.
func versionAtLeast(got, min [3]int) bool {
	for i := range got {
		if got[i] != min[i] {
			return got[i] > min[i]
		}
	}
	return true
}

// minMultiplexerVersion returns this GOOS's pinned minimum multiplexer
// version: minPsmuxVersion on Windows, minTmuxVersion everywhere else,
// mirroring the engine's own Windows-psmux / POSIX-tmux binary split
// (template_windows.go / template_posix.go).
func minMultiplexerVersion() [3]int {
	if runtime.GOOS == "windows" {
		return minPsmuxVersion
	}
	return minTmuxVersion
}

// parseMultiplexerVersion parses a `-V` output line using this GOOS's
// multiplexer-specific parser: parsePsmuxVersion on Windows,
// parseTmuxVersion everywhere else.
func parseMultiplexerVersion(out string) ([3]int, error) {
	if runtime.GOOS == "windows" {
		return parsePsmuxVersion(out)
	}
	return parseTmuxVersion(out)
}
