// runner.go builds a package's coverage test binary and lists its top-level tests.
// It runs each test alone with its own profile.

package testcover

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
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
}

// RunResult is one top-level test run alone under a coverage binary.
type RunResult struct {
	Name string
	// Elapsed is the seconds the binary reported for the test.
	Elapsed float64
	// Action is ActionPass, ActionFail or ActionSkip.
	Action string
	// Profile holds every block of the run's profile, covered or not; it is nil for a failed run.
	Profile []ProfileBlock
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

// RunAlone runs the top-level test name alone under the coverage binary bin in dir, writing its profile to profile.
// It reads the test's outcome and elapsed time from the binary's verbose output,
// and the profile's blocks for a test that passed, or skipped after writing one.
// A failing test is returned as such, never as an error.
// The error reports a binary that did not start, or a passing test whose profile is missing or malformed.
func (tc Toolchain) RunAlone(bin, dir, profile, name string) (RunResult, error) {
	if err := os.Remove(profile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return RunResult{}, fmt.Errorf("remove the previous profile: %w", err)
	}
	cmd := exec.Command(bin, runAloneArgs(name, profile)...)
	cmd.Dir = dir
	cmd.Env = tc.Env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	logger.Info("testcover: spawning a coverage binary for one test", "binary", bin, "test", name)
	err := cmd.Run()
	logger.Info("testcover: coverage binary ended", "test", name, "error", err)
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return RunResult{}, fmt.Errorf("run %s: %w", name, err)
	}

	run := RunResult{Name: name, Action: ActionFail}
	for _, m := range testResultLine.FindAllStringSubmatch(out.String(), -1) {
		if m[2] == name {
			run.Action = strings.ToLower(m[1])
			run.Elapsed, _ = strconv.ParseFloat(m[3], 64)
		}
	}
	if run.Action == ActionFail {
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

// runAloneArgs returns the coverage binary's arguments for running one top-level test alone.
// Every flag is one `-flag=value` argument, the form `go test` passes:
// a test that runs a cobra command without `SetArgs` reads the process's arguments,
// and a separate flag value would show up there as a positional argument.
func runAloneArgs(name, profile string) []string {
	return []string{
		"-test.run=^" + regexp.QuoteMeta(name) + "$",
		"-test.coverprofile=" + profile,
		"-test.v=true",
	}
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
