// testsupport_test.go holds the fakes and helpers every test file in this package shares: a
// closure-backed ShedProducer fake, a temp-dir-wired Shed builder, and status-file seed/read
// helpers. It declares no Test function of its own; batches 3 and 4 consume these helpers and
// must not redeclare them.

package shedengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// funcProducer adapts a closure onto ShedProducer, so a test's fake producer is one struct
// literal rather than a bespoke named type per scenario.
type funcProducer struct {
	// fn has Call's own signature, so a test supplies exactly the behaviour it needs.
	fn func(ctx context.Context) (Outcome, OutputPointer, error)
	// calls counts every invocation of Call.
	calls int
}

// Call implements ShedProducer by recording the call and delegating to fn. It has a pointer
// receiver so calls survives across every call made through the same *funcProducer.
func (p *funcProducer) Call(ctx context.Context) (Outcome, OutputPointer, error) {
	p.calls++
	return p.fn(ctx)
}

// fixedOutcomeProducer returns a *funcProducer that always reports outcome, a nil error, and an
// OutputPointer naming outputPath, so the common "this producer just succeeds" case is one line
// at each call site.
func fixedOutcomeProducer(outcome Outcome, outputPath string) *funcProducer {
	return &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return outcome, OutputPointer{Path: outputPath}, nil
		},
	}
}

// newTestShed builds a Shed wired to a fresh t.TempDir(): StatusPath under a durable
// subdirectory (whose parent this helper creates, since Shed never creates it) and both lock
// paths under a separate ephemeral subdirectory, with distinct file names. The lock parents are
// deliberately left uncreated, so the ordinary test path exercises Run's own MkdirAll.
// Returns the Shed (with no Producers set -- callers fill that in) and the three paths, so a
// test can seed, inspect, and corrupt them.
func newTestShed(t *testing.T) (shed *Shed, statusPath, lockPath, statusLockPath string) {
	t.Helper()

	root := t.TempDir()
	durableDir := filepath.Join(root, "durable")
	if err := os.MkdirAll(durableDir, 0o755); err != nil {
		t.Fatalf("newTestShed: create durable dir: %v", err)
	}
	statusPath = filepath.Join(durableDir, "status.json")

	ephemeralDir := filepath.Join(root, "ephemeral")
	lockPath = filepath.Join(ephemeralDir, "run.lock")
	statusLockPath = filepath.Join(ephemeralDir, "status.json.lock")

	shed = &Shed{
		StatusPath:     statusPath,
		LockPath:       lockPath,
		StatusLockPath: statusLockPath,
	}
	return shed, statusPath, lockPath, statusLockPath
}

// seedStatus writes want to statusPath/statusLockPath via state.WriteJSON, failing the test on
// error.
// Unlike Shed itself, seedStatus stands in for an external seeder, which -- per the
// external-writer-lock-contract decision -- is already expected to ensure its own lock path is
// usable before writing; state.WriteJSON creates statusPath's parent but not statusLockPath's, so
// this helper creates the latter itself rather than pushing that bookkeeping onto every call site.
func seedStatus(t *testing.T, statusPath, statusLockPath string, want Status) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatalf("seedStatus: create status lock parent dir: %v", err)
	}
	if err := state.WriteJSON(statusPath, statusLockPath, want); err != nil {
		t.Fatalf("seedStatus: state.WriteJSON(...) = %v", err)
	}
}

// readStatus reads the status file at statusPath/statusLockPath via state.ReadJSONStrict, failing
// the test if the read errors or the file is missing.
func readStatus(t *testing.T, statusPath, statusLockPath string) Status {
	t.Helper()
	got, found, err := state.ReadJSONStrict[Status](statusPath, statusLockPath)
	if err != nil {
		t.Fatalf("readStatus: state.ReadJSONStrict(...) = _, _, %v", err)
	}
	if !found {
		t.Fatalf("readStatus: status file %q not found", statusPath)
	}
	return got
}

// commonSeed builds the common status-file seed -- a given current_producer, StateRunning, no
// error, no pause, empty history -- so each test states only what it varies.
func commonSeed(currentProducer string) Status {
	return Status{
		CurrentProducer: currentProducer,
		State:           StateRunning,
		Error:           "",
		PauseRequested:  false,
		Activity:        composeActivity(currentProducer, nil, StateRunning, ""),
		History:         nil,
	}
}

// assertRFC3339UTC fails the test unless at parses as RFC3339 with a zero UTC offset. Tests must
// never assert an exact timestamp literal; this is the structural check they use instead.
func assertRFC3339UTC(t *testing.T, at string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Errorf("assertRFC3339UTC(%q): time.Parse = %v", at, err)
		return
	}
	if _, offset := parsed.Zone(); offset != 0 {
		t.Errorf("assertRFC3339UTC(%q): zone offset = %d; want 0", at, offset)
	}
}

// linearChain builds one ProducerDef per name, in order, pairing each with the matching entry in
// producers, and wires OnDone to reproduce today's sequential advance: each entry's OnDone names
// the next entry, and the last entry's OnDone is left empty, which is what finishes the run. It
// exists so a scenario that only needs "these producers, in this order" does not restate the chain
// by hand.
// It fails loud rather than guessing when names and producers differ in length.
func linearChain(t *testing.T, names []string, producers []ShedProducer) []ProducerDef {
	t.Helper()
	if len(names) != len(producers) {
		t.Fatalf("linearChain: len(names) = %d, len(producers) = %d; want equal", len(names), len(producers))
	}
	defs := make([]ProducerDef, len(names))
	for i, name := range names {
		onDone := ""
		if i < len(names)-1 {
			onDone = names[i+1]
		}
		defs[i] = ProducerDef{
			Name:     name,
			Producer: producers[i],
			OnDone:   onDone,
		}
	}
	return defs
}

// assertHistoryNonDecreasing fails the test unless every entry's At value is parseable RFC3339
// and the sequence is non-decreasing in time.
func assertHistoryNonDecreasing(t *testing.T, entries []HistoryEntry) {
	t.Helper()
	var prev time.Time
	for i, e := range entries {
		parsed, err := time.Parse(time.RFC3339, e.At)
		if err != nil {
			t.Errorf("assertHistoryNonDecreasing: entry %d At %q: time.Parse = %v", i, e.At, err)
			continue
		}
		if i > 0 && parsed.Before(prev) {
			t.Errorf("assertHistoryNonDecreasing: entry %d At %q is before entry %d's %q", i, e.At, i-1, entries[i-1].At)
		}
		prev = parsed
	}
}
