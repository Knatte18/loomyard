// config_test.go table-drives Load, ResolveFan and CheckLoad, and pins the embedded template.yaml's self-consistency:
// every fan entry names a defined lens, the seeded fan lengths and orders, each writer fan holding its reviewer fan's lenses, and the emphasis-never-exclusion posture (no lens text carries "ignore " phrasing).
// Untagged unit tier: no process spawns, fixtures written directly via os.WriteFile under t.TempDir.

package lenses

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// writeLensesYAML writes contents to the lenses.yaml path under baseDir, creating the _lyx/config directory tree first.
func writeLensesYAML(t *testing.T, baseDir, contents string) {
	t.Helper()
	path := configengine.ConfigFile(baseDir, "lenses")
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configengine.ConfigDir(baseDir), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// loadTemplateConfig loads a Config over dirs holding no lenses.yaml, which resolves the embedded template.
func loadTemplateConfig(t *testing.T) Config {
	t.Helper()
	cfg, err := Load(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("Load(no lenses.yaml) returned unexpected error: %v", err)
	}
	return cfg
}

// TestLoad covers Load's layer stack: the per-name fill from the template, per-name layer replacement, the local layer's refusal, the strict decode and the expansion of lens prose.
// Its rows run serially, since logcapture.CaptureVerbose swaps the process-global logger.
func TestLoad(t *testing.T) {
	t.Run("a board default lacking a template lens resolves it with the template's prose", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "lenses:\n  mine: default prose\nfans:\n  mine: [mine, correctness]\n")

		cfg, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		lenses, err := ResolveFan(cfg, "mine")
		if err != nil || len(lenses) != 2 {
			t.Fatalf("ResolveFan(mine) = %+v, %v; want two lenses", lenses, err)
		}
		template := loadTemplateConfig(t)
		if lenses[0].Text != "default prose" || lenses[1].Text != template.Lenses["correctness"] {
			t.Errorf("ResolveFan(mine) texts = %q, %q; want the default's prose and the template's correctness", lenses[0].Text, lenses[1].Text)
		}
	})

	t.Run("a hub-only lens loads and logs no unknown-keys warning", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "lenses:\n  hub-only: hub prose\nfans:\n  hub-fan: [hub-only]\n")
		logs := logcapture.CaptureVerbose(t)

		cfg, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		if cfg.Lenses["hub-only"] != "hub prose" || len(cfg.Fans["hub-fan"]) != 1 {
			t.Errorf("Load() = %+v; want the hub-only lens and fan loaded", cfg)
		}
		if strings.Contains(logs.String(), "keys the template lacks") {
			t.Errorf("Load() logged an unknown-keys warning for a per-name entry: %s", logs.String())
		}
	})

	t.Run("a worktree-layer lens replaces the default's per name", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "lenses:\n  a: default prose\n  b: kept prose\n")
		writeLensesYAML(t, anchor, "lenses:\n  a: worktree prose\n")

		cfg, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		if cfg.Lenses["a"] != "worktree prose" || cfg.Lenses["b"] != "kept prose" {
			t.Errorf("Load().Lenses = %v; want a from the worktree file and b from the default", cfg.Lenses)
		}
	})

	t.Run("a local lenses entry fails the load naming the local file", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "lenses:\n  a: default prose\n")
		localFile := configengine.LocalFile(anchor, "lenses")
		// The worktree needs its _lyx/ for the load to read its layers at all.
		if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", configengine.ConfigDir(anchor), err)
		}
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(localFile), err)
		}
		if err := os.WriteFile(localFile, []byte("lenses:\n  a: local prose\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", localFile, err)
		}

		_, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), localFile) {
			t.Errorf("Load() error = %v; want ErrInvalid naming %s", err, localFile)
		}
	})

	t.Run("an unknown top-level key is refused with the lenses prefix", func(t *testing.T) {
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "weight: 5\n")

		_, err := Load(configengine.Dirs{Anchor: anchor})
		if err == nil || !strings.HasPrefix(err.Error(), "lenses: ") {
			t.Errorf("Load() error = %v; want a decode error with prefix \"lenses: \"", err)
		}
	})

	t.Run("remove drops a layer lens from the lenses, every fan and the hub's own fan", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "lens_set: csharp\nremove: [numerics, numerics]\nfans:\n  mine: [numerics, generic]\n")

		cfg, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir})
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		if _, defined := cfg.Lenses["numerics"]; defined {
			t.Errorf("Load().Lenses holds numerics; want it removed")
		}
		for _, fanName := range []string{"review-code", "full"} {
			if slices.Contains(cfg.Fans[fanName], "numerics") {
				t.Errorf("Load().Fans[%s] = %v; want no numerics", fanName, cfg.Fans[fanName])
			}
		}
		if want := []string{"generic"}; !slices.Equal(cfg.Fans["mine"], want) {
			t.Errorf("Load().Fans[mine] = %v; want %v", cfg.Fans["mine"], want)
		}
	})

	t.Run("a remove name matching no lens loads and warns once per process, naming a repeated name once", func(t *testing.T) {
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "remove: [no-such-lens, no-such-lens]\n")
		logs := logcapture.CaptureVerbose(t)

		for range 2 {
			if _, err := Load(configengine.Dirs{Anchor: anchor}); err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
		}
		warnings := 0
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.Contains(line, "no-such-lens") && strings.Contains(line, configengine.ConfigFile(anchor, "lenses")) {
				warnings++
				if strings.Contains(line, "no-such-lens,no-such-lens") {
					t.Errorf("warning = %q; want the repeated name listed once", line)
				}
			}
		}
		if warnings != 1 {
			t.Errorf("Load() twice logged %d warnings naming no-such-lens and the file; want 1; log: %s", warnings, logs.String())
		}
	})

	t.Run("the skipped-name warning names the highest layer file holding remove", func(t *testing.T) {
		boardDir, anchor := t.TempDir(), t.TempDir()
		writeLensesYAML(t, boardDir, "remove: [other-missing]\n")
		writeLensesYAML(t, anchor, "lenses:\n  mine: prose\n")
		logs := logcapture.CaptureVerbose(t)

		if _, err := Load(configengine.Dirs{Anchor: anchor, Board: boardDir}); err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		var warning string
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.Contains(line, "other-missing") {
				warning = line
			}
		}
		if !strings.Contains(warning, configengine.ConfigFile(boardDir, "lenses")) || !strings.Contains(warning, "layer=default") || strings.Contains(warning, configengine.ConfigFile(anchor, "lenses")) {
			t.Errorf("warning = %q; want it to name the board file and layer=default, never the worktree file", warning)
		}
	})

	t.Run("a lens prose holding a bare dollar sign loads verbatim", func(t *testing.T) {
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "lenses:\n  cost: |\n    Check the $5 fee.\n")

		cfg, err := Load(configengine.Dirs{Anchor: anchor})
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		if cfg.Lenses["cost"] != "Check the $5 fee.\n" {
			t.Errorf("Load().Lenses[cost] = %q; want the prose verbatim", cfg.Lenses["cost"])
		}
	})

	t.Run("a lens prose holding an unset env marker with no default fails the load", func(t *testing.T) {
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "lenses:\n  cost: |\n    Check ${env:LYX_LENSES_TEST_UNSET_NAME}.\n")

		_, err := Load(configengine.Dirs{Anchor: anchor})
		if err == nil || !strings.HasPrefix(err.Error(), "lenses: ") {
			t.Errorf("Load() error = %v; want a failure with prefix \"lenses: \"", err)
		}
	})
}

// TestConfigTemplate_DecodesAndIsSelfConsistent proves the embedded seed template is itself valid lenses.yaml content -- a load over dirs holding no lenses.yaml resolves it, so every template fan resolves -- and that its internal cross-references and lengths hold: every fan entry names a defined lens, and no lens carries hard-exclusion phrasing.
// The spike (docs/research/session-fork-spike.md, Q2) found "ignore everything else" lenses measurably suppressed cross-category coverage, so every lens is emphasis-only by design.
//
// It extends the same checks to every shipped language layer: each loads under its own lens_set, its lenses and fans replace or extend the base's by name, and a layer fan keeps the base fan's entries first.
//
//testtiming:keep pins the seeded lens and fan counts, the defined-lens cross-references and the no-"ignore " phrasing, which the ResolveFan test covering its blocks does not assert, and the per-layer load and superset checks
func TestConfigTemplate_DecodesAndIsSelfConsistent(t *testing.T) {
	t.Parallel()

	cfg := loadTemplateConfig(t)

	const wantLenses = 38
	if len(cfg.Lenses) != wantLenses {
		t.Errorf("seeded template has %d lenses; want %d", len(cfg.Lenses), wantLenses)
	}
	const wantFans = 12
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
	for fanName, wantEntries := range map[string]int{"standard": 5, "full": 8, "discussion": 5, "plan": 5, "twill-discussion": 7, "twill-plan": 5, "review-discussion": 4, "review-plan": 3, "fix-discussion": 4, "fix-plan": 4, "review-code": 4, "fix-code": 4} {
		entries, ok := cfg.Fans[fanName]
		if !ok {
			t.Errorf("seeded template is missing fan %q", fanName)
		}
		if len(entries) != wantEntries {
			t.Errorf("fan %q has %d entries; want %d", fanName, len(entries), wantEntries)
		}
	}
	wantOrders := map[string][]string{
		"discussion":        {"generic", "goal-scope", "pattern-fit", "scenarios", "leanness"},
		"plan":              {"generic", "decision-coverage", "card-structure", "verify-tests", "pattern-fit"},
		"twill-discussion":  {"interactions", "existing-paths", "bound-breaker", "walkthrough", "goal-scope", "board-fit", "leanness"},
		"twill-plan":        {"coverage", "row-validity", "forced-edits", "guard-reach", "way-forward"},
		"review-discussion": {"interactions", "existing-paths", "bound-breaker", "walkthrough"},
		"review-plan":       {"affected-set", "guard-reach", "way-forward"},
		"fix-discussion":    {"fix-consistency", "fix-claims", "fix-proposal", "fix-way-forward"},
		"fix-plan":          {"fix-consistency", "fix-claims", "fix-proposal", "fix-affected-set"},
		"review-code":       {"code-correctness", "code-affected-set", "code-tests", "code-pattern-fit"},
		"fix-code":          {"fix-code-correctness", "fix-consistency", "fix-proposal", "fix-affected-set"},
	}
	procedureLenses := map[string]bool{}
	for fanName, wantOrder := range wantOrders {
		lenses, err := ResolveFan(cfg, fanName)
		if err != nil {
			t.Fatalf("ResolveFan(%s) returned unexpected error: %v", fanName, err)
		}
		if len(lenses) != len(wantOrder) {
			t.Errorf("ResolveFan(%s) has %d lenses; want %d", fanName, len(lenses), len(wantOrder))
			continue
		}
		if strings.HasPrefix(fanName, "review-") || strings.HasPrefix(fanName, "fix-") {
			for _, name := range wantOrder {
				procedureLenses[name] = true
			}
		}
		for i, wantName := range wantOrder {
			if lenses[i].Name != wantName {
				t.Errorf("ResolveFan(%s)[%d].Name = %q; want %q", fanName, i, lenses[i].Name, wantName)
			}
		}
	}

	// A perch's writer runs every procedure its reviewer judges it by, itself or through the writer lens that absorbed it.
	absorbedBy := map[string]string{"affected-set": "forced-edits"}
	for reviewerFan, writerFan := range map[string]string{"review-discussion": "twill-discussion", "review-plan": "twill-plan"} {
		for _, lensName := range wantOrders[reviewerFan] {
			if !slices.Contains(wantOrders[writerFan], lensName) && !slices.Contains(wantOrders[writerFan], absorbedBy[lensName]) {
				t.Errorf("fan %q lacks lens %q of reviewer fan %q", writerFan, lensName, reviewerFan)
			}
		}
	}

	for lensName, wantSentences := range map[string][]string{
		"coverage": {
			"A branch no planned row reaches is a finding.",
			"uses the operand the record names, not a near one",
		},
		"forced-edits": {"a registration no card lists is a finding"},
		"row-validity": {"a zero-value or nil seam of a new field has a stated meaning"},
	} {
		text := strings.Join(strings.Fields(cfg.Lenses[lensName]), " ")
		for _, wantSentence := range wantSentences {
			if !strings.Contains(text, wantSentence) {
				t.Errorf("lens %q text = %q; want it to contain %q", lensName, text, wantSentence)
			}
		}
	}

	const emphasisLine = "Report anything else you notice too — emphasis, never exclusion."
	for name, text := range cfg.Lenses {
		if strings.TrimSpace(text) == "" {
			t.Errorf("lens %q has empty text", name)
		}
		if name != "generic" && !procedureLenses[name] && !strings.HasSuffix(strings.Join(strings.Fields(text), " "), emphasisLine) {
			t.Errorf("lens %q text does not end with the emphasis line", name)
		}
		if strings.Contains(text, "ignore ") {
			t.Errorf("lens %q text contains hard-exclusion phrasing (\"ignore \"): %q", name, text)
		}
	}

	// The exemption from the emphasis line is computed from the base fans only, so a lens a layer adds to a review fan still ends with it.
	baseProcedureLenses := map[string]bool{}
	for fanName, entries := range cfg.Fans {
		if strings.HasPrefix(fanName, "review-") || strings.HasPrefix(fanName, "fix-") {
			for _, name := range entries {
				baseProcedureLenses[name] = true
			}
		}
	}
	for layerName, document := range lensSets() {
		t.Run("layer "+layerName, func(t *testing.T) {
			var raw map[string]any
			if err := yaml.Unmarshal(document, &raw); err != nil {
				t.Fatalf("decode layer %s: %v", layerName, err)
			}
			for key := range raw {
				if key != "lenses" && key != "fans" {
					t.Errorf("layer %s holds top-level key %q; want only lenses and fans", layerName, key)
				}
			}
			var layer Config
			if err := yaml.Unmarshal(document, &layer); err != nil {
				t.Fatalf("decode layer %s: %v", layerName, err)
			}

			anchor := t.TempDir()
			writeLensesYAML(t, anchor, "lens_set: "+layerName+"\n")
			dirs := configengine.Dirs{Anchor: anchor}
			if err := CheckLoad(dirs); err != nil {
				t.Fatalf("CheckLoad(lens_set: %s) = %v; want nil", layerName, err)
			}
			loaded, err := Load(dirs)
			if err != nil {
				t.Fatalf("Load(lens_set: %s): %v", layerName, err)
			}

			for name, text := range loaded.Lenses {
				want, defined := layer.Lenses[name]
				if !defined {
					want = cfg.Lenses[name]
				}
				if text != want {
					t.Errorf("lens %q under lens_set %s = %q; want %q", name, layerName, text, want)
				}
				if strings.Contains(text, "ignore ") {
					t.Errorf("lens %q under lens_set %s contains hard-exclusion phrasing (\"ignore \")", name, layerName)
				}
			}
			for name, entries := range loaded.Fans {
				want, defined := layer.Fans[name]
				if !defined {
					want = cfg.Fans[name]
				}
				if !slices.Equal(entries, want) {
					t.Errorf("fan %q under lens_set %s = %v; want %v", name, layerName, entries, want)
				}
			}
			for name, text := range layer.Lenses {
				if name != "generic" && !baseProcedureLenses[name] && !strings.HasSuffix(strings.Join(strings.Fields(text), " "), emphasisLine) {
					t.Errorf("layer %s lens %q does not end with the emphasis line", layerName, name)
				}
			}
			for name, entries := range layer.Fans {
				baseEntries, inBase := cfg.Fans[name]
				if !inBase {
					continue
				}
				if len(entries) < len(baseEntries) || !slices.Equal(entries[:len(baseEntries)], baseEntries) {
					t.Errorf("layer %s fan %q = %v; want it to start with the base fan %v", layerName, name, entries, baseEntries)
					continue
				}
				for _, extra := range entries[len(baseEntries):] {
					if slices.Contains(baseEntries, extra) {
						t.Errorf("layer %s fan %q adds %q, which the base fan already holds", layerName, name, extra)
					}
				}
			}
			if layerName == "csharp" {
				if _, ok := loaded.Lenses["numerics"]; !ok {
					t.Errorf("lens numerics is undefined under lens_set csharp")
				}
				for _, fanName := range []string{"review-code", "full"} {
					resolved, err := ResolveFan(loaded, fanName)
					if err != nil {
						t.Fatalf("ResolveFan(%s) under lens_set csharp: %v", fanName, err)
					}
					if last := resolved[len(resolved)-1].Name; last != "numerics" {
						t.Errorf("ResolveFan(%s) under lens_set csharp ends with %q; want numerics", fanName, last)
					}
				}
			}
		})
	}

	var goConfig Config
	if err := yaml.Unmarshal(lensSets()["go"], &goConfig); err != nil {
		t.Fatalf("decode layer go: %v", err)
	}
	for _, name := range []string{"error-handling", "test-gaps", "performance", "api-design", "concurrency"} {
		if cfg.Lenses[name] == goConfig.Lenses[name] {
			t.Errorf("base lens %q equals the go layer's prose; want the base language-neutral", name)
		}
		for _, word := range []string{"goroutine", "panic"} {
			if strings.Contains(cfg.Lenses[name], word) {
				t.Errorf("base lens %q names %q; want no language named", name, word)
			}
		}
	}
}

func TestResolveFan(t *testing.T) {
	t.Parallel()

	seeded := loadTemplateConfig(t)

	t.Run("happy path preserves order and repeats", func(t *testing.T) {
		t.Parallel()
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
		// The two "generic" repeats must be independent entries with identical content, not a single deduplicated one.
		if lenses[0].Name != lenses[1].Name || lenses[0].Text != lenses[1].Text {
			t.Errorf("ResolveFan(standard) repeated lens entries diverge: %+v vs %+v", lenses[0], lenses[1])
		}
	})

	overCap := make([]string, MaxFanLenses+1)
	for i := range overCap {
		overCap[i] = "known"
	}
	custom := Config{
		Lenses: map[string]string{"known": "known lens text"},
		Fans: map[string][]string{
			"unknown-lens": {"missing"},
			"empty":        {},
			"over-cap":     overCap,
		},
	}
	refusals := []struct {
		name  string
		cfg   Config
		fan   string
		wants []string
	}{
		{"an unknown fan names the fan", Config{}, "nope", []string{`"nope"`, "unknown fan"}},
		{"an unknown fan lists the known fans", seeded, "nope", []string{"unknown fan", "standard", "full"}},
		{"a fan naming an undefined lens", custom, "unknown-lens", []string{`"unknown-lens"`, "undefined lens", "missing"}},
		{"an empty fan", custom, "empty", []string{`"empty"`, "is empty"}},
		{"a fan longer than MaxFanLenses", custom, "over-cap", []string{`"over-cap"`, "exceeding the maximum"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveFan(tt.cfg, tt.fan)
			if err == nil {
				t.Fatalf("ResolveFan(%q) returned nil error", tt.fan)
			}
			if !strings.HasPrefix(err.Error(), "lenses: ") {
				t.Errorf("ResolveFan(%q) error = %q; want prefix \"lenses: \"", tt.fan, err.Error())
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ResolveFan(%q) error = %q; want it to contain %q", tt.fan, err.Error(), want)
				}
			}
		})
	}

	t.Run("operator-defined fan and lens win over the template", func(t *testing.T) {
		t.Parallel()
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
}

// TestCheckLoad covers CheckLoad's outcomes: the template passes, every failing fan is joined in name order, and a load failure is returned alone with no fan resolved.
func TestCheckLoad(t *testing.T) {
	t.Parallel()

	t.Run("the template passes", func(t *testing.T) {
		t.Parallel()
		if err := CheckLoad(configengine.Dirs{Anchor: t.TempDir()}); err != nil {
			t.Errorf("CheckLoad(template) = %v; want nil", err)
		}
	})

	t.Run("two broken fans return both failures in name order", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "fans:\n  b-undefined: [missing]\n  a-empty: []\n")

		err := CheckLoad(configengine.Dirs{Anchor: anchor})
		if err == nil {
			t.Fatal("CheckLoad() = nil; want both failures")
		}
		text := err.Error()
		emptyAt := strings.Index(text, `fan "a-empty" is empty`)
		undefinedAt := strings.Index(text, `fan "b-undefined" names undefined lens "missing"`)
		if emptyAt == -1 || undefinedAt == -1 || emptyAt > undefinedAt {
			t.Errorf("CheckLoad() = %q; want the a-empty failure, then the b-undefined failure", text)
		}
	})

	t.Run("a remove emptying a fan passes the check and the fan is refused at resolution", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "remove: [code-correctness, code-affected-set, code-tests, code-pattern-fit]\n")
		dirs := configengine.Dirs{Anchor: anchor}

		if err := CheckLoad(dirs); err != nil {
			t.Errorf("CheckLoad() = %v; want nil", err)
		}
		cfg, err := Load(dirs)
		if err != nil {
			t.Fatalf("Load() returned unexpected error: %v", err)
		}
		if _, held := cfg.Fans["review-code"]; held {
			t.Errorf("Load().Fans holds review-code; want it moved to EmptiedFans")
		}
		_, err = ResolveFan(cfg, "review-code")
		if err == nil {
			t.Fatal("ResolveFan(review-code) = nil error; want the emptied-fan refusal")
		}
		for _, want := range []string{"remove", "code-correctness", "way forward"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ResolveFan(review-code) = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("a malformed file returns the load's error alone", func(t *testing.T) {
		t.Parallel()
		anchor := t.TempDir()
		writeLensesYAML(t, anchor, "lenses: [unterminated\n")

		err := CheckLoad(configengine.Dirs{Anchor: anchor})
		if err == nil {
			t.Fatal("CheckLoad() = nil; want the load's error")
		}
		if text := err.Error(); !strings.HasPrefix(text, "lenses: ") || strings.Count(text, "lenses: ") != 1 {
			t.Errorf("CheckLoad() = %q; want one error with prefix \"lenses: \" and no fan failure", text)
		}
	})
}
