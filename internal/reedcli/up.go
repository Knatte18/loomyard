// up.go implements the `up` and `down` reed verbs: up boots the substrate (server + session) this
// worktree's strands render into,
// and down tears this worktree's session and persisted state back down (the shared per-hub server
// dies only when this was its last session).
// Neither verb touches a strand's command — up never launches one.

package reedcli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/spf13/cobra"
)

// upCmd builds the `up` subcommand.
func (c *reedCLI) upCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "up",
		Short:       "boot the reed substrate (server + session) for this worktree",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `up ensures this hub's named tmux server and this worktree's session
exist (booting them if absent, a no-op if already up), then reconciles dead
panes and re-applies the current strand layout.

Before booting, up verifies the configured multiplexer binary meets the
pinned minimum version and exposes the required command surface, failing
loud with a JSON error ({"ok":false,"error":...}) if it does not.

up is substrate-only: it never launches or relaunches a strand command.
Bringing strand content back after a server restart is "lyx reed resume"'s
job, not up's.

Setting debug_log in reed.yaml (or LYX_REED_DEBUG=1) enables server verbose
logging to <hub>/_board/.lyx/logs/, as forensics for unexplained server deaths; it
applies only when this up actually boots the shared per-hub server, and
an existing reed.yaml lacking the key takes the default until "lyx config reconcile" writes it.

Example:
  lyx reed up`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.eng.Up()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Attempted after Up returns without error, before the envelope write: the daemon
			// needs a live session to discover, and up is exactly the op that ensures one exists.
			c.ensureWatchdogSpawned()

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"socket":  result.Socket,
				"strands": result.Strands,
			}))
			return nil
		},
	}
}

// endPairProcesses ends the registered processes of the pair the invoking worktree is, before `down` kills its session, and returns what it ended.
// The prime is no pair, so it ends nothing and returns an empty list; an unresolvable worktree is returned as an error.
// It reads local state only and makes no network call.
func (c *reedCLI) endPairProcesses(ctx context.Context) ([]runproc.Ended, error) {
	location, err := lyxcwd.Resolve(c.anchorPath)
	if err != nil {
		return nil, err
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, fmt.Errorf("resolve the hub's prime: %w", err)
	}
	if primeName == location.WorktreeName {
		return nil, nil
	}
	prime, err := lyxcwd.ResolveWorktree(filepath.Join(location.HubPath, primeName))
	if err != nil {
		return nil, fmt.Errorf("locate the hub's prime %q: %w", primeName, err)
	}
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return nil, err
	}
	return teardown.EndBeforeSessionDown(ctx, location.WorktreeName)
}

// endedProcessesField renders the ended processes as the envelope's list of {pid, role}, empty when none.
func endedProcessesField(ended []runproc.Ended) []map[string]any {
	list := make([]map[string]any, len(ended))
	for i, process := range ended {
		list[i] = map[string]any{"pid": process.PID, "role": process.Role}
	}
	return list
}

// downCmd builds the `down` subcommand.
func (c *reedCLI) downCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "down",
		Short:       "kill this worktree's reed session and clear its strand state",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `down kills this worktree's tmux session and deletes its persisted
strand state. Sibling worktrees sharing the hub's server are untouched;
when this was the server's last session, the now-empty server is shut down
too (down waits until the server process has actually exited). When down
ends the hub's server it also asks the watchdog daemon to stop and waits up
to ten seconds for the daemon to leave, warning and still succeeding on
timeout. down is idempotent: calling it again with no session up still
succeeds.

In a task pair, down first ends the pair's registered processes, its loom
driver, step loop and batten among them, and lists each {pid, role} in
ended_processes. It reads local state only and makes no network call. It does
so when a pause covers the pair, when its board entry reads done, when it
finished its Finalize and batten is no longer running, or when the run is not
running. A run that is running and not paused is refused with the way forward
"lyx tender pause <slug>", then "lyx reed down" again, and its session is left
alone. The prime is no pair and is brought down as before.

When this worktree's state was recorded against a DIFFERENT session that is
still running on the hub's server — after the worktree directory was renamed,
or a .lyx directory was copied here from another worktree — down does not kill
that session (it may be a sibling worktree's live work) but reports it as
"abandonedSession", because deleting the state file removes the only record
naming it. Tear it down by hand if it is not one you need.

Example:
  lyx reed down`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			endProcesses := c.endProcesses
			if endProcesses == nil {
				endProcesses = c.endPairProcesses
			}
			ended, err := endProcesses(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			result, err := c.strandEngine().Down()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if result.EndedServer {
				// The wait asks nothing of the watchdog: Down already wrote the stop, and this only holds the verb until the daemon has left its loop.
				wait, poll, probe, sleep := c.downLockWait, c.downLockPoll, c.downLockProbe, c.downLockSleep
				if wait == 0 {
					wait = watchdogDownWait
				}
				if poll == 0 {
					poll = watchdogDownPoll
				}
				if probe == nil {
					probe = probeWatchdogLock
				}
				if sleep == nil {
					sleep = time.Sleep
				}
				waitWatchdogLockFree(c.hubPath, wait, poll, probe, sleep)
			}

			payload := map[string]any{"session": result.Session, "ended_processes": endedProcessesField(ended)}
			// Emitted only when there IS an abandoned session, so the ordinary teardown envelope is
			// unchanged and the key's mere presence is the signal.
			if result.AbandonedSession != "" {
				payload["abandonedSession"] = result.AbandonedSession
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, payload))
			return nil
		},
	}
}
