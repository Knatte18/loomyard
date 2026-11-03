// config_test.go verifies reed.yaml's template parses, defaults resolve through LoadConfig, and
// environment overrides + the template-fallback path behave the way shuttleengine's config tests
// establish the pattern.

package reedengine_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// seedLymConfig creates the minimal on-disk config structure for LoadConfig.
func seedLymConfig(t *testing.T, tmpDir, module, content string) {
	t.Helper()
	lymDir := filepath.Join(tmpDir, lymdirs.LymDirName)
	if err := os.Mkdir(lymDir, 0o755); err != nil {
		t.Fatalf("mkdir _lym: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lym/config: %v", err)
	}
	configFile := configengine.ConfigFile(tmpDir, module)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

//testtiming:keep pins every shipped template default through a seeded reed.yaml: tmux and shell per GOOS, width, height, debug_log off, mouse on, window_share_max 2 and window_split width; its covering tests run this code without asserting it
func TestLoadConfig_TemplateDefaultsResolve(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	seedLymConfig(t, tmpDir, "reed", reedengine.ConfigTemplate())

	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantTmux, wantShell := "tmux", "pwsh"
	if runtime.GOOS != "windows" {
		wantTmux, wantShell = "tmux", "bash"
	}
	if cfg.Tmux != wantTmux {
		t.Errorf("Tmux = %q, want %q", cfg.Tmux, wantTmux)
	}
	if cfg.Shell != wantShell {
		t.Errorf("Shell = %q, want %q", cfg.Shell, wantShell)
	}
	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}
	if cfg.Height != 50 {
		t.Errorf("Height = %d, want 50", cfg.Height)
	}
	if cfg.OperatorQuietS != 5 {
		t.Errorf("OperatorQuietS = %d, want 5", cfg.OperatorQuietS)
	}
	if cfg.WindowShareMax != 2 {
		t.Errorf("WindowShareMax = %d, want 2", cfg.WindowShareMax)
	}
	if cfg.WindowSplit != "width" {
		t.Errorf("WindowSplit = %q, want %q", cfg.WindowSplit, "width")
	}
	if cfg.DebugLog != "0" {
		t.Errorf("DebugLog = %q, want %q", cfg.DebugLog, "0")
	}
	// Mouse defaults ON: with it off tmux never claims the wheel, so the terminal's own
	// alternate-screen translation delivers arrow keys into the live agent's input instead of
	// scrolling anything -- which for an interactive producer means an operator's scroll gesture
	// types into the interview.
	if cfg.Mouse != "on" {
		t.Errorf("Mouse = %q, want %q", cfg.Mouse, "on")
	}
	// The package runs under tmuxkit.Main, which unsets LYM_REED_TMUX_CONF, so an operator's own value cannot reach this.
	if cfg.TmuxConf != "" {
		t.Errorf("TmuxConf = %q, want empty", cfg.TmuxConf)
	}
	if !maps.Equal(cfg.SegmentColors, wantSegmentColors) {
		t.Errorf("SegmentColors = %v, want %v", cfg.SegmentColors, wantSegmentColors)
	}

	wantTaskButtons := []buttonWant{
		{Label: "HALT", Run: "lym tender pause {slug}"},
		{Label: "CANCEL", Run: "lym batten cancel {slug}", Confirm: "Cancel {slug}?"},
		{Label: "code", Run: "lym ide spawn {slug}"},
		{Label: "terminal", Run: "lym reed window {worktree} --title {slug}"},
		{Label: "agent", Run: "cd {worktree} && lym shuttle open --role {answer} --model 'opus[high]'", Ask: "Agent name:", AskDefault: "agent"},
	}
	wantBeamButtons := []buttonWant{
		{Label: "code", Run: "lym ide spawn {slug}"},
		{Label: "terminal", Run: "lym reed window {worktree} --title {slug}"},
		{Label: "agent", Run: "cd {worktree} && lym shuttle open --role {answer} --model 'opus[high]'", Ask: "Agent name:", AskDefault: "agent"},
		{Label: "distill", Run: "lym orch distill", Confirm: "Distill the orch?"},
		{Label: "refresh", Run: "lym orch refresh", Confirm: "Refresh the orch?"},
	}
	if got := decodeButtons(t, cfg.Buttons); !reflect.DeepEqual(got, shippedButtons{Task: wantTaskButtons, Beam: wantBeamButtons, Prime: []buttonWant{}}) {
		t.Errorf("Buttons = %+v; want task %+v, beam %+v and an empty prime", got, wantTaskButtons, wantBeamButtons)
	}

	t.Run("BothTemplatesShipTheSameButtons", func(t *testing.T) {
		for _, file := range []string{"template_posix.yaml", "template_windows.yaml"} {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			var doc struct {
				Buttons yaml.Node `yaml:"buttons"`
			}
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal %s: %v", file, err)
			}
			if got := decodeButtons(t, doc.Buttons); !reflect.DeepEqual(got, shippedButtons{Task: wantTaskButtons, Beam: wantBeamButtons, Prime: []buttonWant{}}) {
				t.Errorf("%s buttons = %+v; want the shipped default set", file, got)
			}
		}
	})

	t.Run("ButtonsNullIsRefused", func(t *testing.T) {
		anchor := t.TempDir()
		seedLymConfig(t, anchor, "reed", "buttons: null\n")

		_, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "reed")
		if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), "buttons") {
			t.Fatalf("err = %v; want ErrInvalid naming buttons", err)
		}
	})

	t.Run("GroupWrittenAsMappingLoads", func(t *testing.T) {
		anchor := t.TempDir()
		seedLymConfig(t, anchor, "reed", "buttons: {task: {label: x}}\n")

		if _, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "reed"); err != nil {
			t.Fatalf("err = %v; want nil", err)
		}
	})

	t.Run("GroupReplacesTheDefaultWhole", func(t *testing.T) {
		anchor := t.TempDir()
		seedLymConfig(t, anchor, "reed", "buttons: {task: [{label: x, run: y}]}\n")

		cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "reed")
		if err != nil {
			t.Fatalf("err = %v; want nil", err)
		}
		got := decodeButtons(t, cfg.Buttons)
		if want := []buttonWant{{Label: "x", Run: "y"}}; !reflect.DeepEqual(got.Task, want) {
			t.Errorf("task = %+v; want %+v", got.Task, want)
		}
		if !reflect.DeepEqual(got.Beam, wantBeamButtons) {
			t.Errorf("beam = %+v; want the template default %+v", got.Beam, wantBeamButtons)
		}
	})

	t.Run("LocalButtonsAreRefused", func(t *testing.T) {
		anchor := t.TempDir()
		localFile := configengine.LocalFile(anchor, "reed")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("mkdir local dir: %v", err)
		}
		if err := os.WriteFile(localFile, []byte("buttons: {task: []}\n"), 0o644); err != nil {
			t.Fatalf("write local file: %v", err)
		}

		_, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "reed")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if !strings.Contains(err.Error(), localFile) || !strings.Contains(err.Error(), `"buttons.task"`) {
			t.Errorf("err = %q, want it to name %s and key buttons.task", err, localFile)
		}
	})

	t.Run("RepoDefaultResolvesBeneathSparseWorktreeFile", func(t *testing.T) {
		board, anchor := t.TempDir(), t.TempDir()
		seedLymConfig(t, board, "reed", strings.Replace(reedengine.ConfigTemplate(), "width: 220", "width: 300", 1))
		seedLymConfig(t, anchor, "reed", "height: 40\n")

		cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "reed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Width != 300 {
			t.Errorf("Width = %d, want 300 (repo default)", cfg.Width)
		}
		if cfg.Height != 40 {
			t.Errorf("Height = %d, want 40 (worktree file)", cfg.Height)
		}
	})

	t.Run("LocalExecutableKeyIsRefused", func(t *testing.T) {
		anchor := t.TempDir()
		localFile := configengine.LocalFile(anchor, "reed")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("mkdir local dir: %v", err)
		}
		if err := os.WriteFile(localFile, []byte("shell: other\n"), 0o644); err != nil {
			t.Fatalf("write local file: %v", err)
		}

		_, err := reedengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "reed")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if !strings.Contains(err.Error(), localFile) || !strings.Contains(err.Error(), `"shell"`) {
			t.Errorf("err = %q, want it to name %s and key shell", err, localFile)
		}
	})
}

var wantSegmentColors = map[string]string{
	"coordinator": "blue",
	"discussion":  "purple",
	"plan":        "cyan",
	"webster":     "green",
	"review":      "orange",
	"describe":    "yellow",
	"landing":     "pink",
	"darn":        "red",
	"merge":       "red",
}

// TestLoadConfig_MissingSegmentColorsTakeTemplateDefaults pins that a reed.yaml lacking the whole block, or a single key of it, loads with the template defaults.
func TestLoadConfig_MissingSegmentColorsTakeTemplateDefaults(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{name: "WholeBlockMissing", content: "width: 100\n", want: wantSegmentColors},
		{
			name:    "OneKeyMissing",
			content: "segment_colors:\n  review: red\n",
			want: func() map[string]string {
				colors := maps.Clone(wantSegmentColors)
				colors["review"] = "red"
				return colors
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seedLymConfig(t, tmpDir, "reed", tt.content)

			cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !maps.Equal(cfg.SegmentColors, tt.want) {
				t.Errorf("SegmentColors = %v, want %v", cfg.SegmentColors, tt.want)
			}
		})
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	t.Setenv("LYM_REED_TMUX", `D:\tools\tmux.exe`)
	t.Setenv("LYM_REED_TMUX_CONF", "/home/operator/tmux.conf")
	seedLymConfig(t, tmpDir, "reed", reedengine.ConfigTemplate())

	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tmux != `D:\tools\tmux.exe` {
		t.Errorf("Tmux = %q, want env override", cfg.Tmux)
	}
	// Loading applies no validation, so the value arrives verbatim.
	if cfg.TmuxConf != "/home/operator/tmux.conf" {
		t.Errorf("TmuxConf = %q, want env override", cfg.TmuxConf)
	}
}

func TestLoadConfig_ModuleArgIsThreadedThrough(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	// Seed under a non-"reed" module name with a config whose width differs
	// from the template default, so a hardcoded module name would be caught
	// either way: this module reads back its seeded value, and the
	// never-seeded "reed" module reads back the template default instead.
	seeded := strings.Replace(reedengine.ConfigTemplate(), "width: 220", "width: 300", 1)
	seedLymConfig(t, tmpDir, "otherreed", seeded)

	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "otherreed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Width != 300 {
		t.Errorf("Width = %d, want 300 (seeded value)", cfg.Width)
	}

	// The never-seeded "reed" module must fall back to the template default,
	// not the "otherreed" module's seeded value.
	defaultCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultCfg.Width != 220 {
		t.Errorf("Width = %d, want 220 (template default)", defaultCfg.Width)
	}
}

func TestLoadConfig_UninitializedFallsBackToTemplate(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	// Do NOT create _lym/ -- LoadConfig must degrade to the embedded
	// template. Only assert GOOS-invariant keys: Tmux/Shell differ between
	// template_posix.yaml and template_windows.yaml.

	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Width != 220 {
		t.Errorf("Width = %d, want 220", cfg.Width)
	}
}

// TestLoadConfig_UnknownKeysFromAnOlderConfigAreIgnored pins the no-removal-logic decision.
// An un-reconciled reed.yaml that still carries a stale header: block, the retired strand_name key or the retired status_line block must unmarshal cleanly.
// The old keys are simply ignored, because nothing unmarshals them into Config any more.
//
//testtiming:keep pins an older reed.yaml with a stale header block or the retired strand_name key loading cleanly with the rest of the config undisturbed; its covering tests run this code without asserting it
func TestLoadConfig_UnknownKeysFromAnOlderConfigAreIgnored(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		extra string
		// droppedKeys are the retired keys the merged document must no longer name.
		droppedKeys []string
	}{
		{"StaleHeaderBlock", "\nheader:\n  template: \"stale\"\n  height_rows: 5\n", nil},
		{"RetiredStrandNameKey", "\nstrand_name: '<ROLE>:<ROUND>:<SHORT_GUID>'\n", nil},
		{"RetiredStatusLineBlock", "\nstatus_line:\n  template: \"{{.repo}}\"\n", nil},
		{"RetiredLayoutKeys", "\ncollapsed_rows: 3\nmin_full_rows: 3\nselvage:\n  height_rows: 1\n", []string{"collapsed_rows", "min_full_rows", "selvage"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dirs := configengine.Dirs{Anchor: tmpDir}
			seedLymConfig(t, tmpDir, "reed", reedengine.ConfigTemplate()+tt.extra)

			cfg, err := reedengine.LoadConfig(dirs, "reed")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Width != 220 {
				t.Errorf("Width = %d, want 220 (an old key must not disturb the rest)", cfg.Width)
			}
			if len(tt.droppedKeys) == 0 {
				return
			}
			merged, err := configengine.LoadOrTemplate(dirs, reedengine.ConfigSpec())
			if err != nil {
				t.Fatalf("LoadOrTemplate: %v", err)
			}
			for _, key := range tt.droppedKeys {
				if strings.Contains(string(merged), key+":") {
					t.Errorf("merged document names retired key %q, want it dropped on load:\n%s", key, merged)
				}
			}
		})
	}
}

// buttonWant is one button entry as the shipped default set spells it.
type buttonWant struct {
	Label      string `yaml:"label"`
	Run        string `yaml:"run"`
	Confirm    string `yaml:"confirm"`
	Ask        string `yaml:"ask"`
	AskDefault string `yaml:"ask_default"`
}

// shippedButtons is the buttons mapping decoded into its three groups.
type shippedButtons struct {
	Task  []buttonWant `yaml:"task"`
	Beam  []buttonWant `yaml:"beam"`
	Prime []buttonWant `yaml:"prime"`
}

// decodeButtons decodes a buttons node into its three groups.
func decodeButtons(t *testing.T, node yaml.Node) shippedButtons {
	t.Helper()
	var got shippedButtons
	if err := node.Decode(&got); err != nil {
		t.Fatalf("decode buttons node: %v", err)
	}
	return got
}
