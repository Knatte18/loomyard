// config.go loads loom's configuration: the Config type mirrors the keys of loom.yaml.
// LoadConfig validates and resolves those keys.
// A bad value fails at load time, not hours into a run when its producer first spawns.
// The plan perch's keys live in tapestry.yaml, which LoadConfig never reads.
// Config.WithTapestry copies them in for the callers that serve the Plan-Review segment.

package loomengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"gopkg.in/yaml.v3"
)

// stepIdleTimeoutFloorMin is the smallest step_idle_timeout_min value, in minutes.
// A lower configured value is raised to it with one warning.
const stepIdleTimeoutFloorMin = 1

// discussionDirName is the relative-path segment loomengine joins onto lymdirs.LymDirName.
// The joined path is the discussion phase's output directory.
// loomengine is this segment's sole declarer.
const discussionDirName = "discussion"

// landingDirName is the relative-path segment loomengine joins onto lymdirs.LymDirName.
// The joined path is the directory holding the landing change description.
// loomengine is this segment's sole declarer.
const landingDirName = "landing"

// projectDirName is the relative-path segment loomengine joins onto lymdirs.LymDirName.
// The joined path is the directory of every beam's project record, with one subdirectory per beam slug.
// loomengine is this segment's sole declarer.
const projectDirName = "project"

// reworkDirName is the relative-path segment loomengine joins onto LoomDurableDir.
// The joined path is the directory holding one round-<N> directory per rejection round.
// loomengine is this segment's sole declarer.
const reworkDirName = "rework"

// loomDirName is the relative-path segment loomengine joins onto lymdirs.LymDirName or lymdirs.DotLymDirName.
// The joined path scopes every loom-owned path outside the shed run directory under its own subdirectory.
// That subdirectory keeps loom's paths distinct from those of the other products that also configure Shed.
// The loom status file and its two locks live in internal/shedrun's run directory, not under this segment.
// loomengine is this segment's sole declarer.
const loomDirName = "loom"

// reviewsDirName is the relative-path segment loomengine joins onto lymdirs.LymDirName.
// The joined path is the durable run root of the review segments.
// loomengine is this segment's sole declarer.
const reviewsDirName = "reviews"

// amendmentRecordFileName is the file name loomengine joins onto LoomDurableDir.
// The joined path is the plan amendment record.
// loomengine is this segment's sole declarer.
const amendmentRecordFileName = "amendment.yaml"

// frictionDirName is the relative-path segment loomengine joins onto LoomDurableDir.
// The joined path is the Tier 2 friction leaf's directory.
// Joined onto LoomScratchDir with a .lock suffix, the same segment names the lock.
// loomengine is this segment's sole declarer.
const frictionDirName = "friction"

// DiscussionDirRel returns the discussion output directory relative to the worktree anchor.
// It is the join of lymdirs.LymDirName and discussionDirName.
// Callers building a fabric commit pathspec use it so they never name a directory segment loomengine owns.
func DiscussionDirRel() string {
	return filepath.Join(lymdirs.LymDirName, discussionDirName)
}

// DiscussionDir returns the path to the Discussion phase's output directory for this worktree.
// The directory holds the decision-record.md and support-log.md pair.
// The path is anchored at the worktree anchor of l.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), DiscussionDirRel())
}

// LandingDirRel returns the landing directory relative to the worktree anchor.
// It is the join of lymdirs.LymDirName and landingDirName.
// Callers building a fabric commit pathspec use it so they never name a directory segment loomengine owns.
func LandingDirRel() string {
	return filepath.Join(lymdirs.LymDirName, landingDirName)
}

// LandingDir returns the path to the directory holding the landing change description for this worktree.
// The description file inside it is named by the summaryparser package.
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LandingDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LandingDirRel())
}

// beamReadyFileName is the file name of a beam's ready record within the landing directory.
// loomengine is the only package that declares it.
const beamReadyFileName = "beam-ready.json"

// BeamReadyPath returns the path of the ready record of the beam whose pair is at l.
// The record sits beside the landing change description.
// It is AnchorPath-anchored.
func BeamReadyPath(l *lymcwd.Location) string {
	return filepath.Join(LandingDir(l), beamReadyFileName)
}

// BeamReadyLockPath returns the lock path that guards the ready record of the beam at l.
// It lies at the mirrored, never-tracked landing path under the dot-lym directory.
func BeamReadyLockPath(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, landingDirName, beamReadyFileName+".lock")
}

// beamLandingFileName is the file name of a beam's landing record within the landing directory.
// loomengine is the only package that declares it.
const beamLandingFileName = "beam-landing.json"

// BeamLandingPath returns the path of the landing record of the beam whose pair is at l.
// The record sits beside the ready record.
// It is AnchorPath-anchored.
func BeamLandingPath(l *lymcwd.Location) string {
	return filepath.Join(LandingDir(l), beamLandingFileName)
}

// BeamLandingLockPath returns the lock path that guards the landing record of the beam at l.
// It lies under the checkout's never-tracked lock directory.
func BeamLandingLockPath(l *lymcwd.Location) string {
	return lock.TrackedFileLockPath(BeamLandingPath(l), ".lock")
}

// DiscussionDecisionRecordRel returns the worktree-anchor-relative path of the decision record.
// A caller building a fabric commit pathspec for the record alone never has to name a segment loomengine owns.
func DiscussionDecisionRecordRel() string {
	return filepath.Join(DiscussionDirRel(), "decision-record.md")
}

// DiscussionDecisionRecord returns the path to the distilled decision record.
// It is the Plan producer's design input from `_lym/discussion/`.
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDecisionRecord(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), DiscussionDecisionRecordRel())
}

// DiscussionSupportLog returns the path to the raw support log, which only the Discussion-review gate reads.
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionSupportLog(l *lymcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "support-log.md")
}

// DiscussionRelevant returns the path to the Discussion-Write lead's list of the code it read.
// The list is the plan writer's head start.
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionRelevant(l *lymcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "relevant.md")
}

// ProjectDirRel returns the worktree-anchor-relative path of the project directory for the beam slug.
// It holds the project record and its support log.
// The slug keeps a beam's record apart from its members' own records and from an enclosing beam's;
// a pair forked from a beam inherits every record the beam carries.
func ProjectDirRel(slug string) string {
	return filepath.Join(lymdirs.LymDirName, projectDirName, slug)
}

// ProjectDir returns the path of the directory holding the project record of the beam slug, in the worktree at l.
// It is AnchorPath-anchored.
func ProjectDir(l *lymcwd.Location, slug string) string {
	return filepath.Join(l.AnchorPath(), ProjectDirRel(slug))
}

// ProjectDecisionRecordRel returns the worktree-anchor-relative path of the project record of the beam slug.
func ProjectDecisionRecordRel(slug string) string {
	return filepath.Join(ProjectDirRel(slug), "decision-record.md")
}

// ProjectDecisionRecord returns the path of the project record of the beam slug, in the worktree at l.
// The decision record binds every member of the beam.
// It is AnchorPath-anchored.
func ProjectDecisionRecord(l *lymcwd.Location, slug string) string {
	return filepath.Join(l.AnchorPath(), ProjectDecisionRecordRel(slug))
}

// ProjectSupportLog returns the path of the support log beside the project record of the beam slug, in the worktree at l.
// It is AnchorPath-anchored.
func ProjectSupportLog(l *lymcwd.Location, slug string) string {
	return filepath.Join(ProjectDir(l, slug), "support-log.md")
}

// LoomDriverLog returns the path to the file capturing the stdout and stderr of the detached driver.
// It holds the output of the driver for this worktree's session bootstrap.
// It is AnchorPath-anchored; it lives under the ephemeral tree at the mirrored subpath of the durable status file.
// It follows the Durable-vs-Ephemeral State Invariant.
// It exists as an accessor rather than an inline path because cmd/lym's transient guard walks
// constructors, not call sites.
func LoomDriverLog(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, loomDirName, "driver.log")
}

// LoomBootstrapLock returns the path to the advisory lock file serialising the bootstrap's probe-and-spawn sequence.
// It is AnchorPath-anchored; it lives under the ephemeral tree at the mirrored subpath of the durable status file.
// It follows the Durable-vs-Ephemeral State Invariant.
// It is a third lock, distinct from the per-persist status lock and from the whole-run lock.
// It is released before the success envelope is printed.
func LoomBootstrapLock(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, loomDirName, "bootstrap.lock")
}

// LoomHandoffVoucher returns the path to the machine-local handoff voucher.
// `lym loom step` records it after every completed step.
// `lym loom start` records it right before it spawns a driver.
// It holds the persisted history length and state as that step or spawn left them.
// It suppresses at most one entry observation, the first.
// It does so only when the voucher's history length and state equal what was recorded.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it.
// Until then, it suppresses at most one later matching observation.
// A driver start that is spawned and then dies mid-run is not noted,
// and its evidence stays in the driver log and trace.
// It is AnchorPath-anchored; it lives under the ephemeral tree at the mirrored subpath of the durable status file.
// It is ephemeral because the voucher is never tracked, per the Durable-vs-Ephemeral State Invariant.
// It exists because a completed step leaves the status file byte-identical to a driver that died mid-run:
// state running, run lock free, history non-empty.
// Without this voucher, the next run's entry observation reads as a crash-resume for a task in which nothing crashed.
// A step killed mid-producer never writes it, so a genuine step-crash still reports.
// Losing the voucher, as a fresh clone or a fabric re-wire does, costs at most one spurious crash-resume note.
// That cost is why it is machine-local rather than durable.
func LoomHandoffVoucher(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, loomDirName, "handoff-voucher.json")
}

// LoomHandoffVoucherLock returns the path to the advisory lock that guards the handoff voucher.
// It is AnchorPath-anchored under the ephemeral tree, like the voucher it guards.
func LoomHandoffVoucherLock(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, loomDirName, "handoff-voucher.json.lock")
}

// LoomScratchDir returns the path to loom's ephemeral scratch directory for this worktree.
// It is AnchorPath-anchored, under lymdirs.DotLymDirName joined with loomDirName.
// It is the mirrored-subpath counterpart of the durable "loom" segment, per the Durable-vs-Ephemeral State Invariant.
// The directory is the run directory that shedrun owns.
func LoomScratchDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, loomDirName)
}

// LoomReworkDirRel returns the worktree-anchor-relative path of the rework directory.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment loomengine owns.
func LoomReworkDirRel() string {
	return filepath.Join(LoomDurableDirRel(), reworkDirName)
}

// LoomReworkDir returns the root holding one round-<N> directory per rejection round for this worktree.
// It is durable: each round's findings, record, coverage map and prior-generation archive are tracked run content.
// The round that wrote them commits them with the new plan generation.
// It sits under the durable loom directory.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReworkDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomReworkDirRel())
}

// LoomReviewsDirRel returns the path of the reviews directory relative to the worktree anchor:
// the join of lymdirs.LymDirName and reviewsDirName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment loomengine owns.
func LoomReviewsDirRel() string {
	return filepath.Join(lymdirs.LymDirName, reviewsDirName)
}

// LoomReviewsDir returns the path to the root every review segment's `run_subdir` resolves against for this worktree.
// It is the value that shedrecipe.Env.RunRoot takes.
// It is durable: the files written under it are tracked content committed by loom's per-transition status commit.
// Its timestamped archive siblings are committed the same way.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReviewsDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomReviewsDirRel())
}

// LoomDurableDirRel returns the path of loom's durable run directory relative to the worktree anchor:
// the join of lymdirs.LymDirName and loomDirName.
// Everything under this directory is tracked run content committed by loom's per-transition status commit.
func LoomDurableDirRel() string {
	return filepath.Join(lymdirs.LymDirName, loomDirName)
}

// LoomDurableDir returns loom's durable run directory for this worktree, located under the worktree anchor.
// Everything under this directory is tracked run content committed by loom's per-transition status commit.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomDurableDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomDurableDirRel())
}

// LoomAmendmentRecordRel returns the path of the plan amendment record relative to the worktree anchor:
// the durable directory's relative path joined with amendmentRecordFileName.
// It exists so a caller building a fabric commit pathspec never has to name a segment loomengine owns.
func LoomAmendmentRecordRel() string {
	return filepath.Join(LoomDurableDirRel(), amendmentRecordFileName)
}

// LoomAmendmentRecord returns the path of the plan amendment record for this worktree.
// It is durable: the run's routing reads it, so it is tracked content that survives adoption on another machine.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomAmendmentRecord(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomAmendmentRecordRel())
}

// PathsOutsideRunRecords returns the absolute paths outside the run's own records directories, in input order.
// The records directories are the run's directory, the loom durable directory and the reviews directory:
// the ones the status commit stages.
// A path under one of them is run state a hand-over leaves to the next driver;
// a path that only shares a directory's name as a prefix is outside it.
func PathsOutsideRunRecords(l *lymcwd.Location, runID string, paths []string) []string {
	records := []string{shedrun.RunDir(l, runID), LoomDurableDir(l), LoomReviewsDir(l)}
	var outside []string
	for _, path := range paths {
		inside := false
		for _, dir := range records {
			if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
				inside = true
				break
			}
		}
		if !inside {
			outside = append(outside, path)
		}
	}
	return outside
}

// LoomFrictionDir returns the path to the Tier 2 friction leaf's directory for this worktree:
// the per-agent friction notes an unsupervised run's producers append to.
// The notes are run records, so they live under the durable tree and are committed with the run.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomFrictionDir(l *lymcwd.Location) string {
	return filepath.Join(LoomDurableDir(l), frictionDirName)
}

// LoomFrictionArchivePrefix returns the absolute path prefix from which the friction archive names are built.
// internal/frictionengine appends its own compact timestamp to this prefix;
// it then moves the covered friction files into the directory that results.
// The accessor exists so internal/frictionengine is told the prefix rather than deriving it;
// the "friction" segment is still named in one place.
func LoomFrictionArchivePrefix(l *lymcwd.Location) string {
	return LoomFrictionDir(l) + "-"
}

// LoomFrictionDirRel returns the path of the friction directory relative to the worktree anchor:
// the join of the durable directory's relative path and the friction segment.
// A reader of a git tree applies filepath.ToSlash to it.
func LoomFrictionDirRel() string {
	return filepath.Join(LoomDurableDirRel(), frictionDirName)
}

// LoomFrictionArchivesRelPattern returns a git pathspec glob, relative to the worktree anchor:
// it matches every timestamped reflection-archive sibling of the friction directory and everything beneath it.
// It suits a pathspec only, never a file path, since git's glob lets its `*` match across `/`.
func LoomFrictionArchivesRelPattern() string {
	return LoomFrictionDirRel() + "-*"
}

// IsLoomFrictionArchive reports whether name is a timestamped reflection-archive sibling of the friction directory.
// The name is a directory name directly under the durable directory.
// A name matches when it is the friction segment followed by "-" and at least one more character;
// the friction directory itself is not an archive.
func IsLoomFrictionArchive(name string) bool {
	rest, ok := strings.CutPrefix(name, frictionDirName+"-")
	return ok && rest != ""
}

// landingCheckpointSubjectPrefix is the commit subject prefix of the parent pair's run-records checkpoint for a landing.
const landingCheckpointSubjectPrefix = "loom: run records checkpoint for landing "

// LandingCheckpointSubject returns the commit subject of the parent pair's run-records checkpoint for the landing run slug.
func LandingCheckpointSubject(slug string) string {
	return landingCheckpointSubjectPrefix + slug
}

// LandingCheckpointSlug reports the landing run slug that a checkpoint commit subject encodes.
// It returns false when subject lacks the checkpoint prefix or has an empty remainder.
func LandingCheckpointSlug(subject string) (string, bool) {
	slug, ok := strings.CutPrefix(subject, landingCheckpointSubjectPrefix)
	if !ok || slug == "" {
		return "", false
	}
	return slug, true
}

// LoomFrictionLock returns the path to the advisory lock against concurrent reflections over one friction directory.
//
// The reflection runs only inside the terminal Friction-Reflect row,
// so the lock orders the rows' reflections of two drivers over one friction directory.
// An operator can resume a halted task and reach a new driver's row while an earlier driver's row still reflects,
// and the second reflection would archive covered files out from under the first one's agent.
// Both would also have declared the same reflection-report.md as an output.
// The lock file is not inside the friction directory, which holds only reflection inputs and outputs.
// It sits at the ephemeral subpath that mirrors the durable friction directory, under the scratch directory.
func LoomFrictionLock(l *lymcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), frictionDirName+".lock")
}

// Config represents the resolved loom.yaml configuration: role model-specs and timeout knobs.
// Its Plan-prefixed fields hold the plan perch's values, which tapestry.yaml owns;
// they stay at their zero value until tapestry.yaml is applied to the config.
type Config struct {
	Discussion            string `yaml:"discussion"`
	DiscussionTimeoutMin  int    `yaml:"discussion_timeout_min"`
	DiscussionInteractive bool   `yaml:"discussion_interactive"`
	Plan                  string `yaml:"-"`
	PlanTimeoutMin        int    `yaml:"-"`

	// Raddle is the model-spec of the raddle agent, which proposes how a Sketch splits into tasks.
	// RaddleAutoApprove makes a raddle run file its proposal once the proposal's gate passes, with no operator approval.
	// A run whose recipe rows hold a split perch never reads it, since the perch's approval files the proposal.
	Raddle            string `yaml:"raddle"`
	RaddleAutoApprove bool   `yaml:"raddle_auto_approve"`

	// ReviewerModel and FixerModel are the shared reviewer and fixer model lists;
	// every review segment falls back to them when its own key is empty.
	// Each segment's own pair replaces them for that segment when set.
	ReviewerModel           burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel              burlerengine.ModelSpecList `yaml:"fixer_model"`
	DiscussionReviewerModel burlerengine.ModelSpecList `yaml:"discussion_reviewer_model"`
	DiscussionFixerModel    burlerengine.ModelSpecList `yaml:"discussion_fixer_model"`
	PlanReviewerModel       burlerengine.ModelSpecList `yaml:"-"`
	PlanFixerModel          burlerengine.ModelSpecList `yaml:"-"`
	CodeReviewerModel       burlerengine.ModelSpecList `yaml:"code_reviewer_model"`
	CodeFixerModel          burlerengine.ModelSpecList `yaml:"code_fixer_model"`
	// SplitReviewerModel and SplitFixerModel are the split perch's own lists.
	// An empty SplitFixerModel takes the raddle key's model, so the split is fixed by a model of the raddle agent's strength.
	SplitReviewerModel burlerengine.ModelSpecList `yaml:"split_reviewer_model"`
	SplitFixerModel    burlerengine.ModelSpecList `yaml:"split_fixer_model"`

	// Each review segment's per-round reviewer and fixer fan lists.
	DiscussionReviewerFan lenses.FanList `yaml:"discussion_reviewer_fan"`
	PlanReviewerFan       lenses.FanList `yaml:"-"`
	CodeReviewerFan       lenses.FanList `yaml:"code_reviewer_fan"`
	SplitReviewerFan      lenses.FanList `yaml:"split_reviewer_fan"`
	DiscussionFixerFan    lenses.FanList `yaml:"discussion_fixer_fan"`
	PlanFixerFan          lenses.FanList `yaml:"-"`
	CodeFixerFan          lenses.FanList `yaml:"code_fixer_fan"`
	SplitFixerFan         lenses.FanList `yaml:"split_fixer_fan"`

	// Each review segment's assistant reviewer model list and fan list, one entry per round;
	// an empty model entry runs no assistant that round.
	DiscussionAssistReviewerModel burlerengine.ModelSpecList `yaml:"discussion_assist_reviewer_model"`
	PlanAssistReviewerModel       burlerengine.ModelSpecList `yaml:"-"`
	CodeAssistReviewerModel       burlerengine.ModelSpecList `yaml:"code_assist_reviewer_model"`
	DiscussionAssistReviewerFan   lenses.FanList             `yaml:"discussion_assist_reviewer_fan"`
	PlanAssistReviewerFan         lenses.FanList             `yaml:"-"`
	CodeAssistReviewerFan         lenses.FanList             `yaml:"code_assist_reviewer_fan"`

	// AssistReviewerTimeoutMin is the cap, in minutes, on the assistant reviewer's run.
	// It keeps the run inside the reviewer's own timeout.
	AssistReviewerTimeoutMin int `yaml:"assist_reviewer_timeout_min"`

	Judge              string `yaml:"judge"`
	Seed               string `yaml:"seed"`
	DiscussionJudge    string `yaml:"discussion_judge"`
	PlanJudge          string `yaml:"-"`
	CodeJudge          string `yaml:"code_judge"`
	SplitJudge         string `yaml:"split_judge"`
	DiscussionSeed     string `yaml:"discussion_seed"`
	PlanSeed           string `yaml:"-"`
	CodeSeed           string `yaml:"code_seed"`
	ReviewTimeoutMin   int    `yaml:"review_timeout_min"`
	Friction           string `yaml:"friction"`
	FrictionTimeoutMin int    `yaml:"friction_timeout_min"`
	Driver             string `yaml:"driver"`
	StepIdleTimeoutMin int    `yaml:"step_idle_timeout_min"`

	// DiscussionAdvisors are the Discussion-Write advisors, one seat each; an empty list runs the lead alone.
	DiscussionAdvisors DiscussionAdvisorList `yaml:"discussion_advisors"`

	// DiscussionLenses names the lenses.yaml fan whose lenses the Discussion-Write lead forks itself for.
	// The forks start once its record is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan and nothing on the review side.
	DiscussionLenses string `yaml:"discussion_lenses"`

	// DiscussionGateFixer is the model-spec of the cold fixer of Discussion-Write's discussion gate; empty turns the fixer off.
	DiscussionGateFixer string `yaml:"discussion_gate_fixer"`

	// SubagentModel is the model the writers on Claude give the fresh subagents they hand lookups to.
	// A writer on Codex runs them on its own model.
	SubagentModel string `yaml:"subagent_model"`

	// PlanLenses names the lenses.yaml fan whose lenses the Plan-Write lead forks itself for.
	// The forks start once its plan is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan and nothing on the review side.
	PlanLenses string `yaml:"-"`

	// The plan-split keys below are the ones tapestry.yaml owns; WithTapestry fills them.
	PlanSplit            bool   `yaml:"-"`
	PartsDepth           string `yaml:"-"`
	PartsMax             int    `yaml:"-"`
	Parts                string `yaml:"-"`
	PartWriter           string `yaml:"-"`
	PartsChair           string `yaml:"-"`
	PartsChairTimeoutMin *int   `yaml:"-"`

	ReviewCirclingCheckpoint int `yaml:"review_circling_checkpoint"`
	ReviewMaxBounces         int `yaml:"review_max_bounces"`
	ReviewHardMaxBounces     int `yaml:"review_hard_max_bounces"`
	// ReviewAmendMaxBounces is the review rounds an amendment generation gets after a settled segment's subject changed.
	// It is clamped to the perch's own budget; 0 settles every amendment approved with no review round.
	ReviewAmendMaxBounces int `yaml:"review_amend_max_bounces"`
	// ReviewStalemateRounds is how many unchanged review rounds in a row escalate a stalemate; 0 turns the check off.
	ReviewStalemateRounds int `yaml:"review_stalemate_rounds"`

	// Each perch's bounce budget, hard cap and auto-accept flag.
	// A nil budget takes ReviewMaxBounces, and a budget of 0 runs no review round for the perch.
	// A hard cap of 0 takes ReviewHardMaxBounces.
	// A flag true makes the perch accept a harvested round at a spent budget instead of running the soft-cap rule.
	DiscussionMaxBounces     *int `yaml:"discussion_max_bounces"`
	PlanMaxBounces           *int `yaml:"-"`
	CodeMaxBounces           *int `yaml:"code_max_bounces"`
	DiscussionHardMaxBounces int  `yaml:"discussion_hard_max_bounces"`
	PlanHardMaxBounces       int  `yaml:"-"`
	CodeHardMaxBounces       int  `yaml:"code_hard_max_bounces"`
	DiscussionAutoAccept     bool `yaml:"discussion_auto_accept"`
	PlanAutoAccept           bool `yaml:"-"`
	CodeAutoAccept           bool `yaml:"code_auto_accept"`
	SplitMaxBounces          *int `yaml:"split_max_bounces"`
	SplitHardMaxBounces      int  `yaml:"split_hard_max_bounces"`
	SplitAutoAccept          bool `yaml:"split_auto_accept"`

	// PRReviewAskParent makes the PR-Review row ask the parent orch for a review of the run's open pull request.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	PRReviewAskParent bool `yaml:"pr_review_ask_parent"`
	// PRReviewSmallMax is the most findings PR-Review fixes in place when none is BLOCKING.
	// The count runs between two send-backs, or before the first.
	// It is open to the local layer, so a task's run_settings sets it for one run.
	PRReviewSmallMax int `yaml:"pr_review_small_max"`

	// DiscussionReviewAskParent makes each Discussion-Review round ask the parent orch to review the decision record.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	DiscussionReviewAskParent bool `yaml:"discussion_review_ask_parent"`
	// DiscussionReviewParentTimeoutMin is the minutes from a parent-review ask to its deadline, before the cap.
	// The cap leaves the reviewer room to consolidate.
	// It is a positive integer.
	DiscussionReviewParentTimeoutMin int `yaml:"discussion_review_parent_timeout_min"`

	// CodeReviewAskParent makes each Code-Review round's Bouncer ask the parent orch to review the task's code change.
	// The ask goes out once the round's fixer report exists, before the Bouncer judges the round.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	CodeReviewAskParent bool `yaml:"code_review_ask_parent"`
	// CodeReviewParentTimeoutMin is the minutes from a Code-Review parent-review ask to its deadline, before the cap.
	// The cap is the one run_timeout_min sets on the Bouncer's judge, which waits for the parent.
	// It is a positive integer.
	CodeReviewParentTimeoutMin int `yaml:"code_review_parent_timeout_min"`

	// NotifyParentOn names the recipe rows whose start lym mails the run's parent orch.
	// The parent can then brief the row's agent.
	// lym mails once each time the run reaches a row, and again on a re-call after an escalation's answer or a resumed halt.
	// It is open to the local layer, so a task's run_settings sets it for one run.
	NotifyParentOn []string `yaml:"notify_parent_on"`
}

// DiscussionAdvisor is one Discussion-Write advisor: Name is its seat name and Model its model-spec.
type DiscussionAdvisor struct {
	Name  string `yaml:"name"`
	Model string `yaml:"model"`
}

// DiscussionAdvisorList is the discussion_advisors value: a sequence of {name, model} mappings.
type DiscussionAdvisorList []DiscussionAdvisor

// advisorWayForward is the way forward every discussion_advisors refusal about its entries ends with.
const advisorWayForward = "set the key to a list of {name, model} entries, each with a distinct seat name and a model-spec the registry defines, or to an empty list for no advisors"

// advisorEntryWayForward is the way forward of an entry that is not a {name, model} mapping.
const advisorEntryWayForward = "rewrite the entry as {name: <seat name>, model: <model-spec>}"

// advisorShapeError is the error for a discussion_advisors value that is not a sequence of {name, model} mappings.
// Its text names the entry or the line and gives the way forward.
type advisorShapeError struct {
	text string
}

func (e *advisorShapeError) Error() string {
	return e.text
}

// UnmarshalYAML accepts a sequence of mappings with the keys name and model and rejects any other shape, a string entry included.
func (l *DiscussionAdvisorList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return &advisorShapeError{text: fmt.Sprintf("value at line %d is not a list of {name, model} entries; %s", node.Line, advisorWayForward)}
	}
	advisors := make(DiscussionAdvisorList, 0, len(node.Content))
	for i, entry := range node.Content {
		var advisor DiscussionAdvisor
		if entry.Kind != yaml.MappingNode || entry.Decode(&advisor) != nil {
			return &advisorShapeError{text: fmt.Sprintf("entry %d (line %d) is not a {name, model} mapping; %s", i+1, entry.Line, advisorEntryWayForward)}
		}
		advisors = append(advisors, advisor)
	}
	*l = advisors
	return nil
}

// ConfigOpenMaps returns the loom.yaml keys whose value is a scalar, a per-round list or a list of entries.
// The configengine carries those values whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{
		"reviewer_model", "fixer_model",
		"discussion_reviewer_model", "discussion_fixer_model",
		"code_reviewer_model", "code_fixer_model",
		"split_reviewer_model", "split_fixer_model",
		"discussion_reviewer_fan", "code_reviewer_fan", "split_reviewer_fan",
		"discussion_fixer_fan", "code_fixer_fan", "split_fixer_fan",
		"discussion_assist_reviewer_model", "code_assist_reviewer_model",
		"discussion_assist_reviewer_fan", "code_assist_reviewer_fan",
		"discussion_advisors",
	}
}

// ConfigSpec returns loom's layered-load spec.
// Only the keys that can switch off or hollow a review or approval gate are barred from the local layer.
// The budget, hard-cap and auto-accept keys come from reviewbudget.
// A budget of 0 is the per-run off switch and no guard; the auto-accept keys are guards.
// discussion_advisors and discussion_lenses are barred too.
// No local override or task run_settings can start advisors, turn the lens forks off or change their fan.
// code_reviewer_fan is the one reviewer fan key open to the local layer.
// It is open because its value names only a fan the guarded lenses.yaml defines, or "" for a solo round.
// The perch's round count, rubric and whole-diff read stay barred.
// raddle_auto_approve is the one approval key open to the local layer.
// A run profile's raddle block sets it, and batten writes that block into the pair's local config.
// The key is the sanctioned way to file a raddle proposal with no operator approval.
// It is read once, at Raddle-Write's first spawn in a run, and never in a run whose recipe rows hold a split perch.
func ConfigSpec() configengine.Spec {
	none := configengine.CategoryNone
	guard := configengine.CategoryGuard
	spec := configengine.Spec{
		Name:     "loom",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"discussion":                none,
			"discussion_timeout_min":    none,
			"discussion_interactive":    none,
			"raddle":                    none,
			"raddle_auto_approve":       none,
			"discussion_advisors":       guard,
			"discussion_lenses":         guard,
			"discussion_gate_fixer":     none,
			subagentModelKey:            none,
			"reviewer_model":            none,
			"fixer_model":               none,
			"discussion_reviewer_model": none,
			"discussion_fixer_model":    none,
			"code_reviewer_model":       none,
			"code_fixer_model":          none,
			"discussion_reviewer_fan":   guard,
			"code_reviewer_fan":         none,
			"discussion_fixer_fan":      guard,
			"code_fixer_fan":            guard,
			"split_reviewer_model":      none,
			"split_fixer_model":         none,
			"split_reviewer_fan":        guard,
			"split_fixer_fan":           guard,
			"split_judge":               none,

			"discussion_assist_reviewer_model": none,
			"code_assist_reviewer_model":       none,
			"discussion_assist_reviewer_fan":   none,
			"code_assist_reviewer_fan":         none,
			"assist_reviewer_timeout_min":      none,
			"judge":                            none,
			"seed":                             none,
			"discussion_judge":                 none,
			"code_judge":                       none,
			"discussion_seed":                  none,
			"code_seed":                        none,
			"review_timeout_min":               none,
			"friction":                         none,
			"friction_timeout_min":             none,
			"driver":                           none,
			"step_idle_timeout_min":            none,
			"review_circling_checkpoint":       none,
			"review_stalemate_rounds":          none,
			"pr_review_ask_parent":             none,
			"pr_review_small_max":              none,

			"discussion_review_ask_parent":         none,
			"discussion_review_parent_timeout_min": none,
			"code_review_ask_parent":               none,
			"code_review_parent_timeout_min":       none,
			"notify_parent_on":                     none,
		},
		RetiredKeys: map[string]configengine.Category{
			"fan_review": none,
			"pr_review":  none,
		},
		RetiredWayForward: map[string]string{},
	}
	for key, category := range reviewbudget.Categories(reviewPerchNames()) {
		spec.Categories[key] = category
	}
	for _, key := range shedOnlyKeys {
		spec.RetiredKeys[key] = guard
		spec.RetiredWayForward[key] = shedOnlyKeyWayForward
	}
	for _, key := range tapestryPlanKeys {
		spec.RetiredKeys[key] = guard
		spec.RetiredWayForward[key] = tapestryPlanKeyWayForward
	}
	return spec
}

// shedOnlyKeys are the driver, tackler and arbiter keys that live in shed.yaml alone.
// loom.yaml does not read them, and a local file, profile block or run_settings object that sets one is refused.
var shedOnlyKeys = []string{
	"driver_kind",
	"tackler", "tackler_timeout_min", "tackler_max_spawns",
	"arbiter", "arbiter_timeout_min", "arbiter_max_spawns",
}

// shedOnlyKeyWayForward is the way forward a refusal of a shedOnlyKeys key ends with.
const shedOnlyKeyWayForward = "remove the key from the refused local file, profile block or run_settings object, and set it under the shed module instead: in a local file, in the pair's local shed.yaml; in a profile block or run_settings, in the shed object"

// tapestryPlanKeys are the plan perch keys and the implementer key that live in tapestry.yaml alone.
// loom.yaml does not read them, and a local file, profile block or run_settings object that sets one is refused.
var tapestryPlanKeys = []string{
	"plan", "plan_timeout_min", "implementer", "plan_lenses",
	"plan_reviewer_model", "plan_fixer_model", "plan_reviewer_fan", "plan_fixer_fan",
	"plan_judge", "plan_seed", "plan_max_bounces", "plan_hard_max_bounces", "plan_auto_accept",
}

// tapestryPlanKeyWayForward is the way forward a refusal of a tapestryPlanKeys key ends with.
const tapestryPlanKeyWayForward = "set it under the tapestry module instead: in a local file, in the pair's local tapestry.yaml; in a profile block or run_settings, in the tapestry object"

// segmentModelList is one per-segment reviewer or fixer model-spec list with the loom.yaml key it came from.
type segmentModelList struct {
	key   string
	specs burlerengine.ModelSpecList
}

// segmentModelKeys returns the per-segment reviewer and fixer model-spec lists of cfg, each optional.
func segmentModelKeys(cfg Config) []segmentModelList {
	return []segmentModelList{
		{"discussion_reviewer_model", cfg.DiscussionReviewerModel},
		{"discussion_fixer_model", cfg.DiscussionFixerModel},
		{"code_reviewer_model", cfg.CodeReviewerModel},
		{"code_fixer_model", cfg.CodeFixerModel},
		{"split_reviewer_model", cfg.SplitReviewerModel},
		{"split_fixer_model", cfg.SplitFixerModel},
	}
}

const (
	// loomModule is the config module whose YAML file holds the model keys a seat's refusal names.
	loomModule = "loom"
	// discussionAdvisorsKey is the loom.yaml key holding the Discussion-Write advisors.
	discussionAdvisorsKey = "discussion_advisors"
	// discussionLensesKey is the loom.yaml key naming the fan the Discussion-Write lead forks its lenses from.
	discussionLensesKey = "discussion_lenses"
	// subagentModelKey is the loom.yaml key naming the writers' lookup subagent model; its value fills the stencil marker.
	subagentModelKey = "subagent_model"
	// planLensesKey is the tapestry.yaml key naming the fan the Plan-Write lead forks its lenses from.
	planLensesKey = "plan_lenses"
)

// validateDiscussionAdvisors rejects an advisor whose name is missing or blank, or repeats an earlier entry.
// It rejects a name that seatengine.ValidateSeatName refuses.
// It rejects a model that is no model-spec or that the registry does not define.
// Each error names the key and the 1-based entry.
func validateDiscussionAdvisors(advisors DiscussionAdvisorList, reg modelspec.Registry) error {
	refuse := func(entry int, err error) error {
		return fmt.Errorf("loom config key %q entry %d: %w; %s", discussionAdvisorsKey, entry, err, advisorWayForward)
	}
	firstEntryOfName := make(map[string]int, len(advisors))
	for i, advisor := range advisors {
		entry := i + 1
		if strings.TrimSpace(advisor.Name) == "" {
			return refuse(entry, errors.New("the name is missing or blank"))
		}
		if first, repeated := firstEntryOfName[advisor.Name]; repeated {
			return refuse(entry, fmt.Errorf("the name %q repeats entry %d", advisor.Name, first))
		}
		firstEntryOfName[advisor.Name] = entry
		if err := seatengine.ValidateSeatName(advisor.Name); err != nil {
			return refuse(entry, fmt.Errorf("the name %q is no seat name: %w", advisor.Name, err))
		}
		spec, err := modelspec.Parse(advisor.Model)
		if err != nil {
			return refuse(entry, err)
		}
		if _, err := reg.Resolve(spec); err != nil {
			return refuse(entry, err)
		}
	}
	return nil
}

// fanKeys returns the review perches' reviewer and fixer fan keys of cfg.
func fanKeys(cfg Config) []lenses.FanKey {
	return []lenses.FanKey{
		{Key: "discussion_reviewer_fan", Fans: cfg.DiscussionReviewerFan},
		{Key: "code_reviewer_fan", Fans: cfg.CodeReviewerFan},
		{Key: "discussion_fixer_fan", Fans: cfg.DiscussionFixerFan},
		{Key: "code_fixer_fan", Fans: cfg.CodeFixerFan},
		{Key: "split_reviewer_fan", Fans: cfg.SplitReviewerFan},
		{Key: "split_fixer_fan", Fans: cfg.SplitFixerFan},
		{Key: "discussion_assist_reviewer_fan", Fans: cfg.DiscussionAssistReviewerFan},
		{Key: "code_assist_reviewer_fan", Fans: cfg.CodeAssistReviewerFan},
	}
}

// ValidateLocalValues checks the review budget and hard-cap keys a local-layer document sets, through reviewbudget.
// Other keys are left to the config load.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	var budgets map[string]any
	if err := yaml.Unmarshal(document, &budgets); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal loom local values: %w", err))
	}
	return reviewbudget.CheckLocal(budgets, reviewPerchNames())
}

// LoadConfig loads and unmarshals configuration for the loom module strictly through the layer stack under dirs.
// It validates model-spec grammar at load time, and reads lenses.yaml and models.yaml through the same dirs.
// review_max_bounces and the per-perch budgets are refused below 0:
// null taking review_max_bounces and 0 giving zero rounds,
// and each refusal names the command for the layer its value comes from.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := loadResolved(dirs, spec)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		var shape *burlerengine.ModelSpecShapeError
		if errors.As(err, &shape) {
			return Config{}, fmt.Errorf("loom config key %q: %w", configengine.KeyAtLine(resolved, shape.Line), err)
		}
		var fanShape *lenses.FanShapeError
		if errors.As(err, &fanShape) {
			return Config{}, fmt.Errorf("loom config key %q: %w", configengine.KeyAtLine(resolved, fanShape.Line), err)
		}
		var advisorShape *advisorShapeError
		if errors.As(err, &advisorShape) {
			return Config{}, fmt.Errorf("loom config key %q: %w", discussionAdvisorsKey, err)
		}
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Discussion); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "discussion", err)
	}

	if _, err := modelspec.Parse(cfg.Raddle); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "raddle", err)
	}

	if cfg.SubagentModel == "" {
		return Config{}, fmt.Errorf("loom config key %q must not be empty: it names the model of the writers' lookup subagents; set it with lym config loom --set %s=sonnet", subagentModelKey, subagentModelKey)
	}

	if err := burlerengine.ValidateModelSpecList(loomModule, "reviewer_model", cfg.ReviewerModel); err != nil {
		return Config{}, err
	}

	if err := burlerengine.ValidateModelSpecList(loomModule, "fixer_model", cfg.FixerModel); err != nil {
		return Config{}, err
	}

	for _, segment := range segmentModelKeys(cfg) {
		if segment.specs.Unset() {
			continue
		}
		if err := burlerengine.ValidateModelSpecList(loomModule, segment.key, segment.specs); err != nil {
			return Config{}, err
		}
	}

	for _, assist := range []segmentModelList{
		{"discussion_assist_reviewer_model", cfg.DiscussionAssistReviewerModel},
		{"code_assist_reviewer_model", cfg.CodeAssistReviewerModel},
	} {
		if err := burlerengine.ValidateAssistModelList(loomModule, assist.key, assist.specs); err != nil {
			return Config{}, err
		}
	}

	if err := lenses.ValidateFanKeys(dirs, "loom", fanKeys(cfg)); err != nil {
		return Config{}, err
	}

	if len(cfg.DiscussionAdvisors) == 0 {
		cfg.DiscussionAdvisors = nil
	} else {
		reg, err := modelspec.LoadRegistry(dirs)
		if err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w; fix models.yaml, or set the key to an empty list for no advisors", discussionAdvisorsKey, err)
		}
		if err := validateDiscussionAdvisors(cfg.DiscussionAdvisors, reg); err != nil {
			return Config{}, err
		}
	}

	if _, err := modelspec.Parse(cfg.Judge); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "judge", err)
	}

	// The judge and seed override keys are validated only when non-empty:
	// an empty value takes its fallback key at run start and must load cleanly.
	for _, override := range []struct {
		key   string
		value string
	}{
		{"seed", cfg.Seed},
		{"discussion_judge", cfg.DiscussionJudge},
		{"code_judge", cfg.CodeJudge},
		{"split_judge", cfg.SplitJudge},
		{"discussion_seed", cfg.DiscussionSeed},
		{"code_seed", cfg.CodeSeed},
	} {
		if override.value == "" {
			continue
		}
		if _, err := modelspec.Parse(override.value); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", override.key, err)
		}
	}

	// An empty discussion_gate_fixer turns the fixer off and must load cleanly.
	if cfg.DiscussionGateFixer != "" {
		if _, err := modelspec.Parse(cfg.DiscussionGateFixer); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w; set a model-spec with \"lym config loom\", or \"\" to turn the fixer off", "discussion_gate_fixer", err)
		}
	}

	// friction is validated only when non-empty: a present-but-empty value means Tier 2 is off.
	// It must load cleanly, unlike the four role keys above, which are always required.
	if cfg.Friction != "" {
		if _, err := modelspec.Parse(cfg.Friction); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "friction", err)
		}
	}

	// driver is validated only when non-empty, like friction above: an empty value means
	// "defer to the engine default", the meaning an empty shuttleengine.Spec.Model already carries.
	// It must load cleanly, not be forced to name a literal alias.
	if cfg.Driver != "" {
		if _, err := modelspec.Parse(cfg.Driver); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "driver", err)
		}
	}

	// The four timeouts are checked here, so a value that can only be a mistake fails at load time.
	// A negative minute count flows into time.Duration(n) * time.Minute on a shuttleengine.Spec:
	// it is caught only when the producer it governs first spawns or resumes.
	// Zero is accepted and means something per key.
	// For three of them Spec.Timeout treats 0 as "defer to shuttle's own run_timeout_min".
	// For assist_reviewer_timeout_min, 0 takes the whole room left before review_timeout_min.
	const deferToShuttle = "defer to shuttle's run_timeout_min"
	for _, knob := range []struct {
		key       string
		minutes   int
		zeroMeans string
	}{
		{"discussion_timeout_min", cfg.DiscussionTimeoutMin, deferToShuttle},
		{"review_timeout_min", cfg.ReviewTimeoutMin, deferToShuttle},
		{"assist_reviewer_timeout_min", cfg.AssistReviewerTimeoutMin, "take the whole room left before review_timeout_min"},
		{"friction_timeout_min", cfg.FrictionTimeoutMin, deferToShuttle},
	} {
		if knob.minutes < 0 {
			return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; use 0 to %s", knob.key, knob.minutes, knob.zeroMeans)
		}
	}

	// step_idle_timeout_min is a wake interval's cousin: a value below the floor is raised to it with one Warn and never refused.
	if cfg.StepIdleTimeoutMin < stepIdleTimeoutFloorMin {
		logger.Warn("loom config key is below its floor and is raised to it", "key", "step_idle_timeout_min", "value", cfg.StepIdleTimeoutMin, "floor", stepIdleTimeoutFloorMin)
		cfg.StepIdleTimeoutMin = stepIdleTimeoutFloorMin
	}

	// A checkpoint above the budget is accepted: such a run never rules CIRCLING and reaches the soft-cap rule instead.
	if cfg.ReviewCirclingCheckpoint < 1 {
		return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", "review_circling_checkpoint", cfg.ReviewCirclingCheckpoint)
	}

	// The stalemate check only moves an escalation earlier or later, so 0 is legal and turns it off.
	if cfg.ReviewStalemateRounds < 0 {
		route, err := reviewbudget.Route(dirs, ConfigSpec(), "review_stalemate_rounds")
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; set it to 0 to turn the check off or to a positive integer; way forward: %s", "review_stalemate_rounds", cfg.ReviewStalemateRounds, route)
	}

	if cfg.DiscussionReviewParentTimeoutMin < 1 {
		return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", "discussion_review_parent_timeout_min", cfg.DiscussionReviewParentTimeoutMin)
	}
	if cfg.CodeReviewParentTimeoutMin < 1 {
		return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", "code_review_parent_timeout_min", cfg.CodeReviewParentTimeoutMin)
	}

	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{
		RunSoft:  cfg.ReviewMaxBounces,
		RunHard:  cfg.ReviewHardMaxBounces,
		RunAmend: cfg.ReviewAmendMaxBounces,
		Perches:  reviewPerchNames(),
		PerchSoft: map[string]*int{
			string(PerchDiscussion): cfg.DiscussionMaxBounces,
			string(PerchCode):       cfg.CodeMaxBounces,
			string(PerchSplit):      cfg.SplitMaxBounces,
		},
		PerchHard: map[string]int{
			string(PerchDiscussion): cfg.DiscussionHardMaxBounces,
			string(PerchCode):       cfg.CodeHardMaxBounces,
			string(PerchSplit):      cfg.SplitHardMaxBounces,
		},
	}); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
