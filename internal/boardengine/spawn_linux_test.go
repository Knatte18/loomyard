// spawn_linux_test.go — the registration of the detached board sync child in the owner record, driven with a harmless real child that is reaped by the test.

package boardengine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestSpawnSync_RegistersUnderTheHubOwner covers the owner record of a started board sync: it lands under the hub owner with role sync, and a record directory that cannot be written logs a warning and still spawns the child.
// It is not parallel: it replaces a package variable and captures the process-wide logger.
func TestSpawnSync_RegistersUnderTheHubOwner(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true binary on PATH")
	}
	previous := syncExecutable
	syncExecutable = func() (string, error) { return truePath, nil }
	t.Cleanup(func() { syncExecutable = previous })

	tests := []struct {
		name    string
		blocked bool
	}{
		{name: "RegistersUnderTheHubOwner"},
		{name: "UnwritableRecordDirStillSpawns", blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			board := fabricengine.BoardDir(hub)
			if err := os.MkdirAll(board, 0o755); err != nil {
				t.Fatal(err)
			}
			buf := logcapture.Capture(t)

			if tt.blocked {
				if err := os.MkdirAll(fabricengine.HubScratchDir(hub), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fabricengine.RunProcDir(hub), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					var status syscall.WaitStatus
					_, _ = syscall.Wait4(-1, &status, 0, nil)
				})
				if err := spawnSync(board); err != nil {
					t.Fatalf("spawnSync() = %v; want nil", err)
				}
				if got := strings.Count(buf.String(), "board sync not registered"); got != 1 {
					t.Errorf("log = %q; want one registration warning", buf.String())
				}
				return
			}

			if err := spawnSync(board); err != nil {
				t.Fatalf("spawnSync() = %v; want nil", err)
			}
			ownerDir := filepath.Join(fabricengine.RunProcDir(hub), runproc.HubOwner)
			records, err := filepath.Glob(filepath.Join(ownerDir, "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 {
				t.Fatalf("hub owner holds %d record files; want 1", len(records))
			}
			data, err := os.ReadFile(records[0])
			if err != nil {
				t.Fatal(err)
			}
			var rec runproc.Record
			if err := json.Unmarshal(data, &rec); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				var status syscall.WaitStatus
				_, _ = syscall.Wait4(rec.PID, &status, 0, nil)
			})
			if rec.Role != runproc.RoleSync || rec.PID <= 0 {
				t.Errorf("record = %+v; want role %q and a pid", rec, runproc.RoleSync)
			}
		})
	}
}
