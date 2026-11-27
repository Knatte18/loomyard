// scratchlink.go implements the scratch step of fabric wiring.
// The `.scratch` at the warp worktree's anchor becomes a directory link into the pair's weft-side `.lym`.
// Files an agent writes under `.scratch/` never change the link's own modification time.
// Source-scan tests hash that modification time into their `go test` cache key.
// The step runs under every wiring verb and adopts an existing real `.scratch` by moving it into the link target.
// At a subpath anchor it also retires the root `.scratch` link and exclude line that fabric wiring owns.
// Nothing of lym's lands at the repository root in that case.
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// scratchTargetName is the directory under the weft-side `.lym` the anchor's `.scratch` link points at.
const scratchTargetName = "scratch"

// scratchReconcileHint is the closing way forward every scratch report shares.
const scratchReconcileHint = "then rerun `lym fabric reconcile`"

// scratchBeforeLinkHook, when non-nil, runs between the classification of the anchor's `.scratch` and the creation of its link.
// It exists so a test can recreate `.scratch` in that window; production never sets it.
var scratchBeforeLinkHook func()

// scratchPathTracked answers whether the warp index tracks a path.
// It is a variable so a test can make the answer fail; production uses the real check.
var scratchPathTracked = PathTracked

// scratchOutcome is what the scratch step reports.
type scratchOutcome struct {
	// changed names what the step changed; empty when nothing changed.
	changed string
	// left names what the step left as found or could not do, with its way forward; empty when nothing was left.
	left string
}

func (o scratchOutcome) detail() string {
	return joinNonEmpty(o.changed, o.left)
}

// and returns the outcome of this step followed by next, each part joined with "; ".
func (o scratchOutcome) and(next scratchOutcome) scratchOutcome {
	return scratchOutcome{changed: joinNonEmpty(o.changed, next.changed), left: joinNonEmpty(o.left, next.left)}
}

func joinNonEmpty(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "; ")
}

// scratchLinkPath returns the `.scratch` link at the anchor of slug's warp worktree, the worktree root at anchor ".".
func scratchLinkPath(l *lymcwd.Location, slug string) string {
	return filepath.Join(WorktreePath(l, slug), l.AnchorRel, scratchDirName)
}

// scratchLinkTarget returns the weft-side directory the `.scratch` link of slug's warp worktree points at.
// It is built like every weft-side junction target and never by resolving the warp `.lym` link.
func scratchLinkTarget(l *lymcwd.Location, slug string) string {
	return filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.DotLymDirName, scratchTargetName)
}

// anchorIsRoot reports whether l's anchor is the worktree root.
func anchorIsRoot(l *lymcwd.Location) bool {
	return filepath.Clean(l.AnchorRel) == "."
}

// withoutScratchLink returns names without the scratch link's name.
// The link sits in the anchored directory beside the junctions, so an on-disk junction scan finds it, yet it is never a junction.
func withoutScratchLink(names []string) []string {
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if name != scratchDirName {
			kept = append(kept, name)
		}
	}
	return kept
}

// seedScratchLinkExclude makes git ignore the `.scratch` link at anchorRel of the warp worktree at worktreeDir.
// It writes a `/<anchorRel>/.scratch` line, or `/.scratch` at anchor ".".
// The pattern carries no trailing slash, because git matches a link as a file.
// A directory-only pattern would not exclude it.
// A legacy line with the trailing slash is dropped.
// It records KindFileWritten at the exclude file only when the content changed.
// It returns the exclude path as far as it is known.
func seedScratchLinkExclude(rec *Mutations, worktreeDir, anchorRel string) (excludePath string, err error) {
	pattern := excludePatternFor(anchorRel, scratchDirName)
	legacyPattern := pattern + "/"

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

// unseedScratchExclude drops the `.scratch` exclude line of anchorRel from the exclude file of slug's repository.
// The line goes with or without a trailing slash.
// It keeps the line while another worktree has an entry at that anchored `.scratch` path, link or not.
// The exclude file is shared by every worktree of the repository.
// It records KindFileWritten at the exclude file only when the content changed.
func unseedScratchExclude(rec *Mutations, l *lymcwd.Location, slug, anchorRel string) error {
	pattern := excludePatternFor(anchorRel, scratchDirName)
	excludePath, changed, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
		lines := strings.Split(content, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if trimmed := strings.TrimSpace(line); trimmed == pattern || trimmed == pattern+"/" {
				continue
			}
			kept = append(kept, line)
		}
		if len(kept) == len(lines) {
			return content, nil
		}
		// The census runs inside the exclude lock, so a concurrent wire in a sibling cannot invalidate it before the write.
		if scratchLinkedInSibling(l, slug, anchorRel) {
			return content, nil
		}
		return strings.Join(kept, "\n"), nil
	})
	if err != nil {
		return err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return nil
}

// scratchLinkedInSibling reports whether a warp worktree other than slug's has an entry at `<anchorRel>/.scratch`.
// The entry may be a link, a directory or a file.
// A worktree list that cannot be read, or a sibling entry that cannot be inspected, answers true.
// Keeping a dead line costs less than dirtying a live entry.
func scratchLinkedInSibling(l *lymcwd.Location, slug, anchorRel string) bool {
	entries, err := List(WorktreePath(l, slug))
	if err != nil {
		entries, err = List(l.WorktreePath())
	}
	if err != nil {
		return true
	}
	ownPath := filepath.Clean(WorktreePath(l, slug))
	for _, entry := range entries {
		entryPath := filepath.Clean(filepath.FromSlash(entry.Path))
		if entryPath == ownPath {
			continue
		}
		if _, err := os.Lstat(filepath.Join(entryPath, anchorRel, scratchDirName)); !errors.Is(err, fs.ErrNotExist) {
			return true
		}
	}
	return false
}

// wireScratchLink converges the scratch link of slug's warp worktree.
// At a subpath anchor it first retires the root `.scratch` link and exclude line.
// It then makes the `.scratch` at the anchor a directory link to the `scratch` directory under the weft-side `.lym`.
// It never returns an error; anything it leaves as found or cannot do comes back in the outcome with its way forward.
func wireScratchLink(rec *Mutations, l *lymcwd.Location, slug string) scratchOutcome {
	return retireRootScratchLink(rec, l, slug).and(wireAnchorScratchLink(rec, l, slug))
}

// retireRootScratchLink removes, at a subpath anchor, the fabric-owned root `.scratch` link of slug's warp worktree.
// It then removes the root `/.scratch` exclude line.
// An entry at the root that is not such a link is left as found and reported, with its exclude line kept.
// The line stays because dropping it would show that entry as untracked.
// It does nothing at anchor ".", where the root is the anchor.
func retireRootScratchLink(rec *Mutations, l *lymcwd.Location, slug string) scratchOutcome {
	if anchorIsRoot(l) {
		return scratchOutcome{}
	}
	root := filepath.Join(WorktreePath(l, slug), scratchDirName)
	anchorLink := scratchLinkPath(l, slug)

	var outcome scratchOutcome
	removed, err := removeOwnedScratchLink(rec, l, slug, root)
	if err != nil {
		return scratchOutcome{left: fmt.Sprintf("root scratch link not retired at %s: %v; remove the root %s link by hand, %s", root, err, scratchDirName, scratchReconcileHint)}
	}
	if removed {
		outcome.changed = fmt.Sprintf("root scratch link retired at %s", root)
	}
	if _, err := os.Lstat(root); err == nil {
		return outcome.and(scratchOutcome{left: fmt.Sprintf(
			"root %s left as found at %s: not a scratch link of this pair; move its content into %s and remove the root %s, %s",
			scratchDirName, root, anchorLink, scratchDirName, scratchReconcileHint)})
	} else if !errors.Is(err, fs.ErrNotExist) {
		return outcome.and(scratchOutcome{left: fmt.Sprintf("root %s not inspected at %s: %v; clear what the error names, %s", scratchDirName, root, err, scratchReconcileHint)})
	}

	if err := unseedScratchExclude(rec, l, slug, "."); err != nil {
		return outcome.and(scratchOutcome{left: fmt.Sprintf("root scratch exclude line not removed: %v; clear what blocks writing the exclude file, %s", err, scratchReconcileHint)})
	}
	return outcome
}

// wireAnchorScratchLink makes the `.scratch` at the anchor of slug's warp worktree a directory link.
// The link points at the `scratch` directory under the weft-side `.lym`.
// In order it seeds the exclude, refuses to link over tracked paths, then classifies the entry.
// An absent entry is linked.
// A correct link has its target restored.
// A real directory with an absent target is moved into the target and linked.
// Every other shape is left as found.
// It removes nothing, and creates nothing when the pair's weft worktree is absent.
func wireAnchorScratchLink(rec *Mutations, l *lymcwd.Location, slug string) scratchOutcome {
	warp := WorktreePath(l, slug)
	link := scratchLinkPath(l, slug)
	target := scratchLinkTarget(l, slug)
	relLink := filepath.ToSlash(filepath.Join(l.AnchorRel, scratchDirName))

	excludePath, err := seedScratchLinkExclude(rec, warp, l.AnchorRel)
	if err != nil {
		// Resolving the exclude path can fail and leave it empty; the warp worktree then stands in as the location named.
		if excludePath == "" {
			excludePath = warp
		}
		return scratchOutcome{left: fmt.Sprintf("scratch exclude not written at %s: %v; clear what blocks writing the exclude file, %s", excludePath, err, scratchReconcileHint)}
	}

	tracked, err := scratchPathTracked(warp, relLink)
	if err != nil {
		return scratchFailure(link, target, link, err)
	}
	if tracked {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch left as found at %s: tracked by git; stop tracking %s on the warp branch, move the untracked content into the target %s and remove %s, %s",
			link, relLink, target, link, scratchReconcileHint)}
	}

	info, err := os.Lstat(link)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return linkAbsentScratch(rec, l, slug, link, target)
		}
		return scratchFailure(link, target, link, err)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil {
		return scratchFailure(link, target, link, err)
	}
	if isLink {
		return restoreScratchTarget(rec, l, slug, link, target)
	}
	if !info.IsDir() {
		return scratchLeave(link, target, "a regular file")
	}
	return adoptScratchDirectory(rec, l, slug, link, target)
}

// unlinkScratchLink removes slug's scratch links through the destructive gate and leaves their target untouched.
// It removes the link at the anchor and, at a subpath anchor, a root `.scratch` link an earlier binary wired.
// It skips an entry that is absent, is not a link, or is a link that does not resolve into the pair's weft worktree;
// git's own worktree removal unlinks such a link without following it.
// A gate refusal comes back naming the link and the manual removal that lets a rerun proceed.
func unlinkScratchLink(rec *Mutations, l *lymcwd.Location, slug string) error {
	links := []string{scratchLinkPath(l, slug)}
	if !anchorIsRoot(l) {
		links = append(links, filepath.Join(WorktreePath(l, slug), scratchDirName))
	}
	for _, link := range links {
		if _, err := removeOwnedScratchLink(rec, l, slug, link); err != nil {
			return fmt.Errorf("remove scratch link %s: %w; remove the %s link by hand and rerun `lym fabric remove`", link, err, link)
		}
	}
	return nil
}

// removeOwnedScratchLink removes link through the destructive gate when it is a link into slug's weft worktree.
// It reports whether it removed the link.
// It inspects the path itself, so it finds a link that a scan of the anchor's junctions misses.
func removeOwnedScratchLink(rec *Mutations, l *lymcwd.Location, slug, link string) (removed bool, err error) {
	if _, err := os.Lstat(link); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect scratch link: %w", err)
	}
	isLink, err := fslink.IsLink(link)
	if err != nil || !isLink {
		return false, err
	}
	owned, err := linkIsFabricOwned(l, slug, link)
	if err != nil || !owned {
		return false, err
	}

	req := pathRequest{
		what:      "remove scratch link",
		container: WorktreePath(l, slug),
		target:    link,
		slug:      nil,
		ownership: ownedDriftedWiredJunction([]string{link}),
		dirtiness: dirtinessNA("a link holds no content; the weft target it points at is untouched"),
		force:     false,
	}
	if err := removeLink(rec, req); err != nil {
		return false, err
	}
	return true, nil
}

// linkAbsentScratch creates the target and the link for an anchor `.scratch` that does not exist.
func linkAbsentScratch(rec *Mutations, l *lymcwd.Location, slug, link, target string) scratchOutcome {
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}
	if failedPath, err := mkdirScratchTarget(rec, target); err != nil {
		return scratchFailure(link, target, failedPath, err)
	}
	return createScratchLink(rec, link, target, scratchOutcome{changed: fmt.Sprintf("scratch linked: %s → %s", link, target)})
}

// restoreScratchTarget handles an anchor `.scratch` that is a link.
// A link to the expected target gets the target recreated when absent.
// A link to anything else is left.
func restoreScratchTarget(rec *Mutations, l *lymcwd.Location, slug, link, target string) scratchOutcome {
	raw, err := fslink.RawTarget(link)
	if err != nil {
		return scratchFailure(link, target, link, err)
	}
	if filepath.Clean(raw) != filepath.Clean(target) {
		return scratchLeave(link, target, "a link to another target")
	}
	exists, err := scratchTargetExists(target)
	if err != nil {
		return scratchFailure(link, target, target, err)
	}
	if exists {
		return scratchOutcome{}
	}
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}
	if failedPath, err := mkdirScratchTarget(rec, target); err != nil {
		return scratchFailure(link, target, failedPath, err)
	}
	return scratchOutcome{changed: fmt.Sprintf("scratch target restored at %s", target)}
}

// adoptScratchDirectory moves a real anchor `.scratch` into the absent target and links it.
// With the target present, it leaves the directory in place.
func adoptScratchDirectory(rec *Mutations, l *lymcwd.Location, slug, link, target string) scratchOutcome {
	exists, err := scratchTargetExists(target)
	if err != nil {
		return scratchFailure(link, target, target, err)
	}
	if exists {
		return scratchLeave(link, target, fmt.Sprintf("a real directory while the target %s already exists", target))
	}
	if !weftWorktreePresent(l, slug) {
		return scratchOutcome{}
	}

	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return scratchFailure(link, target, parent, err)
	}
	if err := os.Rename(link, target); err != nil {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch left as found at %s: a real directory whose move to %s failed: %v; stop reed (an open handle inside %s is the expected cause on Windows), move its content into the target %s and remove %s, %s",
			link, target, err, scratchDirName, target, link, scratchReconcileHint)}
	}
	rec.Append(KindPathRenamed, target, hubRelativeTarget(l.HubPath, link))

	if scratchBeforeLinkHook != nil {
		scratchBeforeLinkHook()
	}
	if err := fslink.CreateDirLink(link, target); err != nil {
		return scratchOutcome{left: fmt.Sprintf(
			"scratch link not created at %s after its content moved to %s: %v; move the recreated %s's content into %s, remove %s, %s",
			link, target, err, scratchDirName, target, link, scratchReconcileHint)}
	}
	rec.Append(KindLinkCreated, link, "")
	return scratchOutcome{changed: fmt.Sprintf("moved %s into %s and linked it", link, target)}
}

// createScratchLink runs the before-link hook and links link to target, returning success when the link was made.
func createScratchLink(rec *Mutations, link, target string, success scratchOutcome) scratchOutcome {
	if scratchBeforeLinkHook != nil {
		scratchBeforeLinkHook()
	}
	if err := fslink.CreateDirLink(link, target); err != nil {
		return scratchFailure(link, target, link, err)
	}
	rec.Append(KindLinkCreated, link, "")
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

// scratchTargetExists reports whether anything stands at target.
// A missing path and a parent that is not a directory both report false.
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
// The scratch steps create nothing without it.
// A weft torn down mid-repair would otherwise get a plain directory recreated at its `.lym/scratch` path.
func weftWorktreePresent(l *lymcwd.Location, slug string) bool {
	_, err := os.Stat(RecordsWorktreePath(l, slug))
	return !errors.Is(err, fs.ErrNotExist)
}

// scratchLeave reports an anchor `.scratch` left as found because of its shape.
func scratchLeave(link, target, shape string) scratchOutcome {
	return scratchOutcome{left: fmt.Sprintf(
		"scratch left as found at %s: %s; move its content into the target %s and remove %s, %s",
		link, shape, target, link, scratchReconcileHint)}
}

// scratchFailure reports a step that failed on failedPath with the anchor `.scratch` left as found.
func scratchFailure(link, target, failedPath string, err error) scratchOutcome {
	return scratchOutcome{left: fmt.Sprintf(
		"scratch link not created at %s: %s: %v; clear what the error names, moving any content under a real %s into the target %s before removing it, %s",
		link, failedPath, err, link, target, scratchReconcileHint)}
}
