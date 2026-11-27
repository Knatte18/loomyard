// spawn_linux_test.go — tests the registration of the detached board sync child in the owner record.
// It drives a harmless real child that the test reaps.

package boardengine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSpawnSync_RegistersUnderTheHubOwner covers what SpawnSync, the call of every board sync caller, starts:
// the board pusher's command line with no push-now mark, registered under the hub owner with role sync.
// A record directory that cannot be written logs a warning and still spawns the child.
// It is not parallel: it replaces package variables and captures the process-wide logger.
func TestSpawnSync_RegistersUnderTheHubOwner(t *testing.T) {
	tiergate.Require(t, 0)
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true binary on PATH")
	}
	previous := syncExecutable
	syncExecutable = func() (string, error) { return truePath, nil }
	spawnUnderTest = true
	t.Cleanup(func() { syncExecutable, spawnUnderTest = previous, false })

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
			board := fabriclayout.BoardDir(hub)
			if err := os.MkdirAll(board, 0o755); err != nil {
				t.Fatal(err)
			}
			buf := logcapture.Capture(t)

			if tt.blocked {
				if err := os.MkdirAll(fabriclayout.HubScratchDir(hub), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fabriclayout.RunProcDir(hub), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					var status syscall.WaitStatus
					_, _ = syscall.Wait4(-1, &status, 0, nil)
				})
				if err := SpawnSync(board); err != nil {
					t.Fatalf("SpawnSync() = %v; want nil", err)
				}
				if got := strings.Count(buf.String(), "board pusher not registered"); got != 1 {
					t.Errorf("log = %q; want one registration warning", buf.String())
				}
				return
			}

			if err := SpawnSync(board); err != nil {
				t.Fatalf("SpawnSync() = %v; want nil", err)
			}
			ownerDir := filepath.Join(fabriclayout.RunProcDir(hub), runproc.HubOwner)
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
			wantArgs := []string{"loom", "pusher", "--worktree", board, "--board"}
			if len(rec.Argv) == 0 || !slices.Equal(rec.Argv[1:], wantArgs) {
				t.Errorf("record argv = %q; want %q after the binary", rec.Argv, wantArgs)
			}
		})
	}
}
