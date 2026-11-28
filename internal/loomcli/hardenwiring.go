// hardenwiring.go holds the wiring a hardener run adds to wire.
// It covers these parts of the run:
// the Harden-Review segment's settings, the campaign and its seams, and the reap's production seams.
// It also covers the live gate's spawn, the Harden-Review budget and hard cap, and the recipe-keyed tree scope.
// A recipe without a HardenFix row leaves all of it unwired.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// hardenFixEngine is the engine name of the recipe row whose presence makes a run a hardener run.
const hardenFixEngine = "HardenFix"

// hardenReviewSegment is the name of the recipe segment whose rows read hardener.yaml's round keys.
const hardenReviewSegment = "Harden-Review"

// hardenConfigFile is the config file the Harden-Review segment's keys live in.
const hardenConfigFile = hardenshed.ConfigModule + ".yaml"

// hardenResumeClause closes the way forward of a hardener model-spec refusal.
const hardenResumeClause = `then run "lym loom resume"`

// hardenSegmentSettings builds the Harden-Review segment's settings from hardener.yaml.
// The reviewer and fixer lists are expanded cyclically to hardener.hard_max_bounces and resolved through registry.
// Round n therefore takes entry n modulo the list's length, up to the hard cap.
// The Bouncer's judge and seed are hardener.judge and hardener.seed when set.
// Otherwise they are loom.code.judge and loom.code.seed.
// A model-spec the registry cannot resolve is refused naming its hardener.yaml key.
func hardenSegmentSettings(cfg hardenshed.Config, loomCfg loomengine.Config, registry modelspec.Registry) (shedrecipe.SegmentSettings, error) {
	reviewKey := shuttleengine.ModelKey{Module: hardenshed.ConfigModule, Key: "reviewer"}
	fixKey := shuttleengine.ModelKey{Module: hardenshed.ConfigModule, Key: "fixer"}
	review, err := resolveHardenModels(reviewKey, hardenshed.Rotate(cfg.Reviewer, cfg.HardMaxBounces), registry)
	if err != nil {
		return shedrecipe.SegmentSettings{}, err
	}
	fix, err := resolveHardenModels(fixKey, hardenshed.Rotate(cfg.Fixer, cfg.HardMaxBounces), registry)
	if err != nil {
		return shedrecipe.SegmentSettings{}, err
	}
	judges, err := loomengine.ResolveJudge(loomCfg, registry)
	if err != nil {
		return shedrecipe.SegmentSettings{}, err
	}
	bouncer := shedadapters.BouncerModels(judges.Code)
	for _, told := range []struct {
		key  string
		spec string
		into *shuttleengine.ModelChoice
	}{{"judge", cfg.Judge, &bouncer.Judge}, {"seed", cfg.Seed, &bouncer.Seed}} {
		if told.spec == "" {
			continue
		}
		if *told.into, err = resolveHardenModel(shuttleengine.ModelKey{Module: hardenshed.ConfigModule, Key: told.key}, told.spec, registry); err != nil {
			return shedrecipe.SegmentSettings{}, err
		}
	}
	return shedrecipe.SegmentSettings{
		Models:            burlerengine.RoundModels{Review: review, Fix: fix},
		ReviewKey:         reviewKey,
		FixKey:            fixKey,
		Bouncer:           bouncer,
		MaxBounces:        cfg.MaxBounces,
		MaxBouncesKey:     "max_bounces",
		HardMaxBounces:    cfg.HardMaxBounces,
		HardMaxBouncesKey: "hard_max_bounces",
		AmendMaxBounces:   loomCfg.Review.AmendMaxBounces,
		ConfigFile:        hardenConfigFile,
	}, nil
}

// resolveHardenModels resolves each model-spec of specs through registry, naming key and the entry in a refusal.
func resolveHardenModels(key shuttleengine.ModelKey, specs []string, registry modelspec.Registry) ([]shuttleengine.ModelChoice, error) {
	choices := make([]shuttleengine.ModelChoice, 0, len(specs))
	for _, raw := range specs {
		choice, err := resolveHardenModel(key, raw, registry)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

// resolveHardenModel resolves one model-spec through registry, naming key in a refusal.
func resolveHardenModel(key shuttleengine.ModelKey, raw string, registry modelspec.Registry) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(raw)
	if err == nil {
		var resolved modelspec.Resolved
		if resolved, err = registry.Resolve(spec); err == nil {
			return shuttleengine.NewModelChoice(resolved), nil
		}
	}
	return shuttleengine.ModelChoice{}, fmt.Errorf("hardener: %s key %q entry %q: %w; way forward: set the entry in lym config %s to a model-spec the registry defines, %s",
		hardenConfigFile, key.Key, raw, err, hardenshed.ConfigModule, hardenResumeClause)
}

// hardenCampaign builds the run's campaign over cfg.
// Its seams are the live gate's spawn, the reap, the head and commit reads, the archive commit and the board entry read.
// runDir is the run directory the Harden rows share.
func hardenCampaign(location *lymcwd.Location, cfg hardenshed.Config, runDir string) *hardenshed.Campaign {
	return &hardenshed.Campaign{
		Config:             cfg,
		WorktreeRoot:       location.WorktreePath(),
		AnchorPath:         location.AnchorPath(),
		RunDir:             runDir,
		RunLive:            hardenLiveRunner(location),
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		Reap: func(row string, point hardenshed.ReapPoint) ([]hardenshed.StrayEntry, error) {
			deps, err := HardenReapDeps(location, row, point)
			if err != nil {
				return nil, err
			}
			return hardenshed.Reap(deps)
		},
		Head: func() (string, error) {
			handle, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			return handle.HeadSHA()
		},
		Commits: func(from string) ([]string, error) {
			handle, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			return handle.CommitLog(from, "HEAD")
		},
		Commit: func(relPaths []string, message string) error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, relPaths, message, fabricengine.EnvSyncOptions())
			return err
		},
		Entry: hardenBoardEntry(location),
	}
}

// boardTaskReader is the one board read the campaign's entry seam makes.
type boardTaskReader interface {
	GetTask(idOrSlug any) (boardengine.Task, bool, error)
}

// readBoardEntry returns the title and labels of the board entry slug, read through the board that open returns.
// An entry the board lacks is refused with a way forward that names the modules through hardener.packages.
func readBoardEntry(open func() (boardTaskReader, error), slug string) (string, []string, error) {
	board, err := open()
	if err != nil {
		return "", nil, fmt.Errorf("open the board: %w", err)
	}
	task, found, err := board.GetTask(slug)
	if err != nil {
		return "", nil, fmt.Errorf("read the board entry %q: %w", slug, err)
	}
	if !found {
		return "", nil, fmt.Errorf("the board holds no entry %q; way forward: name the modules in the task's run_settings with hardener.packages, then run \"lym loom resume\"", slug)
	}
	return task.Title, task.Labels, nil
}

// hardenBoardEntry returns the campaign's entry seam.
// It reads the title and labels of the worktree's own entry on the hub's board.
func hardenBoardEntry(location *lymcwd.Location) func() (string, []string, error) {
	return func() (string, []string, error) {
		return readBoardEntry(func() (boardTaskReader, error) { return boardengine.OpenHub(location.HubPath) }, seedSlug(location.WorktreeName))
	}
}

// HardenReapDeps builds the production seams of a sandbox reap of the worktree at location, run for row at point.
// It is the one builder the Harden rows, Finalize's reap and `lym harden sandbox clear` share,
// so each ends the same processes and servers.
// The process scan reads the process table, which only Linux provides, and the scan is off elsewhere.
func HardenReapDeps(location *lymcwd.Location, row string, point hardenshed.ReapPoint) (hardenshed.ReapDeps, error) {
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		return hardenshed.ReapDeps{}, err
	}
	tmuxPath, err := exec.LookPath(reedCfg.Tmux)
	if err != nil {
		return hardenshed.ReapDeps{}, fmt.Errorf("find the tmux binary %q the reed config names: %w", reedCfg.Tmux, err)
	}
	deps := hardenshed.ReapDeps{
		Root:  hardenshed.SandboxRoot(location.WorktreePath()),
		Row:   row,
		Point: point,
		Owners: func(hubPath string) (map[string][]runproc.Record, error) {
			return runproc.Owners(fabriclayout.RunProcDir(hubPath))
		},
		End: func(hubPath, owner string, callerPID int) ([]runproc.Ended, error) {
			return runproc.End(fabriclayout.RunProcDir(hubPath), owner, callerPID)
		},
		EndProcess: runproc.EndProcess,
		StopServer: func(hubPath string) (bool, error) {
			return reedengine.StopServer(tmuxPath, hubPath)
		},
		CallerPID: os.Getpid(),
	}
	if runtime.GOOS == "linux" {
		deps.ListProcesses = proc.ListProcesses
		deps.ListStats = proc.ListStats
	}
	return deps, nil
}

// hardenLiveArgs returns the arguments of a live gate call.
// They are `gate test --wait --no-carry --gate <key>`, then `--copies <n>` when copies is above 0, then the packages.
// A package path go would read as an import path, such as `internal/x`, gets a `./` prefix.
// Go resolves a bare path in the standard library, and the call fails before any test runs.
// It names no tier, so the hub's gate key alone says what the call runs.
// It carries `--no-carry`: a live run that a carried green record replaced would start no test binary and prove nothing.
func hardenLiveArgs(key string, copies int, packages []string) []string {
	args := []string{"gate", "test", "--wait", "--" + gateslot.NoCarryFlag, "--gate", key}
	if copies > 0 {
		args = append(args, "--copies", fmt.Sprint(copies))
	}
	for _, pkg := range packages {
		if !build.IsLocalImport(pkg) {
			pkg = "./" + pkg
		}
		args = append(args, pkg)
	}
	return args
}

// hardenLiveEnv returns env without the held gate slot's variable,
// so the live call is a top-level one that acquires its own slots and never runs inside a slot the row holds.
func hardenLiveEnv(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		return strings.HasPrefix(entry, gateslot.InheritEnv+"=")
	})
}

// hardenLiveRun reads a finished live gate call into a LiveRun.
// Passed is exit code 0.
// Output is stderr followed by stdout, since the summary lines, the started lines and the failure report go to stderr.
// Refused is the error of stdout's last non-empty line when it is a JSON object with ok false and a non-empty error.
// It is set only when stderr holds no started line.
// Every tier run that reports prints a started line, so that matches only a call halted before any tier ran.
func hardenLiveRun(exitCode int, stdout, stderr []byte) hardenshed.LiveRun {
	run := hardenshed.LiveRun{Passed: exitCode == 0, Output: string(stderr) + string(stdout)}
	if run.Passed || len(gateslot.ParseStartedLines(string(stderr))) > 0 {
		return run
	}
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(lines[len(lines)-1])), &envelope) == nil && envelope.OK != nil && !*envelope.OK {
		run.Refused = envelope.Error
	}
	return run
}

// hardenLiveRunner returns the live gate's spawn.
// It runs `lym` with hardenLiveArgs in the worktree and reads the finished call through hardenLiveRun.
// A spawn or wait failure is its error; a nonzero exit is the call's exit code.
func hardenLiveRunner(location *lymcwd.Location) func(key string, copies int, packages []string) (hardenshed.LiveRun, error) {
	return func(key string, copies int, packages []string) (hardenshed.LiveRun, error) {
		binary, err := selfexec.LymBinary()
		if err != nil {
			return hardenshed.LiveRun{}, fmt.Errorf("resolve the lym binary: %w", err)
		}
		args := hardenLiveArgs(key, copies, packages)
		cmd := exec.Command(binary, args...)
		cmd.Dir = location.WorktreePath()
		cmd.Env = hardenLiveEnv(os.Environ())
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		logger.Info("loomcli: starting the hardener live gate call", "args", args, "dir", cmd.Dir)
		runErr := cmd.Run()
		exitCode := 0
		var exitErr *exec.ExitError
		switch {
		case errors.As(runErr, &exitErr):
			exitCode = exitErr.ExitCode()
		case runErr != nil:
			return hardenshed.LiveRun{}, fmt.Errorf("run lym %s: %w", strings.Join(args, " "), runErr)
		}
		logger.Info("loomcli: the hardener live gate call ended", "args", args, "exit_code", exitCode)
		return hardenLiveRun(exitCode, stdout.Bytes(), stderr.Bytes()), nil
	}
}

// hardenPreflight returns the campaign check Preflight runs for a hardener run over cfg.
// It reads the hub's gate config at each call, so a gate.yaml edit between two steps takes effect.
// A hub with no gate.yaml is refused with the config's own way forward, since the live gate could not run.
// A config naming no package reads the run's board entry through entry.
// A missing one is so refused before any agent spends tokens.
func hardenPreflight(location *lymcwd.Location, cfg hardenshed.Config, entry func() (string, []string, error)) func() error {
	return func() error {
		dirs := fabricengine.ConfigDirs(location)
		gateCfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return err
		}
		if len(cfg.Packages) == 0 {
			if _, _, err := entry(); err != nil {
				return err
			}
		}
		return hardenshed.CheckCampaign(hardenshed.CampaignCheck{
			Config:        cfg,
			WorktreeRoot:  location.WorktreePath(),
			Live:          hardenLiveKey(gateCfg, hardenshed.LiveGateKey),
			HoldsTierTest: impactset.HoldsTierTest,
			Route: func(key string) (string, error) {
				return reviewbudget.Route(dirs, hardenshed.ConfigSpec(), key)
			},
		})
	}
}

// hardenLiveKey reads what the campaign check needs of the gate key name as gateCfg resolves it, template included:
// its tiers, and the tiers a run of it tests, each tier's covers included.
func hardenLiveKey(gateCfg gateslot.Config, name string) hardenshed.LiveKey {
	gate, found := gateCfg.Gates[name]
	if !found {
		return hardenshed.LiveKey{}
	}
	tested := slices.Clone(gate.Tiers)
	for _, tier := range gate.Tiers {
		tested = append(tested, gateCfg.Tiers[fmt.Sprintf("tier%d", tier)].Covers...)
	}
	slices.Sort(tested)
	return hardenshed.LiveKey{Tiers: gate.Tiers, Tested: slices.Compact(tested)}
}

// wireHarden fills the Env fields a hardener run reads: the Harden-Review segment, the campaign and Preflight's check.
// A recipe without a HardenFix row leaves them unset.
// hardener.yaml loads strictly, so a malformed file is refused before any row builds, naming its own way forward.
func (c *loomCLI) wireHarden(location *lymcwd.Location, loomCfg loomengine.Config, registry modelspec.Registry) error {
	if !recipeHasEngine(c.recipe, hardenFixEngine) {
		return nil
	}
	cfg, err := hardenshed.LoadConfig(fabricengine.ConfigDirs(location), hardenshed.ConfigModule)
	if err != nil {
		return err
	}
	settings, err := hardenSegmentSettings(cfg, loomCfg, registry)
	if err != nil {
		return err
	}
	subdir, found, err := loomrecipe.BouncerRunSubdir(loomshed.NameHardenBouncer)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("loomcli: the hardener recipe's %s row has no run_subdir, so this lym build is broken", loomshed.NameHardenBouncer)
	}
	c.env.Segments[hardenReviewSegment] = settings
	campaign := hardenCampaign(location, cfg, filepath.Join(loomengine.LoomReviewsDir(location), subdir))
	c.env.Harden = campaign
	c.env.PreflightHarden = hardenPreflight(location, cfg, campaign.Entry)
	return nil
}

// withHardenBudget returns budgets with the Harden-Review budget added when recipe holds a HardenFix row.
// That budget is hardener.max_bounces.
// A file that does not load falls back to the template's value with a warning.
// A status or pause verb is therefore not refused over it, as with the loom review budget and loom.yaml.
// Any other recipe's budgets come back unchanged.
func withHardenBudget(budgets map[string]int, recipe string, dirs configengine.Dirs) (map[string]int, error) {
	if !recipeHasEngine(recipe, hardenFixEngine) {
		return budgets, nil
	}
	cfg, err := hardenshed.LoadConfig(dirs, hardenshed.ConfigModule)
	if err != nil {
		logger.Warn("loom: could not read hardener.yaml for the Harden-Review bounce budget; reporting the template default", "error", err)
		cfg = hardenshed.Config{}
		if err := yaml.Unmarshal([]byte(hardenshed.ConfigTemplate()), &cfg); err != nil {
			return nil, err
		}
	}
	withBudget := make(map[string]int, len(budgets)+1)
	maps.Copy(withBudget, budgets)
	withBudget[hardenReviewSegment] = cfg.MaxBounces
	return withBudget, nil
}

// hardenHardCap returns the hard cap of the Harden-Bouncer row and the way forward that raises it.
// The cap is hardener.hard_max_bounces, and its raise is routed through the hardener module.
func hardenHardCap(location *lymcwd.Location) (int, string, error) {
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := hardenshed.LoadConfig(dirs, hardenshed.ConfigModule)
	if err != nil {
		return 0, "", err
	}
	raise, err := reviewbudget.Route(dirs, hardenshed.ConfigSpec(), "hard_max_bounces")
	if err != nil {
		return 0, "", err
	}
	return cfg.HardMaxBounces, raise, nil
}

// recipeTrees returns the reader of the tree map the run's verify scope classifies a diff by.
// A recipe holding a HardenFix row reads hardener.tree_scope, and any other reads darn.tree_scope.
// Each call reads the file afresh, so an edit between steps is seen.
// A load error carries its way forward.
// That is the editor's for a malformed tree key, and the hub reconcile's for darn.yaml otherwise.
func recipeTrees(l *lymcwd.Location, recipe string) func() (map[string]string, error) {
	return func() (map[string]string, error) {
		dirs := fabricengine.ConfigDirs(l)
		if recipeHasEngine(recipe, hardenFixEngine) {
			cfg, err := hardenshed.LoadConfig(dirs, hardenshed.ConfigModule)
			if err != nil {
				return nil, err
			}
			return cfg.TreeScope, nil
		}
		cfg, err := darnengine.LoadConfig(dirs, "darn")
		if err != nil {
			if errors.Is(err, darnengine.ErrTreeScopeKey) {
				return nil, darnengine.WithEditorWayForward(err)
			}
			return nil, fmt.Errorf("%w; %s", err, hubConfigWayForward)
		}
		return cfg.TreeScope, nil
	}
}
