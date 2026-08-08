// scoutdaemon_test.go tests the worktree-anchored DaemonStateFile/DaemonLock path constructors over
// a hand-built *lyxcwd.Location — pure path arithmetic, no spawning, untagged (Tier 1).

package scoutengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestDaemonStateFile(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")
	l := &lyxcwd.Location{HubPath: filepath.Dir(worktreePath), WorktreeName: filepath.Base(worktreePath), AnchorRel: "."}

	want := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.json")
	if got := DaemonStateFile(l, "go"); got != want {
		t.Errorf("DaemonStateFile(%v, %q) = %q; want %q", l, "go", got, want)
	}
}

func TestDaemonLock(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")
	l := &lyxcwd.Location{HubPath: filepath.Dir(worktreePath), WorktreeName: filepath.Base(worktreePath), AnchorRel: "."}

	want := filepath.Join(worktreePath, ".lyx", "scout", "go", "daemon.lock")
	if got := DaemonLock(l, "go"); got != want {
		t.Errorf("DaemonLock(%v, %q) = %q; want %q", l, "go", got, want)
	}
}

func TestDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	worktreePath := filepath.Join("home", "user", "repo-HUB", "repo")
	l := &lyxcwd.Location{HubPath: filepath.Dir(worktreePath), WorktreeName: filepath.Base(worktreePath), AnchorRel: "."}

	goPath := DaemonStateFile(l, "go")
	pythonPath := DaemonStateFile(l, "python")
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
	l := &lyxcwd.Location{HubPath: filepath.Dir(worktreePath), WorktreeName: filepath.Base(worktreePath), AnchorRel: "."}

	goPath := DaemonLock(l, "go")
	pythonPath := DaemonLock(l, "python")
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
