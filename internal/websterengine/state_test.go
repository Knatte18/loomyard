// state_test.go covers LoadState/SaveState's documented cases: round-tripping a populated State
// through disk (including a persisted digest and per-card SHA trail), an absent state.json
// returning (nil, nil), a corrupt state.json returning a
// wrapped error rather than a guessed value, and the state-mutation lease's cross-holder exclusion.
// It also covers webster's own durable-vs-scratch dir split: SaveState writes state.json into the
// durable dir and state.json.lock into a DISTINCT scratch dir, the durable dir ends up with no
// .lock file at all, LoadState round-trips across the two dirs, and AcquireStateMutation's lease
// file lands in the scratch dir.
// All plain t.TempDir() files — no git, no subprocess spawns — Test Tier Purity Invariant.

package websterengine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// scratchSibling builds a durable dir and a DISTINCT scratch dir under one
// shared temp base, mirroring the .lyx/_lyx mirrored-subpath split in
// production, rather than reusing one temp dir for both — the split this
// test file exists to pin would otherwise go untested.
func scratchSibling(t *testing.T) (websterDir, scratchDir string) {
	t.Helper()
	base := t.TempDir()
	return filepath.Join(base, "_lyx", "webster"), filepath.Join(base, ".lyx", "webster")
}

// TestState_RoundTrip pins that a populated State survives a save/load across the durable and
// scratch dirs: the verify gate's pre-fix head, the per-card SHA trail, the persisted digest
// (begin-batch(N+1) reads it back rather than re-distilling a report) and the audit ledger included.
//
//testtiming:keep pins every State and BatchState field surviving save and load, and the lock landing in the scratch dir only; the run-level test round-trips only the fields its run sets
func TestState_RoundTrip(t *testing.T) {
	t.Parallel()

	websterDir, scratchDir := scratchSibling(t)
	want := &websterengine.State{
		RunGUID:         "run-1",
		PlanFingerprint: "abc123",
		CurrentBatch:    2,
		MasterStrand:    "master-strand-1",
		MasterSessionID: "session-1",
		PreFixHead:      "cafef00d",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug:      "first",
				StartSHA:  "deadbeef",
				Kind:      "fork",
				SpawnedAt: "2026-07-11T12:00:00Z",
				Terminal:  true,
				Status:    "done",
				Digest: &websterengine.Digest{
					Batch:      "01-seam-extensions",
					Status:     websterengine.DigestStatusDone,
					HeadSHA:    "deadbeef",
					Deviations: []string{"internal/extra.go"},
				},
				CardSHAs:        []string{"deadbeef"},
				ForkTranscripts: []string{"subagents/abc.jsonl"},
				AuditWarnings:   []websterengine.AuditWarning{{Identity: "i", Class: "c", Detail: "d"}},
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
		SeenForkTranscripts: []string{"subagents/abc.jsonl"},
		AuditDispositions:   map[string]string{"i": "warned"},
		AuditWarnings:       []websterengine.AuditWarning{{Identity: "r", Class: "c", Detail: "d"}},
	}

	if err := websterengine.SaveState(websterDir, scratchDir, want); err != nil {
		t.Fatalf("SaveState error = %v; want nil", err)
	}

	if _, err := os.Stat(filepath.Join(websterDir, "state.json")); err != nil {
		t.Errorf("state.json missing from durable dir %s: %v", websterDir, err)
	}
	if _, err := os.Stat(filepath.Join(scratchDir, "state.json.lock")); err != nil {
		t.Errorf("state.json.lock missing from scratch dir %s: %v", scratchDir, err)
	}
	entries, err := os.ReadDir(websterDir)
	if err != nil {
		t.Fatalf("ReadDir(websterDir) error = %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".lock" {
			t.Errorf("durable dir %s contains a .lock file %q; want every lock relocated to the scratch dir", websterDir, e.Name())
		}
	}

	got, err := websterengine.LoadState(websterDir, scratchDir)
	if err != nil {
		t.Fatalf("LoadState error = %v; want nil", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadState() = %+v; want %+v", got, want)
	}
}

// TestLoadState_UnusualFiles pins LoadState on a state.json that is absent (nil, nil), corrupt (a wrapped error, never a guessed value) or written before the integration stage was retired (the retired integrationFix and assertedModel fields are ignored, every other field loads intact and a save writes no assertedModel key, the reserved -1 batch record stays an ordinary entry, and the audit-ledger fields it predates decode as nil).
func TestLoadState_UnusualFiles(t *testing.T) {
	t.Parallel()

	const legacy = `{
  "runGuid": "g",
  "masterStrand": "master-strand-1",
  "assertedModel": "opus",
  "batches": {
    "1": {"slug": "alpha", "kind": "fork", "terminal": true, "status": "done"},
    "-1": {"terminal": true, "status": "stuck"}
  },
  "integrationFix": {"preFixHead": "abc", "strandGuid": "fix-strand", "spawnedAt": "2026-01-01T00:00:00Z", "result": "failed"}
}`
	const retiredStartup = `{
  "runGuid": "g",
  "partition": [{"cards": ["01-alpha"], "profile": "cautious", "estimate": 70000, "breakdown": {"weights": {"startup_context": 60000, "fork_messages": 4}, "startup": 61200, "read_union": 0, "cards": []}}]
}`
	tests := []struct {
		name string
		// content is the state.json body; absent leaves the file out.
		content string
		absent  bool
		wantErr bool
		// wantLegacyBatches asserts batches 1 and -1 loaded.
		wantLegacyBatches bool
		// wantRetiredStartup asserts the partition's breakdown decoded its retired startup_context weight.
		wantRetiredStartup bool
	}{
		{name: "absent file returns nil", absent: true},
		{name: "corrupt file errors", content: "not valid json{{{", wantErr: true},
		{name: "legacy integration records still load", content: legacy, wantLegacyBatches: true},
		{name: "a breakdown recorded with the retired startup_context weight still loads", content: retiredStartup, wantRetiredStartup: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			websterDir, scratchDir := scratchSibling(t)
			if !tt.absent {
				if err := os.MkdirAll(websterDir, 0o755); err != nil {
					t.Fatalf("mkdir websterDir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(websterDir, "state.json"), []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write state.json: %v", err)
				}
			}

			got, err := websterengine.LoadState(websterDir, scratchDir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadState error = nil; want error")
				}
				if got != nil {
					t.Errorf("LoadState = %+v; want nil on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadState error = %v; want nil", err)
			}
			if tt.wantRetiredStartup {
				if got == nil || len(got.Partition) != 1 || got.Partition[0].Breakdown == nil || got.Partition[0].Breakdown.Weights.RetiredStartupContext != 60000 {
					t.Errorf("LoadState = %+v; want the partition breakdown's retired startup_context weight 60000 decoded", got)
				}
				return
			}
			if !tt.wantLegacyBatches {
				if got != nil {
					t.Errorf("LoadState = %+v; want nil", got)
				}
				return
			}
			if got == nil || got.Batches[1] == nil || got.Batches[-1] == nil {
				t.Fatalf("LoadState = %+v; want batches 1 and -1 loaded", got)
			}
			if got.AuditDispositions != nil || got.AuditWarnings != nil || got.Batches[1].AuditWarnings != nil {
				t.Errorf("LoadState = %+v; want the audit-ledger fields nil", got)
			}
			if got.RunGUID != "g" || got.MasterStrand != "master-strand-1" {
				t.Errorf("LoadState = %+v; want RunGUID %q and MasterStrand %q intact beside the retired assertedModel", got, "g", "master-strand-1")
			}
			if err := websterengine.SaveState(websterDir, scratchDir, got); err != nil {
				t.Fatalf("SaveState error = %v; want nil", err)
			}
			saved, err := os.ReadFile(filepath.Join(websterDir, "state.json"))
			if err != nil {
				t.Fatalf("read saved state.json: %v", err)
			}
			if strings.Contains(string(saved), "assertedModel") {
				t.Errorf("saved state.json = %s; want no assertedModel key", saved)
			}
		})
	}
}

// TestAcquireStateMutation_ExcludesSecondHolder proves the state-mutation lease is a real
// cross-holder exclusive lock: while held, a second non-blocking acquire of the same lease file
// fails, and after Release it succeeds — the property every verb's load-mutate-save section relies
// on.
//
//testtiming:keep pins the lease's cross-holder exclusion and release directly; the covering run-level test only observes ErrRunBusy
func TestAcquireStateMutation_ExcludesSecondHolder(t *testing.T) {
	scratchDir := t.TempDir()

	held, err := websterengine.AcquireStateMutation(scratchDir)
	if err != nil {
		t.Fatalf("AcquireStateMutation() error = %v; want nil", err)
	}

	_, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, "mutate.lock"))
	if err != nil {
		t.Fatalf("TryAcquireWriteLock() error = %v; want nil", err)
	}
	if locked {
		t.Fatal("TryAcquireWriteLock() = locked while AcquireStateMutation held the lease; want excluded")
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v; want nil", err)
	}
	second, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, "mutate.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() after Release = locked=%v err=%v; want locked=true, err=nil", locked, err)
	}
	_ = second.Release()
}

// TestRunActive_ReflectsRunLockHeld proves the ownerless-run probe: an unheld run.lock reads as no
// live run (and the probe releases what it briefly acquired, so a real run right after is never
// blocked), while a held run.lock reads as a live run owning the state — the signal the bracket
// verbs' zombie-Master warning keys off (round fable-r1's F17).
//
//testtiming:keep pins RunActive's not-active result releasing the probed lock and its active result while the lock is held; the plan-reset tests only observe the busy refusal
func TestRunActive_ReflectsRunLockHeld(t *testing.T) {
	scratchDir := t.TempDir()

	active, err := websterengine.RunActive(scratchDir)
	if err != nil {
		t.Fatalf("RunActive() on a free lock error = %v; want nil", err)
	}
	if active {
		t.Error("RunActive() = true with no run.lock held; want false")
	}

	// The probe must have released the lock it briefly took, so it is
	// acquirable now.
	held, locked, err := lock.TryAcquireWriteLock(filepath.Join(scratchDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("run.lock acquire after probe = locked=%v err=%v; want the probe to have released it", locked, err)
	}

	active, err = websterengine.RunActive(scratchDir)
	if err != nil {
		t.Fatalf("RunActive() on a held lock error = %v; want nil", err)
	}
	if !active {
		t.Error("RunActive() = false while run.lock is held; want true (a live run owns the state)")
	}

	_ = held.Release()
}
