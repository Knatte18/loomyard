// config_test.go verifies that weaver.yaml's template loads through LoadConfig, that a bad key refuses naming itself,
// and that the default ladder resolves to one model choice per rung.
// It seeds files with plain os.MkdirAll and os.WriteFile against a t.TempDir(), so it spawns nothing.

package weaverengine_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// seedConfig writes content to <baseDir>/_lym/config/<module>.yaml and creates the directories above it.
func seedConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestLoadConfig pins how LoadConfig resolves weaver.yaml:
// the template loads its three defaults, and each invalid key refuses naming itself, marked configengine.ErrInvalid.
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	templateDefaults := weaverengine.Config{
		Ladder:          []string{"sonnet[medium]", "opus[medium]", "opus[high]"},
		FixerTimeoutMin: 60,
		VerifyGate:      "webster",
	}
	tests := []struct {
		name       string
		body       string
		want       weaverengine.Config
		wantErrKey string
	}{
		{name: "the template seeded verbatim loads its defaults", body: weaverengine.ConfigTemplate(), want: templateDefaults},
		{
			name: "an absent key loads the template default",
			body: "fixer_timeout_min: 30\n",
			want: weaverengine.Config{Ladder: templateDefaults.Ladder, FixerTimeoutMin: 30, VerifyGate: "webster"},
		},
		{name: "an unparseable ladder spec names the key", body: "ladder:\n  - \"opus \"\n", wantErrKey: "ladder"},
		{name: "a zero fixer_timeout_min names the key", body: "fixer_timeout_min: 0\n", wantErrKey: "fixer_timeout_min"},
		{name: "an empty verify_gate names the key", body: "verify_gate: \"\"\n", wantErrKey: "verify_gate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			seedConfig(t, baseDir, "weaver", tt.body)

			cfg, err := weaverengine.LoadConfig(configengine.Dirs{Anchor: baseDir}, "weaver")
			if tt.wantErrKey != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrKey) || !errors.Is(err, configengine.ErrInvalid) {
					t.Fatalf("LoadConfig() error = %v; want an ErrInvalid error naming %q", err, tt.wantErrKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(cfg, tt.want) {
				t.Errorf("LoadConfig() = %+v; want %+v", cfg, tt.want)
			}
		})
	}

	t.Run("an uninitialized hub falls back to the template", func(t *testing.T) {
		t.Parallel()

		cfg, err := weaverengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "weaver")
		if err != nil || !reflect.DeepEqual(cfg, templateDefaults) {
			t.Errorf("LoadConfig() = %+v, %v; want the template defaults", cfg, err)
		}
	})
}

// TestResolveLadder pins that the default ladder resolves to one choice per rung in order,
// and that an empty ladder and an unknown alias each refuse naming `lym config weaver`.
func TestResolveLadder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	registry := modelspec.Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
	}
	t.Run("the default ladder resolves in order", func(t *testing.T) {
		t.Parallel()

		cfg, err := weaverengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "weaver")
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		got, err := weaverengine.ResolveLadder(cfg, registry)
		if err != nil {
			t.Fatalf("ResolveLadder() error = %v; want nil", err)
		}
		want := []shuttleengine.ModelChoice{
			{Engine: "claude", Model: "sonnet", Effort: "medium"},
			{Engine: "claude", Model: "opus", Effort: "medium"},
			{Engine: "claude", Model: "opus", Effort: "high"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ResolveLadder() = %+v; want %+v", got, want)
		}
	})

	refusals := []struct {
		name   string
		ladder []string
	}{
		{name: "an empty ladder", ladder: nil},
		{name: "an unknown alias", ladder: []string{"nosuchalias"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name+" refuses", func(t *testing.T) {
			t.Parallel()

			_, err := weaverengine.ResolveLadder(weaverengine.Config{Ladder: tt.ladder}, registry)
			if err == nil || !strings.Contains(err.Error(), "ladder") || !strings.Contains(err.Error(), "lym config weaver") {
				t.Errorf("ResolveLadder() error = %v; want one naming ladder and `lym config weaver`", err)
			}
		})
	}
}
