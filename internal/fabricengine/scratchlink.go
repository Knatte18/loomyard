// scratchlink.go implements the scratch step of fabric wiring.
// The warp worktree's root `.scratch` becomes a directory link into the pair's weft-side `.lyx`.
// Files an agent writes under `.scratch/` then never change the root entry's own modification time, which source-scan tests hash into their `go test` cache key.
// The step runs under every wiring verb and adopts an existing real `.scratch` by moving it into the link target.
// It never fails wiring: what it cannot settle it leaves as found and reports with a way forward.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// scratchTargetName is the directory under the weft-side `.lyx` the root `.scratch` link points at.
const scratchTargetName = "scratch"

// scratchReconcileHint is the closing way forward every scratch report shares.
const scratchReconcileHint = "then rerun `lyx fabric reconcile`"

// scratchBeforeLinkHook, when non-nil, runs between the classification of the root `.scratch` and the creation of its link.
// It exists only so a test can recreate `.scratch` in that window; production never sets it.
var scratchBeforeLinkHook func()

// scratchPathTracked answers whether the warp index tracks a path.
// It is a variable only so a test can make the answer fail; production keeps PathTracked.
var scratchPathTracked = PathTracked

// scratchOutcome is what the scratch step reports.
type scratchOutcome struct {
	// changed names what the step changed; empty when nothing changed.
	changed string
	// left names what the step left as found or could not do, with its way forward; empty when nothing was left.
	left string
}

// detail joins the non-empty parts of the outcome with "; ".
func (o scratchOutcome) detail() string {
	parts := make([]string, 0, 2)
	if o.changed != "" {
		parts = append(parts, o.changed)
	}
	if o.left != "" {
		parts = append(parts, o.left)
	}
	return strings.Join(parts, "; ")
}

// scratchLinkTarget returns the weft-side directory the root `.scratch` link of slug's warp worktree points at.
// It is built like every weft-side junction target and never by resolving the warp `.lyx` link.
func scratchLinkTarget(l *lyxcwd.Location, slug string) string {
	return filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, lyxdirs.DotLyxDirName, scratchTargetName)
}

// withoutRootScratchLink returns names without the root scratch link's name when the anchor is the worktree root.
// It returns names unchanged otherwise, since a subpath anchor's on-disk scan never sees the root link.
func withoutRootScratchLink(l *lyxcwd.Location, names []string) []string {
	if filepath.Clean(l.AnchorRel) != "." {
		return names
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if name != scratchDirName {
			kept = append(kept, name)
		}
	}
	return kept
}

// seedScratchLinkExclude makes git ignore the root `.scratch` link of the warp worktree at worktreeDir.
// The pattern carries no trailing slash, since git matches a link as a file and a directory-only pattern would not exclude it;
// a legacy `/.scratch/` line is dropped.
// It records KindFileWritten at the exclude file only when the content changed, and returns the exclude path as far as it is known.
func seedScratchLinkExclude(rec *Mutations, worktreeDir string) (excludePath string, err error) {
	const pattern = "/" + scratchDirName
	const legacyPattern = pattern + "/"

	excludePath, changed, err := mutateGitExclude(worktreeDir, func(content string) (string, error) {
		found := false
		kept := make([]string, 0)
		for _, line := range strings.Split(content, "\n") {
			switch strings.TrimSpace(line) {
			case legacyPattern:
				continue
			case pattern:
				found = true
			}
			kept = append(kept, line)
		}
		content = strings.Join(kept, "\n")
		if found {
			return content, nil
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + pattern + "\n", nil
	})
	if err != nil {
		return excludePath, err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return excludePath, nil
}

// wireScratchLink makes the root `.scratch` of slug's warp worktree a directory link to scratchLinkTarget.
// In order it seeds the exclude, refuses to link over tracked paths, then classifies the root entry:
// an absent root is linked, a correct link has its target restored, a real directory with an absent target is moved into the target and linked,
// and every other shape is left as found.
// It never returns an error and removes nothing; a leave or a failure comes back in the outcome with its way forward.
// It creates nothing when the pair's weft worktree is absent.
func wireScratchLink(rec *Mutations, l *lyxcwd.Location, slug string) scratchOutcome {
	warp := WorktreePath(l, slug)
	root := filepath.Join(warp, scratchDirName)
	target := scratchLinkTarget(l, slug)

	excludePath, err := seedScratchLinkExclude(rec, warp)
	if err != nil {
		return scratchOutcome{left: fmt.Sprintf("scratch exclude not written at %s: %v; clear what blocks writing the exclude file, %s", excludePath, err, scratchReconcileHint)}
	}

	tracked, err := scratchPathTracked(warp, scratchDirName)
	if err != nil {
		return scratchFailure(root, target, root, err)
	}
	if tracked {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch left as found at %s: tracked by git; stop tracking %s on the warp branch, move the untracked content into the target %s and remove the root %s, %s",
			root, scratchDirName, target, scratchDirName, scratchReconcileHint)}
	}

	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return linkAbsentScratchRoot(rec, l, slug, root, target)
		}
		return scratchFailure(root, target, root, err)
	}

	isLink, err := fslink.IsLink(root)
	if err != nil {
		return scratchFailure(root, target, root, err)
	}
	if isLink {
		return restoreScratchTarget(rec, l, slug, root, target)
	}
	if !info.IsDir() {
		return scratchLeave(root, target, "a regular file")
	}
	return adoptScratchDirectory(rec, l, slug, root, target)
}

// unlinkScratchLink removes the root `.scratch` link of slug's warp worktree through the destructive gate, leaving its target untouched.
// It reads the root entry by its own path, so it sees the link at a subpath anchor too, where the anchored junction scan does not.
// It does nothing when the entry is absent, is not a link, or is a link that does not resolve into the pair's weft worktree;
// git's own worktree removal unlinks such a link without following it.
// A gate refusal comes back naming the root and the manual removal that lets a rerun proceed.
func unlinkScratchLink(rec *Mutations, l *lyxcwd.Location, slug string) error {
	warp := WorktreePath(l, slug)
	root := filepath.Join(warp, scratchDirName)

	if _, err := os.Lstat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect scratch link %s: %w", root, err)
	}
	isLink, err := fslink.IsLink(root)
	if err != nil || !isLink {
		return err
	}
	owned, err := linkIsFabricOwned(l, slug, root)
	if err != nil || !owned {
		return err
	}

	req := pathRequest{
		what:      "remove scratch link",
		container: warp,
		target:    root,
		slug:      nil,
		ownership: ownedDriftedWiredJunction([]string{root}),
		dirtiness: dirtinessNA("a link holds no content; the weft target it points at is untouched"),
		force:     false,
	}
	if err := removeLink(rec, req); err != nil {
		return fmt.Errorf("remove scratch link %s: %w; remove the root %s link by hand and rerun `lyx fabric remove`", root, err, scratchDirName)
	}
	return nil
}

// linkAbsentScratchRoot creates the target and the link for a root `.scratch` that does not exist.
func linkAbsentScratchRoot(rec *Mutations, l *lyxcwd.Location, slug, root, target string) scratchOutcome {
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}
	if failedPath, err := mkdirScratchTarget(rec, target); err != nil {
		return scratchFailure(root, target, failedPath, err)
	}
	return createScratchLink(rec, root, target, scratchOutcome{changed: fmt.Sprintf("scratch linked: %s → %s", root, target)})
}

// restoreScratchTarget handles a root `.scratch` that is a link: a link to the expected target gets the target recreated when absent,
// and a link to anything else is left.
func restoreScratchTarget(rec *Mutations, l *lyxcwd.Location, slug, root, target string) scratchOutcome {
	raw, err := fslink.RawTarget(root)
	if err != nil {
		return scratchFailure(root, target, root, err)
	}
	if filepath.Clean(raw) != filepath.Clean(target) {
		return scratchLeave(root, target, "a link to another target")
	}
	exists, err := scratchTargetExists(target)
	if err != nil {
		return scratchFailure(root, target, target, err)
	}
	if exists {
		return scratchOutcome{}
	}
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}
	if failedPath, err := mkdirScratchTarget(rec, target); err != nil {
		return scratchFailure(root, target, failedPath, err)
	}
	return scratchOutcome{changed: fmt.Sprintf("scratch target restored at %s", target)}
}

// adoptScratchDirectory moves a real root `.scratch` into the absent target and links it; with the target present it leaves the directory.
func adoptScratchDirectory(rec *Mutations, l *lyxcwd.Location, slug, root, target string) scratchOutcome {
	exists, err := scratchTargetExists(target)
	if err != nil {
		return scratchFailure(root, target, target, err)
	}
	if exists {
		return scratchLeave(root, target, fmt.Sprintf("a real directory while the target %s already exists", target))
	}
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}

	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return scratchFailure(root, target, parent, err)
	}
	if err := os.Rename(root, target); err != nil {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch left as found at %s: a real directory whose move to %s failed: %v; stop reed (an open handle inside %s is the expected cause on Windows), move its content into the target %s and remove the root %s, %s",
			root, target, err, scratchDirName, target, scratchDirName, scratchReconcileHint)}
	}
	rec.Append(KindPathRenamed, target, hubRelativeTarget(l.HubPath, root))

	if scratchBeforeLinkHook != nil {
		scratchBeforeLinkHook()
	}
	if err := fslink.CreateDirLink(root, target); err != nil {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch link not created at %s after its content moved to %s: %v; move the recreated %s's content into %s, remove the root %s, %s",
			root, target, err, scratchDirName, target, scratchDirName, scratchReconcileHint)}
	}
	rec.Append(KindLinkCreated, root, "")
	return scratchOutcome{changed: fmt.Sprintf("moved %s into %s and linked it", scratchDirName, target)}
}

// createScratchLink runs the before-link hook and links root to target, returning success when the link was made.
func createScratchLink(rec *Mutations, root, target string, success scratchOutcome) scratchOutcome {
	if scratchBeforeLinkHook != nil {
		scratchBeforeLinkHook()
	}
	if err := fslink.CreateDirLink(root, target); err != nil {
		return scratchFailure(root, target, root, err)
	}
	rec.Append(KindLinkCreated, root, "")
	return success
}

// mkdirScratchTarget creates the target directory and records KindDirCreated when it was absent.
// On failure it returns the path the error concerns.
func mkdirScratchTarget(rec *Mutations, target string) (failedPath string, err error) {
	exists, err := scratchTargetExists(target)
	if err != nil {
		return target, err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return target, err
	}
	if !exists {
		rec.Append(KindDirCreated, target, "")
	}
	return "", nil
}

// scratchTargetExists reports whether anything stands at target; a missing path and a parent that is not a directory both report false.
func scratchTargetExists(target string) (bool, error) {
	_, err := os.Lstat(target)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	return false, err
}

// weftWorktreePresent reports whether slug's weft worktree directory exists.
// The step creates nothing without it, since a weft torn down mid-repair would otherwise get a plain directory recreated at its `.lyx/scratch` path.
func weftWorktreePresent(l *lyxcwd.Location, slug string) bool {
	_, err := os.Stat(RecordsWorktreePath(l, slug))
	return !errors.Is(err, fs.ErrNotExist)
}

// scratchLeave reports a root `.scratch` left as found because of its shape.
func scratchLeave(root, target, shape string) scratchOutcome {
	return scratchOutcome{left: fmt.Sprintf(
		"scratch left as found at %s: %s; move its content into the target %s and remove the root %s, %s",
		root, shape, target, scratchDirName, scratchReconcileHint)}
}

// scratchFailure reports a step that failed on failedPath with the root left as found.
func scratchFailure(root, target, failedPath string, err error) scratchOutcome {
	return scratchOutcome{left: fmt.Sprintf(
		"scratch link not created at %s: %s: %v; clear what the error names, moving any content under a root %s into the target %s before removing it, %s",
		root, failedPath, err, scratchDirName, target, scratchReconcileHint)}
}
