// segment_test.go drives the segment grouping over fake reads: a role's segment and label, the most expensive parts, the cost per run and the segment tree's order.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestPartOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role, segment, label string
	}{
		{"discussion", "Discussion", "writer"},
		{"discussion+sub", "Discussion", "writer forks"},
		{"plan-burler-review", "Plan", "reviewer"},
		{"plan-burler-fix+sub", "Plan", "fixer forks"},
		{"plan-extra", "Plan", "extra"},
		{"webster", "Webster", "Master"},
		{"webster+sub", "Webster", "forks"},
		{"webster-bouncer-judge", "Webster", "judge"},
		{"recovery", "Webster", "recovery"},
		{"recovery+sub", "Webster", "recovery forks"},
		{"merge-bouncer-seed", "Merge", "seed"},
		{"alignment", "Alignment", "session"},
		{"planner", "Other", "planner"},
		{"describe+sub", "Other", "describe forks"},
		{"untitled", "Other", "untitled"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			t.Parallel()
			if segment, label := partOf(tt.role); segment != tt.segment || label != tt.label {
				t.Errorf("partOf(%q) = %q, %q; want %q, %q", tt.role, segment, label, tt.segment, tt.label)
			}
		})
	}
}

func TestSegmentReport(t *testing.T) {
	t.Parallel()
	// The fake prices model-a at $20 per million tokens, so a role's output in millions times 20 is its cost.
	type role struct {
		name string
		// main and fork are output millions of the role's session and of its one fork, 0 for none.
		main, fork float64
	}
	one := []role{
		{"webster", 0.3, 5},
		{"plan", 3, 2},
		{"plan-burler-review", 1.5, 1.25},
		{"discussion", 1, 0},
		{"discussion-burler-fix", 0.9, 0},
		{"discussion-bouncer-seed", 0.8, 0},
		{"merge-bouncer-judge", 0.7, 0},
		{"alignment", 0.6, 0},
		{"recovery", 0.5, 0},
		{"describe", 0.4, 0},
	}
	two := []role{{"webster", 0, 5}}
	fake := newFakeTranscripts()
	var runs []RunTally
	for _, r := range []struct {
		slug  string
		roles []role
	}{{"one", one}, {"two", two}} {
		worktree := filepath.Join("/hub", r.slug)
		for i, ro := range r.roles {
			at := time.Date(2026, 1, 2, 10, i, 0, 0, time.UTC).Format(time.RFC3339)
			var entries []shuttleengine.TranscriptEntry
			if ro.main > 0 {
				entries = append(entries, timedAssistant(ro.name, "model-a", at, int(ro.main*1e6)))
			}
			var forks []shuttleengine.TranscriptRead
			if ro.fork > 0 {
				forks = append(forks, readOf(ro.name+"-fork.jsonl", "", timedAssistant(ro.name+"-fork", "model-a", at, int(ro.fork*1e6))))
			}
			fake.add(worktree, time.Time{}, sessionOf(readOf(ro.name+".jsonl", fmt.Sprintf("ly:%s:%s", r.slug, ro.name), entries...), forks...))
		}
		run, err := CountRun(fakeReadSet(fake), worktree, r.slug, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
	}

	var buf bytes.Buffer
	if err := (Report{Runs: runs}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	report := buf.String()
	// The ten costliest parts of both runs, $459 in all, each with its cost per run; Webster recovery, Other describe and Webster Master fall below them.
	wantParts := "## Most expensive parts\n\n| part | cost | share | per run |\n|---|---|---|---|\n" +
		"| Webster forks | $200.00 | 43.6% | $100.00 |\n" +
		"| Plan writer | $60.00 | 13.1% | $30.00 |\n" +
		"| Plan writer forks | $40.00 | 8.7% | $20.00 |\n" +
		"| Plan reviewer | $30.00 | 6.5% | $15.00 |\n" +
		"| Plan reviewer forks | $25.00 | 5.4% | $12.50 |\n" +
		"| Discussion writer | $20.00 | 4.4% | $10.00 |\n" +
		"| Discussion fixer | $18.00 | 3.9% | $9.00 |\n" +
		"| Discussion seed | $16.00 | 3.5% | $8.00 |\n" +
		"| Merge judge | $14.00 | 3.1% | $7.00 |\n" +
		"| Alignment session | $12.00 | 2.6% | $6.00 |\n\n"
	wantPerRun := "## Cost per run\n\n| run | Discussion | Plan | Webster | Merge | Alignment | Orch | Other | Total |\n|---|---|---|---|---|---|---|---|---|\n" +
		"| one | $54.00 | $155.00 | $116.00 | $14.00 | $12.00 | $0.00 | $8.00 | $359.00 |\n" +
		"| two | $0.00 | $0.00 | $100.00 | $0.00 | $0.00 | $0.00 | $0.00 | $100.00 |\n" +
		"| average | $27.00 | $77.50 | $108.00 | $7.00 | $6.00 | $0.00 | $4.00 | $229.50 |\n\n## All runs\n"
	if !strings.HasPrefix(report, wantParts+wantPerRun) {
		t.Errorf("report does not open with the most expensive parts and the cost per run:\n%s", report)
	}

	all, _, _ := strings.Cut(report, "## one\n")
	// Segments, and the parts within each, run most expensive first, and each segment's parts sum to its row.
	rest := all
	for _, want := range []string{
		"| segment | cost | share | time | time share | models (messages) |\n|---|---|---|---|---|---|\n",
		"| **Webster** | $216.00 | 47.1% |",
		"| └ forks | $200.00 | 43.6% |", "| └ recovery | $10.00 | 2.2% |", "| └ Master | $6.00 | 1.3% |",
		"| **Plan** | $155.00 | 33.8% |",
		"| └ writer | $60.00 | 13.1% |", "| └ writer forks | $40.00 | 8.7% |", "| └ reviewer | $30.00 | 6.5% |", "| └ reviewer forks | $25.00 | 5.4% |",
		"| **Discussion** | $54.00 | 11.8% |",
		"| **Merge** | $14.00 | 3.1% |",
		"| **Alignment** | $12.00 | 2.6% |",
		"| **Other** | $8.00 | 1.7% |",
		"| idle |",
	} {
		i := strings.Index(rest, want)
		if i < 0 {
			t.Fatalf("All runs section lacks %q after the rows before it:\n%s", want, all)
		}
		rest = rest[i+len(want):]
	}
	if strings.Contains(report, forkSuffix) {
		t.Errorf("report shows %q:\n%s", forkSuffix, report)
	}
}
