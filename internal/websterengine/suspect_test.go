//go:build integration

// suspect_test.go exercises the suspect-path check and AcceptPendingAudit's evidence rule over real scratch git repositories.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// suspectFixture is a scratch repo with a tracked file, a git-ignored one, and plan and scratch directories outside it.
type suspectFixture struct {
	geom Geometry
	st   *State
	head string
	// start is the commit before head, recorded as batch 1's StartSHA.
	start string
}

func newSuspectFixture(t *testing.T) *suspectFixture {
	t.Helper()
	root := gitwrapNewScratchRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitwrapMustGit(t, root, "add", ".gitignore")
	gitwrapMustGit(t, root, "commit", "-m", "ignore")
	start := strings.TrimSpace(gitwrapMustGit(t, root, "rev-parse", "HEAD"))
	head := gitwrapCommitFile(t, root, "tracked.txt", "x", "add tracked")
	if err := os.WriteFile(filepath.Join(root, "ignored.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	planDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(planDir, "01-card.md"), []byte("card"), 0o644); err != nil {
		t.Fatal(err)
	}
	geom := Geometry{
		WorktreeRoot: root,
		PlanDir:      planDir,
		WebsterDir:   filepath.Join(root, "_lyx", "webster"),
		ScratchDir:   filepath.Join(root, ".lyx", "webster"),
	}
	st := &State{Batches: map[int]*BatchState{
		1: {Slug: "one", StartSHA: start, Terminal: true, Status: DigestStatusDone, Digest: &Digest{HeadSHA: head}},
	}}
	if err := restampFingerprint(st, planDir, geom.WebsterDir); err != nil {
		t.Fatal(err)
	}
	return &suspectFixture{geom: geom, st: st, head: head, start: start}
}

func TestCheckSuspectPaths(t *testing.T) {
	fx := newSuspectFixture(t)
	root := fx.geom.WorktreeRoot
	tracked := filepath.Join(root, "tracked.txt")

	check := func(t *testing.T, base string, paths ...string) (differing, unverifiable []string) {
		t.Helper()
		d, u, err := checkSuspectPaths(fx.geom, fx.st, base, paths)
		if err != nil {
			t.Fatalf("checkSuspectPaths() error = %v", err)
		}
		return d, u
	}
	expect := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if len(got) == 0 && len(want) == 0 {
			return
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v; want %v", got, want)
		}
	}

	t.Run("unchanged tracked file", func(t *testing.T) {
		d, u := check(t, fx.head, "tracked.txt")
		expect(t, d)
		expect(t, u)
	})
	t.Run("ignored and run-state paths are unverifiable", func(t *testing.T) {
		scratchFile := filepath.Join(fx.geom.ScratchDir, "x.lock")
		d, u := check(t, fx.head, "ignored.log", "_lyx/webster/state.json", scratchFile, "/elsewhere/file")
		expect(t, d)
		expect(t, u, "/elsewhere/file", scratchFile, "_lyx/webster/state.json", "ignored.log")
	})
	t.Run("plan file hash", func(t *testing.T) {
		card := filepath.Join(fx.geom.PlanDir, "01-card.md")
		d, u := check(t, fx.head, card)
		expect(t, d)
		expect(t, u)
		if err := os.WriteFile(card, []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, u = check(t, fx.head, card)
		expect(t, d, card)
		expect(t, u)
	})
	t.Run("empty base", func(t *testing.T) {
		d, u := check(t, "", "tracked.txt")
		expect(t, d)
		expect(t, u, "tracked.txt")
	})
	t.Run("base not in the repository", func(t *testing.T) {
		d, u := check(t, "0123456789abcdef0123456789abcdef01234567", "tracked.txt")
		expect(t, d)
		expect(t, u, "tracked.txt")
	})
	t.Run("untracked new file differs", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, _ := check(t, fx.head, "new.txt")
		expect(t, d, "new.txt")
	})
	t.Run("edited then committed past base differs", func(t *testing.T) {
		if err := os.WriteFile(tracked, []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, _ := check(t, fx.head, "tracked.txt")
		expect(t, d, "tracked.txt")
		gitwrapMustGit(t, root, "commit", "-am", "edit")
		d, _ = check(t, fx.head, "tracked.txt")
		expect(t, d, "tracked.txt")
	})
}

// reversedOrderFixture is newSuspectFixture's repo with batch 02 run before batch 01:
// batch 2 started at c0 and ended at c1, batch 1 started at c1 and ended at c2, where c2 changes tracked.txt.
func reversedOrderFixture(t *testing.T) (fx *suspectFixture, c0, c1, c2 string) {
	t.Helper()
	fx = newSuspectFixture(t)
	root := fx.geom.WorktreeRoot
	c0 = fx.start
	c1 = fx.head
	c2 = gitwrapCommitFile(t, root, "tracked.txt", "changed by batch one", "batch one")
	fx.st.Batches = map[int]*BatchState{
		2: {Slug: "two", StartSHA: c0, Terminal: true, Status: DigestStatusDone, Digest: &Digest{HeadSHA: c1}},
		1: {Slug: "one", StartSHA: c1, Terminal: true, Status: DigestStatusDone, Digest: &Digest{HeadSHA: c2}},
	}
	return fx, c0, c1, c2
}

func TestRunEvidenceBases_PicksByAncestry(t *testing.T) {
	fx, c0, _, c2 := reversedOrderFixture(t)
	root := fx.geom.WorktreeRoot
	fx.st.Batches[integrationBatchKey] = &BatchState{Terminal: true, Digest: &Digest{HeadSHA: "integration"}}
	fx.st.Batches[3] = &BatchState{Slug: "three"}

	got, err := runEvidenceBases(root, fx.st)
	if err != nil {
		t.Fatalf("runEvidenceBases() error = %v", err)
	}
	if got.Start != c0 || got.Last != c2 || len(got.Missing) != 0 {
		t.Errorf("runEvidenceBases() = %+v; want Start %s, Last %s, nothing missing", got, c0, c2)
	}

	missing := "0123456789abcdef0123456789abcdef01234567"
	fx.st.Batches[1].Digest.HeadSHA = missing
	got, err = runEvidenceBases(root, fx.st)
	if err != nil {
		t.Fatalf("runEvidenceBases() error = %v", err)
	}
	if got.Last != "" || !reflect.DeepEqual(got.Missing, []string{missing}) {
		t.Errorf("runEvidenceBases() = %+v; want Last empty and Missing [%s]", got, missing)
	}

	got, err = runEvidenceBases(root, &State{})
	if err != nil || !reflect.DeepEqual(got, evidenceBases{}) {
		t.Errorf("runEvidenceBases(empty) = %+v, %v; want the zero value", got, err)
	}
}

func TestAcceptPendingAudit_RefusesMissingCommit(t *testing.T) {
	fx := newSuspectFixture(t)
	missing := "0123456789abcdef0123456789abcdef01234567"
	fx.st.Batches[1].Digest.HeadSHA = missing
	fx.st.PendingAuditFindings = []PendingAuditFinding{{ID: "f1", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}}}

	_, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit() error = %v; want ErrAuditNotAcceptable", err)
	}
	for _, want := range []string{missing, "git"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q; want it to contain %q", err, want)
		}
	}
	if len(fx.st.PendingAuditFindings) != 1 {
		t.Errorf("PendingAuditFindings = %v; want unchanged", fx.st.PendingAuditFindings)
	}
}

func TestAcceptPendingAudit_UsesExecutionOrderHead(t *testing.T) {
	fx, _, _, _ := reversedOrderFixture(t)
	fx.st.PendingAuditFindings = []PendingAuditFinding{{ID: "f1", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}}}

	got, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if err != nil {
		t.Fatalf("AcceptPendingAudit() error = %v; want accepted against the execution-order head", err)
	}
	if len(got) != 1 || len(fx.st.PendingAuditFindings) != 0 {
		t.Errorf("returned %v, pending %v; want one cleared finding", got, fx.st.PendingAuditFindings)
	}
}

func TestAcceptPendingAudit_ClearsWhenPathsMatchHead(t *testing.T) {
	fx := newSuspectFixture(t)
	fx.st.PendingAuditFindings = []PendingAuditFinding{{ID: "s1/parent:parent-write:1", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}}}

	got, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if err != nil {
		t.Fatalf("AcceptPendingAudit() error = %v", err)
	}
	if len(got) != 1 || len(fx.st.PendingAuditFindings) != 0 {
		t.Errorf("returned %v, pending %v; want one cleared finding and an empty list", got, fx.st.PendingAuditFindings)
	}
	if len(fx.st.AuditDispositions) != 0 {
		t.Errorf("AuditDispositions = %v; want none recorded", fx.st.AuditDispositions)
	}
}

func TestAcceptPendingAudit_RefusesDifferingPath(t *testing.T) {
	fx := newSuspectFixture(t)
	fx.st.PendingAuditFindings = []PendingAuditFinding{
		{ID: "f1", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}},
		{ID: "f2", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}},
	}
	if err := os.WriteFile(filepath.Join(fx.geom.WorktreeRoot, "tracked.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit() error = %v; want ErrAuditNotAcceptable", err)
	}
	for _, want := range []string{"tracked.txt", "git checkout " + fx.head} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q; want it to contain %q", err, want)
		}
	}
	// Two findings naming one path list it once.
	if n := strings.Count(err.Error(), "tracked.txt"); n != 1 {
		t.Errorf("error = %q names tracked.txt %d times; want once", err, n)
	}
	if len(fx.st.PendingAuditFindings) != 2 {
		t.Errorf("PendingAuditFindings = %v; want unchanged", fx.st.PendingAuditFindings)
	}
}

// committedPastHead commits a change to tracked.txt on top of the fixture's head, then restores the worktree file to the head's content.
func committedPastHead(t *testing.T, fx *suspectFixture) {
	t.Helper()
	root := fx.geom.WorktreeRoot
	gitwrapCommitFile(t, root, "tracked.txt", "suspect write", "master commit")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fx.st.PendingAuditFindings = []PendingAuditFinding{{ID: "f1", Class: "parent-write", Detail: "d", Paths: []string{"tracked.txt"}}}
}

func TestAcceptPendingAudit_RefusesCommitPastHead(t *testing.T) {
	fx := newSuspectFixture(t)
	committedPastHead(t, fx)

	_, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit() error = %v; want ErrAuditNotAcceptable", err)
	}
	for _, want := range []string{fx.head, "move HEAD back"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q; want it to contain %q", err, want)
		}
	}
	if len(fx.st.PendingAuditFindings) != 1 {
		t.Errorf("PendingAuditFindings = %v; want unchanged", fx.st.PendingAuditFindings)
	}
}

func TestAcceptPendingAudit_AcceptsAfterHeadReset(t *testing.T) {
	fx := newSuspectFixture(t)
	committedPastHead(t, fx)
	gitwrapMustGit(t, fx.geom.WorktreeRoot, "reset", "--hard", fx.head)

	got, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if err != nil {
		t.Fatalf("AcceptPendingAudit() error = %v; want accepted after the reset", err)
	}
	if len(got) != 1 || len(fx.st.PendingAuditFindings) != 0 {
		t.Errorf("returned %v, pending %v; want one cleared finding", got, fx.st.PendingAuditFindings)
	}
}

func TestAcceptPendingAudit_RefusesUnverifiablePath(t *testing.T) {
	fx := newSuspectFixture(t)
	fx.st.PendingAuditFindings = []PendingAuditFinding{{ID: "f1", Class: "parent-write", Detail: "d", Paths: []string{"ignored.log"}}}

	_, err := AcceptPendingAudit(fx.st, fx.geom, nil)
	if !errors.Is(err, ErrAuditNotAcceptable) {
		t.Fatalf("AcceptPendingAudit() error = %v; want ErrAuditNotAcceptable", err)
	}
	for _, want := range []string{"ignored.log", "run --fresh", fx.start} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q; want it to contain %q", err, want)
		}
	}
}
