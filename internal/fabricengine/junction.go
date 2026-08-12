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

// WireJunctions creates directory junctions and seeds git-exclude entries for the given slug over
// the caller-supplied wired name-set.
// The caller must supply the filtered name-set (not loaded by this function).
// Idempotent.
// Enforces the warp-pristine invariant: returns an error if the warp contains a real directory
// predating weft.
func WireJunctions(l *lyxcwd.Location, slug string, names []string) error {
	// Create or verify warp junctions
	if err := seedLyxJunction(l, slug, names); err != nil {
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
	if err := seedGitExclude(l, slug, names); err != nil {
		return err
	}

	return nil
}

// seedLyxJunction creates, verifies, or re-points the warp junctions pointing
// to weft directories. Materializes each junction's weft-side target first.
// A correct link is left alone; a dangling or wrong link is re-pointed;
// a real directory is refused.
func seedLyxJunction(l *lyxcwd.Location, slug string, names []string) error {
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
				if removeErr := repointLink("re-point junction", container, link, ownedDriftedWiredJunction(links)); removeErr != nil {
					return fmt.Errorf("re-point junction %s: %w", link, removeErr)
				}
				if createErr := fslink.CreateDirLink(link, target); createErr != nil {
					return createErr
				}
				continue
			}

			// A real (non-link) directory predating weft; refuse to touch it for
			// `_lyx` — it may hold user content, which fabric never deletes:
			// this now also protects `_lyx/PATTERN.md`, described throughout
			// as the warp repo's hand-authored invariants, which makes "create
			// _lyx/ in the repo and start writing" the natural operator
			// mistake this guard exists to catch.
			// `.lyx` is the one exception: content under it is always lyx's own
			// machine-local scratch (the logger, reed, shuttle, scout and
			// burler all write it unconditionally), so "never touch what might
			// be the user's hand-authored content" does not apply there.
			// Every worktree in existence today has a real `.lyx` for exactly
			// that reason, so without this adoption branch the first
			// `reconcile` after `.lyx` joined the wired name-set would
			// hard-error everywhere.
			if j.Name == lyxdirs.DotLyxDirName {
				if err := adoptDotLyxContent(link, target); err != nil {
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
	}

	return nil
}

// adoptDotLyxContent moves every entry from the warp-side real directory at link into the weft-side
// target, then removes the now-empty warp directory and creates the junction in its place.
// It is the only path seedLyxJunction takes for `.lyx`: every worktree in existence before this
// change holds a real `.lyx` (the logger, reed, shuttle, scout and burler all write it
// unconditionally), so without adoption the first `reconcile` after `.lyx` joined the wired name-set
// would hard-error everywhere.
//
// Refuses before moving anything if any warp-side entry name already exists in target, returning an
// error naming the colliding path and leaving both sides untouched — a collision means an earlier
// adoption already ran, and `.lyx` is disposable enough that the operator can delete the warp-side
// copy; fabric never overwrites or deletes content on its own.
//
// A rename failure is wrapped in an actionable error naming the entry and instructing the operator to
// stop reed/scout and re-run `lyx fabric reconcile` — on Windows, moving a directory with an open
// handle inside it fails, and that is the expected cause.
// The error names whatever was already moved, so a partial move is never reported as success.
//
// Idempotent: a second call finds a link, not a real directory, so seedLyxJunction never reaches this
// helper again for an already-adopted `.lyx`.
func adoptDotLyxContent(link, target string) error {
	entries, err := os.ReadDir(link)
	if err != nil {
		return fmt.Errorf("read %s for adoption: %w", link, err)
	}

	for _, entry := range entries {
		if _, err := os.Lstat(filepath.Join(target, entry.Name())); err == nil {
			return fmt.Errorf(
				"adopt %s into %s: %s already exists at the weft target; an earlier adoption already ran — delete the warp-side copy at %s and re-run `lyx fabric reconcile`",
				link, target, entry.Name(), filepath.Join(link, entry.Name()),
			)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("lstat %s: %w", filepath.Join(target, entry.Name()), err)
		}
	}

	for _, entry := range entries {
		src := filepath.Join(link, entry.Name())
		dst := filepath.Join(target, entry.Name())
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf(
				"adopt %s into %s: move %s failed: %w — stop reed/scout (an open handle inside %s is the expected cause on Windows) and re-run `lyx fabric reconcile`",
				link, target, entry.Name(), err, link,
			)
		}
	}

	if err := os.Remove(link); err != nil {
		return fmt.Errorf("remove now-empty %s after adoption: %w", link, err)
	}

	return fslink.CreateDirLink(link, target)
}

// wireBoardLink creates or repairs the operator-convenience `_board` junction
// at filepath.Join(WorktreePath(l, slug), l.AnchorRel, BoardDirName), pointing
// at BoardDir(l.HubPath), and seeds "_board" into the warp worktree's
// .git/info/exclude via a standalone seedGitExclude(l, slug, []string{BoardDirName})
// call — not by folding BoardDirName into the names slice WireJunctions passes
// down, since WarpJunctions(l, slug, names) would then compute a Target under
// WeftWorktreePath(l, slug) that this junction never uses.
//
// Unlike WireJunctions/seedLyxJunction, this junction is wire-only and
// unmonitored (see junction.go's batch doc): nothing ever reports it broken,
// so this function is the only place its drift is ever corrected, and every
// caller must invoke it unconditionally rather than behind a health check.
//
// Idempotent: an already-correct link is a no-op costing one stat before the
// exclude-seed call. A dangling or mis-pointed link is re-pointed at the
// canonical target, mirroring seedLyxJunction's repair behaviour. A real
// directory predating this wiring is refused, exactly as seedLyxJunction
// refuses to clobber user content — that case cannot arise for a name lyx
// itself reserves, but the guard costs nothing and keeps this function's
// per-link handling in lockstep with seedLyxJunction's.
func wireBoardLink(l *lyxcwd.Location, slug string) error {
	link := filepath.Join(WorktreePath(l, slug), l.AnchorRel, BoardDirName)
	target := BoardDir(l.HubPath)

	if _, err := os.Lstat(link); err == nil {
		isLink, err := fslink.IsLink(link)
		if err != nil {
			return fmt.Errorf("islink %s: %w", link, err)
		}
		if !isLink {
			return fmt.Errorf(
				"warp repo already contains a real %s at %s; it predates the board junction — move its content aside, or remove this directory, then re-run `lyx fabric reconcile` to create the junction",
				filepath.Base(link), link,
			)
		}

		targetResolved, targetErr := filepath.EvalSymlinks(target)
		if targetErr != nil {
			return fmt.Errorf("weft directory does not exist at %s; cannot validate board junction target", target)
		}
		linkResolved, resolveErr := fslink.PointsTo(link)
		if resolveErr != nil || linkResolved != targetResolved {
			// Dangling or wrong-target — corrupted or externally-modified
			// wiring, not user content. Re-point it, mirroring
			// seedLyxJunction's repair path.
			if removeErr := repointLink("re-point board junction", WorktreePath(l, slug), link, ownedDriftedWiredJunction([]string{link})); removeErr != nil {
				return fmt.Errorf("re-point board junction %s: %w", link, removeErr)
			}
			if createErr := fslink.CreateDirLink(link, target); createErr != nil {
				return createErr
			}
		}
	} else if os.IsNotExist(err) {
		if createErr := fslink.CreateDirLink(link, target); createErr != nil {
			return createErr
		}
	} else {
		return fmt.Errorf("lstat %s: %w", link, err)
	}

	return seedGitExclude(l, slug, []string{BoardDirName})
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
func UnwireJunctions(l *lyxcwd.Location, slug string, names []string) (UnwireResult, error) {
	removed, err := unseedLyxJunction(l, slug, names)
	if err != nil {
		return UnwireResult{JunctionsRemoved: removed}, err
	}

	changed, err := unseedGitExclude(l, slug, names)
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
func unseedLyxJunction(l *lyxcwd.Location, slug string, names []string) (removed []string, err error) {
	return unseedJunctionRecords(WorktreePath(l, slug), WarpJunctions(l, slug, names))
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
func unseedJunctionRecords(container string, junctions []WarpJunction) (removed []string, err error) {
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
		if err := removeLink(req); err != nil {
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
func unseedGitExclude(l *lyxcwd.Location, slug string, names []string) (changed bool, err error) {
	return mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
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
func seedGitExclude(l *lyxcwd.Location, slug string, names []string) error {
	_, err := mutateGitExclude(WorktreePath(l, slug), func(content string) (string, error) {
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
	return err
}
