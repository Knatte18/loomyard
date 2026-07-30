// logger.go provides a thin log/slog wrapper for lyx: a package-level level
// threshold, an injectable io.Writer sink (defaulting to os.Stderr), and
// Debug/Info/Warn helpers. Callers never see slog directly; they call
// SetVerbosity to raise the threshold and Debug/Info/Warn to emit.

// Package logger is a minimal log/slog wrapper shared across lyx's internal
// packages. It keeps stdout free of log noise (reed and other commands write
// their JSON envelope to stdout via internal/output) by routing all log
// output to a dedicated sink, which defaults to os.Stderr and is silent
// unless the caller opts in via SetVerbosity.
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
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// levelVar is the package-level threshold shared by the slog handler. It is
// initialised to slog.LevelWarn in init so a normal run emits zero log
// lines; -v/-vv (wired to SetVerbosity in cmd/lyx/main.go) lower it to
// surface more detail, as does LYX_LOG_LEVEL for entry points that never
// reach that flag parsing (see the package doc).
var levelVar slog.LevelVar

// out is the sink log lines are written to. It defaults to os.Stderr,
// overridable at init via LYX_LOG_FILE, and otherwise only replaced via
// SetOutput, which exists as a test seam so tests can capture output into a
// buffer instead of touching the real stderr.
var out io.Writer = os.Stderr

// log is the slog.Logger built over out and levelVar. It is rebuilt whenever
// SetOutput changes the sink, since slog.NewTextHandler captures its writer
// by value at construction time; the level itself lives in levelVar and
// survives the rebuild.
var log = newLogger(out)

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

// newLogger builds a text-handler slog.Logger writing to w, gated by the
// package's shared levelVar so verbosity changes take effect without
// rebuilding the logger.
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: &levelVar}))
}

// Debug logs msg at debug level with the given key/value args. It is a
// no-op unless SetVerbosity(2) or higher has been called.
func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

// Info logs msg at info level with the given key/value args. It is a no-op
// unless SetVerbosity(1) or higher has been called.
func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

// Warn logs msg at warn level with the given key/value args. Warn is the
// default threshold, so Warn calls are emitted even without SetVerbosity.
func Warn(msg string, args ...any) {
	log.Warn(msg, args...)
}

// SetVerbosity maps a -v repeat count to a log level: count<=0 keeps the
// default Warn threshold (silent normal run), count==1 lowers it to Info,
// and count>=2 lowers it to Debug. cmd/lyx/main.go calls this once at
// startup from the root -v/--verbose flag.
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

// SetOutput rebinds the log sink to w and rebuilds the underlying handler.
// It exists as a test seam so tests can assert on captured output without
// writing to the real os.Stderr; production code never calls it.
func SetOutput(w io.Writer) {
	out = w
	log = newLogger(out)
}
