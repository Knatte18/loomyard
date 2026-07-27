//go:build integration

// weft_integration_test.go covers weftCommit's composed behavior against real
// git repositories, mirroring buildercli's own weft_integration_test.go. Its
// one scenario pins the CommitWeft error-branch contract: a commit that lands
// but fails its correspondence record must still be reported as
// committed=true alongside the error, never swallowed into a false "no
// commit was made".

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newHostWeftPair builds a hub directory holding a "host" git repo and its
// "host-weft" sibling git repo (each with one commit, so both have a HEAD),
// plus an uncommitted _lyx change in the weft worktree for weftCommit to
// stage, and returns the layout weftCommit resolves the pair from.
func newHostWeftPair(t *testing.T) (*hubgeometry.Layout, string) {
	t.Helper()

	hub := t.TempDir()
	host := filepath.Join(hub, "host")
	weft := filepath.Join(hub, "host-weft")
	for _, dir := range []string{host, weft} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		mustGit(t, dir, "init")
		mustGit(t, dir, "config", "user.name", "Test User")
		mustGit(t, dir, "config", "user.email", "test@example.com")
	}
	commitFile(t, host, "base.txt", "base", "host base commit")
	commitFile(t, weft, "base.txt", "base", "weft base commit")

	// An uncommitted change under the webster pathspec, so CommitWeft has
	// something real to commit.
	lyxDir := filepath.Join(weft, hubgeometry.LyxDirName, "webster")
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir weft _lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lyxDir, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write weft state.json: %v", err)
	}

	return &hubgeometry.Layout{Hub: hub, WorktreeRoot: host, Cwd: host, RelPath: "."}, weft
}

// TestWeftCommit_ReportsCommittedWhenCorrespondenceRecordFails proves the
// CommitWeft error branch passes committed through instead of forcing it to
// false: with a directory squatting on the correspondence index path, the
// weft commit itself lands but RecordCorrespondence fails, and weftCommit
// must report (true, err) -- the commit is real, and CommitWeft's contract
// says the caller gets to know that alongside the error.
func TestWeftCommit_ReportsCommittedWhenCorrespondenceRecordFails(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")
	layout, weft := newHostWeftPair(t)

	// A directory where RecordCorrespondence expects its index file makes
	// the record step fail after the commit has already landed.
	if err := os.MkdirAll(filepath.Join(weft, ".git", "fabric-corrindex.json"), 0o755); err != nil {
		t.Fatalf("squat corrindex path: %v", err)
	}

	committed, err := weftCommit(layout, "corr-fail probe")

	if err == nil {
		t.Fatal("weftCommit() error = nil; want the RecordCorrespondence failure propagated")
	}
	if !committed {
		t.Error("weftCommit() committed = false; want true, the commit landed before the record step failed")
	}

	// The commit must genuinely exist with the webster message stem -- the
	// committed=true report above is about this commit, not a phantom.
	subject := strings.TrimSpace(mustGit(t, weft, "log", "-1", "--format=%s"))
	if subject != "webster: corr-fail probe" {
		t.Errorf("weft HEAD subject = %q; want %q", subject, "webster: corr-fail probe")
	}
}
