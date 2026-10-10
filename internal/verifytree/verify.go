// verify.go declares Verify, the one plan-verify function, and the record and paths it works over.

package verifytree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// Timeout is the production bound on one verify command.
const Timeout = 60 * time.Minute

const (
	dirName    = "verify"
	recordName = "verified-tree.yaml"
	markerName = "running.yaml"
	logName    = "verify.log"

	publishFailureName    = "publish-failure.yaml"
	publishFailureLogName = "publish-failure.log"
)

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
	// Label is one of `webster gate`, `Webster-Burler gate`, `Publish`, `Finalize`, `webster verify`.
	Label string
	// Attempt is 0 where no attempt counter applies.
	Attempt int
	// BaseCommand names the command whose record entry survives the pruning a pass performs; empty names none.
	BaseCommand string
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

// Paths names the worktree Verify checks and the files it keeps in the verify directory.
type Paths struct {
	Worktree string
	Record   string
	Marker   string
	Log      string
	// PublishFailure is the Publish failure record, and PublishFailureLog the copy of the log it names.
	PublishFailure    string
	PublishFailureLog string
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
	// TimedOut is set when the verify command outlived the timeout and was killed;
	// the status is then StatusFailed with ExitCode -1.
	TimedOut bool
	// Detail carries the cause of a shell that could not start, or names the timeout of a timed-out run.
	Detail string
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
		Worktree: worktree,
		Record:   filepath.Join(dir, recordName),
		Marker:   filepath.Join(dir, markerName),
		Log:      filepath.Join(dir, logName),

		PublishFailure:    filepath.Join(dir, publishFailureName),
		PublishFailureLog: filepath.Join(dir, publishFailureLogName),
	}
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
// Otherwise the marker is written, the command runs with its output in p.Log, the marker is removed whatever happened, and an exit 0 returns StatusPassed.
// A non-nil slots pool gates the run: the marker is first written in the waiting state, a slot is acquired under ctx, and the marker is rewritten as running with a fresh start time before the timeout begins to count.
// The command then runs with the lease's environment and the slot is released whatever the outcome.
// A nil slots runs unslotted with the parent's environment.
// The pass writes the record only when HEAD still names the tree and commit the run started on, so a commit that lands mid-run costs the next call a re-run rather than recording a tree the command did not run on.
// The write replaces the entry of command and drops every other command's entry naming a different tree, except site.BaseCommand's.
// A non-zero exit is StatusFailed with the exit code, and a shell that could not start is StatusFailed with exit code -1 and the cause in Detail.
// A command still running after timeout is killed and returns StatusFailed with exit code -1, TimedOut set and the timeout in Detail;
// no record is written.
// A cancelled ctx is a returned error and writes no record.
func Verify(ctx context.Context, p Paths, site Site, command string, timeout time.Duration, slots *gateslot.Pool) (Result, error) {
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

	if err := os.MkdirAll(filepath.Dir(p.Marker), 0o755); err != nil {
		return Result{}, fmt.Errorf("verifytree: create verify directory: %w", err)
	}
	marker := Marker{Site: site.Label, Attempt: site.Attempt, Command: command, Started: time.Now(), PID: os.Getpid(), State: MarkerStateRunning}
	if slots != nil {
		marker.State = MarkerStateWaiting
		marker.WaitStarted = marker.Started
	}
	if err := writeMarker(p.Marker, marker); err != nil {
		return Result{}, err
	}
	defer os.Remove(p.Marker)

	// A verify command never inherits the prebuilt-lyx variable: its own go test builds lyx once per test binary.
	env := gateslot.StripPrebuilt(os.Environ())
	if slots != nil {
		lease, err := slots.Acquire(ctx, gateslot.Holder{Worktree: p.Worktree, Site: site.Label})
		if err != nil {
			return Result{}, fmt.Errorf("verifytree: wait for gate slot: %w", err)
		}
		defer func() {
			if err := lease.Release(); err != nil {
				logger.Warn("verifytree: release gate slot", "worktree", p.Worktree, "cause", err)
			}
		}()
		env = lease.Env(env)
		marker.State = MarkerStateRunning
		marker.Started = time.Now()
		if err := writeMarker(p.Marker, marker); err != nil {
			return Result{}, err
		}
	}

	logFile, err := os.Create(p.Log)
	if err != nil {
		return Result{}, fmt.Errorf("verifytree: create verify log %s: %w", p.Log, err)
	}
	defer logFile.Close()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	code, runErr := verifyrun.Run(runCtx, command, p.Worktree, env, logFile)
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, fmt.Errorf("verifytree: verify cancelled: %w", ctxErr)
		}
		if runCtx.Err() != nil {
			detail := fmt.Sprintf("the verify command did not finish within %s and was killed", timeout)
			return Result{Status: StatusFailed, ExitCode: -1, Tree: tree, TimedOut: true, Detail: detail}, nil
		}
		return Result{Status: StatusFailed, ExitCode: -1, Tree: tree, Detail: runErr.Error()}, nil
	}
	if code != 0 {
		return Result{Status: StatusFailed, ExitCode: code, Tree: tree}, nil
	}

	// A commit that landed mid-run means the command read a tree that was not the recorded one, so the pass is not recorded.
	afterTree, err := headTree(p.Worktree)
	if err != nil {
		return Result{}, err
	}
	afterCommit, err := headCommit(p.Worktree)
	if err != nil {
		return Result{}, err
	}
	if afterTree != tree || afterCommit != commit {
		return Result{Status: StatusPassed, Tree: tree}, nil
	}

	next := readRecord(p.Record).withPass(command, site.BaseCommand, Pass{Tree: tree, Commit: commit, VerifiedAt: time.Now()})
	if err := writeRecord(p.Record, next); err != nil {
		return Result{}, err
	}
	return Result{Status: StatusPassed, Tree: tree}, nil
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
