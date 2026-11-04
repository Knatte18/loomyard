// hub.go fills the front task's seams over a real hub: the board, the cancel, the run-presence read, the owner read and the detached start of a land action.

package fronttask

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// recordFileName is the record file inside the front-task directory.
const recordFileName = "records.json"

// recordPathOf returns the record file of the hub at hubPath.
func recordPathOf(hubPath string) string {
	return filepath.Join(fabricengine.FrontTaskScratchDir(hubPath), recordFileName)
}

// HubDeps fills Deps over the hub of prime.
// The board writes carry no strand name, so the run-agent task guard does not judge them.
// A slug has a run when its board status is a run status, its shed status file exists or its pair worktree exists.
func HubDeps(prime *lymcwd.Location) (Deps, error) {
	board, err := boardengine.OpenHub(prime.HubPath)
	if err != nil {
		return Deps{}, err
	}
	return Deps{
		Tasks:       board.ListTasksFull,
		Create:      board.FileFrontTask,
		Release:     func(slug string) error { return board.SetStatus(slug, nil) },
		RemoveEntry: func(slug string) error { return board.RemoveTask(slug) },
		HasRun: func(slug string) (bool, error) {
			task, found, err := board.GetTask(slug)
			if err != nil {
				return false, err
			}
			if found && boardengine.IsRunStatus(task.Status) {
				return true, nil
			}
			for _, path := range []string{shedrun.StatusFile(prime, slug), fabricengine.WorktreePath(prime, slug)} {
				if _, err := os.Stat(path); err == nil {
					return true, nil
				} else if !errors.Is(err, fs.ErrNotExist) {
					return false, err
				}
			}
			return false, nil
		},
		Cancel: func(ctx context.Context, slug string) error {
			return battencli.Cancel(ctx, prime, slug, true)
		},
		RecordPath: recordPathOf(prime.HubPath),
	}, nil
}

// HubSettleDeps fills SettleDeps over the hub of prime for the worktree of beam, the prime's own when beam is empty.
// This machine owns the worktree's subject unless the board records another machine as its effective owner.
// An action starts as a detached `lym` of the running executable in the prime's anchor, and starts nothing under a test binary.
func HubSettleDeps(prime *lymcwd.Location, beam string) (SettleDeps, error) {
	board, err := boardengine.OpenHub(prime.HubPath)
	if err != nil {
		return SettleDeps{}, err
	}
	subject := beam
	if beam == "" {
		subject = boardengine.RootSubject
	}
	return SettleDeps{
		Project: beam,
		Subject: subject,
		Tasks:   board.ListTasksFull,
		OwnedHere: func() (bool, error) {
			here, err := machinename.Name()
			if err != nil {
				return false, err
			}
			owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
			if err != nil {
				return false, err
			}
			owner := owners.Prime
			if beam != "" {
				owner, err = owners.Effective(beam, func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) })
				if err != nil {
					return false, err
				}
			}
			return !boardengine.OwnedElsewhere(owner, here), nil
		},
		RecordPath: recordPathOf(prime.HubPath),
		RunAction: func(key, subject string) error {
			return startAction(prime, key, subject, startDetached)
		},
	}, nil
}

// startDetached starts command detached, reaps it in the background and returns its pid.
// It starts nothing and returns an error in a test binary, where forking the running binary would re-run the test binary.
func startDetached(command *exec.Cmd) (int, error) {
	if testing.Testing() {
		return 0, errors.New("a front task's action is not started under go test")
	}
	proc.Detach(command)
	if err := command.Start(); err != nil {
		return 0, err
	}
	proc.Reap(command)
	return command.Process.Pid, nil
}

// startAction builds the lym command of the action table's row key for subject, to run in the prime's anchor with its output appended to a file in the front-task directory, and starts it through start.
// It logs the spawn with the executable, arguments, directory, pid and output file, and logs a failed start.
func startAction(prime *lymcwd.Location, key, subject string, start func(*exec.Cmd) (int, error)) error {
	row, known := actions[key]
	if !known {
		return fmt.Errorf("the action %q is not in the action table", key)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the lym executable: %w", err)
	}
	outDir := fabricengine.FrontTaskScratchDir(prime.HubPath)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create the front-task directory: %w", err)
	}
	outPath := filepath.Join(outDir, key+".out")
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open the action output: %w", err)
	}
	defer outFile.Close()

	args := row.args(subject)
	command := exec.Command(executable, args...)
	command.Dir = prime.AnchorPath()
	command.Stdout = outFile
	command.Stderr = outFile
	pid, err := start(command)
	if err != nil {
		logger.Warn("fronttask: a land action did not start", "exe", executable, "args", args, "dir", command.Dir, "error", err)
		return err
	}
	logger.Info("fronttask: spawned a land action", "exe", executable, "args", args, "dir", command.Dir, "pid", pid, "out", outPath)
	return nil
}
