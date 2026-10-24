// cleanup_helper_test.go holds the status and file writers the cleanup tests of every tier share.

package battencli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func statusJSON(t *testing.T, state string) []byte {
	t.Helper()
	data, err := json.Marshal(shedengine.Status{CurrentProducer: "Worktree-Teardown", State: shedengine.State(state), History: []shedengine.HistoryEntry{}})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	return data
}

func writeCleanupFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
