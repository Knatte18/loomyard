// resume_test.go covers checkResumable over a temp project directory, a fixture session registry and a fake liveness probe:
// a malformed id, a missing transcript and a live holder refuse,
// while a dead or non-matching holder proceeds, and an absent registry or an undecodable entry proceeds with a warning.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const resumeTestID = "01234567-89ab-4cde-8f01-23456789abcd"

// resumeFixture builds a project dir holding the transcript (when withTranscript) and a registry dir.
func resumeFixture(t *testing.T, withTranscript bool) (projectDir, registryDir string) {
	t.Helper()
	root := t.TempDir()
	projectDir = filepath.Join(root, "project")
	registryDir = filepath.Join(root, "sessions")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withTranscript {
		if err := os.WriteFile(filepath.Join(projectDir, resumeTestID+".jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir, registryDir
}

func writeRegistryEntry(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckResumable(t *testing.T) {
	aliveAll := func(int) bool { return true }
	aliveNone := func(int) bool { return false }

	tests := []struct {
		name        string
		id          string
		transcript  bool
		setup       func(t *testing.T, registryDir string) string // returns the registry dir to use
		alive       func(int) bool
		wantErr     string
		wantWarning bool
	}{
		{
			name:       "malformed id refuses before any file is read",
			id:         "not-a-uuid",
			transcript: true,
			alive:      aliveAll,
			wantErr:    "/status",
		},
		{
			name:       "missing transcript refuses",
			id:         resumeTestID,
			transcript: false,
			alive:      aliveAll,
			wantErr:    "only a session run from this directory",
		},
		{
			name:       "live matching pid refuses",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q}`, resumeTestID))
				return dir
			},
			alive:   aliveAll,
			wantErr: "4242",
		},
		{
			name:       "dead matching pid proceeds silently",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", fmt.Sprintf(`{"pid":4242,"sessionId":%q}`, resumeTestID))
				return dir
			},
			alive: aliveNone,
		},
		{
			name:       "non-matching live entry proceeds",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "a.json", `{"pid":4242,"sessionId":"ffffffff-ffff-4fff-8fff-ffffffffffff"}`)
				return dir
			},
			alive: aliveAll,
		},
		{
			name:       "absent registry directory proceeds with a warning",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "missing")
			},
			alive:       aliveAll,
			wantWarning: true,
		},
		{
			name:       "undecodable entry proceeds with a warning",
			id:         resumeTestID,
			transcript: true,
			setup: func(t *testing.T, dir string) string {
				writeRegistryEntry(t, dir, "bad.json", "{not json")
				return dir
			},
			alive:       aliveAll,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir, registryDir := resumeFixture(t, tt.transcript)
			if tt.setup != nil {
				registryDir = tt.setup(t, registryDir)
			}
			warning, err := checkResumable(tt.id, projectDir, registryDir, tt.alive)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (warning != "") != tt.wantWarning {
				t.Fatalf("warning = %q, wantWarning %v", warning, tt.wantWarning)
			}
		})
	}
}
