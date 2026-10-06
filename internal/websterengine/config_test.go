// config_test.go verifies webster.yaml's template parses, defaults resolve through LoadConfig,
// overrides round-trip, a malformed role model-spec fails loud naming the offending key, and an
// absent _lyx/ degrades to the embedded template,
// seeded via plain os.MkdirAll/os.WriteFile against a
// t.TempDir() rather than gitkit's config fixture-copy helpers: configengine.LoadOrTemplate
// only requires a filesystem _lyx/config/<module>.yaml, no git repository, so this test stays
// untagged and spawn-free (Test Tier Purity Invariant).

package websterengine_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// seedConfig writes module's content to <baseDir>/_lyx/config/<module>.yaml,
// creating the config directory (and its _lyx parent) as needed. It is a
// plain-filesystem stand-in for gitkit's config seeding, deliberately avoiding
// that helper's git spawn since configengine.Load never needs a repository.
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

// TestConfigTemplate pins that every yaml tag of Config appears in the embedded template's text, so a
// struct field added without a matching template line is caught mechanically rather than relying on
// review to notice the gap.
//
//testtiming:keep pins every Config yaml tag having a template line; the load test only observes the keys the template already holds
func TestConfigTemplate(t *testing.T) {
	t.Parallel()

	text := websterengine.ConfigTemplate()
	typ := reflect.TypeOf(websterengine.Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" {
			t.Fatalf("Config field %q has no yaml tag", typ.Field(i).Name)
		}
		if !containsKey(text, tag) {
			t.Errorf("ConfigTemplate() does not contain key %q for field %q", tag, typ.Field(i).Name)
		}
	}
}

// containsKey reports whether text contains a "<key>:" line-start token,
// the shape every one of this template's keys takes.
func containsKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}

// TestLoadConfig pins how LoadConfig resolves a hub webster.yaml:
// the template seeded verbatim loads its defaults, overrides round-trip, a file still carrying the retired poll_wait_s key keeps loading (the
// unmarshal ignores an unknown key), a hand-written file missing numeric knobs loads them at the
// template defaults rather than Go's zero value (RecoveryTimeoutMin at 0 would classify every
// recovery batch dead/timeout on the first poll), an uninitialized _lyx/ degrades to the embedded
// template, a malformed role model-spec fails loud naming the offending key, and webster's own
// positive-integer check still refuses a knob the file sets to zero, which the template fill never touches.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	templateDefaults := websterengine.Config{
		Master:             "sonnet[medium]",
		Recovery:           "opus[high]",
		SelfFixCap:         2,
		MasterTimeoutMin:   480,
		RecoveryTimeoutMin: 60,
		VerifyGateAttempts: 3,
	}
	absentKnobs := websterengine.Config{
		Master:             "sonnet",
		Recovery:           "opus",
		SelfFixCap:         2,
		MasterTimeoutMin:   480,
		RecoveryTimeoutMin: 60,
		VerifyGateAttempts: 3,
	}
	tests := []struct {
		name string
		// body is the webster.yaml content; uninitialized skips seeding any _lyx/ at all.
		body          string
		uninitialized bool
		want          websterengine.Config
		wantErrKey    string
	}{
		{
			name: "overrides round-trip",
			body: "master: opus[effort=high]\nrecovery: opus[effort=max]\nself_fix_cap: 5\nmaster_timeout_min: 120\nrecovery_timeout_min: 30\nverify_gate_attempts: 4\n",
			want: websterengine.Config{
				Master:             "opus[effort=high]",
				Recovery:           "opus[effort=max]",
				SelfFixCap:         5,
				MasterTimeoutMin:   120,
				RecoveryTimeoutMin: 30,
				VerifyGateAttempts: 4,
			},
		},
		{
			name: "the retired poll_wait_s key still loads",
			body: "master: sonnet\nrecovery: opus\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\nverify_gate_attempts: 3\npoll_wait_s: 480\n",
			want: absentKnobs,
		},
		{
			name: "every numeric knob absent loads the template defaults",
			body: "master: sonnet\nrecovery: opus\n",
			want: absentKnobs,
		},
		{
			name: "recovery_timeout_min absent loads the template default",
			body: "master: sonnet\nrecovery: opus\nself_fix_cap: 2\nmaster_timeout_min: 480\nverify_gate_attempts: 3\n",
			want: absentKnobs,
		},
		{
			name: "verify_gate_attempts absent loads the template default",
			body: "master: sonnet\nrecovery: opus\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\n",
			want: absentKnobs,
		},
		{
			name: "the template seeded verbatim loads its defaults",
			body: websterengine.ConfigTemplate(),
			want: templateDefaults,
		},
		{
			// Do NOT create _lyx/ — LoadConfig must degrade to the embedded template.
			name:          "an uninitialized hub falls back to the template",
			uninitialized: true,
			want:          templateDefaults,
		},
		{
			// "opus " has a trailing space — Parse rejects whitespace anywhere in a spec string.
			name:       "a bad role grammar names the key",
			body:       "master: sonnet\nrecovery: \"opus \"\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\nverify_gate_attempts: 3\n",
			wantErrKey: "recovery",
		},
		{
			name:       "verify_gate_attempts explicitly zero names the key",
			body:       "master: sonnet\nrecovery: opus\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\nverify_gate_attempts: 0\n",
			wantErrKey: "verify_gate_attempts",
		},
		{
			name:       "verify_gate_attempts negative names the key",
			body:       "master: sonnet\nrecovery: opus\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\nverify_gate_attempts: -1\n",
			wantErrKey: "verify_gate_attempts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			if !tt.uninitialized {
				seedConfig(t, baseDir, "webster", tt.body)
			}

			cfg, err := websterengine.LoadConfig(baseDir, "webster")
			if tt.wantErrKey != "" {
				if err == nil {
					t.Fatal("LoadConfig() = nil error; want an error naming the offending key")
				}
				if !strings.Contains(err.Error(), tt.wantErrKey) {
					t.Errorf("LoadConfig() error = %q; want it to name the offending key %q", err.Error(), tt.wantErrKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if cfg != tt.want {
				t.Errorf("LoadConfig() = %+v; want %+v", cfg, tt.want)
			}
		})
	}
}
