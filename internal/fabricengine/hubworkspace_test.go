package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// mkdirAll creates dir and fails the test on error.
func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// TestHubWorkspacePathAndFolders pins the workspace file path and folder geometry for a root anchor and a nested one.
//
//testtiming:keep the workspace file path and folder geometry for a root and a nested anchor; coverage of its blocks by other tests does not show an assertion of this
func TestHubWorkspacePathAndFolders(t *testing.T) {
	t.Parallel()

	const prime = "loomyard"

	for _, anchor := range []string{".", "wts/some-task"} {
		anchor := anchor
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			l := locationkit.Location(hub, prime, anchor)

			wantDirs := []string{
				filepath.Join(WorktreePath(l, prime), anchor),
				filepath.Join(hub, "_board"),
				filepath.Join(hub, "_portals", anchor),
			}
			for _, d := range wantDirs {
				mkdirAll(t, d)
			}

			file := HubWorkspacePath(l, prime)
			wantFile := filepath.Join(hub, "_launchers", anchor, prime+".code-workspace")
			if file != wantFile {
				t.Fatalf("HubWorkspacePath() = %q; want %q", file, wantFile)
			}

			folders, err := HubWorkspaceFolders(l, prime)
			if err != nil {
				t.Fatalf("HubWorkspaceFolders() error: %v", err)
			}
			wantNames := []string{prime, "_board", "_portals"}
			if len(folders) != len(wantNames) {
				t.Fatalf("got %d folders; want %d", len(folders), len(wantNames))
			}
			for i, f := range folders {
				if f.Name != wantNames[i] {
					t.Errorf("folder %d name = %q; want %q", i, f.Name, wantNames[i])
				}
				if strings.Contains(f.Path, `\`) {
					t.Errorf("folder %d path %q contains a backslash", i, f.Path)
				}
				got := filepath.Join(filepath.Dir(file), filepath.FromSlash(f.Path))
				if got != wantDirs[i] {
					t.Errorf("folder %d resolves to %q; want %q", i, got, wantDirs[i])
				}
			}
		})
	}
}

// TestHubWorkspaceFolders_OmitsMissingDirectories pins that a folder whose target directory does not exist is left out
// while the present ones keep their order.
func TestHubWorkspaceFolders_OmitsMissingDirectories(t *testing.T) {
	t.Parallel()

	const prime = "loomyard"
	hub := t.TempDir()
	l := locationkit.Location(hub, prime, filepath.Join("wts", "some-task"))
	mkdirAll(t, filepath.Join(WorktreePath(l, prime), l.AnchorRel))
	mkdirAll(t, filepath.Join(hub, "_board"))

	folders, err := HubWorkspaceFolders(l, prime)
	if err != nil {
		t.Fatalf("HubWorkspaceFolders() error: %v", err)
	}
	if len(folders) != 2 || folders[0].Name != prime || folders[1].Name != "_board" {
		t.Fatalf("HubWorkspaceFolders() = %+v; want the prime then _board, with the absent _portals anchor omitted", folders)
	}
}

// TestWriteHubWorkspace covers first write, identical rewrite and changed rewrite at both anchors.
func TestWriteHubWorkspace(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(t.TempDir(), "prime", anchor)
			const prime = "prime"
			path := HubWorkspacePath(l, prime)
			first := []byte("{\n  \"folders\": []\n}\n")

			wrote, err := WriteHubWorkspace(l, prime, first)
			if err != nil {
				t.Fatalf("first write: %v", err)
			}
			if !wrote {
				t.Fatalf("first write returned false, want true")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read workspace file: %v", err)
			}
			if string(got) != string(first) {
				t.Fatalf("file bytes = %q, want %q", got, first)
			}
			portals := filepath.Join(l.HubPath, "_portals", l.AnchorRel)
			if info, err := os.Stat(portals); err != nil || !info.IsDir() {
				t.Fatalf("_portals/<AnchorRel> not created at %s: %v", portals, err)
			}

			wrote, err = WriteHubWorkspace(l, prime, first)
			if err != nil {
				t.Fatalf("identical rewrite: %v", err)
			}
			if wrote {
				t.Fatalf("identical rewrite returned true, want false")
			}
			got, _ = os.ReadFile(path)
			if string(got) != string(first) {
				t.Fatalf("bytes changed on identical rewrite: %q", got)
			}

			second := []byte("{\n  \"folders\": [],\n  \"settings\": {}\n}\n")
			wrote, err = WriteHubWorkspace(l, prime, second)
			if err != nil {
				t.Fatalf("changed rewrite: %v", err)
			}
			if !wrote {
				t.Fatalf("changed rewrite returned false, want true")
			}
			got, _ = os.ReadFile(path)
			if string(got) != string(second) {
				t.Fatalf("file bytes = %q, want %q", got, second)
			}
		})
	}
}

// TestWriteHubWorkspace_ReadErrorFailsWithReadCause plants a directory at the workspace file path and asserts the write fails with the read cause.
func TestWriteHubWorkspace_ReadErrorFailsWithReadCause(t *testing.T) {
	t.Parallel()

	l := locationkit.Location(t.TempDir(), "prime", ".")
	mkdirAll(t, HubWorkspacePath(l, "prime"))

	_, err := WriteHubWorkspace(l, "prime", []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "read workspace file") {
		t.Fatalf("WriteHubWorkspace error = %v; want the read cause", err)
	}
}

// TestWriteHubWorkspace_RefusesEscapingLaunchersSymlink plants _launchers as a symlink out of the hub and asserts the write fails with nothing landing at the symlink's target.
func TestWriteHubWorkspace_RefusesEscapingLaunchersSymlink(t *testing.T) {
	t.Parallel()

	l := locationkit.Location(t.TempDir(), "prime", ".")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(l.HubPath, "_launchers")); err != nil {
		t.Fatalf("plant escaping symlink: %v", err)
	}

	if _, err := WriteHubWorkspace(l, "prime", []byte("{}\n")); err == nil {
		t.Fatalf("WriteHubWorkspace succeeded through an escaping _launchers symlink; want an error")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("write leaked outside the hub: %v", entries)
	}
}
