// gitignore_test.go — table-driven tests for the gitignore package.
//
// Covers: new-file creation, set-merge across modules, idempotency,
// outside-block preservation, delimiter correctness, and Remove's mirror
// behavior (block deletion, partial removal, and no-op cases).

package gitignore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitignore"
)

// TestEnsureNewFileCreation tests that a new .gitignore is created with delimiters and entries.
func TestEnsureNewFileCreation(t *testing.T) {
	tmpDir := t.TempDir()

	changed, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	if !changed {
		t.Errorf("expected changed=true for new file, got false")
	}

	// Verify .gitignore exists and contains the expected content
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Check for start marker
	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("expected start marker in .gitignore, got: %s", contentStr)
	}

	// Check for end marker
	if !strings.Contains(contentStr, "# === end lyx-managed ===") {
		t.Errorf("expected end marker in .gitignore, got: %s", contentStr)
	}

	// Check for .lyx/ entry
	if !strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ entry in .gitignore, got: %s", contentStr)
	}

	// Verify final newline
	if !strings.HasSuffix(contentStr, "\n") {
		t.Errorf("expected .gitignore to end with newline")
	}
}

// TestEnsureAddEntryToExistingBlock tests adding an entry to an existing block.
func TestEnsureAddEntryToExistingBlock(t *testing.T) {
	tmpDir := t.TempDir()

	// First call: create with .lyx/
	changed1, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("first Ensure failed: %v", err)
	}
	if !changed1 {
		t.Errorf("expected changed=true for new file")
	}

	// Second call: add .vscode/
	changed2, err := gitignore.Ensure(tmpDir, ".vscode/")
	if err != nil {
		t.Fatalf("second Ensure failed: %v", err)
	}
	if !changed2 {
		t.Errorf("expected changed=true when adding new entry")
	}

	// Verify both entries exist in sorted order
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Check that both entries are present
	if !strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ in .gitignore")
	}
	if !strings.Contains(contentStr, ".vscode/") {
		t.Errorf("expected .vscode/ in .gitignore")
	}

	// Verify they're in sorted order by checking .lyx/ comes before .vscode/
	lyxIdx := strings.Index(contentStr, ".lyx/")
	vscodeIdx := strings.Index(contentStr, ".vscode/")
	if lyxIdx > vscodeIdx {
		t.Errorf("entries not in sorted order: .lyx/ at %d, .vscode/ at %d", lyxIdx, vscodeIdx)
	}
}

// TestEnsureIdempotency tests that re-adding the same entry returns changed=false.
func TestEnsureIdempotency(t *testing.T) {
	tmpDir := t.TempDir()

	// First call: create with .lyx/
	changed1, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("first Ensure failed: %v", err)
	}
	if !changed1 {
		t.Errorf("expected changed=true for new file")
	}

	// Capture the file content
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content1, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	// Second call: re-add the same entry
	changed2, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("second Ensure failed: %v", err)
	}
	if changed2 {
		t.Errorf("expected changed=false for idempotent re-add, got true")
	}

	// Verify content is identical
	content2, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore after second call: %v", err)
	}

	if string(content1) != string(content2) {
		t.Errorf("content changed on idempotent re-add")
	}
}

// TestEnsureTwoModuleSetMerge tests that two separate module calls leave both entries.
func TestEnsureTwoModuleSetMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Module 1: board init contributes .lyx/
	changed1, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("first Ensure failed: %v", err)
	}
	if !changed1 {
		t.Errorf("expected changed=true for new file")
	}

	// Module 2: ide contributes .vscode/
	changed2, err := gitignore.Ensure(tmpDir, ".vscode/")
	if err != nil {
		t.Fatalf("second Ensure failed: %v", err)
	}
	if !changed2 {
		t.Errorf("expected changed=true when adding new entry")
	}

	// Verify both entries coexist in one block
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Count the start markers (should be exactly 1)
	startCount := strings.Count(contentStr, "# === lyx-managed ===")
	if startCount != 1 {
		t.Errorf("expected exactly 1 start marker, found %d", startCount)
	}

	// Count the end markers (should be exactly 1)
	endCount := strings.Count(contentStr, "# === end lyx-managed ===")
	if endCount != 1 {
		t.Errorf("expected exactly 1 end marker, found %d", endCount)
	}

	// Verify both entries are present
	if !strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ in .gitignore")
	}
	if !strings.Contains(contentStr, ".vscode/") {
		t.Errorf("expected .vscode/ in .gitignore")
	}
}

// TestEnsurePreservesOutsideBlock tests that content outside the block is preserved.
func TestEnsurePreservesOutsideBlock(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore with content outside the block
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	outsideContent := "*.log\ntemp/\n"
	err := os.WriteFile(gitignorePath, []byte(outsideContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write initial .gitignore: %v", err)
	}

	// Call Ensure
	changed, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}

	// Verify outside content is preserved
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "*.log") {
		t.Errorf("expected *.log to be preserved")
	}
	if !strings.Contains(contentStr, "temp/") {
		t.Errorf("expected temp/ to be preserved")
	}

	// Verify the managed block is present
	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("expected start marker")
	}
	if !strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ entry")
	}
}

// TestEnsureDelimiterExactness tests that delimiters are exact.
func TestEnsureDelimiterExactness(t *testing.T) {
	tmpDir := t.TempDir()

	changed, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true for new file")
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Check exact delimiters
	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("expected exact start marker: '# === lyx-managed ==='")
	}
	if !strings.Contains(contentStr, "# === end lyx-managed ===") {
		t.Errorf("expected exact end marker: '# === end lyx-managed ==='")
	}

	// Verify no typos or variations
	if strings.Contains(contentStr, "# === lyx-managed") && !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("found incorrect variant of start marker")
	}
	if strings.Contains(contentStr, "# === end lyx-managed") && !strings.Contains(contentStr, "# === end lyx-managed ===") {
		t.Errorf("found incorrect variant of end marker")
	}
}

// TestRemoveDeletesBlockWhenOnlyRemovedEntryPresent tests that Remove drops
// the entire managed block (both markers) when the only entry it contains
// is the one being removed, while preserving surrounding content.
func TestRemoveDeletesBlockWhenOnlyRemovedEntryPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Write outside content, then seed the block with only .lyx/ via Ensure.
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*.log\n"), 0o644); err != nil {
		t.Fatalf("failed to write initial .gitignore: %v", err)
	}
	if _, err := gitignore.Ensure(tmpDir, ".lyx/"); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	changed, err := gitignore.Remove(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if !changed {
		t.Errorf("Remove(dir, \".lyx/\") changed = false; want true")
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	contentStr := string(content)

	if strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("expected start marker to be gone, got: %q", contentStr)
	}
	if strings.Contains(contentStr, "# === end lyx-managed ===") {
		t.Errorf("expected end marker to be gone, got: %q", contentStr)
	}
	if strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ entry to be gone, got: %q", contentStr)
	}
	if !strings.Contains(contentStr, "*.log") {
		t.Errorf("expected surrounding content *.log to be preserved, got: %q", contentStr)
	}
}

// TestRemoveKeepsBlockWhenOtherEntriesRemain tests that Remove drops only
// the targeted entry when other modules' entries remain in the block.
func TestRemoveKeepsBlockWhenOtherEntriesRemain(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := gitignore.Ensure(tmpDir, ".lyx/", ".vscode/"); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	changed, err := gitignore.Remove(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if !changed {
		t.Errorf("Remove(dir, \".lyx/\") changed = false; want true")
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "# === lyx-managed ===") {
		t.Errorf("expected the block to survive, got: %q", contentStr)
	}
	if !strings.Contains(contentStr, "# === end lyx-managed ===") {
		t.Errorf("expected the block to survive, got: %q", contentStr)
	}
	if strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/ entry to be gone, got: %q", contentStr)
	}
	if !strings.Contains(contentStr, ".vscode/") {
		t.Errorf("expected .vscode/ entry to remain, got: %q", contentStr)
	}
}

// TestRemoveNoOpWhenEntryNotPresent tests that Remove leaves the file
// byte-for-byte unchanged when the requested entry was never in the block.
func TestRemoveNoOpWhenEntryNotPresent(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := gitignore.Ensure(tmpDir, ".vscode/"); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	before, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	changed, err := gitignore.Remove(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if changed {
		t.Errorf("Remove(dir, \".lyx/\") changed = true; want false")
	}

	after, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore after Remove: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("expected file content unchanged, before: %q, after: %q", before, after)
	}
}

// TestRemoveNoOpWhenFileMissing tests that Remove no-ops without creating a
// .gitignore file when none exists.
func TestRemoveNoOpWhenFileMissing(t *testing.T) {
	tmpDir := t.TempDir()

	changed, err := gitignore.Remove(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if changed {
		t.Errorf("Remove(dir, \".lyx/\") changed = true; want false")
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); !os.IsNotExist(err) {
		t.Errorf("expected no .gitignore to be created, stat err: %v", err)
	}
}

// TestEnsureMultipleEntriesAtOnce tests adding multiple entries in one call.
func TestEnsureMultipleEntriesAtOnce(t *testing.T) {
	tmpDir := t.TempDir()

	changed, err := gitignore.Ensure(tmpDir, ".lyx/", ".vscode/", ".idea/")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true for new file")
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Verify all entries are present
	if !strings.Contains(contentStr, ".lyx/") {
		t.Errorf("expected .lyx/")
	}
	if !strings.Contains(contentStr, ".vscode/") {
		t.Errorf("expected .vscode/")
	}
	if !strings.Contains(contentStr, ".idea/") {
		t.Errorf("expected .idea/")
	}

	// Verify they're in sorted order
	idxIdea := strings.Index(contentStr, ".idea/")
	idxLyx := strings.Index(contentStr, ".lyx/")
	idxVscode := strings.Index(contentStr, ".vscode/")

	if idxIdea > idxLyx || idxLyx > idxVscode {
		t.Errorf("entries not in sorted order: .idea/ at %d, .lyx/ at %d, .vscode/ at %d", idxIdea, idxLyx, idxVscode)
	}
}

// TestEnsureBlankLineBeforeBlock tests that a blank line is added before the block when preceding content exists.
func TestEnsureBlankLineBeforeBlock(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore with existing content
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	outsideContent := "*.log\n"
	err := os.WriteFile(gitignorePath, []byte(outsideContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write initial .gitignore: %v", err)
	}

	// Call Ensure
	changed, err := gitignore.Ensure(tmpDir, ".lyx/")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	contentStr := string(content)

	// Check that there's a blank line before the start marker
	lines := strings.Split(contentStr, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "# === lyx-managed ===" {
			if i == 0 {
				t.Errorf("start marker should not be at line 0 when preceding content exists")
			}
			if strings.TrimSpace(lines[i-1]) != "" {
				t.Errorf("expected blank line before start marker, found: %s", lines[i-1])
			}
			break
		}
	}
}
