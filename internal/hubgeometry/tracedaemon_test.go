// tracedaemon_test.go tests the WorktreeRoot-anchored
// TraceDaemonStateFile/TraceDaemonLock accessors on a hand-built
// Layout — pure path arithmetic, no spawning, untagged (Tier 1).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestTraceDaemonStateFile(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "go", "daemon.json")
	if got := l.TraceDaemonStateFile("go"); got != want {
		t.Errorf("TraceDaemonStateFile(%q) = %q; want %q", "go", got, want)
	}
}

func TestTraceDaemonLock(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "go", "daemon.lock")
	if got := l.TraceDaemonLock("go"); got != want {
		t.Errorf("TraceDaemonLock(%q) = %q; want %q", "go", got, want)
	}
}

func TestTraceDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	// Two different languages must resolve to distinct, non-colliding
	// state files under the same worktree -- this is the one behavior
	// specific to this accessor's per-lang scoping that
	// LoomStatusFile/LoomStatusLock, which are not parameterized, have no
	// equivalent test for.
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.TraceDaemonStateFile("go")
	pythonPath := l.TraceDaemonStateFile("python")
	if goPath == pythonPath {
		t.Errorf("TraceDaemonStateFile(%q) and TraceDaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("TraceDaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("TraceDaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestTraceDaemonLock_DistinctPerLanguage(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.TraceDaemonLock("go")
	pythonPath := l.TraceDaemonLock("python")
	if goPath == pythonPath {
		t.Errorf("TraceDaemonLock(%q) and TraceDaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("TraceDaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "trace", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("TraceDaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
