// gitload.go times a git workload shaped like a fabric test fixture:
// init a bare repository, clone it, add and commit a file, and push, once per round.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitBinary is the git the probe starts.
const gitBinary = "git"

// The workload's steps, in the order each round runs them.
const (
	stepInit   = "init"
	stepClone  = "clone"
	stepAdd    = "add"
	stepCommit = "commit"
	stepPush   = "push"
)

var gitSteps = []string{stepInit, stepClone, stepAdd, stepCommit, stepPush}

// probeGitConfig replaces the machine's global git config for the workload,
// so every machine runs the same commands with no hooks, signing or maintenance of its own.
const probeGitConfig = "[user]\n\tname = machinecost\n\temail = machinecost@localhost\n" +
	"[commit]\n\tgpgsign = false\n[gc]\n\tauto = 0\n[maintenance]\n\tauto = false\n[advice]\n\tdetachedHead = false\n"

// probeFileName is the file each round adds and commits.
const probeFileName = "probe.txt"

// traceStartEvent marks one git process's start in a GIT_TRACE2_EVENT file.
const traceStartEvent = `"event":"start"`

// stepCost is one step's wall time and the user and system time of the git it ran and every child that git waited on.
type stepCost struct {
	wall, user, system time.Duration
}

// measureGitWorkload runs rounds rounds of the workload in tempDir.
// It adds each step's median wall, user and system time under git/<step>/.
// A further round under git's event trace counts the git processes each step starts;
// it is kept out of the timed rounds, since the trace writes a file from every process.
func measureGitWorkload(ctx context.Context, tempDir string, rounds int, measurements *collector) error {
	configPath := filepath.Join(tempDir, "gitconfig")
	if err := os.WriteFile(configPath, []byte(probeGitConfig), 0o644); err != nil {
		return fmt.Errorf("write git config: %w", err)
	}
	baseEnv := append(environWithoutGitVariables(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+configPath)
	untraced := func(string) []string { return baseEnv }

	costs := make(map[string][]stepCost, len(gitSteps))
	for round := range rounds {
		roundCosts, err := runGitRound(ctx, filepath.Join(tempDir, fmt.Sprintf("round-%d", round)), untraced)
		if err != nil {
			return err
		}
		for step, cost := range roundCosts {
			costs[step] = append(costs[step], cost)
		}
	}

	traceDir := filepath.Join(tempDir, "trace")
	if err := os.Mkdir(traceDir, 0o755); err != nil {
		return fmt.Errorf("create trace folder: %w", err)
	}
	traced := func(step string) []string {
		return slices.Concat(baseEnv, []string{"GIT_TRACE2_EVENT=" + filepath.Join(traceDir, step)})
	}
	if _, err := runGitRound(ctx, filepath.Join(tempDir, "round-traced"), traced); err != nil {
		return err
	}

	for _, step := range gitSteps {
		trace, err := os.ReadFile(filepath.Join(traceDir, step))
		if err != nil {
			return fmt.Errorf("read git trace of %s: %w", step, err)
		}
		walls := make([]time.Duration, 0, rounds)
		users := make([]time.Duration, 0, rounds)
		systems := make([]time.Duration, 0, rounds)
		for _, cost := range costs[step] {
			walls = append(walls, cost.wall)
			users = append(users, cost.user)
			systems = append(systems, cost.system)
		}
		prefix := groupGit + "/" + step + "/"
		measurements.add(prefix+"wall_ms", unitMilliseconds, milliseconds(median(walls)))
		measurements.add(prefix+"user_ms", unitMilliseconds, milliseconds(median(users)))
		measurements.add(prefix+"sys_ms", unitMilliseconds, milliseconds(median(systems)))
		measurements.add(prefix+"processes", unitCount, float64(strings.Count(string(trace), traceStartEvent)))
	}
	return nil
}

// runGitRound runs one round of the workload in roundDir and returns each step's cost.
// Each step's git runs under envFor(step).
// It deletes roundDir.
func runGitRound(ctx context.Context, roundDir string, envFor func(step string) []string) (map[string]stepCost, error) {
	if err := os.Mkdir(roundDir, 0o755); err != nil {
		return nil, fmt.Errorf("create round folder: %w", err)
	}
	defer os.RemoveAll(roundDir)
	bare := filepath.Join(roundDir, "origin.git")
	work := filepath.Join(roundDir, "work")

	costs := make(map[string]stepCost, len(gitSteps))
	runStep := func(step, dir string, args ...string) error {
		cost, err := runGit(ctx, dir, envFor(step), args)
		if err != nil {
			return fmt.Errorf("git workload step %s: %w", step, err)
		}
		costs[step] = cost
		return nil
	}
	if err := runStep(stepInit, roundDir, "init", "--bare", "-q", bare); err != nil {
		return nil, err
	}
	if err := runStep(stepClone, roundDir, "clone", "-q", bare, work); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, probeFileName), []byte("probe\n"), 0o644); err != nil {
		return nil, fmt.Errorf("write probe file: %w", err)
	}
	if err := runStep(stepAdd, work, "add", probeFileName); err != nil {
		return nil, err
	}
	if err := runStep(stepCommit, work, "commit", "-q", "-m", "probe"); err != nil {
		return nil, err
	}
	if err := runStep(stepPush, work, "push", "-q", "origin", "HEAD"); err != nil {
		return nil, err
	}
	return costs, nil
}

// environWithoutGitVariables returns the caller's environment without its GIT_* variables.
// Those can add config, trace a timed git or point it at another repository.
func environWithoutGitVariables() []string {
	return slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GIT_") })
}

func runGit(ctx context.Context, dir string, env, args []string) (stepCost, error) {
	cmd := exec.CommandContext(ctx, gitBinary, args...)
	cmd.Dir = dir
	cmd.Env = env
	started := time.Now()
	if out, err := cmd.CombinedOutput(); err != nil {
		return stepCost{}, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return stepCost{
		wall:   time.Since(started),
		user:   cmd.ProcessState.UserTime(),
		system: cmd.ProcessState.SystemTime(),
	}, nil
}
