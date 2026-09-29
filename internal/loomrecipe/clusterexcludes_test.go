// clusterexcludes_test.go pins the agreement between each Bouncer row's cluster_excludes and its
// partner BurlerRound row's cluster fan.
//
// The rule: a Bouncer whose on_stuck names a BurlerRound row sets cluster_excludes: true exactly
// when that row's profile.cluster-fan is non-empty. The Bouncer's judge is asked for exclude_lenses
// only when the partner round has a fan to trim (see cluster_excludes on bouncerEntry in
// internal/shedrecipe); otherwise BurlerProducer drops the excludes with a WARN. shedbuild.Build
// runs no cross-row analysis, so this test is the only authoring-time guard.
//
// The fix for a failing author: set cluster_excludes: true on the Bouncer row when adding
// profile.cluster-fan to its BurlerRound partner (and remove it again when removing the fan).
//
// It parses the real embedded recipe and reads no worktree, staying untagged and offline.

package loomrecipe

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// clusterExcludesMismatches returns one message per Bouncer row, partnered with a BurlerRound row
// through on_stuck, whose cluster_excludes disagrees with the partner's cluster fan, or whose
// relevant config values have the wrong type.
func clusterExcludesMismatches(r shedbuild.Recipe) []string {
	rows := make(map[string]shedbuild.Row, len(r.Producers))
	for _, row := range r.Producers {
		rows[row.Name] = row
	}

	var msgs []string
	for _, b := range r.Producers {
		if b.Engine != "Bouncer" {
			continue
		}
		partner, ok := rows[b.OnStuck]
		if !ok || partner.Engine != "BurlerRound" {
			continue
		}

		excludes := false
		if v, present := b.Config["cluster_excludes"]; present {
			flag, isBool := v.(bool)
			if !isBool {
				msgs = append(msgs, fmt.Sprintf("Bouncer row %q: cluster_excludes is %T, want bool", b.Name, v))
				continue
			}
			excludes = flag
		}

		fan := ""
		if v, present := partner.Config["profile"]; present {
			profile, isMap := v.(map[string]any)
			if !isMap {
				msgs = append(msgs, fmt.Sprintf("BurlerRound row %q (partner of Bouncer row %q): profile is %T, want map", partner.Name, b.Name, v))
				continue
			}
			if fv, present := profile["cluster-fan"]; present {
				s, isString := fv.(string)
				if !isString {
					msgs = append(msgs, fmt.Sprintf("BurlerRound row %q (partner of Bouncer row %q): profile.cluster-fan is %T, want string", partner.Name, b.Name, fv))
					continue
				}
				fan = s
			}
		}

		if excludes != (fan != "") {
			msgs = append(msgs, fmt.Sprintf("Bouncer row %q has cluster_excludes=%t but its partner BurlerRound row %q has cluster-fan=%q; set cluster_excludes: true exactly when the partner has a fan", b.Name, excludes, partner.Name, fan))
		}
	}
	return msgs
}

// TestShippedRecipe_ClusterExcludesAgreesWithPartnerFan asserts the shipped loom recipe has no
// Bouncer/BurlerRound pair whose cluster_excludes disagrees with the partner's cluster fan.
func TestShippedRecipe_ClusterExcludesAgreesWithPartnerFan(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}
	if msgs := clusterExcludesMismatches(r); len(msgs) != 0 {
		t.Errorf("clusterExcludesMismatches(shipped recipe) = %v; want none", msgs)
	}
}

// TestClusterExcludesMismatches_Cases pins the rule function against hand-authored two-row recipes.
func TestClusterExcludesMismatches_Cases(t *testing.T) {
	recipe := func(bouncerOnStuck, bouncerConfig, burlerConfig string) string {
		return `version: 1
entry: B
terminals: [Z]
producers:
  - name: B
    engine: Bouncer
    on_stuck: ` + bouncerOnStuck + `
    config:
` + bouncerConfig + `
  - name: R
    engine: BurlerRound
    on_stuck: B
    config:
` + burlerConfig + `
  - name: Z
    engine: Other
`
	}
	const (
		excludesTrue = "      cluster_excludes: true"
		excludesStr  = "      cluster_excludes: \"yes\""
		noExcludes   = "      run_subdir: x"
		withFan      = "      profile:\n        cluster-fan: fan-a"
		numericFan   = "      profile:\n        cluster-fan: 3"
		noFan        = "      run_subdir: x"
	)

	cases := []struct {
		name         string
		yaml         string
		wantMessages int
		wantNames    string
	}{
		{"key true, no fan", recipe("R", excludesTrue, noFan), 1, `"B"`},
		{"key absent, fan", recipe("R", noExcludes, withFan), 1, `"B"`},
		{"key true, fan", recipe("R", excludesTrue, withFan), 0, ""},
		{"key absent, no fan", recipe("R", noExcludes, noFan), 0, ""},
		{"on_stuck names non-BurlerRound row", recipe("Z", excludesTrue, noFan), 0, ""},
		{"non-bool cluster_excludes", recipe("R", excludesStr, noFan), 1, `"B"`},
		{"non-string cluster-fan", recipe("R", noExcludes, numericFan), 1, `"B"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := shedbuild.Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("shedbuild.Parse error = %v; want nil", err)
			}
			msgs := clusterExcludesMismatches(r)
			if len(msgs) != tc.wantMessages {
				t.Fatalf("clusterExcludesMismatches = %v (%d messages); want %d", msgs, len(msgs), tc.wantMessages)
			}
			if tc.wantNames != "" && !strings.Contains(msgs[0], tc.wantNames) {
				t.Errorf("message %q does not name the Bouncer row %s", msgs[0], tc.wantNames)
			}
		})
	}
}
