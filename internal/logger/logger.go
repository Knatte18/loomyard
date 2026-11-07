// logger.go holds the package's stderr threshold and stderr sink, and the Debug, Info, Warn and Error helpers.
// Each helper sends its record to the stderr sink and, independently, to the durable trace-file sink (sink.go).
// Callers never see slog directly; they set the stderr threshold with SetVerbosity.

// Package logger is a minimal log/slog wrapper shared across lym's internal packages.
// It adds a process-wide trace identity, explicitly threaded diagnostic spans, and a durable trace-file sink.
// Stdout stays free of log noise, because commands write their JSON envelope to stdout through internal/output.
// All log output goes to a dedicated stderr sink instead.
// The stderr sink defaults to DefaultOutput at the Warn threshold, which SetVerbosity changes.
// Under `go test`, DefaultOutput is io.Discard, so a test run leaves no logger output on stderr.
// So with -v, a failing test shows no logger lines unless it redirects the sink with SetOutput or logcapture.
// Every Debug, Info and Warn call also goes to the durable sink (sink.go).
// That sink admits Info and above, and Debug at Debug verbosity, whatever the stderr threshold.
//
// # Trace identity and spans
//
// Every process has exactly one trace identity: a 16-character lowercase hex ID.
// It is minted or adopted once, and every log line carries it as trace=<id>, at every level.
// TraceID() (trace.go) resolves it lazily on first use,
// so any code path can call it before cmd/lym's root hook has run.
// Resolution order: a value already set by MintOrAdoptAndExport, then an inherited LYM_TRACE_ID, then a fresh mint.
// MintOrAdoptAndExport is the root hook's explicit call.
// It resolves the same value and exports it into the process environment as LYM_TRACE_ID,
// so every spawned child inherits it at any spawn site that does not override cmd.Env.
// A whole process tree therefore shares one trace ID.
//
// A Span (span.go) is a plain value holding a dotted path that StartSpan starts and Child extends.
// Its Debug, Info and Warn methods also stamp span=<path> next to trace=.
// No global holds a current span; a caller holds its own *Span and passes it explicitly.
// A span's open (StartSpan, Child) and successful close, End(nil), log at Debug.
// End with a non-nil error logs at Warn and carries the error as a field.
// So a failing span's close is the one open or close record that reaches the durable sink on an otherwise quiet run.
//
// # The durable sink and its retention
//
// Every Info-and-above record also lands in the durable sink (sink.go).
// Every Debug record does too when the process verbosity is Debug.
// Each process writes one plain-text trace file to the current worktree's logs directory.
// LogsDir returns that directory: `<AnchorPath>/.lym/logs/`.
// The file opens lazily, on whichever comes first:
// the first record the durable sink admits, or the process exiting with a non-zero code through NotifyExit.
// So a run that logs nothing the durable sink admits, but still fails, leaves a reconstructable trace file behind.
// The file's first line is a header record naming the command, argv, trace ID, PID, and worktree root.
// Each file is capped at 8 MiB.
// Once a write would cross the cap, one truncation-marker line is written.
// Further writes to that file become silent no-ops.
//
// Sweep (retention.go) keeps the logs directory bounded, with no background task.
// A process runs it once at exit, through cmd/lym's exit hook, and only when its sink armed in the current generation.
// Arming alone never deletes a file.
// The bounds come from `_lym/config/logger.yaml`.
// The compiled-in defaults, from DefaultRetentionBounds, apply for a redirected sink or an unusable file.
// Files are grouped by trace ID.
// A group's activity time is the newest mtime among its files,
// so a long-running step that keeps writing stays recent, however long ago its file was named.
// Groups whose activity is older than `MaxAge` are deleted whole.
// Of the groups that remain, only the `Count` most recently active are kept.
// A group with a file whose PID belongs to a running process, the sweeping process included, is never deleted.
// Such a group never counts toward `Count`, whatever its age.
//
// # Activation outside the lym CLI
//
// SetVerbosity is wired to the lym CLI's -v/-vv flag in cmd/lym/main.go.
// A `go test` binary, or any other entry point that never runs cmd/lym/main, never reaches that flag parsing.
// For those callers, init reads two environment variables.
// They set verbosity and the output sink from outside the process, with no code change or flag plumbing:
//
//   - LYM_LOG_LEVEL: "debug", "info", or "warn", case-insensitive.
//     It sets the initial threshold as SetVerbosity(2), (1), or (0) would.
//     An unset or unrecognised value leaves the default Warn threshold in place.
//   - LYM_LOG_FILE: a file path.
//     If set, log output is appended to that file instead of DefaultOutput, and the file is created if absent.
//     A failure to open it keeps the stderr sink at DefaultOutput.
//     It prints a one-line diagnostic to os.Stderr, since the logger cannot log its own bootstrap failure.
//     The variable exists because a test runner often already owns the process's stdout and stderr,
//     and a stderr-only sink is easy to lose track of across many concurrent test processes.
//     A fixed file path that an operator or orchestrator can point at removes that ambiguity.
//
// Both are read once, at package init.
// A caller's explicit SetVerbosity or SetOutput call after init always wins,
// so the -v/-vv flag still takes precedence over an env var left set in the shell.
//
// Two more environment variables govern trace identity and the durable sink:
//
//   - LYM_TRACE_ID: an inherited trace identity.
//     TraceID's lazy resolution and MintOrAdoptAndExport adopt it, so a child process continues its parent's trace.
//     Only a value of 16 lowercase hex characters, once trimmed, is adopted; any other value is ignored.
//     Another value would name a trace file that Sweep can never rank or remove.
//     Only MintOrAdoptAndExport sets the variable, through os.Setenv, and cmd/lym's root hook calls it.
//   - LYM_TRACE=1: the gate that lets the durable sink run under `go test`.
//     Under `go test`, the durable sink stays closed for the whole process unless LYM_TRACE is "1".
//     It stays closed however many records it admits and however the process exits.
//     This keeps ordinary test runs from filling the worktree's .lym/logs with test-process trace files.
//     It does not change trace-ID resolution: TraceID() and the trace= and span= fields are the same with or without it.
//
// A record may reach both LYM_LOG_FILE, when set, and the durable trace file.
// That is correct: the two differ in lifetime and audience.
// One is an operator-chosen, unmanaged file that takes every record at the stderr threshold.
// The other is a lym-managed file that retention sweeps, and neither should suppress the other.
//
// # Level policy
//
// Call sites that log through this package follow one policy for choosing a level:
//
//   - Warn: a notable but recoverable failure: a retry, an unconfirmed teardown, or a swallowed fallback error.
//   - Info: an OS-process spawn or teardown, one recorded durable state mutation, or a shed step boundary.
//     The durable sink records Info and above always, and Debug too at Debug verbosity.
//     So a mutation or step the trace does not record cannot be repaired from, and a -vv run leaves its full trace.
//   - Debug: everything else worth a line.
//
// Hard rule: nothing logs at Warn inside a loop body that can iterate more than roughly 10 times between state changes.
// A Warn in such a loop turns one notable condition into a flood.
// The flood reaches stderr at -v/-vv and, since Warn also reaches the durable sink, the trace file.
//
// Three more rules cover the states that a loop or poll logs:
//
//   - An ordinary state is Info at its change and never Warn.
//     Examples are a wait, a pause, a halt for the operator, and an ordinary retry iteration.
//   - A failure that repeats each tick logs Warn once per episode, Debug while it persists, and Info at recovery;
//     Episode and EpisodeSet implement this.
//   - A log line's dedupe key holds no text that changes while the state stays the same.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// levelVar is the threshold of the stderr sink.
var levelVar slog.LevelVar

// DefaultOutput returns the stderr sink's default destination:
// io.Discard inside a `go test` binary, os.Stderr otherwise.
// A test that restores the stderr sink after a redirect restores this value, never os.Stderr.
func DefaultOutput() io.Writer {
	if testing.Testing() {
		return io.Discard
	}
	return os.Stderr
}

var out io.Writer = DefaultOutput()

// currentStderr is the stderr half's inner handler.
// SetOutput rebuilds it to follow output redirections.
var currentStderr slog.Handler = slog.NewTextHandler(out, &slog.HandlerOptions{Level: &levelVar})

// stderrHandlerSnapshot returns the current stderr-half handler, read under sinkMu.
// The caller must not hold sinkMu across the Handle call that follows, to avoid deadlock.
func stderrHandlerSnapshot() slog.Handler {
	sinkMu.Lock()
	h := currentStderr
	sinkMu.Unlock()
	return h
}

// log is the package logger: a slog.Logger over the composite handler, which fans each record out to two halves.
// The stderr half is gated by levelVar.
// The durable-sink half admits Info and above whatever levelVar holds, and Debug too while levelVar holds Debug.
// SetOutput never reassigns log; it changes only currentStderr, which the handler reads on each call.
var log = slog.New(newDualHandler())

// dualHandler is the composite slog.Handler behind the package logger.
// It fans every record out to two halves: the stderr half, gated by levelVar;
// the durable-sink half admits Info and above whatever levelVar holds.
type dualHandler struct {
	// transform is the chain of WithAttrs and WithGroup calls that built this handler.
	// It is applied, in order, to the stderr handler read at Enabled and Handle time.
	// It is not baked into a stored stderr handler,
	// so a SetOutput rebind stays visible to handlers derived before the rebind.
	transform func(slog.Handler) slog.Handler
	durable   durableHandler
}

// newDualHandler builds the root dualHandler with no accumulated
// WithAttrs/WithGroup transform.
func newDualHandler() dualHandler {
	return dualHandler{transform: func(h slog.Handler) slog.Handler { return h }, durable: newDurableHandler()}
}

// stderr returns the stderr half with this handler's transform applied.
// It reads the current stderr handler on each call, so a SetOutput rebind is picked up.
func (d dualHandler) stderr() slog.Handler {
	return d.transform(stderrHandlerSnapshot())
}

// Enabled implements slog.Handler as the OR of the stderr and durable halves' own Enabled.
// The OR lets an Info record reach the durable sink at the default Warn verbosity.
// The stderr half alone would reject that record.
func (d dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.stderr().Enabled(ctx, level) || d.durable.Enabled(ctx, level)
}

// Handle implements slog.Handler by re-checking each half's own Enabled before delegating.
// It delegates only to the halves that pass.
// A single shared gate would leak an Info record to stderr at the default Warn threshold.
// The stderr half's own Enabled rejects an Info record at that threshold.
// Each delegate gets its own record.Clone().
// slog.Record documents that a Record passed to more than one Handler must be cloned for each use beyond the first;
// its internal small-attrs storage may otherwise be shared.
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

// WithAttrs implements slog.Handler by binding the same attrs into both halves.
// A logger built via slog.Logger.With keeps fanning out to both halves rather than collapsing to one.
func (d dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	prevTransform := d.transform
	return dualHandler{
		transform: func(h slog.Handler) slog.Handler { return prevTransform(h).WithAttrs(attrs) },
		durable:   d.durable.withAttrs(attrs),
	}
}

// WithGroup implements slog.Handler by binding the same group name into both halves.
func (d dualHandler) WithGroup(name string) slog.Handler {
	prevTransform := d.transform
	return dualHandler{
		transform: func(h slog.Handler) slog.Handler { return prevTransform(h).WithGroup(name) },
		durable:   d.durable.withGroup(name),
	}
}

// durableWriter adapts the durable sink's ensureDurableSink and writeDurable (sink.go) to an io.Writer.
// That lets durableHandler format records with an ordinary slog.NewTextHandler.
// The bytes still flow through the sink's lazy open and size cap.
type durableWriter struct{}

// Write opens the durable sink on first use and writes p to it.
// Opening is a no-op after the first call, via sink.go's sinkArmed flag.
// ensureDurableSink and writeDurable run one after the other, never nested and never while the caller holds sinkMu.
// Both take sinkMu, a plain non-reentrant Mutex, so either arrangement would self-deadlock.
// A closed or unarmed sink (sinkOK false) is not an error here.
// A diagnostic sink that never opens must not surface as a write failure, per sink.go's lazy-open rule.
// Write therefore reports success and discards p.
func (durableWriter) Write(p []byte) (int, error) {
	if !ensureDurableSink() {
		return len(p), nil
	}
	return writeDurable(p)
}

// durableLevel is the durable half's threshold: Debug while levelVar holds Debug, Info otherwise.
// Info and above are therefore admitted unconditionally, and Debug only at Debug verbosity.
type durableLevel struct{}

// Level implements slog.Leveler.
func (durableLevel) Level() slog.Level {
	if levelVar.Level() == slog.LevelDebug {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// durableHandler is the durable-sink half of the composite handler.
// It writes each record that the durable threshold admits to the durable sink.
type durableHandler struct {
	inner slog.Handler
}

// newDurableHandler builds the durable half over durableWriter, formatting records as slog.NewTextHandler does.
func newDurableHandler() durableHandler {
	return durableHandler{inner: slog.NewTextHandler(durableWriter{}, &slog.HandlerOptions{Level: durableLevel{}})}
}

// Enabled reports whether level meets the durable threshold.
func (d durableHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= durableLevel{}.Level()
}

// Handle formats record with the wrapped text handler; durableWriter carries the output to the durable sink.
func (d durableHandler) Handle(ctx context.Context, record slog.Record) error {
	return d.inner.Handle(ctx, record)
}

// withAttrs returns a copy of d with attrs bound into the wrapped handler.
// It returns durableHandler, not slog.Handler, so a caller can store the result without a type assertion.
func (d durableHandler) withAttrs(attrs []slog.Attr) durableHandler {
	return durableHandler{inner: d.inner.WithAttrs(attrs)}
}

// withGroup returns a copy of d with name bound into the wrapped handler.
func (d durableHandler) withGroup(name string) durableHandler {
	return durableHandler{inner: d.inner.WithGroup(name)}
}

func init() {
	levelVar.Set(slog.LevelWarn)
	configureFromEnv()
}

// configureFromEnv applies LYM_LOG_LEVEL and LYM_LOG_FILE, if set, on top of the Warn and stderr defaults.
// It is separate from init so that tests can call it.
func configureFromEnv() {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LYM_LOG_LEVEL"))) {
	case "debug":
		levelVar.Set(slog.LevelDebug)
	case "info":
		levelVar.Set(slog.LevelInfo)
	case "warn", "":
		// Warn is already the default set by init; "" means unset.
	}

	if path := strings.TrimSpace(os.Getenv("LYM_LOG_FILE")); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logger: LYM_LOG_FILE=%q: %v (falling back to stderr)\n", path, err)
			return
		}
		SetOutput(f)
	}
}

// Debug logs msg at debug level with the given key/value args.
// It stamps a trace key with TraceID()'s current value on every line.
// It reaches the stderr half and the durable sink only at Debug verbosity.
// SetVerbosity(2) or higher, or LYM_LOG_LEVEL=debug, sets that verbosity.
// At lower verbosity it writes nothing.
// Calling TraceID here also triggers its first resolution when nothing has resolved it yet.
func Debug(msg string, args ...any) {
	log.With("trace", TraceID()).Debug(msg, args...)
}

// Info logs msg at info level with the given key/value args.
// It stamps a trace key with TraceID()'s current value on every line.
// It reaches the durable sink at every verbosity.
// It reaches the stderr half only once SetVerbosity(1) or higher has been called.
func Info(msg string, args ...any) {
	log.With("trace", TraceID()).Info(msg, args...)
}

// Warn logs msg at warn level with the given key/value args and adds the current trace id as the trace= field.
// Warn meets the default threshold, so it reaches both the stderr output and the durable sink at default verbosity.
func Warn(msg string, args ...any) {
	log.With("trace", TraceID()).Warn(msg, args...)
}

// Error logs msg at error level with the given key/value args and adds the current trace id as the trace= field.
// Error reaches both the durable sink and the stderr output at any verbosity.
// Its level sits above the default warn threshold, so no verbosity setting filters it out.
func Error(msg string, args ...any) {
	log.With("trace", TraceID()).Error(msg, args...)
}

// SetVerbosity maps a -v repeat count to a log level.
// A count of zero or less keeps the default warn threshold, so a normal run writes nothing to stderr below warn.
// A count of one lowers the threshold to info, and a count of two or more lowers it to debug.
// The root -v/--verbose flag calls it when the flag is set, before the command runs.
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

// SetOutput redirects the stderr output of the log sink to w.
// It rebuilds the stderr handler and leaves the durable sink untouched.
// Redirecting stderr therefore never detaches the durable sink.
// Tests use it to capture log output without writing to the real os.Stderr.
// Setting LYM_LOG_FILE sends the stderr output to an operator-chosen file that retention does not manage.
// That file receives every record the current verbosity admits.
// The durable sink writes its own trace file in the logs directory, which retention sweeps.
// A record both outputs admit therefore lands in both files when LYM_LOG_FILE is set.
// The duplication is intended: the two files have different lifetimes, and neither should suppress the other.
//
// SetOutput holds sinkMu around both steps.
// An in-flight Handle call reads the stderr handler through stderrHandlerSnapshot, so the swap must not race with it.
func SetOutput(w io.Writer) {
	sinkMu.Lock()
	out = w
	currentStderr = slog.NewTextHandler(w, &slog.HandlerOptions{Level: &levelVar})
	sinkMu.Unlock()
}
