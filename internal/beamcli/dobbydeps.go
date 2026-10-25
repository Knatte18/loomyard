// dobbydeps.go fills dobbyengine's seams over a real hub: the runs and tasks it reads, the pass it spawns, the notice, owner pause and board tidy it sends, and the registration of the dobby process itself.

package beamcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
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

// hubDobbyDeps fills every seam of dobbyengine.Run over the hub of prime, for the dobby of geom.
// The dobby's own working directory is the hub root, so every path comes from prime and geom, and only the pass spawn starts its process in the prime's anchor.
func hubDobbyDeps(prime *lyxcwd.Location, geom dobbyengine.Geometry) (dobbyengine.Deps, error) {
	worktree, err := lyxcwd.ResolveWorktree(geom.WorktreeRoot)
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
	parents := &parentCache{prime: prime, known: map[string]string{}}
	owners := func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) }
	slugOwner := func(slug string) (string, error) {
		read, err := owners()
		if err != nil {
			return "", err
		}
		return read.Effective(slug, parents.of)
	}
	deps := dobbyengine.Deps{
		Now:   time.Now,
		Awake: pauserecord.MonotonicAwake(),
		Sleep: sleepContext,
		Watch: func() (<-chan struct{}, func(), error) {
			return openDobbyWatch(dobbyWatchDirs(prime, pause.RecordPath, boardDir))
		},
		Machine:       machinename.Name,
		CheckBinary:   selfexec.New().Check,
		ProbeInterval: probe,
		Owner: func() (string, error) {
			read, err := owners()
			if err != nil {
				return "", err
			}
			if geom.Slug == "" {
				return read.Prime, nil
			}
			return read.Effective(geom.Slug, parents.of)
		},
		Runs: func() ([]dobbyengine.RunView, error) { return dobbyRuns(prime, geom, parents, pause.RecordPath) },
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
		LiftOwnerPause: func(_ context.Context, slug string) error {
			return liftOwnerPause(prime, pause.RecordPath, slug)
		},
		Tasks:      func() ([]dobbyengine.TaskView, error) { return dobbyTasks(prime, boardDir, parents) },
		LoadConfig: func() (dobbyengine.Config, error) { return dobbyengine.LoadConfig(dirs) },
		StatusExists: func(slug string) (bool, error) {
			return fileExists(shedrun.StatusFile(prime, slug))
		},
		PauseCovers: func(slug string) (bool, error) {
			coverage, err := pauserecord.ReadCoverage(pause.RecordPath, slug, time.Now())
			return coverage.Covered, err
		},
		Landings:    func() ([]string, error) { return dobbyLandings(prime, geom, parents) },
		StencilsDir: fabricengine.StencilsDir(prime.HubPath),
		NearestOrch: func() (string, error) {
			parent, err := hubgeom.NearestLiveOrch(worktree, orchcli.OrchLive)
			return parent.Name, err
		},
	}
	if choice, err := resolveTidyModel(dirs); err != nil {
		logger.Warn("beamcli: the board-tidy model does not resolve; the board tidy is off", "spec", tidyModelSpec, "error", err)
	} else {
		deps.TidyModel = choice
		deps.TidyShuttle = &runnerTidyShuttle{worktree: worktree}
	}
	return deps, nil
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

// parentCache answers hubgeom.ParentOf once per slug, since a run's parent never changes and the answer reads the worktree list.
type parentCache struct {
	prime *lyxcwd.Location
	mu    sync.Mutex
	known map[string]string
}

// of returns the slug of the pair slug was forked from, "" for the prime.
func (c *parentCache) of(slug string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if parent, ok := c.known[slug]; ok {
		return parent, nil
	}
	parent, err := hubgeom.ParentOf(c.prime, slug)
	if err != nil {
		return "", err
	}
	c.known[slug] = parent
	return parent, nil
}

// pairLocation returns slug's task pair as a location built from the prime's, with no git call.
func pairLocation(prime *lyxcwd.Location, slug string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: prime.HubPath, WorktreeName: slug, AnchorRel: prime.AnchorRel}
}

// readStatus reads the status file at path, reporting false when it does not exist.
func readStatus(path string) (shedengine.Status, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return shedengine.Status{}, false, nil
	}
	if err != nil {
		return shedengine.Status{}, false, err
	}
	status, err := shedengine.ParseStatusTolerant(data)
	if err != nil {
		return shedengine.Status{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return status, true, nil
}

// modTime returns the modification time of path, the zero time when it does not exist.
func modTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// lockHeld reports whether the lock at path is held, tried without blocking and released at once when it was free.
// A lock whose directory does not exist has never been taken.
func lockHeld(path string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// parentRun is one batten run whose parent worktree is the dobby's own, with its status as last written.
type parentRun struct {
	slug   string
	status shedengine.Status
}

// parentRuns lists the batten runs of the prime whose parent worktree is geom's, read from the prime's local status files whatever each run's state.
// A run whose seed or parent cannot be read is left out with one Warn, so one broken run never stops the rest.
func parentRuns(prime *lyxcwd.Location, geom dobbyengine.Geometry, parents *parentCache) ([]parentRun, error) {
	slugs, err := shedrun.List(prime)
	if err != nil {
		return nil, err
	}
	var runs []parentRun
	for _, slug := range slugs {
		seed, found, err := shedrun.ReadSeed(prime, slug)
		if err != nil {
			logger.Warn("beamcli: a run's seed is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if !found || seed.Recipe != shedrun.RecipeBatten {
			continue
		}
		parent, err := parents.of(slug)
		if err != nil {
			logger.Warn("beamcli: a run's parent is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if parent != geom.Slug {
			continue
		}
		status, found, err := readStatus(shedrun.StatusFile(prime, slug))
		if err != nil {
			logger.Warn("beamcli: a run's status is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if found {
			runs = append(runs, parentRun{slug: slug, status: status})
		}
	}
	return runs, nil
}

// dobbyRuns builds the RunView of every batten run whose parent worktree is geom's.
func dobbyRuns(prime *lyxcwd.Location, geom dobbyengine.Geometry, parents *parentCache, pauseRecord string) ([]dobbyengine.RunView, error) {
	runs, err := parentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	record, err := pauserecord.Read(pauseRecord)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	views := make([]dobbyengine.RunView, 0, len(runs))
	for _, run := range runs {
		slug := run.slug
		view := dobbyengine.RunView{
			Slug:     slug,
			State:    string(run.status.State),
			Producer: run.status.CurrentProducer,
			Covered:  record.CoverageOf(slug, now).Covered,
		}
		pair := pairLocation(prime, slug)
		reads := []func() error{
			func() (err error) { view.StatusModified, err = modTime(shedrun.StatusFile(prime, slug)); return },
			func() (err error) {
				view.ChildStatusModified, err = modTime(shedrun.StatusFile(pair, shedrun.SelfRunID))
				return
			},
			func() (err error) { view.CancelMarker, err = fileExists(shedrun.CancelMarker(prime, slug)); return },
			func() (err error) { view.OriginOwed, err = fileExists(shedrun.OriginOwedMarker(prime, slug)); return },
			func() (err error) { view.DriverLockHeld, err = lockHeld(shedrun.DriverLock(prime, slug)); return },
		}
		for _, read := range reads {
			if err := read(); err != nil {
				return nil, fmt.Errorf("read the run %s: %w", slug, err)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// dobbyLandings lists the slugs of the runs of geom's parent worktree whose batten history holds a Land that finished.
func dobbyLandings(prime *lyxcwd.Location, geom dobbyengine.Geometry, parents *parentCache) ([]string, error) {
	runs, err := parentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	var landed []string
	for _, run := range runs {
		if slices.ContainsFunc(run.status.History, func(entry shedengine.HistoryEntry) bool {
			return entry.Producer == battenrecipe.NameLand && entry.Outcome == shedengine.Done
		}) {
			landed = append(landed, run.slug)
		}
	}
	return landed, nil
}

// dobbyTasks reads the local board as TaskViews.
// Only a task a dobby could start, one with an empty status and every dependency done, gets its parent worktree read; the others carry none.
// A dependency no longer on the board was pruned as done.
func dobbyTasks(prime *lyxcwd.Location, boardDir string, parents *parentCache) ([]dobbyengine.TaskView, error) {
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return nil, err
	}
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return nil, err
	}
	here, err := machinename.Name()
	if err != nil {
		return nil, err
	}
	statusOf := make(map[string]*string, len(tasks))
	for _, task := range tasks {
		statusOf[task.Slug] = task.Status
	}
	views := make([]dobbyengine.TaskView, 0, len(tasks))
	for _, task := range tasks {
		view := dobbyengine.TaskView{Slug: task.Slug, Kind: task.Kind, Status: task.Status, DependenciesDone: true}
		for _, dep := range task.DependsOn {
			if status, present := statusOf[dep]; present && (status == nil || *status != "done") {
				view.DependenciesDone = false
				break
			}
		}
		if task.Kind == boardengine.KindTask && boardengine.IsEmptyStatus(task.Status) && view.DependenciesDone {
			parent, err := parents.of(task.Slug)
			if err != nil {
				return nil, fmt.Errorf("read the parent of %s: %w", task.Slug, err)
			}
			if parent == "" {
				parent = prime.WorktreeName
			}
			view.Parent = parent
			owner, err := owners.Effective(task.Slug, parents.of)
			if err != nil {
				return nil, fmt.Errorf("read the owner of %s: %w", task.Slug, err)
			}
			view.OwnedElsewhere = boardengine.OwnedElsewhere(owner, here)
		}
		encoded, err := json.Marshal(task)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(encoded)
		view.Fingerprint = hex.EncodeToString(sum[:8])
		views = append(views, view)
	}
	return views, nil
}

// liftOwnerPause lifts the live pause entry of slug's own scope that carries the owner caller.
func liftOwnerPause(prime *lyxcwd.Location, recordPath, slug string) error {
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

// spawnPass starts `<lyx> batten run <slug>` for geom as its own process, in the prime's anchor, and delivers its exit once.
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
	cmd := exec.Command(geom.LyxExecutable, args...)
	cmd.Dir = geom.PrimeAnchor
	cmd.Stdout = output
	cmd.Stderr = output
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("beamcli: spawn batten pass failed", "exe", geom.LyxExecutable, "args", args, "dir", geom.PrimeAnchor, "error", err)
		return nil, err
	}
	logger.Info("beamcli: spawned batten pass", "exe", geom.LyxExecutable, "args", args, "dir", geom.PrimeAnchor, "pid", cmd.Process.Pid)
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
	worktree *lyxcwd.Location
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
