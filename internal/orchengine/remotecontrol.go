// remotecontrol.go holds the record of the remote control lym last set for the orch strand.
// The RC button on reed's status bar shows it.

package orchengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// RemoteControlRecord is what lym last set of an orch strand's remote control.
// Claude Code gives lym no reading of the state, so the record holds what a launch or an RC click set:
// a session whose remote control the operator turned off inside Claude Code still reads on.
type RemoteControlRecord struct {
	// Strand is the guid of the orch strand the record is about.
	Strand string `json:"strand"`
	// On reports that a launch with the provider's flag or an RC click turned remote control on.
	On bool `json:"on"`
	// At is when the record was written.
	At time.Time `json:"at"`
}

// WriteRemoteControl writes rec, creating p.Dir first.
func WriteRemoteControl(p Paths, rec RemoteControlRecord) error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("orch: create state dir: %w", err)
	}
	if err := state.WriteJSON(p.RemoteControlPath, p.RemoteControlLockPath, rec); err != nil {
		return fmt.Errorf("orch: save the remote-control record: %w", err)
	}
	return nil
}

// ReadRemoteControl returns the record and true when the record file names strand.
// An absent record, or one naming another strand, reports false.
// It reads without the lock and creates nothing, so the status bar's redraw never blocks a writer;
// internal/state writes the file atomically.
func ReadRemoteControl(p Paths, strand string) (RemoteControlRecord, bool, error) {
	data, err := os.ReadFile(p.RemoteControlPath)
	if errors.Is(err, os.ErrNotExist) {
		return RemoteControlRecord{}, false, nil
	}
	if err != nil {
		return RemoteControlRecord{}, false, fmt.Errorf("orch: read the remote-control record: %w", err)
	}
	var rec RemoteControlRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return RemoteControlRecord{}, false, fmt.Errorf("orch: read the remote-control record: %w", err)
	}
	if rec.Strand != strand {
		return RemoteControlRecord{}, false, nil
	}
	return rec, true, nil
}

// RemoteControlOn reports whether the record names strand and reads on.
// An absent record, or one naming another strand, reads off.
func RemoteControlOn(p Paths, strand string) (bool, error) {
	rec, recorded, err := ReadRemoteControl(p, strand)
	return recorded && rec.On, err
}
