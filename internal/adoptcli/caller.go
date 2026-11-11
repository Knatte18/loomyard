// caller.go is the `lym adopt` call itself.
// It validates the request and starts the detached worker that runs the steps.
// It then follows the adopt record to the end, or discards a record that no worker will finish.

package adoptcli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// workerOutName is the file beside the adopt record that receives the worker's stdout and stderr.
const workerOutName = "adopt.out"

// callerDeps holds the seams of a `lym adopt` call.
// Strand is the caller's full agent name, empty for a plain shell.
// Steps is what validateRequest reads.
// KeepWarm answers the longest follow an agent session of the strand may block on.
// Discard removes an unfinished record, reporting whether it removed one.
// Spawn starts a detached worker running args and returns its pid and a channel that closes when it ends.
// Stderr receives the line of each step that ends.
type callerDeps struct {
	HubPath    string
	Strand     string
	Steps      adoptDeps
	ReadRecord func() (adoptrecord.Record, bool, error)
	Alive      func(pid int, startTicks string) bool
	Now        func() time.Time
	Sleep      func(time.Duration)
	KeepWarm   func(strand string) time.Duration
	Discard    func() (removed bool, err error)
	Spawn      func(args []string) (pid int, exited <-chan struct{}, err error)
	Stderr     io.Writer
}

// hubCallerDeps fills callerDeps over the hub the prime belongs to.
func hubCallerDeps(prime *lymcwd.Location) (callerDeps, error) {
	steps, err := hubDepsAs(prime, machinename.Name)
	if err != nil {
		return callerDeps{}, err
	}
	hubPath := prime.HubPath
	dirs := fabricengine.ConfigDirs(prime)
	return callerDeps{
		HubPath:    hubPath,
		Strand:     os.Getenv(agentname.StrandNameEnv),
		Steps:      steps,
		ReadRecord: func() (adoptrecord.Record, bool, error) { return adoptrecord.Read(hubPath) },
		Alive:      adoptrecord.WorkerAlive,
		Now:        time.Now,
		Sleep:      time.Sleep,
		KeepWarm:   func(strand string) time.Duration { return shuttleengines.KeepWarm(dirs, strand).Cap },
		Discard:    func() (bool, error) { return discard(hubPath) },
		Spawn:      func(args []string) (int, <-chan struct{}, error) { return spawnWorker(prime, args) },
		Stderr:     os.Stderr,
	}, nil
}

// runsInOrch reports whether strand, a full agent name, is an orch session, a numbered orch included.
// An empty or unparsable name is not.
func runsInOrch(strand string) bool {
	name, err := agentname.Parse(strand)
	return err == nil && agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// runAdopt runs a `lym adopt` call.
// It validates req and starts a worker unless one already runs the same adopt.
// It then follows the record to its end and prints the end envelope on out.
// With discardFlag it removes an unfinished record instead.
// It returns the exit code.
func runAdopt(deps callerDeps, req request, discardFlag bool, out io.Writer) int {
	if discardFlag {
		if len(req.Slugs) > 0 || req.Root || req.All || req.KeepLocal || req.DropLocal || req.NoOrch {
			return output.Err(out, "lym adopt: --discard takes no subject and no other flag: run `lym adopt --discard` alone")
		}
		removed, err := deps.Discard()
		if err != nil {
			return output.Err(out, err.Error())
		}
		return output.Ok(out, map[string]any{"removed": removed})
	}

	req = normalizedRequest(req)
	req.NoOrch = req.NoOrch || runsInOrch(deps.Strand)
	if _, _, err := validateRequest(deps.Steps, req); err != nil {
		return output.Err(out, err.Error())
	}
	call := req.recorded()
	record, found, err := deps.ReadRecord()
	if err != nil {
		return output.Err(out, err.Error())
	}
	verdict, err := judgeStanding(record, found, record.ReadState(deps.Alive), call)
	if err != nil {
		return output.Err(out, err.Error())
	}

	since := deps.Now()
	pid := record.PID
	var exited <-chan struct{}
	if verdict != verdictFollow {
		if pid, exited, err = deps.Spawn(workerArgs(call)); err != nil {
			return output.Err(out, fmt.Sprintf("lym adopt: start the worker: %v; way forward: fix the cause it names, then re-run `%s`", err, call.Command()))
		}
	}

	final, err := follow(followDeps{
		ReadRecord: deps.ReadRecord,
		Alive:      deps.Alive,
		Exited:     exited,
		Now:        deps.Now,
		Sleep:      deps.Sleep,
		Cap:        followCap(deps.KeepWarm, deps.Strand),
		Steps:      deps.Stderr,
	}, call, pid, since)
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := maps.Clone(final.Envelope)
	if fields == nil {
		fields = map[string]any{}
	}
	if final.State == adoptrecord.StateStopped {
		message := final.Error
		if final.WayForward != "" {
			message += "; " + final.WayForward
		}
		return output.ErrFields(out, message, fields)
	}
	return output.Ok(out, fields)
}

// workerArgs returns the lym arguments that run the worker for req.
func workerArgs(req adoptrecord.Request) []string {
	args := []string{"adopt", "worker"}
	args = append(args, req.Slugs...)
	if req.Root {
		args = append(args, "root")
	}
	for _, flag := range []struct {
		set  bool
		text string
	}{
		{req.All, "--all"},
		{req.KeepLocal, "--keep-local"},
		{req.DropLocal, "--drop-local"},
		{req.NoOrch, "--no-orch"},
	} {
		if flag.set {
			args = append(args, flag.text)
		}
	}
	return args
}

// workerCommand returns the command that runs executable with args in dir.
func workerCommand(executable, dir string, args []string) *exec.Cmd {
	command := exec.Command(executable, args...)
	command.Dir = dir
	return command
}

// spawnWorker starts the worker for args as a detached process in the prime, in a session of its own.
// Its stdout and stderr are appended to the worker output file beside the adopt record.
// It returns the worker's pid and a channel that closes once the worker has ended and been waited for.
// The worker is registered under the hub owner, so the run's process teardown reaches it.
// In a test binary it starts nothing and returns an error:
// forking the running binary there would re-run the test binary.
func spawnWorker(prime *lymcwd.Location, args []string) (pid int, exited <-chan struct{}, err error) {
	if testing.Testing() {
		return 0, nil, errors.New("the adopt worker is not started under go test")
	}
	executable, err := selfexec.LymBinary()
	if err != nil {
		return 0, nil, fmt.Errorf("locate the lym executable: %w", err)
	}
	outPath := filepath.Join(fabricengine.AdoptScratchDir(prime.HubPath), workerOutName)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, nil, fmt.Errorf("create the adopt scratch directory: %w", err)
	}
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, nil, fmt.Errorf("open the adopt worker output: %w", err)
	}
	defer outFile.Close()

	dir := prime.AnchorPath()
	command := workerCommand(executable, dir, args)
	command.Stdout = outFile
	command.Stderr = outFile
	proc.Detach(command)
	if err := command.Start(); err != nil {
		logger.Warn("adoptcli: the adopt worker did not start", "exe", executable, "args", args, "dir", dir, "error", err)
		return 0, nil, err
	}
	pid = command.Process.Pid
	logger.Info("adoptcli: spawned the adopt worker", "exe", executable, "args", args, "dir", dir, "pid", pid, "out", outPath)

	ended := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(ended)
	}()
	record := runproc.ChildRecord(pid, runproc.RoleAdopt, args)
	if err := runproc.Register(fabricengine.RunProcDir(prime.HubPath), runproc.HubOwner, record); err != nil {
		logger.Warn("adoptcli: the adopt worker was not registered with the run process owner", "pid", pid, "error", err)
	}
	return pid, ended, nil
}

// discard removes the hub's adopt record when it is unfinished or does not decode.
// It takes adopt.lock without blocking, so a record a running worker holds stays.
// A done record, or none, stays and reports nothing removed.
// It lifts no pause and undoes no owner write.
func discard(hubPath string) (removed bool, err error) {
	release, acquired, err := tryAdoptLock(hubPath)
	if err != nil {
		return false, fmt.Errorf("lym adopt: take the adopt lock: %w", err)
	}
	if !acquired {
		return false, errors.New("lym adopt: an adopt holds the adopt lock, so its record stays: `lym beam status` shows it")
	}
	defer func() { _ = release() }()

	record, found, err := adoptrecord.Read(hubPath)
	switch {
	case errors.Is(err, adoptrecord.ErrDecode):
	case err != nil:
		return false, err
	case !found || !record.Unfinished():
		return false, nil
	}
	if err := adoptrecord.Remove(hubPath); err != nil {
		return false, err
	}
	return true, nil
}
