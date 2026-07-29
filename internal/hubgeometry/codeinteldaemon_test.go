// codeinteldaemon_test.go tests the WorktreeRoot-anchored
// CodeintelDaemonStateFile/CodeintelDaemonLock accessors on a hand-built
// Layout — pure path arithmetic, no spawning, untagged (Tier 1).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

func TestCodeintelDaemonStateFile(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "go", "daemon.json")
	if got := l.CodeintelDaemonStateFile("go"); got != want {
		t.Errorf("CodeintelDaemonStateFile(%q) = %q; want %q", "go", got, want)
	}
}

func TestCodeintelDaemonLock(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		// Cwd deliberately differs from WorktreeRoot to prove the accessor
		// ignores Cwd and stays anchored to WorktreeRoot.
		Cwd: filepath.Join("home", "user", "repo", "sub", "dir"),
	}

	want := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "go", "daemon.lock")
	if got := l.CodeintelDaemonLock("go"); got != want {
		t.Errorf("CodeintelDaemonLock(%q) = %q; want %q", "go", got, want)
	}
}

func TestCodeintelDaemonStateFile_DistinctPerLanguage(t *testing.T) {
	// Two different languages must resolve to distinct, non-colliding
	// state files under the same worktree -- this is the one behavior
	// specific to this accessor's per-lang scoping that
	// LoomStatusFile/LoomStatusLock, which are not parameterized, have no
	// equivalent test for.
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.CodeintelDaemonStateFile("go")
	pythonPath := l.CodeintelDaemonStateFile("python")
	if goPath == pythonPath {
		t.Errorf("CodeintelDaemonStateFile(%q) and CodeintelDaemonStateFile(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "go", "daemon.json")
	if goPath != wantGo {
		t.Errorf("CodeintelDaemonStateFile(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "python", "daemon.json")
	if pythonPath != wantPython {
		t.Errorf("CodeintelDaemonStateFile(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}

func TestCodeintelDaemonLock_DistinctPerLanguage(t *testing.T) {
	l := &Layout{
		WorktreeRoot: filepath.Join("home", "user", "repo"),
		Cwd:          filepath.Join("home", "user", "repo"),
	}

	goPath := l.CodeintelDaemonLock("go")
	pythonPath := l.CodeintelDaemonLock("python")
	if goPath == pythonPath {
		t.Errorf("CodeintelDaemonLock(%q) and CodeintelDaemonLock(%q) collided: both %q", "go", "python", goPath)
	}

	wantGo := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "go", "daemon.lock")
	if goPath != wantGo {
		t.Errorf("CodeintelDaemonLock(%q) = %q; want %q", "go", goPath, wantGo)
	}
	wantPython := filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "python", "daemon.lock")
	if pythonPath != wantPython {
		t.Errorf("CodeintelDaemonLock(%q) = %q; want %q", "python", pythonPath, wantPython)
	}
}
