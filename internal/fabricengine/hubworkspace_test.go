//go:build tier0

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

// assertNoHubContainers fails the test when the hub holds a _portals or _launchers directory.
func assertNoHubContainers(t *testing.T, hub string) {
	t.Helper()
	for _, name := range []string{"_portals", "_launchers"} {
		if _, err := os.Lstat(filepath.Join(hub, name)); !os.IsNotExist(err) {
			t.Errorf("%s exists in the hub after the write; want absent (stat err: %v)", name, err)
		}
	}
}

// TestHubWorkspacePathAndFolders pins the workspace file path for a root anchor and a nested one.
//
//testtiming:keep the workspace file path for a root and a nested anchor; coverage of its blocks by other tests does not show an assertion of this
func TestHubWorkspacePathAndFolders(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			l := locationkit.Location(hub, "loomyard", anchor)
			l.RepoName = "loomyard"

			got := HubWorkspacePath(l)
			want := filepath.Join(hub, "loomyard.code-workspace")
			if got != want {
				t.Fatalf("HubWorkspacePath() = %q; want %q", got, want)
			}
		})
	}
}

// TestWriteHubWorkspace covers first write, identical rewrite and changed rewrite at both anchors, and that no container directory is created.
func TestWriteHubWorkspace(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(t.TempDir(), "prime", anchor)
			l.RepoName = "prime"
			path := HubWorkspacePath(l)
			first := []byte("{\n  \"folders\": []\n}\n")

			wrote, err := writeHubWorkspace(l, first)
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
			assertNoHubContainers(t, l.HubPath)

			wrote, err = writeHubWorkspace(l, first)
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
			wrote, err = writeHubWorkspace(l, second)
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
			assertNoHubContainers(t, l.HubPath)
		})
	}
}

// TestWriteHubWorkspace_ReadErrorFailsWithReadCause plants a directory at the workspace file path and asserts the write fails with the read cause.
func TestWriteHubWorkspace_ReadErrorFailsWithReadCause(t *testing.T) {
	t.Parallel()

	l := locationkit.Location(t.TempDir(), "prime", ".")
	l.RepoName = "prime"
	mkdirAll(t, HubWorkspacePath(l))

	_, err := writeHubWorkspace(l, []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "read workspace file") {
		t.Fatalf("writeHubWorkspace error = %v; want the read cause", err)
	}
	assertNoHubContainers(t, l.HubPath)
}

// TestWriteHubWorkspace_RefusesEscapingSymlink plants a symlink out of the hub at the workspace file path and asserts the write fails with nothing landing at the symlink's target.
func TestWriteHubWorkspace_RefusesEscapingSymlink(t *testing.T) {
	t.Parallel()

	l := locationkit.Location(t.TempDir(), "prime", ".")
	l.RepoName = "prime"
	outside := t.TempDir()
	if err := os.Symlink(outside, HubWorkspacePath(l)); err != nil {
		t.Fatalf("plant escaping symlink: %v", err)
	}

	if _, err := writeHubWorkspace(l, []byte("{}\n")); err == nil {
		t.Fatalf("writeHubWorkspace succeeded through an escaping symlink; want an error")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("write leaked outside the hub: %v", entries)
	}
}
