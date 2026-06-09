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
		defaults := DefaultConfig()
		content := generateCommentedBoardYAML(defaults)
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

// generateCommentedBoardYAML returns a fully-commented YAML template
// based on the default config values.
func generateCommentedBoardYAML(defaults Config) string {
	var sb strings.Builder

	sb.WriteString("# path: " + defaults.Path + "   # board dir (tasks.json + rendered output); relative to cwd; may contain $env:NAME\n")
	sb.WriteString("# home: " + defaults.Home + "   # home page file name; relative to board dir\n")
	sb.WriteString("# sidebar: " + defaults.Sidebar + "   # sidebar file name; relative to board dir\n")
	sb.WriteString("# proposal_prefix: " + defaults.ProposalPrefix + "   # prefix for proposal files; relative to board dir\n")

	return sb.String()
}

// updateGitignoreBlock maintains a managed block in .gitignore delimited by
// # === mhgo-managed === and # === end mhgo-managed ===.
// Returns true if the file was created or the block's interior changed, false if unchanged.
func updateGitignoreBlock(gitignorePath string) (bool, error) {
	const startMarker = "# === mhgo-managed ==="
	const endMarker = "# === end mhgo-managed ==="
	const blockContent = ".mhgo/"

	// Read existing .gitignore if it exists
	var existingContent string
	info, err := os.Stat(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to stat .gitignore: %w", err)
	}

	if !os.IsNotExist(err) {
		// File exists, read it
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			return false, fmt.Errorf("failed to read .gitignore: %w", err)
		}
		existingContent = string(content)
	}

	// Extract current block if it exists
	var newContent string
	var blockExists bool
	var oldBlockContent string

	lines := strings.Split(existingContent, "\n")
	var result []string
	var inBlock bool

	for _, line := range lines {
		if strings.TrimSpace(line) == startMarker {
			inBlock = true
			blockExists = true
			result = append(result, line) // Keep the start marker
		} else if strings.TrimSpace(line) == endMarker {
			inBlock = false
			result = append(result, line) // Keep the end marker
		} else if inBlock {
			// Capture the old block content (trimmed lines)
			if strings.TrimSpace(line) != "" {
				oldBlockContent += strings.TrimSpace(line) + "\n"
			}
		} else {
			result = append(result, line)
		}
	}

	// Compare old block content (trimmed) with new block content (trimmed)
	newBlockContent := strings.TrimSpace(blockContent)
	oldBlockContentTrimmed := strings.TrimSpace(oldBlockContent)

	// Reconstruct with new block content
	if blockExists {
		// Replace the old block with the new one
		newResult := []string{}
		inBlock := false
		for i, line := range result {
			if strings.TrimSpace(line) == startMarker {
				inBlock = true
				newResult = append(newResult, line)
				// Add the new block content after the start marker
				newResult = append(newResult, blockContent)
			} else if strings.TrimSpace(line) == endMarker {
				inBlock = false
				newResult = append(newResult, line)
			} else if !inBlock {
				// Only add non-block lines
				if !(i > 0 && strings.TrimSpace(result[i-1]) == startMarker) {
					newResult = append(newResult, line)
				}
			}
		}
		newContent = strings.Join(newResult, "\n")
	} else {
		// Block doesn't exist, append it
		newResult := result
		newResult = append(newResult, "")
		newResult = append(newResult, startMarker)
		newResult = append(newResult, blockContent)
		newResult = append(newResult, endMarker)
		newContent = strings.Join(newResult, "\n")
	}

	// Ensure final newline
	newContent = strings.TrimRight(newContent, "\n") + "\n"

	// Check if content changed (compare trimmed interior)
	changed := oldBlockContentTrimmed != newBlockContent

	// If file didn't exist, it's also "changed"
	fileIsNew := os.IsNotExist(err)

	if fileIsNew || changed {
		// Write the updated content
		if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
			return false, fmt.Errorf("failed to write .gitignore: %w", err)
		}
		return true
	}

	return false, nil
}

// outputInitError writes {"ok":false,"error":"..."} and is a helper for RunInit.
func outputInitError(out io.Writer, message string) {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": message})
	fmt.Fprintln(out, string(data))
}
