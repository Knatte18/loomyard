// sink.go implements the durable second sink: a per-process trace file
// opened lazily on one of two triggers (discussion.md's `sink-open-triggers`
// decision) — the first Info-or-above record, or the process exiting with a
// non-zero code — and written to under a size cap with a single truncation
// marker once that cap is crossed. Unlike the stderr sink logger.go already
// provides, this file is never the default `Debug`/`Info`/`Warn` output
// path itself; batch 5 wires those helpers to fan out to it via
// ensureDurableSink once this file's open logic has run.

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

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// sinkMaxBytes is the durable sink's size cap: once a file's running byte
// count would cross this many bytes, writeDurable stops accepting further
// writes and appends a single truncation-marker line instead, per
// discussion.md's `retention` decision.
const sinkMaxBytes = 8 * 1024 * 1024

// sinkHeader is the two-phase header record discussion.md's
// `sink-open-triggers` decision describes: the Command/Argv/TraceID/PID
// fields are captured once, statically, by armHeader (Card 9); WorktreeRoot
// is filled in later, only at ensureDurableSink's open (Card 10), since
// geometry resolution is not something armHeader itself performs.
type sinkHeader struct {
	Command      string
	Argv         []string
	TraceID      string
	PID          int
	WorktreeRoot string
}

// headerOnce guards the static-field capture performed by armHeader. It is
// distinct from traceOnce (trace.go) and sinkOnce (below) per the
// concurrency-contract decision: header composition and its two consumers
// (Arm's explicit call, ensureDurableSink's fallback call) must not be
// conflated with either the trace-ID's own once-guard or the sink-open
// once-guard.
var headerOnce sync.Once

// header holds the process's durable-sink header record once headerOnce has
// fired. Its WorktreeRoot field remains zero-value until ensureDurableSink
// resolves geometry (or is left permanently zero-value on the test-seam
// path, see SetDurableSinkDir).
var header sinkHeader

// armHeader captures the header's static fields — Command, Argv (a copy,
// never an alias of os.Args), TraceID, and PID — inside headerOnce.Do, so
// it is safe to call multiple times or from multiple call sites (Arm and
// ensureDurableSink's fallback) with only the first call taking effect.
func armHeader() {
	headerOnce.Do(func() {
		header.Command = os.Args[0]
		header.Argv = append([]string(nil), os.Args[1:]...)
		header.TraceID = TraceID()
		header.PID = os.Getpid()
	})
}

// Arm captures the durable sink's static header fields ahead of the first
// log call. cmd/lyx's root command hook calls this (batch 7) so the header
// reflects the process's real argv even if the first Info+/Warn record is
// emitted from deep inside a subcommand after flag parsing has already
// mutated os.Args-derived state elsewhere. It is a thin wrapper over the
// unexported armHeader so ensureDurableSink's own fallback call reaches the
// identical, idempotent capture logic — header composition is unaffected by
// which path arms the sink, per discussion.md's `no-arming-under-test`
// decision.
func Arm() {
	armHeader()
}

// sinkOnce guards ensureDurableSink's open logic. It is distinct from
// traceOnce (trace.go) per the concurrency-contract decision: traceOnce
// must fire on any emit, including a Debug call that never reaches the
// durable sink, while sinkOnce only fires when one of the two open triggers
// (an Info+/Warn record, or NotifyExit's non-zero code) actually occurs.
var sinkOnce sync.Once

// sinkWriter is the opened durable-sink file, valid once sinkOK is true.
var sinkWriter io.Writer

// sinkOK reports whether ensureDurableSink's open succeeded. It stays false
// for the lifetime of the process when the LYX_TRACE=1 test-entry-activation
// gate is closed, or when geometry/filesystem resolution fails — per
// `lazy-sink-open`'s "failure to open a diagnostic sink must never break the
// operation being diagnosed" rule, a false sinkOK is never itself an error
// condition callers need to handle beyond skipping the durable write.
var sinkOK bool

// sinkDirOverride, when non-empty, makes ensureDurableSink use this
// directory directly instead of resolving geometry via hubgeometry.Resolve.
// Set only by the test seam SetDurableSinkDir.
var sinkDirOverride string

// sinkMu guards, as one critical section, the durable sink's running byte
// counter, the size-cap check, the truncation-marker flag, and the actual
// write — see writeDurable and the concurrency-contract decision this
// implements: an atomic counter alone would let two goroutines both observe
// the cap crossing and each emit a marker.
var sinkMu sync.Mutex

// sinkBytesWritten is the running count of bytes written to sinkWriter so
// far, guarded by sinkMu.
var sinkBytesWritten int64

// sinkTruncated reports whether the size-cap truncation marker has already
// been written to the current sink file, guarded by sinkMu. Once true,
// writeDurable stops accepting further writes to this file.
var sinkTruncated bool

// ensureDurableSink lazily opens the durable trace-file sink, at most once
// per process, and returns the opened writer plus whether the open
// succeeded.
//
// Activation: when SetDurableSinkDir has set a non-empty override
// directory, that call itself is treated as the opt-in signal and the sink
// opens directly against that directory, regardless of testing.Testing().
// Only when no override directory is set does this function fall through
// to the LYX_TRACE=1 test-entry-activation gate: if testing.Testing() is
// true and LYX_TRACE is not "1", ensureDurableSink performs no filesystem
// or geometry access at all and returns sinkOK == false. This order is
// deliberate and load-bearing — checking testing.Testing() first would
// defeat every unit test's SetDurableSinkDir(t.TempDir()) seam, since
// testing.Testing() is always true under `go test` and those tests never
// set LYX_TRACE=1.
//
// LYX_TRACE=1 is read once per process, inside the same sinkOnce.Do call
// that performs the open, matching the outside-the-process activation
// model LYX_LOG_LEVEL/LYX_LOG_FILE already use (see logger.go's package
// doc, "Activation outside the lyx CLI"). This env var is independent of
// testing.Testing() on the trace-ID/mint side: TraceID() always lazily
// resolves regardless of LYX_TRACE. LYX_TRACE=1 gates only whether the
// durable sink opens, not whether trace=/span= fields are computed and
// stamped on stderr output.
//
// This function is called by batch 5's dual-handler on every Info+/Warn
// record (open trigger (a)) and by NotifyExit (open trigger (b)) — both
// call sites go through the same sinkOnce.Do, so the open logic runs at
// most once per process regardless of which trigger fires first.
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
			cwd, err := hubgeometry.Getwd()
			if err != nil {
				sinkOK = false
				return
			}
			layout, err := hubgeometry.Resolve(cwd)
			if err != nil {
				sinkOK = false
				return
			}
			dir = layout.WorktreeLogsDir()
			header.WorktreeRoot = layout.WorktreeRoot
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			sinkOK = false
			return
		}

		// A sweep failure does not abort the open; Sweep already tolerates
		// per-file failures internally and logs nothing about its own
		// outcome.
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

// writeDurable writes p to the durable sink under sinkMu, implementing the
// size cap and single truncation marker as one atomic critical section with
// the byte counter and marker flag: an atomic counter alone would let two
// goroutines both observe the cap crossing and each emit a marker.
//
// Once the running byte count plus len(p) would cross sinkMaxBytes, and the
// truncation marker has not yet been written, writeDurable writes one
// terminal record noting the truncation, sets the marker-written flag, and
// stops accepting further writes to this file — subsequent calls become
// no-ops that report success so callers never see a write error from a
// capped sink.
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

// SetDurableSinkDir points ensureDurableSink at dir directly instead of
// having it resolve geometry via hubgeometry.Resolve, following SetOutput's
// existing precedent (logger.go) as the model for a seam that lets tests
// point at a controlled location without going through production
// resolution. Passing a non-empty dir leaves header.WorktreeRoot empty
// (no geometry ran), which is the seam-path behavior the sink tests and
// this package's header documentation both depend on.
//
// Every call to SetDurableSinkDir — including with a non-empty dir, not
// only SetDurableSinkDir("") — is also the full sink-state reset point
// tests must use between cases: sinkOnce, sinkWriter, sinkOK, header,
// headerOnce, the byte counter, and the truncation-marker flag are all
// package-level state that otherwise persists for the lifetime of the test
// binary, so without this reset only the first test in the whole package
// that ever triggers ensureDurableSink would actually open a file.
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

// NotifyExit implements discussion.md's `sink-open-triggers` trigger (b):
// the process exiting with a non-zero exit code. A code of 0 is a no-op.
// A non-zero code forces ensureDurableSink's sinkOnce-guarded open (and
// header write) to run even if no Info+/Warn record was ever emitted, so a
// run that fails having logged nothing above Debug still leaves a
// reconstructable trace file. cmd/lyx's run()/main() calls this (batch 7)
// right after capturing clihelp.RunRoot's exit code and before returning it
// or calling os.Exit.
func NotifyExit(code int) {
	if code == 0 {
		return
	}
	ensureDurableSink()
}
