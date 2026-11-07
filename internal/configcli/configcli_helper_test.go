// configcli_helper_test.go holds the fakes and fixture helpers that this package's test files share.
// The package's one test binary holds every tier, so this file serves them all.

package configcli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// fakeSyncTracker records whether the sync function it supplies was called, and the exit code it returns.
type fakeSyncTracker struct {
	called   bool
	exitCode int
}

// syncFunc returns a sync function that marks the tracker as called and returns its exit code.
func (t *fakeSyncTracker) syncFunc() syncFunc {
	return func(w io.Writer) int {
		t.called = true
		return t.exitCode
	}
}

// makeNeverCalledEditor returns an editor that fails the test if it is called.
// Tests of the --print path pass it to prove that path never opens an editor.
func makeNeverCalledEditor(t *testing.T) configengine.EditorFunc {
	t.Helper()
	return func(path string) error {
		t.Helper()
		t.Errorf("editor was called on path %q; --print must never launch the editor", path)
		return nil
	}
}

// makeLayoutAt returns a minimal *lymcwd.Location for the worktree baseDir, anchored at its root.
func makeLayoutAt(baseDir string) *lymcwd.Location {
	return &lymcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "."}
}

// seedModuleConfig writes YAML content to the config file for the named module under baseDir.
func seedModuleConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()
	dir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed config for module %s: %v", module, err)
	}
}

// assertJSONErrContains asserts that output is a JSON error envelope with ok:false.
// When wantSubstr is not empty, the envelope's error field must contain it.
func assertJSONErrContains(t *testing.T, output, wantSubstr string) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v; got %q", err, output)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("JSON envelope ok = true; want false")
	}
	if wantSubstr != "" {
		msg, _ := env["error"].(string)
		if !strings.Contains(msg, wantSubstr) {
			t.Errorf("JSON error field missing %q; got %q", wantSubstr, msg)
		}
	}
}
