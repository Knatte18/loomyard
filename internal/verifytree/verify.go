// verify.go declares Verify, the one plan-verify function, and the record and paths it works over.

package verifytree

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

const (
	dirName    = "verify"
	recordName = "verified-tree.yaml"
	markerName = "running.yaml"

	landingFailureName = "landing-failure.yaml"

	logPrefix = "verify-"
	logSuffix = ".log"
)

// verifyLogKeep is how many of the newest verify logs a run's pruning keeps, beside the one the landing failure record names.
const verifyLogKeep = 20

// Status is the outcome of one Verify call.
type Status string

const (
	// StatusPassed means the command ran and exited 0.
	// The record now names the tree the run started on, unless HEAD moved during the run.
	StatusPassed Status = "passed"
	// StatusSkipped means the record already named HEAD's tree and the same command, so nothing ran.
	StatusSkipped Status = "skipped"
	// StatusFailed means the command exited non-zero or its shell could not start.
	StatusFailed Status = "failed"
	// StatusDirty means the worktree had uncommitted changes, so nothing ran.
	StatusDirty Status = "dirty"
)

// Site names the call site and its attempt for the running marker.
type Site struct {
	// Label is one of `webster gate`, `Webster-Burler-Fix gate`, `Publish`, `Finalize`, `webster verify`.
	Label string
	// Attempt is 0 where no attempt counter applies.
	Attempt int
	// BaseCommand names the command whose record entry survives the pruning a pass performs; empty names none.
	BaseCommand string
	// Hold is the site's pause hold; nil runs Verify with no hold.
	Hold *PauseHold
}

// Pass is one recorded pass of a command.
type Pass struct {
	// Tree is the tree SHA the command passed on.
	Tree string
	// Commit is the commit SHA of HEAD at that pass.
	Commit string
	// VerifiedAt is the time of the pass.
	VerifiedAt time.Time
}

// Paths names the worktree Verify checks, the verify directory and the fixed files it keeps there.
type Paths struct {
	Worktree string
	// Dir is the verify directory, which also holds one `verify-<n>.log` per run.
	Dir    string
	Record string
	Marker string
	// LandingFailure is the landing failure record.
	LandingFailure string
}

// Result is the outcome of one Verify call.
type Result struct {
	Status Status
	// Dirty lists the dirty paths of a StatusDirty result.
	Dirty []string
	// ExitCode is the command's exit code, -1 when its shell could not start.
	ExitCode int
	// Tree is HEAD's tree SHA, empty for a dirty result.
	Tree string
	// TimedOut is set when the failed command's log holds a gate tier timeout line;
	// the status is then StatusFailed with the command's own exit code.
	TimedOut bool
	// Detail carries the cause of a shell that could not start, or names the tier and timeout of a timed-out run.
	Detail string
	// Log is this run's own verify log, empty for a dirty or skipped result.
	Log string
}

// record is the verified-tree record Verify writes after a pass: one entry per command.
type record struct {
	Entries []entry `yaml:"entries"`
}

// entry is the pass of one command.
type entry struct {
	Command    string    `yaml:"command"`
	Tree       string    `yaml:"tree"`
	Commit     string    `yaml:"commit"`
	VerifiedAt time.Time `yaml:"verified_at"`
}

// withPass returns the record after command passed on pass: its own entry is replaced, and every entry of another command naming a different tree is dropped, except the entry of baseCommand.
func (r record) withPass(command, baseCommand string, pass Pass) record {
	next := record{Entries: []entry{{Command: command, Tree: pass.Tree, Commit: pass.Commit, VerifiedAt: pass.VerifiedAt}}}
	for _, e := range r.Entries {
		if e.Command == command {
			continue
		}
		if e.Tree == pass.Tree || e.Command == baseCommand {
			next.Entries = append(next.Entries, e)
		}
	}
	return next
}

// find returns the entry of command.
func (r record) find(command string) (entry, bool) {
	for _, e := range r.Entries {
		if e.Command == command {
			return e, true
		}
	}
	return entry{}, false
}

// LatestPass returns the recorded pass of command, false when the record holds none.
// A record in an older format, or a malformed one, reads as none.
func LatestPass(p Paths, command string) (Pass, bool) {
	e, ok := readRecord(p.Record).find(command)
	if !ok {
		return Pass{}, false
	}
	return Pass{Tree: e.Tree, Commit: e.Commit, VerifiedAt: e.VerifiedAt}, true
}

// Dir returns the verify directory under anchorRoot.
// Every teller calls it rather than joining its own, so the Lyxdirs Single-Declarer Invariant holds.
func Dir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lyxdirs.DotLyxDirName, dirName)
}

// NewPaths names the verify files inside dir for worktree.
func NewPaths(worktree, dir string) Paths {
	return Paths{
		Worktree:       worktree,
		Dir:            dir,
		Record:         filepath.Join(dir, recordName),
		Marker:         filepath.Join(dir, markerName),
		LandingFailure: filepath.Join(dir, landingFailureName),
	}
}

// IsLogPath reports whether path names a `verify-<n>.log` directly inside p.Dir.
func IsLogPath(p Paths, path string) bool {
	clean := filepath.Clean(path)
	if filepath.Dir(clean) != filepath.Clean(p.Dir) {
		return false
	}
	_, ok := logNumber(filepath.Base(clean))
	return ok
}

// logNumber returns n for a file named `verify-<n>.log` with n a positive integer in canonical decimal form.
func logNumber(name string) (int, bool) {
	digits, ok := strings.CutPrefix(name, logPrefix)
	if !ok {
		return 0, false
	}
	digits, ok = strings.CutSuffix(digits, logSuffix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || strconv.Itoa(n) != digits {
		return 0, false
	}
	return n, true
}

// numberedLog is one verify log in the verify directory and its sequence number.
type numberedLog struct {
	path   string
	number int
}

// listLogs returns the verify logs in dir, newest first.
func listLogs(dir string) ([]numberedLog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("verifytree: list verify directory %s: %w", dir, err)
	}
	var logs []numberedLog
	for _, e := range entries {
		if n, ok := logNumber(e.Name()); ok && e.Type().IsRegular() {
			logs = append(logs, numberedLog{path: filepath.Join(dir, e.Name()), number: n})
		}
	}
	slices.SortFunc(logs, func(a, b numberedLog) int { return cmp.Compare(b.number, a.number) })
	return logs, nil
}

// nextLogPath returns the path of the next run's log in dir: `verify-<n>.log`, n one above the highest existing number.
func nextLogPath(dir string) (string, error) {
	logs, err := listLogs(dir)
	if err != nil {
		return "", err
	}
	next := 1
	if len(logs) > 0 {
		next = logs[0].number + 1
	}
	return filepath.Join(dir, logPrefix+strconv.Itoa(next)+logSuffix), nil
}

// pruneLogs removes every verify log in p.Dir beyond the newest verifyLogKeep, except the one the landing failure record names.
func pruneLogs(p Paths) error {
	logs, err := listLogs(p.Dir)
	if err != nil {
		return err
	}
	if len(logs) <= verifyLogKeep {
		return nil
	}
	failure, _, err := ReadLandingFailure(p)
	if err != nil {
		return err
	}
	for _, log := range logs[verifyLogKeep:] {
		if failure.LogPath != "" && filepath.Clean(failure.LogPath) == log.path {
			continue
		}
		if err := os.Remove(log.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("verifytree: remove verify log %s: %w", log.path, err)
		}
	}
	return nil
}

// DirtyPaths returns the paths `git status --porcelain -z` reports in worktree, ignored files excluded.
func DirtyPaths(worktree string) ([]string, error) {
	out, err := gitexec.Run([]string{"status", "--porcelain", "-z"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("verifytree: git status in %s: %w", worktree, err)
	}
	return parsePorcelainZ(out), nil
}

// parsePorcelainZ returns the paths of `git status --porcelain -z` output.
// The -z form neither quotes nor escapes a path, so each comes back verbatim.
// A rename or copy entry names its destination and is followed by a record holding its source, which is skipped.
func parsePorcelainZ(out string) []string {
	var paths []string
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 4 {
			continue
		}
		paths = append(paths, rec[3:])
		if strings.ContainsAny(rec[:2], "RC") {
			i++
		}
	}
	return paths
}

// Verify runs command in p.Worktree unless the tree is dirty or already verified.
// A dirty tree returns StatusDirty and runs nothing.
// A record entry of the same command naming HEAD's tree returns StatusSkipped.
// Otherwise the marker is written, the command runs with its output in a fresh `verify-<n>.log` in p.Dir that the result names, the marker is removed whatever happened, and an exit 0 returns StatusPassed.
// Once the log is written, whatever the exit, every log beyond the newest verifyLogKeep is pruned except the one the landing failure record names;
// a prune failure is logged and leaves the result unchanged.
// The command is one `lyx gate test --wait` call, or such calls joined by `&&`, which take their own slots and bound themselves, so Verify takes no slot and applies no timeout.
// The pass writes the record only when HEAD still names the tree and commit the run started on, so a commit that lands mid-run costs the next call a re-run rather than recording a tree the command did not run on.
// The write replaces the entry of command and drops every other command's entry naming a different tree, except site.BaseCommand's.
// A non-zero exit is StatusFailed with the exit code, and a shell that could not start is StatusFailed with exit code -1 and the cause in Detail.
// A failed command whose log holds a tier timeout line is also marked TimedOut, with the tier and its timeout in Detail and the command's own exit code.
// A cancelled ctx is a returned error and writes no record.
// With a site.Hold, a covering pause holds Verify before its command and stops a running command at the pause's grace end, then Verify starts over from the dirty check after the resume.
// A held Verify removes its marker on every return, its dirty and skipped returns included.
func Verify(ctx context.Context, p Paths, site Site, command string) (Result, error) {
	if site.Hold != nil {
		if err := os.MkdirAll(filepath.Dir(p.Marker), 0o755); err != nil {
			return Result{}, fmt.Errorf("verifytree: create verify directory: %w", err)
		}
		defer os.Remove(p.Marker)
	}
	for {
		dirty, err := DirtyPaths(p.Worktree)
		if err != nil {
			return Result{}, err
		}
		if len(dirty) > 0 {
			return Result{Status: StatusDirty, Dirty: dirty}, nil
		}

		tree, err := headTree(p.Worktree)
		if err != nil {
			return Result{}, err
		}
		commit, err := headCommit(p.Worktree)
		if err != nil {
			return Result{}, err
		}
		if e, ok := readRecord(p.Record).find(command); ok && e.Tree == tree {
			return Result{Status: StatusSkipped, Tree: tree}, nil
		}

		if site.Hold != nil {
			held, err := waitOutPause(ctx, p, site, command)
			if err != nil {
				return Result{}, err
			}
			if held {
				continue
			}
		}

		result, stopped, err := runAndRecord(ctx, p, site, command, tree, commit)
		if !stopped {
			return result, err
		}
		logger.Info("verifytree: pause stopped the verify command at its grace end; it reruns after the resume", "command", command, "log", result.Log)
		if _, err := waitOutPause(ctx, p, site, command); err != nil {
			return Result{}, err
		}
	}
}

// waitOutPause writes a paused marker and waits until no pause covers the site's scope, reporting whether it had to wait.
// A marker is written only when the wait starts, so an uncovered scope costs nothing.
func waitOutPause(ctx context.Context, p Paths, site Site, command string) (bool, error) {
	held := false
	var markerErr error
	err := site.Hold.Wait(ctx, func() {
		held = true
		now := time.Now()
		marker := Marker{Site: site.Label, Attempt: site.Attempt, Command: command, Started: now, PID: os.Getpid(), State: MarkerStatePaused, WaitStarted: now}
		markerErr = writeMarker(p.Marker, marker)
	})
	if err != nil {
		return false, fmt.Errorf("verifytree: verify cancelled: %w", err)
	}
	return held, markerErr
}

// runAndRecord runs command on the tree and commit the caller read, with the running marker written for the run and removed after it, and records a pass.
// stopped is true when the site's hold stopped the command past a pause's grace end while ctx itself had not ended; the result then names only the stopped run's log.
func runAndRecord(ctx context.Context, p Paths, site Site, command, tree, commit string) (result Result, stopped bool, err error) {
	if err := os.MkdirAll(filepath.Dir(p.Marker), 0o755); err != nil {
		return Result{}, false, fmt.Errorf("verifytree: create verify directory: %w", err)
	}
	marker := Marker{Site: site.Label, Attempt: site.Attempt, Command: command, Started: time.Now(), PID: os.Getpid(), State: MarkerStateRunning}
	if err := writeMarker(p.Marker, marker); err != nil {
		return Result{}, false, err
	}
	defer os.Remove(p.Marker)

	// The verify command runs without the strand name, so a test driving `lyx fabric` never meets a role guard that reads it.
	// It never inherits the prebuilt-lyx variable either: its own go test builds lyx once per test binary.
	env := slices.DeleteFunc(gateslot.StripPrebuilt(os.Environ()), func(entry string) bool { return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") })

	logPath, err := nextLogPath(p.Dir)
	if err != nil {
		return Result{}, false, err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return Result{}, false, fmt.Errorf("verifytree: create verify log %s: %w", logPath, err)
	}
	defer func() {
		logFile.Close()
		if err := pruneLogs(p); err != nil {
			logger.Warn("verifytree: prune verify logs", "dir", p.Dir, "cause", err)
		}
	}()

	code, pauseStopped, runErr := runUnderHold(ctx, site.Hold, command, p.Worktree, env, logFile)
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, false, fmt.Errorf("verifytree: verify cancelled: %w", ctxErr)
		}
		if pauseStopped {
			return Result{Log: logPath}, true, nil
		}
		return Result{Status: StatusFailed, ExitCode: -1, Tree: tree, Detail: runErr.Error(), Log: logPath}, false, nil
	}
	if code != 0 {
		failed := Result{Status: StatusFailed, ExitCode: code, Tree: tree, Log: logPath}
		failed.TimedOut, failed.Detail = tierTimeoutDetail(logPath)
		return failed, false, nil
	}

	// A commit that landed mid-run means the command read a tree that was not the recorded one, so the pass is not recorded.
	afterTree, err := headTree(p.Worktree)
	if err != nil {
		return Result{}, false, err
	}
	afterCommit, err := headCommit(p.Worktree)
	if err != nil {
		return Result{}, false, err
	}
	if afterTree != tree || afterCommit != commit {
		return Result{Status: StatusPassed, Tree: tree, Log: logPath}, false, nil
	}

	next := readRecord(p.Record).withPass(command, site.BaseCommand, Pass{Tree: tree, Commit: commit, VerifiedAt: time.Now()})
	if err := writeRecord(p.Record, next); err != nil {
		return Result{}, false, err
	}
	return Result{Status: StatusPassed, Tree: tree, Log: logPath}, false, nil
}

// runUnderHold runs command through verifyrun.Run, and with a non-nil hold cancels it once hold.PastGrace reports true, polled every hold.Poll only while the command runs.
// pauseStopped reports that the hold cancelled the run.
func runUnderHold(ctx context.Context, hold *PauseHold, command, dir string, env []string, out io.Writer) (code int, pauseStopped bool, runErr error) {
	if hold == nil {
		code, runErr = verifyrun.Run(ctx, command, dir, env, out)
		return code, false, runErr
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stopped atomic.Bool
	finished := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		ticker := time.NewTicker(hold.Poll)
		defer ticker.Stop()
		for {
			select {
			case <-finished:
				return
			case <-ticker.C:
				if hold.PastGrace() {
					stopped.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	code, runErr = verifyrun.Run(runCtx, command, dir, env, out)
	close(finished)
	watcher.Wait()
	return code, stopped.Load(), runErr
}

// tierTimeoutDetail reports whether the log at logPath holds a gate tier timeout line, and names the tier and its timeout when it does.
func tierTimeoutDetail(logPath string) (bool, string) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return false, ""
	}
	tier, ok := gateslot.FindTierTimeout(string(data))
	if !ok {
		return false, ""
	}
	detail := fmt.Sprintf("gate %s outlived its timeout_min and was killed", tier)
	for _, line := range strings.Split(string(data), "\n") {
		_, after, found := strings.Cut(line, tier+" exceeded timeout_min ")
		if !found {
			continue
		}
		timeout, _, _ := strings.Cut(after, ";")
		detail = fmt.Sprintf("gate %s exceeded timeout_min %s and was killed", tier, strings.TrimSpace(timeout))
		break
	}
	return true, detail
}

// headTree returns HEAD's tree SHA in worktree.
func headTree(worktree string) (string, error) {
	out, err := gitexec.Run([]string{"rev-parse", "HEAD^{tree}"}, worktree)
	if err != nil {
		return "", fmt.Errorf("verifytree: resolve HEAD tree in %s: %w", worktree, err)
	}
	return strings.TrimSpace(out), nil
}

// readRecord reads the record at path.
// An absent, unreadable, malformed or older-format record reads as an empty one, which only costs a re-run.
func readRecord(path string) record {
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}
	}
	var rec record
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return record{}
	}
	return rec
}

// headCommit returns HEAD's commit SHA in worktree.
func headCommit(worktree string) (string, error) {
	out, err := gitexec.Run([]string{"rev-parse", "HEAD"}, worktree)
	if err != nil {
		return "", fmt.Errorf("verifytree: resolve HEAD commit in %s: %w", worktree, err)
	}
	return strings.TrimSpace(out), nil
}

// writeRecord writes rec to path.
func writeRecord(path string, rec record) error {
	data, err := yaml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("verifytree: encode record: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("verifytree: write record %s: %w", path, err)
	}
	return nil
}
