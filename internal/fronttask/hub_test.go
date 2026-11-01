//go:build tier0

// hub_test.go pins the spawn of a land action: the command it builds, its output file, its spawn log and the refusal of a key outside the action table.

package fronttask

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// TestStartAction runs without t.Parallel: it captures the process-global logger output.
func TestStartAction(t *testing.T) {
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetVerbosity(0)
		logger.SetOutput(io.Discard)
	})

	tests := []struct {
		name         string
		key          string
		startErr     error
		wantErr      string
		wantStarted  bool
		wantLogParts []string
	}{
		{
			name:         "SpawnsTheAdoptCheckOfTheSubjectAndLogsIt",
			key:          ActionAdoptCheck,
			wantStarted:  true,
			wantLogParts: []string{"spawned a land action", "adopt check alpha", "pid=4242"},
		},
		{
			name:         "FailedStartIsReturnedAndLogged",
			key:          ActionAdoptCheck,
			startErr:     errors.New("fork failed"),
			wantErr:      "fork failed",
			wantStarted:  true,
			wantLogParts: []string{"did not start", "fork failed"},
		},
		{
			name:    "KeyOutsideTheTableIsRefusedWithoutStarting",
			key:     "nonsense",
			wantErr: `"nonsense"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			logger.SetOutput(&logged)
			hub := t.TempDir()
			prime := &lymcwd.Location{HubPath: hub, WorktreeName: "prime"}

			var started *exec.Cmd
			err := startAction(prime, tt.key, "alpha", func(command *exec.Cmd) (int, error) {
				started = command
				return 4242, tt.startErr
			})

			if tt.wantErr == "" && err != nil {
				t.Fatalf("startAction() = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("startAction() = %v; want an error containing %q", err, tt.wantErr)
			}
			if (started != nil) != tt.wantStarted {
				t.Fatalf("start called = %v; want %v", started != nil, tt.wantStarted)
			}
			for _, want := range tt.wantLogParts {
				if !strings.Contains(logged.String(), want) {
					t.Errorf("log = %q; want it to contain %q", logged.String(), want)
				}
			}
			if started == nil {
				return
			}
			if got, want := started.Args[1:], []string{"adopt", "check", "alpha"}; !slices.Equal(got, want) {
				t.Errorf("spawned arguments = %v; want %v", got, want)
			}
			if got, want := started.Dir, prime.AnchorPath(); got != want {
				t.Errorf("spawned directory = %q; want %q", got, want)
			}
			outPath := filepath.Join(fabricengine.FrontTaskScratchDir(hub), tt.key+".out")
			if _, err := os.Stat(outPath); err != nil {
				t.Errorf("output file %q: %v; want it created", outPath, err)
			}
		})
	}
}
