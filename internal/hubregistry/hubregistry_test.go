// hubregistry_test.go covers Record, Lookup and Live over temporary hubs that hold only a Bolt shortname record.
// Nothing is spawned.

package hubregistry_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newHub creates a hub directory under root holding a Bolt shortname record of shortname, and returns its path.
func newHub(t *testing.T, root, name, shortname string) string {
	t.Helper()
	hub := filepath.Join(root, name)
	if err := os.MkdirAll(fabricengine.BoltDir(hub), 0o755); err != nil {
		t.Fatalf("MkdirAll(bolt) = %v; want nil", err)
	}
	if shortname != "" {
		if err := fabricengine.WriteShortname(fabricengine.BoltDir(hub), shortname); err != nil {
			t.Fatalf("WriteShortname = %v; want nil", err)
		}
	}
	return hub
}

// recordedPaths returns the hub paths of the entries in dir's record that are still live.
// An entry is live when its hub's Bolt shortname record still reads the entry's own shortname.
func recordedPaths(t *testing.T, dir string) []string {
	t.Helper()
	live, err := hubregistry.Live(dir)
	if err != nil {
		t.Fatalf("Live() error = %v; want nil", err)
	}
	var paths []string
	for _, entry := range live {
		paths = append(paths, entry.HubPath)
	}
	return paths
}

// TestRecord covers the upsert, the drops of vanished and renamed hubs, and the entries it keeps.
// It also covers concurrent recorders and a corrupt file.
func TestRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	t.Run("FirstRecord", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		hub := newHub(t, root, "a", "aa")
		if err := hubregistry.Record(dir, hub, "aa", now); err != nil {
			t.Fatalf("Record() error = %v; want nil", err)
		}
		live, err := hubregistry.Live(dir)
		if err != nil || len(live) != 1 || live[0].HubPath != hub || live[0].Shortname != "aa" || !live[0].RecordedAt.Equal(now) {
			t.Errorf("Live() = %+v, %v; want the one entry %s aa %v", live, err, hub, now)
		}
	})

	t.Run("SecondRecordUpdatesTime", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		hub := newHub(t, root, "a", "aa")
		later := now.Add(time.Hour)
		for _, at := range []time.Time{now, later} {
			if err := hubregistry.Record(dir, hub, "aa", at); err != nil {
				t.Fatalf("Record() error = %v; want nil", err)
			}
		}
		live, err := hubregistry.Live(dir)
		if err != nil || len(live) != 1 || !live[0].RecordedAt.Equal(later) {
			t.Errorf("Live() = %+v, %v; want one entry recorded at %v", live, err, later)
		}
	})

	t.Run("GonePathIsDropped", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		gone := newHub(t, root, "gone", "gg")
		kept := newHub(t, root, "kept", "kk")
		if err := hubregistry.Record(dir, gone, "gg", now); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(gone); err != nil {
			t.Fatal(err)
		}
		if err := hubregistry.Record(dir, kept, "kk", now); err != nil {
			t.Fatal(err)
		}
		if got := recordedPaths(t, dir); len(got) != 1 || got[0] != kept {
			t.Errorf("Live() paths = %v; want only %s", got, kept)
		}
	})

	t.Run("PathUnderRegularFileIsKept", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		file := filepath.Join(root, "file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		unsure := filepath.Join(file, "hub")
		other := newHub(t, root, "other", "oo")
		if err := hubregistry.Record(dir, unsure, "uu", now); err != nil {
			t.Fatal(err)
		}
		if err := hubregistry.Record(dir, other, "oo", now); err != nil {
			t.Fatal(err)
		}
		// The unsure entry has no readable Bolt record, so Live drops it from its answer, but Record kept it in the file.
		if _, err := hubregistry.Lookup(dir, "uu"); !errors.Is(err, hubregistry.ErrUnresolved) {
			t.Fatalf("Lookup(uu) error = %v; want ErrUnresolved", err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, hubregistry.RecordFileName))
		if err != nil || !strings.Contains(string(raw), unsure) {
			t.Errorf("record = %q, %v; want it to still hold %s", raw, err, unsure)
		}
	})

	t.Run("ChangedShortnameIsDropped", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		changed := newHub(t, root, "changed", "aa")
		other := newHub(t, root, "other", "oo")
		if err := hubregistry.Record(dir, changed, "aa", now); err != nil {
			t.Fatal(err)
		}
		if err := fabricengine.WriteShortname(fabricengine.BoltDir(changed), "bb"); err != nil {
			t.Fatal(err)
		}
		if err := hubregistry.Record(dir, other, "oo", now); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, hubregistry.RecordFileName))
		if err != nil || strings.Contains(string(raw), changed) {
			t.Errorf("record = %q, %v; want it without %s", raw, err, changed)
		}
	})

	t.Run("AbsentShortnameRecordIsKept", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		bare := newHub(t, root, "bare", "")
		other := newHub(t, root, "other", "oo")
		if err := hubregistry.Record(dir, bare, "bb", now); err != nil {
			t.Fatal(err)
		}
		if err := hubregistry.Record(dir, other, "oo", now); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, hubregistry.RecordFileName))
		if err != nil || !strings.Contains(string(raw), bare) {
			t.Errorf("record = %q, %v; want it to still hold %s", raw, err, bare)
		}
	})

	t.Run("ConcurrentRecordersLoseNoEntry", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		const recorders = 12
		var wg sync.WaitGroup
		errs := make(chan error, recorders)
		for i := range recorders {
			hub := newHub(t, root, fmt.Sprintf("h%d", i), fmt.Sprintf("h%c", 'a'+i))
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- hubregistry.Record(dir, hub, fmt.Sprintf("h%c", 'a'+i), now)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("Record() error = %v; want nil", err)
			}
		}
		if got := recordedPaths(t, dir); len(got) != recorders {
			t.Errorf("Live() holds %d entries; want %d", len(got), recorders)
		}
	})

	t.Run("CorruptFileRefusedNamingPath", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		path := filepath.Join(dir, hubregistry.RecordFileName)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := hubregistry.Record(dir, newHub(t, root, "a", "aa"), "aa", now)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "way forward") {
			t.Errorf("Record() error = %v; want a refusal naming %s and a way forward", err, path)
		}
	})
}

// TestLookup covers the one match, both refusals, hubs that drop out, an absent file, Live and a corrupt file.
func TestLookup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	t.Run("OneMatch", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		hub := newHub(t, root, "a", "aa")
		other := newHub(t, root, "b", "bb")
		for shortname, path := range map[string]string{"aa": hub, "bb": other} {
			if err := hubregistry.Record(dir, path, shortname, now); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := hubregistry.Lookup(dir, "aa"); err != nil || got != hub {
			t.Errorf("Lookup(aa) = %q, %v; want %q", got, err, hub)
		}
	})

	t.Run("UnknownShortnameRefused", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := hubregistry.Lookup(dir, "zz")
		if !errors.Is(err, hubregistry.ErrUnresolved) || !strings.Contains(err.Error(), "lym creel inbox") {
			t.Errorf("Lookup(zz) error = %v; want ErrUnresolved naming lym creel inbox", err)
		}
	})

	t.Run("SharedShortnameRefusedNamingBothPaths", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		first := newHub(t, root, "first", "aa")
		second := newHub(t, root, "second", "aa")
		for _, path := range []string{first, second} {
			if err := hubregistry.Record(dir, path, "aa", now); err != nil {
				t.Fatal(err)
			}
		}
		_, err := hubregistry.Lookup(dir, "aa")
		if !errors.Is(err, hubregistry.ErrUnresolved) || !strings.Contains(err.Error(), first) || !strings.Contains(err.Error(), second) || !strings.Contains(err.Error(), "distinct shortnames") {
			t.Errorf("Lookup(aa) error = %v; want ErrUnresolved naming distinct shortnames and both paths", err)
		}
	})

	t.Run("ChangedAndRemovedHubsDropOut", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		changed := newHub(t, root, "changed", "aa")
		removed := newHub(t, root, "removed", "rr")
		kept := newHub(t, root, "kept", "kk")
		for shortname, path := range map[string]string{"aa": changed, "rr": removed, "kk": kept} {
			if err := hubregistry.Record(dir, path, shortname, now); err != nil {
				t.Fatal(err)
			}
		}
		if err := fabricengine.WriteShortname(fabricengine.BoltDir(changed), "bb"); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(removed); err != nil {
			t.Fatal(err)
		}
		for _, shortname := range []string{"aa", "bb", "rr"} {
			if _, err := hubregistry.Lookup(dir, shortname); !errors.Is(err, hubregistry.ErrUnresolved) {
				t.Errorf("Lookup(%s) error = %v; want ErrUnresolved", shortname, err)
			}
		}
		if got := recordedPaths(t, dir); len(got) != 1 || got[0] != kept {
			t.Errorf("Live() paths = %v; want only %s", got, kept)
		}
	})

	t.Run("AbsentFileIsNoEntries", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if live, err := hubregistry.Live(dir); err != nil || len(live) != 0 {
			t.Errorf("Live() = %+v, %v; want none", live, err)
		}
	})

	t.Run("LiveIsInShortnameOrder", func(t *testing.T) {
		t.Parallel()
		dir, root := t.TempDir(), t.TempDir()
		for _, shortname := range []string{"cc", "aa", "bb"} {
			if err := hubregistry.Record(dir, newHub(t, root, shortname, shortname), shortname, now); err != nil {
				t.Fatal(err)
			}
		}
		live, err := hubregistry.Live(dir)
		if err != nil || len(live) != 3 || live[0].Shortname != "aa" || live[1].Shortname != "bb" || live[2].Shortname != "cc" {
			t.Errorf("Live() = %+v, %v; want aa, bb, cc in order", live, err)
		}
	})

	t.Run("CorruptFileIsNotUnresolved", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, hubregistry.RecordFileName)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		for name, call := range map[string]func() error{
			"Lookup": func() error { _, err := hubregistry.Lookup(dir, "aa"); return err },
			"Live":   func() error { _, err := hubregistry.Live(dir); return err },
		} {
			err := call()
			if err == nil || errors.Is(err, hubregistry.ErrUnresolved) || !strings.Contains(err.Error(), path) {
				t.Errorf("%s() error = %v; want a refusal naming %s that is not ErrUnresolved", name, err, path)
			}
		}
	})
}
