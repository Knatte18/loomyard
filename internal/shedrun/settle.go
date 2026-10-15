// settle.go declares the Go driver's settle-record format: the record, the stop identity it names, the settler and action vocabularies, and the tolerant decoder over a file's bytes.

package shedrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// StopKey identifies a stop: the status file's current producer, history length and state.
type StopKey struct {
	CurrentProducer string `json:"current_producer"`
	HistoryLength   int    `json:"history_length"`
	State           string `json:"state"`
}

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

// ParseSettleRecords decodes the bytes of a settle-records file into one record per non-blank line, in order.
// An unknown key is read and dropped, so a record a newer binary wrote stays readable.
// A line that does not decode is skipped and returned as an error naming its 1-based line number, so a torn append never blocks the records around it.
func ParseSettleRecords(data []byte) ([]SettleRecord, []error) {
	var records []SettleRecord
	var skipped []error
	for number, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record SettleRecord
		if err := json.Unmarshal(line, &record); err != nil {
			skipped = append(skipped, fmt.Errorf("settle records line %d does not decode: %w", number+1, err))
			continue
		}
		records = append(records, record)
	}
	return records, skipped
}
