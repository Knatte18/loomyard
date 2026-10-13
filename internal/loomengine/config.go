// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which uses internal/configengine.Load with ConfigTemplate() to strictly validate and resolve loom's config file,
// then validates the discussion, plan, judge, friction, and driver role model-specs and every entry of the review and fix model-spec lists' grammar via modelspec.Parse, plus every entry of the six per-segment lists (discussion_review, discussion_fix, plan_review, plan_fix, webster_review, webster_fix) that are set, an empty value meaning the run-wide list, rejects a negative value on each of the four timeout knobs, and rejects a parent_review_wait_min, review_circling_checkpoint, review_max_bounces or review_hard_max_bounces below 1, rejects a discussion_max_bounces, plan_max_bounces, webster_max_bounces, merge_max_bounces or any per-perch hard cap below 0, rejects a perch whose resolved hard cap is below its resolved budget, validates every entry of fan_review, and resolves each non-empty discussion_fan and plan_fan through burlerengine.ResolveFan,
// so a mistake in any of those validated keys fails loud at load time rather than hours into a run when the discussion, plan, review, judge, friction, or driver producer first spawns.
// step_idle_timeout_min is the minutes a --until-stop step may show no activity before the loop kills it; a value below 1 is raised to 1 with one Warn and never refused.
// discussion_fan and plan_fan each name a fan from burler.yaml and turn the lens fan on for Discussion-Review and Plan-Review; empty, the default, runs that segment solo.
// fan_review is the reviewer model-spec list of a fanned segment, and the forks' model too, since forks run on the reviewer session's model.
// There is no webster_fan key: Webster-Review always runs solo.
// discussion_max_bounces, plan_max_bounces and webster_max_bounces set one perch's bounce budget, and 0 takes review_max_bounces;
// review_hard_max_bounces is the round number at or past which no perch continues a round, refused below 1, and discussion_hard_max_bounces, plan_hard_max_bounces, webster_hard_max_bounces and merge_hard_max_bounces set one perch's own, where 0 takes review_hard_max_bounces and a negative value is refused;
// a perch whose resolved hard cap is below its resolved budget is refused too, and every one of these refusals names the command that changes the key in the layer its value comes from.
// discussion_auto_accept, plan_auto_accept and webster_auto_accept make that perch accept a harvested round at a spent budget instead of running the soft-cap rule.
// merge_max_bounces, merge_hard_max_bounces and merge_auto_accept do the same for the Merge-Review perch, which sends a round whose ledger holds an open BLOCKING finding to the soft-cap rule instead of accepting it;
// skip_merge_review skips only that review, and Merge-In, the parent lock and the reroute on a moved parent still run.
// discussion_producer selects the Discussion-Write row's producer and must be single, seats or twill.
// plan_producer selects the Plan-Write row's producer, single or twill, and an empty value reads as single.
// discussion_advisors lists the seats producer's advisor model-specs; every entry is resolved through the model registry at load, whatever discussion_producer holds, and an unset list loads as no advisors.
// Under seats, no advisors runs the chair alone, deciding each question itself.
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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
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

// DiscussionReadyMarker returns the path to the twill writer's ready marker.
// It is untracked state the writer creates once its decision record passes the self-check, and the condition its lens forks wait on.
// It sits under the anchor's lyxdirs.DotLyxDirName at the subpath mirroring DiscussionDir.
func DiscussionReadyMarker(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, discussionDirName, "decision-record.md.ready")
}

// RemoveDiscussionReadyMarker removes the ready marker and treats a missing one as success.
// It runs before every fresh Discussion-Write spawn, so a marker left by an earlier spawn never wakes a later spawn's forks.
func RemoveDiscussionReadyMarker(l *lyxcwd.Location) error {
	if err := os.Remove(DiscussionReadyMarker(l)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loom: remove discussion ready marker: %w", err)
	}
	return nil
}

// PlanReadyMarker returns the path to the twill plan writer's ready marker.
// It is untracked state the writer creates once its plan passes the self-check, and the condition its lens forks wait on.
// It sits under the anchor's lyxdirs.DotLyxDirName at the subpath mirroring the plan directory.
func PlanReadyMarker(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, planparser.PlanDirName, filepath.Base(planparser.PlanOverview(l.AnchorPath()))+".ready")
}

// RemovePlanReadyMarker removes the plan ready marker and treats a missing one as success.
// It runs before every fresh Plan-Write spawn, so a marker left by an earlier spawn never wakes a later spawn's forks.
func RemovePlanReadyMarker(l *lyxcwd.Location) error {
	if err := os.Remove(PlanReadyMarker(l)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("loom: remove plan ready marker: %w", err)
	}
	return nil
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

const (
	// DiscussionProducerSingle selects the single-agent writer for the Discussion-Write row.
	DiscussionProducerSingle = "single"
	// DiscussionProducerSeats selects the chair with its advisors for the Discussion-Write row.
	DiscussionProducerSeats = "seats"
	// DiscussionProducerTwill selects the single-agent writer with lens forks for the Discussion-Write row.
	DiscussionProducerTwill = "twill"
)

const (
	// PlanProducerSingle selects the single-agent writer for the Plan-Write row.
	PlanProducerSingle = "single"
	// PlanProducerTwill selects the single-agent writer with lens forks for the Plan-Write row.
	PlanProducerTwill = "twill"
)

// Config represents the resolved loom.yaml configuration: role model-specs and timeout knobs.
type Config struct {
	Discussion            string        `yaml:"discussion"`
	DiscussionTimeoutMin  int           `yaml:"discussion_timeout_min"`
	DiscussionInteractive bool          `yaml:"discussion_interactive"`
	DiscussionProducer    string        `yaml:"discussion_producer"`
	DiscussionAdvisors    ModelSpecList `yaml:"discussion_advisors"`
	Plan                  string        `yaml:"plan"`
	PlanTimeoutMin        int           `yaml:"plan_timeout_min"`
	PlanProducer          string        `yaml:"plan_producer"`
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
	Judge                 string        `yaml:"judge"`
	Seed                  string        `yaml:"seed"`
	DiscussionJudge       string        `yaml:"discussion_judge"`
	PlanJudge             string        `yaml:"plan_judge"`
	WebsterJudge          string        `yaml:"webster_judge"`
	DiscussionSeed        string        `yaml:"discussion_seed"`
	PlanSeed              string        `yaml:"plan_seed"`
	WebsterSeed           string        `yaml:"webster_seed"`
	Rework                string        `yaml:"rework"`
	ReviewTimeoutMin      int           `yaml:"review_timeout_min"`
	Friction              string        `yaml:"friction"`
	FrictionTimeoutMin    int           `yaml:"friction_timeout_min"`
	Driver                string        `yaml:"driver"`
	ParentReviewWaitMin   int           `yaml:"parent_review_wait_min"`
	StepIdleTimeoutMin    int           `yaml:"step_idle_timeout_min"`

	// The arbiter's model-spec, timeout and spawn budget.
	// An empty Arbiter takes opus[high], a zero ArbiterTimeoutMin takes 30 minutes, and a zero ArbiterMaxSpawns turns the arbiter off.
	Arbiter           string `yaml:"arbiter"`
	ArbiterTimeoutMin int    `yaml:"arbiter_timeout_min"`
	ArbiterMaxSpawns  int    `yaml:"arbiter_max_spawns"`

	// The driver a task's loom run is seeded with when nothing typed one, and the tackler's model-spec, timeout and spawn budget.
	// An empty DriverKind reads as llm; an empty Tackler takes haiku, a zero TacklerTimeoutMin takes 10 minutes, and a zero TacklerMaxSpawns turns the tackler off.
	DriverKind        string `yaml:"driver_kind"`
	Tackler           string `yaml:"tackler"`
	TacklerTimeoutMin int    `yaml:"tackler_timeout_min"`
	TacklerMaxSpawns  int    `yaml:"tackler_max_spawns"`

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
	// A budget of 0 takes ReviewMaxBounces and a hard cap of 0 takes ReviewHardMaxBounces;
	// a flag true makes the perch accept a harvested round at a spent budget instead of running the soft-cap rule.
	DiscussionMaxBounces     int  `yaml:"discussion_max_bounces"`
	PlanMaxBounces           int  `yaml:"plan_max_bounces"`
	WebsterMaxBounces        int  `yaml:"webster_max_bounces"`
	DiscussionHardMaxBounces int  `yaml:"discussion_hard_max_bounces"`
	PlanHardMaxBounces       int  `yaml:"plan_hard_max_bounces"`
	WebsterHardMaxBounces    int  `yaml:"webster_hard_max_bounces"`
	DiscussionAutoAccept     bool `yaml:"discussion_auto_accept"`
	PlanAutoAccept           bool `yaml:"plan_auto_accept"`
	WebsterAutoAccept        bool `yaml:"webster_auto_accept"`

	// The Merge-Review perch's budget, hard cap and auto-accept flag, and the switch that skips only the review.
	// A budget of 0 takes ReviewMaxBounces and a hard cap of 0 takes ReviewHardMaxBounces.
	MergeMaxBounces     int  `yaml:"merge_max_bounces"`
	MergeHardMaxBounces int  `yaml:"merge_hard_max_bounces"`
	MergeAutoAccept     bool `yaml:"merge_auto_accept"`
	SkipMergeReview     bool `yaml:"skip_merge_review"`
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
	return []string{"review", "fix", "discussion_review", "discussion_fix", "plan_review", "plan_fix", "webster_review", "webster_fix", "fan_review", "discussion_advisors"}
}

// ConfigSpec returns loom's layered-load spec.
// Only the keys that can switch off or hollow a review or approval gate are barred from the local layer.
// The agent's approve at PR-Gate lands on the parent with no later gate, so a weaker alignment model, a re-opened auto-landing (alignment_pr_approve) or a local skip_alignment: false under a hub true hollows the approval gate;
// a shorter alignment_timeout_min only routes a review to the parent.
func ConfigSpec() configengine.Spec {
	none := configengine.CategoryNone
	guard := configengine.CategoryGuard
	return configengine.Spec{
		Name:     "loom",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		Categories: map[string]configengine.Category{
			"discussion":                  none,
			"discussion_timeout_min":      none,
			"discussion_interactive":      none,
			"discussion_producer":         guard,
			"discussion_advisors":         none,
			"plan":                        none,
			"plan_timeout_min":            none,
			"plan_producer":               guard,
			"review":                      none,
			"fix":                         none,
			"discussion_review":           none,
			"discussion_fix":              none,
			"plan_review":                 none,
			"plan_fix":                    none,
			"webster_review":              none,
			"webster_fix":                 none,
			"discussion_fan":              guard,
			"plan_fan":                    guard,
			"fan_review":                  none,
			"judge":                       none,
			"seed":                        none,
			"discussion_judge":            none,
			"plan_judge":                  none,
			"webster_judge":               none,
			"discussion_seed":             none,
			"plan_seed":                   none,
			"webster_seed":                none,
			"rework":                      none,
			"review_timeout_min":          none,
			"friction":                    none,
			"friction_timeout_min":        none,
			"driver":                      none,
			"arbiter":                     none,
			"arbiter_timeout_min":         none,
			"arbiter_max_spawns":          none,
			"driver_kind":                 none,
			"tackler":                     none,
			"tackler_timeout_min":         none,
			"tackler_max_spawns":          none,
			"alignment":                   guard,
			"alignment_timeout_min":       none,
			"skip_alignment":              guard,
			"alignment_pr_approve":        guard,
			"parent_review_wait_min":      guard,
			"step_idle_timeout_min":       none,
			"review_circling_checkpoint":  none,
			"review_max_bounces":          none,
			"review_hard_max_bounces":     none,
			"discussion_max_bounces":      none,
			"plan_max_bounces":            none,
			"webster_max_bounces":         none,
			"discussion_hard_max_bounces": none,
			"plan_hard_max_bounces":       none,
			"webster_hard_max_bounces":    none,
			"discussion_auto_accept":      guard,
			"plan_auto_accept":            guard,
			"webster_auto_accept":         guard,
			"merge_max_bounces":           none,
			"merge_hard_max_bounces":      none,
			"merge_auto_accept":           guard,
			"skip_merge_review":           guard,
		},
	}
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

// discussionAdvisorsKey is the loom.yaml key holding the seat-table producer's advisor model-specs.
const discussionAdvisorsKey = "discussion_advisors"

// advisorWayForward is the way forward every discussion_advisors refusal about its entries ends with.
const advisorWayForward = "set the key to a list of model-specs the registry defines, one per advisor, or to an empty list for no advisors"

// validateAdvisorSpecs rejects an empty entry, one that is not a model-spec and one the registry does not define, naming key and the 1-based entry index.
func validateAdvisorSpecs(key string, specs ModelSpecList, reg modelspec.Registry) error {
	for i, raw := range specs {
		spec, err := modelspec.Parse(raw)
		if err != nil {
			return fmt.Errorf("loom config key %q entry %d: %w; %s", key, i+1, err, advisorWayForward)
		}
		if _, err := reg.Resolve(spec); err != nil {
			return fmt.Errorf("loom config key %q entry %d: %w; %s", key, i+1, err, advisorWayForward)
		}
	}
	return nil
}

// validateFanKeys resolves each non-empty fan key of cfg through burler.yaml, which falls back per name to the embedded template.
// A fan that does not resolve is refused naming the key, the unknown name and the fans that exist,
// and an unreadable burler.yaml is refused naming the first set key.
// With both fan keys empty, burler.yaml is never read.
func validateFanKeys(dirs configengine.Dirs, cfg Config) error {
	var burlerCfg *burlerengine.Config
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
		if burlerCfg == nil {
			loaded, err := burlerengine.LoadConfig(dirs)
			if err != nil {
				return fmt.Errorf("loom config key %q: %w; fix burler.yaml, or set the key to empty to run the segment solo", fan.key, err)
			}
			burlerCfg = &loaded
		}
		if _, err := burlerengine.ResolveFan(*burlerCfg, fan.name); err != nil {
			return fmt.Errorf("loom config key %q: %w; set the key to one of those fans, or to empty to run the segment solo", fan.key, err)
		}
	}
	return nil
}

// MigrateConfig raises a below-floor or null step_idle_timeout_min in a loom.yaml document to its floor, for lyx config reconcile, and returns the rewritten bytes with one line describing the rewrite.
// Everything else is carried through with its comments;
// the loader never calls it.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	rewritten, line, err := yamlengine.RaiseIntFloor(existing, "step_idle_timeout_min", stepIdleTimeoutFloorMin)
	if err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate loom config: %w", err))
	}
	if line == "" {
		return rewritten, nil, nil
	}
	return rewritten, []string{line}, nil
}

// validateDriverKind refuses a driver_kind other than empty, llm or go, through shedrun.ValidateDriver so the vocabulary stays shedrun's.
func validateDriverKind(driverKind string) error {
	if driverKind == "" {
		return nil
	}
	if err := shedrun.ValidateDriver(driverKind); err != nil {
		return fmt.Errorf("loom config key %q: %w; set driver_kind to llm or go in loom.yaml or the task's run_settings", "driver_kind", err)
	}
	return nil
}

// validateTacklerKnobs refuses a negative tackler_timeout_min or tackler_max_spawns; zero is a meaning on both.
func validateTacklerKnobs(timeoutMin, maxSpawns int) error {
	for _, knob := range []struct {
		key   string
		value int
		zero  string
	}{
		{"tackler_timeout_min", timeoutMin, "0 for the 10 minute default"},
		{"tackler_max_spawns", maxSpawns, "0 to turn the tackler off"},
	} {
		if knob.value < 0 {
			return fmt.Errorf("loom config key %q: must not be negative, got %d; set it to %s, or to a positive integer, in loom.yaml or the task's run_settings", knob.key, knob.value, knob.zero)
		}
	}
	return nil
}

// ValidateLocalValues checks the values of the driver_kind, tackler_timeout_min and tackler_max_spawns keys a local-layer document sets, and leaves every other key to LoadConfig.
// A refusal is marked configengine.ErrInvalid and names the key.
func ValidateLocalValues(document []byte) error {
	var set struct {
		DriverKind        *string `yaml:"driver_kind"`
		TacklerTimeoutMin *int    `yaml:"tackler_timeout_min"`
		TacklerMaxSpawns  *int    `yaml:"tackler_max_spawns"`
	}
	if err := yaml.Unmarshal(document, &set); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal loom local values: %w", err))
	}
	if set.DriverKind != nil {
		if err := validateDriverKind(*set.DriverKind); err != nil {
			return configengine.MarkInvalid(err)
		}
	}
	var timeoutMin, maxSpawns int
	if set.TacklerTimeoutMin != nil {
		timeoutMin = *set.TacklerTimeoutMin
	}
	if set.TacklerMaxSpawns != nil {
		maxSpawns = *set.TacklerMaxSpawns
	}
	if err := validateTacklerKnobs(timeoutMin, maxSpawns); err != nil {
		return configengine.MarkInvalid(err)
	}
	return nil
}

// LoadConfig loads and unmarshals configuration for the loom module strictly through the layer stack under dirs.
// It validates model-spec grammar at load time, and reads burler.yaml and models.yaml through the same dirs.
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
		var shape *modelSpecShapeError
		if errors.As(err, &shape) {
			key := keyAtLine(resolved, shape.line)
			if key == discussionAdvisorsKey {
				return Config{}, fmt.Errorf("loom config key %q: value at line %d is neither a model-spec nor a list of model-specs; %s", key, shape.line, advisorWayForward)
			}
			return Config{}, fmt.Errorf("loom config key %q: %w", key, err)
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

	if err := validateFanKeys(dirs, cfg); err != nil {
		return Config{}, err
	}

	if isUnsetModelSpecList(cfg.DiscussionAdvisors) {
		cfg.DiscussionAdvisors = nil
	} else {
		reg, err := modelspec.LoadRegistry(dirs)
		if err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w; fix models.yaml, or set the key to an empty list for no advisors", discussionAdvisorsKey, err)
		}
		if err := validateAdvisorSpecs(discussionAdvisorsKey, cfg.DiscussionAdvisors, reg); err != nil {
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

	if err := validateDriverKind(cfg.DriverKind); err != nil {
		return Config{}, err
	}

	// tackler is validated only when non-empty: an empty value takes ResolveTackler's default.
	if cfg.Tackler != "" {
		if _, err := modelspec.Parse(cfg.Tackler); err != nil {
			return Config{}, fmt.Errorf("loom config key %q: %w; set it to a model-spec the registry defines, or to empty to take haiku", "tackler", err)
		}
	}

	if err := validateTacklerKnobs(cfg.TacklerTimeoutMin, cfg.TacklerMaxSpawns); err != nil {
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

	// A per-perch budget of 0 takes review_max_bounces, validated above, so only a negative value is refused here.
	for _, knob := range []struct {
		key   string
		value int
	}{
		{"discussion_max_bounces", cfg.DiscussionMaxBounces},
		{"plan_max_bounces", cfg.PlanMaxBounces},
		{"webster_max_bounces", cfg.WebsterMaxBounces},
		{"merge_max_bounces", cfg.MergeMaxBounces},
	} {
		if knob.value < 0 {
			return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; set it to 0 to take review_max_bounces, or to a positive integer, in loom.yaml", knob.key, knob.value)
		}
	}

	// A hard cap below 1 has no way to hold, so it is refused with the route of the layer its value comes from.
	if cfg.ReviewHardMaxBounces < 1 {
		route, err := CapRoute(dirs, "review_hard_max_bounces")
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("loom config key %q: must be at least 1, got %d; way forward: %s, then run \"lyx loom resume\"", "review_hard_max_bounces", cfg.ReviewHardMaxBounces, route)
	}

	// A per-perch hard cap of 0 takes review_hard_max_bounces, so only a negative value is refused here.
	for _, knob := range []struct {
		key   string
		value int
	}{
		{"discussion_hard_max_bounces", cfg.DiscussionHardMaxBounces},
		{"plan_hard_max_bounces", cfg.PlanHardMaxBounces},
		{"webster_hard_max_bounces", cfg.WebsterHardMaxBounces},
		{"merge_hard_max_bounces", cfg.MergeHardMaxBounces},
	} {
		if knob.value < 0 {
			route, err := CapRoute(dirs, knob.key)
			if err != nil {
				return Config{}, err
			}
			return Config{}, fmt.Errorf("loom config key %q: must not be negative, got %d; set it to 0 to take review_hard_max_bounces, or to a positive integer; way forward: %s, then run \"lyx loom resume\"", knob.key, knob.value, route)
		}
	}

	// Equal caps load: a round at the budget is already at the hard cap.
	for _, perch := range []Perch{PerchDiscussion, PerchPlan, PerchWebster, PerchMerge} {
		caps := cfg.Caps(perch)
		if caps.Hard >= caps.Soft {
			continue
		}
		hardRoute, err := CapRoute(dirs, caps.HardKey)
		if err != nil {
			return Config{}, err
		}
		softRoute, err := CapRoute(dirs, caps.SoftKey)
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("loom config key %q: resolved hard cap %d is below %q's resolved soft cap %d; way forward: raise the hard cap: %s, or lower the soft cap: %s, then run \"lyx loom resume\"", caps.HardKey, caps.Hard, caps.SoftKey, caps.Soft, hardRoute, softRoute)
	}

	switch cfg.DiscussionProducer {
	case DiscussionProducerSingle, DiscussionProducerSeats, DiscussionProducerTwill:
	default:
		return Config{}, fmt.Errorf("loom config key %q: unknown value %q; set it to %q, %q or %q", "discussion_producer", cfg.DiscussionProducer, DiscussionProducerSingle, DiscussionProducerSeats, DiscussionProducerTwill)
	}

	if cfg.PlanProducer == "" {
		cfg.PlanProducer = PlanProducerSingle
	}
	switch cfg.PlanProducer {
	case PlanProducerSingle, PlanProducerTwill:
	default:
		return Config{}, fmt.Errorf("loom config key %q: unknown value %q; set it to %q or %q", "plan_producer", cfg.PlanProducer, PlanProducerSingle, PlanProducerTwill)
	}

	return cfg, nil
}
