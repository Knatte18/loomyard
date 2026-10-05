// entryobservation_test.go holds the status-file helpers the entry-observation and handoff-voucher tests share.

package loomcli

import (
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// writeStatusFixture writes st to path/lockPath via the production WriteJSON primitive.
func writeStatusFixture(t *testing.T, path, lockPath string, st shedengine.Status) {
	t.Helper()
	if err := state.WriteJSON(path, lockPath, st); err != nil {
		t.Fatalf("write status file: %v", err)
	}
}

// productJSON marshals p for embedding as a shedengine.Status.Product payload.
func productJSON(t *testing.T, p loomengine.Status) []byte {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	return raw
}
