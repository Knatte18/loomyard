// config_test.go — untagged Tier-1 unit tests for loomengine.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/loom.yaml file (no real hub, no SeedConfig, no
// git spawn) since configengine.Load's env-source build tolerates a missing .env — a live fabric
// fixture would be integration-tagged (like websterengine/config_test.go), which is out of scope
// for this package's plain "go test ./internal/loomengine/" run.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// seedLoomConfig creates <baseDir>/_lyx/config/loom.yaml with the given contents.
func seedLoomConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// malformedBurlerYAML is a burler.yaml that fails to parse, for the fan-key rows that pin when LoadConfig reads it.
const malformedBurlerYAML = "fans: [unclosed\n"

// seedBurlerConfig creates <baseDir>/_lyx/config/burler.yaml with the given contents.
func seedBurlerConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	cfgPath := filepath.Join(baseDir, "_lyx", "config", "burler.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// writeLoomConfigWithKey seeds a loom.yaml built from the shipped template with exactly one key's
// value replaced, so a test can vary a single knob without restating the whole file and drifting
// from the template's key set (which configengine.Load is strict about).
func writeLoomConfigWithKey(t *testing.T, baseDir, key, value string) {
	t.Helper()
	writeLoomConfigWithKeys(t, baseDir, map[string]string{key: value})
}

// writeLoomConfigWithKeys seeds baseDir's loom.yaml with the template's text and each key in values replaced by its value.
func writeLoomConfigWithKeys(t *testing.T, baseDir string, values map[string]string) {
	t.Helper()
	var b strings.Builder
	replaced := map[string]bool{}
	for _, line := range strings.Split(ConfigTemplate(), "\n") {
		matched := false
		for key, value := range values {
			if strings.HasPrefix(line, key+":") {
				b.WriteString(key + ": " + value + "\n")
				replaced[key] = true
				matched = true
				break
			}
		}
		if !matched {
			b.WriteString(line + "\n")
		}
	}
	for key := range values {
		if !replaced[key] {
			t.Fatalf("key %q not present in ConfigTemplate(); the fixture would silently test nothing", key)
		}
	}
	seedLoomConfig(t, baseDir, b.String())
}

// templateConfig is the Config the shipped template's own values load to.
func templateConfig() Config {
	return Config{
		Discussion:               "opus[medium]",
		DiscussionTimeoutMin:     480,
		DiscussionInteractive:    false,
		DiscussionProducer:       "single",
		DiscussionAdvisors:       ModelSpecList{"sonnet[high]"},
		Plan:                    "opus[medium]",
		PlanTimeoutMin:           120,
		Review:                   ModelSpecList{"opus[medium]"},
		Fix:                      ModelSpecList{"opus[medium]"},
		DiscussionReview:         ModelSpecList{""},
		DiscussionFix:            ModelSpecList{""},
		PlanReview:               ModelSpecList{""},
		PlanFix:                  ModelSpecList{""},
		WebsterReview:            ModelSpecList{"sonnet[high]"},
		WebsterFix:               ModelSpecList{""},
		DiscussionFan:            "",
		PlanFan:                  "",
		FanReview:                ModelSpecList{"sonnet[high]"},
		Judge:                    "sonnet[medium]",
		ReviewTimeoutMin:         240,
		Friction:                 "sonnet[medium]",
		FrictionTimeoutMin:       30,
		Driver:                   "sonnet[medium]",
		ParentReviewWaitMin:      60,
		ReviewCirclingCheckpoint: 3,
		ReviewMaxBounces:         3,
		FixStart:                 "parallel",
	}
}

// TestLoadConfig_Loads verifies each loom.yaml shape that must load, against the whole Config it yields:
// the template's own values, an unparseable burler.yaml beside empty fan keys (never read), a retired key ignored, explicit overrides, present-but-empty and null friction and driver values (Tier 2 off, "defer to the provider default"), keys absent from an already-seeded file at their template defaults, zero timeouts (shuttleengine.Spec treats zero as "defer to shuttle's run_timeout_min", so the negative guard must not sweep them up), and a checkpoint above the budget (a run that never rules CIRCLING and reaches the budget escalation instead).
func TestLoadConfig_Loads(t *testing.T) {
	t.Parallel()
	// legacyContents is a file seeded before the friction, driver, judge, parent-review and review-budget keys existed.
	const legacyContents = `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`
	tests := []struct {
		name     string
		contents string
		values   map[string]string
		burler   string
		mutate   func(*Config)
	}{
		{name: "template defaults", mutate: func(*Config) {}},
		{
			name:   "empty fan keys never read a malformed burler.yaml",
			burler: malformedBurlerYAML,
			mutate: func(*Config) {},
		},
		{
			name:     "retired selfreport key ignored",
			contents: ConfigTemplate() + "selfreport: false\n",
			mutate:   func(*Config) {},
		},
		{
			name:   "review as a per-round list",
			values: map[string]string{"review": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.Review = ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "fix as a per-round list",
			values: map[string]string{"fix": "\n  - opus[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.Fix = ModelSpecList{"opus[low]", "opus[high]"} },
		},
		{
			name:   "plan_review as a per-round list",
			values: map[string]string{"plan_review": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.PlanReview = ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "webster_fix as one model-spec",
			values: map[string]string{"webster_fix": "opus[low]"},
			mutate: func(c *Config) { c.WebsterFix = ModelSpecList{"opus[low]"} },
		},
		{
			name:   "empty segment list takes the run-wide list",
			values: map[string]string{"discussion_fix": "[]"},
			mutate: func(c *Config) { c.DiscussionFix = ModelSpecList{} },
		},
		{
			name:   "discussion_fan and plan_fan name template fans, fan_review a per-round list",
			values: map[string]string{"discussion_fan": "standard", "plan_fan": "full", "fan_review": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) {
				c.DiscussionFan, c.PlanFan = "standard", "full"
				c.FanReview = ModelSpecList{"sonnet[low]", "opus[high]"}
			},
		},
		{
			name:   "fix_start after-review",
			values: map[string]string{"fix_start": "after-review"},
			mutate: func(c *Config) { c.FixStart = "after-review" },
		},
		{
			name:   "discussion_interactive true",
			values: map[string]string{"discussion_interactive": "true"},
			mutate: func(c *Config) { c.DiscussionInteractive = true },
		},
		{
			name:   "discussion_producer seats",
			values: map[string]string{"discussion_producer": "seats"},
			mutate: func(c *Config) { c.DiscussionProducer = "seats" },
		},
		{
			name:   "discussion_advisors as an empty string means no advisors",
			values: map[string]string{"discussion_advisors": `""`},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "bare discussion_advisors key is null and means no advisors",
			values: map[string]string{"discussion_advisors": ""},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "discussion_advisors as an empty list means no advisors",
			values: map[string]string{"discussion_advisors": "[]"},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "discussion_advisors as a list of one empty string means no advisors",
			values: map[string]string{"discussion_advisors": `[""]`},
			mutate: func(c *Config) { c.DiscussionAdvisors = nil },
		},
		{
			name:   "discussion_advisors as one model-spec",
			values: map[string]string{"discussion_advisors": "opus[low]"},
			mutate: func(c *Config) { c.DiscussionAdvisors = ModelSpecList{"opus[low]"} },
		},
		{
			name:   "discussion_advisors as a two-entry list",
			values: map[string]string{"discussion_advisors": "\n  - sonnet[low]\n  - opus[high]"},
			mutate: func(c *Config) { c.DiscussionAdvisors = ModelSpecList{"sonnet[low]", "opus[high]"} },
		},
		{
			name:   "empty friction turns Tier 2 off",
			values: map[string]string{"friction": `""`},
			mutate: func(c *Config) { c.Friction = "" },
		},
		{
			name:   "valid driver spec",
			values: map[string]string{"driver": "opus[effort=high]"},
			mutate: func(c *Config) { c.Driver = "opus[effort=high]" },
		},
		{
			name:   "bare driver key is null",
			values: map[string]string{"driver": ""},
			mutate: func(c *Config) { c.Driver = "" },
		},
		{
			name:   "empty driver string",
			values: map[string]string{"driver": `""`},
			mutate: func(c *Config) { c.Driver = "" },
		},
		{
			name:   "explicit parent_review_wait_min",
			values: map[string]string{"parent_review_wait_min": "5"},
			mutate: func(c *Config) { c.ParentReviewWaitMin = 5 },
		},
		{
			name:   "explicit review checkpoint and budget",
			values: map[string]string{"review_circling_checkpoint": "2", "review_max_bounces": "7"},
			mutate: func(c *Config) { c.ReviewCirclingCheckpoint, c.ReviewMaxBounces = 2, 7 },
		},
		{
			name:   "checkpoint above budget",
			values: map[string]string{"review_circling_checkpoint": "9", "review_max_bounces": "2"},
			mutate: func(c *Config) { c.ReviewCirclingCheckpoint, c.ReviewMaxBounces = 9, 2 },
		},
		{
			name:     "keys absent from a seeded file take template defaults",
			contents: legacyContents,
			mutate: func(c *Config) {
				c.Discussion, c.Plan, c.Review = "opus[effort=high]", "opus[effort=high]", ModelSpecList{"opus[effort=high]"}
			},
		},
		{
			name:   "zero discussion timeout",
			values: map[string]string{"discussion_timeout_min": "0"},
			mutate: func(c *Config) { c.DiscussionTimeoutMin = 0 },
		},
		{
			name:   "zero plan timeout",
			values: map[string]string{"plan_timeout_min": "0"},
			mutate: func(c *Config) { c.PlanTimeoutMin = 0 },
		},
		{
			name:   "zero review timeout",
			values: map[string]string{"review_timeout_min": "0"},
			mutate: func(c *Config) { c.ReviewTimeoutMin = 0 },
		},
		{
			name:   "zero friction timeout",
			values: map[string]string{"friction_timeout_min": "0"},
			mutate: func(c *Config) { c.FrictionTimeoutMin = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			switch {
			case tt.contents != "":
				seedLoomConfig(t, baseDir, tt.contents)
			case tt.values != nil:
				writeLoomConfigWithKeys(t, baseDir, tt.values)
			default:
				seedLoomConfig(t, baseDir, ConfigTemplate())
			}
			if tt.burler != "" {
				seedBurlerConfig(t, baseDir, tt.burler)
			}

			want := templateConfig()
			tt.mutate(&want)

			got, err := LoadConfig(baseDir, "loom")
			if err != nil {
				t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadConfig() = %+v; want %+v", got, want)
			}
		})
	}
}

// TestLoadConfig_Refuses verifies each invalid loom.yaml is refused at load time, naming the offending key:
// an ungrammatical model-spec (rather than being silently carried into a producer's spawn site), a parent_review_wait_min or review key below one (the sibling timeouts' "0 defers to shuttle" carve-out does not apply to them) with the way forward, a negative timeout (which would otherwise surface only when the producer it governs first spawns), and a set fan key beside an unparseable burler.yaml.
func TestLoadConfig_Refuses(t *testing.T) {
	t.Parallel()
	const positiveIntegerForward = "set it to a positive integer in loom.yaml"
	tests := []struct {
		name   string
		key    string
		value  string
		wantIn []string
		burler string
	}{
		{"malformed discussion spec", "discussion", `"opus[effort"`, nil, ""},
		{"malformed plan spec", "plan", `"opus[effort"`, nil, ""},
		{"malformed review spec", "review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed fix spec", "fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later review entry", "review", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"empty review list", "review", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"empty fix list", "fix", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_review spec", "discussion_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed discussion_fix spec", "discussion_fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed plan_review spec", "plan_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed plan_fix spec", "plan_fix", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed webster_review spec", "webster_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"malformed later webster_fix entry", "webster_fix", "\n  - sonnet[low]\n  - \"opus[effort\"", []string{"entry 2", "a non-empty list of model-specs"}, ""},
		{"unknown discussion_fan", "discussion_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "standard", "to empty to run the segment solo"}, ""},
		{"unknown plan_fan", "plan_fan", "nosuchfan", []string{`"nosuchfan"`, "known fans: ", "full", "to empty to run the segment solo"}, ""},
		{"set discussion_fan with a malformed burler.yaml", "discussion_fan", "standard", []string{"burler: parse ", "fix burler.yaml, or set the key to empty"}, malformedBurlerYAML},
		{"malformed fan_review spec", "fan_review", `"opus[effort"`, []string{"entry 1", "a non-empty list of model-specs"}, ""},
		{"empty fan_review list", "fan_review", "[]", []string{"empty", "a non-empty list of model-specs"}, ""},
		{"unknown fix_start", "fix_start", "sideways", []string{`"parallel"`, `"after-review"`}, ""},
		{"unknown discussion_producer", "discussion_producer", "sideways", []string{`"single"`, `"seats"`}, ""},
		{"empty discussion_advisors entry beside another", "discussion_advisors", `["", "sonnet[high]"]`, []string{"entry 1", advisorWayForward}, ""},
		{"malformed discussion_advisors entry", "discussion_advisors", `["opus[effort"]`, []string{"entry 1", advisorWayForward}, ""},
		{"unknown discussion_advisors alias", "discussion_advisors", `["sonnet[high]", "ghost"]`, []string{"entry 2", "ghost", advisorWayForward}, ""},
		{"mapping discussion_advisors value", "discussion_advisors", "\n  model: opus", []string{"line ", advisorWayForward}, ""},
		{"mapping review value", "review", "\n  model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"mapping inside a fix list", "fix", "\n  - model: opus", []string{"a non-empty list of model-specs"}, ""},
		{"malformed judge spec", "judge", `"sonnet[medium"`, nil, ""},
		{"malformed friction spec", "friction", `"opus[effort"`, nil, ""},
		{"malformed driver spec", "driver", `"opus[effort"`, nil, ""},
		{"zero parent_review_wait_min", "parent_review_wait_min", "0", []string{"attempts: 0"}, ""},
		{"negative parent_review_wait_min", "parent_review_wait_min", "-1", []string{"attempts: 0"}, ""},
		{"zero review_circling_checkpoint", "review_circling_checkpoint", "0", []string{positiveIntegerForward}, ""},
		{"negative review_circling_checkpoint", "review_circling_checkpoint", "-1", []string{positiveIntegerForward}, ""},
		{"zero review_max_bounces", "review_max_bounces", "0", []string{positiveIntegerForward}, ""},
		{"negative review_max_bounces", "review_max_bounces", "-1", []string{positiveIntegerForward}, ""},
		{"negative discussion timeout", "discussion_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative plan timeout", "plan_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative review timeout", "review_timeout_min", "-1", []string{"must not be negative"}, ""},
		{"negative friction timeout", "friction_timeout_min", "-1", []string{"must not be negative"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeLoomConfigWithKey(t, dir, tt.key, tt.value)
			if tt.burler != "" {
				seedBurlerConfig(t, dir, tt.burler)
			}

			_, err := LoadConfig(dir, "loom")
			if err == nil {
				t.Fatalf("LoadConfig() error = nil; want a refusal for %s: %s", tt.key, tt.value)
			}
			for _, want := range append([]string{tt.key}, tt.wantIn...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

	t.Run("a mapping discussion_advisors value names no entry index", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLoomConfigWithKey(t, dir, "discussion_advisors", "\n  model: opus")

		_, err := LoadConfig(dir, "loom")
		if err == nil || strings.Contains(err.Error(), "entry") {
			t.Errorf("LoadConfig() error = %v; want a refusal that names no entry", err)
		}
	})
}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint.
func TestLoadConfig_NotInitialized(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lyx fabric reconcile"`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}

// TestLoomDriverLogAndBootstrapLock covers LoomDriverLog and LoomBootstrapLock at both an unanchored and a subpath-anchored *lyxcwd.Location, hand-built rather than spawned.
//
// It also proves the two survive card 12's status/run-lock relocation onto shedrun.RunDir:
// LoomBootstrapLock must never collide with shedrun's own StatusLock or RunLock for the same location and shedrun.SelfRunID, exactly as it never collided with loomengine's own now-deleted LoomStatusLock/LoomRunLock before the relocation -- this is what makes the overview's loomDirName-survives-the-status-relocation decision checkable rather than asserted.
//
// LoomScratchDir names exactly the directory the driver log and the bootstrap lock share, so the three never drift apart, and that directory is loom's own scratch tree under .lyx/loom/, distinct from the run directory's .lyx/shed/<runID>/ mirror that shedrun.ScratchDir names.
func TestLoomDriverLogAndBootstrapLock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", "."},
		{"subpath-anchored", filepath.Join("sub", "dir")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := &lyxcwd.Location{
				HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
				WorktreeName: "repo",
				AnchorRel:    tt.anchorRel,
			}

			wantDriverLog := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "driver.log")
			if got := LoomDriverLog(l); got != wantDriverLog {
				t.Errorf("LoomDriverLog() = %q; want %q", got, wantDriverLog)
			}

			wantBootstrapLock := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "bootstrap.lock")
			if got := LoomBootstrapLock(l); got != wantBootstrapLock {
				t.Errorf("LoomBootstrapLock() = %q; want %q", got, wantBootstrapLock)
			}

			// Three distinct lock files is a correctness property, not a coincidence:
			// LoomBootstrapLock must never collide with either the per-persist status
			// lock or the whole-run lock, both now on shedrun's run directory.
			if got := LoomBootstrapLock(l); got == shedrun.StatusLock(l, shedrun.SelfRunID) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from shedrun.StatusLock() = %q", got, shedrun.StatusLock(l, shedrun.SelfRunID))
			}
			if got := LoomBootstrapLock(l); got == shedrun.RunLock(l, shedrun.SelfRunID) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from shedrun.RunLock() = %q", got, shedrun.RunLock(l, shedrun.SelfRunID))
			}

			got := LoomScratchDir(l)
			if want := filepath.Dir(LoomDriverLog(l)); got != want {
				t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomDriverLog()))", got, want)
			}
			if want := filepath.Dir(LoomBootstrapLock(l)); got != want {
				t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomBootstrapLock()))", got, want)
			}
			if got == shedrun.ScratchDir(l, shedrun.SelfRunID) {
				t.Errorf("LoomScratchDir() = %q; must differ from shedrun.ScratchDir() = %q", got, shedrun.ScratchDir(l, shedrun.SelfRunID))
			}
		})
	}
}

// TestLandingAndApprovalAccessors pins the landing directory, its commit-pathspec form and the
// approval record path for a hand-built location.
func TestLandingAndApprovalAccessors(t *testing.T) {
	t.Parallel()
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
	}

	if got, want := LandingDirRel(), filepath.Join("_lyx", "landing"); got != want {
		t.Errorf("LandingDirRel() = %q; want %q", got, want)
	}
	if got, want := LandingDir(l), filepath.Join(l.AnchorPath(), "_lyx", "landing"); got != want {
		t.Errorf("LandingDir() = %q; want %q", got, want)
	}
	if got, want := LoomApprovalPath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "approval.json"); got != want {
		t.Errorf("LoomApprovalPath() = %q; want %q", got, want)
	}
	if got, want := LoomRejectionPath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "rejection.json"); got != want {
		t.Errorf("LoomRejectionPath() = %q; want %q", got, want)
	}
	if got, want := LoomReworkCoveragePath(l), filepath.Join(l.AnchorPath(), ".lyx", "loom", "rework-coverage.md"); got != want {
		t.Errorf("LoomReworkCoveragePath() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDirRel(), filepath.Join("_lyx", "loom", "rework"); got != want {
		t.Errorf("LoomReworkDirRel() = %q; want %q", got, want)
	}
	if got, want := LoomReworkDir(l), filepath.Join(l.AnchorPath(), "_lyx", "loom", "rework"); got != want {
		t.Errorf("LoomReworkDir() = %q; want %q", got, want)
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks Config's fields via reflection and asserts every yaml tag appears in the template text -- so a struct field added without a matching template line is caught mechanically rather than relying on review to notice the gap.
// The Config Strictness Invariant makes a struct field with no matching template key a silent hole rather than a load error, which is exactly what this check guards against.
//
//testtiming:keep pins that every Config yaml tag has a template line, a reflection check no LoadConfig case makes
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	t.Parallel()
	text := ConfigTemplate()

	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" {
			t.Fatalf("Config field %q has no yaml tag", typ.Field(i).Name)
		}
		if !containsConfigKey(text, tag) {
			t.Errorf("ConfigTemplate() does not contain key %q for field %q", tag, typ.Field(i).Name)
		}
	}
}

// containsConfigKey reports whether text contains a "<key>:" line-start token, the shape every one
// of this template's keys takes.
func containsConfigKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}
