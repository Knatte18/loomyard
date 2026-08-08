// logger.go provides a thin log/slog wrapper for lyx: a package-level level threshold, an
// injectable io.Writer stderr sink (defaulting to os.Stderr), and Debug/Info/Warn helpers that fan
// every call out to that stderr sink and, independently, to the durable trace-file sink (sink.go)
// via the composite dualHandler defined below.
// Callers never see slog directly;
// they call SetVerbosity to raise the stderr threshold and Debug/Info/Warn to emit.

// Package logger is a minimal log/slog wrapper shared across lyx's internal
// packages, extended with a process-wide trace identity, explicit-parent
// diagnostic spans, and a durable per-process trace-file sink. It keeps
// stdout free of log noise (reed and other commands write their JSON
// envelope to stdout via internal/output) by routing all log output to a
// dedicated stderr sink, which defaults to os.Stderr and is silent unless
// the caller opts in via SetVerbosity. Every Debug/Info/Warn call also fans
// out to the durable trace-file sink (sink.go), independently gated at Info
// and above regardless of the stderr threshold -- see dualHandler's doc
// comment for the composite's exact semantics.
//
// # Trace identity and spans
//
// Every process has exactly one trace identity: a 16-lowercase-hex-character
// ID, minted or adopted once and stamped as trace=<id> on every emitted log
// line regardless of level. TraceID() (trace.go) is the lazy accessor any
// code path can call before cmd/lyx's root hook has run; it resolves on
// first use via a fixed precedence -- a value already set by
// MintOrAdoptAndExport, then an inherited LYX_TRACE_ID (see below), then a
// fresh mint. MintOrAdoptAndExport is the root hook's own explicit call: it
// resolves the same value and additionally exports it into the process
// environment as LYX_TRACE_ID so every spawned child inherits it at any
// spawn site that does not override cmd.Env, giving a whole process tree the
// same trace ID.
//
// A Span (span.go) is a plain value carrying a dotted path built up via
// StartSpan (root) and Child (append a segment), with its own
// Debug/Info/Warn methods that additionally stamp span=<path> alongside
// trace=. There is no ambient "current span" global -- a caller always
// holds and threads its own *Span explicitly. A span's open (StartSpan/
// Child) and successful close (End(nil)) log at Debug; a close with a
// non-nil error logs at Warn, carrying the error as a field, so a failing
// span's close is the one open/close record that reaches the durable sink
// even on an otherwise-quiet run.
//
// # The durable sink and its retention
//
// Alongside the stderr sink above, every Info+ record also lands in a
// second, durable sink (sink.go): one plain-text trace file per process,
// under the current worktree's own LogsDir(l) (sink.go)
// (<AnchorPath>/.lyx/logs/), this package's own declaration of that path. The file opens lazily on whichever of two triggers fires
// first: (a) the first Info-or-above log record in the process, or (b) the
// process exiting with a non-zero code, via NotifyExit -- so a run that logs
// nothing above Debug but still fails leaves a reconstructable trace file
// behind. The file's first line is a header record naming the command,
// argv, trace ID, PID, and worktree root. Each file is capped at 8 MiB;
// once a write would cross the cap, a single truncation-marker line is
// written and further writes to that file become silent no-ops rather than
// growing it further.
//
// A background-free sweep (retention.go's Sweep, run once per sink open)
// keeps the logs directory bounded: files older than 14 days are deleted,
// then only the newest 50 (by filename timestamp) of what remains are kept.
// A file whose PID belongs to a currently-running process (including the
// sweeping process itself) is never deleted and never counts toward the
// newest-50 bound, regardless of age.
//
// # Activation outside the lyx CLI
//
// SetVerbosity is wired to the lyx CLI's -v/-vv flag in cmd/lyx/main.go, but
// nothing reaches that flag parsing when this package is used from a `go
// test` binary or any other entry point that never runs cmd/lyx/main — which
// is exactly the case for the smoke/cluster tests that drive real reed/
// shuttle/burler substrate. For those callers, init reads two environment
// variables so verbosity and the output sink can be set from OUTSIDE the
// process, with no code change and no dependency on CLI flag plumbing:
//
//   - LYX_LOG_LEVEL: "debug", "info", or "warn" (case-insensitive). Sets the
//     initial threshold exactly like SetVerbosity(2)/(1)/(0) would. Unset or
//     unrecognised leaves the default Warn threshold untouched.
//   - LYX_LOG_FILE: a file path. If set, log output is appended to that file
//     instead of os.Stderr (created if absent). This exists because a live
//     substrate test's own stdout/stderr redirection is often already
//     spoken for by the test runner, and because a stderr-only sink is easy
//     to lose track of across many concurrent test processes; a fixed file
//     path any operator or orchestrator can point at removes that ambiguity.
//     A failure to open the file falls back to os.Stderr with a one-line
//     diagnostic (logger cannot log its own bootstrap failure through
//     itself).
//
// Both are read once at package init; a caller's own explicit SetVerbosity/
// SetOutput call after that always wins (this is exactly how the existing
// -v/-vv CLI flag continues to take precedence over any env var left set in
// the shell).
//
// Two further environment variables govern trace identity and the durable
// sink specifically, independent of LYX_LOG_LEVEL/LYX_LOG_FILE above:
//
//   - LYX_TRACE_ID: an inherited trace identity. When set to a
//     non-whitespace-only value, both TraceID()'s lazy resolution and
//     MintOrAdoptAndExport adopt it instead of minting a fresh one, so a
//     child process continues its parent's trace rather than starting a new
//     one. cmd/lyx's root hook is what exports this variable for children in
//     the first place (see "Trace identity and spans" above); nothing else
//     in this package ever calls os.Setenv.
//   - LYX_TRACE=1: the test-entry-activation gate for the durable sink only.
//     Under `go test` (testing.Testing() true), the durable sink stays
//     closed for the whole process unless LYX_TRACE=1 is explicitly set,
//     regardless of how many Info+ records are logged or how the process
//     exits -- this is what keeps ordinary `go test` runs from littering a
//     worktree's .lyx/logs with test-process trace files. It has no effect
//     on trace-ID resolution itself: TraceID() and the trace=/span= fields
//     stamped on stderr output are computed the same way whether or not
//     LYX_TRACE is set.
//
// Note that an Info+ record reaching both LYX_LOG_FILE (if set) and the
// durable trace file is correct behavior, not a bug to "fix" by suppressing
// one or the other: the two are different artifacts with different
// lifetimes and different audiences (an operator-chosen, unmanaged,
// whole-verbosity file vs. a lyx-managed, Info+-only, retention-swept one),
// and neither should suppress the other.
//
// # Level policy
//
// Every call site in this codebase that logs through this package follows
// one policy for choosing a level:
//
//   - Warn: a notable-but-recoverable failure -- a retry, an unconfirmed
//     teardown, an error swallowed on a fallback path.
//   - Info: a real OS-process spawn/teardown lifecycle event.
//   - Debug: everything else worth a line.
//
// Hard rule: nothing logs at Warn inside a loop body that can iterate more
// than roughly 10 times without an intervening state change -- a Warn inside
// such a loop turns one notable condition into a flood, both on stderr at
// -v/-vv and, since Warn also reaches the durable sink, in the trace file.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// levelVar is the package-level threshold controlling stderr verbosity.
var levelVar slog.LevelVar

// out is the stderr output destination, defaulting to os.Stderr.
var out io.Writer = os.Stderr

// currentStderr is the stderr-half inner handler, rebuilt by SetOutput to
// respond to output redirections.
var currentStderr slog.Handler = slog.NewTextHandler(out, &slog.HandlerOptions{Level: &levelVar})

// stderrHandlerSnapshot returns the current stderr-half handler under a
// narrow critical section; caller must not hold sinkMu across the following
// Handle call to avoid deadlock.
func stderrHandlerSnapshot() slog.Handler {
	sinkMu.Lock()
	h := currentStderr
	sinkMu.Unlock()
	return h
}

// log is the slog.Logger built over the composite dualHandler (see
// dual_handler in this file), which fans every record out to the stderr
// half (gated by levelVar, exactly as before) and the durable-sink half
// (batch 4's ensureDurableSink/writeDurable, gated at Info+ unconditionally
// of levelVar). log itself is never reassigned by SetOutput; only
// currentStderr, which dualHandler reads dynamically, changes.
var log = slog.New(newDualHandler())

// dualHandler is the composite slog.Handler discussion.md's
// dual-handler-fan-out decision requires: Enabled is the OR of the stderr
// and durable halves' own Enabled, so an Info record is accepted by the
// composite even when the stderr half alone would reject it at the default
// Warn threshold (this is what lets an Info record reach the durable sink
// regardless of -v/-vv). Handle independently re-checks each half's own
// Enabled before delegating to it -- it never forwards to one half just
// because the composite's own Enabled, or the other half's Enabled, already
// returned true. Getting that independent re-check wrong would leak an Info
// record to stderr at the default Warn threshold, even though stderr's own
// Enabled(Info) is false there.
type dualHandler struct {
	// transform accumulates the WithAttrs/WithGroup chain applied to a
	// logger derived via slog.Logger.With/WithGroup, applied in order to a
	// freshly read stderr snapshot at Enabled/Handle time rather than baked
	// into a stored handler value -- this is what lets a rebind via
	// SetOutput remain visible even to a dualHandler value produced by an
	// earlier .With() call, since Card 19 keeps the package-level log var,
	// and therefore any handler value derived from it earlier in the same
	// call chain, unchanged across the rebind.
	transform func(slog.Handler) slog.Handler
	durable   durableHandler
}

// newDualHandler builds the root dualHandler with no accumulated
// WithAttrs/WithGroup transform.
func newDualHandler() dualHandler {
	return dualHandler{transform: func(h slog.Handler) slog.Handler { return h }, durable: newDurableHandler()}
}

// stderr resolves this dualHandler's stderr-half handler by reading the
// current package-level inner handler and applying this handler's
// accumulated WithAttrs/WithGroup transform on top of it.
func (d dualHandler) stderr() slog.Handler {
	return d.transform(stderrHandlerSnapshot())
}

// Enabled implements slog.Handler as the OR of the stderr and durable halves' own Enabled -- see
// the dualHandler doc comment for why this must be OR, not stderr's gate alone.
func (d dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.stderr().Enabled(ctx, level) || d.durable.Enabled(ctx, level)
}

// Handle implements slog.Handler by independently re-checking each half's own Enabled and
// delegating only to the halves that pass -- see the dualHandler doc comment for why this
// independent re-check, rather than a single shared gate, is load-bearing.
// record.Clone() is used for all but the last delegate because slog.Record documents that a Record
// passed to more than one Handler must be cloned for each use beyond the first, since its internal
// small-attrs storage may otherwise be shared.
func (d dualHandler) Handle(ctx context.Context, record slog.Record) error {
	stderr := d.stderr()
	stderrEnabled := stderr.Enabled(ctx, record.Level)
	durableEnabled := d.durable.Enabled(ctx, record.Level)

	var firstErr error
	if stderrEnabled {
		if err := stderr.Handle(ctx, record.Clone()); err != nil {
			firstErr = err
		}
	}
	if durableEnabled {
		if err := d.durable.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WithAttrs implements slog.Handler by extending both halves' accumulated transform with the same
// attrs, so a logger built via slog.Logger.With keeps fanning out to both halves rather than
// collapsing to one.
func (d dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	prevTransform := d.transform
	return dualHandler{
		transform: func(h slog.Handler) slog.Handler { return prevTransform(h).WithAttrs(attrs) },
		durable:   d.durable.withAttrs(attrs),
	}
}

// WithGroup implements slog.Handler by extending both halves' accumulated transform with the same
// group, mirroring WithAttrs.
func (d dualHandler) WithGroup(name string) slog.Handler {
	prevTransform := d.transform
	return dualHandler{
		transform: func(h slog.Handler) slog.Handler { return prevTransform(h).WithGroup(name) },
		durable:   d.durable.withGroup(name),
	}
}

// durableWriter adapts the durable sink's ensureDurableSink/writeDurable
// pair (sink.go) to an io.Writer so durableHandler can format records with
// an ordinary slog.NewTextHandler -- "formats the record as
// slog.NewTextHandler would" per the batch plan -- while the actual bytes
// still flow through the sink's lazy-open and size-cap machinery.
type durableWriter struct{}

// Write opens the durable sink on first use (a no-op on every call after the first, via sinkOnce)
// and writes p to it.
// A closed/unarmed sink (sinkOK false) is not an error here -- per sink.go's lazy-sink-open rule, a
// diagnostic sink that never opens must never surface as a write failure to the logging call site,
// so Write reports success and discards p.
func (durableWriter) Write(p []byte) (int, error) {
	if _, ok := ensureDurableSink(); !ok {
		return len(p), nil
	}
	return writeDurable(p)
}

// durableHandler is dualHandler's durable-sink half. Its Enabled is
// unconditional at Info and above -- never gated by levelVar -- which is
// the property that lets an Info record reach the durable sink at the
// default Warn verbosity even though the stderr half alone would reject it
// there.
type durableHandler struct {
	inner slog.Handler
}

// newDurableHandler builds the durable half over durableWriter, formatting
// exactly as slog.NewTextHandler would.
func newDurableHandler() durableHandler {
	return durableHandler{inner: slog.NewTextHandler(durableWriter{}, &slog.HandlerOptions{Level: slog.LevelInfo})}
}

// Enabled reports whether level is Info or above, unconditionally of levelVar -- see the
// durableHandler doc comment.
func (d durableHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

// Handle formats and writes record via the wrapped text handler, which in turn writes through
// durableWriter into the durable sink.
func (d durableHandler) Handle(ctx context.Context, record slog.Record) error {
	return d.inner.Handle(ctx, record)
}

// withAttrs returns a copy of d with attrs bound into the wrapped handler.
// Kept unexported and typed (durableHandler, not slog.Handler) so
// dualHandler.WithAttrs can store it back into its durable field without a
// type assertion.
func (d durableHandler) withAttrs(attrs []slog.Attr) durableHandler {
	return durableHandler{inner: d.inner.WithAttrs(attrs)}
}

// withGroup returns a copy of d with name bound into the wrapped handler,
// mirroring withAttrs.
func (d durableHandler) withGroup(name string) durableHandler {
	return durableHandler{inner: d.inner.WithGroup(name)}
}

func init() {
	levelVar.Set(slog.LevelWarn)
	configureFromEnv()
}

// configureFromEnv applies LYX_LOG_LEVEL and LYX_LOG_FILE, if set, on top of
// the just-initialised Warn/stderr defaults. Split out from init as its own
// function purely for test seams. See the package doc for the two
// variables' exact semantics.
func configureFromEnv() {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LYX_LOG_LEVEL"))) {
	case "debug":
		levelVar.Set(slog.LevelDebug)
	case "info":
		levelVar.Set(slog.LevelInfo)
	case "warn", "":
		// Warn is already the default set above; "" means unset.
	}

	if path := strings.TrimSpace(os.Getenv("LYX_LOG_FILE")); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logger: LYX_LOG_FILE=%q: %v (falling back to stderr)\n", path, err)
			return
		}
		SetOutput(f)
	}
}

// Debug logs msg at debug level with the given key/value args, stamping a trace key with
// TraceID()'s current value (batch 2) on every line as every level does.
// Debug never reaches the durable sink -- durableHandler.Enabled only accepts Info and above -- so
// a Debug record is a no-op sink-wise regardless of trace stamping;
// it reaches the stderr half only once SetVerbosity(2) or higher has been called.
// Calling TraceID() here is also what triggers the trace-ID's own first resolution if nothing has
// resolved it yet.
func Debug(msg string, args ...any) {
	log.With("trace", TraceID()).Debug(msg, args...)
}

// Info logs msg at info level with the given key/value args, stamping trace= as Debug does.
// It reaches the durable sink unconditionally (durableHandler.Enabled never consults levelVar),
// and reaches the stderr half only once SetVerbosity(1) or higher has been called.
func Info(msg string, args ...any) {
	log.With("trace", TraceID()).Info(msg, args...)
}

// Warn logs msg at warn level with the given key/value args, stamping trace= as Debug does.
// Warn is the default threshold, so a Warn call reaches both halves -- stderr and the durable sink
// -- even without SetVerbosity.
func Warn(msg string, args ...any) {
	log.With("trace", TraceID()).Warn(msg, args...)
}

// SetVerbosity maps a -v repeat count to a log level: count<=0 keeps the default Warn threshold
// (silent normal run), count==1 lowers it to Info, and count>=2 lowers it to Debug.
// cmd/lyx/main.go calls this once at startup from the root -v/--verbose flag.
func SetVerbosity(count int) {
	switch {
	case count <= 0:
		levelVar.Set(slog.LevelWarn)
	case count == 1:
		levelVar.Set(slog.LevelInfo)
	default:
		levelVar.Set(slog.LevelDebug)
	}
}

// SetOutput rebinds the stderr half of the log sink to w and rebuilds the composite handler's
// stderr-half inner handler in place;
// the durable half (sink.go's ensureDurableSink/writeDurable) is untouched, per discussion.md's
// dual-handler-fan-out decision -- redirecting stderr must never silently detach the durable sink.
// It is a seam both tests (withCapturedOutput in logger_test.go, so tests can assert on captured
// output without writing to the real os.Stderr) and production code use: configureFromEnv already
// calls SetOutput for LYX_LOG_FILE, independent of anything this package's trace/sink work adds.
// LYX_LOG_FILE redirects only the stderr half to an operator-chosen, unmanaged, whole-verbosity
// file;
// the durable sink independently opens its own Info+-only, lyx-managed, retention-swept trace file
// under this package's own LogsDir(l).
// An Info+ line therefore lands in both files when LYX_LOG_FILE is set -- that duplication is
// intended, not a bug to reconcile: the two are different artifacts with different lifetimes, and
// neither should suppress the other.
//
// The write to out and the rebuild of currentStderr happen under sinkMu (sink.go), the same mutex
// guarding the durable sink's state, per discussion.md's concurrency-contract decision: SetOutput
// can race with an in-flight Handle call reading currentStderr via stderrHandlerSnapshot.
func SetOutput(w io.Writer) {
	sinkMu.Lock()
	out = w
	currentStderr = slog.NewTextHandler(w, &slog.HandlerOptions{Level: &levelVar})
	sinkMu.Unlock()
}
