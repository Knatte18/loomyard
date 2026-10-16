package discussionparser

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// patternChangesSection returns a `## PATTERN changes` section holding the given lines, closed by a blank line.
func patternChangesSection(lines ...string) string {
	return patternChangesHeading + "\n\n" + strings.Join(lines, "\n") + "\n\n"
}

// TestValidate_PatternChanges asserts each defect of the `## PATTERN changes` section yields exactly one pattern-change-malformed finding at the decision record's path, naming the line.
func TestValidate_PatternChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		section string
		wantLn  string
	}{
		{"UnknownAction", patternChangesSection("- `PATTERN-a` — renames: x"), "unknown action"},
		{"AddsWithoutText", patternChangesSection("- `PATTERN-a` — adds"), "needs its new text"},
		{"ChangesWithoutText", patternChangesSection("- `PATTERN-a` — changes:"), "needs its new text"},
		{"RemovesWithText", patternChangesSection("- `PATTERN-a` — removes: x"), "takes no text"},
		{"StrayLine", patternChangesSection("some prose"), "not a bullet"},
		{"IDOutsideCharset", patternChangesSection("- `PATTERN-Foo_bar` — removes"), "not a bullet"},
		{"RepeatedID", patternChangesSection("- `PATTERN-a` — removes", "- `PATTERN-a` — adds: x"), "second bullet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decisionPath, supportPath := writeFixture(t, t.TempDir(), allSectionsContent()+tt.section, "log")

			findings, err := Validate(decisionPath, supportPath)
			if err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
			}
			if len(findings) != 1 {
				t.Fatalf("Validate() findings = %v; want exactly one", findings)
			}
			f := findings[0]
			if f.Check != checkPatternChangeMalformed {
				t.Errorf("Check = %q; want %q", f.Check, checkPatternChangeMalformed)
			}
			if f.Path != decisionPath {
				t.Errorf("Path = %q; want %q", f.Path, decisionPath)
			}
			if !strings.Contains(f.Detail, "line ") || !strings.Contains(f.Detail, tt.wantLn) {
				t.Errorf("Detail = %q; want it to name the line and contain %q", f.Detail, tt.wantLn)
			}
		})
	}
}

// TestDeclaredPatternChanges asserts the rendered bullets of a clean section, nil for a missing path, file or section, and an ErrPatternChangesInvalid refusal naming the validate verb for a malformed one.
func TestDeclaredPatternChanges(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, content string) string {
		t.Helper()
		path, _ := writeFixture(t, t.TempDir(), content, "log")
		return path
	}

	t.Run("NothingDeclared", func(t *testing.T) {
		t.Parallel()
		for name, path := range map[string]string{
			"EmptyPath":     "",
			"AbsentFile":    filepath.Join(t.TempDir(), "missing.md"),
			"NoSection":     write(t, allSectionsContent()),
			"EmptySection":  write(t, allSectionsContent()+patternChangesSection()),
			"SectionInHere": write(t, "```\n"+patternChangesHeading+"\n- junk\n```\n"),
		} {
			got, err := DeclaredPatternChanges(path)
			if err != nil || got != nil {
				t.Errorf("%s: DeclaredPatternChanges() = %v, %v; want nil, nil", name, got, err)
			}
		}
	})

	t.Run("CleanSection", func(t *testing.T) {
		t.Parallel()
		path := write(t, allSectionsContent()+patternChangesSection(
			"- `PATTERN-a` — adds: New rule.",
			"- `PATTERN-b` — removes — added after Discussion (parent, 2026-10-16)",
		))
		got, err := DeclaredPatternChanges(path)
		if err != nil {
			t.Fatalf("DeclaredPatternChanges() error = %v; want nil", err)
		}
		want := []string{"- `PATTERN-a` — adds: New rule.", "- `PATTERN-b` — removes"}
		if !slices.Equal(got, want) {
			t.Errorf("DeclaredPatternChanges() = %q; want %q", got, want)
		}
	})

	t.Run("MalformedSection", func(t *testing.T) {
		t.Parallel()
		path := write(t, allSectionsContent()+patternChangesSection("- `PATTERN-a` — adds"))
		got, err := DeclaredPatternChanges(path)
		if !errors.Is(err, ErrPatternChangesInvalid) {
			t.Fatalf("DeclaredPatternChanges() error = %v; want it to wrap ErrPatternChangesInvalid", err)
		}
		if got != nil {
			t.Errorf("DeclaredPatternChanges() = %q; want nil alongside the error", got)
		}
		for _, want := range []string{path, patternChangesHeading, "lyx loom validate-discussion"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q; want it to contain %q", err, want)
			}
		}
	})
}

// sectionsWithout returns a decision record carrying every required heading except the given ones.
func sectionsWithout(skip ...string) string {
	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		if slices.Contains(skip, h) {
			continue
		}
		b.WriteString(h)
		b.WriteString("\n\nbody text.\n\n")
	}
	return b.String()
}

// TestValidate_CleanRecords asserts a record carrying every required heading yields no finding, whatever else the file holds: trailing whitespace on a heading, headings in any order, an extra heading, the optional notes section absent, and a directory standing in for the support log.
//
//testtiming:keep pins that a record with every required heading yields no finding across whitespace, order, extra-heading and support-directory variants, which its covering test does not assert
func TestValidate_CleanRecords(t *testing.T) {
	t.Parallel()

	reversed := slices.Clone(requiredDiscussionSections)
	slices.Reverse(reversed)
	var outOfOrder strings.Builder
	for _, h := range reversed {
		outOfOrder.WriteString(h)
		outOfOrder.WriteString("\n\nbody text.\n\n")
	}

	var padded strings.Builder
	for _, h := range requiredDiscussionSections {
		padded.WriteString(h + "  \t \r")
		padded.WriteString("\n\nbody text.\n\n")
	}

	tests := []struct {
		name         string
		content      string
		supportIsDir bool
	}{
		{name: "all sections present", content: allSectionsContent()},
		{name: "heading with trailing whitespace still counts", content: padded.String()},
		{name: "headings out of order are not validated", content: outOfOrder.String()},
		{name: "extra unexpected heading", content: allSectionsContent() + "## Some Extra Heading\n\nextra body.\n"},
		{name: "PATTERN changes section of every action", content: allSectionsContent() + patternChangesSection(
			"- `PATTERN-cwd-resolution` — adds: Resolve a cwd only in `internal/lyxcwd`.",
			"- `PATTERN-hub-suffix` — changes: `-LYXHUB` is the sole suffix.",
			"- `PATTERN-old-rule` — removes",
			"- `PATTERN-late-rule` — changes: Reworded. — added after Discussion (parent, 2026-10-16)",
			"- `PATTERN-late-gone` — removes — added after Discussion (arbiter, 2026-10-16)",
		) + "## Notes for the plan writer\n\nhint.\n"},
		// os.Stat accepts a directory as "exists", so the support-log check is exhaustively "both files exist"; an is-regular-file test would be a new check.
		{name: "support log path is a directory", content: allSectionsContent(), supportIsDir: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			decisionPath, supportPath := writeFixture(t, dir, tt.content, "log")
			if tt.supportIsDir {
				supportPath = filepath.Join(dir, "support-dir")
				if err := os.Mkdir(supportPath, 0o755); err != nil {
					t.Fatalf("mkdir support log: %v", err)
				}
			}

			findings, err := Validate(decisionPath, supportPath)
			if err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
			}
			if len(findings) != 0 {
				t.Errorf("Validate() findings = %v; want none", findings)
			}
		})
	}
}

// TestValidate_MissingHeadings asserts one section-missing finding per absent required heading, each at the decision record's path and naming its heading, in heading order.
//
//testtiming:keep pins the section-missing finding per absent heading with its path and detail, which its covering test does not assert
func TestValidate_MissingHeadings(t *testing.T) {
	t.Parallel()

	fenced := requiredDiscussionSections[0]
	midSentence := requiredDiscussionSections[1]
	var hidden strings.Builder
	for _, h := range requiredDiscussionSections {
		switch h {
		case fenced:
			// Indented by two leading spaces, as a fenced example block quoting the heading
			// format typically would be -- missingSections right-trims but never left-trims, so
			// this line is not an exact match for h and must not count as present.
			hidden.WriteString("```\n  " + h + "\n```\n\n")
		case midSentence:
			hidden.WriteString("See " + h + " above for details.\n\n")
		default:
			hidden.WriteString(h + "\n\nbody text.\n\n")
		}
	}

	several := []string{requiredDiscussionSections[0], requiredDiscussionSections[2], requiredDiscussionSections[5]}
	tests := []struct {
		name        string
		content     string
		wantMissing []string
	}{
		{name: "several at once", content: sectionsWithout(several...), wantMissing: several},
		{name: "in a fence or mid-sentence", content: hidden.String(), wantMissing: []string{fenced, midSentence}},
	}
	for _, missing := range requiredDiscussionSections {
		tests = append(tests, struct {
			name        string
			content     string
			wantMissing []string
		}{name: "only " + missing, content: sectionsWithout(missing), wantMissing: []string{missing}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decisionPath, supportPath := writeFixture(t, t.TempDir(), tt.content, "log")

			findings, err := Validate(decisionPath, supportPath)
			if err != nil {
				t.Fatalf("Validate() error = %v; want nil", err)
			}
			if len(findings) != len(tt.wantMissing) {
				t.Fatalf("Validate() findings = %v; want %d, one per missing heading", findings, len(tt.wantMissing))
			}
			for i, f := range findings {
				if f.Check != checkSectionMissing {
					t.Errorf("findings[%d].Check = %q; want %q", i, f.Check, checkSectionMissing)
				}
				if f.Path != decisionPath {
					t.Errorf("findings[%d].Path = %q; want %q", i, f.Path, decisionPath)
				}
				if !strings.Contains(f.Detail, tt.wantMissing[i]) {
					t.Errorf("findings[%d].Detail = %q; want it to name %q", i, f.Detail, tt.wantMissing[i])
				}
			}
		})
	}
}

func TestValidate_DecisionRecordAbsent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestValidate_DecisionRecordPathIsDirectory(t *testing.T) {
	t.Parallel()
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

// TestMissingSections_ASingleHugeLineDoesNotHideEveryHeadingBelowIt is R6-28's regression test.
// A bufio.Scanner stops at the first line over bufio.MaxScanTokenSize (64 KB) and reports it only through scanner.Err(), which was never checked — so one pasted base64 blob or minified snippet, entirely ordinary in an agent-written discussion document, made every heading below it report missing. loomshed's Discussion-Write and Discussion-Burler-Fix gates map that to a re-prompt against the still-live session, holding the handoff until the gate passes or its attempt budget escalates to a human.
//
//testtiming:keep pins the R6-28 regression that a line over 64 KB does not hide the headings below it, which its covering test does not assert
func TestMissingSections_ASingleHugeLineDoesNotHideEveryHeadingBelowIt(t *testing.T) {
	t.Parallel()

	required := []string{"## First", "## Second"}
	huge := strings.Repeat("A", 128*1024)
	content := "## First\n" + huge + "\n## Second\n"

	if got := missingSections(content, required); len(got) != 0 {
		t.Errorf("missingSections(...) = %v; want none — every required heading is present, one just sits below a 128 KB line", got)
	}
}
