// trace.go implements the process-wide trace-identity primitive: a single
// 16-hex-character ID that is minted or adopted once per process and stamped
// onto every emitted log line (batch 5's Debug/Info/Warn rewrite wires the
// call site). It provides two entry points: the lazy TraceID() accessor,
// used by any code path that logs before cmd/lyx's root hook has run, and
// MintOrAdoptAndExport, the root hook's explicit call that also exports the
// ID into the environment so spawned children inherit it.

package logger

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

// traceOnce guards the lazy resolution of traceID performed by TraceID.
// It is distinct from sinkOnce (batch 4) per the concurrency-contract
// decision: traceOnce must fire on any emit, including a Debug call that
// never reaches the durable sink, because trace= is stamped on every line
// at every level regardless of sink/verbosity state.
var traceOnce sync.Once

// traceID holds the process's resolved trace-identity once traceOnce has
// fired. It is either set directly by MintOrAdoptAndExport (when the root
// hook runs before any log call) or resolved lazily inside traceOnce.Do by
// TraceID's first caller.
var traceID string

// mintTraceID generates a fresh trace-identity: 8 random bytes rendered as
// 16 lowercase hex characters. It is the fallback used whenever no trace-ID
// has already been set or adopted from the environment.
func mintTraceID() string {
	buf := make([]byte, 8)
	// crypto/rand.Read only errors if the system CSPRNG is unavailable, a
	// condition this process cannot meaningfully recover from; panicking
	// here matches the rest of the package's stdlib-only, no-fallback
	// posture rather than silently minting a weaker/predictable ID.
	if _, err := rand.Read(buf); err != nil {
		panic("logger: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// resolveTraceID implements the three-way precedence discussion.md's
// trace-id-mint-and-propagate decision requires: (1) a value already set by
// MintOrAdoptAndExport before traceOnce fires wins outright, (2) otherwise
// LYX_TRACE_ID is adopted when set to a non-whitespace-only value, (3)
// otherwise a fresh ID is minted. It is only ever invoked from inside
// traceOnce.Do, so it runs at most once per process.
func resolveTraceID() {
	// Step 1: MintOrAdoptAndExport may have already stored a value into
	// traceID via its own traceOnce.Do call before any emit happened; that
	// value takes precedence and nothing further needs resolving.
	if traceID != "" {
		return
	}

	// Step 2: adopt an inherited trace-ID from a parent process, using the
	// same trim-then-empty-check idiom configureFromEnv uses for
	// LYX_LOG_LEVEL so a whitespace-only value is treated as unset.
	if adopted := strings.TrimSpace(os.Getenv("LYX_TRACE_ID")); adopted != "" {
		traceID = adopted
		return
	}

	// Step 3: no inherited or pre-set value; this process originates its
	// own trace-ID.
	traceID = mintTraceID()
}

// TraceID returns this process's trace-identity, resolving it on first call
// via resolveTraceID's precedence (root-hook value, then LYX_TRACE_ID, then
// a fresh mint). It is what every Debug/Info/Warn call stamps onto emitted
// lines, and is safe to call from a `go test` binary or any other entry
// point that never runs cmd/lyx's root hook.
func TraceID() string {
	traceOnce.Do(resolveTraceID)
	return traceID
}

// MintOrAdoptAndExport resolves this process's trace-identity and exports
// it into the environment so every spawned child inherits it via
// os.Environ() at any spawn site that does not override cmd.Env. It is the
// only function in this package that calls os.Setenv, and is called
// exclusively by cmd/lyx/main.go's root PersistentPreRunE — never from the
// lazy TraceID() path, so a test binary that only calls TraceID() never
// mutates the environment as a side effect of logging.
func MintOrAdoptAndExport() string {
	// Reuse traceOnce so a later TraceID() call in the same process sees
	// this resolved value via resolveTraceID's step 1, rather than
	// re-resolving from the environment independently.
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
