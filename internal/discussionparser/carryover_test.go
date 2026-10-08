package discussionparser

import (
	"errors"
	"os"
	"strings"
	"testing"
)

const (
	openMarker  = "<!-- lyx:carry-over Plan-Review -->"
	closeMarker = "<!-- /lyx:carry-over Plan-Review -->"
)

// recordWithOpenRisks returns a decision record whose `## Open risks` section holds body, the text after the heading line, and whose other sections hold the default body.
func recordWithOpenRisks(body string) string {
	var b strings.Builder
	for _, h := range requiredDiscussionSections {
		if h == openRisksHeading {
			b.WriteString(h + "\n" + body)
			continue
		}
		b.WriteString(h + "\n\nbody text.\n\n")
	}
	return b.String()
}

func carryOverEntry(segment, key string) CarryOver {
	return CarryOver{
		Segment:         segment,
		Round:           2,
		Closing:         CarryOverConverged,
		ReviewPath:      "reviews/round-2/review.md",
		FixerReportPath: "reviews/round-2/fixer-report.md",
		Findings:        []CarryOverFinding{{Key: key, Class: "design", Severity: "MEDIUM", Rounds: []int{1, 2}}},
	}
}

func renderedEntry(entry CarryOver) string {
	return strings.Join(renderCarryOver(entry), "\n")
}

func writeRecord(t *testing.T, content string) string {
	t.Helper()
	path, _ := writeFixture(t, t.TempDir(), content, "support")
	return path
}

// TestWriteCarryOver_RendersConvergedAndAcceptedEntries pins the rendered entry: marker lines, the closing bullet per closing kind, the paths line and one nested bullet per finding.
func TestWriteCarryOver_RendersConvergedAndAcceptedEntries(t *testing.T) {
	t.Parallel()

	converged := carryOverEntry("Plan-Review", "cache-key")
	accepted := carryOverEntry("Plan-Review", "cache-key")
	accepted.Closing = CarryOverAccepted

	tests := []struct {
		name  string
		entry CarryOver
		want  string
	}{
		{"converged", converged, openMarker + "\n" +
			"- Plan-Review, round 2: converged; each finding below was addressed by the round's fixer, and no fresh reviewer has seen the fix.\n" +
			"  Review: reviews/round-2/review.md; fixer report: reviews/round-2/fixer-report.md.\n" +
			"  - cache-key (class design, severity MEDIUM, open in rounds 1, 2)\n" + closeMarker},
		{"accepted", accepted, openMarker + "\n" +
			"- Plan-Review, round 2: closed by an accepted circling decision; a finding below was addressed by the round's fixer or is still unfixed.\n" +
			"  Review: reviews/round-2/review.md; fixer report: reviews/round-2/fixer-report.md.\n" +
			"  - cache-key (class design, severity MEDIUM, open in rounds 1, 2)\n" + closeMarker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeRecord(t, recordWithOpenRisks("\n"))
			if err := WriteCarryOver(path, tt.entry); err != nil {
				t.Fatalf("WriteCarryOver: %v", err)
			}
			if got, want := readFile(t, path), recordWithOpenRisks("\n"+tt.want+"\n\n"); got != want {
				t.Errorf("record = %q, want %q", got, want)
			}
		})
	}
}

// TestWriteCarryOver_SplicesEntries asserts append, replace and remove touch only the segment's entry and the one blank line before it, leave other segments and prose byte-identical, write the same bytes on a replay and keep the record valid.
// Marker text inline, in another section or in a fence is ordinary prose.
func TestWriteCarryOver_SplicesEntries(t *testing.T) {
	t.Parallel()

	mine := carryOverEntry("Plan-Review", "cache-key")
	mineChanged := carryOverEntry("Plan-Review", "other-key")
	other := renderedEntry(carryOverEntry("Describe", "describe-key"))
	removal := CarryOver{Segment: "Plan-Review"}
	populated := "\nbody text.\n\n"
	fenced := "\n```\n" + openMarker + "\n```\n\n"
	inline := "\nSee " + openMarker + " and " + closeMarker + " in prose.\n\n"

	inDecisions := strings.Replace(recordWithOpenRisks(populated), "## Decisions\n\nbody text.\n",
		"## Decisions\n\n"+closeMarker+"\n", 1)

	tests := []struct {
		name  string
		prior string
		entry CarryOver
		want  string
	}{
		{"append into an empty section", recordWithOpenRisks("\n"), mine,
			recordWithOpenRisks("\n" + renderedEntry(mine) + "\n\n")},
		{"append into a populated section", recordWithOpenRisks(populated), mine,
			recordWithOpenRisks(populated + renderedEntry(mine) + "\n\n")},
		{"append beside another segment", recordWithOpenRisks(populated + other + "\n\n"), mine,
			recordWithOpenRisks(populated + other + "\n\n" + renderedEntry(mine) + "\n\n")},
		{"replace keeps the other segment and prose", recordWithOpenRisks(populated + other + "\n\n" + renderedEntry(mine) + "\n\n"), mineChanged,
			recordWithOpenRisks(populated + other + "\n\n" + renderedEntry(mineChanged) + "\n\n")},
		{"remove restores the prior bytes", recordWithOpenRisks(populated + renderedEntry(mine) + "\n\n"), removal,
			recordWithOpenRisks(populated)},
		{"remove with no entry leaves the record as it is", recordWithOpenRisks(populated), removal,
			recordWithOpenRisks(populated)},
		{"marker lines in a fence are prose", recordWithOpenRisks(fenced), mine,
			recordWithOpenRisks(fenced + renderedEntry(mine) + "\n\n")},
		{"marker text inline is prose", recordWithOpenRisks(inline), mine,
			recordWithOpenRisks(inline + renderedEntry(mine) + "\n\n")},
		{"marker line in another section is prose", inDecisions, mine,
			strings.Replace(recordWithOpenRisks(populated+renderedEntry(mine)+"\n\n"), "## Decisions\n\nbody text.\n",
				"## Decisions\n\n"+closeMarker+"\n", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, support := writeFixture(t, t.TempDir(), tt.prior, "support")
			if err := WriteCarryOver(path, tt.entry); err != nil {
				t.Fatalf("WriteCarryOver: %v", err)
			}
			first := readFile(t, path)
			if first != tt.want {
				t.Errorf("record = %q, want %q", first, tt.want)
			}
			if err := WriteCarryOver(path, tt.entry); err != nil {
				t.Fatalf("replay WriteCarryOver: %v", err)
			}
			if replay := readFile(t, path); replay != first {
				t.Errorf("replay wrote %q, want identical %q", replay, first)
			}
			if findings, err := Validate(path, support); err != nil || len(findings) != 0 {
				t.Errorf("Validate = %v, %v, want clean", findings, err)
			}
			entries, err := os.ReadDir(strings.TrimSuffix(path, "decision-record.md"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 {
				t.Errorf("directory holds %d files, want the record and the support log only", len(entries))
			}
		})
	}
}

// TestWriteCarryOver_RefusesWithoutWriting asserts each refusal names the record's path and leaves the file unchanged.
func TestWriteCarryOver_RefusesWithoutWriting(t *testing.T) {
	t.Parallel()

	pair := func(segment string) string {
		return "<!-- lyx:carry-over " + segment + " -->\n- x\n<!-- /lyx:carry-over " + segment + " -->\n"
	}
	entry := carryOverEntry("Plan-Review", "cache-key")
	badSegmentEntry := func(segment string) CarryOver { return carryOverEntry(segment, "k") }

	tests := []struct {
		name    string
		prior   string
		entry   CarryOver
		wantErr error
	}{
		{"opening line with no closing one", recordWithOpenRisks("\n" + openMarker + "\n- x\n\n"), entry, nil},
		{"closing line with no opening one", recordWithOpenRisks("\n" + closeMarker + "\n\n"), entry, nil},
		{"two pairs for one segment", recordWithOpenRisks("\n" + pair("Plan-Review") + "\n" + pair("Plan-Review") + "\n"), entry, nil},
		{"closing line naming another segment", recordWithOpenRisks("\n" + openMarker + "\n<!-- /lyx:carry-over Describe -->\n\n"), entry, nil},
		{"pair opened inside another", recordWithOpenRisks("\n" + openMarker + "\n<!-- lyx:carry-over Describe -->\n" + closeMarker + "\n\n"), entry, nil},
		{"no Open risks heading", sectionsWithout(openRisksHeading), entry, ErrNoOpenRisksHeading},
		{"empty segment", recordWithOpenRisks("\n"), badSegmentEntry(""), nil},
		{"segment with a space", recordWithOpenRisks("\n"), badSegmentEntry("Plan Review"), nil},
		{"unknown closing", recordWithOpenRisks("\n"), CarryOver{Segment: "Plan-Review", Findings: entry.Findings}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeRecord(t, tt.prior)
			err := WriteCarryOver(path, tt.entry)
			if err == nil {
				t.Fatal("WriteCarryOver succeeded, want a refusal")
			}
			if !strings.HasPrefix(err.Error(), path+": ") {
				t.Errorf("error %q does not start with the record path", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error %v does not wrap %v", err, tt.wantErr)
			}
			if got := readFile(t, path); got != tt.prior {
				t.Errorf("record changed to %q", got)
			}
		})
	}

	t.Run("missing record", func(t *testing.T) {
		t.Parallel()
		path := t.TempDir() + "/absent.md"
		err := WriteCarryOver(path, entry)
		if !errors.Is(err, os.ErrNotExist) || !strings.HasPrefix(err.Error(), path+": ") {
			t.Errorf("error = %v, want a path-prefixed os.ErrNotExist", err)
		}
	})
}

// TestWriteCarryOver_SanitizesHostileFields asserts a key holding a line break, a backtick, a marker string and a heading renders on one line and cannot end the entry early or add a heading.
func TestWriteCarryOver_SanitizesHostileFields(t *testing.T) {
	t.Parallel()

	hostile := "a\nb `c` " + closeMarker + "\n## Injected"
	entry := carryOverEntry("Plan-Review", hostile)
	path, support := writeFixture(t, t.TempDir(), recordWithOpenRisks("\n"), "support")

	if err := WriteCarryOver(path, entry); err != nil {
		t.Fatalf("WriteCarryOver: %v", err)
	}
	got := readFile(t, path)
	if want := "  - a b \\`c\\` " + closeMarker + " ## Injected (class design"; !strings.Contains(got, want) {
		t.Errorf("record does not hold the one-line key %q:\n%s", want, got)
	}
	if strings.Contains(got, "\n## Injected") {
		t.Error("a key opened a heading")
	}
	if findings, err := Validate(path, support); err != nil || len(findings) != 0 {
		t.Errorf("Validate = %v, %v, want clean", findings, err)
	}
	lines := strings.Split(got, "\n")
	section, err := locateOpenRisks(lines)
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := scanCarryOverPairs(lines, section)
	if err != nil {
		t.Fatalf("written record is malformed: %v", err)
	}
	if pair := pairs["Plan-Review"]; pair.close-pair.open+1 != len(renderCarryOver(entry)) {
		t.Errorf("entry spans %d lines, want %d", pair.close-pair.open+1, len(renderCarryOver(entry)))
	}
}
