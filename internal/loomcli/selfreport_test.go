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
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
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
	statusPath      string
	statusLockPath  string
	markerPath      string
	markerLockPath  string
	runLockPath     string
	isLedgerCalls   int
	readLedgerCalls int
	filed           []filedCall
	fileErr         error
}

// newSelfreportTestFixture builds a fresh fixture rooted at t.TempDir(), with IsLedgerPath and
// ReadLedger stubs that count their own calls but recognize nothing by default -- a test that
// wants ledger discovery to fire replaces both fields.
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
		IsLedgerPath: func(path string) bool {
			f.isLedgerCalls++
			return false
		},
		ReadLedger: func(path string) (shedadapters.Ledger, error) {
			f.readLedgerCalls++
			return shedadapters.Ledger{}, fmt.Errorf("unexpected ReadLedger call for %s", path)
		},
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
	// noticed, and the call-counting IsLedgerPath/ReadLedger stubs below pin the rest.

	detectAndFileAnomalies(f.deps(context.Background(), crashResumeEntry(), fmt.Errorf("wrapped: %w", shedengine.ErrShedBusy)))

	if len(f.filed) != 0 {
		t.Errorf("filed calls = %d; want 0: %+v", len(f.filed), f.filed)
	}
	if f.isLedgerCalls != 0 {
		t.Errorf("IsLedgerPath calls = %d; want 0", f.isLedgerCalls)
	}
}

// TestDetectAndFileAnomalies_SelfreportDisabled_TotalInaction asserts a false selfreport bool
// gives total inaction, asserted on both the filing seam and the ledger seam -- what distinguishes
// skipping everything from detecting and then declining to file.
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
	if f.isLedgerCalls != 0 {
		t.Errorf("IsLedgerPath calls = %d; want 0", f.isLedgerCalls)
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
	if f.isLedgerCalls != 0 {
		t.Errorf("IsLedgerPath calls = %d; want 0 -- the cancelled-context path must read no ledger", f.isLedgerCalls)
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

// ledgerFixture is one entry in a fake ledger store keyed by path, used by the ledger-discovery
// and carry-forward-collapse tests below to stub ReadLedger without touching the filesystem.
type ledgerFixture struct {
	round   int
	entries []shedadapters.LedgerEntry
}

// withLedgerSeams overrides deps's IsLedgerPath/ReadLedger with closures over store:
// IsLedgerPath recognizes exactly the keys present in store, and ReadLedger looks up a
// recognized key, both counting their own calls into f.
func withLedgerSeams(deps selfreportDeps, f *selfreportTestFixture, store map[string]ledgerFixture) selfreportDeps {
	deps.IsLedgerPath = func(path string) bool {
		f.isLedgerCalls++
		// Mirrors shedadapters.IsLedgerPath's own filename-shape recognition, so this stub accepts
		// a path by NAMING convention alone -- a missing-on-disk ledger is a legal accepted path
		// that fails only at the read step, never at the predicate.
		return strings.Contains(path, "bouncer-ledger.md")
	}
	deps.ReadLedger = func(path string) (shedadapters.Ledger, error) {
		f.readLedgerCalls++
		lf, ok := store[path]
		if !ok {
			return shedadapters.Ledger{}, fmt.Errorf("no such ledger fixture: %s", path)
		}
		return shedadapters.Ledger{Round: lf.round, Entries: lf.entries}, nil
	}
	return deps
}

// TestRunFilingPass_CarryForwardCollapse pins the collapse step written ahead of it: three ledger
// files for one segment at rounds three, four, and five, each carrying the same open key with a
// progressively longer rounds list, all reachable from history, collapse into exactly one
// recurring-finding anomaly whose body carries the round-five entry's rounds list, filed exactly
// once. The assertion is at the filing-pass level -- the collapse must be why there is one issue,
// not a side effect of filing order.
func TestRunFilingPass_CarryForwardCollapse(t *testing.T) {
	f := newSelfreportTestFixture(t)

	const round3 = "/run/round-3-bouncer-ledger.md"
	const round4 = "/run/round-4-bouncer-ledger.md"
	const round5 = "/run/round-5-bouncer-ledger.md"
	store := map[string]ledgerFixture{
		round3: {round: 3, entries: []shedadapters.LedgerEntry{{Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"}}},
		round4: {round: 4, entries: []shedadapters.LedgerEntry{{Key: "finding-a", Rounds: []int{1, 2, 3, 4}, Status: "open"}}},
		round5: {round: 5, entries: []shedadapters.LedgerEntry{{Key: "finding-a", Rounds: []int{1, 2, 3, 4, 5}, Status: "open"}}},
	}

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

	deps := withLedgerSeams(f.deps(context.Background(), loomengine.EntryObservation{}, nil), f, store)
	detectAndFileAnomalies(deps)

	if len(f.filed) != 1 {
		t.Fatalf("filed calls = %d; want 1: %+v", len(f.filed), f.filed)
	}
	if !containsAll(f.filed[0].body, "1", "2", "3", "4", "5") {
		t.Errorf("filed body = %q; want it to carry the round-five entry's full rounds list", f.filed[0].body)
	}
}

// containsAll reports whether every one of wants is a substring of s.
func containsAll(s string, wants ...string) bool {
	for _, w := range wants {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}

// TestDiscoverLedgers_MixedHistory asserts ledger discovery against a mixed history containing an
// empty output, a discussion-write entry whose output is a decision record, a Burler entry whose
// output is a round review file, a Bouncer entry whose output is a real ledger, and a Bouncer
// entry whose ledger path no longer exists: exactly one ledger is read, neither the producer
// artifact nor the Burler review file is ever opened, and no skip produces an anomaly or an error.
// Alongside it, the generation case: a history entry whose ledger path now holds a different
// generation's file, with round numbering restarted, is read as ordinary content and contributes
// its anomaly, raising no error -- the accepted imprecision pinned as behaviour.
func TestDiscoverLedgers_MixedHistory(t *testing.T) {
	f := newSelfreportTestFixture(t)

	const decisionRecord = "/run/decision-record.md"
	const reviewFile = "/run/round-2-review.md"
	const realLedger = "/run/round-3-bouncer-ledger.md"
	const missingLedger = "/run/round-4-bouncer-ledger.md"

	store := map[string]ledgerFixture{
		realLedger: {round: 3, entries: []shedadapters.LedgerEntry{{Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"}}},
	}

	final := shedengine.Status{
		History: []shedengine.HistoryEntry{
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Done, Output: "", At: "t0"},
			{Producer: "Discussion-Write", Outcome: shedengine.Done, Output: decisionRecord, At: "t1"},
			{Producer: "Discussion-Burler", Outcome: shedengine.Stuck, Output: reviewFile, At: "t2"},
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck, Output: realLedger, At: "t3"},
			{Producer: "Discussion-Bouncer", Outcome: shedengine.Stuck, Output: missingLedger, At: "t4"},
		},
	}

	// Only IsLedgerPath/ReadLedger are exercised by discoverLedgers; nothing else on deps matters.
	deps := withLedgerSeams(selfreportDeps{}, f, store)

	ledgers := discoverLedgers(deps, final)

	if len(ledgers) != 1 {
		t.Fatalf("discovered ledgers = %d; want 1: %+v", len(ledgers), ledgers)
	}
	if ledgers[0].Key != "finding-a" {
		t.Errorf("discovered ledger key = %q; want %q", ledgers[0].Key, "finding-a")
	}
	if f.readLedgerCalls != 2 {
		t.Errorf("ReadLedger calls = %d; want 2 (the real ledger plus the missing one, never the decision record or the review file)", f.readLedgerCalls)
	}
}

// TestDiscoverLedgers_GenerationMismatchStillReads pins the accepted-imprecision behaviour: a
// history entry whose ledger path now holds a different generation's file, with round numbering
// restarted, is read as ordinary content, contributes its observation, and raises no error.
func TestDiscoverLedgers_GenerationMismatchStillReads(t *testing.T) {
	f := newSelfreportTestFixture(t)
	const path = "/run/round-1-bouncer-ledger.md"
	store := map[string]ledgerFixture{
		path: {round: 1, entries: []shedadapters.LedgerEntry{{Key: "finding-z", Rounds: []int{1}, Status: "open"}}},
	}
	final := shedengine.Status{
		History: []shedengine.HistoryEntry{
			{Producer: "Plan-Bouncer", Outcome: shedengine.Stuck, Output: path, At: "t0"},
		},
	}

	deps := withLedgerSeams(selfreportDeps{}, f, store)
	ledgers := discoverLedgers(deps, final)

	if len(ledgers) != 1 {
		t.Fatalf("discovered ledgers = %d; want 1: %+v", len(ledgers), ledgers)
	}
	if ledgers[0].Round != 1 {
		t.Errorf("discovered ledger round = %d; want 1 (the restarted generation's own round, read as ordinary content)", ledgers[0].Round)
	}
}
