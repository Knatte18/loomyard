// config_test.go covers `ideconfig.LoadConfig` over a bare board and anchor, with no hub and no spawn.

package ideconfig_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/ideconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadConfig covers the template defaults, a hub file, and a local override that sets the keys.
// It also covers the open_files entries that `LoadConfig` refuses.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		hub     string
		local   string
		want    ideconfig.Config
		wantErr string
	}{
		{
			name: "TemplateDefaults",
			want: ideconfig.Config{OpenFiles: ideconfig.OpenFiles{Prime: []string{}, Beam: []string{"README.md"}, Task: []string{}}, RenderMarkdown: true},
		},
		{
			name: "HubFileSetsBothKeys",
			hub:  "open_files:\n  prime: [docs/overview.md]\n  beam: [README.md, \"docs/{beam}.md\"]\n  task: []\nrender_markdown: false\n",
			want: ideconfig.Config{OpenFiles: ideconfig.OpenFiles{Prime: []string{"docs/overview.md"}, Beam: []string{"README.md", "docs/{beam}.md"}, Task: []string{}}},
		},
		{
			name:  "LocalOverrideSetsRenderMarkdown",
			local: "render_markdown: false\n",
			want:  ideconfig.Config{OpenFiles: ideconfig.OpenFiles{Prime: []string{}, Beam: []string{"README.md"}, Task: []string{}}},
		},
		{name: "AbsoluteEntryRefused", hub: "open_files:\n  task: [/etc/hosts]\n", wantErr: `ide config key "open_files.task" holds "/etc/hosts"`},
		{name: "ClimbingEntryRefused", hub: "open_files:\n  beam: [../README.md]\n", wantErr: `ide config key "open_files.beam" holds "../README.md"`},
		{name: "EmptyEntryRefused", hub: "open_files:\n  prime: [\"\"]\n", wantErr: `ide config key "open_files.prime" holds ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Board: t.TempDir(), Anchor: t.TempDir()}
			writeLayer(t, configengine.ConfigDir(dirs.Board), configengine.ConfigFile(dirs.Board, "ide"), tt.hub)
			if tt.local != "" {
				// The anchor gets a _lym/config/ too, since a worktree with no _lym/ stacks no local layer.
				writeLayer(t, configengine.ConfigDir(dirs.Anchor), "", "")
				writeLayer(t, configengine.ScratchDir(dirs.Anchor), configengine.LocalFile(dirs.Anchor, "ide"), tt.local)
			}

			got, err := ideconfig.LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			sameFiles := slices.Equal(got.OpenFiles.Prime, tt.want.OpenFiles.Prime) && slices.Equal(got.OpenFiles.Beam, tt.want.OpenFiles.Beam) && slices.Equal(got.OpenFiles.Task, tt.want.OpenFiles.Task)
			if !sameFiles || got.RenderMarkdown != tt.want.RenderMarkdown {
				t.Errorf("LoadConfig() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// writeLayer creates dir and, when contents is not empty, writes contents to path.
func writeLayer(t *testing.T, dir, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	if contents == "" {
		return
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}
