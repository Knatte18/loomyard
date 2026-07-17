// state_test.go covers LoadState/SaveState's documented cases:
// round-tripping a populated State through disk (including a persisted
// digest, the field builderengine never needed to persist), an absent
// state.json returning (nil, nil), a corrupt state.json returning a wrapped
// error rather than a guessed value, and the state-mutation lease's
// cross-holder exclusion. All plain t.TempDir() files — no git, no
// subprocess spawns — Test Tier Purity Invariant.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestState_RoundTrip(t *testing.T) {
	t.Parallel()

	websterDir := t.TempDir()
	want := &websterengine.State{
		RunGUID:         "run-1",
		PlanFingerprint: "abc123",
		CurrentBatch:    2,
		MasterStrand:    "master-strand-1",
		MasterSessionID: "session-1",
		AssertedModel:   "master_oversized",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug:            "first",
				StartSHA:        "deadbeef",
				Kind:            "fork",
				SpawnedAt:       "2026-07-11T12:00:00Z",
				Terminal:        true,
				Status:          "done",
				ForkTranscripts: []string{"subagents/abc.jsonl"},
			},
			2: {
				Slug:          "second",
				StartSHA:      "cafef00d",
				Kind:          "recovery",
				SpawnedAt:     "2026-07-11T13:00:00Z",
				Terminal:      false,
				StrandGUID:    "strand-2",
				ShuttleRunDir: "/runs/2",
				EventsPath:    "/runs/2/events.jsonl",
			},
		},
		ChainStartSHAs:      map[int]string{4: "cafef00d"},
		SeenForkTranscripts: []string{"subagents/abc.jsonl"},
	}

	if err := websterengine.SaveState(websterDir, want); err != nil {
		t.Fatalf("SaveState error = %v; want nil", err)
	}

	got, err := websterengine.LoadState(websterDir)
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
	if got.MasterStrand != want.MasterStrand {
		t.Errorf("MasterStrand = %q; want %q", got.MasterStrand, want.MasterStrand)
	}
	if got.MasterSessionID != want.MasterSessionID {
		t.Errorf("MasterSessionID = %q; want %q", got.MasterSessionID, want.MasterSessionID)
	}
	if got.AssertedModel != want.AssertedModel {
		t.Errorf("AssertedModel = %q; want %q", got.AssertedModel, want.AssertedModel)
	}
	if got.ChainStartSHAs[4] != want.ChainStartSHAs[4] {
		t.Errorf("ChainStartSHAs[4] = %q; want %q", got.ChainStartSHAs[4], want.ChainStartSHAs[4])
	}
	if len(got.SeenForkTranscripts) != 1 || got.SeenForkTranscripts[0] != "subagents/abc.jsonl" {
		t.Errorf("SeenForkTranscripts = %v; want %v", got.SeenForkTranscripts, want.SeenForkTranscripts)
	}

	gotBatch1, ok := got.Batches[1]
	if !ok {
		t.Fatal("Batches[1] missing after round-trip")
	}
	wantBatch1 := want.Batches[1]
	if gotBatch1.Slug != wantBatch1.Slug ||
		gotBatch1.StartSHA != wantBatch1.StartSHA ||
		gotBatch1.Kind != wantBatch1.Kind ||
		gotBatch1.SpawnedAt != wantBatch1.SpawnedAt ||
		gotBatch1.Terminal != wantBatch1.Terminal ||
		gotBatch1.Status != wantBatch1.Status ||
		len(gotBatch1.ForkTranscripts) != 1 ||
		gotBatch1.ForkTranscripts[0] != wantBatch1.ForkTranscripts[0] {
		t.Errorf("Batches[1] = %+v; want %+v", gotBatch1, wantBatch1)
	}

	gotBatch2, ok := got.Batches[2]
	if !ok {
		t.Fatal("Batches[2] missing after round-trip")
	}
	wantBatch2 := want.Batches[2]
	if gotBatch2.Kind != wantBatch2.Kind ||
		gotBatch2.StrandGUID != wantBatch2.StrandGUID ||
		gotBatch2.ShuttleRunDir != wantBatch2.ShuttleRunDir ||
		gotBatch2.EventsPath != wantBatch2.EventsPath {
		t.Errorf("Batches[2] = %+v; want %+v", gotBatch2, wantBatch2)
	}
}

// TestState_DigestPersistsAcrossSaveLoad proves BatchState.Digest — the
// field builderengine never persisted — survives a save/load round-trip
// intact, since begin-batch(N+1) depends on reading it back rather than
// re-Distilling a report.
func TestState_DigestPersistsAcrossSaveLoad(t *testing.T) {
	t.Parallel()

	websterDir := t.TempDir()
	digest := &builderengine.Digest{
		Batch:        "01-seam-extensions",
		Status:       builderengine.DigestStatusDone,
		Tests:        "green",
		FilesChanged: 7,
		Dirty:        false,
	}
	want := &websterengine.State{
		RunGUID: "run-1",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug:     "seam-extensions",
				Terminal: true,
				Status:   "done",
				Digest:   digest,
			},
		},
	}

	if err := websterengine.SaveState(websterDir, want); err != nil {
		t.Fatalf("SaveState error = %v; want nil", err)
	}

	got, err := websterengine.LoadState(websterDir)
	if err != nil {
		t.Fatalf("LoadState error = %v; want nil", err)
	}
	if got == nil {
		t.Fatal("LoadState() = nil; want the saved state")
	}

	gotBatch, ok := got.Batches[1]
	if !ok {
		t.Fatal("Batches[1] missing after round-trip")
	}
	if gotBatch.Digest == nil {
		t.Fatal("Batches[1].Digest = nil after round-trip; want the persisted digest")
	}
	if gotBatch.Digest.Batch != digest.Batch ||
		gotBatch.Digest.Status != digest.Status ||
		gotBatch.Digest.Tests != digest.Tests ||
		gotBatch.Digest.FilesChanged != digest.FilesChanged ||
		gotBatch.Digest.Dirty != digest.Dirty {
		t.Errorf("Batches[1].Digest = %+v; want %+v", gotBatch.Digest, digest)
	}
}

func TestState_AbsentFileReturnsNil(t *testing.T) {
	t.Parallel()

	websterDir := t.TempDir()

	got, err := websterengine.LoadState(websterDir)
	if err != nil {
		t.Fatalf("LoadState(absent) error = %v; want nil", err)
	}
	if got != nil {
		t.Errorf("LoadState(absent) = %+v; want nil", got)
	}
}

func TestState_CorruptFileErrors(t *testing.T) {
	t.Parallel()

	websterDir := t.TempDir()
	if err := os.MkdirAll(websterDir, 0o755); err != nil {
		t.Fatalf("mkdir websterDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("write corrupt state.json: %v", err)
	}

	got, err := websterengine.LoadState(websterDir)
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
	websterDir := t.TempDir()

	held, err := websterengine.AcquireStateMutation(websterDir)
	if err != nil {
		t.Fatalf("AcquireStateMutation() error = %v; want nil", err)
	}

	_, locked, err := lock.TryAcquireWriteLock(filepath.Join(websterDir, "mutate.lock"))
	if err != nil {
		t.Fatalf("TryAcquireWriteLock() error = %v; want nil", err)
	}
	if locked {
		t.Fatal("TryAcquireWriteLock() = locked while AcquireStateMutation held the lease; want excluded")
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v; want nil", err)
	}
	second, locked, err := lock.TryAcquireWriteLock(filepath.Join(websterDir, "mutate.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() after Release = locked=%v err=%v; want locked=true, err=nil", locked, err)
	}
	_ = second.Release()
}
