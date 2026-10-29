//go:build tier0

// color_test.go covers the beam and worktree colour pickers, the sibling scan over the hub's settings files, and the luminance text colour.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

func TestPaletteStartsWithThePrimeGreen(t *testing.T) {
	t.Parallel()
	if palette[0] != PrimeColor || PrimeColor != "#1b5e20" {
		t.Errorf("palette[0] = %s, PrimeColor = %s; want both #1b5e20", palette[0], PrimeColor)
	}
}

func TestWorkspaceColorIsNoPaletteColor(t *testing.T) {
	t.Parallel()
	if slices.Contains(palette, WorkspaceColor) {
		t.Errorf("palette %v holds WorkspaceColor %s; want it outside the palette, so no worktree picks it", palette, WorkspaceColor)
	}
}

func TestPickBeamColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		held []string
		want string
	}{
		{"NoneHeld", nil, palette[1]},
		{"SkipsHeldCaseInsensitively", []string{"#7D2D6B"}, palette[2]},
		{"GreenHeldChangesNothing", []string{PrimeColor}, palette[1]},
		{"AllHeldTakesFirstNonGreen", palette[1:], palette[1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PickBeamColor(tt.held); got != tt.want {
				t.Errorf("PickBeamColor(%v) = %s; want %s", tt.held, got, tt.want)
			}
		})
	}
}

func TestPickWorktreeColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		project  string
		current  string
		siblings []string
		want     string
	}{
		{"UnderPrimeNeverGreen", PrimeColor, "", nil, palette[1]},
		{"UnderBeamSkipsBeamColor", palette[1], "", nil, PrimeColor},
		{"UnderBeamSkipsSiblings", palette[1], "", []string{PrimeColor, palette[2]}, palette[3]},
		{"KeepsAllowedFreeCurrent", PrimeColor, "#2D6B6B", []string{palette[1]}, palette[5]},
		{"DropsCurrentEqualToProject", palette[3], palette[3], nil, PrimeColor},
		{"DropsCurrentHeldBySibling", PrimeColor, palette[2], []string{palette[2]}, palette[1]},
		{"DropsCurrentOutsidePalette", PrimeColor, "#2d7d46", nil, palette[1]},
		{"AllTakenTakesFirstOtherThanProject", PrimeColor, "", palette, palette[1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PickWorktreeColor(tt.project, tt.current, tt.siblings); got != tt.want {
				t.Errorf("PickWorktreeColor(%s, %s, %v) = %s; want %s", tt.project, tt.current, tt.siblings, got, tt.want)
			}
		})
	}
}

// writeSettingsFile writes content as <hub>/<name>/<anchor>/.vscode/settings.json.
func writeSettingsFile(t *testing.T, hub, name, anchor, content string) {
	t.Helper()
	dir := filepath.Join(hub, name, anchor, ".vscode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// barSettings returns settings.json content holding title and activity as the title-bar and activity-bar backgrounds.
func barSettings(t *testing.T, title, activity string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{colorCustomizationsKey: map[string]any{titleBarActiveBackground: title, activityBarBackground: activity}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSiblingWorktreeColorsTakesSameProjectActivityColors(t *testing.T) {
	t.Parallel()
	hub := t.TempDir()
	const anchor = "sub"
	writeSettingsFile(t, hub, "prime", anchor, barSettings(t, PrimeColor, PrimeColor))
	writeSettingsFile(t, hub, "sibling", anchor, "{\n  // JSONC\n  "+barSettings(t, PrimeColor, "#7D2D6B")[1:])
	writeSettingsFile(t, hub, "self", anchor, barSettings(t, PrimeColor, palette[2]))
	writeSettingsFile(t, hub, "beam-member", anchor, barSettings(t, palette[3], palette[4]))
	writeSettingsFile(t, hub, "old-style", anchor, `{"workbench.colorCustomizations":{"titleBar.activeBackground":"#1b5e20"}}`)
	writeSettingsFile(t, hub, "broken", anchor, "{not json")
	if err := os.MkdirAll(filepath.Join(hub, "no-settings"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := SiblingWorktreeColors(&lymcwd.Location{HubPath: hub, AnchorRel: anchor}, PrimeColor, "self")
	slices.Sort(got)
	want := []string{PrimeColor, palette[1]}
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SiblingWorktreeColors = %v; want %v", got, want)
	}
}

func TestActivityBarColor(t *testing.T) {
	t.Parallel()
	hub := t.TempDir()
	writeSettingsFile(t, hub, "w", ".", barSettings(t, PrimeColor, "#7D2D6B"))
	if got := ActivityBarColor(filepath.Join(hub, "w")); got != palette[1] {
		t.Errorf("ActivityBarColor = %q; want %q", got, palette[1])
	}
	if got := ActivityBarColor(filepath.Join(hub, "absent")); got != "" {
		t.Errorf("ActivityBarColor of a missing file = %q; want empty", got)
	}
}

func TestForeground(t *testing.T) {
	t.Parallel()
	tests := []struct {
		background string
		want       string
	}{
		{"#ffffff", "#000000"},
		{"#ffeb3b", "#000000"},
		{"#000000", "#ffffff"},
		{"#808080", "#000000"},
		{"#757575", "#ffffff"},
		{"not a colour", "#ffffff"},
	}
	for _, color := range palette {
		tests = append(tests, struct {
			background string
			want       string
		}{color, "#ffffff"})
	}
	for _, tt := range tests {
		if got := Foreground(tt.background); got != tt.want {
			t.Errorf("Foreground(%s) = %s; want %s", tt.background, got, tt.want)
		}
	}
}
