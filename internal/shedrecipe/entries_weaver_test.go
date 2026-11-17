// entries_weaver_test.go pins the weaver implementer entry.
// It covers what builds the producer and what refuses when the row builds.

package shedrecipe

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// writeWeaverConfig writes content as the anchor's weaver.yaml.
func writeWeaverConfig(t *testing.T, anchorPath, content string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, lymdirs.LymDirName, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "weaver.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write weaver.yaml: %v", err)
	}
}

func TestWeaverEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("FullEnvBuildsTheWeaverProducer", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		producer, err := weaverEntry(RowIdentity{Name: "Weaver"}, Config{}, env)
		if err != nil {
			t.Fatalf("weaverEntry() error = %v; want nil", err)
		}
		want := reflect.TypeOf(loomshed.NewWeaverProducer("", env.ConfigDirs, nil, weaverengine.RunDeps{}, env.ImplementerSeam))
		if got := reflect.TypeOf(producer); got != want {
			t.Errorf("weaverEntry() producer type = %v; want %v", got, want)
		}
	})

	t.Run("EachRequiredSeamMissingIsRefusedNamingIt", func(t *testing.T) {
		t.Parallel()
		fields := []string{
			"ConfigDirs.Anchor", "WeaverRun", "WeaverDeps.Runner", "WeaverDeps.Fixer",
			"WeaverDeps.Reed", "WeaverDeps.CommitRecords", "WeaverDeps.RecordsHead",
		}
		for _, field := range fields {
			t.Run(strings.ReplaceAll(field, ".", "_"), func(t *testing.T) {
				t.Parallel()
				env := zeroEnvField(newTestEnv(t), field)
				producer, err := weaverEntry(RowIdentity{Name: "Weaver"}, Config{}, env)
				assertErrContains(t, err, field)
				if producer != nil {
					t.Errorf("weaverEntry() producer = %v; want nil", producer)
				}
			})
		}
	})

	t.Run("UnknownKeyIsRefused", func(t *testing.T) {
		t.Parallel()
		_, err := weaverEntry(RowIdentity{Name: "Weaver"}, Config{"bogus": true}, newTestEnv(t))
		assertErrContains(t, err, `"bogus"`)
	})

	t.Run("UnusableLadderIsRefusedNamingTheConfigCommand", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			config string
		}{
			{"EmptyLadder", "ladder: []\n"},
			{"UnknownAlias", "ladder:\n  - nosuchalias[high]\n"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				env := newTestEnv(t)
				writeWeaverConfig(t, env.ConfigDirs.Anchor, tt.config)
				producer, err := weaverEntry(RowIdentity{Name: "Weaver"}, Config{}, env)
				assertErrContains(t, err, "lym config weaver")
				if producer != nil {
					t.Errorf("weaverEntry() producer = %v; want nil", producer)
				}
			})
		}
	})

	t.Run("GatesEntryReachesTheRunDeps", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		env.ImplementerSeam = weaverengine.ActiveDeps{AnchorRoot: t.TempDir()}
		var received []weaverengine.RunDeps
		env.WeaverRun = func(deps weaverengine.RunDeps) (weaverengine.RunResult, error) {
			received = append(received, deps)
			return weaverengine.RunResult{Outcome: "done"}, nil
		}
		producer, err := weaverEntry(RowIdentity{Name: "Weaver"}, gatesCfg("drift", 3), env)
		if err != nil {
			t.Fatalf("weaverEntry() error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, producer, shedengine.Done)
		if len(received) != 1 || len(received[0].Gate) != 1 || received[0].Gate[0].Name != "drift" || received[0].Gate[0].Attempts != 3 || received[0].Gate[0].Gate == nil {
			t.Errorf("Weaver run was handed gates %+v; want the one drift entry with 3 attempts and its closure", received)
		}
		if len(received) == 1 && len(received[0].Ladder) == 0 {
			t.Error("Weaver run was handed an empty ladder; want the resolved weaver.yaml ladder")
		}
	})
}
