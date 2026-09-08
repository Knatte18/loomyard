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

// armHeader captures the static header fields once per sink generation.
// Callers hold sinkMu: headerOnce is REASSIGNED by resetDurableSinkLocked, so reading and running it
// outside that mutex would be a data race on the sync.Once value itself, not merely on the fields it
// guards.
func armHeader() {
	headerOnce.Do(func() {
		header.Command = os.Args[0]
		header.Argv = append([]string(nil), os.Args[1:]...)
		header.TraceID = TraceID()
		header.PID = os.Getpid()
	})
}

// Arm captures the durable sink's static header fields ahead of the first log call.
// It takes sinkMu for the same reason ensureDurableSink does: every reader and writer of headerOnce
// and header must agree on one mutex, and this is the one exported entry point that would otherwise
// touch them unguarded.
func Arm() {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	armHeader()
}

// sinkArmed reports whether the lazy first-open below has already run for the CURRENT sink
// generation, and is a plain bool guarded by sinkMu rather than a sync.Once for one reason: the sink
// is re-armable. resetDurableSinkLocked reassigns the generation whenever a caller redirects the
// sink directory, and a sync.Once that is reassigned under sinkMu while ensureDurableSink reads and
// runs it under no lock at all is a data race on the Once value itself -- with the losing order
// leaving the PRE-redirect sinkPath installed (R4 review finding R4-20). That redirect is the
// mechanism keeping a standalone run's trace files out of the operator's own repository, so the race
// is worth closing even though both standalone CLIs redirect single-threaded in their cobra pre-run
// today.
var sinkArmed bool
var sinkPath string
var sinkOK bool
var sinkDirOverride string
var sinkMu sync.Mutex
var sinkBytesWritten int64
var sinkTruncated bool

// ensureDurableSink lazily resolves and arms the durable trace-file sink, at most once per sink
// generation, under sinkMu -- the same mutex writeDurable, SetOutput, and every SetDurableSinkDir*
// caller take, so the arm can never interleave with a redirect.
// It never keeps a file handle open between calls: the trace file is opened, header-written, and
// closed again here, and every subsequent record goes through writeDurable's own open-append-close
// under sinkMu — this is what lets the sink survive its directory being renamed mid-process, since
// no descriptor is ever held pinned to the old location.
// It must never be reached from anything already holding sinkMu: sinkMu is a plain, non-reentrant
// Mutex, and durableWriter.Write's sequential ensureDurableSink-then-writeDurable pair is the only
// shape that is safe.
func ensureDurableSink() bool {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	if !sinkArmed {
		sinkArmed = true
		sinkOK = armDurableSinkLocked()
	}
	return sinkOK
}

// armDurableSinkLocked performs the one-time resolve-open-header-write ensureDurableSink guards,
// reporting whether the sink came up. Callers hold sinkMu.
// It is split out from ensureDurableSink so every failure path is a plain `return false` at one
// level of nesting rather than a sinkOK assignment inside a closure -- the shape that made the
// unlocked writes easy to miss in the first place.
func armDurableSinkLocked() bool {
	dir := sinkDirOverride
	if dir == "" {
		if testing.Testing() && os.Getenv("LYX_TRACE") != "1" {
			return false
		}
	}

	armHeader()

	if dir == "" {
		cwd, err := lyxcwd.Getwd()
		if err != nil {
			return false
		}
		layout, err := lyxcwd.Resolve(cwd)
		if err != nil {
			return false
		}
		if !isLyxWorktree(layout) {
			return false
		}
		dir = LogsDir(layout)
		// header.WorktreeRoot records the worktree root as trace metadata, a
		// separate concern from where the trace file itself lands (LogsDir is
		// AnchorPath-anchored), so the two lines below disagree by design.
		header.WorktreeRoot = layout.WorktreePath()
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
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
		return false
	}

	line := headerLine()
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return false
	}
	_ = f.Close()

	sinkPath = path
	sinkBytesWritten = int64(len(line))
	return true
}

// isLyxWorktree reports whether layout names a worktree lyx actually owns, by the presence of the
// durable _lyx tree at its anchor.
//
// It gates the cwd-anchored fallback above because lyxcwd.Resolve succeeds for ANY plain git
// repository standing at its root — resolveCore defaults AnchorRel to "." when no hub records one, so
// no hub is required — and the fallback then creates <repo>/.lyx/logs/trace-*.log inside a checkout
// lyx does not own. cmd/lyx's logger.NotifyExit(code) force-arms the sink on EVERY non-zero exit, so
// every refusal reached that fallback: a standalone webster or burler invocation refused before
// wireStandalone's own redirect could point the sink at the derived state directory, and an unknown
// subcommand that never reached wiring at all. Standalone mode's whole premise is that nothing lyx
// writes lands in the repository it drives, and this was the one path that did (crucible round
// opus-medium-r6, R6-6).
//
// The residual, stated rather than papered over: a command run inside a git worktree that is not yet
// lyx-wired now writes no trace file at all. That is the intended trade — it is precisely the case
// where writing one would be the defect — and it does not reach fabric's own bring-up verbs, which
// run from the hub, where lyxcwd.Resolve already fails and the fallback was never armed.
func isLyxWorktree(layout *lyxcwd.Location) bool {
	_, err := os.Stat(filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName))
	return err == nil
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

// resetDurableSinkLocked resets all durable-sink state and sets the sink directory override to
// dir. Callers must hold sinkMu.
// It zeroes header and headerOnce as part of the reset, which is why
// SetDurableSinkDirWithWorktreeRoot must set header.WorktreeRoot only after calling this, never
// before -- a worktree root set beforehand would be silently wiped here.
func resetDurableSinkLocked(dir string) {
	sinkDirOverride = dir
	sinkArmed = false
	sinkPath = ""
	sinkOK = false
	header = sinkHeader{}
	headerOnce = sync.Once{}
	sinkBytesWritten = 0
	sinkTruncated = false
}

// SetDurableSinkDir sets the durable sink directory to dir with no worktree root supplied,
// resetting all sink state.
// It is the documented shorthand for "this directory, no worktree root supplied", used by tests
// and by production alike; a caller that has a worktree root to supply should call
// SetDurableSinkDirWithWorktreeRoot instead.
// The directory only takes effect if it is set before the first record that arms the sink,
// because ensureDurableSink reads sinkDirOverride exactly once per sink generation and this call
// starts a fresh generation rather than moving an already-opened file.
func SetDurableSinkDir(dir string) {
	sinkMu.Lock()
	defer sinkMu.Unlock()

	resetDurableSinkLocked(dir)
}

// SetDurableSinkDirWithWorktreeRoot sets the durable sink directory to dir and the trace header's
// worktree root to worktreeRoot, resetting all other sink state, in one atomic call.
// worktreeRoot is trace metadata answering "which repository was this process working on", a
// separate concern from where the trace file lands -- the same distinction the comment beside
// header.WorktreeRoot = layout.WorktreePath() in ensureDurableSink already draws.
// The directory only takes effect if it is set before the first record that arms the sink,
// because ensureDurableSink reads sinkDirOverride exactly once per sink generation and this call
// starts a fresh generation rather than moving an already-opened file.
// The reset happens before the worktree root is set, never after: resetDurableSinkLocked zeroes
// header as part of its reset, so setting worktreeRoot first would be silently wiped. Setting it
// after the reset is safe because armHeader populates Command, Argv, TraceID, and PID only and
// never writes WorktreeRoot.
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
