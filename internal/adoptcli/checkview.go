// checkview.go renders the adopt check's record as the section `lym beam status` shows: its state as shown, the tier in flight, the tiers done and the paths of its log and output.

package adoptcli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// checkEndedWithoutVerdict is the state shown for a running record whose process is gone.
const checkEndedWithoutVerdict = "ended without a verdict"

// CheckView is the adopt check as `lym beam status` shows it.
type CheckView struct {
	// State is running, green, failed or ended without a verdict.
	State string `json:"state"`
	// Running is the tier in flight, nil between tiers and for a check that ended.
	Running *int `json:"running,omitempty"`
	// Tiers holds the tiers done, with their outcomes.
	Tiers      []CheckTier `json:"tiers,omitempty"`
	EndedAt    time.Time   `json:"ended_at,omitzero"`
	FailedTier *int        `json:"failed_tier,omitempty"`
	// SuiteLog and CheckOut are the paths of the gate's log and of the check process's stdout and stderr.
	SuiteLog string `json:"suite_log"`
	CheckOut string `json:"check_out"`
	// Lines is the rendered section.
	Lines []string `json:"lines"`
}

// ReadCheckView reads the adopt check of the hub at hubPath and returns its view, or nil when the hub holds no record.
// An absent record creates no directory or lock file.
// A running record whose process is gone, or whose pid a later boot reused, shows as ended without a verdict.
func ReadCheckView(hubPath string) (*CheckView, error) {
	dir := fabricengine.AdoptScratchDir(hubPath)
	record, found, err := ReadCheck(filepath.Join(dir, checkRecordName))
	if err != nil || !found {
		return nil, err
	}
	alive := record.State != checkRunning || checkAlive(record)
	view := checkView(record, alive, dir)
	return &view, nil
}

// checkView builds the view of record, whose adopt scratch directory is dir.
// alive says whether the process that wrote a running record is still running; it decides nothing for a record that ended.
func checkView(record CheckRecord, alive bool, dir string) CheckView {
	view := CheckView{
		State: record.State, Running: record.Running, Tiers: record.Tiers, EndedAt: record.EndedAt, FailedTier: record.FailedTier,
		SuiteLog: filepath.Join(dir, suiteLogName), CheckOut: filepath.Join(dir, checkOutName),
	}
	done := make([]string, len(record.Tiers))
	for i, tier := range record.Tiers {
		done[i] = fmt.Sprintf("tier%d %s", tier.Tier, tier.Outcome)
	}
	doneText := "no tier done"
	if len(done) > 0 {
		doneText = "done: " + strings.Join(done, ", ")
	}
	ended := record.EndedAt.Format(time.RFC3339)

	switch {
	case record.State == checkRunning && !alive:
		view.State = checkEndedWithoutVerdict
		view.Running = nil
		view.Lines = []string{
			fmt.Sprintf("adopt check: %s; %s", checkEndedWithoutVerdict, doneText),
			fmt.Sprintf("  see %s and %s, or rerun lym gate test --gate %s in the prime, which writes no record", view.SuiteLog, view.CheckOut, checkGateKey),
			"  the record clears at the next lym adopt with an owner write, which starts a fresh check",
		}
	case record.State == checkRunning:
		inFlight := "between tiers"
		if record.Running != nil {
			inFlight = "tier" + strconv.Itoa(*record.Running) + " running"
		}
		view.Lines = []string{fmt.Sprintf("adopt check: running, %s; %s; log %s", inFlight, doneText, view.SuiteLog)}
	case record.State == checkFailed:
		failure := record.Error
		if record.FailedTier != nil {
			failure = "tier" + strconv.Itoa(*record.FailedTier) + " failed"
		}
		view.Lines = []string{fmt.Sprintf("adopt check: failed at %s, %s; %s; log %s", ended, failure, doneText, view.SuiteLog)}
	default:
		view.Lines = []string{fmt.Sprintf("adopt check: %s at %s; %s; log %s", record.State, ended, doneText, view.SuiteLog)}
	}
	return view
}
