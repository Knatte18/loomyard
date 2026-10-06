// auditledger_test.go covers the audit ledger's once-per-identity rule, the parent/fork identity split, ordering of RecordedAuditWarnings, and the pathless-finding refusal.
// Plain t.TempDir() files only — Test Tier Purity Invariant.

package websterengine

import (
	"errors"
	"reflect"
	"strings"
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

//testtiming:keep pins a parent finding's identity carrying its session, so one key under two sessions is two findings, and a fork finding's identity being its bare key; no RecordBatch test records one parent key under two sessions
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

//testtiming:keep pins the order of recorded warnings: batch warnings in batch-list order, not batch number, then run-level warnings last; the covering tests record a single batch
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

func TestAcceptPendingAudit_RefusesPathlessFinding(t *testing.T) {
	t.Parallel()
	st := &State{PendingAuditFindings: []PendingAuditFinding{{ID: "s1/parent:named-spawn:1", Class: "named-spawn", Detail: "d"}}}

	_, _, err := AcceptPendingAudit(nil, st, Geometry{WorktreeRoot: t.TempDir(), PlanDir: t.TempDir(), ScratchDir: t.TempDir()}, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit() error = %v; want ErrAuditNotAcceptable", err)
	}
	if !strings.Contains(err.Error(), "lyx webster run --fresh") {
		t.Errorf("error = %q; want it to name lyx webster run --fresh", err)
	}
	if len(st.PendingAuditFindings) != 1 {
		t.Errorf("PendingAuditFindings = %v; want unchanged", st.PendingAuditFindings)
	}
}
