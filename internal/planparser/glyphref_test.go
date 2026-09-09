// glyphref_test.go covers parseGlyph's pure delegation to glyph.Parse and planLanguage's mapping
// from Plan.Language to the glyph package's own Language type.

package planparser

import (
	"testing"

	"github.com/Knatte18/quarry/glyph"
)

func TestParseGlyph(t *testing.T) {
	t.Parallel()

	t.Run("member glyph parses", func(t *testing.T) {
		t.Parallel()
		g, err := parseGlyph(glyph.Go, "internal/boardcli#RowJSON")
		if err != nil {
			t.Fatalf("parseGlyph() error = %v; want nil", err)
		}
		if g.Unit != "internal/boardcli" || g.Name != "RowJSON" {
			t.Errorf("parseGlyph() = %+v; want Unit=internal/boardcli Name=RowJSON", g)
		}
	})

	t.Run("malformed glyph fails", func(t *testing.T) {
		t.Parallel()
		if _, err := parseGlyph(glyph.Go, "not-a-glyph"); err == nil {
			t.Fatalf("parseGlyph() error = nil; want an error for a #-free string")
		}
	})
}

func TestPlanLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		wantLang glyph.Language
		wantOK   bool
	}{
		{name: "go", language: "go", wantLang: glyph.Go, wantOK: true},
		{name: "absent (zero value) defaults to go", language: "", wantLang: glyph.Go, wantOK: true},
		{name: "none opts out", language: "none", wantOK: false},
		{name: "unrecognized value opts out", language: "python", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &Plan{Language: tt.language}
			gotLang, gotOK := planLanguage(plan)
			if gotOK != tt.wantOK {
				t.Errorf("planLanguage(%q) ok = %v; want %v", tt.language, gotOK, tt.wantOK)
			}
			if tt.wantOK && gotLang != tt.wantLang {
				t.Errorf("planLanguage(%q) lang = %v; want %v", tt.language, gotLang, tt.wantLang)
			}
		})
	}
}

// TestPlanGlyphLanguage asserts Plan.GlyphLanguage — the exported delegate to planLanguage —
// agrees with planLanguage on every case.
func TestPlanGlyphLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		wantLang glyph.Language
		wantOK   bool
	}{
		{name: "absent (zero value) defaults to go", language: "", wantLang: glyph.Go, wantOK: true},
		{name: "go", language: "go", wantLang: glyph.Go, wantOK: true},
		{name: "none opts out", language: "none", wantOK: false},
		{name: "unrecognized value opts out", language: "python", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &Plan{Language: tt.language}
			gotLang, gotOK := plan.GlyphLanguage()
			wantLang, wantOK := planLanguage(plan)
			if gotOK != wantOK || gotOK != tt.wantOK {
				t.Errorf("Plan.GlyphLanguage(%q) ok = %v; want %v (planLanguage agrees: %v)", tt.language, gotOK, tt.wantOK, wantOK)
			}
			if tt.wantOK && (gotLang != tt.wantLang || gotLang != wantLang) {
				t.Errorf("Plan.GlyphLanguage(%q) lang = %v; want %v (planLanguage agrees: %v)", tt.language, gotLang, tt.wantLang, wantLang)
			}
		})
	}
}
