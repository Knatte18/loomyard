// config_test.go covers desktophook.LoadConfig over a bare board and anchor, with no hub and no spawn.

package desktophook_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadConfig covers the template defaults, a machine override setting both keys, a refused negative wait and a local override refused for a machine key.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		machine string
		local   string
		want    desktophook.Config
		wantErr string
		// wantTaskRuns holds the run text of each entry the resolved buttons node holds in its task group.
		wantTaskRuns []string
		// wantErrDetail is a second substring the error must hold, the reason beside the key wantErr names.
		wantErrDetail string
	}{
		{name: "TemplateDefaults", machine: "", want: desktophook.Config{WindowWaitMs: 1500}},
		{
			name:    "MachineOverrideSetsBothKeys",
			machine: "window_command: \"~/.local/bin/arrange-to-tiles.sh\"\nwindow_wait_ms: 200\n",
			want:    desktophook.Config{WindowCommand: "~/.local/bin/arrange-to-tiles.sh", WindowWaitMs: 200},
		},
		{name: "NegativeWaitRefused", machine: "window_wait_ms: -1\n", wantErr: `desktop config key "window_wait_ms" holds -1`},
		{name: "LocalOverrideRefused", local: "window_command: \"true\"\n", wantErr: "window_command"},
		{
			name:         "MachineOverrideAddsButtons",
			machine:      "buttons: {task: [{label: x, run: y}]}\n",
			want:         desktophook.Config{WindowWaitMs: 1500},
			wantTaskRuns: []string{"y"},
		},
		{name: "MachineGroupAsEmptyMappingRefused", machine: "buttons: {task: {}}\n", wantErr: "buttons.task", wantErrDetail: "different shape"},
		{name: "MachineGroupAsMappingRefused", machine: "buttons: {task: {label: x}}\n", wantErr: "buttons.task.label", wantErrDetail: "not in the desktop template"},
		{name: "LocalButtonsRefused", local: "buttons: {task: []}\n", wantErr: "buttons.task"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := newDirsWithMachineConfig(t, tt.machine)
			if tt.local != "" {
				// The anchor gets a _lym/config/ too, since a worktree with no _lym/ stacks no local layer.
				for _, dir := range []string{configengine.ConfigDir(dirs.Anchor), configengine.ScratchDir(dirs.Anchor)} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
					}
				}
				if err := os.WriteFile(configengine.LocalFile(dirs.Anchor, "desktop"), []byte(tt.local), 0o644); err != nil {
					t.Fatalf("WriteFile local override = %v; want nil", err)
				}
			}

			got, err := desktophook.LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), tt.wantErrDetail) || !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if got.WindowCommand != tt.want.WindowCommand || got.WindowWaitMs != tt.want.WindowWaitMs {
				t.Errorf("LoadConfig() = {WindowCommand: %q, WindowWaitMs: %d}; want {WindowCommand: %q, WindowWaitMs: %d}", got.WindowCommand, got.WindowWaitMs, tt.want.WindowCommand, tt.want.WindowWaitMs)
			}
			var buttons struct {
				Task []struct {
					Run string `yaml:"run"`
				} `yaml:"task"`
			}
			if err := got.Buttons.Decode(&buttons); err != nil {
				t.Fatalf("decode Buttons node = %v; want nil", err)
			}
			gotRuns := []string{}
			for _, entry := range buttons.Task {
				gotRuns = append(gotRuns, entry.Run)
			}
			if !slices.Equal(gotRuns, append([]string{}, tt.wantTaskRuns...)) {
				t.Errorf("LoadConfig() Buttons task runs = %v; want %v", gotRuns, tt.wantTaskRuns)
			}
		})
	}
}
