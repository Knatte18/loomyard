// scoutdaemon_test.go tests the WorktreePath-anchored
// ScoutDaemonStateFile/ScoutDaemonLock accessors on a hand-built
// Location — pure path arithmetic, no spawning, untagged (Tier 1).

package lyxcwd

import (
	"path/filepath"
	"testing"
)

func TestScoutDaemonStateFile(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately set to prove the accessor ignores it and
		// stays anchored to WorktreePath.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.WorktreePath(), ".lyx", "scout", "go", "daemon.json")
	if got := l.ScoutDaemonStateFile("go"); got != want {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "go", got, want)
	}
}

func TestScoutDaemonLock(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.WorktreePath(), ".lyx", "scout", "go", "daemon.lock")
	if got := l.ScoutDaemonLock("go"); got != want {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "go", got, want)
	}
}

func TestScoutDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	goPath := l.ScoutDaemonStateFile("go")
	pythonPath := l.ScoutDaemonStateFile("python")
	if goPath == pythonPath {
		t.Errorf("ScoutDaemonStateFile(%q) and ScoutDaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreePath(), ".lyx", "scout", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreePath(), ".lyx", "scout", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("ScoutDaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestScoutDaemonLock_DistinctPerLanguage(t *testing.T) {
	l := &Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    ".",
	}

	goPath := l.ScoutDaemonLock("go")
	pythonPath := l.ScoutDaemonLock("python")
	if goPath == pythonPath {
		t.Errorf("ScoutDaemonLock(%q) and ScoutDaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreePath(), ".lyx", "scout", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreePath(), ".lyx", "scout", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("ScoutDaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
