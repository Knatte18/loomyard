package discussionparser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// addedDecision returns a complete AddedDecision dated 2026-03-04 in a non-UTC zone, so the test
// also pins that the heading date is the UTC day.
func addedDecision() AddedDecision {
	zone := time.FixedZone("late", -5*3600)
	return AddedDecision{
		By:        "parent",
		Title:     "Keep the cache",
		Decision:  "Keep the cache per run.",
		Rationale: "A shared cache races.",
		Date:      time.Date(2026, 3, 3, 22, 30, 0, 0, zone),
	}
}

const wantEntry = "\n### Added after Discussion (parent, 2026-03-04): Keep the cache\n\n" +
	"Decision: Keep the cache per run.\n\nRationale: A shared cache races.\n"

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestAppendDecision_LandsAtEndOfDecisionsSection(t *testing.T) {
	before := "# Record\n\n## Goal\n\nbody.\n\n## Scope\n\nbody.\n\n## Decisions\n\n### First\n\nDecision: a.\n\n"
	after := "## Constraints\n\nbody.\n\n## Auto-mode assumptions\n\nbody.\n\n## Open risks\n\nbody.\n\n## Acceptance criteria\n\nbody.\n"
	decisionPath, supportPath := writeFixture(t, t.TempDir(), before+after, "log")

	findings, err := AppendDecision(decisionPath, supportPath, addedDecision())
	if err != nil || len(findings) != 0 {
		t.Fatalf("AppendDecision() = %v, %v; want no findings and nil error", findings, err)
	}

	// The entry follows the section's last non-blank line; the blank line before the next H2 stays.
	want := strings.TrimSuffix(before, "\n") + wantEntry + "\n" + after
	if got := readFile(t, decisionPath); got != want {
		t.Errorf("record =\n%q\nwant\n%q", got, want)
	}
}

func TestAppendDecision_DecisionsIsLastSection(t *testing.T) {
	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		if h == "## Decisions" {
			continue
		}
		b.WriteString(h + "\n\nbody.\n\n")
	}
	b.WriteString("## Decisions\n\n### First\n\nDecision: a.\n")
	prior := b.String()
	decisionPath, supportPath := writeFixture(t, t.TempDir(), prior, "log")

	findings, err := AppendDecision(decisionPath, supportPath, addedDecision())
	if err != nil || len(findings) != 0 {
		t.Fatalf("AppendDecision() = %v, %v; want no findings and nil error", findings, err)
	}
	if got, want := readFile(t, decisionPath), prior+wantEntry; got != want {
		t.Errorf("record tail =\n%q\nwant\n%q", got, want)
	}
}

func TestAppendDecision_LastSectionWithoutTrailingNewline(t *testing.T) {
	prior := strings.Replace(allSectionsContent(), "## Decisions\n\nbody text.\n\n", "", 1) +
		"## Decisions\n\nlast line"
	decisionPath, supportPath := writeFixture(t, t.TempDir(), prior, "log")

	findings, err := AppendDecision(decisionPath, supportPath, addedDecision())
	if err != nil || len(findings) != 0 {
		t.Fatalf("AppendDecision() = %v, %v; want no findings and nil error", findings, err)
	}
	if got, want := readFile(t, decisionPath), prior+"\n"+wantEntry; got != want {
		t.Errorf("record tail =\n%q\nwant\n%q", got, want)
	}
}

func TestAppendDecision_HeadingInsideFenceIsIgnored(t *testing.T) {
	prior := strings.Replace(allSectionsContent(), "## Decisions\n\nbody text.\n",
		"## Decisions\n\n```\n## Not a heading\n```\n", 1)
	decisionPath, supportPath := writeFixture(t, t.TempDir(), prior, "log")

	if _, err := AppendDecision(decisionPath, supportPath, addedDecision()); err != nil {
		t.Fatalf("AppendDecision() error = %v", err)
	}
	got := readFile(t, decisionPath)
	if fence, entry := strings.Index(got, "## Not a heading"), strings.Index(got, "### Added after"); entry < fence {
		t.Errorf("entry landed before the fenced block ends:\n%s", got)
	}
}

func TestAppendDecision_EmptyFieldWritesNothing(t *testing.T) {
	for name, mutate := range map[string]func(*AddedDecision){
		"by":        func(d *AddedDecision) { d.By = "  " },
		"title":     func(d *AddedDecision) { d.Title = "" },
		"decision":  func(d *AddedDecision) { d.Decision = "\n" },
		"rationale": func(d *AddedDecision) { d.Rationale = "" },
	} {
		t.Run(name, func(t *testing.T) {
			decisionPath, supportPath := writeFixture(t, t.TempDir(), allSectionsContent(), "log")
			d := addedDecision()
			mutate(&d)

			if _, err := AppendDecision(decisionPath, supportPath, d); err == nil {
				t.Fatal("AppendDecision() error = nil; want an error")
			}
			if got := readFile(t, decisionPath); got != allSectionsContent() {
				t.Errorf("record changed:\n%q", got)
			}
		})
	}
}

func TestAppendDecision_MissingDecisionsHeadingWritesNothing(t *testing.T) {
	prior := strings.Replace(allSectionsContent(), "## Decisions", "## Choices", 1)
	decisionPath, supportPath := writeFixture(t, t.TempDir(), prior, "log")

	if _, err := AppendDecision(decisionPath, supportPath, addedDecision()); err == nil {
		t.Fatal("AppendDecision() error = nil; want an error")
	}
	if got := readFile(t, decisionPath); got != prior {
		t.Errorf("record changed:\n%q", got)
	}
}

func TestAppendDecision_MissingRecordIsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := AppendDecision(filepath.Join(dir, "absent.md"), filepath.Join(dir, "log.md"), addedDecision())
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("AppendDecision() error = %v; want one satisfying errors.Is(err, os.ErrNotExist)", err)
	}
}

func TestAppendDecision_FindingRestoresRecord(t *testing.T) {
	prior := strings.Replace(allSectionsContent(), "## Goal", "## Aim", 1)
	decisionPath, supportPath := writeFixture(t, t.TempDir(), prior, "log")

	findings, err := AppendDecision(decisionPath, supportPath, addedDecision())
	if err != nil {
		t.Fatalf("AppendDecision() error = %v; want nil", err)
	}
	if len(findings) != 1 || findings[0].Check != checkSectionMissing {
		t.Errorf("findings = %v; want one %s finding", findings, checkSectionMissing)
	}
	if got := readFile(t, decisionPath); got != prior {
		t.Errorf("record not restored:\n%q", got)
	}
}

func TestAppendDecision_SupportLogUntouched(t *testing.T) {
	decisionPath, supportPath := writeFixture(t, t.TempDir(), allSectionsContent(), "log")

	if _, err := AppendDecision(decisionPath, supportPath, addedDecision()); err != nil {
		t.Fatalf("AppendDecision() error = %v", err)
	}
	if got := readFile(t, supportPath); got != "log" {
		t.Errorf("support log = %q; want it unchanged", got)
	}
}
