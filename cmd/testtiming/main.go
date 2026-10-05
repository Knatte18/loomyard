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
// It shells out to `go test ./... -json -count=1` (adding `-tags <tags>` when tags are given),
// parses the JSON event stream, and prints per-package times, the measured wall-clock, and the slowest top-level tests.
// Each package row carries its top-level test count (TESTS) and serial time (SERIAL), the sum of those tests' elapsed seconds; subtests count in neither.
// The header line shows the exact command run, tags included.
// Exit code mirrors the underlying `go test`: 0 on success, 1 if any package failed to build or any test failed.
//
// The -redundancy mode replaces the timing table with a coverage report:
//
//	go run ./cmd/testtiming -redundancy [-pkg ./internal/x] [-out report.md] [-tags t]
//
// -tags defaults to integration,tmux in this mode, every tier but llm.
// For each package it runs every top-level test alone under one coverage binary and writes, per package,
// the tests whose covered blocks other tests already cover, the removable set among them,
// and the tests whose coverage cannot be judged (skipped, covering nothing, or spawning a subprocess, which the static scan in spawnscan.go decides).
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
	pkg := flag.String("pkg", defaultRedundancyPkg, "with -redundancy: the packages to measure")
	flag.Parse()

	var err error
	if *redundancy {
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

	cmd := exec.Command("go", args...)
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
