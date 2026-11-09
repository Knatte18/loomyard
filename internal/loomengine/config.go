// config.go loads loom's configuration: the Config type mirrors the keys of loom.yaml.
// LoadConfig validates and resolves those keys.
// A bad value fails at load time, not hours into a run when its producer first spawns.
// The plan perch's keys live in tapestry.yaml, which LoadConfig never reads.
// Config.WithTapestry copies them in for the callers that serve the Plan-Review segment.
// MigrateConfig renames or removes the retired keys.

package loomengine

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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
	"github.com/Knatte18/loomyard/internal/yamlengine"
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

// loomReworkCoverageFileName is the filename of the rework agent's finding-to-card coverage map within LoomScratchDir.
// loomengine is this segment's sole declarer.
const loomReworkCoverageFileName = "rework-coverage.md"

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

// LoomReworkCoveragePath returns the path to the rework agent's completion file.
// The file maps each finding to the new cards that cover it.
// It lies in the ephemeral loom scratch directory.
// The file is never tracked, per the Durable-vs-Ephemeral State Invariant.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func LoomReworkCoveragePath(l *lymcwd.Location) string {
	return filepath.Join(LoomScratchDir(l), loomReworkCoverageFileName)
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

	// Each review segment's per-round reviewer and fixer fan lists.
	DiscussionReviewerFan lenses.FanList `yaml:"discussion_reviewer_fan"`
	PlanReviewerFan       lenses.FanList `yaml:"-"`
	CodeReviewerFan       lenses.FanList `yaml:"code_reviewer_fan"`
	DiscussionFixerFan    lenses.FanList `yaml:"discussion_fixer_fan"`
	PlanFixerFan          lenses.FanList `yaml:"-"`
	CodeFixerFan          lenses.FanList `yaml:"code_fixer_fan"`

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

	// SubagentModel is the model name the writers pass to the Agent tool for the fresh subagents they hand lookups to.
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

	// PRReviewAskParent makes the PR-Review row ask the parent orch for a review of the run's open pull request.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	PRReviewAskParent bool `yaml:"pr_review_ask_parent"`
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
		"discussion_reviewer_fan", "code_reviewer_fan",
		"discussion_fixer_fan", "code_fixer_fan",
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
			"pr_review_ask_parent":             none,
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
	// MigrateConfig writes it into a loom.yaml document, and the reconcile then moves it to tapestry.yaml.
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
		{Key: "discussion_assist_reviewer_fan", Fans: cfg.DiscussionAssistReviewerFan},
		{Key: "code_assist_reviewer_fan", Fans: cfg.CodeAssistReviewerFan},
	}
}

// MigrateConfig rewrites a loom.yaml document for lym config reconcile.
// It returns the rewritten bytes and one report line per rewrite.
// It raises a below-floor or null step_idle_timeout_min to its floor and replaces the retired producer keys.
// It retires skip_merge_review and fan_review, and renames the retired reviewer, fixer and fan keys.
// A discussion_producer of twill becomes discussion_lenses twill-discussion;
// any other value, including single and seats, leaves an empty discussion_lenses.
// A plan_producer of twill becomes plan_lenses twill-plan, and any other value leaves an empty plan_lenses.
// A document that set discussion_producer also gets discussion_advisors written to match.
// Under seats, each string entry n becomes {name: advisor-n, model: <entry>};
// the report line asks the operator to rename it.
// Under any other value the list is set to empty, even when the document set none.
// That keeps a lower layer's list off where this document turned it off.
// A seats document with no list of its own is left as it is, and the report line says where its advisors come from.
// A document holding skip_merge_review sets each per-perch budget of integer 0 to null;
// it also sets skip_merge_review true to merge_max_bounces: 0, and removes the key.
// Under the old schema a budget of 0 meant take review_max_bounces.
// A document without skip_merge_review keeps every budget as written, since its 0 already means zero rounds.
// Last, yamlengine.DropKeys removes each retired loom key, the Merge-Review perch's and alignment keys included;
// each dropped key reports the line `<key>: retired, removed`.
// review becomes reviewer_model, and fix becomes fixer_model.
// Each <segment>_review becomes <segment>_reviewer_model, and each <segment>_fix becomes <segment>_fixer_model.
// Each <segment>_fan becomes <segment>_reviewer_fan, and each <segment>_fix_fan becomes <segment>_fixer_fan.
// Each rename keeps the value; a document already holding the new name keeps its own value and drops the old key.
// Before the renames, a document holding webster_review or webster_fix gets the matching Merge key set from it.
// A Merge key already there is kept, so Merge-Review keeps the models it ran on.
// The value is the webster key's value;
// the renamed webster_reviewer_model or webster_fixer_model value replaces it when the document holds that too.
// After the renames, each webster_* review-perch key becomes its code_* successor with its value.
// The range runs from webster_reviewer_model to webster_auto_accept, so webster_review ends as code_reviewer_model.
// A document already holding the code_* name keeps its value and drops the webster_* key.
// A document without a retired key or comment is returned unchanged, so a migrated file is unchanged on a second pass.
// Everything else is carried through yaml node editing with its comments.
// The loader never calls it.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	raised, floorLine, err := yamlengine.RaiseIntFloor(existing, "step_idle_timeout_min", stepIdleTimeoutFloorMin)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	var rewrites []string
	if floorLine != "" {
		rewrites = append(rewrites, floorLine)
	}
	migrated, producerLines, err := migrateProducerKeys(raised)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, producerLines...)
	migrated, retiredLines, err := retireSkipMergeReview(migrated)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, retiredLines...)
	migrated, fanReviewLines, err := retireFanReview(migrated)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, fanReviewLines...)
	migrated, mergeLines, err := copyWebsterModelsOntoMerge(migrated)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, mergeLines...)
	migrated, renameLines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), migrated, retiredReviewKeyRenames)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, renameLines...)
	migrated, perchRenameLines, err := yamlengine.RenameKeys([]byte(ConfigTemplate()), migrated, retiredPerchKeyRenames)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, perchRenameLines...)
	migrated, dropLines, err := yamlengine.DropKeys(migrated, retiredLoomKeys)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	rewrites = append(rewrites, dropLines...)
	return migrated, rewrites, nil
}

// migrateProducerKeys replaces the retired discussion_producer and plan_producer keys of document.
// It returns the rewritten document and the report lines of the rewrites.
// It returns document unchanged, with no lines, when it holds neither key.
func migrateProducerKeys(document []byte) ([]byte, []string, error) {
	var tree yaml.Node
	if err := yaml.Unmarshal(document, &tree); err != nil {
		return nil, nil, err
	}
	if tree.Kind != yaml.DocumentNode || len(tree.Content) == 0 || tree.Content[0].Kind != yaml.MappingNode {
		return document, nil, nil
	}
	root := tree.Content[0]
	discussionProducer := mappingValueNode(root, "discussion_producer")
	planProducer := mappingValueNode(root, "plan_producer")
	if discussionProducer == nil && planProducer == nil {
		return document, nil, nil
	}

	var lines []string
	if discussionProducer != nil {
		producer := discussionProducer.Value
		lines = append(lines, replaceProducerKey(root, "discussion_producer", discussionLensesKey, "twill-discussion", discussionProducer))
		lines = append(lines, migrateAdvisors(root, producer))
	}
	if planProducer != nil {
		lines = append(lines, replaceProducerKey(root, "plan_producer", planLensesKey, "twill-plan", planProducer))
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&tree); err != nil {
		return nil, nil, fmt.Errorf("encode migrated loom config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode migrated loom config: %w", err)
	}
	return out.Bytes(), slices.DeleteFunc(lines, func(line string) bool { return line == "" }), nil
}

// replaceProducerKey renames the retired producer key of root to lensesKey in place, keeping its comments.
// It sets the key's value to fan when the producer was twill, and to an empty value otherwise.
// value is the producer key's value node.
// The returned line reports the rewrite.
func replaceProducerKey(root *yaml.Node, producerKey, lensesKey, fan string, value *yaml.Node) string {
	old := value.Value
	lenses := ""
	if old == "twill" {
		lenses = fan
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == producerKey {
			root.Content[i].Value = lensesKey
		}
	}
	value.Kind, value.Tag, value.Value, value.Content = yaml.ScalarNode, "!!str", lenses, nil
	value.Style = 0
	if lenses == "" {
		value.Style = yaml.DoubleQuotedStyle
	}
	return fmt.Sprintf("%s: %s replaced by %s: %q", producerKey, old, lensesKey, lenses)
}

// migrateAdvisors rewrites root's discussion_advisors to match the retired discussion_producer value producer.
// It returns the line reporting the rewrite, or "" when nothing changed.
func migrateAdvisors(root *yaml.Node, producer string) string {
	advisors := mappingValueNode(root, discussionAdvisorsKey)
	if producer != "seats" {
		if advisors == nil {
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: discussionAdvisorsKey}, emptySequenceNode())
			return fmt.Sprintf("%s: set to [] because discussion_producer was %s", discussionAdvisorsKey, producer)
		}
		if len(advisorEntryNodes(advisors)) == 0 {
			return ""
		}
		*advisors = emptySequenceNodeKeeping(advisors)
		return fmt.Sprintf("%s: emptied because discussion_producer was %s", discussionAdvisorsKey, producer)
	}
	if advisors == nil {
		return fmt.Sprintf("discussion_producer was seats and %s is not set here, so its advisors come from a lower layer whose migration may have emptied them; set %s to a list of {name, model} entries", discussionAdvisorsKey, discussionAdvisorsKey)
	}
	entries := advisorEntryNodes(advisors)
	if len(entries) == 0 {
		*advisors = emptySequenceNodeKeeping(advisors)
		return ""
	}
	renamed := 0
	rewritten := make([]*yaml.Node, len(entries))
	for i, entry := range entries {
		if entry.Kind != yaml.ScalarNode {
			rewritten[i] = entry
			continue
		}
		renamed++
		rewritten[i] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprintf("advisor-%d", i+1)},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "model"},
			entry,
		}}
	}
	advisors.Kind, advisors.Tag, advisors.Value, advisors.Style, advisors.Content = yaml.SequenceNode, "!!seq", "", 0, rewritten
	if renamed == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %d string entries rewritten as {name: advisor-<n>, model: <entry>}; rename each advisor to its seat name", discussionAdvisorsKey, renamed)
}

// advisorEntryNodes returns the entries of a retired-form discussion_advisors value.
// A sequence yields its own entries, and a scalar yields itself alone.
// An empty or null value yields none, and so does a list holding one empty string.
func advisorEntryNodes(value *yaml.Node) []*yaml.Node {
	entries := value.Content
	if value.Kind == yaml.ScalarNode {
		scalar := *value
		entries = []*yaml.Node{&scalar}
	}
	if len(entries) == 1 && entries[0].Kind == yaml.ScalarNode && entries[0].Value == "" {
		return nil
	}
	return entries
}

// emptySequenceNode returns a flow-style sequence node holding no entries.
func emptySequenceNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
}

// emptySequenceNodeKeeping returns an empty flow-style sequence node carrying node's comments.
func emptySequenceNodeKeeping(node *yaml.Node) yaml.Node {
	empty := emptySequenceNode()
	empty.HeadComment, empty.LineComment, empty.FootComment = node.HeadComment, node.LineComment, node.FootComment
	return *empty
}

// mappingValueNode returns the value node stored under key in mapping, or nil when mapping lacks the key.
func mappingValueNode(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// retireSkipMergeReview rewrites a document holding the retired skip_merge_review key onto the zero-round budgets.
// It returns the rewritten bytes and one line per rewrite.
// A document without the key comes back unchanged.
func retireSkipMergeReview(document []byte) ([]byte, []string, error) {
	var parsed yaml.Node
	if err := yaml.Unmarshal(document, &parsed); err != nil {
		return nil, nil, fmt.Errorf("unmarshal yaml: %w", err)
	}
	if parsed.Kind != yaml.DocumentNode || len(parsed.Content) == 0 || parsed.Content[0].Kind != yaml.MappingNode {
		return document, nil, nil
	}
	root := parsed.Content[0]
	skipAt := mappingKeyIndex(root, "skip_merge_review")
	if skipAt < 0 {
		return document, nil, nil
	}

	var lines []string
	for _, perch := range []Perch{PerchDiscussion, PerchPlan, PerchCode} {
		key := reviewbudget.SoftKey(string(perch))
		at := mappingKeyIndex(root, key)
		if at < 0 {
			continue
		}
		value := root.Content[at+1]
		if value.Kind == yaml.ScalarNode && value.Tag == "!!int" && value.Value == "0" {
			value.Tag, value.Style, value.Value = "!!null", 0, "~"
			lines = append(lines, key+": 0 rewritten to null, which takes review_max_bounces")
		}
	}

	var skip bool
	if err := root.Content[skipAt+1].Decode(&skip); err == nil && skip {
		mergeKey := "merge_max_bounces"
		if at := mappingKeyIndex(root, mergeKey); at >= 0 {
			value := root.Content[at+1]
			value.Kind, value.Tag, value.Style, value.Value = yaml.ScalarNode, "!!int", 0, "0"
		} else {
			root.Content = append(root.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: mergeKey},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "0"})
		}
		lines = append(lines, "skip_merge_review: true rewritten to merge_max_bounces: 0")
	}

	skipAt = mappingKeyIndex(root, "skip_merge_review")
	root.Content = append(root.Content[:skipAt], root.Content[skipAt+2:]...)
	lines = append(lines, "skip_merge_review: removed")

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&parsed); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), lines, nil
}

// retireFanReview removes the retired fan_review key from document.
// It returns the rewritten bytes and one line reporting the removal.
// A document without the key comes back unchanged.
func retireFanReview(document []byte) ([]byte, []string, error) {
	var parsed yaml.Node
	if err := yaml.Unmarshal(document, &parsed); err != nil {
		return nil, nil, fmt.Errorf("unmarshal yaml: %w", err)
	}
	if parsed.Kind != yaml.DocumentNode || len(parsed.Content) == 0 || parsed.Content[0].Kind != yaml.MappingNode {
		return document, nil, nil
	}
	root := parsed.Content[0]
	at := mappingKeyIndex(root, "fan_review")
	if at < 0 {
		return document, nil, nil
	}
	root.Content = append(root.Content[:at], root.Content[at+2:]...)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&parsed); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), []string{"fan_review: removed; a fanned round's reviewer now runs on its segment's own reviewer list"}, nil
}

// retiredReviewKeyRenames maps each retired reviewer, fixer and fan key of loom.yaml onto the key that replaced it.
var retiredReviewKeyRenames = []yamlengine.KeyRename{
	{From: "review", To: "reviewer_model"},
	{From: "fix", To: "fixer_model"},
	{From: "discussion_review", To: "discussion_reviewer_model"},
	{From: "discussion_fix", To: "discussion_fixer_model"},
	{From: "plan_review", To: "plan_reviewer_model"},
	{From: "plan_fix", To: "plan_fixer_model"},
	{From: "webster_review", To: "webster_reviewer_model"},
	{From: "webster_fix", To: "webster_fixer_model"},
	{From: "discussion_fan", To: "discussion_reviewer_fan"},
	{From: "plan_fan", To: "plan_reviewer_fan"},
	{From: "webster_fan", To: "webster_reviewer_fan"},
	{From: "merge_fan", To: "merge_reviewer_fan"},
	{From: "discussion_fix_fan", To: "discussion_fixer_fan"},
	{From: "plan_fix_fan", To: "plan_fixer_fan"},
	{From: "webster_fix_fan", To: "webster_fixer_fan"},
	{From: "merge_fix_fan", To: "merge_fixer_fan"},
}

// retiredPerchKeyRenames maps each webster_* review-perch key of loom.yaml onto the code_* key of the Code-Review perch.
// Apply it after the review-key renames,
// so a retired webster_review reaches code_reviewer_model through webster_reviewer_model.
var retiredPerchKeyRenames = []yamlengine.KeyRename{
	{From: "webster_reviewer_model", To: "code_reviewer_model"},
	{From: "webster_fixer_model", To: "code_fixer_model"},
	{From: "webster_reviewer_fan", To: "code_reviewer_fan"},
	{From: "webster_fixer_fan", To: "code_fixer_fan"},
	{From: "webster_judge", To: "code_judge"},
	{From: "webster_seed", To: "code_seed"},
	{From: "webster_max_bounces", To: "code_max_bounces"},
	{From: "webster_hard_max_bounces", To: "code_hard_max_bounces"},
	{From: "webster_auto_accept", To: "code_auto_accept"},
}

// retiredLoomKeys lists the loom.yaml keys no binary reads.
// Each is removed after the key renames, so a retired key's value can still reach its successor first.
var retiredLoomKeys = []string{
	"merge_reviewer_model",
	"merge_fixer_model",
	"merge_reviewer_fan",
	"merge_fixer_fan",
	"merge_max_bounces",
	"merge_hard_max_bounces",
	"merge_auto_accept",
	"rework",
	"alignment_pr_approve",
	"alignment",
	"alignment_timeout_min",
	"skip_alignment",
	"parent_review_wait_min",
}

// copyWebsterModelsOntoMerge sets merge_reviewer_model and merge_fixer_model of document from their Webster keys.
// webster_review feeds merge_reviewer_model and webster_fix feeds merge_fixer_model.
// Each copy happens only when the document holds the Webster key and not the Merge key.
// A renamed key, webster_reviewer_model or webster_fixer_model, wins over its retired key, since the rename keeps it.
// The copy keeps Merge-Review on the models it ran on before it had keys of its own.
// It returns the rewritten bytes, with one line per copied key.
// It returns document itself, with no lines, when nothing is copied.
func copyWebsterModelsOntoMerge(document []byte) ([]byte, []string, error) {
	var parsed yaml.Node
	if err := yaml.Unmarshal(document, &parsed); err != nil {
		return nil, nil, fmt.Errorf("unmarshal yaml: %w", err)
	}
	if parsed.Kind != yaml.DocumentNode || len(parsed.Content) == 0 || parsed.Content[0].Kind != yaml.MappingNode {
		return document, nil, nil
	}
	var template yaml.Node
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &template); err != nil {
		return nil, nil, fmt.Errorf("unmarshal loom template: %w", err)
	}
	root := parsed.Content[0]
	var lines []string
	for _, copied := range []struct{ webster, websterRenamed, merge string }{
		{"webster_review", "webster_reviewer_model", "merge_reviewer_model"},
		{"webster_fix", "webster_fixer_model", "merge_fixer_model"},
	} {
		retiredAt := mappingKeyIndex(root, copied.webster)
		if retiredAt < 0 || mappingKeyIndex(root, copied.merge) >= 0 {
			continue
		}
		// The rename keeps the new Webster key over the retired one, so Merge follows the new key's value.
		source, at := copied.webster, retiredAt
		if renamedAt := mappingKeyIndex(root, copied.websterRenamed); renamedAt >= 0 {
			source, at = copied.websterRenamed, renamedAt
		}
		value := *root.Content[at+1]
		if templated := mappingValueNode(template.Content[0], copied.merge); templated != nil {
			value.LineComment = templated.LineComment
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: copied.merge}, &value)
		lines = append(lines, fmt.Sprintf("%s: set to %s's value, the models Merge-Review ran on", copied.merge, source))
	}
	if len(lines) == 0 {
		return document, nil, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&parsed); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), lines, nil
}

// mappingKeyIndex returns the index of key in mapping's Content, or -1 when mapping lacks it.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
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

	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{
		RunSoft:  cfg.ReviewMaxBounces,
		RunHard:  cfg.ReviewHardMaxBounces,
		RunAmend: cfg.ReviewAmendMaxBounces,
		Perches:  reviewPerchNames(),
		PerchSoft: map[string]*int{
			string(PerchDiscussion): cfg.DiscussionMaxBounces,
			string(PerchCode):       cfg.CodeMaxBounces,
		},
		PerchHard: map[string]int{
			string(PerchDiscussion): cfg.DiscussionHardMaxBounces,
			string(PerchCode):       cfg.CodeHardMaxBounces,
		},
	}); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
