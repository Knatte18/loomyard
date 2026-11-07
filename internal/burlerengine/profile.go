// profile.go defines Profile, the content contract for one burler round:
// what to review, what to judge it against, and how the round may write its fixes.
// The file also holds Profile's fail-loud validation; the shuttle interaction lives in engine.go.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// FileSet names what a review phase reads: paths and/or free-form instructions.
type FileSet struct {
	Paths        []string
	Instructions string
}

// FixScope selects the write surface and git discipline for the fix half.
type FixScope string

// The two legal FixScope values.
const (
	FixScopeOverlay FixScope = "overlay" // lym state, no git
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
	// ClusterExclude names lenses to drop from the resolved ClusterFan.
	// It is a per-call advisory filter, not config.
	ClusterExclude []string
	// ClusterExcludeHeld names the exclusions carried from earlier rounds of the generation.
	// The caller fills it with a subset of ClusterExclude.
	// It applies in place of ClusterExclude when ClusterExclude would empty the fan.
	ClusterExcludeHeld []string
	clusterLenses      []lenses.Lens
	// FixFan names the fan whose lenses a forking fixer's self-check runs; empty is the single fixer.
	// Naming a fan is what makes the fix half a twill round.
	FixFan            string
	fixLenses         []lenses.Lens
	ReviewPath        string
	FixerReportPath   string
	PriorReviews      []string
	PriorFixerReports []string
	// FocusDirective is an optional path to this round's focus directive file.
	// It is steering input the reviewer reads before forming findings,
	// and it is exempt from the prior-rounds clean-room rule.
	FocusDirective string
	// Segment is the segment both halves' sessions are colored by; the zero value keeps segmentcolor.Review.
	Segment segmentcolor.Segment
	// LanguageSkills marks a code round.
	// When true, every seat of the half loads burlerSkills followed by the engine's language skills;
	// when false, burlerSkills alone.
	// It only adds skills to a seat's load turn and changes no rubric, lens, fan or gate.
	LanguageSkills bool
	// RolePrefix leads both halves' session roles, as `<RolePrefix>-burler-review` and `<RolePrefix>-burler-fix`;
	// the zero value keeps the roles unprefixed.
	RolePrefix string
	// assistant marks the copy of a profile that the review half's assistant seat reviews under.
	// The copy's fanned cluster rules spawn through its own engine's subagents.
	assistant bool
}

// rolePrefix returns base led by the profile's RolePrefix, or base alone when there is none.
func (p *Profile) rolePrefix(base string) string {
	return ledRole(p.RolePrefix, base)
}

// ledRole returns base led by lead, or base alone when lead is empty.
func ledRole(lead, base string) string {
	if lead == "" {
		return base
	}
	return lead + "-" + base
}

// segment returns the segment the round's sessions are colored by.
func (p *Profile) segment() segmentcolor.Segment {
	if p.Segment == "" {
		return segmentcolor.Review
	}
	return p.Segment
}

// RunOpts carries run-tuning knobs for one burler round.
// Review and Fix map onto their own half's shuttleengine.Spec; every other field maps onto both halves.
// A zero value defers to the engine or config default.
type RunOpts struct {
	// Review and Fix are the reviewer's and the fixer's model choices.
	Review shuttleengine.ModelChoice
	Fix    shuttleengine.ModelChoice
	// ReviewKey names the config key the reviewer's model came from.
	// A refusal of an engine that cannot run the reviewer names that key to change.
	// The zero value stands for a reviewer whose key no caller names.
	ReviewKey shuttleengine.ModelKey
	// FixKey names the config key the fixer's model came from.
	// FixFanKey names the key or profile field the fixer fan came from.
	// A refusal of an engine that cannot fork names both.
	// Both are read only when the profile names a fixer fan.
	FixKey    shuttleengine.ModelKey
	FixFanKey string
	Timeout   time.Duration
	Round     string
	// NoteID is the caller-supplied friction-note stem for this round, unique across sites.
	// friction.NotePath supplies uniqueness across invocations of the same site.
	// An empty NoteID composes no note path for this round, as an empty friction directory does.
	NoteID string
	// Gate is the per-invocation list of mechanical validators this round's shuttle run is gated by.
	// An empty list is ungated; the Webster segment's round always passes an empty list.
	// A func value would need exclusions from the validated, path-resolved data that the round renders into prompts.
	// The gate therefore lives here, per invocation.
	Gate shuttleengine.GateSpec
	// DecisionsRouted marks a fix whose caller sends the review's decision-class findings to the run's parent.
	// Those findings are then not fixer work; false keeps every finding as fixer work.
	DecisionsRouted bool
	// Assist is the review half's assistant reviewer's model choice; a choice with no model runs no assistant.
	// AssistFan names the assistant's fan, empty for a solo assistant, and AssistKey the config key its model came from.
	Assist    shuttleengine.ModelChoice
	AssistFan string
	AssistKey shuttleengine.ModelKey
	// AssistTimeout bounds the assistant's run, already capped to end before the chair's own timeout.
	// AssistSkip, when set, keeps a named assistant from starting, its end marker naming this reason.
	AssistTimeout time.Duration
	AssistSkip    string
}

// validateReview normalizes p in place and reports a fail-loud error if the review half is not runnable.
// Checks run in fixed order; the first failure is returned.
func (p *Profile) validateReview(worktreeRoot string, cfg lenses.Config) error {
	p.Target.Paths = resolvePaths(worktreeRoot, p.Target.Paths)
	p.Fasit.Paths = resolvePaths(worktreeRoot, p.Fasit.Paths)
	p.PriorReviews = resolvePaths(worktreeRoot, p.PriorReviews)
	p.PriorFixerReports = resolvePaths(worktreeRoot, p.PriorFixerReports)
	p.FocusDirective = resolvePath(worktreeRoot, p.FocusDirective)
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
	if p.FocusDirective != "" {
		if err := requireExistingPaths("FocusDirective", []string{p.FocusDirective}); err != nil {
			return err
		}
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
	if p.ClusterFan == "" && len(p.ClusterExcludeHeld) > 0 {
		return fmt.Errorf("burler: profile.ClusterExcludeHeld is set but profile.ClusterFan is empty — there is no fan to trim")
	}
	if p.ClusterFan != "" {
		// Resolve fan now so bad names fail upfront.
		resolved, err := resolveRecordedFan(cfg, p.ClusterFan)
		if err != nil {
			return err
		}
		p.clusterLenses = applyClusterExclude(p.ClusterFan, resolved, p.ClusterExclude, p.ClusterExcludeHeld)
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

// resolveRecordedFan resolves fan against cfg and adds the way forward for a fan that no longer resolves.
// A fan reaches a half from a name recorded at spawn time.
// An edit of lenses.yaml before a later half or a resume can leave that name undefined.
func resolveRecordedFan(cfg lenses.Config, fan string) ([]lenses.Lens, error) {
	resolved, err := lenses.ResolveFan(cfg, fan)
	if err != nil {
		return nil, fmt.Errorf("%w; way forward: define the fan in lenses.yaml, then resume the run or re-run the verb", err)
	}
	return resolved, nil
}

// validateFix normalizes the fix half's path fields in place and returns an error if the fix half is not runnable.
// The resolved Target.Paths must exist, and FixScope must be legal.
// The review and fixer-report paths must be non-empty and distinct.
// A non-empty FixFan must resolve against cfg into fixLenses.
// The fixer holds no fasit or rubric, so neither is required.
func (p *Profile) validateFix(worktreeRoot string, cfg lenses.Config) error {
	p.Target.Paths = resolvePaths(worktreeRoot, p.Target.Paths)
	p.ReviewPath = resolvePath(worktreeRoot, p.ReviewPath)
	p.FixerReportPath = resolvePath(worktreeRoot, p.FixerReportPath)

	if err := requireExistingPaths("Target.Paths", p.Target.Paths); err != nil {
		return err
	}
	if p.FixScope != FixScopeOverlay && p.FixScope != FixScopeSource {
		return fmt.Errorf("burler: profile.FixScope must be %q or %q, got %q", FixScopeOverlay, FixScopeSource, p.FixScope)
	}
	if p.ReviewPath == "" {
		return fmt.Errorf("burler: profile.ReviewPath must not be empty")
	}
	if p.FixerReportPath == "" {
		return fmt.Errorf("burler: profile.FixerReportPath must not be empty")
	}
	if p.ReviewPath == p.FixerReportPath {
		return fmt.Errorf("burler: profile.ReviewPath and profile.FixerReportPath must not be the same path (got %q for both)", p.ReviewPath)
	}
	if p.FixFan != "" {
		resolved, err := resolveRecordedFan(cfg, p.FixFan)
		if err != nil {
			return err
		}
		p.fixLenses = resolved
	}
	return nil
}

// applyClusterExclude drops every lens in resolved whose Name appears in exclude.
// It keeps the survivors in fan order.
// A nil or empty exclude returns resolved unchanged.
// A name in exclude that matches no lens is a no-op for that name.
// It is logged as a warning naming fan and the unmatched name.
// Exclude is an advisory, per-call directive over a config-owned fan, so a stale name is stale rather than wrong.
// If excluding would empty the survivors, the exclusions held from earlier rounds are applied instead,
// and the dropped names of exclude are logged as a warning naming fan.
// If held would empty the fan too, resolved is returned unchanged and logged as a warning naming fan.
// A round demanding zero forks is never what an exclusion meant.
// Running more lenses costs tokens, never correctness.
func applyClusterExclude(fan string, resolved []lenses.Lens, exclude, held []string) []lenses.Lens {
	if len(exclude) == 0 {
		return resolved
	}

	survivors, matched := dropLenses(resolved, exclude)
	for _, name := range exclude {
		if _, ok := matched[name]; !ok {
			logger.Warn("burler: cluster exclude named a lens absent from the resolved fan", "fan", fan, "lens", name)
		}
	}
	if len(survivors) > 0 {
		return survivors
	}

	logger.Warn("burler: cluster exclude would empty the fan — falling back to the held exclusions", "fan", fan, "dropped", exclude)
	survivors, _ = dropLenses(resolved, held)
	if len(survivors) > 0 {
		return survivors
	}

	logger.Warn("burler: held exclusions would empty the fan too — keeping the fan intact", "fan", fan)
	return resolved
}

// dropLenses returns the lenses whose Name is not in names, in order, and the set of names that matched a lens.
func dropLenses(resolved []lenses.Lens, names []string) ([]lenses.Lens, map[string]struct{}) {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	matched := make(map[string]struct{}, len(names))
	survivors := make([]lenses.Lens, 0, len(resolved))
	for _, lens := range resolved {
		if _, dropped := nameSet[lens.Name]; dropped {
			matched[lens.Name] = struct{}{}
			continue
		}
		survivors = append(survivors, lens)
	}
	return survivors, matched
}

// resolvePath resolves path against worktreeRoot.
// An already-absolute path is returned verbatim, and a relative path is joined onto worktreeRoot and cleaned.
// An empty path resolves to empty, so the caller checks for emptiness on the result.
func resolvePath(worktreeRoot, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(worktreeRoot, path))
}

// resolvePaths resolves every entry of paths against worktreeRoot.
// It returns a new slice, so the caller's backing array is never aliased.
func resolvePaths(worktreeRoot string, paths []string) []string {
	resolved := make([]string, len(paths))
	for i, p := range paths {
		resolved[i] = resolvePath(worktreeRoot, p)
	}
	return resolved
}

// requireExistingPaths returns an error if any path in paths does not exist on disk.
// The error names fieldName and the first missing entry.
// A file or a directory both satisfy the check.
func requireExistingPaths(fieldName string, paths []string) error {
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("burler: profile.%s entry %q does not exist: %w", fieldName, p, err)
		}
	}
	return nil
}
