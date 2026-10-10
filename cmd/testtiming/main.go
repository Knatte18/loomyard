// Command testtiming runs the repo's Go test suite and prints a wall-clock
// timing table, so a slow package or test is visible on its own rather than
// hidden in one combined number.
//
// It is the runnable companion to docs/benchmarks/test-suite-timing.md and
// produces the tiers documented there:
//
//	go run ./cmd/testtiming                        # Tier 1 — offline, fast (no git subprocesses)
//	go run ./cmd/testtiming -full                  # Tier 2 — integration, slow (real git; ~a minute)
//	go run ./cmd/testtiming -tags integration,tmux # any tag set; -full means -tags integration
//
// Passing both -full and -tags is refused.
//
// It shells out to `go test ./... -json -count=1` (adding `-tags <tags>` when tags are given), parses the JSON event stream, and prints per-package times, the measured wall-clock, and the slowest top-level tests.
// Each package row carries its top-level test count (TESTS) and serial time (SERIAL), the sum of those tests' elapsed seconds; subtests count in neither.
// The header line shows the exact command run, tags included.
// Exit code mirrors the underlying `go test`: 0 on success, 1 if any package failed to build or any test failed.
//
// The -resources mode replaces the timing table with a per-package resource table:
//
//	go run ./cmd/testtiming -resources [-pkg ./internal/x,./internal/y] [-tags t | -full]
//
// It runs the selected packages one at a time, each as its own `go test -json -count=1` child over one package, inside one gate slot when the working directory is in a hub, and under the template's -p cap otherwise.
// -pkg takes comma-separated package patterns and defaults to every package; -tags and -full pick the tier as in the timing mode.
// Each package gets a private temp directory its child's TMPDIR, TMP and TEMP point at.
// The columns are PACKAGE, WALL (child start to exit), CPU, PEAK_MEM, PROCS and LEFTOVER, plus a FAIL mark.
// CPU and PEAK_MEM come from the child's cgroup when `systemd-run --user --scope` works, read from cpu.stat's usage_usec and memory.peak while the scope lives, and from the child's rusage otherwise, where PEAK_MEM is the largest single process's max RSS and the row carries a rusage mark.
// PROCS is the delta of the `processes` line of /proc/stat across the child, so it is system-wide and blank where /proc/stat is unreadable.
// LEFTOVER counts the processes still referencing the package's temp directory through cwd, executable or argv after the child exited; a trailing list names each as `pid argv`.
// GIT_FIXTURE and GIT_CODE count the git processes the package ran, split on the fixture marker `LYX_FIXTURE_GIT`: git that gitkit's spawn helpers or a hub build started carries it, and all other git is code under test.
// The census reads git's trace2 event files, one per git process, from a trace directory the child's environment points at, and a second table after the first lists the processes by subcommand with total, fixture and code counts.
// The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, so a before-to-after comparison reads the total.
// The table ends with the one-minute load average at start and at end, so a report reads "quiet machine" as a precondition.
// The measurements are Linux-first: elsewhere the cgroup and /proc readings are absent and the columns that need them stay blank.
// -resources together with -redundancy is refused.
//
// The -redundancy mode replaces the timing table with a coverage report:
//
//	go run ./cmd/testtiming -redundancy [-pkg ./internal/x,./internal/y] [-out report.md] [-tags t]
//
// -pkg takes comma-separated package patterns, each passed to `go` as its own argument.
// -tags defaults to integration,tmux in this mode, every tier but llm.
// A run over every package of the module, or with no report at -out yet, writes the whole report.
// Any other run splices into the existing report: only the measured packages' sections change, a package with no section yet is inserted in import-path order, and a package that lists no tests loses its section.
// The header and every unmeasured section stay byte-identical, and a run whose tags differ from the report's header refuses and writes nothing.
// A package that fails to list or build keeps its old section, and the run exits 1 after writing the others.
// For each package it runs every top-level test alone under one coverage binary and writes, per package, the tests whose covered blocks other tests already cover, the removable set among them, and the tests whose coverage cannot be judged (skipped, covering nothing, or possibly running this module's code in another process).
// The static scan in spawnscan.go decides the last: a test is excluded as out of process when, directly or through a followed same-package or testkit call, it references the lyxbin kit, os.Executable, os.Args[0] or exec.Command("go", ...), or when it sits in a tmux-tier file.
// A test in a file with no tier tag is always judged, and a tier-tagged test with a call the scan cannot resolve is excluded as an unclassifiable call.
// The scan does not see a hook, alias or helper that a test installs so that git runs module code.
// Reading each candidate before deleting it is the backstop.
// A test the prune keeps carries `//testtiming:keep <reason>` on the line directly above its `func Test…` line.
// A test that also carries `//lyx:guard` stacks the two as contiguous directive lines above the `func Test…` line, in either order; the keep may sit above `//lyx:` lines but never above another keep.
// The report lists it under "Kept" with the reason and leaves it out of the candidates table, while it stays in the covering pool and the removable computation.
// A directive with an empty reason, or not directly above a top-level test (past any `//lyx:` lines), aborts the run before anything is measured or written.
// It exits 1 after writing the report if any package reported an error.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// testEvent is one line of `go test -json` output (decoded fields only).
type testEvent struct {
	Action  string  // "start" | "run" | "pass" | "fail" | "skip" | "output" | ...
	Package string  // import path, e.g. "github.com/Knatte18/loomyard/internal/boardengine"
	Test    string  // empty for package-level events; "TestX" / "TestX/sub" for tests
	Elapsed float64 // seconds; meaningful on the terminal pass/fail/skip event
	Output  string  // raw test output (only on Action == "output")
}

// pkgResult is the rolled-up outcome for one package.
type pkgResult struct {
	pkg     string
	elapsed float64
	action  string  // terminal action: "pass" | "fail" | "skip"
	noTests bool    // package had no test files (absent from this tier)
	tests   int     // top-level tests that reached a terminal action
	serial  float64 // sum of those tests' elapsed seconds
}

// testResult is one top-level test's timing (subtests are excluded).
type testResult struct {
	pkg     string
	test    string
	elapsed float64
	action  string
}

func main() {
	full := flag.Bool("full", false, "run the integration tier (same as -tags integration): real git, slow (~a minute)")
	tagFlag := flag.String("tags", "", "build tags to run under, comma-separated (e.g. integration,tmux); excludes -full")
	top := flag.Int("top", 15, "how many of the slowest top-level tests to list")
	redundancy := flag.Bool("redundancy", false, "per-test coverage redundancy mode: write a markdown report instead of the timing table")
	out := flag.String("out", defaultRedundancyOut, "with -redundancy: the markdown report to write")
	pkg := flag.String("pkg", defaultRedundancyPkg, "with -redundancy or -resources: the package patterns to measure, comma-separated")
	resources := flag.Bool("resources", false, "per-package resource mode: measure wall, CPU, peak memory, processes and leftovers, one package at a time in a gate slot")
	flag.Parse()

	err := modeConflict(*redundancy, *resources)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testtiming:", err)
		os.Exit(1)
	}
	if *resources {
		var tags string
		if tags, err = resolveTags(*full, *tagFlag); err == nil {
			err = runResources(tags, *pkg)
		}
	} else if *redundancy {
		var tags string
		if tags, err = redundancyTags(*full, *tagFlag); err == nil {
			err = runRedundancy(tags, *pkg, *out)
		}
	} else {
		var tags string
		if tags, err = resolveTags(*full, *tagFlag); err == nil {
			err = run(tags, *top)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testtiming:", err)
		os.Exit(1)
	}
}

// modeConflict refuses -resources together with -redundancy, the two modes that replace the timing table.
func modeConflict(redundancy, resources bool) error {
	if redundancy && resources {
		return errors.New("-resources and -redundancy are mutually exclusive: each replaces the timing table")
	}
	return nil
}

// resolveTags turns the -full and -tags flags into the build-tag string, empty for the untagged tier.
func resolveTags(full bool, tags string) (string, error) {
	if full && tags != "" {
		return "", errors.New("-full and -tags are mutually exclusive: -full means -tags integration")
	}
	if full {
		return "integration", nil
	}
	return tags, nil
}

func run(tags string, top int) error {
	args := []string{"test"}
	cmdline := "go test"
	tier := "Tier 1 (offline)"
	if tags != "" {
		args = append(args, "-tags", tags)
		cmdline += " -tags " + tags
		tier = "Tags: " + tags
	}
	args = append(args, "./...", "-json", "-count=1")
	cmdline += " ./... -count=1"

	root, err := lyxcwd.Getwd()
	if err != nil {
		return fmt.Errorf("read the working directory: %w", err)
	}
	env, cleanup, err := prebuildLyx("go", tags, root, os.Environ())
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.Command("go", args...)
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe stdout: %w", err)
	}
	cmd.Stderr = os.Stderr

	fmt.Printf("Running %s  —  %s\n", tier, cmdline)
	if tags != "" {
		fmt.Println("(real local git; this can take ~a minute)")
	}
	fmt.Println()

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start go test: %w", err)
	}

	pkgs := map[string]*pkgResult{}
	var tests []testResult

	reader := bufio.NewReader(stdout)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			parseLine(line, pkgs, &tests)
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return fmt.Errorf("read go test output: %w", readErr)
			}
			break
		}
	}

	wallErr := cmd.Wait()
	wall := time.Since(start)

	printReport(tier, cmdline, wall, pkgs, tests, top)

	if wallErr != nil {
		var exitErr *exec.ExitError
		if errors.As(wallErr, &exitErr) {
			return fmt.Errorf("go test reported failures (exit %d) — see FAIL rows above", exitErr.ExitCode())
		}
		return fmt.Errorf("go test: %w", wallErr)
	}
	return nil
}

// parseLine decodes one JSON event and folds it into the package/test maps.
func parseLine(line []byte, pkgs map[string]*pkgResult, tests *[]testResult) {
	var ev testEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return
	}
	if ev.Package == "" {
		return
	}

	p := pkgs[ev.Package]
	if p == nil {
		p = &pkgResult{pkg: ev.Package}
		pkgs[ev.Package] = p
	}

	if ev.Action == "output" && strings.Contains(ev.Output, "[no test files]") {
		p.noTests = true
		return
	}

	terminal := ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip"
	if !terminal {
		return
	}

	if ev.Test == "" {
		p.elapsed = ev.Elapsed
		p.action = ev.Action
		return
	}

	// Only keep top-level tests (no "/" => not a subtest).
	if strings.Contains(ev.Test, "/") {
		return
	}
	p.tests++
	p.serial += ev.Elapsed
	*tests = append(*tests, testResult{pkg: ev.Package, test: ev.Test, elapsed: ev.Elapsed, action: ev.Action})
}

// shortPkg trims the module prefix from pkg for display.
func shortPkg(pkg string) string {
	const prefix = "github.com/Knatte18/loomyard/"
	return strings.TrimPrefix(pkg, prefix)
}

func printReport(tier, cmdline string, wall time.Duration, pkgs map[string]*pkgResult, tests []testResult, top int) {
	ordered := make([]*pkgResult, 0, len(pkgs))
	for _, p := range pkgs {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].noTests != ordered[j].noTests {
			return !ordered[i].noTests
		}
		return ordered[i].elapsed > ordered[j].elapsed
	})

	var sum float64
	var failures []string
	for _, p := range ordered {
		sum += p.elapsed
		if p.action == "fail" {
			failures = append(failures, shortPkg(p.pkg))
		}
	}

	fmt.Println("PACKAGE                                   ELAPSED   TESTS    SERIAL")
	fmt.Println("----------------------------------------  --------  -----  --------")
	for _, p := range ordered {
		elapsed := fmt.Sprintf("%.2fs", p.elapsed)
		if p.noTests {
			elapsed = "(no test files)"
		}
		mark := ""
		if p.action == "fail" {
			mark = "  FAIL"
		}
		fmt.Printf("%-40s  %8s  %5d  %7.2fs%s\n", shortPkg(p.pkg), elapsed, p.tests, p.serial, mark)
	}

	fmt.Println()
	fmt.Printf("Wall-clock: %.2fs   (sum of package times: %.2fs across %d packages)\n",
		wall.Seconds(), sum, len(ordered))

	sort.Slice(tests, func(i, j int) bool { return tests[i].elapsed > tests[j].elapsed })
	if n := top; n > 0 && len(tests) > 0 {
		if n > len(tests) {
			n = len(tests)
		}
		fmt.Printf("\nSlowest %d top-level tests\n", n)
		fmt.Println("TEST                                      PACKAGE                         ELAPSED")
		fmt.Println("----------------------------------------  ------------------------------  --------")
		for _, t := range tests[:n] {
			mark := ""
			if t.action == "fail" {
				mark = "  FAIL"
			}
			fmt.Printf("%-40s  %-30s  %7.2fs%s\n", t.test, shortPkg(t.pkg), t.elapsed, mark)
		}
	}

	fmt.Println()
	if len(failures) > 0 {
		fmt.Printf("RESULT: FAIL — %d package(s) failed: %s\n", len(failures), strings.Join(failures, ", "))
	} else {
		fmt.Println("RESULT: all packages passed")
	}
}
