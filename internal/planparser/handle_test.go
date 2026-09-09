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

func TestDeclaredHandles(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Cards: []Card{
			{Number: 1, Slug: "one", Declarations: []CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing()"}}},
			{Number: 2, Slug: "two", Declarations: []CardDeclaration{{Handle: "plan:internal/bar#NewOther", Decl: "func NewOther()"}}},
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
}

func TestReferencedHandles(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Cards: []Card{
			{Number: 1, Slug: "one", Targets: []string{"plan:internal/foo#NewThing", "internal/foo/other.go#"}},
			{Number: 2, Slug: "two", Uses: []string{"plan:internal/foo#NewThing"}},
		},
	}

	referenced := referencedHandles(plan)
	want := []string{"1-one", "2-two"}
	if !slices.Equal(referenced["plan:internal/foo#NewThing"], want) {
		t.Errorf("referencedHandles()[%q] = %v; want %v", "plan:internal/foo#NewThing", referenced["plan:internal/foo#NewThing"], want)
	}
	if _, ok := referenced["internal/foo/other.go#"]; ok {
		t.Errorf("referencedHandles() carries an entry for a non-handle ref")
	}
}

// TestCheckHandleMalformed_FileUnitHandle is F-B8's (round fable5-high-r3) regression test: a
// handle whose unit half names a ".go" file canonicalizes (via quarry.Name, which accepts it) to a
// member spelling quarry's Resolve can never answer, so the plan validated clean and the run then
// wedged at the creating card's own record-batch done-check — the pure layer must refuse the
// spelling up front, naming the package-directory fix.
func TestCheckHandleMalformed_FileUnitHandle(t *testing.T) {
	plan := &Plan{
		Language: "go",
		Cards: []Card{{
			Number: 1, Slug: "one",
			Targets:      []string{"plan:greeter/farewell.go#Farewell"},
			Declarations: []CardDeclaration{{Handle: "plan:greeter/farewell.go#Farewell", Decl: "func Farewell(name string) string"}},
		}},
	}

	got := checkHandleMalformed(plan)
	if len(got) != 1 || got[0].Check != "handle-malformed" {
		t.Fatalf("checkHandleMalformed(file-unit handle) = %+v; want exactly one handle-malformed finding", got)
	}
	if !strings.Contains(got[0].Detail, "greeter/farewell.go") || !strings.Contains(got[0].Detail, `"greeter"`) {
		t.Errorf("finding detail = %q; want it to name the file unit and the package-directory fix", got[0].Detail)
	}

	plan.Language = "none"
	if got := checkHandleMalformed(plan); len(got) != 0 {
		t.Errorf("checkHandleMalformed(language none) = %+v; want the file-unit rule gated off", got)
	}

	plan.Language = "go"
	plan.Cards[0].Targets = []string{"plan:greeter#Farewell"}
	plan.Cards[0].Declarations = []CardDeclaration{{Handle: "plan:greeter#Farewell", Decl: "func Farewell(name string) string"}}
	if got := checkHandleMalformed(plan); len(got) != 0 {
		t.Errorf("checkHandleMalformed(package-unit handle) = %+v; want no findings", got)
	}
}

func TestIsHandleRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "handle-shaped", raw: "plan:internal/foo#NewThing", want: true},
		{name: "handle prefix alone", raw: "plan:", want: true},
		{name: "glyph-shaped", raw: "internal/foo#NewThing", want: false},
		{name: "path-shaped", raw: "internal/foo/bar.go", want: false},
		{name: "empty", raw: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsHandleRef(tt.raw); got != tt.want {
				t.Errorf("IsHandleRef(%q) = %v; want %v", tt.raw, got, tt.want)
			}
		})
	}
}

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

func TestHandleMember(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handle     string
		wantMember string
		wantOK     bool
	}{
		{name: "member handle", handle: "plan:internal/alpha#Renamed", wantMember: "Renamed", wantOK: true},
		{name: "qualified member handle", handle: "plan:internal/alpha#Counter.Tally", wantMember: "Counter.Tally", wantOK: true},
		{name: "no # at all", handle: "plan:internal/alpha", wantOK: false},
		{name: "empty member", handle: "plan:internal/alpha#", wantMember: "", wantOK: true},
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
		})
	}
}

// TestHandleIdentifier ports planglyph's draftHandleIdentifier table verbatim as its assertion
// base (internal/planglyph/handle_test.go's TestDraftHandleIdentifier), per the overview's
// behavior-preservation Shared Decision.
func TestHandleIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		handle string
		want   string
		wantOK bool
	}{
		{handle: "plan:internal/alpha#Renamed", want: "Renamed", wantOK: true},
		{handle: "plan:internal/alpha#Counter.Tally", want: "Tally", wantOK: true},
		{handle: "plan:internal/alpha#", wantOK: false},
		{handle: "plan:internal/alpha", wantOK: false},
	}

	for _, tc := range cases {
		got, ok := HandleIdentifier(tc.handle)
		if ok != tc.wantOK {
			t.Fatalf("HandleIdentifier(%q) ok = %v; want %v", tc.handle, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Errorf("HandleIdentifier(%q) = %q; want %q", tc.handle, got, tc.want)
		}
	}
}

// TestCheckHandleMalformed_FileUnitRuleScopedToDeclarations is R9-5's regression: the file-unit
// rule binds a handle whose unit half is actually READ, which is a Create declaration's, and must
// not bind one claimed ONLY as a Rename pair's to-side.
//
// internal/planglyph's renameDeclSource takes the derived declaration's Unit from the RESOLVED old
// side, deliberately, so a Rename to-side draft that misspells the unit is corrected by
// canonicalization rather than propagated — the rule's own stated consequence cannot arise for it,
// and firing anyway refused a plan that would have canonicalized correctly.
func TestCheckHandleMalformed_FileUnitRuleScopedToDeclarations(t *testing.T) {
	renameOnly := &Plan{
		Language: "go",
		Cards: []Card{{
			Number: 1, Slug: "one",
			Targets: []string{"greeter#Hello", "plan:greeter/farewell.go#Farewell"},
			Pairs:   []MovePair{{Old: "greeter#Hello", New: "plan:greeter/farewell.go#Farewell"}},
		}},
	}
	if got := checkHandleMalformed(renameOnly); len(got) != 0 {
		t.Errorf("checkHandleMalformed(rename-to-side-only file-unit handle) = %+v; want no findings", got)
	}

	alsoDeclared := &Plan{
		Language: "go",
		Cards: []Card{{
			Number: 1, Slug: "one",
			Targets: []string{"greeter#Hello", "plan:greeter/farewell.go#Farewell"},
			Pairs:   []MovePair{{Old: "greeter#Hello", New: "plan:greeter/farewell.go#Farewell"}},
		}, {
			Number: 2, Slug: "two",
			Targets:      []string{"plan:greeter/farewell.go#Farewell"},
			Declarations: []CardDeclaration{{Handle: "plan:greeter/farewell.go#Farewell", Decl: "func Farewell(name string) string"}},
		}},
	}
	if got := checkHandleMalformed(alsoDeclared); len(got) != 2 {
		t.Errorf("checkHandleMalformed(handle claimed by both sources) = %+v; want one finding per referencing card", got)
	}

	dangling := &Plan{
		Language: "go",
		Cards: []Card{{
			Number: 1, Slug: "one",
			Targets: []string{"plan:greeter/farewell.go#Farewell"},
		}},
	}
	if got := checkHandleMalformed(dangling); len(got) != 1 {
		t.Errorf("checkHandleMalformed(unclaimed file-unit handle) = %+v; want the rule to still bind", got)
	}
}
