// run_persist_test.go covers the boundary between Shed and disk: the read-modify-write merge
// against a concurrent external writer, the opaque product passthrough, the deliberate
// destruction of a stray external key, the two ways a persist can fail, the five read-gate and
// lookup hard errors, and the locking rules (already-held run lock, lock-parent creation, and the
// same-path validation error).

package shedengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// assertZeroResult fails the test unless result is the Result zero value. Result contains a
// slice field, so it cannot be compared with != against a literal; this checks each field
// individually instead.
func assertZeroResult(t *testing.T, result Result) {
	t.Helper()
	if result.Outcome != "" || result.HaltedProducer != "" || result.Reason != "" || result.History != nil {
		t.Errorf("Result = %+v; want the zero value", result)
	}
}

// externalUpdate applies mutate to the status file at statusPath/statusLockPath through
// state.UpdateJSON against a lenient map[string]any, the lock-cooperating shape every external
// writer in this batch uses -- the same StatusLockPath the Shed under test was told, never a bare
// os.WriteFile. It stands in for a product's pause verb or its own spawn-time seeder mutating the
// file out from under a running Shed.
func externalUpdate(t *testing.T, statusPath, statusLockPath string, mutate func(cur map[string]any)) {
	t.Helper()
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur map[string]any, found bool) (map[string]any, error) {
		if !found {
			return nil, fmt.Errorf("externalUpdate: status file %q not found", statusPath)
		}
		mutate(cur)
		return cur, nil
	})
	if err != nil {
		t.Fatalf("externalUpdate: state.UpdateJSON(...) = %v", err)
	}
}

// TestRun_ExternalWriteSurvivesPersist is the regression test for the whole-file-clobber hazard,
// and the reason the loop re-reads at the top of every iteration and merges rather than
// rewriting from an in-memory copy: as originally specified, a pause requested during a long
// producer call was both never observed (step 6 routed to step 2, past step 3's data source) and
// silently destroyed (a blind rewrite from the in-memory copy read at step 1).
func TestRun_ExternalWriteSurvivesPersist(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	replacedProduct := json.RawMessage(`{"replaced":true}`)
	p1 := &funcProducer{}
	p1.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		// Both mutations happen from inside the producer's own Call -- the only point in
		// the loop guaranteed to fall between step 1's read and step 5/6's persist.
		externalUpdate(t, statusPath, statusLockPath, func(cur map[string]any) {
			cur["pause_requested"] = true
			cur["product"] = replacedProduct
		})
		return Done, OutputPointer{}, nil
	}
	p2 := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{p1, p2})
	seed := commonSeed("A")
	seed.Product = json.RawMessage(`{"original":true}`)
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	// The pause must be honoured at the very next iteration: the run exits RunPaused without
	// ever calling the second producer.
	if result.Outcome != RunPaused {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunPaused)
	}
	if p2.calls != 0 {
		t.Errorf("p2.calls = %d; want 0 -- the external pause request must be observed before B is called", p2.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, StatePaused)
	}
	var gotProduct, wantProduct any
	if err := json.Unmarshal(got.Product, &gotProduct); err != nil {
		t.Fatalf("unmarshal persisted product: %v", err)
	}
	if err := json.Unmarshal(replacedProduct, &wantProduct); err != nil {
		t.Fatalf("unmarshal replaced product: %v", err)
	}
	if !reflect.DeepEqual(gotProduct, wantProduct) {
		t.Errorf("persisted Product = %s; want semantically equal to %s", got.Product, replacedProduct)
	}
}

// TestRun_ProductPassthrough seeds a status file carrying an arbitrary product payload, runs a
// list to completion, and asserts the payload survives with semantic equality. product is
// json.RawMessage, a type Shed never decodes into anything of its own -- the payload used here is
// a shape Shed has no type for at all, which is the point.
func TestRun_ProductPassthrough(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Done, "")},
	}
	// Deliberately compact, unindented JSON: persistence goes through json.MarshalIndent,
	// which re-indents an embedded raw message, so this payload survives semantically but
	// not byte-for-byte -- the reason the assertion below compares through an any unmarshal
	// rather than a raw byte compare.
	product := json.RawMessage(`{"slug":"loom","nested":{"list":[1,2,3]},"note":"a shape Shed has no type for"}`)
	seed := commonSeed("A")
	seed.Product = product
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	got := readStatus(t, statusPath, statusLockPath)
	var gotAny, wantAny any
	if err := json.Unmarshal(got.Product, &gotAny); err != nil {
		t.Fatalf("unmarshal persisted product: %v", err)
	}
	if err := json.Unmarshal(product, &wantAny); err != nil {
		t.Fatalf("unmarshal seeded product: %v", err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Errorf("persisted Product = %s; want semantically equal to %s", got.Product, product)
	}
	// Byte identity is deliberately not asserted here: MarshalIndent's re-indentation of the
	// embedded raw message means the two forms legitimately differ byte-for-byte even though
	// they carry the same value.
	if bytes.Equal(got.Product, product) {
		t.Logf("persisted Product happened to be byte-identical to the seed; the semantic-equality assertion above is still the one that matters")
	}
}

// TestRun_StrayExternalKeyDestroyed pins that an unrecognised top-level key written by an
// external actor after the read gate has already passed is silently dropped by the full-struct
// marshal, not caught by a later strict read -- because the merge strips it and the next read
// then sees a clean file. This is a corrected misconception rather than an obvious fact: product
// is the sanctioned channel for everything an external writer legitimately owns, so a top-level
// key outside it is a mistake nothing in this design promises to preserve. A key present before
// the read gate is a different case, covered by TestRun_UnknownTopLevelKeyBeforeRead below.
func TestRun_StrayExternalKeyDestroyed(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	p1 := &funcProducer{}
	p1.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		externalUpdate(t, statusPath, statusLockPath, func(cur map[string]any) {
			cur["a_stray_top_level_key"] = "an external writer's mistake"
		})
		return Done, OutputPointer{}, nil
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: p1}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error -- the run completes normally rather than erroring", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	// readStatus uses the strict decoder; it would itself fail the test if the key
	// survived, but a direct raw-map check states the property being pinned explicitly.
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal status file: %v", err)
	}
	if _, present := raw["a_stray_top_level_key"]; present {
		t.Errorf("stray key survived the persist; want it silently dropped by the full-struct marshal")
	}
	readStatus(t, statusPath, statusLockPath) // fails the test itself if the strict read now rejects the file.
}

// TestRun_StatusFileDeletedMidRun proves the persist's own missing-file guard holds the
// never-seed rule at the write as well as at the read: state.UpdateJSON treats a missing file as
// a non-error and would otherwise write whatever the mutate returned, silently re-creating the
// file from a zero value.
func TestRun_StatusFileDeletedMidRun(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	p1 := &funcProducer{}
	p1.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		if err := os.Remove(statusPath); err != nil {
			t.Fatalf("remove status file: %v", err)
		}
		return Done, OutputPointer{}, nil
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: p1}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	_, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
		t.Errorf("os.Stat(statusPath) = _, %v; want a not-exist error -- the file must not be silently re-created", statErr)
	}
}

// TestRun_PersistFailure injects a genuine persist-side failure the one way that works on every
// platform regardless of privilege: removing the status file's parent directory and creating a
// regular file at that same path, so the persist's own directory creation fails with a
// not-a-directory error.
//
// Simpler injections were rejected: a merely-absent parent directory does not fail, because the
// update primitive creates it first; an unwritable parent directory is a no-op under root and on
// Windows; and routing the status path through an existing regular file fails too early, at the
// read gate rather than at the persist, testing the wrong thing.
//
// Only two assertions are made, deliberately. "No compensating failed-state write was attempted"
// is unobservable under this injection, because the injection destroys the write target, so a
// hypothetical compensating write would fail identically to the persist itself and the test
// cannot distinguish "never tried" from "tried and also failed" -- that the compensating write is
// forbidden is a design decision enforced by review, not by this test. Nor does this test assert
// the file keeps its last-good contents: this injection necessarily destroys the file that
// assertion would read; that property is covered by batch 3's crash-recovery scenario instead.
func TestRun_PersistFailure(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	durableDir := filepath.Dir(statusPath)

	p1 := &funcProducer{}
	p1.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		// Runs after the read gate for this iteration has already succeeded.
		if err := os.RemoveAll(durableDir); err != nil {
			t.Fatalf("remove durable dir: %v", err)
		}
		if err := os.WriteFile(durableDir, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("replace durable dir with a regular file: %v", err)
		}
		return Done, OutputPointer{}, nil
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: p1}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	_, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	if _, statErr := os.Stat(statusPath); statErr == nil {
		t.Errorf("os.Stat(statusPath) = nil error; want no status file at the target path")
	}
}

// TestRun_MissingStatusFile covers the missing-status-file hard error. Assert the error, and
// assert specifically that the status file was not created. Lock files and their parent
// directories are asserted present, not absent: acquiring a lock creates the lock file, and only
// the status file is covered by the never-seed rule.
func TestRun_MissingStatusFile(t *testing.T) {
	shed, statusPath, lockPath, statusLockPath := newTestShed(t)
	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	// No seedStatus call: the status file never exists.

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	assertZeroResult(t, result)
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}

	if _, statErr := os.Stat(statusPath); !os.IsNotExist(statErr) {
		t.Errorf("os.Stat(statusPath) = _, %v; want a not-exist error", statErr)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Errorf("run lock file not created: %v", statErr)
	}
	if _, statErr := os.Stat(statusLockPath); statErr != nil {
		t.Errorf("status lock file not created: %v", statErr)
	}
}

// TestRun_CurrentProducerNotFound covers current_producer naming an absent producer. Shed never
// guesses, neither restarting from the first producer nor advancing to the nearest match, so the
// file is left byte-identical.
func TestRun_CurrentProducerNotFound(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Ghost"))

	before, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file before Run: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	assertZeroResult(t, result)
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}

	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after Run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("status file bytes changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestRun_MalformedStatusFile covers a status file whose bytes are not valid JSON.
func TestRun_MalformedStatusFile(t *testing.T) {
	shed, statusPath, _, _ := newTestShed(t)
	if err := os.WriteFile(statusPath, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatalf("write malformed status file: %v", err)
	}

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	assertZeroResult(t, result)
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}
}

// TestRun_UnknownTopLevelKeyBeforeRead covers an unknown top-level key present before the read
// gate runs -- the strict read gate rejecting a key that was on disk when the gate ran, which is
// deliberately a different case from TestRun_StrayExternalKeyDestroyed's key written after the
// gate passed.
func TestRun_UnknownTopLevelKeyBeforeRead(t *testing.T) {
	shed, statusPath, _, _ := newTestShed(t)

	raw := map[string]any{
		"current_producer": "A",
		"state":            string(StateRunning),
		"error":            "",
		"pause_requested":  false,
		"activity":         map[string]any{"now": "A", "last": "", "wait": ""},
		"history":          []any{},
		"unexpected_key":   true,
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal seed with unknown key: %v", err)
	}
	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	assertZeroResult(t, result)
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}
}

// TestRun_UnrecognisedState covers two cases of an unrecognised persisted state value: a typo,
// and the empty string. The empty string is rejected rather than tolerated because a mandatory
// enum string left empty by a partial seed would otherwise be silently treated as running, or as
// done.
func TestRun_UnrecognisedState(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"Typo", State("runing")},
		{"Empty", State("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			seed := commonSeed("A")
			seed.State = tt.state
			seedStatus(t, statusPath, statusLockPath, seed)

			producer := fixedOutcomeProducer(Done, "")
			shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

			result, err := shed.Run(context.Background())
			if err == nil {
				t.Fatalf("Run(...) = _, nil; want a non-nil error")
			}
			assertZeroResult(t, result)
			if producer.calls != 0 {
				t.Errorf("producer.calls = %d; want 0", producer.calls)
			}
		})
	}
}

// TestRun_LockAlreadyHeld covers Run's fail-fast refusal when another invocation already holds
// LockPath. It acquires the lock directly via internal/lock's non-blocking acquire -- no second
// process is needed -- rather than spawning one, keeping this test hermetic and fast.
func TestRun_LockAlreadyHeld(t *testing.T) {
	shed, statusPath, lockPath, statusLockPath := newTestShed(t)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create run lock parent dir: %v", err)
	}
	held, locked, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("TryAcquireWriteLock(...) = _, _, %v", err)
	}
	if !locked {
		t.Fatalf("TryAcquireWriteLock(...) = _, false, nil; want the lock to be free at test start")
	}
	defer held.Release()

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	before, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file before Run: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a non-nil error")
	}
	if !errors.Is(err, ErrShedBusy) {
		t.Errorf("errors.Is(err, ErrShedBusy) = false for err = %v; want true", err)
	}
	assertZeroResult(t, result)
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}

	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after Run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("status file bytes changed while the run lock was refused:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestRun_LockParentCreation builds a Shed whose LockPath sits under a directory that does not
// yet exist, and asserts Run succeeds rather than failing with a raw lock-acquire error, creating
// the parent itself. StatusLockPath's own parent is created ahead of time by this test's seed
// step: internal/state never creates a lock path's parent, only the value path's, so an external
// seeder (the same obligation any real external writer has) must ensure it is usable before
// writing -- that property therefore applies here to LockPath alone, the one directory Run is
// solely responsible for making usable.
func TestRun_LockParentCreation(t *testing.T) {
	root := t.TempDir()
	durableDir := filepath.Join(root, "durable")
	if err := os.MkdirAll(durableDir, 0o755); err != nil {
		t.Fatalf("create durable dir: %v", err)
	}
	statusPath := filepath.Join(durableDir, "status.json")

	statusLockDir := filepath.Join(root, "status-lock")
	if err := os.MkdirAll(statusLockDir, 0o755); err != nil {
		t.Fatalf("create status lock parent dir: %v", err)
	}
	statusLockPath := filepath.Join(statusLockDir, "status.json.lock")

	lockPath := filepath.Join(root, "run-lock-parent", "run.lock")

	shed := &Shed{StatusPath: statusPath, LockPath: lockPath, StatusLockPath: statusLockPath}
	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error -- a raw lock-acquire error means the parent-dir MkdirAll did not run", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	if _, statErr := os.Stat(filepath.Dir(lockPath)); statErr != nil {
		t.Errorf("run lock parent dir not created: %v", statErr)
	}
	if _, statErr := os.Stat(statusLockDir); statErr != nil {
		t.Errorf("status lock parent dir missing: %v", statErr)
	}
}

// TestRun_SameLockPathIsValidationError covers a Shed whose LockPath and StatusLockPath name the
// same file. The assertion is the error itself: a test that deadlocked here would hang the whole
// suite rather than fail it, which is precisely the failure shape the same-path rule exists to
// convert into an error. This test deliberately does not bound the call with a timer or a
// goroutine -- validation runs before any lock is acquired, so a correct implementation cannot
// reach the blocking acquire at all, and a wrapper that tolerated a hang would hide the bug
// rather than catch it.
func TestRun_SameLockPathIsValidationError(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.LockPath = statusLockPath
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err == nil {
		t.Fatalf("Run(...) = _, nil; want a validation error")
	}
	assertZeroResult(t, result)
}
