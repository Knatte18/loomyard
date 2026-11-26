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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// TestLoad pins what Load reads: the hub file as it stands, never filled from the template;
// the template when no file is present;
// a refusal naming the entry for a wrong shape or a default that names no fitting profile.
func TestLoad(t *testing.T) {
	tiergate.Require(t, 0)
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
			wantDefaults: map[string]string{"tapestry": "solid", "motif": "solid", "darn": "light", "pawl": "light", "raddle": "light", "arras": "solid"},
		},
		{
			name:         "a removed starting profile stays removed",
			contents:     new("light:\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\nbroad:\n  darn:\n    loom: {code: {max_bounces: 2}}\n"),
			wantNames:    []string{"broad", "light"},
			wantDefaults: map[string]string{},
		},
		{name: "a comments-only file holds no profile", contents: new("# no profiles\n"), wantNames: []string{}, wantDefaults: map[string]string{}},
		{name: "a profile that is not a mapping is refused", contents: new("light: 3\n"), wantRefusal: []string{`profile "light"`, "mapping of recipe name", "way forward"}},
		{name: "a recipe block that is not a mapping is refused", contents: new("light:\n  tapestry: 3\n"), wantRefusal: []string{`profile "light"`, `recipe "tapestry"`}},
		{name: "a profile in the old module-to-keys shape is refused", contents: new("light:\n  tapestry: {max_bounces: 1}\n"), wantRefusal: []string{`profile "light"`, `recipe "tapestry"`, `module "max_bounces"`, "lym config profiles"}},
		{name: "defaults that are not a mapping are refused", contents: new("defaults: solid\n"), wantRefusal: []string{"defaults must be a mapping", "way forward"}},
		{name: "a profile named defaults is refused", contents: new("defaults:\n  tapestry:\n    tapestry: {plan: {max_bounces: 1}}\n"), wantRefusal: []string{`recipe "tapestry" must name a profile`}},
		{name: "a default naming an unknown profile is refused", contents: new("defaults: {tapestry: heavy}\n"), wantRefusal: []string{`recipe "tapestry"`, `unknown profile "heavy"`}},
		{name: "a default naming a profile without its recipe's block is refused", contents: new("defaults: {tapestry: docs}\ndocs:\n  darn:\n    darn: {writer: opus}\n"), wantRefusal: []string{`recipe "tapestry"`, `profile "docs"`, "no tapestry block"}},
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

// TestTemplateLadder pins the template's ladder: each profile's review rounds and hard caps per perch for loom;
// the Code perch's for darn, every hard cap its rounds plus one.
// It also pins that each profile's sampler block carries its darn block's rounds;
// sampler.code_review is on from light up, and sampler has no default profile.
// Only the darn block sets the Code-Review reviewer, to opus[medium].
// Each profile's motif block holds its tapestry block's Discussion perch and nothing else.
// Each profile's arras block holds that Discussion perch's rounds and the split perch's alike, with a hard cap one above.
//
//testtiming:keep pins the template's per-block code.reviewer_model values, which the lookup tests over a fixture file cannot see
func TestTemplateLadder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{})
	if err != nil {
		t.Fatal(err)
	}
	perch := func(rounds int) map[string]int {
		return map[string]int{"max": rounds, "hard": rounds + 1}
	}
	if got, err := profiles.Default("pawl"); got != "light" || err != nil {
		t.Errorf("Default(pawl) = %q, %v; want light", got, err)
	}
	if got, err := profiles.Default("motif"); got != "solid" || err != nil {
		t.Errorf("Default(motif) = %q, %v; want solid", got, err)
	}
	if got, err := profiles.Default("sampler"); got != "" || err != nil {
		t.Errorf("Default(sampler) = %q, %v; want none, so a sampler task naming no profile runs on the hub's config", got, err)
	}
	tests := []struct {
		profile                string
		discussion, plan, code int
		darnCode               int
		solo                   []string
	}{
		{"speed", 0, 0, 1, 0, []string{"tapestry"}},
		{"light", 1, 1, 1, 1, nil},
		{"solid", 2, 2, 1, 2, nil},
		{"wide", 3, 2, 1, 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			t.Parallel()
			tapestry, err := profiles.Settings(tt.profile, "tapestry")
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
				module := "loom"
				if segment == "plan" {
					module = "tapestry"
				}
				got[segment] = areaRounds(tapestry[module], segment)
			}
			darnKeys := darn["loom"].(map[string]any)
			got["darn code"] = areaRounds(darnKeys, "code")
			pawl, err := profiles.Settings(tt.profile, "pawl")
			if err != nil {
				t.Fatal(err)
			}
			pawlKeys := pawl["loom"].(map[string]any)
			got["pawl code"] = areaRounds(pawlKeys, "code")
			want["pawl code"] = perch(tt.darnCode)
			sampler, err := profiles.Settings(tt.profile, "sampler")
			if err != nil {
				t.Fatal(err)
			}
			samplerLoom := sampler["loom"].(map[string]any)
			got["sampler code"] = areaRounds(samplerLoom, "code")
			want["sampler code"] = perch(tt.darnCode)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Settings(%s) rounds = %v; want %v", tt.profile, got, want)
			}
			motif, err := profiles.Settings(tt.profile, "motif")
			if err != nil {
				t.Fatal(err)
			}
			caps := func() map[string]any {
				return map[string]any{"max_bounces": tt.discussion, "hard_max_bounces": tt.discussion + 1}
			}
			wantMotif := map[string]any{"loom": map[string]any{"discussion": caps()}}
			if !reflect.DeepEqual(motif, wantMotif) {
				t.Errorf("Settings(%s, motif) = %v; want %v", tt.profile, motif, wantMotif)
			}
			arras, err := profiles.Settings(tt.profile, "arras")
			if err != nil {
				t.Fatal(err)
			}
			wantArras := map[string]any{"loom": map[string]any{"discussion": caps(), "split": caps()}}
			if !reflect.DeepEqual(arras, wantArras) {
				t.Errorf("Settings(%s, arras) = %v; want %v", tt.profile, arras, wantArras)
			}
			wantReview := tt.profile != "speed"
			samplerKeys, _ := sampler["sampler"].(map[string]any)
			if wantReview && samplerKeys["code_review"] != true {
				t.Errorf("Settings(%s, sampler) sampler.code_review = %v; want true", tt.profile, samplerKeys["code_review"])
			}
			if !wantReview && sampler["sampler"] != nil {
				t.Errorf("Settings(%s, sampler) sampler = %v; want no sampler key, leaving code_review false", tt.profile, sampler["sampler"])
			}
			blockKeys := map[string]map[string]any{"tapestry": tapestry["loom"].(map[string]any), "darn": darnKeys, "pawl": pawlKeys, "sampler": samplerLoom}
			for block, keys := range blockKeys {
				code := keys["code"].(map[string]any)
				fan, present := code["reviewer_fan"]
				if wantSolo := slices.Contains(tt.solo, block); present != wantSolo || (present && fan != "") {
					t.Errorf("Settings(%s, %s) code.reviewer_fan = %v, present %v; want %q present %v", tt.profile, block, fan, present, "", wantSolo)
				}
				model, present := code["reviewer_model"]
				if wantOpus := block == "darn"; present != wantOpus || (present && model != "opus[medium]") {
					t.Errorf("Settings(%s, %s) code.reviewer_model = %v, present %v; want %q present %v", tt.profile, block, model, present, "opus[medium]", wantOpus)
				}
			}
		})
	}
}

// areaRounds returns the bounce budget and hard cap that the area of a block's module settings holds.
func areaRounds(settings any, area string) map[string]int {
	keys := settings.(map[string]any)[area].(map[string]any)
	return map[string]int{"max": keys["max_bounces"].(int), "hard": keys["hard_max_bounces"].(int)}
}

// TestTemplateDocsProfile pins the template's darn-only docs profile:
// a Sonnet medium writer that commits its report under docs/research.
// It has one solo Code round with its hard cap and an Opus medium reviewer, and drift scope off.
// Drift scope is off since a docs task changes no captured value.
//
//testtiming:keep pins the docs darn block's full settings map, including its code.reviewer_model, which the lookup tests over a fixture file cannot see
func TestTemplateDocsProfile(t *testing.T) {
	tiergate.Require(t, 0)
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
		"darn":  map[string]any{"writer": "sonnet[medium]", "report_dir": "docs/research"},
		"drift": map[string]any{"scope": "off"},
		"loom":  map[string]any{"code": map[string]any{"max_bounces": 1, "hard_max_bounces": 2, "reviewer_fan": "", "reviewer_model": "opus[medium]"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Settings(docs, darn) = %v; want %v", got, want)
	}
	if _, err := profiles.Settings("docs", "tapestry"); err == nil {
		t.Error("Settings(docs, tapestry) error = nil; want a refusal for the missing tapestry block")
	}
}

// TestFormerRecipeNameRefusal pins that Settings and Default refuse a recipe held only under its former name.
// A profile or the defaults may hold such a recipe; the refusal names the hand rename as the way forward.
func TestFormerRecipeNameRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	profiles, err := runprofile.Load(configengine.Dirs{Board: boardWithProfiles(t, "defaults: {loom: light}\nlight:\n  loom:\n    tapestry: {plan: {max_bounces: 1}}\n")})
	if err != nil {
		t.Fatal(err)
	}
	const wayForward = `lym config profiles`

	_, err = profiles.Settings("light", "tapestry")
	for _, want := range []string{`"light"`, "no tapestry block but a loom block", wayForward} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Settings(light, tapestry) error = %v; want it to name %q", err, want)
		}
	}

	name, err := profiles.Default("tapestry")
	for _, want := range []string{"no tapestry entry but a loom entry", wayForward} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Default(tapestry) = %q, %v; want an error naming %q", name, err, want)
		}
	}
	if name, err := profiles.Default("darn"); name != "" || err != nil {
		t.Errorf("Default(darn) = %q, %v; want empty and no error for a recipe with no default under any name", name, err)
	}
}

// TestSettings pins the lookups of a loaded file: a profile's block for a recipe, and a recipe's default.
// It also pins a refusal naming the way forward for an unknown profile or a missing recipe block.
func TestSettings(t *testing.T) {
	tiergate.Require(t, 0)
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
		{"a profile without the recipe's block", "docs", "tapestry", []string{`"docs"`, "no tapestry block", "covers: darn", "way forward"}},
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

	if got, err := profiles.Default("darn"); got != "docs" || err != nil {
		t.Errorf("Default(darn) = %q, %v; want docs", got, err)
	}
	if got, err := profiles.Default("tapestry"); got != "" || err != nil {
		t.Errorf("Default(tapestry) = %q, %v; want empty", got, err)
	}
}
