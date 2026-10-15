// settle.go declares the Go driver's appender and reader of the settle-records file:
// one JSON line per action the driver takes at a stop, in the told settle-records file.
// The repair cap, the one verify retry and the one hand-over per build are counted from these records, so a restarted driver keeps every cap.

package sheddriver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// SettleRecord is one line of the settle-records file.
type SettleRecord = shedrun.SettleRecord

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
	records, skipped := shedrun.ParseSettleRecords(data)
	for _, skip := range skipped {
		logger.Warn("settle records: skipped a line that does not decode", "path", path, "error", skip)
	}
	return records, nil
}

// GoVerifyRetrySpent reports whether any record is a verify retry, the one read the Go driver and a publish retry share.
func GoVerifyRetrySpent(records []SettleRecord) bool {
	for _, record := range records {
		if record.Action == shedrun.SettleVerifyRetry {
			return true
		}
	}
	return false
}
