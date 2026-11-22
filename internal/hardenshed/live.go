// live.go holds the live gate, Go's independent check of each fix round, and the round's live report it writes.

package hardenshed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// The marks a live report carries.
const (
	// MarkRed says the round's `harden-live` run failed, or started no test binary for a campaign that names packages.
	MarkRed = "red"
	// MarkGreen says the round's `harden-live` run passed, and started a test binary when the campaign names packages.
	MarkGreen = "green"
)

// strayCoverCall is the call that puts a package's strays under the test kit's check when its test files hold it.
const strayCoverCall = "tmuxkit.Main("

// copiesClampPrefix opens the line a copies call prints when the pool's slots hold fewer copies than were asked for.
const copiesClampPrefix = "gate test: --copies asked for"

// liveWayForward closes the error of a live run that could not be started or awaited.
const liveWayForward = "way forward: check the key with lym config gate, fix it from the prime, then run lym loom resume"

// LiveRun is the outcome of one `lym gate test` call the live gate made.
type LiveRun struct {
	// Passed reports whether the call exited green.
	Passed bool
	// Output is the call's output, its started lines included.
	Output string
	// Refused is the refusal text when the call refused before running any tier, empty otherwise.
	Refused string
}

// LiveDeps is what the live gate needs told.
type LiveDeps struct {
	// RunDir is the run directory the round's live report is written in.
	RunDir string
	// WorktreeRoot is the absolute worktree root the Packages are relative to.
	WorktreeRoot string
	// Round names the round being fixed.
	Round func() (int, error)
	// Packages are the worktree-relative package directories the campaign drives.
	Packages []string
	// LLMDriving says the live tests drive a billed LLM session, which turns the copies off.
	LLMDriving bool
	// Concurrency is the copies the copies call asks for; one or less runs none.
	Concurrency int
	// Run makes one `lym gate test` call of the gate key with the copies and packages, as a call taking its own slots.
	// A spawn or wait failure is its error; a refusal before any tier ran is LiveRun.Refused.
	Run func(key string, copies int, packages []string) (LiveRun, error)
}

// LiveRunReport is what the live report records of one call.
type LiveRunReport struct {
	// Passed reports whether the call exited green.
	Passed bool `json:"passed"`
	// Started is the number of test binaries the call started, summed over its runs.
	Started int `json:"started"`
	// FailingLines are the lines of the call's output that name a failure.
	FailingLines []string `json:"failing_lines,omitempty"`
	// Clamp is the line the call printed when the pool held fewer copies than were asked for.
	Clamp string `json:"clamp,omitempty"`
	// Refused is the refusal text of a call that refused before running any tier.
	Refused string `json:"refused,omitempty"`
}

// LiveReport is one round's live report, written as `harden-live-<round>.json` in the run directory.
type LiveReport struct {
	// Round is the round the report is of.
	Round int `json:"round"`
	// Mark is MarkRed or MarkGreen; the copies run never changes it.
	Mark string `json:"mark"`
	// Live is the `harden-live` call.
	Live LiveRunReport `json:"live"`
	// Copies is the `harden-live` call run with the concurrent copies, nil when none was made.
	Copies *LiveRunReport `json:"copies,omitempty"`
	// OutsideStrayCover are the packages whose test files hold no `tmuxkit.Main(` call.
	OutsideStrayCover []string `json:"outside_stray_cover,omitempty"`
	// Deferred are the failing lines of a red `harden-live` call, deferred to the run's findings.
	// A red call whose output names no failing test defers one line naming the report instead.
	Deferred []string `json:"deferred,omitempty"`
}

// liveReportPath returns the path of the live report of round in runDir.
func liveReportPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("harden-live-%d.json", round))
}

// ReadLiveReport reads the live report of round from runDir, and reports whether one was written.
func ReadLiveReport(runDir string, round int) (LiveReport, bool, error) {
	path := liveReportPath(runDir, round)
	return state.ReadJSON[LiveReport](path, lock.TrackedFileLockPath(path, ".lock"))
}

// failingLine reports whether a line of a gate call's output names a failure.
func failingLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "--- FAIL") || strings.HasPrefix(trimmed, "FAIL") || strings.HasPrefix(trimmed, "panic:")
}

// reportRun summarises one gate call's output.
func reportRun(run LiveRun) LiveRunReport {
	report := LiveRunReport{Passed: run.Passed, Refused: run.Refused}
	for _, started := range gateslot.ParseStartedLines(run.Output) {
		report.Started += started.Started
	}
	for _, line := range strings.Split(run.Output, "\n") {
		line = strings.TrimRight(line, "\r")
		if failingLine(line) {
			report.FailingLines = append(report.FailingLines, strings.TrimSpace(line))
		}
		if report.Clamp == "" && strings.HasPrefix(line, copiesClampPrefix) {
			report.Clamp = line
		}
	}
	return report
}

// packagesOutsideStrayCover returns the packages whose test files, read under root, hold no `tmuxkit.Main(` call.
func packagesOutsideStrayCover(root string, packages []string) ([]string, error) {
	var outside []string
	for _, pkg := range packages {
		tests, err := filepath.Glob(filepath.Join(root, pkg, "*_test.go"))
		if err != nil {
			return nil, fmt.Errorf("list the test files of package %q: %w", pkg, err)
		}
		covered := false
		for _, test := range tests {
			source, err := os.ReadFile(test)
			if err != nil {
				return nil, fmt.Errorf("read test file %q: %w", test, err)
			}
			if strings.Contains(string(source), strayCoverCall) {
				covered = true
				break
			}
		}
		if !covered {
			outside = append(outside, pkg)
		}
	}
	return outside, nil
}

// NewLiveGate returns the live gate, which runs the campaign's live tests after a fix round and reports the round.
// Each call runs `harden-live` once, over the campaign's packages, or over the key's own scope when it names none.
// It then runs `harden-live` again with the concurrent copies when copies run,
// so the copies run on the tier the key names:
// the tests drive no LLM session, concurrency is above 1 and the campaign names packages.
// It writes the round's live report and fails when the mark is red.
// The mark is red when the `harden-live` run failed.
// It is also red when the run started no test binary for a campaign that names packages;
// with none named, a scope that holds no test starts none.
// A refused `harden-live` call, or a call that could not run, returns an error and writes no report.
// A refused copies call is recorded as refused and leaves the mark alone.
func NewLiveGate(deps LiveDeps) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		round, err := deps.Round()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("live gate: name the round: %w", err)
		}
		live, err := deps.Run(LiveGateKey, 0, deps.Packages)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("live gate: run %s: %w; %s", LiveGateKey, err, liveWayForward)
		}
		if live.Refused != "" {
			return shuttleengine.GateResult{}, fmt.Errorf("live gate: %w", errors.New(live.Refused))
		}
		report := LiveReport{Round: round, Live: reportRun(live)}
		if copiesRun(deps.LLMDriving, deps.Concurrency, deps.Packages) {
			copies, err := deps.Run(LiveGateKey, deps.Concurrency, deps.Packages)
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("live gate: run %s with %d copies: %w; %s", LiveGateKey, deps.Concurrency, err, liveWayForward)
			}
			copiesReport := reportRun(copies)
			report.Copies = &copiesReport
		}
		if report.OutsideStrayCover, err = packagesOutsideStrayCover(deps.WorktreeRoot, deps.Packages); err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("live gate: %w", err)
		}

		path := liveReportPath(deps.RunDir, round)
		report.Mark = MarkGreen
		if !report.Live.Passed || (report.Live.Started == 0 && len(deps.Packages) > 0) {
			report.Mark = MarkRed
			report.Deferred = report.Live.FailingLines
			if len(report.Deferred) == 0 {
				report.Deferred = []string{fmt.Sprintf("the `%s` run failed or started no test binary and its output names no failing test; read %s", LiveGateKey, path)}
			}
		}
		if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), report); err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("live gate: write the live report: %w", err)
		}
		if report.Mark == MarkGreen {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return shuttleengine.GateResult{Findings: redFindings(path, report.Live)}, nil
	}
}

// redFindings renders the findings of a red live run: the report to read and the failing lines.
func redFindings(path string, live LiveRunReport) string {
	var findings strings.Builder
	fmt.Fprintf(&findings, "The live run of %s is red; its report is %s.\n", LiveGateKey, path)
	if live.Started == 0 {
		fmt.Fprintf(&findings, "It started no test binary, so no live test ran.\n")
	}
	for _, line := range live.FailingLines {
		fmt.Fprintf(&findings, "- %s\n", line)
	}
	return findings.String()
}
