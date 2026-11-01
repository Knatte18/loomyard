//go:build tier0

// config_test.go covers desktophook.LoadConfig over a bare board and anchor, with no hub and no spawn.

package desktophook_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
)

// TestLoadConfig covers the template defaults, a machine override setting both keys, a refused negative wait and a local override refused for a machine key.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		machine string
		local   string
		want    desktophook.Config
		wantErr string
	}{
		{name: "TemplateDefaults", machine: "", want: desktophook.Config{WindowWaitMs: 1500}},
		{
			name:    "MachineOverrideSetsBothKeys",
			machine: "window_command: \"~/.local/bin/arrange-to-tiles.sh\"\nwindow_wait_ms: 200\n",
			want:    desktophook.Config{WindowCommand: "~/.local/bin/arrange-to-tiles.sh", WindowWaitMs: 200},
		},
		{name: "NegativeWaitRefused", machine: "window_wait_ms: -1\n", wantErr: `desktop config key "window_wait_ms" holds -1`},
		{name: "LocalOverrideRefused", local: "window_command: \"true\"\n", wantErr: "window_command"},
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
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
