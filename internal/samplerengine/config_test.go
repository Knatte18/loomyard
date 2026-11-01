// config_test.go — tier 0 unit tests for samplerengine.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lym/config/sampler.yaml file (no real hub, no git spawn).

package samplerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedHubConfig creates <board>/_lym/config/sampler.yaml with the given contents.
func seedHubConfig(t *testing.T, board, contents string) {
	t.Helper()
	configDir := filepath.Join(board, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sampler.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile sampler.yaml = %v; want nil", err)
	}
}

// writeLocalConfig writes contents as anchor's local sampler override.
// anchor gets an empty _lym/config/ so the layered load sees an initialised worktree.
func writeLocalConfig(t *testing.T, anchor, contents string) {
	t.Helper()
	for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
		}
	}
	localPath := configengine.LocalFile(anchor, "sampler")
	if err := os.WriteFile(localPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
	}
}

// TestLoadConfig covers the template's defaults for a hub with no file, a hub file and a local layer setting code_review, an agent failing the model-spec grammar, and a base directory with no config.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		hub     string
		local   string
		hasHub  bool
		want    Config
		wantErr string
	}{
		{name: "template defaults", hub: ConfigTemplate(), hasHub: true, want: Config{Agent: "opus[high]"}},
		{
			name: "a hub file setting code_review loads true", hasHub: true,
			hub:  strings.Replace(ConfigTemplate(), "code_review: false", "code_review: true", 1),
			want: Config{Agent: "opus[high]", CodeReview: true},
		},
		{
			name: "a local layer setting code_review loads true", hasHub: true,
			hub: ConfigTemplate(), local: "code_review: true\n",
			want: Config{Agent: "opus[high]", CodeReview: true},
		},
		{
			name: "an invalid agent spec is refused naming the key", hasHub: true,
			hub:     strings.Replace(ConfigTemplate(), "agent: opus[high]", "agent: \"opus[[bad\"", 1),
			wantErr: `sampler config key "agent"`,
		},
		{name: "not initialized", wantErr: `not initialized here; run "lym fabric reconcile"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			board, anchor := t.TempDir(), t.TempDir()
			if tt.hasHub {
				seedHubConfig(t, board, tt.hub)
			}
			dirs := configengine.Dirs{Board: board}
			if tt.local != "" {
				writeLocalConfig(t, anchor, tt.local)
				dirs.Anchor = anchor
			}
			got, err := LoadConfig(dirs, "sampler")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadConfig() = _, %v; want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil error", err)
			}
			if got != tt.want {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
