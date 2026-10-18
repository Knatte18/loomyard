//go:build integration

// rules_integration_test.go proves `lyx orch rules` over a real hub: it commits only the hub-wide rules file on Bolt, pushes it, and commits nothing for unchanged content.

package orchcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestOrchIntegration_RulesCommitsOnBolt does not call t.Parallel: it clears FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH with t.Setenv, which is process-global state.
func TestOrchIntegration_RulesCommitsOnBolt(t *testing.T) {
	t.Setenv("FABRIC_SKIP_GIT", "")
	t.Setenv("FABRIC_SKIP_PUSH", "")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	anchor := h.Location.AnchorPath()
	boardDir := h.BoardDir()
	rulesPath := filepath.Join(boardDir, rulesRel())
	draft := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(draft, []byte("### Rule\n\nDo the thing.\n"), 0o644); err != nil {
		t.Fatalf("write draft: %v", err)
	}
	commitCount := func() int { return gitkit.RevListCount(t, boardDir, "HEAD") }

	before := commitCount()
	var out bytes.Buffer
	if code := RunCLIIn(anchor, &out, []string{"rules", "--from", draft}); code != 0 {
		t.Fatalf("rules exit = %d; want 0; output: %s", code, out.String())
	}
	got, err := os.ReadFile(rulesPath)
	if err != nil || string(got) != "### Rule\n\nDo the thing.\n" {
		t.Errorf("board rules file = %q (err %v); want the draft", got, err)
	}
	if after := commitCount(); after != before+1 {
		t.Errorf("board commit count = %d; want %d", after, before+1)
	}
	touched := gitkit.Git(t, boardDir, "show", "--name-only", "--format=", "HEAD")
	if want := filepath.ToSlash(rulesRel()); touched != want {
		t.Errorf("HEAD touches %q; want only %q", touched, want)
	}
	head := gitkit.RevParse(t, boardDir, "HEAD")
	if upstream := gitkit.RevParse(t, boardDir, "@{u}"); upstream != head {
		t.Errorf("@{u} = %s; want HEAD %s", upstream, head)
	}

	out.Reset()
	if code := RunCLIIn(anchor, &out, []string{"rules", "--from", draft}); code != 0 {
		t.Fatalf("second rules exit = %d; want 0; output: %s", code, out.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
	}
	if envelope["committed"] != false || envelope["unpushed"] != false {
		t.Errorf("second envelope = %v; want committed false and unpushed false", envelope)
	}
	if again := gitkit.RevParse(t, boardDir, "HEAD"); again != head {
		t.Errorf("HEAD moved to %s on unchanged content; want %s", again, head)
	}
}
