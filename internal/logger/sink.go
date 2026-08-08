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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

const sinkMaxBytes = 8 * 1024 * 1024

// LogsDir returns the path to the worktree-level directory where this package's durable trace sink
// writes one file per process.
// It is AnchorPath-anchored so it is a directory sibling of the durable, fabric-synced _lyx tree —
// the old WorktreePath-anchored name is gone because it would assert an anchor this function no
// longer uses.
// It lives under the ephemeral .lyx directory, never the durable, fabric-synced _lyx.
func LogsDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "logs")
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
var sinkPath string
var sinkOK bool
var sinkDirOverride string
var sinkMu sync.Mutex
var sinkBytesWritten int64
var sinkTruncated bool

// ensureDurableSink lazily resolves and arms the durable trace-file sink, at most once per process.
// It never keeps a file handle open between calls: the trace file is opened, header-written, and
// closed again here, and every subsequent record goes through writeDurable's own open-append-close
// under sinkMu — this is what lets the sink survive its directory being renamed mid-process, since
// no descriptor is ever held pinned to the old location.
func ensureDurableSink() bool {
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
			dir = LogsDir(layout)
			// header.WorktreeRoot records the worktree root as trace metadata, a
			// separate concern from where the trace file itself lands (LogsDir is
			// AnchorPath-anchored), so the two lines below disagree by design.
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
		path := filepath.Join(dir, filename)

		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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
		_ = f.Close()

		sinkPath = path
		sinkBytesWritten = int64(len(line))
		sinkOK = true
	})
	return sinkOK
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
// It opens sinkPath with the same open-append-close flags ensureDurableSink used for the header,
// appends, and closes again, all under sinkMu — so the extra open/close pair sits under a lock this
// function already takes, and no descriptor survives the call to be invalidated by a directory
// rename.
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
// Callers hold sinkMu; this is the sole place a durable-sink file descriptor is created after
// ensureDurableSink's initial header write.
func appendToSink(p []byte) (int, error) {
	f, err := os.OpenFile(sinkPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}

// SetDurableSinkDir sets the durable sink directory for testing, resetting all sink state.
func SetDurableSinkDir(dir string) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	sinkDirOverride = dir
	sinkOnce = sync.Once{}
	sinkPath = ""
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
