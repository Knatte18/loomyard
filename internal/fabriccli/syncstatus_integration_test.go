// syncstatus_integration_test.go drives `lym fabric sync-status` over a hubforge hub.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The field takes its idle interval from the hub's config, and an unreadable ledger prints a placeholder and exits zero.
func TestCommand_SyncStatus(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	config := strings.Replace(fabricengine.ConfigTemplate(), "push_interval_s: 60", "push_interval_s: 42", 1)
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "fabric"), []byte(config), 0o644); err != nil {
		t.Fatalf("write the fabric config: %v", err)
	}
	requireLine := func(want string) {
		t.Helper()
		code, out := runFabric(t, h.PrimeWorktree(), "sync-status", "--hub", h.Path)
		if code != 0 || out != want+"\n" {
			t.Errorf("sync-status = %d, %q; want 0 and %q", code, out, want+"\n")
		}
	}

	requireLine("sync: 0m · 42s")

	err := pushledger.Update(h.Path, pushledger.LandingKey, time.Now(), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return pushledger.Entry{FirstOwed: time.Now(), IntervalS: 120}, true, nil
	})
	if err != nil {
		t.Fatalf("pushledger.Update() = %v", err)
	}
	requireLine("sync: 0m · 120s")

	entry := pushledger.EntryPath(h.Path, pushledger.LandingKey)
	if err := os.WriteFile(filepath.Clean(entry), []byte("not json"), 0o644); err != nil {
		t.Fatalf("corrupt the ledger entry: %v", err)
	}
	requireLine("sync: ?")
}
