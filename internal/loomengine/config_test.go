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

// writeLoomConfigWithKey seeds a loom.yaml built from the shipped template with exactly one key's
// value replaced, so a test can vary a single knob without restating the whole file and drifting
// from the template's key set (which configengine.Load is strict about).
func writeLoomConfigWithKey(t *testing.T, baseDir, key, value string) {
	t.Helper()
	var b strings.Builder
	replaced := false
	for _, line := range strings.Split(ConfigTemplate(), "\n") {
		if strings.HasPrefix(line, key+":") {
			b.WriteString(key + ": " + value + "\n")
			replaced = true
			continue
		}
		b.WriteString(line + "\n")
	}
	if !replaced {
		t.Fatalf("key %q not present in ConfigTemplate(); the fixture would silently test nothing", key)
	}
	seedLoomConfig(t, baseDir, b.String())
}

// TestLoadConfig_WellFormed verifies the template's default values round-trip.
func TestLoadConfig_WellFormed(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
	}
	if cfg.Discussion != "opus[effort=high]" {
		t.Errorf("cfg.Discussion = %q; want %q", cfg.Discussion, "opus[effort=high]")
	}
	if cfg.DiscussionTimeoutMin != 480 {
		t.Errorf("cfg.DiscussionTimeoutMin = %d; want %d", cfg.DiscussionTimeoutMin, 480)
	}
	if cfg.DiscussionInteractive != false {
		t.Errorf("cfg.DiscussionInteractive = %v; want %v", cfg.DiscussionInteractive, false)
	}
	if cfg.Plan != "opus[effort=high]" {
		t.Errorf("cfg.Plan = %q; want %q", cfg.Plan, "opus[effort=high]")
	}
	if cfg.PlanTimeoutMin != 120 {
		t.Errorf("cfg.PlanTimeoutMin = %d; want %d", cfg.PlanTimeoutMin, 120)
	}
	if cfg.Review != "opus[effort=high]" {
		t.Errorf("cfg.Review = %q; want %q", cfg.Review, "opus[effort=high]")
	}
	if cfg.ReviewTimeoutMin != 240 {
		t.Errorf("cfg.ReviewTimeoutMin = %d; want %d", cfg.ReviewTimeoutMin, 240)
	}
	if cfg.Selfreport != true {
		t.Errorf("cfg.Selfreport = %v; want %v", cfg.Selfreport, true)
	}
	if cfg.Friction != "opus[effort=high]" {
		t.Errorf("cfg.Friction = %q; want %q", cfg.Friction, "opus[effort=high]")
	}
	if cfg.FrictionTimeoutMin != 30 {
		t.Errorf("cfg.FrictionTimeoutMin = %d; want %d", cfg.FrictionTimeoutMin, 30)
	}
	if cfg.Driver != "" {
		t.Errorf("cfg.Driver = %q; want \"\" (the template's own default: defer to the provider default)", cfg.Driver)
	}
}

// TestLoadConfig_SelfreportFalse verifies a hand-edited loom.yaml with selfreport: false round-trips
// to Config.Selfreport == false, distinct from the template's own true default -- this, not an
// omitted key, is how a fork or CI run disarms automatic filing, since configengine.Load's strict
// loading does not fall back to the template for a key an existing on-disk file omits.
func TestLoadConfig_SelfreportFalse(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "selfreport", "false")

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
	}
	if cfg.Selfreport != false {
		t.Errorf("cfg.Selfreport = %v; want %v", cfg.Selfreport, false)
	}
}

// TestLoadConfig_DiscussionInteractiveTrue verifies a hand-edited loom.yaml with
// discussion_interactive: true round-trips to Config.DiscussionInteractive == true, distinct from
// the template's own false default.
func TestLoadConfig_DiscussionInteractiveTrue(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "discussion_interactive", "true")

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
	}
	if cfg.DiscussionInteractive != true {
		t.Errorf("cfg.DiscussionInteractive = %v; want %v", cfg.DiscussionInteractive, true)
	}
}

// TestLoadConfig_MalformedDiscussionSpec verifies a hand-edited loom.yaml with an ungrammatical
// discussion model-spec fails loud at load time, naming the "discussion" key, rather than being
// silently carried into the discussion producer's spawn site.
func TestLoadConfig_MalformedDiscussionSpec(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "discussion", `"opus[effort"`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed discussion spec")
	}
	if !strings.Contains(err.Error(), "discussion") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "discussion")
	}
}

// TestLoadConfig_MalformedPlanSpec verifies a hand-edited loom.yaml with a well-formed discussion
// spec but an ungrammatical plan model-spec fails loud at load time, naming the "plan" key, rather
// than being silently carried into the plan producer's spawn site.
func TestLoadConfig_MalformedPlanSpec(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "plan", `"opus[effort"`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed plan spec")
	}
	if !strings.Contains(err.Error(), "plan") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "plan")
	}
}

// TestLoadConfig_MalformedReviewSpec verifies a hand-edited loom.yaml with well-formed discussion
// and plan specs but an ungrammatical review model-spec fails loud at load time, naming the
// "review" key, rather than being silently carried into the review producers' spawn site.
func TestLoadConfig_MalformedReviewSpec(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "review", `"opus[effort"`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed review spec")
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "review")
	}
}

// TestLoadConfig_EmptyFrictionLoadsCleanly verifies a present-but-empty friction value -- Tier 2
// self-reporting turned off -- loads cleanly and yields the zero value for Config.Friction, unlike
// the discussion/plan/review role keys, which are always required.
func TestLoadConfig_EmptyFrictionLoadsCleanly(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
selfreport: true
friction: ""
friction_timeout_min: 30
driver: ""
`)

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error for a present-but-empty friction value", baseDir, err)
	}
	if cfg.Friction != "" {
		t.Errorf("cfg.Friction = %q; want \"\" (Tier 2 off)", cfg.Friction)
	}
}

// TestLoadConfig_MalformedFrictionSpec verifies a hand-edited loom.yaml with well-formed
// discussion/plan/review specs but an ungrammatical non-empty friction model-spec fails loud at
// load time, naming the "friction" key.
func TestLoadConfig_MalformedFrictionSpec(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
selfreport: true
friction: "opus[effort"
friction_timeout_min: 30
driver: ""
`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed friction spec")
	}
	if !strings.Contains(err.Error(), "friction") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "friction")
	}
}

// TestLoadConfig_MissingFrictionKeys verifies a loom.yaml genuinely lacking the friction and
// friction_timeout_min keys fails LoadConfig with configengine's "missing keys" error rather than
// defaulting -- the migration contract the Config Strictness Invariant requires for an
// already-seeded worktree.
func TestLoadConfig_MissingFrictionKeys(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for a loom.yaml missing the friction keys")
	}
	if !strings.Contains(err.Error(), "missing keys") {
		t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), "missing keys")
	}
	if !strings.Contains(err.Error(), "friction") {
		t.Errorf("LoadConfig() error = %q; want it to name the missing %q key", err.Error(), "friction")
	}
}

// TestLoadConfig_ValidDriverSpec verifies a hand-edited loom.yaml with a well-formed driver
// model-spec loads cleanly and round-trips onto Config.Driver.
func TestLoadConfig_ValidDriverSpec(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "driver", "opus[effort=high]")

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
	}
	if cfg.Driver != "opus[effort=high]" {
		t.Errorf("cfg.Driver = %q; want %q", cfg.Driver, "opus[effort=high]")
	}
}

// TestLoadConfig_MalformedDriverSpec verifies a hand-edited loom.yaml with well-formed
// discussion/plan/review/friction specs but an ungrammatical non-empty driver model-spec fails
// loud at load time, naming the "driver" key, rather than being silently carried into the driver
// session's spawn site.
func TestLoadConfig_MalformedDriverSpec(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "driver", `"opus[effort"`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed driver spec")
	}
	if !strings.Contains(err.Error(), "driver") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "driver")
	}
}

// TestLoadConfig_AbsentDriverLoadsCleanly verifies a driver key present with no value at all (YAML
// null, the shape a bare "driver:" line takes) loads cleanly and yields the zero value for
// Config.Driver -- "defer to the provider default" must not require an operator to type a literal
// empty string.
func TestLoadConfig_AbsentDriverLoadsCleanly(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
selfreport: true
friction: opus[effort=high]
friction_timeout_min: 30
driver:
`)

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error for a driver key with no value", baseDir, err)
	}
	if cfg.Driver != "" {
		t.Errorf("cfg.Driver = %q; want \"\" (defer to the provider default)", cfg.Driver)
	}
}

// TestLoadConfig_EmptyDriverLoadsCleanly verifies a present-but-explicitly-empty driver value loads
// cleanly and yields the zero value for Config.Driver, exactly like friction's own empty case,
// distinct from TestLoadConfig_AbsentDriverLoadsCleanly's bare-key shape.
func TestLoadConfig_EmptyDriverLoadsCleanly(t *testing.T) {
	baseDir := t.TempDir()
	writeLoomConfigWithKey(t, baseDir, "driver", `""`)

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error for a present-but-empty driver value", baseDir, err)
	}
	if cfg.Driver != "" {
		t.Errorf("cfg.Driver = %q; want \"\" (defer to the provider default)", cfg.Driver)
	}
}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint.
func TestLoadConfig_NotInitialized(t *testing.T) {
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

// TestLoomDriverLogAndBootstrapLock covers LoomDriverLog and LoomBootstrapLock at both an
// unanchored and a subpath-anchored *lyxcwd.Location, hand-built rather than spawned.
//
// It also proves the two survive card 12's status/run-lock relocation onto shedrun.RunDir:
// LoomBootstrapLock must never collide with shedrun's own StatusLock or RunLock for the same
// location and shedrun.SelfRunID, exactly as it never collided with loomengine's own
// now-deleted LoomStatusLock/LoomRunLock before the relocation -- this is what makes the
// overview's loomDirName-survives-the-status-relocation decision checkable rather than asserted.
func TestLoomDriverLogAndBootstrapLock(t *testing.T) {
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", "."},
		{"subpath-anchored", filepath.Join("sub", "dir")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		})
	}
}

// TestLoomScratchDir_MirrorsDriverLogAndBootstrapLockParent verifies LoomScratchDir names exactly
// the directory LoomDriverLog and LoomBootstrapLock already share, so the three never drift apart.
// LoomRunLock dropped out of this pin when card 12 relocated it onto shedrun.ScratchDir, which is a
// different directory from LoomScratchDir -- loom's own scratch tree under .lyx/loom/, not the run
// directory's own .lyx/shed/<runID>/ mirror.
func TestLoomScratchDir_MirrorsDriverLogAndBootstrapLockParent(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
	}

	got := LoomScratchDir(l)

	if want := filepath.Dir(LoomDriverLog(l)); got != want {
		t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomDriverLog()))", got, want)
	}
	if want := filepath.Dir(LoomBootstrapLock(l)); got != want {
		t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomBootstrapLock()))", got, want)
	}
}

// TestLoomScratchDir_DiffersFromShedrunScratchDir proves LoomScratchDir and
// shedrun.ScratchDir(l, shedrun.SelfRunID) are distinct directories for the same location, per the
// overview's loomDirName-survives-the-status-relocation decision: the "loom" segment still backs a
// real, distinct ephemeral tree of its own, it is simply no longer where the status file's locks
// live.
func TestLoomScratchDir_DiffersFromShedrunScratchDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
	}

	if got := LoomScratchDir(l); got == shedrun.ScratchDir(l, shedrun.SelfRunID) {
		t.Errorf("LoomScratchDir() = %q; must differ from shedrun.ScratchDir() = %q", got, shedrun.ScratchDir(l, shedrun.SelfRunID))
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks Config's fields via reflection and asserts
// every yaml tag appears in the template text -- so a struct field added without a matching
// template line is caught mechanically rather than relying on review to notice the gap.
// The Config Strictness Invariant makes a struct field with no matching template key a silent hole
// rather than a load error, which is exactly what this check guards against.
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
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

// TestLoadConfig_RejectsNegativeTimeouts pins the load-time guard on the four timeout knobs.
// Without it a negative minute count flowed into a shuttleengine.Spec's Timeout and was caught only
// when the producer it governs first spawned -- which is exactly the deferred failure LoadConfig's
// own header says the model-spec checks beside it exist to prevent.
func TestLoadConfig_RejectsNegativeTimeouts(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"DiscussionTimeout", "discussion_timeout_min"},
		{"PlanTimeout", "plan_timeout_min"},
		{"ReviewTimeout", "review_timeout_min"},
		{"FrictionTimeout", "friction_timeout_min"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeLoomConfigWithKey(t, dir, tt.key, "-1")

			_, err := LoadConfig(dir, "loom")
			if err == nil {
				t.Fatalf("LoadConfig() error = nil; want a refusal for a negative %s", tt.key)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("LoadConfig() error = %q; want it to name the offending key %q", err.Error(), tt.key)
			}
			if !strings.Contains(err.Error(), "must not be negative") {
				t.Errorf("LoadConfig() error = %q; want it to state the rule", err.Error())
			}
		})
	}
}

// TestLoadConfig_AcceptsZeroTimeouts pins the deliberate carve-out: shuttleengine.Spec treats a zero
// Timeout as "defer to shuttle's own run_timeout_min", so zero is a legitimate configuration and must
// not be swept up by the negative guard.
func TestLoadConfig_AcceptsZeroTimeouts(t *testing.T) {
	for _, key := range []string{"discussion_timeout_min", "plan_timeout_min", "review_timeout_min", "friction_timeout_min"} {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			writeLoomConfigWithKey(t, dir, key, "0")

			if _, err := LoadConfig(dir, "loom"); err != nil {
				t.Errorf("LoadConfig() error = %v; want nil (zero defers to shuttle's run_timeout_min)", err)
			}
		})
	}
}
