// state_test.go covers LoadState/SaveState's three documented cases:
// round-tripping a populated State through disk, an absent state.json
// returning (nil, nil), and a corrupt state.json returning a wrapped
// error rather than a guessed value.

package builderengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lock"
)

func TestState_RoundTrip(t *testing.T) {
	t.Parallel()

	builderDir := t.TempDir()
	want := &builderengine.State{
		RunGUID:         "run-1",
		PlanFingerprint: "abc123",
		CurrentBatch:    2,
		Batches: map[int]*builderengine.BatchState{
			1: {
				Slug:          "first",
				StartSHA:      "deadbeef",
				Role:          "implementer",
				StrandGUID:    "strand-1",
				ShuttleRunDir: "/runs/1",
				EventsPath:    "/runs/1/events.jsonl",
				SpawnedAt:     "2026-07-11T12:00:00Z",
				Terminal:      true,
				Status:        "done",
			},
		},
		ChainStartSHAs: map[int]string{4: "cafef00d"},
	}

	if err := builderengine.SaveState(builderDir, want); err != nil {
		t.Fatalf("SaveState error = %v; want nil", err)
	}

	got, err := builderengine.LoadState(builderDir)
	if err != nil {
		t.Fatalf("LoadState error = %v; want nil", err)
	}
	if got == nil {
		t.Fatal("LoadState() = nil; want the saved state")
	}

	if got.RunGUID != want.RunGUID {
		t.Errorf("RunGUID = %q; want %q", got.RunGUID, want.RunGUID)
	}
	if got.PlanFingerprint != want.PlanFingerprint {
		t.Errorf("PlanFingerprint = %q; want %q", got.PlanFingerprint, want.PlanFingerprint)
	}
	if got.CurrentBatch != want.CurrentBatch {
		t.Errorf("CurrentBatch = %d; want %d", got.CurrentBatch, want.CurrentBatch)
	}
	if got.ChainStartSHAs[4] != want.ChainStartSHAs[4] {
		t.Errorf("ChainStartSHAs[4] = %q; want %q", got.ChainStartSHAs[4], want.ChainStartSHAs[4])
	}

	gotBatch, ok := got.Batches[1]
	if !ok {
		t.Fatal("Batches[1] missing after round-trip")
	}
	wantBatch := want.Batches[1]
	if gotBatch.Slug != wantBatch.Slug ||
		gotBatch.StartSHA != wantBatch.StartSHA ||
		gotBatch.Role != wantBatch.Role ||
		gotBatch.StrandGUID != wantBatch.StrandGUID ||
		gotBatch.ShuttleRunDir != wantBatch.ShuttleRunDir ||
		gotBatch.EventsPath != wantBatch.EventsPath ||
		gotBatch.SpawnedAt != wantBatch.SpawnedAt ||
		gotBatch.Terminal != wantBatch.Terminal ||
		gotBatch.Status != wantBatch.Status {
		t.Errorf("Batches[1] = %+v; want %+v", gotBatch, wantBatch)
	}
}

func TestState_AbsentFileReturnsNil(t *testing.T) {
	t.Parallel()

	builderDir := t.TempDir()

	got, err := builderengine.LoadState(builderDir)
	if err != nil {
		t.Fatalf("LoadState(absent) error = %v; want nil", err)
	}
	if got != nil {
		t.Errorf("LoadState(absent) = %+v; want nil", got)
	}
}

func TestState_CorruptFileErrors(t *testing.T) {
	t.Parallel()

	builderDir := t.TempDir()
	if err := os.MkdirAll(builderDir, 0o755); err != nil {
		t.Fatalf("mkdir builderDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(builderDir, "state.json"), []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("write corrupt state.json: %v", err)
	}

	got, err := builderengine.LoadState(builderDir)
	if err == nil {
		t.Fatal("LoadState(corrupt) error = nil; want error")
	}
	if got != nil {
		t.Errorf("LoadState(corrupt) = %+v; want nil on error", got)
	}
}

// TestAcquireStateMutation_ExcludesSecondHolder proves the state-mutation
// lease is a real cross-holder exclusive lock: while held, a second
// non-blocking acquire of the same lease file fails, and after Release it
// succeeds — the property every verb's load-mutate-save section relies on.
func TestAcquireStateMutation_ExcludesSecondHolder(t *testing.T) {
	builderDir := t.TempDir()

	held, err := builderengine.AcquireStateMutation(builderDir)
	if err != nil {
		t.Fatalf("AcquireStateMutation() error = %v; want nil", err)
	}

	_, locked, err := lock.TryAcquireWriteLock(filepath.Join(builderDir, "mutate.lock"))
	if err != nil {
		t.Fatalf("TryAcquireWriteLock() error = %v; want nil", err)
	}
	if locked {
		t.Fatal("TryAcquireWriteLock() = locked while AcquireStateMutation held the lease; want excluded")
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v; want nil", err)
	}
	second, locked, err := lock.TryAcquireWriteLock(filepath.Join(builderDir, "mutate.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() after Release = locked=%v err=%v; want locked=true, err=nil", locked, err)
	}
	_ = second.Release()
}
