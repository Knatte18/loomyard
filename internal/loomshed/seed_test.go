package loomshed

import (
	"encoding/json"
	"errors"
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

	err = Seed(statusPath, statusLockPath, "other-slug", "other-parent")
	if err == nil {
		t.Fatal("second Seed() error = nil; want a non-nil refusal error")
	}
	if !errors.Is(err, ErrSeedExists) {
		t.Errorf("second Seed() error = %v; want errors.Is(err, ErrSeedExists)", err)
	}

	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after second seed attempt: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("status file changed after a refused second Seed():\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestSeed_RefusesUndecodableFileAsExists is crucible round fable5-high-r5's F3 regression guard: a
// status file that is PRESENT but cannot be decoded (malformed JSON, or an unknown top-level field)
// is still present, so Seed must refuse it via ErrSeedExists — never overwrite it, and never
// escalate the decode failure as its own error. Escalating it made `lyx loom run` refuse on the
// envelope before ever spawning a driver, for the exact "poisoned status file" state
// manifest/designs/loom.md's crash-recovery section promises never looks like bootstrap's own gate;
// mapping it to ErrSeedExists lets the bootstrap proceed and defers the decode diagnosis to the
// driver's own Shed.Run step-1 read gate, exactly as a cleanly-decoding existing file already does.
//
// Reproduced live before this test: `lyx loom run` against a status file overwritten with non-JSON
// returned `{"error":"unmarshal state: invalid character...","ok":false}` and never spawned a
// driver.
func TestSeed_RefusesUndecodableFileAsExists(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "malformed JSON", content: `{ "current_producer": "Discussion-Write", "state": "run`},
		{name: "not JSON at all", content: `THIS IS NOT JSON AT ALL {{{`},
		{name: "unknown top-level field", content: `{"current_producer":"Preflight","state":"running","history":[],"__unknown__":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			statusLockPath := filepath.Join(dir, "status.json.lock")
			if err := os.WriteFile(statusPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write poisoned status fixture: %v", err)
			}

			err := Seed(statusPath, statusLockPath, "my-slug", "my-parent")
			if err == nil {
				t.Fatal("Seed() error = nil; want a non-nil refusal for a present-but-undecodable file")
			}
			if !errors.Is(err, ErrSeedExists) {
				t.Errorf("Seed() error = %v; want errors.Is(err, ErrSeedExists) so the bootstrap tolerates it and the driver's own read gate diagnoses the decode failure", err)
			}

			after, rerr := os.ReadFile(statusPath)
			if rerr != nil {
				t.Fatalf("read status file after refused Seed: %v", rerr)
			}
			if string(after) != tt.content {
				t.Errorf("status file changed after a refused Seed():\nbefore: %s\nafter:  %s — a corrupt file must be left untouched (it may be the only forensic record)", tt.content, after)
			}
		})
	}
}

// TestSeed_OtherFailureDoesNotMatchErrSeedExists proves ErrSeedExists names the refusal case
// exclusively: a Seed failure caused by something other than an existing status file -- here, a
// status path whose parent directory cannot be created -- must not match the sentinel, so a
// re-entrant caller that treats ErrSeedExists as success does not also swallow an unrelated error.
func TestSeed_OtherFailureDoesNotMatchErrSeedExists(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file where a directory component of statusPath must be, so
	// os.MkdirAll for the status file's parent fails with something other than the
	// found-branch refusal.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	statusPath := filepath.Join(blocker, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")

	err := Seed(statusPath, statusLockPath, "my-slug", "my-parent")
	if err == nil {
		t.Fatal("Seed() error = nil; want a non-nil error when the status path's parent cannot be created")
	}
	if errors.Is(err, ErrSeedExists) {
		t.Errorf("Seed() error = %v; want it NOT to match ErrSeedExists", err)
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
