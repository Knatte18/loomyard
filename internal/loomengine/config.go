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

// Config represents the resolved loom.yaml configuration, one nested struct per area of the file.
// Its Plan and Parts areas hold the plan perch's values, which tapestry.yaml owns;
// they stay at their zero value until tapestry.yaml is applied to the config.
type Config struct {
	Discussion DiscussionConfig `yaml:"discussion"`
	Code       CodeConfig       `yaml:"code"`
	Split      SplitConfig      `yaml:"split"`
	Raddle     RaddleConfig     `yaml:"raddle"`
	Friction   FrictionConfig   `yaml:"friction"`
	PR         PRConfig         `yaml:"pr"`
	Review     ReviewConfig     `yaml:"review"`
	Plan       PlanConfig       `yaml:"-"`
	Parts      PartsConfig      `yaml:"-"`

	Driver             string `yaml:"driver"`
	StepIdleTimeoutMin int    `yaml:"step_idle_timeout_min"`

	// SubagentModel is the model the writers on Claude give the fresh subagents they hand lookups to.
	// A writer on Codex runs them on its own model.
	SubagentModel string `yaml:"subagent_model"`

	// NotifyParentOn names the recipe rows whose start lym mails the run's parent orch.
	// The parent can then brief the row's agent.
	// lym mails once each time the run reaches a row, and again on a re-call after an escalation's answer or a resumed halt.
	// It is open to the local layer, so a task's run_settings sets it for one run.
	NotifyParentOn []string `yaml:"notify_parent_on"`
}

// DiscussionConfig is the discussion area: the Discussion-Write agent and the Discussion-Review perch.
type DiscussionConfig struct {
	// Model is the model-spec of the discussion agent.
	Model       string `yaml:"model"`
	TimeoutMin  int    `yaml:"timeout_min"`
	Interactive bool   `yaml:"interactive"`

	// Advisors are the Discussion-Write advisors, one seat each; an empty list runs the lead alone.
	Advisors DiscussionAdvisorList `yaml:"advisors"`
	// Lenses names the lenses.yaml fan whose lenses the Discussion-Write lead forks itself for.
	// The forks start once its record is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan and nothing on the review side.
	Lenses string `yaml:"lenses"`
	// GateFixer is the model-spec of the cold fixer of Discussion-Write's discussion gate; empty turns the fixer off.
	GateFixer string `yaml:"gate_fixer"`

	ReviewerModel       burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel          burlerengine.ModelSpecList `yaml:"fixer_model"`
	ReviewerFan         lenses.FanList             `yaml:"reviewer_fan"`
	FixerFan            lenses.FanList             `yaml:"fixer_fan"`
	AssistReviewerModel burlerengine.ModelSpecList `yaml:"assist_reviewer_model"`
	AssistReviewerFan   lenses.FanList             `yaml:"assist_reviewer_fan"`
	Judge               string                     `yaml:"judge"`
	Seed                string                     `yaml:"seed"`

	MaxBounces     *int `yaml:"max_bounces"`
	HardMaxBounces int  `yaml:"hard_max_bounces"`
	AutoAccept     bool `yaml:"auto_accept"`

	// ReviewAskParent makes each Discussion-Review round ask the parent orch to review the decision record.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	ReviewAskParent bool `yaml:"review_ask_parent"`
	// ReviewParentTimeoutMin is the minutes from a parent-review ask to its deadline, before the cap.
	// The cap leaves the reviewer room to consolidate.
	// It is a positive integer.
	ReviewParentTimeoutMin int `yaml:"review_parent_timeout_min"`
}

// CodeConfig is the code area: the Code-Review perch.
type CodeConfig struct {
	ReviewerModel       burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel          burlerengine.ModelSpecList `yaml:"fixer_model"`
	ReviewerFan         lenses.FanList             `yaml:"reviewer_fan"`
	FixerFan            lenses.FanList             `yaml:"fixer_fan"`
	AssistReviewerModel burlerengine.ModelSpecList `yaml:"assist_reviewer_model"`
	AssistReviewerFan   lenses.FanList             `yaml:"assist_reviewer_fan"`
	Judge               string                     `yaml:"judge"`
	Seed                string                     `yaml:"seed"`

	MaxBounces     *int `yaml:"max_bounces"`
	HardMaxBounces int  `yaml:"hard_max_bounces"`
	AutoAccept     bool `yaml:"auto_accept"`

	// ReviewAskParent makes each Code-Review round's Bouncer ask the parent orch to review the task's code change.
	// The ask goes out once the round's fixer report exists, before the Bouncer judges the round.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	ReviewAskParent bool `yaml:"review_ask_parent"`
	// ReviewParentTimeoutMin is the minutes from a Code-Review parent-review ask to its deadline, before the cap.
	// The cap is the one run_timeout_min sets on the Bouncer's judge, which waits for the parent.
	// It is a positive integer.
	ReviewParentTimeoutMin int `yaml:"review_parent_timeout_min"`
}

// SplitConfig is the split area: the Raddle-Review perch over a raddle proposal.
type SplitConfig struct {
	// ReviewerModel and FixerModel are the split perch's own lists.
	// An empty FixerModel takes the raddle agent's model, so the split is fixed by a model of that agent's strength.
	ReviewerModel burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel    burlerengine.ModelSpecList `yaml:"fixer_model"`
	ReviewerFan   lenses.FanList             `yaml:"reviewer_fan"`
	FixerFan      lenses.FanList             `yaml:"fixer_fan"`
	Judge         string                     `yaml:"judge"`

	MaxBounces     *int `yaml:"max_bounces"`
	HardMaxBounces int  `yaml:"hard_max_bounces"`
	AutoAccept     bool `yaml:"auto_accept"`
}

// RaddleConfig is the raddle area: the agent that proposes how a Sketch splits into tasks.
type RaddleConfig struct {
	// Model is the model-spec of the raddle agent.
	Model string `yaml:"model"`
	// AutoApprove makes a raddle run file its proposal once the proposal's gate passes, with no operator approval.
	// A run whose recipe rows hold a split perch never reads it, since the perch's approval files the proposal.
	AutoApprove bool `yaml:"auto_approve"`
}

// FrictionConfig is the friction area: the Tier 2 friction-reflection agent; an empty Model turns it off.
type FrictionConfig struct {
	Model      string `yaml:"model"`
	TimeoutMin int    `yaml:"timeout_min"`
}

// PRConfig is the pr area: the PR-Review row.
type PRConfig struct {
	// ReviewAskParent makes the PR-Review row ask the parent orch for a review of the run's open pull request.
	// It is open to the local layer, so a task's run_settings turns it on or off for one run.
	ReviewAskParent bool `yaml:"review_ask_parent"`
	// ReviewParentTimeoutMin is the minutes from a PR-Review ask to the parent orch to its deadline.
	// It is a positive integer, and a task's run_settings sets it for one run.
	ReviewParentTimeoutMin int `yaml:"review_parent_timeout_min"`
	// ReviewSmallMax is the most findings PR-Review fixes in place when none is BLOCKING.
	// The count runs between two send-backs, or before the first.
	// It is open to the local layer, so a task's run_settings sets it for one run.
	ReviewSmallMax int `yaml:"review_small_max"`
}

// ReviewConfig is the review area: the settings every review segment shares.
type ReviewConfig struct {
	// ReviewerModel and FixerModel are the shared reviewer and fixer model lists;
	// every review segment falls back to them when its own key is empty.
	ReviewerModel burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel    burlerengine.ModelSpecList `yaml:"fixer_model"`
	Judge         string                     `yaml:"judge"`
	Seed          string                     `yaml:"seed"`

	// AssistReviewerTimeoutMin is the cap, in minutes, on the assistant reviewer's run.
	// It keeps the run inside the reviewer's own timeout.
	AssistReviewerTimeoutMin int `yaml:"assist_reviewer_timeout_min"`
	TimeoutMin               int `yaml:"timeout_min"`
	CirclingCheckpoint       int `yaml:"circling_checkpoint"`
	// StalemateRounds is how many unchanged review rounds in a row escalate a stalemate; 0 turns the check off.
	StalemateRounds int `yaml:"stalemate_rounds"`

	// MaxBounces is the budget of every perch with none of its own.
	// HardMaxBounces is the hard cap of every perch whose own is 0.
	MaxBounces     int `yaml:"max_bounces"`
	HardMaxBounces int `yaml:"hard_max_bounces"`
	// AmendMaxBounces is the review rounds an amendment generation gets after a settled segment's subject changed.
	// It is clamped to the perch's own budget; 0 settles every amendment approved with no review round.
	AmendMaxBounces int `yaml:"amend_max_bounces"`
}

// The loom keys the run's autonomy forces.
const (
	keyPRReviewAskParent     = prReviewAskParentKey
	keyDiscussionInteractive = discussionInteractiveKey
	keyRaddleAutoApprove     = raddleAutoApproveKey
)

// ApplyAutonomy returns cfg with the keys the run's autonomy forces, when autonomous is true:
// the pr area's parent ask and the raddle auto-approval on, and the discussion interview off.
// The code and discussion areas' own parent asks keep their values.
// It rewrites no layer file, so autonomy off restores each key's layered value at the next load.
func ApplyAutonomy(cfg Config, autonomous bool) Config {
	if autonomous {
		cfg.PR.ReviewAskParent, cfg.Discussion.Interactive, cfg.Raddle.AutoApprove = true, false, true
	}
	return cfg
}

// AutonomyForcedSetting answers the value the run's autonomy forces on the loom key, and whether it forces one.
// It forces the three keys ApplyAutonomy forces and no other.
func AutonomyForcedSetting(key string) (value bool, forced bool) {
	switch key {
	case keyPRReviewAskParent, keyRaddleAutoApprove:
		return true, true
	case keyDiscussionInteractive:
		return false, true
	}
	return false, false
}

// DiscussionAdvisor is one Discussion-Write advisor: Name is its seat name and Model its model-spec.
type DiscussionAdvisor struct {
	Name  string `yaml:"name"`
	Model string `yaml:"model"`
}

// DiscussionAdvisorList is the discussion.advisors value: a sequence of {name, model} mappings.
type DiscussionAdvisorList []DiscussionAdvisor

// advisorWayForward is the way forward every discussion.advisors refusal about its entries ends with.
const advisorWayForward = "set the key to a list of {name, model} entries, each with a distinct seat name and a model-spec the registry defines, or to an empty list for no advisors"

// advisorEntryWayForward is the way forward of an entry that is not a {name, model} mapping.
const advisorEntryWayForward = "rewrite the entry as {name: <seat name>, model: <model-spec>}"

// advisorShapeError is the error for a discussion.advisors value that is not a sequence of {name, model} mappings.
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

// ConfigOpenMaps returns the loom.yaml paths whose value is a scalar, a per-round list or a list of entries.
// The configengine carries those values whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{
		"review.reviewer_model", "review.fixer_model",
		"discussion.reviewer_model", "discussion.fixer_model",
		"code.reviewer_model", "code.fixer_model",
		"split.reviewer_model", "split.fixer_model",
		"discussion.reviewer_fan", "code.reviewer_fan", "split.reviewer_fan",
		"discussion.fixer_fan", "code.fixer_fan", "split.fixer_fan",
		"discussion.assist_reviewer_model", "code.assist_reviewer_model",
		"discussion.assist_reviewer_fan", "code.assist_reviewer_fan",
		"discussion.advisors",
	}
}

// ConfigSpec returns loom's layered-load spec.
// Only the keys that can switch off or hollow a review or approval gate are barred from the local layer.
// The budget, hard-cap and auto-accept keys come from reviewbudget.
// A budget of 0 is the per-run off switch and no guard; the auto-accept keys are guards.
// discussion.advisors and discussion.lenses are barred too.
// No local override or task run_settings can start advisors, turn the lens forks off or change their fan.
// code.reviewer_fan is the one reviewer fan key open to the local layer.
// It is open because its value names only a fan the guarded lenses.yaml defines, or "" for a solo round.
// The perch's round count, rubric and whole-diff read stay barred.
// raddle.auto_approve is the one approval key open to the local layer.
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
			"discussion.model":          none,
			"discussion.timeout_min":    none,
			"discussion.interactive":    none,
			"raddle.model":              none,
			"raddle.auto_approve":       none,
			"discussion.advisors":       guard,
			"discussion.lenses":         guard,
			"discussion.gate_fixer":     none,
			subagentModelKey:            none,
			"review.reviewer_model":     none,
			"review.fixer_model":        none,
			"discussion.reviewer_model": none,
			"discussion.fixer_model":    none,
			"code.reviewer_model":       none,
			"code.fixer_model":          none,
			"discussion.reviewer_fan":   guard,
			"code.reviewer_fan":         none,
			"discussion.fixer_fan":      guard,
			"code.fixer_fan":            guard,
			"split.reviewer_model":      none,
			"split.fixer_model":         none,
			"split.reviewer_fan":        guard,
			"split.fixer_fan":           guard,
			"split.judge":               none,

			"discussion.assist_reviewer_model":   none,
			"code.assist_reviewer_model":         none,
			"discussion.assist_reviewer_fan":     none,
			"code.assist_reviewer_fan":           none,
			"review.assist_reviewer_timeout_min": none,
			"review.judge":                       none,
			"review.seed":                        none,
			"discussion.judge":                   none,
			"code.judge":                         none,
			"discussion.seed":                    none,
			"code.seed":                          none,
			"review.timeout_min":                 none,
			"friction.model":                     none,
			"friction.timeout_min":               none,
			"driver":                             none,
			"step_idle_timeout_min":              none,
			"review.circling_checkpoint":         none,
			"review.stalemate_rounds":            none,
			"pr.review_ask_parent":               none,
			"pr.review_parent_timeout_min":       none,
			"pr.review_small_max":                none,

			"discussion.review_ask_parent":         none,
			"discussion.review_parent_timeout_min": none,
			"code.review_ask_parent":               none,
			"code.review_parent_timeout_min":       none,
			"notify_parent_on":                     none,
		},
		RetiredKeys: map[string]configengine.Category{
			"fan_review": none,
			"pr_review":  none,
		},
		RetiredWayForward: map[string]string{},
		MovedKeys:         loomMovedKeys(),
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
		spec.RetiredWayForward[key] = tapestryPlanKeyWayForward(key)
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

// tapestryPlanKeyWayForward returns the way forward a refusal of the tapestryPlanKeys key key ends with.
// It names the full name the key has in the tapestry module.
func tapestryPlanKeyWayForward(key string) string {
	path := key
	if rest, ok := strings.CutPrefix(key, areaPlan+"_"); ok {
		path = areaPlan + "." + rest
	} else if key == areaPlan {
		path = planModelKey
	}
	return fmt.Sprintf("set it under the tapestry module as %s instead: in a local file, in the pair's local tapestry.yaml; in a profile block or run_settings, in the tapestry object", configengine.KeyName(tapestryModule, path))
}

// segmentModelList is one per-segment reviewer or fixer model-spec list with the loom.yaml key it came from.
type segmentModelList struct {
	key   string
	specs burlerengine.ModelSpecList
}

// segmentModelKeys returns the per-segment reviewer and fixer model-spec lists of cfg, each optional.
func segmentModelKeys(cfg Config) []segmentModelList {
	return []segmentModelList{
		{segmentKey(areaDiscussion, leafReviewerModel), cfg.Discussion.ReviewerModel},
		{segmentKey(areaDiscussion, leafFixerModel), cfg.Discussion.FixerModel},
		{segmentKey(areaCode, leafReviewerModel), cfg.Code.ReviewerModel},
		{segmentKey(areaCode, leafFixerModel), cfg.Code.FixerModel},
		{segmentKey(string(PerchSplit), leafReviewerModel), cfg.Split.ReviewerModel},
		{segmentKey(string(PerchSplit), leafFixerModel), cfg.Split.FixerModel},
	}
}

// loomModule is the config module whose YAML file holds the model keys a seat's refusal names.
const loomModule = "loom"

// validateDiscussionAdvisors rejects an advisor whose name is missing or blank, or repeats an earlier entry.
// It rejects a name that seatengine.ValidateSeatName refuses.
// It rejects a model that is no model-spec or that the registry does not define.
// Each error names the key and the 1-based entry.
func validateDiscussionAdvisors(advisors DiscussionAdvisorList, reg modelspec.Registry) error {
	refuse := func(entry int, err error) error {
		return fmt.Errorf("config key %q entry %d: %w; %s", FullKeyName(discussionAdvisorsKey), entry, err, advisorWayForward)
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
		{Key: segmentKey(areaDiscussion, leafReviewerFan), Fans: cfg.Discussion.ReviewerFan},
		{Key: segmentKey(areaCode, leafReviewerFan), Fans: cfg.Code.ReviewerFan},
		{Key: segmentKey(areaDiscussion, leafFixerFan), Fans: cfg.Discussion.FixerFan},
		{Key: segmentKey(areaCode, leafFixerFan), Fans: cfg.Code.FixerFan},
		{Key: segmentKey(string(PerchSplit), leafReviewerFan), Fans: cfg.Split.ReviewerFan},
		{Key: segmentKey(string(PerchSplit), leafFixerFan), Fans: cfg.Split.FixerFan},
		{Key: segmentKey(areaDiscussion, leafAssistReviewerFan), Fans: cfg.Discussion.AssistReviewerFan},
		{Key: segmentKey(areaCode, leafAssistReviewerFan), Fans: cfg.Code.AssistReviewerFan},
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
	return reviewbudget.CheckLocal(loomModule, budgets, reviewPerchNames())
}

// LoadConfig loads and unmarshals configuration for the loom module strictly through the layer stack under dirs.
// It validates model-spec grammar at load time, and reads lenses.yaml and models.yaml through the same dirs.
// review.max_bounces and the per-perch budgets are refused below 0:
// null taking review.max_bounces and 0 giving zero rounds,
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
			return Config{}, fmt.Errorf("config key %q: %w", configengine.KeyName(loomModule, configengine.KeyAtLine(resolved, shape.Line)), err)
		}
		var fanShape *lenses.FanShapeError
		if errors.As(err, &fanShape) {
			return Config{}, fmt.Errorf("config key %q: %w", configengine.KeyName(loomModule, configengine.KeyAtLine(resolved, fanShape.Line)), err)
		}
		var advisorShape *advisorShapeError
		if errors.As(err, &advisorShape) {
			return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(discussionAdvisorsKey), err)
		}
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Discussion.Model); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(discussionModelKey), err)
	}

	if _, err := modelspec.Parse(cfg.Raddle.Model); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(raddleModelKey), err)
	}

	if cfg.SubagentModel == "" {
		return Config{}, fmt.Errorf("config key %q must not be empty: it names the model of the writers' lookup subagents; set it with lym config loom --set %s=sonnet", FullKeyName(subagentModelKey), subagentModelKey)
	}

	if err := burlerengine.ValidateModelSpecList(loomModule, reviewReviewerModelKey, cfg.Review.ReviewerModel); err != nil {
		return Config{}, err
	}

	if err := burlerengine.ValidateModelSpecList(loomModule, reviewFixerModelKey, cfg.Review.FixerModel); err != nil {
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
		{segmentKey(areaDiscussion, leafAssistReviewerModel), cfg.Discussion.AssistReviewerModel},
		{segmentKey(areaCode, leafAssistReviewerModel), cfg.Code.AssistReviewerModel},
	} {
		if err := burlerengine.ValidateAssistModelList(loomModule, assist.key, assist.specs); err != nil {
			return Config{}, err
		}
	}

	if err := lenses.ValidateFanKeys(dirs, "loom", fanKeys(cfg)); err != nil {
		return Config{}, err
	}

	if len(cfg.Discussion.Advisors) == 0 {
		cfg.Discussion.Advisors = nil
	} else {
		reg, err := modelspec.LoadRegistry(dirs)
		if err != nil {
			return Config{}, fmt.Errorf("config key %q: %w; fix models.yaml, or set the key to an empty list for no advisors", FullKeyName(discussionAdvisorsKey), err)
		}
		if err := validateDiscussionAdvisors(cfg.Discussion.Advisors, reg); err != nil {
			return Config{}, err
		}
	}

	if _, err := modelspec.Parse(cfg.Review.Judge); err != nil {
		return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(reviewJudgeKey), err)
	}

	// The judge and seed override keys are validated only when non-empty:
	// an empty value takes its fallback key at run start and must load cleanly.
	for _, override := range []struct {
		key   string
		value string
	}{
		{reviewSeedKey, cfg.Review.Seed},
		{segmentKey(areaDiscussion, leafJudge), cfg.Discussion.Judge},
		{segmentKey(areaCode, leafJudge), cfg.Code.Judge},
		{segmentKey(string(PerchSplit), leafJudge), cfg.Split.Judge},
		{segmentKey(areaDiscussion, leafSeed), cfg.Discussion.Seed},
		{segmentKey(areaCode, leafSeed), cfg.Code.Seed},
	} {
		if override.value == "" {
			continue
		}
		if _, err := modelspec.Parse(override.value); err != nil {
			return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(override.key), err)
		}
	}

	// An empty discussion.gate_fixer turns the fixer off and must load cleanly.
	if cfg.Discussion.GateFixer != "" {
		if _, err := modelspec.Parse(cfg.Discussion.GateFixer); err != nil {
			return Config{}, fmt.Errorf("config key %q: %w; set a model-spec with \"lym config loom\", or \"\" to turn the fixer off", FullKeyName(discussionGateFixerKey), err)
		}
	}

	// friction.model is validated only when non-empty: a present-but-empty value means Tier 2 is off.
	// It must load cleanly, unlike the four role keys above, which are always required.
	if cfg.Friction.Model != "" {
		if _, err := modelspec.Parse(cfg.Friction.Model); err != nil {
			return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(frictionModelKey), err)
		}
	}

	// driver is validated only when non-empty, like friction.model above: an empty value means
	// "defer to the engine default", the meaning an empty shuttleengine.Spec.Model already carries.
	// It must load cleanly, not be forced to name a literal alias.
	if cfg.Driver != "" {
		if _, err := modelspec.Parse(cfg.Driver); err != nil {
			return Config{}, fmt.Errorf("config key %q: %w", FullKeyName(driverKey), err)
		}
	}

	// The four timeouts are checked here, so a value that can only be a mistake fails at load time.
	// A negative minute count flows into time.Duration(n) * time.Minute on a shuttleengine.Spec:
	// it is caught only when the producer it governs first spawns or resumes.
	// Zero is accepted and means something per key.
	// For three of them Spec.Timeout treats 0 as "defer to shuttle's own run_timeout_min".
	// For review.assist_reviewer_timeout_min, 0 takes the whole room left before review.timeout_min.
	deferToShuttle := "defer to " + shuttleRunTimeoutName()
	for _, knob := range []struct {
		key       string
		minutes   int
		zeroMeans string
	}{
		{discussionTimeoutMinKey, cfg.Discussion.TimeoutMin, deferToShuttle},
		{reviewTimeoutMinKey, cfg.Review.TimeoutMin, deferToShuttle},
		{reviewAssistTimeoutMinKey, cfg.Review.AssistReviewerTimeoutMin, "take the whole room left before " + FullKeyName(reviewTimeoutMinKey)},
		{frictionTimeoutMinKey, cfg.Friction.TimeoutMin, deferToShuttle},
	} {
		if knob.minutes < 0 {
			return Config{}, fmt.Errorf("config key %q: must not be negative, got %d; use 0 to %s", FullKeyName(knob.key), knob.minutes, knob.zeroMeans)
		}
	}

	// step_idle_timeout_min is a wake interval's cousin: a value below the floor is raised to it with one Warn and never refused.
	if cfg.StepIdleTimeoutMin < stepIdleTimeoutFloorMin {
		logger.Warn("loom config key is below its floor and is raised to it", "key", FullKeyName(stepIdleTimeoutMinKey), "value", cfg.StepIdleTimeoutMin, "floor", stepIdleTimeoutFloorMin)
		cfg.StepIdleTimeoutMin = stepIdleTimeoutFloorMin
	}

	// A checkpoint above the budget is accepted: such a run never rules CIRCLING and reaches the soft-cap rule instead.
	if cfg.Review.CirclingCheckpoint < 1 {
		return Config{}, fmt.Errorf("config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", FullKeyName(reviewCirclingCheckpointKey), cfg.Review.CirclingCheckpoint)
	}

	// The stalemate check only moves an escalation earlier or later, so 0 is legal and turns it off.
	if cfg.Review.StalemateRounds < 0 {
		route, err := reviewbudget.Route(dirs, ConfigSpec(), reviewStalemateRoundsKey)
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("config key %q: must not be negative, got %d; set it to 0 to turn the check off or to a positive integer; way forward: %s", FullKeyName(reviewStalemateRoundsKey), cfg.Review.StalemateRounds, route)
	}

	if cfg.Discussion.ReviewParentTimeoutMin < 1 {
		return Config{}, fmt.Errorf("config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", FullKeyName(discussionReviewParentTimeoutKey), cfg.Discussion.ReviewParentTimeoutMin)
	}
	if cfg.Code.ReviewParentTimeoutMin < 1 {
		return Config{}, fmt.Errorf("config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", FullKeyName(codeReviewParentTimeoutKey), cfg.Code.ReviewParentTimeoutMin)
	}
	if cfg.PR.ReviewParentTimeoutMin < 1 {
		return Config{}, fmt.Errorf("config key %q: must be at least 1, got %d; set it to a positive integer in loom.yaml", FullKeyName(prReviewParentTimeoutKey), cfg.PR.ReviewParentTimeoutMin)
	}

	if err := reviewbudget.CheckResolved(dirs, spec, reviewbudget.Values{
		RunSoft:  cfg.Review.MaxBounces,
		RunHard:  cfg.Review.HardMaxBounces,
		RunAmend: cfg.Review.AmendMaxBounces,
		Perches:  reviewPerchNames(),
		PerchSoft: map[string]*int{
			string(PerchDiscussion): cfg.Discussion.MaxBounces,
			string(PerchCode):       cfg.Code.MaxBounces,
			string(PerchSplit):      cfg.Split.MaxBounces,
		},
		PerchHard: map[string]int{
			string(PerchDiscussion): cfg.Discussion.HardMaxBounces,
			string(PerchCode):       cfg.Code.HardMaxBounces,
			string(PerchSplit):      cfg.Split.HardMaxBounces,
		},
	}); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
