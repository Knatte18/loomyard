// runner.go builds a package's coverage test binary and lists its top-level tests.
// It runs each test alone with its own profile, each command under its own bound when the toolchain sets one.

package testcover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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
	// Dir is the working directory of each go command; empty runs it in the process's.
	Dir string
	// Ctx ends every command it runs once it is done, killing the command's process group; nil never ends one.
	Ctx context.Context
	// BuildTimeout bounds each go command; zero leaves it unbounded.
	BuildTimeout time.Duration
	// TestTimeout bounds each test run alone; zero leaves it unbounded.
	TestTimeout time.Duration
	// MemoryLimit caps the bytes each test binary's run may hold, with every process it starts; zero leaves it uncapped.
	MemoryLimit int64
	// GuardKilled reports whether a memory guard outside the toolchain killed the test binary with a pid,
	// or a test binary that binary started.
	// Nil means none runs.
	// RunAlone asks it once about each run that failed, and returns a run it names as GuardKilled.
	GuardKilled func(pid int) bool
	// Grace bounds the wait for a bounded command's exit and output once it ended or was killed; zero waits for both.
	Grace time.Duration
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
	// TimedOut marks a run killed at TestTimeout; its Action is ActionFail.
	TimedOut bool
	// MemoryCapped marks a run ended for passing MemoryLimit; its Action is ActionFail.
	MemoryCapped bool
	// GuardKilled marks a run the toolchain's memory guard killed for the machine's memory, not the test's own cap;
	// its Action is ActionFail, and it says nothing about the test.
	GuardKilled bool
}

// BuildBinary builds the coverage test binary of the package at importPath into out.
// The binary counts blocks in set mode over the packages coverpkg names, comma-separated, under the build tags tags.
// The error carries the go command's stderr, or says the build timed out.
func (tc Toolchain) BuildBinary(importPath, coverpkg, tags, out string) error {
	args := appendTags([]string{"test", "-c", "-o", out, "-cover", "-covermode=set", "-coverpkg=" + coverpkg}, tags)
	if _, err := tc.goOutput(append(args, importPath), "CGO_ENABLED=1"); err != nil {
		return fmt.Errorf("coverage binary: %w", err)
	}
	return nil
}

// linkedPackagesFormat is the `go list -f` template of LinkedPackages.
// It prints, for each package of the main module, its import path, name and the package it is compiled for.
// The fields are tab-separated.
const linkedPackagesFormat = `{{if and .Module .Module.Main}}{{.ImportPath}}{{"\t"}}{{.Name}}{{"\t"}}{{.ForTest}}{{end}}`

// LinkedPackages returns the import paths of the main module's packages that importPath's test binary links.
// The result is sorted and includes the package itself.
// They are the packages a coverage build of the package can instrument.
// The external test package and the generated test main are left out, since no other package imports them.
func (tc Toolchain) LinkedPackages(importPath string) ([]string, error) {
	out, err := tc.goOutput([]string{"list", "-deps", "-test", "-f", linkedPackagesFormat, importPath})
	if err != nil {
		return nil, fmt.Errorf("list linked packages: %w", err)
	}
	var linked []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		// A package compiled for a test carries its test binary's name in brackets after its import path.
		path, _, _ := strings.Cut(fields[0], " ")
		name, forTest := fields[1], fields[2]
		if (name == "main" && strings.HasSuffix(path, ".test")) || (forTest != "" && path == forTest+"_test") {
			continue
		}
		linked = append(linked, path)
	}
	slices.Sort(linked)
	return slices.Compact(linked), nil
}

// GoEnvironment returns the Go version, GOOS and GOARCH the toolchain's go command reports under Env.
// The error carries the go command's failure, or names output that is not those three lines.
func (tc Toolchain) GoEnvironment() (version, goos, goarch string, err error) {
	out, err := tc.goOutput([]string{"env", "GOVERSION", "GOOS", "GOARCH"})
	if err != nil {
		return "", "", "", fmt.Errorf("go env: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		return "", "", "", fmt.Errorf("go env printed %q; want three lines: GOVERSION, GOOS and GOARCH", out)
	}
	return lines[0], lines[1], lines[2], nil
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
// A failed test that GuardKilled names is returned marked GuardKilled instead, and is never MemoryCapped.
// The error reports a binary that did not start, or a passing test whose profile is missing or malformed.
func (tc Toolchain) RunAlone(bin, dir, profile, name string) (RunResult, error) {
	if profile != "" {
		if err := os.Remove(profile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return RunResult{}, fmt.Errorf("remove the previous profile: %w", err)
		}
	}
	cmd, ctx, cancel := tc.command(tc.TestTimeout, bin, runAloneArgs(name, profile)...)
	defer cancel()
	cmd.Dir = dir
	cmd.Env = tc.Env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	logger.Info("testcover: spawning a coverage binary for one test", "binary", bin, "test", name, "memory_limit", tc.MemoryLimit)
	killed, err := tc.runTestBinary(cmd)
	guardKilled := err != nil && tc.GuardKilled != nil && cmd.Process != nil && tc.GuardKilled(cmd.Process.Pid)
	capped := tc.MemoryLimit > 0 && err != nil && !guardKilled && (killed || outOfMemoryLine.Match(out.Bytes()))
	logger.Info("testcover: coverage binary ended", "test", name, "error", err, "memory_capped", capped, "guard_killed", guardKilled)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return RunResult{Name: name, Action: ActionFail, TimedOut: true}, nil
	}
	if guardKilled {
		return RunResult{Name: name, Action: ActionFail, GuardKilled: true}, nil
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
	for _, m := range testResultLine.FindAllStringSubmatch(out.String(), -1) {
		if m[2] == name {
			run.Action = strings.ToLower(m[1])
			run.Elapsed, _ = strconv.ParseFloat(m[3], 64)
		}
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

// ProfileBlocks runs the coverage binary bin in dir with no test, writing its profile to profile.
// It returns the profile's blocks.
// The profile lists every block the binary counts, none of them covered.
// The error reports a binary that did not start, failed, passed MemoryLimit or outlasted TestTimeout,
// or a profile missing or malformed.
func (tc Toolchain) ProfileBlocks(bin, dir, profile string) ([]ProfileBlock, error) {
	cmd, ctx, cancel := tc.command(tc.TestTimeout, bin, "-test.run=^$", "-test.coverprofile="+profile)
	defer cancel()
	cmd.Dir = dir
	cmd.Env = tc.Env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	logger.Info("testcover: spawning a coverage binary for its block table", "binary", bin, "memory_limit", tc.MemoryLimit)
	_, err := tc.runTestBinary(cmd)
	logger.Info("testcover: coverage binary ended", "binary", bin, "error", err)
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	switch {
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, fmt.Errorf("block table run: timed out after %s", tc.TestTimeout)
	case err != nil:
		return nil, fmt.Errorf("block table run: %w: %s", err, strings.TrimSpace(out.String()))
	}
	f, err := os.Open(profile)
	if err != nil {
		return nil, fmt.Errorf("block table run wrote no coverage profile: %w", err)
	}
	defer f.Close()
	return ParseProfile(f)
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

// compileErrorLine matches a compiler error line, capturing its file and line.
var compileErrorLine = regexp.MustCompile(`(?m)^(\S+\.go):(\d+):\d+: `)

// MutantBuildError is a mutant build the compiler refused, with the source lines its errors name.
type MutantBuildError struct {
	Lines []SourceLine
	err   error
}

// SourceLine is one line of a package's file, the file named by its base name.
type SourceLine struct {
	File string
	Line int
}

func (e *MutantBuildError) Error() string { return "mutant binary: " + e.err.Error() }

func (e *MutantBuildError) Unwrap() error { return e.err }

// BuildMutantBinary builds the test binary of the package at importPath into out.
// The files the overlay file names stand in place of the package's own.
// It runs no vet, and asks the compiler for every error of the package rather than its first few.
// A build the compiler refused returns a *MutantBuildError naming the source lines of its errors.
// Any other failure returns an error carrying the go command's stderr, or saying the build timed out.
func (tc Toolchain) BuildMutantBinary(importPath, overlay, out string) error {
	_, err := tc.goOutput([]string{"test", "-c", "-o", out, "-vet=off", "-overlay=" + overlay, "-gcflags=" + importPath + "=-e", importPath})
	if err == nil {
		return nil
	}
	var lines []SourceLine
	for _, m := range compileErrorLine.FindAllStringSubmatch(err.Error(), -1) {
		line, _ := strconv.Atoi(m[2])
		lines = append(lines, SourceLine{File: path.Base(filepath.ToSlash(m[1])), Line: line})
	}
	if len(lines) > 0 {
		return &MutantBuildError{Lines: lines, err: err}
	}
	return fmt.Errorf("mutant binary: %w", err)
}

// appendTags appends the `-tags` flag for tags to args, or nothing when tags is empty.
func appendTags(args []string, tags string) []string {
	if tags == "" {
		return args
	}
	return append(args, "-tags", tags)
}

// goOutput runs the go command with args under the toolchain's environment plus extraEnv, and returns its stdout.
// The error carries stderr, or says the command passed BuildTimeout.
func (tc Toolchain) goOutput(args []string, extraEnv ...string) (string, error) {
	cmd, ctx, cancel := tc.command(tc.BuildTimeout, tc.Go, args...)
	defer cancel()
	cmd.Dir = tc.Dir
	cmd.Env = append(slices.Clone(tc.Env), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	logger.Info("testcover: spawning go", "binary", tc.Go, "args", args)
	err := cmd.Run()
	logger.Info("testcover: go ended", "args", args, "error", err)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("%s %s: timed out after %s", tc.Go, strings.Join(args, " "), tc.BuildTimeout)
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", tc.Go, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// command returns the command running name with args, and the context that bounds it.
// A timeout above zero, or a Ctx, runs the command in its own process group.
// The group is killed whole once the timeout passes or Ctx is done, so no descendant outlives it.
// The wait for the command's exit and output then lasts at most Grace.
// The returned cancel releases the bound once the command ended.
func (tc Toolchain) command(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	if timeout <= 0 && tc.Ctx == nil {
		return exec.Command(name, args...), context.Background(), func() {}
	}
	parent := tc.Ctx
	if parent == nil {
		parent = context.Background()
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, timeout)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	proc.ConfigureGroupKill(cmd, func(pid int, groupErr error) {
		logger.Info("testcover: killed a timed-out process group", "pid", pid, "error", groupErr)
	})
	cmd.WaitDelay = tc.Grace
	return cmd, ctx, cancel
}
