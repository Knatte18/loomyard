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

// TestVerify_Scenario is a scenario over one scratch repo, run as named steps in one order, each reaching one rule of Verify: a dirty tree is refused, a record is written only after a pass and only when HEAD did not move during the run, the marker is present only while the command runs, a skip needs a record naming HEAD's tree and the same command, and DirtyPaths names special and renamed paths verbatim.
// The steps share one repo, so the test is parallel as a whole and no step is.
// The steps up to the first pass rely on no record existing yet, so they run before it; each step after it relies on the record the one before left, and the last step relies on being last because it dirties the tree.
func TestVerify_Scenario(t *testing.T) {
	t.Parallel()

	p := newScratch(t)

	if !t.Run("a dirty tree is refused with its paths and runs nothing", func(t *testing.T) {
		stray := filepath.Join(p.Worktree, "stray.txt")
		if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
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
		// Clean again for the steps after this one.
		if err := os.Remove(stray); err != nil {
			t.Fatal(err)
		}
	}) {
		return
	}

	if !t.Run("a failing command writes no record and leaves no marker", func(t *testing.T) {
		res := mustVerify(t, p, "exit 3")
		if res.Status != StatusFailed || res.ExitCode != 3 {
			t.Fatalf("Verify = (%q, %d); want (%q, 3)", res.Status, res.ExitCode, StatusFailed)
		}
		if fileExists(p.Record) {
			t.Error("a record was written after a failure")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a failing command")
		}
	}) {
		return
	}

	if !t.Run("a commit during the run leaves no record", func(t *testing.T) {
		command := "echo c > c.txt && git add c.txt && git commit -q -m mid-run"
		if res := mustVerify(t, p, command); res.Status != StatusPassed {
			t.Fatalf("Verify = %q; want %q", res.Status, StatusPassed)
		}
		if fileExists(p.Record) {
			t.Error("a record was written although HEAD moved during the run")
		}
	}) {
		return
	}

	if !t.Run("a cancelled run leaves no record and no marker, and a pass then writes the record", func(t *testing.T) {
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
			t.Fatalf("Verify after the cancelled run = %q; want %q", res.Status, StatusPassed)
		}
		if !fileExists(p.Record) {
			t.Error("no record after a pass")
		}
		if fileExists(p.Marker) {
			t.Error("the marker survived a passing command")
		}
	}) {
		return
	}

	if !t.Run("a record naming the tree and the command skips the run", func(t *testing.T) {
		if res := mustVerify(t, p, "true"); res.Status != StatusSkipped {
			t.Errorf("second Verify = %q; want %q", res.Status, StatusSkipped)
		}
	}) {
		return
	}

	if !t.Run("a different command runs", func(t *testing.T) {
		if res := mustVerify(t, p, "exit 0"); res.Status != StatusPassed {
			t.Errorf("Verify with a different command = %q; want %q", res.Status, StatusPassed)
		}
	}) {
		return
	}

	if !t.Run("the marker is present while the command runs", func(t *testing.T) {
		seen := filepath.Join(t.TempDir(), "seen")
		mustVerify(t, p, "cp "+p.Marker+" "+seen)
		m, ok, err := ReadMarker(seen)
		if err != nil || !ok {
			t.Fatalf("ReadMarker of the copy = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		if m.Site != "webster verify" {
			t.Errorf("marker site = %q; want %q", m.Site, "webster verify")
		}
	}) {
		return
	}

	if !t.Run("a new commit makes the next verify run", func(t *testing.T) {
		gitkit.CommitFile(t, p.Worktree, "b.txt", "b\n", "second")
		if res := mustVerify(t, p, "true"); res.Status != StatusPassed {
			t.Errorf("Verify after a new commit = %q; want %q", res.Status, StatusPassed)
		}
	}) {
		return
	}

	t.Run("DirtyPaths names special and renamed paths verbatim", func(t *testing.T) {
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
	})
}
