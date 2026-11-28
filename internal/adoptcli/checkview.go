// checkview.go renders each subject's adopt check record as the section `lym beam status` shows.

package adoptcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// checkEndedWithoutVerdict is the state shown for a running record whose process is gone.
const checkEndedWithoutVerdict = "ended without a verdict"

// CheckView is one subject's adopt check as `lym beam status` shows it.
type CheckView struct {
	// Subject is the beam slug or `root` the check ran for.
	Subject string `json:"subject"`
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

// ReadCheckViews returns the adopt check of every subject of the hub at hubPath, sorted by subject.
// It returns none when no subject directory holds a record.
// An absent scratch directory or record creates no directory or lock file.
// A running record whose process is gone, or whose pid a later boot reused, shows as ended without a verdict.
func ReadCheckViews(hubPath string) ([]CheckView, error) {
	adoptDir := fabricengine.AdoptScratchDir(hubPath)
	entries, err := os.ReadDir(adoptDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var views []CheckView
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(adoptDir, entry.Name())
		record, found, err := ReadCheck(filepath.Join(dir, checkRecordName))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		alive := record.State != checkRunning || checkAlive(record)
		views = append(views, checkView(record, alive, dir))
	}
	return views, nil
}

// checkView builds the view of record, whose subject's scratch directory is dir.
// alive says whether the process that wrote a running record is still running;
// it decides nothing for a record that ended.
func checkView(record CheckRecord, alive bool, dir string) CheckView {
	view := CheckView{
		Subject: record.Subject,
		State:   record.State, Running: record.Running, Tiers: record.Tiers, EndedAt: record.EndedAt, FailedTier: record.FailedTier,
		SuiteLog: filepath.Join(dir, suiteLogName), CheckOut: filepath.Join(dir, checkOutName),
	}
	opening := "adopt check of " + record.Subject + ":"
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
			fmt.Sprintf("%s %s; %s", opening, checkEndedWithoutVerdict, doneText),
			fmt.Sprintf("  see %s and %s, or re-run lym adopt %s", view.SuiteLog, view.CheckOut, record.Subject),
			"  the record clears at the next lym adopt with an owner write, which starts a fresh check",
		}
	case record.State == checkRunning:
		inFlight := "between tiers"
		if record.Running != nil {
			inFlight = "tier" + strconv.Itoa(*record.Running) + " running"
		}
		view.Lines = []string{fmt.Sprintf("%s running, %s; %s; log %s", opening, inFlight, doneText, view.SuiteLog)}
	case record.State == checkFailed:
		failure := record.Error
		if record.FailedTier != nil {
			failure = "tier" + strconv.Itoa(*record.FailedTier) + " failed"
		}
		view.Lines = []string{fmt.Sprintf("%s failed at %s, %s; %s; log %s", opening, ended, failure, doneText, view.SuiteLog)}
	default:
		view.Lines = []string{fmt.Sprintf("%s %s at %s; %s; log %s", opening, record.State, ended, doneText, view.SuiteLog)}
	}
	return view
}
