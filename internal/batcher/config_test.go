// config_test.go verifies batcher.yaml's template parses and Active resolves the configured profile, including its two degrading-absence cases (absent _lyx/, absent batcher.yaml) and its load-error paths, seeded via plain os.MkdirAll/os.WriteFile against a t.TempDir() rather than gitkit's config fixture-copy helpers: configengine.LoadOrTemplate only requires a filesystem _lyx/config/<module>.yaml, no git repository, so this test stays untagged and spawn-free.
// Tier-1 (pure logic, no git, no TestMain), per the go-test-tiers-and-hermetic-git Shared Decision.

package batcher_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// assertRetiredKey fails unless err wraps ErrRetiredKey and ends in the reconcile way forward exactly when want is set.
func assertRetiredKey(t *testing.T, err error, want bool) {
	t.Helper()
	if got := errors.Is(err, batcher.ErrRetiredKey); got != want {
		t.Errorf("error %q wraps ErrRetiredKey = %v; want %v", err.Error(), got, want)
	}
	if want {
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("retired-key error %q is not marked configengine.ErrInvalid", err.Error())
		}
		if !strings.Contains(err.Error(), `way forward: run "lyx config reconcile --apply"`) {
			t.Errorf("retired-key error %q lacks the reconcile way forward", err.Error())
		}
	}
}

// seedConfig writes content to <baseDir>/_lyx/config/<module>.yaml,
// creating the config directory (and its _lyx parent) as needed. It is a
// plain-filesystem stand-in for gitkit's own git-spawning config-seeding
// helper, deliberately avoiding that helper's git spawn since
// configengine.LoadOrTemplate never needs a repository.
func seedConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()

	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := configengine.ConfigFile(baseDir, module)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestActive asserts Active resolves the configured profile from batcher.yaml:
// the template verbatim (which must itself parse as plain YAML) builds a cost batcher named after its cautious profile, an empty active: resolves identity both on the template's profiles and without a profiles: block, an identity active: resolves identity without a profiles: block, an absent batcher.yaml or an absent _lyx/ degrades to the embedded template's cautious batchifier rather than erroring, and every load error names batcher.yaml.
// The no-such-profile row holds only an operator profile, so it fails only while profiles: is an open map that the template's profiles are not filled into.
func TestActive(t *testing.T) {
	t.Parallel()
	template := batcher.ConfigTemplate()
	if !strings.HasPrefix(template, `active: "cautious"`) {
		t.Fatalf("ConfigTemplate() does not open with active: \"cautious\":\n%s", template)
	}
	tests := []struct {
		name string
		// seed prepares baseDir; a nil seed leaves it bare, with no _lyx/ at all.
		seed        func(t *testing.T, baseDir string)
		wantName    string
		wantErrWith []string
		// wantRetired asserts the error wraps ErrRetiredKey and names the reconcile way forward.
		wantRetired bool
	}{
		{
			name: "templateDefaultResolvesCautious",
			seed: func(t *testing.T, baseDir string) {
				var out map[string]any
				if err := yaml.Unmarshal([]byte(template), &out); err != nil {
					t.Fatalf("ConfigTemplate() does not parse as YAML: %v", err)
				}
				seedConfig(t, baseDir, "batcher", template)
			},
			wantName: "cautious",
		},
		{
			name: "emptyActiveOnTemplateResolvesIdentity",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, `active: "cautious"`, `active: ""`, 1))
			},
			wantName: batcher.DefaultName,
		},
		{
			name: "emptyActiveWithoutProfilesResolvesIdentity",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", `active: ""`+"\n")
			},
			wantName: batcher.DefaultName,
		},
		{
			name: "identityActiveWithoutProfilesResolvesIdentity",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", `active: "identity"`+"\n")
			},
			wantName: "identity",
		},
		{
			name: "absentConfigResolvesTemplate",
			seed: func(t *testing.T, baseDir string) {
				if err := os.MkdirAll(filepath.Join(baseDir, "_lyx"), 0o755); err != nil {
					t.Fatalf("mkdir _lyx: %v", err)
				}
			},
			wantName: "cautious",
		},
		{
			name:     "absentLyxDirResolvesTemplate",
			wantName: "cautious",
		},
		{
			name: "noSuchProfileErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", "active: \"cautious\"\nprofiles:\n  mine:\n    batchifier: identity\n")
			},
			wantErrWith: []string{"batcher.yaml", "cautious"},
		},
		{
			name: "unknownBatchifierErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", "active: \"mine\"\nprofiles:\n  mine:\n    batchifier: bogus\n")
			},
			wantErrWith: []string{"batcher.yaml", "mine", "bogus"},
		},
		{
			name: "missingThresholdErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "    budget: ", "    renamed_budget: ", 1))
			},
			wantErrWith: []string{"batcher.yaml", "cautious", "budget"},
		},
		{
			name: "nonPositiveBudgetErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "budget: 450000", "budget: 0", 1))
			},
			wantErrWith: []string{"batcher.yaml", "cautious", "budget"},
		},
		{
			name: "retiredAloneAboveErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "    budget: ", "    alone_above: 1200000\n    budget: ", 1))
			},
			wantErrWith: []string{"batcher.yaml", "cautious", "alone_above"},
			wantRetired: true,
		},
		{
			name: "retiredMasterBaseErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "orientation: 31400", "master_base: 52000", 1))
			},
			wantErrWith: []string{"batcher.yaml", "cautious", "master_base"},
			wantRetired: true,
		},
		{
			name: "maxCardsBelowTwoErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "max_cards: 6", "max_cards: 1", 1))
			},
			wantErrWith: []string{"batcher.yaml", "cautious", "max_cards"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if tt.seed != nil {
				tt.seed(t, baseDir)
			}

			got, err := batcher.Active(baseDir)
			if len(tt.wantErrWith) > 0 {
				if err == nil {
					t.Fatal("Active = nil error; want a load error")
				}
				for _, want := range tt.wantErrWith {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Active error = %q; want it to contain %q", err.Error(), want)
					}
				}
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("Active error = %q; want it marked configengine.ErrInvalid", err.Error())
				}
				assertRetiredKey(t, err, tt.wantRetired)
				return
			}
			if err != nil {
				t.Fatalf("Active = _, %v; want nil error", err)
			}
			if got.Name() != tt.wantName {
				t.Errorf("Active().Name() = %q; want %q", got.Name(), tt.wantName)
			}
		})
	}
}

// TestProfileWeights asserts ProfileWeights returns a valid profile's coefficients and errors naming batcher.yaml for an absent profile, a missing coefficient, a negative one and an unknown key.
func TestProfileWeights(t *testing.T) {
	t.Parallel()
	const valid = `profiles:
  cautious:
    weights:
      orientation: 10
      batch_growth: 3
      fork_messages: 2
      message_context: 8
      target_messages: 3
      test_file_messages: 4
      uses_messages: 5
      context_per_line: 0.5
      package_context: 7
      write_per_card_line: 9
`
	tests := []struct {
		name        string
		config      string
		profile     string
		want        batcher.Weights
		wantErrWith []string
		wantRetired bool
	}{
		{
			name:    "validProfile",
			config:  valid,
			profile: "cautious",
			want: batcher.Weights{
				Orientation: 10, BatchGrowth: 3, ForkMessages: 2, MessageContext: 8, TargetMessages: 3, TestFileMessages: 4,
				UsesMessages: 5, ContextPerLine: 0.5, PackageContext: 7, WritePerCardLine: 9,
			},
		},
		{
			name:        "absentProfile",
			config:      valid,
			profile:     "other",
			wantErrWith: []string{"batcher.yaml", "other"},
		},
		{
			name:        "missingCoefficient",
			config:      strings.Replace(valid, "      package_context: 7\n", "", 1),
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "package_context"},
		},
		{
			name:        "negativeCoefficient",
			config:      strings.Replace(valid, "fork_messages: 2", "fork_messages: -1", 1),
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "fork_messages"},
		},
		{
			name:        "missingOrientation",
			config:      strings.Replace(valid, "      orientation: 10\n", "", 1),
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "orientation"},
		},
		{
			name:        "negativeBatchGrowth",
			config:      strings.Replace(valid, "batch_growth: 3", "batch_growth: -1", 1),
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "batch_growth"},
		},
		{
			name:        "retiredStartupContext",
			config:      valid + "      startup_context: 60000\n",
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "cautious", "startup_context"},
			wantRetired: true,
		},
		{
			name:        "retiredMasterBase",
			config:      valid + "      master_base: 52000\n",
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "cautious", "master_base"},
			wantRetired: true,
		},
		{
			name:        "unknownKey",
			config:      valid + "      bogus_weight: 1\n",
			profile:     "cautious",
			wantErrWith: []string{"batcher.yaml", "bogus_weight"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			seedConfig(t, baseDir, "batcher", tt.config)

			got, err := batcher.ProfileWeights(baseDir, tt.profile)
			if len(tt.wantErrWith) > 0 {
				if err == nil {
					t.Fatal("ProfileWeights = nil error; want an error")
				}
				for _, want := range tt.wantErrWith {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ProfileWeights error = %q; want it to contain %q", err.Error(), want)
					}
				}
				assertRetiredKey(t, err, tt.wantRetired)
				return
			}
			if err != nil {
				t.Fatalf("ProfileWeights = _, %v; want nil error", err)
			}
			if got != tt.want {
				t.Errorf("ProfileWeights = %+v; want %+v", got, tt.want)
			}
		})
	}
	t.Run("templateCautiousCarriesTheMeasuredStart", func(t *testing.T) {
		t.Parallel()
		got, err := batcher.ProfileWeights(t.TempDir(), "cautious")
		if err != nil {
			t.Fatalf("ProfileWeights = _, %v; want nil error", err)
		}
		if got.Orientation != 31400 || got.BatchGrowth != 7000 || got.RetiredMasterBase != 0 || got.RetiredStartupContext != 0 {
			t.Errorf("template cautious start = orientation %v, batch_growth %v, master_base %v, startup_context %v; want 31400, 7000, 0, 0", got.Orientation, got.BatchGrowth, got.RetiredMasterBase, got.RetiredStartupContext)
		}
	})
}
