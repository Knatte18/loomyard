// finalize_test.go covers Finalize against a faked resolver and a faked parent-pair opener closure returning a scripted merge outcome.
// This tier lives in the package itself too, for the same reason as its sibling: the resolver seam and the parentMerger seam it substitutes are both unexported.
// None of its tests runs in parallel: each builds its Deps through newTestDeps, which swaps the package-level NewGitHubClient.

package landingshed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// recordingParentMerger is the in-package fake standing in for the unexported parentMerger seam. It
// records every call's source/opts, and returns the scripted result/err at position len(calls)
// (clamped to the last entry), so a test can script a first-attempt failure followed by a
// second-attempt success.
type recordingParentMerger struct {
	calls   []mergeCall
	results []mergeCallResult
	// pushCalls records every PushBranch call's options; pushErr is returned by each of them.
	pushCalls []fabricengine.SyncOptions
	pushErr   error
	// headSHA and headErr script HeadSHA's return.
	headSHA string
	headErr error
	// order, when non-nil, receives "merge" and "push" as those calls happen, so a test can
	// assert their order against other seams that append to the same slice.
	order *[]string
}

type mergeCall struct {
	source string
	opts   fabricengine.MergeOptions
}

type mergeCallResult struct {
	result fabricengine.MergeResult
	err    error
}

func (m *recordingParentMerger) Merge(source string, opts fabricengine.MergeOptions) (fabricengine.MergeResult, error) {
	idx := len(m.calls)
	m.calls = append(m.calls, mergeCall{source: source, opts: opts})
	if m.order != nil {
		*m.order = append(*m.order, "merge")
	}
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	if idx < 0 {
		return fabricengine.MergeResult{}, nil
	}
	return m.results[idx].result, m.results[idx].err
}

func (m *recordingParentMerger) PushBranch(opts fabricengine.SyncOptions) (fabricengine.PushResult, error) {
	m.pushCalls = append(m.pushCalls, opts)
	if m.order != nil {
		*m.order = append(*m.order, "push")
	}
	return fabricengine.PushResult{}, m.pushErr
}

func (m *recordingParentMerger) HeadSHA() (string, error) { return m.headSHA, m.headErr }

// requireFinalizeReason returns the stuck reason carried on ptr, failing the test when it is empty.
func requireFinalizeReason(t *testing.T, ptr shedengine.OutputPointer) string {
	t.Helper()
	if ptr.Reason == "" {
		t.Fatal("OutputPointer.Reason is empty; want the producer's stuck reason")
	}
	return ptr.Reason
}

// --- NewFinalize construction ---

func TestNewFinalize_Refusals(t *testing.T) {
	nilFabric := func() (*fabricengine.Fabric, error) { return nil, nil }
	cases := []struct {
		name     string
		mutate   func(*Deps)
		wantName string
	}{
		{"nil OpenFabric", func(d *Deps) { d.OpenParentFabric = nilFabric }, "Deps.OpenFabric"},
		{"nil OpenParentFabric", func(d *Deps) { d.OpenFabric = nilFabric }, "Deps.OpenParentFabric"},
		{"empty DescriptionPath", func(d *Deps) { d.OpenFabric, d.OpenParentFabric, d.DescriptionPath = nilFabric, nilFabric, "" }, "Deps.DescriptionPath"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps(t)
			tc.mutate(&deps)
			if _, err := NewFinalize(deps); err == nil || !strings.Contains(err.Error(), tc.wantName) {
				t.Fatalf("NewFinalize() error = %v; want an error naming %s", err, tc.wantName)
			}
		})
	}
}

// --- Call behaviour ---

// TestFinalize_MergeMarkPushOrderAndVerdict pins that a landed merge is marked done on the board and then pushed to the parent's upstream before Done, honouring Deps.PushSkipped, that a failed push is Stuck rather than Done, and that neither a failing push nor a failing board seam changes what the other is called for.
func TestFinalize_MergeMarkPushOrderAndVerdict(t *testing.T) {
	tests := []struct {
		name        string
		pushSkipped bool
		pushErr     error
		markErr     error
		wantOutcome shedengine.Outcome
	}{
		{"pushed", false, nil, nil, shedengine.Done},
		{"push skipped", true, nil, nil, shedengine.Done},
		{"failing push still marks", false, errors.New("remote rejected"), nil, shedengine.Stuck},
		{"erroring board seam still Done", false, nil, errors.New("board unavailable"), shedengine.Done},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.PushSkipped = tt.pushSkipped
			deps.MarkTaskDone = func() error {
				order = append(order, "mark")
				return tt.markErr
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
				pushErr: tt.pushErr,
				order:   &order,
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if got, want := strings.Join(order, ","), "merge,mark,push"; got != want {
				t.Errorf("call order = %q; want %q", got, want)
			}
			if len(merger.pushCalls) != 1 {
				t.Fatalf("PushBranch calls = %d; want 1", len(merger.pushCalls))
			}
			if got := merger.pushCalls[0].SkipPush; got != tt.pushSkipped {
				t.Errorf("PushBranch SkipPush = %v; want %v", got, tt.pushSkipped)
			}
			if tt.pushErr != nil {
				if got := requireFinalizeReason(t, ptr); !strings.Contains(got, "push it by hand") {
					t.Errorf("reason = %q; want it to tell the operator to push by hand", got)
				}
			}
		})
	}
}

// TestFinalize_MergeOptionsCarriesComposedMessage asserts the merge-in runs first and a successful parent-side merge passes MergeOptions carrying the task branch as its source, both the composed final-summary message and the configured Squash value, for both merge shapes -- the message is set whether or not Squash is true.
//
//testtiming:keep pins the composed MergeOptions.Message, its Co-Authored-By trailer and the Squash value for both merge shapes, which its covering tests do not assert
func TestFinalize_MergeOptionsCarriesComposedMessage(t *testing.T) {
	tests := []struct {
		name   string
		squash bool
	}{
		{"Squash", true},
		{"NoSquash", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Config.Squash = tt.squash
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			if !res.called {
				t.Error("resolver.Resolve was not called; want merge-in to run first")
			}
			if len(merger.calls) != 1 {
				t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
			}
			if merger.calls[0].source != deps.TaskBranch {
				t.Errorf("parent-side merge source = %q; want %q", merger.calls[0].source, deps.TaskBranch)
			}
			got := merger.calls[0].opts
			want := summary.LandingMessage(deps.Config.CoAuthoredBy)
			if got.Message != want {
				t.Errorf("MergeOptions.Message = %q; want %q", got.Message, want)
			}
			if trailer := "Co-Authored-By: " + deps.Config.CoAuthoredBy; !strings.HasSuffix(got.Message, trailer) {
				t.Errorf("MergeOptions.Message = %q; want suffix %q", got.Message, trailer)
			}
			if got.Squash != tt.squash {
				t.Errorf("MergeOptions.Squash = %v; want %v (threaded from configuration)", got.Squash, tt.squash)
			}
		})
	}
}

// TestFinalize_UnusableSummaryArtifact_ErrorBeforeMergeOrCommit asserts a missing or malformed final-summary artifact makes Call return an error with no merge attempted and no status commit performed -- proving the top-of-Call parse runs before either.
func TestFinalize_UnusableSummaryArtifact_ErrorBeforeMergeOrCommit(t *testing.T) {
	tests := []struct {
		name   string
		damage func(t *testing.T, path string)
	}{
		{"missing", func(t *testing.T, _ string) {}},
		{"malformed", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("not a heading\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(malformed summary): %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
			tt.damage(t, deps.DescriptionPath)
			committed := false
			deps.CommitStatus = func() error { committed = true; return nil }
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, _, err := fz.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want an error for an unusable change description")
			}
			if outcome != "" {
				t.Errorf("Call() outcome = %q; want empty on a hard error", outcome)
			}
			if len(merger.calls) != 0 {
				t.Errorf("parent-side merge calls = %d; want 0 -- the parse runs before any merge", len(merger.calls))
			}
			if committed {
				t.Error("CommitStatus was called; want the parse to run before the status commit")
			}
		})
	}
}

// TestFinalize_MergeInRequiredRetry pins that an ErrMergeInRequired merge is retried exactly once with the same composed message, and that the board seam runs once, after the retry that lands, never after a failed attempt.
func TestFinalize_MergeInRequiredRetry(t *testing.T) {
	tests := []struct {
		name        string
		secondTry   mergeCallResult
		wantOutcome shedengine.Outcome
		wantOrder   string
	}{
		{"retry lands", mergeCallResult{result: fabricengine.MergeResult{Committed: true}}, shedengine.Done, "merge,merge,mark,push"},
		{"retry also required stays Stuck", mergeCallResult{err: &fabricengine.ErrMergeInRequired{}}, shedengine.Stuck, "merge,merge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newTestDeps(t)
			deps.MarkTaskDone = func() error { order = append(order, "mark"); return nil }
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}}, tt.secondTry},
				order:   &order,
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			ptr := shedfake.RequireOutcome(t, fz, tt.wantOutcome)
			if len(merger.calls) != 2 {
				t.Fatalf("parent-side merge calls = %d; want exactly 2 (one attempt, one retry)", len(merger.calls))
			}
			want := summary.LandingMessage(deps.Config.CoAuthoredBy)
			for i, call := range merger.calls {
				if call.opts.Message != want {
					t.Errorf("attempt %d MergeOptions.Message = %q; want %q (the same composed message every attempt)", i+1, call.opts.Message, want)
				}
			}
			if got := strings.Join(order, ","); got != tt.wantOrder {
				t.Errorf("call order = %q; want %q", got, tt.wantOrder)
			}
			if tt.wantOutcome == shedengine.Stuck {
				requireFinalizeReason(t, ptr)
			}
		})
	}
}

// TestFinalize_StuckReasons pins, for each way a landing stops Stuck, the reason it surfaces, that the board seam is never called, and that no stuck-reason file is written.
func TestFinalize_StuckReasons(t *testing.T) {
	guardErr := &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}}
	tests := []struct {
		name       string
		resolved   mergeresolve.Result
		openerErr  error
		mergeErr   error
		wantMerges int
		// wantInReason names what the stuck reason must carry, given the deps the case ran under.
		wantInReason func(d Deps) string
	}{
		{
			name:         "parent opener error names the parent branch",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			openerErr:    errors.New("no such worktree"),
			wantInReason: func(d Deps) string { return d.ParentBranch },
		},
		{
			name:         "dirty-worktree guard error surfaces verbatim and is not retried",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:     guardErr,
			wantMerges:   1,
			wantInReason: func(Deps) string { return guardErr.Error() },
		},
		{
			name:         "unrecognized merge error surfaces",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			mergeErr:     errors.New("some unrecognized failure"),
			wantMerges:   1,
			wantInReason: func(Deps) string { return "some unrecognized failure" },
		},
		{
			name:         "merge-in stuck surfaces the resolver's reason and never reaches the parent-side merge",
			resolved:     mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"},
			wantInReason: func(Deps) string { return "conflict could not be resolved" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			markCalled := false
			deps.MarkTaskDone = func() error { markCalled = true; return nil }
			res := &recordingResolver{result: tt.resolved}
			merger := &recordingParentMerger{}
			if tt.mergeErr != nil {
				merger.results = []mergeCallResult{{err: tt.mergeErr}}
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) {
				if tt.openerErr != nil {
					return nil, tt.openerErr
				}
				return merger, nil
			}}

			ptr := shedfake.RequireOutcome(t, fz, shedengine.Stuck)
			got := requireFinalizeReason(t, ptr)
			if want := tt.wantInReason(deps); !strings.Contains(got, want) {
				t.Errorf("stuck reason %q does not carry %q", got, want)
			}
			if len(merger.calls) != tt.wantMerges {
				t.Errorf("parent-side merge calls = %d; want %d", len(merger.calls), tt.wantMerges)
			}
			if markCalled {
				t.Error("MarkTaskDone was called on a stuck landing; want it never called")
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

func TestFinalize_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{}
	fz := &Finalize{deps: deps, resolver: res}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, _, err := fz.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error on entry cancellation")
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty on a cancelled entry", outcome)
	}
}

// --- Closing the pull request after landing ---

// closeServer records every request the GitHub fake receives as "METHOD path" plus decoded body.
type closeServer struct {
	requests []string
	bodies   []map[string]any
	listBody string
	// failClose makes the PATCH (close) call answer 500.
	failClose bool
}

func installCloseServer(t *testing.T, s *closeServer) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.bodies = append(s.bodies, body)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(s.listBody))
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case http.MethodPatch:
			if s.failClose {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			_, _ = w.Write([]byte(`{"number":7,"state":"closed"}`))
		}
	}))
	t.Cleanup(srv.Close)
	client := github.NewClient(nil).WithAuthToken("test-token")
	parsed, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client.BaseURL = parsed
	orig := NewGitHubClient
	NewGitHubClient = func() (*github.Client, error) { return client, nil }
	t.Cleanup(func() { NewGitHubClient = orig })
}

// TestFinalize_ClosingPullRequestAfterLanding pins what a landed task does to its pull request:
// an open one gets a comment naming the landing head and the parent branch and is closed, a merge that reports AlreadyUpToDate counts as landed and names the head the parent reports, and a non-open pull request, a failed close, a parent needing no pull request and a skipped push each still end Done.
// Every case marks the task done and calls the push once.
func TestFinalize_ClosingPullRequestAfterLanding(t *testing.T) {
	lookupOnly := []string{"GET /repos/acme/widgets/pulls"}
	lookupCommentClose := []string{"GET /repos/acme/widgets/pulls", "POST /repos/acme/widgets/issues/7/comments", "PATCH /repos/acme/widgets/pulls/7"}
	committed := fabricengine.MergeResult{Committed: true}
	tests := []struct {
		name         string
		listBody     string
		failClose    bool
		mutate       func(*Deps)
		merge        fabricengine.MergeResult
		headSHA      string
		wantRequests []string
		// wantCommentHas names what the comment on the pull request carries; empty when no comment is expected.
		wantCommentHas []string
	}{
		{"closes the open pull request", `[{"number":7,"state":"open"}]`, false, nil, committed, "abc123landed", lookupCommentClose, []string{"abc123landed", "main"}},
		{"already up to date counts as landed", `[{"number":7,"state":"open"}]`, false, nil, fabricengine.MergeResult{AlreadyUpToDate: true}, "parentheadsha", lookupCommentClose, []string{"parentheadsha"}},
		{"leaves a non-open pull request alone", `[{"number":7,"state":"closed"}]`, false, nil, committed, "abc123landed", lookupOnly, nil},
		{"a failed close still ends Done", `[{"number":7,"state":"open"}]`, true, nil, committed, "abc123landed", lookupCommentClose, nil},
		{"no GitHub call when the parent needs no pull request", `[{"number":7,"state":"open"}]`, false, func(d *Deps) { d.Config.RequirePRToBase = []string{"other"} }, committed, "abc123landed", nil, nil},
		{"no GitHub call when the push is skipped", `[{"number":7,"state":"open"}]`, false, func(d *Deps) { d.PushSkipped = true }, committed, "abc123landed", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &closeServer{listBody: tt.listBody, failClose: tt.failClose}
			deps := newTestDeps(t)
			installCloseServer(t, s)
			deps.OriginURL = "https://github.com/acme/widgets.git"
			markedDone := 0
			deps.MarkTaskDone = func() error { markedDone++; return nil }
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: tt.merge}}, headSHA: tt.headSHA}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			shedfake.RequireOutcome(t, fz, shedengine.Done)
			if markedDone != 1 {
				t.Errorf("MarkTaskDone called %d time(s); want 1", markedDone)
			}
			if len(merger.pushCalls) != 1 {
				t.Errorf("PushBranch called %d time(s); want 1", len(merger.pushCalls))
			}
			if strings.Join(s.requests, "|") != strings.Join(tt.wantRequests, "|") {
				t.Fatalf("requests = %v; want %v", s.requests, tt.wantRequests)
			}
			if len(tt.wantCommentHas) > 0 {
				comment, _ := s.bodies[1]["body"].(string)
				for _, want := range tt.wantCommentHas {
					if !strings.Contains(comment, want) {
						t.Errorf("comment = %q; want it to carry %q", comment, want)
					}
				}
				if got := s.bodies[2]["state"]; got != "closed" {
					t.Errorf("close state = %v; want closed", got)
				}
			}
		})
	}
}

// TestFinalize_TransportPushFailure_ReturnsClassifiedError pins that a parent push failing on transport is an error the Shed classifier marks, while a plain push error keeps its Stuck verdict.
func TestFinalize_TransportPushFailure_ReturnsClassifiedError(t *testing.T) {
	deps := newTestDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{
		results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
		pushErr: transportPushErr(),
	}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	_, _, err := fz.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a transient error")
	}
	if got := shedtransient.Class(err); got != shedengine.TransientGitTransport {
		t.Errorf("Class(err) = %q; want %q", got, shedengine.TransientGitTransport)
	}
}
