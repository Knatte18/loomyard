//go:build tier0

// parse_test.go tests Parse over a full parts file and the errors it names, plus the model reads External and SpotPaths.

package partsparser

import (
	"reflect"
	"strings"
	"testing"
)

// validParts is a parts file of two parts and two shared spots that Parse reads and Check passes for the decisions D1 to D3.
const validParts = `# Parts: Add widget

## Split note

The task splits in two.

## Part: core

- Decisions: D1
- Takes in: the widget request
- Delivers: the widget type
- Exports:
  - ` + "`plan:internal/x#NewThing`" + `
  - ` + "`internal/x/newfile.go`" + `
- Area:
  - ` + "`internal/x`" + `

## Part: wiring

- Decisions: D2, D3
- Takes in: the NewThing type
  and the registry row
- Delivers: the wiring
- Depends on: core
- Area:
  - ` + "`internal/y`" + `
  - ` + "`internal/reg`" + `

## Shared spot: ` + "`internal/reg/reg.go`" + `

- Decisions: D3
- Needed by: core, wiring
- Needs: a registry row
- Exports:
  - ` + "`plan:internal/reg#Registered`" + `
- Uses:
  - ` + "`plan:internal/x#NewThing`" + `

## Shared spot: ` + "`cmd/main.go`" + `

- Needed by: wiring
- Needs: the wire
`

func mustParse(t *testing.T, text string) *Parts {
	t.Helper()
	parts, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("Parse() = _, %v; want nil error", err)
	}
	return parts
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("a full file is read into the model", func(t *testing.T) {
		t.Parallel()
		want := &Parts{
			Title:     "Add widget",
			SplitNote: "The task splits in two.",
			Parts: []Part{
				{
					Name: "core", Decisions: []string{"D1"}, TakesIn: "the widget request", Delivers: "the widget type",
					Exports: []string{"plan:internal/x#NewThing", "internal/x/newfile.go"}, Area: []string{"internal/x"},
				},
				{
					Name: "wiring", Decisions: []string{"D2", "D3"}, TakesIn: "the NewThing type\nand the registry row", Delivers: "the wiring",
					DependsOn: []string{"core"}, Area: []string{"internal/y", "internal/reg"},
				},
			},
			Spots: []SharedSpot{
				{
					Path: "internal/reg/reg.go", Decisions: []string{"D3"}, NeededBy: []string{"core", "wiring"}, Needs: "a registry row",
					Exports: []string{"plan:internal/reg#Registered"}, Uses: []string{"plan:internal/x#NewThing"},
				},
				{Path: "cmd/main.go", NeededBy: []string{"wiring"}, Needs: "the wire"},
			},
		}
		got := mustParse(t, validParts)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse() = %+v; want %+v", got, want)
		}

		if part, ok := got.Part("wiring"); !ok || part.Name != "wiring" {
			t.Errorf("Part(wiring) = %+v, %v; want the wiring part", part, ok)
		}
		if _, ok := got.Part("ghost"); ok {
			t.Error("Part(ghost) ok = true; want false")
		}
		if got, want := got.SpotPaths(), []string{"internal/reg/reg.go", "cmd/main.go"}; !reflect.DeepEqual(got, want) {
			t.Errorf("SpotPaths() = %v; want %v", got, want)
		}
		parts := mustParse(t, validParts)
		wantExternal := map[string][]string{
			"core":   {"plan:internal/reg#Registered"},
			"wiring": {"plan:internal/x#NewThing", "internal/x/newfile.go", "plan:internal/reg#Registered"},
			"ghost":  nil,
		}
		for name, want := range wantExternal {
			if got := parts.External(name); !reflect.DeepEqual(got, want) {
				t.Errorf("External(%q) = %v; want %v", name, got, want)
			}
		}
	})

	t.Run("a fenced line is read as text", func(t *testing.T) {
		t.Parallel()
		text := strings.Replace(validParts, "The task splits in two.", "Split like this:\n```\n## Part: not-a-part\n```", 1)
		parts := mustParse(t, text)
		if len(parts.Parts) != 2 || !strings.Contains(parts.SplitNote, "## Part: not-a-part") {
			t.Errorf("Parse() parts = %d, split note = %q; want the fenced heading kept in the note", len(parts.Parts), parts.SplitNote)
		}
	})

	errorRows := []struct {
		name     string
		mutate   func(string) string
		wantLine string
		wantIn   string
	}{
		{"missing H1", func(s string) string { return strings.Replace(s, "# Parts: Add widget", "Add widget", 1) }, "line 1:", "does not open with"},
		{"missing split note", func(s string) string {
			return strings.Replace(s, "## Split note\n\nThe task splits in two.\n\n", "", 1)
		}, "line 3:", "is not `## Split note`"},
		{"empty split note", func(s string) string { return strings.Replace(s, "The task splits in two.", "", 1) }, "line 3:", "split note is empty"},
		{"unknown section heading", func(s string) string { return strings.Replace(s, "## Part: wiring", "## Chapter: wiring", 1) }, "line 18:", "unknown or misplaced section heading"},
		{"unknown field label", func(s string) string {
			return strings.Replace(s, "- Delivers: the widget type", "- Gives: the widget type", 1)
		}, "line 11:", "unknown field label"},
		{"required field missing", func(s string) string { return strings.Replace(s, "- Delivers: the widget type\n", "", 1) }, "line 7:", `lacks the required field "Delivers"`},
		{"required field empty", func(s string) string { return strings.Replace(s, "- Delivers: the widget type", "- Delivers:", 1) }, "line 11:", `field "Delivers" is empty`},
		{"ref sub-bullet not backticked", func(s string) string { return strings.Replace(s, "  - `internal/x`\n", "  - internal/x\n", 1) }, "line 16:", "not one backticked ref"},
		{"field repeated", func(s string) string {
			return strings.Replace(s, "- Decisions: D1\n", "- Decisions: D1\n- Decisions: D9\n", 1)
		}, "line 10:", `field "Decisions" is repeated`},
		{"part after a spot", func(s string) string { return s + "\n## Part: late\n" }, "line 44:", "follows a shared spot"},
		{"spot without its required Needs", func(s string) string { return strings.Replace(s, "- Needs: the wire\n", "", 1) }, "line 39:", `lacks the required field "Needs"`},
	}
	for _, tt := range errorRows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tt.mutate(validParts)))
			if err == nil {
				t.Fatal("Parse() error = nil; want a refusal")
			}
			for _, want := range []string{tt.wantLine, tt.wantIn, "way forward:", "contracts/specs/loom-parts-spec.md"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Parse() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}
