// Package gitignore manages a single mhgo-managed block in .gitignore
// that is shared across multiple modules.
//
// The Ensure function maintains entries as a set: multiple modules can
// contribute entries without clobbering each other. The block is delimited
// by # === mhgo-managed === and # === end mhgo-managed ===.
package gitignore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	startMarker = "# === mhgo-managed ==="
	endMarker   = "# === end mhgo-managed ==="
)

// Ensure maintains the mhgo-managed block in <repoRoot>/.gitignore, ensuring
// that all provided entries exist within it. The block is treated as a set:
// entries are deduplicated and sorted deterministically. Content outside the
// block is preserved verbatim.
//
// Returns changed=true when the file is created or the block interior changes,
// false when unchanged (idempotent). Returns an error if I/O fails.
func Ensure(repoRoot string, entries ...string) (changed bool, err error) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

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

	// Normalize line endings: convert CRLF to LF for consistent parsing
	existingContent = strings.ReplaceAll(existingContent, "\r\n", "\n")

	// Parse the file: capture before-block, block interior, and after-block
	var blockExists bool
	var oldEntries map[string]bool
	var beforeBlock, afterBlock []string
	var inBlock bool
	var blockEnded bool

	lines := strings.Split(existingContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			blockExists = true
			oldEntries = make(map[string]bool)
		} else if trimmed == endMarker {
			inBlock = false
			blockEnded = true
		} else if inBlock {
			// Capture old block entries (non-empty trimmed lines)
			if trimmed != "" {
				oldEntries[trimmed] = true
			}
		} else if blockEnded {
			// After block has ended, collect lines after
			afterBlock = append(afterBlock, line)
		} else {
			// Before block starts, collect lines before
			beforeBlock = append(beforeBlock, line)
		}
	}

	// Build the new set of entries: union of old and new, deduplicated, sorted
	if oldEntries == nil {
		oldEntries = make(map[string]bool)
	}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			oldEntries[entry] = true
		}
	}

	// Sort entries deterministically
	var sortedEntries []string
	for entry := range oldEntries {
		sortedEntries = append(sortedEntries, entry)
	}
	sort.Strings(sortedEntries)

	// Check if content has changed
	oldSorted := getOldSortedEntries(lines)
	if !fileIsNew && blockExists && entriesEqual(oldSorted, sortedEntries) {
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
	for _, entry := range sortedEntries {
		result = append(result, entry)
	}
	result = append(result, endMarker)

	// Add after-block content
	result = append(result, afterBlock...)

	newContent := strings.Join(result, "\n")
	// Ensure final newline
	newContent = strings.TrimRight(newContent, "\n") + "\n"

	// Write the file
	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return true, nil
}

// getOldSortedEntries extracts entries from the existing block and returns them sorted.
func getOldSortedEntries(lines []string) []string {
	var entries []string
	var inBlock bool
	var oldEntries map[string]bool = make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
		} else if trimmed == endMarker {
			inBlock = false
		} else if inBlock {
			if trimmed != "" {
				oldEntries[trimmed] = true
			}
		}
	}

	for entry := range oldEntries {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries
}

// entriesEqual checks if two sorted entry lists are equal.
func entriesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
