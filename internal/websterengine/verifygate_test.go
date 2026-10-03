package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/verifytree"
)

// gateFake scripts the seams of a verify gate closure and counts what it was asked to do.
type gateFake struct {
	outcome   string
	command   string
	dirty     []string
	head      string
	commits   []string
	rejection [2]string
	// results are the verify results, consumed one per verify call; the last one repeats.
	results []verifytree.Result
	// logs are the verify logs, consumed one per read; the last one repeats.
	logs []string

	verifyCalls    int
	rejectionBases []string
	sites          []verifytree.Site
}

func (f *gateFake) seams() verifyGateSeams {
	logIdx := 0
	return verifyGateSeams{
		outcome:       func() (string, error) { return f.outcome, nil },
		verifyCommand: func() (string, error) { return f.command, nil },
		dirty:         func() ([]string, error) { return f.dirty, nil },
		head:          func() (string, error) { return f.head, nil },
		fixRejection: func(base string) (string, string, error) {
			f.rejectionBases = append(f.rejectionBases, base)
			return f.rejection[0], f.rejection[1], nil
		},
		commitsSince: func(string) ([]string, error) { return f.commits, nil },
		verify: func(site verifytree.Site, command string) (verifytree.Result, error) {
			f.sites = append(f.sites, site)
			res := f.results[min(f.verifyCalls, len(f.results)-1)]
			f.verifyCalls++
			return res, nil
		},
		readLog: func() (string, error) {
			log := f.logs[min(logIdx, len(f.logs)-1)]
			logIdx++
			return log, nil
		},
		logPath: "/verify/verify.log",
		trail: func() ([]string, []string, error) {
			return []string{"c1", "c2"}, []string{"01-first", "02-second"}, nil
		},
		changedPaths: func(sha string) ([]string, error) {
			if sha == "c2" {
				return []string{"internal/a/a.go"}, nil
			}
			return []string{"internal/b/b.go"}, nil
		},
		modulePath: func() string { return testModulePath },
	}
}

const failingPackageLog = "FAIL\texample.com/mod/internal/a\t0.01s\n"

func failedResult() verifytree.Result {
	return verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1}
}

func passedResult() verifytree.Result {
	return verifytree.Result{Status: verifytree.StatusPassed}
}

func TestVerifyGate_NonDoneOutcomePassesWithoutVerifying(t *testing.T) {
	for _, outcome := range []string{outcomeStuck, outcomePaused} {
		f := &gateFake{outcome: outcome, command: "go test ./...", dirty: []string{"x.go"}}
		gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, f.seams())

		res, err := gate()
		if err != nil || !res.Passed {
			t.Fatalf("gate() under outcome %q = %+v, %v; want a pass", outcome, res, err)
		}
		if f.verifyCalls != 0 {
			t.Errorf("verify ran %d time(s) under outcome %q; want none", f.verifyCalls, outcome)
		}
	}
}

func TestVerifyGate_NoVerifySectionPasses(t *testing.T) {
	f := &gateFake{outcome: outcomeDone}
	gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, f.seams())

	res, err := gate()
	if err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass for a plan with no verify section", res, err)
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s); want none", f.verifyCalls)
	}
}

func TestVerifyGate_FlakyFailurePassesOnRerunWithNote(t *testing.T) {
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		results: []verifytree.Result{failedResult(), passedResult()},
		logs:    []string{failingPackageLog},
	}
	notes := &VerifyGateNotes{}
	gate := newVerifyGate(t.TempDir(), 3, notes, f.seams())

	res, err := gate()
	if err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass on rerun", res, err)
	}
	if f.verifyCalls != 2 {
		t.Errorf("verify ran %d time(s); want the run and one rerun", f.verifyCalls)
	}
	if want := []string{"example.com/mod/internal/a"}; !reflect.DeepEqual(notes.Flaky(), want) {
		t.Errorf("notes.Flaky() = %v; want %v", notes.Flaky(), want)
	}
	for _, site := range f.sites {
		if site.Label != "webster gate" || site.Attempt != 1 {
			t.Errorf("verify site = %+v; want label %q, attempt 1", site, "webster gate")
		}
	}
}

func TestVerifyGate_FailedEvaluationWritesReport(t *testing.T) {
	reports := t.TempDir()
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		head:    "head0",
		results: []verifytree.Result{failedResult()},
		logs:    []string{failingPackageLog},
	}
	gate := newVerifyGate(reports, 3, &VerifyGateNotes{}, f.seams())

	first, err := gate()
	if err != nil || first.Passed || first.Terminal {
		t.Fatalf("first gate() = %+v, %v; want a plain failure", first, err)
	}
	f.commits = []string{"fix1", "fix2"}
	second, err := gate()
	if err != nil || second.Passed {
		t.Fatalf("second gate() = %+v, %v; want a failure", second, err)
	}

	report, err := readVerifyGateReport(VerifyGateReportPath(reports))
	if err != nil {
		t.Fatalf("readVerifyGateReport() error = %v", err)
	}
	if report.Attempt != 2 || report.Cap != 3 {
		t.Errorf("report attempt/cap = %d/%d; want 2/3", report.Attempt, report.Cap)
	}
	if len(report.Failures) != 1 || report.Failures[0].ID != "example.com/mod/internal/a" {
		t.Errorf("report failures = %+v; want the failing package", report.Failures)
	}
	if want := []string{"02-second"}; !reflect.DeepEqual(report.Hint, want) {
		t.Errorf("report hint = %v; want %v", report.Hint, want)
	}
	if want := []string{"fix1", "fix2"}; !reflect.DeepEqual(report.FixCommits, want) {
		t.Errorf("report fix commits = %v; want %v", report.FixCommits, want)
	}
	if report.LogPath != "/verify/verify.log" {
		t.Errorf("report log path = %q; want the verify log", report.LogPath)
	}
	for _, want := range []string{"example.com/mod/internal/a", "02-second", "attempt 2 of 3"} {
		if !strings.Contains(second.Findings, want) {
			t.Errorf("findings = %q; want %q in it", second.Findings, want)
		}
	}
}

func TestVerifyGate_DirtyFailureRecordsPreFixHead(t *testing.T) {
	reports := t.TempDir()
	f := &gateFake{
		outcome: outcomeDone,
		command: "go test ./...",
		head:    "head0",
		dirty:   []string{"loose.go"},
		results: []verifytree.Result{passedResult()},
	}
	gate := newVerifyGate(reports, 3, &VerifyGateNotes{}, f.seams())

	res, err := gate()
	if err != nil || res.Passed {
		t.Fatalf("gate() on a dirty tree = %+v, %v; want a failure", res, err)
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s) on a dirty tree; want none", f.verifyCalls)
	}
	if len(f.rejectionBases) != 0 {
		t.Fatalf("commit check ran %v before any pre-fix head was recorded; want none", f.rejectionBases)
	}
	report, err := readVerifyGateReport(VerifyGateReportPath(reports))
	if err != nil {
		t.Fatalf("readVerifyGateReport() error = %v", err)
	}
	if want := []string{"loose.go"}; !reflect.DeepEqual(report.Dirty, want) {
		t.Errorf("report dirty = %v; want %v", report.Dirty, want)
	}

	f.dirty = nil
	if res, err := gate(); err != nil || !res.Passed {
		t.Fatalf("gate() after the tree was cleaned = %+v, %v; want a pass", res, err)
	}
	if want := []string{"head0"}; !reflect.DeepEqual(f.rejectionBases, want) {
		t.Errorf("commit check bases = %v; want the recorded pre-fix head %v", f.rejectionBases, want)
	}
}

func TestVerifyGate_RejectedFixCommitFailsTerminal(t *testing.T) {
	f := &gateFake{
		outcome:   outcomeDone,
		command:   "go test ./...",
		head:      "head0",
		dirty:     []string{"loose.go"},
		rejection: [2]string{"mergeabc", "it merged a commit that is not on the run's parent branch"},
		results:   []verifytree.Result{passedResult()},
	}
	gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, f.seams())
	if res, err := gate(); err != nil || res.Passed {
		t.Fatalf("first gate() = %+v, %v; want a dirty failure", res, err)
	}

	f.dirty = nil
	res, err := gate()
	if err != nil {
		t.Fatalf("second gate() error = %v", err)
	}
	if res.Passed || !res.Terminal {
		t.Fatalf("second gate() = %+v; want a Terminal failure", res)
	}
	for _, want := range []string{"mergeabc", "not on the run's parent branch", "head0"} {
		if !strings.Contains(res.Findings, want) {
			t.Errorf("findings = %q; want %q in it", res.Findings, want)
		}
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s) after a rejected fix commit; want none", f.verifyCalls)
	}
}

func TestVerifyGate_UnreadableOutcomePassesWithoutVerifying(t *testing.T) {
	f := &gateFake{command: "go test ./...", results: []verifytree.Result{failedResult()}}
	s := f.seams()
	s.outcome = func() (string, error) { return "", errors.New("no outcome.yaml") }
	gate := newVerifyGate(t.TempDir(), 3, &VerifyGateNotes{}, s)

	if res, err := gate(); err != nil || !res.Passed {
		t.Fatalf("gate() = %+v, %v; want a pass, leaving the unreadable file to the run's own mapping", res, err)
	}
	if f.verifyCalls != 0 {
		t.Errorf("verify ran %d time(s); want none", f.verifyCalls)
	}
}

func TestReadModulePath(t *testing.T) {
	dir := t.TempDir()
	if got := readModulePath(dir); got != "" {
		t.Errorf("readModulePath() with no go.mod = %q; want empty", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// header\nmodule example.com/mod\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if got := readModulePath(dir); got != testModulePath {
		t.Errorf("readModulePath() = %q; want %q", got, testModulePath)
	}
}
