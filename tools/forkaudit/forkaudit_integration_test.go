//go:build integration

// forkaudit_integration_test.go runs forkaudit.sh against temporary directories and checks its exit codes, refusals and report.

package forkaudit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// workMarker is the file that marks a directory as a forkaudit work directory.
const workMarker = ".forkaudit-work"

// noForksSentence is the report line for a run that matched no fork.
const noForksSentence = "No Webster implementer forks matched."

// runForkaudit runs forkaudit.sh with the given extra environment and arguments from a fresh working directory.
// Unless the arguments carry --records, it points the script at a records directory that does not exist, so no run reads the live records repository.
// It returns what the script printed and its exit code.
func runForkaudit(t *testing.T, env []string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("forkaudit.sh runs under bash, which is not assumed on Windows")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatalf("forkaudit tests need jq on PATH: %v", err)
	}
	script, err := filepath.Abs("forkaudit.sh")
	if err != nil {
		t.Fatalf("resolve forkaudit.sh: %v", err)
	}
	if !slices.Contains(args, "--records") {
		args = append([]string{"--records", filepath.Join(tempDir(t), "absent-records")}, args...)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		exit = exitErr.ExitCode()
	default:
		t.Fatalf("run forkaudit.sh: %v", err)
	}
	return outBuf.String(), errBuf.String(), exit
}

// tempDir returns a fresh temporary directory with its symlinks resolved, so it equals what the script canonicalizes it to.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return dir
}

// repoRoot returns the canonical path of the repository root, two levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// auditDirs is the set of directories one script run is pointed at, all below one temporary base.
type auditDirs struct {
	repoRoot, base, hub, projects, outDir, out, work string
}

// newAuditDirs creates the hub, projects and --out directories; --work is left absent.
func newAuditDirs(t *testing.T) auditDirs {
	t.Helper()
	base := tempDir(t)
	d := auditDirs{
		repoRoot: repoRoot(t),
		base:     base,
		hub:      filepath.Join(base, "hub"),
		projects: filepath.Join(base, "projects"),
		outDir:   filepath.Join(base, "out"),
		work:     filepath.Join(base, "work"),
	}
	d.out = filepath.Join(d.outDir, "report.md")
	for _, dir := range []string{d.hub, d.projects, d.outDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	return d
}

// args returns the flags that point a run at d, followed by extra.
func (d auditDirs) args(extra ...string) []string {
	return append([]string{"--out", d.out, "--work", d.work, "--projects", d.projects, "--hub", d.hub}, extra...)
}

// snapshot maps every path below root to its content, a directory to the empty string and a symlink to its target.
// A root that does not exist yields an empty map.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	info, err := os.Lstat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return files
	case err != nil:
		t.Fatalf("lstat %s: %v", root, err)
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(root)
		if err != nil {
			t.Fatalf("readlink %s: %v", root, err)
		}
		files["."] = "-> " + target
		return files
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[rel+"/"] = ""
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return files
}

// makeReadOnly sets dir to mode 0555 and registers the cleanup that restores 0755 before the temporary tree is removed.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
}

// writeFile creates a file with its parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// markedDir creates dir holding only the work marker.
func markedDir(t *testing.T, dir string) string {
	t.Helper()
	writeFile(t, filepath.Join(dir, workMarker), "")
	return dir
}

// commandPath builds a directory of symlinks to the commands the script requires, leaving out omit.
// With dateStub, date is a script that always fails instead of a link.
func commandPath(t *testing.T, omit string, dateStub bool) string {
	t.Helper()
	dir := tempDir(t)
	for _, name := range []string{"bash", "git", "find", "awk", "mktemp", "realpath", "date", "jq"} {
		switch {
		case name == omit:
		case name == "date" && dateStub:
			writeFile(t, filepath.Join(dir, name), "#!/bin/sh\nexit 1\n")
			if err := os.Chmod(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatalf("chmod date stub: %v", err)
			}
		default:
			source, err := exec.LookPath(name)
			if err != nil {
				t.Fatalf("find %s: %v", name, err)
			}
			if err := os.Symlink(source, filepath.Join(dir, name)); err != nil {
				t.Fatalf("link %s: %v", name, err)
			}
		}
	}
	return dir
}

// forkFixture is one transcript fixture to place under a run's project directory.
type forkFixture struct {
	// fixture names the file under testdata, without its extension.
	fixture string
	run     string
	session string
	// agent is the agent id; the transcript is named agent-<agent>.jsonl.
	agent string
	// modTime is the transcript's modification time; zero means two minutes after the fixture's first line.
	modTime time.Time
	// truncate appends a half-written JSON line.
	truncate bool
}

var (
	firstTimestamp  = regexp.MustCompile(`"timestamp":"([^"]+)"`)
	nonAlphanumeric = regexp.MustCompile(`[^A-Za-z0-9]`)
)

// placeForks copies each fixture under its run's project directory in projects.
// The worktree placeholder becomes <hub>/<run> and the modification time is set.
func placeForks(t *testing.T, projects, hub string, forks []forkFixture) {
	t.Helper()
	encodedHub := nonAlphanumeric.ReplaceAllString(hub, "-")
	for _, fork := range forks {
		raw, err := os.ReadFile(filepath.Join("testdata", fork.fixture+".jsonl"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		text := strings.ReplaceAll(string(raw), "@WORKTREE@", filepath.Join(hub, fork.run))
		if fork.truncate {
			text += `{"type":"assistant","message":{"id":"m`
		}
		path := filepath.Join(projects, encodedHub+"-"+fork.run, fork.session, "subagents", "agent-"+fork.agent+".jsonl")
		writeFile(t, path, text)
		modTime := fork.modTime
		if modTime.IsZero() {
			start, err := time.Parse(time.RFC3339, firstTimestamp.FindStringSubmatch(text)[1])
			if err != nil {
				t.Fatalf("parse the first timestamp of %s: %v", fork.fixture, err)
			}
			modTime = start.Add(2 * time.Minute)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set the modification time of %s: %v", path, err)
		}
	}
}

// mainForks is the placement most report rows share: run alpha holds three forks and three non-forks, run alpha-two four forks.
func mainForks() []forkFixture {
	return []forkFixture{
		{fixture: "fork-file", run: "alpha", session: "s-alpha", agent: "ffile"},
		{fixture: "fork-file-noedit", run: "alpha", session: "s-alpha", agent: "fnoedit"},
		{fixture: "fork-raf", run: "alpha", session: "s-alpha", agent: "fraf"},
		{fixture: "not-webster", run: "alpha", session: "s-alpha", agent: "notweb"},
		{fixture: "no-prompt-read", run: "alpha", session: "s-alpha", agent: "noprompt"},
		{fixture: "other-prompt", run: "alpha", session: "s-alpha", agent: "otherprompt"},
		{fixture: "fork-inline", run: "alpha-two", session: "s-two", agent: "finline"},
		{fixture: "fork-none", run: "alpha-two", session: "s-two", agent: "fnone"},
		{fixture: "fork-unknown", run: "alpha-two", session: "s-two", agent: "funknown"},
		{fixture: "fork-raf", run: "alpha-two", session: "s-two", agent: "rafcut", truncate: true, modTime: time.Date(2026, 10, 14, 9, 20, 0, 0, time.UTC)},
	}
}

// tableCells returns the lines of the markdown table under heading as rows of cells, header row first.
func tableCells(t *testing.T, report, heading string) [][]string {
	t.Helper()
	_, after, found := strings.Cut(report, "\n"+heading+"\n")
	if !found {
		t.Fatalf("report has no %q section:\n%s", heading, report)
	}
	var rows [][]string
	for _, line := range strings.Split(after, "\n") {
		switch {
		case strings.HasPrefix(line, "| ---"):
		case strings.HasPrefix(line, "|"):
			rows = append(rows, strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "| "), " |"), " | "))
		case len(rows) > 0:
			return rows
		}
	}
	return rows
}

// tableRows returns the table under heading as one map per data row, keyed by header.
func tableRows(t *testing.T, report, heading string) []map[string]string {
	t.Helper()
	cells := tableCells(t, report, heading)
	var rows []map[string]string
	for _, line := range cells[1:] {
		row := map[string]string{}
		for i, header := range cells[0] {
			row[header] = line[i]
		}
		rows = append(rows, row)
	}
	return rows
}

// rowWhere returns the table row whose key column equals value.
func rowWhere(t *testing.T, rows []map[string]string, key, value string) map[string]string {
	t.Helper()
	for _, row := range rows {
		if row[key] == value {
			return row
		}
	}
	t.Fatalf("no row with %s = %q in %v", key, value, rows)
	return nil
}

// cell returns the cell of a table whose rows are metrics and whose columns are arms.
func cell(t *testing.T, report, heading, metric, arm string) string {
	t.Helper()
	return rowWhere(t, tableRows(t, report, heading), "metric", metric)[arm]
}

// withoutGenerated drops the generation-time line, the only line two runs over the same input differ in.
func withoutGenerated(report string) string {
	var kept []string
	for _, line := range strings.Split(report, "\n") {
		if !strings.HasPrefix(line, "Generated: ") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// refusalRow is one invocation the script must refuse without touching --out's directory or --work.
type refusalRow struct {
	name string
	// prepare builds the row's directories and returns its arguments and extra environment.
	prepare func(t *testing.T, d *auditDirs) (args []string, env []string)
	// denial marks a row that needs permission denial, which root does not get.
	denial bool
	// failsAfterWork marks a row whose failure comes after --work is adopted, so --work legitimately changes.
	failsAfterWork bool
	wantExit       int
	wantStderr     []string
}

func TestForkauditRefusals(t *testing.T) {
	t.Parallel()
	since := []string{"--since", "2026-10-14"}
	rows := []refusalRow{
		{name: "neither selector", wantExit: 2, wantStderr: []string{"--since", "--runs"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args(), nil }},
		{name: "both selectors", wantExit: 2, wantStderr: []string{"--since", "--runs"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "2026-10-14", "--runs", "a"), nil
			}},
		{name: "until alone", wantExit: 2, wantStderr: []string{"--until"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--runs", "a", "--until", "2026-10-14"), nil
			}},
		{name: "unparsable since", wantExit: 2, wantStderr: []string{"--since"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "not a date"), nil
			}},
		{name: "unparsable until", wantExit: 2, wantStderr: []string{"--until"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "2026-10-14", "--until", "not a date"), nil
			}},
		{name: "until before since", wantExit: 2, wantStderr: []string{"--until", "--since"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "2026-10-14", "--until", "2026-10-13"), nil
			}},
		{name: "empty run list", wantExit: 2, wantStderr: []string{"--runs"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args("--runs", ""), nil }},
		{name: "empty run name", wantExit: 2, wantStderr: []string{"--runs"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args("--runs", "a,,b"), nil }},
		{name: "uppercase run name", wantExit: 2, wantStderr: []string{"--runs", "Alpha"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args("--runs", "Alpha"), nil }},
		{name: "run name with a space", wantExit: 2, wantStderr: []string{"--runs"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args("--runs", "a b"), nil }},
		{name: "unknown flag", wantExit: 2, wantStderr: []string{"--bogus"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) { return d.args("--bogus"), nil }},
		{name: "out without a value", wantExit: 2, wantStderr: []string{"--out"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return []string{"--since", "2026-10-14", "--work", d.work, "--out"}, nil
			}},
		{name: "jq missing", wantExit: 1, wantStderr: []string{"missing command", "jq"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args(since...), []string{"PATH=" + commandPath(t, "jq", false)}
			}},
		{name: "date is not GNU date", wantExit: 1, wantStderr: []string{"missing command", "GNU date"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args(since...), []string{"PATH=" + commandPath(t, "", true)}
			}},
		{name: "out directory read-only", denial: true, wantExit: 1, wantStderr: []string{"--out"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				makeReadOnly(t, d.outDir)
				return d.args(since...), nil
			}},
		{name: "out directory missing", wantExit: 1, wantStderr: []string{"--out"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				d.out = filepath.Join(d.base, "absent", "report.md")
				return d.args(since...), nil
			}},
		{name: "out names a directory", wantExit: 1, wantStderr: []string{"--out"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				d.out = d.outDir
				return d.args(since...), nil
			}},
		{name: "work is a symlink to the repo root", wantExit: 1, wantStderr: []string{"--work", "overlaps the repo root"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				if err := os.Symlink(d.repoRoot, d.work); err != nil {
					t.Fatalf("link work: %v", err)
				}
				return d.args(since...), nil
			}},
		{name: "work is a symlink to a marked hub", wantExit: 1, wantStderr: []string{"--work", "overlaps the hub"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				markedDir(t, d.hub)
				if err := os.Symlink(d.hub, d.work); err != nil {
					t.Fatalf("link work: %v", err)
				}
				return d.args(since...), nil
			}},
		{name: "work is an ancestor of out spelled with dotdot", wantExit: 1, wantStderr: []string{"--work", "overlaps --out's directory"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				ancestor := markedDir(t, filepath.Join(d.base, "ancestor"))
				d.outDir = filepath.Join(ancestor, "inner")
				d.out = filepath.Join(d.outDir, "report.md")
				writeFile(t, filepath.Join(d.outDir, "keep.txt"), "kept")
				d.work = filepath.Join(ancestor, "inner", "..")
				return d.args(since...), nil
			}},
		{name: "work is the out directory", wantExit: 1, wantStderr: []string{"--work", "overlaps --out's directory"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				markedDir(t, d.outDir)
				d.work = d.outDir
				return d.args(since...), nil
			}},
		{name: "work holds out in a subdirectory", wantExit: 1, wantStderr: []string{"--work", "overlaps --out's directory"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				markedDir(t, d.work)
				d.outDir = filepath.Join(d.work, "sub")
				d.out = filepath.Join(d.outDir, "report.md")
				writeFile(t, d.out, "previous report")
				return d.args(since...), nil
			}},
		{name: "work non-empty without the marker", wantExit: 1, wantStderr: []string{"--work", "not a forkaudit work directory"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				writeFile(t, filepath.Join(d.work, "foreign.txt"), "foreign")
				return d.args(since...), nil
			}},
		{name: "marked work read-only", denial: true, wantExit: 1, wantStderr: []string{"--work", "not writable"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				markedDir(t, d.work)
				makeReadOnly(t, d.work)
				return d.args(since...), nil
			}},
		{name: "unreadable candidate transcript", denial: true, failsAfterWork: true, wantExit: 1, wantStderr: []string{"agent-ffile"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				placeForks(t, d.projects, d.hub, mainForks()[:1])
				encodedHub := nonAlphanumeric.ReplaceAllString(d.hub, "-")
				transcript := filepath.Join(d.projects, encodedHub+"-alpha", "s-alpha", "subagents", "agent-ffile.jsonl")
				if err := os.Chmod(transcript, 0); err != nil {
					t.Fatalf("chmod transcript: %v", err)
				}
				return d.args("--since", "2026-10-13"), nil
			}},
		{name: "pack check with a missing code directory", wantExit: 1, wantStderr: []string{"--code"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "2026-10-14", "--pack-check", "--code", filepath.Join(d.base, "absent-code"), "--records", tempDir(t)), nil
			}},
		{name: "pack check with a missing records directory", wantExit: 1, wantStderr: []string{"--records"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				return d.args("--since", "2026-10-14", "--pack-check", "--records", filepath.Join(d.base, "absent-records")), nil
			}},
		{name: "work under a read-only parent", denial: true, wantExit: 1, wantStderr: []string{"--work", "not writable"},
			prepare: func(t *testing.T, d *auditDirs) ([]string, []string) {
				parent := filepath.Join(d.base, "parent")
				if err := os.Mkdir(parent, 0o755); err != nil {
					t.Fatalf("create parent: %v", err)
				}
				makeReadOnly(t, parent)
				d.work = filepath.Join(parent, "work")
				return d.args(since...), nil
			}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if row.denial && os.Geteuid() == 0 {
				t.Skip("root is not denied by file permissions")
			}
			d := newAuditDirs(t)
			writeFile(t, d.out, "previous report")
			args, env := row.prepare(t, &d)
			outBefore, workBefore := snapshot(t, filepath.Dir(d.out)), snapshot(t, d.work)
			stdout, stderr, exit := runForkaudit(t, env, args...)
			if exit != row.wantExit {
				t.Fatalf("exit = %d, want %d; stderr: %s", exit, row.wantExit, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			for _, want := range row.wantStderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q: %s", want, stderr)
				}
			}
			if outAfter := snapshot(t, filepath.Dir(d.out)); !maps.Equal(outBefore, outAfter) {
				t.Errorf("--out's directory changed: before %v, after %v", outBefore, outAfter)
			}
			if workAfter := snapshot(t, d.work); !row.failsAfterWork && !maps.Equal(workBefore, workAfter) {
				t.Errorf("--work changed: before %v, after %v", workBefore, workAfter)
			}
			if _, err := os.Stat(filepath.Join(d.repoRoot, workMarker)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a work marker appeared in the repo root: %v", err)
			}
		})
	}
}

// reportRow is one successful invocation and what its report must hold.
type reportRow struct {
	name string
	// prepare builds the row's directories and returns the flags that select its forks.
	prepare func(t *testing.T, d *auditDirs) []string
	// check asserts on the report text; args are the flags prepare returned.
	check func(t *testing.T, d auditDirs, args []string, report string)
}

func TestForkauditReport(t *testing.T) {
	t.Parallel()
	rows := []reportRow{
		{name: "runs name matching no project directory",
			prepare: func(t *testing.T, d *auditDirs) []string { return []string{"--runs", "ghost"} },
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				if !strings.Contains(report, noForksSentence) {
					t.Errorf("report lacks the no-forks sentence:\n%s", report)
				}
				if !strings.Contains(report, "Runs with no project directory: ghost") {
					t.Errorf("report does not name ghost as unmatched:\n%s", report)
				}
				if _, err := os.Stat(filepath.Join(d.work, workMarker)); err != nil {
					t.Errorf("--work was not marked: %v", err)
				}
			}},
		{name: "since over a marked work directory holding a stale intermediate",
			prepare: func(t *testing.T, d *auditDirs) []string {
				markedDir(t, d.work)
				writeFile(t, filepath.Join(d.work, "candidates.txt"), "stale candidate")
				writeFile(t, filepath.Join(d.work, "keep.txt"), "kept bytes")
				return []string{"--since", "2026-10-14"}
			},
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				if !strings.Contains(report, noForksSentence) {
					t.Errorf("report lacks the no-forks sentence:\n%s", report)
				}
				if strings.Contains(report, "Runs with no project directory") {
					t.Errorf("report names an unmatched run under --since:\n%s", report)
				}
				if stale, err := os.ReadFile(filepath.Join(d.work, "candidates.txt")); err == nil && strings.Contains(string(stale), "stale candidate") {
					t.Errorf("the stale intermediate survived: %s", stale)
				}
				if kept, err := os.ReadFile(filepath.Join(d.work, "keep.txt")); err != nil || string(kept) != "kept bytes" {
					t.Errorf("keep.txt = %q, %v; want its bytes unchanged", kept, err)
				}
			}},
		{name: "since over the fixtures counts the forks of every arm",
			prepare: func(t *testing.T, d *auditDirs) []string {
				placeForks(t, d.projects, d.hub, mainForks())
				return []string{"--since", "2026-10-13"}
			},
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				for _, want := range []string{
					"- Candidates found: 10",
					"- Transcripts holding the job heading: 9",
					"- Forks kept: 7",
					"- Transcript lines dropped as unparsable: 1",
				} {
					if !strings.Contains(report, want+"\n") {
						t.Errorf("report header lacks %q:\n%s", want, report)
					}
				}
				samples := tableRows(t, report, "## Samples")
				wantSamples := map[string][3]string{
					"file":    {"2", "1", "3"},
					"raf":     {"2", "2", "2"},
					"inline":  {"1", "1", "1"},
					"none":    {"1", "1", "1"},
					"unknown": {"1", "1", "0"},
				}
				if len(samples) != len(wantSamples) {
					t.Errorf("samples table has %d rows, want %d", len(samples), len(wantSamples))
				}
				for arm, want := range wantSamples {
					row := rowWhere(t, samples, "arm", arm)
					if got := [3]string{row["forks"], row["runs"], row["cards"]}; got != want {
						t.Errorf("arm %s: forks, runs, cards = %v, want %v", arm, got, want)
					}
				}
				if strings.Contains(report, "Arms with no forks") {
					t.Errorf("report names an arm with no forks although every arm has one:\n%s", report)
				}
				perFork := tableRows(t, report, "## Per-fork")
				wantFile := map[string]string{
					"arm": "file", "run": "alpha", "cards": "2", "pack opened": "yes", "pack turn": "2", "pack before edit": "yes",
					"first work turn": "2", "first parallel": "1", "spans": "6", "strict before edit": "3", "file-level before edit": "5",
					"strict in first turn": "2", "calls before edit": "5", "net before edit": "4", "net exploration": "7",
					"net whole reads": "3", "partial reads": "2", "greps": "1", "quarry": "1", "turns": "9", "output k": "9",
					"cache read M": "0.9", "edits": "2", "edits to no-span files": "1", "dropped lines": "0",
				}
				fork := rowWhere(t, perFork, "agent", "agent-ffile")
				for column, want := range wantFile {
					if fork[column] != want {
						t.Errorf("agent-ffile %s = %q, want %q", column, fork[column], want)
					}
				}
				cut := rowWhere(t, perFork, "agent", "agent-rafcut")
				if cut["arm"] != "raf" || cut["dropped lines"] != "1" {
					t.Errorf("the truncated fork row = arm %q, dropped lines %q; want raf, 1", cut["arm"], cut["dropped lines"])
				}
				wantCells := []struct{ heading, metric, arm, want string }{
					{"## Pack use", "pack read before the first edit", "file", "1 of 1 (n=1)"},
					{"## Pack use", "spans reached before the first edit, strict share, median", "file", "50% (n=1)"},
					{"## Pack use", "spans reached before the first edit, strict share, pooled", "file", "50% (n=1)"},
					{"## Pack use", "spans reached in the first work turn, strict share, pooled", "file", "43%"},
					{"## Pack use", "forks reaching half their spans (strict) before the first edit", "file", "1 of 1 (n=1)"},
					{"## Pack use", "spans reached before the first edit, strict share, median", "inline", "n/a"},
					{"## Pack use", "spans reached in the first work turn, file-level share, pooled", "inline", "n/a"},
					{"## Arm comparison", "forks with 3+ parallel calls in the first work turn, share", "file", "50%"},
					{"## Arm comparison", "exploration calls, raw, median per card", "file", "4.5"},
					{"## Arm comparison", "exploration calls, raw, pooled per card", "file", "4.33"},
					{"## Arm comparison", "exploration calls, net, median per card", "file", "3.75"},
					{"## Arm comparison", "turns, median per card", "file", "3.75"},
					{"## Arm comparison", "output tokens, median per card", "file", "3750"},
					{"## Arm comparison", "calls before the first edit, net, median per card", "file", "2 (n=1)"},
				}
				for _, want := range wantCells {
					if got := cell(t, report, want.heading, want.metric, want.arm); got != want.want {
						t.Errorf("%s: %s, arm %s = %q, want %q", want.heading, want.metric, want.arm, got, want.want)
					}
				}
				for _, row := range tableRows(t, report, "## Pack use") {
					if row["none"] != "n/a" {
						t.Errorf("pack use %q, arm none = %q, want n/a", row["metric"], row["none"])
					}
				}
				for _, row := range tableRows(t, report, "## Arm comparison") {
					if strings.Contains(row["metric"], "per card") && row["unknown"] != "n/a" {
						t.Errorf("arm comparison %q, arm unknown = %q, want n/a", row["metric"], row["unknown"])
					}
				}
				quartiles := rowWhere(t, tableRows(t, report, "## Exploration quartiles"), "arm", "file")
				if got := []string{quartiles["n"], quartiles["min"], quartiles["q1"], quartiles["median"], quartiles["q3"], quartiles["max"]}; !slices.Equal(got, []string{"2", "3.5", "3.5", "4", "4", "4"}) {
					t.Errorf("file arm quartile row = %v, want n=2 and 3.5 3.5 4 4 4", got)
				}
			}},
		{name: "a second run replaces the first report with an identical one",
			prepare: func(t *testing.T, d *auditDirs) []string {
				placeForks(t, d.projects, d.hub, mainForks())
				return []string{"--since", "2026-10-13", "--records", filepath.Join(tempDir(t), "absent-records")}
			},
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				writeFile(t, d.out, "replace me")
				if _, stderr, exit := runForkaudit(t, nil, d.args(args...)...); exit != 0 {
					t.Fatalf("second run: exit = %d; stderr: %s", exit, stderr)
				}
				second, err := os.ReadFile(d.out)
				if err != nil {
					t.Fatalf("read the second report: %v", err)
				}
				if withoutGenerated(report) != withoutGenerated(string(second)) {
					t.Errorf("the two reports differ beyond the Generated line:\n%s\n---\n%s", report, second)
				}
			}},
		{name: "runs keeps only the named run's forks",
			prepare: func(t *testing.T, d *auditDirs) []string {
				placeForks(t, d.projects, d.hub, mainForks())
				return []string{"--runs", "alpha"}
			},
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				if !strings.Contains(report, "- Forks kept: 3\n") {
					t.Errorf("want 3 forks kept:\n%s", report)
				}
				if !strings.Contains(report, "Arms with no forks: inline, none, unknown.") {
					t.Errorf("report does not name the arms with no forks:\n%s", report)
				}
				for _, agent := range []string{"agent-finline", "agent-fnone", "agent-funknown", "agent-rafcut"} {
					if strings.Contains(report, agent) {
						t.Errorf("report holds %s from the other run", agent)
					}
				}
				for _, heading := range []string{"## Pack use", "## Arm comparison"} {
					if header := tableCells(t, report, heading)[0]; !slices.Equal(header, []string{"metric", "file", "raf"}) {
						t.Errorf("%s columns = %v, want metric, file, raf", heading, header)
					}
				}
				for _, heading := range []string{"## Samples", "## Exploration quartiles"} {
					for _, row := range tableRows(t, report, heading) {
						if arm := row["arm"]; arm != "file" && arm != "raf" {
							t.Errorf("%s has a row for arm %s with no forks", heading, arm)
						}
					}
				}
			}},
		{name: "until bounds the modification window",
			prepare: func(t *testing.T, d *auditDirs) []string {
				placeForks(t, d.projects, d.hub, mainForks())
				return []string{"--since", "2026-10-13", "--until", "2026-10-14T09:04:40Z"}
			},
			check: func(t *testing.T, d auditDirs, args []string, report string) {
				if !strings.Contains(report, "- Forks kept: 3\n") {
					t.Errorf("want 3 forks kept:\n%s", report)
				}
				if strings.Contains(report, "agent-finline") {
					t.Errorf("report holds a fork modified after the bound")
				}
			}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			d := newAuditDirs(t)
			args := row.prepare(t, &d)
			stdout, stderr, exit := runForkaudit(t, nil, d.args(args...)...)
			if exit != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", exit, stderr)
			}
			if stdout != d.out+"\n" {
				t.Errorf("stdout = %q, want the --out path", stdout)
			}
			report, err := os.ReadFile(d.out)
			if err != nil {
				t.Fatalf("read report: %v", err)
			}
			row.check(t, d, args, string(report))
		})
	}
}

var recordsSuffixLine = regexp.MustCompile(`(?m)^RECORDS_SUFFIX="([^"]*)"`)

// recordsSuffix returns the suffix of a run's records branch, read from the script's variable block so the test spells no second copy.
func recordsSuffix(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile("forkaudit.sh")
	if err != nil {
		t.Fatalf("read forkaudit.sh: %v", err)
	}
	match := recordsSuffixLine.FindSubmatch(script)
	if match == nil {
		t.Fatal("forkaudit.sh has no RECORDS_SUFFIX line")
	}
	return string(match[1])
}

// batchFixture is one batch of a run's webster state.
type batchFixture struct {
	number    int
	cards     []string
	startSHA  string
	spawnedAt string
}

// recordsRun is one run's state history in the records repository.
type recordsRun struct {
	name string
	// kind is where the state is committed: "local" branch, "origin" remote-tracking branch or "archive" tag.
	kind    string
	batches []batchFixture
	// extraKey, when set, is a UTC second stored under a state key that is no batch.
	extraKey string
	// planCards maps a path under the plan directory to the card's content.
	planCards map[string]string
}

// stateText renders run's webster state file.
func stateText(t *testing.T, run recordsRun) string {
	t.Helper()
	batches := map[string]any{}
	for _, batch := range run.batches {
		batches[strconv.Itoa(batch.number)] = map[string]any{"cards": batch.cards, "startSha": batch.startSHA, "spawnedAt": batch.spawnedAt}
	}
	state := map[string]any{"batches": batches}
	if run.extraKey != "" {
		state["note"] = run.extraKey
	}
	text, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	return string(text)
}

// recordsRepo builds a records repository holding each run's state under the run's records ref.
func recordsRepo(t *testing.T, runs []recordsRun) string {
	t.Helper()
	dir := tempDir(t)
	statePath := filepath.Join(lyxdirs.LyxDirName, "webster", "state.json")
	gitkit.Git(t, dir, "init", "-b", "main")
	gitkit.Git(t, dir, "commit", "--allow-empty", "-m", "initial")
	for _, run := range runs {
		branch := run.name + recordsSuffix(t)
		gitkit.Git(t, dir, "checkout", "-b", branch, "main")
		gitkit.CommitFileOnBranch(t, dir, branch, statePath, stateText(t, run), "state of "+run.name)
		gitkit.Git(t, dir, "checkout", "main")
		switch run.kind {
		case "local":
		case "origin":
			gitkit.Git(t, dir, "update-ref", "refs/remotes/origin/"+branch, branch)
			gitkit.Git(t, dir, "branch", "-D", branch)
		case "archive":
			gitkit.Git(t, dir, "tag", "archive/"+run.name+"/final", branch)
			gitkit.Git(t, dir, "branch", "-D", branch)
		default:
			t.Fatalf("unknown records ref kind %q", run.kind)
		}
	}
	return dir
}

// forkStart returns the first-line timestamp of a fork fixture.
func forkStart(t *testing.T, fixture string) time.Time {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", fixture+".jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	start, err := time.Parse(time.RFC3339, firstTimestamp.FindStringSubmatch(string(raw))[1])
	if err != nil {
		t.Fatalf("parse the first timestamp of %s: %v", fixture, err)
	}
	return start
}

// traceSpec is one begin-batch trace line of a fork's worktree.
type traceSpec struct {
	batch  int
	layout string
	// before is how long before the fork's start the line was logged; negative means after.
	before time.Duration
}

// traceLine renders a `webster: kickstart pack` trace line logged spec.before ahead of start, with a local UTC offset.
func traceLine(start time.Time, spec traceSpec) string {
	logged := start.Add(-spec.before).In(time.FixedZone("local", 2*3600))
	return fmt.Sprintf(`time=%s level=INFO msg="webster: kickstart pack" trace=abc batch=%d kickstart_pack=on resolved=1 unresolved=0 pack_layout=%s`,
		logged.Format("2006-01-02T15:04:05.000-07:00"), spec.batch, spec.layout)
}

// writeTrace writes a trace file with a header record and lines into the logs directory of the worktree <hub>/<run>.
func writeTrace(t *testing.T, hub, run string, lines []string) {
	t.Helper()
	content := "command=lyx argv=webster trace=abc pid=1 worktree_root=" + filepath.Join(hub, run) + "\n" + strings.Join(lines, "\n") + "\n"
	writeFile(t, filepath.Join(hub, run, lyxdirs.DotLyxDirName, "logs", "trace-20261014T090000Z-abc-1.log"), content)
}

// traceCase is one fork with its run's records and trace file, and the cells its report row must hold.
type traceCase struct {
	fixture string
	// run names the fork's run, its records branch and its agent: agent-<run>.
	run string
	// kind is the records ref kind of the run's state; empty means the run has no records refs.
	kind  string
	batch int
	// spawnLead is how long before the fork's first line the batch spawned; zero means 3 seconds.
	spawnLead time.Duration
	// extraOnly keeps the spawn second under a non-batch state key and holds no batch.
	extraOnly bool
	// noTraceFile leaves the worktree without a trace file.
	noTraceFile bool
	trace       []traceSpec
	// want maps per-fork table columns to the expected cells.
	want map[string]string
}

// traceRow is one report over a set of trace cases.
type traceRow struct {
	name           string
	cases          []traceCase
	missingRecords bool
	// wantReport lists substrings of the report.
	wantReport []string
	// wantCrossChecked maps an arm to its cross-checked count in the samples table.
	wantCrossChecked map[string]string
}

func TestForkauditTraceCrossCheck(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	oneLine := func(layout string) []traceSpec {
		return []traceSpec{{batch: 1, layout: layout, before: 1500 * time.Millisecond}}
	}
	rows := []traceRow{
		{name: "agreeing layouts", cases: []traceCase{
			{fixture: "fork-raf", run: "ra", kind: "local", batch: 1, trace: oneLine("inline"),
				want: map[string]string{"arm": "raf", "batch": "1", "trace": "inline", "cross-check": "agrees"}},
			{fixture: "fork-file", run: "rb", kind: "local", batch: 1, trace: oneLine("file"),
				want: map[string]string{"arm": "file", "batch": "1", "trace": "file", "cross-check": "agrees"}},
			{fixture: "fork-none", run: "rc", kind: "local", batch: 1, trace: oneLine("none"),
				want: map[string]string{"arm": "none", "batch": "1", "trace": "none", "cross-check": "agrees"}},
		}, wantCrossChecked: map[string]string{"raf": "1", "file": "1", "none": "1"}},
		{name: "disagreeing layouts keep the prompt's arm", cases: []traceCase{
			{fixture: "fork-file", run: "rd", kind: "local", batch: 1, trace: oneLine("inline"),
				want: map[string]string{"arm": "file", "trace": "inline", "cross-check": "disagrees"}},
			{fixture: "fork-file-noedit", run: "re", kind: "local", batch: 1, trace: oneLine("perhand"),
				want: map[string]string{"arm": "file", "trace": "perhand", "cross-check": "disagrees"}},
		}, wantReport: []string{"- agent-rd (rd): arm file, trace inline", "- agent-re (re): arm file, trace perhand"}},
		{name: "unknown forks take a new layout as their arm or stay unknown", cases: []traceCase{
			{fixture: "fork-unknown", run: "rf", kind: "local", batch: 1, trace: oneLine("perhand"),
				want: map[string]string{"arm": "trace:perhand", "trace": "perhand", "cross-check": "trace arm"}},
			{fixture: "fork-unknown", run: "rg", kind: "local", batch: 1, trace: oneLine("raf"),
				want: map[string]string{"arm": "unknown", "trace": "raf", "cross-check": "unrecognized"}},
			{fixture: "fork-unknown", run: "rh", kind: "local", batch: 1, trace: oneLine("unknown"),
				want: map[string]string{"arm": "unknown", "trace": "unknown", "cross-check": "unrecognized"}},
			{fixture: "fork-unknown", run: "ri", kind: "local", batch: 1, trace: oneLine("a|b"),
				want: map[string]string{"arm": "unknown", "trace": "a|b", "cross-check": "unrecognized"}},
		}, wantReport: []string{"Unrecognized trace values: a|b, raf, unknown."}, wantCrossChecked: map[string]string{"trace:perhand": "1", "unknown": "3"}},
		{name: "the last trace line before the fork's start counts", cases: []traceCase{
			{fixture: "fork-file", run: "rj", kind: "local", batch: 1,
				trace: []traceSpec{{batch: 1, layout: "file", before: 2 * time.Second}, {batch: 1, layout: "inline", before: -2 * time.Second}},
				want:  map[string]string{"trace": "file", "cross-check": "agrees"}},
			{fixture: "fork-file-noedit", run: "rk", kind: "local", batch: 1,
				trace: []traceSpec{{batch: 1, layout: "file", before: -2 * time.Second}},
				want:  map[string]string{"batch": "1", "trace": "-", "cross-check": "-"}},
			{fixture: "fork-raf", run: "rl", kind: "local", batch: 1,
				trace: []traceSpec{{batch: 2, layout: "none", before: time.Second}},
				want:  map[string]string{"batch": "1", "trace": "-", "cross-check": "-"}},
		}},
		{name: "runs sharing a spawn second resolve to their own batches", cases: []traceCase{
			{fixture: "fork-raf", run: "alpha", kind: "local", batch: 1, trace: oneLine("inline"),
				want: map[string]string{"batch": "1", "trace": "inline", "cross-check": "agrees"}},
			{fixture: "fork-raf", run: "beta", kind: "local", batch: 2, trace: []traceSpec{{batch: 2, layout: "none", before: 1500 * time.Millisecond}},
				want: map[string]string{"batch": "2", "trace": "none", "cross-check": "disagrees"}},
		}},
		{name: "batches under an origin branch and an archive tag resolve", cases: []traceCase{
			{fixture: "fork-file", run: "rm", kind: "origin", batch: 4, trace: []traceSpec{{batch: 4, layout: "file", before: time.Second}},
				want: map[string]string{"batch": "4", "cross-check": "agrees"}},
			{fixture: "fork-raf", run: "rn", kind: "archive", batch: 5, trace: []traceSpec{{batch: 5, layout: "inline", before: time.Second}},
				want: map[string]string{"batch": "5", "cross-check": "agrees"}},
		}},
		{name: "an unmatched spawn second gives no cross-check", cases: []traceCase{
			{fixture: "fork-file", run: "ro", kind: "local", batch: 1, spawnLead: 20 * time.Second, trace: oneLine("file"),
				want: map[string]string{"batch": "-", "trace": "-", "cross-check": "-"}},
			{fixture: "fork-raf", run: "rp", kind: "local", extraOnly: true, trace: oneLine("inline"),
				want: map[string]string{"batch": "-", "trace": "-", "cross-check": "-"}},
			{fixture: "fork-none", run: "rq", trace: oneLine("none"),
				want: map[string]string{"batch": "-", "trace": "-", "cross-check": "-"}},
		}},
		{name: "a worktree without a trace file is not cross-checked", cases: []traceCase{
			{fixture: "fork-file", run: "rr", kind: "local", batch: 1, noTraceFile: true,
				want: map[string]string{"batch": "-", "trace": "-", "cross-check": "-"}},
		}, wantCrossChecked: map[string]string{"file": "0"}},
		{name: "a missing records directory disables the cross-check", missingRecords: true, cases: []traceCase{
			{fixture: "fork-file", run: "rs", trace: oneLine("file"),
				want: map[string]string{"batch": "-", "trace": "-", "cross-check": "-"}},
		}, wantReport: []string{"No cross-check: records repo "}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			d := newAuditDirs(t)
			var forks []forkFixture
			var runs []recordsRun
			for _, c := range row.cases {
				forks = append(forks, forkFixture{fixture: c.fixture, run: c.run, session: "s-" + c.run, agent: c.run})
				start := forkStart(t, c.fixture)
				if c.kind != "" {
					lead := c.spawnLead
					if lead == 0 {
						lead = 3 * time.Second
					}
					spawned := start.Add(-lead).UTC().Format("2006-01-02T15:04:05Z")
					run := recordsRun{name: c.run, kind: c.kind}
					if c.extraOnly {
						run.extraKey = spawned
					} else {
						run.batches = []batchFixture{{number: c.batch, cards: []string{"01-alpha"}, startSHA: sha, spawnedAt: spawned}}
					}
					runs = append(runs, run)
				}
				if !c.noTraceFile {
					var lines []string
					for _, spec := range c.trace {
						lines = append(lines, traceLine(start, spec))
					}
					writeTrace(t, d.hub, c.run, lines)
				}
			}
			placeForks(t, d.projects, d.hub, forks)
			records := filepath.Join(tempDir(t), "absent-records")
			if !row.missingRecords {
				records = recordsRepo(t, runs)
			}
			stdout, stderr, exit := runForkaudit(t, nil, d.args("--since", "2026-10-13", "--records", records)...)
			if exit != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", exit, stderr)
			}
			if stdout != d.out+"\n" {
				t.Errorf("stdout = %q, want the --out path", stdout)
			}
			content, err := os.ReadFile(d.out)
			if err != nil {
				t.Fatalf("read report: %v", err)
			}
			report := string(content)
			perFork := tableRows(t, report, "## Per-fork")
			for _, c := range row.cases {
				fork := rowWhere(t, perFork, "agent", "agent-"+c.run)
				for column, want := range c.want {
					if fork[column] != want {
						t.Errorf("%s: %s = %q, want %q", c.run, column, fork[column], want)
					}
				}
			}
			for _, want := range row.wantReport {
				if !strings.Contains(report, want) {
					t.Errorf("report lacks %q:\n%s", want, report)
				}
			}
			samples := tableRows(t, report, "## Samples")
			for arm, want := range row.wantCrossChecked {
				if got := rowWhere(t, samples, "arm", arm)["cross-checked"]; got != want {
					t.Errorf("arm %s cross-checked = %q, want %q", arm, got, want)
				}
			}
		})
	}
}

// codeRepo builds a code repository in which pkg/a.go and docs/b.md put each span of the fork-file fixture in its intended class.
// It returns the repository and its head sha.
func codeRepo(t *testing.T) (dir, head string) {
	t.Helper()
	dir = tempDir(t)
	gitkit.Git(t, dir, "init", "-b", "main")
	lines := make([]string, 21)
	lines[0] = "package pkg"
	lines[2] = "// Correct is declared on the next line."
	lines[3] = "func Correct() {"
	lines[4] = "}"
	lines[6] = "func Declared(y string) {"
	lines[7] = "}"
	lines[19] = "func Stale() {"
	lines[20] = "}"
	gitkit.CommitFile(t, dir, "pkg/a.go", strings.Join(lines, "\n")+"\n", "a")
	gitkit.CommitFile(t, dir, "pkg/extra.go", "package pkg\n", "extra")
	head = gitkit.CommitFile(t, dir, "docs/b.md", "# Doc B\n\nText without the heading.\n", "b")
	return dir, head
}

func TestForkauditPackCheck(t *testing.T) {
	t.Parallel()
	d := newAuditDirs(t)
	code, head := codeRepo(t)
	const absentSHA = "ffffffffffffffffffffffffffffffffffffffff"
	spawned := func(fixture string) string {
		return forkStart(t, fixture).Add(-3 * time.Second).UTC().Format("2006-01-02T15:04:05Z")
	}
	records := recordsRepo(t, []recordsRun{
		{name: "alpha", kind: "local", batches: []batchFixture{
			{number: 1, cards: []string{"01-alpha", "02-beta"}, startSHA: head, spawnedAt: spawned("fork-file")},
			{number: 2, cards: []string{"04-delta"}, startSHA: head, spawnedAt: spawned("fork-raf")},
		}},
		{name: "beta", kind: "local", batches: []batchFixture{
			{number: 1, cards: []string{"01-alpha", "02-beta"}, startSHA: absentSHA, spawnedAt: spawned("fork-file")},
		}},
	})
	placeForks(t, d.projects, d.hub, []forkFixture{
		{fixture: "fork-file", run: "alpha", session: "s-alpha", agent: "ffile"},
		{fixture: "fork-file-noedit", run: "alpha", session: "s-alpha", agent: "fnoedit"},
		{fixture: "fork-raf", run: "alpha", session: "s-alpha", agent: "fraf"},
		{fixture: "fork-file", run: "beta", session: "s-beta", agent: "bfile"},
	})
	stdout, stderr, exit := runForkaudit(t, nil, d.args("--since", "2026-10-13", "--records", records, "--pack-check", "--code", code)...)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", exit, stderr)
	}
	content, err := os.ReadFile(d.out)
	if err != nil || stdout != d.out+"\n" {
		t.Fatalf("report at %q: %v, stdout %q", d.out, err, stdout)
	}
	report := string(content)
	if !strings.Contains(report, "- Code: `"+code+"`\n") {
		t.Errorf("report header does not name --code:\n%s", report)
	}
	counts := map[string]map[string]string{
		"agent-ffile":   {"correct": "1", "correct-decl": "1", "stale": "1", "wrong": "1", "nofile": "1", "notree": "0", "unchecked": "1", "stale shifts": "8"},
		"agent-fraf":    {"correct": "1", "correct-decl": "0", "stale": "0", "wrong": "0", "nofile": "0", "notree": "0", "unchecked": "0"},
		"agent-fnoedit": {"correct": "0", "notree": "1"},
		"agent-bfile":   {"correct": "0", "notree": "6"},
	}
	perFork := tableRows(t, report, "### Span entries by class, per fork")
	for agent, want := range counts {
		row := rowWhere(t, perFork, "agent", agent)
		for column, cell := range want {
			if row[column] != cell {
				t.Errorf("%s %s = %q, want %q", agent, column, row[column], cell)
			}
		}
	}
	type missed struct{ agent, file, edits, writes, reads, existed string }
	var gotMissed []missed
	for _, row := range tableRows(t, report, "### Missed-file list") {
		gotMissed = append(gotMissed, missed{row["agent"], row["file"], row["edits"], row["writes"], row["reads"], row["existed"]})
	}
	wantMissed := []missed{
		{"agent-bfile", "pkg/extra.go", "1", "0", "0", "unknown"},
		{"agent-bfile", "pkg/new.go", "0", "1", "0", "unknown"},
		{"agent-ffile", "pkg/extra.go", "1", "0", "0", "yes"},
		{"agent-ffile", "pkg/new.go", "0", "1", "0", "no"},
		{"agent-fnoedit", "pkg/other.go", "0", "0", "1", "unknown"},
	}
	if !slices.Equal(gotMissed, wantMissed) {
		t.Errorf("missed-file list = %v, want %v", gotMissed, wantMissed)
	}
	byArm := rowWhere(t, tableRows(t, report, "### Missed files by arm"), "arm", "file")
	if got := []string{byArm["missed files"], byArm["existed"], byArm["absent"], byArm["unknown"]}; !slices.Equal(got, []string{"5", "1", "1", "3"}) {
		t.Errorf("file arm missed files, existed, absent, unknown = %v, want 5 1 1 3", got)
	}
}
