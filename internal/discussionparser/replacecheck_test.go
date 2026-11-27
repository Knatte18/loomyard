package discussionparser

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckCarryOversKept asserts a replacement keeps every carry-over block byte for byte.
// Each dropped, changed or added block is one finding naming its segment, in segment order.
// A malformed marker set in either record is one finding carrying the scan error.
func TestCheckCarryOversKept(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	planReview := renderedEntry(carryOverEntry("Plan-Review", "cache-key"))
	planReviewChanged := renderedEntry(carryOverEntry("Plan-Review", "other-key"))
	describe := renderedEntry(carryOverEntry("Describe", "describe-key"))
	withBlocks := func(blocks ...string) []byte {
		body := "\n"
		for _, block := range blocks {
			body += block + "\n\n"
		}
		return []byte(recordWithOpenRisks(body))
	}
	malformed := []byte(recordWithOpenRisks("\n" + openMarker + "\n- x\n\n"))

	tests := []struct {
		name    string
		current []byte
		next    []byte
		// wantDetails holds the text each finding's Detail must contain, in finding order.
		wantDetails [][]string
	}{
		{"identical records", withBlocks(planReview), withBlocks(planReview), nil},
		{"no blocks in either record", withBlocks(), withBlocks(), nil},
		{"no Open risks section in either record", []byte(sectionsWithout(openRisksHeading)), []byte(sectionsWithout(openRisksHeading)), nil},
		{"a dropped block", withBlocks(planReview), withBlocks(), [][]string{{"Plan-Review", "dropped", "only Go writes"}}},
		{"a changed block", withBlocks(planReview), withBlocks(planReviewChanged), [][]string{{"Plan-Review", "changed", "only Go writes"}}},
		{"an added block", withBlocks(), withBlocks(planReview), [][]string{{"Plan-Review", "added", "only Go writes"}}},
		{"an added block when the current record has no section", []byte(sectionsWithout(openRisksHeading)), withBlocks(planReview),
			[][]string{{"Plan-Review", "added"}}},
		{"a kept block beside a dropped one", withBlocks(describe, planReview), withBlocks(planReview), [][]string{{"Describe", "dropped"}}},
		{"findings come sorted by segment", withBlocks(planReview, describe), withBlocks(),
			[][]string{{"Describe", "dropped"}, {"Plan-Review", "dropped"}}},
		{"a malformed marker set in the file", withBlocks(planReview), malformed, [][]string{{"malformed", "has no closing line"}}},
		{"a malformed marker set in the current record", malformed, withBlocks(planReview), [][]string{{"malformed", "has no closing line"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckCarryOversKept(tt.current, tt.next)
			if len(got) != len(tt.wantDetails) {
				t.Fatalf("CheckCarryOversKept() = %+v; want %d findings", got, len(tt.wantDetails))
			}
			for i, finding := range got {
				if finding.Check != checkCarryOverChanged {
					t.Errorf("finding %d check = %q; want %q", i, finding.Check, checkCarryOverChanged)
				}
				for _, want := range tt.wantDetails[i] {
					if !strings.Contains(finding.Detail, want) {
						t.Errorf("finding %d detail %q does not contain %q", i, finding.Detail, want)
					}
				}
			}
		})
	}
}

// TestCheckAddedEntriesSubset asserts every added entry of the replacement equals its own entry of the current record.
// Dropping entries is no finding; an inserted or rewritten entry is one finding naming its heading.
// A heading inside a fenced block starts no entry.
func TestCheckAddedEntriesSubset(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	heading := func(title string) string {
		return "### Added after Discussion (alice, 2026-03-01): " + title
	}
	entry := func(title, body string) string {
		return "\n" + heading(title) + "\n\nDecision: " + body + "\n"
	}
	record := func(entries ...string) []byte {
		return []byte(strings.Replace(allSectionsContent(), "## Decisions\n\nbody text.\n",
			"## Decisions\n\nbody text.\n"+strings.Join(entries, ""), 1))
	}
	one, two := entry("one", "keep one."), entry("two", "keep two.")
	fenced := "\n```\n" + heading("fenced") + "\n\nDecision: not an entry.\n```\n"

	tests := []struct {
		name         string
		current      []byte
		next         []byte
		wantHeadings []string
	}{
		{"identical records", record(one, two), record(one, two), nil},
		{"no entries in either record", record(), record(), nil},
		{"the file drops an entry", record(one, two), record(one), nil},
		{"the file drops every entry", record(one, two), record(), nil},
		{"the file rewrites an entry body", record(one, two), record(entry("one", "rewritten."), two), []string{heading("one")}},
		{"the file inserts a new entry", record(one), record(one, two), []string{heading("two")}},
		{"the file inserts an entry into a record with none", record(), record(one), []string{heading("one")}},
		{"the file inserts two entries", record(one), record(entry("three", "x."), one, two), []string{heading("three"), heading("two")}},
		{"the file repeats an entry", record(one), record(one, one), []string{heading("one")}},
		{"the file keeps an entry the record repeats", record(one, one), record(one, one), nil},
		{"a heading inside a fenced block of the file", record(one), record(fenced, one), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckAddedEntriesSubset(tt.current, tt.next)
			if len(got) != len(tt.wantHeadings) {
				t.Fatalf("CheckAddedEntriesSubset() = %+v; want %d findings", got, len(tt.wantHeadings))
			}
			for i, finding := range got {
				if finding.Check != checkAddedDecisionRewritten {
					t.Errorf("finding %d check = %q; want %q", i, finding.Check, checkAddedDecisionRewritten)
				}
				if !strings.Contains(finding.Detail, tt.wantHeadings[i]) {
					t.Errorf("finding %d detail %q does not name %q", i, finding.Detail, tt.wantHeadings[i])
				}
			}
		})
	}
}
