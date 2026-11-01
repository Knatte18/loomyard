// landed_test.go checks how a finished run's landed commit is found from its archived change description, and the Recipes section that compares runs by recipe.

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeLandingRepo is an in-memory landingRepo: commits by subject, newest first, and changed lines by commit.
type fakeLandingRepo struct {
	commits map[string][]gitrepo.SubjectCommit
	lines   map[string]int
}

func (f fakeLandingRepo) CommitsWithSubject(subject string) ([]gitrepo.SubjectCommit, error) {
	return f.commits[subject], nil
}

func (f fakeLandingRepo) ChangedLines(sha string) (int, error) {
	return f.lines[sha], nil
}

func TestFindLanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	at := func(hms string) time.Time {
		parsed, err := time.Parse(time.RFC3339, "2026-01-01T"+hms+"Z")
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	span := Interval{at("10:00:00"), at("11:00:00")}
	rec := ShedRecord{Tag: ArchiveTag{Name: "archive/r/1", Commit: "c"}}
	const title = "Land the change"
	described := fakeArchiveTrees{{"c", summaryRel}: "# " + title + "\n\nWhat changed.\n"}
	commits := func(times ...string) map[string][]gitrepo.SubjectCommit {
		var found []gitrepo.SubjectCommit
		for _, hms := range times {
			found = append(found, gitrepo.SubjectCommit{SHA: hms, Committed: at(hms)})
		}
		return map[string][]gitrepo.SubjectCommit{title: found}
	}
	lines := map[string]int{"10:30:00": 7, "10:10:00": 3, "11:00:30": 5}
	tests := []struct {
		name    string
		trees   fakeArchiveTrees
		commits map[string][]gitrepo.SubjectCommit
		want    Landing
		// parseFails marks a row whose Landing names noParsableDescription and the parse error.
		parseFails bool
	}{
		{"the newest commit within the span", described, commits("11:05:00", "10:30:00", "10:10:00"), Landing{SHA: "10:30:00", Lines: 7}, false},
		{"a commit within a minute past the span", described, commits("11:00:30"), Landing{SHA: "11:00:30", Lines: 5}, false},
		{"no titled commit within the span", described, commits("09:58:00"), Landing{Missing: noTitledCommit}, false},
		{"no change description at the tag", fakeArchiveTrees{}, commits("10:30:00"), Landing{Missing: noDescription}, false},
		{"a change description that does not parse", fakeArchiveTrees{{"c", summaryRel}: "no heading\n"}, commits("10:30:00"), Landing{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := findLanding(tt.trees, fakeLandingRepo{tt.commits, lines}, "r", rec, span)
			if err != nil {
				t.Fatal(err)
			}
			if tt.parseFails {
				if got.SHA != "" || !strings.HasPrefix(got.Missing, noParsableDescription+": ") {
					t.Errorf("findLanding = %+v; want no commit and a reason opening %q", got, noParsableDescription)
				}
				return
			}
			if got != tt.want {
				t.Errorf("findLanding = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestWriteRecipes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	// runOf builds a run of recipe, empty for none read, costing usd over span, with landing.
	runOf := func(slug, recipe string, usd float64, span time.Duration, landing *Landing) RunTally {
		run := RunTally{
			Slug:    slug,
			Roles:   map[string]*RoleTally{"darn": {Role: "darn", Cost: Cost{USD: usd}, Models: map[string]int{}}},
			Span:    Interval{start, start.Add(span)},
			Landing: landing,
		}
		if recipe != "" {
			run.Shed = &ShedRecord{Recipe: recipe}
		}
		return run
	}
	runs := []RunTally{
		runOf("t1", "tapestry", 100, 2*time.Hour, &Landing{SHA: "a", Lines: 50}),
		runOf("t2", "tapestry", 60, time.Hour, &Landing{Missing: noTitledCommit}),
		runOf("d1", "darn", 10, 20*time.Minute, &Landing{SHA: "b", Lines: 20}),
		runOf("d2", "darn", 14, 40*time.Minute, &Landing{SHA: "c", Lines: 30}),
		runOf("u", "", 5, 0, nil),
		runOf("b1", "bare", 8, 10*time.Minute, &Landing{SHA: "d", Lines: 0}),
		runOf("o1", "odd", 4, 10*time.Minute, &Landing{SHA: "e", Lines: 10}),
		runOf("p1", "only", 0, 10*time.Minute, &Landing{SHA: "f", Lines: 10}),
	}
	// A cost holding unpriced messages is partly or wholly unpriced.
	for _, i := range []int{6, 7} {
		runs[i].Roles["darn"].Cost.Unpriced = map[string]int{"m": 1}
	}
	perRun := make([][]Segment, len(runs))
	for i, run := range runs {
		perRun[i] = run.Segments(nil)
	}
	var buf bytes.Buffer
	writeRecipes(&buf, runs, perRun)
	report := buf.String()
	// Recipes run by name; the cost per changed line takes only the runs that landed a commit.
	wantRows := "| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |\n|---|---|---|---|---|---|---|\n" +
		"| bare | 1 | $8.00 | 10m0s | 1 | 0 | n/a |\n" +
		"| darn | 2 | $12.00 | 30m0s | 2 | 25 | $0.480 |\n" +
		"| odd | 1 | $4.00 + unpriced | 10m0s | 1 | 10 | $0.400 + unpriced |\n" +
		"| only | 1 | unpriced | 10m0s | 1 | 10 | unpriced |\n" +
		"| tapestry | 2 | $80.00 | 1h30m0s | 1 | 50 | $2.000 |\n" +
		"| unknown | 1 | $5.00 | 0s | 0 | n/a | n/a |\n"
	if !strings.HasPrefix(report, "## Recipes\n\n"+wantRows) {
		t.Errorf("Recipes section lacks the rows %q:\n%s", wantRows, report)
	}
	wantMissing := "No landed commit: t2 (" + noTitledCommit + "), u (" + noShedRecord + ").\n"
	if !strings.Contains(report, wantMissing) {
		t.Errorf("Recipes section lacks %q:\n%s", wantMissing, report)
	}
}
