// Package plankit is the one test-side writer of valid plans.
// It renders a plan directory through `planparser`'s format constant and its Card Index and card-file grammar, so a format bump fails at one site instead of in every fixture.
// Render returns the bytes per file, so a rejection test can corrupt one file of an otherwise valid plan without spelling the format itself.
// WriteTree and Repo write the file-tree fixture glyph resolution needs.
// Its only assertions are `t.Fatalf` on its own setup.
package plankit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

const overviewFile = "00-overview.md"

// Plan describes one plan directory.
type Plan struct {
	Approved bool
	// Framing is the prose between the title and the Card Index.
	Framing string
	// FirstCard renders `first_card:` when non-zero, and is the number of the first card.
	FirstCard int
	// Root and Language render `root:` and `language:` when non-empty.
	Root     string
	Language string
	// Sections are the plan-level `## <Heading>` sections written after the Card Index, in order.
	Sections []Section
	Cards    []Card
}

// Section is one plan-level section of the overview, such as `verify:` or `Shared Decisions`.
type Section struct {
	Heading string
	Body    string
}

// Card describes one card file and its Card Index line.
type Card struct {
	Number int
	Slug   string
	// Summary is the Card Index line's intent text; it defaults to the slug.
	Summary string
	// Groups render one bold type label each, in order.
	Groups []Group
	Uses   []string
	// Intent defaults to the Summary.
	Intent        string
	ImpactSummary string
	Commit        string
}

// Group is one type label, without its colon, and its target bullets.
// Each target is written inside backticks, except under `Rename`, whose targets are written verbatim as complete "`old` -> `new`" lines.
type Group struct {
	Label   string
	Targets []string
}

// Render returns the plan's files keyed by name: the overview and one card file per card.
func Render(p Plan) map[string][]byte {
	files := map[string][]byte{overviewFile: []byte(renderOverview(p))}
	for _, c := range p.Cards {
		files[fmt.Sprintf("%02d-%s.md", c.Number, c.Slug)] = []byte(renderCard(c))
	}
	return files
}

// Write renders p into dir, creating it when absent.
func Write(t *testing.T, dir string, p Plan) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", dir, err)
	}
	for name, data := range Render(p) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", name, err)
		}
	}
}

// WriteTree writes files, keyed by slash-separated path relative to root, under root.
func WriteTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
}

// Repo writes files under a fresh t.TempDir() and returns its absolute path.
func Repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	WriteTree(t, root, files)
	return root
}

func renderOverview(p Plan) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "format: %d\n", planparser.RecognizedFormat)
	fmt.Fprintf(&b, "approved: %t\n", p.Approved)
	if p.FirstCard != 0 {
		fmt.Fprintf(&b, "first_card: %d\n", p.FirstCard)
	}
	if p.Root != "" {
		fmt.Fprintf(&b, "root: %s\n", p.Root)
	}
	if p.Language != "" {
		fmt.Fprintf(&b, "language: %s\n", p.Language)
	}
	b.WriteString("---\n\n# Plan: test plan\n\n")
	if p.Framing != "" {
		b.WriteString(p.Framing + "\n\n")
	}
	b.WriteString("## Card Index\n\n")
	for _, c := range p.Cards {
		fmt.Fprintf(&b, "%d — %s — %s\n", c.Number, c.Slug, summary(c))
	}
	for _, s := range p.Sections {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", s.Heading, strings.TrimSpace(s.Body))
	}
	return b.String()
}

func renderCard(c Card) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Card %d — %s\n", c.Number, c.Slug)
	for _, g := range c.Groups {
		fmt.Fprintf(&b, "\n**%s:**\n", g.Label)
		for _, target := range g.Targets {
			if g.Label == "Rename" {
				fmt.Fprintf(&b, "- %s\n", target)
				continue
			}
			fmt.Fprintf(&b, "- `%s`\n", target)
		}
	}
	if len(c.Uses) > 0 {
		b.WriteString("\n**Uses:**\n")
		for _, u := range c.Uses {
			fmt.Fprintf(&b, "- `%s`\n", u)
		}
	}
	intent := c.Intent
	if intent == "" {
		intent = summary(c)
	}
	fmt.Fprintf(&b, "\n**Intent:** %s\n", intent)
	if c.ImpactSummary != "" {
		fmt.Fprintf(&b, "\n**ImpactSummary:** %s\n", c.ImpactSummary)
	}
	if c.Commit != "" {
		fmt.Fprintf(&b, "\n**Commit:** `%s`\n", c.Commit)
	}
	return b.String()
}

func summary(c Card) string {
	if c.Summary != "" {
		return c.Summary
	}
	return c.Slug
}
