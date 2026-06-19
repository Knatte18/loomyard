// weft_integration_test.go — integration tests for weft git operations with real bare remotes.

package weft

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPushIntegration_CommitLandsOnBare(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	bareRepo := addWeftRemote(t, weftRepo)

	// Modify and commit
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := exec.Command("sh", "-c", "echo 'v1' > "+lyxFile).Run(); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Push
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify commit is in bare repo
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = bareRepo
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if len(output) == 0 {
		t.Fatalf("bare repo should have commits after push")
	}
}

func TestPushIntegration_RebaseRetryOnNFF(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	bareRepo := addWeftRemote(t, weftRepo)

	// Make a commit in weft
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := exec.Command("sh", "-c", "echo 'local' > "+lyxFile).Run(); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Simulate a competing commit in the bare repo by committing directly to bare
	// Create a new clone of bare for the competing commit
	competingRepo := t.TempDir()
	mustRun(t, competingRepo, "git", "clone", bareRepo, ".")

	// Make a competing commit in the competing repo
	competingFile := filepath.Join(competingRepo, "_lyx", "config.yaml")
	if err := exec.Command("sh", "-c", "echo 'remote' > "+competingFile).Run(); err != nil {
		t.Fatalf("failed to modify file in competing repo: %v", err)
	}

	mustRun(t, competingRepo, "git", "add", "-A")
	mustRun(t, competingRepo, "git", "config", "user.email", "test@test.com")
	mustRun(t, competingRepo, "git", "config", "user.name", "Test")
	mustRun(t, competingRepo, "git", "commit", "-m", "competing commit")
	mustRun(t, competingRepo, "git", "push", "origin", "main")

	// Now push from weftRepo should rebase and succeed
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push with rebase: %v", err)
	}

	// Verify the push succeeded
	cmd := exec.Command("git", "log", "--oneline", "-2")
	cmd.Dir = bareRepo
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if len(output) == 0 {
		t.Fatalf("bare repo should have commits after rebase-retry push")
	}
}

func TestPullIntegration_FastForward(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	bareRepo := addWeftRemote(t, weftRepo)

	// Create a new clone to simulate a remote-ahead state
	remoteRepo := t.TempDir()
	mustRun(t, remoteRepo, "git", "clone", bareRepo, ".")

	// Make a commit in the remote repo
	remoteFile := filepath.Join(remoteRepo, "_lyx", "config.yaml")
	if err := exec.Command("sh", "-c", "echo 'remote' > "+remoteFile).Run(); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	mustRun(t, remoteRepo, "git", "add", "-A")
	mustRun(t, remoteRepo, "git", "commit", "-m", "remote commit")
	mustRun(t, remoteRepo, "git", "push", "origin", "main")

	// Pull in weftRepo should fast-forward
	if err := Pull(weftRepo); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Verify the pull succeeded by checking commit count
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = weftRepo
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if len(output) == 0 {
		t.Fatalf("weftRepo should have commits after pull")
	}
}

func TestSyncIntegration_EventuallyPushed(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	bareRepo := addWeftRemote(t, weftRepo)

	// Commit a change
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := exec.Command("sh", "-c", "echo 'sync-test' > "+lyxFile).Run(); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Spawn push (detached) — we need to poll for the push to complete
	if err := spawnPush(weftRepo); err != nil {
		t.Fatalf("spawnPush: %v", err)
	}

	// Poll the bare repo for the commit (bounded retry loop)
	const maxAttempts = 50
	const delayMs = 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = bareRepo
		if output, err := cmd.Output(); err == nil && len(output) > 0 {
			// Check if the commit is the sync commit
			cmd := exec.Command("git", "log", "-1", "--oneline", "--grep=weft sync")
			cmd.Dir = bareRepo
			if output, err := cmd.Output(); err == nil && len(output) > 0 {
				// Found the sync commit
				return
			}
		}

		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	t.Fatalf("detached sync push did not complete within %dms", maxAttempts*delayMs)
}
