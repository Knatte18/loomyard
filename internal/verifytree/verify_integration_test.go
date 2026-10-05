//go:build integration

// verify_integration_test.go exercises Verify against a real scratch repo and real shell processes, so it carries the integration tag.

package verifytree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// newScratch returns Paths over a fresh one-commit repo whose verify directory lies outside the worktree.
func newScratch(t *testing.T) Paths {
	t.Helper()
	worktree := t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	return NewPaths(worktree, t.TempDir())
}

func mustVerify(t *testing.T, p Paths, command string) Result {
	t.Helper()
	res, err := Verify(context.Background(), p, Site{Label: "webster verify"}, command)
	if err != nil {
		t.Fatalf("Verify(%q): %v", command, err)
	}
	return res
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestVerify_SkipsWhenRecordNamesTreeAndCommand(t *testing.T) {
	p := newScratch(t)
	if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
		t.Fatalf("first Verify = %q; want %q", res.Status, StatusPassed)
	}
	if res := mustVerify(t, p, "true"); res.Status != StatusSkipped {
		t.Errorf("second Verify = %q; want %q", res.Status, StatusSkipped)
	}
}

func TestVerify_RunsWhenCommandDiffers(t *testing.T) {
	p := newScratch(t)
	mustVerify(t, p, "true")
	if res := mustVerify(t, p, "exit 0"); res.Status != StatusPassed {
		t.Errorf("Verify with a different command = %q; want %q", res.Status, StatusPassed)
	}
}

func TestVerify_RunsWhenTreeDiffers(t *testing.T) {
	p := newScratch(t)
	mustVerify(t, p, "true")
	gitkit.CommitFile(t, p.Worktree, "b.txt", "b\n", "second")
	if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
		t.Errorf("Verify after a new commit = %q; want %q", res.Status, StatusPassed)
	}
}

func TestVerify_DirtyTreeRefusedWithPathsAndNoRun(t *testing.T) {
	p := newScratch(t)
	if err := os.WriteFile(filepath.Join(p.Worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := filepath.Join(t.TempDir(), "ran")
	res := mustVerify(t, p, "touch "+ran)
	if res.Status != StatusDirty {
		t.Fatalf("Verify = %q; want %q", res.Status, StatusDirty)
	}
	if len(res.Dirty) != 1 || res.Dirty[0] != "stray.txt" {
		t.Errorf("Dirty = %v; want [stray.txt]", res.Dirty)
	}
	if fileExists(ran) {
		t.Error("the command ran on a dirty tree")
	}
	if fileExists(p.Record) {
		t.Error("a record was written for a dirty tree")
	}
}

func TestDirtyPaths_NamesSpecialAndRenamedPathsVerbatim(t *testing.T) {
	p := newScratch(t)
	gitkit.Git(t, p.Worktree, "mv", "a.txt", "renamed.txt")
	special := "café -> x.txt"
	if err := os.WriteFile(filepath.Join(p.Worktree, special), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DirtyPaths(p.Worktree)
	if err != nil {
		t.Fatalf("DirtyPaths: %v", err)
	}
	want := []string{"renamed.txt", special}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("DirtyPaths = %q; want %q", got, want)
	}
}

func TestVerify_RecordWrittenOnlyOnPass(t *testing.T) {
	p := newScratch(t)
	res := mustVerify(t, p, "exit 3")
	if res.Status != StatusFailed || res.ExitCode != 3 {
		t.Fatalf("Verify = (%q, %d); want (%q, 3)", res.Status, res.ExitCode, StatusFailed)
	}
	if fileExists(p.Record) {
		t.Error("a record was written after a failure")
	}
	mustVerify(t, p, "true")
	if !fileExists(p.Record) {
		t.Error("no record after a pass")
	}
}

func TestVerify_CommitDuringRunLeavesNoRecord(t *testing.T) {
	p := newScratch(t)
	command := "echo c > c.txt && git add c.txt && git commit -q -m mid-run"
	if res := mustVerify(t, p, command); res.Status != StatusPassed {
		t.Fatalf("Verify = %q; want %q", res.Status, StatusPassed)
	}
	if fileExists(p.Record) {
		t.Error("a record was written although HEAD moved during the run")
	}
}

func TestVerify_CancelledRunLeavesNoRecord(t *testing.T) {
	p := newScratch(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Verify(ctx, p, Site{Label: "Publish"}, "true"); err == nil {
		t.Fatal("Verify with a cancelled ctx returned no error")
	}
	if fileExists(p.Record) {
		t.Error("a record was written by a cancelled run")
	}
	if fileExists(p.Marker) {
		t.Error("the marker survived a cancelled run")
	}
	if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
		t.Errorf("Verify after the cancelled run = %q; want %q", res.Status, StatusPassed)
	}
}

func TestVerify_MarkerGoneAfterPassAndFailure(t *testing.T) {
	p := newScratch(t)
	for _, command := range []string{"true", "exit 1"} {
		mustVerify(t, p, command)
		if fileExists(p.Marker) {
			t.Errorf("marker survived %q", command)
		}
	}
}

func TestVerify_MarkerPresentWhileRunning(t *testing.T) {
	p := newScratch(t)
	seen := filepath.Join(t.TempDir(), "seen")
	mustVerify(t, p, "cp "+p.Marker+" "+seen)
	m, ok, err := ReadMarker(seen)
	if err != nil || !ok {
		t.Fatalf("ReadMarker of the copy = (_, %v, %v); want (_, true, nil)", ok, err)
	}
	if m.Site != "webster verify" {
		t.Errorf("marker site = %q; want %q", m.Site, "webster verify")
	}
}
