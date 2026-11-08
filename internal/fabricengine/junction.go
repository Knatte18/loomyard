// junction.go wires the warp-to-weft directory junctions of a fabric worktree pair, atomically and idempotently.
// It is keyed by the worktree's slug and refuses a warp that holds a real directory predating its weft.
// Wiring ends by linking the anchor's `.scratch` into the weft-side `.lym`, in scratchlink.go.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// WorktreePath returns the path to a sibling worktree with the given slug, directly under the hub.
func WorktreePath(l *lymcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug)
}

// CodeLymLink returns the path to the _lym junction link in a named slug's warp worktree.
// It is the warp-side end of the junction that points into the paired weft worktree.
func CodeLymLink(l *lymcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, lymdirs.LymDirName)
}

// WarpLymLinkHere returns the path to the _lym junction link in the current warp worktree.
// It is the warp-side end of the junction that points into the paired weft worktree.
func WarpLymLinkHere(l *lymcwd.Location) string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, lymdirs.LymDirName)
}

// LymJunctionWiredHere reports whether the current warp worktree's _lym is a link, as wiring leaves it.
// A pair still being created has no link there yet.
// A reader that goes through the path before then reads nothing and must create nothing there.
func LymJunctionWiredHere(l *lymcwd.Location) (bool, error) {
	return fslink.IsLink(WarpLymLinkHere(l))
}

// WarpJunction represents a directory junction in the warp worktree that links to a weft directory.
type WarpJunction struct {
	Name   string // Name is the directory name (e.g., "_lym")
	Link   string // Link is the warp-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// WarpJunctions returns one record per name in names, in the order given.
// Each record holds the warp-side Link, under the worktree of slug, and the weft-side Target.
func WarpJunctions(l *lymcwd.Location, slug string, names []string) []WarpJunction {
	junctions := make([]WarpJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, WarpJunction{
			Name:   name,
			Link:   filepath.Join(WorktreePath(l, slug), l.AnchorRel, name),
			Target: filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, name),
		})
	}
	return junctions
}

// WarpJunctionsHere returns one record per name in names, in the order given, for the current worktree.
// Each record holds the warp-side Link, under the current worktree, and the weft-side Target.
// It serves health checks that run in the current worktree and have no slug.
func WarpJunctionsHere(l *lymcwd.Location, names []string) []WarpJunction {
	junctions := make([]WarpJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, WarpJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreePath(), l.AnchorRel, name),
			Target: filepath.Join(RecordsWorktree(l), l.AnchorRel, name),
		})
	}
	return junctions
}

// WireJunctionsWith creates the warp junctions for names of the worktree of slug and seeds their git-exclude entries.
// It records every junction it creates or re-points into rec.
// names is the filtered set of wired names; the caller supplies it.
// The scratch step runs after the junctions and never fails wiring; a leave or failure is logged as a warning.
// It is idempotent and returns an error if the warp holds a real directory that predates weft.
func WireJunctionsWith(rec *Mutations, l *lymcwd.Location, slug string, names []string) error {
	if err := wireJunctionSet(rec, l, slug, names); err != nil {
		return err
	}

	// The scratch step never fails wiring.
	// Its leave or failure report goes to the log, since the wiring verbs' results carry no detail field.
	if outcome := wireScratchLink(rec, l, slug); outcome.left != "" {
		logger.Warn("fabric: scratch link not wired", "slug", slug, "left", outcome.left)
	}
	return nil
}

// wireJunctionSet creates the warp junctions for names and seeds the weft-side exclude.
// It also appends the junction names to the warp git-exclude and seeds the warp exclude list.
// It loads the fabric config first, so a load failure fails the wiring before anything is written.
// It records every change into rec.
func wireJunctionSet(rec *Mutations, l *lymcwd.Location, slug string, names []string) error {
	cfg, err := LoadConfig(ConfigDirs(l))
	if err != nil {
		return fmt.Errorf("load fabric config for wiring %s: %w", slug, err)
	}

	if err := seedLymJunction(rec, l, slug, names); err != nil {
		return err
	}

	// Seed the weft-side .lym exclude here as well as from ensureWeftLockDir.
	// seedLymJunction has just materialised the weft-side target.
	// So the exclude entry exists before anything writes into .lym.
	// Without it, scratch shows as untracked dirt and trips Remove's no-force dirty gate.
	// ensureWeftLockDir still seeds it on the self-healing path for machines that never re-wire.
	// Resolve the path with RecordsWorktreePath(l, slug), not a junction's Target parent:
	// that parent is the worktree root only when AnchorRel is ".", and a subdirectory otherwise.
	if err := seedWeftArtifactExcludes(RecordsWorktreePath(l, slug)); err != nil {
		return err
	}

	if err := seedGitExclude(rec, l, slug, names); err != nil {
		return err
	}

	return seedWarpExcludeLines(rec, WorktreePath(l, slug), l.AnchorRel, cfg.WarpExcludePaths())
}

// WireJunctions wires the warp junctions for names in the worktree of slug and records no changes.
// It exists for callers, such as tests, that have no recorder to pass.
func WireJunctions(l *lymcwd.Location, slug string, names []string) error {
	return WireJunctionsWith(nil, l, slug, names)
}

// seedLymJunction creates, verifies, or re-points the warp junctions pointing to weft directories.
// It materializes each junction's weft-side target before any check.
// A correct link is left alone; a dangling or wrong link is re-pointed; a real directory is refused.
// rec is the calling verb's recorder, which the re-point and creation steps write to.
func seedLymJunction(rec *Mutations, l *lymcwd.Location, slug string, names []string) error {
	junctions := WarpJunctions(l, slug, names)
	container := WorktreePath(l, slug)
	links := make([]string, len(junctions))
	for i, j := range junctions {
		links[i] = j.Link
	}

	for _, j := range junctions {
		link := j.Link
		target := j.Target

		// Materialise the weft-side target before the checks below, which resolve it.
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("materialise weft target %s: %w", target, err)
		}

		_, err := os.Lstat(link)
		if err == nil {
			targetResolved, errTarget := filepath.EvalSymlinks(target)
			if errTarget != nil {
				return fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
			}

			isLink, errIsLink := fslink.IsLink(link)
			if errIsLink != nil {
				return fmt.Errorf("islink %s: %w", link, errIsLink)
			}
			if isLink {
				linkResolved, errResolve := fslink.PointsTo(link)
				if errResolve == nil && linkResolved == targetResolved {
					// Idempotent: junction exists and resolves correctly
					continue
				}

				// The path is a link that dangles or points elsewhere.
				// That is corrupted or externally modified wiring, not user content.
				// Re-point it at the canonical weft target, so a pair whose junction drifted can be repaired.
				// Reconcile relies on this.
				if removeErr := repointLink(rec, "re-point junction", container, link, ownedDriftedWiredJunction(links)); removeErr != nil {
					return fmt.Errorf("re-point junction %s: %w", link, removeErr)
				}
				if createErr := fslink.CreateDirLink(link, target); createErr != nil {
					return createErr
				}
				rec.Append(KindLinkCreated, link, "")
				continue
			}

			// A real directory that predates weft is refused for `_lym`: it may hold user content, which fabric never deletes.
			// Creating `_lym/` in the repo and writing to it is the natural operator mistake this guard exists to catch.
			// `.lym` is the one exception.
			// Its content is lym's own machine-local scratch, written unconditionally by several of lym's subsystems.
			// The user-content concern does not apply there.
			// Every existing worktree holds a real `.lym`.
			// Without adoption, the first `reconcile` after `.lym` joined the wired name set would hard-error everywhere.
			if j.Name == lymdirs.DotLymDirName {
				if err := adoptDotLymContent(rec, link, target); err != nil {
					return err
				}
				continue
			}

			return fmt.Errorf(
				"warp repo already contains a real %s at %s; it predates weft — move its content into the paired weft worktree's own %s, or remove this directory, then re-run `lym fabric reconcile` to create the junction",
				filepath.Base(link),
				link,
				filepath.Base(link),
			)
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("lstat %s: %w", link, err)
		}

		if err := fslink.CreateDirLink(link, target); err != nil {
			return err
		}
		rec.Append(KindLinkCreated, link, "")
	}

	return nil
}

// adoptDotLymContent merges every entry of the warp-side real directory at link into target.
// It then removes the now-empty warp directory and creates the junction in its place.
// Only `.lym` is adopted this way.
// Several of lym's own subsystems write it unconditionally, so a worktree may already hold a real `.lym`;
// without adoption, `reconcile` would hard-error on it.
//
// A directory present on both sides is merged recursively rather than refused.
// Refusing it is not an option: adoption's own steady state produces this collision.
// `lym fabric unwire` removes the `.lym` junction.
// A later `lym` run that logs at Info or above, or exits non-zero, opens `internal/logger`'s durable sink.
// It is ungated outside `go test` and runs `os.MkdirAll(<anchor>/.lym/logs)`.
// That recreates a real warp-side `.lym/logs` on top of the `logs` an earlier adoption moved into weft.
// Refusing that collision would leave `lym fabric reconcile`, the documented remedy, unable to heal the pair:
// it would refuse every time, and `junction_healthy:false` would stay forever.
// The seeding aborts before the exclude step re-runs, so the warp worktree is left untracked-dirty.
// That trips the destruction gate's own dirtiness checks on a pair the operator never dirtied.
//
// A collision that is not a directory on both sides is still refused.
// That asymmetry is the safety argument: merging two directories composes their contents and destroys nothing.
// Resolving a file-vs-file or file-vs-directory collision would require choosing a winner.
// Fabric never chooses a winner on the operator's behalf.
// The refusal check walks the whole tree before anything moves, so a refusal leaves both sides as it found them.
//
// A rename failure is wrapped in an error that names the entry.
// The operator is told to stop reed and re-run `lym fabric reconcile`.
// On Windows, moving a directory with an open handle inside it fails, which is the expected cause.
// The error names whatever was already moved, so a partial move is never reported as success.
//
// Idempotent: a second call finds a link, not a real directory.
// So this helper is never reached again for an already-adopted `.lym`.
// rec is the calling verb's own recorder.
// It records KindFileWritten at target with Detail "adopted" once the merge completes, before the link is created.
// The moved tree is a hub-visible addition.
// One entry at the destination root covers the whole tree under the coverage rule.
// It then records KindLinkCreated for the link created in the now-empty source's place.
func adoptDotLymContent(rec *Mutations, link, target string) error {
	if err := refuseUnmergeableAdoption(link, target, link, target); err != nil {
		return err
	}

	if err := mergeAdoptionTree(link, target, link, target); err != nil {
		return err
	}

	rec.Append(KindFileWritten, target, "adopted")

	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove now-empty %s after adoption: %w", link, err)
	}

	if err := fslink.CreateDirLink(link, target); err != nil {
		return err
	}
	rec.Append(KindLinkCreated, link, "")
	return nil
}

// refuseUnmergeableAdoption walks src against dst and returns an error naming the first entry that
// collides in a way adoption cannot resolve: present on both sides and not a real directory on both.
//
// It runs to completion before any entry is moved, so a refusal is non-destructive.
// A collision three levels down must not surface after two levels were already renamed away.
// rootLink and rootTarget are the top-level adoption pair.
// They are passed down so a nested refusal names the `.lym` directories, not an interior path.
// os.Lstat, not os.Stat, decides "is a directory" on both sides: a symlink pointing at a directory
// is a link, not a directory to merge into, and renaming one is the safe answer.
func refuseUnmergeableAdoption(src, dst, rootLink, rootTarget string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s for adoption: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		dstInfo, dstErr := os.Lstat(dstPath)
		if os.IsNotExist(dstErr) {
			continue
		}
		if dstErr != nil {
			return fmt.Errorf("lstat %s: %w", dstPath, dstErr)
		}

		srcInfo, srcErr := os.Lstat(srcPath)
		if srcErr != nil {
			return fmt.Errorf("lstat %s: %w", srcPath, srcErr)
		}

		if srcInfo.IsDir() && dstInfo.IsDir() {
			if err := refuseUnmergeableAdoption(srcPath, dstPath, rootLink, rootTarget); err != nil {
				return err
			}
			continue
		}

		return fmt.Errorf(
			"adopt %s into %s: %s already exists at the weft target and is not a directory on both sides, so adoption cannot merge it — move or delete the warp-side copy at %s and re-run `lym fabric reconcile`",
			rootLink, rootTarget, dstPath, srcPath,
		)
	}

	return nil
}

// mergeAdoptionTree moves every entry of src into dst, recursing into a directory present on both
// sides and removing each source subdirectory once its own contents have moved.
//
// It is only called once the whole tree has been checked for unmergeable collisions.
// A collision reaching this function is therefore a directory on both sides by construction;
// anything else would be a broken invariant, not an input to handle.
// rootLink and rootTarget are the top-level adoption pair, used only in error messages.
func mergeAdoptionTree(src, dst, rootLink, rootTarget string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s for adoption: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if _, dstErr := os.Lstat(dstPath); os.IsNotExist(dstErr) {
			if renameErr := os.Rename(srcPath, dstPath); renameErr != nil {
				return fmt.Errorf(
					"adopt %s into %s: move %s failed: %w — stop reed (an open handle inside %s is the expected cause on Windows) and re-run `lym fabric reconcile`",
					rootLink, rootTarget, srcPath, renameErr, rootLink,
				)
			}
			continue
		}

		if err := mergeAdoptionTree(srcPath, dstPath, rootLink, rootTarget); err != nil {
			return err
		}
		// os.Remove, never RemoveAll: it is refused by the OS the moment the directory still holds
		// anything, so it can only ever delete a directory the recursion above has just emptied.
		if err := os.Remove(srcPath); err != nil {
			return fmt.Errorf("remove now-empty %s after adoption: %w", srcPath, err)
		}
	}

	return nil
}

// UnwireResult reports what an unwire actually changed.
// It tells a real reversal apart from a no-op on an already-clean or never-wired worktree.
// It embeds MutationRecord, which carries the mutations recorded over the call.
type UnwireResult struct {
	MutationRecord
	// JunctionsRemoved lists the Name of each junction that was actually removed, in visiting order.
	// It is a name list, not a count or a bool: which junctions were removed is CLI-observable.
	// A count such as "1 of 2 removed" does not say which junction is still wired.
	// A name list does.
	JunctionsRemoved []string
	// ExcludeChanged reports whether a junction-name line was removed from
	// .git/info/exclude.
	ExcludeChanged bool
}

// UnwireJunctions reverses the wiring of the current worktree for the given names, keyed by slug.
// It removes every warp junction those names define, and their shared .git/info/exclude entries.
// It undoes only that wiring: the worktree pairing and weft content are untouched.
// The anchor's scratch link and its exclude line stay.
// It loads no config itself.
//
// The junctions are unwired before the exclude entries, in the reverse of their creation order.
// Any junction inconsistency is a hard error: if junction removal reports one, the exclude file is never touched.
// An unexpected junction state (a real directory, or a link to an unexpected target) aborts the whole operation,
// so a corrupted or externally modified junction is never silently worked around.
//
// It returns an empty UnwireResult and a nil error when no junction was wired.
// That is the no-op case, not an error.
// It returns an error if junction removal aborts partway through.
// It also returns an error if the exclude-file update fails after the junctions were removed.
// JunctionsRemoved then holds whatever was removed before the failure.
// A zero UnwireResult on a mid-loop failure would misreport a partial removal as untouched:
// with two or more junctions, the first may already be gone before the second fails.
func UnwireJunctions(l *lymcwd.Location, slug string, names []string) (res UnwireResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	removed, err := unseedLymJunction(rec, l, slug, names)
	if err != nil {
		return UnwireResult{JunctionsRemoved: removed}, err
	}

	changed, err := unseedGitExclude(rec, l, slug, names, false)
	if err != nil {
		return UnwireResult{JunctionsRemoved: removed}, err
	}

	return UnwireResult{JunctionsRemoved: removed, ExcludeChanged: changed}, nil
}

// unseedLymJunction removes every warp junction of the worktree for the given names.
// It wraps unseedJunctionRecords, which owns the per-junction loop.
// The split exists so that loop's abort-and-accumulate contract can be tested against a synthetic junction slice.
// WarpJunctions always returns exactly one entry, so it cannot produce the multi-junction case itself.
//
// Returns (nil, nil) if no junction exists: none were ever wired, or all were already unwired.
// That no-op is not an error.
// Otherwise it returns the first junction error, with the names removed before it.
// rec is the calling verb's own recorder, passed through to the loop.
func unseedLymJunction(rec *Mutations, l *lymcwd.Location, slug string, names []string) (removed []string, err error) {
	return unseedJunctionRecords(rec, WorktreePath(l, slug), WarpJunctions(l, slug, names))
}

// unseedJunctionRecords removes each junction in junctions, in order.
// It checks each junction in the order seeding does: the target is resolved before the link-type check.
// Seeding and unseeding stay in lockstep as the junction model evolves.
//
// It aborts on the first junction error instead of continuing best-effort.
// A junction inconsistency is a hard error the operator must see.
// The exclude-file update runs only when this function succeeds.
// Continuing past a corrupted junction would silently work around the state this guard exists to surface.
//
// Returns (nil, nil) if junctions is empty or none of its entries exist on disk: the no-op case, not an error.
// Returns (removed, err) when a junction is in an unexpected state:
// its weft-side target is missing or unreachable, its warp path is a real directory rather than a junction,
// or it resolves to an unexpected target.
// removed holds the Name of every junction removed before the failing one.
// Each of these states means corruption or external modification, not a normal unwire.
// container is the boundary every junction must resolve strictly below.
// The removal gate checks containment against it, so the caller must pass it in.
// The caller passes the worktree path.
// rec is the calling verb's own recorder, passed to removeLink for each junction.
func unseedJunctionRecords(rec *Mutations, container string, junctions []WarpJunction) (removed []string, err error) {
	links := make([]string, len(junctions))
	for i, j := range junctions {
		links[i] = j.Link
	}

	for _, j := range junctions {
		link := j.Link
		target := j.Target

		if _, err := os.Lstat(link); err != nil {
			if os.IsNotExist(err) {
				// This junction was never wired, or was already unwired.
				// That is not an error: move on to the next junction.
				continue
			}
			return removed, fmt.Errorf("lstat %s: %w", link, err)
		}

		// The link exists. Resolve the canonical weft-side target first.
		// A missing or unreachable target is then reported distinctly from a wrong-target junction.
		targetResolved, errTarget := filepath.EvalSymlinks(target)
		if errTarget != nil {
			return removed, fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
		}

		isLink, err := fslink.IsLink(link)
		if err != nil {
			return removed, fmt.Errorf("islink %s: %w", link, err)
		}
		if !isLink {
			// A real directory, or anything else that is not a junction, is refused.
			// Touching it could delete user content.
			return removed, fmt.Errorf(
				"warp repo already contains a real %s at %s; it is not a junction — refusing to remove it",
				filepath.Base(link),
				link,
			)
		}

		linkResolved, err := fslink.PointsTo(link)
		if err != nil {
			return removed, fmt.Errorf("resolve link target %s: %w", link, err)
		}
		if linkResolved != targetResolved {
			// The junction points somewhere other than the expected weft directory.
			// That is corruption or external modification, not a normal unwire target.
			return removed, fmt.Errorf(
				"warp junction %s points to unexpected target %s (want %s); refusing to remove it",
				link, linkResolved, targetResolved,
			)
		}

		req := pathRequest{
			what:      "remove warp junction",
			container: container,
			target:    link,
			slug:      nil,
			ownership: ownedWiredJunction(links, targetResolved),
			dirtiness: dirtinessNA("a junction holds no content; the weft target it points at is untouched"),
			force:     false,
		}
		if err := removeLink(rec, req); err != nil {
			return removed, fmt.Errorf("remove warp junction %s: %w", link, err)
		}
		removed = append(removed, j.Name)
	}

	return removed, nil
}

// unseedGitExclude removes the junction-name lines from the warp worktree's .git/info/exclude file.
//
// It resolves the exclude path with `git rev-parse --git-path info/exclude`.
// The worktree path is joined to it when the resolved path is relative.
// For each junction, it removes each line that trims to its anchored exclude pattern or to its legacy bare name.
// The match is line-exact, the same comparison used to detect presence.
// The remaining lines keep their original order.
//
// A line that another warp worktree in the same hub still needs is kept.
// That holds even when this worktree no longer wires the name:
// the anchored line while another worktree has the name linked at the same anchored path,
// and the bare line while another worktree has the name linked at any anchor.
// git resolves info/exclude to the repo's common gitdir, so the repo has one exclude file, not one per worktree.
// Removing a line a sibling still needs would show its live junctions as untracked dirt in git status.
// That would trip the no-force dirty gate on worktrees the caller never touched.
//
// Returns (false, nil) without touching the file if the exclude file does not
// exist, or if no matching line was found; both are no-ops, not errors.
// With dryRun set, the exclude file is not written and nothing is appended to rec.
// The stripped content is still computed under the exclude lock.
// The changed result reports whether it differs from the original.
// Taking that lock still creates the info directory and the lock file when they are absent,
// and it waits behind a concurrent exclude writer.
// rec is the calling verb's own recorder.
// It records KindFileWritten at the exclude file's resolved path only when the rewrite changes it.
func unseedGitExclude(rec *Mutations, l *lymcwd.Location, slug string, names []string, dryRun bool) (changed bool, err error) {
	wouldChange := false
	excludePath, changed, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
		// The sibling census runs inside the lock.
		// A concurrent wire in another worktree cannot invalidate its answer before the write it feeds.
		wired := namesWiredInSiblingWorktrees(l, slug, names)
		stripSet := excludeLinesToStrip(l.AnchorRel, names, wired)
		if len(stripSet) == 0 {
			return content, nil
		}

		stripped := stripExcludeLines(content, stripSet)
		if dryRun {
			wouldChange = stripped != content
			return content, nil
		}
		return stripped, nil
	})
	if err != nil {
		return false, err
	}
	if dryRun {
		return wouldChange, nil
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return changed, nil
}

// excludeLinesToStrip returns the exclude lines an unwire of names at anchorRel may remove, given the sibling census.
// Both spellings of a name are stripped when nothing still needs them.
// One is the anchored pattern this binary writes; the other is the legacy bare name an earlier binary wrote.
// Stripping both lets a hub wired by an earlier binary revert cleanly.
// Only worktrees linked at the same anchored path share the anchored line.
// A bare line matches at every depth, so every worktree wiring the name shares it.
func excludeLinesToStrip(anchorRel string, names []string, wired siblingWiring) map[string]bool {
	stripSet := make(map[string]bool)
	for _, name := range names {
		if !wired.sameAnchor[name] {
			stripSet[excludePatternFor(anchorRel, name)] = true
		}
		if !wired.anyAnchor[name] {
			stripSet[name] = true
		}
	}
	return stripSet
}

// stripExcludeLines returns content without the lines that trim to a key of stripSet.
// The remaining lines keep their original order and spacing.
func stripExcludeLines(content string, stripSet map[string]bool) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if stripSet[strings.TrimSpace(line)] {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// siblingWiring is the census of the names the other warp worktrees of a repo still have linked.
type siblingWiring struct {
	// sameAnchor holds the names that another worktree has linked at the same anchored path as this one.
	sameAnchor map[string]bool
	// anyAnchor holds the names some other worktree has linked at its own anchored directory, whichever anchor that is.
	anyAnchor map[string]bool
}

// namesWiredInSiblingWorktrees takes the census of names over the other warp worktrees registered in this repo.
// Each sibling is checked at its own anchored directory.
// It is what keeps one worktree's unwire from dirtying every sibling:
// the exclude file it would edit is repo-wide.
// An anchored entry is only genuinely dead once no worktree links the same anchored path.
// A bare entry, which matches at every depth, is dead once no worktree wires the name at any anchor.
//
// A failure to enumerate worktrees, or a sibling whose anchor cannot be read, reports every name as still wired.
// Keeping a dead exclude line costs a stale line in a file only lym writes.
// Removing a live one costs untracked-junction noise in a working tree the caller never asked to touch.
func namesWiredInSiblingWorktrees(l *lymcwd.Location, slug string, names []string) siblingWiring {
	wired := siblingWiring{sameAnchor: make(map[string]bool, len(names)), anyAnchor: make(map[string]bool, len(names))}
	markAllWired := func() {
		for _, name := range names {
			wired.sameAnchor[name] = true
			wired.anyAnchor[name] = true
		}
	}

	entries, err := List(WorktreePath(l, slug))
	if err != nil {
		entries, err = List(l.WorktreePath())
	}
	if err != nil {
		markAllWired()
		return wired
	}

	ownPath := filepath.Clean(WorktreePath(l, slug))
	for _, entry := range entries {
		entryPath := filepath.Clean(filepath.FromSlash(entry.Path))
		if entryPath == ownPath {
			continue
		}
		sibling, err := lymcwd.NamedWorktree(l.HubPath, filepath.Base(entryPath))
		if err != nil {
			markAllWired()
			return wired
		}
		sameAnchor := filepath.Clean(sibling.AnchorRel) == filepath.Clean(l.AnchorRel)
		for _, name := range names {
			isLink, linkErr := fslink.IsLink(filepath.Join(entryPath, sibling.AnchorRel, name))
			if linkErr != nil || !isLink {
				continue
			}
			wired.anyAnchor[name] = true
			if sameAnchor {
				wired.sameAnchor[name] = true
			}
		}
	}
	return wired
}

// excludePatternFor returns the gitignore pattern that excludes the junction named name at the anchored directory only.
//
// A bare name matches at any depth in gitignore.
// On a subpath-anchored monorepo, that untracks every same-named directory elsewhere in the repo.
// One such directory, a `frontend/_lym/` the operator tracks, would be untracked.
// Fabric wires one junction at one known path, so the pattern names only that path.
// The leading slash anchors the pattern to the repo root, where .git/info/exclude is evaluated.
// The pattern has no trailing slash:
// a directory junction is a symlink, and git matches a symlink as a file,
// so a directory-only pattern would not exclude it.
func excludePatternFor(anchorRel, name string) string {
	return "/" + filepath.ToSlash(filepath.Join(anchorRel, name))
}

// seedGitExclude adds junction names to the warp worktree's .git/info/exclude file if not already present.
//
// It appends the anchored exclude pattern of each warp junction that the file does not already hold.
// A bare-name line for a seeded junction is replaced by the anchored pattern, not kept beside it.
// An older binary may have written such a line,
// so an existing hub converges on the narrower exclusion the first time it is re-wired.
// Idempotent: re-running when all junction patterns are already present is a no-op.
// rec is the calling verb's recorder.
// It records KindFileWritten at the resolved exclude path via Append, only when the rewrite reports changed.
func seedGitExclude(rec *Mutations, l *lymcwd.Location, slug string, names []string) error {
	excludePath, changed, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
		// Drop any legacy bare-name line for a junction being seeded,
		// so the anchored pattern replaces it rather than accumulating beside it.
		legacy := make(map[string]bool, len(names))
		for _, j := range WarpJunctions(l, slug, names) {
			legacy[j.Name] = true
		}
		kept := make([]string, 0)
		dropped := false
		for _, line := range strings.Split(content, "\n") {
			if legacy[strings.TrimSpace(line)] {
				dropped = true
				continue
			}
			kept = append(kept, line)
		}
		if dropped {
			content = strings.Join(kept, "\n")
		}

		for _, j := range WarpJunctions(l, slug, names) {
			pattern := excludePatternFor(l.AnchorRel, j.Name)

			found := false
			for _, line := range strings.Split(content, "\n") {
				if strings.TrimSpace(line) == pattern {
					found = true
					break
				}
			}
			if found {
				continue
			}

			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += pattern + "\n"
		}

		return content, nil
	})
	if err != nil {
		return err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return nil
}
