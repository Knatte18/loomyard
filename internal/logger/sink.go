// sink.go implements the durable second sink: a per-process trace file opened lazily on one of two
// triggers (discussion.md's `sink-open-triggers` decision) — the first Info-or-above record,
// or the process exiting with a non-zero code — and written to under a size cap with a single
// truncation marker once that cap is crossed.
// Unlike the stderr sink logger.go already provides, this file is never the default
// `Debug`/`Info`/`Warn` output path itself;
// batch 5 wires those helpers to fan out to it via ensureDurableSink once this file's open logic
// has run.

package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// dotLyxDirName is the directory name for ephemeral, machine-bound lyx state,
// this package's own declaration of the token for WorktreeLogsDir's join.
const dotLyxDirName = ".lyx"

const sinkMaxBytes = 8 * 1024 * 1024

// WorktreeLogsDir returns the path to the worktree-level directory where this package's durable
// trace sink writes one file per process.
// It is WorktreePath-anchored so a caller invoked from anywhere in the worktree resolves the same
// logs directory.
// It lives under the ephemeral .lyx directory, never the durable, weft-synced _lyx.
func WorktreeLogsDir(l *lyxcwd.Location) string {
	return filepath.Join(l.WorktreePath(), dotLyxDirName, "logs")
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

func armHeader() {
	headerOnce.Do(func() {
		header.Command = os.Args[0]
		header.Argv = append([]string(nil), os.Args[1:]...)
		header.TraceID = TraceID()
		header.PID = os.Getpid()
	})
}

// Arm captures the durable sink's static header fields ahead of the first log call.
func Arm() {
	armHeader()
}

var sinkOnce sync.Once
var sinkWriter io.Writer
var sinkOK bool
var sinkDirOverride string
var sinkMu sync.Mutex
var sinkBytesWritten int64
var sinkTruncated bool

// ensureDurableSink lazily opens the durable trace-file sink, at most once per process.
func ensureDurableSink() (io.Writer, bool) {
	sinkOnce.Do(func() {
		dir := sinkDirOverride
		if dir == "" {
			if testing.Testing() && os.Getenv("LYX_TRACE") != "1" {
				sinkOK = false
				return
			}
		}

		armHeader()

		if dir == "" {
			cwd, err := lyxcwd.Getwd()
			if err != nil {
				sinkOK = false
				return
			}
			layout, err := lyxcwd.Resolve(cwd)
			if err != nil {
				sinkOK = false
				return
			}
			dir = WorktreeLogsDir(layout)
			header.WorktreeRoot = layout.WorktreePath()
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			sinkOK = false
			return
		}

		_ = Sweep(dir)

		filename := fmt.Sprintf("trace-%s-%s-%d.log",
			time.Now().UTC().Format(traceFileTimestampLayout),
			header.TraceID,
			header.PID,
		)

		f, err := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			sinkOK = false
			return
		}

		line := headerLine()
		if _, err := f.WriteString(line); err != nil {
			_ = f.Close()
			sinkOK = false
			return
		}

		sinkWriter = f
		sinkBytesWritten = int64(len(line))
		sinkOK = true
	})
	return sinkWriter, sinkOK
}

// headerLine renders the durable sink's first-line header record as a
// single plain-text line naming every sinkHeader field.
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
func writeDurable(p []byte) (int, error) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	if sinkTruncated {
		return len(p), nil
	}

	if sinkBytesWritten+int64(len(p)) > sinkMaxBytes {
		marker := "trace sink truncated: size cap reached\n"
		_, _ = sinkWriter.Write([]byte(marker))
		sinkTruncated = true
		return len(p), nil
	}

	n, err := sinkWriter.Write(p)
	sinkBytesWritten += int64(n)
	return n, err
}

// SetDurableSinkDir sets the durable sink directory for testing, resetting all sink state.
func SetDurableSinkDir(dir string) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	sinkDirOverride = dir
	sinkOnce = sync.Once{}
	sinkWriter = nil
	sinkOK = false
	header = sinkHeader{}
	headerOnce = sync.Once{}
	sinkBytesWritten = 0
	sinkTruncated = false
}

// NotifyExit opens the durable sink when the process exits with a non-zero code.
func NotifyExit(code int) {
	if code == 0 {
		return
	}
	ensureDurableSink()
}
