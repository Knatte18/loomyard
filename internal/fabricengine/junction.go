// junction.go implements the atomic, cwd-keyed junction primitive for fabric topology.
//
// WireJunctions creates host↔weft directory junctions and manages their git-exclude
// entries atomically, keyed by the current worktree's slug. It is idempotent,
// guarding against re-entry and enforcing the host-pristine invariant by refusing
// to wire when the host contains a pre-existing real directory predating weft.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// WorktreePath returns the path to a sibling worktree with the given slug.
// It replaces (*lyxcwd.Location).WorktreePath(slug), which collided with
// the no-arg WorktreePath() accessor the coming reshape introduces on the
// same type.
func WorktreePath(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug)
}

// HostLyxLink returns the path to the _lyx junction link in a named slug's host worktree.
// It is the host-side junction endpoint that points into the paired weft worktree via WeftLyxDirFor(l, slug).
func HostLyxLink(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, configengine.LyxDirName)
}

// HostLyxLinkHere returns the path to the _lyx junction link in the current host worktree.
// Derived from l.WorktreePath()+AnchorRel. It serves as the host-side junction endpoint
// paired with WeftLyxDir(l).
func HostLyxLinkHere(l *lyxcwd.Location) string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel, configengine.LyxDirName)
}

// HostJunction represents a directory junction in the host worktree that links to a weft directory.
type HostJunction struct {
	Name   string // Name is the directory name (e.g., "_lyx")
	Link   string // Link is the host-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// HostJunctions returns the list of host junctions for a given slug, one record per name in names,
// in names's own order (no forced sort). For each name, the record is {Name, Link, Target} where
// Link is HubPath/slug-anchored via WorktreePath(l, slug) and Target is computed via
// WeftWorktreePath(l, slug) and AnchorRel.
// HostJunctions is HubPath/slug-anchored; HostJunctionsHere below is the Here-anchored counterpart.
func HostJunctions(l *lyxcwd.Location, slug string, names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(WorktreePath(l, slug), l.AnchorRel, name),
			Target: filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, name),
		})
	}
	return junctions
}

// HostJunctionsHere returns the same HostJunction records as HostJunctions(l, slug, names),
// but resolved against the current worktree rather than a named slug: Link is built from
// l.WorktreePath() and each Target from WeftWorktree(l). This mirrors HostLyxLinkHere(l)/HostLyxLink(l, slug).
// It exists for health-check sites that are Here-anchored and have no slug available.
func HostJunctionsHere(l *lyxcwd.Location, names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreePath(), l.AnchorRel, name),
			Target: filepath.Join(WeftWorktree(l), l.AnchorRel, name),
		})
	}
	return junctions
}

// WireJunctions creates directory junctions and seeds git-exclude entries for
// the given slug over the caller-supplied wired name-set. The caller must supply
// the filtered name-set (not loaded by this function). Idempotent. Enforces
// the host-pristine invariant: returns an error if the host contains a real
// directory predating weft.
func WireJunctions(l *lyxcwd.Location, slug string, names []string) error {
	// Create or verify host junctions
	if err := seedLyxJunction(l, slug, names); err != nil {
		return err
	}

	// Append junction names to git-exclude
	if err := seedGitExclude(l, slug, names); err != nil {
		return err
	}

	return nil
}

// seedLyxJunction creates, verifies, or re-points the host junctions pointing
// to weft directories. Materializes each junction's weft-side target first.
// A correct link is left alone; a dangling or wrong link is re-pointed;
// a real directory is refused.
func seedLyxJunction(l *lyxcwd.Location, slug string, names []string) error {
	junctions := HostJunctions(l, slug, names)

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
				if removeErr := fslink.Remove(link); removeErr != nil {
					return fmt.Errorf("re-point junction %s: %w", link, removeErr)
				}
				if createErr := fslink.CreateDirLink(link, target); createErr != nil {
					return createErr
				}
				continue
			}

			// A real (non-link) directory predating weft; refuse to touch it —
			// it may hold user content, which fabric never deletes. The remedy
			// clause names what the operator can actually do, since it must
			// serve both _lyx (this batch's baseline) and _pattern (a later
			// batch's second junction) alike: PATTERN content is described
			// throughout as the host repo's hand-authored invariants, which
			// makes "create _pattern/ in the repo and start writing" the
			// natural operator mistake this guard exists to catch.
			return fmt.Errorf(
				"host repo already contains a real %s at %s; it predates weft — move its content into the paired weft worktree's own %s, or remove this directory, then re-run `lyx fabric reconcile` to create the junction",
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

// UnwireResult reports which parts of UnwireJunctions actually changed state,
// distinguishing a real reversal from a no-op on an already-clean (or
// never-wired) worktree.
type UnwireResult struct {
	// JunctionsRemoved lists the Name of each junction that was actually present
	// and removed, in HostJunctions(l, slug) order. A name slice, not a count or
	// a bool: which junction(s) were removed is CLI-observable, and "1 of 2
	// removed" tells an operator nothing about which one is still wired.
	JunctionsRemoved []string
	// ExcludeChanged reports whether a junction-name line was removed from
	// .git/info/exclude.
	ExcludeChanged bool
}

// UnwireJunctions reverses WireJunctions for the current worktree, keyed by slug,
// over the same caller-supplied names: it removes every host junction in
// HostJunctions(l, slug, names) and their shared .git/info/exclude entries, undoing
// exactly what WireJunctions seeded — nothing more (the worktree pairing and weft
// content are untouched; see Remove for the larger paired-teardown operation).
// Like WireJunctions, it loads no config itself.
//
// The junctions are unwired before the exclude entries, mirroring WireJunctions'
// creation order in reverse. Per the "any junction inconsistency is a hard error"
// invariant, if unseedLyxJunction reports an error the exclude file is never
// touched: an unexpected junction state (a real directory, or a link pointing
// somewhere unexpected) aborts the whole operation so a corrupted or
// externally-modified junction is never silently worked around.
//
// Returns an empty UnwireResult and nil error when no junction was wired (the
// legitimate no-op case). Returns an error, with JunctionsRemoved reflecting
// whatever was already removed before the failure, if unseedLyxJunction aborts
// partway through its loop, or if the exclude-file update fails after junction
// removal completed. A zero UnwireResult on a mid-loop failure would misreport a
// partial removal as untouched — with two or more junctions, the first may
// already be gone before the second fails.
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

// unseedLyxJunction removes every host junction in HostJunctions(l, slug, names).
// It is a thin wrapper over unseedJunctionRecords, which owns the actual
// per-junction loop; the split exists purely so the loop's abort-and-accumulate
// contract is directly testable against a synthetic junction slice, since
// l.HostJunctions always returns exactly one entry today and cannot itself
// produce the multi-junction scenario the contract is about.
//
// Returns (nil, nil) if no junction exists — none were ever wired, or all were
// already unwired; this is the legitimate no-op case, not an error. See
// unseedJunctionRecords for the error cases.
func unseedLyxJunction(l *lyxcwd.Location, slug string, names []string) (removed []string, err error) {
	return unseedJunctionRecords(HostJunctions(l, slug, names))
}

// unseedJunctionRecords removes each junction in junctions in order, mirroring
// seedLyxJunction's per-junction validation in the same order (target
// resolution before the link-type check) so the two functions stay in lockstep
// as the junction model evolves.
//
// It aborts on the first junction error rather than continuing best-effort —
// deliberately the opposite of removeHostJunction's rule in weftwiring.go, which
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
// junction's host path is a real directory rather than a junction, or if it
// resolves to an unexpected target — all of these indicate corruption or
// external modification rather than a normal unwire.
func unseedJunctionRecords(junctions []HostJunction) (removed []string, err error) {
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
				"host repo already contains a real %s at %s; it is not a junction — refusing to remove it",
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
				"host junction %s points to unexpected target %s (want %s); refusing to remove it",
				link, linkResolved, targetResolved,
			)
		}

		if err := fslink.Remove(link); err != nil {
			return removed, fmt.Errorf("remove host junction %s: %w", link, err)
		}
		removed = append(removed, j.Name)
	}

	return removed, nil
}

// unseedGitExclude removes junction-name lines previously added by seedGitExclude
// from the host worktree's .git/info/exclude file.
//
// It resolves the exclude path exactly as seedGitExclude does (git rev-parse
// --git-path info/exclude, joined with the worktree path if relative), then for
// each junction in HostJunctions(l, slug, names) removes any line that trims to
// exactly that junction's Name (the same line-exact comparison seedGitExclude
// uses to detect presence). The remaining lines are rewritten in their original
// order.
//
// Returns (false, nil) without touching the file if the exclude file does not
// exist, or if no matching line was found — both are legitimate no-op cases.
func unseedGitExclude(l *lyxcwd.Location, slug string, names []string) (changed bool, err error) {
	worktreePath := WorktreePath(l, slug)

	stdout, stderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		worktreePath,
	)
	if err != nil {
		return false, fmt.Errorf("failed to get git-path for info/exclude: %w", err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("git rev-parse --git-path failed: %s", stderr)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing was ever seeded; nothing to revert.
			return false, nil
		}
		return false, fmt.Errorf("read exclude file: %w", err)
	}

	// Build the set of junction names to strip from the caller-supplied names,
	// iterating HostJunctions(l, slug, names) for parity with seedGitExclude.
	stripSet := make(map[string]bool)
	for _, j := range HostJunctions(l, slug, names) {
		stripSet[j.Name] = true
	}

	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if stripSet[strings.TrimSpace(line)] {
			changed = true
			continue
		}
		kept = append(kept, line)
	}

	if !changed {
		return false, nil
	}

	if err := os.WriteFile(excludePath, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return false, fmt.Errorf("write exclude file: %w", err)
	}
	return true, nil
}

// seedGitExclude adds junction names to the host worktree's .git/info/exclude file if not already present.
//
// It iterates over the junctions returned by HostJunctions(l, slug, names) and
// appends each junction's Name to the exclude file if not already present.
// Resolves the exclude path via git rev-parse --git-path info/exclude. If the
// path is relative, joins it with the worktree path. Preserves line-exact
// idempotency per name.
// Idempotent: re-running when all junction names are already present is a no-op.
func seedGitExclude(l *lyxcwd.Location, slug string, names []string) error {
	worktreePath := WorktreePath(l, slug)

	// Get the exclude path via git rev-parse --git-path
	stdout, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		worktreePath,
	)
	if err != nil {
		return fmt.Errorf("failed to get git-path for info/exclude: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("resolve git exclude path for %q failed (git exit %d)", worktreePath, exitCode)
	}

	excludePath := strings.TrimSpace(stdout)

	// If path is relative, join with worktree path
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("mkdir for exclude file: %w", err)
	}

	// Read the file
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read exclude file: %w", err)
	}

	contentStr := string(content)

	// Iterate over junction names and append each if not already present.
	junctions := HostJunctions(l, slug, names)
	for _, j := range junctions {
		name := j.Name

		// Check if name is already present as a line-exact match
		found := false
		for _, line := range strings.Split(contentStr, "\n") {
			if strings.TrimSpace(line) == name {
				found = true
				break
			}
		}

		if found {
			// Already present, skip to next junction
			continue
		}

		// Append name with newline
		if contentStr != "" && !strings.HasSuffix(contentStr, "\n") {
			contentStr += "\n"
		}
		contentStr += name + "\n"
	}

	// Write back
	if err := os.WriteFile(excludePath, []byte(contentStr), 0o644); err != nil {
		return fmt.Errorf("write exclude file: %w", err)
	}

	return nil
}
