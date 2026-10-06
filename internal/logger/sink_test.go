// sink_test.go covers the durable sink's naming, lazy-open header composition, both open triggers,
// and the size-cap truncation marker.
// Every case calls SetDurableSinkDir(t.TempDir()) at its own start, never sharing one call across
// cases, so no lyxcwd.Resolve ever runs (Test Tier Purity) and every case starts from a fully reset
// sink regardless of what an earlier case in this file (or logger_test.go/span_test.go) already
// triggered.
// No test in this file calls t.Parallel: each rewrites the process-global durable sink state that
// SetDurableSinkDir resets.

package logger

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

var sinkTestFilePattern = regexp.MustCompile(`^trace-\d{8}T\d{6}Z-[0-9a-f]{16}-(\d+)\.log$`)

func listSinkDirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) = _, %v; want nil error", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func readSinkFirstLine(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", path, err)
	}
	lines := strings.SplitN(string(data), "\n", 2)
	return lines[0]
}

// readSoleSinkFile returns the content of the one trace file in dir, failing the test when dir holds
// any other number.
func readSoleSinkFile(t *testing.T, dir string) string {
	t.Helper()
	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	return string(data)
}

// TestEnsureDurableSink_CreatesOneNamedFileWithHeader pins the lazy open: exactly one trace file
// lands in the directory SetDurableSinkDir* names and nowhere in the cwd-derived location, its name
// follows trace-<ts>-<16hex>-<pid>.log, and its first line is the header record carrying the trace ID
// and, when the caller supplied one, the worktree root.
//
//testtiming:keep pins the trace file name grammar, the header first line and the worktree root in the header, which TestEnsureDurableSink_ConcurrentRedirectIsRaceFree does not assert
func TestEnsureDurableSink_CreatesOneNamedFileWithHeader(t *testing.T) {
	suppliedRoot := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	tests := []struct {
		name         string
		worktreeRoot string
	}{
		{name: "no worktree root supplied", worktreeRoot: ""},
		{name: "worktree root supplied", worktreeRoot: suppliedRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.worktreeRoot == "" {
				SetDurableSinkDir(dir)
			} else {
				SetDurableSinkDirWithWorktreeRoot(dir, tt.worktreeRoot)
			}
			t.Cleanup(func() { SetDurableSinkDir("") })

			if ok := ensureDurableSink(); !ok {
				t.Fatalf("ensureDurableSink() ok = false; want true")
			}

			if _, err := os.Stat(sinkPath); err != nil {
				t.Fatalf("os.Stat(sinkPath=%q) = %v; want the sink file to exist", sinkPath, err)
			}
			if filepath.Dir(sinkPath) != dir {
				t.Errorf("filepath.Dir(sinkPath) = %q; want %q (the overridden directory)", filepath.Dir(sinkPath), dir)
			}
			files := listSinkDirFiles(t, dir)
			if len(files) != 1 {
				t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file", files)
			}

			match := sinkTestFilePattern.FindStringSubmatch(files[0])
			if match == nil {
				t.Fatalf("filename %q does not match trace-<ts>-<16hex>-<pid>.log grammar", files[0])
			}
			gotPID, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatalf("strconv.Atoi(%q) = _, %v; want nil error", match[1], err)
			}
			if gotPID != os.Getpid() {
				t.Errorf("filename pid segment = %d; want os.Getpid() = %d", gotPID, os.Getpid())
			}

			first := readSinkFirstLine(t, filepath.Join(dir, files[0]))
			if !strings.HasPrefix(first, "command=") {
				t.Errorf("first line = %q; want it to start with the header record's command= field", first)
			}
			if !strings.Contains(first, "trace="+TraceID()) {
				t.Errorf("header line = %q; want it to contain trace=%s", first, TraceID())
			}
			if header.WorktreeRoot != tt.worktreeRoot {
				t.Errorf("header.WorktreeRoot = %q; want %q", header.WorktreeRoot, tt.worktreeRoot)
			}
			if tt.worktreeRoot != "" && !strings.Contains(first, "worktree_root="+tt.worktreeRoot) {
				t.Errorf("header line = %q; want it to contain worktree_root=%s", first, tt.worktreeRoot)
			}

			cwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("os.Getwd() error = %v; want nil", err)
			}
			if _, err := os.Stat(filepath.Join(cwd, ".lyx", "logs")); err == nil {
				t.Errorf("cwd-derived sink location %q exists; want no file there when the override redirects the sink", filepath.Join(cwd, ".lyx", "logs"))
			}
		})
	}
}

// TestEnsureDurableSink_AdoptedTraceIDCannotEscapeTheLogsDirectory is R4-09's end-to-end guard, the
// one that shows why the alphabet check in trace.go is a containment property and not a cosmetic
// one: ensureDurableSink interpolates header.TraceID into the filename and hands the result to
// filepath.Join, which CLEANS it -- so an adopted 'ci-run/../../pwned' used to place the trace file
// two levels above the logs directory. The assertion is positional, not textual: whatever the
// filename ends up being, it must sit inside dir, and dir's grandparent must stay empty.
//
//testtiming:keep a guard that an adopted path-traversal trace ID cannot place the file above the logs directory, which no covering test asserts
func TestEnsureDurableSink_AdoptedTraceIDCannotEscapeTheLogsDirectory(t *testing.T) {
	grandparent := t.TempDir()
	parent := filepath.Join(grandparent, "state", ".lyx")
	dir := filepath.Join(parent, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v; want nil", dir, err)
	}

	resetTraceState(t)
	t.Setenv("LYX_TRACE_ID", "ci-run/../../pwned")
	SetDurableSinkDir(dir)
	t.Cleanup(func() { SetDurableSinkDir("") })

	if ok := ensureDurableSink(); !ok {
		t.Fatalf("ensureDurableSink() ok = false; want true")
	}

	if got := filepath.Dir(sinkPath); got != dir {
		t.Errorf("filepath.Dir(sinkPath=%q) = %q; want %q -- the adopted trace ID walked the write out of the logs directory", sinkPath, got, dir)
	}
	if files := listSinkDirFiles(t, dir); len(files) != 1 {
		t.Errorf("listSinkDirFiles(dir) = %v; want exactly one file inside the logs directory", files)
	}
	for _, escaped := range []string{parent, grandparent} {
		if files := listSinkDirFiles(t, escaped); len(files) != 0 {
			t.Errorf("listSinkDirFiles(%q) = %v; want empty -- no trace file may land above the logs directory", escaped, files)
		}
	}

	// A filename Sweep cannot match is the second, independent half of R4-09: the file would never
	// be ranked or removed and the logs directory would grow without bound.
	files := listSinkDirFiles(t, dir)
	if len(files) == 1 && !sinkTestFilePattern.MatchString(files[0]) {
		t.Errorf("filename %q does not match the retention-sweepable grammar; Sweep would never reclaim it", files[0])
	}
}

// TestSetDurableSinkDirWithWorktreeRoot_AfterArmDoesNotMoveAlreadyOpenedFile is the ordering
// obligation as an executable assertion: setting the directory after the sink is already armed
// does not move the file that was already opened.
//
//testtiming:keep pins that a redirect after arming resets sinkPath, reports not armed and leaves the opened file in place, which TestEnsureDurableSink_ConcurrentRedirectIsRaceFree does not assert
func TestSetDurableSinkDirWithWorktreeRoot_AfterArmDoesNotMoveAlreadyOpenedFile(t *testing.T) {
	firstDir := t.TempDir()
	SetDurableSinkDir(firstDir)
	t.Cleanup(func() { SetDurableSinkDir("") })

	ok := ensureDurableSink()
	if !ok {
		t.Fatalf("ensureDurableSink() ok = false; want true")
	}
	armedPath := sinkPath

	secondDir := t.TempDir()
	SetDurableSinkDirWithWorktreeRoot(secondDir, "irrelevant-worktree-root")

	if sinkPath != "" {
		t.Errorf("sinkPath after SetDurableSinkDirWithWorktreeRoot = %q; want reset to empty until the sink is re-armed", sinkPath)
	}
	if got := CurrentSinkArmState(); got.Armed {
		t.Errorf("CurrentSinkArmState() = %+v; want Armed false after a redirect until the next arm", got)
	}
	if _, err := os.Stat(armedPath); err != nil {
		t.Errorf("os.Stat(armedPath=%q) = %v; want the already-opened file to remain untouched", armedPath, err)
	}
}

// TestEnsureDurableSink_ConcurrentRedirectIsRaceFree is the regression guard for the R4 review's
// R4-20: the lazy first-open read sinkDirOverride and wrote sinkPath, sinkOK, sinkBytesWritten and
// header from inside a sync.Once with NO lock, while resetDurableSinkLocked wrote all of those AND
// reassigned that very sync.Once under sinkMu. A SetDurableSinkDir* concurrent with an in-flight
// first record was therefore a data race on the Once value itself, and the losing order left the
// PRE-redirect sinkPath installed -- and that redirect is the mechanism keeping a standalone run's
// trace files out of the operator's own repository.
//
// It must be run under `go test -race` to observe the race itself; the positional assertion below
// holds either way, and is what catches a half-applied reset leaving a path composed from neither
// generation.
//
//testtiming:keep a regression guard that a redirect racing the first open leaves the sink in one of the two directories, which its covering tests do not assert
func TestEnsureDurableSink_ConcurrentRedirectIsRaceFree(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	t.Cleanup(func() { SetDurableSinkDir("") })

	// Several rounds, because the interleaving that loses is order-dependent: one arm/redirect pair
	// can easily complete in the safe order by luck.
	for round := 0; round < 20; round++ {
		SetDurableSinkDir(firstDir)

		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); ensureDurableSink() }()
		go func() { defer wg.Done(); Arm() }()
		go func() {
			defer wg.Done()
			SetDurableSinkDirWithWorktreeRoot(secondDir, "some-worktree-root")
		}()
		wg.Wait()

		// Whichever order won, re-arming must land the sink in the directory the surviving
		// generation names -- never a stale composite of the two.
		if !ensureDurableSink() {
			t.Fatalf("round %d: ensureDurableSink() ok = false; want true", round)
		}
		if dir := filepath.Dir(sinkPath); dir != firstDir && dir != secondDir {
			t.Fatalf("round %d: filepath.Dir(sinkPath=%q) = %q; want one of %q or %q", round, sinkPath, dir, firstDir, secondDir)
		}
	}
}

//testtiming:keep pins that Arm and the implicit first record compose identical static header fields, which no covering test compares
func TestArm_ExplicitVsImplicitProduceIdenticalStaticHeaderFields(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	Arm()
	explicit := header

	dir2 := t.TempDir()
	SetDurableSinkDir(dir2)
	ensureDurableSink()
	implicit := header

	if explicit.Command != implicit.Command {
		t.Errorf("Command: explicit-Arm = %q, implicit-arm = %q; want equal", explicit.Command, implicit.Command)
	}
	if strings.Join(explicit.Argv, " ") != strings.Join(implicit.Argv, " ") {
		t.Errorf("Argv: explicit-Arm = %v, implicit-arm = %v; want equal", explicit.Argv, implicit.Argv)
	}
	if explicit.TraceID != implicit.TraceID {
		t.Errorf("TraceID: explicit-Arm = %q, implicit-arm = %q; want equal", explicit.TraceID, implicit.TraceID)
	}
	if explicit.PID != implicit.PID {
		t.Errorf("PID: explicit-Arm = %d, implicit-arm = %d; want equal", explicit.PID, implicit.PID)
	}
}

// TestSinkArmTriggers pins what arms the durable sink: an Info-or-above record and a non-zero
// NotifyExit open it with the header as first line, while no record and NotifyExit(0) leave the
// directory empty and the sink unarmed.
func TestSinkArmTriggers(t *testing.T) {
	tests := []struct {
		name      string
		trigger   func()
		wantFiles int
		wantArmed bool
	}{
		{name: "no record arms nothing", trigger: func() {}, wantFiles: 0, wantArmed: false},
		{name: "NotifyExit(0) is a no-op", trigger: func() { NotifyExit(0) }, wantFiles: 0, wantArmed: false},
		{name: "NotifyExit(1) opens the sink with the header only", trigger: func() { NotifyExit(1) }, wantFiles: 1, wantArmed: true},
		{name: "an Info record opens the sink", trigger: func() { Info("arm state probe") }, wantFiles: 1, wantArmed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			SetDurableSinkDir(dir)

			tt.trigger()

			files := listSinkDirFiles(t, dir)
			if len(files) != tt.wantFiles {
				t.Fatalf("listSinkDirFiles(dir) = %v; want %d file(s)", files, tt.wantFiles)
			}
			if tt.wantFiles == 1 {
				if first := readSinkFirstLine(t, filepath.Join(dir, files[0])); !strings.HasPrefix(first, "command=") {
					t.Errorf("first line = %q; want the header record", first)
				}
			}
			want := SinkArmState{}
			if tt.wantArmed {
				want = SinkArmState{Armed: true, Redirected: true, Dir: dir, AnchorPath: ""}
			}
			if got := CurrentSinkArmState(); got.Armed != tt.wantArmed || (tt.wantArmed && got != want) {
				t.Errorf("CurrentSinkArmState() = %+v; want Armed %v (full state %+v when armed)", got, tt.wantArmed, want)
			}
		})
	}
}

func countMarkerLines(data []byte) int {
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "trace sink truncated: size cap reached" {
			count++
		}
	}
	return count
}

//testtiming:keep pins that writes past the size cap are dropped behind a single truncation marker, which TestWriteDurable_ConcurrentWarnCallsProduceOneFileAndOneTruncationMarker does not assert on the dropped payload
func TestWriteDurable_SizeCapStopsWritesAfterSingleTruncationMarker(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	ensureDurableSink()

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file", files)
	}
	path := filepath.Join(dir, files[0])

	big := make([]byte, sinkMaxBytes+1)
	if _, err := writeDurable(big); err != nil {
		t.Fatalf("writeDurable(big) error = %v; want nil", err)
	}
	if _, err := writeDurable([]byte("after cap #1\n")); err != nil {
		t.Fatalf("writeDurable(after cap #1) error = %v; want nil", err)
	}
	if _, err := writeDurable([]byte("after cap #2\n")); err != nil {
		t.Fatalf("writeDurable(after cap #2) error = %v; want nil", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", path, err)
	}
	if got := countMarkerLines(data); got != 1 {
		t.Errorf("countMarkerLines(data) = %d; want exactly 1 truncation marker line", got)
	}
	if strings.Contains(string(data), "after cap #1") || strings.Contains(string(data), "after cap #2") {
		t.Errorf("file contains post-cap payload; want writes past the cap to be dropped, not appended")
	}
}

// TestWriteDurable_SurvivesLogsDirRenameMidProcess is the load-bearing regression guard for this
// batch.
// Against the pre-batch code, sinkWriter is an *os.File opened once and held for the process
// lifetime: a POSIX rename of that file's directory leaves the held descriptor still valid and
// still pointed at the (now differently-named) underlying file, so a second write through that
// stale descriptor never becomes visible under a directory freshly recreated at the original path
// -- exactly what a fabric content-adoption step recreating .lyx/logs after moving the old tree away
// would do.
// Against the handle-free sink (writeDurable opens, appends, and closes sinkPath per record), the
// second write re-opens by path and lands in the freshly recreated directory, because no descriptor
// survives from the first write to intercept it.
// The rename succeeding plus the second record landing under the recreated original directory are
// the only observables available on Linux; enumerating open file descriptors is deliberately out of
// scope.
//
//testtiming:keep a regression guard that a record written after the logs directory is renamed and recreated lands in the recreated directory, which no covering test asserts
func TestWriteDurable_SurvivesLogsDirRenameMidProcess(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "logs")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q) error = %v; want nil", dir, err)
	}
	SetDurableSinkDir(dir)
	if ok := ensureDurableSink(); !ok {
		t.Fatalf("ensureDurableSink() ok = false; want true")
	}

	if _, err := writeDurable([]byte("first record\n")); err != nil {
		t.Fatalf("writeDurable(first record) error = %v; want nil", err)
	}

	renamedDir := filepath.Join(parent, "logs-renamed")
	if err := os.Rename(dir, renamedDir); err != nil {
		t.Fatalf("os.Rename(%q, %q) error = %v; want nil", dir, renamedDir, err)
	}
	// Recreate an empty directory at the original path, as fabric's content adoption would once it
	// has moved the old tree elsewhere -- this is what makes a stale descriptor observable: a write
	// through one never shows up here.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q) (recreate) error = %v; want nil", dir, err)
	}

	if _, err := writeDurable([]byte("second record\n")); err != nil {
		t.Fatalf("writeDurable(second record) error = %v; want nil", err)
	}

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) (recreated original) = %v; want exactly one file holding the second record -- a stale descriptor from before the rename would leave this directory empty", files)
	}

	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile error = %v; want nil", err)
	}
	if !strings.Contains(string(data), "second record") {
		t.Errorf("sink file contents = %q; want the second record present under the recreated original directory", string(data))
	}
}

// TestIsLyxWorktree_GatesTheCwdAnchoredFallback pins R6-6's decision: the durable sink's
// cwd-anchored fallback may arm only inside a worktree lyx actually owns. lyxcwd.Resolve succeeds
// for any plain git repository standing at its root, and cmd/lyx force-arms the sink on every
// non-zero exit, so without this gate every refusal — a standalone webster/burler invocation refused
// before its own sink redirect, or an unknown subcommand that never reached wiring — created
// <repo>/.lyx/logs inside a checkout lyx does not own.
//
//testtiming:keep pins isLyxWorktree for a plain checkout and an anchored worktree without git, which its covering integration test reaches only through the real call site
func TestIsLyxWorktree_GatesTheCwdAnchoredFallback(t *testing.T) {
	t.Parallel()

	t.Run("a plain checkout is not a lyx worktree", func(t *testing.T) {
		t.Parallel()

		hub := t.TempDir()
		if err := os.MkdirAll(filepath.Join(hub, "plain"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		layout := &lyxcwd.Location{RepoName: "plain", HubPath: hub, WorktreeName: "plain", AnchorRel: "."}
		if isLyxWorktree(layout) {
			t.Errorf("isLyxWorktree(%q) = true; want false — arming here writes .lyx into a repository lyx does not own", layout.AnchorPath())
		}
	})

	t.Run("an anchored worktree carrying _lyx is a lyx worktree", func(t *testing.T) {
		t.Parallel()

		hub := t.TempDir()
		anchor := filepath.Join(hub, "wired", "backend")
		if err := os.MkdirAll(filepath.Join(anchor, lyxdirs.LyxDirName), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		layout := &lyxcwd.Location{RepoName: "wired", HubPath: hub, WorktreeName: "wired", AnchorRel: "backend"}
		if !isLyxWorktree(layout) {
			t.Errorf("isLyxWorktree(%q) = false; want true — this is exactly the worktree the fallback exists for", layout.AnchorPath())
		}
	})
}

func TestTraceFileAndDir_EmptyWithoutSink(t *testing.T) {
	SetDurableSinkDir("")
	t.Cleanup(func() { SetDurableSinkDir("") })

	if got := TraceFile(); got != "" {
		t.Errorf("TraceFile() = %q; want empty", got)
	}
	if got := TraceDir(); got != "" {
		t.Errorf("TraceDir() = %q; want empty", got)
	}
}

func TestTraceFile_ArmsOnceAndReturnsStablePath(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	t.Cleanup(func() { SetDurableSinkDir("") })

	first := TraceFile()
	if filepath.Dir(first) != dir {
		t.Fatalf("TraceFile() = %q; want a path inside %q", first, dir)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("os.Stat(%q) = %v; want nil error", first, err)
	}
	if line := readSinkFirstLine(t, first); !strings.HasPrefix(line, "command=") {
		t.Errorf("first line = %q; want header line", line)
	}
	if second := TraceFile(); first != second {
		t.Errorf("TraceFile() second call = %q; want %q", second, first)
	}
	if files := listSinkDirFiles(t, dir); len(files) != 1 {
		t.Errorf("dir holds %v; want exactly one file", files)
	}
}

func TestTraceDir_MatchesOverrideAndCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet")
	SetDurableSinkDir(dir)
	t.Cleanup(func() { SetDurableSinkDir("") })

	if got := TraceDir(); got != dir {
		t.Errorf("TraceDir() = %q; want %q", got, dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) = %v; want not-exist", dir, err)
	}
}

// TestEnsureDurableSink_ArmingDoesNotSweep pins that opening the sink leaves a pre-seeded, aged, dead-pid trace file in place;
// the sweep runs at process exit, never at arm.
//
//testtiming:keep pins that arming leaves an aged dead-pid trace file in place, which no covering test asserts
func TestEnsureDurableSink_ArmingDoesNotSweep(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	t.Cleanup(func() { SetDurableSinkDir("") })
	aged := writeTraceTestFile(t, dir, time.Now().Add(-15*24*time.Hour), hexID(1), deadTestPID)

	Info("arm the sink")

	assertExists(t, aged)
}
