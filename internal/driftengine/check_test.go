// check_test.go — tier 0 tests for Check and the drift gate over it.
// They run on the in-memory commit store, with fakes for the capture and compare seams.

package driftengine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// keyValuePattern keys a line `name=value` by its name.
const keyValuePattern = `^(?P<key>\w+)=(?P<value>.*)$`

// expectsY is an expectation covering the key `y` of `a.txt`.
const expectsY = "cases:\n  a.txt:\n    keys:\n      - key: y\n        why: tuned the setting\n"

// expectsCommandCase is an expectation under diff command for the case `a.txt`.
const expectsCommandCase = "cases:\n  a.txt:\n    why: the compare is expected to differ\n"

// checkFixture is a Check over an in-memory records store whose capture and compare seams are fakes.
type checkFixture struct {
	t     *testing.T
	store *commitStore
	deps  Deps
	// status is the row the Status seam reports.
	status string
	// config is what the Config seam loads.
	config Config
	// current is the files the next current capture writes, and captureFails makes it fail instead.
	current      map[string]string
	captureFails bool
	// compare is what the Compare seam returns.
	compare CompareResult
	// captures and compares count the calls of the two seams.
	captures, compares int
}

// newCheckFixture returns a fixture at Drift-Verify, past Drift-Expect, with the lines mode and the key=value pattern.
func newCheckFixture(t *testing.T) *checkFixture {
	t.Helper()
	store := newCommitStore(t)
	f := &checkFixture{
		t:       t,
		store:   store,
		status:  "Drift-Verify",
		config:  Config{Diff: DiffLines, KeyPatterns: []string{keyValuePattern}},
		compare: CompareResult{Passed: true},
	}
	f.deps = store.deps(NewPaths(store.root))
	f.deps.Rows = []string{"Record-Write", expectRow, "Drift-Verify"}
	f.deps.Config = func() (Config, error) { return f.config, nil }
	f.deps.Status = func() (shedengine.Status, error) { return shedengine.Status{CurrentProducer: f.status}, nil }
	f.deps.CaptureCurrent = func(context.Context) (CurrentCapture, error) {
		f.captures++
		if f.captureFails {
			return CurrentCapture{Log: "/logs/current.log", Reason: "exit 3"}, nil
		}
		dir := t.TempDir()
		writeTree(t, dir, f.current)
		return CurrentCapture{Dir: dir, Log: "/logs/current.log", Passed: true}, nil
	}
	f.deps.Compare = func(context.Context) (CompareResult, error) {
		f.compares++
		return f.compare, nil
	}
	return f
}

// writeTree writes files under dir, creating it.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
}

// countedScope returns a full scope whose capture counted, recorded with the lines mode.
func countedScope() ScopeRecord {
	return ScopeRecord{Scope: ScopeFull, AwaitedAt: "2026-01-01T00:00:00Z", Diff: DiffLines, KeyPatterns: []string{keyValuePattern}}
}

// seed writes the baseline files and a passed capture record at the store's code head.
// It commits the scope record with the baseline's digest as one records commit.
func (f *checkFixture) seed(scope ScopeRecord, baseline map[string]string) {
	f.t.Helper()
	p := f.deps.Paths
	writeTree(f.t, p.BaselineDir, baseline)
	digest, err := BaselineDigest(p.BaselineDir)
	if err != nil {
		f.t.Fatalf("BaselineDigest() = _, %v; want nil", err)
	}
	if err := WriteCapture(p, CaptureRecord{State: CapturePassed, Head: f.store.codeHead}); err != nil {
		f.t.Fatalf("WriteCapture() = %v; want nil", err)
	}
	change := func(s *ScopeRecord, _ *JudgedRecord) error { *s = scope; return nil }
	if err := WriteRecords(f.deps, change, fabricengine.Trailer{Key: BaselineTrailerKey, Value: digest}); err != nil {
		f.t.Fatalf("WriteRecords() = %v; want nil", err)
	}
}

// seal writes expected.yaml and commits it as the seal.
func (f *checkFixture) seal(expected string) {
	f.t.Helper()
	writeTree(f.t, filepath.Dir(f.deps.Paths.Expected), map[string]string{expectedFileName: expected})
	if err := SealExpectation(f.deps); err != nil {
		f.t.Fatalf("SealExpectation() = %v; want nil", err)
	}
}

// edit overwrites a file after it was committed, which the guards read as a change.
func (f *checkFixture) edit(path, content string) {
	f.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// checkScenario is one scenario of TestCheck.
// It sets the fixture up, then names what the report must hold and what the gate must answer.
type checkScenario struct {
	name  string
	setup func(f *checkFixture)
	check func(t *testing.T, f *checkFixture, report CheckReport)
	// gatePasses is whether the gate passes; a failing gate carries the report's findings.
	gatePasses bool
	// wantFindings is text the report's findings hold.
	wantFindings []string
}

// TestCheck covers Check's skips, faults and classification, and the drift gate's verdict over each report.
func TestCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	baseline := map[string]string{"a.txt": "x=1\ny=2\n"}
	changed := map[string]string{"a.txt": "x=1\ny=3\n"}
	skipRow := func(name string, scope ScopeRecord) checkScenario {
		return checkScenario{
			name:  name,
			setup: func(f *checkFixture) { f.seed(scope, baseline); f.current = changed },
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if report.Skip == "" || f.captures != 0 || f.compares != 0 {
					t.Errorf("Check() = %+v after %d captures and %d compares; want a skip with no capture and no compare", report, f.captures, f.compares)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"skipped: "},
		}
	}
	deferred, waived, off := countedScope(), countedScope(), countedScope()
	deferred.Deferred = true
	waived.WaivedAt, waived.WaivedReason = "2026-01-02T00:00:00Z", "no profile"
	off.Scope = ScopeOff
	partial := countedScope()
	partial.Scope, partial.Skip = ScopePartial, []string{"skipped"}

	tests := []checkScenario{
		{
			name:  "an unexpected cell is listed with case, identity, old and new value",
			setup: func(f *checkFixture) { f.seed(countedScope(), baseline); f.current = changed },
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				unexpected := report.Unexpected()
				want := Cell{Case: "a.txt", Identity: Identity{Key: "y", N: 1}, Kind: CellChanged, Old: "2", New: "3"}
				if len(unexpected) != 1 || unexpected[0].Cell != want {
					t.Errorf("Unexpected() = %+v; want the one cell %+v", unexpected, want)
				}
			},
			wantFindings: []string{`case "a.txt"`, `key "y"`, `old "2"`, `new "3"`, "lym drift amend"},
		},
		{
			name:  "every cell expected yields no unexpected cell",
			setup: func(f *checkFixture) { f.seed(countedScope(), baseline); f.seal(expectsY); f.current = changed },
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if len(report.Cells) != 1 || !report.Cells[0].Expected || report.Cells[0].Coverage.Why != "tuned the setting" || len(report.Unexpected()) != 0 {
					t.Errorf("Cells = %+v; want the one cell expected with its covering why", report.Cells)
				}
			},
			gatePasses: true,
		},
		{
			name:  "an expected entry with no cell is uncovered",
			setup: func(f *checkFixture) { f.seed(countedScope(), baseline); f.seal(expectsY); f.current = baseline },
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if want := []string{`key "y" of case "a.txt"`}; !slices.Equal(report.Uncovered, want) {
					t.Errorf("Uncovered = %v; want %v", report.Uncovered, want)
				}
			},
			gatePasses: true,
		},
		skipRow("scope off skips with no capture", off),
		skipRow("a deferral skips with no capture", deferred),
		skipRow("a waiver skips with no capture", waived),
		{
			name: "a row before Drift-Expect skips as not sealed yet and reports no baseline fault",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.AwaitedAt = ""
				f.seed(scope, baseline)
				f.status = "Record-Write"
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if report.Skip != noExpectationSealedYet || report.BaselineFault != "" || f.captures != 0 {
					t.Errorf("Check() = %+v after %d captures; want a skip saying no expectation is sealed yet and no baseline fault", report, f.captures)
				}
			},
			gatePasses:   true,
			wantFindings: []string{noExpectationSealedYet},
		},
		{
			name: "a record with no awaited_at at a row past Drift-Expect is a baseline fault",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.AwaitedAt = ""
				f.seed(scope, baseline)
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.BaselineFault, "never counted") || f.captures != 0 {
					t.Errorf("Check() = %+v after %d captures; want a baseline fault saying the capture never counted and no capture", report, f.captures)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"never counted", "lym drift defer"},
		},
		{
			name: "a missing baseline directory with no capture record is a baseline fault naming defer",
			setup: func(f *checkFixture) {
				f.seed(countedScope(), baseline)
				if err := os.RemoveAll(f.deps.Paths.BaselineDir); err != nil {
					t.Fatalf("RemoveAll() = %v; want nil", err)
				}
				if err := os.Remove(f.deps.Paths.Capture); err != nil {
					t.Fatalf("Remove() = %v; want nil", err)
				}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.BaselineFault, "lym drift defer") || f.captures != 0 {
					t.Errorf("Check() = %+v after %d captures; want a baseline fault naming lym drift defer and no capture", report, f.captures)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"baseline fault", "lym drift defer"},
		},
		{
			name: "a records mismatch reports restore and classifies against the committed amendment",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Amendments = []Amendment{{By: "parent", At: "2026-01-02T00:00:00Z", Kind: amendmentKindEntries, Entries: expectsY}}
				f.seed(scope, baseline)
				f.edit(f.deps.Paths.Scope, `{"scope":"full","awaited_at":"2026-01-01T00:00:00Z","diff":"lines"}`)
				f.current = changed
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.RecordsFault, "--restore") || len(report.Cells) != 1 || len(report.Unexpected()) != 0 {
					t.Errorf("Check() = %+v; want a records fault naming --restore and the one cell expected by the committed amendment", report)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"records fault", "--restore"},
		},
		{
			name: "a seal fault classifies against the sealed copy under lines",
			setup: func(f *checkFixture) {
				f.seed(countedScope(), baseline)
				f.seal(expectsY)
				f.edit(f.deps.Paths.Expected, "cases: {}\n")
				f.current = changed
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.SealFault, "--restore") || len(report.Cells) != 1 || len(report.Unexpected()) != 0 {
					t.Errorf("Check() = %+v; want a seal fault naming --restore and the one cell expected by the sealed copy", report)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"seal fault", "--restore"},
		},
		{
			name: "a records mismatch does not hide an unexpected cell from the gate",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Amendments = []Amendment{{By: "parent", At: "2026-01-02T00:00:00Z", Kind: amendmentKindEntries, Entries: expectsY}}
				f.seed(scope, baseline)
				f.edit(f.deps.Paths.Scope, `{"scope":"full","awaited_at":"2026-01-01T00:00:00Z","diff":"lines"}`)
				f.current = map[string]string{"a.txt": "x=9\ny=3\n"}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				unexpected := report.Unexpected()
				if report.RecordsFault == "" || len(unexpected) != 1 || unexpected[0].Cell.Identity.Key != "x" {
					t.Errorf("Check() = %+v; want a records fault and the one unexpected cell of key x", report)
				}
			},
			wantFindings: []string{"records fault", `key "x"`},
		},
		{
			name: "a seal fault does not hide an unexpected cell from the gate",
			setup: func(f *checkFixture) {
				f.seed(countedScope(), baseline)
				f.seal(expectsY)
				f.edit(f.deps.Paths.Expected, "cases: {}\n")
				f.current = map[string]string{"a.txt": "x=9\ny=3\n"}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				unexpected := report.Unexpected()
				if report.SealFault == "" || len(unexpected) != 1 || unexpected[0].Cell.Identity.Key != "x" {
					t.Errorf("Check() = %+v; want a seal fault and the one unexpected cell of key x", report)
				}
			},
			wantFindings: []string{"seal fault", `key "x"`},
		},
		{
			name: "a seal fault classifies against the sealed copy under command",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Diff = DiffCommand
				f.seed(scope, baseline)
				f.seal(expectsCommandCase)
				f.edit(f.deps.Paths.Expected, "cases: {}\n")
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.SealFault, "--restore") || report.Expectation.Cases["a.txt"].Why != "the compare is expected to differ" || !report.Compared {
					t.Errorf("Check() = %+v; want a seal fault naming --restore, the sealed case's why and a compare", report)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"seal fault", "--restore"},
		},
		{
			name:  "a failed current capture names its log",
			setup: func(f *checkFixture) { f.seed(countedScope(), baseline); f.captureFails = true },
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !strings.Contains(report.CaptureFault, "/logs/current.log") || !strings.Contains(report.CaptureFault, "exit 3") || len(report.Cells) != 0 {
					t.Errorf("Check() = %+v; want a capture fault naming the log and the reason, and no cells", report)
				}
			},
			wantFindings: []string{"capture fault", "/logs/current.log", "way forward"},
		},
		{
			name: "under command the compare runs and the capture does not, and a failing compare passes the gate",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Diff = DiffCommand
				f.seed(scope, baseline)
				f.compare = CompareResult{ExitCode: 2, Log: "/logs/compare.log", Reason: "exit 2"}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if !report.Compared || report.Compare.ExitCode != 2 || f.compares != 1 || f.captures != 0 {
					t.Errorf("Check() = %+v after %d compares and %d captures; want one compare, its exit 2 and no capture", report, f.compares, f.captures)
				}
			},
			gatePasses:   true,
			wantFindings: []string{"exited 2", "/logs/compare.log"},
		},
		{
			name: "an old partial record with a skip list leaves its cases out of the diff",
			setup: func(f *checkFixture) {
				f.seed(partial, map[string]string{"a.txt": "x=1\n", "skipped.txt": "x=1\n"})
				f.current = map[string]string{"a.txt": "x=1\n", "skipped.txt": "x=9\n"}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if len(report.Cells) != 0 {
					t.Errorf("Cells = %+v; want none, the skipped case left out", report.Cells)
				}
			},
			gatePasses: true,
		},
		{
			name: "the recorded diff wins over a changed config",
			setup: func(f *checkFixture) {
				f.seed(countedScope(), baseline)
				f.config = Config{Diff: DiffCommand}
				f.current = changed
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if report.Mode != DiffLines || report.Compared || f.captures != 1 {
					t.Errorf("Check() = %+v after %d captures; want the recorded lines mode with a capture and no compare", report, f.captures)
				}
			},
		},
		{
			name: "a record with no diff reads the config's mode and patterns",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Diff, scope.KeyPatterns = "", nil
				f.seed(scope, baseline)
				f.current = changed
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				unexpected := report.Unexpected()
				if report.Mode != DiffLines || len(unexpected) != 1 || unexpected[0].Cell.Identity.Key != "y" {
					t.Errorf("Check() = %+v; want the config's lines mode with the config's pattern keying the cell", report)
				}
			},
		},
		{
			name: "a volatile file and key the record names leave no cell",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Volatile = Volatile{Files: []string{"_source.txt"}, Keys: []string{"captured_at"}}
				f.seed(scope, map[string]string{"_source.txt": "commit=a\n", "a.txt": "x=1\ncaptured_at=1\n"})
				f.current = map[string]string{"_source.txt": "commit=b\n", "a.txt": "x=1\ncaptured_at=2\n"}
			},
			gatePasses: true,
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if len(report.Cells) != 0 {
					t.Errorf("Check() cells = %+v; want none", report.Cells)
				}
			},
		},
		{
			name: "a change beside the volatile ones stays unexpected",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Volatile = Volatile{Files: []string{"_source.txt"}}
				f.seed(scope, map[string]string{"_source.txt": "commit=a\n", "a.txt": "x=1\n"})
				f.current = map[string]string{"_source.txt": "commit=b\n", "a.txt": "x=2\n"}
			},
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				unexpected := report.Unexpected()
				if len(report.Cells) != 1 || len(unexpected) != 1 || unexpected[0].Cell.Case != "a.txt" {
					t.Errorf("Check() cells = %+v; want the one unexpected cell of a.txt", report.Cells)
				}
			},
		},
		{
			name: "a record with no diff reads the config's volatile lists",
			setup: func(f *checkFixture) {
				scope := countedScope()
				scope.Diff, scope.KeyPatterns = "", nil
				f.config.Volatile = Volatile{Files: []string{"_source.txt"}}
				f.seed(scope, map[string]string{"_source.txt": "commit=a\n"})
				f.current = map[string]string{"_source.txt": "commit=b\n"}
			},
			gatePasses: true,
			check: func(t *testing.T, f *checkFixture, report CheckReport) {
				if len(report.Cells) != 0 {
					t.Errorf("Check() cells = %+v; want none", report.Cells)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCheckFixture(t)
			tt.setup(f)

			report, err := Check(context.Background(), f.deps)
			if err != nil {
				t.Fatalf("Check() = _, %v; want nil", err)
			}
			tt.check(t, f, report)
			for _, want := range tt.wantFindings {
				if !strings.Contains(report.Findings(), want) {
					t.Errorf("Findings() = %q; want it to name %q", report.Findings(), want)
				}
			}

			result, err := NewDriftGate(f.deps)()
			if err != nil {
				t.Fatalf("NewDriftGate()() = _, %v; want nil", err)
			}
			if result.Passed != tt.gatePasses {
				t.Errorf("NewDriftGate()() passed = %v; want %v", result.Passed, tt.gatePasses)
			}
			if wantFindings := map[bool]string{true: "", false: report.Findings()}[tt.gatePasses]; result.Findings != wantFindings {
				t.Errorf("NewDriftGate()() findings = %q; want %q", result.Findings, wantFindings)
			}
		})
	}
}
