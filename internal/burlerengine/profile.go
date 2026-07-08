// profile.go defines Profile, the content contract for one burler round —
// what to review, what to judge it against, and how the round is allowed to
// write its fixes — plus its fail-loud validate method. Profile is pure data;
// the shuttle interaction that consumes it lives in engine.go (added in the
// next batch).

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileSet names what a review phase reads: a list of files/directories
// (Paths — a directory means everything under it) and/or free-form
// Instructions for path-less targets (e.g. "review the diff against main").
// At least one of the two fields must be non-empty; validate enforces this.
type FileSet struct {
	Paths        []string
	Instructions string
}

// FixScope selects the write-surface and git discipline for phase B of a
// round. It is content-agnostic — the distinction is where fixes may land
// and whether they are committed, never what kind of content the target is.
type FixScope string

// The two legal FixScope values. Any other value, including the empty
// string, is rejected by validate — the field selects safety-critical
// behavior (git vs no git) and gets no silent default.
const (
	// FixScopeOverlay marks the target as lyx system/orchestration state
	// (plan, discussion, review artifacts). B's write surface is exactly
	// Target.Paths plus the two output files, and the agent performs no
	// git operations at all — the Weft Git Invariant reserves committing
	// this class of file to the loop owner.
	FixScopeOverlay FixScope = "overlay"
	// FixScopeSource marks the target as the host repo's own files. B's
	// write surface is the host working tree; the agent commits per fix
	// on the host repo and never pushes.
	FixScopeSource FixScope = "source"
)

// ErrClusterUnsupported is the sentinel wrapped into the error returned when
// a Profile requests ClusterN > 0. Cluster fan-out is gated on mux own-window
// anchoring (roadmap milestone 24) and is not implemented in v1; callers can
// test for this specific rejection with errors.Is.
var ErrClusterUnsupported = errors.New("burler: cluster-N > 0 is not supported — cluster reviewers are gated on mux own-window anchoring (roadmap milestone 24); use cluster-N = 0")

// Profile is the content contract for one burler round: what to review
// (Target), what to judge it against (Fasit), the criteria (Rubric), the
// write-surface discipline (FixScope), whether the agent may drive the real
// substrate (ToolUse), cluster fan-out (ClusterN, v1-unsupported above zero),
// the caller-named output paths, and optional prior-round files for
// clean-room hydration. Profile is caller-constructed data; validate is the
// single place that normalizes and checks it before a round runs.
type Profile struct {
	// Target is what to review AND what phase B may fix.
	Target FileSet
	// Fasit is the read-only source of truth the target is judged against.
	// An empty Fasit degenerates the review to internal-consistency
	// checking, which validate rejects.
	Fasit FileSet
	// Rubric is markdown criteria for this artifact type: what counts as
	// BLOCKING/MEDIUM/LOW/NIT for THIS target. A rubric maps its criteria
	// onto the fixed four-value severity vocabulary; it never defines new
	// severity names.
	Rubric string
	// FixScope selects the write-surface and git discipline for phase B.
	FixScope FixScope
	// ToolUse toggles prompt instructions between "drive the real
	// substrate (build/test/run)" and "read-only analysis". It has no
	// effect on the shuttle Spec in v1 — see the tool-use-prompt-level
	// decision.
	ToolUse bool
	// ClusterN selects fan-out review count. Only 0 is supported in v1;
	// any positive value fails validate with ErrClusterUnsupported.
	ClusterN int
	// ReviewPath and FixerReportPath are the caller-supplied output
	// paths for the round's two artifacts. Both are required.
	ReviewPath      string
	FixerReportPath string
	// PriorReviews and PriorFixerReports optionally hydrate the round
	// with earlier-round artifacts for clean-room, form-your-own-findings
	// -first regression checking. When present, every entry must exist on
	// disk — a missing prior-round file is a caller bug, never silently
	// omitted from the prompt.
	PriorReviews      []string
	PriorFixerReports []string
}

// RunOpts carries the run-tuning knobs that are kept off the content
// Profile because they vary per invocation (perch varies model/effort per
// round) rather than per artifact. Each field maps 1:1 onto the
// corresponding shuttleengine.Spec field; zero values defer to the
// engine/config default.
type RunOpts struct {
	Model   string
	Effort  string
	Timeout time.Duration
	Round   string
}

// validate normalizes p in place and reports a fail-loud, burler-prefixed
// error if it is not runnable. It resolves every path field to a cleaned
// absolute path (already-absolute entries kept verbatim, relative entries
// joined onto worktreeRoot), mirroring shuttleengine.Spec.validate so every
// later reader — the prompt, the Spec, Result — sees only absolute paths.
// Checks run in the fixed order documented on the fields below; the first
// failure is returned.
func (p *Profile) validate(worktreeRoot string) error {
	// Resolve every path-bearing field up front, before any existence or
	// content check, so later checks never have to reason about relative
	// vs. absolute paths again.
	p.Target.Paths = resolvePaths(worktreeRoot, p.Target.Paths)
	p.Fasit.Paths = resolvePaths(worktreeRoot, p.Fasit.Paths)
	p.PriorReviews = resolvePaths(worktreeRoot, p.PriorReviews)
	p.PriorFixerReports = resolvePaths(worktreeRoot, p.PriorFixerReports)
	p.ReviewPath = resolvePath(worktreeRoot, p.ReviewPath)
	p.FixerReportPath = resolvePath(worktreeRoot, p.FixerReportPath)

	// Target and Fasit must each carry at least one of Paths /
	// non-whitespace Instructions. An empty Fasit specifically degenerates
	// the review to internal-consistency checking, which is worth calling
	// out by name since it is the failure mode most likely to be an
	// operator oversight rather than a typo.
	if len(p.Target.Paths) == 0 && strings.TrimSpace(p.Target.Instructions) == "" {
		return fmt.Errorf("burler: profile.Target must set at least one of Paths or Instructions")
	}
	if len(p.Fasit.Paths) == 0 && strings.TrimSpace(p.Fasit.Instructions) == "" {
		return fmt.Errorf("burler: profile.Fasit must set at least one of Paths or Instructions — an empty Fasit degenerates the review to internal-consistency checking, which is not a valid round")
	}

	// Every resolved path in the four path-list fields must exist. A file
	// or a directory both count (a directory means "everything under it").
	if err := requireExistingPaths("Target.Paths", p.Target.Paths); err != nil {
		return err
	}
	if err := requireExistingPaths("Fasit.Paths", p.Fasit.Paths); err != nil {
		return err
	}
	if err := requireExistingPaths("PriorReviews", p.PriorReviews); err != nil {
		return err
	}
	if err := requireExistingPaths("PriorFixerReports", p.PriorFixerReports); err != nil {
		return err
	}

	if strings.TrimSpace(p.Rubric) == "" {
		return fmt.Errorf("burler: profile.Rubric must not be empty")
	}

	// FixScope selects safety-critical behavior (git vs no git) and gets
	// no silent default — anything other than the two named constants,
	// empty included, is an error naming both legal values.
	if p.FixScope != FixScopeOverlay && p.FixScope != FixScopeSource {
		return fmt.Errorf("burler: profile.FixScope must be %q or %q, got %q", FixScopeOverlay, FixScopeSource, p.FixScope)
	}

	if p.ClusterN < 0 {
		return fmt.Errorf("burler: profile.ClusterN must not be negative (got %d)", p.ClusterN)
	}
	if p.ClusterN > 0 {
		// Wrap the sentinel with %w so callers can test with errors.Is
		// while still getting a burler-prefixed, self-explanatory message.
		return fmt.Errorf("burler: profile.ClusterN = %d: %w", p.ClusterN, ErrClusterUnsupported)
	}

	if p.ReviewPath == "" {
		return fmt.Errorf("burler: profile.ReviewPath must not be empty")
	}
	if p.FixerReportPath == "" {
		return fmt.Errorf("burler: profile.FixerReportPath must not be empty")
	}

	return nil
}

// resolvePath resolves a single path to a cleaned absolute path: an
// already-absolute path is kept verbatim, a relative path is joined onto
// worktreeRoot and cleaned. An empty path resolves to empty — validate
// checks emptiness itself, using the resolved-but-empty value.
func resolvePath(worktreeRoot, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(worktreeRoot, path))
}

// resolvePaths applies resolvePath to every entry of paths, returning a new
// slice so callers never alias the caller-supplied backing array.
func resolvePaths(worktreeRoot string, paths []string) []string {
	resolved := make([]string, len(paths))
	for i, p := range paths {
		resolved[i] = resolvePath(worktreeRoot, p)
	}
	return resolved
}

// requireExistingPaths reports a burler-prefixed error naming fieldName and
// the first missing entry if any path in paths does not exist on disk. A
// file or a directory both satisfy the check.
func requireExistingPaths(fieldName string, paths []string) error {
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("burler: profile.%s entry %q does not exist: %w", fieldName, p, err)
		}
	}
	return nil
}
