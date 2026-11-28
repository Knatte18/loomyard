// sandbox.go settles the sandbox a Codex launch runs in.
// It probes whether Codex's sandbox can start on this machine and resolves codex.yaml's sandbox_mode against the result.

package codexengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// sandboxProbeTimeout bounds one probe of Codex's sandbox; a probe that outlasts it did not run to a verdict.
const sandboxProbeTimeout = 30 * time.Second

// sandboxProbeCommand is the command the probe runs inside the sandbox: one that does nothing and exits 0.
const sandboxProbeCommand = "true"

// errSandboxProbeNotRun marks a probe that could not run to a verdict.
// The binary may be missing or unexecutable, or a timeout or a signal may have ended the probe.
// Such a probe shows nothing about whether the sandbox starts.
var errSandboxProbeNotRun = errors.New("the sandbox probe did not run to a verdict")

// runSandboxProbe runs `codex sandbox` with the workspace-write policy under the Codex home at home, in dir.
// The command it runs there does nothing, so the probe reports only whether the sandbox itself starts.
// It does not run the launch's writable roots or network setting,
// and an empty dir runs it in the process's working directory.
// It returns nil when the sandbox starts.
// When the executable exits non-zero it returns an error carrying the command's output.
// That output is, for example, bubblewrap's refusal to set up its namespace.
// When the probe could not run to a verdict, the error wraps the did-not-run sentinel instead.
// A caller tells the two apart with errors.Is.
func runSandboxProbe(bin, home, dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sandboxProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "sandbox", "-c", "sandbox_mode="+tomlString(sandboxWorkspaceWrite), "--", sandboxProbeCommand)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), codexHomeEnvKey+"="+home)
	logger.Info("codexengine: probing whether Codex's sandbox starts", "binary", bin, "dir", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if ctx.Err() != nil || !errors.As(err, &exitErr) || exitErr.ExitCode() < 0 {
			return fmt.Errorf("%w: %s sandbox: %w", errSandboxProbeNotRun, bin, err)
		}
		if output := strings.TrimSpace(string(out)); output != "" {
			return fmt.Errorf("%s sandbox: %w: %s", bin, err, output)
		}
		return fmt.Errorf("%s sandbox: %w", bin, err)
	}
	logger.Info("codexengine: Codex's sandbox started", "binary", bin)
	return nil
}

// launchSandboxMode returns the sandbox a launch runs in, from codex.yaml's sandbox_mode and the probe.
// danger-full-access runs no probe and no sandbox.
// Under auto and workspace-write the probe runs once, under the lym home at home, in the engine's anchor;
// a sandbox that starts gives workspace-write.
// One that refuses to start gives danger-full-access with a warning naming the cause under auto,
// so a machine whose bubblewrap may not create a user namespace runs its sessions as a Claude seat runs.
// Under workspace-write it refuses the launch and names the cause.
// A session started there would fail every shell command.
// A probe that did not run to a verdict refuses the launch under both modes and names the cause.
// It shows nothing about the sandbox, so dropping the write bound on it would be unfounded.
func (c *Codex) launchSandboxMode(codex Config, home string) (string, error) {
	if codex.SandboxMode == sandboxFullAccess {
		return sandboxFullAccess, nil
	}
	err := c.sandboxProbe(codexBinary(codex), home, c.dirs.Anchor)
	if err == nil {
		return sandboxWorkspaceWrite, nil
	}
	if errors.Is(err, errSandboxProbeNotRun) {
		return "", fmt.Errorf("codexengine: %w; way forward: fix the Codex executable, or launch again if the cause was transient", err)
	}
	if codex.SandboxMode == sandboxAuto {
		c.warn("codexengine: Codex's sandbox cannot start on this machine, so the session runs with no sandbox", "sandbox_mode", sandboxAuto, "cause", err)
		return sandboxFullAccess, nil
	}
	return "", fmt.Errorf("codexengine: codex.yaml's sandbox_mode is %s, but Codex's sandbox cannot start on this machine: %w; way forward: set it for this machine with lym config codex --layer machine --set sandbox_mode=%s from the hub's prime worktree", sandboxWorkspaceWrite, err, sandboxAuto)
}
