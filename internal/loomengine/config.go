// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which uses internal/configengine.Load with ConfigTemplate() to strictly validate and resolve loom's config file,
// then validates the discussion, plan, judge, friction, and driver role model-specs and every entry of the review and fix model-spec lists' grammar via modelspec.Parse, plus every entry of the six per-segment lists (discussion_review, discussion_fix, plan_review, plan_fix, webster_review, webster_fix) that are set, an empty value meaning the run-wide list, rejects a negative value on each of the four timeout knobs, and rejects a parent_review_wait_min, review_circling_checkpoint or review_max_bounces below 1, and rejects a fix_start that is neither parallel nor after-review,
// and every entry of fan_review, and resolves each non-empty discussion_fan and plan_fan through burlerengine.ResolveFan,
// so a mistake in any of those validated keys fails loud at load time rather than hours into a run when the discussion, plan, review, judge, friction, or driver producer first spawns.
// discussion_fan and plan_fan each name a fan from burler.yaml and turn the lens fan on for Discussion-Review and Plan-Review; empty, the default, runs that segment solo.
// fan_review is the reviewer model-spec list of a fanned segment, and the forks' model too, since forks run on the reviewer session's model.
// There is no webster_fan key: Webster-Review always runs solo.
// friction and driver are the two role keys validated only when non-empty: a present-but-empty
// value means, respectively, Tier 2 self-reporting is off or the engine default model runs the
// driver, and both must load cleanly, unlike the other role keys, which are always required.

package loomengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// discussionDirName is the relative-path segment loomengine joins onto
// lyxdirs.LyxDirName to form the discussion phase's output directory.
// loomengine is this segment's sole declarer.
const discussionDirName = "discussion"

// landingDirName is the relative-path segment loomengine joins onto lyxdirs.LyxDirName to form the
// directory holding the landing change description.
// loomengine is this segment's sole declarer.
const landingDirName = "landing"

// loomApprovalFileName is the filename of the operator approval record within LoomScratchDir.
// loomengine is this segment's sole declarer.
const loomApprovalFileName = "approval.json"

// loomRejectionFileName is the filename of the pending rejection record within LoomScratchDir.
// loomengine is this segment's sole declarer.
const loomRejectionFileName = "rejection.json"

// loomReworkCoverageFileName is the filename of the rework agent's finding-to-card coverage map within LoomScratchDir.
// loomengine is this segment's sole declarer.
const loomReworkCoverageFileName = "rework-coverage.md"

// reworkDirName is the relative-path segment loomengine joins onto LoomDurableDir to form the directory holding one round-<N> directory per rejection round.
// loomengine is this segment's sole declarer.
const reworkDirName = "rework"

// loomDirName is the relative-path segment loomengine joins onto lyxdirs.LyxDirName or
// lyxdirs.DotLyxDirName to scope every loom-owned path that is not part of the shed run directory
// under its own subdirectory, distinct from the other products (e.g. Someday Hardener) that also
// configure Shed. loom's own status file and its two locks moved onto internal/shedrun's run
// directory; this segment now backs only the seven accessors that have no shedrun equivalent.
// loomengine is this segment's sole declarer.
const loomDirName = "loom"

// reviewsDirName is the relative-path segment loomengine joins onto lyxdirs.LyxDirName to form the review segments' durable run root.
// loomengine is this segment's sole declarer.
const reviewsDirName = "reviews"

// parentReviewDirName is the relative-path segment loomengine joins onto the reviews segment to form the parent-review round directories' root.
// loomengine is this segment's sole declarer.
const parentReviewDirName = "parent-review"

// frictionDirName is the relative-path segment loomengine joins onto LoomDurableDir to form the Tier 2 friction leaf's directory, and onto LoomScratchDir (with a .lock suffix) for its lock.
// loomengine is this segment's sole declarer.
const frictionDirName = "friction"

// DiscussionDirRel returns the worktree-anchor-relative form of DiscussionDir's path: the join of
// lyxdirs.LyxDirName and discussionDirName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment
// loomengine owns.
func DiscussionDirRel() string {
	return filepath.Join(lyxdirs.LyxDirName, discussionDirName)
}

// DiscussionDir returns the path to the Discussion phase's output directory for this worktree (the
// decision-record.md/support-log.md pair).
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), DiscussionDirRel())
}

// LandingDirRel returns the worktree-anchor-relative form of LandingDir's path: the join of
// lyxdirs.LyxDirName and landingDirName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment
// loomengine owns.
func LandingDirRel() string {
	return filepath.Join(lyxdirs.LyxDirName, landingDirName)
}

// LandingDir returns the path to the directory holding the landing change description for this
// worktree; the description file itself is summaryparser.Path of this directory.
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LandingDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LandingDirRel())
}

// DiscussionDecisionRecordRel returns the worktree-anchor-relative form of DiscussionDecisionRecord's path.
// It exists so a caller building a fabric commit pathspec for the record alone never has to name a segment loomengine owns.
func DiscussionDecisionRecordRel() string {
	return filepath.Join(DiscussionDirRel(), "decision-record.md")
}

// DiscussionDecisionRecord returns the path to the distilled decision record that is the Plan
// producer's sole input from `_lyx/discussion/`.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDecisionRecord(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), DiscussionDecisionRecordRel())
}

// DiscussionSupportLog returns the path to the raw support log read by the Discussion-review gate
// only.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionSupportLog(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "support-log.md")
}

// LoomDriverLog returns the path to the detached driver's captured stdout and stderr for this
// worktree's session bootstrap.
// It is AnchorPath-anchored, living under the ephemeral tree at the mirrored subpath of the durable
// status file per the Durable-vs-Ephemeral State Invariant.
// It exists as an accessor rather than an inline path because cmd/lyx's transient guard walks
// constructors, not call sites.
func LoomDriverLog(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "driver.log")
}

// LoomBootstrapLock returns the path to the advisory lock file serialising the session bootstrap's
// probe-and-spawn sequence.
// It is AnchorPath-anchored, living under the ephemeral tree at the mirrored subpath of the durable
// status file per the Durable-vs-Ephemeral State Invariant.
// It is a third lock distinct from both LoomStatusLock's per-persist status lock and LoomRunLock's
// whole-run lock,
// and it is released before the success envelope is printed.
func LoomBootstrapLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "bootstrap.lock")
}

// LoomHandoffVoucher returns the path to the machine-local handoff voucher.
// `lyx loom step` records it after every completed step, and `lyx loom start` records it right before it spawns a driver:
// the persisted history length and state as that step or spawn left them.
// It suppresses at most one entry observation, the first, and only when its history length and state equal what was recorded.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it, and suppresses at most one later matching observation;
// a driver start spawned that then dies mid-run is not noted, and its evidence stays in the driver log and trace.
// It is AnchorPath-anchored, living under the ephemeral tree at the mirrored subpath of the durable status file per the Durable-vs-Ephemeral State Invariant, since the voucher is never tracked.
// It exists because a completed step leaves the status file byte-identical to a mid-run driver death -- state running, run lock free, history non-empty --
// so without this voucher the next run's entry observation reads as a crash-resume for a task in which nothing crashed.
// A step killed mid-producer never writes it, so a genuine step-crash still reports.
// Losing the voucher -- a fresh clone, a fabric re-wire -- costs at most one spurious crash-resume note, which is why it is machine-local rather than durable.
func LoomHandoffVoucher(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "handoff-voucher.json")
}

// LoomHandoffVoucherLock returns the path to the advisory lock file guarding concurrent access to
// LoomHandoffVoucher(l).
// It is AnchorPath-anchored under the ephemeral tree, exactly as the voucher it guards.
func LoomHandoffVoucherLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "handoff-voucher.json.lock")
}

// LoomScratchDir returns the path to loom's ephemeral scratch directory for this worktree.
// It is AnchorPath-anchored, like LoomDriverLog and LoomBootstrapLock, and names the directory
// those two already share: lyxdirs.DotLyxDirName joined with loomDirName.
// Per the Durable-vs-Ephemeral State Invariant, this accessor is the mirrored-subpath counterpart of
// the "loom" segment's durable side -- now the run directory shedrun owns, not a path this package
// declares.
func LoomScratchDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName)
}

// LoomApprovalPath returns the path to the operator approval record for this worktree.
// It is built on LoomScratchDir rather than re-joining the .lyx literal, and is ephemeral: the
// record is never tracked, per the Durable-vs-Ephemeral State Invariant.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomApprovalPath(l *lyxcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), loomApprovalFileName)
}

// LoomRejectionPath returns the path to the pending rejection record `lyx loom reject` writes for this worktree.
// It is built on LoomScratchDir rather than re-joining the .lyx literal, and is ephemeral:
// the record is never tracked, per the Durable-vs-Ephemeral State Invariant.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomRejectionPath(l *lyxcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), loomRejectionFileName)
}

// LoomReworkCoveragePath returns the path to the rework agent's completion file, which maps each finding to the new cards covering it.
// It is built on LoomScratchDir rather than re-joining the .lyx literal, and is ephemeral:
// the file is never tracked, per the Durable-vs-Ephemeral State Invariant.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReworkCoveragePath(l *lyxcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), loomReworkCoverageFileName)
}

// LoomReworkDirRel returns the worktree-anchor-relative form of LoomReworkDir's path: the join of LoomDurableDirRel and reworkDirName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment loomengine owns.
func LoomReworkDirRel() string {
	return filepath.Join(LoomDurableDirRel(), reworkDirName)
}

// LoomReworkDir returns the root holding one round-<N> directory per rejection round for this worktree.
// It is durable: each round's findings, record, coverage map and prior-generation archive are tracked run content, committed with the new plan generation in PR-Rework's round commit.
// It is built on LoomDurableDirRel rather than re-joining the durable literal a second time.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReworkDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomReworkDirRel())
}

// LoomReviewsDirRel returns the worktree-anchor-relative form of LoomReviewsDir's path: the join of lyxdirs.LyxDirName and reviewsDirName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment loomengine owns.
func LoomReviewsDirRel() string {
	return filepath.Join(lyxdirs.LyxDirName, reviewsDirName)
}

// LoomReviewsDir returns the path to the root every review segment's `run_subdir` resolves
// against for this worktree -- the value shedrecipe.Env.RunRoot takes.
// It is durable: everything the round producer and the Bouncer write here -- reports, verdicts, ledgers, focus files, and their timestamped archive siblings -- is tracked content committed by loom's per-transition status commit.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReviewsDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomReviewsDirRel())
}

// LoomParentReviewDirRel returns the worktree-anchor-relative form of LoomParentReviewDir's path: LoomReviewsDirRel joined with parentReviewDirName.
// It sits beside the Discussion-Review run directory under the review run root, never under _lyx/discussion/.
func LoomParentReviewDirRel() string {
	return filepath.Join(LoomReviewsDirRel(), parentReviewDirName)
}

// LoomParentReviewDir returns the root of the parent-review round directories (`round-<N>/`) for this worktree.
// It is durable: the requests, briefs, deliveries, verdicts and reviews under it are tracked content committed with the discussion.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomParentReviewDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomParentReviewDirRel())
}

// LoomParentReviewLockDir returns the ephemeral mirror of LoomParentReviewDir under .lyx, where the parent-review store keeps its lock files.
// It is never tracked, per the Durable-vs-Ephemeral State Invariant.
func LoomParentReviewLockDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, reviewsDirName, parentReviewDirName)
}

// LoomDurableDirRel returns the worktree-anchor-relative form of LoomDurableDir's path: the join of lyxdirs.LyxDirName and loomDirName.
// Everything under this directory is tracked run content committed by loom's per-transition status commit.
func LoomDurableDirRel() string {
	return filepath.Join(lyxdirs.LyxDirName, loomDirName)
}

// LoomDurableDir returns loom's durable run directory for this worktree: LoomDurableDirRel joined onto l.AnchorPath().
// It is the durable counterpart of LoomScratchDir.
// Everything under this directory is tracked run content committed by loom's per-transition status commit.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomDurableDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomDurableDirRel())
}

// LoomFrictionDir returns the path to the Tier 2 friction leaf's directory for this worktree: the per-agent friction notes an unsupervised run's producers append to.
// The notes are run records, so they live under the durable tree and are committed with the run.
// It is built on LoomDurableDir rather than re-joining l.AnchorPath() and the durable literal a second time.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomFrictionDir(l *lyxcwd.Location) string {
	return filepath.Join(LoomDurableDir(l), frictionDirName)
}

// LoomFrictionArchivePrefix returns the absolute path prefix a timestamped archive sibling of
// LoomFrictionDir is composed from: internal/frictionengine appends its own compact timestamp to
// this prefix and moves the covered friction files into the directory that results.
// It exists as a second accessor, rather than being derived by the caller from LoomFrictionDir,
// so internal/frictionengine stays told rather than deriving, and the "friction" segment is still
// named in exactly one place.
func LoomFrictionArchivePrefix(l *lyxcwd.Location) string {
	return LoomFrictionDir(l) + "-"
}

// LoomFrictionLock returns the path to the advisory lock guarding the Tier 2 reflection step against
// a second concurrent reflection over the same friction directory.
//
// On a finished run the reflection runs inside the terminal Friction-Reflect row with the run lock
// held; on a blocked halt it runs after shedengine.Run returns, with the run lock free. It is a lock
// of its own rather than a reuse of the run lock because the blocked-halt reflection is not covered
// by the run lock at all: shedengine.Run releases it on return, so for the whole of that reflection
// agent's life the run lock reads as free and a second `lyx loom start` spawns a second driver.
// That second driver is legitimate -- it is an operator resuming a halted run -- but its own
// reflection (the row's) would then archive covered files out from under the first one's
// agent, and both would have declared the same reflection-report.md as an output.
// The lock file is not inside the friction directory, which holds only reflection inputs and outputs.
// It sits at the mirrored ephemeral subpath of the durable friction directory, under LoomScratchDir.
func LoomFrictionLock(l *lyxcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), frictionDirName+".lock")
}

// Config represents the resolved loom.yaml configuration: role model-specs and timeout knobs.
type Config struct {
	Discussion            string        `yaml:"discussion"`
	DiscussionTimeoutMin  int           `yaml:"discussion_timeout_min"`
	DiscussionInteractive bool          `yaml:"discussion_interactive"`
	Plan                  string        `yaml:"plan"`
	PlanTimeoutMin        int           `yaml:"plan_timeout_min"`
	Review                ModelSpecList `yaml:"review"`
	Fix                   ModelSpecList `yaml:"fix"`
	DiscussionReview      ModelSpecList `yaml:"discussion_review"`
	DiscussionFix         ModelSpecList `yaml:"discussion_fix"`
	PlanReview            ModelSpecList `yaml:"plan_review"`
	PlanFix               ModelSpecList `yaml:"plan_fix"`
	WebsterReview         ModelSpecList `yaml:"webster_review"`
	WebsterFix            ModelSpecList `yaml:"webster_fix"`
	DiscussionFan         string        `yaml:"discussion_fan"`
	PlanFan               string        `yaml:"plan_fan"`
	FanReview             ModelSpecList `yaml:"fan_review"`
	Judge               string        `yaml:"judge"`
	ReviewTimeoutMin      int           `yaml:"review_timeout_min"`
	Friction              string        `yaml:"friction"`
	FrictionTimeoutMin    int           `yaml:"friction_timeout_min"`
	Driver                string        `yaml:"driver"`
	ParentReviewWaitMin   int           `yaml:"parent_review_wait_min"`

	ReviewCirclingCheckpoint int `yaml:"review_circling_checkpoint"`
	ReviewMaxBounces         int `yaml:"review_max_bounces"`

	FixStart string `yaml:"fix_start"`
}

// ModelSpecList is a model-spec key's value: one model-spec for every round, or a list of model-specs, one per round, whose last entry serves every later round.
// A YAML scalar loads as a list of one.
type ModelSpecList []string

// modelSpecListWayForward is the way forward every model-spec list refusal ends with.
const modelSpecListWayForward = "set the key to a model-spec, or to a non-empty list of model-specs"

// modelSpecShapeError reports a model-spec list key whose value is neither a scalar nor a sequence of scalars.
// line is the value's line in the file, which LoadConfig uses to name the key.
type modelSpecShapeError struct {
	line int
}

func (e *modelSpecShapeError) Error() string {
	return fmt.Sprintf("value at line %d is neither a model-spec nor a list of model-specs; %s", e.line, modelSpecListWayForward)
}

// UnmarshalYAML accepts a scalar or a block sequence of scalars and rejects any other shape.
func (l *ModelSpecList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*l = ModelSpecList{node.Value}
		return nil
	case yaml.SequenceNode:
		specs := make(ModelSpecList, 0, len(node.Content))
		for _, entry := range node.Content {
			if entry.Kind != yaml.ScalarNode {
				return &modelSpecShapeError{line: entry.Line}
			}
			specs = append(specs, entry.Value)
		}
		*l = specs
		return nil
	default:
		return &modelSpecShapeError{line: node.Line}
	}
}

// ConfigOpenMaps returns the loom.yaml keys whose value is a scalar or a per-round list, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{"review", "fix", "discussion_review", "discussion_fix", "plan_review", "plan_fix", "webster_review", "webster_fix", "fan_review"}
}

// segmentModelList is one per-segment reviewer or fixer model-spec list with the loom.yaml key it came from.
type segmentModelList struct {
	key   string
	specs ModelSpecList
}

// segmentModelKeys returns the per-segment reviewer and fixer model-spec lists of cfg, each optional.
func segmentModelKeys(cfg Config) []segmentModelList {
	return []segmentModelList{
		{"discussion_review", cfg.DiscussionReview},
		{"discussion_fix", cfg.DiscussionFix},
		{"plan_review", cfg.PlanReview},
		{"plan_fix", cfg.PlanFix},
		{"webster_review", cfg.WebsterReview},
		{"webster_fix", cfg.WebsterFix},
	}
}

// isUnsetModelSpecList reports whether specs is empty or a single empty string, which a per-segment key reads as "take the run-wide list".
func isUnsetModelSpecList(specs ModelSpecList) bool {
	return len(specs) == 0 || (len(specs) == 1 && specs[0] == "")
}

// keyAtLine returns the top-level key of contents whose entry spans line, or "" when none does.
// An entry spans from its key's line up to the next key's line.
func keyAtLine(contents []byte, line int) string {
	var root yaml.Node
	if err := yaml.Unmarshal(contents, &root); err != nil || len(root.Content) == 0 {
		return ""
	}
	keys := root.Content[0].Content
	found := ""
	for i := 0; i < len(keys); i += 2 {
		if keys[i].Line <= line {
			found = keys[i].Value
		}
	}
	return found
}

// validateModelSpecList rejects an empty list and an entry that is not a model-spec, naming key and the 1-based entry index.
func validateModelSpecList(key string, specs ModelSpecList) error {
	if len(specs) == 0 {
		return fmt.Errorf("loom config key %q: the list is empty; %s", key, modelSpecListWayForward)
	}
	for i, spec := range specs {
		if _, err := modelspec.Parse(spec); err != nil {
			return fmt.Errorf("loom config key %q entry %d: %w; %s", key, i+1, err, modelSpecListWayForward)
		}
	}
	return nil
}

// validateFanKeys resolves each non-empty fan key of cfg through burler.yaml, which falls back per name to the embedded template.
// A fan that does not resolve is refused naming the key, the unknown name and the fans that exist.
func validateFanKeys(baseDir string, cfg Config) error {
	burlerCfg, err := burlerengine.LoadConfig(baseDir)
	if err != nil {
		return err
	}
	for _, fan := range []struct {
		key  string
		name string
	}{
		{"discussion_fan", cfg.DiscussionFan},
		{"plan_fan", cfg.PlanFan},
	} {
		if fan.name == "" {
			continue
		}
		if _, err := burlerengine.ResolveFan(burlerCfg, fan.name); err != nil {
			return fmt.Errorf("loom config key %q: %w; set the key to one of those fans, or to empty to run the segment solo", fan.key, err)
		}
	}
	return nil
}

// LoadConfig loads and unmarshals configuration for the loom module.
// It validates model-spec grammar at load time.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()), ConfigOpenMaps()...)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		var shape *modelSpecShapeError
		if errors.As(err, &shape) {
			return Config{}, fmt.Errorf("loom config key %q: %w", keyAtLine(resolved, shape.line), err)
		}
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Discussion); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "discussion", err)
	}

	if _, err := modelspec.Parse(cfg.Plan); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "plan", err)
	}

	if err := validateModelSpecList("review", cfg.Review); err != nil {
		return Config{}, err
	}

	if err := validateModelSpecList("fix", cfg.Fix); err != nil {
		return Config{}, err
	}

	for _, segment := range segmentModelKeys(cfg) {
		if isUnsetModelSpecList(segment.specs) {
			continue
		}
		if err := validateModelSpecList(segment.key, segment.specs); err != nil {
			return Config{}, err
		}
	}

	if err := validateModelSpecList("fan_review", cfg.FanReview); err != nil {
		return Config{}, err
	}

	if err := validateFanKeys(baseDir, cfg); err != nil {
		return Config{}, err
	}

	if _, err := modelspec.Parse(cfg.Judge); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "judge", err)
	}

	// friction is validated only when non-empty: a present-but-empty value means Tier 2 is off and
	// must load cleanly, unlike the four role keys above, which are always required.
	if cfg.Friction != "" {
		if _, err := modelspec.Parse(cfg.Friction); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "friction", err)
		}
	}

	// driver is validated only when non-empty, exactly like friction above: an empty value means
	// "defer to the engine default" -- the same meaning shuttleengine.Spec.Model's empty value
	// already carries -- and must load cleanly rather than being forced to name a literal alias.
	if cfg.Driver != "" {
		if _, err := modelspec.Parse(cfg.Driver); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "driver", err)
		}
	}

	// The four timeouts are checked here for the same reason the four model-specs above are: a
	// value that can only be a mistake should fail at load time rather than hours into a run when
	// the producer it governs first spawns. A negative minute count flows into
	// time.Duration(n) * time.Minute on a shuttleengine.Spec, where it is caught only at spawn or
	// resume time -- exactly the deferred failure this function's own header says it exists to
	// prevent, for the keys sitting immediately beside the ones it already guards.
	// Zero is deliberately accepted rather than rejected: Spec.Timeout treats 0 as "defer to
	// shuttle's own run_timeout_min", which is a legitimate configuration.
	for _, knob := range []struct {
		key     string
		minutes int
	}{
		{"discussion_timeout_min", cfg.DiscussionTimeoutMin},
		{"plan_timeout_min", cfg.PlanTimeoutMin},
		{"review_timeout_min", cfg.ReviewTimeoutMin},
		{"friction_timeout_min", cfg.FrictionTimeoutMin},
	} {
		if knob.minutes < 0 {
			return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; use 0 to defer to shuttle's run_timeout_min", knob.key, knob.minutes)
		}
	}

	// parent_review_wait_min has no "0 defers" meaning:
	// a 0 bound would still open a request and notify the parent of a request that expires at its next evaluation,
	// so the one off switch is the gate entry's own attempts, not this key.
	if cfg.ParentReviewWaitMin < 1 {
		return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; set attempts: 0 on Discussion-Write's parent-review gate entry to turn the review off", "parent_review_wait_min", cfg.ParentReviewWaitMin)
	}

	// A checkpoint above the budget is accepted: such a run never rules CIRCLING and reaches the budget escalation instead.
	for _, knob := range []struct {
		key   string
		value int
	}{
		{"review_circling_checkpoint", cfg.ReviewCirclingCheckpoint},
		{"review_max_bounces", cfg.ReviewMaxBounces},
	} {
		if knob.value < 1 {
			return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", knob.key, knob.value)
		}
	}

	switch burlerengine.FixStart(cfg.FixStart) {
	case burlerengine.FixStartParallel, burlerengine.FixStartAfterReview:
	default:
		return Config{}, fmt.Errorf("loom config key %q: unknown value %q; set it to %q or %q", "fix_start", cfg.FixStart, burlerengine.FixStartParallel, burlerengine.FixStartAfterReview)
	}

	return cfg, nil
}
