// trace.go implements the process-wide trace-identity primitive: a single 16-hex-character ID that
// is minted or adopted once per process and stamped onto every emitted log line (batch 5's
// Debug/Info/Warn rewrite wires the call site).
// It provides two entry points: the lazy TraceID() accessor, used by any code path that logs before
// cmd/lyx's root hook has run, and MintOrAdoptAndExport, the root hook's explicit call that also
// exports the ID into the environment so spawned children inherit it.

package logger

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

var traceOnce sync.Once
var traceID string

// mintTraceID generates a fresh trace-identity as 16 lowercase hex characters.
func mintTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic("logger: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// resolveTraceID implements trace-ID precedence: pre-set value, LYX_TRACE_ID env var, or fresh mint.
func resolveTraceID() {
	if traceID != "" {
		return
	}

	if adopted := strings.TrimSpace(os.Getenv("LYX_TRACE_ID")); adopted != "" {
		traceID = adopted
		return
	}

	traceID = mintTraceID()
}

// TraceID returns this process's trace-identity, resolving it on first call.
func TraceID() string {
	traceOnce.Do(resolveTraceID)
	return traceID
}

// MintOrAdoptAndExport resolves the trace-identity and exports it to the environment.
func MintOrAdoptAndExport() string {
	traceOnce.Do(func() {
		if adopted := strings.TrimSpace(os.Getenv("LYX_TRACE_ID")); adopted != "" {
			traceID = adopted
		} else {
			traceID = mintTraceID()
		}
	})
	os.Setenv("LYX_TRACE_ID", traceID)
	return traceID
}
