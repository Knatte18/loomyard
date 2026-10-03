package plankit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestRender_ParsesAndValidates(t *testing.T) {
	create := func(n int, slug, path string) Card {
		return Card{Number: n, Slug: slug, Groups: []Group{{Label: "Create", Targets: []string{path}}}}
	}
	tests := []struct {
		name string
		plan Plan
	}{
		{"single card", Plan{Approved: true, Cards: []Card{create(1, "alpha", "a.go")}}},
		{"unapproved", Plan{Cards: []Card{create(1, "alpha", "a.go")}}},
		{"multi card", Plan{Approved: true, Framing: "Two cards.", Cards: []Card{create(1, "alpha", "a.go"), create(2, "beta", "b.go")}}},
		{"first card offset", Plan{Approved: true, FirstCard: 3, Cards: []Card{create(3, "gamma", "c.go"), create(4, "delta", "d.go")}}},
		{"multi label with uses", Plan{
			Approved: true,
			Language: "go",
			Sections: []Section{{Heading: "verify:", Body: "go build ./..."}},
			Cards: []Card{
				create(1, "alpha", "a.go"),
				{
					Number: 2,
					Slug:   "beta",
					Groups: []Group{
						{Label: "Create", Targets: []string{"b.go"}},
						{Label: "Edit", Targets: []string{"e.go"}},
					},
					Uses:          []string{"a.go"},
					Intent:        "Add b.go and touch e.go.",
					ImpactSummary: "Adds b.go and touches e.go.",
					Commit:        "2: beta",
				},
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := Repo(t, map[string]string{"a.go": "package a\n", "e.go": "package e\n"})
			dir := filepath.Join(root, "_lyx", "plan")
			Write(t, dir, tt.plan)

			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan returned error: %v", err)
			}
			if got := planparser.ValidateFormat(plan, root); len(got) != 0 {
				t.Fatalf("ValidateFormat findings = %v, want none", got)
			}
			if plan.Approved != tt.plan.Approved {
				t.Fatalf("Approved = %t, want %t", plan.Approved, tt.plan.Approved)
			}
			if plan.Format != planparser.RecognizedFormat {
				t.Fatalf("Format = %d, want %d", plan.Format, planparser.RecognizedFormat)
			}
			if len(plan.Cards) != len(tt.plan.Cards) {
				t.Fatalf("parsed %d cards, want %d", len(plan.Cards), len(tt.plan.Cards))
			}
			if tt.plan.FirstCard != 0 && plan.FirstCard != tt.plan.FirstCard {
				t.Fatalf("FirstCard = %d, want %d", plan.FirstCard, tt.plan.FirstCard)
			}
		})
	}
}

func TestRender_ApprovedGatesValidate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plan")
	Write(t, dir, Plan{Cards: []Card{{Number: 1, Slug: "alpha", Groups: []Group{{Label: "Create", Targets: []string{"a.go"}}}}}})

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan returned error: %v", err)
	}
	if got := planparser.Validate(plan, root); len(got) != 1 || got[0].Check != "plan-unapproved" {
		t.Fatalf("Validate findings = %v, want only plan-unapproved", got)
	}
}

func TestRender_BytesCanBeCorrupted(t *testing.T) {
	root := t.TempDir()
	files := Render(Plan{Approved: true, Cards: []Card{{Number: 1, Slug: "alpha", Groups: []Group{{Label: "Create", Targets: []string{"a.go"}}}}}})
	files["01-alpha.md"] = []byte(strings.Replace(string(files["01-alpha.md"]), "**Create:**", "**Frobnicate:**", 1))
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", name, err)
		}
	}

	plan, err := planparser.ParsePlan(root)
	if err != nil {
		t.Fatalf("ParsePlan returned error: %v", err)
	}
	if got := planparser.ValidateFormat(plan, root); len(got) == 0 {
		t.Fatal("ValidateFormat found nothing in a corrupted card")
	}
}

func TestRepo_WritesNestedFiles(t *testing.T) {
	root := Repo(t, map[string]string{"sub/dir/a.go": "package dir\n", "b.go": "package b\n"})
	for rel, want := range map[string]string{"sub/dir/a.go": "package dir\n", "b.go": "package b\n"} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("ReadFile(%q) failed: %v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", rel, got, want)
		}
	}
}
