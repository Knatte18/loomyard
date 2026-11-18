// accountpick_integration_test.go drives the Worktree-Create row's Claude account pick over a hubforge hub.

package battencli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWire_CreateWorktreePicksTheNewWorktreesAccount pins where the create row picks a worktree's account.
// A create picks once after the add.
// A row re-entered over the complete pair picks once keeping an entry.
// A failed add picks nothing.
// A pick that fails leaves the create successful.
func TestWire_CreateWorktreePicksTheNewWorktreesAccount(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	type pick struct {
		slug      string
		keepEntry bool
	}
	var picks []pick
	wireRecording := func(t *testing.T, slug string) *battenCLI {
		t.Helper()
		c := wireForHub(t, h, slug, nil)
		if c.pickAccount == nil {
			t.Fatal("wire left pickAccount nil; want the pick seam filled")
		}
		c.pickAccount = func(_ *lymcwd.Location, slug string, keepEntry bool) { picks = append(picks, pick{slug, keepEntry}) }
		return c
	}

	const created = "pick-created"
	if err := wireRecording(t, created).env.CreateWorktree(context.Background(), holdNothing); err != nil {
		t.Fatalf("CreateWorktree(%s): %v", created, err)
	}
	if want := []pick{{created, false}}; !reflect.DeepEqual(picks, want) {
		t.Errorf("picks after a create = %v; want %v", picks, want)
	}

	picks = nil
	if err := wireRecording(t, created).env.CreateWorktree(context.Background(), holdNothing); err != nil {
		t.Fatalf("CreateWorktree(%s) over the complete pair: %v", created, err)
	}
	if want := []pick{{created, true}}; !reflect.DeepEqual(picks, want) {
		t.Errorf("picks after a row re-entered over the complete pair = %v; want %v", picks, want)
	}

	picks = nil
	const leftover = "pick-leftover"
	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(h.Location))
	if err != nil {
		t.Fatalf("load the fabric config: %v", err)
	}
	mustRunGit(t, h.PrimeWorktree(), "branch", cfg.BranchPrefix+leftover)
	if err := wireRecording(t, leftover).env.CreateWorktree(context.Background(), holdNothing); err == nil {
		t.Fatalf("CreateWorktree(%s) = nil; want the leftover branch refused", leftover)
	}
	if len(picks) != 0 {
		t.Errorf("picks after a failed add = %v; want none", picks)
	}

	// The real pick refuses an account list naming one account twice, and the create still succeeds.
	machineFile := configengine.MachineFile(h.BoardDir(), "claude")
	if err := os.MkdirAll(filepath.Dir(machineFile), 0o755); err != nil {
		t.Fatalf("create the machine layer directory: %v", err)
	}
	duplicate := "accounts:\n  - {name: a, config_dir: /accounts/a, weight: 1}\n  - {name: a, config_dir: /accounts/b, weight: 1}\n"
	if err := os.WriteFile(machineFile, []byte(duplicate), 0o644); err != nil {
		t.Fatalf("write the machine layer file: %v", err)
	}
	const failing = "pick-failing"
	if err := wireForHub(t, h, failing, nil).env.CreateWorktree(context.Background(), holdNothing); err != nil {
		t.Errorf("CreateWorktree(%s) with a failing pick = %v; want nil", failing, err)
	}
}
