// settle.go declares the Go driver's settle records and their appender and reader:
// one JSON line per action the driver takes at a stop, in the told settle-records file.
// The repair cap, the one verify retry and the one hand-over per build are counted from these records, so a restarted driver keeps every cap.

package sheddriver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// The settlers a record names: who decided the action.
const (
	SettlerGoRule  = "go-rule"
	SettlerTackler = "tackler"
	SettlerArbiter = "arbiter"
	SettlerParent  = "parent"
)

// The actions a record names: what the driver did at the stop.
const (
	SettleRestep      = "restep"
	SettleVerifyRetry = "verify-retry"
	SettleHandOver    = "hand-over"
	SettleSpawn       = "spawn"
	SettleHandBack    = "hand-back"
	SettleBusyExit    = "busy-exit"
	SettleRunningExit = "running-exit"
	SettleMoved       = "moved"
)

// SettleRecord is one line of the settle-records file.
// Build is the build identity the action was taken under and Target the row or verb it aimed at; each is omitted when empty.
type SettleRecord struct {
	StopKey StopKey   `json:"stop_key"`
	Settler string    `json:"settler"`
	Action  string    `json:"action"`
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
	Build   string    `json:"build,omitempty"`
	Target  string    `json:"target,omitempty"`
}

// AppendSettleRecord appends r as one JSON line to the file at path, creating the file and its directory.
func AppendSettleRecord(path string, r SettleRecord) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("sheddriver:encode settle record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("sheddriver:create settle record directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("sheddriver:open settle records %q: %w", path, err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("sheddriver:append settle record %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("sheddriver:close settle records %q: %w", path, err)
	}
	return nil
}

// ReadSettleRecords reads every record of the file at path, in order.
// An absent file reads nil, an unknown key is kept readable, and a line that does not decode is skipped with one Warn, so a torn append never blocks a cap.
func ReadSettleRecords(path string) ([]SettleRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sheddriver:read settle records %q: %w", path, err)
	}
	var records []SettleRecord
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record SettleRecord
		if err := json.Unmarshal(line, &record); err != nil {
			logger.Warn("settle records: skipped a line that does not decode", "path", path, "error", err)
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

// GoVerifyRetrySpent reports whether any record is a verify retry, the one read the Go driver and a publish retry share.
func GoVerifyRetrySpent(records []SettleRecord) bool {
	for _, record := range records {
		if record.Action == SettleVerifyRetry {
			return true
		}
	}
	return false
}
