// constraintchokepoint_test.go enforces the Glyph Conversion Chokepoint Invariant in planparser's package doc:
// loomyard performs no glyph<->path conversion of its own outside quarry's glyph package.
// glyph.Self is the only path->glyph call.
// Glyph.UnitPath is the only glyph->path call.
// glyph.Parse plus Glyph.String are the only glyph grammar.
// The check is a same-line raw-substring heuristic over production (non-test) source.
// It catches a direct textual call site, not a value threaded through a local variable or a transitive helper,
// so it narrows the gap without closing it.

package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// glyphUnitPathContextTokens are the path-construction call substrings.
// On a line that also contains ".Unit", one of them marks a Glyph.Unit value being read as a disk path.
// Such a read is the violation that Glyph.UnitPath exists to prevent.
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

// isHashTrimmingSuffixCallLine reports whether line calls strings.TrimSuffix on the literal "#" suffix.
// Stripping that suffix in loomyard is forbidden, because the "#" rule is quarry's own contract,
// and a loomyard copy would leak that assumption into a non-Go plan.
func isHashTrimmingSuffixCallLine(line string) bool {
	return strings.Contains(line, "TrimSuffix(") && strings.Contains(line, `"#"`)
}

// isGlyphUnitAsPathLine reports whether line reads a ".Unit" field next to a path-construction call.
// It flags Glyph.Unit being treated as a disk path, where it should pass through Glyph.UnitPath.
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

// glyphChokepointViolation returns the first offending line of data, 1-indexed, and a human-readable reason.
// It skips comment-only lines, so the doc comments that describe the banned patterns are not flagged.
// It returns 0, "" when data carries no violation.
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

// TestGlyphConversionChokepoint_NoLocalConversion walks every non-test *.go file under internal/ and cmd/.
// It fails on each file carrying a chokepoint violation.
//
//lym:guard
func TestGlyphConversionChokepoint_NoLocalConversion(t *testing.T) {
	tiergate.Require(t, 0)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
		if line, reason := glyphChokepointViolation(f.Data); line != 0 {
			failures = append(failures, f.Rel+": line "+strconv.Itoa(line)+": "+reason)
		}
	})

	scankit.RequireFloor(t, scanned, 20, "glyph conversion chokepoint guard")

	if len(failures) > 0 {
		t.Errorf("glyph conversion chokepoint violated (see planparser's package doc):\n%s", strings.Join(failures, "\n"))
	}
}

// TestGlyphChokepointViolation_FiresOnOffendingStaysQuietOnCompliant checks the guard itself.
// It fires on a synthetic offending source and stays quiet on a compliant one.
//
//testtiming:keep proves the TestGlyphConversionChokepoint_NoLocalConversion guard fires on a synthetic offending source and stays quiet on a compliant one
func TestGlyphChokepointViolation_FiresOnOffendingStaysQuietOnCompliant(t *testing.T) {
	tiergate.Require(t, 0)
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
