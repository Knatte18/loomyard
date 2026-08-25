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
}

// TestLoadConfig_DiscussionInteractiveTrue verifies a hand-edited loom.yaml with
// discussion_interactive: true round-trips to Config.DiscussionInteractive == true, distinct from
// the template's own false default.
func TestLoadConfig_DiscussionInteractiveTrue(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: true
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`)

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
	seedLoomConfig(t, baseDir, `discussion: "opus[effort"
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`)

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
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: "opus[effort"
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
`)

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
	seedLoomConfig(t, baseDir, `discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: "opus[effort"
review_timeout_min: 240
`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed review spec")
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "review")
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

// TestLoomStatusRel verifies LoomStatusRel is exactly the durable directory name joined with the
// loom subdirectory and the status filename -- the anchor-relative form a weft commit pathspec
// caller builds from.
func TestLoomStatusRel(t *testing.T) {
	want := filepath.Join(lyxdirs.LyxDirName, "loom", "status.json")
	if got := LoomStatusRel(); got != want {
		t.Errorf("LoomStatusRel() = %q; want %q", got, want)
	}
}

// TestLoomStatusFile_EqualsAnchorPathJoinedWithLoomStatusRel is the regression guard that card
// 11's refactor -- rewriting LoomStatusFile to join through LoomStatusRel -- left the returned
// value byte-identical to a plain AnchorPath()/LoomStatusRel() join.
func TestLoomStatusFile_EqualsAnchorPathJoinedWithLoomStatusRel(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), LoomStatusRel())
	if got := LoomStatusFile(l); got != want {
		t.Errorf("LoomStatusFile() = %q; want %q", got, want)
	}
}

// TestLoomDriverLogAndBootstrapLock covers LoomDriverLog and LoomBootstrapLock at both an
// unanchored and a subpath-anchored *lyxcwd.Location, hand-built rather than spawned.
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
				HubPath:      filepath.Join("home", "user", "repo-HUB"),
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
			// lock or the whole-run lock.
			if got := LoomBootstrapLock(l); got == LoomStatusLock(l) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from LoomStatusLock() = %q", got, LoomStatusLock(l))
			}
			if got := LoomBootstrapLock(l); got == LoomRunLock(l) {
				t.Errorf("LoomBootstrapLock() = %q; must differ from LoomRunLock() = %q", got, LoomRunLock(l))
			}
		})
	}
}

// TestLoomScratchDir_MirrorsRunLockDriverLogAndBootstrapLockParent verifies LoomScratchDir names
// exactly the directory LoomRunLock, LoomDriverLog, and LoomBootstrapLock already share, so the
// four never drift apart.
func TestLoomScratchDir_MirrorsRunLockDriverLogAndBootstrapLockParent(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
	}

	got := LoomScratchDir(l)

	if want := filepath.Dir(LoomRunLock(l)); got != want {
		t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomRunLock()))", got, want)
	}
	if want := filepath.Dir(LoomDriverLog(l)); got != want {
		t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomDriverLog()))", got, want)
	}
	if want := filepath.Dir(LoomBootstrapLock(l)); got != want {
		t.Errorf("LoomScratchDir() = %q; want %q (filepath.Dir(LoomBootstrapLock()))", got, want)
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
