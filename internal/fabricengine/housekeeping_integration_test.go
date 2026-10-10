//go:build integration

// housekeeping_integration_test.go covers the hub stores' gc housekeeping end to end: Add and Remove write gc.auto=0 and maintenance.auto=false into both stores, Remove runs a foreground gc, and a gc failure is a warning that never fails the Remove.

package fabricengine_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
)

// lockedBuffer is a bytes.Buffer safe for the logger's writes from any goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// hubStorePaths returns h's code store and records store working directories, the two places the housekeeping keys live.
func hubStorePaths(t *testing.T, h *hubforge.Hub) (code, records string) {
	t.Helper()

	records, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	return h.PrimeWorktree(), records
}

// unsetHousekeepingKeys removes both keys from dir's config, tolerating a store that never had them.
func unsetHousekeepingKeys(t *testing.T, dir string) {
	t.Helper()

	for _, key := range []string{"gc.auto", "maintenance.auto"} {
		if _, _, _, err := gitexec.RunGit([]string{"config", "--unset-all", key}, dir); err != nil {
			t.Fatalf("unset %s in %s: %v", key, dir, err)
		}
	}
}

// requireHousekeepingKeys fails the test unless dir's config holds both keys at their disabled values, or neither key when want is false.
func requireHousekeepingKeys(t *testing.T, dir string, want bool) {
	t.Helper()

	config := gitkit.Git(t, dir, "config", "--local", "--list")
	for _, entry := range []string{"gc.auto=0", "maintenance.auto=false"} {
		present := strings.Contains("\n"+config+"\n", "\n"+entry+"\n")
		if present != want {
			t.Errorf("config of %s holds %q = %v; want %v", dir, entry, present, want)
		}
	}
}

// fillSampledLooseObjects commits many files in the code store at dir so objects/17, the directory git samples for its auto gc, holds more than a threshold of 1 loose objects.
func fillSampledLooseObjects(t *testing.T, dir string) {
	t.Helper()

	for i := 0; i < 4000; i++ {
		name := filepath.Join(dir, "gc-fill", fmt.Sprintf("file-%04d.txt", i))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(name, []byte(fmt.Sprintf("loose object %d\n", i)), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	gitkit.Git(t, dir, "add", "gc-fill")
	gitkit.Git(t, dir, "commit", "-m", "reachable loose objects")
	if n := sampledLooseObjects(t, dir); n < 2 {
		t.Fatalf("objects/17 holds %d loose objects; the fixture needs more than the lowered threshold's one", n)
	}
}

// sampledLooseObjects returns how many loose objects the code store at dir holds in objects/17.
func sampledLooseObjects(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(dir, ".git", "objects", "17"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read objects/17: %v", err)
	}
	return len(entries)
}

// writeUnreachableObject writes content as a loose blob no ref reaches into the code store at dir and returns the object's file path.
func writeUnreachableObject(t *testing.T, dir, content string) string {
	t.Helper()

	source := filepath.Join(t.TempDir(), "blob.txt")
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", source, err)
	}
	oid := gitkit.Git(t, dir, "hash-object", "-w", source)
	return filepath.Join(dir, ".git", "objects", oid[:2], oid[2:])
}

// TestStoreHousekeeping drives Add's key writes and Remove's foreground gc over hubs from hubforge.
// It is not parallel: it lowers fabricengine's gc threshold and captures the logger's output at Info, all process-global state.
// hubforge's clones may or may not carry the keys, so each subtest unsets them in both stores right before the step whose write it checks.
func TestStoreHousekeeping(t *testing.T) {
	var logs lockedBuffer
	logger.SetOutput(&logs)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})

	const slug = "gc-pair"

	t.Run("Add writes both keys into both stores", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")
		code, records := hubStorePaths(t, h)
		unsetHousekeepingKeys(t, code)
		unsetHousekeepingKeys(t, records)
		requireHousekeepingKeys(t, code, false)
		requireHousekeepingKeys(t, records, false)

		hubforge.AddPair(t, h, slug)

		requireHousekeepingKeys(t, code, true)
		requireHousekeepingKeys(t, records, true)
	})

	t.Run("Remove writes both keys into both stores", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")
		hubforge.AddPair(t, h, slug)
		code, records := hubStorePaths(t, h)
		unsetHousekeepingKeys(t, code)
		unsetHousekeepingKeys(t, records)

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		requireHousekeepingKeys(t, code, true)
		requireHousekeepingKeys(t, records, true)
	})

	t.Run("Remove packs a store above the lowered threshold", func(t *testing.T) {
		fabricengine.SetGCAutoThresholdForTest(t, 1)
		h := hubforge.NewHub(t, ".")
		hubforge.AddPair(t, h, slug)
		code, _ := hubStorePaths(t, h)
		fillSampledLooseObjects(t, code)

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		if n := sampledLooseObjects(t, code); n != 0 {
			t.Errorf("objects/17 still holds %d loose objects after Remove; want them packed", n)
		}
	})

	// The probe answers for the pair being removed: sessions of other pairs or an error skip the gc and keep the keys, an empty answer packs.
	for _, tc := range []struct {
		name     string
		probe    fabricengine.InFlightProbe
		wantPack bool
		wantLog  string
	}{
		{
			name:    "live sessions of other pairs skip the gc",
			probe:   func(string) ([]string, error) { return []string{"other-pair-session"}, nil },
			wantLog: "other-pair-session",
		},
		{
			name:    "a failing probe skips the gc",
			probe:   func(string) ([]string, error) { return nil, fmt.Errorf("probe-boom") },
			wantLog: "probe-boom",
		},
		{
			name:     "an empty answer packs",
			probe:    func(string) ([]string, error) { return nil, nil },
			wantPack: true,
		},
	} {
		t.Run("the in-flight probe: "+tc.name, func(t *testing.T) {
			fabricengine.SetGCAutoThresholdForTest(t, 1)
			h := hubforge.NewHub(t, ".")
			hubforge.AddPair(t, h, slug)
			h.Topology.SetInFlightProbe(tc.probe)
			code, records := hubStorePaths(t, h)
			unsetHousekeepingKeys(t, code)
			unsetHousekeepingKeys(t, records)
			fillSampledLooseObjects(t, code)

			if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
				t.Fatalf("Remove: %v", err)
			}

			requireHousekeepingKeys(t, code, true)
			requireHousekeepingKeys(t, records, true)
			if packed := sampledLooseObjects(t, code) == 0; packed != tc.wantPack {
				t.Errorf("objects/17 packed = %v; want %v", packed, tc.wantPack)
			}
			if tc.wantLog != "" && !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("log lacks %q:\n%s", tc.wantLog, logs.String())
			}
		})
	}

	t.Run("gc prunes unreachable loose objects older than a day and keeps younger ones", func(t *testing.T) {
		fabricengine.SetGCAutoThresholdForTest(t, 1)
		h := hubforge.NewHub(t, ".")
		hubforge.AddPair(t, h, slug)
		code, _ := hubStorePaths(t, h)
		fillSampledLooseObjects(t, code)
		oldObject := writeUnreachableObject(t, code, "unreachable and two days old\n")
		youngObject := writeUnreachableObject(t, code, "unreachable and fresh\n")
		twoDaysAgo := time.Now().Add(-48 * time.Hour)
		if err := os.Chtimes(oldObject, twoDaysAgo, twoDaysAgo); err != nil {
			t.Fatalf("backdate %s: %v", oldObject, err)
		}

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		if _, err := os.Stat(oldObject); !os.IsNotExist(err) {
			t.Errorf("stat of the two-day-old unreachable object = %v; want it pruned", err)
		}
		if _, err := os.Stat(youngObject); err != nil {
			t.Errorf("the fresh unreachable object is gone: %v; want it kept", err)
		}
	})

	t.Run("a failing gc is a warning and never fails Remove", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")
		hubforge.AddPair(t, h, slug)
		_, records := hubStorePaths(t, h)
		// An unparsable size makes `gc --auto` exit 128 on its config parse, whatever the threshold.
		gitkit.Git(t, records, "config", "gc.bigPackThreshold", "bogus")

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v; a failing gc must not fail it", err)
		}

		log := logs.String()
		if !strings.Contains(log, "store gc failed") || !strings.Contains(log, "store=records") {
			t.Errorf("log lacks a warning naming the records store's gc failure:\n%s", log)
		}
	})
}
