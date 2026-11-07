// config_test.go covers creelconfig.LoadConfig over a bare board, with no hub and no spawn.

package creelconfig_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadConfig covers the template's empty list, a hub value and malformed YAML returned as an error.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		hub     string
		want    []string
		wantErr bool
	}{
		{name: "TemplateValue", hub: "", want: []string{}},
		{name: "HubValue", hub: "trusted_hubs:\n  - Knatte18/loomyard\n  - Knatte18/other\n", want: []string{"Knatte18/loomyard", "Knatte18/other"}},
		{name: "MalformedYAML", hub: "trusted_hubs: [ly\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir()}
			if tt.hub != "" {
				if err := os.MkdirAll(configengine.ConfigDir(dirs.Board), 0o755); err != nil {
					t.Fatalf("MkdirAll(config dir) = %v; want nil", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dirs.Board, "creel"), []byte(tt.hub), 0o644); err != nil {
					t.Fatalf("WriteFile hub file = %v; want nil", err)
				}
			}

			got, err := creelconfig.LoadConfig(dirs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig() error = nil; want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if len(got.TrustedHubs) != len(tt.want) || (len(tt.want) > 0 && !reflect.DeepEqual(got.TrustedHubs, tt.want)) {
				t.Errorf("LoadConfig().TrustedHubs = %v; want %v", got.TrustedHubs, tt.want)
			}
		})
	}
}
