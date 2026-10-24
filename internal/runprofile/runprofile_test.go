//go:build tier0

// runprofile_test.go table-drives Load and Lookup against hub files under t.TempDir.

package runprofile_test

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// boardWithProfiles returns a board dir whose profiles.yaml holds contents.
func boardWithProfiles(t *testing.T, contents string) string {
	t.Helper()
	board := t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configengine.ConfigFile(board, runprofile.ModuleName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return board
}

// TestLoad pins what Load reads: the hub file as it stands, never filled from the template, the template when no file is present, and a refusal naming the profile for a wrong shape.
func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		contents    *string
		wantNames   []string
		wantDarn    map[string]any
		wantRefusal []string
	}{
		{
			name:      "no file reads the template's starting profiles",
			wantNames: []string{"budget", "docs", "light"},
			wantDarn:  map[string]any{"review_max_bounces": 1, "review_hard_max_bounces": 2},
		},
		{name: "a removed starting profile stays removed", contents: new("light:\n  loom: {plan_max_bounces: 1}\nwide:\n  darn: {review_max_bounces: 2}\n"), wantNames: []string{"light", "wide"}},
		{name: "a comments-only file holds no profile", contents: new("# no profiles\n"), wantNames: []string{}},
		{name: "a profile that is not a mapping is refused", contents: new("light: 3\n"), wantRefusal: []string{`profile "light"`, "mapping of module name"}},
		{name: "a module that is not a mapping is refused", contents: new("light:\n  loom: 3\n"), wantRefusal: []string{`profile "light"`, `module "loom"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{}
			if tt.contents != nil {
				dirs.Board = boardWithProfiles(t, *tt.contents)
			}
			profiles, err := runprofile.Load(dirs)
			if tt.wantRefusal != nil {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("Load() error = %v; want one marked invalid", err)
				}
				for _, want := range tt.wantRefusal {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Load() error = %q; want it to name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v; want none", err)
			}
			if got := profiles.Names(); !slices.Equal(got, tt.wantNames) {
				t.Errorf("Load() names = %v; want %v", got, tt.wantNames)
			}
			if tt.wantDarn != nil {
				if got := profiles["light"]["darn"]; !reflect.DeepEqual(got, tt.wantDarn) {
					t.Errorf("Load() light darn = %v; want %v", got, tt.wantDarn)
				}
			}
		})
	}
}

// TestLookup pins that a known profile returns its settings and an unknown one names the known profiles and the way forward.
func TestLookup(t *testing.T) {
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{Board: boardWithProfiles(t, "light:\n  loom: {plan_max_bounces: 1}\n")})
	if err != nil {
		t.Fatal(err)
	}

	settings, err := profiles.Lookup("light")
	want := map[string]any{"loom": map[string]any{"plan_max_bounces": 1}}
	if err != nil || !reflect.DeepEqual(settings, want) {
		t.Errorf("Lookup(light) = %v, %v; want %v", settings, err, want)
	}

	_, err = profiles.Lookup("heavy")
	if err == nil {
		t.Fatal("Lookup(heavy) error = nil; want a refusal")
	}
	for _, want := range []string{`"heavy"`, "known: light", "lyx config profiles --set heavy="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Lookup(heavy) error = %q; want it to name %q", err, want)
		}
	}
}
