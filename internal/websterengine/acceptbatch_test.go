// acceptbatch_test.go covers AcceptBatchFabricReference: it clears a failed batch's pathless fabric-reference findings only on the HEAD-at-start, clean-tree evidence,
// records each as a batch audit warning, and refuses every other case without mutating the state.
// fakeGit only — Test Tier Purity Invariant.

package websterengine_test

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// acceptAuditVerb is the verb the accept-audit command passes to AcceptBatchFabricReference.
const acceptAuditVerb = "accept-audit --batch"

// fabricEntry is an Uncheckable entry as record-batch writes it for a pathless fabric reference.
const fabricEntry = "fabric-reference: ran a fabric-referencing command (\"go run ./tools/tokencount -history ../x-records\")"

// everyCommandMatches is a RefMatcher that flags every command, so the audit records whatever command it is handed.
type everyCommandMatches struct{}

func (everyCommandMatches) Matches(string) bool { return true }

// readOnlyEntry builds the Uncheckable entry record-batch writes for a pathless fabric reference whose recorded command is cmd, from the audit's own finding text.
func readOnlyEntry(cmd string) string {
	audit := shuttleengine.ForkAudit{ParentBashCommands: []string{cmd}}
	violations := websterengine.CheckParent(audit, "/w/outcome.yaml", "/w/summary.md", "/w", everyCommandMatches{})
	if len(violations) != 1 {
		panic("CheckParent over one flagged command returned " + strconv.Itoa(len(violations)) + " violations; want 1")
	}
	return string(violations[0].Class) + ": " + violations[0].Detail
}

// acceptBatchFixture returns a state whose batch 8 failed on fabricEntry with no commit, and a geometry over g, whose head is the batch's start.
func acceptBatchFixture(t *testing.T) (*websterengine.State, websterengine.Geometry, *fakeGit) {
	t.Helper()
	g := newFakeGit()
	start := g.head
	st := &websterengine.State{Batches: map[int]*websterengine.BatchState{8: {
		StartSHA:    start,
		Terminal:    true,
		Status:      websterengine.DigestStatusFailed,
		Digest:      &websterengine.Digest{Status: websterengine.DigestStatusFailed, HeadSHA: start},
		Uncheckable: []string{fabricEntry},
	}}}
	root := t.TempDir()
	return st, websterengine.Geometry{WorktreeRoot: root, WebsterDir: filepath.Join(root, "_lyx", "webster"), Git: g}, g
}

func TestAcceptBatchFabricReference_ToleratesTheRunsOwnState(t *testing.T) {
	t.Parallel()
	st, geom, g := acceptBatchFixture(t)
	g.dirtyPaths = []string{"_lyx/", "_lyx/webster/state.json"}

	if _, err := websterengine.AcceptBatchFabricReference(st, geom, 8, fabricengine.IsReadOnlyCommand, acceptAuditVerb); err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil over the run's own untracked state", err)
	}
}

func TestAcceptBatchFabricReference_ClearsOnNoCommitCleanTree(t *testing.T) {
	t.Parallel()
	st, geom, _ := acceptBatchFixture(t)

	accepted, err := websterengine.AcceptBatchFabricReference(st, geom, 8, fabricengine.IsReadOnlyCommand, acceptAuditVerb)
	if err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil", err)
	}
	if !slices.Equal(accepted, []string{fabricEntry}) {
		t.Errorf("accepted = %v; want [%s]", accepted, fabricEntry)
	}
	bs := st.Batches[8]
	if len(bs.Uncheckable) != 0 {
		t.Errorf("Uncheckable = %v; want cleared", bs.Uncheckable)
	}
	if len(bs.AuditWarnings) != 1 || bs.AuditWarnings[0].Class != "fabric-reference" || !strings.Contains(bs.AuditWarnings[0].Detail, "tokencount") {
		t.Errorf("AuditWarnings = %+v; want one fabric-reference warning naming the command", bs.AuditWarnings)
	}
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("batch = terminal %v, status %q; want still terminal failed, for recover-batch", bs.Terminal, bs.Status)
	}
}

func TestAcceptBatchFabricReference_ClearsAfterCommitsDiscarded(t *testing.T) {
	t.Parallel()
	st, geom, g := acceptBatchFixture(t)
	start := g.head
	// The batch committed, then the operator moved HEAD back to the batch's start.
	st.Batches[8].Digest.HeadSHA = g.commit()
	g.head = start

	accepted, err := websterengine.AcceptBatchFabricReference(st, geom, 8, fabricengine.IsReadOnlyCommand, acceptAuditVerb)
	if err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil once HEAD is back at the start on a clean tree", err)
	}
	if !slices.Equal(accepted, []string{fabricEntry}) {
		t.Errorf("accepted = %v; want [%s]", accepted, fabricEntry)
	}
}

func TestAcceptBatchFabricReference_ClearsCommittedReadOnly(t *testing.T) {
	t.Parallel()
	st, geom, g := acceptBatchFixture(t)
	entries := []string{readOnlyEntry("cat ../x-records/a.md | grep needle"), readOnlyEntry("ls ../x-records && wc -l ../x-records/b.md")}
	st.Batches[8].Uncheckable = slices.Clone(entries)
	// The batch committed and its commits stay: the start is an ancestor of HEAD.
	g.commit()

	accepted, err := websterengine.AcceptBatchFabricReference(st, geom, 8, fabricengine.IsReadOnlyCommand, acceptAuditVerb)
	if err != nil {
		t.Fatalf("AcceptBatchFabricReference() error = %v; want nil over a committed batch whose commands only read", err)
	}
	if !slices.Equal(accepted, entries) {
		t.Errorf("accepted = %v; want %v", accepted, entries)
	}
	bs := st.Batches[8]
	if len(bs.Uncheckable) != 0 || len(bs.AuditWarnings) != len(entries) {
		t.Fatalf("Uncheckable = %v, AuditWarnings = %+v; want it cleared with one warning per entry", bs.Uncheckable, bs.AuditWarnings)
	}
	for _, w := range bs.AuditWarnings {
		if w.Class != "fabric-reference" || !strings.Contains(w.Detail, "accepted by "+acceptAuditVerb+", ") || !strings.Contains(w.Detail, "commits were kept") {
			t.Errorf("warning = %+v; want a fabric-reference warning naming %s and saying the commits were kept", w, acceptAuditVerb)
		}
	}
}

// committedWith turns the accepting fixture into a batch that committed and recorded entries.
func committedWith(entries ...string) func(st *websterengine.State, g *fakeGit) {
	return func(st *websterengine.State, g *fakeGit) {
		st.Batches[8].Uncheckable = entries
		g.commit()
	}
}

func TestAcceptBatchFabricReference_Refusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// mutate turns the accepting fixture into the refused case.
		mutate func(st *websterengine.State, g *fakeGit)
		batch  int
		want   string
		// verb is the accepting verb; empty means the accept-audit command's.
		verb string
	}{
		{name: "unrecorded batch", batch: 3, want: "batch 03 is not a failed batch"},
		{name: "done batch", batch: 8, mutate: func(st *websterengine.State, _ *fakeGit) { st.Batches[8].Status = websterengine.DigestStatusDone }, want: "is not a failed batch"},
		{name: "other uncheckable entry", batch: 8, mutate: func(st *websterengine.State, _ *fakeGit) {
			st.Batches[8].Uncheckable = append(st.Batches[8].Uncheckable, ".lyx/webster/pause")
		}, want: "not a pathless fabric reference"},
		{name: "no start commit", batch: 8, mutate: func(st *websterengine.State, _ *fakeGit) { st.Batches[8].StartSHA = "" }, want: "recorded no start commit"},
		{name: "HEAD moved past the start on a command that is not a reader", batch: 8, mutate: func(_ *websterengine.State, g *fakeGit) { g.commit() }, want: "reset --to batch-start --batch 08"},
		{name: "committed batch with a redirection", batch: 8, mutate: committedWith(readOnlyEntry("cat a > b")), want: "reset --to batch-start --batch 08"},
		{name: "committed batch with tee", batch: 8, mutate: committedWith(readOnlyEntry("cat a | tee b")), want: "reset --to batch-start --batch 08"},
		{name: "committed batch with sort", batch: 8, mutate: committedWith(readOnlyEntry("cat a | sort")), want: "reset --to batch-start --batch 08"},
		{name: "committed batch with uniq", batch: 8, mutate: committedWith(readOnlyEntry("cat a | uniq")), want: "reset --to batch-start --batch 08"},
		{name: "committed batch with no recorded command", batch: 8, mutate: committedWith("fabric-reference: an older record without the command"), want: "reset --to batch-start --batch 08"},
		{name: "one entry among read-only ones refuses all", batch: 8, mutate: committedWith(readOnlyEntry("cat a"), readOnlyEntry("cat a > b")), want: "is not: "},
		{name: "start is not an ancestor of HEAD", batch: 8, mutate: func(st *websterengine.State, g *fakeGit) {
			g.commit()
			g.parents["unrelated"] = nil
			st.Batches[8].StartSHA = "unrelated"
		}, want: "1) lyx webster reset --to start; 2) lyx webster run"},
		{name: "committed batch on a dirty worktree", batch: 8, mutate: func(st *websterengine.State, g *fakeGit) {
			committedWith(readOnlyEntry("cat a"))(st, g)
			g.dirtyPaths = []string{"internal/x/x.go"}
		}, want: "untracked changes: internal/x/x.go;"},
		{name: "dirty worktree", batch: 8, mutate: func(_ *websterengine.State, g *fakeGit) { g.dirtyPaths = []string{"_lyx/", "internal/x/x.go"} }, want: "untracked changes: internal/x/x.go;"},
		{name: "dirty worktree names the accept-audit re-run", batch: 8, mutate: func(_ *websterengine.State, g *fakeGit) { g.dirtyPaths = []string{"internal/x/x.go"} }, want: `re-run "lyx webster accept-audit --batch 8"`},
		{name: "dirty worktree names the recover-batch re-run", batch: 8, verb: "recover-batch", mutate: func(_ *websterengine.State, g *fakeGit) { g.dirtyPaths = []string{"internal/x/x.go"} }, want: `re-run "lyx webster recover-batch 8"`},
		{name: "committed batch with a redirection names the recover-batch re-run", batch: 8, verb: "recover-batch", mutate: committedWith(readOnlyEntry("cat a > b")), want: `re-run "lyx webster recover-batch 8"`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st, geom, g := acceptBatchFixture(t)
			if tt.mutate != nil {
				tt.mutate(st, g)
			}
			before := slices.Clone(st.Batches[8].Uncheckable)

			verb := cmp.Or(tt.verb, acceptAuditVerb)

			_, err := websterengine.AcceptBatchFabricReference(st, geom, tt.batch, fabricengine.IsReadOnlyCommand, verb)
			if !errors.Is(err, websterengine.ErrAuditNotAcceptable) {
				t.Fatalf("AcceptBatchFabricReference() error = %v; want ErrAuditNotAcceptable", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q; want it to contain %q", err, tt.want)
			}
			if bs := st.Batches[8]; !slices.Equal(bs.Uncheckable, before) || len(bs.AuditWarnings) != 0 {
				t.Errorf("batch 8 mutated: Uncheckable %v, AuditWarnings %v", bs.Uncheckable, bs.AuditWarnings)
			}
		})
	}
}
