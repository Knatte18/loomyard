// secretpaths_wiring_test.go covers the hub matcher that wireHub loads from fabric.yaml.
// record-batch's secret-file guard reads that matcher.

package webstercli

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWire_HubLoadsSecretPathsFromFabricYAML proves hub wiring hands record-batch a loader over the hub's fabric.yaml.
// An added pattern matches beside the shipped ones, and a malformed one fails the load as an invalid config.
// Serial: wire sets the process-global logger sink, so neither the test nor its subtests call t.Parallel.
func TestWire_HubLoadsSecretPathsFromFabricYAML(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		secretPaths string
		wantPattern string
		wantInvalid bool
	}{
		{"AddedPattern", "secret_paths: \"*.key\"\n", "*.key", false},
		{"MalformedPattern", "secret_paths: \"[unclosed\"\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			loc := hubLocation(t, hub, fabricengine.BoltDirName, ".")
			boardDir := fabricengine.BoardDir(hub)
			if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}
			fabricConfig := configengine.ConfigFile(boardDir, "fabric")
			if err := os.WriteFile(fabricConfig, []byte("branch_prefix: \"\"\npathspec: _lym\n"+tt.secretPaths), 0o644); err != nil {
				t.Fatalf("write %s: %v", fabricConfig, err)
			}

			c := &websterCLI{}
			if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}
			if c.secretPaths == nil {
				t.Fatal("secretPaths = nil; want hub wiring to supply the fabric.yaml loader")
			}

			matcher, err := c.secretPaths()
			if tt.wantInvalid {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("secretPaths() error = %v; want errors.Is(err, configengine.ErrInvalid)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("secretPaths() error = %v; want nil", err)
			}
			if pattern, matched := matcher.Match("tls/server.key"); !matched || pattern != tt.wantPattern {
				t.Errorf("Match(tls/server.key) = (%q, %v); want (%q, true)", pattern, matched, tt.wantPattern)
			}
		})
	}
}
