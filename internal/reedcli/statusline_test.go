// statusline_test.go covers the `statusline` verb's pure command construction (Use, Short) and its
// enveloped RunE, which calls c.eng.StatusLineText() and returns the rendered text on the JSON
// envelope under the "text" key.
// It never drives the verb through RunCLI: that reaches reed's PersistentPreRunE and therefore
// lyxcwd.Resolve, which spawns "git rev-parse", banned in the untagged suite by the Test Tier
// Purity Invariant. The end-to-end PreRunE -> StatusLineText round trip is covered by the reed smoke
// suite instead.

package reedcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestStatuslineCmd_UseAndShort(t *testing.T) {
	c := &reedCLI{}
	cmd := c.statuslineCmd()

	if cmd.Use != "statusline" {
		t.Errorf("statuslineCmd().Use = %q; want %q", cmd.Use, "statusline")
	}
	if cmd.Short == "" {
		t.Error("statuslineCmd().Short is empty; want a non-empty short description")
	}
}

// newStatuslineTestCLI builds a reedCLI whose Engine is real enough to run statuslineCmd's RunE:
// StatusLineText dereferences e.cfg unconditionally, so the bare &reedCLI{} shape
// TestStatuslineCmd_UseAndShort uses would panic here. An empty Config.StatusLine.Template falls
// back to the embedded default template, and RepoName/HubPath are the only two Geometry fields
// tokenvocab.Ctx consumes, so this renders cleanly with no filesystem or process I/O.
func newStatuslineTestCLI(t *testing.T) *reedCLI {
	t.Helper()
	return &reedCLI{eng: reedengine.New(reedengine.Config{}, reedengine.Geometry{RepoName: "test-repo", WorktreeName: "test-worktree", HubPath: t.TempDir()})}
}

func TestStatuslineCmd_ReturnsRenderedTextOnEnvelope(t *testing.T) {
	c := newStatuslineTestCLI(t)
	cmd := c.statuslineCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() = %v; want nil", err)
	}
	if !strings.Contains(buf.String(), `"text"`) {
		t.Errorf("statusline output = %q; want the JSON envelope with a \"text\" field", buf.String())
	}
}
