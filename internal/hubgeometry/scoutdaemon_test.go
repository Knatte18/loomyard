// tracedaemon_test.go tests the WorktreeRoot-anchored
// ScoutDaemonStateFile/ScoutDaemonLock accessors on a hand-built
// Layout — pure path arithmetic, no spawning, untagged (Tier 1).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestScoutDaemonStateFile(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "go", "daemon.json")
	if got := l.ScoutDaemonStateFile("go"); got != want {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "go", got, want)
	}
}

func TestScoutDaemonLock(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "go", "daemon.lock")
	if got := l.ScoutDaemonLock("go"); got != want {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "go", got, want)
	}
}

func TestScoutDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.ScoutDaemonStateFile("go")
	pythonPath := l.ScoutDaemonStateFile("python")
	if goPath == pythonPath {
		t.Errorf("ScoutDaemonStateFile(%q) and ScoutDaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestScoutDaemonLock_DistinctPerLanguage(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.ScoutDaemonLock("go")
	pythonPath := l.ScoutDaemonLock("python")
	if goPath == pythonPath {
		t.Errorf("ScoutDaemonLock(%q) and ScoutDaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "scout", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
