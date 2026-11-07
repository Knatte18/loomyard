// expect_test.go — tier 0 tests for the Drift-Expect producer and its expectation gate.
// They run over the in-memory records store and a scripted seat runner.

package driftengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The expectations the scripted agent writes: one the baseline's cases hold, and one naming a case they lack.
const (
	validExpectation       = "cases:\n  a.txt:\n    keys:\n      - key: alpha\n        why: the task moves alpha\n"
	unknownCaseExpectation = "cases:\n  nope.txt:\n    keys:\n      - key: alpha\n        why: the task moves alpha\n"
)

// expectFixture is the fake world one DriftExpect pass runs in.
// It holds a records store, a baseline of two cases and a seat runner whose agent writes expected.yaml.
type expectFixture struct {
	t     *testing.T
	store *commitStore
	paths Paths
	deps  Deps
	seats *shedfake.SeatRunner

	cfg Config
	// commits is what CommitsSince answers, and liveChair whether a probe finds the agent's chair.
	commits   []string
	liveChair bool
	// agentFile is what the scripted agent writes to expected.yaml at each arrival; it writes none when empty.
	agentFile string
	// listings holds the listing file at each agent arrival, and listingPaths the path each ExpectTable call was handed.
	listings     []string
	listingPaths []string
}

// newExpectFixture returns a fixture whose baseline holds a.txt, with a repeated key, and b.txt.
// Its drift config is diff lines with one key pattern.
func newExpectFixture(t *testing.T) *expectFixture {
	t.Helper()
	store := newCommitStore(t)
	f := &expectFixture{t: t, store: store, paths: NewPaths(store.root)}
	f.cfg = Config{Enabled: true, Diff: DiffLines, KeyPatterns: []string{`^(?P<key>[a-z]+)=(?P<value>.*)$`}}
	writeExpectFile(t, filepath.Join(f.paths.BaselineDir, "a.txt"), "alpha=1\nbeta=2\nfree text\nalpha=3\n")
	writeExpectFile(t, filepath.Join(f.paths.BaselineDir, "b.txt"), "gamma=4\n")

	actions := make([]func(), 12)
	for i := range actions {
		actions[i] = f.agentArrives
	}
	f.seats = gateLoopSeats(t, actions)
	f.seats.ProbeFn = func(seatengine.Table) (seatengine.LiveTable, error) {
		if f.liveChair {
			return seatengine.LiveTable{Chair: fakeChair{}}, nil
		}
		return seatengine.LiveTable{}, nil
	}

	f.deps = store.deps(f.paths)
	f.deps.Config = func() (Config, error) { return f.cfg, nil }
	f.deps.CommitsSince = func(string) ([]string, error) { return f.commits, nil }
	f.deps.ExpectTable = func(listing string) (seatengine.Table, error) {
		f.listingPaths = append(f.listingPaths, listing)
		return seatengine.Table{Seats: []seatengine.Seat{{Outputs: []string{f.paths.Expected}}}}, nil
	}
	f.deps.Seats = f.seats
	f.deps.Now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	return f
}

// writeExpectFile writes text to path, creating its directory.
func writeExpectFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// agentArrives is the scripted agent's action at each arrival: it reads the listing and writes agentFile.
func (f *expectFixture) agentArrives() {
	if listing, err := os.ReadFile(f.paths.Listing); err == nil {
		f.listings = append(f.listings, string(listing))
	}
	if f.agentFile != "" {
		writeExpectFile(f.t, f.paths.Expected, f.agentFile)
	}
}

// seed records a scope record that counted the baseline at the store's code head, with change applied.
// It writes the record through the records guard.
func (f *expectFixture) seed(change func(*ScopeRecord)) {
	f.t.Helper()
	scope := ScopeRecord{
		Scope:        ScopeFull,
		RecordedAt:   "2026-01-01T00:00:00Z",
		AwaitedAt:    "2026-01-01T00:00:00Z",
		BaselineHead: f.store.codeHead,
		Diff:         f.cfg.Diff,
		KeyPatterns:  f.cfg.KeyPatterns,
	}
	if change != nil {
		change(&scope)
	}
	err := WriteRecords(f.deps, func(recorded *ScopeRecord, _ *JudgedRecord) error {
		*recorded = scope
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

// call runs one DriftExpect pass with an expectation entry of two attempts.
func (f *expectFixture) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	entry := shuttleengine.GateEntry{Name: "expectation", Attempts: 2, Gate: NewExpectationGate(f.deps)}
	return NewDriftExpect("Drift-Expect", Row{}, entry, f.deps).Call(context.Background())
}

// records reads the two records as the guard returns them.
func (f *expectFixture) records() Records {
	f.t.Helper()
	records, err := ReadRecords(f.deps)
	if err != nil {
		f.t.Fatal(err)
	}
	return records
}

// commitsWith returns the store's commits that carry the trailer key.
func (f *expectFixture) commitsWith(key string) []fakeRecordsCommit {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	var found []fakeRecordsCommit
	for _, commit := range f.store.commits {
		for _, trailer := range commit.trailers {
			if trailer.Key == key {
				found = append(found, commit)
				break
			}
		}
	}
	return found
}

// trailerOf returns the value of the trailer key on commit, and whether it carries one.
func trailerOf(commit fakeRecordsCommit, key string) (string, bool) {
	for _, trailer := range commit.trailers {
		if trailer.Key == key {
			return trailer.Value, true
		}
	}
	return "", false
}

// agentTouched reports whether the seat runner was asked to probe, run or resume anything.
func (f *expectFixture) agentTouched() bool {
	return f.seats.Calls+f.seats.ProbeCalls+f.seats.ResumeCalls > 0
}

// requireExpectDone fails t unless the pass ended Done without an error.
func requireExpectDone(t *testing.T, outcome shedengine.Outcome, err error) {
	t.Helper()
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done", outcome, err)
	}
}

// TestDriftExpect covers the passes that need no agent and the agent's table with the seal that follows it.
// It also covers the sessions an earlier entry left and the stops for the run's parent.
// It also covers the diff mode recorded once, an exhausted entry, and the expectation gate alone.
func TestDriftExpect(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("DoneWithoutAnAgent", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			prepare      func(*expectFixture)
			wantFound    bool
			wantUnsealed bool
			wantSeals    int
		}{
			{name: "NoScopeRecord", prepare: func(*expectFixture) {}},
			{name: "SwitchedOff", prepare: func(f *expectFixture) { f.cfg.Enabled = false; f.seed(nil) }, wantFound: true},
			{name: "ScopeOff", prepare: func(f *expectFixture) { f.seed(func(s *ScopeRecord) { s.Scope = ScopeOff }) }, wantFound: true, wantUnsealed: true},
			{name: "DeferredCapture", prepare: func(f *expectFixture) {
				f.seed(func(s *ScopeRecord) { s.Deferred = true })
			}, wantFound: true, wantUnsealed: true},
			{name: "AlreadyUnsealedOverEditedRecords", prepare: func(f *expectFixture) {
				f.seed(func(s *ScopeRecord) { s.Scope, s.Unsealed = ScopeOff, true })
				writeExpectFile(f.t, f.paths.Scope, `{"scope":"off"}`)
			}, wantFound: true, wantUnsealed: true},
			{name: "AlreadySealed", prepare: func(f *expectFixture) {
				f.seed(nil)
				writeExpectFile(f.t, f.paths.Expected, validExpectation)
				if err := SealExpectation(f.deps); err != nil {
					f.t.Fatal(err)
				}
			}, wantFound: true, wantSeals: 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				tt.prepare(f)

				outcome, _, err := f.call()

				requireExpectDone(t, outcome, err)
				if f.agentTouched() {
					t.Errorf("the seat runner was used: %d runs, %d probes, %d resumes; want none", f.seats.Calls, f.seats.ProbeCalls, f.seats.ResumeCalls)
				}
				records := f.records()
				if records.ScopeFound != tt.wantFound || records.Scope.Unsealed != tt.wantUnsealed {
					t.Errorf("records: scope found %v, unsealed %v; want %v and %v", records.ScopeFound, records.Scope.Unsealed, tt.wantFound, tt.wantUnsealed)
				}
				if got := len(f.commitsWith(SealTrailerKey)); got != tt.wantSeals {
					t.Errorf("%d seal commits; want %d", got, tt.wantSeals)
				}
			})
		}
	})

	t.Run("AgentWritesAndTheFileIsSealed", func(t *testing.T) {
		t.Parallel()
		f := newExpectFixture(t)
		f.agentFile = validExpectation
		f.seed(nil)

		outcome, pointer, err := f.call()

		requireExpectDone(t, outcome, err)
		if pointer.Path != f.paths.Expected {
			t.Errorf("pointer.Path = %q; want %q", pointer.Path, f.paths.Expected)
		}
		if !reflect.DeepEqual(f.listingPaths, []string{f.paths.Listing}) || f.seats.Calls != 1 {
			t.Errorf("ExpectTable was handed %q with %d runs; want the listing path once and one run", f.listingPaths, f.seats.Calls)
		}
		seals := f.commitsWith(SealTrailerKey)
		if len(seals) != 1 {
			t.Fatalf("%d seal commits; want 1", len(seals))
		}
		if value, _ := trailerOf(seals[0], SealTrailerKey); value != sealDigest([]byte(validExpectation)) {
			t.Errorf("seal trailer = %q; want the digest of the agent's file", value)
		}
		if got := string(seals[0].files[durableRel(expectedFileName)]); got != validExpectation {
			t.Errorf("sealed copy = %q; want the agent's file", got)
		}
		if mark := f.records().Scope.ExpectSession; mark.Open || mark.OpenedAt == "" {
			t.Errorf("expect session mark = %+v after the agent returned; want it closed with its OpenedAt kept", mark)
		}
	})

	t.Run("TheListingNamesEveryBaselineCase", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			diff      string
			agentFile string
			want      listingFile
		}{
			{
				name:      "DiffLines",
				diff:      DiffLines,
				agentFile: validExpectation,
				want: listingFile{Mode: DiffLines, Cases: []listingCase{
					{Case: "a.txt", Lines: []listingLine{
						{Key: "alpha", N: 1, Text: "alpha=1"},
						{Key: "beta", Text: "beta=2"},
						{Line: 1, Text: "free text"},
						{Key: "alpha", N: 2, Text: "alpha=3"},
					}},
					{Case: "b.txt", Lines: []listingLine{{Key: "gamma", Text: "gamma=4"}}},
				}},
			},
			{
				name:      "DiffCommand",
				diff:      DiffCommand,
				agentFile: "cases:\n  a.txt:\n    why: the compare differs\n",
				want:      listingFile{Mode: DiffCommand, Cases: []listingCase{{Case: "a.txt"}, {Case: "b.txt"}}},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.cfg.Diff = tt.diff
				f.agentFile = tt.agentFile
				f.seed(nil)

				outcome, _, err := f.call()

				requireExpectDone(t, outcome, err)
				if len(f.listings) == 0 {
					t.Fatal("the agent found no listing")
				}
				var got listingFile
				if err := yaml.Unmarshal([]byte(f.listings[0]), &got); err != nil {
					t.Fatalf("decode the listing: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("listing = %+v; want %+v", got, tt.want)
				}
			})
		}
	})

	t.Run("TheBaselineDigestRidesTheFirstSeal", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			recordedYet  bool
			wantTrailers bool
		}{
			{"NoDigestRecordedYet", false, true},
			{"DigestAlreadyRecorded", true, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.agentFile = validExpectation
				f.seed(nil)
				digest, err := BaselineDigest(f.paths.BaselineDir)
				if err != nil {
					t.Fatal(err)
				}
				if tt.recordedYet {
					if err := f.deps.CommitWithTrailers(nil, []fabricengine.Trailer{{Key: BaselineTrailerKey, Value: digest}}); err != nil {
						t.Fatal(err)
					}
				}

				outcome, _, err := f.call()

				requireExpectDone(t, outcome, err)
				seals := f.commitsWith(SealTrailerKey)
				if len(seals) != 1 {
					t.Fatalf("%d seal commits; want 1", len(seals))
				}
				value, carried := trailerOf(seals[0], BaselineTrailerKey)
				if carried != tt.wantTrailers || (carried && value != digest) {
					t.Errorf("seal commit carries the baseline digest %q (%v); want carried %v and equal to %q", value, carried, tt.wantTrailers, digest)
				}
			})
		}
	})

	t.Run("AClosedMarkSealsOrSpawnsByTheFileAndTheSession", func(t *testing.T) {
		t.Parallel()
		ended := SessionMark{OpenedAt: "2026-01-01T00:00:00Z"}
		tests := []struct {
			name      string
			mark      SessionMark
			onDisk    string
			wantRuns  int
			wantSeals int
		}{
			{"ValidFileAfterAnEndedSessionSealsWithoutASpawn", ended, validExpectation, 0, 1},
			{"InvalidFileAfterAnEndedSessionSpawnsTheAgent", ended, unknownCaseExpectation, 1, 1},
			{"ValidFileWithNoSessionEverOpenedSpawnsTheAgent", SessionMark{}, validExpectation, 1, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.agentFile = validExpectation
				f.seed(func(s *ScopeRecord) { s.ExpectSession = tt.mark })
				writeExpectFile(t, f.paths.Expected, tt.onDisk)

				outcome, _, err := f.call()

				requireExpectDone(t, outcome, err)
				if f.seats.Calls != tt.wantRuns || len(f.commitsWith(SealTrailerKey)) != tt.wantSeals {
					t.Errorf("%d runs and %d seal commits; want %d and %d", f.seats.Calls, len(f.commitsWith(SealTrailerKey)), tt.wantRuns, tt.wantSeals)
				}
				if tt.wantRuns == 0 && f.agentTouched() {
					t.Errorf("the seat runner was probed or resumed; want it untouched")
				}
			})
		}
	})

	t.Run("AnOpenMarkProbesBeforeSealing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			liveChair   bool
			wantRuns    int
			wantResumes int
		}{
			{"ALiveChairIsResumed", true, 0, 1},
			{"NoLiveChairRespawns", false, 1, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.agentFile = validExpectation
				f.liveChair = tt.liveChair
				f.seed(func(s *ScopeRecord) { s.ExpectSession = SessionMark{Open: true, OpenedAt: "2026-01-01T00:00:00Z"} })

				outcome, _, err := f.call()

				requireExpectDone(t, outcome, err)
				if f.seats.ProbeCalls == 0 || f.seats.Calls != tt.wantRuns || f.seats.ResumeCalls != tt.wantResumes {
					t.Errorf("%d probes, %d runs, %d resumes; want a probe, %d runs and %d resumes", f.seats.ProbeCalls, f.seats.Calls, f.seats.ResumeCalls, tt.wantRuns, tt.wantResumes)
				}
				if len(f.commitsWith(SealTrailerKey)) != 1 || f.records().Scope.ExpectSession.Open {
					t.Errorf("want one seal commit and a closed session mark after the agent returned")
				}
			})
		}
	})

	t.Run("AStopForTheParent", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			seed    func(*ScopeRecord)
			prepare func(*expectFixture)
			// prefix is what the reason opens with, and the decision halt prefix then the defer prefix when empty.
			prefix    string
			fragments []string
		}{
			{
				name: "TheCodeMovedAfterTheBaseline",
				prepare: func(f *expectFixture) {
					f.store.codeHead = "code-head-1"
					f.commits = []string{"c1", "c2"}
				},
				fragments: []string{"c1, c2", "code-head-0", "code-head-1", "lym drift defer", "resets the run's code branch", "lym loom resume"},
			},
			{
				name: "TheCodeMovedAfterTheCaptureRecordsHead",
				seed: func(s *ScopeRecord) { s.BaselineHead = "" },
				prepare: func(f *expectFixture) {
					if err := WriteCapture(f.paths, CaptureRecord{State: CapturePassed, Head: "code-head-0"}); err != nil {
						f.t.Fatal(err)
					}
					f.store.codeHead = "code-head-1"
					f.commits = []string{"c1"}
				},
				fragments: []string{"c1", "code-head-0", "lym drift defer"},
			},
			{
				name:      "TheBaselineDirectoryIsMissing",
				prepare:   func(f *expectFixture) { _ = os.RemoveAll(f.paths.BaselineDir) },
				fragments: []string{"baseline directory", "lym drift defer", "lym loom resume"},
			},
			{
				name:      "TheCaptureNeverCounted",
				seed:      func(s *ScopeRecord) { s.AwaitedAt = "" },
				fragments: []string{"never counted", "lym drift defer", "lym loom resume"},
			},
			{
				name:      "TheRecordsDifferFromTheirCommit",
				prepare:   func(f *expectFixture) { writeExpectFile(f.t, f.paths.Scope, `{"scope":"full"}`) },
				prefix:    shedadapters.DecisionHaltPrefix,
				fragments: []string{"lym drift amend <slug> --restore", "lym loom resume"},
			},
			{
				name:      "TheRecordsDifferFromTheirCommitUnderScopeOff",
				seed:      func(s *ScopeRecord) { s.Scope = ScopeOff },
				prepare:   func(f *expectFixture) { writeExpectFile(f.t, f.paths.Scope, `{"scope":"off"}`) },
				prefix:    shedadapters.DecisionHaltPrefix,
				fragments: []string{"lym drift amend <slug> --restore", "lym loom resume"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.agentFile = validExpectation
				f.seed(tt.seed)
				if tt.prepare != nil {
					tt.prepare(f)
				}

				outcome, pointer, err := f.call()

				if err != nil || outcome != shedengine.Stuck {
					t.Fatalf("Call() = %q, %v; want Stuck", outcome, err)
				}
				want := tt.prefix
				if want == "" {
					want = shedadapters.DecisionHaltPrefix + DeferHaltPrefix
				}
				if !strings.HasPrefix(pointer.Reason, want) {
					t.Errorf("reason = %q; want it to open with %q", pointer.Reason, want)
				}
				for _, fragment := range tt.fragments {
					if !strings.Contains(pointer.Reason, fragment) {
						t.Errorf("reason = %q; want it to name %q", pointer.Reason, fragment)
					}
				}
				if f.agentTouched() || len(f.listingPaths) > 0 || len(f.commitsWith(SealTrailerKey)) > 0 {
					t.Errorf("the pass spawned the agent or sealed a file before it stopped")
				}
			})
		}
	})

	t.Run("AnErroredSpawnIsNotAnEndedSession", func(t *testing.T) {
		t.Parallel()
		f := newExpectFixture(t)
		f.agentFile = validExpectation
		f.seed(nil)
		writeExpectFile(t, f.paths.Expected, validExpectation)
		run := f.seats.RunFn
		f.seats.RunFn = func(seatengine.Table) (seatengine.Result, error) {
			return seatengine.Result{}, errors.New("the spawn failed")
		}

		if _, _, err := f.call(); err == nil {
			t.Fatalf("first Call() error = nil; want the spawn's error")
		}
		if mark := f.records().Scope.ExpectSession; mark.Open || mark.OpenedAt != "" {
			t.Errorf("expect session mark = %+v after an errored spawn; want the zero mark, not an ended session", mark)
		}

		f.seats.RunFn = run
		callsBefore := f.seats.Calls
		outcome, _, err := f.call()

		requireExpectDone(t, outcome, err)
		if f.seats.Calls != callsBefore+1 {
			t.Errorf("the second Call() ran the agent %d times; want once, since the valid file was written before any session ended", f.seats.Calls-callsBefore)
		}
	})

	t.Run("ARecordWithNoDiffRecordsTheConfigs", func(t *testing.T) {
		t.Parallel()
		f := newExpectFixture(t)
		f.agentFile = validExpectation
		f.cfg.Volatile = Volatile{Files: []string{"_source.txt"}, Keys: []string{"captured_at"}}
		f.seed(func(s *ScopeRecord) { s.Diff, s.KeyPatterns = "", nil })

		outcome, _, err := f.call()

		requireExpectDone(t, outcome, err)
		scope := f.records().Scope
		if scope.Diff != f.cfg.Diff || !reflect.DeepEqual(scope.KeyPatterns, f.cfg.KeyPatterns) || !reflect.DeepEqual(scope.Volatile, f.cfg.Volatile) {
			t.Errorf("recorded diff %q, patterns %q and volatile %+v; want the config's %q, %q and %+v", scope.Diff, scope.KeyPatterns, scope.Volatile, f.cfg.Diff, f.cfg.KeyPatterns, f.cfg.Volatile)
		}
	})

	t.Run("AnExhaustedEntryEndsStuckAndTheNextCallSpawnsAgain", func(t *testing.T) {
		t.Parallel()
		f := newExpectFixture(t)
		f.agentFile = unknownCaseExpectation
		f.seed(nil)

		outcome, _, err := f.call()

		if err != nil || outcome != shedengine.Stuck {
			t.Fatalf("first Call() = %q, %v; want Stuck", outcome, err)
		}
		if f.seats.Calls != 1 || len(f.commitsWith(SealTrailerKey)) != 0 || f.records().Scope.ExpectSession.Open {
			t.Errorf("after the exhausted entry: %d runs, %d seal commits, mark open %v; want 1, 0 and a closed mark", f.seats.Calls, len(f.commitsWith(SealTrailerKey)), f.records().Scope.ExpectSession.Open)
		}

		f.agentFile = validExpectation
		outcome, _, err = f.call()

		requireExpectDone(t, outcome, err)
		if f.seats.Calls != 2 || len(f.commitsWith(SealTrailerKey)) != 1 {
			t.Errorf("after the second call: %d runs, %d seal commits; want 2 and 1", f.seats.Calls, len(f.commitsWith(SealTrailerKey)))
		}
	})

	t.Run("TheExpectationGate", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			onDisk     string
			wantPassed bool
			wantFinds  string
		}{
			{"AValidFilePasses", validExpectation, true, ""},
			{"AMissingFileAsksForIt", "", false, "expected.yaml is missing"},
			{"AnUnknownCaseFailsWithTheParsersMessage", unknownCaseExpectation, false, `case "nope.txt" is neither a case of the baseline nor listed under added`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newExpectFixture(t)
				f.seed(nil)
				if tt.onDisk != "" {
					writeExpectFile(t, f.paths.Expected, tt.onDisk)
				}

				result, err := NewExpectationGate(f.deps)()

				if err != nil {
					t.Fatalf("gate error = %v", err)
				}
				if result.Passed != tt.wantPassed || !strings.Contains(result.Findings, tt.wantFinds) {
					t.Errorf("gate = passed %v, findings %q; want passed %v and findings naming %q", result.Passed, result.Findings, tt.wantPassed, tt.wantFinds)
				}
			})
		}
	})
}
