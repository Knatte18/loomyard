// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which uses internal/configengine.Load with ConfigTemplate() to strictly validate and resolve loom's config file,
// then validates the discussion, plan, judge, friction, and driver role model-specs and every entry of the shared reviewer_model and fixer_model lists' grammar via modelspec.Parse, plus every entry of each review segment's own <segment>_reviewer_model and <segment>_fixer_model list that is set, an empty value meaning the shared list, rejects a negative value on each of the four timeout knobs, and rejects a parent_review_wait_min, review_circling_checkpoint or review_hard_max_bounces below 1, rejects a review_max_bounces, discussion_max_bounces, plan_max_bounces, webster_max_bounces or merge_max_bounces below 0 (0 giving zero rounds, null taking review_max_bounces) or any per-perch hard cap below 0, rejects a perch whose resolved hard cap is below its resolved budget, and resolves each non-empty entry of every perch's fan and fix fan key through burlerengine.ValidateFanKeys,
// so a mistake in any of those validated keys fails loud at load time rather than hours into a run when the discussion, plan, review, judge, friction, or driver producer first spawns.
// step_idle_timeout_min is the minutes a --until-stop step may show no activity before the loop kills it; a value below 1 is raised to 1 with one Warn and never refused.
// discussion_reviewer_fan, plan_reviewer_fan, webster_reviewer_fan and merge_reviewer_fan each name a fan from burler.yaml, or list one per round, and turn the lens fan on for Discussion-Review, Plan-Review, Webster-Review and Merge-Review; an empty entry, the default, runs that round solo.
// discussion_fixer_fan, plan_fixer_fan, webster_fixer_fan and merge_fixer_fan do the same for the fixer: an empty entry runs the single fixer, and a fan name runs the twill fixer with that fan's lenses.
// A list's last entry serves every later round, and a reviewer fanned or not runs on its segment's own reviewer list, whose model the forks inherit.
// fan_review is retired: MigrateConfig removes it.
// The retired review, fix, <segment>_review, <segment>_fix, <segment>_fan and <segment>_fix_fan keys are renamed by MigrateConfig.
// discussion_max_bounces, plan_max_bounces and webster_max_bounces set one perch's bounce budget, null takes review_max_bounces and 0 gives the perch zero rounds, its Bouncer settling approved at once;
// review_hard_max_bounces is the round number at or past which no perch continues a round, refused below 1, and discussion_hard_max_bounces, plan_hard_max_bounces, webster_hard_max_bounces and merge_hard_max_bounces set one perch's own, where 0 takes review_hard_max_bounces and a negative value is refused;
// a perch whose resolved hard cap is below its resolved budget is refused too, and every one of these refusals names the command that changes the key in the layer its value comes from.
// discussion_auto_accept, plan_auto_accept and webster_auto_accept make that perch accept a harvested round at a spent budget instead of running the soft-cap rule.
// merge_max_bounces, merge_hard_max_bounces and merge_auto_accept do the same for the Merge-Review perch, which sends a round whose ledger holds an open BLOCKING finding to the soft-cap rule instead of accepting it;
// merge_max_bounces: 0 skips only that review, and Merge-In, the parent lock and the reroute on a moved parent still run.
// skip_merge_review is retired: MigrateConfig rewrites it onto merge_max_bounces.
// discussion_advisors lists the Discussion-Write advisors, each entry a {name, model} pair: name is the advisor's seat name and model its model-spec, resolved through the model registry at load, and an unset list loads as no advisors.
// With no advisors the Discussion-Write lead decides each question itself.
// discussion_lenses and plan_lenses each name the burler.yaml fan whose lenses the Discussion-Write or Plan-Write lead forks itself for once its output is written and self-checked; empty, the default, turns the forks off.
// Neither is the discussion_reviewer_fan or plan_reviewer_fan of a review perch, and all three of these keys are closed to the local layer.
// The retired discussion_producer and plan_producer keys are rewritten into these by MigrateConfig.
// seed, discussion_judge, plan_judge, webster_judge, discussion_seed, plan_seed, webster_seed and rework are role model-specs validated only when non-empty, an empty value meaning the fallback:
// a per-segment judge takes judge, a per-segment seed takes seed, seed takes that segment's resolved judge, and rework takes plan.
// arbiter is a role model-spec validated only when non-empty, an empty value taking opus[high]; arbiter_timeout_min and arbiter_max_spawns are refused when negative, and a zero arbiter_max_spawns turns the arbiter off.
// driver_kind is the driver a task's loom run is seeded with when nothing typed one, llm or go, an empty value reading as llm; tackler is a role model-spec validated only when non-empty, an empty value taking haiku;
// tackler_timeout_min and tackler_max_spawns are refused when negative, a zero tackler_timeout_min taking 10 and a zero tackler_max_spawns turning the tackler off.
// alignment, alignment_timeout_min, skip_alignment and alignment_pr_approve configure the alignment agent at parent review and PR-Gate:
// alignment is a role model-spec validated only when non-empty, an empty value taking opus[high];
// alignment_timeout_min is refused when negative, a zero taking 20, and a resolved value at or above parent_review_wait_min is refused too.
// friction and driver are the two role keys validated only when non-empty: a present-but-empty
// value means, respectively, Tier 2 self-reporting is off or the engine default model runs the
// driver, and both must load cleanly, unlike the other role keys, which are always required.

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
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// stepIdleTimeoutFloorMin is the smallest step_idle_timeout_min LoadConfig returns, in minutes.
const stepIdleTimeoutFloorMin = 1

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
// producer's design input from `_lyx/discussion/`.
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

// DiscussionRelevant returns the path to the Discussion-Write lead's list of the code it read.
// The list is the plan writer's head start.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionRelevant(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "relevant.md")
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

// PathsOutsideRunRecords returns the absolute paths that lie outside the run's own records directories, in their input order.
// The records directories are the run's directory, the loom durable directory and the reviews directory: the ones the status commit stages.
// A path under one of them is run state a hand-over leaves to the next driver; a path that only shares a directory's name as a prefix is outside it.
func PathsOutsideRunRecords(l *lyxcwd.Location, runID string, paths []string) []string {
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

// LoomFrictionDirRel returns the worktree-anchor-relative form of LoomFrictionDir's path: the join of LoomDurableDirRel and the friction segment.
// A reader of a git tree applies filepath.ToSlash to it.
func LoomFrictionDirRel() string {
	return filepath.Join(LoomDurableDirRel(), frictionDirName)
}

// IsLoomFrictionArchive reports whether name, a directory name under LoomDurableDirRel, is a timestamped reflection-archive sibling of the friction directory.
// That is the friction segment followed by "-" and at least one more character; the friction directory itself is not an archive.
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

// LandingCheckpointSlug is the inverse of LandingCheckpointSubject: the landing run slug when subject has the checkpoint prefix and a non-empty rest, else false.
func LandingCheckpointSlug(subject string) (string, bool) {
	slug, ok := strings.CutPrefix(subject, landingCheckpointSubjectPrefix)
	if !ok || slug == "" {
		return "", false
	}
	return slug, true
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
	Discussion            string `yaml:"discussion"`
	DiscussionTimeoutMin  int    `yaml:"discussion_timeout_min"`
	DiscussionInteractive bool   `yaml:"discussion_interactive"`
	Plan                  string `yaml:"plan"`
	PlanTimeoutMin        int    `yaml:"plan_timeout_min"`

	// ReviewerModel and FixerModel are the shared reviewer and fixer model lists, the fallback of every review segment whose own key is empty.
	// Each segment's own pair replaces them for that segment when set.
	ReviewerModel           burlerengine.ModelSpecList `yaml:"reviewer_model"`
	FixerModel              burlerengine.ModelSpecList `yaml:"fixer_model"`
	DiscussionReviewerModel burlerengine.ModelSpecList `yaml:"discussion_reviewer_model"`
	DiscussionFixerModel    burlerengine.ModelSpecList `yaml:"discussion_fixer_model"`
	PlanReviewerModel       burlerengine.ModelSpecList `yaml:"plan_reviewer_model"`
	PlanFixerModel          burlerengine.ModelSpecList `yaml:"plan_fixer_model"`
	WebsterReviewerModel    burlerengine.ModelSpecList `yaml:"webster_reviewer_model"`
	WebsterFixerModel       burlerengine.ModelSpecList `yaml:"webster_fixer_model"`
	MergeReviewerModel      burlerengine.ModelSpecList `yaml:"merge_reviewer_model"`
	MergeFixerModel         burlerengine.ModelSpecList `yaml:"merge_fixer_model"`

	// Each review segment's per-round reviewer and fixer fan lists.
	DiscussionReviewerFan burlerengine.FanList `yaml:"discussion_reviewer_fan"`
	PlanReviewerFan       burlerengine.FanList `yaml:"plan_reviewer_fan"`
	WebsterReviewerFan    burlerengine.FanList `yaml:"webster_reviewer_fan"`
	MergeReviewerFan      burlerengine.FanList `yaml:"merge_reviewer_fan"`
	DiscussionFixerFan    burlerengine.FanList `yaml:"discussion_fixer_fan"`
	PlanFixerFan          burlerengine.FanList `yaml:"plan_fixer_fan"`
	WebsterFixerFan       burlerengine.FanList `yaml:"webster_fixer_fan"`
	MergeFixerFan         burlerengine.FanList `yaml:"merge_fixer_fan"`

	Judge               string `yaml:"judge"`
	Seed                string `yaml:"seed"`
	DiscussionJudge     string `yaml:"discussion_judge"`
	PlanJudge           string `yaml:"plan_judge"`
	WebsterJudge        string `yaml:"webster_judge"`
	DiscussionSeed      string `yaml:"discussion_seed"`
	PlanSeed            string `yaml:"plan_seed"`
	WebsterSeed         string `yaml:"webster_seed"`
	Rework              string `yaml:"rework"`
	ReviewTimeoutMin    int    `yaml:"review_timeout_min"`
	Friction            string `yaml:"friction"`
	FrictionTimeoutMin  int    `yaml:"friction_timeout_min"`
	Driver              string `yaml:"driver"`
	ParentReviewWaitMin int    `yaml:"parent_review_wait_min"`
	StepIdleTimeoutMin  int    `yaml:"step_idle_timeout_min"`

	// DiscussionAdvisors are the Discussion-Write advisors, one seat each; an empty list runs the lead alone.
	DiscussionAdvisors DiscussionAdvisorList `yaml:"discussion_advisors"`

	// DiscussionLenses names the burler.yaml fan whose lenses the Discussion-Write lead forks itself for once its record is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan, not the Discussion-Review perch's DiscussionReviewerFan.
	DiscussionLenses string `yaml:"discussion_lenses"`

	// PlanLenses names the burler.yaml fan whose lenses the Plan-Write lead forks itself for once its plan is written and self-checked; empty turns the forks off.
	// It names the writer's lens fan, not the Plan-Review perch's PlanReviewerFan.
	PlanLenses string `yaml:"plan_lenses"`

	// The arbiter's model-spec, timeout and spawn budget.
	// An empty Arbiter takes opus[high], a zero ArbiterTimeoutMin takes 30 minutes, and a zero ArbiterMaxSpawns turns the arbiter off.
	Arbiter           string `yaml:"arbiter"`
	ArbiterTimeoutMin int    `yaml:"arbiter_timeout_min"`
	ArbiterMaxSpawns  int    `yaml:"arbiter_max_spawns"`

	// The driver a task's loom run is seeded with when nothing typed one, and the tackler's model-spec, timeout and spawn budget; each overrides shed.yaml's key.
	// An empty DriverKind or Tackler, a zero TacklerTimeoutMin and a null TacklerMaxSpawns inherit shed.yaml, and a set TacklerMaxSpawns of 0 turns the tackler off.
	DriverKind        string `yaml:"driver_kind"`
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  *int   `yaml:"tackler_max_spawns"`

	// The alignment agent's model-spec, timeout and switches.
	// An empty Alignment takes opus[high] and a zero AlignmentTimeoutMin takes 20 minutes, kept below ParentReviewWaitMin.
	// SkipAlignment true sends parent review and PR-Gate to the run's parent without the agent.
	// AlignmentPRApprove false makes an agent approve at PR-Gate an escalation, so the parent gives the yes.
	Alignment           string `yaml:"alignment"`
	AlignmentTimeoutMin int    `yaml:"alignment_timeout_min"`
	SkipAlignment       bool   `yaml:"skip_alignment"`
	AlignmentPRApprove  bool   `yaml:"alignment_pr_approve"`

	ReviewCirclingCheckpoint int `yaml:"review_circling_checkpoint"`
	ReviewMaxBounces         int `yaml:"review_max_bounces"`
	ReviewHardMaxBounces     int `yaml:"review_hard_max_bounces"`

	// Each perch's bounce budget, hard cap and auto-accept flag.
	// A nil budget takes ReviewMaxBounces and a budget of 0 runs no review round for the perch; a hard cap of 0 takes ReviewHardMaxBounces;
	// a flag true makes the perch accept a harvested round at a spent budget instead of running the soft-cap rule.
	DiscussionMaxBounces     *int `yaml:"discussion_max_bounces"`
	PlanMaxBounces           *int `yaml:"plan_max_bounces"`
	WebsterMaxBounces        *int `yaml:"webster_max_bounces"`
	DiscussionHardMaxBounces int  `yaml:"discussion_hard_max_bounces"`
	PlanHardMaxBounces       int  `yaml:"plan_hard_max_bounces"`
	WebsterHardMaxBounces    int  `yaml:"webster_hard_max_bounces"`
	DiscussionAutoAccept     bool `yaml:"discussion_auto_accept"`
	PlanAutoAccept           bool `yaml:"plan_auto_accept"`
	WebsterAutoAccept        bool `yaml:"webster_auto_accept"`

	// The Merge-Review perch's budget, hard cap and auto-accept flag.
	// A nil budget takes ReviewMaxBounces and a budget of 0 runs no merge review round; a hard cap of 0 takes ReviewHardMaxBounces.
	MergeMaxBounces     *int `yaml:"merge_max_bounces"`
	MergeHardMaxBounces int  `yaml:"merge_hard_max_bounces"`
	MergeAutoAccept     bool `yaml:"merge_auto_accept"`
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

// advisorShapeError reports a discussion_advisors value that is not a sequence of {name, model} mappings; its text names the entry or the line and the way forward.
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

// ConfigOpenMaps returns the loom.yaml keys whose value is a scalar, a per-round list or a list of entries, which configengine carries whole through reconcile and --set.
func ConfigOpenMaps() []string {
	return []string{
		"reviewer_model", "fixer_model",
		"discussion_reviewer_model", "discussion_fixer_model", "plan_reviewer_model", "plan_fixer_model",
		"webster_reviewer_model", "webster_fixer_model", "merge_reviewer_model", "merge_fixer_model",
		"discussion_reviewer_fan", "plan_reviewer_fan", "webster_reviewer_fan", "merge_reviewer_fan",
		"discussion_fixer_fan", "plan_fixer_fan", "webster_fixer_fan", "merge_fixer_fan",
		"discussion_advisors",
	}
}

// ConfigSpec returns loom's layered-load spec.
// Only the keys that can switch off or hollow a review or approval gate are barred from the local layer.
// The budget, hard-cap and auto-accept keys come from reviewbudget: a budget of 0 is the per-run off switch and no guard, while the auto-accept keys are guards.
// The agent's approve at PR-Gate lands on the parent with no later gate, so a weaker alignment model, a re-opened auto-landing (alignment_pr_approve) or a local skip_alignment: false under a hub true hollows the approval gate;
// a shorter alignment_timeout_min only routes a review to the parent.
// discussion_advisors, discussion_lenses and plan_lenses are barred too, so no local override or task run_settings can start advisors, turn the lens forks off or change their fan.
func ConfigSpec() configengine.Spec {
	none := configengine.CategoryNone
	guard := configengine.CategoryGuard
	spec := configengine.Spec{
		Name:     "loom",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"discussion":                 none,
			"discussion_timeout_min":     none,
			"discussion_interactive":     none,
			"discussion_advisors":        guard,
			"discussion_lenses":          guard,
			"plan":                       none,
			"plan_timeout_min":           none,
			"plan_lenses":                guard,
			"reviewer_model":             none,
			"fixer_model":                none,
			"discussion_reviewer_model":  none,
			"discussion_fixer_model":     none,
			"plan_reviewer_model":        none,
			"plan_fixer_model":           none,
			"webster_reviewer_model":     none,
			"webster_fixer_model":        none,
			"merge_reviewer_model":       none,
			"merge_fixer_model":          none,
			"discussion_reviewer_fan":    guard,
			"plan_reviewer_fan":          guard,
			"webster_reviewer_fan":       guard,
			"merge_reviewer_fan":         guard,
			"discussion_fixer_fan":       guard,
			"plan_fixer_fan":             guard,
			"webster_fixer_fan":          guard,
			"merge_fixer_fan":            guard,
			"judge":                      none,
			"seed":                       none,
			"discussion_judge":           none,
			"plan_judge":                 none,
			"webster_judge":              none,
			"discussion_seed":            none,
			"plan_seed":                  none,
			"webster_seed":               none,
			"rework":                     none,
			"review_timeout_min":         none,
			"friction":                   none,
			"friction_timeout_min":       none,
			"driver":                     none,
			"arbiter":                    none,
			"arbiter_timeout_min":        none,
			"arbiter_max_spawns":         none,
			"driver_kind":                none,
			"tackler":                    none,
			"tackler_timeout_min":        none,
			"tackler_max_spawns":         none,
			"alignment":                  guard,
			"alignment_timeout_min":      none,
			"skip_alignment":             guard,
			"alignment_pr_approve":       guard,
			"parent_review_wait_min":     guard,
			"step_idle_timeout_min":      none,
			"review_circling_checkpoint": none,
		},
		RetiredKeys: map[string]configengine.Category{
			"fan_review": none,
		},
	}
	for key, category := range reviewbudget.Categories(reviewPerchNames()) {
		spec.Categories[key] = category
	}
	return spec
}

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
		{"plan_reviewer_model", cfg.PlanReviewerModel},
		{"plan_fixer_model", cfg.PlanFixerModel},
		{"webster_reviewer_model", cfg.WebsterReviewerModel},
		{"webster_fixer_model", cfg.WebsterFixerModel},
		{"merge_reviewer_model", cfg.MergeReviewerModel},
		{"merge_fixer_model", cfg.MergeFixerModel},
	}
}

const (
	// loomModule is the config module whose YAML file holds the model keys a seat's refusal names.
	loomModule = "loom"
	// discussionAdvisorsKey is the loom.yaml key holding the Discussion-Write advisors.
	discussionAdvisorsKey = "discussion_advisors"
	// discussionLensesKey is the loom.yaml key naming the fan the Discussion-Write lead forks its lenses from.
	discussionLensesKey = "discussion_lenses"
	// planLensesKey is the loom.yaml key naming the fan the Plan-Write lead forks its lenses from.
	planLensesKey = "plan_lenses"
)

// validateDiscussionAdvisors rejects a missing or blank name, a name an earlier entry already holds, a name seatengine.ValidateSeatName refuses, and a model that is no model-spec or that the registry does not define, naming the key and the 1-based entry.
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
func fanKeys(cfg Config) []burlerengine.FanKey {
	return []burlerengine.FanKey{
		{Key: "discussion_reviewer_fan", Fans: cfg.DiscussionReviewerFan},
		{Key: "plan_reviewer_fan", Fans: cfg.PlanReviewerFan},
		{Key: "webster_reviewer_fan", Fans: cfg.WebsterReviewerFan},
		{Key: "merge_reviewer_fan", Fans: cfg.MergeReviewerFan},
		{Key: "discussion_fixer_fan", Fans: cfg.DiscussionFixerFan},
		{Key: "plan_fixer_fan", Fans: cfg.PlanFixerFan},
		{Key: "webster_fixer_fan", Fans: cfg.WebsterFixerFan},
		{Key: "merge_fixer_fan", Fans: cfg.MergeFixerFan},
	}
}

// MigrateConfig rewrites a loom.yaml document for lyx config reconcile and returns the rewritten bytes with one line per rewrite.
// It raises a below-floor or null step_idle_timeout_min to its floor, replaces the retired producer keys, retires skip_merge_review and fan_review and renames the retired reviewer, fixer and fan keys.
// The producer keys:
// discussion_producer twill becomes discussion_lenses twill-discussion, and single, seats and any other value become an empty discussion_lenses;
// plan_producer twill becomes plan_lenses twill-plan, and any other value an empty plan_lenses.
// A document that set discussion_producer also gets discussion_advisors written to match.
// Under seats each string entry n becomes {name: advisor-n, model: <entry>}, which the report line asks the operator to rename.
// Under any other value the list is set to empty even when the document set none, so a lower layer's list stays off where this document turned it off.
// A seats document with no list of its own is left as it is and the report line says where its advisors come from.
// A document holding skip_merge_review was written under the old schema, where a per-perch budget of 0 meant "take review_max_bounces":
// each of the four per-perch budgets that is the integer 0 becomes null, skip_merge_review true becomes merge_max_bounces: 0, and the key is removed.
// A document without skip_merge_review keeps every budget as written, since its 0 already means zero rounds.
// The retired reviewer, fixer and fan keys, review, fix, <segment>_review, <segment>_fix, <segment>_fan and <segment>_fix_fan, are renamed to reviewer_model, fixer_model, <segment>_reviewer_model, <segment>_fixer_model, <segment>_reviewer_fan and <segment>_fixer_fan with their values, and a document already holding the new name keeps its value and drops the old key.
// Before the rename, a document holding webster_review or webster_fix and not merge_reviewer_model or merge_fixer_model gets that Merge key set to the Webster key's value, so Merge-Review keeps the models it ran on.
// The driver_kind, tackler, tackler_timeout_min and tackler_max_spawns keys the retired template spelled out as llm, haiku, 10 and 4 become the values that inherit shed.yaml, and each of the four keys carrying the retired template's comment gets the current one.
// A document without a retired key or comment is returned unchanged, so a migrated file is unchanged on a second pass.
// Everything else is carried through yaml node editing with its comments;
// the loader never calls it.
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
	migrated, shedLines, err := migrateShedOverrides(migrated)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	return migrated, append(rewrites, shedLines...), nil
}

// retiredShedKeyComments holds the line comment the retired loom template carried on each of the four driver and tackler keys, in the order the keys sit in loom.yaml.
// migrateShedOverrides keys on them, so they are never edited.
var retiredShedKeyComments = []struct{ key, comment string }{
	{"driver_kind", "# the driver a task's loom run is seeded with when nothing typed one: llm or go; a seeded run keeps its recorded driver"},
	{"tackler", "# model-spec for the tackler, the cold agent a Go driver asks to settle a stop its rules do not (see contracts/specs/llm-model-spec.md); empty takes haiku"},
	{"tackler_timeout_min", "# minutes one tackler spawn is allowed to run; 0 takes 10"},
	{"tackler_max_spawns", "# tackler spawns a run may use; 0 turns the tackler off, so every stop it would take hands back"},
}

// migrateShedOverrides moves the four driver and tackler keys of document off the retired template's defaults, in two steps of one pass, and returns document itself, with no lines, when neither fires.
// First, when driver_kind, tackler, tackler_timeout_min and tackler_max_spawns hold exactly llm, haiku, 10 and 4 and driver_kind carries the retired comment, they become the inherit values "", "", 0 and null.
// Second, each of the four keys whose line comment is the retired template's text gets the current template's comment, its value untouched.
// A comment edited by hand is kept, and a document without comments, such as a run_settings document, fires neither step.
func migrateShedOverrides(document []byte) ([]byte, []string, error) {
	var tree yaml.Node
	if err := yaml.Unmarshal(document, &tree); err != nil {
		return nil, nil, err
	}
	if tree.Kind != yaml.DocumentNode || len(tree.Content) == 0 || tree.Content[0].Kind != yaml.MappingNode {
		return document, nil, nil
	}
	root := tree.Content[0]

	var current yaml.Node
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &current); err != nil {
		return nil, nil, fmt.Errorf("unmarshal loom config template: %w", err)
	}

	var lines []string
	retiredValues := map[string]string{"driver_kind": "llm", "tackler": "haiku", "tackler_timeout_min": "10", "tackler_max_spawns": "4"}
	allRetired := true
	for key, retired := range retiredValues {
		value := mappingValueNode(root, key)
		if value == nil || value.Kind != yaml.ScalarNode || value.Value != retired {
			allRetired = false
		}
	}
	if allRetired && mappingValueNode(root, "driver_kind").LineComment == retiredShedKeyComments[0].comment {
		inherit := map[string]yaml.Node{
			"driver_kind":         {Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: ""},
			"tackler":             {Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: ""},
			"tackler_timeout_min": {Kind: yaml.ScalarNode, Tag: "!!int", Value: "0"},
			"tackler_max_spawns":  {Kind: yaml.ScalarNode, Tag: "!!null", Value: "~"},
		}
		for key, replacement := range inherit {
			value := mappingValueNode(root, key)
			value.Tag, value.Style, value.Value = replacement.Tag, replacement.Style, replacement.Value
		}
		lines = append(lines, "driver_kind, tackler, tackler_timeout_min and tackler_max_spawns rewritten from the retired llm, haiku, 10 and 4 to the values that inherit shed.yaml")
	}

	for _, retired := range retiredShedKeyComments {
		value := mappingValueNode(root, retired.key)
		if value == nil || value.LineComment != retired.comment {
			continue
		}
		value.LineComment = mappingValueNode(current.Content[0], retired.key).LineComment
		lines = append(lines, retired.key+": comment refreshed to say it overrides shed.yaml")
	}
	if len(lines) == 0 {
		return document, nil, nil
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
	return out.Bytes(), lines, nil
}

// migrateProducerKeys replaces the retired discussion_producer and plan_producer keys of document, as MigrateConfig describes, and returns document itself, with no lines, when it holds neither.
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

// replaceProducerKey renames the retired producer key of root to lensesKey in place, keeping its comments, and gives it fan when the producer was twill and an empty value otherwise.
// value is the producer key's value node, and the returned line reports the rewrite.
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

// migrateAdvisors rewrites root's discussion_advisors to match the retired discussion_producer value producer, and returns the line reporting it, or "" when nothing changed.
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

// advisorEntryNodes returns the entries of a retired-form discussion_advisors value: the sequence's own entries, the lone scalar of a scalar value, and none for an empty or null value, a list of one empty string included.
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

// retireSkipMergeReview rewrites a document holding the retired skip_merge_review key onto the zero-round budgets and returns the rewritten bytes with one line per rewrite.
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
	for _, perch := range reviewPerches {
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
		mergeKey := reviewbudget.SoftKey(string(PerchMerge))
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

// retireFanReview removes the retired fan_review key from document and returns the rewritten bytes with one line reporting it.
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

// copyWebsterModelsOntoMerge sets merge_reviewer_model and merge_fixer_model of document to the values of its retired webster_review and webster_fix, each only when the document holds the Webster key and not the Merge one.
// When the document also holds the renamed webster_reviewer_model or webster_fixer_model, that value is copied instead, since it is the one the rename keeps.
// Merge-Review ran on Webster-Review's models before it had keys of its own, so the copy keeps it on them.
// It returns the rewritten bytes with one line per copied key, and document itself, with no lines, when nothing is copied.
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
	for _, copied := range []struct{ webster, websterRenamed, merge, darn string }{
		{"webster_review", "webster_reviewer_model", "merge_reviewer_model", "darn_reviewer_model"},
		{"webster_fix", "webster_fixer_model", "merge_fixer_model", "darn_fixer_model"},
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
		value.LineComment = mappingValueNode(template.Content[0], copied.merge).LineComment
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: copied.merge}, &value)
		lines = append(lines, fmt.Sprintf("%s: set to %s's value, the models Merge-Review ran on; Darn-Review now reads %s in darn.yaml, which this rewrite cannot set", copied.merge, source, copied.darn))
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

// ShedOverride returns the driver and tackler keys of loom.yaml as the override of shed.yaml's.
func (c Config) ShedOverride() sheddriver.Override {
	return sheddriver.Override{
		DriverKind:        c.DriverKind,
		Tackler:           c.Tackler,
		TacklerTimeoutMin: c.TacklerTimeoutMin,
		TacklerMaxSpawns:  c.TacklerMaxSpawns,
	}
}

// RecipeOverride loads the shed override of a run's recipe from the config layers under dirs:
// darn.yaml for shedrun.RecipeDarn, and loom.yaml for shedrun.RecipeLoom and the empty recipe.
func RecipeOverride(dirs configengine.Dirs, recipe string) (sheddriver.Override, error) {
	if recipe == shedrun.RecipeDarn {
		cfg, err := darnengine.LoadConfig(dirs, "darn")
		if err != nil {
			return sheddriver.Override{}, err
		}
		return cfg.ShedOverride(), nil
	}
	cfg, err := LoadConfig(dirs, "loom")
	if err != nil {
		return sheddriver.Override{}, err
	}
	return cfg.ShedOverride(), nil
}

// ValidateLocalValues checks the values of the driver_kind, tackler, tackler_timeout_min and tackler_max_spawns keys a local-layer document sets, and of every review budget and hard-cap key through reviewbudget, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	if err := sheddriver.CheckLocalOverride("loom", document); err != nil {
		return err
	}
	var budgets map[string]any
	if err := yaml.Unmarshal(document, &budgets); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal loom local values: %w", err))
	}
	return reviewbudget.CheckLocal(budgets, reviewPerchNames())
}

// LoadConfig loads and unmarshals configuration for the loom module strictly through the layer stack under dirs.
// It validates model-spec grammar at load time, and reads burler.yaml and models.yaml through the same dirs.
// review_max_bounces and the per-perch budgets are refused below 0, null taking review_max_bounces and 0 giving zero rounds, and each refusal names the command for the layer its value comes from.
func LoadConfig(dirs configengine.Dirs, module string) (Config, error) {
	spec := ConfigSpec()
	spec.Name = module
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		var shape *burlerengine.ModelSpecShapeError
		if errors.As(err, &shape) {
			return Config{}, fmt.Errorf("loom config key %q: %w", configengine.KeyAtLine(resolved, shape.Line), err)
		}
		var fanShape *burlerengine.FanShapeError
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

	if _, err := modelspec.Parse(cfg.Plan); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "plan", err)
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

	if err := burlerengine.ValidateFanKeys(dirs, "loom", fanKeys(cfg)); err != nil {
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

	// The judge, seed and rework override keys are validated only when non-empty:
	// an empty value takes its fallback key at run start and must load cleanly.
	for _, override := range []struct {
		key   string
		value string
	}{
		{"seed", cfg.Seed},
		{"discussion_judge", cfg.DiscussionJudge},
		{"plan_judge", cfg.PlanJudge},
		{"webster_judge", cfg.WebsterJudge},
		{"discussion_seed", cfg.DiscussionSeed},
		{"plan_seed", cfg.PlanSeed},
		{"webster_seed", cfg.WebsterSeed},
		{"rework", cfg.Rework},
	} {
		if override.value == "" {
			continue
		}
		if _, err := modelspec.Parse(override.value); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", override.key, err)
		}
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

	// arbiter is validated only when non-empty: an empty value takes ResolveArbiter's default.
	if cfg.Arbiter != "" {
		if _, err := modelspec.Parse(cfg.Arbiter); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "arbiter", err)
		}
	}

	// alignment is validated only when non-empty: an empty value takes ResolveAlignment's default.
	if cfg.Alignment != "" {
		if _, err := modelspec.Parse(cfg.Alignment); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w", "alignment", err)
		}
	}

	if cfg.AlignmentTimeoutMin < 0 {
		return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; set it to 0 for the %d minute default, or to a positive integer below parent_review_wait_min", "alignment_timeout_min", cfg.AlignmentTimeoutMin, alignmentDefaultTimeoutMin)
	}

	// Zero is a meaning on both arbiter knobs, so only a negative value is refused.
	for _, knob := range []struct {
		key   string
		value int
		zero  string
	}{
		{"arbiter_timeout_min", cfg.ArbiterTimeoutMin, "0 for the 30 minute default"},
		{"arbiter_max_spawns", cfg.ArbiterMaxSpawns, "0 to turn the arbiter off"},
	} {
		if knob.value < 0 {
			return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; set it to %s, or to a positive integer, in loom.yaml", knob.key, knob.value, knob.zero)
		}
	}

	if err := sheddriver.CheckOverride("loom", cfg.ShedOverride()); err != nil {
		return Config{}, err
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

	// The agent must give up before the parent-review wait does, or the wait would pass with no verdict while the agent still runs.
	// The check runs on the merged layers, so a local layer, run_settings included, is checked like the hub file.
	alignmentTimeoutMin := cfg.AlignmentTimeoutMin
	if alignmentTimeoutMin == 0 {
		alignmentTimeoutMin = alignmentDefaultTimeoutMin
	}
	if alignmentTimeoutMin >= cfg.ParentReviewWaitMin {
		return Config{}, fmt.Errorf("loom config key %q: resolved timeout %d minutes (set %d) is not below parent_review_wait_min %d; set alignment_timeout_min below parent_review_wait_min, or raise parent_review_wait_min in the hub's loom.yaml", "alignment_timeout_min", alignmentTimeoutMin, cfg.AlignmentTimeoutMin, cfg.ParentReviewWaitMin)
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
		RunSoft: cfg.ReviewMaxBounces,
		RunHard: cfg.ReviewHardMaxBounces,
		Perches: reviewPerchNames(),
		PerchSoft: map[string]*int{
			string(PerchDiscussion): cfg.DiscussionMaxBounces,
			string(PerchPlan):       cfg.PlanMaxBounces,
			string(PerchWebster):    cfg.WebsterMaxBounces,
			string(PerchMerge):      cfg.MergeMaxBounces,
		},
		PerchHard: map[string]int{
			string(PerchDiscussion): cfg.DiscussionHardMaxBounces,
			string(PerchPlan):       cfg.PlanHardMaxBounces,
			string(PerchWebster):    cfg.WebsterHardMaxBounces,
			string(PerchMerge):      cfg.MergeHardMaxBounces,
		},
	}); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
