// warpexclude_test.go — tier 0 unit tests for AppendExcludePaths over in-memory fabric.yaml bytes; no hub, no git spawn.

package fabricengine_test

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAppendExcludePaths pins the rewrite that adds a path to a hub's own warp exclude list:
// a list lacking the path gains it with its comment and the other keys kept, and one line names it.
// A file leaving the key unset, or already listing the path, comes back unchanged with no line.
// An unparseable document is an ErrInvalid error, and a second run over a result changes nothing.
func TestAppendExcludePaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	rewrite := fabricengine.AppendExcludePaths("new/file.json")
	tests := []struct {
		name string
		yaml string
		// wantList is the list the result declares; empty wants the bytes back unchanged.
		wantList string
		wantErr  bool
	}{
		{name: "KeyUnsetKeepsTheTemplateList", yaml: "branch_prefix: x\n"},
		{name: "ListHoldingThePathIsKept", yaml: "warp_exclude: A.md new/file.json\n"},
		{name: "ListLackingThePathGainsIt", yaml: "branch_prefix: x\nwarp_exclude: A.md .heddle # mine\n", wantList: "A.md .heddle new/file.json"},
		{name: "EmptyListGainsThePath", yaml: "warp_exclude: \"\"\n", wantList: "new/file.json"},
		{name: "UnparseableIsInvalid", yaml: "warp_exclude: [x\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rewritten, rewrote, flagged, err := rewrite([]byte(tt.yaml), "/root", "/root")
			if tt.wantErr {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("rewrite(%q) error = %v; want one marked ErrInvalid", tt.yaml, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("rewrite(%q) = %v", tt.yaml, err)
			}
			if len(flagged) != 0 {
				t.Errorf("rewrite(%q) flagged %v; want none", tt.yaml, flagged)
			}
			if tt.wantList == "" {
				if string(rewritten) != tt.yaml || len(rewrote) != 0 {
					t.Errorf("rewrite(%q) = %q, %v; want the bytes back unchanged and no line", tt.yaml, rewritten, rewrote)
				}
				return
			}
			var cfg fabricengine.Config
			if err := yaml.Unmarshal(rewritten, &cfg); err != nil {
				t.Fatalf("rewritten %q does not decode: %v", rewritten, err)
			}
			if cfg.WarpExclude != tt.wantList {
				t.Errorf("warp_exclude = %q; want %q", cfg.WarpExclude, tt.wantList)
			}
			if strings.Contains(tt.yaml, "branch_prefix") && cfg.BranchPrefix != "x" {
				t.Errorf("branch_prefix = %q; want it kept as x", cfg.BranchPrefix)
			}
			if strings.Contains(tt.yaml, "# mine") && !strings.Contains(string(rewritten), "# mine") {
				t.Errorf("rewritten %q lost the line comment", rewritten)
			}
			if len(rewrote) != 1 || !strings.Contains(rewrote[0], "new/file.json") {
				t.Errorf("rewrote = %v; want one line naming new/file.json", rewrote)
			}
			again, rewroteAgain, _, err := rewrite(rewritten, "/root", "/root")
			if err != nil || string(again) != string(rewritten) || len(rewroteAgain) != 0 {
				t.Errorf("second rewrite = %q, %v, %v; want the bytes back unchanged and no line", again, rewroteAgain, err)
			}
		})
	}
}
