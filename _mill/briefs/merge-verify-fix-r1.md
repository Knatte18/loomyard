# Verify-Fix Brief

The verify command `go test -tags integration ./internal/ide/` failed after a merge. Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
FAIL	github.com/Knatte18/loomyard/internal/ide [setup failed]
FAIL
# github.com/Knatte18/loomyard/internal/ide
internal\ide\spawn_test.go:117:1: expected declaration, found '}'
```

## Merge Diff

```diff
diff --git a/internal/board/board.go b/internal/board/board.go
index 591ac4b..8a5bec7 100644
--- a/internal/board/board.go
+++ b/internal/board/board.go
@@ -67,7 +67,7 @@ func (b *Board) writeOp(mutate func(*Store) (any, error), _ string) (any, error)
 
 	// (4) Save the store first — tasks.json is the source of truth, persisted
 	// before the derived .md view (so a crash never leaves .md ahead of the data).
-	if err := store.Save(b.boardPath, "tasks.json"); err != nil {
+	if err := store.Save(); err != nil {
 		return nil, fmt.Errorf("save store: %w", err)
 	}
 
diff --git a/internal/board/store.go b/internal/board/store.go
index 0d4dbee..70b3050 100644
--- a/internal/board/store.go
+++ b/internal/board/store.go
@@ -8,13 +8,9 @@
 package board
 
 import (
-	"encoding/json"
 	"fmt"
-	"os"
-	"path/filepath"
 
-	"github.com/Knatte18/loomyard/internal/fsx"
-	flock "github.com/Knatte18/loomyard/internal/lock"
+	"github.com/Knatte18/loomyard/internal/state"
 )
 
 // swapLockSuffix names the fine-grained lock that fences readers of a file
@@ -58,29 +54,15 @@ func (s *Store) Load() error {
 		return nil
 	}
 
-	// Hold a shared swap lock only for the read itself: it overlaps with other
-	// readers but is fenced against a writer's rename, so we never open
-	// tasks.json mid-swap (which on Windows would fail with a sharing violation
-	// and otherwise silently look like an empty wiki).
-	lock, err := flock.AcquireReadLock(s.filePath + swapLockSuffix)
+	// Read via state.ReadJSON, which acquires the swap lock and unmarshals.
+	// It surfaces corruption as an error instead of silently producing an empty list.
+	tasks, found, err := state.ReadJSON[[]Task](s.filePath, s.filePath+swapLockSuffix)
 	if err != nil {
-		return fmt.Errorf("acquire read lock: %w", err)
-	}
-	content, err := os.ReadFile(s.filePath)
-	lock.Release()
-	if err != nil {
-		if os.IsNotExist(err) {
-			s.tasks = []Task{}
-			return nil
-		}
-		// A real read error must surface, not masquerade as an empty wiki.
-		return fmt.Errorf("read %s: %w", s.filePath, err)
+		return fmt.Errorf("load store: %w", err)
 	}
 
-	var tasks []Task
-	err = json.Unmarshal(content, &tasks)
-	if err != nil {
-		// Silent fallback on parse error
+	// If the file does not exist, initialize to empty and return success.
+	if !found {
 		s.tasks = []Task{}
 		return nil
 	}
@@ -96,26 +78,8 @@ func (s *Store) Load() error {
 	return nil
 }
 
-func (s *Store) Save(boardPath, relPath string) error {
-	content, err := json.MarshalIndent(s.tasks, "", "  ")
-	if err != nil {
-		return fmt.Errorf("marshal tasks: %w", err)
-	}
-
-	// Hold the exclusive swap lock across the write so no reader has tasks.json
-	// open during the rename. The body is just a temp-write + rename, so readers
-	// are fenced out for microseconds, not for the surrounding git round-trip.
-	lock, err := flock.AcquireWriteLock(filepath.Join(boardPath, relPath) + swapLockSuffix)
-	if err != nil {
-		return fmt.Errorf("acquire swap lock: %w", err)
-	}
-	defer lock.Release()
-
-	if err := fsx.AtomicWriteBytes(filepath.Join(boardPath, relPath), content); err != nil {
-		return fmt.Errorf("atomic write: %w", err)
-	}
-
-	return nil
+func (s *Store) Save() error {
+	return state.WriteJSON(s.filePath, s.filePath+swapLockSuffix, s.tasks)
 }
 
 func (s *Store) Tasks() []Task {
diff --git a/internal/board/store_test.go b/internal/board/store_test.go
index 5f1d922..b521320 100644
--- a/internal/board/store_test.go
+++ b/internal/board/store_test.go
@@ -6,6 +6,8 @@
 package board_test
 
 import (
+	"os"
+	"path/filepath"
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/board"
@@ -604,6 +606,84 @@ func TestUpsertTasksBatch(t *testing.T) {
 	})
 }
 
+// TestLoadNilDependsOnNormalization verifies that Load normalizes a nil DependsOn
+// to an empty slice and that a missing file yields an empty store with no error.
+//
+// Folds: TestLoadNormalizesNilDependsOn, TestLoadMissingFileReturnsEmpty
+func TestLoadNilDependsOnNormalization(t *testing.T) {
+	t.Run("TestLoadNormalizesNilDependsOn", func(t *testing.T) {
+		tmpDir := t.TempDir()
+		taskPath := filepath.Join(tmpDir, "tasks.json")
+
+		// Write tasks.json with a task that has nil DependsOn
+		err := os.WriteFile(taskPath, []byte(`[{"id":0,"slug":"task1","title":"Task 1"}]`), 0o644)
+		if err != nil {
+			t.Fatalf("failed to write test file: %v", err)
+		}
+
+		store := board.NewStore(taskPath)
+		err = store.Load()
+		if err != nil {
+			t.Fatalf("unexpected error: %v", err)
+		}
+
+		tasks := store.Tasks()
+		if len(tasks) != 1 {
+			t.Fatalf("expected 1 task, got %d", len(tasks))
+		}
+
+		// Verify DependsOn is normalized to an empty slice, not nil
+		if tasks[0].DependsOn == nil {
+			t.Errorf("expected empty slice for DependsOn, got nil")
+		}
+		if len(tasks[0].DependsOn) != 0 {
+			t.Errorf("expected empty DependsOn, got %v", tasks[0].DependsOn)
+		}
+	})
+
+	t.Run("TestLoadMissingFileReturnsEmpty", func(t *testing.T) {
+		tmpDir := t.TempDir()
+		taskPath := filepath.Join(tmpDir, "tasks.json")
+
+		// Do not create the file; test that Load handles missing file gracefully
+		store := board.NewStore(taskPath)
+		err := store.Load()
+		if err != nil {
+			t.Fatalf("expected no error for missing file, got %v", err)
+		}
+
+		tasks := store.Tasks()
+		if len(tasks) != 0 {
+			t.Errorf("expected empty task list for missing file, got %d tasks", len(tasks))
+		}
+	})
+}
+
+// TestLoadCorruptTasksJSON verifies that Load surfaces a corrupt tasks.json
+// as an error instead of silently producing an empty task list.
+func TestLoadCorruptTasksJSON(t *testing.T) {
+	tmpDir := t.TempDir()
+	taskPath := filepath.Join(tmpDir, "tasks.json")
+
+	// Write syntactically corrupt JSON
+	err := os.WriteFile(taskPath, []byte(`{this is not valid json`), 0o644)
+	if err != nil {
+		t.Fatalf("failed to write corrupt test file: %v", err)
+	}
+
+	store := board.NewStore(taskPath)
+	err = store.Load()
+	if err == nil {
+		t.Fatalf("expected error for corrupt tasks.json, got nil")
+	}
+
+	// Verify the error message indicates a load error
+	errMsg := err.Error()
+	if !stringContains(errMsg, "load store") {
+		t.Errorf("expected 'load store' in error, got: %v", err)
+	}
+}
+
 func sliceEqualStrings(a, b []string) bool {
 	if len(a) != len(b) {
 		return false
diff --git a/internal/ide/cli.go b/internal/ide/cli.go
index 7b5caca..0aa952f 100644
--- a/internal/ide/cli.go
+++ b/internal/ide/cli.go
@@ -1,15 +1,13 @@
-// Package ide provides a one-shot VS Code launcher with spawn and interactive menu.
+// Package ide provides a one-shot launcher with spawn and interactive menu
+// for managing worktrees. Spawn assigns a color and launches the worktree.
+// Menu presents an interactive picker over active worktrees.
 //
-// The spawn command generates a worktree's .vscode/ config (only when absent),
-// assigns a title-bar color to the window, registers .vscode/ in the managed
-// .gitignore, and launches VS Code.
+// The spawn command delegates config generation (settings.json, tasks.json,
+// .gitignore registration), color picking, and VS Code launch to internal/vscode.
+// The menu command resolves titles from the board facade.
 //
-// The menu command presents an interactive picker over active worktrees, resolving
-// titles from the board facade.
-//
-// VS Code launch and the menu are Windows-only (POSIX no-ops/errors with a clear
-// message); config generation and color picking are cross-platform. Mill values
-// (palette, settings keys, cmd /c code) are baked — no external Python is read.
+// Spawn and menu are Windows-only (POSIX no-ops/errors with a clear message);
+// cross-platform support is in the delegated vscode package.
 package ide
 
 import (
diff --git a/internal/ide/launch_other.go b/internal/ide/launch_other.go
deleted file mode 100644
index b7edbde..0000000
--- a/internal/ide/launch_other.go
+++ /dev/null
@@ -1,9 +0,0 @@
-//go:build !windows
-
-package ide
-
-// launchCode returns an error on non-Windows platforms (POSIX).
-// VS Code launch is a Windows-only feature; POSIX systems are not supported.
-func launchCode(worktreeDir string) error {
-	return ErrIDEUnsupported
-}
diff --git a/internal/ide/spawn.go b/internal/ide/spawn.go
index 77f4e81..2f6e1d7 100644
--- a/internal/ide/spawn.go
+++ b/internal/ide/spawn.go
@@ -7,18 +7,19 @@ import (
 	"path/filepath"
 
 	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/vscode"
 )
 
 // codeLauncher is a package-level injectable seam that can be overridden in tests.
-// It defaults to launchCode but can be stubbed to record its argument for testing.
-var codeLauncher = launchCode
+// It defaults to vscode.Launch but can be stubbed to record its argument for testing.
+var codeLauncher = vscode.Launch
 
 // Spawn generates a worktree's .vscode/ config (if absent) and launches VS Code.
 //
 // It performs the following steps:
 //  1. Compute worktreeDir := l.WorktreePath(slug)
-//  2. Compute color := pickColor(l)
-//  3. Call writeVSCodeConfig(worktreeDir, l.RelPath, slug, color)
+//  2. Compute color := vscode.PickColor(l)
+//  3. Call vscode.WriteConfig(worktreeDir, l.RelPath, slug, color)
 //  4. Open the worktree at its relpath (dir holding _lyx/ and .vscode/) via codeLauncher
 //
 // Returns an error if any step fails.
@@ -27,10 +28,10 @@ func Spawn(l *paths.Layout, slug string) error {
 	worktreeDir := l.WorktreePath(slug)
 
 	// Compute color for this worktree
-	color := pickColor(l)
+	color := vscode.PickColor(l)
 
 	// Generate VS Code config (settings.json, tasks.json, register in .gitignore)
-	if err := writeVSCodeConfig(worktreeDir, l.RelPath, slug, color); err != nil {
+	if err := vscode.WriteConfig(worktreeDir, l.RelPath, slug, color); err != nil {
 		return err
 	}
 
diff --git a/internal/ide/spawn_test.go b/internal/ide/spawn_test.go
index 87110ee..5df8910 100644
--- a/internal/ide/spawn_test.go
+++ b/internal/ide/spawn_test.go
@@ -114,3 +114,4 @@ func TestSpawn(t *testing.T) {
 		})
 	}
 }
+}
diff --git a/internal/muxpoc/attach.go b/internal/muxpoc/attach.go
index cd00755..50f78cb 100644
--- a/internal/muxpoc/attach.go
+++ b/internal/muxpoc/attach.go
@@ -17,7 +17,7 @@ import (
 func cmdAttach(out io.Writer, cfg Config) int {
 	cwd := cfg.WorktreeRoot
 
-	state, _, err := LoadState(cwd)
+	state, err := LoadState(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("load state: %v", err))
 	}
diff --git a/internal/muxpoc/daemon.go b/internal/muxpoc/daemon.go
index 0757218..6378e61 100644
--- a/internal/muxpoc/daemon.go
+++ b/internal/muxpoc/daemon.go
@@ -54,14 +54,11 @@ func cmdDaemon(out io.Writer, cfg Config) int {
 
 		case <-ticker.C:
 			// Poll for session health
-			state, warn, err := LoadState(cwd)
+			state, err := LoadState(cwd)
 			if err != nil {
 				fmt.Fprintf(os.Stderr, "error loading state: %v (will retry)\n", err)
 				continue
 			}
-			if warn != "" {
-				fmt.Fprintf(os.Stderr, "%s\n", warn)
-			}
 
 			// If no state, nothing to watch
 			if state == nil {
diff --git a/internal/muxpoc/down.go b/internal/muxpoc/down.go
index 8ee3414..d9b584b 100644
--- a/internal/muxpoc/down.go
+++ b/internal/muxpoc/down.go
@@ -18,7 +18,7 @@ import (
 func cmdDown(out io.Writer, cfg Config) int {
 	cwd := cfg.WorktreeRoot
 
-	state, _, err := LoadState(cwd)
+	state, err := LoadState(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("load state: %v", err))
 	}
diff --git a/internal/muxpoc/muxpoc_smoke_test.go b/internal/muxpoc/muxpoc_smoke_test.go
index c2a4536..48587d3 100644
--- a/internal/muxpoc/muxpoc_smoke_test.go
+++ b/internal/muxpoc/muxpoc_smoke_test.go
@@ -207,7 +207,7 @@ func TestSmokeFullLifecycle(t *testing.T) {
 	}
 
 	// Verify state file still exists after crash
-	_, _, err = LoadState(cfg.WorktreeRoot)
+	_, err = LoadState(cfg.WorktreeRoot)
 	if err != nil {
 		t.Fatalf("state should exist after crash: %v", err)
 	}
diff --git a/internal/muxpoc/review.go b/internal/muxpoc/review.go
index e8edc52..e3b2d6c 100644
--- a/internal/muxpoc/review.go
+++ b/internal/muxpoc/review.go
@@ -18,7 +18,7 @@ import (
 func cmdReview(out io.Writer, cfg Config) int {
 	cwd := cfg.WorktreeRoot
 
-	state, _, err := LoadState(cwd)
+	state, err := LoadState(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("load state: %v", err))
 	}
diff --git a/internal/muxpoc/state.go b/internal/muxpoc/state.go
index 80af958..7607efd 100644
--- a/internal/muxpoc/state.go
+++ b/internal/muxpoc/state.go
@@ -10,20 +10,17 @@ package muxpoc
 
 import (
 	"crypto/rand"
-	"encoding/json"
 	"fmt"
 	"os"
 	"path/filepath"
 	"regexp"
 	"strings"
 
-	"github.com/Knatte18/loomyard/internal/fsx"
-	flock "github.com/Knatte18/loomyard/internal/lock"
+	"github.com/Knatte18/loomyard/internal/state"
 )
 
 const (
 	stateRelPath = ".lyx/muxpoc-state.json"
-	lockRelPath  = ".lyx/muxpoc-state.lock"
 )
 
 // Pane represents a single psmux pane in the session.
@@ -42,43 +39,24 @@ type MuxpocState struct {
 }
 
 // LoadState reads the muxpoc state from cwd/.lyx/muxpoc-state.json under a
-// shared read lock. Returns (nil, "", nil) if the file is absent. Returns
-// (nil, "<warn msg>", nil) if the file is corrupt/unparseable (no error returned
-// — treat as no session). Returns (*state, "", nil) on success.
-func LoadState(cwd string) (*MuxpocState, string, error) {
+// shared read lock. Returns (nil, nil) if the file is absent. Returns (nil, error)
+// if the file is corrupt/unparseable or on other read errors. Returns (*state, nil) on success.
+func LoadState(cwd string) (*MuxpocState, error) {
 	statePath := filepath.Join(cwd, stateRelPath)
-	lockPath := filepath.Join(cwd, lockRelPath)
+	lockPath := statePath + ".lock"
 
-	// Ensure parent directory exists so lock file can be created
-	lockDir := filepath.Dir(lockPath)
-	if err := os.MkdirAll(lockDir, 0o755); err != nil {
-		return nil, "", fmt.Errorf("mkdir: %w", err)
-	}
-
-	lock, err := flock.AcquireReadLock(lockPath)
-	if err != nil {
-		return nil, "", fmt.Errorf("acquire read lock: %w", err)
-	}
-	defer lock.Release()
-
-	content, err := os.ReadFile(statePath)
+	v, found, err := state.ReadJSON[MuxpocState](statePath, lockPath)
 	if err != nil {
-		if os.IsNotExist(err) {
-			return nil, "", nil
-		}
-		return nil, "", fmt.Errorf("read state: %w", err)
+		return nil, err
 	}
-
-	var state MuxpocState
-	if err := json.Unmarshal(content, &state); err != nil {
-		return nil, fmt.Sprintf("state file corrupt: %v", err), nil
+	if !found {
+		return nil, nil
 	}
-
-	return &state, "", nil
+	return &v, nil
 }
 
 // SaveState creates .lyx/ if absent, acquires an exclusive write lock on
-// .lyx/muxpoc-state.lock, and writes the state atomically (temp file + rename).
+// .lyx/muxpoc-state.json.lock, and writes the state atomically (temp file + rename).
 // Releases the lock via defer.
 func SaveState(cwd string, s *MuxpocState) error {
 	if s == nil {
@@ -86,30 +64,9 @@ func SaveState(cwd string, s *MuxpocState) error {
 	}
 
 	statePath := filepath.Join(cwd, stateRelPath)
-	lockPath := filepath.Join(cwd, lockRelPath)
-
-	// Create .lyx/ directory if absent
-	lyxDir := filepath.Dir(statePath)
-	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
-		return fmt.Errorf("mkdir .lyx: %w", err)
-	}
-
-	lock, err := flock.AcquireWriteLock(lockPath)
-	if err != nil {
-		return fmt.Errorf("acquire write lock: %w", err)
-	}
-	defer lock.Release()
-
-	content, err := json.MarshalIndent(s, "", "  ")
-	if err != nil {
-		return fmt.Errorf("marshal state: %w", err)
-	}
+	lockPath := statePath + ".lock"
 
-	if err := fsx.AtomicWrite(cwd, stateRelPath, string(content)); err != nil {
-		return fmt.Errorf("atomic write: %w", err)
-	}
-
-	return nil
+	return state.WriteJSON(statePath, lockPath, s)
 }
 
 // DeleteState removes .lyx/muxpoc-state.json. Returns nil if the file is absent.
diff --git a/internal/muxpoc/state_test.go b/internal/muxpoc/state_test.go
index 01822fa..6acc354 100644
--- a/internal/muxpoc/state_test.go
+++ b/internal/muxpoc/state_test.go
@@ -165,14 +165,11 @@ func TestSocketName(t *testing.T) {
 func TestLoadStateMissing(t *testing.T) {
 	tmpDir := t.TempDir()
 
-	state, warn, err := LoadState(tmpDir)
+	state, err := LoadState(tmpDir)
 
 	if state != nil {
 		t.Errorf("expected nil state, got %v", state)
 	}
-	if warn != "" {
-		t.Errorf("expected no warning, got %q", warn)
-	}
 	if err != nil {
 		t.Errorf("expected nil error, got %v", err)
 	}
@@ -193,16 +190,13 @@ func TestLoadStateCorrupt(t *testing.T) {
 		t.Fatalf("failed to write corrupt state: %v", err)
 	}
 
-	state, warn, err := LoadState(tmpDir)
+	state, err := LoadState(tmpDir)
 
 	if state != nil {
 		t.Errorf("expected nil state, got %v", state)
 	}
-	if warn == "" {
-		t.Error("expected non-empty warning")
-	}
-	if err != nil {
-		t.Errorf("expected nil error, got %v", err)
+	if err == nil {
+		t.Error("expected non-nil error")
 	}
 }
 
@@ -227,14 +221,17 @@ func TestSaveLoadRoundtrip(t *testing.T) {
 		t.Fatalf("SaveState failed: %v", err)
 	}
 
+	// Verify lock file location: .lyx/muxpoc-state.json.lock
+	lockPath := filepath.Join(tmpDir, ".lyx", "muxpoc-state.json.lock")
+	if _, err := os.Stat(lockPath); err != nil {
+		t.Errorf("lock file not found at expected location %q: %v", lockPath, err)
+	}
+
 	// Load
-	loaded, warn, err := LoadState(tmpDir)
+	loaded, err := LoadState(tmpDir)
 	if err != nil {
 		t.Fatalf("LoadState failed: %v", err)
 	}
-	if warn != "" {
-		t.Errorf("LoadState returned warning: %q", warn)
-	}
 	if loaded == nil {
 		t.Fatalf("LoadState returned nil state")
 	}
diff --git a/internal/muxpoc/status.go b/internal/muxpoc/status.go
index 2eb27e9..3047649 100644
--- a/internal/muxpoc/status.go
+++ b/internal/muxpoc/status.go
@@ -20,7 +20,7 @@ func cmdStatus(out io.Writer, cfg Config) int {
 	haveState := false
 	var state *MuxpocState
 
-	state, _, err := LoadState(cwd)
+	state, err := LoadState(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("load state: %v", err))
 	}
diff --git a/internal/muxpoc/up.go b/internal/muxpoc/up.go
index 904a757..5efe9df 100644
--- a/internal/muxpoc/up.go
+++ b/internal/muxpoc/up.go
@@ -21,13 +21,10 @@ import (
 func cmdUp(out io.Writer, cfg Config) int {
 	cwd := cfg.WorktreeRoot
 
-	state, warn, err := LoadState(cwd)
+	state, err := LoadState(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("load state: %v", err))
 	}
-	if warn != "" {
-		fmt.Fprintln(os.Stderr, warn)
-	}
 
 	mux := NewPsmuxCmd(cfg)
 
diff --git a/internal/state/state.go b/internal/state/state.go
index 46cfad1..2032f63 100644
--- a/internal/state/state.go
+++ b/internal/state/state.go
@@ -18,15 +18,15 @@ import (
 
 // WriteJSON writes a value as indented JSON to the given path atomically.
 // It creates missing parent directories, acquires an exclusive write lock on
-// path + ".lock", marshals the value to indented JSON (2 spaces), and writes
+// the caller-supplied lockPath, marshals the value to indented JSON (2 spaces), and writes
 // it atomically via fsx.AtomicWriteBytes. The lock is released via defer.
-func WriteJSON[T any](path string, v T) error {
+func WriteJSON[T any](path, lockPath string, v T) error {
 	dir := filepath.Dir(path)
 	if err := os.MkdirAll(dir, 0o755); err != nil {
 		return fmt.Errorf("mkdir: %w", err)
 	}
 
-	l, err := lock.AcquireWriteLock(path + ".lock")
+	l, err := lock.AcquireWriteLock(lockPath)
 	if err != nil {
 		return fmt.Errorf("acquire write lock: %w", err)
 	}
@@ -42,18 +42,18 @@ func WriteJSON[T any](path string, v T) error {
 
 // ReadJSON reads a JSON value from the given path into a value of type T.
 // It creates missing parent directories, acquires a shared read lock on
-// path + ".lock", reads the file, and unmarshals it. Returns (zero, false, nil)
+// the caller-supplied lockPath, reads the file, and unmarshals it. Returns (zero, false, nil)
 // if the file does not exist. Returns (zero, false, err) on other read errors.
 // Returns (zero, false, err) on unmarshal errors (corruption is not swallowed).
 // Returns (value, true, nil) on success. The lock is released via defer.
-func ReadJSON[T any](path string) (T, bool, error) {
+func ReadJSON[T any](path, lockPath string) (T, bool, error) {
 	var zero T
 	dir := filepath.Dir(path)
 	if err := os.MkdirAll(dir, 0o755); err != nil {
 		return zero, false, fmt.Errorf("mkdir: %w", err)
 	}
 
-	l, err := lock.AcquireReadLock(path + ".lock")
+	l, err := lock.AcquireReadLock(lockPath)
 	if err != nil {
 		return zero, false, fmt.Errorf("acquire read lock: %w", err)
 	}
diff --git a/internal/state/state_test.go b/internal/state/state_test.go
index 0c8d236..d62a903 100644
--- a/internal/state/state_test.go
+++ b/internal/state/state_test.go
@@ -19,13 +19,14 @@ type sample struct {
 func TestRoundTrip(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "state.json")
+	lockPath := path + ".lock"
 
 	orig := sample{Name: "test", N: 42}
-	if err := state.WriteJSON(path, orig); err != nil {
+	if err := state.WriteJSON(path, lockPath, orig); err != nil {
 		t.Fatalf("WriteJSON() error: %v", err)
 	}
 
-	got, found, err := state.ReadJSON[sample](path)
+	got, found, err := state.ReadJSON[sample](path, lockPath)
 	if err != nil {
 		t.Fatalf("ReadJSON() error: %v", err)
 	}
@@ -42,8 +43,9 @@ func TestRoundTrip(t *testing.T) {
 func TestMissingFile(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "subdir", "missing.json")
+	lockPath := path + ".lock"
 
-	got, found, err := state.ReadJSON[sample](path)
+	got, found, err := state.ReadJSON[sample](path, lockPath)
 	if err != nil {
 		t.Fatalf("ReadJSON() on missing file error: %v", err)
 	}
@@ -60,7 +62,6 @@ func TestMissingFile(t *testing.T) {
 		t.Errorf("parent directory does not exist: %v", err)
 	}
 
-	lockPath := path + ".lock"
 	if _, err := os.Stat(lockPath); err != nil {
 		t.Errorf("lock file does not exist at %s: %v", lockPath, err)
 	}
@@ -70,13 +71,14 @@ func TestMissingFile(t *testing.T) {
 func TestCorruptFile(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "corrupt.json")
+	lockPath := path + ".lock"
 
 	// Write corrupt JSON.
 	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
 		t.Fatalf("setup: WriteFile() error: %v", err)
 	}
 
-	_, _, err := state.ReadJSON[sample](path)
+	_, _, err := state.ReadJSON[sample](path, lockPath)
 	if err == nil {
 		t.Fatal("ReadJSON() on corrupt file error = nil; want non-nil")
 	}
@@ -87,9 +89,10 @@ func TestCorruptFile(t *testing.T) {
 func TestNoTempLeak(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "state.json")
+	lockPath := path + ".lock"
 
 	v := sample{Name: "test", N: 42}
-	if err := state.WriteJSON(path, v); err != nil {
+	if err := state.WriteJSON(path, lockPath, v); err != nil {
 		t.Fatalf("WriteJSON() error: %v", err)
 	}
 
@@ -126,18 +129,19 @@ func TestNoTempLeak(t *testing.T) {
 func TestOverwrite(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "state.json")
+	lockPath := path + ".lock"
 
 	v1 := sample{Name: "first", N: 1}
-	if err := state.WriteJSON(path, v1); err != nil {
+	if err := state.WriteJSON(path, lockPath, v1); err != nil {
 		t.Fatalf("first WriteJSON() error: %v", err)
 	}
 
 	v2 := sample{Name: "second", N: 2}
-	if err := state.WriteJSON(path, v2); err != nil {
+	if err := state.WriteJSON(path, lockPath, v2); err != nil {
 		t.Fatalf("second WriteJSON() error: %v", err)
 	}
 
-	got, found, err := state.ReadJSON[sample](path)
+	got, found, err := state.ReadJSON[sample](path, lockPath)
 	if err != nil {
 		t.Fatalf("ReadJSON() error: %v", err)
 	}
@@ -153,21 +157,21 @@ func TestOverwrite(t *testing.T) {
 func TestLockFileLocation(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "data.json")
-	expectedLockPath := path + ".lock"
+	lockPath := path + ".lock"
 
 	// Write and read to ensure lock files are created.
 	v := sample{Name: "test", N: 42}
-	if err := state.WriteJSON(path, v); err != nil {
+	if err := state.WriteJSON(path, lockPath, v); err != nil {
 		t.Fatalf("WriteJSON() error: %v", err)
 	}
 
-	if _, _, err := state.ReadJSON[sample](path); err != nil {
+	if _, _, err := state.ReadJSON[sample](path, lockPath); err != nil {
 		t.Fatalf("ReadJSON() error: %v", err)
 	}
 
 	// Verify lock file exists at the expected path.
-	if _, err := os.Stat(expectedLockPath); err != nil {
-		t.Errorf("lock file not found at %s: %v", expectedLockPath, err)
+	if _, err := os.Stat(lockPath); err != nil {
+		t.Errorf("lock file not found at %s: %v", lockPath, err)
 	}
 
 	// Verify the data file and lock are the only files in tmpDir.
@@ -196,9 +200,10 @@ func TestLockFileLocation(t *testing.T) {
 func TestJSONFormatting(t *testing.T) {
 	tmpDir := t.TempDir()
 	path := filepath.Join(tmpDir, "state.json")
+	lockPath := path + ".lock"
 
 	v := sample{Name: "test", N: 42}
-	if err := state.WriteJSON(path, v); err != nil {
+	if err := state.WriteJSON(path, lockPath, v); err != nil {
 		t.Fatalf("WriteJSON() error: %v", err)
 	}
 
diff --git a/internal/ide/color.go b/internal/vscode/color.go
similarity index 91%
rename from internal/ide/color.go
rename to internal/vscode/color.go
index a9f4a16..55458ba 100644
--- a/internal/ide/color.go
+++ b/internal/vscode/color.go
@@ -2,7 +2,7 @@
 // worktree the first unused non-green color, scanning sibling worktrees' VS Code
 // settings so two open worktrees never share a color. Green is reserved for main.
 
-package ide
+package vscode
 
 import (
 	"encoding/json"
@@ -14,8 +14,8 @@ import (
 	"github.com/Knatte18/loomyard/internal/paths"
 )
 
-// ErrIDEUnsupported is returned when ide launch is attempted on an unsupported platform.
-var ErrIDEUnsupported = errors.New("ide launch unsupported on this platform")
+// ErrUnsupported is returned when vscode launch is attempted on an unsupported platform.
+var ErrUnsupported = errors.New("vscode launch unsupported on this platform")
 
 // Color palette (order matters; green is reserved for main).
 var palette = []string{
@@ -32,7 +32,7 @@ var palette = []string{
 // mainColor is the reserved color for the main worktree.
 var mainColor = "#2d7d46"
 
-// pickColor selects an unused non-green color for a child worktree,
+// PickColor selects an unused non-green color for a child worktree,
 // scanning sibling .vscode/settings.json files for existing color assignments.
 //
 // Algorithm:
@@ -42,7 +42,7 @@ var mainColor = "#2d7d46"
 //   - Return the first palette color that is not mainColor and not in use
 //   - If all non-green colors are used, return the first non-green (palette[1])
 //   - If hub/dirs missing, return first non-green
-func pickColor(l *paths.Layout) string {
+func PickColor(l *paths.Layout) string {
 	used := make(map[string]bool)
 
 	// Try to read the hub directory
diff --git a/internal/ide/color_test.go b/internal/vscode/color_test.go
similarity index 99%
rename from internal/ide/color_test.go
rename to internal/vscode/color_test.go
index 1cc6d7d..8671dc8 100644
--- a/internal/ide/color_test.go
+++ b/internal/vscode/color_test.go
@@ -1,7 +1,7 @@
 // color_test.go covers the palette picker, including scanning sibling worktrees'
 // VS Code settings for colors already in use.
 
-package ide
+package vscode
 
 import (
 	"encoding/json"
@@ -133,3 +133,4 @@ func TestPickColor(t *testing.T) {
 		})
 	}
 }
+}
diff --git a/internal/ide/vscode.go b/internal/vscode/config.go
similarity index 84%
rename from internal/ide/vscode.go
rename to internal/vscode/config.go
index baa8485..93649f3 100644
--- a/internal/ide/vscode.go
+++ b/internal/vscode/config.go
@@ -1,7 +1,9 @@
-// vscode.go generates a worktree's .vscode/ settings.json and tasks.json (only
-// when absent) and registers .vscode/ in the lyx-managed .gitignore block.
+// Package vscode generates VS Code configuration and manages VS Code-specific
+// launch behavior for worktrees. It is responsible for config generation (settings.json
+// and tasks.json), color-palette selection, and launching VS Code. The mill values
+// (palette, settings keys, cmd /c code) are baked in — no external Python is read.
 
-package ide
+package vscode
 
 import (
 	"encoding/json"
@@ -11,7 +13,7 @@ import (
 	"github.com/Knatte18/loomyard/internal/gitignore"
 )
 
-// writeVSCodeConfig generates VS Code configuration files in a worktree,
+// WriteConfig generates VS Code configuration files in a worktree,
 // only if they don't already exist (never clobbering operator edits).
 //
 // It writes two files into <worktreeDir>/<relpath>/.vscode/:
@@ -21,7 +23,7 @@ import (
 // After writing, it registers .vscode/ in the managed .gitignore via gitignore.Ensure().
 //
 // Returns an error if I/O fails (but not if files already exist).
-func writeVSCodeConfig(worktreeDir, relpath, slug, color string) error {
+func WriteConfig(worktreeDir, relpath, slug, color string) error {
 	dir := filepath.Join(worktreeDir, relpath)
 	vscodePath := filepath.Join(dir, ".vscode")
 
diff --git a/internal/ide/vscode_test.go b/internal/vscode/config_test.go
similarity index 92%
rename from internal/ide/vscode_test.go
rename to internal/vscode/config_test.go
index e87cfeb..54c1e4e 100644
--- a/internal/ide/vscode_test.go
+++ b/internal/vscode/config_test.go
@@ -1,7 +1,7 @@
-// vscode_test.go covers config generation and its non-clobbering behavior when
+// config_test.go covers config generation and its non-clobbering behavior when
 // .vscode files already exist.
 
-package ide
+package vscode
 
 import (
 	"encoding/json"
@@ -19,9 +19,9 @@ func TestWriteVSCodeConfigCreatesFilesWhenAbsent(t *testing.T) {
 	slug := "test-slug"
 	color := "#2d7d46"
 
-	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
+	err := WriteConfig(worktreeDir, relpath, slug, color)
 	if err != nil {
-		t.Fatalf("writeVSCodeConfig failed: %v", err)
+		t.Fatalf("WriteConfig failed: %v", err)
 	}
 
 	// Check settings.json exists and is valid
@@ -135,10 +135,10 @@ func TestWriteVSCodeConfigDoesNotClobber(t *testing.T) {
 		t.Fatalf("failed to write original tasks.json: %v", err)
 	}
 
-	// Call writeVSCodeConfig
-	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
+	// Call WriteConfig
+	err := WriteConfig(worktreeDir, relpath, slug, color)
 	if err != nil {
-		t.Fatalf("writeVSCodeConfig failed: %v", err)
+		t.Fatalf("WriteConfig failed: %v", err)
 	}
 
 	// Verify settings.json was not modified
@@ -180,9 +180,9 @@ func TestWriteVSCodeConfigRegistersInGitignore(t *testing.T) {
 	slug := "test-slug"
 	color := "#2d7d46"
 
-	err := writeVSCodeConfig(worktreeDir, relpath, slug, color)
+	err := WriteConfig(worktreeDir, relpath, slug, color)
 	if err != nil {
-		t.Fatalf("writeVSCodeConfig failed: %v", err)
+		t.Fatalf("WriteConfig failed: %v", err)
 	}
 
 	// Check .gitignore exists and contains .vscode/
diff --git a/internal/vscode/launch_other.go b/internal/vscode/launch_other.go
new file mode 100644
index 0000000..14ba681
--- /dev/null
+++ b/internal/vscode/launch_other.go
@@ -0,0 +1,9 @@
+//go:build !windows
+
+package vscode
+
+// Launch returns an error on non-Windows platforms (POSIX).
+// VS Code launch is a Windows-only feature; POSIX systems are not supported.
+func Launch(worktreeDir string) error {
+	return ErrUnsupported
+}
diff --git a/internal/ide/launch_windows.go b/internal/vscode/launch_windows.go
similarity index 82%
rename from internal/ide/launch_windows.go
rename to internal/vscode/launch_windows.go
index 3669a7e..78d38d3 100644
--- a/internal/ide/launch_windows.go
+++ b/internal/vscode/launch_windows.go
@@ -1,6 +1,6 @@
 //go:build windows
 
-package ide
+package vscode
 
 import (
 	"fmt"
@@ -10,11 +10,11 @@ import (
 
 const createNoWindow = 0x08000000
 
-// launchCode launches VS Code for the given worktree directory on Windows.
+// Launch launches VS Code for the given worktree directory on Windows.
 //
 // It uses exec.Command to run "cmd /c code <worktreeDir>", which allows PATH resolution
 // of code.cmd and applies the no-console-window flag pattern to prevent flashing.
-func launchCode(worktreeDir string) error {
+func Launch(worktreeDir string) error {
 	cmd := exec.Command("cmd", "/c", "code", worktreeDir)
 
 	// Apply no-console-window flag pattern (from git_windows.go/spawn_windows.go)

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures. Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go test -tags integration ./internal/ide/` after each fix attempt using `git -C C:\Code\loomyard\wts\prune-board-tests` for git commands.
4. Commit each fix attempt with a clear commit message.
5. Self-fix up to `3` times. If the verify command still fails after `3` attempts, stop and report stuck.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success:

{"status":"success","commit_sha":"<last-HEAD-sha>"}

After exhausting fix rounds:

{"status":"stuck","stuck_type":"verify","reason":"<one-line description of what still fails>","commit_sha":"<last-HEAD-sha>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\prune-board-tests` for git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\prune-board-tests`.
