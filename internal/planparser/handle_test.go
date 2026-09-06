// handle_test.go covers splitHandleDeclaration and handleUnit, the two pure string helpers card 9
// adds, plus the declaredHandles/referencedHandles predicates card 10 adds beside them.

package planparser

import (
	"slices"
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
