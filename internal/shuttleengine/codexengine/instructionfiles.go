// instructionfiles.go works out the instruction files Codex reads in a launch and the CLAUDE.md files it does not.
// It sizes the cap on the project instructions Codex reads, and writes the pointers to the files left unread.

package codexengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/claudeimports"
	"github.com/Knatte18/loomyard/internal/heddleengine"
)

const (
	// agentsOverrideFile is the file Codex reads in a directory before every other instruction file.
	agentsOverrideFile = heddleengine.CodexOverrideInstructionsFile
	// agentsFile is the file Codex reads in a directory that holds no override.
	agentsFile = "AGENTS.md"
	// defaultProjectDocMaxBytes is Codex's own cap on the project instruction text it reads.
	defaultProjectDocMaxBytes = 32 * 1024
	// instructionPointersItemName labels the instruction-file pointers in a refusal that names the largest item.
	instructionPointersItemName = "the instruction-file pointers"
	// instructionPointersWayForward shrinks the instruction-file pointers.
	instructionPointersWayForward = "move the rules into fewer CLAUDE.md files"
)

// unreadScanSkippedDirectories names the directories the scan for unread CLAUDE.md files never enters:
// version control's own directory, and the dependency trees whose CLAUDE.md files are not the repository's rules.
var unreadScanSkippedDirectories = []string{".git", "node_modules", "vendor"}

// codexReadOrder lists the files Codex looks for in a directory, of which it reads the first that exists.
var codexReadOrder = []string{agentsOverrideFile, agentsFile, instructionFallbackFile}

// projectDocMaxBytes returns the cap on the project instruction text a session starting in workDir needs.
// Codex reads one file in each directory from the checkout root down to workDir:
// the first of AGENTS.override.md, AGENTS.md and CLAUDE.md that exists there.
// The cap is the sum of their sizes, and never less than Codex's own default.
// A file that cannot be examined for a reason other than being absent refuses the launch, naming it.
func projectDocMaxBytes(found checkout, workDir string) (int, error) {
	relative, err := filepath.Rel(found.root, workDir)
	if err != nil {
		return 0, fmt.Errorf("codexengine: place the working directory %s in the checkout %s: %w", workDir, found.root, err)
	}
	directories := []string{found.root}
	if relative != "." {
		directory := found.root
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			directory = filepath.Join(directory, part)
			directories = append(directories, directory)
		}
	}
	total := 0
	for _, directory := range directories {
		for _, name := range codexReadOrder {
			info, err := os.Stat(filepath.Join(directory, name))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return 0, fmt.Errorf("codexengine: examine the instruction file %s: %w; way forward: fix its permissions or remove it", filepath.Join(directory, name), err)
			}
			total += int(info.Size())
			break
		}
	}
	return max(total, defaultProjectDocMaxBytes), nil
}

// unreadClaudeFiles returns, sorted, the absolute path of each CLAUDE.md in the checkout that Codex does not read itself.
// The walk starts at the checkout root, follows no link and skips the directories in unreadScanSkippedDirectories.
// It never skips the root or a directory that holds workDir, since a session working there reads its files.
// A CLAUDE.md is unread when it lies in a directory below workDir.
// It is also unread when it sits beside an AGENTS.md.
// The exception is workDir when the AGENTS.override.md there opens with heddleengine.Marker.
// Heddle's render carries the CLAUDE.md, imports expanded, in that case, whether or not an AGENTS.md is there.
// An override without the marker is the repository's own, so the CLAUDE.md beside it stays unread.
// A CLAUDE.md on the path from the root to workDir that Codex reads, whose imports resolve to files, is unread too:
// Codex reads it without expanding them.
// A directory the walk cannot read is skipped and named in a warning, since the pointers are advisory.
func (c *Codex) unreadClaudeFiles(found checkout, workDir string) []string {
	var unread []string
	filepath.WalkDir(found.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			c.warn("codexengine: a directory the scan for unread CLAUDE.md files cannot read is skipped; way forward: fix its permissions", "path", path, "error", err)
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if slices.Contains(unreadScanSkippedDirectories, entry.Name()) && path != found.root && !isWithin(path, workDir) {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Name() == instructionFallbackFile && claudeFileIsUnread(path, workDir) {
			unread = append(unread, path)
		}
		return nil
	})
	slices.Sort(unread)
	return unread
}

// claudeFileIsUnread reports whether the CLAUDE.md at path is a file Codex does not read itself in a session in workDir.
func claudeFileIsUnread(path, workDir string) bool {
	directory := filepath.Dir(path)
	if directory != workDir && isWithin(workDir, directory) {
		return true
	}
	if directory == workDir && heddleRendered(filepath.Join(directory, agentsOverrideFile)) {
		return false
	}
	if fileExists(filepath.Join(directory, agentsFile)) {
		return true
	}
	if !isWithin(directory, workDir) {
		return false
	}
	expanded, err := claudeimports.Expand(path)
	return err == nil && len(expanded.Files) > 0
}

// isWithin reports whether child is parent or lies below it.
func isWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// fileExists reports whether path names something that can be examined.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// heddleRendered reports whether the file at path opens with the marker heddle's render writes.
func heddleRendered(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.HasPrefix(string(data), heddleengine.Marker)
}

// instructionPointers returns the passage that names each CLAUDE.md in paths and tells the session to read it.
// It returns "" for no path.
func instructionPointers(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	lines := []string{"These CLAUDE.md files hold rules Codex does not read itself." +
		" Read each one in full, and the files its `@` lines import, before you work in its directory:"}
	for _, path := range paths {
		lines = append(lines, "- "+path)
	}
	return strings.Join(lines, "\n")
}
