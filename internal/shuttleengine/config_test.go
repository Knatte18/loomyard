// config_test.go verifies shuttle.yaml's template parses, defaults resolve through LoadConfig, and
// environment overrides + the template-fallback path behave the way reedengine's config tests
// establish the pattern.

package shuttleengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// seedLyxConfig creates <tmpDir>/_lyx/config/<module>.yaml with content, the
// minimal on-disk shape LoadConfig needs (no git repository required).
func seedLyxConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	configFile := configengine.ConfigFile(tmpDir, module)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestLoadConfig_TemplateDefaultsResolve also pins that shuttle.yaml carries no Claude key.
//
//testtiming:keep pins every shipped template default, which the BackgroundShellWaitMin table does not assert
func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Seed the config file with the template itself: this is exactly the
	// file "lyx config reconcile" would produce, so LoadConfig must accept
	// it verbatim and every default must resolve.
	seedLyxConfig(t, tmpDir, "shuttle", shuttleengine.ConfigTemplate())

	cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RunDir != "" {
		t.Errorf("RunDir = %q, want empty default", cfg.RunDir)
	}
	if cfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000", cfg.PollIntervalMS)
	}
	if cfg.LivenessEveryNPolls != 10 {
		t.Errorf("LivenessEveryNPolls = %d, want 10", cfg.LivenessEveryNPolls)
	}
	if cfg.RunTimeoutMin != 30 {
		t.Errorf("RunTimeoutMin = %d, want 30", cfg.RunTimeoutMin)
	}
	if cfg.StartupTimeoutS != 90 {
		t.Errorf("StartupTimeoutS = %d, want 90", cfg.StartupTimeoutS)
	}
	if cfg.BackgroundShellWaitMin != 10 {
		t.Errorf("BackgroundShellWaitMin = %d, want 10", cfg.BackgroundShellWaitMin)
	}
	if cfg.SubmitRedrawSettleMS != 300 {
		t.Errorf("SubmitRedrawSettleMS = %d, want 300", cfg.SubmitRedrawSettleMS)
	}
	if cfg.SubmitSettleMS != 300 {
		t.Errorf("SubmitSettleMS = %d, want 300", cfg.SubmitSettleMS)
	}
	if cfg.SendReadyTimeoutS != 60 {
		t.Errorf("SendReadyTimeoutS = %d, want 60", cfg.SendReadyTimeoutS)
	}
	if cfg.SubmitConfirmTimeoutS != 30 {
		t.Errorf("SubmitConfirmTimeoutS = %d, want 30", cfg.SubmitConfirmTimeoutS)
	}
	if cfg.Engine != "claude" {
		t.Errorf("Engine = %q, want claude", cfg.Engine)
	}
	if !cfg.DenyPython {
		t.Error("DenyPython = false, want true")
	}
	for _, line := range strings.Split(shuttleengine.ConfigTemplate(), "\n") {
		if strings.HasPrefix(line, "claude") {
			t.Errorf("template holds a top-level key starting with claude: %q", line)
		}
	}

	t.Run("RepoDefaultResolvesBeneathSparseWorktreeFile", func(t *testing.T) {
		board, anchor := t.TempDir(), t.TempDir()
		seedLyxConfig(t, board, "shuttle", strings.Replace(shuttleengine.ConfigTemplate(), "run_timeout_min: 30", "run_timeout_min: 45", 1))
		seedLyxConfig(t, anchor, "shuttle", "startup_timeout_s: 120\n")

		cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "shuttle")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RunTimeoutMin != 45 {
			t.Errorf("RunTimeoutMin = %d, want 45 (repo default)", cfg.RunTimeoutMin)
		}
		if cfg.StartupTimeoutS != 120 {
			t.Errorf("StartupTimeoutS = %d, want 120 (worktree file)", cfg.StartupTimeoutS)
		}
	})

	t.Run("LocalGuardKeyIsRefused", func(t *testing.T) {
		anchor := t.TempDir()
		localFile := configengine.LocalFile(anchor, "shuttle")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("mkdir local dir: %v", err)
		}
		if err := os.WriteFile(localFile, []byte("deny_python: false\n"), 0o644); err != nil {
			t.Fatalf("write local file: %v", err)
		}

		_, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "shuttle")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if !strings.Contains(err.Error(), localFile) || !strings.Contains(err.Error(), `"deny_python"`) {
			t.Errorf("err = %q, want it to name %s and key deny_python", err, localFile)
		}
	})
}

// TestLoadConfig_RetiredClaudeKeys pins that the Claude keys moved to claude.yaml still load from an unreconciled layer and are dropped.
func TestLoadConfig_RetiredClaudeKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		localBody string
		wantErr   string
	}{
		{name: "retired TTL roles map in the local layer loads", localBody: "claude_prompt_cache_ttl_roles:\n  a: 1h\n  b: 5m\n"},
		{name: "retired TTL map in the local layer loads", localBody: "claude_prompt_cache_ttl: {a: 1}\n"},
		{name: "retired executable key in the local layer is refused", localBody: "claude: /opt/claude\n", wantErr: `"claude"`},
		{name: "retired guard key in the local layer is refused", localBody: "claude_deny_agent_tool: false\n", wantErr: `"claude_deny_agent_tool"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			anchor := t.TempDir()
			localFile := configengine.LocalFile(anchor, "shuttle")
			if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
				t.Fatalf("mkdir local dir: %v", err)
			}
			if err := os.WriteFile(localFile, []byte(tt.localBody), 0o644); err != nil {
				t.Fatalf("write local file: %v", err)
			}
			dirs := configengine.Dirs{Anchor: anchor}

			_, err := shuttleengine.LoadConfig(dirs, "shuttle")
			if tt.wantErr != "" {
				if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadConfig err = %v, want ErrInvalid naming %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}

			merged, err := configengine.LoadOrTemplate(dirs, shuttleengine.ConfigSpec())
			if err != nil {
				t.Fatalf("LoadOrTemplate: %v", err)
			}
			if strings.Contains(string(merged), "claude_prompt_cache_ttl") {
				t.Errorf("merged document keeps a retired key:\n%s", merged)
			}
		})
	}

	t.Run("worktree file holding every retired key loads with the python switch at its default", func(t *testing.T) {
		t.Parallel()

		anchor := t.TempDir()
		seedLyxConfig(t, anchor, "shuttle", shuttleengine.ConfigTemplate()+
			"claude: /opt/claude\nclaude_deny_agent_tool: false\nclaude_deny_ask_user_question: false\n"+
			"claude_prompt_cache_ttl: 1h\nclaude_prompt_cache_ttl_roles:\n  orch: 1h\nclaude_deny_python: false\n")

		cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "shuttle")
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if !cfg.DenyPython {
			t.Error("DenyPython = false, want true: claude_deny_python is renamed by a reconcile walk, not read by the loader")
		}
	})
}

// TestMigrateConfig_RenamesDenyPython pins the claude_deny_python to deny_python rename, which keeps the value and comment.
func TestMigrateConfig_RenamesDenyPython(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantDoc   string
		wantLines []string
	}{
		{
			name:      "rename keeps value and comment",
			in:        "run_timeout_min: 30\nclaude_deny_python: false  # off here\nengine: claude\n",
			wantDoc:   "run_timeout_min: 30\ndeny_python: false # off here\nengine: claude\n",
			wantLines: []string{"claude_deny_python: renamed to deny_python"},
		},
		{
			name:    "document without the key is unchanged",
			in:      "run_timeout_min: 30\n",
			wantDoc: "run_timeout_min: 30\n",
		},
		{
			name:      "both keys keep deny_python and report the dropped value",
			in:        "deny_python: true\nclaude_deny_python: false\n",
			wantDoc:   "deny_python: true\n",
			wantLines: []string{"claude_deny_python: false dropped, deny_python already set"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, lines, err := shuttleengine.MigrateConfig([]byte(tt.in))
			if err != nil {
				t.Fatalf("MigrateConfig: %v", err)
			}
			if string(got) != tt.wantDoc {
				t.Errorf("MigrateConfig document = %q, want %q", got, tt.wantDoc)
			}
			if !slices.Equal(lines, tt.wantLines) {
				t.Errorf("MigrateConfig lines = %q, want %q", lines, tt.wantLines)
			}

			again, againLines, err := shuttleengine.MigrateConfig(got)
			if err != nil {
				t.Fatalf("second MigrateConfig: %v", err)
			}
			if string(again) != string(got) || len(againLines) != 0 {
				t.Errorf("second pass = %q with lines %q, want the first pass's output and no lines", again, againLines)
			}
		})
	}
}

func TestLoadConfig_EngineKey(t *testing.T) {
	t.Parallel()

	const templateLine = "engine: claude"
	tests := []struct {
		name       string
		engineLine string
		want       string
		wantErr    string
	}{
		{name: "override loads", engineLine: "engine: other", want: "other"},
		{name: "absent key resolves the template engine", engineLine: "", want: "claude"},
		{name: "empty value refused", engineLine: `engine: ""`, wantErr: "engine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			// The template line carries a trailing comment, so the row replaces the whole line.
			var seeded []string
			for _, line := range strings.Split(shuttleengine.ConfigTemplate(), "\n") {
				if !strings.HasPrefix(line, templateLine) {
					seeded = append(seeded, line)
					continue
				}
				if tt.engineLine != "" {
					seeded = append(seeded, tt.engineLine)
				}
			}
			seedLyxConfig(t, tmpDir, "shuttle", strings.Join(seeded, "\n"))

			cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadConfig accepted %q", tt.engineLine)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not name the %s key", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Engine != tt.want {
				t.Errorf("Engine = %q, want %q", cfg.Engine, tt.want)
			}
		})
	}
}

func TestLoadConfig_BackgroundShellWaitMin(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "positive override loads", value: "25", want: 25},
		{name: "zero refused", value: "0", wantErr: true},
		{name: "negative refused", value: "-5", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seeded := strings.Replace(shuttleengine.ConfigTemplate(), "background_shell_wait_min: 10", "background_shell_wait_min: "+tt.value, 1)
			seedLyxConfig(t, tmpDir, "shuttle", seeded)

			cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig accepted background_shell_wait_min: %s", tt.value)
				}
				if !strings.Contains(err.Error(), "background_shell_wait_min") || !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not name the key and value %s", err, tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.BackgroundShellWaitMin != tt.want {
				t.Errorf("BackgroundShellWaitMin = %d, want %d", cfg.BackgroundShellWaitMin, tt.want)
			}
		})
	}
}

func TestLoadConfig_SubmitSettleKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		def     string
		value   string
		want    int
		wantErr bool
		got     func(shuttleengine.Config) int
	}{
		{name: "submit settle positive override loads", key: "submit_settle_ms", value: "150", want: 150, got: func(c shuttleengine.Config) int { return c.SubmitSettleMS }},
		{name: "submit settle zero loads", key: "submit_settle_ms", value: "0", want: 0, got: func(c shuttleengine.Config) int { return c.SubmitSettleMS }},
		{name: "submit settle negative refused", key: "submit_settle_ms", value: "-1", wantErr: true},
		{name: "redraw settle positive override loads", key: "submit_redraw_settle_ms", value: "150", want: 150, got: func(c shuttleengine.Config) int { return c.SubmitRedrawSettleMS }},
		{name: "redraw settle zero loads", key: "submit_redraw_settle_ms", value: "0", want: 0, got: func(c shuttleengine.Config) int { return c.SubmitRedrawSettleMS }},
		{name: "redraw settle negative refused", key: "submit_redraw_settle_ms", value: "-1", wantErr: true},
		{name: "send ready timeout positive override loads", key: "send_ready_timeout_s", def: "60", value: "5", want: 5, got: func(c shuttleengine.Config) int { return c.SendReadyTimeoutS }},
		{name: "send ready timeout zero refused", key: "send_ready_timeout_s", def: "60", value: "0", wantErr: true},
		{name: "send ready timeout negative refused", key: "send_ready_timeout_s", def: "60", value: "-1", wantErr: true},
		{name: "submit confirm timeout positive override loads", key: "submit_confirm_timeout_s", def: "30", value: "5", want: 5, got: func(c shuttleengine.Config) int { return c.SubmitConfirmTimeoutS }},
		{name: "submit confirm timeout zero refused", key: "submit_confirm_timeout_s", def: "30", value: "0", wantErr: true},
		{name: "submit confirm timeout negative refused", key: "submit_confirm_timeout_s", def: "30", value: "-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := tt.def
			if def == "" {
				def = "300"
			}
			tmpDir := t.TempDir()
			seeded := strings.Replace(shuttleengine.ConfigTemplate(), tt.key+": "+def, tt.key+": "+tt.value, 1)
			seedLyxConfig(t, tmpDir, "shuttle", seeded)

			cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadConfig accepted %s: %s", tt.key, tt.value)
				}
				if !strings.Contains(err.Error(), tt.key) || !strings.Contains(err.Error(), tt.value) {
					t.Errorf("error %q does not name the key and value %s", err, tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := tt.got(cfg); got != tt.want {
				t.Errorf("%s = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tmpDir := t.TempDir()
	// Seed under a non-"shuttle" module name with a config whose
	// poll_interval_ms differs from the template default, so a hardcoded
	// module name would be caught either way: this module reads back its
	// seeded value, and the never-seeded "shuttle" module reads back the
	// template default instead.
	seeded := strings.Replace(shuttleengine.ConfigTemplate(), "poll_interval_ms: 1000", "poll_interval_ms: 1500", 1)
	seedLyxConfig(t, tmpDir, "othershuttle", seeded)

	logs := logcapture.Capture(t)
	cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "othershuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollIntervalMS != 1500 {
		t.Errorf("PollIntervalMS = %d, want 1500 (seeded value)", cfg.PollIntervalMS)
	}
	if strings.Contains(logs.String(), "poll_interval_ms") {
		t.Errorf("a poll interval at or above the floor warned: %q", logs.String())
	}

	// A module seeded below the floor loads, logging the one warning that names the key and the floor.
	below := strings.Replace(shuttleengine.ConfigTemplate(), "poll_interval_ms: 1000", "poll_interval_ms: 250", 1)
	if err := os.WriteFile(configengine.ConfigFile(tmpDir, "belowshuttle"), []byte(below), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	logs.Reset()
	if _, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "belowshuttle"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(logs.String(), "level=WARN"); got != 1 || !strings.Contains(logs.String(), "key=poll_interval_ms") || !strings.Contains(logs.String(), "floor_ms=1000") {
		t.Errorf("below-floor load logged %q, want one warning naming poll_interval_ms and the floor", logs.String())
	}

	// The never-seeded "shuttle" module must fall back to the template
	// default, not the "othershuttle" module's seeded value.
	defaultCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000 (template default)", defaultCfg.PollIntervalMS)
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ -- LoadConfig must degrade to the embedded template.

	cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "shuttle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PollIntervalMS != 1000 {
		t.Errorf("PollIntervalMS = %d, want 1000", cfg.PollIntervalMS)
	}
	if cfg.LivenessEveryNPolls != 10 {
		t.Errorf("LivenessEveryNPolls = %d, want 10", cfg.LivenessEveryNPolls)
	}
	if cfg.RunTimeoutMin != 30 {
		t.Errorf("RunTimeoutMin = %d, want 30", cfg.RunTimeoutMin)
	}
	if cfg.StartupTimeoutS != 90 {
		t.Errorf("StartupTimeoutS = %d, want 90", cfg.StartupTimeoutS)
	}
	if !cfg.DenyPython {
		t.Error("DenyPython = false, want true")
	}
}
