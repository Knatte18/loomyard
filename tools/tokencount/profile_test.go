// profile_test.go drives the per-profile cost and quality report over an in-memory history of webster records and review files: the per-batch attribution, the marked runs, the findings rule and the printed tables.
// Tier-1 (no git, no spawn).

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeRecords is an in-memory RunRecords: the plan history plus the files directly in a directory at a revision.
type fakeRecords struct{ fakeHistory }

func (f fakeRecords) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	var names []string
	for key := range f.files {
		if rest, ok := strings.CutPrefix(key, rev+":"+dir+"/"); ok && !strings.Contains(rest, "/") {
			names = append(names, rest)
		}
	}
	return names, nil
}

// reviewFile renders a review file that parses and holds the given number of findings.
func reviewFile(findings int) string {
	if findings == 0 {
		return "---\nverdict: APPROVED\nfindings: []\n---\n"
	}
	var b strings.Builder
	b.WriteString("---\nverdict: APPROVED\nfindings:\n")
	for i := 1; i <= findings; i++ {
		fmt.Fprintf(&b, "  - id: f%d\n    severity: MEDIUM\n    class: scope\n    location: internal/a/a.go\n    summary: a finding\n", i)
	}
	b.WriteString("---\n")
	return b.String()
}

func TestBuildProfileReport(t *testing.T) {
	t.Parallel()

	firstFork := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	history := fakeRecords{fakeHistory{commits: map[string][]gitrepo.SubjectCommit{}, files: map[string]string{}}}
	record := func(slug, sha string) {
		subject := websterRecordSubjectPrefix + slug
		history.commits[subject] = append(history.commits[subject], gitrepo.SubjectCommit{SHA: sha, Committed: firstFork.Add(time.Hour)})
	}
	state := func(sha string, st websterengine.State) {
		data, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		history.files[sha+":"+path.Join(websterengine.DirRel(), "state.json")] = string(data)
	}
	review := func(sha, name string, findings int) {
		history.files[sha+":"+path.Join(loomengine.LoomReviewsDirRel(), "webster", name)] = reviewFile(findings)
	}
	transcripts := func(names ...string) []string { return names }

	// Two profiles: batch 1 of two cards ran as a fork, batch 2 of one card was a recovery.
	// Its stamped review sibling and its archived generation are not review files of the run.
	record("multi", "multirec")
	state("multirec", websterengine.State{
		RunGUID: "g",
		Partition: []websterengine.PartitionBatch{
			{Cards: []string{"01-a", "02-b"}, Profile: "cautious"},
			{Cards: []string{"03-c"}, Profile: "wide"},
		},
		Batches: map[int]*websterengine.BatchState{
			1: {Kind: "fork", Terminal: true, Status: "done", ForkTranscripts: transcripts("subagents/agent-1.jsonl")},
			3: {Kind: "recovery", Terminal: true, Status: "done", ForkTranscripts: transcripts("subagents/agent-2.jsonl")},
		},
	})
	review("multirec", "round-1-review.md", 2)
	review("multirec", "round-2-review.md", 1)
	review("multirec", "round-2-review-20260820T101500Z.md", 9)
	history.files["multirec:"+path.Join(loomengine.LoomReviewsDirRel(), "webster", "prior-generation", "round-1-review.md")] = reviewFile(9)

	record("solo", "solorec")
	state("solorec", websterengine.State{
		RunGUID:   "g",
		Partition: []websterengine.PartitionBatch{{Cards: []string{"01-x", "02-y"}, Profile: "cautious"}},
		Batches:   map[int]*websterengine.BatchState{1: {Kind: "fork", Terminal: true, Status: "done", ForkTranscripts: transcripts("agent-1.jsonl")}},
	})
	review("solorec", "round-1-review.md", 1)

	// No partition: counted as identity from the plan commit, and no review files.
	record("plain", "plainrec")
	state("plainrec", websterengine.State{
		RunGUID: "g",
		Batches: map[int]*websterengine.BatchState{1: {Kind: "fork", Terminal: true, Status: "failed", ForkTranscripts: transcripts("agent-1.jsonl")}},
	})
	history.commits[planCommitSubjectPrefix+"plain"] = []gitrepo.SubjectCommit{{SHA: "plainplan", Committed: firstFork.Add(-time.Hour)}}
	for name, data := range planWith(map[int]string{1: "internal/a/a.go", 2: "internal/b/b.go"}) {
		history.files["plainplan:_lyx/plan/"+name] = data
	}

	// Two run guids: the run was restarted.
	record("fresh", "freshnew")
	record("fresh", "freshold")
	state("freshnew", websterengine.State{RunGUID: "g2"})
	state("freshold", websterengine.State{RunGUID: "g1"})

	record("broken", "brokenrec")
	history.files["brokenrec:"+path.Join(websterengine.DirRel(), "state.json")] = "not json"

	forkOf := func(file string, usage Usage, peak int) ForkTally {
		return ForkTally{File: file, Started: firstFork, Usage: usage, PeakContext: peak}
	}
	runs := []RunTally{
		{Slug: "multi", Forks: []ForkTally{
			forkOf("agent-1.jsonl", Usage{Input: 2_000_000}, 300_000),
			forkOf("agent-2.jsonl", Usage{Output: 400_000}, 150_000),
			forkOf("agent-3.jsonl", Usage{Input: 9_000_000}, 1),
		}},
		{Slug: "solo", Forks: []ForkTally{forkOf("agent-1.jsonl", Usage{Input: 1_000_000}, 100_000)}},
		{Slug: "plain", Forks: []ForkTally{forkOf("agent-1.jsonl", Usage{Input: 500_000}, 50_000)}},
		{Slug: "fresh"},
		{Slug: "broken"},
		{Slug: "unrecorded"},
	}

	got, err := BuildProfileReport(runs, history)
	if err != nil {
		t.Fatalf("BuildProfileReport: %v", err)
	}

	wantLines := map[string]ProfileLine{
		// The two-profile run's batch counts here, its findings do not.
		"cautious": {Profile: "cautious", Runs: 2, Batches: 2, Cards: 4, Weight: 3_000_000, FindingsRuns: 1, Findings: 1, FindingsCards: 2},
		"identity": {Profile: "identity", Runs: 1, Batches: 2, Cards: 2, Weight: 500_000, Failed: 1},
		"wide":     {Profile: "wide", Runs: 1, Batches: 1, Cards: 1, Weight: 2_000_000, Failed: 1, Recoveries: 1},
	}
	if len(got.Profiles) != len(wantLines) {
		t.Fatalf("profile lines = %+v; want %+v", got.Profiles, wantLines)
	}
	for _, line := range got.Profiles {
		if want := wantLines[line.Profile]; line != want {
			t.Errorf("profile line %q = %+v; want %+v", line.Profile, line, want)
		}
	}

	var out bytes.Buffer
	got.WriteMarkdown(&out)
	for _, want := range []string{
		"| multi | 1 | cautious | 01-a, 02-b | 300000 | 2.0M |",
		"| multi | 3 | wide | 03-c | 150000 | 2.0M |",
		// A run whose batches name two profiles shows its findings per card in its own row only.
		"| multi | cautious, wide | 2 | 3 | 1 | 1 | 3 | 1.000 |  |",
		"| plain | identity | 2 | 2 | 1 | 0 | not read | n/a | findings not read: no review files |",
		"| fresh | n/a | n/a | n/a | n/a | n/a | n/a | n/a | marked: restarted with --fresh: its records carry 2 run guids |",
		"| broken | n/a | n/a | n/a | n/a | n/a | n/a | n/a | marked: state at brokenrec unreadable: ",
		"| unrecorded | n/a | n/a | n/a | n/a | n/a | n/a | n/a | marked: no webster run record |",
		"| cautious | 2 | 2 | 4 | 0.750M | 0 | 0 | 0.500 (1 runs) |",
		"| identity | 1 | 2 | 2 | 0.250M | 1 | 0 | n/a |",
		"| wide | 1 | 1 | 1 | 2.000M | 1 | 1 | n/a |",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}
