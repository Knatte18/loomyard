//go:build integration

// hubworkspace_integration_test.go drives WriteHubWorkspace against real hubs from hubforge.NewHub, at the root anchor and at a nested one.
//
// Package fabricengine_test shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestWriteHubWorkspace covers first write, identical rewrite and changed rewrite at both anchors.
func TestWriteHubWorkspace(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()

			h := hubforge.NewHub(t, anchor)
			l := h.Location
			const prime = "prime"
			path := fabricengine.HubWorkspacePath(l, prime)
			first := []byte("{\n  \"folders\": []\n}\n")

			wrote, err := fabricengine.WriteHubWorkspace(l, prime, first)
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

			wrote, err = fabricengine.WriteHubWorkspace(l, prime, first)
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
			wrote, err = fabricengine.WriteHubWorkspace(l, prime, second)
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

	h := hubforge.NewHub(t, ".")
	l := h.Location
	if err := os.MkdirAll(fabricengine.HubWorkspacePath(l, "prime"), 0o755); err != nil {
		t.Fatalf("plant directory: %v", err)
	}

	_, err := fabricengine.WriteHubWorkspace(l, "prime", []byte("{}\n"))
	if err == nil || !strings.Contains(err.Error(), "read workspace file") {
		t.Fatalf("WriteHubWorkspace error = %v; want the read cause", err)
	}
}

// TestWriteHubWorkspace_RefusesEscapingLaunchersSymlink plants _launchers as a symlink out of the hub and asserts the write fails with nothing landing at the symlink's target.
func TestWriteHubWorkspace_RefusesEscapingLaunchersSymlink(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	l := h.Location
	outside := t.TempDir()

	launchers := filepath.Join(l.HubPath, "_launchers")
	if err := os.RemoveAll(launchers); err != nil {
		t.Fatalf("remove _launchers: %v", err)
	}
	if err := os.Symlink(outside, launchers); err != nil {
		t.Fatalf("plant escaping symlink: %v", err)
	}

	if _, err := fabricengine.WriteHubWorkspace(l, "prime", []byte("{}\n")); err == nil {
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
