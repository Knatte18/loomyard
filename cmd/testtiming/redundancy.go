package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testcover"
)

const (
	defaultRedundancyTags = "tier0,tier1,tier2"
	defaultRedundancyOut  = "docs/benchmarks/test-redundancy.md"
	defaultRedundancyPkg  = "./..."
)

// verdict is the redundancy classification of one test, judged by the blocks of its own package's files.
type verdict struct {
	name string
	// reason is non-empty for a test listed under "no coverage".
	reason string
	// candidate is set when other tests of the package cover every block of the package the test covers.
	candidate bool
	// removable is set when the greedy pass drops the test, so it is deletable together with the other removable tests.
	removable bool
	// covering names, for a candidate, the tests that together cover its blocks, most overlap first.
	covering []string
	// moduleContained is set when other tests of the package also cover every block of the module the test covers.
	moduleContained bool
	// keep is the reason of the test's //testtiming:keep directive, non-empty for a test listed under "Kept".
	keep string
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

// redundancyTags resolves the build tags of the redundancy mode from -full or -tags.
// It falls back to defaultRedundancyTags when neither is set.
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

// measuredRun is one test run alone under the coverage binary, with the spawn scan's verdict on its function.
type measuredRun struct {
	result testcover.RunResult
	spawn  testcover.SpawnVerdict
}

// judgePackage classifies the runs of the package at importPath by the blocks of the package's own files.
// It marks as well which of them are contained over every block of the module the runs cover.
// A test named in keeps carries its keep reason, which changes the report only, never the classification.
func judgePackage(importPath string, runs []measuredRun, keeps map[string]string) []verdict {
	packageRuns := make([]testcover.Run, len(runs))
	moduleRuns := make([]testcover.Run, len(runs))
	for i, m := range runs {
		moduleRuns[i] = testcover.Run{
			Name:    m.result.Name,
			Elapsed: m.result.Elapsed,
			Action:  m.result.Action,
			Blocks:  testcover.CoveredPositions(m.result.Profile),
			Spawn:   m.spawn,
		}
		packageRuns[i] = moduleRuns[i]
		packageRuns[i].Blocks = testcover.CoveredPositions(testcover.PackageBlocks(m.result.Profile, importPath))
	}
	moduleVerdicts := testcover.Judge(moduleRuns)
	verdicts := make([]verdict, len(runs))
	for i, v := range testcover.Judge(packageRuns) {
		verdicts[i] = verdict{
			name:            v.Name,
			reason:          v.Reason,
			candidate:       v.Contained,
			removable:       v.Removable,
			covering:        v.Covering,
			moduleContained: moduleVerdicts[i].Contained,
			keep:            keeps[v.Name],
		}
	}
	return verdicts
}

// renderPackage renders one package's section of the report.
// The section holds a header, a table of the candidates without the kept tests, the Kept list and the no coverage list.
func renderPackage(r pkgReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", r.pkg)
	if r.err != "" {
		fmt.Fprintf(&b, "Error: %s\n", strings.Join(strings.Fields(r.err), " "))
		return b.String()
	}
	fmt.Fprintf(&b, "%d tests, wall %.2fs, serial %.2fs.\n\n", r.tests, r.wall, r.serial)

	var candidates, kept, uncovered []verdict
	for _, v := range r.verdicts {
		if v.candidate && v.keep == "" {
			candidates = append(candidates, v)
		}
		if v.keep != "" {
			kept = append(kept, v)
		}
		if v.reason != "" {
			uncovered = append(uncovered, v)
		}
	}
	if len(candidates) == 0 {
		b.WriteString("No candidates.\n\n")
	} else {
		b.WriteString("| Test | Covering tests | Removable | Contained module-wide |\n|---|---|---|---|\n")
		for _, v := range candidates {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", v.name, codeList(v.covering), yesNo(v.removable), yesNo(v.moduleContained))
		}
		b.WriteString("\n")
	}
	if len(kept) > 0 {
		b.WriteString("Kept:\n\n")
		for _, v := range kept {
			fmt.Fprintf(&b, "- `%s`: %s\n", v.name, v.keep)
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

func yesNo(set bool) string {
	if set {
		return "yes"
	}
	return "no"
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
	b.WriteString("Each test ran alone under one coverage binary per package, instrumented over the whole module.\n")
	b.WriteString("A candidate has every covered block of its own package's files also covered by another test of its package.\n")
	b.WriteString("The last column says whether that holds over every block of the module too; a cut contained under both is the safer one.\n")
	b.WriteString("A candidate is evidence, not a verdict: coverage blocks do not see assertions.\n")
	b.WriteString("Only the removable set may be deleted together; the reading rules are in `pattern/PATTERN-test-speed.md`'s test economy section.\n")
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

// runRedundancy writes the redundancy report for the packages matching pkgFlag.
// pkgFlag is a comma-separated list of package patterns.
// A missing report, or a run over every package of the module, writes the whole report.
// Any other run rewrites the header and its own packages' sections of the existing one.
// It must run from the module root.
func runRedundancy(tags, pkgFlag, outPath string) error {
	layout, err := loadLayout(tags)
	if err != nil {
		return err
	}
	patterns := splitPatterns(pkgFlag)
	if len(patterns) == 0 {
		return errors.New("-pkg names no package pattern")
	}
	targets, err := listPackages(tags, patterns)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(outPath)
	splicing := err == nil && !coversModule(layout, targets)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if splicing {
		// Refuse before the minutes of measuring; spliceReport checks again over the text it rewrites.
		if err := requireSameTags(string(existing), tags); err != nil {
			return err
		}
	}
	keeps := map[string]map[string]string{}
	for _, importPath := range targets {
		dir, ok := layout.dirs[importPath]
		if !ok {
			continue
		}
		if keeps[importPath], err = testcover.ScanKeeps(dir); err != nil {
			return fmt.Errorf("keep directive: %w", err)
		}
	}
	return withGateSlot(layout.dir, "testtiming -redundancy", func(slotEnv []string, parallel int) error {
		env, cleanupLym, err := prebuildLym("go", tags, layout.dir, slotEnv, parallel)
		if err != nil {
			return err
		}
		defer cleanupLym()
		pkgs, err := runPackageTimings(tags, patterns, env)
		if err != nil {
			return err
		}

		tmp, err := lymtemp.MkdirTemp("testtiming-redundancy-")
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
		var measured []measuredPackage
		failed := 0
		for _, importPath := range targets {
			timing := pkgs[importPath]
			if timing == nil || timing.noTests || (timing.tests == 0 && timing.action != "fail") {
				measured = append(measured, measuredPackage{pkg: shortPkg(importPath), noTests: true})
				continue
			}
			report := pkgReport{pkg: shortPkg(importPath), tests: timing.tests, wall: timing.elapsed, serial: timing.serial}
			// A package whose own run failed is still measured.
			// Each failing test is listed under "no coverage" as failed.
			fmt.Fprintf(os.Stderr, "redundancy: %s (%d tests)\n", report.pkg, timing.tests)
			report.verdicts, err = measurePackage(layout, tags, importPath, strings.Join(coverpkg, ","), tmp, keeps[importPath], env)
			if err != nil {
				report.err = err.Error()
			}
			if report.err != "" {
				failed++
			}
			reports = append(reports, report)
			measured = append(measured, measuredPackage{pkg: report.pkg, section: renderPackage(report), failed: report.err != ""})
		}
		sort.Slice(reports, func(i, j int) bool { return reports[i].pkg < reports[j].pkg })

		text := renderReport(tags, reports)
		if splicing {
			if text, err = spliceReport(string(existing), tags, measured); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "redundancy: wrote %s (%d packages measured)\n", outPath, len(reports))
		if failed > 0 {
			return fmt.Errorf("%d package(s) reported an error — see the report", failed)
		}
		return nil
	})
}

// measurePackage builds the package's coverage binary, runs each top-level test alone and classifies the runs.
// A test the spawn scan does not find counts as unclassifiable.
// A test named in keeps carries its keep reason on its verdict.
// env is the environment every command runs under.
func measurePackage(layout moduleLayout, tags, importPath, coverpkg, tmp string, keeps map[string]string, env []string) ([]verdict, error) {
	dir := layout.dirs[importPath]
	toolchain := testcover.Toolchain{Go: "go", Env: env}
	names, err := toolchain.ListTests(importPath, tags)
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "pkg.test")
	if err := toolchain.BuildBinary(importPath, coverpkg, tags, bin); err != nil {
		return nil, err
	}
	defer os.Remove(bin)

	spawns, err := testcover.ScanSpawns(layout.path, filepath.Join(layout.dir, "internal", "testkit"), dir)
	if err != nil {
		return nil, fmt.Errorf("spawn scan: %w", err)
	}

	profile := filepath.Join(tmp, "cover.out")
	runs := make([]measuredRun, 0, len(names))
	for _, name := range names {
		result, err := toolchain.RunAlone(bin, dir, profile, name)
		if err != nil {
			return nil, err
		}
		spawn, ok := spawns[name]
		if !ok {
			spawn = testcover.SpawnVerdict{Unresolved: true}
		}
		runs = append(runs, measuredRun{result: result, spawn: spawn})
	}
	return judgePackage(importPath, runs, keeps), nil
}

// coversModule reports whether targets are every package of the module.
func coversModule(layout moduleLayout, targets []string) bool {
	if len(targets) != len(layout.dirs) {
		return false
	}
	for _, importPath := range targets {
		if _, ok := layout.dirs[importPath]; !ok {
			return false
		}
	}
	return true
}

// runPackageTimings runs the packages matching the patterns once under `go test -json`.
// It folds the stream with `parseLine`.
// A failing test is recorded on its package and does not fail this call.
func runPackageTimings(tags string, patterns []string, env []string) (map[string]*pkgResult, error) {
	args := []string{"test"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "-json", "-count=1")
	args = append(args, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Env = env
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

// listPackages returns the import paths matching the patterns, sorted.
func listPackages(tags string, patterns []string) ([]string, error) {
	args := []string{"list"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	out, err := goOutput(nil, append(args, patterns...), "")
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
