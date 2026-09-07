// sink_test.go covers the durable sink's naming, lazy-open header composition, both open triggers,
// and the size-cap truncation marker.
// Every case calls SetDurableSinkDir(t.TempDir()) at its own start, never sharing one call across
// cases, so no lyxcwd.Resolve ever runs (Test Tier Purity) and every case starts from a fully reset
// sink regardless of what an earlier case in this file (or logger_test.go/span_test.go) already
// triggered.

package logger

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
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

func TestEnsureDurableSink_DebugOnlyNeverOpensFile(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	if got := listSinkDirFiles(t, dir); len(got) != 0 {
		t.Errorf("listSinkDirFiles(dir) = %v; want empty (no sink file from Debug-only activity)", got)
	}
}

func TestEnsureDurableSink_CreatesExactlyOneFileWithHeaderFirstLine(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	ok := ensureDurableSink()
	if !ok {
		t.Fatalf("ensureDurableSink() ok = false; want true")
	}
	if _, err := os.Stat(sinkPath); err != nil {
		t.Fatalf("os.Stat(sinkPath=%q) = %v; want the sink file to exist", sinkPath, err)
	}

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file", files)
	}

	first := readSinkFirstLine(t, filepath.Join(dir, files[0]))
	if !strings.HasPrefix(first, "command=") {
		t.Errorf("first line = %q; want it to start with the header record's command= field", first)
	}
}

func TestEnsureDurableSink_FilenameGrammarAndFields(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	ensureDurableSink()

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file", files)
	}
	name := files[0]

	match := sinkTestFilePattern.FindStringSubmatch(name)
	if match == nil {
		t.Fatalf("filename %q does not match trace-<ts>-<16hex>-<pid>.log grammar", name)
	}
	gotPID, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("strconv.Atoi(%q) = _, %v; want nil error", match[1], err)
	}
	if gotPID != os.Getpid() {
		t.Errorf("filename pid segment = %d; want os.Getpid() = %d", gotPID, os.Getpid())
	}

	first := readSinkFirstLine(t, filepath.Join(dir, name))
	if !strings.Contains(first, "trace="+TraceID()) {
		t.Errorf("header line = %q; want it to contain trace=%s", first, TraceID())
	}
}

// TestEnsureDurableSink_AdoptedTraceIDCannotEscapeTheLogsDirectory is R4-09's end-to-end guard, the
// one that shows why the alphabet check in trace.go is a containment property and not a cosmetic
// one: ensureDurableSink interpolates header.TraceID into the filename and hands the result to
// filepath.Join, which CLEANS it -- so an adopted 'ci-run/../../pwned' used to place the trace file
// two levels above the logs directory. The assertion is positional, not textual: whatever the
// filename ends up being, it must sit inside dir, and dir's grandparent must stay empty.
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

// TestEnsureDurableSink_NoWorktreeRootSuppliedLeavesHeaderEmpty pins that an empty
// header.WorktreeRoot is now a choice the caller made by picking the no-worktree-root-supplied
// shorthand (SetDurableSinkDir), not a property of the seam itself.
func TestEnsureDurableSink_NoWorktreeRootSuppliedLeavesHeaderEmpty(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	ensureDurableSink()

	if header.WorktreeRoot != "" {
		t.Errorf("header.WorktreeRoot = %q; want empty when SetDurableSinkDir (no-worktree-root-supplied shorthand) was called", header.WorktreeRoot)
	}
}

// TestEnsureDurableSink_WorktreeRootSuppliedReachesHeaderAndFile is the populated counterpart to
// TestEnsureDurableSink_NoWorktreeRootSuppliedLeavesHeaderEmpty: SetDurableSinkDirWithWorktreeRoot
// leaves header.WorktreeRoot equal to the supplied value, and the trace file's first line carries
// it.
func TestEnsureDurableSink_WorktreeRootSuppliedReachesHeaderAndFile(t *testing.T) {
	dir := t.TempDir()
	worktreeRoot := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot)
	t.Cleanup(func() { SetDurableSinkDir("") })

	ensureDurableSink()

	if header.WorktreeRoot != worktreeRoot {
		t.Errorf("header.WorktreeRoot = %q; want %q", header.WorktreeRoot, worktreeRoot)
	}

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file", files)
	}
	first := readSinkFirstLine(t, filepath.Join(dir, files[0]))
	if !strings.Contains(first, "worktree_root="+worktreeRoot) {
		t.Errorf("header line = %q; want it to contain worktree_root=%s", first, worktreeRoot)
	}
}

// TestSetDurableSinkDirWithWorktreeRoot_RedirectsSinkFileLocation pins that the override actually
// redirects the trace file: with the override set before the first record, the file lands in the
// given directory and no file appears in the cwd-derived location.
func TestSetDurableSinkDirWithWorktreeRoot_RedirectsSinkFileLocation(t *testing.T) {
	dir := t.TempDir()
	worktreeRoot := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot)
	t.Cleanup(func() { SetDurableSinkDir("") })

	ok := ensureDurableSink()
	if !ok {
		t.Fatalf("ensureDurableSink() ok = false; want true")
	}

	if _, err := os.Stat(sinkPath); err != nil {
		t.Fatalf("os.Stat(sinkPath=%q) = %v; want the sink file to exist under the overridden directory", sinkPath, err)
	}
	if filepath.Dir(sinkPath) != dir {
		t.Errorf("filepath.Dir(sinkPath) = %q; want %q (the overridden directory)", filepath.Dir(sinkPath), dir)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v; want nil", err)
	}
	if cwd == dir {
		t.Fatalf("cwd = %q equals overridden dir %q; test fixture invalid", cwd, dir)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".lyx", "logs")); err == nil {
		t.Errorf("cwd-derived sink location %q exists; want no file there when the override redirects the sink", filepath.Join(cwd, ".lyx", "logs"))
	}
}

// TestSetDurableSinkDirWithWorktreeRoot_AfterArmDoesNotMoveAlreadyOpenedFile is the ordering
// obligation as an executable assertion: setting the directory after the sink is already armed
// does not move the file that was already opened.
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

func TestNotifyExit_ZeroCodeNeverOpensSink(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	NotifyExit(0)

	if got := listSinkDirFiles(t, dir); len(got) != 0 {
		t.Errorf("listSinkDirFiles(dir) = %v; want empty (NotifyExit(0) must be a no-op)", got)
	}
}

func TestNotifyExit_NonZeroCodeOpensSinkWithHeaderOnly(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	NotifyExit(1)

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one file (trigger (b) in isolation)", files)
	}
	first := readSinkFirstLine(t, filepath.Join(dir, files[0]))
	if !strings.HasPrefix(first, "command=") {
		t.Errorf("first line = %q; want the header record", first)
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
