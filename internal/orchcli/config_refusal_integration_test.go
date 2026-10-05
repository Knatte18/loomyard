//go:build integration

// config_refusal_integration_test.go proves the config refusal end to end: an orch.yaml with a non-bypass permission_mode makes `status` and the watcher start fail with the fix named, before any pane is touched.

package orchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestOrchIntegration_PermissionModeRefusal drives `status` and the hidden `watch` verb against a prime whose orch.yaml says permission_mode: prompt.
func TestOrchIntegration_PermissionModeRefusal(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	anchor := h.Location.AnchorPath()

	if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(anchor, "orch"), []byte("permission_mode: prompt\n"), 0o644); err != nil {
		t.Fatalf("write orch.yaml: %v", err)
	}

	for _, verb := range []string{"status", "watch"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			code := RunCLIIn(anchor, &out, []string{verb})

			if code != 1 {
				t.Fatalf("exit code = %d; want 1; output: %s", code, out.String())
			}
			if !strings.Contains(out.String(), "set permission_mode: bypass or remove the key") {
				t.Errorf("refusal = %q; want the fix named", out.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(anchor, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
		t.Error(".lyx/orch was created under the prime; the refusal must land first")
	}
}
