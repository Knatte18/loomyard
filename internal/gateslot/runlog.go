// runlog.go keeps one diagnostic record and log per tier run under the gate directory, and renders and finds the tier timeout and tier summary lines a site reads from a log.

package gateslot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	runLogDirName = "runs"
	runLogMaxAge  = 14 * 24 * time.Hour

	tierTimeoutPrefix = "lyx gate test: tier timeout: "
	tierTimeoutMid    = " exceeded timeout_min "
)

var tierSummaryPattern = regexp.MustCompile(`^gate test: tier(\d+) .* discovered \d+ passed \d+ failed \d+ skipped \d+ wall \S+s exit (-?\d+) log .*$`)

// RunRecord is the diagnostic record of one tier run.
// It is a domain log: no run reads it to proceed.
type RunRecord struct {
	Worktree string `json:"worktree"`
	// Slug is the task slug of the worktree, empty when it has none.
	Slug      string    `json:"slug"`
	TierList  []int     `json:"tier_list"`
	Tier      int       `json:"tier"`
	Command   string    `json:"command"`
	RunDir    string    `json:"run_dir"`
	ScopeForm string    `json:"scope_form"`
	Slots     int       `json:"slots"`
	WaitStart time.Time `json:"wait_start"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	WallSec   float64   `json:"wall_seconds"`
	ExitCode  int       `json:"exit_code"`
	TimedOut  bool      `json:"timed_out"`
	// Outcome is "passed" or "failed".
	Outcome    string `json:"outcome"`
	GuardStep  bool   `json:"guard_step"`
	Discovered int    `json:"discovered"`
	Passed     int    `json:"passed"`
	Failed     int    `json:"failed"`
	Skipped    int    `json:"skipped"`
}

// RunLogDir returns the directory under gateDir that keeps the run records and logs.
func RunLogDir(gateDir string) string {
	return filepath.Join(gateDir, runLogDirName)
}

// WriteRunRecord writes record as `<stamp>-tier<N>-<pid>.json` in dir with the run's full output beside it as the same stem with `.log`, and returns the record's path.
// Both files are written atomically, the log first, so a record always has its log.
// It first removes every `.json` and `.log` file in dir last modified more than 14 days before now; a failed removal is logged and never returned.
func WriteRunRecord(dir string, record RunRecord, output []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create gate run log directory: %w", err)
	}
	pruneRunLog(dir, now)

	stem := fmt.Sprintf("%s-tier%d-%d", now.UTC().Format("20060102T150405Z"), record.Tier, os.Getpid())
	if err := fsx.AtomicWriteBytes(filepath.Join(dir, stem+".log"), output); err != nil {
		return "", fmt.Errorf("write gate run log: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode gate run record: %w", err)
	}
	path := filepath.Join(dir, stem+".json")
	if err := fsx.AtomicWriteBytes(path, data); err != nil {
		return "", fmt.Errorf("write gate run record: %w", err)
	}
	return path, nil
}

func pruneRunLog(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Warn("list gate run log for pruning", "dir", dir, "error", err.Error())
		}
		return
	}
	cutoff := now.Add(-runLogMaxAge)
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if entry.IsDir() || (ext != ".json" && ext != ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("prune gate run log", "file", entry.Name(), "error", err.Error())
		}
	}
}

// TierTimeoutLine renders the line the gate prints to stderr and into the log when tier outlives timeout.
// It names the tier, the timeout and the way forward.
func TierTimeoutLine(tier string, timeout time.Duration) string {
	return fmt.Sprintf("%s%s%s%s; find the hang in this log; when the tier is slow but healthy, rewrite the tier whole from the prime with \"lyx config gate --set 'tiers.%s={...}'\", carrying every key of the tier and the raised timeout_min, since a dotted key under the tier writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it",
		tierTimeoutPrefix, tier, tierTimeoutMid, timeout, tier)
}

// FindTierTimeout returns the tier of the first tier timeout line anywhere in log, and false when the log holds none.
func FindTierTimeout(log string) (tier string, ok bool) {
	for _, line := range strings.Split(log, "\n") {
		rest, found := strings.CutPrefix(line, tierTimeoutPrefix)
		if !found {
			continue
		}
		if tier, _, found := strings.Cut(rest, tierTimeoutMid); found {
			return tier, true
		}
	}
	return "", false
}

// TierSummaryLine renders the one stderr line the gate prints after each tier run.
func TierSummaryLine(record RunRecord, logPath string) string {
	return fmt.Sprintf("gate test: tier%d %s discovered %d passed %d failed %d skipped %d wall %.1fs exit %d log %s",
		record.Tier, record.ScopeForm, record.Discovered, record.Passed, record.Failed, record.Skipped, record.WallSec, record.ExitCode, logPath)
}

// FailedTier returns the tier number of the last tier summary line in log whose exit code is not 0, and false when the log holds none.
func FailedTier(log string) (tier int, ok bool) {
	lines := strings.Split(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		match := tierSummaryPattern.FindStringSubmatch(strings.TrimRight(lines[i], "\r"))
		if match == nil || match[2] == "0" {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		return number, true
	}
	return 0, false
}
