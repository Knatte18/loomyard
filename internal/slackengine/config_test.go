// config_test.go covers slackengine.LoadConfig over bare board dirs, with no hub and no spawn.

package slackengine_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestLoadConfig covers the template default, a hub value, and a value below the floor.
// That value is refused with an error that names the key and the way forward.
// The template holds no key but remind_after_min,
// so no `lym config slack` write can name a channel, an operator or a token.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		hub     string
		want    slackengine.Config
		wantErr string
	}{
		{name: "TemplateValue", hub: "", want: slackengine.Config{RemindAfterMin: 30}},
		{name: "HubValue", hub: "remind_after_min: 45\n", want: slackengine.Config{RemindAfterMin: 45}},
		{name: "ZeroRefused", hub: "remind_after_min: 0\n", wantErr: `slack config key "remind_after_min" holds 0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir()}
			if tt.hub != "" {
				if err := os.MkdirAll(configengine.ConfigDir(dirs.Board), 0o755); err != nil {
					t.Fatalf("MkdirAll(config dir) = %v; want nil", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(dirs.Board, "slack"), []byte(tt.hub), 0o644); err != nil {
					t.Fatalf("WriteFile hub file = %v; want nil", err)
				}
			}

			got, err := slackengine.LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "lym config slack") || !errors.Is(err, configengine.ErrInvalid) {
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

	t.Run("TemplateKeysAreExactlyRemindAfterMin", func(t *testing.T) {
		t.Parallel()
		spec := slackengine.ConfigSpec()
		keys, err := yamlengine.LeafPaths(spec.Template, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			t.Fatalf("LeafPaths(slack template) = %v; want nil", err)
		}
		if want := []string{"remind_after_min"}; !slices.Equal(keys, want) {
			t.Errorf("slack template keys = %v; want %v", keys, want)
		}
	})
}
