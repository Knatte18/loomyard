// wiring_geometry_test.go covers liveHubGeometries, the seam that lists the machine's live hubs, over a temp machine record.

package slackcli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLiveHubGeometries reads the machine record of hubs from a temp per-user state directory.
// It is not parallel: it points the process environment at that directory, which t.Setenv forbids a parallel test.
func TestLiveHubGeometries(t *testing.T) {
	tiergate.Require(t, 0)

	// machineWithHub points the per-user state and config directories at temp ones and records one live hub in the former.
	// It returns the state directory and the hub's path.
	machineWithHub := func(t *testing.T) (stateDir, hubPath string) {
		t.Helper()
		base := t.TempDir()
		t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
		t.Setenv("LOCALAPPDATA", base)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
		stateDir, err := standalonestate.UserStateDir()
		if err != nil {
			t.Fatalf("UserStateDir() = %v", err)
		}
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v", stateDir, err)
		}
		hubPath = filepath.Join(base, "hub")
		boltDir := fabricengine.BoltDir(hubPath)
		if err := os.MkdirAll(boltDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v", boltDir, err)
		}
		if err := fabricengine.WriteShortname(boltDir, "hub"); err != nil {
			t.Fatalf("WriteShortname() = %v", err)
		}
		if err := hubregistry.Record(stateDir, hubPath, "hub", time.Now()); err != nil {
			t.Fatalf("Record() = %v", err)
		}
		return stateDir, hubPath
	}

	t.Run("BuildsOneGeometryPerLiveHub", func(t *testing.T) {
		_, hubPath := machineWithHub(t)
		got, err := liveHubGeometries()
		if err != nil || len(got) != 1 || got[0].HubPath != hubPath || got[0].Shortname != "hub" {
			t.Errorf("liveHubGeometries() = (%+v, %v); want one geometry of the hub at %q with shortname hub", got, err, hubPath)
		}
	})

	t.Run("AnUnreadableRecordIsRefusedNamingItsPathAndWrappingTheReadError", func(t *testing.T) {
		stateDir, _ := machineWithHub(t)
		recordPath := filepath.Join(stateDir, hubregistry.RecordFileName)
		if err := os.WriteFile(recordPath, []byte("not json"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", recordPath, err)
		}
		got, err := liveHubGeometries()
		wantNaming := "read the machine record of hubs " + recordPath
		if err == nil || !strings.Contains(err.Error(), wantNaming) || !errors.Is(err, state.ErrDecode) {
			t.Errorf("liveHubGeometries() = (%+v, %v); want an error containing %q and wrapping state.ErrDecode", got, err, wantNaming)
		}
	})

	t.Run("AHubWhoseGeometryFailsIsNamedByItsPath", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the token directory and the state directory share LOCALAPPDATA on windows")
		}
		_, hubPath := machineWithHub(t)
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		got, err := liveHubGeometries()
		if err == nil || !strings.Contains(err.Error(), hubPath) {
			t.Errorf("liveHubGeometries() = (%+v, %v); want an error naming the hub at %q", got, err, hubPath)
		}
	})
}
