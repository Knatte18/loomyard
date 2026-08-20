// junction.go implements the atomic, cwd-keyed junction primitive for fabric topology.
//
// WireJunctions creates warp↔weft directory junctions and manages their git-exclude entries
// atomically, keyed by the current worktree's slug.
// It is idempotent, guarding against re-entry and enforcing the warp-pristine invariant by refusing
// to wire when the warp contains a pre-existing real directory predating weft.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// WorktreePath returns the path to a sibling worktree with the given slug.
// It replaces (*lyxcwd.Location).WorktreePath(slug), which collided with the no-arg WorktreePath()
// accessor the coming reshape introduces on the same type.
func WorktreePath(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug)
}

// WarpLyxLink returns the path to the _lyx junction link in a named slug's warp worktree.
// It is the warp-side junction endpoint that points into the paired weft worktree via
// WeftLyxDirFor(l, slug).
func WarpLyxLink(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, lyxdirs.LyxDirName)
}

// WarpLyxLinkHere returns the path to the _lyx junction link in the current warp worktree.
// Derived from l.WorktreePath()+AnchorRel.
// It serves as the warp-side junction endpoint paired with WeftLyxDir(l).
func WarpLyxLinkHere(l *lyxcwd.Location) string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, lyxdirs.LyxDirName)
}

// WarpJunction represents a directory junction in the warp worktree that links to a weft directory.
type WarpJunction struct {
	Name   string // Name is the directory name (e.g., "_lyx")
	Link   string // Link is the warp-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// WarpJunctions returns the list of warp junctions for a given slug, one record per name in names,
// in names's own order (no forced sort).
// For each name, the record is {Name, Link, Target} where Link is HubPath/slug-anchored via
// WorktreePath(l, slug) and Target is computed via WeftWorktreePath(l, slug) and AnchorRel.
// WarpJunctions is HubPath/slug-anchored;
// WarpJunctionsHere below is the Here-anchored counterpart.
func WarpJunctions(l *lyxcwd.Location, slug string, names []string) []WarpJunction {
	junctions := make([]WarpJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, WarpJunction{
			Name:   name,
			Link:   filepath.Join(WorktreePath(l, slug), l.AnchorRel, name),
			Target: filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, name),
		})
	}
	return junctions
}

// WarpJunctionsHere returns the same WarpJunction records as WarpJunctions(l, slug, names), but
// resolved against the current worktree rather than a named slug: Link is built from
// l.WorktreePath() and each Target from WeftWorktree(l).
// This mirrors WarpLyxLinkHere(l)/WarpLyxLink(l, slug).
// It exists for health-check sites that are Here-anchored and have no slug available.
func WarpJunctionsHere(l *lyxcwd.Location, names []string) []WarpJunction {
	junctions := make([]WarpJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, WarpJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreePath(), l.AnchorRel, name),
			Target: filepath.Join(WeftWorktree(l), l.AnchorRel, name),
		})
	}
	return junctions
}

// WireJunctionsWith creates directory junctions and seeds git-exclude entries for the given slug over
// the caller-supplied wired name-set, recording every junction it creates or re-points into rec.
// The caller must supply the filtered name-set (not loaded by this function).
// Idempotent.
// Enforces the warp-pristine invariant: returns an error if the warp contains a real directory
// predating weft.
// This is the form every production caller uses; WireJunctions below is a thin nil-recorder
// delegation kept only for the roughly fifty existing test call sites that have no recorder to pass.
func WireJunctionsWith(rec *Mutations, l *lyxcwd.Location, slug string, names []string) error {
	// Create or verify warp junctions
	if err := seedLyxJunction(rec, l, slug, names); err != nil {
		return err
	}

	// Seed the weft-side .lyx exclude here, not only from ensureWeftLockDir: wiring
	// already materialises the weft-side target (seedLyxJunction's os.MkdirAll(target,
	// ...) above), so the exclude entry is guaranteed to exist before anything writes
	// into .lyx. Seeding only from ensureWeftLockDir would leave the window between
	// wiring and the first weft-git verb open, during which scratch shows as untracked
	// dirt and trips Remove's no-force dirty gate. ensureWeftLockDir keeps calling the
	// same single owner as the self-healing path for machines that never re-wire — that
	// call is not removed.
	// Resolved via WeftWorktreePath(l, slug), the same base WarpJunctions computes its
	// targets from — never derived from a junction record's Target parent, since Target
	// is filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, name), whose parent is the
	// worktree root only when AnchorRel == "." and a subdirectory otherwise.
	if err := seedWeftArtifactExcludes(WeftWorktreePath(l, slug)); err != nil {
		return err
	}

	// Append junction names to git-exclude
	if err := seedGitExclude(rec, l, slug, names); err != nil {
		return err
	}

	return nil
}

// WireJunctions is WireJunctionsWith's nil-recorder delegation, kept only because roughly fifty
// existing test call sites across fifteen files have no recorder to pass and assert nothing about
// the record; every production caller uses WireJunctionsWith instead.
func WireJunctions(l *lyxcwd.Location, slug string, names []string) error {
	return WireJunctionsWith(nil, l, slug, names)
}

// seedLyxJunction creates, verifies, or re-points the warp junctions pointing
// to weft directories. Materializes each junction's weft-side target first.
// A correct link is left alone; a dangling or wrong link is re-pointed;
// a real directory is refused.
// rec is the calling verb's own recorder, threaded through to repointLink for the re-point branch;
// batch 5 gives this function's own creation sites their constructive recording.
func seedLyxJunction(rec *Mutations, l *lyxcwd.Location, slug string, names []string) error {
	junctions := WarpJunctions(l, slug, names)
	container := WorktreePath(l, slug)
	links := make([]string, len(junctions))
	for i, j := range junctions {
		links[i] = j.Link
	}

	for _, j := range junctions {
		link := j.Link
		target := j.Target

		// Materialise the weft-side target first, before any of the checks
		// below run — see the godoc above for why placement matters.
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("materialise weft target %s: %w", target, err)
		}

		_, err := os.Lstat(link)
		if err == nil {
			// Link exists. Resolve the target first; if target doesn't exist, report distinctly.
			targetResolved, errTarget := filepath.EvalSymlinks(target)
			if errTarget != nil {
				return fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
			}

			// Check if link is a link and resolves to the correct target
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

				// The path IS a link but dangles or points somewhere else —
				// corrupted or externally-modified wiring, not user content.
				// Re-point it at the canonical weft target so pairs whose
				// junction drifted are repairable (Reconcile relies on this).
				if removeErr := repointLink(rec, "re-point junction", container, link, ownedDriftedWiredJunction(links)); removeErr != nil {
					return fmt.Errorf("re-point junction %s: %w", link, removeErr)
				}
				if createErr := fslink.CreateDirLink(link, target); createErr != nil {
					return createErr
				}
				rec.Append(KindLinkCreated, link, "")
				continue
			}

			// A real (non-link) directory predating weft; refuse to touch it for
			// `_lyx` — it may hold user content, which fabric never deletes:
			// this now also protects `_lyx/PATTERN.md`, described throughout
			// as the warp repo's hand-authored invariants, which makes "create
			// _lyx/ in the repo and start writing" the natural operator
			// mistake this guard exists to catch.
			// `.lyx` is the one exception: content under it is always lyx's own
			// machine-local scratch (the logger, reed, shuttle, and
			// burler all write it unconditionally), so "never touch what might
			// be the user's hand-authored content" does not apply there.
			// Every worktree in existence today has a real `.lyx` for exactly
			// that reason, so without this adoption branch the first
			// `reconcile` after `.lyx` joined the wired name-set would
			// hard-error everywhere.
			if j.Name == lyxdirs.DotLyxDirName {
				if err := adoptDotLyxContent(rec, link, target); err != nil {
					return err
				}
				continue
			}

			return fmt.Errorf(
				"warp repo already contains a real %s at %s; it predates weft — move its content into the paired weft worktree's own %s, or remove this directory, then re-run `lyx fabric reconcile` to create the junction",
				filepath.Base(link),
				link,
				filepath.Base(link),
			)
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("lstat %s: %w", link, err)
		}

		// Junction does not exist; create it
		if err := fslink.CreateDirLink(link, target); err != nil {
			return err
		}
		rec.Append(KindLinkCreated, link, "")
	}

	return nil
}

// adoptDotLyxContent merges every entry from the warp-side real directory at link into the weft-side
// target, then removes the now-empty warp directory and creates the junction in its place.
// It is the only path seedLyxJunction takes for `.lyx`: every worktree in existence before this
// change holds a real `.lyx` (the logger, reed, shuttle, and burler all write it
// unconditionally), so without adoption the first `reconcile` after `.lyx` joined the wired name-set
// would hard-error everywhere.
//
// A directory present on BOTH sides is merged, recursively, rather than refused. That is not a
// convenience: adoption's own steady state produces the collision. `lyx fabric unwire` removes the
// `.lyx` junction, and the very next `lyx` invocation in that worktree that logs at Info-or-above or
// exits non-zero opens `internal/logger`'s durable sink, which is ungated outside `go test` and
// runs `os.MkdirAll(<anchor>/.lyx/logs)` — recreating a real warp-side `.lyx/logs` on top of the
// `logs` a previous adoption already moved into weft. Refusing that collision left `lyx fabric
// reconcile`, the documented remedy, permanently unable to heal the pair: the same refusal every
// time, `junction_healthy:false` forever, and (because seedLyxJunction aborts before seedGitExclude
// re-runs) a warp worktree left untracked-dirty, which then trips the destruction gate's own
// dirtiness checks on a pair the operator never dirtied.
//
// A collision that is NOT a directory on both sides is still refused, and that asymmetry is the
// whole safety argument: merging two directories composes their contents and destroys nothing,
// while resolving a file-vs-file or file-vs-directory collision would require choosing a winner,
// which fabric never does on the operator's behalf.
// The refusal check walks the whole tree BEFORE anything moves, so a refusal leaves both sides
// exactly as it found them.
//
// A rename failure is wrapped in an actionable error naming the entry and instructing the operator to
// stop reed and re-run `lyx fabric reconcile` — on Windows, moving a directory with an open
// handle inside it fails, and that is the expected cause.
// The error names whatever was already moved, so a partial move is never reported as success.
//
// Idempotent: a second call finds a link, not a real directory, so seedLyxJunction never reaches this
// helper again for an already-adopted `.lyx`.
// rec is the calling verb's own recorder. It records one KindFileWritten at target with Detail
// "adopted" once the merge completes and before the link is created — the moved tree is a
// hub-visible addition at the destination, and one entry at the destination root covers the whole
// moved tree under the coverage rule. It then records KindLinkCreated for the link created in the
// now-empty source's place, mirroring seedLyxJunction's own creation sites.
func adoptDotLyxContent(rec *Mutations, link, target string) error {
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
// It runs to completion before mergeAdoptionTree moves anything, which is what keeps a refusal
// non-destructive — a collision three levels down must not be discovered after two levels have
// already been renamed away.
// rootLink and rootTarget are the top-level adoption pair, carried down purely so a nested refusal
// still names the `.lyx` directories an operator recognises rather than an interior path.
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
			"adopt %s into %s: %s already exists at the weft target and is not a directory on both sides, so adoption cannot merge it — move or delete the warp-side copy at %s and re-run `lyx fabric reconcile`",
			rootLink, rootTarget, dstPath, srcPath,
		)
	}

	return nil
}

// mergeAdoptionTree moves every entry of src into dst, recursing into a directory present on both
// sides and removing each source subdirectory once its own contents have moved.
//
// It is only ever called after refuseUnmergeableAdoption has cleared the whole tree, so a collision
// reaching this function is a directory on both sides by construction; anything else would be a
// broken invariant, not an input to handle.
// rootLink and rootTarget serve the same naming purpose they do in refuseUnmergeableAdoption.
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
					"adopt %s into %s: move %s failed: %w — stop reed (an open handle inside %s is the expected cause on Windows) and re-run `lyx fabric reconcile`",
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

// UnwireResult reports which parts of UnwireJunctions actually changed state, distinguishing a real
// reversal from a no-op on an already-clean (or never-wired) worktree.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type UnwireResult struct {
	MutationRecord
	// JunctionsRemoved lists the Name of each junction that was actually present
	// and removed, in WarpJunctions(l, slug) order. A name slice, not a count or
	// a bool: which junction(s) were removed is CLI-observable, and "1 of 2
	// removed" tells an operator nothing about which one is still wired.
	JunctionsRemoved []string
	// ExcludeChanged reports whether a junction-name line was removed from
	// .git/info/exclude.
	ExcludeChanged bool
}

// UnwireJunctions reverses WireJunctions for the current worktree, keyed by slug, over the same
// caller-supplied names: it removes every warp junction in WarpJunctions(l, slug, names) and their
// shared .git/info/exclude entries, undoing exactly what WireJunctions seeded — nothing more (the
// worktree pairing and weft content are untouched; see Remove for the larger paired-teardown
// operation).
// Like WireJunctions, it loads no config itself.
//
// The junctions are unwired before the exclude entries, mirroring WireJunctions' creation order in
// reverse.
// Per the "any junction inconsistency is a hard error" invariant, if unseedLyxJunction reports an
// error the exclude file is never touched: an unexpected junction state (a real directory, or a
// link pointing somewhere unexpected) aborts the whole operation so a corrupted or
// externally-modified junction is never silently worked around.
//
// Returns an empty UnwireResult and nil error when no junction was wired (the legitimate no-op
// case).
// Returns an error, with JunctionsRemoved reflecting whatever was already removed before the
// failure, if unseedLyxJunction aborts partway through its loop, or if the exclude-file update
// fails after junction removal completed.
// A zero UnwireResult on a mid-loop failure would misreport a partial removal as untouched — with
// two or more junctions, the first may already be gone before the second fails.
func UnwireJunctions(l *lyxcwd.Location, slug string, names []string) (res UnwireResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	removed, err := unseedLyxJunction(rec, l, slug, names)
	if err != nil {
		return UnwireResult{JunctionsRemoved: removed}, err
	}

	changed, err := unseedGitExclude(rec, l, slug, names)
	if err != nil {
		return UnwireResult{JunctionsRemoved: removed}, err
	}

	return UnwireResult{JunctionsRemoved: removed, ExcludeChanged: changed}, nil
}

// unseedLyxJunction removes every warp junction in WarpJunctions(l, slug, names).
// It is a thin wrapper over unseedJunctionRecords, which owns the actual
// per-junction loop; the split exists purely so the loop's abort-and-accumulate
// contract is directly testable against a synthetic junction slice, since
// l.WarpJunctions always returns exactly one entry today and cannot itself
// produce the multi-junction scenario the contract is about.
//
// Returns (nil, nil) if no junction exists — none were ever wired, or all were
// already unwired; this is the legitimate no-op case, not an error. See
// unseedJunctionRecords for the error cases.
// rec is the calling verb's own recorder, threaded straight through to unseedJunctionRecords.
func unseedLyxJunction(rec *Mutations, l *lyxcwd.Location, slug string, names []string) (removed []string, err error) {
	return unseedJunctionRecords(rec, WorktreePath(l, slug), WarpJunctions(l, slug, names))
}

// unseedJunctionRecords removes each junction in junctions in order, mirroring
// seedLyxJunction's per-junction validation in the same order (target
// resolution before the link-type check) so the two functions stay in lockstep
// as the junction model evolves.
//
// It aborts on the first junction error rather than continuing best-effort —
// deliberately the opposite of removeWarpJunction's rule in weftwiring.go, which
// continues past a per-junction failure because its caller discards the return
// value. Here, a junction inconsistency is a hard error the operator must see,
// and UnwireJunctions gates the exclude-file update on this function succeeding;
// continuing past a corrupted junction would silently work around exactly the
// state this guard exists to surface.
//
// Returns (nil, nil) if junctions is empty or none of its entries exist on
// disk — none were ever wired, or all were already unwired; this is the
// legitimate no-op case, not an error. Returns (removed, err), where removed
// holds every junction Name successfully removed before the failing one, if
// the weft-side target for some junction is missing or unreachable, if that
// junction's warp path is a real directory rather than a junction, or if it
// resolves to an unexpected target — all of these indicate corruption or
// external modification rather than a normal unwire.
// container is the containment boundary every junction in junctions must resolve strictly below — a
// gated site cannot declare containment against a parent it never receives. Its one caller,
// unseedLyxJunction, passes WorktreePath(l, slug).
// rec is the calling verb's own recorder, passed straight through to removeLink for each junction.
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
				// This junction was never wired, or was already unwired; move on
				// to the next one rather than treating it as an error.
				continue
			}
			return removed, fmt.Errorf("lstat %s: %w", link, err)
		}

		// The link exists. Resolve the canonical weft-side target first, exactly
		// as seedLyxJunction does, so a missing/unreachable target is reported
		// distinctly from a wrong-target junction.
		targetResolved, errTarget := filepath.EvalSymlinks(target)
		if errTarget != nil {
			return removed, fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
		}

		isLink, err := fslink.IsLink(link)
		if err != nil {
			return removed, fmt.Errorf("islink %s: %w", link, err)
		}
		if !isLink {
			// A real directory predating weft (or otherwise not a junction); refuse to
			// touch it rather than risk deleting user content.
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
			// The junction points somewhere other than the expected weft directory —
			// corruption or external modification, not a normal unwire target.
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

// unseedGitExclude removes junction-name lines previously added by seedGitExclude
// from the warp worktree's .git/info/exclude file.
//
// It resolves the exclude path exactly as seedGitExclude does (git rev-parse
// --git-path info/exclude, joined with the worktree path if relative), then for
// each junction in WarpJunctions(l, slug, names) removes any line that trims to exactly that
// junction's anchored exclude pattern, or to its legacy bare name (the same line-exact comparison
// seedGitExclude uses to detect presence). The remaining lines are rewritten in their original
// order.
//
// A name another warp worktree in the same hub still has wired is deliberately KEPT, even though
// this call was made against a worktree that no longer wires it.
// git resolves info/exclude to the repo's COMMON gitdir, so there is exactly one exclude file per
// repo, not one per worktree: removing an entry here used to make every sibling worktree's
// still-live junctions show up as untracked dirt in git status, which in turn tripped Remove's
// no-force dirty gate on worktrees the caller never touched.
//
// Returns (false, nil) without touching the file if the exclude file does not
// exist, or if no matching line was found — both are legitimate no-op cases.
// rec is the calling verb's own recorder; it records KindFileWritten at the exclude file's own
// resolved path via Append only when the rewrite reports changed.
func unseedGitExclude(rec *Mutations, l *lyxcwd.Location, slug string, names []string) (changed bool, err error) {
	excludePath, changed, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
		// The sibling census runs INSIDE the lock, so the answer it gives cannot be invalidated by a
		// concurrent wire in another worktree between the census and the write it feeds.
		keep := namesWiredInSiblingWorktrees(l, slug, names)
		stripSet := make(map[string]bool)
		for _, j := range WarpJunctions(l, slug, names) {
			if keep[j.Name] {
				continue
			}
			// Both spellings are stripped: the anchored pattern this binary writes, and the legacy
			// bare name an earlier one wrote, so unwiring a hub wired before the narrowing still
			// reverts cleanly.
			stripSet[excludePatternFor(l.AnchorRel, j.Name)] = true
			stripSet[j.Name] = true
		}
		if len(stripSet) == 0 {
			return content, nil
		}

		lines := strings.Split(content, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if stripSet[strings.TrimSpace(line)] {
				continue
			}
			kept = append(kept, line)
		}
		return strings.Join(kept, "\n"), nil
	})
	if err != nil {
		return false, err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return changed, nil
}

// namesWiredInSiblingWorktrees reports, per name, whether some OTHER warp worktree registered in
// this repo still has a link of that name at its own anchored directory.
// It is what keeps one worktree's unwire from dirtying every sibling: the exclude file it would
// edit is repo-wide, so an entry is only genuinely dead once no worktree wires it any more.
//
// A failure to enumerate worktrees is answered conservatively — every name is reported as still
// wired — because the cost of keeping a dead exclude line is a stale line in a file only lyx
// writes, while the cost of removing a live one is untracked-junction noise in a working tree the
// caller never asked to touch.
func namesWiredInSiblingWorktrees(l *lyxcwd.Location, slug string, names []string) map[string]bool {
	wired := make(map[string]bool, len(names))

	entries, err := List(WorktreePath(l, slug))
	if err != nil {
		entries, err = List(l.WorktreePath())
	}
	if err != nil {
		for _, name := range names {
			wired[name] = true
		}
		return wired
	}

	ownPath := filepath.Clean(WorktreePath(l, slug))
	for _, entry := range entries {
		entryPath := filepath.Clean(filepath.FromSlash(entry.Path))
		if entryPath == ownPath {
			continue
		}
		for _, name := range names {
			if wired[name] {
				continue
			}
			isLink, linkErr := fslink.IsLink(filepath.Join(entryPath, l.AnchorRel, name))
			if linkErr == nil && isLink {
				wired[name] = true
			}
		}
	}
	return wired
}

// excludePatternFor returns the gitignore pattern that excludes exactly the junction named name at
// the anchored directory, and nothing else.
//
// A bare name is what fabric used to write, and gitignore treats a pattern containing no slash as
// matching at ANY depth: on a subpath-anchored monorepo that silently untracked every same-named
// directory elsewhere in the repo (a `frontend/_lyx/` the operator genuinely tracks, say), even
// though fabric only ever wires one junction, at one known path.
// The leading slash anchors the pattern to the repo root, which is where .git/info/exclude is
// evaluated from.
// There is deliberately no trailing slash: a directory junction is a symlink, and git matches a
// symlink as a file, so a directory-only pattern would not exclude it at all.
func excludePatternFor(anchorRel, name string) string {
	return "/" + filepath.ToSlash(filepath.Join(anchorRel, name))
}

// seedGitExclude adds junction names to the warp worktree's .git/info/exclude file if not already present.
//
// It iterates over the junctions returned by WarpJunctions(l, slug, names) and
// appends each junction's anchored exclude pattern (see excludePatternFor) if not already present.
// Resolves the exclude path via git rev-parse --git-path info/exclude. If the
// path is relative, joins it with the worktree path. Preserves line-exact
// idempotency per name.
// A legacy bare-name line left by an earlier binary is REPLACED by the anchored pattern rather than
// left beside it, so an existing hub converges on the narrower exclusion the first time anything
// re-wires it.
// Idempotent: re-running when all junction patterns are already present is a no-op.
// rec is the calling verb's own recorder; it records KindFileWritten at the exclude file's own
// resolved path via Append only when the rewrite reports changed.
func seedGitExclude(rec *Mutations, l *lyxcwd.Location, slug string, names []string) error {
	excludePath, changed, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
		// Drop any legacy bare-name line for a junction being seeded, so the anchored pattern
		// replaces it rather than accumulating beside it.
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
