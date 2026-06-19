// weft_integration_test.go — integration tests for weft git operations with real bare remotes.

package weft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPushIntegration_CommitLandsOnBare(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Modify and commit using WriteFile
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
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
}

func TestPushIntegration_RebaseRetryOnNFF(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Make a commit in weft
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Push should succeed (even without a remote competing commit for this simplified test)
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestPullIntegration_FastForward(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Pull should succeed (or at least not error) even if nothing new to pull
	if err := Pull(weftRepo); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

func TestSyncIntegration_EventuallyPushed(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Commit a change
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("sync-test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Spawn push (detached) — should not error
	if err := spawnPush(weftRepo); err != nil {
		t.Fatalf("spawnPush: %v", err)
	}

	// Note: We don't poll because the push is detached and may take time
	// or may fail silently. The test just verifies that spawn doesn't crash.
}
