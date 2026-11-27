// runner.go builds a package's coverage test binary and lists its top-level tests.
// It runs each test alone with its own profile, each test under its own bound when the toolchain sets one.

package testcover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// Outcomes a test run reads from the coverage binary's verbose output.
const (
	ActionPass = "pass"
	ActionFail = "fail"
	ActionSkip = "skip"
)

// Toolchain runs the go command and the coverage binaries it builds.
type Toolchain struct {
	// Go names the go command.
	Go string
	// Env is the whole environment every command runs under.
	Env []string
	// TestTimeout bounds each test run alone; zero leaves it unbounded.
	TestTimeout time.Duration
	// MemoryLimit caps the bytes each test binary's run may hold, with every process it starts; zero leaves it uncapped.
	MemoryLimit int64
	// Grace bounds the wait for a bounded command's exit and output once it ended or was killed; zero waits for both.
	Grace time.Duration
	// Now stamps the arrival of a test run's `=== RUN` line and result line; nil uses time.Now.
	Now func() time.Time
}

// RunResult is one top-level test run alone under a coverage binary.
type RunResult struct {
	Name string
	// Elapsed is the test's seconds, to the microsecond, from the arrival of its `=== RUN` line to that of its result line.
	// A run whose output holds no `=== RUN` line before the result line has the binary's own figure.
	// The binary rounds that figure to the hundredth.
	Elapsed float64
	// Action is ActionPass, ActionFail or ActionSkip.
	Action string
	// Profile holds every block of the run's profile, covered or not; it is nil for a failed run.
	Profile []ProfileBlock
	// TimedOut marks a run killed at TestTimeout; its Action is ActionFail.
	TimedOut bool
	// MemoryCapped marks a run ended for passing MemoryLimit; its Action is ActionFail.
	MemoryCapped bool
}

// BuildBinary builds the coverage test binary of the package at importPath into out.
// The binary counts blocks in set mode over the packages coverpkg names, comma-separated, under the build tags tags.
// The error carries the go command's stderr.
func (tc Toolchain) BuildBinary(importPath, coverpkg, tags, out string) error {
	args := appendTags([]string{"test", "-c", "-o", out, "-cover", "-covermode=set", "-coverpkg=" + coverpkg}, tags)
	if _, err := tc.goOutput(append(args, importPath), "CGO_ENABLED=1"); err != nil {
		return fmt.Errorf("coverage binary: %w", err)
	}
	return nil
}

var testNameLine = regexp.MustCompile(`^(Test|Example|Fuzz)\w*$`)

// ListTests returns the top-level test names of the package at importPath under the build tags tags.
func (tc Toolchain) ListTests(importPath, tags string) ([]string, error) {
	out, err := tc.goOutput(append(appendTags([]string{"test", "-list", ".*"}, tags), importPath))
	if err != nil {
		return nil, fmt.Errorf("list tests: %w", err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); testNameLine.MatchString(name) {
			names = append(names, name)
		}
	}
	return names, nil
}

var testResultLine = regexp.MustCompile(`(?m)^--- (PASS|FAIL|SKIP): (\S+) \(([0-9.]+)s\)$`)

// outOfMemoryLine matches the Go runtime's fatal error for memory the host refused it.
// A test binary prints it where a memory cap fails the allocation instead of killing the process.
var outOfMemoryLine = regexp.MustCompile(`(?m)^fatal error: .*(out of memory|cannot allocate memory)`)

// RunAlone runs the top-level test name alone under the test binary bin in dir, writing its profile to profile.
// It reads the test's outcome and elapsed time from the binary's verbose output,
// and the profile's blocks for a test that passed, or skipped after writing one.
// A profile of "" runs the test with no coverage profile, as a binary built without coverage needs, and reads none.
// A failing test is returned as such, never as an error, and so is a test killed at TestTimeout, marked TimedOut.
// So is a test ended for passing MemoryLimit, marked MemoryCapped:
// the kernel killed it, or the run failed with the Go runtime's out-of-memory error in its output.
// The error reports a binary that did not start, or a passing test whose profile is missing or malformed.
func (tc Toolchain) RunAlone(bin, dir, profile, name string) (RunResult, error) {
	if profile != "" {
		if err := os.Remove(profile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RunResult{}, fmt.Errorf("remove the previous profile: %w", err)
		}
	}
	cmd, ctx, cancel := tc.testCommand(bin, runAloneArgs(name, profile)...)
	defer cancel()
	cmd.Dir = dir
	cmd.Env = tc.Env
	out := &testOutput{name: name, now: tc.Now}
	if out.now == nil {
		out.now = time.Now
	}
	cmd.Stdout, cmd.Stderr = out, out
	logger.Info("testcover: spawning a coverage binary for one test", "binary", bin, "test", name, "memory_limit", tc.MemoryLimit)
	killed, err := tc.runTestBinary(cmd)
	capped := tc.MemoryLimit > 0 && err != nil && (killed || outOfMemoryLine.Match(out.buf.Bytes()))
	logger.Info("testcover: coverage binary ended", "test", name, "error", err, "memory_capped", capped)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return RunResult{Name: name, Action: ActionFail, TimedOut: true}, nil
	}
	if capped {
		return RunResult{Name: name, Action: ActionFail, MemoryCapped: true}, nil
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return RunResult{}, fmt.Errorf("run %s: %w", name, err)
	}

	run := RunResult{Name: name, Action: ActionFail}
	for _, m := range testResultLine.FindAllStringSubmatch(out.buf.String(), -1) {
		if m[2] == name {
			run.Action = strings.ToLower(m[1])
			run.Elapsed, _ = strconv.ParseFloat(m[3], 64)
		}
	}
	if !out.ended.IsZero() {
		run.Elapsed = out.ended.Sub(out.started).Round(time.Microsecond).Seconds()
	}
	if run.Action == ActionFail || profile == "" {
		return run, nil
	}
	f, err := os.Open(profile)
	if err != nil {
		if run.Action == ActionSkip {
			return run, nil
		}
		return RunResult{}, fmt.Errorf("test %s wrote no coverage profile: %w", name, err)
	}
	defer f.Close()
	if run.Profile, err = ParseProfile(f); err != nil {
		return RunResult{}, fmt.Errorf("test %s: %w", name, err)
	}
	return run, nil
}

// runLinePrefix opens the line a verbose test binary prints as a test starts, before the test's name.
const runLinePrefix = "=== RUN   "

// testOutput collects the output of a test binary running one top-level test.
// It stamps the arrival of the test's `=== RUN` line and of the result line that follows it.
// The binary's own figure is rounded to the hundredth and records most tests as taking no time.
// A binary's stdout and stderr share one testOutput, which exec then writes from a single goroutine.
type testOutput struct {
	name string
	now  func() time.Time
	buf  bytes.Buffer
	// scanned is the length of the complete lines of buf already stamped.
	scanned int
	// started and ended are the arrival times of the two lines, zero until each arrives.
	started, ended time.Time
}

// Write implements io.Writer, stamping each line it completes.
func (o *testOutput) Write(p []byte) (int, error) {
	o.buf.Write(p)
	for {
		rest := o.buf.Bytes()[o.scanned:]
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			return len(p), nil
		}
		o.stamp(string(rest[:end]))
		o.scanned += end + 1
	}
}

// stamp records the arrival of line when it is the test's first `=== RUN` line.
// It also records the arrival of the first result line of the test after it.
func (o *testOutput) stamp(line string) {
	switch {
	case o.started.IsZero():
		if line == runLinePrefix+o.name {
			o.started = o.now()
		}
	case o.ended.IsZero() && strings.HasPrefix(line, "--- "):
		if m := testResultLine.FindStringSubmatch(line); m != nil && m[2] == o.name {
			o.ended = o.now()
		}
	}
}

// runAloneArgs returns the test binary's arguments for running one top-level test alone.
// It passes no profile flag for profile "".
// Every flag is one `-flag=value` argument, the form `go test` passes:
// a test that runs a cobra command without `SetArgs` reads the process's arguments,
// and a separate flag value would show up there as a positional argument.
func runAloneArgs(name, profile string) []string {
	args := []string{"-test.run=^" + regexp.QuoteMeta(name) + "$"}
	if profile != "" {
		args = append(args, "-test.coverprofile="+profile)
	}
	return append(args, "-test.v=true")
}

// runTestBinary runs cmd, the command of a test binary, as cmd.Run does, under MemoryLimit when the toolchain sets one.
// killed reports a run the kernel killed for passing the limit.
// The go commands never run through it, since a build's memory is not a test's.
func (tc Toolchain) runTestBinary(cmd *exec.Cmd) (killed bool, err error) {
	if tc.MemoryLimit <= 0 {
		return false, cmd.Run()
	}
	return proc.RunMemoryCapped(cmd, tc.MemoryLimit)
}

// appendTags appends the `-tags` flag for tags to args, or nothing when tags is empty.
func appendTags(args []string, tags string) []string {
	if tags == "" {
		return args
	}
	return append(args, "-tags", tags)
}

// goOutput runs the go command with args under the toolchain's environment plus extraEnv, and returns its stdout.
// The error carries stderr.
func (tc Toolchain) goOutput(args []string, extraEnv ...string) (string, error) {
	cmd := exec.Command(tc.Go, args...)
	cmd.Env = append(slices.Clone(tc.Env), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	logger.Info("testcover: spawning go", "binary", tc.Go, "args", args)
	err := cmd.Run()
	logger.Info("testcover: go ended", "args", args, "error", err)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", tc.Go, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// testCommand returns the command running a test binary name with args, and the context that bounds it.
// A TestTimeout above zero runs the command in its own process group.
// The group is killed whole once the timeout passes, so no descendant outlives it.
// The wait for the command's exit and output then lasts at most Grace.
// The returned cancel releases the bound once the command ended.
func (tc Toolchain) testCommand(name string, args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	if tc.TestTimeout <= 0 {
		return exec.Command(name, args...), context.Background(), func() {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), tc.TestTimeout)
	cmd := exec.CommandContext(ctx, name, args...)
	proc.ConfigureGroupKill(cmd, func(pid int, groupErr error) {
		logger.Info("testcover: killed a timed-out process group", "pid", pid, "error", groupErr)
	})
	cmd.WaitDelay = tc.Grace
	return cmd, ctx, cancel
}
