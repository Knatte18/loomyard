//go:build tier0

// roundgate_test.go covers the told gate-key call source the verify gate and the landing producers read at each arrival.

package loomcli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestGateCallSource(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	hub := loc.HubPath
	// The hub's board directory exists from the start, as in a real hub, so the config dirs the source reads name it before the gate.yaml does.
	if err := os.MkdirAll(fabricengine.BoardDir(hub), 0o755); err != nil {
		t.Fatal(err)
	}
	source := gateCallSource(fabricengine.ConfigDirs(loc))

	if _, err := source("code-burler-fix"); !errors.Is(err, gateslot.ErrConfigAbsent) {
		t.Errorf("gateCallSource(dirs)(code-burler-fix) before any gate.yaml exists = %v; want an error wrapping gateslot.ErrConfigAbsent", err)
	}

	writeHubGateConfig(t, hub, gateConfigWithTwoTiers())
	const wantCall = "lyx gate test --wait --gate code-burler-fix"
	if got, err := source("code-burler-fix"); err != nil || got != wantCall {
		t.Errorf("gateCallSource(dirs)(code-burler-fix) = %q, %v; want %q, nil", got, err, wantCall)
	}

	// An edit between two calls is seen: listing a tier the hub does not declare turns the key into a refusal.
	edited := strings.NewReplacer("  code-burler-fix: {tiers: [0, 1]", "  code-burler-fix: {tiers: [0, 7]").Replace(gateConfigWithTwoTiers())
	writeHubGateConfig(t, hub, edited)
	if got, err := source("code-burler-fix"); err == nil || !strings.Contains(err.Error(), "lyx config gate") {
		t.Errorf("gateCallSource(dirs)(code-burler-fix) after the edit = %q, %v; want a refusal naming lyx config gate", got, err)
	}

	if got, err := source("nosuch"); err == nil || !strings.Contains(err.Error(), "lyx config gate") {
		t.Errorf("gateCallSource(dirs)(nosuch) = %q, %v; want a refusal naming lyx config gate", got, err)
	}
}
