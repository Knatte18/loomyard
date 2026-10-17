// publish_test.go covers Publish against a faked push closure recording its call order, and a faked GitHub client swapped in through the NewGitHubClient seam pointed at a local httptest server -- exactly the way internal/selfreportengine's own test does it.
// No test contacts a real service or a real model.
//
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// recordingResolver is the in-package fake standing in for the unexported resolver seam: it records
// whether Resolve was called and what source it was called with, and returns a scripted result/err.
type recordingResolver struct {
	called    bool
	gotSource string
	result    mergeresolve.Result
	err       error
}

func (r *recordingResolver) Resolve(ctx context.Context, source string) (mergeresolve.Result, error) {
	r.called = true
	r.gotSource = source
	return r.result, r.err
}

// newTestDeps is the package's one Deps builder, for Publish and Finalize tests alike: a base-branch list requiring a pull request, a well-formed final-summary artifact already written at DescriptionPath (Finalize's top-of-Call parse requires one; a Publish test rewrites it), and a scratch dir under t.TempDir().
// NewGitHubClient is swapped for a failing factory so no test reaches the real GitHub API;
// a test driving the client installs its own over it.
// Its parent lock seams are a temporary store and tip reads that equal a written merge record, so the lock-taking rows find the parent unmoved.
// A test needing a different variant mutates the returned value.
func newTestDeps(t *testing.T) Deps {
	t.Helper()
	installFailingGitHubClientFactory(t, errors.New("no GitHub client in this test"))
	summaryPath := summaryparser.Path(t.TempDir())
	writeSummary(t, summaryPath, "A landing title", "A landing body.")
	scratchDir := filepath.Join(t.TempDir(), "scratch")
	recordPath := MergeRecordPath(scratchDir)
	if err := WriteMergeRecord(recordPath, MergeRecord{ParentBranch: "main", MergedTip: testParentTip}); err != nil {
		t.Fatalf("write merge record: %v", err)
	}
	owner := parentlock.Owner{Slug: "me", Pair: "/p/me", RunID: "r1", PID: 222, StartTime: "t2"}
	return Deps{
		WorktreeRoot:    t.TempDir(),
		TaskBranch:      "task-branch",
		ParentBranch:    "main",
		DescriptionPath: summaryPath,
		RemoteOnlyCommits: func() (string, []string, error) {
			return "", nil, errors.New("no remote read in this test")
		},
		StencilsDir: t.TempDir(),
		ScratchDir:  scratchDir,
		OriginURL:   "https://github.com/acme/proj.git",
		Config: Config{
			RequirePRToBase:    []string{"main"},
			Squash:             true,
			Conflict:           "sonnet",
			ConflictTimeoutMin: 30,
			CoAuthoredBy:       "Test Author <test@example.com>",
		},
		Lock: ParentLockDeps{
			Store:    &parentlock.Store{Dir: filepath.Join(t.TempDir(), "locks"), Alive: func(int, string) bool { return true }},
			Owner:    func() (parentlock.Owner, error) { return owner, nil },
			WaitPath: parentlock.WaitRecordPath(scratchDir),
		},
		ReadParentTips: func(bool) (fabricengine.ParentTips, error) {
			return fabricengine.ParentTips{Local: testParentTip, Upstream: testParentTip, Relation: fabricengine.ParentEqual}, nil
		},
		HoldsWork:       func(string, string) (bool, error) { return false, nil },
		TaskHead:        func() (string, error) { return "task-head", nil },
		MergeRecordPath: recordPath,
	}
}

// testParentTip is the parent tip newTestDeps's merge record names and its tip reads report.
const testParentTip = "parent-tip"

// writeSummary writes a well-formed summary artifact at path.
func writeSummary(t *testing.T, path, title, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fmt.Sprintf("# %s\n\n%s\n", title, body)), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// publishGitHubServer is a scripted httptest server standing in for the GitHub API: it answers a pull-request list query, a create call and an edit call, appending "list"/"create"/"edit" to order (shared with the push closure's own "push" append) so a test can assert relative call ordering.
type publishGitHubServer struct {
	server *httptest.Server
	order  *[]string

	listStatus int
	listBody   string

	createStatus  int
	createBody    string
	createdBodies []map[string]any

	editStatus   int
	editBody     string
	editedBodies []map[string]any
}

func newPublishGitHubServer(t *testing.T, order *[]string) *publishGitHubServer {
	t.Helper()
	s := &publishGitHubServer{order: order, listStatus: http.StatusOK, listBody: "[]", createStatus: http.StatusCreated, editStatus: http.StatusOK, editBody: `{"number":7,"state":"open"}`}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			*order = append(*order, "list")
			w.WriteHeader(s.listStatus)
			_, _ = w.Write([]byte(s.listBody))
		case http.MethodPost:
			*order = append(*order, "create")
			raw, _ := jsonDecodeBody(r)
			s.createdBodies = append(s.createdBodies, raw)
			w.WriteHeader(s.createStatus)
			_, _ = w.Write([]byte(s.createBody))
		case http.MethodPatch:
			*order = append(*order, "edit")
			raw, _ := jsonDecodeBody(r)
			s.editedBodies = append(s.editedBodies, raw)
			w.WriteHeader(s.editStatus)
			_, _ = w.Write([]byte(s.editBody))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(s.server.Close)
	if s.createBody == "" {
		s.createBody = `{"number":1,"state":"open"}`
	}
	return s
}

func jsonDecodeBody(r *http.Request) (map[string]any, error) {
	var decoded map[string]any
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&decoded)
	return decoded, err
}

// install swaps NewGitHubClient with a closure returning a real go-github client pointed at s's
// server, restoring the original on test cleanup -- mirrors selfreportengine's installGitHubClient.
func (s *publishGitHubServer) install(t *testing.T) {
	t.Helper()
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(s.server.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed

	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// installFailingGitHubClientFactory replaces NewGitHubClient with a closure that itself fails.
func installFailingGitHubClientFactory(t *testing.T, err error) {
	t.Helper()
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return nil, err }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// requireReason returns the stuck reason carried on ptr, failing the test when it is empty.
func requireReason(t *testing.T, ptr shedengine.OutputPointer) string {
	t.Helper()
	if ptr.Reason == "" {
		t.Fatal("OutputPointer.Reason is empty; want the producer's stuck reason")
	}
	return ptr.Reason
}

// --- NewPublish construction ---

func TestNewPublish_Refusals(t *testing.T) {
	noPush := func() error { return nil }
	tests := []struct {
		name     string
		mutate   func(*Deps)
		wantName string
	}{
		{"nil PushBranch", func(d *Deps) {}, "Deps.PushBranch"},
		{"nil RemoteOnlyCommits", func(d *Deps) { d.PushBranch, d.RemoteOnlyCommits = noPush, nil }, "Deps.RemoteOnlyCommits"},
		{"empty DescriptionPath", func(d *Deps) { d.PushBranch, d.DescriptionPath = noPush, "" }, "Deps.DescriptionPath"},
		{"nil Lock.Store", func(d *Deps) { d.PushBranch, d.Lock.Store = noPush, nil }, "Deps.Lock.Store"},
		{"nil Lock.Owner", func(d *Deps) { d.PushBranch, d.Lock.Owner = noPush, nil }, "Deps.Lock.Owner"},
		{"nil ReadParentTips", func(d *Deps) { d.PushBranch, d.ReadParentTips = noPush, nil }, "Deps.ReadParentTips"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			tt.mutate(&deps)
			if _, err := NewPublish(deps); err == nil || !strings.Contains(err.Error(), tt.wantName) {
				t.Fatalf("NewPublish() error = %v; want an error naming %s", err, tt.wantName)
			}
		})
	}
}

// --- Call behaviour ---

// TestPublish_NoPullRequestRequired_DoneWithoutPush pins that a parent branch outside the require-PR list ends Done with no push, whether or not the push is skipped.
func TestPublish_NoPullRequestRequired_DoneWithoutPush(t *testing.T) {
	tests := []struct {
		name        string
		pushSkipped bool
	}{
		{"parent branch not in the base list", false},
		{"push skipped", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Config.RequirePRToBase = []string{"other-base"}
			deps.PushSkipped = tt.pushSkipped
			pushed := false
			deps.PushBranch = func() error { pushed = true; return nil }
			p := &Publish{deps: deps}

			shedfake.RequireOutcome(t, p, shedengine.Done)
			if pushed {
				t.Error("PushBranch was called; want no push when no pull request is required")
			}
		})
	}
}

// TestPublish_StuckBeforePullRequest pins each way Publish stops Stuck before it reaches a pull request:
// the reason it surfaces, whether it pushed, and that no stuck-reason file is written.
func TestPublish_StuckBeforePullRequest(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*Deps)
		pushErr      error
		wantInReason string
		// wantReason, when set, is the whole reason and replaces the substring check.
		wantReason     string
		wantPushCalled bool
	}{
		{
			name:         "push skipped while a pull request is required refuses before the push",
			mutate:       func(d *Deps) { d.PushSkipped = true },
			wantInReason: "push skipped",
		},
		{
			name:           "push failure",
			pushErr:        errors.New("boom"),
			wantReason:     "push failed: boom",
			wantPushCalled: true,
		},
		{
			name:           "exhausted push retries are a push failure, not a rejected push",
			pushErr:        fmt.Errorf("gitrepo: git push: %w", gitrepo.ErrPushRetriesExhausted),
			wantReason:     "push failed: gitrepo: git push: gitrepo: push retries exhausted",
			wantPushCalled: true,
		},
		{
			name:    "push rejected has its own reason naming the remote tip, the count and the resume",
			pushErr: gitrepo.ErrPushRejected,
			mutate: func(d *Deps) {
				d.RemoteOnlyCommits = func() (string, []string, error) { return "abc123", []string{"c2", "c1"}, nil }
			},
			wantReason:     "push rejected by the remote after the merge-in against parent branch \"main\"; the remote task branch is at abc123 and holds 2 commit(s) the local branch lacks; way forward: run `git merge origin/task-branch` in the task worktree, then resume the run with `lyx loom start`",
			wantPushCalled: true,
		},
		{
			name:    "push rejected with no remote-only commit names a remote rule and no merge",
			pushErr: gitrepo.ErrPushRejected,
			mutate: func(d *Deps) {
				d.RemoteOnlyCommits = func() (string, []string, error) { return "abc123", nil, nil }
			},
			wantReason:     "push rejected by the remote after the merge-in against parent branch \"main\"; the remote task branch holds no commit the local branch lacks, so a merge adds nothing and the remote rejected the push for its own rule, such as a hook; way forward: clear what the remote's rule objects to, then resume the run with `lyx loom start`",
			wantPushCalled: true,
		},
		{
			name:    "push rejected with no remote task branch names a remote rule and no merge",
			pushErr: gitrepo.ErrPushRejected,
			mutate: func(d *Deps) {
				d.RemoteOnlyCommits = func() (string, []string, error) { return "", nil, nil }
			},
			wantReason:     "push rejected by the remote after the merge-in against parent branch \"main\"; the remote has no task branch, so a merge adds nothing and the remote rejected the push for its own rule, such as a hook; way forward: clear what the remote's rule objects to, then resume the run with `lyx loom start`",
			wantPushCalled: true,
		},
		{
			name:    "push rejected with a failed remote read keeps the rejection and the way forward",
			pushErr: gitrepo.ErrPushRejected,
			mutate: func(d *Deps) {
				d.RemoteOnlyCommits = func() (string, []string, error) { return "", nil, errors.New("fetch failed") }
			},
			wantReason:     "push rejected by the remote after the merge-in against parent branch \"main\"; the remote tip could not be read: fetch failed; way forward: run `git merge origin/task-branch` in the task worktree, then resume the run with `lyx loom start`",
			wantPushCalled: true,
		},
		{
			name:           "empty origin URL",
			mutate:         func(d *Deps) { d.OriginURL = "" },
			wantInReason:   "origin URL unusable",
			wantPushCalled: true,
		},
		{
			name:           "origin URL that is not a URL",
			mutate:         func(d *Deps) { d.OriginURL = "not-a-url" },
			wantInReason:   "origin URL unusable",
			wantPushCalled: true,
		},
		{
			name:           "origin on a host that is not GitHub",
			mutate:         func(d *Deps) { d.OriginURL = "https://gitlab.com/acme/proj.git" },
			wantInReason:   "origin URL unusable",
			wantPushCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			pushed := false
			deps.PushBranch = func() error { pushed = true; return tt.pushErr }
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			p := &Publish{deps: deps}

			got := runAndGetReason(t, p)
			if tt.wantReason != "" {
				if got != tt.wantReason {
					t.Errorf("reason = %q; want %q", got, tt.wantReason)
				}
			} else if !strings.Contains(got, tt.wantInReason) {
				t.Errorf("reason = %q; want it to carry %q", got, tt.wantInReason)
			}
			if pushed != tt.wantPushCalled {
				t.Errorf("PushBranch called = %v; want %v", pushed, tt.wantPushCalled)
			}
			matches, err := filepath.Glob(filepath.Join(deps.ScratchDir, "*-stuck.md"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Errorf("stuck-reason files = %v; want none", matches)
			}
		})
	}
}

func runAndGetReason(t *testing.T, p *Publish) string {
	t.Helper()
	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	return requireReason(t, ptr)
}

// TestPublish_GitHubFailures pins that each GitHub failure site ends with its usual verdict -- Stuck for a client the factory cannot build or a non-transient API error, a classified transient error for a 5xx -- and that the site's logger.Warn line carries the action and cause field keys, plus owner and repo wherever a request was made.
// The cases that fail through the API are the ones reason-classification itself is exercised for, in TestPublishGitHubErrorReason_ClassifiesDistinctly.
func TestPublish_GitHubFailures(t *testing.T) {
	tests := []struct {
		name          string
		clientFails   bool
		setup         func(s *publishGitHubServer)
		wantTransient bool
		wantFields    []string
	}{
		{name: "client unavailable", clientFails: true, wantFields: []string{"WARN", "action=", "cause="}},
		{
			name: "query rejected", wantFields: []string{"WARN", "action=", "owner=", "repo=", "cause="},
			setup: func(s *publishGitHubServer) {
				s.listStatus = http.StatusUnprocessableEntity
				s.listBody = `{"message":"server exploded"}`
			},
		},
		{
			name: "create rejected", wantFields: []string{"WARN", "action=", "owner=", "repo=", "cause="},
			setup: func(s *publishGitHubServer) {
				s.createStatus = http.StatusUnprocessableEntity
				s.createBody = `{"message":"server exploded"}`
			},
		},
		{
			name: "query 503", wantTransient: true, wantFields: []string{"WARN", "action=", "owner=", "repo=", "cause="},
			setup: func(s *publishGitHubServer) {
				s.listStatus = http.StatusServiceUnavailable
				s.listBody = `{"message":"unavailable"}`
			},
		},
		{
			name: "create 502", wantTransient: true, wantFields: []string{"WARN", "action=", "owner=", "repo=", "cause="},
			setup: func(s *publishGitHubServer) {
				s.createStatus = http.StatusBadGateway
				s.createBody = `{"message":"bad gateway"}`
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			writeSummary(t, deps.DescriptionPath, "Title", "Body.")
			var order []string
			deps.PushBranch = func() error { order = append(order, "push"); return nil }
			p := &Publish{deps: deps}
			if tt.clientFails {
				installFailingGitHubClientFactory(t, errors.New("boom"))
			} else {
				srv := newPublishGitHubServer(t, &order)
				tt.setup(srv)
				srv.install(t)
			}
			buf := logcapture.Capture(t)

			if tt.wantTransient {
				_, _, err := p.Call(context.Background())
				if err == nil {
					t.Fatal("Call() error = nil; want a transient error")
				}
				if got := shedtransient.Class(err); got != shedengine.TransientGitHubAPI {
					t.Errorf("Class(err) = %q; want %q", got, shedengine.TransientGitHubAPI)
				}
			} else {
				runAndGetReason(t, p)
			}

			logged := buf.String()
			for _, field := range tt.wantFields {
				if !strings.Contains(logged, field) {
					t.Errorf("log output = %q; want %s", logged, field)
				}
			}
		})
	}
}

// TestPublish_NoExistingPR_CreatesAndLogsOnce pins the create path: push, then list, then create, with the change description as title and body, and the one GitHub write landing in the durable trace exactly once carrying the created pull request's number.
// It never runs in parallel: the sink override is package-level state.
func TestPublish_NoExistingPR_CreatesAndLogsOnce(t *testing.T) {
	logger.SetDurableSinkDir(t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	deps := newTestDeps(t)
	writeSummary(t, deps.DescriptionPath, "My PR Title", "My PR body.")
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	p := &Publish{deps: deps}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	shedfake.RequireOutcome(t, p, shedengine.Done)

	if got, want := strings.Join(order, ","), "push,list,create"; got != want {
		t.Errorf("call order = %q; want %q", got, want)
	}
	if len(srv.createdBodies) != 1 {
		t.Fatalf("created pull requests = %d; want 1", len(srv.createdBodies))
	}
	got := srv.createdBodies[0]
	if got["title"] != "My PR Title" {
		t.Errorf("created PR title = %v; want %q", got["title"], "My PR Title")
	}
	if got["body"] != "\nMy PR body.\n" {
		t.Errorf("created PR body = %v; want %q", got["body"], "\nMy PR body.\n")
	}

	data, err := os.ReadFile(logger.TraceFile())
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	var created []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, `msg="landingshed: pull request created"`) {
			created = append(created, line)
		}
	}
	if len(created) != 1 {
		t.Fatalf("pull request created records = %d; want 1\n%s", len(created), data)
	}
	if !strings.Contains(created[0], "number=1") {
		t.Errorf("record = %q; want number=1", created[0])
	}
}

// TestPublish_OpenPullRequest pins what Publish does when an open pull request already exists: it edits the title and body only when they differ from the change description, never creates, and treats a 5xx edit failure as a classified transient error but any other as Stuck.
func TestPublish_OpenPullRequest(t *testing.T) {
	tests := []struct {
		name       string
		prTitle    string
		prBody     string
		createdAt  string
		runStart   time.Time
		editStatus int
		wantEdits  int
		wantOut    shedengine.Outcome
		wantClass  shedengine.TransientClass
	}{
		{"differing description is edited", "Old Title", "Old body.", "", time.Time{}, http.StatusOK, 1, shedengine.Done, ""},
		{"matching description is left alone", "New Title", "\nNew body.\n", "", time.Time{}, http.StatusOK, 0, shedengine.Done, ""},
		{"502 edit failure is an error", "Old", "Old", "", time.Time{}, http.StatusBadGateway, 1, "", shedengine.TransientGitHubAPI},
		{"422 edit failure stays Stuck", "Old", "Old", "", time.Time{}, http.StatusUnprocessableEntity, 1, shedengine.Stuck, ""},
		{"created after the run started is edited", "Old Title", "Old body.", "2026-07-01T00:00:00Z", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), http.StatusOK, 1, shedengine.Done, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.RunStart = tt.runStart
			writeSummary(t, deps.DescriptionPath, "New Title", "New body.")
			var order []string
			deps.PushBranch = func() error { order = append(order, "push"); return nil }
			p := &Publish{deps: deps}
			srv := newPublishGitHubServer(t, &order)
			// No "merged"/"merged_at" key at all: the real List Pull Requests endpoint never sets "merged"
			// on an open PR and reports "merged_at": null (crucible round 3, F-R3-1) -- an absent key decodes
			// to the same nil field either way.
			entry := map[string]any{"number": 7, "state": "open", "title": tt.prTitle, "body": tt.prBody}
			if tt.createdAt != "" {
				entry["created_at"] = tt.createdAt
			}
			list, err := json.Marshal([]map[string]any{entry})
			if err != nil {
				t.Fatalf("marshal list body: %v", err)
			}
			srv.listBody = string(list)
			srv.editStatus = tt.editStatus
			if tt.editStatus != http.StatusOK {
				srv.editBody = `{"message":"nope"}`
			}
			srv.install(t)

			outcome, ptr, err := p.Call(context.Background())
			switch {
			case tt.wantClass != "":
				if got := shedtransient.Class(err); got != tt.wantClass {
					t.Errorf("Class(err) = %q; want %q (err = %v)", got, tt.wantClass, err)
				}
			case err != nil || outcome != tt.wantOut:
				t.Fatalf("Call() = %q, %v; want %q, nil", outcome, err, tt.wantOut)
			case tt.wantOut == shedengine.Stuck:
				requireReason(t, ptr)
			}
			if len(srv.createdBodies) != 0 {
				t.Errorf("created pull requests = %d; want 0 (an open PR already exists)", len(srv.createdBodies))
			}
			if len(srv.editedBodies) != tt.wantEdits {
				t.Fatalf("edit requests = %d; want %d", len(srv.editedBodies), tt.wantEdits)
			}
			if tt.wantEdits == 1 {
				got := srv.editedBodies[0]
				if got["title"] != "New Title" || got["body"] != "\nNew body.\n" {
					t.Errorf("edit request = %v; want the new title and body", got)
				}
			}
		})
	}
}

// TestPublish_ExistingClosedPullRequest pins the verdict for a closed pull request: a merged one is Done, found through "merged_at" because GitHub's List Pull Requests endpoint never populates the "merged" boolean on any item (confirmed live against a real merged PR -- only the single-PR Get endpoint reports "merged":true, crucible round 3's F-R3-1);
// an unmerged one is Stuck with a reason naming the closure, ending with the pull request's URL when it carries one.
// The list bodies are deliberately the real List response shape, so a mock the real API would never produce cannot pass here.
func TestPublish_ExistingClosedPullRequest(t *testing.T) {
	const prURL = "https://github.com/owner/repo/pull/7"
	runStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		listBody    string
		runStart    time.Time
		wantOut     shedengine.Outcome
		wantReason  string
		wantCreates int
	}{
		{"merged", `[{"number":7,"state":"closed","merged_at":"2026-09-13T16:49:32Z"}]`, time.Time{}, shedengine.Done, "", 0},
		{"unmerged without a URL", `[{"number":7,"state":"closed"}]`, time.Time{}, shedengine.Stuck, "the pull request was closed without being merged", 0},
		{"unmerged with a URL", `[{"number":7,"state":"closed","html_url":"` + prURL + `"}]`, time.Time{}, shedengine.Stuck, "the pull request was closed without being merged: " + prURL, 0},
		{"older unmerged is replaced", `[{"number":7,"state":"closed","created_at":"2026-01-01T00:00:00Z"}]`, runStart, shedengine.Done, "", 1},
		{"older merged is replaced", `[{"number":7,"state":"closed","created_at":"2026-01-01T00:00:00Z","merged_at":"2026-01-02T00:00:00Z"}]`, runStart, shedengine.Done, "", 1},
		{"unmerged created after the run started", `[{"number":7,"state":"closed","created_at":"2026-07-01T00:00:00Z"}]`, runStart, shedengine.Stuck, "the pull request was closed without being merged", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.PushBranch = func() error { return nil }
			deps.RunStart = tt.runStart
			p := &Publish{deps: deps}

			var order []string
			srv := newPublishGitHubServer(t, &order)
			srv.listBody = tt.listBody
			srv.install(t)

			ptr := shedfake.RequireOutcome(t, p, tt.wantOut)
			if tt.wantOut == shedengine.Stuck && ptr.Reason != tt.wantReason {
				t.Errorf("reason = %q; want %q", ptr.Reason, tt.wantReason)
			}
			if len(srv.createdBodies) != tt.wantCreates {
				t.Errorf("created pull requests = %d; want %d", len(srv.createdBodies), tt.wantCreates)
			}
		})
	}
}

func TestPublish_MissingSummary_FailsLoudlyNoCreate(t *testing.T) {
	deps := newTestDeps(t)
	if err := os.Remove(deps.DescriptionPath); err != nil {
		t.Fatalf("remove summary.md: %v", err)
	}
	var order []string
	deps.PushBranch = func() error { order = append(order, "push"); return nil }
	p := &Publish{deps: deps}

	srv := newPublishGitHubServer(t, &order)
	srv.install(t)

	outcome, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want an error for a missing change description")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a hard error", outcome)
	}
	if len(srv.createdBodies) != 0 {
		t.Errorf("created pull requests = %d; want 0 on a missing summary", len(srv.createdBodies))
	}
}

func TestPublish_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	deps := newTestDeps(t)
	p := &Publish{deps: deps}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error on entry cancellation")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a cancelled entry", outcome)
	}
}

// --- publishGitHubErrorReason classification ---

func TestPublishGitHubErrorReason_ClassifiesDistinctly(t *testing.T) {
	tokenReason := publishGitHubErrorReason("query", githubclient.ErrTokenUnresolvable)
	apiReason := publishGitHubErrorReason("query", &github.ErrorResponse{Message: "nope"})
	networkReason := publishGitHubErrorReason("query", errors.New("connection refused"))

	if tokenReason == apiReason || tokenReason == networkReason || apiReason == networkReason {
		t.Errorf("classification reasons are not all distinct: token=%q api=%q network=%q", tokenReason, apiReason, networkReason)
	}
}

// TestFindPullRequest_QueryParametersAndEmptyList pins the one list query and how the run's start splits the listed pull requests into the run's own and the newest older open one.
func TestFindPullRequest_QueryParametersAndEmptyList(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		listBody      string
		since         time.Time
		wantPR        int
		wantOlderOpen int
	}{
		{"empty list", `[]`, since, 0, 0},
		{"zero since returns an older closed one", `[{"number":3,"state":"closed","created_at":"2026-01-01T00:00:00Z"}]`, time.Time{}, 3, 0},
		{"only an older closed one", `[{"number":3,"state":"closed","created_at":"2026-01-01T00:00:00Z"}]`, since, 0, 0},
		{"newer closed above an older open one", `[{"number":5,"state":"closed","created_at":"2026-07-01T00:00:00Z"},{"number":3,"state":"open","created_at":"2026-01-01T00:00:00Z"}]`, since, 5, 3},
		{"older closed above an older open one", `[{"number":4,"state":"closed","created_at":"2026-02-01T00:00:00Z"},{"number":3,"state":"open","created_at":"2026-01-01T00:00:00Z"}]`, since, 0, 3},
		{"two older open ones", `[{"number":4,"state":"open","created_at":"2026-02-01T00:00:00Z"},{"number":3,"state":"open","created_at":"2026-01-01T00:00:00Z"}]`, since, 0, 4},
		{"created exactly at since", `[{"number":6,"state":"open","created_at":"2026-06-01T00:00:00Z"}]`, since, 6, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				_, _ = w.Write([]byte(tt.listBody))
			}))
			t.Cleanup(srv.Close)

			client := github.NewClient(nil).WithAuthToken("test-token")
			parsed, err := url.Parse(srv.URL + "/")
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}
			client.BaseURL = parsed

			pr, olderOpen, err := FindPullRequest(context.Background(), client, "o", "r", "task", "parent", tt.since)
			if err != nil {
				t.Fatalf("FindPullRequest: %v", err)
			}
			if pr.GetNumber() != tt.wantPR || (pr == nil) != (tt.wantPR == 0) {
				t.Errorf("pr = %v; want number %d", pr, tt.wantPR)
			}
			if olderOpen.GetNumber() != tt.wantOlderOpen || (olderOpen == nil) != (tt.wantOlderOpen == 0) {
				t.Errorf("olderOpen = %v; want number %d", olderOpen, tt.wantOlderOpen)
			}
			want := map[string]string{"state": "all", "head": "o:task", "base": "parent", "sort": "created", "direction": "desc"}
			for k, v := range want {
				if got.Get(k) != v {
					t.Errorf("query %s = %q, want %q", k, got.Get(k), v)
				}
			}
		})
	}
}

// transportPushErr is the error a push returns when the remote is unreachable: a *gitexec.GitError whose stderr shedtransient classifies as git-transport.
func transportPushErr() error {
	return fmt.Errorf("gitrepo: git push: %w", &gitexec.GitError{
		Args:     []string{"-c", "push.autoSetupRemote=true", "push"},
		ExitCode: 128,
		Stderr:   "fatal: unable to access '...': Failed to connect to 127.0.0.1 port 1",
	})
}

func TestPublish_TransportPushFailure(t *testing.T) {
	tests := []struct {
		name string
		// cancelDuringPush cancels the context inside the push, so the transport failure arrives with the run already cancelled.
		cancelDuringPush bool
	}{
		{"is a classified transient error and reaches no GitHub", false},
		{"under a cancelled context is cancellation, not transient", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			var order []string
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deps.PushBranch = func() error {
				if tt.cancelDuringPush {
					cancel()
				}
				return transportPushErr()
			}
			p := &Publish{deps: deps}
			srv := newPublishGitHubServer(t, &order)
			srv.install(t)

			_, _, err := p.Call(ctx)
			if tt.cancelDuringPush {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Call() error = %v; want it to wrap context.Canceled", err)
				}
				if got := shedtransient.Class(err); got != "" {
					t.Errorf("Class(err) = %q; want empty", got)
				}
				return
			}
			if err == nil {
				t.Fatal("Call() error = nil; want a transient error")
			}
			if got := shedtransient.Class(err); got != shedengine.TransientGitTransport {
				t.Errorf("Class(err) = %q; want %q", got, shedengine.TransientGitTransport)
			}
			if len(order) != 0 {
				t.Errorf("GitHub calls = %v; want none", order)
			}
		})
	}
}
