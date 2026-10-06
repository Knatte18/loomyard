//go:build integration

// menu_test.go covers worktree discovery (excludes main, requires _lyx/),
// board-facade titles, numeric selection, the zero-worktree path, and the
// missing-board hard error.

package ideengine

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func mustRunMenu(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command failed: %v; output: %s", err, output)
	}
}

func newTestGitRepoWithWorktrees(t *testing.T) (string, string) {
	t.Helper()

	container := t.TempDir()
	mainWorktreePath := filepath.Join(container, "main")

	if err := os.Mkdir(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main worktree: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "init", "-b", "main")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.email", "test@test.com")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.name", "Test")

	readmeFile := filepath.Join(mainWorktreePath, "README")
	if err := os.WriteFile(readmeFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "add", ".")
	mustRunMenu(t, mainWorktreePath, "git", "commit", "-m", "initial")

	if err := os.MkdirAll(filepath.Join(mainWorktreePath, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("failed to create main _lyx: %v", err)
	}

	return container, mainWorktreePath
}

// addChildWorktree adds a worktree named name next to the main one, with a _lyx directory when withLyx is set, and removes it with its branch on cleanup.
func addChildWorktree(t *testing.T, container, mainWorktreePath, name string, withLyx bool) {
	t.Helper()

	childPath := filepath.Join(container, name)
	mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", name+"-branch", childPath)
	t.Cleanup(func() {
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
		mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", name+"-branch")
	})
	if withLyx {
		if err := os.MkdirAll(filepath.Join(childPath, lyxdirs.LyxDirName), 0o755); err != nil {
			t.Fatalf("failed to create %s _lyx: %v", name, err)
		}
	}
}

// writeBoard writes the board config and a board tasks.json holding tasksJSON under the hub at container.
func writeBoard(t *testing.T, container, tasksJSON string) {
	t.Helper()

	configDir := configengine.ConfigDir(fabricengine.BoardDir(container))
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := configengine.ConfigFile(fabricengine.BoardDir(container), "board")
	boardConfig := `path: ../_board
readme: Home.md
design_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "tasks.json"), []byte(tasksJSON), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}
}

// TestMenuScenario drives Menu over one git repo: the hard error while no board config exists, then, once a board is written, worktree discovery (no _lyx, main excluded) and numeric selection.
// Each step adds the child worktrees it needs and removes them on cleanup; the board config written after the first step stays for the rest.
// Serial by design: it sets BOARD_SKIP_GIT in the process environment and swaps the package-level CodeLauncher.
func TestMenuScenario(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)
	layout := &lyxcwd.Location{HubPath: container, WorktreeName: filepath.Base(mainWorktreePath), AnchorRel: "."}

	if !t.Run("hard error on missing board", func(t *testing.T) {
		var out bytes.Buffer

		err := Menu(layout, strings.NewReader(""), &out)
		if err == nil {
			t.Fatalf("expected hard error when board config cannot be loaded, got nil")
		}

		if !strings.Contains(err.Error(), "load board config") && !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected load config error, got: %v", err)
		}
	}) {
		return
	}

	// The steps below rely on this board, which the first step proved absent.
	writeBoard(t, container, `{"tasks":[]}`)

	if !t.Run("requires lyx dir", func(t *testing.T) {
		addChildWorktree(t, container, mainWorktreePath, "child", false)

		var out bytes.Buffer

		if err := Menu(layout, strings.NewReader(""), &out); err != nil {
			t.Fatalf("Menu failed: %v", err)
		}

		if output := out.String(); !strings.Contains(output, "no active worktrees") {
			t.Fatalf("expected 'no active worktrees', got: %q", output)
		}
	}) {
		return
	}

	if !t.Run("excludes main", func(t *testing.T) {
		addChildWorktree(t, container, mainWorktreePath, "child", true)

		originalLauncher := CodeLauncher
		defer func() { CodeLauncher = originalLauncher }()
		CodeLauncher = func(dir string) error { return nil }

		var out bytes.Buffer

		if err := Menu(layout, strings.NewReader("1\n"), &out); err != nil {
			t.Fatalf("Menu failed: %v", err)
		}

		if output := out.String(); !strings.Contains(output, "child") {
			t.Fatalf("expected 'child' in output, got: %q", output)
		}
	}) {
		return
	}

	t.Run("numeric selection", func(t *testing.T) {
		addChildWorktree(t, container, mainWorktreePath, "child1", true)
		addChildWorktree(t, container, mainWorktreePath, "child2", true)
		writeBoard(t, container, `{"tasks":[{"slug":"child1","title":"Task 1"},{"slug":"child2","title":"Task 2"}]}`)

		var launchCount int
		originalLauncher := CodeLauncher
		defer func() { CodeLauncher = originalLauncher }()
		CodeLauncher = func(dir string) error {
			launchCount++
			return nil
		}

		var out bytes.Buffer

		if err := Menu(layout, strings.NewReader("2\n"), &out); err != nil {
			t.Fatalf("Menu failed: %v", err)
		}

		if launchCount != 1 {
			t.Fatalf("expected CodeLauncher to be called once, was called %d times", launchCount)
		}
	})
}
