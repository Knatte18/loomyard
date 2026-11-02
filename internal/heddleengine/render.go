// render.go renders heddle's file targets into a worktree's anchor directory and reads the prompt passages of a prompt target.

package heddleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// Marker opens every file heddle renders, so a later render knows the file is its own to replace or remove.
const Marker = "<!-- rendered by lym heddle: change it with lym heddle set, never by hand -->"

// DefaultBoltLockWait is how long a render waits for the Bolt write lock when its Renderer sets no wait.
const DefaultBoltLockWait = 30 * time.Second

const (
	// tempDirName is the directory at a worktree's anchor a render writes its files into before renaming them onto their targets.
	tempDirName = ".heddle"
	// tempSuffix ends the name of a file inside tempDirName.
	tempSuffix = ".tmp"
	// renderLockName is the per-worktree lock file a render holds, under the scratch directory's heddle directory.
	renderLockName = "render.lock"
	// renderVerb is the command every render refusal names as the way to run it again.
	renderVerb = "lym heddle render"
)

// RenderRequest is one worktree to render.
// Anchor is the worktree's anchor directory, where the file targets render and the repository's own CLAUDE and AGENTS files are read; it is the worktree's root in a worktree with no anchor.
// ScratchDir is the pair's never-tracked directory, BoltDir the Bolt checkout holding the store, and Excluded the anchor-relative paths fabric's exclude list names.
// Only names the file targets to render, all of them when empty.
type RenderRequest struct {
	Anchor     string
	ScratchDir string
	BoltDir    string
	Excluded   []string
	Only       []string
}

// Report lists the files a render wrote and removed, as anchor-relative paths, and the warnings it raised.
type Report struct {
	Written  []string
	Removed  []string
	Warnings []string
}

// Renderer renders and reads the store.
// BoltLockWait is how long a read waits for the Bolt write lock, zero meaning DefaultBoltLockWait.
type Renderer struct {
	BoltLockWait time.Duration
}

// Render writes the file targets of req into its anchor from the entries of the shipped layer and the store.
// It holds the worktree's render lock for the whole read and write, so two renders of one worktree serialize and the last one's text stands whole.
// A target with no entry loses its marked file; a target the exclude list does not cover, or whose file lacks the marker, is left alone with a warning.
// The lock wait running out fails the render before it writes anything.
func (r Renderer) Render(req RenderRequest) (Report, error) {
	var report Report
	lockDir := filepath.Join(req.ScratchDir, lymdirs.HeddleDirName)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return report, fmt.Errorf("create %s: %w", lockDir, err)
	}
	renderLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, renderLockName))
	if err != nil {
		return report, err
	}
	defer func() { _ = renderLock.Release() }()

	layers, err := r.read(req.BoltDir)
	if err != nil {
		return report, err
	}
	if layers.OverrideWarning != "" {
		report.warn(layers.OverrideWarning)
	}
	entries := layers.Ordered()

	tempDir := filepath.Join(req.Anchor, tempDirName)
	blocked, err := tempDirBlocked(tempDir)
	if err != nil {
		return report, err
	}
	if blocked {
		report.warn(fmt.Sprintf("%s holds something heddle did not write, so no file is written; way forward: move or remove it, then run `%s`", tempDir, renderVerb))
	}

	for _, target := range targetTable {
		if target.Delivery != DeliveryFile || (len(req.Only) > 0 && !slices.Contains(req.Only, target.Name)) {
			continue
		}
		text, held, err := fileText(target, req.Anchor, entries)
		if err != nil {
			return report, err
		}
		path := filepath.Join(req.Anchor, target.File)
		existing, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return report, fmt.Errorf("read %s: %w", path, err)
		}
		marked := err == nil && strings.HasPrefix(string(existing), Marker)
		if !held {
			if marked {
				if err := os.Remove(path); err != nil {
					return report, fmt.Errorf("remove %s: %w", path, err)
				}
				report.Removed = append(report.Removed, target.File)
			}
			continue
		}
		switch {
		case blocked:
		case !slices.Contains(req.Excluded, target.File):
			report.warn(fabricengine.ExcludeWayForward(target.File))
		case !slices.Contains(req.Excluded, tempDirName):
			report.warn(fabricengine.ExcludeWayForward(tempDirName))
		case err == nil && !marked:
			report.warn(fmt.Sprintf("%s exists and was not rendered by heddle, so it is left untouched; way forward: move or remove it, then run `%s`", path, renderVerb))
		default:
			if err := writeRendered(tempDir, target.File, path, text); err != nil {
				return report, err
			}
			report.Written = append(report.Written, target.File)
		}
	}
	if !blocked {
		if err := removeTempDir(tempDir); err != nil {
			return report, err
		}
	}
	return report, nil
}

// warn adds a warning once.
func (r *Report) warn(text string) {
	if !slices.Contains(r.Warnings, text) {
		r.Warnings = append(r.Warnings, text)
	}
}

// read loads the layers from boltDir while holding the Bolt write lock, so it sees the old store or the new, never an absent file in mid-move.
// The lock wait running out is an error naming the render verb as the way to run it again.
func (r Renderer) read(boltDir string) (Layers, error) {
	wait := r.BoltLockWait
	if wait == 0 {
		wait = DefaultBoltLockWait
	}
	boltLock, acquired, err := lock.AcquireWriteLockWithin(filepath.Join(boltDir, fabricengine.BoardWriteLockFile), wait)
	if err != nil {
		return Layers{}, err
	}
	if !acquired {
		return Layers{}, fmt.Errorf("the Bolt checkout stayed locked for %s, so %s was not read; way forward: run `%s` again", wait, FileName, renderVerb)
	}
	defer func() { _ = boltLock.Release() }()
	return ReadLayers(boltDir)
}

// tempDirBlocked reports whether dir is a file, or a directory holding anything but files that end in tempSuffix.
func tempDirBlocked(dir string) (bool, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return true, nil
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, dirEntry := range dirEntries {
		if !dirEntry.Type().IsRegular() || !strings.HasSuffix(dirEntry.Name(), tempSuffix) {
			return true, nil
		}
	}
	return false, nil
}

// writeRendered writes text to tempDir/<name>.tmp, overwriting a leftover, and renames it onto path.
func writeRendered(tempDir, name, path, text string) error {
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", tempDir, err)
	}
	tempPath := filepath.Join(tempDir, name+tempSuffix)
	if err := os.WriteFile(tempPath, []byte(text), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename %s onto %s: %w", tempPath, path, err)
	}
	return nil
}

// removeTempDir removes the leftover files of dir that end in tempSuffix and then dir, when it exists.
func removeTempDir(dir string) error {
	dirEntries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, dirEntry := range dirEntries {
		if err := os.Remove(filepath.Join(dir, dirEntry.Name())); err != nil {
			return fmt.Errorf("remove %s: %w", filepath.Join(dir, dirEntry.Name()), err)
		}
	}
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}

// fileText builds the text a file target renders to at anchor from the layered entries that name it, and reports false when none does.
// It opens with Marker.
// The shipped layer's texts follow under their own heading, then the repository's, under a heading of their own only when shipped texts precede them.
// The claude-file target imports the AGENTS.md files the anchor holds when the anchor holds no CLAUDE.md, and the codex-file target opens with the anchor's AGENTS.md.
func fileText(target Target, anchor string, entries []LayeredEntry) (string, bool, error) {
	var shipped, repository []string
	for _, entry := range entries {
		if !slices.Contains(entry.Targets, target.Name) {
			continue
		}
		text := strings.Trim(entry.Text, "\n")
		if entry.Layer == LayerShipped {
			shipped = append(shipped, text)
		} else {
			repository = append(repository, text)
		}
	}
	var texts []string
	if len(shipped) > 0 {
		texts = append(append(texts, shippedHeading), shipped...)
		if len(repository) > 0 {
			texts = append(texts, repositoryHeading)
		}
	}
	texts = append(texts, repository...)
	if len(texts) == 0 {
		return "", false, nil
	}
	parts := []string{Marker}
	switch target.Name {
	case "claude-file":
		if !fileExists(filepath.Join(anchor, "CLAUDE.md")) && !fileExists(filepath.Join(anchor, ".claude", "CLAUDE.md")) {
			var imports []string
			for _, agents := range []string{"AGENTS.md", ".claude/AGENTS.md"} {
				if fileExists(filepath.Join(anchor, filepath.FromSlash(agents))) {
					imports = append(imports, "@"+agents)
				}
			}
			if len(imports) > 0 {
				parts = append(parts, strings.Join(imports, "\n"))
			}
		}
	case "codex-file":
		content, err := os.ReadFile(filepath.Join(anchor, "AGENTS.md"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, fmt.Errorf("read AGENTS.md: %w", err)
		}
		if err == nil {
			parts = append(parts, strings.Trim(string(content), "\n"))
		}
	}
	return strings.Join(append(parts, texts...), "\n\n") + "\n", true, nil
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Show returns the text the named target renders to now for the worktree anchor, from the shipped layer and the store, and writes nothing.
// A prompt target gives its passages joined by blank lines, and a target with no entry gives no text.
func (r Renderer) Show(boltDir, target, anchor string) (string, error) {
	named, ok := LookupTarget(target)
	if !ok {
		return "", fmt.Errorf("unknown target %q; way forward: use one of %s", target, strings.Join(TargetNames(), ", "))
	}
	layers, err := r.read(boltDir)
	if err != nil {
		return "", err
	}
	if named.Delivery == DeliveryPrompt {
		return strings.Join(passagesOf(layers, named.Name), "\n\n"), nil
	}
	text, _, err := fileText(named, anchor, layers.Ordered())
	return text, err
}

// Passages returns the texts of the entries aimed at a prompt target that reaches a session of engine whose strand role is role, the shipped layer's first, each layer in order.
// An entry aimed at several such targets gives its text once, and a session no prompt target reaches gets none.
func (r Renderer) Passages(boltDir, engine, role string) ([]string, error) {
	targets := promptTargetsFor(engine, role)
	if len(targets) == 0 {
		return nil, nil
	}
	layers, err := r.read(boltDir)
	if err != nil {
		return nil, err
	}
	return passagesOf(layers, targets...), nil
}

// passagesOf returns the texts of the layered entries that name one of targets, in render order.
func passagesOf(layers Layers, targets ...string) []string {
	var passages []string
	for _, entry := range layers.Ordered() {
		if slices.ContainsFunc(entry.Targets, func(name string) bool { return slices.Contains(targets, name) }) {
			passages = append(passages, strings.Trim(entry.Text, "\n"))
		}
	}
	return passages
}
