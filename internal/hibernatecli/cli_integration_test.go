//go:build integration && linux

// cli_integration_test.go drives `lyx hibernate status`, `cancel`, `request` and `watch` through the cobra commands over a hubforge hub with one pair, and the operator index the root renders.
// The test calls no t.Parallel: it sets the strand-name variable.

package hibernatecli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

const pairSlug = hubforge.TemplatePairSlug

// verb runs a hibernate-module command in cwd and decodes its envelope.
func verb(t *testing.T, cwd string, args ...string) (envelope.Envelope, int) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), cwd, &out, args)
	return envelope.Decode(t, out.String()), code
}

func TestHibernateVerbs_WireTheHub(t *testing.T) {
	t.Setenv(agentname.StrandNameEnv, "")
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	prime := h.PrimeWorktree()
	hibernateDir := filepath.Join(fabricengine.HubScratchDir(h.Path), lyxdirs.HibernateDirName)
	requestPath := filepath.Join(hibernateDir, requestFileName)

	status, code := verb(t, h.PairCodeWorktree(pairSlug), "status")
	if !status.OK || code != 0 {
		t.Fatalf("status: ok=%v code=%d error=%q", status.OK, code, status.Error)
	}
	body, _ := status.Raw["status"].(map[string]any)
	if request, present := body["request"]; !present || request != nil {
		t.Errorf("status request = %v; want null", body["request"])
	}
	if _, err := os.Stat(hibernateDir); !os.IsNotExist(err) {
		t.Errorf("status left the hibernate directory behind: stat err = %v", err)
	}

	cancel, code := verb(t, prime, "cancel")
	if !cancel.OK || code != 0 {
		t.Fatalf("cancel: ok=%v code=%d error=%q", cancel.OK, code, cancel.Error)
	}
	if cancelled, _ := cancel.Raw["cancel"].(map[string]any); cancelled["cancelled"] != false {
		t.Errorf("cancel = %v; want cancelled false", cancel.Raw["cancel"])
	}
	if _, err := os.Stat(requestPath); !os.IsNotExist(err) {
		t.Errorf("cancel wrote a request file: stat err = %v", err)
	}

	agent, err := agentname.Format("hub", pairSlug, "implementer")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(agentname.StrandNameEnv, agent)
	refused, code := verb(t, prime, "request")
	if refused.OK || code == 0 || !strings.Contains(refused.Error, "parent session") {
		t.Errorf("request by an agent: ok=%v error=%q; want a refusal naming the parent session", refused.OK, refused.Error)
	}
	if _, err := os.Stat(requestPath); !os.IsNotExist(err) {
		t.Errorf("a refused request wrote a request file: stat err = %v", err)
	}
	t.Setenv(agentname.StrandNameEnv, "")

	watch, code := verb(t, prime, "watch")
	if !watch.OK || code != 0 {
		t.Fatalf("watch: ok=%v code=%d error=%q", watch.OK, code, watch.Error)
	}
	if _, err := os.Stat(requestPath); !os.IsNotExist(err) {
		t.Errorf("watch with no request wrote a request file: stat err = %v", err)
	}

	root := &cobra.Command{Use: "lyx"}
	root.AddCommand(Command())
	index := clihelp.RenderIndex(root, clihelp.AudienceOperator)
	for _, line := range []string{"- `request`", "- `cancel`", "- `status`"} {
		if !strings.Contains(index, line) {
			t.Errorf("operator index lacks a %q line:\n%s", line, index)
		}
	}
	if strings.Contains(index, "`watch") {
		t.Errorf("operator index lists the hidden watch verb:\n%s", index)
	}
}
