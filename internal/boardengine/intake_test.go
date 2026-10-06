// intake_test.go — unit tests for importing and folding inbox issues (intake.go).
//
// Every test runs a Board over a temp dir with a configured vocabulary and SkipGit;
// a refused or no-op import must leave board.json byte-identical.

package boardengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func newIntakeBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
	return boardengine.New(cfg), boardPath
}

func testIssue(number int, labels ...string) boardengine.InboxIssue {
	return boardengine.InboxIssue{Number: number, Title: "Issue title", Body: "Issue body.", URL: "https://example.test/issues/1", Labels: labels, Open: true}
}

func readBoardJSON(t *testing.T, boardPath string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	return string(raw)
}

func mustUpsert(t *testing.T, b *boardengine.Board, fields map[string]any) {
	t.Helper()
	if _, err := b.UpsertTask(fields); err != nil {
		t.Fatalf("UpsertTask(%v): %v", fields, err)
	}
}

// TestImportIssueNewEntry asserts importing an issue creates a note carrying the issue's title, body link, number and configured labels,
// drops the labels the vocabulary lacks, and honours an explicit title, brief and label set.
func TestImportIssueNewEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		req         boardengine.ImportRequest
		wantTitle   string
		wantBrief   string
		wantLabels  []string
		wantDropped []string
	}{
		{
			name:        "issue labels filtered to the vocabulary",
			req:         boardengine.ImportRequest{Issue: testIssue(7, "bug", "undecided", "stray"), Slug: "from-inbox"},
			wantTitle:   "Issue title",
			wantLabels:  []string{"bug", "undecided"},
			wantDropped: []string{"stray"},
		},
		{
			name:       "explicit title, brief and labels win",
			req:        boardengine.ImportRequest{Issue: testIssue(7, "stray"), Slug: "x", Title: "Mine", Brief: "b", Labels: []string{"enhancement"}},
			wantTitle:  "Mine",
			wantBrief:  "b",
			wantLabels: []string{"enhancement"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, _ := newIntakeBoard(t)
			res, err := b.ImportIssue(tt.req)
			if err != nil {
				t.Fatalf("ImportIssue: %v", err)
			}
			e := res.Entry
			if e.Kind != boardengine.KindNote || e.Title != tt.wantTitle || e.Brief != tt.wantBrief || !slices.Equal(e.Labels, tt.wantLabels) || !slices.Equal(e.Issues, []int{7}) {
				t.Errorf("entry = %+v", e)
			}
			wantBody := "Imported from [issue #7](https://example.test/issues/1).\n\nIssue body."
			if e.Body != wantBody {
				t.Errorf("body = %q, want %q", e.Body, wantBody)
			}
			if !slices.Equal(res.Dropped, tt.wantDropped) {
				t.Errorf("dropped = %v, want %v", res.Dropped, tt.wantDropped)
			}
		})
	}
}

func TestImportIssueFold(t *testing.T) {
	b, _ := newIntakeBoard(t)
	mustUpsert(t, b, map[string]any{"slug": "work", "kind": "task", "labels": bugLabels, "body": "Existing."})
	mustUpsert(t, b, map[string]any{"slug": "old", "kind": "task", "labels": bugLabels, "status": "done"})
	mustUpsert(t, b, map[string]any{"slug": "idea", "labels": bugLabels})

	res, err := b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(3, "enhancement", "undecided", "stray", "bug"), Into: "work"})
	if err != nil {
		t.Fatalf("fold into task: %v", err)
	}
	if !slices.Equal(res.Entry.Labels, []string{"bug", "enhancement", "undecided"}) || !slices.Equal(res.Dropped, []string{"stray"}) || !slices.Equal(res.Entry.Issues, []int{3}) {
		t.Errorf("task fold = %+v", res)
	}
	wantBody := "Existing.\n\n## From issue #3\n\nImported from [issue #3](https://example.test/issues/1).\n\nIssue body."
	if res.Entry.Body != wantBody {
		t.Errorf("body = %q, want %q", res.Entry.Body, wantBody)
	}

	res, err = b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(4, "undecided"), Into: "old"})
	if err != nil {
		t.Fatalf("fold into done entry: %v", err)
	}
	if !slices.Equal(res.Entry.Labels, []string{"bug", "undecided"}) || !strings.HasPrefix(res.Entry.Body, "## From issue #4") {
		t.Errorf("done fold = %+v", res.Entry)
	}

	res, err = b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(5, "enhancement", "undecided"), Into: "idea"})
	if err != nil {
		t.Fatalf("fold into note: %v", err)
	}
	if !slices.Equal(res.Entry.Labels, []string{"bug", "undecided"}) || !slices.Equal(res.Dropped, []string{"enhancement"}) {
		t.Errorf("note fold = %+v", res)
	}
}

func TestImportIssueFoldIntoStaleLabelRefused(t *testing.T) {
	b, boardPath := newIntakeBoard(t)
	mustUpsert(t, b, map[string]any{"slug": "work", "labels": bugLabels})
	raw := strings.Replace(readBoardJSON(t, boardPath), `"bug"`, `"retired"`, 1)
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(3), Into: "work"})
	if err == nil || !strings.Contains(err.Error(), "retired") || !strings.Contains(err.Error(), "board.yaml") {
		t.Errorf("err = %v, want the unconfigured-label refusal", err)
	}
	if readBoardJSON(t, boardPath) != raw {
		t.Error("a refused fold changed board.json")
	}
}

func TestImportIssueRecordedIsNoOp(t *testing.T) {
	b, boardPath := newIntakeBoard(t)
	if _, err := b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(7, "bug"), Slug: "first"}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	mustUpsert(t, b, map[string]any{"slug": "by-hand", "labels": bugLabels, "issues": []int{9}})
	before := readBoardJSON(t, boardPath)

	for _, tc := range []struct {
		number int
		want   string
	}{{7, "first"}, {9, "by-hand"}} {
		res, err := b.ImportIssue(boardengine.ImportRequest{Issue: testIssue(tc.number), Slug: "anything", Into: "also-given"})
		if err != nil {
			t.Fatalf("#%d: %v", tc.number, err)
		}
		if !slices.Equal(res.Recorded, []string{tc.want}) {
			t.Errorf("#%d recorded = %v, want [%s]", tc.number, res.Recorded, tc.want)
		}
	}
	if readBoardJSON(t, boardPath) != before {
		t.Error("a no-op import changed board.json")
	}

	recorded, err := b.RecordedIssues()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(recorded[7], []string{"first"}) || !slices.Equal(recorded[9], []string{"by-hand"}) {
		t.Errorf("RecordedIssues = %v", recorded)
	}
}

func TestImportIssueRefusals(t *testing.T) {
	b, boardPath := newIntakeBoard(t)
	mustUpsert(t, b, map[string]any{"slug": "taken", "labels": bugLabels})
	before := readBoardJSON(t, boardPath)

	pr := testIssue(1, "bug")
	pr.PullRequest = true
	closed := testIssue(2, "bug")
	closed.Open = false

	for name, tc := range map[string]struct {
		req  boardengine.ImportRequest
		want []string
	}{
		"pull request":    {boardengine.ImportRequest{Issue: pr, Slug: "x"}, []string{"pull request"}},
		"closed":          {boardengine.ImportRequest{Issue: closed, Slug: "x"}, []string{"closed"}},
		"taken slug":      {boardengine.ImportRequest{Issue: testIssue(3, "bug"), Slug: "taken"}, []string{`"taken"`}},
		"missing into":    {boardengine.ImportRequest{Issue: testIssue(3, "bug"), Into: "nope"}, []string{`"nope"`}},
		"into and title":  {boardengine.ImportRequest{Issue: testIssue(3, "bug"), Into: "taken", Title: "t"}, []string{"takes no title"}},
		"slug and into":   {boardengine.ImportRequest{Issue: testIssue(3, "bug"), Into: "taken", Slug: "s"}, []string{"both"}},
		"neither":         {boardengine.ImportRequest{Issue: testIssue(3, "bug")}, []string{"neither"}},
		"no type label":   {boardengine.ImportRequest{Issue: testIssue(7, "undecided"), Slug: "x"}, []string{"no type label", "labels"}},
		"two type labels": {boardengine.ImportRequest{Issue: testIssue(7, "bug", "enhancement"), Slug: "x"}, []string{"bug, enhancement"}},
	} {
		_, err := b.ImportIssue(tc.req)
		for _, want := range tc.want {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s: err = %v, want it to contain %q", name, err, want)
			}
		}
	}
	if readBoardJSON(t, boardPath) != before {
		t.Error("a refused import changed board.json")
	}
}
