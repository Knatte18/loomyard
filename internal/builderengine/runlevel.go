// runlevel.go implements Run, the `run` verb's engine core: the run-lock,
// the never-instantly-re-pause clear, the automatic validation gate, the
// plan-fingerprint crash/resume guard (with its --fresh archive/re-init
// escape), the always-fresh orchestrator spawn (stencil-filled prompt,
// rendered batch index + progress), and the shuttle-outcome-to-RunResult
// mapping the discussion's distinct-envelope decision pins. Named runlevel.go
// (not run.go) to avoid colliding with the poll/spawn files' own naming, per
// this batch's own file-naming note.

package builderengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// runLockName is the exclusive-lease file name inside the builder dir, held
// for the ENTIRE duration of one Run call — perchengine's ErrBlockBusy
// pattern applied to builder's own run-level mutex (see the discussion's
// crash/resume decision): without it, two concurrent `lyx builder run`
// invocations would each cold-start from the same state.json and reports,
// then both drive the batch loop at once.
const runLockName = "run.lock"

// ErrRunBusy marks Run's fail-fast refusal when another invocation already
// holds the builder dir's run.lock. It is a sentinel (matched via
// errors.Is) because the caller must treat this refusal differently from
// every other hard error: the losing call touched NOTHING on disk — the
// winner is mid-run and owns the state — so a caller must not run its own
// exit-time bookkeeping (buildercli's backstop weft-commit) for it.
var ErrRunBusy = errors.New("builder: run is already in progress")

// ErrFingerprintMismatch marks Run's fail-loud refusal when the on-disk
// plan's fingerprint no longer matches the fingerprint recorded in
// state.json at first init — the discussion's plan-fingerprint decision:
// stale reports from a superseded plan must never be misread as progress.
// The refusal is unconditional UNLESS the caller passed RunOptions.Fresh,
// which instead archives the stale state and reports and re-inits (see Run).
var ErrFingerprintMismatch = errors.New("builder: on-disk plan fingerprint does not match this run's recorded state")

// OrchestratorAskingError marks Run's mapping of a shuttle OutcomeAsking
// result for the orchestrator's own spawn: the orchestrator ended its turn
// asking a question instead of ever reaching its own outcome-file final
// action. Unwrap returns ErrOrchestratorAsking so a caller can classify via
// errors.Is without needing the concrete type; the concrete type itself
// carries the per-call SessionID, RunDir, and LastAssistantMessage a caller
// needs to log or resume from — the discussion's "each carrying SessionID
// and the kept RunDir, and for asking the LastAssistantMessage" requirement.
type OrchestratorAskingError struct {
	SessionID string
	RunDir    string
	Message   string
}

func (e *OrchestratorAskingError) Error() string {
	return fmt.Sprintf("builder: orchestrator asked a question instead of finishing (session %s, kept run dir %s): %s", e.SessionID, e.RunDir, e.Message)
}

func (e *OrchestratorAskingError) Unwrap() error { return ErrOrchestratorAsking }

// ErrOrchestratorAsking is the sentinel OrchestratorAskingError wraps.
var ErrOrchestratorAsking = errors.New("builder: orchestrator asking")

// OrchestratorDiedError marks Run's mapping of a shuttle OutcomeDied result
// for the orchestrator's own spawn: its pane died (or it never became
// ready) before it ever reached its own outcome-file final action.
type OrchestratorDiedError struct {
	SessionID string
	RunDir    string
}

func (e *OrchestratorDiedError) Error() string {
	return fmt.Sprintf("builder: orchestrator pane died (session %s, kept run dir %s)", e.SessionID, e.RunDir)
}

func (e *OrchestratorDiedError) Unwrap() error { return ErrOrchestratorDied }

// ErrOrchestratorDied is the sentinel OrchestratorDiedError wraps.
var ErrOrchestratorDied = errors.New("builder: orchestrator died")

// OrchestratorTimeoutError marks Run's mapping of a shuttle OutcomeTimeout
// result for the orchestrator's own spawn: its wall-clock Timeout elapsed
// before it ever reached its own outcome-file final action.
type OrchestratorTimeoutError struct {
	SessionID string
	RunDir    string
}

func (e *OrchestratorTimeoutError) Error() string {
	return fmt.Sprintf("builder: orchestrator run timed out (session %s, kept run dir %s)", e.SessionID, e.RunDir)
}

func (e *OrchestratorTimeoutError) Unwrap() error { return ErrOrchestratorTimeout }

// ErrOrchestratorTimeout is the sentinel OrchestratorTimeoutError wraps.
var ErrOrchestratorTimeout = errors.New("builder: orchestrator timed out")

// BlockingRunner is the seam Run spawns the orchestrator through: exactly
// (*shuttleengine.Runner).Run's signature (Start+Wait combined — Run's own
// caller, `lyx builder run`, blocks for the whole plan run anyway, so there
// is no reason to hold a non-blocking handle the way SpawnBatch's Starter
// does). Production code passes a real *shuttleengine.Runner directly; tests
// pass a local fake (builderengine's own fakes are test-file-local, per the
// discussion's test-conventions decision).
type BlockingRunner interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// RunDeps carries every seam Run needs, so a test can fake each one
// independently: Runner spawns and blocks on the orchestrator; PlanDir,
// BuilderDir, and ReportsDir are the hubgeometry-resolved _lyx/plan,
// _lyx/builder, and _lyx/builder/reports directories; WorktreeRoot is the
// host repo checkout Validate's context estimate resolves Scope/Where
// entries against; Config is the loaded builder.yaml; Roles is the
// pre-flight-resolved role->model-spec map (see ResolveRoles).
type RunDeps struct {
	Runner       BlockingRunner
	PlanDir      string
	BuilderDir   string
	ReportsDir   string
	WorktreeRoot string
	Config       Config
	Roles        map[Role]modelspec.Resolved
}

// RunOptions carries one `run` invocation's caller-supplied choices. Fresh
// requests the fingerprint-mismatch escape: archive the stale state.json and
// reports dir and re-init, rather than refusing with ErrFingerprintMismatch.
type RunOptions struct {
	Fresh bool
}

// RunResult is what one successful Run call hands back to its caller
// (internal/buildercli's `run` verb): the orchestrator's own outcome-file
// judgment (Outcome/StuckReason/BatchesDone) plus the SessionID and RunDir
// the CLI envelope surfaces alongside it.
type RunResult struct {
	// Outcome is one of OutcomeDone, OutcomeStuck, or OutcomePaused, taken
	// verbatim from the parsed outcome.yaml.
	Outcome string
	// StuckReason is the parsed outcome.yaml's stuck_reason, verbatim.
	StuckReason string
	// BatchesDone is the parsed outcome.yaml's batches_done, verbatim.
	BatchesDone int
	// SessionID is the orchestrator's shuttle session identity.
	SessionID string
	// RunDir is the orchestrator's shuttle run directory.
	RunDir string
}

// newRunGUID returns a 128-bit random identifier, hex-encoded, generated
// from crypto/rand — mirroring internal/muxengine's own newGUID, the
// pattern this package's own RunGUID field (see state.go) is minted with:
// once, at first init, never regenerated across a resume.
func newRunGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("builder: mint run guid: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// archiveTimestamp is the UTC compact timestamp format Run's own
// fingerprint-mismatch archiving shares with outcome.go's
// ArchiveStaleOutcome, so every archived builder artifact sorts and reads
// identically regardless of which one archived it.
const archiveTimestampFormat = "20060102T150405Z"

// firstFreeArchivePath returns the first path in the sequence
// candidate(""), candidate("-1"), candidate("-2"), ... that does not
// currently exist on disk — the same same-second collision rule
// ArchiveStaleOutcome documents, shared here so archiveStateFile and
// archiveReportsDir need not each duplicate the collision loop.
func firstFreeArchivePath(candidate func(suffix string) string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		path := candidate(suffix)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
	}
}

// archiveStateFile renames builderDir's state.json, if present, to
// state-<UTC-compact-timestamp>.json in place — the discussion's
// fingerprint-mismatch escape's first half. Absent file: ("", nil), a no-op.
func archiveStateFile(builderDir string, now func() time.Time) (string, error) {
	path := filepath.Join(builderDir, stateFileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("builder: stat state file %s: %w", path, err)
	}

	stamp := now().UTC().Format(archiveTimestampFormat)
	target, err := firstFreeArchivePath(func(suffix string) string {
		return filepath.Join(builderDir, fmt.Sprintf("state-%s%s.json", stamp, suffix))
	})
	if err != nil {
		return "", fmt.Errorf("builder: find archive target for state file %s: %w", path, err)
	}

	if err := os.Rename(path, target); err != nil {
		return "", fmt.Errorf("builder: archive stale state file %s: %w", path, err)
	}
	return target, nil
}

// archiveReportsDir renames reportsDir wholesale, if present, to
// <reportsDir>-<UTC-compact-timestamp> — "clears the reports dir the same
// way" the discussion's fingerprint-mismatch escape pins — then recreates an
// empty reportsDir so the re-initialized run has somewhere to write the
// first batch's report into. Absent dir: a no-op (still recreates an empty
// one, since a fresh run needs it regardless of whether a prior one ever
// existed).
func archiveReportsDir(reportsDir string, now func() time.Time) error {
	if _, err := os.Stat(reportsDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("builder: stat reports dir %s: %w", reportsDir, err)
		}
	} else {
		stamp := now().UTC().Format(archiveTimestampFormat)
		parent := filepath.Dir(reportsDir)
		base := filepath.Base(reportsDir)
		target, err := firstFreeArchivePath(func(suffix string) string {
			return filepath.Join(parent, fmt.Sprintf("%s-%s%s", base, stamp, suffix))
		})
		if err != nil {
			return fmt.Errorf("builder: find archive target for reports dir %s: %w", reportsDir, err)
		}
		if err := os.Rename(reportsDir, target); err != nil {
			return fmt.Errorf("builder: archive stale reports dir %s: %w", reportsDir, err)
		}
	}

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("builder: recreate reports dir %s: %w", reportsDir, err)
	}
	return nil
}

// renderBatchIndex renders plan's Batch Index into the ordered-list text
// {{.batch_index}} fills with: one line per batch, "NN — slug — intent",
// annotated with "(oversized)" and/or "(verify: deferred; chain-end NN)"
// where the batch's own frontmatter declares them — the orchestrator's
// pinned navigation source, per the discussion's decision that Go renders
// the batch list from the validated plan rather than the orchestrator
// reading 00-overview.md itself.
func renderBatchIndex(plan *Plan) string {
	lines := make([]string, 0, len(plan.Batches))
	for _, b := range plan.Batches {
		line := fmt.Sprintf("%02d — %s — %s", b.Number, b.Slug, b.Intent)

		var annotations []string
		if b.Oversized {
			annotations = append(annotations, "oversized")
		}
		if b.VerifyDeferred {
			annotations = append(annotations, fmt.Sprintf("verify: deferred; chain-end %02d", b.ChainEnd))
		}
		if len(annotations) > 0 {
			line += " (" + strings.Join(annotations, "; ") + ")"
		}

		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderProgress renders {{.progress}}'s per-batch state summary for
// resume: every batch in plan whose batch-report file already exists in
// reportsDir is summarized as done — the discussion's resume-on-files rule
// ("reports present are summarized done") — one "NN-slug: done" line per
// such batch, in plan order. A batch with no report yet is omitted
// entirely, never listed as "pending", since the orchestrator's own batch
// index already enumerates every batch — this field's only job is telling a
// resumed session what already happened. Returns the literal word "none"
// when no batch has reported yet (a fresh run, or a resume before the first
// batch ever completed).
func renderProgress(plan *Plan, reportsDir string) (string, error) {
	var lines []string
	for _, b := range plan.Batches {
		reportPath := filepath.Join(reportsDir, BatchReportFileName(b.Number, b.Slug))
		if _, err := os.Stat(reportPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("builder: stat batch report %s: %w", reportPath, err)
		}
		// Summarize each reported batch by its report's OWN status, not merely
		// by the report's presence: a batch that reported stuck still needs
		// recovery, so labeling it "done" here would tell a resumed
		// orchestrator it already finished and make it skip the recovery the
		// stuck batch actually needs — a silent false-success across a
		// crash/resume boundary (poll commits a stuck report the same as a done
		// one). A report that will not parse is corruption on the resume path:
		// fail loud, the same discipline ParseReport applies everywhere else,
		// never a guessed status.
		report, err := ParseReport(reportPath)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%02d-%s: %s", b.Number, b.Slug, report.Status))
	}
	if len(lines) == 0 {
		return "none", nil
	}
	return strings.Join(lines, "\n"), nil
}

// Run drives one `lyx builder run` invocation to completion: the run-level
// mutex, the never-instantly-re-pause clear, the automatic validation gate,
// the plan-fingerprint crash/resume guard (with its --fresh escape), the
// stale-outcome-file archive, the always-fresh orchestrator spawn, and the
// shuttle-outcome-to-RunResult mapping. Every returned error is
// "builder: "-prefixed (via the helpers it calls); ErrRunBusy and
// ErrFingerprintMismatch are exported sentinels a caller matches via
// errors.Is, and a non-done shuttle outcome for the orchestrator's own spawn
// returns one of the three distinct *Orchestrator*Error types above rather
// than ever attempting to parse a (non-existent) outcome.yaml.
func Run(deps RunDeps, opts RunOptions) (RunResult, error) {
	if err := os.MkdirAll(deps.BuilderDir, 0o755); err != nil {
		return RunResult{}, fmt.Errorf("builder: create builder dir %s: %w", deps.BuilderDir, err)
	}

	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(deps.BuilderDir, runLockName))
	if err != nil {
		return RunResult{}, fmt.Errorf("builder: acquire run lock in %s: %w", deps.BuilderDir, err)
	}
	if !locked {
		return RunResult{}, fmt.Errorf("%w: %q (run.lock held); wait for it to finish, or check `lyx builder status`", ErrRunBusy, deps.BuilderDir)
	}
	defer runLock.Release()

	// Never-instantly-re-pause: a resumed run must not immediately refuse
	// its own first spawn-batch call on a flag left over from the pause
	// this very invocation is resuming from.
	if err := ClearPause(deps.BuilderDir); err != nil {
		return RunResult{}, err
	}

	plan, err := ParsePlan(deps.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	caps := ValidateCaps{
		ContextCapTokens: deps.Config.BatchContextCapTokens,
		CardCap:          deps.Config.BatchCardCap,
	}
	if findings := Validate(plan, deps.WorktreeRoot, caps); len(findings) > 0 {
		msgs := make([]string, len(findings))
		for i, f := range findings {
			msgs[i] = f.Error()
		}
		return RunResult{}, fmt.Errorf("builder: plan validation refused this run (%d finding(s)): %s", len(findings), strings.Join(msgs, "; "))
	}

	fingerprint, err := Fingerprint(deps.PlanDir)
	if err != nil {
		return RunResult{}, err
	}

	st, err := LoadState(deps.BuilderDir)
	if err != nil {
		return RunResult{}, err
	}

	switch {
	case st == nil:
		guid, err := newRunGUID()
		if err != nil {
			return RunResult{}, err
		}
		st = &State{
			RunGUID:         guid,
			PlanFingerprint: fingerprint,
			Batches:         map[int]*BatchState{},
			ChainStartSHAs:  map[int]string{},
		}
		if err := SaveState(deps.BuilderDir, st); err != nil {
			return RunResult{}, err
		}

	case st.PlanFingerprint != fingerprint:
		if !opts.Fresh {
			return RunResult{}, fmt.Errorf("%w: on-disk plan fingerprint %s does not match this run's recorded fingerprint %s; the plan changed since state.json was created — re-run with --fresh to archive the stale state and reports and start over", ErrFingerprintMismatch, fingerprint, st.PlanFingerprint)
		}

		if _, err := archiveStateFile(deps.BuilderDir, time.Now); err != nil {
			return RunResult{}, err
		}
		if err := archiveReportsDir(deps.ReportsDir, time.Now); err != nil {
			return RunResult{}, err
		}

		guid, err := newRunGUID()
		if err != nil {
			return RunResult{}, err
		}
		st = &State{
			RunGUID:         guid,
			PlanFingerprint: fingerprint,
			Batches:         map[int]*BatchState{},
			ChainStartSHAs:  map[int]string{},
		}
		if err := SaveState(deps.BuilderDir, st); err != nil {
			return RunResult{}, err
		}
	}

	if _, err := ArchiveStaleOutcome(deps.BuilderDir, time.Now); err != nil {
		return RunResult{}, err
	}

	batchIndex := renderBatchIndex(plan)
	progress, err := renderProgress(plan, deps.ReportsDir)
	if err != nil {
		return RunResult{}, err
	}
	outcomePath, err := filepath.Abs(filepath.Join(deps.BuilderDir, outcomeFileName))
	if err != nil {
		return RunResult{}, fmt.Errorf("builder: resolve outcome path: %w", err)
	}

	values := map[string]string{
		"batch_index":  batchIndex,
		"progress":     progress,
		"outcome_path": outcomePath,
		"self_fix_cap": fmt.Sprintf("%d", deps.Config.SelfFixCap),
		"poll_wait_s":  fmt.Sprintf("%d", deps.Config.PollWaitS),
	}
	prompt, err := stencil.Fill(OrchestratorTemplate(), values)
	if err != nil {
		return RunResult{}, fmt.Errorf("builder: fill orchestrator template: %w", err)
	}

	resolved, ok := deps.Roles[RoleOrchestrator]
	if !ok {
		return RunResult{}, fmt.Errorf("builder: no resolved model-spec for role %q", RoleOrchestrator)
	}

	spec := shuttleengine.Spec{
		Prompt:      string(prompt),
		OutputFiles: []string{outcomePath},
		Model:       resolved.Model,
		Effort:      resolved.Params["effort"],
		Version:     resolved.Params["version"],
		Role:        string(RoleOrchestrator),
		Interactive: false,
		Timeout:     time.Duration(deps.Config.OrchestratorTimeoutMin) * time.Minute,
	}

	result, err := deps.Runner.Run(spec)
	if err != nil {
		return RunResult{}, fmt.Errorf("builder: run orchestrator: %w", err)
	}

	switch result.Outcome {
	case shuttleengine.OutcomeDone:
		outcome, err := ParseOutcome(outcomePath)
		if err != nil {
			return RunResult{}, err
		}

		if outcome.Outcome != OutcomePaused {
			if err := ClearPause(deps.BuilderDir); err != nil {
				return RunResult{}, err
			}
		}

		return RunResult{
			Outcome:     outcome.Outcome,
			StuckReason: outcome.StuckReason,
			BatchesDone: outcome.BatchesDone,
			SessionID:   result.SessionID,
			RunDir:      result.RunDir,
		}, nil

	case shuttleengine.OutcomeAsking:
		return RunResult{}, &OrchestratorAskingError{SessionID: result.SessionID, RunDir: result.RunDir, Message: result.LastAssistantMessage}

	case shuttleengine.OutcomeDied:
		return RunResult{}, &OrchestratorDiedError{SessionID: result.SessionID, RunDir: result.RunDir}

	case shuttleengine.OutcomeTimeout:
		return RunResult{}, &OrchestratorTimeoutError{SessionID: result.SessionID, RunDir: result.RunDir}

	default:
		return RunResult{}, fmt.Errorf("builder: orchestrator run returned unrecognized shuttle outcome %q", result.Outcome)
	}
}
