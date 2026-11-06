// hubdirectory_test.go covers the production HubDirectory over temporary hubs that hold only a shortname record and a board creel.yaml, so nothing is spawned.
// The rows replace the package's client seam, which is process-global state, so the test does not run in parallel.

package creelcli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newShortnameHub creates a hub directory holding only a shortname record and returns its path.
func newShortnameHub(t *testing.T, root, name, shortname string) string {
	t.Helper()
	hub := filepath.Join(root, name)
	boltDir := fabricengine.BoltDir(hub)
	if err := os.MkdirAll(boltDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", boltDir, err)
	}
	if err := fabricengine.WriteShortname(boltDir, shortname); err != nil {
		t.Fatalf("WriteShortname(%q) = %v", shortname, err)
	}
	return hub
}

// writeBoardCreelConfig writes the hub's board creel.yaml.
func writeBoardCreelConfig(t *testing.T, hub, text string) {
	t.Helper()
	board := fabricengine.BoltDir(hub)
	if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) = %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(board, "creel"), []byte(text), 0o644); err != nil {
		t.Fatalf("WriteFile(creel.yaml) = %v", err)
	}
}

func TestHubDirectory(t *testing.T) {
	tiergate.Require(t, 0)

	t.Run("an unknown shortname's error matches ErrHubUnresolved and a corrupt record's does not", func(t *testing.T) {
		registry := t.TempDir()
		directory := newHubDirectory("/hubs/self", "ly", t.TempDir(), registry, nil)
		if _, err := directory.Lookup("zz"); !errors.Is(err, creelengine.ErrHubUnresolved) {
			t.Errorf("Lookup of an unknown shortname = %v; want an error matching ErrHubUnresolved", err)
		}
		if err := os.WriteFile(filepath.Join(registry, hubregistry.RecordFileName), []byte("{not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(record) = %v", err)
		}
		_, err := directory.Lookup("zz")
		if err == nil || errors.Is(err, creelengine.ErrHubUnresolved) {
			t.Errorf("Lookup over a corrupt record = %v; want an error that does not match ErrHubUnresolved", err)
		}
	})

	t.Run("a recorded hub is found by its shortname", func(t *testing.T) {
		root, registry := t.TempDir(), t.TempDir()
		other := newShortnameHub(t, root, "other", "xx")
		if err := newHubDirectory(other, "xx", root, registry, nil).Record(); err != nil {
			t.Fatalf("Record() = %v", err)
		}
		got, err := newHubDirectory("/hubs/self", "ly", root, registry, nil).Lookup("xx")
		if err != nil || got != other {
			t.Errorf("Lookup(xx) = (%q, %v); want (%q, nil)", got, err, other)
		}
	})

	t.Run("Geometry names the other hub's socket under the temp root", func(t *testing.T) {
		root, tempRoot := t.TempDir(), t.TempDir()
		other := newShortnameHub(t, root, "other", "xx")
		geom, err := newHubDirectory("/hubs/self", "ly", tempRoot, t.TempDir(), nil).Geometry(other)
		if err != nil {
			t.Fatalf("Geometry() = %v", err)
		}
		if want := filepath.Join(tempRoot, creelengine.SocketName("xx", other)); geom.SocketPath != want || geom.HubPath != other || geom.Shortname != "xx" {
			t.Errorf("Geometry() = %+v; want the socket %q of the hub %q", geom, want, other)
		}
	})

	t.Run("TrustedHubs reads a rewritten base file at the next call with no restart", func(t *testing.T) {
		self := newShortnameHub(t, t.TempDir(), "self", "ly")
		directory := newHubDirectory(self, "ly", t.TempDir(), t.TempDir(), nil)
		for _, text := range []string{"trusted_hubs:\n  - aa\n", "trusted_hubs:\n  - bb\n  - cc\n"} {
			writeBoardCreelConfig(t, self, text)
			got, err := directory.TrustedHubs()
			if err != nil {
				t.Fatalf("TrustedHubs() = %v", err)
			}
			want := []string{"aa"}
			if strings.Contains(text, "bb") {
				want = []string{"bb", "cc"}
			}
			if !slices.Equal(got, want) {
				t.Errorf("TrustedHubs() after writing %q = %v; want %v", text, got, want)
			}
		}
	})

	t.Run("Spawn hands the client seam the hub path and the log path", func(t *testing.T) {
		previous := seam
		t.Cleanup(func() { seam = previous })
		var gotHub, gotLog string
		seam.spawn = func(hubPath, logPath string) (creelengine.Spawn, error) {
			gotHub, gotLog = hubPath, logPath
			return creelengine.Spawn{PID: 7}, nil
		}
		spawned, err := newHubDirectory("/hubs/self", "ly", t.TempDir(), t.TempDir(), nil).Spawn("/hubs/other", "/logs/other.log")
		if err != nil || spawned.PID != 7 || gotHub != "/hubs/other" || gotLog != "/logs/other.log" {
			t.Errorf("Spawn() = (%+v, %v) with the seam called for (%q, %q); want pid 7 for (/hubs/other, /logs/other.log)", spawned, err, gotHub, gotLog)
		}
	})

	t.Run("a registryDir that fails makes Lookup and Record name the cause while the directory still builds", func(t *testing.T) {
		previous := seam
		t.Cleanup(func() { seam = previous })
		seam.registryDir = func() (string, error) { return "", errors.New("no home here") }
		directory := daemonHubs("/hubs/self", "ly", t.TempDir())
		_, lookupErr := directory.Lookup("xx")
		for name, err := range map[string]error{"Lookup": lookupErr, "Record": directory.Record()} {
			if err == nil || !strings.Contains(err.Error(), "no home here") || !strings.Contains(err.Error(), "XDG_STATE_HOME") {
				t.Errorf("%s with a failing registryDir = %v; want the cause and the way forward naming XDG_STATE_HOME", name, err)
			}
		}
		if errors.Is(lookupErr, creelengine.ErrHubUnresolved) {
			t.Errorf("Lookup with a failing registryDir = %v; want an error that does not match ErrHubUnresolved", lookupErr)
		}
	})
}
