// spawn_linux_test.go covers the registration of the detached push child in the owner record.
// It drives that registration with a harmless real child, which the test reaps.

package fabricengine

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

// TestStartDetachedPush_RegistersTheChild covers the owner record of a started push child.
// A pair's and the prime's request both register under the hub owner with the push role, running the pusher's line.
// A record directory that cannot be written logs a warning and still starts the child.
// It is not parallel: it captures the process-wide logger.
func TestStartDetachedPush_RegistersTheChild(t *testing.T) {
	tiergate.Require(t, 0)
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true binary on PATH")
	}

	tests := []struct {
		name     string
		req      fabriclayout.PushSpawn
		wantArgs []string
		blocked  bool
	}{
		{
			name:     "PairPushOwnsUnderTheHubOwner",
			req:      fabriclayout.PushSpawn{WarpPath: "/hub/task", WeftPath: "/hub/task-weft"},
			wantArgs: []string{"loom", "pusher", "--worktree", "/hub/task", "--slug", "task"},
		},
		{
			name:     "PrimePushOwnsUnderTheHubOwner",
			req:      fabriclayout.PushSpawn{WeftPath: "/hub/prime-weft"},
			wantArgs: []string{"loom", "pusher", "--worktree", "/hub/prime"},
		},
		{name: "UnwritableRecordDirStillStartsTheChild", req: fabriclayout.PushSpawn{WeftPath: "/hub/prime-weft"}, blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			board := fabriclayout.BoardDir(hub)
			if err := os.MkdirAll(board, 0o755); err != nil {
				t.Fatal(err)
			}
			req := tt.req
			req.WorkDir = board
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
			records, err := filepath.Glob(filepath.Join(fabriclayout.RunProcDir(hub), runproc.HubOwner, "*.json"))
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 {
				t.Fatalf("the hub owner holds %d record files; want 1", len(records))
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
			if !slices.Equal(rec.Argv, tt.wantArgs) {
				t.Errorf("record argv = %q; want %q", rec.Argv, tt.wantArgs)
			}
		})
	}
}
