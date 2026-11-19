// live_test.go covers the live gate's verdicts and the live report it writes.

package hardenshed

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLiveGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	started := func(run string, count int) string { return gateslot.FormatStartedLine(run, count) + "\n" }
	green := LiveRun{Passed: true, Output: started("tier2", 2)}
	clamp := "gate test: --copies asked for 3 copies of tier2 and 2 ran, as the pool's slots hold no more"
	noFailingLine := "the `harden-live` run failed or started no test binary and its output names no failing test; read {report}"

	tests := []struct {
		name        string
		llmDriving  bool
		concurrency int
		live        LiveRun
		liveErr     error
		copies      LiveRun
		copiesErr   error
		withMain    bool

		wantPassed     bool
		wantErr        []string
		wantNoReport   bool
		wantMark       string
		wantDeferred   []string
		wantCopies     *LiveRunReport
		wantCallKeys   []string
		wantOutside    []string
		wantFindingsIn []string
	}{
		{
			name: "GreenLivePasses", concurrency: 1, live: green, withMain: true,
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey},
		},
		{
			name: "RedLiveFailsWithDeferredLines", concurrency: 1, withMain: true,
			live:     LiveRun{Passed: false, Output: started("tier2", 1) + "--- FAIL: TestA (0.1s)\nok pkg\nFAIL\tpkg\t0.2s\n"},
			wantMark: MarkRed, wantCallKeys: []string{LiveGateKey},
			wantDeferred:   []string{"--- FAIL: TestA (0.1s)", "FAIL\tpkg\t0.2s"},
			wantFindingsIn: []string{"harden-live-1.json", "--- FAIL: TestA (0.1s)"},
		},
		{
			name: "PassingLiveStartingNoBinaryIsRed", concurrency: 1, withMain: true,
			live:     LiveRun{Passed: true, Output: started("tier2", 0)},
			wantMark: MarkRed, wantCallKeys: []string{LiveGateKey},
			wantDeferred:   []string{noFailingLine},
			wantFindingsIn: []string{"started no test binary"},
		},
		{
			name: "PassingLiveWithoutStartedLineIsRed", concurrency: 1, withMain: true,
			live:     LiveRun{Passed: true, Output: "ok pkg\n"},
			wantMark: MarkRed, wantCallKeys: []string{LiveGateKey},
			wantDeferred:   []string{noFailingLine},
			wantFindingsIn: []string{"started no test binary"},
		},
		{
			name: "FailedLiveNamingNoFailingTestIsRedWithItsReportDeferred", concurrency: 1, withMain: true,
			live:     LiveRun{Passed: false, Output: started("tier2", 1) + "build step failed\n"},
			wantMark: MarkRed, wantCallKeys: []string{LiveGateKey},
			wantDeferred:   []string{noFailingLine},
			wantFindingsIn: []string{"harden-live-1.json"},
		},
		{
			name: "RedCopiesStaysGreenAndIsRecorded", concurrency: 3, withMain: true, live: green,
			copies:     LiveRun{Passed: false, Output: started("tier2 copy 1", 1) + started("tier2 copy 2", 1) + "--- FAIL: TestB\n" + clamp + "\n"},
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey, LiveCopiesGateKey},
			wantCopies: &LiveRunReport{Started: 2, FailingLines: []string{"--- FAIL: TestB"}, Clamp: clamp},
		},
		{
			name: "CopiesStartingNoneStaysGreenAndIsRecorded", concurrency: 3, withMain: true, live: green,
			copies:     LiveRun{Passed: true, Output: started("tier2 copy 1", 0)},
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey, LiveCopiesGateKey},
			wantCopies: &LiveRunReport{Passed: true},
		},
		{
			name: "LLMDrivingMakesNoCopiesCall", llmDriving: true, concurrency: 3, withMain: true, live: green,
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey},
		},
		{
			name: "RefusedCopiesIsRecordedNotFailed", concurrency: 3, withMain: true, live: green,
			copies:     LiveRun{Refused: "gate test: refused; way forward: lower the copies"},
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey, LiveCopiesGateKey},
			wantCopies: &LiveRunReport{Refused: "gate test: refused; way forward: lower the copies"},
		},
		{
			name: "PackageWithoutTmuxkitMainIsOutsideTheCover", concurrency: 1, live: green,
			wantPassed: true, wantMark: MarkGreen, wantCallKeys: []string{LiveGateKey}, wantOutside: []string{"pkg"},
		},
		{
			name: "RunErrorCarriesItsWayForwardAndWritesNoReport", concurrency: 1, withMain: true,
			liveErr: errors.New("spawn failed"), wantNoReport: true, wantCallKeys: []string{LiveGateKey},
			wantErr: []string{"spawn failed", "way forward: check the key with lym config gate, fix it from the prime, then run lym loom resume"},
		},
		{
			name: "RefusedLiveIsAnErrorWithItsRefusalAndWritesNoReport", concurrency: 3, withMain: true,
			live:         LiveRun{Refused: "gate test: gates.harden-live is not set; way forward: set it"},
			wantNoReport: true, wantCallKeys: []string{LiveGateKey},
			wantErr: []string{"gates.harden-live is not set; way forward: set it"},
		},
		{
			name: "CopiesRunErrorCarriesItsWayForwardAndWritesNoReport", concurrency: 3, withMain: true, live: green,
			copiesErr: errors.New("wait failed"), wantNoReport: true, wantCallKeys: []string{LiveGateKey, LiveCopiesGateKey},
			wantErr: []string{"wait failed", "way forward:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, runDir := t.TempDir(), t.TempDir()
			pkgDir := filepath.Join(root, "pkg")
			if err := os.MkdirAll(pkgDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := "package pkg\n"
			if tt.withMain {
				source += "func TestMain(m *testing.M) { os.Exit(tmuxkit.Main(m)) }\n"
			}
			if err := os.WriteFile(filepath.Join(pkgDir, "a_test.go"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var keys []string
			gate := NewLiveGate(LiveDeps{
				RunDir: runDir, WorktreeRoot: root, Packages: []string{"pkg"},
				Round:      func() (int, error) { return 1, nil },
				LLMDriving: tt.llmDriving, Concurrency: tt.concurrency,
				Run: func(key string, copies int, packages []string) (LiveRun, error) {
					keys = append(keys, key)
					if !reflect.DeepEqual(packages, []string{"pkg"}) {
						t.Errorf("Run(%s) packages = %v; want [pkg]", key, packages)
					}
					if wantCopies := map[string]int{LiveGateKey: 0, LiveCopiesGateKey: tt.concurrency}[key]; copies != wantCopies {
						t.Errorf("Run(%s) copies = %d; want %d", key, copies, wantCopies)
					}
					if key == LiveCopiesGateKey {
						return tt.copies, tt.copiesErr
					}
					return tt.live, tt.liveErr
				},
			})
			result, err := gate()

			if !reflect.DeepEqual(keys, tt.wantCallKeys) {
				t.Errorf("gate keys run = %v; want %v", keys, tt.wantCallKeys)
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("gate() error = nil; want one holding %v", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("gate() error = %q; want it to hold %q", err, want)
					}
				}
			} else if err != nil {
				t.Fatalf("gate() error = %v", err)
			}
			report, found, readErr := ReadLiveReport(runDir, 1)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tt.wantNoReport {
				if found {
					t.Errorf("a live report was written; want none")
				}
				return
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("gate() passed = %v; want %v", result.Passed, tt.wantPassed)
			}
			for _, want := range tt.wantFindingsIn {
				if !strings.Contains(result.Findings, want) {
					t.Errorf("gate() findings = %q; want it to hold %q", result.Findings, want)
				}
			}
			if !found || report.Round != 1 || report.Mark != tt.wantMark {
				t.Errorf("report = %+v, found %v; want round 1 and mark %s", report, found, tt.wantMark)
			}
			var wantDeferred []string
			for _, line := range tt.wantDeferred {
				wantDeferred = append(wantDeferred, strings.ReplaceAll(line, "{report}", liveReportPath(runDir, 1)))
			}
			if !reflect.DeepEqual(report.Deferred, wantDeferred) {
				t.Errorf("report deferred = %v; want %v", report.Deferred, wantDeferred)
			}
			if !reflect.DeepEqual(report.Copies, tt.wantCopies) {
				t.Errorf("report copies = %+v; want %+v", report.Copies, tt.wantCopies)
			}
			if !reflect.DeepEqual(report.OutsideStrayCover, tt.wantOutside) {
				t.Errorf("report outside_stray_cover = %v; want %v", report.OutsideStrayCover, tt.wantOutside)
			}
		})
	}
}
