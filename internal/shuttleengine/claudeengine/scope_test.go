// scope_test.go covers the session scope: the scope keys each kind of session gets in its settings document.
// It also covers reading Claude Code's installed plugins.
// It also covers the scope reaching the settings that Prepare and OpenLaunch write.

package claudeengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scopeSettingsKeys are the settings keys a session scope writes.
var scopeSettingsKeys = []string{"enabledPlugins", "skillOverrides", "disableClaudeAiConnectors", "disableBundledSkills", "disableWorkflows", "autoMemoryEnabled", "permissions"}

// scopeKeysOf returns the scope keys a settings document holds, with their values.
func scopeKeysOf(t *testing.T, data []byte) map[string]any {
	t.Helper()
	doc := parseSettings(t, data)
	got := map[string]any{}
	for _, key := range scopeSettingsKeys {
		if value, ok := doc[key]; ok {
			got[key] = value
		}
	}
	return got
}

// writeInstalledPlugins writes a Claude Code config directory whose plugins file names each id in ids with one install.
// It returns the directory.
func writeInstalledPlugins(t *testing.T, ids []string) string {
	t.Helper()
	configDir := t.TempDir()
	plugins := map[string][]map[string]string{}
	for _, id := range ids {
		plugins[id] = []map[string]string{{"scope": "user", "installPath": "/plugins/" + id}}
	}
	data, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		t.Fatalf("marshal installed plugins: %v", err)
	}
	path := filepath.Join(configDir, installedPluginsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write installed plugins: %v", err)
	}
	return configDir
}

// writeMalformedPlugins writes a Claude Code config directory whose plugins file is not valid JSON.
// It returns the directory.
func writeMalformedPlugins(t *testing.T) string {
	t.Helper()
	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, filepath.Dir(installedPluginsPath)), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, installedPluginsPath), []byte("{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return configDir
}

// TestSessionScope_WrittenSettingsPerRole pins the scope keys of the settings document each kind of session gets:
// the orch, which keeps its memory and the ListAgents tool;
// an opened session with no skills;
// the board session with the ly plugin and scribe skills;
// a darn writer with its language's skills; and a scope switched off.
func TestSessionScope_WrittenSettingsPerRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	installed := []string{"ly@loomyard", "mill@millhouse", "prowler@scribe", "scribe@scribe"}
	enabled := ScopeConfig{Enabled: true, SkillsOff: []string{"anthropic-skills:docx", "scribe:prose"}}
	tests := []struct {
		name      string
		cfg       ScopeConfig
		preloaded []string
		orch      bool
		want      string
	}{
		{
			name:      "Orch",
			cfg:       enabled,
			preloaded: []string{"scribe:prose", "scribe:conversation", "ly:board", "ly:board-policy"},
			orch:      true,
			want:      `{"enabledPlugins":{"ly@loomyard":true,"mill@millhouse":false,"prowler@scribe":false,"scribe@scribe":true},"skillOverrides":{"anthropic-skills:docx":"off"},"disableClaudeAiConnectors":true,"disableBundledSkills":true,"disableWorkflows":true}`,
		},
		{
			name: "OpenedSessionWithoutSkills",
			cfg:  enabled,
			want: `{"enabledPlugins":{"ly@loomyard":false,"mill@millhouse":false,"prowler@scribe":false,"scribe@scribe":false},"skillOverrides":{"anthropic-skills:docx":"off","scribe:prose":"off"},"disableClaudeAiConnectors":true,"disableBundledSkills":true,"disableWorkflows":true,"autoMemoryEnabled":false,"permissions":{"deny":["ListAgents"]}}`,
		},
		{
			name:      "BoardSession",
			cfg:       enabled,
			preloaded: []string{"scribe:prose", "scribe:conversation", "ly:board-policy", "ly:board"},
			want:      `{"enabledPlugins":{"ly@loomyard":true,"mill@millhouse":false,"prowler@scribe":false,"scribe@scribe":true},"skillOverrides":{"anthropic-skills:docx":"off"},"disableClaudeAiConnectors":true,"disableBundledSkills":true,"disableWorkflows":true,"autoMemoryEnabled":false,"permissions":{"deny":["ListAgents"]}}`,
		},
		{
			name:      "DarnWriter",
			cfg:       enabled,
			preloaded: []string{"scribe:prose", "scribe:golang-testing"},
			want:      `{"enabledPlugins":{"ly@loomyard":false,"mill@millhouse":false,"prowler@scribe":false,"scribe@scribe":true},"skillOverrides":{"anthropic-skills:docx":"off"},"disableClaudeAiConnectors":true,"disableBundledSkills":true,"disableWorkflows":true,"autoMemoryEnabled":false,"permissions":{"deny":["ListAgents"]}}`,
		},
		{
			name:      "ScopeOff",
			preloaded: []string{"scribe:prose"},
			want:      `{}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scope := resolveSessionScope(tt.cfg, installed, tt.preloaded, tt.orch)
			data, err := buildSettings("/c/run/events.jsonl", tt.orch, shuttleengine.Config{}, Config{}, false, false, "", "", scope)
			if err != nil {
				t.Fatalf("buildSettings() error: %v", err)
			}
			var want map[string]any
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if got := scopeKeysOf(t, data); !reflect.DeepEqual(got, want) {
				t.Errorf("scope keys = %v; want %v", got, want)
			}
		})
	}
}

// TestReadInstalledPlugins pins the reading of Claude Code's plugins file: the id of each plugin, sorted.
// An absent file reads as no plugins, and a malformed one is an error.
func TestReadInstalledPlugins(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	malformed := writeMalformedPlugins(t)
	tests := []struct {
		name      string
		configDir string
		want      []string
		wantErr   bool
	}{
		{
			name:      "NamesEachPluginSorted",
			configDir: writeInstalledPlugins(t, []string{"scribe@scribe", "bare@market"}),
			want:      []string{"bare@market", "scribe@scribe"},
		},
		{name: "AbsentFileNamesNone", configDir: t.TempDir()},
		{name: "MalformedFileIsAnError", configDir: malformed, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := readInstalledPlugins(tt.configDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readInstalledPlugins() error = %v; want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) && !(len(got) == 0 && len(tt.want) == 0) {
				t.Errorf("readInstalledPlugins() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// TestClaudeConfigDir pins the config directory's resolution.
// The directory CLAUDE_CONFIG_DIR names wins; otherwise it is .claude under the home directory.
func TestClaudeConfigDir(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	override := t.TempDir()
	tests := []struct {
		name     string
		override string
		want     string
	}{
		{"EnvironmentOverride", override, override},
		{"HomeDefault", "", filepath.Join(home, claudeConfigDirName)},
	}
	for _, tt := range tests {
		// t.Setenv changes process-global state, so these subtests are not parallel.
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv(claudeConfigDirEnv, tt.override)
			got, err := claudeConfigDir()
			if err != nil {
				t.Fatalf("claudeConfigDir() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("claudeConfigDir() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestSessionScope_ReachesWrittenSettings pins that Prepare and OpenLaunch write the scope.
// That scope is the one the shipped claude.yaml turns on.
// A run's and an opened session's plugins follow their preloaded skills, and their auto-memory is off.
// An opened orch session's auto-memory is left alone.
// If Prepare cannot read the plugins file, connectors, bundled skills and the Workflow tool are off,
// and no plugin is named.
func TestSessionScope_ReachesWrittenSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	installed := []string{"ly@loomyard", "scribe@scribe"}

	t.Run("Prepare", func(t *testing.T) {
		t.Parallel()
		engine, _ := engineWithAnchor(t)
		engine.claudeDir = writeInstalledPlugins(t, installed)
		runDir := t.TempDir()
		if _, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing", Skills: []string{"scribe:prose"}}, shuttleengine.Config{}); err != nil {
			t.Fatalf("Prepare() error: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(runDir, settingsFileName))
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		got := scopeKeysOf(t, data)
		if want := map[string]any{"ly@loomyard": false, "scribe@scribe": true}; !reflect.DeepEqual(got["enabledPlugins"], want) {
			t.Errorf("enabledPlugins = %v; want %v", got["enabledPlugins"], want)
		}
		if got["autoMemoryEnabled"] != false {
			t.Errorf("autoMemoryEnabled = %v; want false for an autonomous run", got["autoMemoryEnabled"])
		}
	})

	t.Run("PrepareWithUnreadablePluginsFile", func(t *testing.T) {
		t.Parallel()
		engine, _ := engineWithAnchor(t)
		engine.claudeDir = writeMalformedPlugins(t)
		runDir := t.TempDir()
		if _, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing", Skills: []string{"scribe:prose"}}, shuttleengine.Config{}); err != nil {
			t.Fatalf("Prepare() error: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(runDir, settingsFileName))
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		got := scopeKeysOf(t, data)
		for _, key := range []string{"disableClaudeAiConnectors", "disableBundledSkills", "disableWorkflows"} {
			if got[key] != true {
				t.Errorf("%s = %v; want true", key, got[key])
			}
		}
		if _, ok := got["enabledPlugins"]; ok {
			t.Errorf("enabledPlugins = %v; want it absent when no plugin is known", got["enabledPlugins"])
		}
	})

	openLaunchTests := []struct {
		name string
		role string
		// wantDenied is whether the settings turn auto-memory off and deny ListAgents.
		wantDenied bool
	}{
		{"OpenLaunch", "board", true},
		{"OpenLaunchOrch", "orch", false},
	}
	for _, tt := range openLaunchTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine, _ := engineWithAnchor(t)
			engine.claudeDir = writeInstalledPlugins(t, installed)
			engine.dirs.Board = t.TempDir()
			dir := filepath.Join(t.TempDir(), "open")
			if _, err := engine.OpenLaunch(dir, shuttleengine.OpenSpec{Role: tt.role, Skills: []string{"ly:board"}}, shuttleengine.Config{}); err != nil {
				t.Fatalf("OpenLaunch() error: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			got := scopeKeysOf(t, data)
			if want := map[string]any{"ly@loomyard": true, "scribe@scribe": false}; !reflect.DeepEqual(got["enabledPlugins"], want) {
				t.Errorf("enabledPlugins = %v; want %v", got["enabledPlugins"], want)
			}
			if _, ok := got["autoMemoryEnabled"]; ok != tt.wantDenied {
				t.Errorf("autoMemoryEnabled present = %v; want %v for role %q", ok, tt.wantDenied, tt.role)
			}
			if _, ok := got["permissions"]; ok != tt.wantDenied {
				t.Errorf("permissions present = %v; want %v for role %q", ok, tt.wantDenied, tt.role)
			}
		})
	}
}
