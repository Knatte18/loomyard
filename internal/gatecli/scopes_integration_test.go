//go:build tier1

// scopes_integration_test.go drives `lym gate test` over a hubforge hub with a task pair, whose fabric origin record names the prime's branch as its parent, to reach the computed, --base and --card scope forms.
// It is Tier 2: it builds a real hub and spawns git, `go list` and the stand-in tier command.

package gatecli

import (
	"encoding/json"
	"fmt"
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

// scopesConfig returns the gate.yaml of the scopes hub: tier0 over ./..., tier1 covering it, and the given scoper.
func scopesConfig(t *testing.T, scoper string) []byte {
	t.Helper()

	return scopesConfigWith(t, scoper, nil)
}

// scopesConfigWith is scopesConfig after mutate has changed the config.
func scopesConfigWith(t *testing.T, scoper string, mutate func(cfg map[string]any)) []byte {
	t.Helper()

	return scenarioConfig(t, func(cfg map[string]any) {
		cfg["scoper"] = scoper
		tiers := cfg["tiers"].(map[string]any)
		for _, name := range []string{"tier0", "tier1"} {
			tiers[name].(map[string]any)["all_targets"] = "./..."
		}
		tiers["tier1"].(map[string]any)["covers"] = []int{0}
		if mutate != nil {
			mutate(cfg)
		}
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
	// writeDotnet switches the hub to the dotnet scoper with Markdown files inert, and the stand-in command `{targets} {filter}` on both tiers, after mutate has changed the config.
	writeDotnet := func(t *testing.T, mutate func(cfg map[string]any)) {
		t.Helper()
		config := scopesConfigWith(t, "dotnet", func(cfg map[string]any) {
			cfg["inert_paths"] = []string{"*.md"}
			tiers := cfg["tiers"].(map[string]any)
			for _, name := range []string{"tier0", "tier1"} {
				tiers[name].(map[string]any)["command"] = "'" + os.Args[0] + "' {targets} {filter}"
			}
			if mutate != nil {
				mutate(cfg)
			}
		})
		if err := os.WriteFile(configPath, config, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pool := hubgeom.GateSlots(h.Location)

	// The module lives on the prime's branch and the pair is fast-forwarded onto it, so the merge base the pair diffs from holds the module and a computed scope sees only the edits a row makes.
	files := map[string]string{
		"go.mod":             "module example.com/scopes\n\ngo 1.21\n",
		"lib/lib.go":         "package lib\n\nfunc One() int { return 1 }\n",
		"user/user.go":       "package user\n\nimport \"example.com/scopes/lib\"\n\nfunc Two() int { return lib.One() + 1 }\n",
		"other/other.go":     "package other\n",
		"guardpkg/g.go":      "package guardpkg\n",
		"guardpkg/g_test.go": "package guardpkg\n\nimport \"testing\"\n\n//lym:guard\nfunc TestGuard(t *testing.T) {}\n",
		"nested/go.mod":      "module example.com/scopes/nested\n\ngo 1.21\n",
		"nested/n.go":        "package nested\n",
		"docs/note.md":       "note\n",

		"cs/Lib/Lib.csproj":             "<Project />\n",
		"cs/Lib/Circle.cs":              "namespace Lib;\n\npublic class Circle { }\n",
		"cs/Lib/Unused.cs":              "namespace Lib;\n\npublic class Unused { }\n",
		"cs/Lib.Tests/Lib.Tests.csproj": "<Project />\n",
		"cs/Lib.Tests/CircleTests.cs":   "namespace Lib.Tests;\n\n[TestFixture]\npublic class CircleTests\n{\n    [Test]\n    public void Area() { var circle = new Circle(); }\n}\n",
		"cs/Lib.Tests/OtherTests.cs":    "namespace Lib.Tests;\n\n[TestFixture]\npublic class OtherTests\n{\n    [Test]\n    public void Run() { }\n}\n",
	}
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, "add", "go.mod", "lib", "user", "other", "guardpkg", "nested", "docs", "cs")
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
	// gate runs `lym gate test args...` from cwd and returns the finished gate.
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

	t.Run("a computed scope under go runs the changed package and its importer, then the guard step's group through its tier's slots", func(t *testing.T) {
		edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 2 }\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
		listed, _ := runRecordOf(t, g.stdout.String(), 0)
		guard, _ := guardRunRecordOf(t, g.stdout.String(), 0)
		if listed.ScopeForm != "computed" || listed.Slug != hubforge.TemplatePairSlug || !slices.Equal(listed.TierList, []int{0}) {
			t.Errorf("tier0 run record = %+v; want the computed scope of the pair's slug and the tier list [0]", listed)
		}
		if !guard.GuardStep || guard.Tier != 0 || guard.Slots != 1 || guard.ScopeForm != "guard" {
			t.Errorf("guard step run record = %+v; want the guard group of tier0, whose guard test sits in an untagged file, holding tier0's one slot", guard)
		}
		requireFree(t)
	})

	t.Run("a computed scope over a change to a nested module's file falls back to whole, naming the reason, and plans no guard group for the tier it ran whole", func(t *testing.T) {
		edit(t, "nested/n.go", "package nested\n\nvar Changed = true\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./..."})
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
		if trimpath := trimpathLine("tier0", []string{"GOFLAGS=-trimpath -p=3"}); strings.Count(g.stdout.String(), trimpath) != 1 {
			t.Errorf("stdout = %q; want the -trimpath line once, for the guard group alone, since the listed tier runs no command", g.stdout.String())
		}
		requireFree(t)
	})

	// writeInert switches the hub to scoper with Markdown files inert, after mutate has changed the config.
	writeInert := func(t *testing.T, scoper string, mutate func(cfg map[string]any)) {
		t.Helper()
		config := scopesConfigWith(t, scoper, func(cfg map[string]any) {
			cfg["inert_paths"] = []string{"*.md"}
			if mutate != nil {
				mutate(cfg)
			}
		})
		if err := os.WriteFile(configPath, config, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// noCarryTier1 lists tier1 apart from tier0 with carry: false.
	noCarryTier1 := func(cfg map[string]any) {
		tier1 := cfg["tiers"].(map[string]any)["tier1"].(map[string]any)
		delete(tier1, "covers")
		tier1["carry"] = false
	}
	// wantInertEmpty checks that the gate named the inert diff and ran no command of the tiers that carry.
	wantInertEmpty := func(t *testing.T, g *runningGate) {
		t.Helper()
		if !strings.Contains(g.stdout.String(), "every changed file is inert, so the scope is empty") {
			t.Errorf("stdout = %q; want the inert-only diff named", g.stdout.String())
		}
	}

	t.Run("an inert-only diff under go runs no listed tier's command and still runs the guard step, and a carry: false tier beside it runs whole", func(t *testing.T) {
		writeInert(t, "go", noCarryTier1)
		defer writeScoper(t, "go")
		edit(t, "docs/note.md", "inert note\n")
		g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantArgs(t, g, []string{"./..."}, guardInvocation)
		wantInertEmpty(t, g)
		if !strings.Contains(g.stdout.String(), "the scope is empty, so tier0 runs no command") {
			t.Errorf("stdout = %q; want tier0 to run no command", g.stdout.String())
		}
	})

	t.Run("an inert-only diff under none runs no listed tier's command and plans no guard step, and a carry: false tier beside it runs whole", func(t *testing.T) {
		writeInert(t, "none", noCarryTier1)
		defer writeScoper(t, "go")
		edit(t, "docs/note.md", "inert note\n")
		g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantArgs(t, g, []string{"./..."})
		wantInertEmpty(t, g)
	})

	t.Run("an inert-only diff under dotnet runs no listed tier's command, and a carry: false tier beside it runs whole", func(t *testing.T) {
		writeDotnet(t, noCarryTier1)
		defer writeScoper(t, "go")
		edit(t, "docs/note.md", "inert note\n")
		wantArgs(t, gate(t, pair, nil, "--tier", "0", "--tier", "1"), []string{"./..."})
	})

	t.Run("an untracked non-inert file beside an inert change keeps the go scope as it is derived", func(t *testing.T) {
		writeInert(t, "go", nil)
		defer writeScoper(t, "go")
		edit(t, "docs/note.md", "inert note\n")
		edit(t, "other/added.go", "package other\n\nvar Added = 1\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./other"}, guardInvocation)
		if strings.Contains(g.stdout.String(), "every changed file is inert") {
			t.Errorf("stdout = %q; want the diff not held inert", g.stdout.String())
		}
	})

	// withSharedBuild declares the hub's build step over the stand-in, sets shared_build on tier0 and leaves tier1 out of it and no longer covering tier0.
	withSharedBuild := func(cfg map[string]any) {
		cfg["build"] = map[string]any{"command": "'" + os.Args[0] + "' sharedbuild", "timeout_min": 1}
		tiers := cfg["tiers"].(map[string]any)
		tiers["tier0"].(map[string]any)["shared_build"] = true
		delete(tiers["tier1"].(map[string]any), "covers")
	}
	// builds returns the shared build's invocations in g.
	builds := func(t *testing.T, g *runningGate) []standInRecord {
		t.Helper()
		return slices.DeleteFunc(g.records(t), func(record standInRecord) bool { return !record.Build })
	}

	t.Run("a dotnet scope that selected no test class runs the build once in tier0's slots, runs no tier0 command and runs tier1 whole", func(t *testing.T) {
		writeDotnet(t, withSharedBuild)
		defer writeScoper(t, "go")
		edit(t, "cs/Lib/Unused.cs", "namespace Lib;\n\npublic class Unused { public int Edge; }\n")
		g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantArgs(t, g, []string{"./..."})
		if got := builds(t, g); len(got) != 1 || filepath.Dir(got[0].Slot) != pool.Dir {
			t.Errorf("build invocations = %+v; want the one build, inside a slot of %s", got, pool.Dir)
		}
		if !strings.Contains(g.stdout.String(), "gate test: the scope selected no test class, so tier1 runs whole") {
			t.Errorf("stdout = %q; want tier1 named as run whole", g.stdout.String())
		}
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.Discovered != 0 || run.ExitCode != 0 {
			t.Errorf("tier0 run record = %+v; want an empty pass", run)
		}
		requireFree(t)

		failing := startGate(t, gateRun{cwd: pair, args: []string{"test", "--tier", "0", "--tier", "1"}, env: []string{helperBuildExitEnv + "=3"}})
		if code := failing.wait(t); code != 3 {
			t.Errorf("exit code with a build exiting 3 = %d, stdout %q; want 3", code, failing.stdout.String())
		}
	})

	t.Run("an inert-only dotnet diff and an empty one run no build", func(t *testing.T) {
		writeDotnet(t, withSharedBuild)
		defer writeScoper(t, "go")
		wantNothingRun := func(name string) {
			t.Helper()
			g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
			if got := g.records(t); g.exit != 0 || len(got) != 0 {
				t.Errorf("%s diff: exit %d, stand-in records %+v; want no build and no tier command", name, g.exit, got)
			}
		}
		wantNothingRun("empty")
		edit(t, "docs/note.md", "inert note\n")
		wantNothingRun("inert-only")
	})

	t.Run("an empty dotnet diff with a carry: false tier that opts into the build runs the build before that tier's whole command", func(t *testing.T) {
		writeDotnet(t, func(cfg map[string]any) {
			withSharedBuild(cfg)
			cfg["tiers"].(map[string]any)["tier0"].(map[string]any)["carry"] = false
		})
		defer writeScoper(t, "go")
		g := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		records := g.records(t)
		if g.exit != 0 || len(records) != 2 || !records[0].Build || records[1].Build || !slices.Equal(records[1].Args, []string{"./..."}) {
			t.Errorf("gate = exit %d, stand-in records %+v; want the build, then tier0's whole command", g.exit, records)
		}
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

	circleTestsArgs := []string{"cs/Lib.Tests/Lib.Tests.csproj", "--filter", "FullyQualifiedName~Lib.Tests.CircleTests"}

	t.Run("a computed scope under dotnet runs the selected test project once with its class filter", func(t *testing.T) {
		writeDotnet(t, nil)
		defer writeScoper(t, "go")
		edit(t, "cs/Lib/Circle.cs", "namespace Lib;\n\npublic class Circle { public int Radius; }\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, circleTestsArgs)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "computed" {
			t.Errorf("tier0 run record scope form = %q; want computed", run.ScopeForm)
		}
	})

	t.Run("a project file change under dotnet falls back to whole, naming the path", func(t *testing.T) {
		writeDotnet(t, nil)
		defer writeScoper(t, "go")
		edit(t, "cs/Lib/Lib.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\" />\n")
		g := gate(t, pair, nil)
		wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "falls back to whole: cs/Lib/Lib.csproj is neither inert nor a .cs file") {
			t.Errorf("stdout = %q; want the fallback and its reason", g.stdout.String())
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

	t.Run("--card under dotnet runs the project of the test class naming a card's member glyph target", func(t *testing.T) {
		writeDotnet(t, nil)
		defer writeScoper(t, "go")
		csharpPlan := plan
		csharpPlan.Language = "csharp"
		csharpPlan.Cards = []plankit.Card{{Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"Lib#Circle.Area"}}}}}
		plankit.Write(t, planparser.PlanDir(pair), csharpPlan)
		t.Cleanup(func() { plankit.Write(t, planparser.PlanDir(pair), plan) })
		g := gate(t, pair, nil, "--card", "1")
		wantArgs(t, g, circleTestsArgs)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" {
			t.Errorf("tier0 run record scope form = %q; want card", run.ScopeForm)
		}
	})

	// vetSummary returns the summary line of tier1's vet run in output, failing the test when there is none.
	vetSummary := func(t *testing.T, output string) string {
		t.Helper()
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "gate test: tier1 vet ") {
				return line
			}
		}
		t.Fatalf("output %q holds no summary line of tier1's vet run", output)
		return ""
	}
	tier1File := "//go:build tier1\n\npackage lib\n\nimport \"testing\"\n\nfunc TestUse(t *testing.T) { _ = %s() }\n"

	t.Run("--card vets a card's packages per module under the tag of a tier that adds a file to one, between the listed tier and the guard step", func(t *testing.T) {
		edit(t, "lib/use_test.go", fmt.Sprintf(tier1File, "One"))
		g := gate(t, pair, nil, "--card", "1")
		wantArgs(t, g, []string{"./lib"}, []string{"."}, []string{"vet", "-tags", "tier1", "./lib"}, []string{"vet", "-tags", "tier1", "."}, guardInvocation)
		if line := vetSummary(t, g.stdout.String()); !strings.Contains(line, " exit 0 ") {
			t.Errorf("vet summary line = %q; want exit 0", line)
		}
		requireFree(t)
	})

	t.Run("--card fails a card whose tier1 file uses a helper only a tier0 file defines, naming the vet run", func(t *testing.T) {
		edit(t, "lib/helper_test.go", "//go:build tier0\n\npackage lib\n\nfunc helper() int { return One() }\n")
		edit(t, "lib/use_test.go", fmt.Sprintf(tier1File, "helper"))
		g := startGate(t, gateRun{cwd: pair, args: []string{"test", "--card", "1"}, realGo: true})
		g.exit = g.wait(t)
		output := g.stdout.String()
		if g.exit == 0 {
			t.Errorf("exit code = 0, stdout %q; want the vet run's failure", output)
		}
		if line := vetSummary(t, output); strings.Contains(line, " exit 0 ") {
			t.Errorf("vet summary line = %q; want a non-zero exit", line)
		}
		for _, want := range []string{"undefined: helper", "failed runs: vet tier1"} {
			if !strings.Contains(output, want) {
				t.Errorf("stdout = %q; want it to hold %q", output, want)
			}
		}
		_, logPath, _ := strings.Cut(vetSummary(t, output), " log ")
		if vet, _ := runRecordAt(t, logPath, 1); !slices.Equal(vet.TierList, []int{0}) {
			t.Errorf("vet run record tier list = %v; want [0], the listed tiers alone", vet.TierList)
		}
		if guard, _ := guardRunRecordOf(t, output, 0); !guard.GuardStep {
			t.Errorf("guard run record = %+v; want the guard step to have run after the failed vet", guard)
		}
		requireFree(t)
	})

	t.Run("--card with a tier that outlives timeout_min across its invocations is killed with the timeout line", func(t *testing.T) {
		// A dirty tree is never carried, so the packages the row before ran green run again.
		edit(t, "docs/note.md", "a dirtying note\n")
		g := gate(t, pair, []string{helperHoldEnv + "=1", helperUnitEnv + "=1s"}, "--card", "1")
		records := g.tierRecords(t)
		t.Cleanup(func() {
			for _, record := range records {
				_ = (&os.Process{Pid: record.Grandchild}).Kill()
			}
		})
		// The timed-out tier stops its own invocations, but not the guard step after it.
		if got, want := g.tierArgs(t), [][]string{{"./lib"}, guardInvocation}; !reflect.DeepEqual(got, want) {
			t.Errorf("tier invocations = %q; want the first module's run alone, killed before the second starts, then the guard step: %q", got, want)
		}
		if tier, ok := gateslot.FindTierTimeout(g.stdout.String()); g.exit != 1 || !ok || tier != "tier0" {
			t.Errorf("gate = exit %d, FindTierTimeout = (%q, %v), stdout %q; want exit 1 and the tier0 timeout line", g.exit, tier, ok, g.stdout.String())
		}
		requireFree(t)
	})

	for _, scoper := range []string{"go", "none", "dotnet"} {
		t.Run("--card with an unknown id is refused before any slot naming the plan's ids under "+scoper, func(t *testing.T) {
			writeScoper(t, scoper)
			defer writeScoper(t, "go")
			wantRefusal(t, gate(t, pair, nil, "--card", "9"), "1-span", "way forward")
		})
	}

	t.Run("--card in a worktree with no plan is refused naming --all", func(t *testing.T) {
		wantRefusal(t, gate(t, prime, nil, "--card", "1"), "--all", "explicit targets")
	})

	t.Run("--gate with scope card and no --card is refused naming the key and the way forward", func(t *testing.T) {
		wantRefusal(t, gate(t, pair, nil, "--gate", "card"), `"card"`, "--card <id>", "way forward")
	})

	t.Run("--gate with scope card runs the --card packages and records the key", func(t *testing.T) {
		edit(t, "docs/note.md", "a dirtying note\n")
		g := gate(t, pair, nil, "--gate", "card", "--card", "1")
		wantArgs(t, g, []string{"./lib"}, []string{"."}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" || run.GateKey != "card" {
			t.Errorf("tier0 run record = %+v; want the card scope form and gate key card", run)
		}
	})

	t.Run("--gate with scope all runs every tier whole, ignoring --card ids", func(t *testing.T) {
		edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 4 }\n")
		g := gate(t, pair, nil, "--gate", "finalize", "--card", "1")
		wantArgs(t, g, []string{"./..."})
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "all" || run.GateKey != "finalize" {
			t.Errorf("tier0 run record = %+v; want the all scope form and gate key finalize", run)
		}
	})

	t.Run("--gate with scope diff runs the computed scope", func(t *testing.T) {
		edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 5 }\n")
		g := gate(t, pair, nil, "--gate", "webster-fix")
		wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "computed" || run.GateKey != "webster-fix" {
			t.Errorf("tier0 run record = %+v; want the computed scope form and gate key webster-fix", run)
		}
	})

	t.Run("explicit targets beside --gate replace the key's scope", func(t *testing.T) {
		g := gate(t, pair, nil, "--gate", "card", "./other")
		wantArgs(t, g, []string{"./other"})
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "targets" || run.GateKey != "card" {
			t.Errorf("tier0 run record = %+v; want the targets scope form and gate key card", run)
		}
	})

	t.Run("--gate with an unknown key is refused with the config's way forward naming lym config gate", func(t *testing.T) {
		wantRefusal(t, gate(t, pair, nil, "--gate", "nosuch"), `"nosuch"`, "lym config gate")
	})

	t.Run("a misplaced guard marker refuses a computed call before any slot with its file and line", func(t *testing.T) {
		edit(t, "other/o_test.go", "package other\n\n//lym:guard\nfunc helper() {}\n")
		wantRefusal(t, gate(t, pair, nil), "other/o_test.go:3", "way forward")
	})

	t.Run("the prime with no parent runs whole", func(t *testing.T) {
		g := gate(t, prime, nil)
		wantArgs(t, g, []string{"./..."})
		if strings.Contains(g.stdout.String(), "falls back") || strings.Contains(g.stdout.String(), "diff scope from") {
			t.Errorf("stdout = %q; want no fallback or derivation notice, since there is no parent to diff from", g.stdout.String())
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

	// The rows from here commit to the pair's branch and run in order; the one row after them runs whole and reads no history.
	// real runs `lym gate test args...` from the pair with the real go command, which the green record lists packages with.
	real := func(t *testing.T, args ...string) *runningGate {
		t.Helper()
		g := startGate(t, gateRun{cwd: pair, realGo: true, args: append([]string{"test"}, args...)})
		g.exit = g.wait(t)
		return g
	}
	wantCarried := func(t *testing.T, g *runningGate) {
		t.Helper()
		if g.exit != 0 || len(g.tierArgs(t)) != 0 || !strings.Contains(g.stdout.String(), " all carried ") {
			t.Errorf("gate = exit %d, tier invocations %q, stdout %q; want every tier carried", g.exit, g.tierArgs(t), g.stdout.String())
		}
	}
	greenPath := filepath.Join(gateslot.WaitDir(pair), greenFileName)

	t.Run("an all run after a green diff run runs only the packages the diff run did not cover, and a third all run carries whole", func(t *testing.T) {
		if err := os.Remove(greenPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		gitkit.CommitFile(t, pair, "lib/lib.go", "package lib\n\nfunc One() int { return 7 }\n", "change lib")
		base := gitkit.RevParse(t, pair, "HEAD~1")

		diff := real(t, "--base", base)
		wantArgs(t, diff, []string{"./lib", "./user"}, guardInvocation)

		second := real(t, "--all")
		wantArgs(t, second, []string{"./guardpkg", "./other"})

		wantCarried(t, real(t, "--all"))
	})

	t.Run("a changed tier command, a different tree and -- -count=1 each break the carry", func(t *testing.T) {
		wantCarried(t, real(t, "--all"))

		if withArgs := real(t, "--all", "--", "-count=1"); withArgs.exit != 0 || !reflect.DeepEqual(withArgs.tierArgs(t), [][]string{{"./...", "-count=1"}}) {
			t.Errorf("--all -- -count=1 = exit %d, tier invocations %q; want the tier to run whole under the new args", withArgs.exit, withArgs.tierArgs(t))
		}
		wantCarried(t, real(t, "--all"))

		gitkit.CommitFile(t, pair, "docs/note.md", "a committed note\n", "change the tree")
		wantArgs(t, real(t, "--all"), []string{"./..."})

		changed := scopesConfigWith(t, "go", func(cfg map[string]any) {
			tier0 := cfg["tiers"].(map[string]any)["tier0"].(map[string]any)
			tier0["command"] = tier0["command"].(string) + " extra"
		})
		if err := os.WriteFile(configPath, changed, 0o644); err != nil {
			t.Fatal(err)
		}
		defer writeScoper(t, "go")
		wantArgs(t, real(t, "--all"), []string{"./...", "extra"})
	})

	t.Run("--card carries a vet run that passed on the same clean tree", func(t *testing.T) {
		if err := os.Remove(greenPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		before := gitkit.RevParse(t, pair, "HEAD")
		gitkit.CommitFile(t, pair, "lib/use_test.go", fmt.Sprintf(tier1File, "One"), "add a tier1 file")
		t.Cleanup(func() { gitkit.Git(t, pair, "reset", "-q", "--hard", before) })

		first := real(t, "--card", "1")
		if line := vetSummary(t, first.stdout.String()); first.exit != 0 || !strings.Contains(line, " exit 0 ") || strings.Contains(line, " carried ") {
			t.Fatalf("first call = exit %d, vet summary line %q; want a pass that ran the vet", first.exit, line)
		}

		second := real(t, "--card", "1")
		// wantCarried names the all scope form, which a --card call never prints.
		if got := second.tierArgs(t); second.exit != 0 || len(got) != 0 || strings.Count(second.stdout.String(), " carried ") != 3 {
			t.Errorf("second call = exit %d, tier invocations %q, stdout %q; want the listed tier, the vet run and the guard step carried", second.exit, got, second.stdout.String())
		}
		if line := vetSummary(t, second.stdout.String()); !strings.Contains(line, " carried ") {
			t.Errorf("second call vet summary line = %q; want it marked carried", line)
		}
	})

	t.Run("a tier whose command edits a tracked file and commits it before passing records nothing", func(t *testing.T) {
		if err := os.Remove(greenPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		committing := scopesConfigWith(t, "go", func(cfg map[string]any) {
			tier0 := cfg["tiers"].(map[string]any)["tier0"].(map[string]any)
			tier0["command"] = tier0["command"].(string) + " && echo more >> docs/note.md && git commit -q -am more"
		})
		if err := os.WriteFile(configPath, committing, 0o644); err != nil {
			t.Fatal(err)
		}
		defer writeScoper(t, "go")
		if g := real(t, "--all"); g.exit != 0 {
			t.Fatalf("committing tier = exit %d, stdout %q; want a pass", g.exit, g.stdout.String())
		}
		if entries := readGreen(greenPath).Entries; len(entries) != 0 {
			t.Errorf("green record = %+v; want nothing recorded for a tier that changed HEAD's tree", entries)
		}
	})

	// The rows from here read the diff base each tier takes: the merge base is the commit the prime stands on, since the pair was fast-forwarded onto it.
	mergeBase := gitkit.RevParse(t, prime, "HEAD")
	wantDerivation := func(t *testing.T, g *runningGate, base, tiers string) {
		t.Helper()
		if line := "gate test: diff scope from " + base + " for " + tiers; !strings.Contains(g.stdout.String(), line) {
			t.Errorf("stdout = %q; want it to hold %q", g.stdout.String(), line)
		}
	}
	// commitChange commits content to rel on the pair and returns the new commit.
	commitChange := func(t *testing.T, rel, content string) string {
		t.Helper()
		gitkit.CommitFile(t, pair, rel, content, "change "+rel)
		return gitkit.RevParse(t, pair, "HEAD")
	}
	removeGreen := func(t *testing.T) {
		t.Helper()
		if err := os.Remove(greenPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	t.Run("a diff run diffs from the newest whole-green ancestor, else from the parent merge base", func(t *testing.T) {
		removeGreen(t)
		commitChange(t, "other/other.go", "package other\n\nvar Seen = 1\n")
		g := gate(t, pair, nil)
		wantDerivation(t, g, mergeBase, "tier0")

		green := commitChange(t, "other/other.go", "package other\n\nvar Seen = 2\n")
		if whole := gate(t, pair, nil, "--all"); whole.exit != 0 {
			t.Fatalf("--all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 8 }\n")
		g = gate(t, pair, nil)
		wantDerivation(t, g, green, "tier0")
		wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
	})

	t.Run("a dirty worktree diffs from the parent merge base although the record holds a whole-green ancestor", func(t *testing.T) {
		removeGreen(t)
		if whole := gate(t, pair, nil, "--all"); whole.exit != 0 {
			t.Fatalf("--all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 9 }\n")
		edit(t, "docs/note.md", "a dirtying note\n")
		wantDerivation(t, gate(t, pair, nil), mergeBase, "tier0")
	})

	t.Run("tiers on one base share one derivation line and tiers on different bases print one each", func(t *testing.T) {
		separate := scopesConfigWith(t, "go", func(cfg map[string]any) {
			delete(cfg["tiers"].(map[string]any)["tier1"].(map[string]any), "covers")
		})
		if err := os.WriteFile(configPath, separate, 0o644); err != nil {
			t.Fatal(err)
		}
		defer writeScoper(t, "go")
		removeGreen(t)

		shared := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantDerivation(t, shared, mergeBase, "tier0, tier1")
		if count := strings.Count(shared.stdout.String(), "gate test: diff scope from"); count != 1 {
			t.Errorf("derivation lines = %d in stdout %q; want 1", count, shared.stdout.String())
		}

		removeGreen(t)
		green := commitChange(t, "other/other.go", "package other\n\nvar Seen = 3\n")
		if whole := gate(t, pair, nil, "--tier", "0", "--all"); whole.exit != 0 {
			t.Fatalf("--tier 0 --all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		commitChange(t, "other/other.go", "package other\n\nvar Seen = 4\n")
		split := gate(t, pair, nil, "--tier", "0", "--tier", "1")
		wantDerivation(t, split, green, "tier0")
		wantDerivation(t, split, mergeBase, "tier1")
		if count := strings.Count(split.stdout.String(), "gate test: diff scope from"); count != 2 {
			t.Errorf("derivation lines = %d in stdout %q; want 2", count, split.stdout.String())
		}
	})

	t.Run("a narrowed dotnet pass records nothing, a --all run records the tier whole, and two vstest tiers then carry", func(t *testing.T) {
		const project = "cs/Lib.Tests/Lib.Tests.csproj"
		writeDotnet(t, func(cfg map[string]any) {
			tiers := cfg["tiers"].(map[string]any)
			for _, name := range []string{"tier0", "tier1"} {
				tier := tiers[name].(map[string]any)
				tier["all_targets"] = project
				tier["counter"] = "vstest"
			}
			delete(tiers["tier1"].(map[string]any), "covers")
		})
		defer writeScoper(t, "go")
		removeGreen(t)

		base := gitkit.RevParse(t, pair, "HEAD")
		commitChange(t, "cs/Lib/Circle.cs", "namespace Lib;\n\npublic class Circle { public int Edge; }\n")
		wantArgs(t, gate(t, pair, nil, "--base", base), circleTestsArgs)
		if entries := readGreen(greenPath).Entries; len(entries) != 0 {
			t.Errorf("green record = %+v; want nothing recorded for a narrowed project run", entries)
		}

		wantArgs(t, gate(t, pair, nil, "--all"), []string{project})
		first := gate(t, pair, nil, "--all", "--tier", "0", "--tier", "1")
		wantArgs(t, first, []string{project})
		wantCarried(t, gate(t, pair, nil, "--all", "--tier", "0", "--tier", "1"))
	})

	t.Run("an --all run carries over a committed inert-only change from a whole ancestor and writes the current tree's entry", func(t *testing.T) {
		writeInert(t, "go", nil)
		defer writeScoper(t, "go")
		removeGreen(t)
		wantArgs(t, real(t, "--all"), []string{"./..."})

		current := commitChange(t, "docs/note.md", "an inert commit\n")
		wantCarried(t, real(t, "--all"))
		if !slices.ContainsFunc(readGreen(greenPath).Entries, func(entry greenEntry) bool { return entry.Commit == current && entry.Whole }) {
			t.Errorf("green record = %+v; want a whole entry for the current commit %s", readGreen(greenPath).Entries, current)
		}

		// The entry written for the current tree carries the next call directly: removing the ancestor's leaves it alone.
		record := readGreen(greenPath)
		record.Entries = slices.DeleteFunc(record.Entries, func(entry greenEntry) bool { return entry.Commit != current })
		removeGreen(t)
		for _, entry := range record.Entries {
			if err := recordGreen(greenPath, entry, entry.Written); err != nil {
				t.Fatal(err)
			}
		}
		wantCarried(t, real(t, "--all"))
	})

	t.Run("an --all run runs after a committed non-inert change although a whole ancestor is green", func(t *testing.T) {
		writeInert(t, "go", nil)
		defer writeScoper(t, "go")
		commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 10 }\n")
		wantArgs(t, real(t, "--all"), []string{"./..."})
	})

	t.Run("an --all run on a dirty tree runs whole although the history is inert-only", func(t *testing.T) {
		writeInert(t, "go", nil)
		defer writeScoper(t, "go")
		commitChange(t, "docs/note.md", "another inert commit\n")
		edit(t, "docs/note.md", "a dirtying note\n")
		wantArgs(t, real(t, "--all"), []string{"./..."})
	})

	t.Run("a carry: false tier runs again over inert-only history", func(t *testing.T) {
		writeInert(t, "go", func(cfg map[string]any) { cfg["tiers"].(map[string]any)["tier0"].(map[string]any)["carry"] = false })
		defer writeScoper(t, "go")
		removeGreen(t)
		wantArgs(t, real(t, "--all"), []string{"./..."})
		commitChange(t, "docs/note.md", "a third inert commit\n")
		wantArgs(t, real(t, "--all"), []string{"./..."})
	})

	t.Run("a changed inert_paths list voids the entries an inert carry would use", func(t *testing.T) {
		defer writeScoper(t, "go")
		writeInert(t, "go", nil)
		removeGreen(t)
		wantArgs(t, real(t, "--all"), []string{"./..."})
		writeInert(t, "go", func(cfg map[string]any) { cfg["inert_paths"] = []string{"*.md", "*.txt"} })
		commitChange(t, "docs/note.md", "a fourth inert commit\n")
		wantArgs(t, real(t, "--all"), []string{"./..."})
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
