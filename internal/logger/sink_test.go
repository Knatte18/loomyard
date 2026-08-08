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

func TestEnsureDurableSink_SeamPathLeavesWorktreeRootEmpty(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)

	ensureDurableSink()

	if header.WorktreeRoot != "" {
		t.Errorf("header.WorktreeRoot = %q; want empty on the SetDurableSinkDir seam path (geometry never resolved)", header.WorktreeRoot)
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
