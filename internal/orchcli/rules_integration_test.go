//go:build tier1

// rules_integration_test.go proves `lym orch rules` over a real hub: the draft becomes the orch rules entry of heddle.json in the Bolt checkout, aimed at the orch-prompt target, and an empty draft removes it.

package orchcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

func TestOrchIntegration_RulesStoreInHeddle(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	anchor := h.Location.AnchorPath()
	boltDir := fabricengine.BoltDir(h.Path)
	draft := filepath.Join(t.TempDir(), "draft.md")
	run := func(content string) map[string]any {
		t.Helper()
		if err := os.WriteFile(draft, []byte(content), 0o644); err != nil {
			t.Fatalf("write draft: %v", err)
		}
		var out bytes.Buffer
		if code := RunCLIIn(anchor, &out, []string{"rules", "--from", draft}); code != 0 {
			t.Fatalf("rules exit = %d; want 0; output: %s", code, out.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
		}
		return envelope
	}

	envelope := run("### Rule\n\nDo the thing.\n")
	if envelope["slug"] != orchRulesSlug || envelope["cleared"] != false {
		t.Errorf("envelope = %v; want slug %q and cleared false", envelope, orchRulesSlug)
	}
	store, err := heddleengine.Load(boltDir)
	if err != nil {
		t.Fatalf("load heddle.json: %v", err)
	}
	entry, ok := store.Get(orchRulesSlug)
	if !ok || entry.Text != "### Rule\n\nDo the thing.\n" || entry.Order != orchRulesOrder || !slices.Equal(entry.Targets, []string{heddleengine.OrchPromptTarget}) {
		t.Errorf("orch rules entry = %+v, %v; want the draft aimed at %s with order %d", entry, ok, heddleengine.OrchPromptTarget, orchRulesOrder)
	}

	envelope = run("")
	if envelope["cleared"] != true {
		t.Errorf("envelope = %v; want cleared true", envelope)
	}
	store, err = heddleengine.Load(boltDir)
	if err != nil {
		t.Fatalf("load heddle.json: %v", err)
	}
	if entry, ok := store.Get(orchRulesSlug); ok {
		t.Errorf("orch rules entry = %+v after an empty draft; want none", entry)
	}
}
