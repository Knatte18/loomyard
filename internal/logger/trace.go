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
	"regexp"
	"strings"
	"sync"
)

var traceOnce sync.Once
var traceID string

// traceIDPattern is the exact alphabet mintTraceID produces, and the gate every ADOPTED value must
// pass before it becomes this process's trace-identity.
//
// The trace-identity is not merely a log field: sink.go interpolates it into the trace filename and
// filepath.Join CLEANS the composed name, so a value carrying separators or ".." walks the write out
// of the logs directory entirely (LYX_TRACE_ID='ci-run/../../pwned' landed a trace file one level
// ABOVE .lyx/logs, and a longer chain escapes the state directory). Validating here rather than at
// the filename is deliberate: MintOrAdoptAndExport re-exports the resolved value into the
// environment, so one unvalidated value would propagate to every spawned child.
//
// The alphabet is pinned to mintTraceID's own output for a second, independent reason:
// retention.go's traceFilePattern only matches a filename whose trace half is exactly 16 lowercase
// hex characters, so any other adopted value produces a trace file Sweep can never rank or remove
// and the logs directory grows without bound.
var traceIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// mintTraceID generates a fresh trace-identity as 16 lowercase hex characters.
func mintTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic("logger: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// adoptOrMintTraceID returns LYX_TRACE_ID's value when it matches the minted alphabet exactly, and a
// freshly minted identity otherwise.
//
// A rejected value is replaced silently rather than reported, because there is no one to report to:
// this runs before any sink exists, and the caller's contract is "this process has a trace
// identity", which a fresh mint satisfies. Losing the parent's correlation is strictly better than
// spending an attacker-shaped, or merely malformed, value as a filename component.
func adoptOrMintTraceID() string {
	if adopted := strings.TrimSpace(os.Getenv("LYX_TRACE_ID")); traceIDPattern.MatchString(adopted) {
		return adopted
	}
	return mintTraceID()
}

// resolveTraceID implements trace-ID precedence: pre-set value, a VALID LYX_TRACE_ID env var, or
// fresh mint.
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

// MintOrAdoptAndExport resolves the trace-identity and exports it to the environment.
//
// The export is why adoption goes through adoptOrMintTraceID here too and not only in
// resolveTraceID: this function OVERWRITES LYX_TRACE_ID with whatever it resolved, so validating on
// only one of the two entry points would leave the other free to launder a malformed value into
// every child process this one spawns.
func MintOrAdoptAndExport() string {
	traceOnce.Do(func() {
		traceID = adoptOrMintTraceID()
	})
	os.Setenv("LYX_TRACE_ID", traceID)
	return traceID
}
