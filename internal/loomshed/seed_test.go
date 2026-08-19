package loomshed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestSeed_WritesExpectedStatus(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")

	if err := Seed(statusPath, statusLockPath, "my-slug", "my-parent"); err != nil {
		t.Fatalf("Seed() error = %v; want nil", err)
	}

	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}

	var got shedengine.Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal status file: %v", err)
	}

	if got.CurrentProducer != NamePreflight {
		t.Errorf("CurrentProducer = %q; want %q", got.CurrentProducer, NamePreflight)
	}
	if got.State != shedengine.StateRunning {
		t.Errorf("State = %q; want %q", got.State, shedengine.StateRunning)
	}
	if got.History == nil || len(got.History) != 0 {
		t.Errorf("History = %#v; want an empty non-nil slice", got.History)
	}
	if got.PauseRequested {
		t.Errorf("PauseRequested = true; want false")
	}

	var product loomengine.Status
	if err := json.Unmarshal(got.Product, &product); err != nil {
		t.Fatalf("unmarshal product payload: %v", err)
	}
	if product.Slug != "my-slug" {
		t.Errorf("product.Slug = %q; want %q", product.Slug, "my-slug")
	}
	if product.Parent != "my-parent" {
		t.Errorf("product.Parent = %q; want %q", product.Parent, "my-parent")
	}
	if product.StartSha != nil {
		t.Errorf("product.StartSha = %v; want nil", *product.StartSha)
	}
}

func TestSeed_RefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")

	if err := Seed(statusPath, statusLockPath, "my-slug", "my-parent"); err != nil {
		t.Fatalf("first Seed() error = %v; want nil", err)
	}

	before, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after first seed: %v", err)
	}

	if err := Seed(statusPath, statusLockPath, "other-slug", "other-parent"); err == nil {
		t.Fatal("second Seed() error = nil; want a non-nil refusal error")
	}

	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after second seed attempt: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("status file changed after a refused second Seed():\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestSeed_SucceedsWhenLockParentDirMissing(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	// statusLockPath's parent directory (dir/nested-lock-dir) does not exist yet.
	statusLockPath := filepath.Join(dir, "nested-lock-dir", "status.json.lock")

	if err := Seed(statusPath, statusLockPath, "my-slug", "my-parent"); err != nil {
		t.Fatalf("Seed() error = %v; want nil even when the lock file's parent directory is missing", err)
	}
	if _, err := os.Stat(statusPath); err != nil {
		t.Errorf("status file was not written: %v", err)
	}
}
