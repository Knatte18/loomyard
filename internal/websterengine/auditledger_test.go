// auditledger_test.go covers the audit ledger's once-per-identity rule, the parent/fork identity split, ordering of RecordedAuditWarnings, and the state.json round trip of the ledger fields.
// Plain t.TempDir() files only — Test Tier Purity Invariant.

package websterengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestRecordBatchWarning_RepeatAddsNothing(t *testing.T) {
	t.Parallel()
	st := &State{}
	bs := &BatchState{}

	text, added := recordBatchWarning(st, bs, "id-1", "named-spawn", "spawned x")
	if !added || text != "audit warning (named-spawn): spawned x" {
		t.Fatalf("first record = (%q, %v); want the warning text and added", text, added)
	}
	if _, added := recordBatchWarning(st, bs, "id-1", "named-spawn", "spawned x"); added {
		t.Error("second record of one identity added = true; want false")
	}
	if len(bs.AuditWarnings) != 1 {
		t.Errorf("len(AuditWarnings) = %d; want 1", len(bs.AuditWarnings))
	}
}

func TestFindingIdentity_ParentKeyPerSession(t *testing.T) {
	t.Parallel()
	parent := AuditViolation{Key: "parent:named-spawn:1"}
	a, b := findingIdentity("s1", parent), findingIdentity("s2", parent)
	if a == b {
		t.Errorf("same parent key under two sessions gave one identity %q", a)
	}
	if a != "s1/parent:named-spawn:1" {
		t.Errorf("parent identity = %q; want s1/parent:named-spawn:1", a)
	}

	fork := AuditViolation{TranscriptPath: "t.jsonl", Key: "fork:t.jsonl:nested-agent:1"}
	if got := findingIdentity("s1", fork); got != fork.Key {
		t.Errorf("fork identity = %q; want the bare key", got)
	}
}

func TestRecordFailedFinding_DispositionsWithoutWarning(t *testing.T) {
	t.Parallel()
	st := &State{}
	recordFailedFinding(st, "id-1")
	if !isDispositioned(st, "id-1") {
		t.Error("isDispositioned = false after recordFailedFinding")
	}
	if st.AuditDispositions["id-1"] != dispositionFailed {
		t.Errorf("disposition = %q; want %q", st.AuditDispositions["id-1"], dispositionFailed)
	}
	if len(st.AuditWarnings) != 0 {
		t.Errorf("AuditWarnings = %v; want none", st.AuditWarnings)
	}
}

func TestRecordedAuditWarnings_BatchesThenRunLevel(t *testing.T) {
	t.Parallel()
	st := &State{Batches: map[int]*BatchState{1: {}, 3: {}}}
	recordBatchWarning(st, st.Batches[1], "a", "c1", "d1")
	recordBatchWarning(st, st.Batches[3], "b", "c3", "d3")
	recordRunWarning(st, "r", "cr", "dr")

	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 3, Slug: "three"}}},
		{Cards: []planparser.Card{{Number: 1, Slug: "one"}}},
	}
	want := []string{
		"audit warning (c3): d3",
		"audit warning (c1): d1",
		"audit warning (cr): dr",
	}
	if got := RecordedAuditWarnings(st, batches); !reflect.DeepEqual(got, want) {
		t.Errorf("RecordedAuditWarnings = %v; want %v", got, want)
	}
}

func TestAuditLedger_StateRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	websterDir, scratchDir := filepath.Join(root, "w"), filepath.Join(root, "s")

	want := &State{
		RunGUID:           "g",
		Batches:           map[int]*BatchState{1: {Slug: "one", AuditWarnings: []AuditWarning{{Identity: "i", Class: "c", Detail: "d"}}}},
		AuditDispositions: map[string]string{"i": dispositionWarned},
		AuditWarnings:     []AuditWarning{{Identity: "r", Class: "c", Detail: "d"}},
	}
	if err := SaveState(websterDir, scratchDir, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(websterDir, scratchDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v; want %+v", got, want)
	}

	legacy := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, stateFileName), []byte(`{"runGuid":"g","planFingerprint":"p","currentBatch":0,"batches":{"1":{"slug":"one","startSha":"x","kind":"fork","spawnedAt":"t","terminal":false,"status":""}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := LoadState(legacy, filepath.Join(root, "legacy-scratch"))
	if err != nil {
		t.Fatalf("LoadState legacy: %v", err)
	}
	if old.AuditDispositions != nil || old.AuditWarnings != nil || old.Batches[1].AuditWarnings != nil {
		t.Errorf("legacy state decoded ledger fields non-nil: %+v", old)
	}
}
