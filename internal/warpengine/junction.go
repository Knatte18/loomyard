// junction.go implements the atomic, cwd-keyed junction primitive for warp topology.
//
// WireJunctions creates host↔weft directory junctions and manages their git-exclude
// entries atomically, keyed by the current worktree's slug. It is idempotent,
// guarding against re-entry and enforcing the host-pristine invariant by refusing
// to wire when the host contains a pre-existing real directory predating weft.

package warpengine

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
//   - Creates the directory junction via fslink.CreateDirLink (idempotent via fslink.IsLink/PointsTo)
//   - Appends the junction Name to the host worktree's .git/info/exclude (line-exact idempotent)
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

// seedLyxJunction creates or verifies the host junctions pointing to weft directories.
//
// It iterates over the junctions returned by l.HostJunctions(slug), applying the same
// create-or-verify logic per junction using each record's Link and Target.
//
// For each junction, if it already exists:
//   - Validates that it resolves to the correct target via fslink.PointsTo
//   - Checks using fslink.IsLink to determine if it's a link
//   - Returns nil (idempotent)
//
// If os.Lstat fails with not-exist:
//   - Creates the junction via fslink.CreateDirLink
//
// Otherwise:
//   - Returns an error indicating the host repo contains a real directory that predates weft
func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
	junctions := l.HostJunctions(slug)

	for _, j := range junctions {
		link := j.Link
		target := j.Target

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
			}

			// Not a link or points elsewhere; this is a real directory issue
			return fmt.Errorf(
				"host repo already contains a real %s at %s; it predates weft — migrate via the hub-creator",
				filepath.Base(link),
				link,
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
