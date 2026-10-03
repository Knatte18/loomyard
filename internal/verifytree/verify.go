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

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

const (
	dirName    = "verify"
	recordName = "verified-tree.yaml"
	markerName = "running.yaml"
	logName    = "verify.log"
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
}

// Paths names the worktree Verify checks and the files it keeps in the verify directory.
type Paths struct {
	Worktree string
	Record   string
	Marker   string
	Log      string
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
	// Detail carries the cause of a shell that could not start.
	Detail string
}

// record is the verified-tree record Verify writes after a pass.
type record struct {
	Tree       string    `yaml:"tree"`
	Command    string    `yaml:"command"`
	VerifiedAt time.Time `yaml:"verified_at"`
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
	}
}

// DirtyPaths returns the paths `git status --porcelain` reports in worktree, ignored files excluded.
func DirtyPaths(worktree string) ([]string, error) {
	out, err := gitexec.Run([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("verifytree: git status in %s: %w", worktree, err)
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if _, after, ok := strings.Cut(path, " -> "); ok {
			path = after
		}
		paths = append(paths, strings.Trim(path, `"`))
	}
	return paths, nil
}

// Verify runs command in p.Worktree unless the tree is dirty or already verified.
// A dirty tree returns StatusDirty and runs nothing.
// A record naming HEAD's tree and the same command returns StatusSkipped.
// Otherwise the marker is written, the command runs with its output in p.Log, the marker is removed whatever happened, and an exit 0 returns StatusPassed.
// The pass writes the record only when HEAD still names the tree the run started on, so a commit that lands mid-run costs the next call a re-run rather than recording a tree the command did not run on.
// A non-zero exit is StatusFailed with the exit code, and a shell that could not start is StatusFailed with exit code -1 and the cause in Detail.
// A cancelled ctx is a returned error and writes no record.
func Verify(ctx context.Context, p Paths, site Site, command string) (Result, error) {
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
	if rec, ok := readRecord(p.Record); ok && rec.Tree == tree && rec.Command == command {
		return Result{Status: StatusSkipped, Tree: tree}, nil
	}

	if err := os.MkdirAll(filepath.Dir(p.Marker), 0o755); err != nil {
		return Result{}, fmt.Errorf("verifytree: create verify directory: %w", err)
	}
	if err := writeMarker(p.Marker, Marker{Site: site.Label, Attempt: site.Attempt, Started: time.Now(), PID: os.Getpid()}); err != nil {
		return Result{}, err
	}
	defer os.Remove(p.Marker)

	logFile, err := os.Create(p.Log)
	if err != nil {
		return Result{}, fmt.Errorf("verifytree: create verify log %s: %w", p.Log, err)
	}
	defer logFile.Close()

	code, runErr := verifyrun.Run(ctx, command, p.Worktree, logFile)
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, fmt.Errorf("verifytree: verify cancelled: %w", ctxErr)
		}
		return Result{Status: StatusFailed, ExitCode: -1, Tree: tree, Detail: runErr.Error()}, nil
	}
	if code != 0 {
		return Result{Status: StatusFailed, ExitCode: code, Tree: tree}, nil
	}

	// A commit that landed mid-run means the command read a tree that was not the recorded one, so the pass is not recorded.
	after, err := headTree(p.Worktree)
	if err != nil {
		return Result{}, err
	}
	if after != tree {
		return Result{Status: StatusPassed, Tree: tree}, nil
	}

	if err := writeRecord(p.Record, record{Tree: tree, Command: command, VerifiedAt: time.Now()}); err != nil {
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

// readRecord reads the record at path; an absent, unreadable or malformed record reads as none, which only costs a re-run.
func readRecord(path string) (record, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}, false
	}
	var rec record
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return record{}, false
	}
	return rec, true
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
