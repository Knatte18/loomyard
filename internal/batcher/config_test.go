// config_test.go tests batcher.yaml: its template, Active's profile resolution and ProfileWeights.
// Its fixtures are plain files in a temporary directory, with no git repository.

package batcher_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// assertRetiredKey fails the test unless err wraps ErrRetiredKey exactly when want is set.
// A retired-key error offers the reconcile way forward exactly when handDeleteOnly is not set.
func assertRetiredKey(t *testing.T, err error, want, handDeleteOnly bool) {
	t.Helper()
	if got := errors.Is(err, batcher.ErrRetiredKey); got != want {
		t.Errorf("error %q wraps ErrRetiredKey = %v; want %v", err.Error(), got, want)
	}
	if want {
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Errorf("retired-key error %q is not marked configengine.ErrInvalid", err.Error())
		}
		if !strings.Contains(err.Error(), "by hand") {
			t.Errorf("retired-key error %q lacks the hand-delete way forward", err.Error())
		}
		if offersReconcile := strings.Contains(err.Error(), `"lym config reconcile --apply"`); offersReconcile == handDeleteOnly {
			t.Errorf("retired-key error %q offers the reconcile way forward = %v; want %v", err.Error(), offersReconcile, !handDeleteOnly)
		}
	}
}

// seedConfig writes content to <baseDir>/_lym/config/<module>.yaml, creating the directories as needed.
// It writes the file directly, with no git spawn, since the config loader needs no repository.
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

// TestActive asserts Active resolves the configured profile from batcher.yaml.
// Each layer's profiles add to the template's by name, merging key by key.
// An absent batcher.yaml or _lym/ degrades to the embedded template's wide batchifier,
// and every load error names batcher.yaml.
func TestActive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	template := batcher.ConfigTemplate()
	if !strings.HasPrefix(template, `active: "wide"`) {
		t.Fatalf("ConfigTemplate() does not open with active: \"wide\":\n%s", template)
	}
	tests := []struct {
		name string
		// seed prepares baseDir; a nil seed leaves it bare, with no _lym/ at all.
		seed        func(t *testing.T, baseDir string)
		wantName    string
		wantErrWith []string
		// wantRetired asserts the error wraps ErrRetiredKey.
		wantRetired bool
		// wantHandDeleteOnly asserts the retired-key way forward offers no reconcile verb.
		wantHandDeleteOnly bool
	}{
		{
			name: "templateDefaultResolvesWide",
			seed: func(t *testing.T, baseDir string) {
				var out map[string]any
				if err := yaml.Unmarshal([]byte(template), &out); err != nil {
					t.Fatalf("ConfigTemplate() does not parse as YAML: %v", err)
				}
				seedConfig(t, baseDir, "batcher", template)
			},
			wantName: "wide",
		},
		{
			name: "emptyActiveOnTemplateResolvesIdentity",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, `active: "wide"`, `active: ""`, 1))
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
				if err := os.MkdirAll(filepath.Join(baseDir, "_lym"), 0o755); err != nil {
					t.Fatalf("mkdir _lym: %v", err)
				}
			},
			wantName: "wide",
		},
		{
			name:     "absentLymDirResolvesTemplate",
			wantName: "wide",
		},
		{
			name: "noSuchProfileErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", "active: \"nope\"\nprofiles:\n  mine:\n    batchifier: identity\n")
			},
			wantErrWith: []string{"batcher.yaml", "nope"},
		},
		{
			name: "profilesThatIsAListErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", "profiles:\n  - cautious\n")
			},
			wantErrWith: []string{"batcher.yaml", "profiles", "sequence"},
		},
		{
			name: "operatorProfileAddsToTheTemplates",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", "active: \"cautious\"\nprofiles:\n  mine:\n    batchifier: identity\n")
			},
			wantName: "cautious",
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
				operatorCopy := strings.NewReplacer(`active: "wide"`, `active: "mine"`, "  wide:", "  mine:", "    budget: ", "    renamed_budget: ").Replace(template)
				seedConfig(t, baseDir, "batcher", operatorCopy)
			},
			wantErrWith: []string{"batcher.yaml", "mine", "budget"},
		},
		{
			name: "nonPositiveBudgetErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "budget: 300000", "budget: 0", 1))
			},
			wantErrWith: []string{"batcher.yaml", "wide", "budget"},
		},
		{
			name: "retiredAloneAboveErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "    budget: ", "    alone_above: 1200000\n    budget: ", 1))
			},
			wantErrWith: []string{"batcher.yaml", "wide", "alone_above"},
			wantRetired: true,
		},
		{
			name: "retiredMasterBaseErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "orientation: 31400", "master_base: 52000", 1))
			},
			wantErrWith: []string{"batcher.yaml", "wide", "master_base"},
			wantRetired: true,
		},
		{
			name: "retiredAloneAboveInAnOperatorNamedProfileOffersOnlyTheHandDelete",
			seed: func(t *testing.T, baseDir string) {
				operatorCopy := strings.NewReplacer(`active: "wide"`, `active: "mine"`, "  wide:", "  mine:").Replace(template)
				seedConfig(t, baseDir, "batcher", strings.Replace(operatorCopy, "    budget: ", "    alone_above: 1200000\n    budget: ", 1))
			},
			wantErrWith:        []string{"batcher.yaml", "mine", "alone_above"},
			wantRetired:        true,
			wantHandDeleteOnly: true,
		},
		{
			name: "maxCardsBelowTwoErrors",
			seed: func(t *testing.T, baseDir string) {
				seedConfig(t, baseDir, "batcher", strings.Replace(template, "max_cards: 6", "max_cards: 1", 1))
			},
			wantErrWith: []string{"batcher.yaml", "wide", "max_cards"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if tt.seed != nil {
				tt.seed(t, baseDir)
			}

			got, err := batcher.Active(configengine.Dirs{Anchor: baseDir})
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
				assertRetiredKey(t, err, tt.wantRetired, tt.wantHandDeleteOnly)
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

	hubTests := []struct {
		name         string
		repoDefault  string
		worktreeFile string
		wantName     string
		wantErrWith  []string
		// wantDefaultFileInError asserts the error names the repo default's file.
		wantDefaultFileInError bool
	}{
		{
			name:         "activeSetOnlyInRepoDefaultSelectsItsProfile",
			repoDefault:  `active: "identity"` + "\n",
			worktreeFile: "profiles:\n  mine:\n    batchifier: identity\n",
			wantName:     "identity",
		},
		{
			name:         "worktreeProfilesAddToTheDefaults",
			repoDefault:  template,
			worktreeFile: "profiles:\n  mine:\n    batchifier: identity\n",
			wantName:     "wide",
		},
		{
			name:        "repoDefaultTuningOneKeyKeepsTheTemplatesOthers",
			repoDefault: "profiles:\n  wide:\n    budget: 250000\n",
			wantName:    "wide",
		},
		{
			name:        "repoDefaultTunedKeyReachesTheProfile",
			repoDefault: "profiles:\n  wide:\n    max_cards: 1\n",
			wantErrWith: []string{"batcher.yaml", "wide", "max_cards"},
		},
		{
			name:                   "retiredKeyInRepoDefaultNamesDefaultFileAndDefaultLayerReconcile",
			repoDefault:            strings.Replace(template, "    budget: ", "    alone_above: 1200000\n    budget: ", 1),
			worktreeFile:           "profiles:\n  mine:\n    batchifier: identity\n",
			wantErrWith:            []string{"alone_above", `or run "lym config reconcile --layer default --apply"`, "the orch runs it"},
			wantDefaultFileInError: true,
		},
	}
	for _, tt := range hubTests {
		t.Run("hubStack/"+tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir, anchor := t.TempDir(), t.TempDir()
			seedConfig(t, boardDir, "batcher", tt.repoDefault)
			if tt.worktreeFile != "" {
				seedConfig(t, anchor, "batcher", tt.worktreeFile)
			}

			got, err := batcher.Active(configengine.Dirs{Anchor: anchor, Board: boardDir})
			if len(tt.wantErrWith) > 0 {
				if err == nil {
					t.Fatal("Active = nil error; want a load error")
				}
				for _, want := range tt.wantErrWith {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Active error = %q; want it to contain %q", err.Error(), want)
					}
				}
				if tt.wantDefaultFileInError {
					if wantFile := configengine.ConfigFile(boardDir, "batcher"); !strings.Contains(err.Error(), wantFile) {
						t.Errorf("Active error = %q; want it to name the repo default %q", err.Error(), wantFile)
					}
					if !errors.Is(err, batcher.ErrRetiredKey) || !errors.Is(err, configengine.ErrInvalid) {
						t.Errorf("Active error = %q; want it to wrap ErrRetiredKey and be marked configengine.ErrInvalid", err.Error())
					}
				}
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

// TestProfileWeights asserts ProfileWeights returns a valid profile's coefficients.
// It also asserts that an absent or invalid profile fails with an error naming batcher.yaml.
// The template's wide and cautious profiles carry the same weights.
func TestProfileWeights(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const valid = `profiles:
  tuned:
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
			profile: "tuned",
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
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "package_context"},
		},
		{
			name:        "negativeCoefficient",
			config:      strings.Replace(valid, "fork_messages: 2", "fork_messages: -1", 1),
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "fork_messages"},
		},
		{
			name:        "missingOrientation",
			config:      strings.Replace(valid, "      orientation: 10\n", "", 1),
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "orientation"},
		},
		{
			name:        "negativeBatchGrowth",
			config:      strings.Replace(valid, "batch_growth: 3", "batch_growth: -1", 1),
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "batch_growth"},
		},
		{
			name:        "retiredStartupContext",
			config:      valid + "      startup_context: 60000\n",
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "tuned", "startup_context"},
			wantRetired: true,
		},
		{
			name:        "retiredMasterBase",
			config:      valid + "      master_base: 52000\n",
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "tuned", "master_base"},
			wantRetired: true,
		},
		{
			name:        "unknownKey",
			config:      valid + "      bogus_weight: 1\n",
			profile:     "tuned",
			wantErrWith: []string{"batcher.yaml", "bogus_weight"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			seedConfig(t, baseDir, "batcher", tt.config)

			got, err := batcher.ProfileWeights(configengine.Dirs{Anchor: baseDir}, tt.profile)
			if len(tt.wantErrWith) > 0 {
				if err == nil {
					t.Fatal("ProfileWeights = nil error; want an error")
				}
				for _, want := range tt.wantErrWith {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ProfileWeights error = %q; want it to contain %q", err.Error(), want)
					}
				}
				assertRetiredKey(t, err, tt.wantRetired, true)
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
	t.Run("templateProfilesShareTheMeasuredWeights", func(t *testing.T) {
		t.Parallel()
		dirs := configengine.Dirs{Anchor: t.TempDir()}
		wide, err := batcher.ProfileWeights(dirs, "wide")
		if err != nil {
			t.Fatalf("ProfileWeights(wide) = _, %v; want nil error", err)
		}
		cautious, err := batcher.ProfileWeights(dirs, "cautious")
		if err != nil {
			t.Fatalf("ProfileWeights(cautious) = _, %v; want nil error", err)
		}
		if wide != cautious {
			t.Errorf("template wide weights = %+v; want cautious's %+v", wide, cautious)
		}
		if wide.Orientation != 31400 || wide.BatchGrowth != 7000 {
			t.Errorf("template wide start = orientation %v, batch_growth %v; want 31400, 7000", wide.Orientation, wide.BatchGrowth)
		}
	})
	// No accessor exposes a profile's budget or max_cards, so the template is decoded directly.
	t.Run("templateWideDiffersFromCautiousOnlyByBudget", func(t *testing.T) {
		t.Parallel()
		var template struct {
			Profiles map[string]struct {
				Batchifier string `yaml:"batchifier"`
				MaxCards   int    `yaml:"max_cards"`
				Budget     int    `yaml:"budget"`
			} `yaml:"profiles"`
		}
		if err := yaml.Unmarshal([]byte(batcher.ConfigTemplate()), &template); err != nil {
			t.Fatalf("unmarshal template: %v", err)
		}
		wide, cautious := template.Profiles["wide"], template.Profiles["cautious"]
		if wide.Budget != 300000 || cautious.Budget != 200000 {
			t.Errorf("template budgets = wide %d, cautious %d; want 300000, 200000", wide.Budget, cautious.Budget)
		}
		if wide.MaxCards != cautious.MaxCards || wide.Batchifier != cautious.Batchifier {
			t.Errorf("template wide = batchifier %q, max_cards %d; want cautious's %q, %d", wide.Batchifier, wide.MaxCards, cautious.Batchifier, cautious.MaxCards)
		}
	})
}
