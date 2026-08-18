// scoutdaemon_test.go tests the DaemonStateFile/DaemonLock path constructors as told-string path
// math over three fixtures — an unanchored worktree root, a subpath-anchored root, and a plain told
// directory driven with no anchoring type at all — pure path arithmetic, no spawning, untagged
// (Tier 1).

package scoutengine

import (
	"path/filepath"
	"testing"
)

func TestDaemonStateFile(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")

	want := filepath.Join(worktreeRoot, ".lyx", "scout", "go", "daemon.json")
	if got := DaemonStateFile(worktreeRoot, "go"); got != want {
		t.Errorf("DaemonStateFile(%q, %q) = %q; want %q", worktreeRoot, "go", got, want)
	}
}

func TestDaemonLock(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")

	want := filepath.Join(worktreeRoot, ".lyx", "scout", "go", "daemon.lock")
	if got := DaemonLock(worktreeRoot, "go"); got != want {
		t.Errorf("DaemonLock(%q, %q) = %q; want %q", worktreeRoot, "go", got, want)
	}
}

// TestDaemonStateFile_SubpathAnchored proves DaemonStateFile resolves consistently against a
// subpath-anchored root, moving down by the anchored subpath the same way the unanchored fixture
// above does at its root.
func TestDaemonStateFile_SubpathAnchored(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")
	anchorRoot := filepath.Join(worktreeRoot, "backend")

	want := filepath.Join(anchorRoot, ".lyx", "scout", "go", "daemon.json")
	if got := DaemonStateFile(anchorRoot, "go"); got != want {
		t.Errorf("DaemonStateFile(%q, %q) = %q; want %q", anchorRoot, "go", got, want)
	}
}

// TestDaemonLock_SubpathAnchored is DaemonStateFile_SubpathAnchored's DaemonLock counterpart.
func TestDaemonLock_SubpathAnchored(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")
	anchorRoot := filepath.Join(worktreeRoot, "backend")

	want := filepath.Join(anchorRoot, ".lyx", "scout", "go", "daemon.lock")
	if got := DaemonLock(anchorRoot, "go"); got != want {
		t.Errorf("DaemonLock(%q, %q) = %q; want %q", anchorRoot, "go", got, want)
	}
}

// TestDaemonStateFile_ToldDirectory drives DaemonStateFile with a plain told directory that is not
// derived from any anchoring type at all, proving the constructor needs only a string.
func TestDaemonStateFile_ToldDirectory(t *testing.T) {
	anchorRoot := "/var/lib/lyx-standalone/state"

	want := filepath.Join(anchorRoot, ".lyx", "scout", "go", "daemon.json")
	if got := DaemonStateFile(anchorRoot, "go"); got != want {
		t.Errorf("DaemonStateFile(%q, %q) = %q; want %q", anchorRoot, "go", got, want)
	}
}

// TestDaemonLock_ToldDirectory is DaemonStateFile_ToldDirectory's DaemonLock counterpart.
func TestDaemonLock_ToldDirectory(t *testing.T) {
	anchorRoot := "/var/lib/lyx-standalone/state"

	want := filepath.Join(anchorRoot, ".lyx", "scout", "go", "daemon.lock")
	if got := DaemonLock(anchorRoot, "go"); got != want {
		t.Errorf("DaemonLock(%q, %q) = %q; want %q", anchorRoot, "go", got, want)
	}
}

func TestDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")

	goPath := DaemonStateFile(worktreeRoot, "go")
	pythonPath := DaemonStateFile(worktreeRoot, "python")
	if goPath == pythonPath {
		t.Errorf("DaemonStateFile(%q) and DaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(worktreeRoot, ".lyx", "scout", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("DaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(worktreeRoot, ".lyx", "scout", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("DaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestDaemonLock_DistinctPerLanguage(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo-HUB", "repo")

	goPath := DaemonLock(worktreeRoot, "go")
	pythonPath := DaemonLock(worktreeRoot, "python")
	if goPath == pythonPath {
		t.Errorf("DaemonLock(%q) and DaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(worktreeRoot, ".lyx", "scout", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("DaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(worktreeRoot, ".lyx", "scout", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("DaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
