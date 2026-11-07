// trace.go holds the process-wide trace identity, a 16-hex-character ID that every log line carries.
// TraceID() is the lazy accessor for code that logs before cmd/lym's root hook has run.
// MintOrAdoptAndExport() is the root hook's explicit call; it also exports the ID so spawned children inherit it.

package logger

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"sync"
)

// TraceIDEnv names the environment variable that carries the trace identity between processes.
// A process adopts the identity from it and exports its own identity to it, so spawned children inherit it.
const TraceIDEnv = "LYM_TRACE_ID"

var traceOnce sync.Once
var traceID string

// traceIDPattern matches a trace identity: exactly 16 lowercase hex characters.
// Every adopted value must match it before it becomes this process's trace identity.
//
// The trace identity is not merely a log field: the sink builds the trace filename from it.
// filepath.Join cleans the composed name, so a value with a path separator or ".." writes outside the logs directory.
// Validation happens here, not at the filename, because the resolved value is exported to the environment:
// one unvalidated value would reach every spawned child.
//
// Log retention ranks and removes only trace files whose ID is 16 lowercase hex characters.
// Any other ID leaves a trace file that is never ranked or removed, so the logs directory grows without bound.
var traceIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// mintTraceID generates a fresh trace-identity as 16 lowercase hex characters.
func mintTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic("logger: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// NewTraceID mints a fresh trace identity for a child process.
// It leaves this process's own identity and the environment untouched.
func NewTraceID() string {
	return mintTraceID()
}

// adoptOrMintTraceID returns the trimmed LYM_TRACE_ID value when it is 16 lowercase hex characters.
// Otherwise it returns a freshly minted trace identity.
//
// A rejected value is replaced silently rather than reported, because there is no one to report to:
// this runs before any sink exists,
// and the caller's contract is "this process has a trace identity", which a fresh mint satisfies.
// Losing the parent's correlation is better than spending an attacker-shaped or malformed value as a filename component.
func adoptOrMintTraceID() string {
	if adopted := strings.TrimSpace(os.Getenv(TraceIDEnv)); traceIDPattern.MatchString(adopted) {
		return adopted
	}
	return mintTraceID()
}

// resolveTraceID sets traceID from the first of these that applies:
// a pre-set value, a valid LYM_TRACE_ID, or a fresh mint.
func resolveTraceID() {
	if traceID != "" {
		return
	}

	traceID = adoptOrMintTraceID()
}

// TraceID returns this process's trace-identity, resolving it on first call.
func TraceID() string {
	traceOnce.Do(resolveTraceID)
	return traceID
}

// MintOrAdoptAndExport resolves the trace identity and exports it to the environment as LYM_TRACE_ID.
//
// The adopted value is validated before export, because every child process this one spawns inherits it.
func MintOrAdoptAndExport() string {
	traceOnce.Do(func() {
		traceID = adoptOrMintTraceID()
	})
	os.Setenv(TraceIDEnv, traceID)
	return traceID
}
