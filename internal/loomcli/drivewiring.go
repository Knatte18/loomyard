// drivewiring.go wires the Go driver's seams to the real run: the loop's step and goto, the status and lock reads, the records and reports, the batten and pause reads, the hand-back, the agents, the worktree reads and the hand-over, plus loom's hidden `drive` verb and the detached spawn of a driver.

package loomcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// driveRefusal is the refusal of a second Go driver for a run one already drives.
const driveRefusal = "loom: drive: a Go driver already drives this run; way forward: transient, `lyx loom status` shows the run, and the live driver settles it"

// driveCmd builds the hidden `drive` subcommand.
func (c *loomCLI) driveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "drive",
		Hidden:      true,
		Short:       "drive this worktree's go-seeded loom run, settling its stops by rule (spawned by lyx loom start)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `drive is the Go driver of a go-seeded loom run, spawned by lyx loom start, by
lyx loom resume and by a hand-over from a driver of an older build. It steps the
run through the --until-stop loop, settles each stop by a closed rule table, asks
a cold tackler agent about a stop no rule settles, and hands everything it cannot
settle back to the run's parent in a stop report.

It holds the run's driver lock for its whole life, so a second driver refuses at
once. It removes the park marker and the resume report on start.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			stale := []string{shedrun.ParkMarker(c.location, c.runID), shedrun.ResumeReport(c.location, c.runID)}
			clihelp.SetExit(ctx, driveVerb(cmd.OutOrStdout(), shedrun.DriverLock(c.location, c.runID), stale, func(release func() error) error {
				deps, err := c.goDriverDepsFor(release)
				if err != nil {
					return err
				}
				return (&goDriver{deps: deps}).run(ctx)
			}))
			return nil
		},
	}
}

// driveEnvelopePath composes the path of one saved stop envelope, drive-envelope-<compact-timestamp>-<4-hex>.json beside the run's drive reports.
func driveEnvelopePath(l *lyxcwd.Location, runID string, now func() time.Time, rand func() string) string {
	filename := fmt.Sprintf("drive-envelope-%s-%s.json", now().Format(driverReportTimestampLayout), rand())
	return filepath.Join(shedrun.DriveReportsDir(l, runID), filename)
}

// The one-line recipe descriptions the tackler's prompt names under its run heading.
const (
	loomTacklerRecipe = "loom: a phased task run (discussion, plan, webster's coded batches, reviews, landing)"
	darnTacklerRecipe = "darn: a one-writer task run (one darn writer session, its review, verify and landing)"
)

// goDriverDepsFor assembles the Go driver's seams for the run the pre-run armed.
// release is the driver lock's early release, which the hand-over calls before it spawns its successor.
// Every closure reads on demand, and the on-disk lyx path is resolved once, here.
func (c *loomCLI) goDriverDepsFor(release func() error) (goDriverDeps, error) {
	arbiterDeps, err := c.arbiterDepsFor()
	if err != nil {
		return goDriverDeps{}, err
	}
	tacklerSettings, err := loomengine.ResolveTackler(c.cfg, c.registry)
	if err != nil {
		return goDriverDeps{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return goDriverDeps{}, fmt.Errorf("loom: drive: resolve the lyx executable: %w", err)
	}

	location := c.location
	runID := shedrun.ResolveRunID(location, c.runID)
	settlePath := shedrun.SettleRecords(location, c.runID)
	reportRand := newDriverReportRand()
	readStatus := circlingStatusReader(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	verifyPaths := verifytree.NewPaths(location.WorktreePath(), verifytree.Dir(location.AnchorPath()))
	pauseGeometry := func() (pauserecord.Geometry, error) { return hubgeom.PauseGeometry(location) }

	tacklerRecipe := loomTacklerRecipe
	if c.recipe == shedrun.RecipeDarn {
		tacklerRecipe = darnTacklerRecipe
	}
	tacklerDeps := sheddriver.TacklerDeps{
		Closed: func(key sheddriver.StopKey, spawned time.Time) (bool, error) {
			records, err := sheddriver.ReadSettleRecords(settlePath)
			if err != nil {
				return false, err
			}
			for _, record := range records {
				if record.StopKey == key && record.Action == sheddriver.SettleHandBack && record.At.After(spawned) {
					return true, nil
				}
			}
			return false, nil
		},
		ReadStatus: readStatus,
		Settings:   tacklerSettings,
		TacklerDir: shedrun.TacklerDir(location, c.runID),
		AttemptDir: func(number int) string { return shedrun.TacklerAttemptDir(location, c.runID, number) },
		MarkerPath: shedrun.TacklerMarker(location, c.runID),
		Producers:  c.routing.Producers,
		Snapshot:   func() (sheddriver.WorktreeSnapshot, error) { return worktreeSnapshot(location) },
		PausedWithin: func(from, to time.Time) (time.Duration, error) {
			geometry, err := pauseGeometry()
			if err != nil {
				return 0, err
			}
			coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, to)
			if err != nil {
				return 0, err
			}
			return pauserecord.Excluded(coverage.Intervals, from, to), nil
		},
		Recipe:         tacklerRecipe,
		GotoRoutedFrom: true,
		Prompt: sheddriver.TacklerPromptInput{
			Slug:      seedSlug(location.WorktreeName),
			Worktree:  location.WorktreePath(),
			TraceDir:  logger.TraceDir(),
			DriverLog: loomengine.LoomDriverLog(location),
		},
		StencilsDir: c.runDeps.Geom.StencilsDir,
		Shuttle:     sheddriver.RunnerShuttle{Runner: c.runner},
		Now:         time.Now,
	}

	deps := goDriverDeps{
		RunID: runID,
		Step: func(ctx context.Context, attempt string) ([]byte, error) {
			var envelope bytes.Buffer
			shedverbs.RunUntilStop(ctx, c.spec, []string{"loom", "step"}, attempt, &envelope)
			if envelope.Len() == 0 {
				return nil, errors.New("the step printed no envelope")
			}
			return envelope.Bytes(), nil
		},
		Goto: func(ctx context.Context, target string) error {
			_, err := shedverbs.RunGoto(ctx, c.spec, target)
			return err
		},
		ReadStatus:  readStatus,
		RunLockHeld: c.runLockHeld,
		SaveEnvelope: func(raw []byte) (string, error) {
			path := driveEnvelopePath(location, c.runID, time.Now, reportRand)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return "", err
			}
			return path, os.WriteFile(path, raw, 0o644)
		},
		SettlePath:  settlePath,
		FrictionDir: loomengine.LoomFrictionDir(location),
		EscalationPending: func(producer string) (bool, error) {
			return arbiterstop.EscalationEligible(producer, arbiterDeps.Stop), nil
		},
		VerifyRecord: func() (battenshed.ChildPublishFailure, bool, error) {
			record, found, err := verifytree.ReadPublishFailure(verifyPaths)
			if err != nil || !found {
				return battenshed.ChildPublishFailure{}, false, err
			}
			return battenshed.ChildPublishFailure{Row: record.Row, Head: record.Head, Target: record.Target, WrittenAt: record.WrittenAt}, true, nil
		},
		Head: func() (string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			return fab.HeadSHA()
		},
		VerifyRows: []string{loomshed.NameMergeIn, loomshed.NamePublish},
		Watch: func() (battenWatch, bool) {
			return readBattenWatch(shedrun.BattenWatchedMarker(location, c.runID), proc.IsAlive)
		},
		BattenRetrySpent: func(w battenWatch) (bool, error) { return battenshed.PublishRetrySpent(w.ScratchDir, w.Producer) },
		Pause: func(now time.Time) (pauserecord.Coverage, error) {
			geometry, err := pauseGeometry()
			if err != nil {
				return pauserecord.Coverage{}, err
			}
			return pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, now)
		},
		PauseHold: func(ctx context.Context) error {
			geometry, err := pauseGeometry()
			if err != nil {
				return err
			}
			return pauserecord.WaitUncovered(ctx, geometry.RecordPath, geometry.Slug, pauserecord.Waiter{})
		},
		NewReportPath: func() string { return driverReportPath(location, c.runID, time.Now, reportRand) },
		Park: driveParkDeps{
			Commit:       loomCommitStatusDeps(location, c.runID),
			MarkerPath:   shedrun.ParkMarker(location, runID),
			Notify:       func(line string) error { return orchcli.NotifyPrime(location, line) },
			OrchRecorded: func() (bool, error) { return primeOrchRecorded(location) },
			Now:          time.Now,
		},
		CommitRecords: func() error {
			var envelope bytes.Buffer
			if code := commitRecordsVerb(&envelope, loomCommitStatusDeps(location, c.runID)); code != 0 {
				return errors.New(strings.TrimSpace(envelope.String()))
			}
			return nil
		},
		WriteFrictionNote: func(stop driveStop) error { return writeDrivePausedNote(c.frictionDir, runID, stop) },
		Tackler: func(stop driveStop, reportPath string) (sheddriver.TacklerEnvelope, error) {
			call, err := sheddriver.NewTacklerCall(tacklerDeps, sheddriver.TacklerInput{RunID: runID, EnvelopePath: stop.EnvelopePath, ReportPath: reportPath, TraceCopy: stop.TraceCopy, StderrPath: stop.StderrPath})
			if err != nil {
				return sheddriver.TacklerEnvelope{}, err
			}
			return call.Settle()
		},
		ConsumeTackler: func(attemptDir string) error {
			return sheddriver.ConsumeTacklerAttempt(attemptDir, shedengine.RelaunchRequest{
				StatusPath:              c.shedPaths.StatusPath,
				LockPath:                c.shedPaths.LockPath,
				StatusLockPath:          c.shedPaths.StatusLockPath,
				Producers:               c.routing.Producers,
				RunID:                   runID,
				MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
			})
		},
		Arbiter: func(envelopePath, reportPath, caseBriefPath string) (loomengine.ArbiterEnvelope, error) {
			call, err := newArbiterCall(arbiterDeps, arbiterInput{RunID: runID, EnvelopePath: envelopePath, ReportPath: reportPath, CaseBriefPath: caseBriefPath})
			if err != nil {
				return loomengine.ArbiterEnvelope{}, err
			}
			return call.settle()
		},
		Snapshot: func() (sheddriver.WorktreeSnapshot, error) { return worktreeSnapshot(location) },
		OpenAttempts: func(key sheddriver.StopKey) (string, string, bool, error) {
			return newestAttemptsFor(shedrun.TacklerDir(location, c.runID), shedrun.ArbiterDir(location, c.runID), key)
		},
		AttemptSnapshot: func(tacklerDir string) (sheddriver.WorktreeSnapshot, error) {
			attempt, err := sheddriver.ReadTacklerAttempt(tacklerDir)
			return attempt.Snapshot, err
		},
		OnDiskBuild: func() (buildvcs.Identity, error) { return buildvcs.OfFile(executable) },
		BuildChanged: func(onDisk buildvcs.Identity) bool {
			return shedverbs.LastStepBuildChanged(c.spec.StepsDir, onDisk)
		},
		DirtyPaths:  func() ([]string, error) { return dirtyPathsOutsideRecords(location, c.runID) },
		ReleaseLock: release,
		Executable:  executable,
		SpawnSuccessor: func(executable string) error {
			_, err := c.spawnGoDriverFrom(executable)
			return err
		},
		Now: time.Now,
	}
	return deps, nil
}

// primeOrchRecorded reports whether the hub's prime has an orch strand recorded to receive notices.
func primeOrchRecorded(location *lyxcwd.Location) (bool, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return false, err
	}
	prime, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
	if err != nil {
		return false, err
	}
	orch, err := orchengine.LoadState(orchcli.PrimePaths(prime))
	if err != nil {
		return false, err
	}
	return orch.Strand != "", nil
}

// writeDrivePausedNote writes the `go-driver-paused` friction note for a paused stop that carries no reason, naming the stop, its producer and its envelope.
// It is a no-op when frictionDir is empty, which is Tier 2 off.
func writeDrivePausedNote(frictionDir, runID string, stop driveStop) error {
	if frictionDir == "" {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "go-driver-paused")
	if path == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("the Go driver found run " + runID + " paused with no reason at " + stop.CurrentProducer + "\n\n")
	b.WriteString("state: " + stop.key().State + "\n")
	b.WriteString("current_producer: " + stop.CurrentProducer + "\n")
	b.WriteString("history_entries: " + fmt.Sprint(stop.HistoryLength) + "\n")
	b.WriteString("envelope: " + stop.EnvelopePath + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("loom: write go-driver-paused note %s: %w", path, err)
	}
	return nil
}

// worktreeSnapshot reads the state of the task's code worktree a tackler must leave as it found it: HEAD, branch and the SHA-256 of each tracked dirty path.
// The records side is left out, since the run's own records change under every step.
func worktreeSnapshot(location *lyxcwd.Location) (sheddriver.WorktreeSnapshot, error) {
	fab, err := fabricengine.Open(location)
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	head, err := fab.HeadSHA()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	branch, err := fab.CurrentBranch()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	paths, err := fab.DirtyPaths()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	codeRoot := location.WorktreePath() + string(filepath.Separator)
	dirty := map[string]string{}
	for _, path := range paths {
		if !strings.HasPrefix(path, codeRoot) {
			continue
		}
		content, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			dirty[path] = "deleted"
		case err != nil:
			return sheddriver.WorktreeSnapshot{}, fmt.Errorf("hash dirty path %s: %w", path, err)
		default:
			sum := sha256.Sum256(content)
			dirty[path] = hex.EncodeToString(sum[:])
		}
	}
	return sheddriver.WorktreeSnapshot{Head: head, Branch: branch, Dirty: dirty}, nil
}

// dirtyPathsOutsideRecords lists the pair's uncommitted or untracked paths outside the run's own records directories.
// Both sides join onto the task worktree, since the records side reaches it through the `_lyx` junction.
func dirtyPathsOutsideRecords(location *lyxcwd.Location, runID string) ([]string, error) {
	fab, err := fabricengine.Open(location)
	if err != nil {
		return nil, err
	}
	entries, err := fab.Status()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(location.WorktreePath(), filepath.FromSlash(entry.Path)))
	}
	return loomengine.PathsOutsideRunRecords(location, runID, paths), nil
}

// newestAttemptsFor names the newest tackler and arbiter attempt directories recorded for key, empty when there is none, and whether the tackler attempt carries the applied mark.
func newestAttemptsFor(tacklerDir, arbiterDir string, key sheddriver.StopKey) (tackler, arbiter string, tacklerApplied bool, err error) {
	tacklers, err := sheddriver.ListTacklerAttempts(tacklerDir)
	if err != nil {
		return "", "", false, err
	}
	for i := len(tacklers) - 1; i >= 0; i-- {
		if tacklers[i].StopKey == key {
			tackler = tacklers[i].Dir
			break
		}
	}
	if tackler != "" {
		if tacklerApplied, err = sheddriver.TacklerApplied(tackler); err != nil {
			return "", "", false, err
		}
	}
	arbiters, err := loomengine.ListArbiterAttempts(arbiterDir)
	if err != nil {
		return "", "", false, err
	}
	for i := len(arbiters) - 1; i >= 0; i-- {
		if arbiters[i].StopKey == key {
			arbiter = arbiters[i].Dir
			break
		}
	}
	return tackler, arbiter, tacklerApplied, nil
}

// spawnDetachedDrive starts `lyx loom drive` from the lyx at exe for the task worktree l as a detached process, its output appended to the driver log, and returns its pid.
// The child is reaped in the background, so a driver that exits at once never reads alive as a zombie.
func spawnDetachedDrive(l *lyxcwd.Location, exe string) (int, error) {
	logPath := loomengine.LoomDriverLog(l)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	driveCmd := exec.Command(exe, "loom", "drive")
	driveCmd.Dir = l.AnchorPath()
	driveCmd.Stdout = logFile
	driveCmd.Stderr = logFile
	proc.Detach(driveCmd)
	if err := driveCmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	pid := driveCmd.Process.Pid
	logger.Info("loom: spawned detached Go driver", "pid", pid, "log", logPath)
	// The child holds its own duplicate of the log descriptor.
	_ = logFile.Close()
	go func() { _ = driveCmd.Wait() }()
	return pid, nil
}
