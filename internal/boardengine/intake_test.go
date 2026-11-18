// intake_test.go — unit tests for importing and folding GitHub issues (intake.go).
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
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// testIssueWithComments returns an issue with two comments, oldest first;
// the first is created late on 2 January in UTC-5, so on 3 January in UTC.
func testIssueWithComments(number int, labels ...string) boardengine.InboxIssue {
	issue := testIssue(number, labels...)
	issue.Comments = []boardengine.IssueComment{
		{Body: "First comment.", CreatedAt: time.Date(2026, 1, 2, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60))},
		{Body: "Second comment.\n\nWith a paragraph.", CreatedAt: time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)},
	}
	return issue
}

// testIssueWithBody returns a bug issue with body and comments in place of the default body.
func testIssueWithBody(number int, body string, comments ...boardengine.IssueComment) boardengine.InboxIssue {
	issue := testIssue(number, "bug")
	issue.Body = body
	issue.Comments = comments
	return issue
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

// TestImportIssueNewEntry asserts that importing an issue creates a note.
// The note carries the issue's title, body link, number and configured labels.
// Labels the vocabulary lacks are dropped, and an explicit title, brief and label set are honoured.
// The issue's comments follow its body as second-level sections in order,
// and an issue without comments keeps the body without any.
// A blank body and paths under `.lym/` in the body or comments each get an import note and a result flag.
func TestImportIssueNewEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const linkLine = "Imported from [issue #7](https://example.test/issues/1)."
	const plainBody = linkLine + "\n\nIssue body."
	const pathsBody = "Compare `.lym/shuttle/abc/startup-capture.txt` with wt/.lym/shuttle/def/x.txt. Not _lym/discussion/r.md."
	const emphasisedPathsBody = `**.lym/a.txt** and |.lym/b|, key=.lym/c and .lym\shuttle\d.txt, foo.lym\e`
	const pathsComment = "See .lym/x.log, and again `.lym/shuttle/abc/startup-capture.txt`."
	tests := []struct {
		name           string
		req            boardengine.ImportRequest
		wantTitle      string
		wantBrief      string
		wantLabels     []string
		wantDropped    []string
		wantBody       string
		wantEmptyBody  bool
		wantLocalPaths []string
	}{
		{
			name:        "issue labels filtered to the vocabulary",
			req:         boardengine.ImportRequest{Issue: testIssue(7, "bug", "undecided", "stray"), Slug: "from-inbox"},
			wantTitle:   "Issue title",
			wantLabels:  []string{"bug", "undecided"},
			wantDropped: []string{"stray"},
			wantBody:    plainBody,
		},
		{
			name:       "explicit title, brief and labels win",
			req:        boardengine.ImportRequest{Issue: testIssue(7, "stray"), Slug: "x", Title: "Mine", Brief: "b", Labels: []string{"enhancement"}},
			wantTitle:  "Mine",
			wantBrief:  "b",
			wantLabels: []string{"enhancement"},
			wantBody:   plainBody,
		},
		{
			name:       "comments follow the body as second-level sections",
			req:        boardengine.ImportRequest{Issue: testIssueWithComments(7, "bug"), Slug: "commented"},
			wantTitle:  "Issue title",
			wantLabels: []string{"bug"},
			wantBody: plainBody +
				"\n\n## Issue comment, 2026-01-03\n\nFirst comment." +
				"\n\n## Issue comment, 2026-02-01\n\nSecond comment.\n\nWith a paragraph.",
		},
		{
			name:          "blank body flagged with the title",
			req:           boardengine.ImportRequest{Issue: testIssueWithBody(7, " \n"), Slug: "empty"},
			wantTitle:     "Issue title",
			wantLabels:    []string{"bug"},
			wantEmptyBody: true,
			wantBody:      linkLine + "\n\nImport note: the issue body was empty; the title \"Issue title\" and any comments below are all the issue carried.",
		},
		{
			name:           "emphasised, delimited and backslash local paths flagged",
			req:            boardengine.ImportRequest{Issue: testIssueWithBody(7, emphasisedPathsBody), Slug: "marked"},
			wantTitle:      "Issue title",
			wantLabels:     []string{"bug"},
			wantLocalPaths: []string{".lym/a.txt", ".lym/b", ".lym/c", `.lym\shuttle\d.txt`},
			wantBody: linkLine +
				"\n\nImport note: the issue names files under `.lym/`, which is deleted with its worktree, so only what the issue quotes of them reached the board:" +
				" `.lym/a.txt`, `.lym/b`, `.lym/c`, `.lym\\shuttle\\d.txt`." +
				"\n\n" + emphasisedPathsBody,
		},
		{
			name: "local paths in body and comments flagged",
			req: boardengine.ImportRequest{
				Issue: testIssueWithBody(7, pathsBody, boardengine.IssueComment{Body: pathsComment, CreatedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)}),
				Slug:  "paths",
			},
			wantTitle:      "Issue title",
			wantLabels:     []string{"bug"},
			wantLocalPaths: []string{".lym/shuttle/abc/startup-capture.txt", "wt/.lym/shuttle/def/x.txt", ".lym/x.log"},
			wantBody: linkLine +
				"\n\nImport note: the issue names files under `.lym/`, which is deleted with its worktree, so only what the issue quotes of them reached the board:" +
				" `.lym/shuttle/abc/startup-capture.txt`, `wt/.lym/shuttle/def/x.txt`, `.lym/x.log`." +
				"\n\n" + pathsBody + "\n\n## Issue comment, 2026-03-04\n\n" + pathsComment,
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
			if e.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", e.Body, tt.wantBody)
			}
			if !slices.Equal(res.Dropped, tt.wantDropped) {
				t.Errorf("dropped = %v, want %v", res.Dropped, tt.wantDropped)
			}
			if res.EmptyBody != tt.wantEmptyBody {
				t.Errorf("empty body = %v, want %v", res.EmptyBody, tt.wantEmptyBody)
			}
			if !slices.Equal(res.LocalPaths, tt.wantLocalPaths) {
				t.Errorf("local paths = %v, want %v", res.LocalPaths, tt.wantLocalPaths)
			}
		})
	}
}

func TestImportIssueFold(t *testing.T) {
	tiergate.Require(t, 0)
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

	mustUpsert(t, b, map[string]any{"slug": "commented", "kind": "task", "labels": bugLabels})
	res, err = b.ImportIssue(boardengine.ImportRequest{Issue: testIssueWithComments(6), Into: "commented"})
	if err != nil {
		t.Fatalf("fold an issue with comments: %v", err)
	}
	wantBody = "## From issue #6\n\nImported from [issue #6](https://example.test/issues/1).\n\nIssue body." +
		"\n\n### Issue comment, 2026-01-03\n\nFirst comment." +
		"\n\n### Issue comment, 2026-02-01\n\nSecond comment.\n\nWith a paragraph."
	if res.Entry.Body != wantBody {
		t.Errorf("body with comments = %q, want %q", res.Entry.Body, wantBody)
	}
	if res.EmptyBody || len(res.LocalPaths) != 0 {
		t.Errorf("plain fold flags = empty body %v, local paths %v, want none", res.EmptyBody, res.LocalPaths)
	}

	mustUpsert(t, b, map[string]any{"slug": "gaps", "kind": "task", "labels": bugLabels})
	gapIssue := testIssueWithBody(8, " ", boardengine.IssueComment{Body: "See .lym/x.log.", CreatedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)})
	res, err = b.ImportIssue(boardengine.ImportRequest{Issue: gapIssue, Into: "gaps"})
	if err != nil {
		t.Fatalf("fold an issue with a blank body and a local path: %v", err)
	}
	wantBody = "## From issue #8\n\nImported from [issue #8](https://example.test/issues/1)." +
		"\n\nImport note: the issue body was empty; the title \"Issue title\" and any comments below are all the issue carried." +
		"\n\nImport note: the issue names files under `.lym/`, which is deleted with its worktree, so only what the issue quotes of them reached the board: `.lym/x.log`." +
		"\n\n### Issue comment, 2026-03-04\n\nSee .lym/x.log."
	if res.Entry.Body != wantBody {
		t.Errorf("body with gaps = %q, want %q", res.Entry.Body, wantBody)
	}
	if !res.EmptyBody || !slices.Equal(res.LocalPaths, []string{".lym/x.log"}) {
		t.Errorf("gap fold flags = empty body %v, local paths %v, want true and [.lym/x.log]", res.EmptyBody, res.LocalPaths)
	}
}

func TestImportIssueFoldIntoStaleLabelRefused(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
