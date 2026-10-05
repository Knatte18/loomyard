// selfreport_test.go is the untagged Tier-1 branch suite driving detectAndFileAnomalies directly
// with stub seams, following wiring_commitstatus_test.go's told-seam shape: no tmux, no git, no
// real run. It binds the told-seam boundary -- every branch, filing-pass-order, collapse, marker,
// and config assertion. selfreport_github_test.go binds the engine boundary instead; no test here
// resolves a real go-github client.

package loomcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// filedCall records one FileIssue invocation the stub filing seam captured.
type filedCall struct {
	title  string
	body   string
	labels []string
}

// selfreportTestFixture bundles the paths and call-counting stub seams every detectAndFileAnomalies
// test needs.
type selfreportTestFixture struct {
	statusPath     string
	statusLockPath string
	markerPath     string
	markerLockPath string
	runLockPath    string
	filed          []filedCall
	fileErr        error
}

// newSelfreportTestFixture builds a fresh fixture rooted at t.TempDir().
func newSelfreportTestFixture(t *testing.T) *selfreportTestFixture {
	t.Helper()
	dir := t.TempDir()
	return &selfreportTestFixture{
		statusPath:     filepath.Join(dir, "status.json"),
		statusLockPath: filepath.Join(dir, "status.json.lock"),
		markerPath:     filepath.Join(dir, "selfreport-filed.json"),
		markerLockPath: filepath.Join(dir, "selfreport-filed.json.lock"),
		runLockPath:    filepath.Join(dir, "run.lock"),
	}
}

// deps builds a selfreportDeps against f, with ctx, entry, and runErr told by the caller.
func (f *selfreportTestFixture) deps(ctx context.Context, entry loomengine.EntryObservation, runErr error) selfreportDeps {
	return selfreportDeps{
		Ctx:            ctx,
		Selfreport:     true,
		Entry:          entry,
		StatusPath:     f.statusPath,
		StatusLockPath: f.statusLockPath,
		MarkerPath:     f.markerPath,
		MarkerLockPath: f.markerLockPath,
		RunErr:         runErr,
		FileIssue: func(title string, body *string, labels []string) (string, int, error) {
			b := ""
			if body != nil {
				b = *body
			}
			f.filed = append(f.filed, filedCall{title: title, body: b, labels: labels})
			if f.fileErr != nil {
				return "", 0, f.fileErr
			}
			return "https://example.invalid/issues/1", 1, nil
		},
	}
}

// writeSelfreportStatus writes st to path/lockPath via the production WriteJSON primitive.
func writeSelfreportStatus(t *testing.T, path, lockPath string, st shedengine.Status) {
	t.Helper()
	if err := state.WriteJSON(path, lockPath, st); err != nil {
		t.Fatalf("write status file: %v", err)
	}
}

// productJSON marshals p for embedding as a shedengine.Status.Product payload.
func productJSON(t *testing.T, p loomengine.Status) []byte {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	return raw
}

// crashResumeEntry returns an EntryObservation that loomengine.DetectCrashResume reports an
// anomaly for: running, lock not held, non-empty history.
func crashResumeEntry() loomengine.EntryObservation {
	return loomengine.EntryObservation{
		Observed:        true,
		RunLockHeld:     false,
		State:           shedengine.StateRunning,
		CurrentProducer: "Discussion-Write",
		HistoryLength:   2,
		Slug:            "a-task",
		Parent:          "main",
	}
}

// haltStatus returns a shedengine.Status that loomengine.DetectAnomalies reports exactly one
// producer-hard-failure anomaly for.
func haltStatus() shedengine.Status {
	return shedengine.Status{
		CurrentProducer: "Plan-Write",
		State:           shedengine.StateFailed,
		Error:           "boom",
		History: []shedengine.HistoryEntry{
			{Producer: "Plan-Write", Outcome: shedengine.Done, At: "2026-01-01T00:00:00Z"},
		},
	}
}

// TestDetectAndFileAnomalies_NilRunError_RunsPassAndFiles asserts a nil run error runs the pass
// and the stubbed filing seam records the expected call.
func TestDetectAndFileAnomalies_NilRunError_RunsPassAndFiles(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))

	if len(f.filed) != 1 {
		t.Fatalf("filed calls = %d; want 1: %+v", len(f.filed), f.filed)
	}
}

// TestDetectAndFileAnomalies_NonBusyRunError_StillRunsPass is the regression test for the
// reachability defect: a non-nil run error that is not the busy sentinel must still run the pass,
// without which the natural implementation of appending after the success envelope passes every
// other case here.
func TestDetectAndFileAnomalies_NonBusyRunError_StillRunsPass(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, errors.New("some other failure")))

	if len(f.filed) != 1 {
		t.Fatalf("filed calls = %d; want 1: %+v", len(f.filed), f.filed)
	}
}

// TestDetectAndFileAnomalies_BusySentinel_ReadsNothingDetectsNothingFilesNothing asserts the busy
// sentinel reads nothing, detects nothing, and files nothing.
func TestDetectAndFileAnomalies_BusySentinel_ReadsNothingDetectsNothingFilesNothing(t *testing.T) {
	f := newSelfreportTestFixture(t)
	// Deliberately do not seed a status file: a read attempt would fail loudly enough to be
	// noticed.

	detectAndFileAnomalies(f.deps(context.Background(), crashResumeEntry(), fmt.Errorf("wrapped: %w", shedengine.ErrShedBusy)))

	if len(f.filed) != 0 {
		t.Errorf("filed calls = %d; want 0: %+v", len(f.filed), f.filed)
	}
}

// TestDetectAndFileAnomalies_SelfreportDisabled_TotalInaction asserts a false selfreport bool
// gives total inaction on the filing seam -- what distinguishes skipping everything from detecting
// and then declining to file.
func TestDetectAndFileAnomalies_SelfreportDisabled_TotalInaction(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	deps := f.deps(context.Background(), crashResumeEntry(), nil)
	deps.Selfreport = false
	detectAndFileAnomalies(deps)

	if len(f.filed) != 0 {
		t.Errorf("filed calls = %d; want 0: %+v", len(f.filed), f.filed)
	}
}

// TestDetectAndFileAnomalies_DoneContextNoCrash_TotalInaction asserts an already-done context
// whose entry observation carries no crash-resume gives total inaction.
func TestDetectAndFileAnomalies_DoneContextNoCrash_TotalInaction(t *testing.T) {
	f := newSelfreportTestFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	detectAndFileAnomalies(f.deps(ctx, loomengine.EntryObservation{}, nil))

	if len(f.filed) != 0 {
		t.Errorf("filed calls = %d; want 0: %+v", len(f.filed), f.filed)
	}
}

// TestDetectAndFileAnomalies_DoneContextWithCrash_FilesExactlyOne and its paired complement
// TestDetectAndFileAnomalies_LiveContext_FilesEverything assert the skip keys on the context
// rather than on the paused outcome: the same crash-shaped entry and the same anomaly-bearing
// status file produce exactly one filing call under a done context, and everything under a live
// one.
func TestDetectAndFileAnomalies_DoneContextWithCrash_FilesExactlyOne(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	detectAndFileAnomalies(f.deps(ctx, crashResumeEntry(), nil))

	if len(f.filed) != 1 {
		t.Fatalf("filed calls = %d; want 1: %+v", len(f.filed), f.filed)
	}
}

func TestDetectAndFileAnomalies_LiveContext_FilesEverything(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	detectAndFileAnomalies(f.deps(context.Background(), crashResumeEntry(), nil))

	if len(f.filed) != 2 {
		t.Fatalf("filed calls = %d; want 2 (crash-resume and producer-hard-failure): %+v", len(f.filed), f.filed)
	}
}

// TestSelfreportFiledMarker_RoundTrip is the regression test for the every-resume-re-files
// failure the marker exists to prevent: write, read back, confirm the set survives, and confirm a
// second pass over the identical status file files nothing.
func TestSelfreportFiledMarker_RoundTrip(t *testing.T) {
	f := newSelfreportTestFixture(t)
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 1 {
		t.Fatalf("filed calls after first pass = %d; want 1: %+v", len(f.filed), f.filed)
	}
	firstTitle := f.filed[0].title

	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if !marker.has(firstTitle) {
		t.Fatalf("marker after first pass does not carry %q: %+v", firstTitle, marker)
	}

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 1 {
		t.Errorf("filed calls after second identical pass = %d; want still 1 (no re-file)", len(f.filed))
	}
}

// TestSelfreportFiledMarker_PendingRoundTrip asserts a pending entry survives a write and a read, and that a marker file written before Pending existed reads with no pending entries.
func TestSelfreportFiledMarker_PendingRoundTrip(t *testing.T) {
	f := newSelfreportTestFixture(t)
	want := selfreportFiledMarker{
		Titles:  []string{"filed"},
		Pending: []pendingAnomaly{{Title: "owed", Body: "the body"}},
	}
	writeFiledMarker(f.markerPath, f.markerLockPath, want)

	got := readFiledMarker(f.markerPath, f.markerLockPath)
	if !got.has("filed") || !got.isPending("owed") || len(got.Pending) != 1 || got.Pending[0].Body != "the body" {
		t.Errorf("marker after round-trip = %+v; want %+v", got, want)
	}

	if err := os.WriteFile(f.markerPath, []byte(`{"titles":["old"]}`), 0o644); err != nil {
		t.Fatalf("write legacy marker: %v", err)
	}
	legacy := readFiledMarker(f.markerPath, f.markerLockPath)
	if !legacy.has("old") || len(legacy.Pending) != 0 {
		t.Errorf("legacy marker = %+v; want titles [old] and no pending entries", legacy)
	}
}

// pendHaltTitle runs one pass over a halt status with a failing filer, leaving the halt's title pending, and returns that title and the body the filer was sent.
func pendHaltTitle(t *testing.T, f *selfreportTestFixture) (title, body string) {
	t.Helper()
	st := haltStatus()
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	f.fileErr = errors.New("github down")
	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	f.fileErr = nil

	if len(f.filed) != 1 {
		t.Fatalf("filed calls after the failing pass = %d; want 1: %+v", len(f.filed), f.filed)
	}
	title, body = f.filed[0].title, f.filed[0].body
	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if marker.has(title) || !marker.isPending(title) || marker.Pending[0].Body != body {
		t.Fatalf("marker after the failing pass = %+v; want %q pending with the sent body and unrecorded", marker, title)
	}
	f.filed = nil
	return title, body
}

// TestDetectAndFileAnomalies_FailedFilingResumedPast_IsRetriedOnce is the done-when case: a halt whose filing failed is filed with its stored body once the status no longer shows the halt.
func TestDetectAndFileAnomalies_FailedFilingResumedPast_IsRetriedOnce(t *testing.T) {
	f := newSelfreportTestFixture(t)
	title, body := pendHaltTitle(t, f)

	resumed := shedengine.Status{CurrentProducer: "Plan-Write", State: shedengine.StateRunning}
	resumed.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, resumed)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 1 {
		t.Fatalf("filed calls on the retry pass = %d; want 1: %+v", len(f.filed), f.filed)
	}
	if f.filed[0].title != title || f.filed[0].body != body {
		t.Errorf("retried call = %+v; want title %q and body %q", f.filed[0], title, body)
	}
	if !slices.Equal(f.filed[0].labels, selfreportengine.DefaultLabels()) {
		t.Errorf("retried labels = %v; want %v", f.filed[0].labels, selfreportengine.DefaultLabels())
	}
	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if !marker.has(title) || len(marker.Pending) != 0 {
		t.Errorf("marker after the retry = %+v; want %q recorded and nothing pending", marker, title)
	}

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 1 {
		t.Errorf("filed calls after a third pass = %d; want still 1", len(f.filed))
	}
}

// TestDetectAndFileAnomalies_PendingRetryFailsAgain_StaysPendingAndNewStillFiles asserts a pending entry whose retry fails stays pending while a newly detected anomaly is still filed.
func TestDetectAndFileAnomalies_PendingRetryFailsAgain_StaysPendingAndNewStillFiles(t *testing.T) {
	f := newSelfreportTestFixture(t)
	title, _ := pendHaltTitle(t, f)

	crashStatus := shedengine.Status{CurrentProducer: "Plan-Write", State: shedengine.StateRunning}
	crashStatus.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, crashStatus)

	deps := f.deps(context.Background(), crashResumeEntry(), nil)
	inner := deps.FileIssue
	deps.FileIssue = func(t2 string, body *string, labels []string) (string, int, error) {
		url, n, _ := inner(t2, body, labels)
		if t2 == title {
			return "", 0, errors.New("still down")
		}
		return url, n, nil
	}
	detectAndFileAnomalies(deps)

	if len(f.filed) != 2 {
		t.Fatalf("filed calls = %d; want 2 (the pending retry, then the crash-resume): %+v", len(f.filed), f.filed)
	}
	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if !marker.isPending(title) || marker.has(title) {
		t.Errorf("marker = %+v; want %q still pending", marker, title)
	}
	if !marker.has(f.filed[1].title) {
		t.Errorf("marker = %+v; want the new anomaly %q recorded", marker, f.filed[1].title)
	}
}

// TestDetectAndFileAnomalies_PendingTitleDetectedAgain_FiledOnce asserts a title that is pending and detected again in the same pass is filed once.
func TestDetectAndFileAnomalies_PendingTitleDetectedAgain_FiledOnce(t *testing.T) {
	f := newSelfreportTestFixture(t)
	title, _ := pendHaltTitle(t, f)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))

	if len(f.filed) != 1 || f.filed[0].title != title {
		t.Fatalf("filed calls = %+v; want exactly one for %q", f.filed, title)
	}
	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if !marker.has(title) || len(marker.Pending) != 0 {
		t.Errorf("marker = %+v; want %q recorded and nothing pending", marker, title)
	}
}

// TestDetectAndFileAnomalies_PendingRetryFailsAndDetectedAgain_FiledOnceStaysPendingOnce asserts a pending title whose retry fails and that the same pass detects again is sent to the filer once and stays in the pending list once and out of the recorded titles.
func TestDetectAndFileAnomalies_PendingRetryFailsAndDetectedAgain_FiledOnceStaysPendingOnce(t *testing.T) {
	f := newSelfreportTestFixture(t)
	title, _ := pendHaltTitle(t, f)

	f.fileErr = errors.New("still down")
	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))

	if len(f.filed) != 1 || f.filed[0].title != title {
		t.Fatalf("filed calls = %+v; want exactly one for %q", f.filed, title)
	}
	marker := readFiledMarker(f.markerPath, f.markerLockPath)
	if marker.has(title) || len(marker.Pending) != 1 || marker.Pending[0].Title != title {
		t.Errorf("marker = %+v; want %q pending once and unrecorded", marker, title)
	}
}

// TestDetectAndFileAnomalies_UnusableStatus_StillRetriesPending asserts a missing status file, an unreadable one and an undecodable product each still retry the pending entries.
func TestDetectAndFileAnomalies_UnusableStatus_StillRetriesPending(t *testing.T) {
	cases := map[string]func(t *testing.T, f *selfreportTestFixture){
		"missing": func(t *testing.T, f *selfreportTestFixture) {
			if err := os.Remove(f.statusPath); err != nil {
				t.Fatalf("remove status: %v", err)
			}
		},
		"unreadable": func(t *testing.T, f *selfreportTestFixture) {
			if err := os.WriteFile(f.statusPath, []byte("not json"), 0o644); err != nil {
				t.Fatalf("write status: %v", err)
			}
		},
		"undecodable product": func(t *testing.T, f *selfreportTestFixture) {
			st := haltStatus()
			st.Product = []byte(`"not an object"`)
			writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)
		},
	}
	for name, breakStatus := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSelfreportTestFixture(t)
			title, _ := pendHaltTitle(t, f)
			breakStatus(t, f)

			detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))

			if len(f.filed) != 1 || f.filed[0].title != title {
				t.Fatalf("filed calls = %+v; want one retry of %q", f.filed, title)
			}
			if marker := readFiledMarker(f.markerPath, f.markerLockPath); !marker.has(title) || len(marker.Pending) != 0 {
				t.Errorf("marker = %+v; want %q recorded and nothing pending", marker, title)
			}
		})
	}
}

// TestDetectAndFileAnomalies_Skips_LeavePendingInPlace asserts the knob, busy and cancelled-context skips retry nothing.
func TestDetectAndFileAnomalies_Skips_LeavePendingInPlace(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := map[string]func(d *selfreportDeps){
		"knob off":                   func(d *selfreportDeps) { d.Selfreport = false },
		"busy run error":             func(d *selfreportDeps) { d.RunErr = shedengine.ErrShedBusy },
		"cancelled, no crash-resume": func(d *selfreportDeps) { d.Ctx = cancelled },
	}
	for name, skip := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSelfreportTestFixture(t)
			title, _ := pendHaltTitle(t, f)

			deps := f.deps(context.Background(), loomengine.EntryObservation{}, nil)
			skip(&deps)
			detectAndFileAnomalies(deps)

			if len(f.filed) != 0 {
				t.Errorf("filed calls = %+v; want none", f.filed)
			}
			if marker := readFiledMarker(f.markerPath, f.markerLockPath); !marker.isPending(title) {
				t.Errorf("marker = %+v; want %q still pending", marker, title)
			}
		})
	}
}

// TestDetectAndFileAnomalies_CancelledWithCrashResume_FilesItAndRetriesPending asserts the cancelled-context arm files a lone crash-resume and also retries the pending entries.
func TestDetectAndFileAnomalies_CancelledWithCrashResume_FilesItAndRetriesPending(t *testing.T) {
	f := newSelfreportTestFixture(t)
	title, _ := pendHaltTitle(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	detectAndFileAnomalies(f.deps(ctx, crashResumeEntry(), nil))

	if len(f.filed) != 2 || f.filed[0].title != title {
		t.Fatalf("filed calls = %+v; want the pending retry of %q then the crash-resume", f.filed, title)
	}
	if marker := readFiledMarker(f.markerPath, f.markerLockPath); len(marker.Pending) != 0 || !marker.has(title) {
		t.Errorf("marker = %+v; want nothing pending", marker)
	}
}

// TestSelfreportFiledMarker_NewDistinctHaltStillFiles is the marker round-trip test's
// counterpart: a status whose history has advanced to a second, distinct halt files a new issue
// despite the marker already holding the first halt's title -- the over-suppression failure the
// title discriminator exists to prevent.
func TestSelfreportFiledMarker_NewDistinctHaltStillFiles(t *testing.T) {
	f := newSelfreportTestFixture(t)
	first := haltStatus()
	first.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, first)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 1 {
		t.Fatalf("filed calls after first halt = %d; want 1: %+v", len(f.filed), f.filed)
	}
	firstTitle := f.filed[0].title

	second := haltStatus()
	second.CurrentProducer = "Webster-Write"
	second.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, second)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))
	if len(f.filed) != 2 {
		t.Fatalf("filed calls after second, distinct halt = %d; want 2: %+v", len(f.filed), f.filed)
	}
	if f.filed[1].title == firstTitle {
		t.Errorf("second halt's title = %q; want it distinct from the first halt's %q", f.filed[1].title, firstTitle)
	}
}

// TestDetectAndFileAnomalies_LedgerInHistory_FilesNothing asserts a final status whose history
// publishes ledger files, each carrying the same key open across a growing run of rounds, files
// nothing for them: a recurring ledger finding is no longer an anomaly.
func TestDetectAndFileAnomalies_LedgerInHistory_FilesNothing(t *testing.T) {
	f := newSelfreportTestFixture(t)

	const round3 = "/run/round-3-bouncer-ledger.md"
	const round4 = "/run/round-4-bouncer-ledger.md"
	const round5 = "/run/round-5-bouncer-ledger.md"

	st := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateDone,
		History: []shedengine.HistoryEntry{
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck, Output: round3, At: "t1"},
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck, Output: round4, At: "t2"},
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Done, Output: round5, At: "t3"},
		},
	}
	st.Product = productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"})
	writeSelfreportStatus(t, f.statusPath, f.statusLockPath, st)

	detectAndFileAnomalies(f.deps(context.Background(), loomengine.EntryObservation{}, nil))

	if len(f.filed) != 0 {
		t.Fatalf("filed calls = %d; want 0: %+v", len(f.filed), f.filed)
	}
}
