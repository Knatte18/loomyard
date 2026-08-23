package discussionparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allSectionsContent returns a decision record body carrying every heading in
// requiredDiscussionSections, in order, each with a one-line body.
func allSectionsContent() string {
	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		b.WriteString(h)
		b.WriteString("\n\nbody text.\n\n")
	}
	return b.String()
}

// writeFixture writes content to decisionRecordPath and supportLogPath under dir, creating both
// as regular files, and returns their absolute paths.
func writeFixture(t *testing.T, dir, decisionContent, supportContent string) (string, string) {
	t.Helper()
	decisionPath := filepath.Join(dir, "decision-record.md")
	supportPath := filepath.Join(dir, "support-log.md")
	if err := os.WriteFile(decisionPath, []byte(decisionContent), 0o644); err != nil {
		t.Fatalf("write decision record: %v", err)
	}
	if err := os.WriteFile(supportPath, []byte(supportContent), 0o644); err != nil {
		t.Fatalf("write support log: %v", err)
	}
	return decisionPath, supportPath
}

func TestValidate_AllSectionsPresent(t *testing.T) {
	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, allSectionsContent(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none", findings)
	}
}

func TestValidate_EachHeadingMissingIndividually(t *testing.T) {
	for _, missing := range requiredDiscussionSections {
		missing := missing
		t.Run(missing, func(t *testing.T) {
			var b strings.Builder
			for _, h := range requiredDiscussionSections {
				if h == missing {
					continue
				}
				b.WriteString(h)
				b.WriteString("\n\nbody text.\n\n")
			}

			dir := t.TempDir()
			decisionPath, supportPath := writeFixture(t, dir, b.String(), "log")

			findings, err := Validate(decisionPath, supportPath)
			if err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
			}
			if len(findings) != 1 {
				t.Fatalf("Validate() findings = %v; want exactly one finding", findings)
			}
			if findings[0].Check != checkSectionMissing {
				t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, checkSectionMissing)
			}
			if findings[0].Path != decisionPath {
				t.Errorf("findings[0].Path = %q; want %q", findings[0].Path, decisionPath)
			}
			if !strings.Contains(findings[0].Detail, missing) {
				t.Errorf("findings[0].Detail = %q; want it to name %q", findings[0].Detail, missing)
			}
		})
	}
}

func TestValidate_SeveralHeadingsMissingAtOnce(t *testing.T) {
	skip := map[string]bool{
		requiredDiscussionSections[0]: true,
		requiredDiscussionSections[2]: true,
		requiredDiscussionSections[5]: true,
	}

	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		if skip[h] {
			continue
		}
		b.WriteString(h)
		b.WriteString("\n\nbody text.\n\n")
	}

	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, b.String(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != len(skip) {
		t.Fatalf("Validate() findings = %v; want %d findings, one per missing heading", findings, len(skip))
	}
	for _, f := range findings {
		if f.Check != checkSectionMissing {
			t.Errorf("finding.Check = %q; want %q", f.Check, checkSectionMissing)
		}
	}
}

func TestValidate_DecisionRecordAbsent(t *testing.T) {
	dir := t.TempDir()
	decisionPath := filepath.Join(dir, "decision-record.md")
	supportPath := filepath.Join(dir, "support-log.md")
	if err := os.WriteFile(supportPath, []byte("log"), 0o644); err != nil {
		t.Fatalf("write support log: %v", err)
	}

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("Validate() findings = %v; want exactly one finding", findings)
	}
	if findings[0].Check != checkFileMissing {
		t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, checkFileMissing)
	}
	if findings[0].Path != decisionPath {
		t.Errorf("findings[0].Path = %q; want %q", findings[0].Path, decisionPath)
	}
}

func TestValidate_SupportLogAbsent(t *testing.T) {
	dir := t.TempDir()
	supportPath := filepath.Join(dir, "support-log.md")
	// decisionPath is deliberately a directory, not a file: if Validate ever read it despite the
	// support log being absent, os.ReadFile would return a non-not-exist error, and the test below
	// asserts a finding with a nil error instead -- proving the decision record is never read once
	// the support log's absence short-circuits.
	decisionPath := filepath.Join(dir, "decision-record.md")
	if err := os.Mkdir(decisionPath, 0o755); err != nil {
		t.Fatalf("mkdir decision record: %v", err)
	}

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil (decision record must never be read)", err)
	}
	if len(findings) != 1 {
		t.Fatalf("Validate() findings = %v; want exactly one finding", findings)
	}
	if findings[0].Check != checkFileMissing {
		t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, checkFileMissing)
	}
	if findings[0].Path != supportPath {
		t.Errorf("findings[0].Path = %q; want %q", findings[0].Path, supportPath)
	}
}

func TestValidate_HeadingWithTrailingWhitespaceStillCounts(t *testing.T) {
	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		b.WriteString(h + "  \t \r")
		b.WriteString("\n\nbody text.\n\n")
	}

	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, b.String(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none", findings)
	}
}

func TestValidate_HeadingInFenceOrMidSentenceDoesNotCount(t *testing.T) {
	fenced := requiredDiscussionSections[0]
	midSentence := requiredDiscussionSections[1]

	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		switch h {
		case fenced:
			// Indented by two leading spaces, as a fenced example block quoting the heading
			// format typically would be -- missingSections right-trims but never left-trims, so
			// this line is not an exact match for h and must not count as present.
			b.WriteString("```\n  ")
			b.WriteString(h)
			b.WriteString("\n```\n\n")
		case midSentence:
			b.WriteString("See " + h + " above for details.\n\n")
		default:
			b.WriteString(h)
			b.WriteString("\n\nbody text.\n\n")
		}
	}

	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, b.String(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 2 {
		t.Fatalf("Validate() findings = %v; want exactly two findings (fenced + mid-sentence)", findings)
	}
}

func TestValidate_NotesForThePlanWriterAbsentIsNotAFinding(t *testing.T) {
	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, allSectionsContent(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none (## Notes for the plan writer is optional)", findings)
	}
}

func TestValidate_HeadingsOutOfOrderIsNotAFinding(t *testing.T) {
	reversed := make([]string, len(requiredDiscussionSections))
	copy(reversed, requiredDiscussionSections)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	var b strings.Builder
	for _, h := range reversed {
		b.WriteString(h)
		b.WriteString("\n\nbody text.\n\n")
	}

	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, b.String(), "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none (section order is not validated)", findings)
	}
}

func TestValidate_ExtraUnexpectedHeadingIsNotAFinding(t *testing.T) {
	content := allSectionsContent() + "## Some Extra Heading\n\nextra body.\n"

	dir := t.TempDir()
	decisionPath, supportPath := writeFixture(t, dir, content, "log")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none (an extra heading is not a violation)", findings)
	}
}

func TestValidate_DecisionRecordPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	decisionPath := filepath.Join(dir, "decision-record.md")
	if err := os.Mkdir(decisionPath, 0o755); err != nil {
		t.Fatalf("mkdir decision record: %v", err)
	}
	supportPath := filepath.Join(dir, "support-log.md")
	if err := os.WriteFile(supportPath, []byte("log"), 0o644); err != nil {
		t.Fatalf("write support log: %v", err)
	}

	findings, err := Validate(decisionPath, supportPath)
	if err == nil {
		t.Fatal("Validate() error = nil; want a returned error (decision record path is a directory)")
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want an empty slice alongside the error", findings)
	}
}

// TestValidate_SupportLogPathIsDirectory preserves today's behaviour deliberately rather than
// tightening it: os.Stat accepts a directory as "exists", so the support-log check is exhaustively
// "both files exist" -- nothing more. An is-regular-file test would be a new check smuggled in
// under an extraction, which this batch does not do.
func TestValidate_SupportLogPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	supportPath := filepath.Join(dir, "support-log")
	if err := os.Mkdir(supportPath, 0o755); err != nil {
		t.Fatalf("mkdir support log: %v", err)
	}
	decisionPath, _ := writeFixture(t, dir, allSectionsContent(), "unused")

	findings, err := Validate(decisionPath, supportPath)
	if err != nil {
		t.Fatalf("Validate() error = %v; want nil (a directory counts as present)", err)
	}
	if len(findings) != 0 {
		t.Errorf("Validate() findings = %v; want none", findings)
	}
}
