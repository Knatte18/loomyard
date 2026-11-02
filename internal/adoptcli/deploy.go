// deploy.go is adopt's deploy step: before the owner write that gives this machine the prime, it runs the hub's landing deploy when the running lym was built from a commit the prime's head is ahead of, so no run goes on under a lym older than the code it lands.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// deployLogName is the file the deploy step's command writes its output to, in root's adopt scratch directory.
const deployLogName = "deploy.log"

// deployState is what the deploy step reads before it decides.
// Command is the landing config's deploy command, empty when the config deploys nothing into the prime's branch.
// Installed labels the build of the running lym, Head is the prime's head commit, and Behind reports that Head is ahead of the commit the running lym was built from.
type deployState struct {
	Command   string
	Installed string
	Head      string
	Behind    bool
}

// deployOutcome is a deploy the step ran: the build it replaced, the prime's head it deployed and the log of its command.
type deployOutcome struct {
	Installed string `json:"installed"`
	Head      string `json:"prime_head"`
	Log       string `json:"log"`
}

// deployStale runs the landing deploy when the state deps.DeployState reads has a command and the prime's head ahead of the running lym's build, and returns what it ran.
// A state that does not read is a warning naming the hand deploy, and nothing runs.
// A deploy that fails is the verb's refusal, naming its log and the re-run, since a run let go under the old lym fails on code that lym does not know.
func deployStale(deps adoptDeps) (ran *deployOutcome, warning string, err error) {
	if deps.DeployState == nil {
		return nil, "", nil
	}
	state, err := deps.DeployState()
	if err != nil {
		return nil, fmt.Sprintf("whether the installed lym predates the prime's head could not be read (%v), so no deploy ran; way forward: if it does, run the hub's landing deploy in the prime before any run goes on", err), nil
	}
	if state.Command == "" || !state.Behind {
		return nil, "", nil
	}
	logPath, err := deps.RunDeploy(state.Command)
	ran = &deployOutcome{Installed: state.Installed, Head: state.Head, Log: logPath}
	if err != nil {
		return ran, "", fmt.Errorf("lym adopt: the installed lym was built from %s, which the prime's head %s is ahead of, and the landing deploy failed: %w; log: %s; no owner was recorded and the holds stay, so no run goes on under the old lym: fix the cause, %s", state.Installed, state.Head, err, logPath, reentry)
	}
	return ran, "", nil
}

// runningLymBuild reads the VCS stamp of the lym executable that is running.
func runningLymBuild() (buildvcs.Identity, error) {
	executable, err := os.Executable()
	if err != nil {
		return buildvcs.Identity{}, fmt.Errorf("locate the lym executable: %w", err)
	}
	return buildvcs.OfFile(executable)
}

// readDeployState reads the deploy step's state over the prime: the landing config's deploy command when its deploy_branches list the branch the prime is on, the running lym's build as readInstalled answers it, and the prime's head.
// When the config has no deploy command or deploys nothing into the prime's branch the state is empty, and readInstalled is not called.
func readDeployState(prime *lymcwd.Location, readInstalled func() (buildvcs.Identity, error)) (deployState, error) {
	cfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(prime), "landing")
	if err != nil {
		return deployState{}, fmt.Errorf("read the landing config: %w", err)
	}
	if cfg.Deploy == "" {
		return deployState{}, nil
	}
	fab, err := fabricengine.Open(prime)
	if err != nil {
		return deployState{}, err
	}
	branch, err := fab.CurrentBranch()
	if err != nil {
		return deployState{}, err
	}
	if !slices.Contains(cfg.DeployBranches, branch) {
		return deployState{}, nil
	}
	installed, err := readInstalled()
	if err != nil {
		return deployState{}, err
	}
	head, err := fab.HeadSHA()
	if err != nil {
		return deployState{}, err
	}
	behind, err := installedBehind(installed, head, fab.CommitExists, fab.IsAncestor)
	if err != nil {
		return deployState{}, err
	}
	return deployState{Command: cfg.Deploy, Installed: installed.Label(), Head: head, Behind: behind}, nil
}

// installedBehind reports whether head is ahead of the commit the installed build was stamped with, judged through exists and ancestor over the prime's repository.
// A build with no VCS stamp is an error, since nothing can judge it.
// A build of head itself, of a commit the repository lacks, as a build of unpushed local work, or of a commit that is not an ancestor of head is not behind.
func installedBehind(installed buildvcs.Identity, head string, exists func(sha string) bool, ancestor func(sha, ref string) (bool, error)) (bool, error) {
	if installed.Revision == "" {
		return false, errors.New("the installed lym carries no VCS stamp")
	}
	if installed.Revision == head || !exists(installed.Revision) {
		return false, nil
	}
	return ancestor(installed.Revision, head)
}

// runAdoptDeploy runs command in the prime's worktree with its output in deployLogName in root's adopt scratch directory, ending it after the Land row's deploy timeout, and returns the log's path.
// A shell that could not start, a timeout and a non-zero exit are errors.
// It runs nothing and returns an error in a test binary, where the hub's shell command must not run.
func runAdoptDeploy(prime *lymcwd.Location, command string) (string, error) {
	if testing.Testing() {
		return "", errors.New("the deploy runs the hub's shell command, which a test binary must not")
	}
	dir := subjectDir(prime.HubPath, boardengine.RootSubject)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create the adopt scratch directory: %w", err)
	}
	logPath := filepath.Join(dir, deployLogName)
	result, err := battencli.RunDeploy(context.Background(), prime.WorktreePath(), command, logPath, battenshed.DeployTimeout)
	return logPath, deployFailure(result, err)
}

// deployFailure maps how a deploy command ended to an error: err when the shell could not start, then a timeout, then a non-zero exit code.
// A command that ran and exited zero is nil.
func deployFailure(result battenshed.DeployResult, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("the shell could not start: %w", err)
	case result.TimedOut:
		return fmt.Errorf("timed out after %s", battenshed.DeployTimeout)
	case result.ExitCode != 0:
		return fmt.Errorf("exit code %d", result.ExitCode)
	}
	return nil
}
