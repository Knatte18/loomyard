// config_test.go table-drives LoadConfig and ResolveFan,
// and pins the embedded template.yaml's self-consistency: every fan entry names a defined lens, the
// two seeded fan lengths, and the emphasis-never-exclusion posture (no lens text carries "ignore "
// phrasing).
// Untagged Tier-1: no process spawns, fixtures written directly via os.WriteFile under t.TempDir.

package burlerengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// writeBurlerYAML writes contents to the burler.yaml path under baseDir,
// creating the _lyx/config directory tree first. Mirrors modelspec's
// load_test.go writeModelsYAML helper.
func writeBurlerYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := configengine.ConfigFile(baseDir, "burler")
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configengine.ConfigDir(baseDir), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// loadSeededConfig writes ConfigTemplate()'s output as burler.yaml under a
// fresh t.TempDir and loads it back through LoadConfig — the same decode path
// a real hub's seeded file goes through.
func loadSeededConfig(t *testing.T) Config {
	t.Helper()
	baseDir := t.TempDir()
	writeBurlerYAML(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(configengine.Dirs{Anchor: baseDir})
	if err != nil {
		t.Fatalf("LoadConfig(seeded template) returned unexpected error: %v", err)
	}
	return cfg
}

// TestLoadConfig covers LoadConfig's three non-template outcomes: an absent file and a comments-only file both yield the zero Config, and an unknown top-level field is rejected with a burler:-prefixed decode error.
func TestLoadConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		write    bool
		wantErr  bool
	}{
		{name: "absent file yields zero config"},
		{name: "empty file yields zero config", contents: "# comments only, no entries\n", write: true},
		{name: "unknown top-level field is rejected", contents: "weight: 5\n", write: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir := t.TempDir()
			if tt.write {
				writeBurlerYAML(t, baseDir, tt.contents)
			}

			cfg, err := LoadConfig(configengine.Dirs{Anchor: baseDir})

			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadConfig() returned nil error; want a decode error")
				}
				if !strings.HasPrefix(err.Error(), "burler: ") {
					t.Errorf("LoadConfig() error = %q; want prefix \"burler: \"", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() returned unexpected error: %v", err)
			}
			if len(cfg.Lenses) != 0 || len(cfg.Fans) != 0 {
				t.Errorf("LoadConfig() = %+v; want zero Config", cfg)
			}
		})
	}

	t.Run("a fan defined only in the repo default resolves", func(t *testing.T) {
		t.Parallel()
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeBurlerYAML(t, boardDir, "lenses:\n  a: default prose\nfans:\n  mine: [a]\n")

		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("LoadConfig() returned unexpected error: %v", err)
		}
		lenses, err := ResolveFan(cfg, "mine")
		if err != nil || len(lenses) != 1 || lenses[0].Text != "default prose" {
			t.Errorf("ResolveFan(mine) = %+v, %v; want the default's lens a", lenses, err)
		}
	})

	t.Run("a worktree-file lens replaces the repo default's whole", func(t *testing.T) {
		t.Parallel()
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeBurlerYAML(t, boardDir, "lenses:\n  a: default prose\n  b: kept prose\n")
		writeBurlerYAML(t, anchor, "lenses:\n  a: worktree prose\n")

		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("LoadConfig() returned unexpected error: %v", err)
		}
		if cfg.Lenses["a"] != "worktree prose" || cfg.Lenses["b"] != "kept prose" {
			t.Errorf("LoadConfig().Lenses = %v; want a from the worktree file and b from the default", cfg.Lenses)
		}
	})

	t.Run("a local lenses entry fails the load naming the local file", func(t *testing.T) {
		t.Parallel()
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeBurlerYAML(t, boardDir, "lenses:\n  a: default prose\n")
		localFile := configengine.LocalFile(anchor, "burler")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(localFile), err)
		}
		if err := os.WriteFile(localFile, []byte("lenses:\n  a: local prose\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", localFile, err)
		}

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), localFile) {
			t.Errorf("LoadConfig() error = %v; want ErrInvalid naming %s", err, localFile)
		}
	})
}

// TestConfigTemplate_DecodesAndIsSelfConsistent proves the embedded seed template is itself valid burler.yaml content -- it decodes cleanly through LoadConfig's own strict decode path, the same one every real hub's seeded file goes through -- and that its internal cross-references and lengths hold: both seeded fans exist, every fan entry names a defined lens, and no lens carries hard-exclusion phrasing.
// The spike (docs/research/session-fork-spike.md, Q2) found "ignore everything else" lenses measurably suppressed cross-category coverage, so every lens is emphasis-only by design.
//
//testtiming:keep pins the seeded lens and fan counts, the defined-lens cross-references and the no-"ignore " phrasing, which the ResolveFan test covering its blocks does not assert
func TestConfigTemplate_DecodesAndIsSelfConsistent(t *testing.T) {
	t.Parallel()

	cfg := loadSeededConfig(t)

	const wantLenses = 23
	if len(cfg.Lenses) != wantLenses {
		t.Errorf("seeded template has %d lenses; want %d", len(cfg.Lenses), wantLenses)
	}
	const wantFans = 6
	if len(cfg.Fans) != wantFans {
		t.Errorf("seeded template has %d fans; want %d", len(cfg.Fans), wantFans)
	}
	for fanName, entries := range cfg.Fans {
		for _, lensName := range entries {
			if _, ok := cfg.Lenses[lensName]; !ok {
				t.Errorf("fan %q names undefined lens %q", fanName, lensName)
			}
		}
	}
	for fanName, wantEntries := range map[string]int{"standard": 5, "full": 8, "discussion": 5, "plan": 5, "twill-discussion": 6, "twill-plan": 5} {
		entries, ok := cfg.Fans[fanName]
		if !ok {
			t.Errorf("seeded template is missing fan %q", fanName)
		}
		if len(entries) != wantEntries {
			t.Errorf("fan %q has %d entries; want %d", fanName, len(entries), wantEntries)
		}
	}
	wantOrders := map[string][]string{
		"discussion":       {"generic", "goal-scope", "pattern-fit", "scenarios", "leanness"},
		"plan":             {"generic", "decision-coverage", "card-structure", "verify-tests", "pattern-fit"},
		"twill-discussion": {"premises", "goal-scope", "board-fit", "pattern-fit", "scenarios", "leanness"},
		"twill-plan":       {"criteria-coverage", "branch-coverage", "row-reachability", "forced-edits", "test-economy"},
	}
	for fanName, wantOrder := range wantOrders {
		lenses, err := ResolveFan(Config{}, fanName)
		if err != nil {
			t.Fatalf("ResolveFan(%s) returned unexpected error: %v", fanName, err)
		}
		for i, wantName := range wantOrder {
			if lenses[i].Name != wantName {
				t.Errorf("ResolveFan(%s)[%d].Name = %q; want %q", fanName, i, lenses[i].Name, wantName)
			}
		}
	}

	for lensName, wantSentence := range map[string]string{
		"branch-coverage":   "A branch no planned row reaches is a finding.",
		"forced-edits":      "a registration no card lists is a finding",
		"criteria-coverage": "uses the operand the record names, not a near one",
	} {
		if text := strings.Join(strings.Fields(cfg.Lenses[lensName]), " "); !strings.Contains(text, wantSentence) {
			t.Errorf("lens %q text = %q; want it to contain %q", lensName, text, wantSentence)
		}
	}

	const emphasisLine = "Report anything else you notice too — emphasis, never exclusion."
	for name, text := range cfg.Lenses {
		if strings.TrimSpace(text) == "" {
			t.Errorf("lens %q has empty text", name)
		}
		if name != "generic" && !strings.HasSuffix(strings.Join(strings.Fields(text), " "), emphasisLine) {
			t.Errorf("lens %q text does not end with the emphasis line", name)
		}
		if strings.Contains(text, "ignore ") {
			t.Errorf("lens %q text contains hard-exclusion phrasing (\"ignore \"): %q", name, text)
		}
	}
}

func TestResolveFan(t *testing.T) {
	seeded := loadSeededConfig(t)

	custom := Config{
		Lenses: map[string]string{"known": "known lens text"},
		Fans: map[string][]string{
			"unknown-lens": {"missing"},
			"empty":        {},
			"over-cap":     make([]string, maxClusterN+1),
		},
	}
	for i := range custom.Fans["over-cap"] {
		custom.Fans["over-cap"][i] = "known"
	}

	t.Run("happy path preserves order and repeats", func(t *testing.T) {
		lenses, err := ResolveFan(seeded, "standard")
		if err != nil {
			t.Fatalf("ResolveFan(standard) returned unexpected error: %v", err)
		}
		wantOrder := []string{"generic", "generic", "correctness", "error-handling", "test-gaps"}
		if len(lenses) != len(wantOrder) {
			t.Fatalf("ResolveFan(standard) returned %d lenses; want %d", len(lenses), len(wantOrder))
		}
		for i, name := range wantOrder {
			if lenses[i].Name != name {
				t.Errorf("ResolveFan(standard)[%d].Name = %q; want %q", i, lenses[i].Name, name)
			}
			if lenses[i].Text != seeded.Lenses[name] {
				t.Errorf("ResolveFan(standard)[%d].Text does not match cfg.Lenses[%q]", i, name)
			}
		}
		// The two "generic" repeats must be independent entries with
		// identical content, not a single deduplicated one.
		if lenses[0].Name != lenses[1].Name || lenses[0].Text != lenses[1].Text {
			t.Errorf("ResolveFan(standard) repeated lens entries diverge: %+v vs %+v", lenses[0], lenses[1])
		}
	})

	t.Run("unknown fan lists known fans", func(t *testing.T) {
		_, err := ResolveFan(seeded, "nope")
		if err == nil {
			t.Fatal("ResolveFan(unknown fan) returned nil error")
		}
		requireContains(t, err.Error(), "unknown fan")
		requireContains(t, err.Error(), "standard")
		requireContains(t, err.Error(), "full")
	})

	t.Run("unknown lens inside fan", func(t *testing.T) {
		_, err := ResolveFan(custom, "unknown-lens")
		if err == nil {
			t.Fatal("ResolveFan(fan naming undefined lens) returned nil error")
		}
		requireContains(t, err.Error(), "undefined lens")
		requireContains(t, err.Error(), "missing")
	})

	t.Run("empty fan", func(t *testing.T) {
		_, err := ResolveFan(custom, "empty")
		if err == nil {
			t.Fatal("ResolveFan(empty fan) returned nil error")
		}
		requireContains(t, err.Error(), "empty")
	})

	t.Run("fan longer than maxClusterN", func(t *testing.T) {
		_, err := ResolveFan(custom, "over-cap")
		if err == nil {
			t.Fatal("ResolveFan(over-cap fan) returned nil error")
		}
		requireContains(t, err.Error(), "exceeding the maximum")
	})

	t.Run("zero Config resolves from the template", func(t *testing.T) {
		lenses, err := ResolveFan(Config{}, "standard")
		if err != nil {
			t.Fatalf("ResolveFan(zero Config, standard) returned unexpected error: %v", err)
		}
		if want := len(seeded.Fans["standard"]); len(lenses) != want {
			t.Errorf("ResolveFan(zero Config, standard) returned %d lenses; want %d", len(lenses), want)
		}
	})

	t.Run("operator file without the fan resolves it from the template", func(t *testing.T) {
		lenses, err := ResolveFan(custom, "standard")
		if err != nil {
			t.Fatalf("ResolveFan(custom, standard) returned unexpected error: %v", err)
		}
		if lenses[0].Text != seeded.Lenses[lenses[0].Name] {
			t.Errorf("lens %q text does not come from the template", lenses[0].Name)
		}
	})

	t.Run("operator-defined fan and lens win over the template", func(t *testing.T) {
		operator := Config{
			Lenses: map[string]string{"generic": "operator generic"},
			Fans:   map[string][]string{"standard": {"generic"}},
		}
		lenses, err := ResolveFan(operator, "standard")
		if err != nil {
			t.Fatalf("ResolveFan(operator, standard) returned unexpected error: %v", err)
		}
		if len(lenses) != 1 || lenses[0].Text != "operator generic" {
			t.Errorf("ResolveFan(operator, standard) = %+v; want the operator's single generic lens", lenses)
		}
	})

	t.Run("fan undefined in both names the fan", func(t *testing.T) {
		_, err := ResolveFan(Config{}, "nope")
		if err == nil {
			t.Fatal("ResolveFan(zero Config, nope) returned nil error")
		}
		requireContains(t, err.Error(), `"nope"`)
	})
}
