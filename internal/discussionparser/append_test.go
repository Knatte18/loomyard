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

// wantBullet is the PATTERN changes bullet addedDecision's author and day produce for a `PATTERN-new` adds change.
const wantBullet = "- `PATTERN-new` — adds: New rule. — added after Discussion (parent, 2026-03-04)"

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestAppendDecision_AppendsAtEndOfDecisionsSection asserts the entry lands after the Decisions section's last non-blank line, ignoring a heading inside a fence, and that the support log and the rest of the record stay byte-identical.
func TestAppendDecision_AppendsAtEndOfDecisionsSection(t *testing.T) {
	t.Parallel()

	middleBefore := "# Record\n\n## Goal\n\nbody.\n\n## Scope\n\nbody.\n\n## Decisions\n\n### First\n\nDecision: a.\n\n"
	middleAfter := "## Constraints\n\nbody.\n\n## Auto-mode assumptions\n\nbody.\n\n## Open risks\n\nbody.\n\n## Acceptance criteria\n\nbody.\n"

	var decisionsLast strings.Builder
	for _, h := range requiredDiscussionSections {
		if h == "## Decisions" {
			continue
		}
		decisionsLast.WriteString(h + "\n\nbody.\n\n")
	}
	decisionsLast.WriteString("## Decisions\n\n### First\n\nDecision: a.\n")

	withoutDecisions := strings.Replace(allSectionsContent(), "## Decisions\n\nbody text.\n\n", "", 1)
	unterminated := withoutDecisions + "## Decisions\n\nlast line"
	fenced := strings.Replace(allSectionsContent(), "## Decisions\n\nbody text.\n",
		"## Decisions\n\n```\n## Not a heading\n```\n", 1)

	// withEntry returns record with wantEntry placed after its Decisions section's body, where AppendDecision puts it.
	withEntry := func(record string) string {
		return strings.Replace(record, "## Decisions\n\nbody text.\n", "## Decisions\n\nbody text.\n"+wantEntry, 1)
	}
	pattern := &PatternChange{ID: "PATTERN-new", Action: "adds", Text: "New rule."}
	notes := "## Notes for the plan writer\n\nhint.\n"
	withSection := allSectionsContent() + "## PATTERN changes\n\n- `PATTERN-old` — removes\n\n" + notes

	tests := []struct {
		name          string
		prior         string
		relevant      string
		patternChange *PatternChange
		want          string
	}{
		{
			name:          "PATTERN change without the section goes before the notes",
			prior:         allSectionsContent() + notes,
			patternChange: pattern,
			want:          withEntry(allSectionsContent()) + "## PATTERN changes\n\n" + wantBullet + "\n\n" + notes,
		},
		{
			name:          "PATTERN change without the section or the notes goes at the end",
			prior:         allSectionsContent(),
			patternChange: pattern,
			want:          withEntry(allSectionsContent()) + "## PATTERN changes\n\n" + wantBullet + "\n",
		},
		{
			name:          "PATTERN change with the section goes after its last bullet",
			prior:         withSection,
			patternChange: pattern,
			want:          withEntry(strings.Replace(withSection, "removes\n", "removes\n"+wantBullet+"\n", 1)),
		},
		{
			// The entry follows the section's last non-blank line; the blank line before the next H2 stays.
			name:  "followed by further sections",
			prior: middleBefore + middleAfter,
			want:  strings.TrimSuffix(middleBefore, "\n") + wantEntry + "\n" + middleAfter,
		},
		{
			name:     "a malformed relevant list beside the record is ignored",
			prior:    middleBefore + middleAfter,
			relevant: "a stray paragraph line\n",
			want:     strings.TrimSuffix(middleBefore, "\n") + wantEntry + "\n" + middleAfter,
		},
		{name: "decisions is the last section", prior: decisionsLast.String(), want: decisionsLast.String() + wantEntry},
		{name: "last line has no trailing newline", prior: unterminated, want: unterminated + "\n" + wantEntry},
		{
			name:  "heading inside a fence is ignored",
			prior: fenced,
			want:  strings.Replace(fenced, "```\n\n## Constraints", "```\n"+wantEntry+"\n## Constraints", 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			decisionPath, supportPath := writeFixture(t, dir, tt.prior, "log")
			if tt.relevant != "" {
				if err := os.WriteFile(filepath.Join(dir, "relevant.md"), []byte(tt.relevant), 0o644); err != nil {
					t.Fatalf("write relevant list: %v", err)
				}
			}

			d := addedDecision()
			d.PatternChange = tt.patternChange
			findings, err := AppendDecision(decisionPath, supportPath, d)
			if err != nil || len(findings) != 0 {
				t.Fatalf("AppendDecision() = %v, %v; want no findings and nil error", findings, err)
			}
			if got := readFile(t, decisionPath); got != tt.want {
				t.Errorf("record =\n%q\nwant\n%q", got, tt.want)
			}
			if got := readFile(t, supportPath); got != "log" {
				t.Errorf("support log = %q; want it unchanged", got)
			}
			added, err := ParseAddedDecisions([]byte(readFile(t, decisionPath)))
			if err != nil || len(added) != 1 || added[0].By != "parent" || added[0].Title != "Keep the cache" || !added[0].Date.Equal(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("ParseAddedDecisions() = %+v, %v; want the appended entry's author, date and title", added, err)
			}
			if tt.patternChange != nil {
				changes, problems := ParsePatternChanges([]byte(readFile(t, decisionPath)))
				last := changes[len(changes)-1]
				if len(problems) != 0 || last.ID != "PATTERN-new" || last.By != "parent" || !last.Date.Equal(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("ParsePatternChanges() = %+v, %v; want the appended bullet last with its author and date", changes, problems)
				}
			}
		})
	}
}

func TestAppendDecision_EmptyFieldWritesNothing(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*AddedDecision){
		"by":        func(d *AddedDecision) { d.By = "  " },
		"title":     func(d *AddedDecision) { d.Title = "" },
		"decision":  func(d *AddedDecision) { d.Decision = "\n" },
		"rationale": func(d *AddedDecision) { d.Rationale = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	_, err := AppendDecision(filepath.Join(dir, "absent.md"), filepath.Join(dir, "log.md"), addedDecision())
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("AppendDecision() error = %v; want one satisfying errors.Is(err, os.ErrNotExist)", err)
	}
}

func TestAppendDecision_FindingRestoresRecord(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		prior         string
		patternChange *PatternChange
		wantCheck     string
	}{
		{"MissingSection", strings.Replace(allSectionsContent(), "## Goal", "## Aim", 1), nil, checkSectionMissing},
		{"UnknownPatternAction", allSectionsContent(), &PatternChange{ID: "PATTERN-new", Action: "renames", Text: "x"}, checkPatternChangeMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decisionPath, supportPath := writeFixture(t, t.TempDir(), tt.prior, "log")
			d := addedDecision()
			d.PatternChange = tt.patternChange

			findings, err := AppendDecision(decisionPath, supportPath, d)
			if err != nil {
				t.Fatalf("AppendDecision() error = %v; want nil", err)
			}
			if len(findings) != 1 || findings[0].Check != tt.wantCheck {
				t.Errorf("findings = %v; want one %s finding", findings, tt.wantCheck)
			}
			if got := readFile(t, decisionPath); got != tt.prior {
				t.Errorf("record not restored:\n%q", got)
			}
		})
	}
}

func TestParseAddedDecisions(t *testing.T) {
	t.Parallel()

	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }
	entry := func(by, date, title string) string {
		return "\n### Added after Discussion (" + by + ", " + date + "): " + title + "\n\nDecision: d.\n\nRationale: r.\n"
	}

	tests := []struct {
		name    string
		record  string
		want    []AddedDecision
		wantErr string
	}{
		{
			name:   "entries in order",
			record: "## Decisions\n" + entry("alice", "2026-03-01", "one") + entry("alice", "2026-03-02", "two") + entry("bob", "2026-03-03", "three"),
			want: []AddedDecision{
				{By: "alice", Date: day(1), Title: "one"},
				{By: "alice", Date: day(2), Title: "two"},
				{By: "bob", Date: day(3), Title: "three"},
			},
		},
		{
			name:   "heading inside a fence is skipped",
			record: "## Decisions\n\n```\n### Added after Discussion (alice, 2026-03-01): fenced\n```\n" + entry("bob", "2026-03-03", "real"),
			want:   []AddedDecision{{By: "bob", Date: day(3), Title: "real"}},
		},
		{
			name:    "heading without a date names its line",
			record:  "## Decisions\n\n### Added after Discussion (alice): t\n",
			wantErr: "line 3",
		},
		{name: "no entries", record: "## Decisions\n\n### First\n", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAddedDecisions([]byte(tt.record))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseAddedDecisions() error = %v; want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddedDecisions() error = %v; want nil", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseAddedDecisions() = %+v; want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %+v; want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCheckAddedDecisionsKept asserts a record is held to every added entry its history held: each dropped entry is one finding naming its heading, an entry with the same title on another date is distinct, an unparseable heading in the record is a finding, and one in a history version contributes nothing.
func TestCheckAddedDecisionsKept(t *testing.T) {
	t.Parallel()

	entry := func(date, title string) string {
		return "\n### Added after Discussion (alice, " + date + "): " + title + "\n\nDecision: d.\n\nRationale: r.\n"
	}
	record := func(entries ...string) []byte {
		return []byte("## Decisions\n" + strings.Join(entries, ""))
	}
	heading := func(date, title string) string {
		return "### Added after Discussion (alice, " + date + "): " + title
	}

	appended := "- `PATTERN-a` — removes — added after Discussion (alice, 2026-03-01)"
	withSection := func(lines ...string) []byte {
		return []byte("## Decisions\n\n## PATTERN changes\n\n" + strings.Join(lines, "\n") + "\n")
	}

	tests := []struct {
		name         string
		record       []byte
		versions     [][]byte
		wantHeadings []string
		wantBullets  []string
		wantParseErr bool
	}{
		{
			name:        "a record dropping an appended bullet a version held",
			record:      record(),
			versions:    [][]byte{withSection(appended)},
			wantBullets: []string{"PATTERN-a"},
		},
		{
			name:     "a record keeping the appended bullet",
			record:   withSection(appended),
			versions: [][]byte{withSection(appended)},
		},
		{
			name:     "a bullet without the suffix is the writer's to drop",
			record:   record(),
			versions: [][]byte{withSection("- `PATTERN-a` — removes")},
		},
		{
			name:     "a version whose section holds a malformed bullet contributes no bullet",
			record:   record(),
			versions: [][]byte{withSection(appended, "stray line")},
		},
		{
			name:     "a record whose own section is malformed yields no bullet finding",
			record:   withSection("stray line"),
			versions: [][]byte{withSection(appended)},
		},
		{name: "no versions", record: record(entry("2026-03-01", "one"))},
		{
			name:     "a version whose entry the record keeps",
			record:   record(entry("2026-03-01", "one")),
			versions: [][]byte{record(entry("2026-03-01", "one"))},
		},
		{
			name:         "two entries in history and the record dropping one",
			record:       record(entry("2026-03-01", "one")),
			versions:     [][]byte{record(entry("2026-03-01", "one"), entry("2026-03-02", "two"))},
			wantHeadings: []string{heading("2026-03-02", "two")},
		},
		{
			name:         "the same title on another date is a distinct entry",
			record:       record(entry("2026-03-02", "one")),
			versions:     [][]byte{record(entry("2026-03-01", "one"))},
			wantHeadings: []string{heading("2026-03-01", "one")},
		},
		{
			name:         "dropped entries come in order of first appearance across versions",
			record:       record(),
			versions:     [][]byte{record(entry("2026-03-02", "two")), record(entry("2026-03-01", "one"), entry("2026-03-02", "two"))},
			wantHeadings: []string{heading("2026-03-02", "two"), heading("2026-03-01", "one")},
		},
		{
			name:         "an unparseable heading in the record is a finding",
			record:       []byte("## Decisions\n\n### Added after Discussion (alice): t\n"),
			versions:     [][]byte{record(entry("2026-03-01", "one"))},
			wantParseErr: true,
		},
		{
			name:     "an unparseable heading in a version contributes nothing",
			record:   record(),
			versions: [][]byte{[]byte("## Decisions\n\n### Added after Discussion (alice): t\n")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckAddedDecisionsKept(tt.record, tt.versions)
			if tt.wantParseErr {
				if len(got) != 1 || got[0].Check != checkAddedDecisionDropped || !strings.Contains(got[0].Detail, "line 3") {
					t.Fatalf("CheckAddedDecisionsKept() = %+v; want one finding carrying the parse error naming line 3", got)
				}
				return
			}
			if len(got) != len(tt.wantHeadings)+len(tt.wantBullets) {
				t.Fatalf("CheckAddedDecisionsKept() = %+v; want %d findings", got, len(tt.wantHeadings)+len(tt.wantBullets))
			}
			for i, want := range tt.wantHeadings {
				if got[i].Check != checkAddedDecisionDropped || !strings.Contains(got[i].Detail, want) {
					t.Errorf("finding %d = %+v; want check %q naming %q", i, got[i], checkAddedDecisionDropped, want)
				}
			}
			for i, want := range tt.wantBullets {
				f := got[len(tt.wantHeadings)+i]
				if f.Check != checkPatternChangeDropped || !strings.Contains(f.Detail, want) {
					t.Errorf("finding %d = %+v; want check %q naming %q", len(tt.wantHeadings)+i, f, checkPatternChangeDropped, want)
				}
			}
		})
	}
}
