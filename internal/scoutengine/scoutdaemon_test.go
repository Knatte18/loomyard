// scoutdaemon_test.go tests the worktree-anchored DaemonStateFile/DaemonLock path constructors over
// a plain worktree path — pure path arithmetic, no spawning, untagged (Tier 1).

package scoutengine

import (
	"path/filepath"
	"testing"
)

func TestDaemonStateFile(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")

	want := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.json")
	if got := DaemonStateFile(worktreePath, "go"); got != want {
		t.Errorf("DaemonStateFile(%q, %q) = %q; want %q", worktreePath, "go", got, want)
	}
}

func TestDaemonLock(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")

	want := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.lock")
	if got := DaemonLock(worktreePath, "go"); got != want {
		t.Errorf("DaemonLock(%q, %q) = %q; want %q", worktreePath, "go", got, want)
	}
}

func TestDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")

	goPath := DaemonStateFile(worktreePath, "go")
	pythonPath := DaemonStateFile(worktreePath, "python")
	if goPath == pythonPath {
		t.Errorf("DaemonStateFile(%q) and DaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("DaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(worktreePath, ".lyx", "scout", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("DaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestDaemonLock_DistinctPerLanguage(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")

	goPath := DaemonLock(worktreePath, "go")
	pythonPath := DaemonLock(worktreePath, "python")
	if goPath == pythonPath {
		t.Errorf("DaemonLock(%q) and DaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("DaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(worktreePath, ".lyx", "scout", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("DaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
