// profile.go defines Profile, the content contract for one burler round — what to review, what to
// judge it against, and how the round is allowed to write its fixes — plus its fail-loud validate
// method.
// Profile is pure data;
// the shuttle interaction that consumes it lives in engine.go (added in the next batch).

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// FileSet names what a review phase reads: paths and/or free-form instructions.
type FileSet struct {
	Paths        []string
	Instructions string
}

// FixScope selects the write-surface and git discipline for phase B.
type FixScope string

// The two legal FixScope values.
const (
	FixScopeOverlay FixScope = "overlay" // lyx state, no git
	FixScopeSource  FixScope = "source"  // the repo, with git commits
)

// Profile is the content contract for one burler round.
type Profile struct {
	Target     FileSet
	Fasit      FileSet
	Rubric     string
	FixScope   FixScope
	ToolUse    bool
	ClusterFan string
	// ClusterExclude names lenses to drop from the resolved ClusterFan — a
	// per-call advisory filter, not config.
	ClusterExclude    []string
	clusterLenses     []Lens
	ReviewPath        string
	FixerReportPath   string
	PriorReviews      []string
	PriorFixerReports []string
}

// RunOpts carries run-tuning knobs kept off Profile.
// Each field maps 1:1 onto shuttleengine.Spec;
// zero values defer to engine/config default.
type RunOpts struct {
	Model   string
	Effort  string
	Timeout time.Duration
	Round   string
}

// validate normalizes p in place and reports a fail-loud error if not runnable.
// Checks run in fixed order; the first failure is returned.
func (p *Profile) validate(worktreeRoot string, cfg Config) error {
	// Resolve all path fields to absolute first.
	p.Target.Paths = resolvePaths(worktreeRoot, p.Target.Paths)
	p.Fasit.Paths = resolvePaths(worktreeRoot, p.Fasit.Paths)
	p.PriorReviews = resolvePaths(worktreeRoot, p.PriorReviews)
	p.PriorFixerReports = resolvePaths(worktreeRoot, p.PriorFixerReports)
	p.ReviewPath = resolvePath(worktreeRoot, p.ReviewPath)
	p.FixerReportPath = resolvePath(worktreeRoot, p.FixerReportPath)

	// Target and Fasit must each carry at least one of Paths/Instructions.
	if len(p.Target.Paths) == 0 && strings.TrimSpace(p.Target.Instructions) == "" {
		return fmt.Errorf("burler: profile.Target must set at least one of Paths or Instructions")
	}
	if len(p.Fasit.Paths) == 0 && strings.TrimSpace(p.Fasit.Instructions) == "" {
		return fmt.Errorf("burler: profile.Fasit must set at least one of Paths or Instructions — an empty Fasit degenerates the review to internal-consistency checking, which is not a valid round")
	}

	// Every resolved path must exist (files or directories).
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

	// FixScope selects safety-critical behavior and gets no silent default.
	if p.FixScope != FixScopeOverlay && p.FixScope != FixScopeSource {
		return fmt.Errorf("burler: profile.FixScope must be %q or %q, got %q", FixScopeOverlay, FixScopeSource, p.FixScope)
	}

	if p.ClusterFan == "" && len(p.ClusterExclude) > 0 {
		return fmt.Errorf("burler: profile.ClusterExclude is set but profile.ClusterFan is empty — there is no fan to trim")
	}
	if p.ClusterFan != "" {
		// Resolve fan now so bad names fail upfront.
		lenses, err := ResolveFan(cfg, p.ClusterFan)
		if err != nil {
			return err
		}
		p.clusterLenses = applyClusterExclude(p.ClusterFan, lenses, p.ClusterExclude)
	}

	if p.ReviewPath == "" {
		return fmt.Errorf("burler: profile.ReviewPath must not be empty")
	}
	if p.FixerReportPath == "" {
		return fmt.Errorf("burler: profile.FixerReportPath must not be empty")
	}
	// A same-path pair would violate the file contract (OutputFiles must be distinct).
	if p.ReviewPath == p.FixerReportPath {
		return fmt.Errorf("burler: profile.ReviewPath and profile.FixerReportPath must not be the same path (got %q for both)", p.ReviewPath)
	}

	return nil
}

// applyClusterExclude drops every lens in lenses whose Name appears in
// exclude, preserving fan order among the survivors.
// A nil or empty exclude returns lenses unchanged.
// A duplicate name in exclude is harmless — the set collapses it.
// A name in exclude that matches no lens in lenses is a no-op for that name,
// logged as a warning naming fan and the unmatched name — exclude is an
// advisory, per-call directive over a config-owned fan, so a stale name is
// stale rather than wrong.
// If excluding would empty the survivors, the exclusion is dropped whole and
// lenses is returned unchanged, logged as a warning naming fan — a round
// demanding zero forks is never what an exclusion meant, and re-running the
// full fan costs tokens, never correctness.
func applyClusterExclude(fan string, lenses []Lens, exclude []string) []Lens {
	if len(exclude) == 0 {
		return lenses
	}

	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}

	matched := make(map[string]struct{}, len(exclude))
	survivors := make([]Lens, 0, len(lenses))
	for _, lens := range lenses {
		if _, excluded := excludeSet[lens.Name]; excluded {
			matched[lens.Name] = struct{}{}
			continue
		}
		survivors = append(survivors, lens)
	}

	for name := range excludeSet {
		if _, ok := matched[name]; !ok {
			logger.Warn("burler: cluster exclude named a lens absent from the resolved fan", "fan", fan, "lens", name)
		}
	}

	if len(survivors) == 0 {
		logger.Warn("burler: cluster exclude would empty the fan — dropping the exclusion whole to keep the fan intact", "fan", fan)
		return lenses
	}

	return survivors
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
