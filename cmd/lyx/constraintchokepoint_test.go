// constraintchokepoint_test.go enforces `PATTERN-glyph-conversion-chokepoint`:
// loomyard performs no glyph<->path conversion of its own outside quarry's glyph package.
// glyph.Self is the only path->glyph call, Glyph.UnitPath is the only glyph->path call, and
// glyph.Parse plus Glyph.String are the only glyph grammar.
// Like tierpurity_test.go's own bannedTokens, this is a same-line raw-substring heuristic over
// production (non-test) source: it catches a direct textual call site, not a value threaded
// through a local variable or a transitive helper, and it narrows the gap rather than closing it.

package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// glyphUnitPathContextTokens are the path-construction call substrings that, appearing on the same
// line as ".Unit", mark a Glyph.Unit value being read as if it were a disk path -- the exact
// violation Glyph.UnitPath exists to prevent.
var glyphUnitPathContextTokens = []string{
	"filepath.Join(",
	"filepath.Abs(",
	"filepath.Dir(",
	"path.Join(",
	"os.Open(",
	"os.Stat(",
	"os.Lstat(",
	"os.ReadFile(",
	"os.WriteFile(",
	"os.MkdirAll(",
	"os.ReadDir(",
}

// isHashTrimmingSuffixCallLine reports whether line contains a strings.TrimSuffix call trimming
// the literal "#" suffix -- the loomyard-side glyph-string-stripping operation the
// glyph-conversion-chokepoint decision forbids, since the "#"-trim rule is quarry's own contract
// and a loomyard copy would leak that assumption into a non-Go plan.
func isHashTrimmingSuffixCallLine(line string) bool {
	return strings.Contains(line, "TrimSuffix(") && strings.Contains(line, `"#"`)
}

// isGlyphUnitAsPathLine reports whether line reads a ".Unit" field alongside one of
// glyphUnitPathContextTokens' own path-construction calls -- Glyph.Unit being treated as a disk
// path rather than passed through Glyph.UnitPath.
func isGlyphUnitAsPathLine(line string) bool {
	if !strings.Contains(line, ".Unit") {
		return false
	}
	for _, tok := range glyphUnitPathContextTokens {
		if strings.Contains(line, tok) {
			return true
		}
	}
	return false
}

// glyphChokepointViolation returns the first offending line (1-indexed) and a human-readable
// reason for the first chokepoint violation found in data, skipping comment-only lines so this
// package's own doc comments describing the banned patterns are not themselves flagged. It
// returns 0, "" when data carries no violation.
func glyphChokepointViolation(data []byte) (line int, reason string) {
	for i, l := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if isHashTrimmingSuffixCallLine(l) {
			return i + 1, `a "#"-trimming strings.TrimSuffix call over a glyph-typed value`
		}
		if isGlyphUnitAsPathLine(l) {
			return i + 1, "Glyph.Unit read in a path-construction context instead of Glyph.UnitPath"
		}
	}
	return 0, ""
}

// TestGlyphConversionChokepoint_NoLocalConversion walks every non-test *.go file under internal/
// and cmd/ and fails on the first file carrying a
// chokepoint violation.
func TestGlyphConversionChokepoint_NoLocalConversion(t *testing.T) {
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
		if line, reason := glyphChokepointViolation(f.Data); line != 0 {
			failures = append(failures, f.Rel+": line "+strconv.Itoa(line)+": "+reason)
		}
	})

	scankit.RequireFloor(t, scanned, 20, "glyph conversion chokepoint guard")

	if len(failures) > 0 {
		t.Errorf("`PATTERN-glyph-conversion-chokepoint` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// TestGlyphChokepointViolation_FiresOnOffendingStaysQuietOnCompliant proves the guard itself fires on a synthetic offending source string and stays quiet on a compliant one, so the guard is tested rather than merely asserted.
//
//testtiming:keep proves the TestGlyphConversionChokepoint_NoLocalConversion guard fires on a synthetic offending source and stays quiet on a compliant one
func TestGlyphChokepointViolation_FiresOnOffendingStaysQuietOnCompliant(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantViolation bool
	}{
		{
			name:          "hash-trimming TrimSuffix over a glyph-typed value",
			src:           "unit := strings.TrimSuffix(g.String(), \"#\")\n",
			wantViolation: true,
		},
		{
			name:          "Glyph.Unit read in a path-construction context",
			src:           "p := filepath.Join(root, g.Unit)\n",
			wantViolation: true,
		},
		{
			name:          "compliant: glyph.Self is the path->glyph call",
			src:           "g, err := glyph.Self(lang, raw)\n",
			wantViolation: false,
		},
		{
			name:          "compliant: Glyph.UnitPath is the glyph->path call",
			src:           "p, ok := g.UnitPath()\n",
			wantViolation: false,
		},
		{
			name:          "compliant: a doc comment describing the banned pattern is not itself flagged",
			src:           "// no strings.TrimSuffix(raw, \"#\") anywhere in this package\n",
			wantViolation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, _ := glyphChokepointViolation([]byte(tt.src))
			gotViolation := line != 0
			if gotViolation != tt.wantViolation {
				t.Errorf("glyphChokepointViolation(%q) violation = %v; want %v", tt.src, gotViolation, tt.wantViolation)
			}
		})
	}
}
