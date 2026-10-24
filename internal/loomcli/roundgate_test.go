// roundgate_test.go covers the told gate-list source the verify gate reads at each arrival.

package loomcli

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

func TestGateListSource(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	hub := loc.HubPath
	// The hub's board directory exists from the start, as in a real hub, so the config dirs the source reads name it before the gate.yaml does.
	if err := os.MkdirAll(fabricengine.BoardDir(hub), 0o755); err != nil {
		t.Fatal(err)
	}
	dirs := fabricengine.ConfigDirs(loc)

	if _, err := gateListSource(dirs, "landing")(); !errors.Is(err, gateslot.ErrConfigAbsent) {
		t.Errorf("gateListSource before any gate.yaml exists = %v; want an error wrapping gateslot.ErrConfigAbsent", err)
	}

	writeHubGateConfig(t, hub, gateConfigWithTwoTiers())
	tests := []struct {
		name      string
		gate      string
		wantTiers []int
		wantCall  string
	}{
		{"Landing", "landing", []int{0, 1}, "lyx gate test --wait --tier 0 --tier 1 --base abc"},
		{"Round", "round", []int{0}, "lyx gate test --wait --tier 0 --base abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := gateListSource(dirs, tt.gate)()
			if err != nil {
				t.Fatalf("gateListSource(%q)() error = %v; want nil", tt.gate, err)
			}
			if !slices.Equal(got.Tiers, tt.wantTiers) {
				t.Errorf("gateListSource(%q)().Tiers = %v; want %v", tt.gate, got.Tiers, tt.wantTiers)
			}
			if call := got.Render("--base", "abc"); call != tt.wantCall {
				t.Errorf("gateListSource(%q)().Render(--base abc) = %q; want %q", tt.gate, call, tt.wantCall)
			}
		})
	}

	_, err := gateListSource(dirs, "nosuch")()
	if err == nil || !strings.Contains(err.Error(), "lyx config gate") {
		t.Errorf("gateListSource(nosuch)() error = %v; want a refusal naming lyx config gate", err)
	}
}
