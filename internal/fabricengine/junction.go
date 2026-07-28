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

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// WireJunctions creates directory junctions and seeds git-exclude entries for the
// current worktree, keyed by slug.
//
// For each junction in l.HostJunctions(slug), WireJunctions:
//   - Materialises the junction's weft-side target via os.MkdirAll
//   - Creates the directory junction via fslink.CreateDirLink (idempotent via fslink.IsLink/PointsTo)
//   - Appends the junction Name to the host worktree's .git/info/exclude (line-exact idempotent)
//
// Materialising the weft-side target is what lets every WireJunctions caller leave
// a resolvable junction behind: initengine/init.go, fabricengine/checkout.go, and
// fabricengine/reconcile.go all call WireJunctions, but of the three only Init
// materialises the weft directory itself. Before this, fslink.CreateDirLink would
// happily create a link to a nonexistent target (a raw reparse point on Windows, a
// dangling symlink elsewhere), leaving checkout's and reconcile's junctions
// dangling until some other path created the target.
//
// The two operations are sequenced such that if either fails, the junction may be
// left partially wired; the caller is responsible for rollback if needed. The
// operations themselves are individually idempotent (re-running is safe).
//
// WireJunctions enforces the host-pristine invariant: it returns an error if the
// host repo contains a real (non-junction) directory predating weft (refusing the
// initial seedLyxJunction call); the exclude-append step never triggers this guard
// since it operates on an exclude file, not the directory tree.
//
// Returns nil on success. Returns an error if:
//   - The host contains a real directory predating weft (violation of pristine invariant)
//   - Junction or exclude operations fail (wrapped with context)
func WireJunctions(l *hubgeometry.Layout, slug string) error {
	// Create or verify host junctions
	if err := seedLyxJunction(l, slug); err != nil {
		return err
	}

	// Append junction names to git-exclude
	if err := seedGitExclude(l, slug); err != nil {
		return err
	}

	return nil
}

// seedLyxJunction creates, verifies, or re-points the host junctions pointing
// to weft directories.
//
// It iterates over the junctions returned by l.HostJunctions(slug), applying the same
// create-or-verify-or-re-point logic per junction using each record's Link and Target.
//
// Before any check runs, each iteration materialises its junction's weft-side
// target via os.MkdirAll — deliberately the first statement of the loop, ahead
// of the os.Lstat(link) call below and both fslink.CreateDirLink call sites.
// fslink.CreateDirLink happily creates a link to a nonexistent target (a raw
// reparse point on Windows, a dangling symlink elsewhere), so a junction a prior
// checkout or reconcile left dangling would otherwise hit the link-exists
// branch's filepath.EvalSymlinks(target) failure below — the hard "weft
// directory does not exist" error — on every subsequent WireJunctions call.
// Materialising the target first gives that worktree a self-repair path instead.
//
// For each junction, if the path already exists:
//   - A link resolving to the correct target is left alone (idempotent).
//   - A link that dangles or resolves to the wrong target is re-pointed —
//     removed and recreated toward the canonical weft target. A link is
//     fabric-owned wiring metadata, never user content, so replacing it is
//     always safe; this is what lets Reconcile repair a corrupted junction.
//   - A real (non-link) directory is refused: it predates weft and may hold
//     user content, which fabric never deletes.
//
// If os.Lstat fails with not-exist:
//   - Creates the junction via fslink.CreateDirLink
func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
	junctions := l.HostJunctions(slug)

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
				"host repo already contains a real %s at %s; it predates weft — move its content into the paired weft worktree's own %s, or remove this directory, then re-run `lyx init` to create the junction",
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
	// JunctionRemoved reports whether the host _lyx junction was present and removed.
	JunctionRemoved bool
	// ExcludeChanged reports whether a junction-name line was removed from
	// .git/info/exclude.
	ExcludeChanged bool
}

// UnwireJunctions reverses WireJunctions for the current worktree, keyed by slug:
// it removes the host _lyx junction and its .git/info/exclude entry, undoing
// exactly what WireJunctions seeded — nothing more (the worktree pairing and weft
// content are untouched; see Remove for the larger paired-teardown operation).
//
// The junction is unwired before the exclude entry, mirroring WireJunctions'
// creation order in reverse. Per the "any junction inconsistency is a hard error"
// invariant, if unseedLyxJunction reports an error the exclude file is never
// touched: an unexpected junction state (a real directory, or a link pointing
// somewhere unexpected) aborts the whole operation so a corrupted or
// externally-modified junction is never silently worked around.
//
// Returns an empty UnwireResult and nil error when the junction was never wired
// (the legitimate no-op case). Returns an error, with JunctionRemoved reflecting
// what already happened, if the exclude-file update fails after a successful
// junction removal.
func UnwireJunctions(l *hubgeometry.Layout, slug string) (UnwireResult, error) {
	removed, err := unseedLyxJunction(l, slug)
	if err != nil {
		return UnwireResult{}, err
	}

	changed, err := unseedGitExclude(l, slug)
	if err != nil {
		return UnwireResult{JunctionRemoved: removed}, err
	}

	return UnwireResult{JunctionRemoved: removed, ExcludeChanged: changed}, nil
}

// unseedLyxJunction removes the host _lyx junction for slug, mirroring
// seedLyxJunction's validation in the same order (target resolution before the
// link-type check) so the two functions stay in lockstep as the junction model
// evolves.
//
// It is deliberately scoped to the single _lyx junction (HostLyxLink/WeftLyxDirFor)
// rather than iterating l.HostJunctions(slug) the way unseedGitExclude does:
// HostJunctions returns exactly one entry today, and UnwireResult.JunctionRemoved
// is a single bool by design to match. If HostJunctions ever grows a second entry,
// this function and UnwireResult should be revisited together.
//
// Returns (false, nil) if the junction does not exist — it was never wired, or was
// already unwired; this is the legitimate no-op case, not an error. Returns an
// error, without touching the link, if the weft-side target is missing or
// unreachable, if the host path is a real directory rather than a junction, or if
// the junction resolves to an unexpected target — all of these indicate corruption
// or external modification rather than a normal unwire.
func unseedLyxJunction(l *hubgeometry.Layout, slug string) (removed bool, err error) {
	link := l.HostLyxLink(slug)

	if _, err := os.Lstat(link); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("lstat %s: %w", link, err)
	}

	// The link exists. Resolve the canonical weft-side target first, exactly as
	// seedLyxJunction does, so a missing/unreachable target is reported distinctly
	// from a wrong-target junction.
	target := l.WeftLyxDirFor(slug)
	targetResolved, errTarget := filepath.EvalSymlinks(target)
	if errTarget != nil {
		return false, fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
	}

	isLink, err := fslink.IsLink(link)
	if err != nil {
		return false, fmt.Errorf("islink %s: %w", link, err)
	}
	if !isLink {
		// A real directory predating weft (or otherwise not a junction); refuse to
		// touch it rather than risk deleting user content.
		return false, fmt.Errorf(
			"host repo already contains a real %s at %s; it is not a junction — refusing to remove it",
			filepath.Base(link),
			link,
		)
	}

	linkResolved, err := fslink.PointsTo(link)
	if err != nil {
		return false, fmt.Errorf("resolve link target %s: %w", link, err)
	}
	if linkResolved != targetResolved {
		// The junction points somewhere other than the expected weft directory —
		// corruption or external modification, not a normal unwire target.
		return false, fmt.Errorf(
			"host junction %s points to unexpected target %s (want %s); refusing to remove it",
			link, linkResolved, targetResolved,
		)
	}

	if err := fslink.Remove(link); err != nil {
		return false, fmt.Errorf("remove host junction %s: %w", link, err)
	}
	return true, nil
}

// unseedGitExclude removes junction-name lines previously added by seedGitExclude
// from the host worktree's .git/info/exclude file.
//
// It resolves the exclude path exactly as seedGitExclude does (git rev-parse
// --git-path info/exclude, joined with the worktree path if relative), then for
// each junction in l.HostJunctions(slug) removes any line that trims to exactly
// that junction's Name (the same line-exact comparison seedGitExclude uses to
// detect presence). The remaining lines are rewritten in their original order.
//
// Returns (false, nil) without touching the file if the exclude file does not
// exist, or if no matching line was found — both are legitimate no-op cases.
func unseedGitExclude(l *hubgeometry.Layout, slug string) (changed bool, err error) {
	worktreePath := l.WorktreePath(slug)

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

	// Build the set of junction names to strip; today this is always the single
	// _lyx entry, but iterate l.HostJunctions(slug) for parity with seedGitExclude.
	names := make(map[string]bool)
	for _, j := range l.HostJunctions(slug) {
		names[j.Name] = true
	}

	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if names[strings.TrimSpace(line)] {
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
// It iterates over the junctions returned by l.HostJunctions(slug) and appends each
// junction's Name to the exclude file if not already present. Resolves the exclude
// path via git rev-parse --git-path info/exclude. If the path is relative, joins it
// with the worktree path. Preserves line-exact idempotency per name.
// Idempotent: re-running when all junction names are already present is a no-op.
func seedGitExclude(l *hubgeometry.Layout, slug string) error {
	worktreePath := l.WorktreePath(slug)

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
	junctions := l.HostJunctions(slug)
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
