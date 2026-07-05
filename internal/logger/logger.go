// logger.go provides a thin log/slog wrapper for lyx: a package-level level
// threshold, an injectable io.Writer sink (defaulting to os.Stderr), and
// Debug/Info/Warn helpers. Callers never see slog directly; they call
// SetVerbosity to raise the threshold and Debug/Info/Warn to emit.

// Package logger is a minimal log/slog wrapper shared across lyx's internal
// packages. It keeps stdout free of log noise (mux and other commands write
// their JSON envelope to stdout via internal/output) by routing all log
// output to a dedicated sink, which defaults to os.Stderr and is silent
// unless the caller opts in via SetVerbosity.
package logger

import (
	"io"
	"log/slog"
	"os"
)

// levelVar is the package-level threshold shared by the slog handler. It is
// initialised to slog.LevelWarn in init so a normal run emits zero log
// lines; -v/-vv (wired to SetVerbosity in cmd/lyx/main.go) lower it to
// surface more detail.
var levelVar slog.LevelVar

// out is the sink log lines are written to. It defaults to os.Stderr and is
// only replaced via SetOutput, which exists as a test seam so tests can
// capture output into a buffer instead of touching the real stderr.
var out io.Writer = os.Stderr

// log is the slog.Logger built over out and levelVar. It is rebuilt whenever
// SetOutput changes the sink, since slog.NewTextHandler captures its writer
// by value at construction time; the level itself lives in levelVar and
// survives the rebuild.
var log = newLogger(out)

func init() {
	levelVar.Set(slog.LevelWarn)
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
