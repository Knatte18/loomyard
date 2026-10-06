package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultRedundancyTags = "integration,tmux"
	defaultRedundancyOut  = "docs/benchmarks/test-redundancy.md"
	defaultRedundancyPkg  = "./..."
)

// Reasons a test is listed under "no coverage" rather than judged for redundancy.
const (
	reasonSkipped    = "skipped"
	reasonSpawns     = "spawns"
	reasonUnresolved = "unclassifiable call"
	reasonNoBlocks   = "no blocks"
	reasonFailed     = "failed"
)

// testRun is one top-level test run alone under the coverage binary.
type testRun struct {
	name    string
	elapsed float64
	action  string // "pass" | "fail" | "skip"
	blocks  map[string]struct{}
	spawn   spawnVerdict
}

// eligible reports whether the run's blocks count as coverage the package keeps.
func (r testRun) eligible() bool { return r.action == "pass" && len(r.blocks) > 0 }

// verdict is the redundancy classification of one test.
type verdict struct {
	name      string
	reason    string   // non-empty: listed under "no coverage"
	candidate bool     // every block is covered by other tests of the package
	removable bool     // dropped by the greedy pass, so deletable together with the other removable tests
	covering  []string // candidates only: tests that together cover its blocks, most overlap first
}

// pkgReport is one package's section of the report.
type pkgReport struct {
	pkg      string // short import path
	tests    int
	wall     float64
	serial   float64
	err      string
	verdicts []verdict
}

// redundancyTags resolves the build tags of the redundancy mode: every tier but llm unless named.
func redundancyTags(full bool, tags string) (string, error) {
	resolved, err := resolveTags(full, tags)
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return defaultRedundancyTags, nil
	}
	return resolved, nil
}

// parseProfile returns the covered blocks of a coverage profile: the lines with a non-zero count, keyed by file, position range and statement count.
func parseProfile(r io.Reader) (map[string]struct{}, error) {
	blocks := map[string]struct{}{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		cut := strings.LastIndexByte(line, ' ')
		if cut < 0 {
			return nil, fmt.Errorf("malformed profile line %q", line)
		}
		count, err := strconv.Atoi(line[cut+1:])
		if err != nil {
			return nil, fmt.Errorf("malformed profile line %q: %w", line, err)
		}
		if count != 0 {
			blocks[line[:cut]] = struct{}{}
		}
	}
	return blocks, sc.Err()
}

// classify judges every run.
// A candidate is covered block for block by the other tests.
// The removable set is a greedy pass over the candidates, slowest first, that re-checks each against the tests still kept.
// A test that is skipped, failed, spawns, cannot be classified or covers nothing is never a candidate.
func classify(runs []testRun) []verdict {
	counts := map[string]int{}
	for _, r := range runs {
		if r.eligible() {
			for b := range r.blocks {
				counts[b]++
			}
		}
	}
	covered := func(r testRun) bool {
		for b := range r.blocks {
			if counts[b] < 2 {
				return false
			}
		}
		return true
	}

	out := make([]verdict, len(runs))
	var candidates []int
	for i, r := range runs {
		out[i].name = r.name
		out[i].reason = noCoverageReason(r)
		if out[i].reason == "" && covered(r) {
			out[i].candidate = true
			candidates = append(candidates, i)
		}
	}

	sort.SliceStable(candidates, func(a, b int) bool {
		ra, rb := runs[candidates[a]], runs[candidates[b]]
		if ra.elapsed != rb.elapsed {
			return ra.elapsed > rb.elapsed
		}
		return ra.name < rb.name
	})
	for _, i := range candidates {
		if !covered(runs[i]) {
			continue
		}
		out[i].removable = true
		for b := range runs[i].blocks {
			counts[b]--
		}
	}

	for _, i := range candidates {
		kept := func(j int) bool { return runs[j].eligible() && !out[j].removable }
		if !out[i].removable {
			kept = func(j int) bool { return runs[j].eligible() }
		}
		out[i].covering = coveringTests(i, runs, kept)
	}
	return out
}

func noCoverageReason(r testRun) string {
	switch {
	case r.action == "skip":
		return reasonSkipped
	case r.action != "pass":
		return reasonFailed
	case r.spawn.spawns:
		return reasonSpawns
	case r.spawn.unresolved:
		return reasonUnresolved
	case len(r.blocks) == 0:
		return reasonNoBlocks
	}
	return ""
}

// coveringTests names the tests of the pool that together cover runs[self]'s blocks, taking the largest overlap first and skipping a test that adds nothing.
func coveringTests(self int, runs []testRun, inPool func(int) bool) []string {
	type overlap struct {
		idx int
		n   int
	}
	var overlaps []overlap
	for j, r := range runs {
		if j == self || !inPool(j) {
			continue
		}
		n := 0
		for b := range runs[self].blocks {
			if _, ok := r.blocks[b]; ok {
				n++
			}
		}
		if n > 0 {
			overlaps = append(overlaps, overlap{j, n})
		}
	}
	sort.Slice(overlaps, func(a, b int) bool {
		if overlaps[a].n != overlaps[b].n {
			return overlaps[a].n > overlaps[b].n
		}
		return runs[overlaps[a].idx].name < runs[overlaps[b].idx].name
	})

	remaining := map[string]struct{}{}
	for b := range runs[self].blocks {
		remaining[b] = struct{}{}
	}
	var names []string
	for _, o := range overlaps {
		if len(remaining) == 0 {
			break
		}
		hit := false
		for b := range remaining {
			if _, ok := runs[o.idx].blocks[b]; ok {
				delete(remaining, b)
				hit = true
			}
		}
		if hit {
			names = append(names, runs[o.idx].name)
		}
	}
	return names
}

// renderPackage renders one package's section: header line, candidates table, "no coverage" list.
func renderPackage(r pkgReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", r.pkg)
	if r.err != "" {
		fmt.Fprintf(&b, "Error: %s\n", strings.Join(strings.Fields(r.err), " "))
		return b.String()
	}
	fmt.Fprintf(&b, "%d tests, wall %.2fs, serial %.2fs.\n\n", r.tests, r.wall, r.serial)

	var candidates, uncovered []verdict
	for _, v := range r.verdicts {
		if v.candidate {
			candidates = append(candidates, v)
		}
		if v.reason != "" {
			uncovered = append(uncovered, v)
		}
	}
	if len(candidates) == 0 {
		b.WriteString("No candidates.\n\n")
	} else {
		b.WriteString("| Test | Covering tests | Removable |\n|---|---|---|\n")
		for _, v := range candidates {
			removable := "no"
			if v.removable {
				removable = "yes"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", v.name, codeList(v.covering), removable)
		}
		b.WriteString("\n")
	}
	if len(uncovered) == 0 {
		b.WriteString("No test lacks coverage.\n")
	} else {
		b.WriteString("No coverage:\n\n")
		for _, v := range uncovered {
			fmt.Fprintf(&b, "- `%s`: %s\n", v.name, v.reason)
		}
	}
	return b.String()
}

func codeList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + n + "`"
	}
	return strings.Join(quoted, ", ")
}

// renderReport renders the whole report, one section per package.
func renderReport(tags string, pkgs []pkgReport) string {
	var b strings.Builder
	b.WriteString("# Test redundancy\n\n")
	fmt.Fprintf(&b, "Generated by `go run ./cmd/testtiming -redundancy` under the tags `%s`.\n", tags)
	b.WriteString("Each test ran alone under one coverage binary per package, and a candidate has every covered block also covered by another test of its package.\n")
	b.WriteString("A candidate is evidence, not a verdict: coverage blocks do not see assertions.\n")
	b.WriteString("Only the removable set may be deleted together; the reading rules are in `pattern/PATTERN-test-economy.md`.\n")
	for _, p := range pkgs {
		b.WriteString("\n")
		b.WriteString(renderPackage(p))
	}
	return b.String()
}

// moduleLayout is what the redundancy run needs to know about the module.
type moduleLayout struct {
	path string            // module import path
	dir  string            // module root
	dirs map[string]string // import path -> directory, every package of the module
}

// runRedundancy writes the redundancy report for the packages matching pkgPattern.
// It must run from the module root, the way the other modes do.
func runRedundancy(tags, pkgPattern, outPath string) error {
	layout, err := loadLayout(tags)
	if err != nil {
		return err
	}
	targets, err := listPackages(tags, pkgPattern)
	if err != nil {
		return err
	}
	pkgs, err := runPackageTimings(tags, pkgPattern)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "testtiming-redundancy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	coverpkg := make([]string, 0, len(layout.dirs))
	for p := range layout.dirs {
		coverpkg = append(coverpkg, p)
	}
	sort.Strings(coverpkg)

	var reports []pkgReport
	failed := 0
	for _, importPath := range targets {
		timing := pkgs[importPath]
		if timing == nil || timing.noTests || (timing.tests == 0 && timing.action != "fail") {
			continue
		}
		report := pkgReport{pkg: shortPkg(importPath), tests: timing.tests, wall: timing.elapsed, serial: timing.serial}
		// A package whose own run failed is still measured.
		// Each failing test is listed under "no coverage" as failed.
		fmt.Fprintf(os.Stderr, "redundancy: %s (%d tests)\n", report.pkg, timing.tests)
		report.verdicts, err = measurePackage(layout, tags, importPath, strings.Join(coverpkg, ","), tmp)
		if err != nil {
			report.err = err.Error()
		}
		if report.err != "" {
			failed++
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].pkg < reports[j].pkg })

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(renderReport(tags, reports)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "redundancy: wrote %s (%d packages)\n", outPath, len(reports))
	if failed > 0 {
		return fmt.Errorf("%d package(s) reported an error — see the report", failed)
	}
	return nil
}

// measurePackage builds the package's coverage binary, runs each top-level test alone and classifies the runs.
func measurePackage(layout moduleLayout, tags, importPath, coverpkg, tmp string) ([]verdict, error) {
	dir := layout.dirs[importPath]
	names, err := listTests(tags, importPath)
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "pkg.test")
	args := []string{"test", "-c", "-o", bin, "-cover", "-covermode=set", "-coverpkg=" + coverpkg}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	if _, err := goOutput([]string{"CGO_ENABLED=1"}, args, importPath); err != nil {
		return nil, fmt.Errorf("coverage binary: %w", err)
	}
	defer os.Remove(bin)

	spawns, err := scanSpawns(layout.path, filepath.Join(layout.dir, "internal", "testkit"), dir)
	if err != nil {
		return nil, fmt.Errorf("spawn scan: %w", err)
	}

	runs := make([]testRun, 0, len(names))
	for _, name := range names {
		run, err := runAlone(bin, dir, tmp, name)
		if err != nil {
			return nil, err
		}
		if v, ok := spawns[name]; ok {
			run.spawn = v
		} else {
			run.spawn = spawnVerdict{unresolved: true}
		}
		runs = append(runs, run)
	}
	return classify(runs), nil
}

var testResultLine = regexp.MustCompile(`(?m)^--- (PASS|FAIL|SKIP): (\S+) \(([0-9.]+)s\)$`)

// runAlone runs one top-level test under the coverage binary and reads its elapsed time, outcome and covered blocks.
// A failing run is returned as such, never as an error.
func runAlone(bin, dir, tmp, name string) (testRun, error) {
	prof := filepath.Join(tmp, "cover.out")
	os.Remove(prof)
	cmd := exec.Command(bin, "-test.run", "^"+regexp.QuoteMeta(name)+"$", "-test.coverprofile", prof, "-test.v")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		return testRun{}, fmt.Errorf("run %s: %w", name, err)
	}

	run := testRun{name: name, action: "fail"}
	for _, m := range testResultLine.FindAllStringSubmatch(out.String(), -1) {
		if m[2] == name {
			run.action = strings.ToLower(m[1])
			run.elapsed, _ = strconv.ParseFloat(m[3], 64)
		}
	}
	if run.action == "fail" {
		// A test that reads the process's own arguments fails under the binary's -test.* flags, so it is listed under "no coverage".
		return run, nil
	}
	f, err := os.Open(prof)
	if err != nil {
		if run.action == "skip" {
			return run, nil
		}
		return testRun{}, fmt.Errorf("test %s wrote no coverage profile: %w", name, err)
	}
	defer f.Close()
	if run.blocks, err = parseProfile(f); err != nil {
		return testRun{}, fmt.Errorf("test %s: %w", name, err)
	}
	return run, nil
}

var testNameLine = regexp.MustCompile(`^(Test|Example|Fuzz)\w*$`)

// listTests returns the package's top-level test names under the tags.
func listTests(tags, importPath string) ([]string, error) {
	args := []string{"test", "-list", ".*"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	out, err := goOutput(nil, args, importPath)
	if err != nil {
		return nil, fmt.Errorf("list tests: %w", err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if testNameLine.MatchString(strings.TrimSpace(line)) {
			names = append(names, strings.TrimSpace(line))
		}
	}
	return names, nil
}

// runPackageTimings runs the packages' tests once under `go test -json` and folds the stream with the timing mode's own parser.
// A failing test is recorded on its package and does not fail this call.
func runPackageTimings(tags, pkgPattern string) (map[string]*pkgResult, error) {
	args := []string{"test"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "-json", "-count=1", pkgPattern)
	cmd := exec.Command("go", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		return nil, fmt.Errorf("go test: %w", err)
	}
	pkgs := map[string]*pkgResult{}
	var tests []testResult
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		if len(line) > 0 {
			parseLine(line, pkgs, &tests)
		}
	}
	return pkgs, nil
}

// loadLayout reads the module path, root and every package directory.
func loadLayout(tags string) (moduleLayout, error) {
	out, err := goOutput(nil, []string{"list", "-m", "-f", "{{.Path}} {{.Dir}}"}, "")
	if err != nil {
		return moduleLayout{}, err
	}
	path, dir, ok := strings.Cut(strings.TrimSpace(out), " ")
	if !ok {
		return moduleLayout{}, fmt.Errorf("unexpected go list -m output %q", out)
	}
	layout := moduleLayout{path: path, dir: dir, dirs: map[string]string{}}
	args := []string{"list"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "-f", "{{.ImportPath}} {{.Dir}}")
	out, err = goOutput(nil, args, "./...")
	if err != nil {
		return moduleLayout{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		if p, d, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			layout.dirs[p] = d
		}
	}
	return layout, nil
}

// listPackages returns the import paths matching pattern, sorted.
func listPackages(tags, pattern string) ([]string, error) {
	args := []string{"list"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	out, err := goOutput(nil, args, pattern)
	if err != nil {
		return nil, err
	}
	paths := strings.Fields(out)
	sort.Strings(paths)
	return paths, nil
}

// goOutput runs `go args... [target]` with extra environment and returns its stdout.
// The error carries stderr.
func goOutput(env, args []string, target string) (string, error) {
	if target != "" {
		args = append(args, target)
	}
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
