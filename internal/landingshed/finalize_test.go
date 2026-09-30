// finalize_test.go covers Finalize against a faked resolver and a faked parent-pair opener closure
// returning a scripted merge outcome. This tier lives in the package itself too, for the same
// reason as its sibling: the resolver seam and the parentMerger seam it substitutes are both
// unexported.

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
	"github.com/Knatte18/loomyard/internal/summaryparser"
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

// newFinalizeDeps returns a minimal Deps for a Finalize test, with a well-formed final-summary
// artifact already written at DescriptionPath -- Call's own top-of-Call parse (see finalize.go's
// step 1a) requires one to exist for every test that does not override this field itself.
func newFinalizeDeps(t *testing.T) Deps {
	t.Helper()
	summaryPath := summaryparser.Path(t.TempDir())
	writeSummary(t, summaryPath, "A landing title", "A landing body.")
	return Deps{
		WorktreeRoot:    t.TempDir(),
		TaskBranch:      "task-branch",
		ParentBranch:    "main",
		DescriptionPath: summaryPath,
		ScratchDir:      filepath.Join(t.TempDir(), "scratch"),
		Config: Config{
			RequirePRToBase:    []string{"main"},
			Squash:             true,
			Conflict:           "sonnet",
			ConflictTimeoutMin: 30,
			CoAuthoredBy:       "Test Author <test@example.com>",
		},
	}
}

// requireFinalizeReason returns the stuck reason carried on ptr, failing the test when it is empty.
func requireFinalizeReason(t *testing.T, ptr shedengine.OutputPointer) string {
	t.Helper()
	if ptr.Reason == "" {
		t.Fatal("OutputPointer.Reason is empty; want the producer's stuck reason")
	}
	return ptr.Reason
}

// --- NewFinalize construction ---

func TestNewFinalize_RejectsNilOpenFabric(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	if _, err := NewFinalize(deps); err == nil {
		t.Fatal("NewFinalize() error = nil; want an error naming Deps.OpenFabric")
	}
}

func TestNewFinalize_RejectsNilOpenParentFabric(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	if _, err := NewFinalize(deps); err == nil {
		t.Fatal("NewFinalize() error = nil; want an error naming Deps.OpenParentFabric")
	}
}

func TestNewFinalize_RejectsEmptyDescriptionPath(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.OpenFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	deps.OpenParentFabric = func() (*fabricengine.Fabric, error) { return nil, nil }
	deps.DescriptionPath = ""
	if _, err := NewFinalize(deps); err == nil {
		t.Fatal("NewFinalize() error = nil; want an error naming Deps.DescriptionPath")
	}
}

// --- Call behaviour ---

// TestFinalize_PushesParentAfterMerge pins that a landed merge is pushed to the parent's upstream
// before Done, honouring Deps.PushSkipped, and that a failed push is Stuck rather than Done.
func TestFinalize_PushesParentAfterMerge(t *testing.T) {
	tests := []struct {
		name        string
		pushSkipped bool
		pushErr     error
		wantOutcome shedengine.Outcome
	}{
		{"pushed", false, nil, shedengine.Done},
		{"push skipped", true, nil, shedengine.Done},
		{"push failed", false, errors.New("remote rejected"), shedengine.Stuck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newFinalizeDeps(t)
			deps.PushSkipped = tt.pushSkipped
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{
				results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
				pushErr: tt.pushErr,
			}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, ptr, err := fz.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
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

func TestFinalize_HappyPath_MergeInThenParentMerge(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if !res.called {
		t.Error("resolver.Resolve was not called; want merge-in to run first")
	}
	if len(merger.calls) != 1 {
		t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
	}
	if merger.calls[0].source != deps.TaskBranch {
		t.Errorf("parent-side merge source = %q; want %q", merger.calls[0].source, deps.TaskBranch)
	}
	if !merger.calls[0].opts.Squash {
		t.Error("parent-side merge Squash = false; want true (threaded from configuration)")
	}
}

// TestFinalize_MergeOptionsCarriesComposedMessage asserts a successful merge passes MergeOptions
// carrying both the composed final-summary message and the configured Squash value, for both merge
// shapes -- the message is set whether or not Squash is true.
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
			deps := newFinalizeDeps(t)
			deps.Config.Squash = tt.squash
			summary, err := summaryparser.Parse(deps.DescriptionPath)
			if err != nil {
				t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
			}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, _, err := fz.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Done {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
			}
			if len(merger.calls) != 1 {
				t.Fatalf("parent-side merge calls = %d; want 1", len(merger.calls))
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
				t.Errorf("MergeOptions.Squash = %v; want %v", got.Squash, tt.squash)
			}
		})
	}
}

// TestFinalize_MissingSummaryArtifact_ErrorBeforeMergeOrCommit asserts a missing final-summary
// artifact makes Call return an error with no merge attempted and no status commit performed --
// proving the top-of-Call parse runs before either.
func TestFinalize_MissingSummaryArtifact_ErrorBeforeMergeOrCommit(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	committed := false
	deps.CommitStatus = func() error { committed = true; return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want an error for a missing change description")
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
}

// TestFinalize_MalformedSummaryArtifact_ErrorBeforeMergeOrCommit is
// TestFinalize_MissingSummaryArtifact_ErrorBeforeMergeOrCommit's sibling case: a summary file exists
// but fails Parse's own validation rather than being absent.
func TestFinalize_MalformedSummaryArtifact_ErrorBeforeMergeOrCommit(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(deps.DescriptionPath, []byte("not a heading\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed summary): %v", err)
	}
	committed := false
	deps.CommitStatus = func() error { committed = true; return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want an error for a malformed change description")
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
}

func TestFinalize_MergeInRequired_RetriesExactlyOnce(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{
		{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
		{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
	}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, ptr, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 2 {
		t.Fatalf("parent-side merge calls = %d; want exactly 2 (one attempt, one retry)", len(merger.calls))
	}
	requireFinalizeReason(t, ptr)
}

// TestFinalize_MergeInRequired_RetryCarriesSameComposedMessage asserts step 5's retry reuses the
// same mergeOpts value as the first attempt, so the retry call's MergeOptions carries the same
// composed message -- no second assignment happens between the first attempt and the retry.
func TestFinalize_MergeInRequired_RetryCarriesSameComposedMessage(t *testing.T) {
	deps := newFinalizeDeps(t)
	summary, err := summaryparser.Parse(deps.DescriptionPath)
	if err != nil {
		t.Fatalf("summaryparser.Parse() error = %v; want nil", err)
	}
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{
		{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
		{result: fabricengine.MergeResult{Committed: true}},
	}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if len(merger.calls) != 2 {
		t.Fatalf("parent-side merge calls = %d; want exactly 2 (one attempt, one retry)", len(merger.calls))
	}
	want := summary.LandingMessage(deps.Config.CoAuthoredBy)
	if merger.calls[0].opts.Message != want {
		t.Errorf("first attempt MergeOptions.Message = %q; want %q", merger.calls[0].opts.Message, want)
	}
	if merger.calls[1].opts.Message != want {
		t.Errorf("retry MergeOptions.Message = %q; want %q (the same composed message as the first attempt)", merger.calls[1].opts.Message, want)
	}
}

func TestFinalize_ParentOpenerError_StuckNamesParentBranch(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return nil, errors.New("no such worktree") }}

	outcome, ptr, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := requireFinalizeReason(t, ptr)
	if !strings.Contains(got, deps.ParentBranch) {
		t.Errorf("stuck reason %q does not name parent branch %q", got, deps.ParentBranch)
	}
}

func TestFinalize_GuardErrorDirtyWorktree_StuckSurfacesReasonVerbatim(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	guardErr := &fabricengine.MergeGuardError{Reasons: []string{"worktree dirty"}}
	merger := &recordingParentMerger{results: []mergeCallResult{{err: guardErr}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, ptr, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 1 {
		t.Errorf("parent-side merge calls = %d; want exactly 1 (no retry on a dirty-worktree guard error)", len(merger.calls))
	}
	got := requireFinalizeReason(t, ptr)
	if !strings.Contains(got, guardErr.Error()) {
		t.Errorf("stuck reason %q does not surface the guard error verbatim (%q)", got, guardErr.Error())
	}
}

func TestFinalize_UnrecognizedMergeError_StuckWithErrorSurfaced(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{err: errors.New("some unrecognized failure")}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, ptr, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	got := requireFinalizeReason(t, ptr)
	if !strings.Contains(got, "some unrecognized failure") {
		t.Errorf("stuck reason %q does not surface the underlying error", got)
	}
}

func TestFinalize_MergeInStuck(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict could not be resolved"}}
	merger := &recordingParentMerger{}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if len(merger.calls) != 0 {
		t.Errorf("parent-side merge calls = %d; want 0 (merge-in never resolved)", len(merger.calls))
	}
}

// TestFinalize_StuckCausesProduceDifferentReasons pins that two different stuck causes leave
// different reasons.
func TestFinalize_StuckCausesProduceDifferentReasons(t *testing.T) {
	deps1 := newFinalizeDeps(t)
	res1 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fz1 := &Finalize{deps: deps1, resolver: res1, parentOpener: func() (parentMerger, error) { return nil, errors.New("no worktree") }}
	_, ptr1, err := fz1.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	reason1 := requireFinalizeReason(t, ptr1)

	deps2 := newFinalizeDeps(t)
	res2 := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger2 := &recordingParentMerger{results: []mergeCallResult{{err: errors.New("some other failure")}}}
	fz2 := &Finalize{deps: deps2, resolver: res2, parentOpener: func() (parentMerger, error) { return merger2, nil }}
	_, ptr2, err := fz2.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	reason2 := requireFinalizeReason(t, ptr2)

	if reason1 == reason2 {
		t.Errorf("two different stuck causes produced identical reasons: %q", reason1)
	}
}

func TestFinalize_CancellationAtEntry_SurfacesAsError(t *testing.T) {
	deps := newFinalizeDeps(t)
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

func TestFinalize_StuckWritesNoReasonFile(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return nil, errors.New("no worktree") }}
	_, ptr, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	requireFinalizeReason(t, ptr)

	matches, err := filepath.Glob(filepath.Join(deps.ScratchDir, "*-stuck.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("stuck-reason files = %v; want none", matches)
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

func runCloseFinalize(t *testing.T, deps Deps) shedengine.Outcome {
	t.Helper()
	deps.OriginURL = "https://github.com/acme/widgets.git"
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{
		results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}},
		headSHA: "abc123landed",
	}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}
	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	return outcome
}

func TestFinalize_ClosesOpenPullRequestAfterLanding(t *testing.T) {
	s := &closeServer{listBody: `[{"number":7,"state":"open"}]`}
	installCloseServer(t, s)

	if outcome := runCloseFinalize(t, newFinalizeDeps(t)); outcome != shedengine.Done {
		t.Fatalf("outcome = %q; want Done", outcome)
	}

	want := []string{"GET /repos/acme/widgets/pulls", "POST /repos/acme/widgets/issues/7/comments", "PATCH /repos/acme/widgets/pulls/7"}
	if strings.Join(s.requests, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v; want %v", s.requests, want)
	}
	comment, _ := s.bodies[1]["body"].(string)
	if !strings.Contains(comment, "abc123landed") || !strings.Contains(comment, "main") {
		t.Errorf("comment = %q; want it to name the landing SHA and parent branch", comment)
	}
	if got := s.bodies[2]["state"]; got != "closed" {
		t.Errorf("close state = %v; want closed", got)
	}
}

func TestFinalize_LeavesNonOpenPullRequestAlone(t *testing.T) {
	s := &closeServer{listBody: `[{"number":7,"state":"closed"}]`}
	installCloseServer(t, s)

	if outcome := runCloseFinalize(t, newFinalizeDeps(t)); outcome != shedengine.Done {
		t.Fatalf("outcome = %q; want Done", outcome)
	}
	if len(s.requests) != 1 {
		t.Errorf("requests = %v; want only the lookup", s.requests)
	}
}

func TestFinalize_CloseFailureStillDone(t *testing.T) {
	s := &closeServer{listBody: `[{"number":7,"state":"open"}]`, failClose: true}
	installCloseServer(t, s)

	if outcome := runCloseFinalize(t, newFinalizeDeps(t)); outcome != shedengine.Done {
		t.Fatalf("outcome = %q; want Done despite the failed close", outcome)
	}
}

func TestFinalize_NoGitHubCallWhenNotRequiredOrPushSkipped(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Deps)
	}{
		{"parent not in require_pr_to_base", func(d *Deps) { d.Config.RequirePRToBase = []string{"other"} }},
		{"push skipped", func(d *Deps) { d.PushSkipped = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &closeServer{listBody: `[{"number":7,"state":"open"}]`}
			installCloseServer(t, s)
			deps := newFinalizeDeps(t)
			tt.mutate(&deps)

			if outcome := runCloseFinalize(t, deps); outcome != shedengine.Done {
				t.Fatalf("outcome = %q; want Done", outcome)
			}
			if len(s.requests) != 0 {
				t.Errorf("requests = %v; want none", s.requests)
			}
		})
	}
}

// --- MarkTaskDone ---

// TestFinalize_MarkTaskDone_OrderAndVerdict pins where the board seam sits relative to the merge
// and the push, and that neither a failing push nor a failing seam changes what it is called for.
func TestFinalize_MarkTaskDone_OrderAndVerdict(t *testing.T) {
	tests := []struct {
		name        string
		pushErr     error
		markErr     error
		wantOutcome shedengine.Outcome
	}{
		{"merge, mark, push", nil, nil, shedengine.Done},
		{"failing push still marks", errors.New("remote rejected"), nil, shedengine.Stuck},
		{"erroring closure still Done", nil, errors.New("board unavailable"), shedengine.Done},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var order []string
			deps := newFinalizeDeps(t)
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

			outcome, _, err := fz.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
			}
			if got, want := strings.Join(order, ","), "merge,mark,push"; got != want {
				t.Errorf("call order = %q; want %q", got, want)
			}
		})
	}
}

// TestFinalize_MarkTaskDone_CalledAfterMergeInRetry asserts the seam runs once, after the retried
// parent-side merge that finally lands, not after the failed first attempt.
func TestFinalize_MarkTaskDone_CalledAfterMergeInRetry(t *testing.T) {
	var order []string
	deps := newFinalizeDeps(t)
	deps.MarkTaskDone = func() error { order = append(order, "mark"); return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{
		results: []mergeCallResult{
			{err: &fabricengine.ErrMergeInRequired{}},
			{result: fabricengine.MergeResult{Committed: true}},
		},
		order: &order,
	}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = (%q, %v); want (Done, nil)", outcome, err)
	}
	if got, want := strings.Join(order, ","), "merge,merge,mark,push"; got != want {
		t.Errorf("call order = %q; want %q", got, want)
	}
}

// TestFinalize_MarkTaskDone_NotCalledOnFailedMerge asserts a merge that never lands never marks the
// task done, and that a nil seam is simply skipped.
func TestFinalize_MarkTaskDone_NotCalledOnFailedMerge(t *testing.T) {
	called := false
	deps := newFinalizeDeps(t)
	deps.MarkTaskDone = func() error { called = true; return nil }
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{err: errors.New("boom")}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() = (%q, %v); want (Stuck, nil)", outcome, err)
	}
	if called {
		t.Error("MarkTaskDone was called after a failed parent-side merge; want it never called")
	}
}

func TestFinalize_MarkTaskDone_NilIsAbsent(t *testing.T) {
	deps := newFinalizeDeps(t)
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = (%q, %v); want (Done, nil)", outcome, err)
	}
}
