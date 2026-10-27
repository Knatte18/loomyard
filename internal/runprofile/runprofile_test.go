//go:build tier0

// runprofile_test.go table-drives Load, Lookup, Settings and Default against hub files under t.TempDir.

package runprofile_test

import (
	"errors"
	"maps"
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

// TestLoad pins what Load reads: the hub file as it stands, never filled from the template, the template when no file is present, and a refusal naming the entry for a wrong shape or a default that names no fitting profile.
func TestLoad(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		contents     *string
		wantNames    []string
		wantDefaults map[string]string
		wantRefusal  []string
	}{
		{
			name:         "no file reads the template's ladder, docs and defaults",
			wantNames:    []string{"docs", "light", "solid", "speed", "wide"},
			wantDefaults: map[string]string{"loom": "solid", "darn": "light", "pawl": "light"},
		},
		{
			name:         "a removed starting profile stays removed",
			contents:     new("light:\n  loom:\n    loom: {plan_max_bounces: 1}\nbroad:\n  darn:\n    loom: {code_max_bounces: 2}\n"),
			wantNames:    []string{"broad", "light"},
			wantDefaults: map[string]string{},
		},
		{name: "a comments-only file holds no profile", contents: new("# no profiles\n"), wantNames: []string{}, wantDefaults: map[string]string{}},
		{name: "a profile that is not a mapping is refused", contents: new("light: 3\n"), wantRefusal: []string{`profile "light"`, "mapping of recipe name", "way forward"}},
		{name: "a recipe block that is not a mapping is refused", contents: new("light:\n  loom: 3\n"), wantRefusal: []string{`profile "light"`, `recipe "loom"`}},
		{name: "a profile in the old module-to-keys shape is refused", contents: new("light:\n  loom: {plan_max_bounces: 1}\n"), wantRefusal: []string{`profile "light"`, `recipe "loom"`, `module "plan_max_bounces"`, "lym config profiles"}},
		{name: "defaults that are not a mapping are refused", contents: new("defaults: solid\n"), wantRefusal: []string{"defaults must be a mapping", "way forward"}},
		{name: "a profile named defaults is refused", contents: new("defaults:\n  loom:\n    loom: {plan_max_bounces: 1}\n"), wantRefusal: []string{`recipe "loom" must name a profile`}},
		{name: "a default naming an unknown profile is refused", contents: new("defaults: {loom: heavy}\n"), wantRefusal: []string{`recipe "loom"`, `unknown profile "heavy"`}},
		{name: "a default naming a profile without its recipe's block is refused", contents: new("defaults: {loom: docs}\ndocs:\n  darn:\n    darn: {writer: opus}\n"), wantRefusal: []string{`recipe "loom"`, `profile "docs"`, "no loom block"}},
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
			if !maps.Equal(profiles.Defaults, tt.wantDefaults) {
				t.Errorf("Load() defaults = %v; want %v", profiles.Defaults, tt.wantDefaults)
			}
		})
	}
}

// TestTemplateLadder pins the template's ladder: each profile's review rounds and hard caps per perch for loom, and the Code perch's for darn, every hard cap its rounds plus one.
func TestTemplateLadder(t *testing.T) {
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{})
	if err != nil {
		t.Fatal(err)
	}
	perch := func(rounds int) map[string]int {
		return map[string]int{"max": rounds, "hard": rounds + 1}
	}
	if got := profiles.Default("pawl"); got != "light" {
		t.Errorf("Default(pawl) = %q; want light", got)
	}
	tests := []struct {
		profile                string
		discussion, plan, code int
		darnCode               int
	}{
		{"speed", 0, 0, 1, 0},
		{"light", 1, 1, 1, 1},
		{"solid", 2, 2, 1, 2},
		{"wide", 3, 2, 1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			t.Parallel()
			loom, err := profiles.Settings(tt.profile, "loom")
			if err != nil {
				t.Fatal(err)
			}
			darn, err := profiles.Settings(tt.profile, "darn")
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]map[string]int{}
			want := map[string]map[string]int{"discussion": perch(tt.discussion), "plan": perch(tt.plan), "code": perch(tt.code), "darn code": perch(tt.darnCode)}
			for _, segment := range []string{"discussion", "plan", "code"} {
				keys := loom["loom"].(map[string]any)
				got[segment] = map[string]int{"max": keys[segment+"_max_bounces"].(int), "hard": keys[segment+"_hard_max_bounces"].(int)}
			}
			darnKeys := darn["loom"].(map[string]any)
			got["darn code"] = map[string]int{"max": darnKeys["code_max_bounces"].(int), "hard": darnKeys["code_hard_max_bounces"].(int)}
			pawl, err := profiles.Settings(tt.profile, "pawl")
			if err != nil {
				t.Fatal(err)
			}
			pawlKeys := pawl["loom"].(map[string]any)
			got["pawl code"] = map[string]int{"max": pawlKeys["code_max_bounces"].(int), "hard": pawlKeys["code_hard_max_bounces"].(int)}
			want["pawl code"] = perch(tt.darnCode)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Settings(%s) rounds = %v; want %v", tt.profile, got, want)
			}
		})
	}
}

// TestTemplateDocsProfile pins the template's darn-only docs profile: a Sonnet medium writer, one Code round with its hard cap, and no loom block.
func TestTemplateDocsProfile(t *testing.T) {
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := profiles.Settings("docs", "darn")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"darn": map[string]any{"writer": "sonnet[medium]"},
		"loom": map[string]any{"code_max_bounces": 1, "code_hard_max_bounces": 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Settings(docs, darn) = %v; want %v", got, want)
	}
	if _, err := profiles.Settings("docs", "loom"); err == nil {
		t.Error("Settings(docs, loom) error = nil; want a refusal for the missing loom block")
	}
}

// TestSettings pins the lookups of a loaded file: a profile's block for a recipe, a refusal naming the way forward for an unknown profile or a missing recipe block, and a recipe's default.
func TestSettings(t *testing.T) {
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{Board: boardWithProfiles(t, "defaults: {darn: docs}\ndocs:\n  darn:\n    darn: {writer: opus}\n")})
	if err != nil {
		t.Fatal(err)
	}

	settings, err := profiles.Settings("docs", "darn")
	want := map[string]any{"darn": map[string]any{"writer": "opus"}}
	if err != nil || !reflect.DeepEqual(settings, want) {
		t.Errorf("Settings(docs, darn) = %v, %v; want %v", settings, err, want)
	}

	refusals := []struct {
		name, profile, recipe string
		want                  []string
	}{
		{"an unknown profile", "heavy", "darn", []string{`"heavy"`, "known: docs", "lym config profiles --set heavy="}},
		{"a profile without the recipe's block", "docs", "loom", []string{`"docs"`, "no loom block", "covers: darn", "way forward"}},
	}
	for _, tt := range refusals {
		if _, err := profiles.Settings(tt.profile, tt.recipe); err == nil {
			t.Errorf("Settings(%s, %s) error = nil; want a refusal (%s)", tt.profile, tt.recipe, tt.name)
		} else {
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Settings(%s, %s) error = %q; want it to name %q", tt.profile, tt.recipe, err, want)
				}
			}
		}
	}

	if got := profiles.Default("darn"); got != "docs" {
		t.Errorf("Default(darn) = %q; want docs", got)
	}
	if got := profiles.Default("loom"); got != "" {
		t.Errorf("Default(loom) = %q; want empty", got)
	}
}
