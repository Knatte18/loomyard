// template.go — worktree.yaml template generator.
//
// Provides the fully-commented default YAML template for worktree configuration.

package worktree

import "strings"

// ConfigTemplate returns a fully-commented YAML template for worktree configuration.
func ConfigTemplate() string {
	var sb strings.Builder

	sb.WriteString("# branch_prefix: $env:LYX_BRANCH_PREFIX ?    # prefix prepended to the slug to form the branch name (e.g. \"hanf/\"); empty = branch == slug\n")

	return sb.String()
}
