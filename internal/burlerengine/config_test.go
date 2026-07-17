// config_test.go table-drives LoadConfig and ResolveFan, and pins the
// embedded template.yaml's self-consistency: every fan entry names a defined
// lens, the two seeded fan lengths, and the emphasis-never-exclusion posture
// (no lens text carries "ignore " phrasing). Untagged Tier-1: no process
// spawns, fixtures written directly via os.WriteFile under t.TempDir.

package burlerengine

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// writeBurlerYAML writes contents to the burler.yaml path under baseDir,
// creating the _lyx/config directory tree first. Mirrors modelspec's
// load_test.go writeModelsYAML helper.
func writeBurlerYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := hubgeometry.ConfigFile(baseDir, "burler")
	if err := os.MkdirAll(hubgeometry.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", hubgeometry.ConfigDir(baseDir), err)
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

	cfg, err := LoadConfig(baseDir)
	if err != nil {
		t.Fatalf("LoadConfig(seeded template) returned unexpected error: %v", err)
	}
	return cfg
}

func TestLoadConfig_AbsentFileYieldsZeroConfig(t *testing.T) {
	baseDir := t.TempDir()

	cfg, err := LoadConfig(baseDir)
	if err != nil {
		t.Fatalf("LoadConfig(absent) returned unexpected error: %v", err)
	}
	if len(cfg.Lenses) != 0 || len(cfg.Fans) != 0 {
		t.Errorf("LoadConfig(absent) = %+v; want zero Config", cfg)
	}
}

func TestLoadConfig_EmptyFileYieldsZeroConfig(t *testing.T) {
	baseDir := t.TempDir()
	writeBurlerYAML(t, baseDir, "# comments only, no entries\n")

	cfg, err := LoadConfig(baseDir)
	if err != nil {
		t.Fatalf("LoadConfig(comments-only) returned unexpected error: %v", err)
	}
	if len(cfg.Lenses) != 0 || len(cfg.Fans) != 0 {
		t.Errorf("LoadConfig(comments-only) = %+v; want zero Config", cfg)
	}
}

func TestLoadConfig_RejectsUnknownTopLevelField(t *testing.T) {
	baseDir := t.TempDir()
	writeBurlerYAML(t, baseDir, "weight: 5\n")

	_, err := LoadConfig(baseDir)
	if err == nil {
		t.Fatal("LoadConfig(unknown field) returned nil error; want a decode error")
	}
	if !strings.HasPrefix(err.Error(), "burler: ") {
		t.Errorf("LoadConfig(unknown field) error = %q; want prefix \"burler: \"", err.Error())
	}
}

// TestConfigTemplate_DecodesThroughLoadConfig proves the embedded seed
// template is itself valid burler.yaml content — it must decode cleanly
// through LoadConfig's own strict decode path, the same one every real hub's
// seeded file goes through.
func TestConfigTemplate_DecodesThroughLoadConfig(t *testing.T) {
	cfg := loadSeededConfig(t)

	const wantLenses = 9
	if len(cfg.Lenses) != wantLenses {
		t.Errorf("seeded template has %d lenses; want %d", len(cfg.Lenses), wantLenses)
	}
	const wantFans = 2
	if len(cfg.Fans) != wantFans {
		t.Errorf("seeded template has %d fans; want %d", len(cfg.Fans), wantFans)
	}
	for _, name := range []string{"standard", "full"} {
		if _, ok := cfg.Fans[name]; !ok {
			t.Errorf("seeded template is missing fan %q", name)
		}
	}
}

// TestConfigTemplate_SelfConsistency asserts the seeded template's internal
// cross-references and lengths hold, and that no lens carries hard-exclusion
// phrasing — the spike (docs/research/session-fork-spike.md, Q2) found "ignore
// everything else" lenses measurably suppressed cross-category coverage, so
// every lens is emphasis-only by design.
func TestConfigTemplate_SelfConsistency(t *testing.T) {
	cfg := loadSeededConfig(t)

	for fanName, entries := range cfg.Fans {
		for _, lensName := range entries {
			if _, ok := cfg.Lenses[lensName]; !ok {
				t.Errorf("fan %q names undefined lens %q", fanName, lensName)
			}
		}
	}

	if got := len(cfg.Fans["standard"]); got != 5 {
		t.Errorf("fan \"standard\" has %d entries; want 5", got)
	}
	if got := len(cfg.Fans["full"]); got != 8 {
		t.Errorf("fan \"full\" has %d entries; want 8", got)
	}

	for name, text := range cfg.Lenses {
		if strings.TrimSpace(text) == "" {
			t.Errorf("lens %q has empty text", name)
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

	t.Run("zero-Config fan lookup mentions reconcile", func(t *testing.T) {
		_, err := ResolveFan(Config{}, "standard")
		if err == nil {
			t.Fatal("ResolveFan(zero Config) returned nil error")
		}
		requireContains(t, err.Error(), "reconcile")
	})
}
