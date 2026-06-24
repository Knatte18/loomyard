// template.go — weft.yaml template generator.
//
// Provides the fully-commented default YAML template for weft configuration.

package weft

// ConfigTemplate returns a fully-commented YAML template for weft configuration.
func ConfigTemplate() string {
	return "# pathspec: _lyx                          # directory path(s) relative to worktree root, whitespace-separated; _lyx is the default\n"
}
