// intake_test.go covers the intake handlers against an httptest server through the selfreportengine.NewGitHubClient seam and a Board over a temp dir with SkipGit.
// No test reaches real token resolution or the network, and every refusal asserts that board.json is unchanged.

package boardcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
)

const fakeIssuesPath = "/repos/Knatte18/loomyard/issues"

// fakeIssue is one issue the fake inbox serves.
type fakeIssue struct {
	number      int
	state       string
	stateReason string
	pullRequest bool
	labels      []string
}

// fakeInbox is a fake GitHub inbox repository that records every mutation it receives.
type fakeInbox struct {
	issues    []fakeIssue
	failClose bool
	comments  []string
	closes    []string
	server    *httptest.Server
}

// mutations returns every comment and close the inbox received.
func (f *fakeInbox) mutations() []string {
	return append(append([]string{}, f.comments...), f.closes...)
}

func (f *fakeInbox) find(number int) (fakeIssue, bool) {
	for _, issue := range f.issues {
		if issue.number == number {
			return issue, true
		}
	}
	return fakeIssue{}, false
}

func (f *fakeInbox) issueJSON(issue fakeIssue) map[string]any {
	labels := []map[string]string{}
	for _, name := range issue.labels {
		labels = append(labels, map[string]string{"name": name})
	}
	view := map[string]any{
		"number":       issue.number,
		"title":        fmt.Sprintf("Issue %d", issue.number),
		"body":         fmt.Sprintf("Body %d.", issue.number),
		"html_url":     fmt.Sprintf("https://example.test/issues/%d", issue.number),
		"state":        issue.state,
		"state_reason": issue.stateReason,
		"labels":       labels,
		"created_at":   "2026-01-02T03:04:05Z",
	}
	if issue.pullRequest {
		view["pull_request"] = map[string]any{"url": "https://example.test/pr"}
	}
	return view
}

func (f *fakeInbox) handle(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, fakeIssuesPath)
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f.list(w, r)
	case strings.HasSuffix(rest, "/comments") && r.Method == http.MethodPost:
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.comments = append(f.comments, strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/comments")+": "+body["body"])
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	case r.Method == http.MethodPatch:
		if f.failClose {
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.closes = append(f.closes, strings.TrimPrefix(rest, "/")+": "+body["state_reason"])
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 1})
	case r.Method == http.MethodGet:
		number, _ := strconv.Atoi(strings.TrimPrefix(rest, "/"))
		issue, ok := f.find(number)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.issueJSON(issue))
	default:
		http.NotFound(w, r)
	}
}

// list serves the open issues in pages of two.
func (f *fakeInbox) list(w http.ResponseWriter, r *http.Request) {
	var open []fakeIssue
	for _, issue := range f.issues {
		if issue.state == "open" {
			open = append(open, issue)
		}
	}
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	const perPage = 2
	start := min((page-1)*perPage, len(open))
	end := min(start+perPage, len(open))
	if end < len(open) {
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d>; rel="next"`, f.server.URL, fakeIssuesPath, page+1))
	}
	views := []map[string]any{}
	for _, issue := range open[start:end] {
		views = append(views, f.issueJSON(issue))
	}
	_ = json.NewEncoder(w).Encode(views)
}

// newFakeInbox starts the fake inbox and installs it behind the NewGitHubClient seam.
func newFakeInbox(t *testing.T, issues ...fakeIssue) *fakeInbox {
	t.Helper()
	inbox := &fakeInbox{issues: issues}
	inbox.server = httptest.NewServer(http.HandlerFunc(inbox.handle))
	t.Cleanup(inbox.server.Close)

	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(inbox.server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed
	orig := selfreportengine.NewGitHubClient
	selfreportengine.NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { selfreportengine.NewGitHubClient = orig })
	return inbox
}

// newIntakeBoard returns a Board over a temp dir with a configured vocabulary and its board.json path.
func newIntakeBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := boardengine.Config{
		Path: dir, Readme: "Home.md", DesignPrefix: "proposal-",
		Types: []string{"bug", "enhancement"}, Labels: []string{"undecided"}, SkipGit: true,
	}
	return boardengine.New(cfg), filepath.Join(dir, "board.json")
}

// readBoard returns board.json's content, or "" when it does not exist yet.
func readBoard(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	return string(raw)
}

// decodeEnvelope decodes one handler output into a map.
func decodeEnvelope(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("parse output %q: %v", out.String(), err)
	}
	return envelope
}

// runHandler runs fn with a fresh buffer and returns the exit code and the decoded envelope.
func runHandler(t *testing.T, fn func(out *bytes.Buffer) int) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := fn(&out)
	return code, decodeEnvelope(t, &out)
}

func TestIntakeList_ExcludesPullRequestsRecordedAndClosedAcrossPages(t *testing.T) {
	newFakeInbox(t,
		fakeIssue{number: 1, state: "open"},
		fakeIssue{number: 2, state: "open", pullRequest: true},
		fakeIssue{number: 3, state: "closed"},
		fakeIssue{number: 4, state: "open"},
		fakeIssue{number: 5, state: "open"},
		fakeIssue{number: 6, state: "open", labels: []string{"bug"}},
	)
	b, _ := newIntakeBoard(t)
	if _, err := b.UpsertTask(map[string]any{"slug": "kept", "labels": []string{"bug"}, "issues": []int{4}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	code, envelope := runHandler(t, func(out *bytes.Buffer) int { return intakeList(out, b) })
	if code != 0 {
		t.Fatalf("exit %d: %v", code, envelope)
	}
	issues := envelope["issues"].([]any)
	var numbers []float64
	for _, v := range issues {
		numbers = append(numbers, v.(map[string]any)["number"].(float64))
	}
	if fmt.Sprint(numbers) != "[1 5 6]" {
		t.Fatalf("issues = %v, want [1 5 6]", numbers)
	}
	first := issues[0].(map[string]any)
	if first["title"] != "Issue 1" || first["body"] != "Body 1." || first["url"] != "https://example.test/issues/1" || first["created_at"] != "2026-01-02T03:04:05Z" {
		t.Errorf("first issue = %v, want its fields carried over", first)
	}
}

func TestIntakeImport_NewNoteCommentsAndCloses(t *testing.T) {
	inbox := newFakeInbox(t, fakeIssue{number: 7, state: "open", labels: []string{"bug", "stray"}})
	b, path := newIntakeBoard(t)

	code, envelope := runHandler(t, func(out *bytes.Buffer) int {
		return intakeImport(out, b, `{"issue":7,"slug":"from-inbox"}`)
	})
	if code != 0 {
		t.Fatalf("exit %d: %v", code, envelope)
	}
	entry := envelope["entry"].(map[string]any)
	if entry["slug"] != "from-inbox" || entry["kind"] != "note" {
		t.Errorf("entry = %v, want note from-inbox", entry)
	}
	if fmt.Sprint(envelope["dropped"]) != "[stray]" {
		t.Errorf("dropped = %v, want [stray]", envelope["dropped"])
	}
	if got := fmt.Sprint(inbox.comments); got != "[7: Imported into the board as `from-inbox`.]" {
		t.Errorf("comments = %v", inbox.comments)
	}
	if got := fmt.Sprint(inbox.closes); got != "[7: completed]" {
		t.Errorf("closes = %v, want completed", inbox.closes)
	}
	if readBoard(t, path) == "" {
		t.Error("board.json not written")
	}
}

func TestIntakeImport_FoldIntoEntry(t *testing.T) {
	inbox := newFakeInbox(t, fakeIssue{number: 8, state: "open"})
	b, _ := newIntakeBoard(t)
	if _, err := b.UpsertTask(map[string]any{"slug": "target", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	code, envelope := runHandler(t, func(out *bytes.Buffer) int {
		return intakeImport(out, b, `{"issue":8,"into":"target"}`)
	})
	if code != 0 {
		t.Fatalf("exit %d: %v", code, envelope)
	}
	entry := envelope["entry"].(map[string]any)
	if fmt.Sprint(entry["issues"]) != "[8]" || !strings.Contains(entry["body"].(string), "## From issue #8") {
		t.Errorf("entry = %v, want the issue folded in", entry)
	}
	if got := fmt.Sprint(inbox.comments); got != "[8: Imported into the board as `target`.]" {
		t.Errorf("comments = %v", inbox.comments)
	}
	if len(inbox.closes) != 1 {
		t.Errorf("closes = %v, want one close", inbox.closes)
	}
}

func TestIntakeImport_RecordedIssueIsNoOp(t *testing.T) {
	inbox := newFakeInbox(t, fakeIssue{number: 9, state: "open"})
	b, path := newIntakeBoard(t)
	if _, err := b.UpsertTask(map[string]any{"slug": "has-it", "labels": []string{"bug"}, "issues": []int{9}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := readBoard(t, path)

	code, envelope := runHandler(t, func(out *bytes.Buffer) int {
		return intakeImport(out, b, `{"issue":9,"slug":"again"}`)
	})
	if code != 0 {
		t.Fatalf("exit %d: %v", code, envelope)
	}
	if fmt.Sprint(envelope["recorded"]) != "[has-it]" {
		t.Errorf("recorded = %v, want [has-it]", envelope["recorded"])
	}
	if got := inbox.mutations(); len(got) != 0 {
		t.Errorf("GitHub mutations = %v, want none", got)
	}
	if readBoard(t, path) != before {
		t.Error("board.json changed on a no-op import")
	}
}

func TestIntakeImport_RefusedBeforeAnyWrite(t *testing.T) {
	cases := []struct {
		name    string
		issue   fakeIssue
		payload string
		want    string
	}{
		{"no usable type label", fakeIssue{number: 10, state: "open", labels: []string{"undecided"}}, `{"issue":10,"slug":"x"}`, "no type label"},
		{"taken slug", fakeIssue{number: 11, state: "open", labels: []string{"bug"}}, `{"issue":11,"slug":"taken"}`, "already exists"},
		{"missing into target", fakeIssue{number: 12, state: "open"}, `{"issue":12,"into":"ghost"}`, "no entry"},
		{"neither slug nor into", fakeIssue{number: 13, state: "open"}, `{"issue":13}`, "neither slug nor into"},
		{"unknown key", fakeIssue{number: 14, state: "open"}, `{"issue":14,"slug":"x","slugg":"y"}`, "unknown field"},
		{"pull request", fakeIssue{number: 15, state: "open", pullRequest: true}, `{"issue":15,"slug":"x"}`, "pull request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inbox := newFakeInbox(t, tc.issue)
			b, path := newIntakeBoard(t)
			if _, err := b.UpsertTask(map[string]any{"slug": "taken", "labels": []string{"bug"}}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			before := readBoard(t, path)

			code, envelope := runHandler(t, func(out *bytes.Buffer) int { return intakeImport(out, b, tc.payload) })
			if code != 1 || !strings.Contains(fmt.Sprint(envelope["error"]), tc.want) {
				t.Fatalf("exit %d, envelope %v, want an error containing %q", code, envelope, tc.want)
			}
			if got := inbox.mutations(); len(got) != 0 {
				t.Errorf("GitHub mutations = %v, want none", got)
			}
			if readBoard(t, path) != before {
				t.Error("board.json changed on a refused import")
			}
		})
	}
}

func TestIntakeImport_CloseFailureReportsEntryAndClosePayload(t *testing.T) {
	inbox := newFakeInbox(t, fakeIssue{number: 16, state: "open", labels: []string{"bug"}})
	inbox.failClose = true
	b, path := newIntakeBoard(t)

	code, envelope := runHandler(t, func(out *bytes.Buffer) int {
		return intakeImport(out, b, `{"issue":16,"slug":"half-done"}`)
	})
	if code != 1 || envelope["ok"] != false {
		t.Fatalf("exit %d, envelope %v, want a failure", code, envelope)
	}
	if envelope["entry"].(map[string]any)["slug"] != "half-done" {
		t.Errorf("entry = %v, want half-done", envelope["entry"])
	}
	closePayload := envelope["close"].(map[string]any)
	if closePayload["issue"] != 16.0 || closePayload["completed"] != true || closePayload["reason"] != "Imported into the board as `half-done`." {
		t.Errorf("close payload = %v", closePayload)
	}
	if readBoard(t, path) == "" {
		t.Error("board.json not written before the GitHub step")
	}
}

func TestIntakeClose(t *testing.T) {
	cases := []struct {
		name       string
		issue      fakeIssue
		payload    string
		want       string
		wantClosed string
	}{
		{"not planned by default", fakeIssue{number: 20, state: "open"}, `{"issue":20,"reason":"Noise."}`, "", "[20: not_planned]"},
		{"completed", fakeIssue{number: 21, state: "open"}, `{"issue":21,"reason":"Done.","completed":true}`, "", "[21: completed]"},
		{"no reason", fakeIssue{number: 22, state: "open"}, `{"issue":22}`, "reason", "[]"},
		{"empty reason", fakeIssue{number: 23, state: "open"}, `{"issue":23,"reason":""}`, "reason", "[]"},
		{"pull request", fakeIssue{number: 24, state: "open", pullRequest: true}, `{"issue":24,"reason":"x"}`, "pull request", "[]"},
		{"closed issue", fakeIssue{number: 25, state: "closed", stateReason: "completed"}, `{"issue":25,"reason":"x"}`, "already closed", "[]"},
		{"unknown key", fakeIssue{number: 26, state: "open"}, `{"issue":26,"reason":"x","why":"y"}`, "unknown field", "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inbox := newFakeInbox(t, tc.issue)
			_, path := newIntakeBoard(t)
			before := readBoard(t, path)

			code, envelope := runHandler(t, func(out *bytes.Buffer) int { return intakeClose(out, tc.payload) })
			if tc.want == "" {
				if code != 0 {
					t.Fatalf("exit %d: %v", code, envelope)
				}
			} else if code != 1 || !strings.Contains(fmt.Sprint(envelope["error"]), tc.want) {
				t.Fatalf("exit %d, envelope %v, want an error containing %q", code, envelope, tc.want)
			}
			if got := fmt.Sprint(inbox.closes); got != tc.wantClosed {
				t.Errorf("closes = %v, want %s", got, tc.wantClosed)
			}
			if readBoard(t, path) != before {
				t.Error("board.json changed by close")
			}
		})
	}
}

func TestIntakeClose_RecordedOpenIssue(t *testing.T) {
	inbox := newFakeInbox(t, fakeIssue{number: 30, state: "open"})
	b, path := newIntakeBoard(t)
	if _, err := b.UpsertTask(map[string]any{"slug": "has-it", "labels": []string{"bug"}, "issues": []int{30}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := readBoard(t, path)

	code, envelope := runHandler(t, func(out *bytes.Buffer) int {
		return intakeClose(out, `{"issue":30,"reason":"Imported into the board as `+"`has-it`"+`.","completed":true}`)
	})
	if code != 0 {
		t.Fatalf("exit %d: %v", code, envelope)
	}
	if got := fmt.Sprint(inbox.closes); got != "[30: completed]" {
		t.Errorf("closes = %v, want completed", got)
	}
	if readBoard(t, path) != before {
		t.Error("board.json changed by close")
	}
}
