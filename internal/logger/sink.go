// sink.go implements the durable second sink: a per-process trace file, opened lazily.
// The file opens on the first record the durable half admits, or when the process exits with a non-zero code.
// Writes are capped in size, and a single truncation marker is written once the cap is crossed.

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

const sinkMaxBytes = 8 * 1024 * 1024

// LogsDir returns the worktree-level directory where the durable trace sink writes one file per process.
// The directory lies under the ephemeral `.lym` directory, which is anchored at `AnchorPath`.
// It is never under the durable, fabric-synced `_lym` tree.
func LogsDir(l *lymcwd.Location) string {
	return LogsDirAt(l.AnchorPath())
}

// LogsDirAt returns the trace-file directory under anchorPath.
// It is for a caller that holds an anchor path rather than a `lymcwd.Location`.
func LogsDirAt(anchorPath string) string {
	return filepath.Join(anchorPath, lymdirs.DotLymDirName, "logs")
}

type sinkHeader struct {
	Command      string
	Argv         []string
	TraceID      string
	PID          int
	WorktreeRoot string
}

var headerOnce sync.Once
var header sinkHeader

// armHeader captures the static header fields once per sink generation.
// Callers hold sinkMu: headerOnce is reassigned when a sink generation resets.
// Running it outside that mutex would race on the sync.Once value itself.
func armHeader() {
	headerOnce.Do(func() {
		header.Command = os.Args[0]
		header.Argv = append([]string(nil), os.Args[1:]...)
		header.TraceID = TraceID()
		header.PID = os.Getpid()
	})
}

// Arm captures the durable sink's static header fields ahead of the first log call.
// It takes sinkMu itself, because every reader and writer of headerOnce and header must agree on one mutex.
// Callers outside the package cannot hold sinkMu, so Arm must take it.
func Arm() {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	armHeader()
}

// sinkArmed reports whether the lazy first open has run for the current sink generation.
// It is a plain bool guarded by sinkMu rather than a sync.Once, because the sink is re-armable:
// each redirect of the sink directory starts a new generation.
// A sync.Once that is reassigned per generation would race with any reader outside sinkMu.
var sinkArmed bool
var sinkPath string
var sinkOK bool
var sinkDirOverride string
var sinkDir string
var sinkAnchorPath string
var sinkMu sync.Mutex
var sinkBytesWritten int64
var sinkTruncated bool

// ensureDurableSink lazily resolves and arms the durable trace-file sink, at most once per sink generation.
// It runs under sinkMu, which the sink's other entry points also take, so an arm never interleaves with a redirect.
// It never keeps a file handle open between calls: the trace file is opened, header-written, and closed again here.
// Every later record is written by opening, appending to and closing the file under sinkMu.
// No descriptor is ever held pinned to the old location, so the sink survives its directory being renamed mid-process.
// Callers must not hold sinkMu: it is a plain, non-reentrant Mutex, so a nested call deadlocks.
func ensureDurableSink() bool {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	if !sinkArmed {
		sinkArmed = true
		sinkOK = armDurableSinkLocked()
	}
	return sinkOK
}

// armDurableSinkLocked performs the one-time resolve, open and header write of the durable sink.
// It reports whether the sink came up.
// Callers hold sinkMu.
// It is a separate function so every failure path is a plain `return false` at one level of nesting.
func armDurableSinkLocked() bool {
	dir, worktreeRoot, anchorPath, ok := resolveSinkDirLocked()
	if !ok {
		return false
	}

	armHeader()
	// The header's `WorktreeRoot` is trace metadata only.
	// The trace file's location is anchored at the anchor path, so the two differ by design.
	if worktreeRoot != "" {
		header.WorktreeRoot = worktreeRoot
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}

	filename := fmt.Sprintf("trace-%s-%s-%d.log",
		time.Now().UTC().Format(traceFileTimestampLayout),
		header.TraceID,
		header.PID,
	)
	path := filepath.Join(dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}

	line := headerLine()
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return false
	}
	_ = f.Close()

	sinkPath = path
	sinkDir = dir
	sinkAnchorPath = anchorPath
	sinkBytesWritten = int64(len(line))
	return true
}

// resolveSinkDirLocked resolves the directory the durable sink writes to.
// It also returns the worktree root to record in the header and the anchor path the resolution used.
// It reports whether a directory could be resolved.
// Callers hold sinkMu.
// An override returns an empty worktreeRoot and anchorPath;
// otherwise the cwd-anchored resolution applies, gated off under `go test` unless LYM_TRACE is "1".
func resolveSinkDirLocked() (dir, worktreeRoot, anchorPath string, ok bool) {
	if sinkDirOverride != "" {
		return sinkDirOverride, "", "", true
	}
	if testing.Testing() && os.Getenv("LYM_TRACE") != "1" {
		return "", "", "", false
	}
	cwd, err := lymcwd.Getwd()
	if err != nil {
		return "", "", "", false
	}
	layout, err := lymcwd.Resolve(cwd)
	if err != nil {
		return "", "", "", false
	}
	if !isLymWorktree(layout) {
		return "", "", "", false
	}
	return LogsDir(layout), layout.WorktreePath(), layout.AnchorPath(), true
}

// SinkArmState is a snapshot of the durable sink's arm state for the current sink generation.
type SinkArmState struct {
	// Armed reports whether the sink came up in the current generation.
	Armed bool
	// Redirected reports whether an override of the sink directory was in force.
	Redirected bool
	// Dir is the sink directory; empty until the sink arms.
	Dir string
	// AnchorPath is the lym worktree anchor the cwd-anchored resolution used; empty when redirected.
	AnchorPath string
}

// CurrentSinkArmState reports the durable sink's arm state without arming the sink, creating anything, or logging.
func CurrentSinkArmState() SinkArmState {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	return SinkArmState{
		Armed:      sinkArmed && sinkOK,
		Redirected: sinkDirOverride != "",
		Dir:        sinkDir,
		AnchorPath: sinkAnchorPath,
	}
}

// TraceFile forces the lazy durable sink open and returns the absolute path of this process's trace file.
// It returns "" when no sink can arm, and the same path on every call within one sink generation.
func TraceFile() string {
	if !ensureDurableSink() {
		return ""
	}
	sinkMu.Lock()
	defer sinkMu.Unlock()

	return sinkPath
}

// TraceDir returns the directory the durable sink would write to, or "" when none resolves.
// It lets a caller list trace files by trace id without arming a sink of its own;
// it never arms the sink, creates anything, or sweeps, so the directory need not exist yet.
func TraceDir() string {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	dir, _, _, ok := resolveSinkDirLocked()
	if !ok {
		return ""
	}
	return dir
}

// isLymWorktree reports whether layout names a worktree lym owns, by the durable `_lym` tree at its anchor.
//
// The gate is needed because `lymcwd.Resolve` succeeds for any plain git repository, since no hub is required.
// Without the gate, the cwd-anchored resolution writes trace files into a checkout lym does not own.
// A non-zero exit, reported through `NotifyExit`, force-arms the sink, so refused commands reach this gate too.
// In standalone mode nothing lym writes may land in the repository it drives.
//
// A command run in a git worktree that lym has not wired therefore writes no trace file.
// That is intended: writing one there would be the defect.
func isLymWorktree(layout *lymcwd.Location) bool {
	_, err := os.Stat(filepath.Join(layout.AnchorPath(), lymdirs.LymDirName))
	return err == nil
}

// headerLine renders the durable sink's first-line header record as one plain-text line.
// The line holds key=value pairs for command, argv, trace, pid and worktree_root, in that order.
func headerLine() string {
	var b strings.Builder
	b.WriteString("command=")
	b.WriteString(header.Command)
	b.WriteString(" argv=")
	b.WriteString(strings.Join(header.Argv, " "))
	b.WriteString(" trace=")
	b.WriteString(header.TraceID)
	b.WriteString(" pid=")
	fmt.Fprintf(&b, "%d", header.PID)
	b.WriteString(" worktree_root=")
	b.WriteString(header.WorktreeRoot)
	b.WriteString("\n")
	return b.String()
}

// writeDurable writes p to the durable sink, enforcing the size cap and truncation marker.
// Each write opens the sink file, appends, and closes it again, all under sinkMu.
// No descriptor outlives the call, so a directory rename cannot invalidate one.
func writeDurable(p []byte) (int, error) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	if sinkTruncated {
		return len(p), nil
	}

	if sinkBytesWritten+int64(len(p)) > sinkMaxBytes {
		marker := "trace sink truncated: size cap reached\n"
		_, _ = appendToSink([]byte(marker))
		sinkTruncated = true
		return len(p), nil
	}

	n, err := appendToSink(p)
	sinkBytesWritten += int64(n)
	return n, err
}

// appendToSink opens sinkPath, appends p, and closes the file again.
// Callers hold sinkMu.
// Every append to the durable sink after its header goes through here.
func appendToSink(p []byte) (int, error) {
	f, err := os.OpenFile(sinkPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}

// resetDurableSinkLocked resets all durable-sink state and sets the sink directory override to dir.
// Callers must hold sinkMu.
// It zeroes header and headerOnce, so any header field set before the call is lost.
func resetDurableSinkLocked(dir string) {
	sinkDirOverride = dir
	sinkArmed = false
	sinkPath = ""
	sinkDir = ""
	sinkAnchorPath = ""
	sinkOK = false
	header = sinkHeader{}
	headerOnce = sync.Once{}
	sinkBytesWritten = 0
	sinkTruncated = false
}

// SetDurableSinkDir sets the durable sink directory to dir with no worktree root supplied, resetting all sink state.
// The directory takes effect only if it is set before the first record arms the sink.
// The directory is read once per sink generation, and this call starts a new generation.
func SetDurableSinkDir(dir string) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	resetDurableSinkLocked(dir)
}

// SetDurableSinkDirWithWorktreeRoot sets the durable sink directory to dir and the trace header's
// worktree root to worktreeRoot, resetting all other sink state, in one atomic call.
// worktreeRoot is trace metadata naming the repository the process worked on,
// and it does not decide where the trace file lands.
// The directory takes effect only if it is set before the first record arms the sink.
// The directory is read once per sink generation, and this call starts a new generation.
// The root is set after the reset, because the reset zeroes header and would wipe a root set before it.
func SetDurableSinkDirWithWorktreeRoot(dir, worktreeRoot string) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	resetDurableSinkLocked(dir)
	header.WorktreeRoot = worktreeRoot
}

// NotifyExit opens the durable sink when the process exits with a non-zero code.
func NotifyExit(code int) {
	if code == 0 {
		return
	}
	ensureDurableSink()
}
