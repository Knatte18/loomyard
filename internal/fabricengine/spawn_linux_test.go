// spawn_linux_test.go — the registration of the detached push child in the owner record, driven with a harmless real child that is reaped by the test.

package fabricengine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestStartDetachedPush_RegistersTheChild covers the owner record of a started push child: a pair request registers under its teardown slug, a prime request under the hub owner, and a record directory that cannot be written logs a warning and still starts the child.
// It is not parallel: it captures the process-wide logger.
func TestStartDetachedPush_RegistersTheChild(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true binary on PATH")
	}

	tests := []struct {
		name      string
		req       PushSpawn
		wantOwner string
		blocked   bool
	}{
		{name: "PairPushOwnsUnderItsSlug", req: PushSpawn{WeftPath: "/weft", PauseSlug: "task", TeardownSlug: "task"}, wantOwner: "task"},
		{name: "PrimePushOwnsUnderTheHubOwner", req: PushSpawn{WeftPath: "/weft", PauseSlug: "gate"}, wantOwner: runproc.HubOwner},
		{name: "UnwritableRecordDirStillStartsTheChild", req: PushSpawn{WeftPath: "/weft", TeardownSlug: "task"}, blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			board := BoardDir(hub)
			if err := os.MkdirAll(board, 0o755); err != nil {
				t.Fatal(err)
			}
			req := tt.req
			req.WorkDir = board
			buf := logcapture.Capture(t)

			if tt.blocked {
				if err := os.MkdirAll(HubScratchDir(hub), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(RunProcDir(hub), nil, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					var status syscall.WaitStatus
					_, _ = syscall.Wait4(-1, &status, 0, nil)
				})
				if err := startDetachedPush(truePath, req); err != nil {
					t.Fatalf("startDetachedPush() = %v; want nil", err)
				}
				if !strings.Contains(buf.String(), "detached push not registered") {
					t.Errorf("log = %q; want the registration warning", buf.String())
				}
				return
			}

			if err := startDetachedPush(truePath, req); err != nil {
				t.Fatalf("startDetachedPush() = %v; want nil", err)
			}
			records, err := filepath.Glob(filepath.Join(RunProcDir(hub), tt.wantOwner, "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 {
				t.Fatalf("owner %q holds %d record files; want 1", tt.wantOwner, len(records))
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
			if rec.Role != runproc.RolePush || rec.PID <= 0 || rec.PGID != rec.PID {
				t.Errorf("record = %+v; want role %q and a pid that leads its group", rec, runproc.RolePush)
			}
		})
	}
}
