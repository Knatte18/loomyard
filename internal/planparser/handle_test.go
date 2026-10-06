// handle_test.go covers splitHandleDeclaration and handleUnit, the two pure string helpers card 9
// adds, plus the declaredHandles/referencedHandles predicates card 10 adds beside them.

package planparser

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitHandleDeclaration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		payload    string
		wantHandle string
		wantDecl   string
		wantOK     bool
	}{
		{
			name:       "well-formed handle declaration",
			payload:    "`plan:internal/foo#NewThing` -> `func NewThing() *Thing`",
			wantHandle: "plan:internal/foo#NewThing",
			wantDecl:   "func NewThing() *Thing",
			wantOK:     true,
		},
		{
			name:    "well-formed arrow pair with no plan: prefix is not a handle declaration",
			payload: "`old.Symbol` -> `new.Symbol`",
			wantOK:  false,
		},
		{
			name:    "plain ref with no arrow at all",
			payload: "`plan:internal/foo#NewThing`",
			wantOK:  false,
		},
		{
			name:    "malformed arrow bullet",
			payload: "plan:internal/foo#NewThing -> func NewThing()",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handle, decl, ok := splitHandleDeclaration(tt.payload)
			if ok != tt.wantOK {
				t.Fatalf("splitHandleDeclaration(%q) ok = %v; want %v", tt.payload, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if handle != tt.wantHandle {
				t.Errorf("splitHandleDeclaration(%q) handle = %q; want %q", tt.payload, handle, tt.wantHandle)
			}
			if decl != tt.wantDecl {
				t.Errorf("splitHandleDeclaration(%q) decl = %q; want %q", tt.payload, decl, tt.wantDecl)
			}
		})
	}
}

//testtiming:keep pins handleUnit's no-# and empty-unit results, which the validate tests reach only as findings
func TestHandleUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		handle   string
		wantUnit string
		wantOK   bool
	}{
		{
			name:     "handle carries a unit",
			handle:   "plan:internal/foo#NewThing",
			wantUnit: "internal/foo",
			wantOK:   true,
		},
		{
			name:   "handle with no # names no unit",
			handle: "plan:internal/foo",
			wantOK: false,
		},
		{
			name:     "handle whose unit is empty",
			handle:   "plan:#NewThing",
			wantUnit: "",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			unit, ok := handleUnit(tt.handle)
			if ok != tt.wantOK {
				t.Fatalf("handleUnit(%q) ok = %v; want %v", tt.handle, ok, tt.wantOK)
			}
			if ok && unit != tt.wantUnit {
				t.Errorf("handleUnit(%q) unit = %q; want %q", tt.handle, unit, tt.wantUnit)
			}
		})
	}
}

// TestHandleIndexes asserts declaredHandles maps each declared handle to the cards declaring it, and referencedHandles maps each handle-shaped target or use to the cards naming it, leaving a non-handle ref out.
//
//testtiming:keep pins declaredHandles and referencedHandles map contents, which TestValidate_HandleConsistency reaches only as findings
func TestHandleIndexes(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Cards: []Card{
			{Number: 1, Slug: "one",
				Declarations: []CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing()"}},
				Targets:      []string{"plan:internal/foo#NewThing", "internal/foo/other.go#"}},
			{Number: 2, Slug: "two",
				Declarations: []CardDeclaration{{Handle: "plan:internal/bar#NewOther", Decl: "func NewOther()"}},
				Uses:         []string{"plan:internal/foo#NewThing"}},
		},
	}

	declared := declaredHandles(plan)
	if want := []string{"1-one"}; !slices.Equal(declared["plan:internal/foo#NewThing"], want) {
		t.Errorf("declaredHandles()[%q] = %v; want %v", "plan:internal/foo#NewThing", declared["plan:internal/foo#NewThing"], want)
	}
	if want := []string{"2-two"}; !slices.Equal(declared["plan:internal/bar#NewOther"], want) {
		t.Errorf("declaredHandles()[%q] = %v; want %v", "plan:internal/bar#NewOther", declared["plan:internal/bar#NewOther"], want)
	}
	if _, ok := declared["plan:nonexistent#X"]; ok {
		t.Errorf("declaredHandles() carries an entry for an undeclared handle")
	}

	referenced := referencedHandles(plan)
	if want := []string{"1-one", "2-two"}; !slices.Equal(referenced["plan:internal/foo#NewThing"], want) {
		t.Errorf("referencedHandles()[%q] = %v; want %v", "plan:internal/foo#NewThing", referenced["plan:internal/foo#NewThing"], want)
	}
	if _, ok := referenced["internal/foo/other.go#"]; ok {
		t.Errorf("referencedHandles() carries an entry for a non-handle ref")
	}
}

// TestCheckHandleMalformed_FileUnitRule asserts a handle whose unit half names a ".go" file is refused up front with a handle-malformed finding naming the package-directory fix: such a handle canonicalizes to a member spelling quarry's Resolve can never answer, so the plan would validate clean and then wedge at the creating card's own done-check.
// The rule binds a handle whose unit half is actually read -- a Create declaration's -- and not one claimed only as a Rename pair's to-side, whose unit canonicalization takes from the resolved old side; it is gated off for language none.
func TestCheckHandleMalformed_FileUnitRule(t *testing.T) {
	t.Parallel()

	fileUnitDeclared := Card{
		Number: 1, Slug: "one",
		Targets:      []string{"plan:greeter/farewell.go#Farewell"},
		Declarations: []CardDeclaration{{Handle: "plan:greeter/farewell.go#Farewell", Decl: "func Farewell(name string) string"}},
	}
	renameToSide := Card{
		Number: 1, Slug: "one",
		Targets: []string{"greeter#Hello", "plan:greeter/farewell.go#Farewell"},
		Pairs:   []MovePair{{Old: "greeter#Hello", New: "plan:greeter/farewell.go#Farewell"}},
	}

	tests := []struct {
		name         string
		language     string
		cards        []Card
		wantFindings int
		wantDetail   []string
	}{
		{
			name: "file-unit declaration", language: "go", cards: []Card{fileUnitDeclared},
			wantFindings: 1, wantDetail: []string{"greeter/farewell.go", `"greeter"`},
		},
		{name: "language none gates the rule off", language: "none", cards: []Card{fileUnitDeclared}},
		{
			name: "package-unit handle", language: "go",
			cards: []Card{{
				Number: 1, Slug: "one",
				Targets:      []string{"plan:greeter#Farewell"},
				Declarations: []CardDeclaration{{Handle: "plan:greeter#Farewell", Decl: "func Farewell(name string) string"}},
			}},
		},
		{name: "rename to-side only", language: "go", cards: []Card{renameToSide}},
		{
			// One finding per referencing card.
			name: "handle claimed by a rename and a declaration", language: "go",
			cards: []Card{renameToSide, {
				Number: 2, Slug: "two",
				Targets:      []string{"plan:greeter/farewell.go#Farewell"},
				Declarations: []CardDeclaration{{Handle: "plan:greeter/farewell.go#Farewell", Decl: "func Farewell(name string) string"}},
			}},
			wantFindings: 2,
		},
		{
			name: "unclaimed file-unit handle still binds", language: "go",
			cards:        []Card{{Number: 1, Slug: "one", Targets: []string{"plan:greeter/farewell.go#Farewell"}}},
			wantFindings: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := checkHandleMalformed(&Plan{Language: tt.language, Cards: tt.cards})
			if len(got) != tt.wantFindings {
				t.Fatalf("checkHandleMalformed() = %+v; want %d finding(s)", got, tt.wantFindings)
			}
			for _, f := range got {
				if f.Check != "handle-malformed" {
					t.Errorf("finding Check = %q; want handle-malformed", f.Check)
				}
			}
			for _, sub := range tt.wantDetail {
				if !strings.Contains(got[0].Detail, sub) {
					t.Errorf("finding detail = %q; want it to contain %q", got[0].Detail, sub)
				}
			}
		})
	}
}

// TestHandleBody asserts HandleBody strips the plan: prefix of a handle-shaped ref and reports every other shape as not a handle, in agreement with IsHandleRef.
//
//testtiming:keep pins HandleBody's prefix strip and IsHandleRef's agreement with it, which TestCardTargetDirs does not assert
func TestHandleBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		wantBody string
		wantOK   bool
	}{
		{name: "handle-shaped", raw: "plan:internal/foo#NewThing", wantBody: "internal/foo#NewThing", wantOK: true},
		{name: "handle prefix alone", raw: "plan:", wantBody: "", wantOK: true},
		{name: "glyph-shaped", raw: "internal/foo#NewThing", wantOK: false},
		{name: "path-shaped", raw: "internal/foo/bar.go", wantOK: false},
		{name: "empty", raw: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, ok := HandleBody(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("HandleBody(%q) ok = %v; want %v", tt.raw, ok, tt.wantOK)
			}
			if ok && body != tt.wantBody {
				t.Errorf("HandleBody(%q) body = %q; want %q", tt.raw, body, tt.wantBody)
			}
			if got := IsHandleRef(tt.raw); got != tt.wantOK {
				t.Errorf("IsHandleRef(%q) = %v; want %v", tt.raw, got, tt.wantOK)
			}
		})
	}
}

func TestNewHandle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		glyphID string
		want    string
	}{
		{name: "member glyph", glyphID: "internal/foo#NewThing", want: "plan:internal/foo#NewThing"},
		{name: "empty", glyphID: "", want: "plan:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NewHandle(tt.glyphID); got != tt.want {
				t.Errorf("NewHandle(%q) = %q; want %q", tt.glyphID, got, tt.want)
			}
		})
	}
}

// TestHandleMember asserts HandleMember returns the text after a handle's #, and HandleIdentifier the final identifier of a qualified member, refusing an empty or absent member.
// The identifier cases are planglyph's draftHandleIdentifier table, ported as the assertion base.
func TestHandleMember(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		handle      string
		wantMember  string
		wantOK      bool
		wantIdent   string
		wantIdentOK bool
	}{
		{name: "member handle", handle: "plan:internal/alpha#Renamed", wantMember: "Renamed", wantOK: true, wantIdent: "Renamed", wantIdentOK: true},
		{name: "qualified member handle", handle: "plan:internal/alpha#Counter.Tally", wantMember: "Counter.Tally", wantOK: true, wantIdent: "Tally", wantIdentOK: true},
		{name: "no # at all", handle: "plan:internal/alpha"},
		{name: "empty member", handle: "plan:internal/alpha#", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			member, ok := HandleMember(tt.handle)
			if ok != tt.wantOK {
				t.Fatalf("HandleMember(%q) ok = %v; want %v", tt.handle, ok, tt.wantOK)
			}
			if ok && member != tt.wantMember {
				t.Errorf("HandleMember(%q) member = %q; want %q", tt.handle, member, tt.wantMember)
			}

			ident, identOK := HandleIdentifier(tt.handle)
			if identOK != tt.wantIdentOK {
				t.Fatalf("HandleIdentifier(%q) ok = %v; want %v", tt.handle, identOK, tt.wantIdentOK)
			}
			if identOK && ident != tt.wantIdent {
				t.Errorf("HandleIdentifier(%q) = %q; want %q", tt.handle, ident, tt.wantIdent)
			}
		})
	}
}
