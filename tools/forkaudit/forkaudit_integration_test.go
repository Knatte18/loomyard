//go:build integration

// forkaudit_integration_test.go runs forkaudit.sh against temporary directories and checks its exit codes, refusals and report.

package forkaudit

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// workMarker is the file that marks a directory as a forkaudit work directory.
const workMarker = ".forkaudit-work"

// noForksSentence is the report line for a run that matched no fork.
const noForksSentence = "No Webster implementer forks matched."

// runForkaudit runs forkaudit.sh with the given extra environment and arguments from a fresh working directory.
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
				return []string{"--since", "2026-10-13"}
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
