// attach.go implements Runner.Attach: the crash-recovery half of the run loop, which answers "is
// there a still-live, never-terminated run for this exact output-file set" before a caller respawns
// a fresh agent over one that may already be working. It scans the run-dir root for a matching
// run.json, dispositions each match against reed's own liveness answer, and — on exactly one live
// match — reconstructs a *Run over the persisted state and hands it to Wait, never calling Start.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// Attach looks for a still-live, never-terminated run matching spec's output files and, if found,
// waits on it instead of starting a second agent.
// The bool reports whether a live run was found: false comes back with a zero Result and a nil
// error, meaning "nothing to attach to, start one"; true means a Run was reconstructed and waited on
// (whose own error, if any, is returned alongside).
// Attach never calls Start — it reconstructs a *Run directly over a matched run.json — so
// sweepOrphansOpportunistic never runs on this path.
func (r *Runner) Attach(spec Spec) (Result, bool, error) {
	if r.toldErr != nil {
		return Result{}, false, r.toldErr
	}

	normalized, err := normalizeAttachSpec(spec, r.worktreeRoot, r.cfg)
	if err != nil {
		return Result{}, false, err
	}

	// Told-Geometry / Lyxdirs Single-Declarer Invariants: the scan root comes from the existing
	// runDirRoot helper, never a path built here.
	root := runDirRoot(r.cfg, r.anchorPath)
	candidates, err := collectAttachCandidates(root, normalized.OutputFiles)
	if err != nil {
		return Result{}, false, err
	}

	// Zero candidates returns immediately, without reading reed state at all. This precedence is
	// load-bearing: reedengine.LoadState returns (nil, nil) for an absent reed.json, and with Attach
	// probed on every SingleLLMProducer.Call, a worktree that has not yet had a reed session would
	// otherwise hard-error on its very first Discussion-Write or Plan-Write call, with nothing to
	// attach to and nothing wrong.
	if len(candidates) == 0 {
		return Result{}, false, nil
	}

	// First reed read: does reed have a state table at all. This must not be answered via
	// ReedOps.Status() — reedengine's loadOrInitStateLocked substitutes an empty &ReedState{} for a
	// not-found file, so Status() succeeds with zero strands for an absent state file,
	// indistinguishable from a healthy table that simply does not list this guid.
	dotLyxDir := filepath.Join(r.anchorPath, lyxdirs.DotLyxDirName)
	st, err := reedengine.LoadState(dotLyxDir)
	if err != nil {
		warnAttachCandidates(candidates, "shuttle: attach: load reed state failed", err)
		return Result{}, false, fmt.Errorf("shuttle: attach: load reed state at %s: %w — an absent or unreadable strand table is not evidence any run is dead; check \"lyx reed status\"", dotLyxDir, err)
	}
	if st == nil {
		warnAttachCandidates(candidates, "shuttle: attach: no reed state file", nil)
		return Result{}, false, fmt.Errorf("shuttle: attach: no reed state file at %s — an absent strand table is not evidence any of the %d matching run dir(s) are dead; check \"lyx reed status\"", dotLyxDir, len(candidates))
	}

	// Second reed read, only once the first reported present: is this guid tracked, and is its pane
	// live.
	status, err := r.reed.Status()
	if err != nil {
		warnAttachCandidates(candidates, "shuttle: attach: reed status failed", err)
		return Result{}, false, fmt.Errorf("shuttle: attach: reed status: %w — check \"lyx reed status\"", err)
	}

	minAge := 2 * time.Duration(r.cfg.StartupTimeoutS) * time.Second
	now := r.clock.Now()

	var attachable, errored []attachCandidate
	for _, c := range candidates {
		switch dispositionCandidate(c, status.Strands, normalized, minAge, now) {
		case verdictAttachable:
			attachable = append(attachable, c)
		case verdictError:
			errored = append(errored, c)
		}
	}

	// The multiplicity rule applies only to the surviving attachable set, never to raw matches — see
	// candidate-evaluation-order.
	if len(errored) > 0 {
		return Result{}, false, fmt.Errorf("shuttle: attach: %d candidate run dir(s) under %s cannot be confirmed dead or alive (untracked or with a cleared pane binding, younger than %s): %s — check \"lyx reed status\" and either wait or clear the stale directory by hand", len(errored), root, minAge, joinRunDirs(errored))
	}
	if len(attachable) > 1 {
		return Result{}, false, fmt.Errorf("shuttle: attach: %d live runs match the same output files, refusing to pick one: %s", len(attachable), joinRunDirs(attachable))
	}
	if len(attachable) == 0 {
		return Result{}, false, nil
	}

	candidate := attachable[0]
	run := &Run{
		runner: r,
		// spec is the caller's own normalized spec, never one rebuilt from run.json: RunState
		// persists only OutputFiles out of the whole Spec, so rebuilding one is not possible.
		spec: normalized,
		// runDir and state come from the matched candidate, so Wait reads the persisted EventsPath,
		// StrandGUID, and SessionID.
		runDir: candidate.runDir,
		state:  candidate.state,
		// offset starts at 0, deliberately replaying the whole events.jsonl: seeding at EOF would
		// mean a terminal Stop that landed while the driver was down is never observed, converting a
		// completed step into an OutcomeTimeout failure — and a replayed backlog ending in an ask is
		// correct in both AwaitOperator modes.
		offset: 0,
		clock:  r.clock,
		// deadline is a fresh now+Timeout computed at attach time, never CreatedAt+Timeout: a run
		// that hit OutcomeTimeout leaves both its strand and its run dir behind, and inheriting
		// CreatedAt would re-attach and re-time-it-out on every resume forever.
		deadline: r.clock.Now().Add(normalized.Timeout),
		// attached seeds Wait's started so it never re-runs the startup probe against a live,
		// mid-turn pane.
		attached: true,
	}

	// Live-Substrate Spawn Observability invariant: a re-attach to a live agent is as instrumented as
	// a spawn.
	logger.Info("shuttle: run attached", "runDir", candidate.runDir, "strandGUID", candidate.state.StrandGUID, "sessionID", candidate.state.SessionID)

	result, err := run.Wait()
	return result, true, err
}

// normalizeAttachSpec returns a normalized copy of spec, performing exactly three of
// Spec.validate's normalizations plus one of its checks — never the full validate, whose
// reject-if-already-exists check would refuse every attach whose agent has written one of its
// output files, a normal mid-interview state.
func normalizeAttachSpec(spec Spec, worktreeRoot string, cfg Config) (Spec, error) {
	// A negative Timeout can only be a caller mistake (0 is the documented "defer to config" value).
	// Checked before scanning and before any reed read: on the attach path its consequence is worse
	// than on the start path it was written for — an attached run's deadline would already be in the
	// past, reporting a LIVE interview as timed out on its very first tick.
	if spec.Timeout < 0 {
		return Spec{}, fmt.Errorf("shuttle: spec.Timeout must not be negative (got %s); use 0 for the config default run_timeout_min", spec.Timeout)
	}

	// Resolve every relative OutputFiles entry against worktreeRoot with the same rule
	// Spec.validate uses. An unresolved relative entry never set-matches a run.json's
	// always-absolute record, so skipping this would fail to match a caller that passed relative
	// entries.
	resolved := make([]string, len(spec.OutputFiles))
	for i, f := range spec.OutputFiles {
		if filepath.IsAbs(f) {
			resolved[i] = f
			continue
		}
		resolved[i] = filepath.Clean(filepath.Join(worktreeRoot, f))
	}
	spec.OutputFiles = resolved

	// A zero Timeout makes the attached run's deadline now+0, classifying OutcomeTimeout on its very
	// first tick — the exact footgun Timeout's own doc comment warns about, reintroduced on the
	// attach path if left unhandled.
	if spec.Timeout == 0 {
		spec.Timeout = time.Duration(cfg.RunTimeoutMin) * time.Minute
	}

	// An empty Anchor makes every binding-cleared strand take checkLivenessTick's hidden-strand
	// carve-out and classify OutcomeDied instead of surfacing errStrandPaneBindingCleared. This
	// normalization matters to the attached RUN's own liveness checks, not to matching — see
	// attach-reconstructs-the-run-explicitly.
	if spec.Display.Anchor == "" {
		spec.Display.Anchor = render.AnchorBelowParent
	}

	return spec, nil
}

// attachCandidate is one run.json whose OutputFiles set-matched the normalized spec's, collected
// during Attach's scan phase.
type attachCandidate struct {
	runDir   string
	dirMtime time.Time
	state    RunState
}

// collectAttachCandidates scans <root>/*/run.json for records whose OutputFiles set-match
// outputFiles, with the same shape and skip-discipline as findRunByStrand: iterate os.ReadDir, skip
// non-directories, and skip an unreadable or absent run.json rather than aborting the scan. A root
// that does not exist is not an error — it is zero candidates.
func collectAttachCandidates(root string, outputFiles []string) ([]attachCandidate, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("shuttle: attach: read run dir root: %w", err)
	}

	var candidates []attachCandidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil || !found {
			// Skip: an unreadable or truncated run.json mid-scan must not abort the scan for every
			// other candidate, exactly as findRunByStrand treats it.
			continue
		}
		if !outputFilesSetEqual(rs.OutputFiles, outputFiles) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, attachCandidate{runDir: runDir, dirMtime: info.ModTime(), state: rs})
	}
	return candidates, nil
}

// outputFilesSetEqual reports whether a and b name the same set of files, order-insensitive and
// duplicate-insensitive: RunState.OutputFiles records whatever order the caller happened to supply,
// and two specs naming the same files in a different order describe the same run.
func outputFilesSetEqual(a, b []string) bool {
	setA := make(map[string]bool, len(a))
	for _, f := range a {
		setA[f] = true
	}
	setB := make(map[string]bool, len(b))
	for _, f := range b {
		setB[f] = true
	}
	if len(setA) != len(setB) {
		return false
	}
	for f := range setA {
		if !setB[f] {
			return false
		}
	}
	return true
}

// attachVerdict is one candidate's disposition, computed independently of every other candidate.
type attachVerdict int

const (
	// verdictRespawnEligible means the candidate is not a live run: an archive-and-respawn is safe
	// for it.
	verdictRespawnEligible attachVerdict = iota
	// verdictAttachable means the candidate is confirmed live and never terminated.
	verdictAttachable
	// verdictError means a live agent behind this candidate cannot be ruled out — never treated as
	// verdictRespawnEligible, since that is precisely the duplicate-agent hazard this whole mechanism
	// exists to prevent.
	verdictError
)

// dispositionCandidate resolves c's strand via reed's strand table and returns exactly one of the
// three verdicts, per the enumeration in mechanism-failures-do-not-attach-and-do-not-blindly-respawn.
func dispositionCandidate(c attachCandidate, strands []reedengine.StrandStatus, spec Spec, minAge time.Duration, now time.Time) attachVerdict {
	strand, tracked := strandStatusByGUID(strands, c.state.StrandGUID)

	if tracked && strand.Live {
		if c.state.Outcome == runOutcomeRunning {
			return verdictAttachable
		}
		// A terminal value, the empty string (a legacy record), or an unrecognized one all mean the
		// record carries a run that already ended, whatever reed still thinks of its pane.
		return verdictRespawnEligible
	}

	if tracked && !strand.Live {
		// Reed reports not-live both for a pane that died and for a strand it holds no pane id for
		// at all, and only the second is the errStrandPaneBindingCleared case this candidate must be
		// routed through the leftover-then-age rule for. A dead pane (PaneID != "") is unambiguous
		// evidence the agent is gone, so it needs neither the age rule nor the output-files
		// tie-breaker.
		bindingCleared := strand.PaneID == "" && spec.Display.Anchor != render.AnchorHidden
		if !bindingCleared {
			return verdictRespawnEligible
		}
		return leftoverThenAgeVerdict(c, spec, minAge, now)
	}

	// Not tracked at all (the errStrandNotTracked case). The age escape exists here — and
	// deliberately not for the absent-state-file answer above — because erroring unconditionally
	// deadlocks resume permanently: the only thing that ever removes such a directory is
	// sweepOrphansOpportunistic, which runs inside Start, which this error path never reaches. An
	// absent or unreadable reed.json is repaired in-band by "lyx reed up", or simply by
	// "lyx loom run", which calls reed.Up() itself.
	return leftoverThenAgeVerdict(c, spec, minAge, now)
}

// leftoverThenAgeVerdict resolves the two candidate answers that are neither confirmed-live nor
// confirmed-dead (untracked, or tracked with a cleared pane binding): a candidate whose output files
// all already exist is a leftover from a run that already finished, respawn-eligible at any
// directory age; otherwise a candidate old enough to rule out a concurrently-starting run
// (sweepOrphans' own minAge guard) is respawn-eligible; a younger one is an error, since a live agent
// cannot yet be ruled out.
func leftoverThenAgeVerdict(c attachCandidate, spec Spec, minAge time.Duration, now time.Time) attachVerdict {
	if allOutputFilesExist(spec.OutputFiles) {
		return verdictRespawnEligible
	}
	if now.Sub(c.dirMtime) >= minAge {
		return verdictRespawnEligible
	}
	return verdictError
}

// warnAttachCandidates logs one logger.Warn per candidate naming its run directory and strand guid,
// for the two reed-state-gate error dispositions that apply uniformly to every candidate collected
// so far — the operator escape for both is out of band, via "lyx reed status".
func warnAttachCandidates(candidates []attachCandidate, msg string, err error) {
	for _, c := range candidates {
		if err != nil {
			logger.Warn(msg, "runDir", c.runDir, "strandGUID", c.state.StrandGUID, "error", err, "seeAlso", "lyx reed status")
		} else {
			logger.Warn(msg, "runDir", c.runDir, "strandGUID", c.state.StrandGUID, "seeAlso", "lyx reed status")
		}
	}
}

// joinRunDirs returns candidates' run directories as a comma-separated list, for an error naming
// every matching run directory rather than picking one silently.
func joinRunDirs(candidates []attachCandidate) string {
	dirs := make([]string, len(candidates))
	for i, c := range candidates {
		dirs[i] = c.runDir
	}
	return strings.Join(dirs, ", ")
}
