// Package gitignore manages a single lyx-managed block in .gitignore that is shared across multiple modules.
//
// The Ensure function maintains entries as a set: multiple modules can contribute entries without clobbering each other.
// The block is delimited by # === lyx-managed === and # === end lyx-managed ===.
package gitignore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	startMarker = "# === lyx-managed ==="
	endMarker   = "# === end lyx-managed ==="
)

// Ensure maintains the lyx-managed block in <repoRoot>/.gitignore with the given entries, treated as a set and deduplicated.
// Returns changed=true when the file is created or the block changes, false when unchanged (idempotent).
func Ensure(repoRoot string, entries ...string) (changed bool, err error) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	_, statErr := os.Stat(gitignorePath)
	fileIsNew := os.IsNotExist(statErr)
	if statErr != nil && !fileIsNew {
		return false, fmt.Errorf("failed to stat .gitignore: %w", statErr)
	}

	var existingContent string
	if !fileIsNew {
		content, err := os.ReadFile(gitignorePath)
		if err != nil {
			return false, fmt.Errorf("failed to read .gitignore: %w", err)
		}
		existingContent = string(content)
	}

	existingContent = strings.ReplaceAll(existingContent, "\r\n", "\n")

	beforeBlock, afterBlock, oldEntries, blockExists := parseManagedBlock(existingContent)

	oldSorted := sortedEntrySet(oldEntries)

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			oldEntries[entry] = true
		}
	}
	sortedEntries := sortedEntrySet(oldEntries)

	if !fileIsNew && blockExists && entriesEqual(oldSorted, sortedEntries) {
		return false, nil
	}

	var result []string
	result = append(result, beforeBlock...)

	if len(beforeBlock) > 0 && strings.TrimSpace(beforeBlock[len(beforeBlock)-1]) != "" {
		result = append(result, "")
	}
	result = append(result, startMarker)
	result = append(result, sortedEntries...)
	result = append(result, endMarker)
	result = append(result, afterBlock...)

	newContent := strings.Join(result, "\n")
	newContent = strings.TrimRight(newContent, "\n") + "\n"

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return true, nil
}

// Remove deletes entries from the lyx-managed block in <repoRoot>/.gitignore, leaving other entries and outside content untouched.
// Returns changed=true when the file is rewritten, false when nothing to remove or file missing.
func Remove(repoRoot string, entries ...string) (changed bool, err error) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read .gitignore: %w", err)
	}

	existingContent := strings.ReplaceAll(string(content), "\r\n", "\n")
	beforeBlock, afterBlock, oldEntries, blockExists := parseManagedBlock(existingContent)
	if !blockExists {
		return false, nil
	}

	var anyPresent bool
	for _, entry := range entries {
		if oldEntries[strings.TrimSpace(entry)] {
			anyPresent = true
			break
		}
	}
	if !anyPresent {
		return false, nil
	}
	for _, entry := range entries {
		delete(oldEntries, strings.TrimSpace(entry))
	}
	remainingEntries := sortedEntrySet(oldEntries)

	var result []string
	result = append(result, beforeBlock...)
	if len(remainingEntries) > 0 {
		if len(beforeBlock) > 0 && strings.TrimSpace(beforeBlock[len(beforeBlock)-1]) != "" {
			result = append(result, "")
		}
		result = append(result, startMarker)
		result = append(result, remainingEntries...)
		result = append(result, endMarker)
	}
	result = append(result, afterBlock...)

	var newContent string
	if len(result) > 0 {
		newContent = strings.Join(result, "\n")
		newContent = strings.TrimRight(newContent, "\n") + "\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil {
		return false, fmt.Errorf("failed to write .gitignore: %w", err)
	}

	return true, nil
}

// parseManagedBlock splits .gitignore content into lines before the block,
// entries inside it, and lines after, keeping Ensure and Remove's views aligned.
func parseManagedBlock(content string) (beforeBlock, afterBlock []string, entries map[string]bool, blockExists bool) {
	entries = make(map[string]bool)
	var inBlock bool
	var blockEnded bool

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == startMarker:
			inBlock = true
			blockExists = true
		case trimmed == endMarker:
			inBlock = false
			blockEnded = true
		case inBlock:
			if trimmed != "" {
				entries[trimmed] = true
			}
		case blockEnded:
			afterBlock = append(afterBlock, line)
		default:
			beforeBlock = append(beforeBlock, line)
		}
	}
	return beforeBlock, afterBlock, entries, blockExists
}

// sortedEntrySet returns sorted entry keys.
func sortedEntrySet(entries map[string]bool) []string {
	var sorted []string
	for entry := range entries {
		sorted = append(sorted, entry)
	}
	sort.Strings(sorted)
	return sorted
}

// entriesEqual reports whether two sorted lists are equal.
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
