// glyphref_test.go covers ParseRef's alphabet choice and planLanguage's mapping
// from Plan.Language to the glyph package's own Language type.

package planparser

import (
	"testing"

	"github.com/Knatte18/quarry/glyph"
)

//testtiming:keep pins ParseRef's alphabet choice per plan language, which TestCardTargetDirs does not assert
func TestParseRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		ref      string
		wantLang glyph.Language
		wantUnit string
		wantName string
		wantErr  bool
	}{
		{name: "go: Go member glyph parses as Go", language: "go", ref: "internal/boardcli#RowJSON", wantLang: glyph.Go, wantUnit: "internal/boardcli", wantName: "RowJSON"},
		{name: "go: #-free string errors", language: "go", ref: "not-a-glyph", wantErr: true},
		{name: "go: C# member glyph with a parameter list errors", language: "go", ref: "Glyphchain.Sharp#Renderer.Draw(int)", wantErr: true},
		{name: "csharp: C# member glyph parses as C#", language: "csharp", ref: "Glyphchain.Sharp#Renderer.Draw(int)", wantLang: glyph.CSharp, wantUnit: "Glyphchain.Sharp", wantName: "Draw"},
		{name: "csharp: namespace self glyph parses as C#", language: "csharp", ref: "Glyphchain.Sharp#", wantLang: glyph.CSharp, wantUnit: "Glyphchain.Sharp"},
		{name: "csharp: a self glyph both alphabets take is C#", language: "csharp", ref: "README.md#", wantLang: glyph.CSharp, wantUnit: "README.md"},
		{name: "csharp: a slashed path self glyph parses as Go", language: "csharp", ref: "src/Foo.cs#", wantLang: glyph.Go, wantUnit: "src/Foo.cs"},
		{name: "csharp: a slashed member neither alphabet takes errors", language: "csharp", ref: "a/b#Draw(int)", wantErr: true},
		{name: "csharp: a Go-only member errors with the C# error", language: "csharp", ref: "a/b#Func", wantErr: true},
		{name: "none: every ref errors", language: "none", ref: "internal/boardcli#RowJSON", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g, err := ParseRef(&Plan{Language: tt.language}, tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRef(%q, %q) = %+v; want an error", tt.language, tt.ref, g)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRef(%q, %q) error = %v; want nil", tt.language, tt.ref, err)
			}
			if g.Lang != tt.wantLang || g.Unit != tt.wantUnit || g.Name != tt.wantName {
				t.Errorf("ParseRef(%q, %q) = {Lang:%q Unit:%q Name:%q}; want {Lang:%q Unit:%q Name:%q}", tt.language, tt.ref, g.Lang, g.Unit, g.Name, tt.wantLang, tt.wantUnit, tt.wantName)
			}
		})
	}
}

// TestPlanGlyphLanguage asserts Plan.GlyphLanguage reports whether the plan's language selects a glyph alphabet.
func TestPlanGlyphLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		want     bool
	}{
		{name: "absent (zero value) defaults to go", language: "", want: true},
		{name: "go", language: "go", want: true},
		{name: "csharp", language: "csharp", want: true},
		{name: "none opts out", language: "none", want: false},
		{name: "unrecognized value opts out", language: "python", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &Plan{Language: tt.language}
			if got := plan.GlyphLanguage(); got != tt.want {
				t.Errorf("Plan.GlyphLanguage(%q) = %v; want %v", tt.language, got, tt.want)
			}
		})
	}
}
