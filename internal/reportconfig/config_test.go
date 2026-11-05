// config_test.go covers reportconfig.LoadConfig over a bare board, with no hub and no spawn.

package reportconfig_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reportconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadConfig covers the template default, a hub value and a zero value refused naming the key.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		hub     string
		want    reportconfig.Config
		wantErr string
	}{
		{name: "TemplateValue", hub: "", want: reportconfig.Config{CostMinRuns: 5}},
		{name: "HubValue", hub: "cost_min_runs: 9\n", want: reportconfig.Config{CostMinRuns: 9}},
		{name: "ZeroRefused", hub: "cost_min_runs: 0\n", wantErr: `report config key "cost_min_runs" holds 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir()}
			if tt.hub != "" {
				if err := os.MkdirAll(configengine.ConfigDir(dirs.Board), 0o755); err != nil {
					t.Fatalf("MkdirAll(config dir) = %v; want nil", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dirs.Board, "report"), []byte(tt.hub), 0o644); err != nil {
					t.Fatalf("WriteFile hub file = %v; want nil", err)
				}
			}

			got, err := reportconfig.LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "lym config report") || !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error containing %q and the way forward", err, tt.wantErr)
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
