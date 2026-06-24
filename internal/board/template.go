// template.go — board.yaml template generator.
//
// Provides the fully-commented default YAML template for board configuration.

package board

import "strings"

// ConfigTemplate returns a fully-commented YAML template for board configuration.
func ConfigTemplate() string {
	var sb strings.Builder

	sb.WriteString("# path: $env:LYX_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute\n")
	sb.WriteString("# home: $env:LYX_HOME ? Home.md           # home page file name; relative to board dir\n")
	sb.WriteString("# sidebar: $env:LYX_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir\n")
	sb.WriteString("# proposal_prefix: $env:LYX_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files\n")

	return sb.String()
}
