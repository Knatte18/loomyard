// config.go defines ide.yaml: the Config type, the template and spec accessors, and LoadConfig.

package ideconfig

import (
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the default YAML template for the ide config module.
func ConfigTemplate() string {
	return configTemplate
}

// configModule is the config module name ide.yaml is loaded under.
const configModule = "ide"

// Keys of ide.yaml.
const (
	openFilesKey      = "open_files"
	renderMarkdownKey = "render_markdown"
)

// BeamPlaceholder is the text an open_files entry names to stand for the beam's slug.
const BeamPlaceholder = "{beam}"

// Config represents the resolved ide.yaml configuration.
type Config struct {
	// OpenFiles is the files a new window opens beside its folder or workspace, per place.
	OpenFiles OpenFiles `yaml:"open_files"`
	// RenderMarkdown reports whether a markdown file opens as VS Code's markdown preview in a seeded worktree.
	RenderMarkdown bool `yaml:"render_markdown"`
}

// OpenFiles is one ordered list of open_files entries per place, each a path relative to the worktree's anchored directory that may name BeamPlaceholder.
type OpenFiles struct {
	Prime []string `yaml:"prime"`
	Beam  []string `yaml:"beam"`
	Task  []string `yaml:"task"`
}

// ConfigSpec returns the ide module's layered-load spec.
// Both keys change only what a window shows, so the local layer may hold them.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     configModule,
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			openFilesKey:      configengine.CategoryNone,
			renderMarkdownKey: configengine.CategoryNone,
		},
	}
}

// LoadConfig loads and unmarshals the ide module's configuration through the hub-wide layer stack under dirs.
// A hub whose base file is absent resolves the template under the local layer present, so opening a window never fails on a hub not yet reconciled.
// It refuses an open_files entry that is empty, absolute or climbs out of the worktree, naming the key.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.LoadOrTemplate(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal ide config: %w", err))
	}
	places := []struct {
		name    string
		entries []string
	}{{"prime", cfg.OpenFiles.Prime}, {"beam", cfg.OpenFiles.Beam}, {"task", cfg.OpenFiles.Task}}
	for _, place := range places {
		for _, entry := range place.entries {
			if !filepath.IsLocal(filepath.FromSlash(entry)) {
				return Config{}, configengine.MarkInvalid(fmt.Errorf("ide config key %q holds %q; want a path relative to the worktree's anchored directory that stays inside it", openFilesKey+"."+place.name, entry))
			}
		}
	}
	return cfg, nil
}
