// init.go — scaffolds the config layer for mhgo.
//
// RunInit creates the _mhgo/ directory, writes a fully-commented board.yaml
// template, and maintains a managed block in .gitignore. It is idempotent and
// never clobbers an existing board.yaml.

package board

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunInit scaffolds the config layer in the current working directory.
// It creates _mhgo/, writes a commented board.yaml (if absent), and maintains
// a managed block in .gitignore containing .mhgo/. Returns a JSON summary
// and process exit code (0 on success, 1 on error).
func RunInit(out io.Writer, args []string) int {
	// Resolve current working directory
	cwd, err := os.Getwd()
	if err != nil {
		outputInitError(out, fmt.Sprintf("failed to get working directory: %v", err))
		return 1
	}

	// Track status for each step
	status := map[string]string{}

	// Step 1: Create _mhgo/ directory
	mhgoDir := filepath.Join(cwd, "_mhgo")
	info, err := os.Stat(mhgoDir)
	if err != nil && !os.IsNotExist(err) {
		outputInitError(out, fmt.Sprintf("failed to stat _mhgo: %v", err))
		return 1
	}

	if os.IsNotExist(err) {
		// Directory doesn't exist, create it
		if err := os.MkdirAll(mhgoDir, 0o755); err != nil {
			outputInitError(out, fmt.Sprintf("failed to create _mhgo directory: %v", err))
			return 1
		}
		status["mhgo_dir"] = "created"
	} else if info.IsDir() {
		// Directory already exists
		status["mhgo_dir"] = "exists"
	} else {
		// Exists but is not a directory
		outputInitError(out, fmt.Sprintf("_mhgo exists but is not a directory"))
		return 1
	}

	// Step 2: Write commented board.yaml (if absent)
	boardYamlPath := filepath.Join(mhgoDir, "board.yaml")
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

	// Step 3: Maintain managed block in .gitignore
	gitignorePath := filepath.Join(cwd, ".gitignore")
	updated, err := updateGitignoreBlock(gitignorePath)
	if err != nil {
		outputInitError(out, fmt.Sprintf("failed to update .gitignore: %v", err))
		return 1
	}

	if updated {
		status["gitignore"] = "updated"
	} else {
		status["gitignore"] = "unchanged"
	}

	// Step 4: Output success JSON
	result := map[string]any{
		"ok":         true,
		"mhgo_dir":   status["mhgo_dir"],
		"board_yaml": status["board_yaml"],
		"gitignore":  status["gitignore"],
	}

	data, _ := json.Marshal(result)
	fmt.Fprintln(out, string(data))
	return 0
}

// generateCommentedBoardYAML returns a fully-commented YAML template.
func generateCommentedBoardYAML() string {
	var sb strings.Builder

	sb.WriteString("# path: $env:MHGO_BOARD_PATH ? ../_board   # board dir (tasks.json + rendered output); relative to cwd or absolute\n")
	sb.WriteString("# home: $env:MHGO_HOME ? Home.md           # home page file name; relative to board dir\n")
	sb.WriteString("# sidebar: $env:MHGO_SIDEBAR ? _Sidebar.md   # sidebar file name; relative to board dir\n")
	sb.WriteString("# proposal_prefix: $env:MHGO_PROPOSAL_PREFIX ? proposal-   # prefix for proposal files\n")

	return sb.String()
}

// updateGitignoreBlock maintains a managed block in .gitignore delimited by
// # === mhgo-managed === and # === end mhgo-managed ===.
// Returns true if the file was created or the block's interior changed, false if unchanged.
func updateGitignoreBlock(gitignorePath string) (bool, error) {
	const startMarker = "# === mhgo-managed ==="
	const endMarker = "# === end mhgo-managed ==="
	const blockContent = ".mhgo/"

	// Check if .gitignore exists
	_, statErr := os.Stat(gitignorePath)
	fileIsNew := os.IsNotExist(statErr)
	if statErr != nil && !fileIsNew {
		return false, fmt.Errorf("failed to stat .gitignore: %w", statErr)
	}

	// Read existing .gitignore if it exists
	var existingContent string
	if !fileIsNew {
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			return false, fmt.Errorf("failed to read .gitignore: %w", err)
		}
		existingContent = string(content)
	}

	// Parse the file: find the block if it exists and capture its interior content
	var blockExists bool
	var oldBlockContent string
	var beforeBlock, afterBlock []string
	var inBlock bool

	lines := strings.Split(existingContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			blockExists = true
		} else if trimmed == endMarker {
			inBlock = false
			afterBlock = append(afterBlock, line)
		} else if inBlock {
			// Capture old block content (non-empty trimmed lines)
			if trimmed != "" {
				oldBlockContent += trimmed + "\n"
			}
		} else if blockExists {
			// After block has ended, collect lines after
			afterBlock = append(afterBlock, line)
		} else {
			// Before block starts, collect lines before
			beforeBlock = append(beforeBlock, line)
		}
	}

	// Trim old block content for comparison
	oldBlockTrimmed := strings.TrimSpace(oldBlockContent)
	newBlockTrimmed := strings.TrimSpace(blockContent)
	contentChanged := oldBlockTrimmed != newBlockTrimmed

	// Only write if file is new, block doesn't exist, or content changed
	if !fileIsNew && blockExists && !contentChanged {
		return false, nil
	}

	// Build the new file content
	var result []string

	// Add before-block content
	result = append(result, beforeBlock...)

	// Add the managed block
	if len(beforeBlock) > 0 && strings.TrimSpace(beforeBlock[len(beforeBlock)-1]) != "" {
		result = append(result, "") // blank line before block if there's content
	}
	result = append(result, startMarker)
	result = append(result, blockContent)
	result = append(result, endMarker)

	// Add after-block content (skip the first empty line from parsing artifact)
	for i, line := range afterBlock {
		if i == 0 && strings.TrimSpace(line) == "" {
			continue // Skip first empty line from end marker parsing
		}
		result = append(result, line)
	}

	newContent := strings.Join(result, "\n")
	// Ensure final newline
	newContent = strings.TrimRight(newContent, "\n") + "\n"

	// Write the file
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return true, nil
}

// outputInitError writes {"ok":false,"error":"..."} and is a helper for RunInit.
func outputInitError(out io.Writer, message string) {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": message})
	fmt.Fprintln(out, string(data))
}
