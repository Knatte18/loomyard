//go:build integration

// scopes_integration_test.go drives `lyx gate test` over a hubforge hub with a task pair, whose fabric origin record names the prime's branch as its parent, to reach the computed, --base and --card scope forms.
// It is Tier 2: it builds a real hub and spawns git, `go list` and the stand-in tier command.

package gatecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// scopesConfig returns the gate.yaml of the scopes hub: tier0 over ./..., tier1 covering it and serving as the guard tier, and the given scoper.
func scopesConfig(t *testing.T, scoper string) []byte {
	t.Helper()

	return scenarioConfig(t, func(cfg map[string]any) {
		cfg["scoper"] = scoper
		cfg["guard_tier"] = 1
		tiers := cfg["tiers"].(map[string]any)
		for _, name := range []string{"tier0", "tier1"} {
			tiers[name].(map[string]any)["all_targets"] = "./..."
		}
		tiers["tier1"].(map[string]any)["covers"] = []int{0}
	})
}

func TestGateTest_Scopes(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := h.PrimeWorktree()
	pair := h.PairCodeWorktree(hubforge.TemplatePairSlug)
	configPath := configengine.ConfigFile(h.BoardDir(), "gate")
	writeScoper := func(t *testing.T, scoper string) {
		t.Helper()
		if err := os.WriteFile(configPath, scopesConfig(t, scoper), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeScoper(t, "go")
	pool := hubgeom.GateSlots(h.Location)

	// The module lives on the prime's branch and the pair is fast-forwarded onto it, so the merge base the pair diffs from holds the module and a computed scope sees only the edits a row makes.
	files := map[string]string{
		"go.mod":             "module example.com/scopes\n\ngo 1.21\n",
		"lib/lib.go":         "package lib\n\nfunc One() int { return 1 }\n",
		"user/user.go":       "package user\n\nimport \"example.com/scopes/lib\"\n\nfunc Two() int { return lib.One() + 1 }\n",
		"other/other.go":     "package other\n",
		"guardpkg/g.go":      "package guardpkg\n",
		"guardpkg/g_test.go": "package guardpkg\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestGuard(t *testing.T) {}\n",
		"nested/go.mod":      "module example.com/scopes/nested\n\ngo 1.21\n",
		"nested/n.go":        "package nested\n",
		"docs/note.md":       "note\n",
	}
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, "add", "go.mod", "lib", "user", "other", "guardpkg", "nested", "docs")
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the scopes module")
	gitkit.Git(t, pair, "merge", "--ff-only", "-q", "main")

	// edit replaces the pair's file rel with content until the test ends.
	edit := func(t *testing.T, rel, content string) {
		t.Helper()
		full := filepath.Join(pair, filepath.FromSlash(rel))
		original, readErr := os.ReadFile(full)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if readErr != nil {
				_ = os.Remove(full)
				return
			}
			if err := os.WriteFile(full, original, 0o644); err != nil {
				t.Error(err)
			}
		})
	}
	// gate runs `lyx gate test args...` from cwd and returns the finished gate.
	gate := func(t *testing.T, cwd string, env []string, args ...string) *runningGate {
		t.Helper()
		g := startGate(t, gateRun{cwd: cwd, args: append([]string{"test"}, args...), env: env})
		g.exit = g.wait(t)
		return g
	}
	requireFree := func(t *testing.T) {
		t.Helper()
		eventually(t, "the slots to be free", func() bool {
			holders, err := pool.Holders()
			return err == nil && len(holders) == 0
		})
	}
	guardInvocation := []string{"./guardpkg", "-run", "^(TestGuard)$"}
	wantArgs := func(t *testing.T, g *runningGate, want ...[]string) {
		t.Helper()
		if g.exit != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0", g.exit, g.stdout.String())
		}
		if got := g.tierArgs(t); !reflect.DeepEqual(got, want) {
			t.Errorf("tier invocations = %q; want %q", got, want)
		}
	}
	// wantRefusal checks that the gate refused before any slot or tier command with text holding each of wants.
	wantRefusal := func(t *testing.T, g *runningGate, wants ...string) {
		t.Helper()
		var envelope struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(g.stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout %q is not a JSON envelope: %v", g.stdout.String(), err)
		}
		if g.exit != 1 || envelope.OK {
			t.Errorf("gate = exit %d, %+v; want exit 1 and ok false", g.exit, envelope)
		}
		for _, want := range wants {
			if !strings.Contains(envelope.Error, want) {
				t.Errorf("gate error = %q; want it to hold %q", envelope.Error, want)
			}
		}
		if _, err := os.Stat(g.record); err == nil {
			t.Error("a tier command ran although the gate refused")
		}
		requireFree(t)
	}

	t.Run("a computed scope under go runs the changed package and its importer, then the guard step through guard_tier's slots", func(t *testing.T) {
		edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 2 }\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
		listed, _ := runRecordOf(t, g.stdout.String(), 0)
		guard, _ := runRecordOf(t, g.stdout.String(), 1)
		if listed.ScopeForm != "computed" || listed.Slug != hubforge.TemplatePairSlug || !slices.Equal(listed.TierList, []int{0}) {
			t.Errorf("tier0 run record = %+v; want the computed scope of the pair's slug and the tier list [0]", listed)
		}
		if !guard.GuardStep || guard.Slots != 0 || guard.ScopeForm != "guard" {
			t.Errorf("guard step run record = %+v; want the guard step of tier1, which holds every slot", guard)
		}
		requireFree(t)
	})

	t.Run("a computed scope over a change to a nested module's file falls back to whole, naming the reason", func(t *testing.T) {
		edit(t, "nested/n.go", "package nested\n\nvar Changed = true\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./..."}, guardInvocation)
		if !strings.Contains(g.stdout.String(), "falls back to whole: nested/n.go is neither a Markdown file nor under a Go package directory") {
			t.Errorf("stdout = %q; want the fallback and its reason", g.stdout.String())
		}
	})

	t.Run("a Markdown-only change runs no tier command, logs zero and runs only the guard step", func(t *testing.T) {
		edit(t, "docs/note.md", "changed note\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.Discovered != 0 || run.ExitCode != 0 || !strings.Contains(g.stdout.String(), "the scope is empty, so tier0 runs no command") {
			t.Errorf("tier0 run record = %+v, stdout %q; want an empty-scope pass with nothing discovered", run, g.stdout.String())
		}
		requireFree(t)
	})

	t.Run("--base runs the packages the diff from that revision reaches, then the guard step", func(t *testing.T) {
		base := gitkit.RevParse(t, pair, "HEAD")
		edit(t, "user/user.go", "package user\n\nimport \"example.com/scopes/lib\"\n\nfunc Two() int { return lib.One() + 2 }\n")
		g := gate(t, pair, nil, "--base", base)
		wantArgs(t, g, []string{"./user"}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "base" {
			t.Errorf("tier0 run record scope form = %q; want base", run.ScopeForm)
		}
	})

	plan := plankit.Plan{Approved: true, Cards: []plankit.Card{{
		Number: 1,
		Slug:   "span",
		Groups: []plankit.Group{{Label: "Edit", Targets: []string{"lib/lib.go", "nested/n.go"}}},
	}}}
	plankit.Write(t, planparser.PlanDir(pair), plan)

	t.Run("--card runs a card's packages once per module, then the guard step over the guard tests outside them", func(t *testing.T) {
		g := gate(t, pair, nil, "--card", "1")
		wantArgs(t, g, []string{"./lib"}, []string{"."}, guardInvocation)
		var cwds []string
		for _, record := range g.tierRecords(t) {
			cwds = append(cwds, record.Cwd)
		}
		if want := []string{pair, filepath.Join(pair, "nested"), pair}; !slices.Equal(cwds, want) {
			t.Errorf("tier cwds = %q; want the root module, the nested module, then the guard step in the root module: %q", cwds, want)
		}
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" {
			t.Errorf("tier0 run record scope form = %q; want card", run.ScopeForm)
		}
	})

	t.Run("--card with a tier that outlives timeout_min across its invocations is killed with the timeout line", func(t *testing.T) {
		g := gate(t, pair, []string{helperHoldEnv + "=1", helperUnitEnv + "=1s"}, "--card", "1")
		records := g.tierRecords(t)
		if len(records) != 1 {
			t.Fatalf("tier invocations = %d; want the first module's run alone, killed before the second starts", len(records))
		}
		t.Cleanup(func() { _ = (&os.Process{Pid: records[0].Grandchild}).Kill() })
		if tier, ok := gateslot.FindTierTimeout(g.stdout.String()); g.exit != 1 || !ok || tier != "tier0" {
			t.Errorf("gate = exit %d, FindTierTimeout = (%q, %v), stdout %q; want exit 1 and the tier0 timeout line", g.exit, tier, ok, g.stdout.String())
		}
		requireFree(t)
	})

	for _, scoper := range []string{"go", "none"} {
		t.Run("--card with an unknown id is refused before any slot naming the plan's ids under "+scoper, func(t *testing.T) {
			writeScoper(t, scoper)
			defer writeScoper(t, "go")
			wantRefusal(t, gate(t, pair, nil, "--card", "9"), "1-span", "way forward")
		})
	}

	t.Run("--card in a worktree with no plan is refused naming --all", func(t *testing.T) {
		wantRefusal(t, gate(t, prime, nil, "--card", "1"), "--all", "explicit targets")
	})

	t.Run("a misplaced guard marker refuses a computed call before any slot with its file and line", func(t *testing.T) {
		edit(t, "other/o_test.go", "package other\n\n//lyx:guard\nfunc helper() {}\n")
		wantRefusal(t, gate(t, pair, nil), "other/o_test.go:3", "way forward")
	})

	t.Run("the prime with no parent runs whole", func(t *testing.T) {
		g := gate(t, prime, nil)
		wantArgs(t, g, []string{"./..."}, guardInvocation)
		if strings.Contains(g.stdout.String(), "falls back") {
			t.Errorf("stdout = %q; want no fallback notice, since there is no parent to diff from", g.stdout.String())
		}
	})

	t.Run("under the none scoper every scope form takes all_targets", func(t *testing.T) {
		writeScoper(t, "none")
		defer writeScoper(t, "go")
		base := gitkit.RevParse(t, pair, "HEAD")

		edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 3 }\n")
		wantArgs(t, gate(t, pair, nil), []string{"./..."})
		wantArgs(t, gate(t, pair, nil, "--card", "1"), []string{"./..."})
		wantArgs(t, gate(t, pair, nil, "--base", base), []string{"./..."})
	})

	t.Run("under the none scoper a Markdown-only diff runs the listed tiers whole, skipping a covered one", func(t *testing.T) {
		writeScoper(t, "none")
		defer writeScoper(t, "go")
		edit(t, "docs/note.md", "another note\n")
		g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "tier0 is covered by tier1") {
			t.Errorf("stdout = %q; want tier0 named as covered", g.stdout.String())
		}
	})
}
