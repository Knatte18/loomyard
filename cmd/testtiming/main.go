// Package main runs the repo's Go test suite and prints a wall-clock timing table.
// It reports each package's time separately, so a slow package or test is not hidden in one combined number.
//
// It produces the tiers documented in docs/benchmarks/test-suite-timing.md:
//
//	go run ./cmd/testtiming                  # tier0 — offline, fast (no git subprocesses); runs -tags tier0
//	go run ./cmd/testtiming -full            # tier1 — slow (real git); runs -tags tier1
//	go run ./cmd/testtiming -tags tier1,tier2 # any tag set; -full means -tags tier1
//
// A test binary holds every tier's tests and runs those of the tiers LYM_TEST_TIER selects.
// Every mode therefore sets it from the tags: tier1 alone runs none of tier0's tests,
// and a run that wants several tiers names every tag.
// The tags must name one range of tiers, such as tier0,tier1,tier2;
// passing both -full and -tags is refused.
//
// It shells out to `go test ./... -json -count=1 -tags <tags>`, with tier0 when no tags are given.
// It parses the JSON event stream.
// It prints per-package times, the measured wall-clock, and the slowest top-level tests.
// Each package row carries its top-level test count (TESTS) and its serial time (SERIAL).
// SERIAL is the sum of those tests' elapsed seconds.
// Subtests count in neither.
// The header line shows the exact command run, tags included.
// Exit code mirrors the underlying `go test`: 0 on success, 1 if any package failed to build or any test failed.
//
// The -resources mode replaces the timing table with a per-package resource table:
//
//	go run ./cmd/testtiming -resources [-pkg ./internal/x,./internal/y] [-tags t | -full]
//
// It runs the selected packages one at a time, each as its own `go test -json -count=1` child over one package.
// In a hub, each child runs inside the one gate slot the run holds; elsewhere it runs under the CPU count.
// Every mode, the timing table and -redundancy included, holds one slot for its whole run in a hub.
// Its `lym` prebuild inherits that slot, and every mode sets its go commands' GOFLAGS through gateslot.HolderEnv.
// It measures outside `lym gate test`, since its modes measure what a tier run hides.
// -pkg takes comma-separated package patterns and defaults to every package;
// -tags and -full pick the tier as in the timing mode, tier0 when neither is given.
// The columns are PACKAGE, WALL (child start to exit), CPU, PEAK_MEM, PROCS and LEFTOVER, plus a FAIL mark.
// CPU and PEAK_MEM come from the child's cgroup when `systemd-run --user --scope` works.
// They are read from cpu.stat's usage_usec and memory.peak while the scope lives.
// Otherwise they come from the child's rusage.
// There PEAK_MEM is the largest single process's max RSS, and the row carries a rusage mark.
// PROCS is the delta of the `processes` line of /proc/stat across the child,
// so it is system-wide and blank where /proc/stat is unreadable.
// LEFTOVER counts the processes still carrying the package's marker after the child exited;
// the marker is an environment variable every descendant of the child inherits.
// A trailing list names each as `pid argv`.
// GIT_FIXTURE and GIT_CODE count the git processes the package ran, split on the fixture marker `LYM_FIXTURE_GIT`:
// git that gitkit's spawn helpers or a hub build started carries it, and all other git is code under test.
// The census reads git's trace2 event files, one per git process.
// They sit in a trace directory the child's environment points at.
// A second table after the first lists the processes by subcommand with total, fixture and code counts.
// The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package,
// so a before-to-after comparison reads the total.
// The table ends with the one-minute load average at start and at end,
// so a report reads "quiet machine" as a precondition.
// The measurements are Linux-first.
// Elsewhere the cgroup and /proc readings are absent, and the columns that need them stay blank.
// -resources together with -redundancy is refused.
//
// The -redundancy mode replaces the timing table with a coverage report:
//
//	go run ./cmd/testtiming -redundancy [-pkg ./internal/x,./internal/y] [-out report.md] [-tags t]
//
// -pkg takes comma-separated package patterns, each passed to `go` as its own argument.
// -tags defaults to tier0,tier1,tier2 in this mode, every tier but tier3.
// A run over every package of the module, or with no report at -out yet, writes the whole report.
// Any other run splices into the existing report: only the measured packages' sections change.
// A package with no section yet is inserted in import-path order, and a package that lists no tests loses its section.
// The header and every unmeasured section stay byte-identical,
// and a run whose tags differ from the report's header refuses and writes nothing.
// A package that fails to list or build keeps its old section,
// and the run exits 1 after writing the others.
// For each package it runs every top-level test alone under one coverage binary.
// Per package it writes the tests whose covered blocks other tests already cover.
// It also writes the removable set among them, and the tests whose coverage cannot be judged.
// Coverage cannot be judged for a test that is skipped or covers nothing.
// Nor can it be judged for one that may run this module's code in another process.
// The static scan in spawnscan.go decides the last of these.
// It excludes a test as out of process when it references the lymbin kit or os.Executable.
// It also excludes one that references os.Args[0] or exec.Command("go", ...).
// Either reference counts directly or through a followed same-package or testkit call.
// It also excludes a test that sits in a tmux-tier file.
// A test in a tier0 file is always judged,
// and a test above tier0 with a call the scan cannot resolve is excluded as an unclassifiable call.
// The scan does not see a hook, alias or helper that a test installs so that git runs module code.
// Reading each candidate before deleting it is the backstop.
// A test the prune keeps carries `//testtiming:keep <reason>` on the line directly above its `func Test…` line.
// A test that also carries `//lym:guard` stacks both directives above its `func Test…` line.
// They are contiguous lines in either order.
// The keep may sit above `//lym:` lines but never above another keep.
// The report lists it under "Kept" with the reason and leaves it out of the candidates table.
// It stays in the covering pool and the removable computation.
// A directive with an empty reason, or one not directly above a top-level test, past any `//lym:` lines, aborts the run.
// It does so before anything is measured or written.
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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
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
	full := flag.Bool("full", false, "run the tier1 tier (same as -tags tier1): real git, slow (~a minute)")
	tagFlag := flag.String("tags", "", "build tags to run under, comma-separated (e.g. tier1,tier2); excludes -full")
	top := flag.Int("top", 15, "how many of the slowest top-level tests to list")
	redundancy := flag.Bool("redundancy", false, "per-test coverage redundancy mode: write a markdown report instead of the timing table")
	out := flag.String("out", defaultRedundancyOut, "with -redundancy: the markdown report to write")
	pkg := flag.String("pkg", defaultRedundancyPkg, "with -redundancy or -resources: the package patterns to measure, comma-separated")
	resources := flag.Bool("resources", false, "per-package resource mode: measure wall, CPU, peak memory, processes and leftovers, one package at a time in a gate slot")
	flag.Parse()

	err := modeConflict(*redundancy, *resources)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testtiming:", err)
		lymtemp.Remove()
		os.Exit(1)
	}
	var tags string
	if *redundancy {
		tags, err = redundancyTags(*full, *tagFlag)
	} else {
		tags, err = resolveTags(*full, *tagFlag)
	}
	if err == nil {
		err = exportTierSelection(tags)
	}
	if err == nil {
		switch {
		case *resources:
			err = runResources(tags, *pkg)
		case *redundancy:
			err = runRedundancy(tags, *pkg, *out)
		default:
			err = run(tags, *top)
		}
	}
	if removeErr := lymtemp.Remove(); removeErr != nil {
		fmt.Fprintln(os.Stderr, "testtiming:", removeErr)
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

// offlineTier is the tag of the offline tier, which runs no git subprocesses;
// a run with no tags of its own measures that tier.
const offlineTier = "tier0"

// resolveTags turns the -full and -tags flags into the build-tag string, empty for a run with no tags of its own.
func resolveTags(full bool, tags string) (string, error) {
	if full && tags != "" {
		return "", errors.New("-full and -tags are mutually exclusive: -full means -tags tier1")
	}
	if full {
		return "tier1", nil
	}
	return tags, nil
}

// isOfflineRun reports whether tags select the offline tier: none, or tier0 alone.
func isOfflineRun(tags string) bool {
	return tags == "" || tags == offlineTier
}

// tierSelectorEnv names the environment variable a test binary reads its tier selection from;
// every go command of a run inherits it from this process.
const tierSelectorEnv = "LYM_TEST_TIER"

// tierSelection returns the LYM_TEST_TIER value that selects the tiers tags name: one tier number, or `<low>-<high>` for several.
// Tags that name no tier are ignored; when no tier is left, the selection is tier0.
// It refuses tiers that do not form one range, such as tier0,tier2, since the selector takes one range.
func tierSelection(tags string) (string, error) {
	var tiers []int
	for _, tag := range strings.Split(tags, ",") {
		number, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(tag), "tier"))
		if err != nil || !strings.HasPrefix(strings.TrimSpace(tag), "tier") || slices.Contains(tiers, number) {
			continue
		}
		tiers = append(tiers, number)
	}
	if len(tiers) == 0 {
		return "0", nil
	}
	low, high := slices.Min(tiers), slices.Max(tiers)
	if high-low+1 != len(tiers) {
		return "", fmt.Errorf("-tags %s names tiers that do not form one range, and %s selects one range; name every tier from tier%d to tier%d", tags, tierSelectorEnv, low, high)
	}
	if low == high {
		return strconv.Itoa(low), nil
	}
	return fmt.Sprintf("%d-%d", low, high), nil
}

// goTestTags returns the tags a `go` command runs under: tags, or the offline tier when a run has none of its own.
func goTestTags(tags string) string {
	if tags == "" {
		return offlineTier
	}
	return tags
}

// exportTierSelection sets LYM_TEST_TIER in this process to the selection that tags name,
// so every go command the run starts inherits it.
// It sets nothing and returns an error for tags that do not form one range.
func exportTierSelection(tags string) error {
	selection, err := tierSelection(goTestTags(tags))
	if err != nil {
		return err
	}
	return os.Setenv(tierSelectorEnv, selection)
}

func run(tags string, top int) error {
	goTags := goTestTags(tags)
	args := []string{"test", "-tags", goTags}
	cmdline := "go test -tags " + goTags
	tier := "Tags: " + goTags
	if isOfflineRun(tags) {
		tier = "Tier 0 (offline)"
	}
	args = append(args, "./...", "-json", "-count=1")
	cmdline += " ./... -count=1"

	root, err := lymcwd.Getwd()
	if err != nil {
		return fmt.Errorf("read the working directory: %w", err)
	}
	return withGateSlot(root, "testtiming", func(slotEnv []string, parallel int) error {
		env, cleanup, err := prebuildLym("go", tags, root, slotEnv, parallel)
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
		if !isOfflineRun(tags) {
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
	})
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

	// Subtest names contain a slash, so a name without one is a top-level test.
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
