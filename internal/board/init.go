// init.go — scaffolds the config layer for Loomyard.
//
// RunInit creates the _lyx/ and _lyx/config/ directories, writes fully-commented
// board.yaml and worktree.yaml templates to _lyx/config/, and maintains a managed
// block in .gitignore. It is idempotent and never clobbers existing files.

package board

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitignore"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunInit scaffolds the config layer in the current working directory.
// It creates _lyx/ and _lyx/config/, writes commented board.yaml and worktree.yaml
// (if absent), and maintains a managed block in .gitignore containing .lyx/.
// Returns a JSON summary and process exit code (0 on success, 1 on error).
func RunInit(out io.Writer, args []string) int {
	// Resolve current working directory
	cwd, err := paths.Getwd()
	if err != nil {
		outputInitError(out, fmt.Sprintf("failed to get working directory: %v", err))
		return 1
	}

	// Track status for each step
	status := map[string]string{}

	// Step 1: Create _lyx/ directory
	lyxDir := filepath.Join(cwd, "_lyx")
	info, err := os.Stat(lyxDir)
	if err != nil && !os.IsNotExist(err) {
		outputInitError(out, fmt.Sprintf("failed to stat _lyx: %v", err))
		return 1
	}

	if os.IsNotExist(err) {
		// Directory doesn't exist, create it
		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
			outputInitError(out, fmt.Sprintf("failed to create _lyx directory: %v", err))
			return 1
		}
		status["lyx_dir"] = "created"
	} else if info.IsDir() {
		// Directory already exists
		status["lyx_dir"] = "exists"
	} else {
		// Exists but is not a directory
		outputInitError(out, fmt.Sprintf("_lyx exists but is not a directory"))
		return 1
	}

	// Create _lyx/config/ subdirectory to hold configuration files.
	configDir := filepath.Join(lyxDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		outputInitError(out, fmt.Sprintf("failed to create _lyx/config directory: %v", err))
		return 1
	}

	// Step 2: Write commented board.yaml (if absent)
	boardYamlPath := filepath.Join(configDir, "board.yaml")
	_, err = os.Stat(boardYamlPath)
	if err != nil && !os.IsNotExist(err) {
		outputInitError(out, fmt.Sprintf("failed to stat board.yaml: %v", err))
		return 1
	}

	if os.IsNotExist(err) {
		// File doesn't exist, write the commented template
		content := generateCommentedBoardYAML()
		if err := os.WriteFile(boardYamlPath, []byte(content), 0o644); err != nil {
			outputInitError(out, fmt.Sprintf("failed to write board.yaml: %v", err))
			return 1
		}
		status["board_yaml"] = "created"
	} else {
		// File already exists
		status["board_yaml"] = "exists"
	}

	// Step 3: Write commented worktree.yaml (if absent)
	worktreeYamlPath := filepath.Join(configDir, "worktree.yaml")
	_, err = os.Stat(worktreeYamlPath)
	if err != nil && !os.IsNotExist(err) {
		outputInitError(out, fmt.Sprintf("failed to stat worktree.yaml: %v", err))
		return 1
	}

	if os.IsNotExist(err) {
		// File doesn't exist, write the commented template
		content := generateCommentedWorktreeYAML()
		if err := os.WriteFile(worktreeYamlPath, []byte(content), 0o644); err != nil {
			outputInitError(out, fmt.Sprintf("failed to write worktree.yaml: %v", err))
			return 1
		}
		status["worktree_yaml"] = "created"
	} else {
		// File already exists
		status["worktree_yaml"] = "exists"
	}

	// Step 4: Maintain managed block in .gitignore
	changed, err := gitignore.Ensure(cwd, ".lyx/")
	if err != nil {
		outputInitError(out, fmt.Sprintf("failed to update .gitignore: %v", err))
		return 1
	}

	if changed {
		status["gitignore"] = "updated"
	} else {
		status["gitignore"] = "unchanged"
	}

	// Step 5: Output success JSON
	return output.Ok(out, map[string]any{
		"lyx_dir":       status["lyx_dir"],
		"board_yaml":    status["board_yaml"],
		"worktree_yaml": status["worktree_yaml"],
		"gitignore":     status["gitignore"],
	})
}

// generateCommentedBoardYAML returns a fully-commented YAML template.
func generateCommentedBoardYAML() string {
	var sb strings.Builder

	sb.WriteString("# path: $env:LYX_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute\n")
	sb.WriteString("# home: $env:LYX_HOME ? Home.md           # home page file name; relative to board dir\n")
	sb.WriteString("# sidebar: $env:LYX_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir\n")
	sb.WriteString("# proposal_prefix: $env:LYX_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files\n")

	return sb.String()
}

// generateCommentedWorktreeYAML returns a fully-commented YAML template for worktree configuration.
func generateCommentedWorktreeYAML() string {
	var sb strings.Builder

	sb.WriteString("# branch_prefix: $env:LYX_BRANCH_PREFIX ?    # prefix prepended to the slug to form the branch name (e.g. \"hanf/\"); empty = branch == slug\n")

	return sb.String()
}

// outputInitError writes {"ok":false,"error":"..."} and is a helper for RunInit.
func outputInitError(out io.Writer, message string) {
	output.Err(out, message)
}
