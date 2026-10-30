// dobbydeps.go fills dobbyengine's seams over a real hub: the runs and tasks it reads, the pass it spawns, the notice, owner pause and board tidy it sends, and the registration of the dobby process itself.

package beamcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

const (
	// dobbyProgram is the program name dobby signs its notices with, scoped to its beam's slug.
	dobbyProgram = "DOBBY"
	// defaultNoticeProbe is the probe interval when the batten recipe's Run-Shed row names none.
	defaultNoticeProbe = 30 * time.Second
	// tidyModelSpec is the model spec the board-tidy agent runs on.
	tidyModelSpec = "sonnet[medium]"
	// passErrorTailBytes bounds the output of a pass dobby keeps to read its error text from.
	passErrorTailBytes = 16 << 10
	// passErrorTextRunes bounds the error text of a pass that printed no envelope.
	passErrorTextRunes = 500
)

// registerDobby records this process as the dobby of geom in the run-process registry under runProcDir.
// The owner is the beam's slug, so the pair teardown of a landed beam ends its dobby, and the hub owner for the prime.
func registerDobby(runProcDir string, geom dobbyengine.Geometry) error {
	owner := geom.Slug
	if owner == "" {
		owner = runproc.HubOwner
	}
	return runproc.Register(runProcDir, owner, runproc.SelfRecord(runproc.RoleDobby))
}

// hubDobbyDeps fills every seam of dobbyengine.Run over the hub of prime, for the dobby of geom, with its log lines appended to the file the dobby tells.
// The dobby's own working directory is the hub root, so every path comes from prime and geom, and only the pass spawn starts its process in the prime's anchor.
func hubDobbyDeps(prime *lymcwd.Location, geom dobbyengine.Geometry) (dobbyengine.Deps, error) {
	worktree, err := lymcwd.ResolveWorktree(geom.WorktreeRoot)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	probe, err := noticeProbeInterval()
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	dirs := fabricengine.ConfigDirs(worktree)
	boardDir := geom.BoardDir
	parents := battencli.NewDobbyParents(prime)
	owners := func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) }
	slugOwner := func(slug string) (string, error) {
		read, err := owners()
		if err != nil {
			return "", err
		}
		return read.Effective(slug, parents.Of)
	}
	deps := dobbyengine.Deps{
		Now:   time.Now,
		Awake: pauserecord.MonotonicAwake(),
		Sleep: sleepContext,
		Watch: func() (<-chan struct{}, func(), error) {
			return openDobbyWatch(dobbyWatchDirs(prime, pause.RecordPath, boardDir))
		},
		Machine:       machinename.Name,
		CheckBinary:   selfexec.New().CheckWith,
		ProbeInterval: probe,
		Owner: func() (string, error) {
			read, err := owners()
			if err != nil {
				return "", err
			}
			if geom.Slug == "" {
				return read.Prime, nil
			}
			return read.Effective(geom.Slug, parents.Of)
		},
		Runs: func() ([]dobbyengine.RunView, error) {
			return battencli.DobbyRuns(prime, geom, parents, pause.RecordPath)
		},
		SpawnPass: func(slug string, continuePass, noReopen bool) (<-chan dobbyengine.PassExit, error) {
			return spawnPass(geom, slug, continuePass, noReopen)
		},
		Notice: func(_ context.Context, line string) error {
			posted, err := orchcli.NotifyFromScoped(worktree, geom.Slug, dobbyProgram, line)
			if err != nil {
				return err
			}
			if !posted {
				return errors.New("no orch strand is recorded to receive the notice")
			}
			return nil
		},
		PauseOwned: func(_ context.Context, slug string) error { return tendercli.PauseOwnedElsewhere(prime, slug) },
		ReleaseParentLock: func(_ context.Context, slug string) error {
			return battencli.ReleaseRunParentLock(prime, slug)
		},
		SlugOwner: slugOwner,
		AdoptSubject: func(slug string) (string, error) {
			read, err := owners()
			if err != nil {
				return "", err
			}
			return read.AdoptSubject(slug, parents.Of)
		},
		LiftOwnerPause: func(_ context.Context, slug string) error {
			return liftOwnerPause(prime, pause.RecordPath, slug)
		},
		Tasks:      func() ([]dobbyengine.TaskView, error) { return battencli.DobbyTasks(prime, boardDir, parents) },
		LoadConfig: func() (dobbyengine.Config, error) { return dobbyengine.LoadConfig(dirs) },
		TaskLimit:  proc.ReadTaskLimit,
		StatusExists: func(slug string) (bool, error) {
			return fileExists(shedrun.StatusFile(prime, slug))
		},
		PauseCovers: func(slug string) (bool, error) {
			coverage, err := pauserecord.ReadCoverage(pause.RecordPath, slug, time.Now())
			return coverage.Covered, err
		},
		Landings: machineLandings(machinename.Name, func(machine string) ([]string, error) {
			return battencli.DobbyLandings(prime, geom, parents, machine)
		}),
		Log: func(path, line string, rotate bool) {
			if err := dobbyengine.AppendLog(path, line, rotate); err != nil {
				logger.Debug("beamcli: a dobby log line was not written", "path", path, "error", err)
			}
		},
		StencilsDir: fabricengine.StencilsDir(prime.HubPath),
		NearestOrch: func() (string, error) {
			parent, err := hubgeom.NearestLiveOrch(worktree, orchcli.OrchLive)
			return parent.Name, err
		},
	}
	if geom.Slug != "" {
		// The removed worktree's config is gone by the time this stops a strand, so the runner is built over the prime.
		deps.PrimeTidyShuttle = &runnerTidyShuttle{worktree: prime}
		deps.CanLand = func() (dobbyengine.LandReading, error) { return beamLandReading(prime, geom.Slug) }
		deps.Land = func() error {
			_, err := land(hubLandDeps(prime), geom.Slug)
			return err
		}
	}
	if choice, err := resolveTidyModel(dirs); err != nil {
		logger.Warn("beamcli: the board-tidy model does not resolve; the board tidy is off", "spec", tidyModelSpec, "error", err)
	} else {
		deps.TidyModel = choice
		deps.TidyShuttle = &runnerTidyShuttle{worktree: worktree}
	}
	return deps, nil
}

// beamLandReading reads whether the beam slug can land, which it can once it is flagged ready and nothing works under it, and, for a beam that can, which other machine owns its parent and what `lym adopt` takes to move it.
func beamLandReading(prime *lymcwd.Location, slug string) (dobbyengine.LandReading, error) {
	ready, err := hubLandDeps(prime).Ready(slug)
	if err != nil || !ready {
		return dobbyengine.LandReading{}, err
	}
	blockers, err := battencli.ReadBeamLandBlockers(prime, slug)
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	if !blockers.Empty() {
		return dobbyengine.LandReading{}, nil
	}
	here, err := machinename.Name()
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) }
	parent, err := parentOf(slug)
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	owner, subject := owners.Prime, boardengine.RootSubject
	if parent != "" {
		if owner, err = owners.Effective(parent, parentOf); err != nil {
			return dobbyengine.LandReading{}, err
		}
		if subject, err = owners.AdoptSubject(parent, parentOf); err != nil {
			return dobbyengine.LandReading{}, err
		}
	}
	reading := dobbyengine.LandReading{Passes: true}
	if boardengine.OwnedElsewhere(owner, here) {
		reading.ParentOwner, reading.ParentSubject = owner, subject
	}
	return reading, nil
}

// noticeProbeInterval reads notice_probe_s from the batten recipe's Run-Shed row, the cadence the run's own wait probes at.
func noticeProbeInterval() (time.Duration, error) {
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		return 0, err
	}
	for _, row := range recipe.Producers {
		if row.Name != battenrecipe.NameRunShed {
			continue
		}
		switch seconds := row.Config["notice_probe_s"].(type) {
		case int:
			return time.Duration(seconds) * time.Second, nil
		case int64:
			return time.Duration(seconds) * time.Second, nil
		case float64:
			return time.Duration(seconds * float64(time.Second)), nil
		}
	}
	return defaultNoticeProbe, nil
}

// sleepContext waits d, or returns early when ctx ends.
func sleepContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// fileExists reports whether a file or directory exists at path.
func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// pairLocation returns slug's task pair as a location at the pair's own anchor, with no git call.
func pairLocation(prime *lymcwd.Location, slug string) (*lymcwd.Location, error) {
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return nil, fmt.Errorf("locate the pair %s: %w", slug, err)
	}
	return pair, nil
}

// liftOwnerPause lifts the live pause entry of slug's own scope that carries the owner caller.
func liftOwnerPause(prime *lymcwd.Location, recordPath, slug string) error {
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return err
	}
	var refs []tenderengine.EntryRef
	for _, entry := range record.Live {
		if entry.Scope == slug && entry.Caller == pauserecord.OwnerCaller {
			refs = append(refs, tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	deps, _, err := tendercli.HubDeps(prime, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := tenderengine.Lift(deps, ref); err != nil {
			return err
		}
	}
	return nil
}

// spawnPass starts `<lym> batten run <slug>` for geom as its own process, in the prime's anchor, and delivers its exit once.
// The pass registers itself in the run-process registry as the run's batten, so dobby registers nothing.
func spawnPass(geom dobbyengine.Geometry, slug string, continuePass, noReopen bool) (<-chan dobbyengine.PassExit, error) {
	if testing.Testing() {
		return nil, dobbyengine.ErrSpawnUnderTest
	}
	args := []string{"batten", "run", slug}
	if continuePass {
		args = append(args, "--continue-pass")
	}
	if noReopen {
		args = append(args, "--no-reopen")
	}
	output := &tailBuffer{limit: passErrorTailBytes}
	cmd := exec.Command(geom.LymExecutable, args...)
	cmd.Dir = geom.PrimeAnchor
	cmd.Stdout = output
	cmd.Stderr = output
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("beamcli: spawn batten pass failed", "exe", geom.LymExecutable, "args", args, "dir", geom.PrimeAnchor, "error", err)
		return nil, err
	}
	logger.Info("beamcli: spawned batten pass", "exe", geom.LymExecutable, "args", args, "dir", geom.PrimeAnchor, "pid", cmd.Process.Pid)
	exit := make(chan dobbyengine.PassExit, 1)
	go func() {
		waitErr := cmd.Wait()
		exit <- dobbyengine.PassExit{Code: cmd.ProcessState.ExitCode(), Error: passErrorText(output.Bytes(), waitErr)}
	}()
	return exit, nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if extra := len(b.data) - b.limit; extra > 0 {
		b.data = b.data[extra:]
	}
	return len(p), nil
}

// Bytes returns a copy of what the buffer holds.
func (b *tailBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.data)
}

// passErrorText reads the error text of a pass from what it printed: the error of the last JSON envelope line, else the tail of the output, else the wait error.
func passErrorText(printed []byte, waitErr error) string {
	lines := strings.Split(strings.TrimSpace(string(printed)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var envelope struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Error != "" {
			return envelope.Error
		}
	}
	if text := strings.TrimSpace(string(printed)); text != "" {
		runes := []rune(text)
		if len(runes) > passErrorTextRunes {
			runes = runes[len(runes)-passErrorTextRunes:]
		}
		return string(runes)
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return ""
}

// resolveTidyModel resolves the board-tidy agent's model spec through the hub's model registry.
func resolveTidyModel(dirs configengine.Dirs) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(tidyModelSpec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	resolved, err := registry.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// runnerTidyShuttle adapts the shuttle runner over the dobby's worktree to dobbyengine.TidyShuttle.
// The runner is built on the first start, so a config that does not load fails the tidy and never the dobby.
type runnerTidyShuttle struct {
	worktree *lymcwd.Location
	once     sync.Once
	runner   *shuttleengine.Runner
	err      error
}

// Start launches spec in a fresh window and returns its strand's GUID once the agent is running.
// The run is waited on in the background, so shuttle enforces the spec's timeout and settles the run.
func (s *runnerTidyShuttle) Start(spec shuttleengine.Spec) (string, error) {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return "", s.err
	}
	run, err := s.runner.StartGated(spec, nil)
	if err != nil {
		return "", err
	}
	go func() {
		result, err := run.Wait()
		logger.Debug("beamcli: the board-tidy agent's run ended", "outcome", result.Outcome, "error", err)
	}()
	return run.StrandGUID(), nil
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s *runnerTidyShuttle) StopStrand(guid string) error {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return s.err
	}
	return s.runner.StopStrand(guid)
}

// machineLandings returns a Landings seam that reads this machine's name on each call and passes it to read.
// A name that cannot be read fails the call, so no landing is told apart from another machine's by a missing name.
func machineLandings(machine func() (string, error), read func(machine string) ([]string, error)) func() ([]string, error) {
	return func() ([]string, error) {
		name, err := machine()
		if err != nil {
			return nil, fmt.Errorf("this machine's name is unknown, so its landings cannot be told apart: %w; way forward: give the machine a hostname", err)
		}
		return read(name)
	}
}
