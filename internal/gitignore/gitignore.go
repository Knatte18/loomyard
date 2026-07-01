// Package gitignore manages a single lyx-managed block in .gitignore
// that is shared across multiple modules.
//
// The Ensure function maintains entries as a set: multiple modules can
// contribute entries without clobbering each other. The block is delimited
// by # === lyx-managed === and # === end lyx-managed ===.
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

// Ensure maintains the lyx-managed block in <repoRoot>/.gitignore, ensuring
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

	beforeBlock, afterBlock, oldEntries, blockExists := parseManagedBlock(existingContent)

	// Capture the entries as they stood before merging in the incoming ones,
	// so the idempotency check below compares like-for-like.
	oldSorted := sortedEntrySet(oldEntries)

	// Build the new set of entries: union of old and new, deduplicated, sorted
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			oldEntries[entry] = true
		}
	}
	sortedEntries := sortedEntrySet(oldEntries)

	// Skip the write entirely when nothing would actually change; this is
	// what makes repeated Ensure calls for the same entries idempotent.
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
	result = append(result, sortedEntries...)
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

// Remove reverses Ensure for the given entries: it deletes them from the
// lyx-managed block in <repoRoot>/.gitignore, leaving any other module's
// entries, the block itself, and all content outside the block untouched.
// It exists so that `lyx init --undo` can revert only the entries it
// originally added via Ensure without disturbing a .gitignore that other
// modules (or the user) have since added their own entries to.
//
// Returns changed=false without touching the file when: the file does not
// exist, it has no managed block, or none of the given entries are
// currently in the block (nothing to remove). Returns changed=true when the
// file was rewritten -- either with just the given entries dropped from the
// block, or with the entire managed block (and the blank line Ensure
// inserts before it) removed when dropping them would leave the block
// empty, restoring the file to what it would look like had Ensure never
// been called with those entries.
func Remove(repoRoot string, entries ...string) (changed bool, err error) {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		// No file means nothing to remove; mirror Ensure's no-op-on-absence
		// idempotency contract rather than treating this as an error.
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// Normalize line endings before parsing, matching Ensure.
	existingContent := strings.ReplaceAll(string(content), "\r\n", "\n")
	beforeBlock, afterBlock, oldEntries, blockExists := parseManagedBlock(existingContent)
	if !blockExists {
		return false, nil
	}

	// Only rewrite the file if at least one of the requested entries is
	// actually present in the block; otherwise this is a no-op removal.
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

	// Build the new file content
	var result []string
	result = append(result, beforeBlock...)
	if len(remainingEntries) > 0 {
		// Other modules' entries remain: keep the block, in the same shape
		// Ensure builds it in.
		if len(beforeBlock) > 0 && strings.TrimSpace(beforeBlock[len(beforeBlock)-1]) != "" {
			result = append(result, "") // blank line before block if there's content
		}
		result = append(result, startMarker)
		result = append(result, remainingEntries...)
		result = append(result, endMarker)
	}
	// If remainingEntries is empty, the block (and the blank line Ensure
	// would have inserted before it) is omitted entirely rather than left
	// behind as an empty shell.
	result = append(result, afterBlock...)

	// An empty result means the file has nothing left in it at all; write
	// it as a genuinely empty file rather than a lone trailing newline.
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

// parseManagedBlock splits .gitignore content into the lines before the
// lyx-managed block, the set of entries currently inside it, and the lines
// after it. Ensure and Remove both build on this single parse so their
// notion of "does the block exist" and "what does it contain" cannot drift
// apart from one another.
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
			// Capture old block entries (non-empty trimmed lines)
			if trimmed != "" {
				entries[trimmed] = true
			}
		case blockEnded:
			// After block has ended, collect lines after
			afterBlock = append(afterBlock, line)
		default:
			// Before block starts, collect lines before
			beforeBlock = append(beforeBlock, line)
		}
	}
	return beforeBlock, afterBlock, entries, blockExists
}

// sortedEntrySet returns the keys of an entry set sorted deterministically,
// matching the order Ensure writes entries into the block.
func sortedEntrySet(entries map[string]bool) []string {
	var sorted []string
	for entry := range entries {
		sorted = append(sorted, entry)
	}
	sort.Strings(sorted)
	return sorted
}

// entriesEqual reports whether two sorted entry lists contain the same
// elements in the same order.
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
