// scopes_integration_test.go drives `lym gate test` over a hubforge hub with a task pair.
// The task pair's fabric origin record names the prime's branch as its parent.
// It reaches the computed, --base and --card scope forms.
// Each row runs over a hub of its own, and spawns git, `go list` and the stand-in tier command.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scopesConfig returns the gate.yaml of the scopes hub: tier0 over ./..., tier1 covering it, and the given scoper.
func scopesConfig(t *testing.T, scoper string) []byte {
	t.Helper()

	return scopesConfigWith(t, scoper, nil)
}

// scopesConfigWith returns the gate.yaml of the scopes hub with the given scoper, then lets mutate change the config.
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

// scopesHub is one hubforge hub seeded with the scopes module and a card plan, owned by a single row.
// The module lives on the prime's branch and the pair is fast-forwarded onto it,
// so the merge base the pair diffs from holds the module,
// and a computed scope sees only the edits a row makes.
type scopesHub struct {
	hub        *hubforge.Hub
	prime      string
	pair       string
	configPath string
	pool       *gateslot.Pool
	// plan is the one-card plan written under the pair's plan directory, which its code repository excludes.
	plan plankit.Plan
	// greenPath is the pair's green record.
	greenPath string
	// mergeBase is the commit the prime stands on, since the pair was fast-forwarded onto it.
	mergeBase string
}

// newScopesHub forges a hub with a task pair and writes the go-scoper config.
// It commits the seed module on the prime, fast-forwards the pair onto it and writes the card plan into the pair.
func newScopesHub(t *testing.T) *scopesHub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	f := &scopesHub{
		hub:        h,
		prime:      h.PrimeWorktree(),
		pair:       h.PairCodeWorktree(hubforge.TemplatePairSlug),
		configPath: configengine.ConfigFile(h.BoardDir(), "gate"),
		pool:       hubgeom.GateSlots(h.Location),
		plan: plankit.Plan{Approved: true, Cards: []plankit.Card{{
			Number: 1,
			Slug:   "span",
			Groups: []plankit.Group{{Label: "Edit", Targets: []string{"lib/lib.go", "nested/n.go"}}},
		}}},
	}
	f.writeScoper(t, "go")

	files := map[string]string{
		"go.mod":             "module example.com/scopes\n\ngo 1.21\n",
		"lib/lib.go":         "package lib\n\nfunc One() int { return 1 }\n",
		"user/user.go":       "package user\n\nimport \"example.com/scopes/lib\"\n\nfunc Two() int { return lib.One() + 1 }\n",
		"other/other.go":     "package other\n",
		"guardpkg/g.go":      "package guardpkg\n",
		"guardpkg/g_test.go": "package guardpkg\n\nimport \"testing\"\n\n//lym:guard\nfunc TestGuard(t *testing.T) {}\n",
		"watched/w.go":       "package watched\n",
		"watched/w_test.go":  "package watched\n\nimport \"testing\"\n\n//lym:watch *_linux.go\nfunc TestWatched(t *testing.T) {}\n",
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
	plankit.WriteTree(t, f.prime, files)
	gitkit.Git(t, f.prime, "add", "go.mod", "lib", "user", "other", "guardpkg", "watched", "nested", "docs", "cs")
	gitkit.Git(t, f.prime, "commit", "-q", "-m", "seed the scopes module")
	gitkit.Git(t, f.pair, "merge", "--ff-only", "-q", "main")
	f.mergeBase = gitkit.RevParse(t, f.prime, "HEAD")
	f.greenPath = filepath.Join(gateslot.WaitDir(f.pair), greenFileName)
	plankit.Write(t, planparser.PlanDir(f.pair), f.plan)
	return f
}

// writeScoper writes the hub config with the given scoper.
func (f *scopesHub) writeScoper(t *testing.T, scoper string) {
	t.Helper()
	if err := os.WriteFile(f.configPath, scopesConfig(t, scoper), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeDotnet writes the hub config with the dotnet scoper and Markdown files inert.
// The stand-in command `{targets} {filter}` runs on both tiers, and mutate, when not nil, changes the config last.
func (f *scopesHub) writeDotnet(t *testing.T, mutate func(cfg map[string]any)) {
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
	if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeInert writes the hub config with scoper and Markdown files inert, then lets mutate change the config.
func (f *scopesHub) writeInert(t *testing.T, scoper string, mutate func(cfg map[string]any)) {
	t.Helper()
	config := scopesConfigWith(t, scoper, func(cfg map[string]any) {
		cfg["inert_paths"] = []string{"*.md"}
		if mutate != nil {
			mutate(cfg)
		}
	})
	if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
}

// edit replaces the pair's file rel with content.
func (f *scopesHub) edit(t *testing.T, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.pair, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gate runs `lym gate test args...` from cwd and returns the finished gate.
func (f *scopesHub) gate(t *testing.T, cwd string, env []string, args ...string) *runningGate {
	t.Helper()
	g := startGate(t, gateRun{cwd: cwd, args: append([]string{"test"}, args...), env: env})
	g.exit = g.wait(t)
	return g
}

// requireFree waits until no gate slot of the hub has a holder.
func (f *scopesHub) requireFree(t *testing.T) {
	t.Helper()
	eventually(t, "the slots to be free", func() bool {
		holders, err := f.pool.Holders()
		return err == nil && len(holders) == 0
	})
}

// wantArgs checks that g exited 0 and that the tier command ran with exactly the invocations in want.
func (f *scopesHub) wantArgs(t *testing.T, g *runningGate, want ...[]string) {
	t.Helper()
	if g.exit != 0 {
		t.Errorf("exit code = %d, stdout %q; want 0", g.exit, g.stdout.String())
	}
	if got := g.tierArgs(t); !reflect.DeepEqual(got, want) {
		t.Errorf("tier invocations = %q; want %q", got, want)
	}
}

// wantRefusal checks that the gate refused before any slot or tier command with text holding each of wants.
func (f *scopesHub) wantRefusal(t *testing.T, g *runningGate, wants ...string) {
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
	f.requireFree(t)
}

// real runs `lym gate test args...` from the pair with the real go command, which the green record lists packages with.
func (f *scopesHub) real(t *testing.T, args ...string) *runningGate {
	t.Helper()
	g := startGate(t, gateRun{cwd: f.pair, realGo: true, args: append([]string{"test"}, args...)})
	g.exit = g.wait(t)
	return g
}

// wantCarried checks that g passed with every tier carried and ran no tier command.
func (f *scopesHub) wantCarried(t *testing.T, g *runningGate) {
	t.Helper()
	if g.exit != 0 || len(g.tierArgs(t)) != 0 || !strings.Contains(g.stdout.String(), " all carried ") {
		t.Errorf("gate = exit %d, tier invocations %q, stdout %q; want every tier carried", g.exit, g.tierArgs(t), g.stdout.String())
	}
}

// commitChange commits content to rel on the pair and returns the new commit.
func (f *scopesHub) commitChange(t *testing.T, rel, content string) string {
	t.Helper()
	gitkit.CommitFile(t, f.pair, rel, content, "change "+rel)
	return gitkit.RevParse(t, f.pair, "HEAD")
}

// removeGreen deletes the pair's green record when there is one.
func (f *scopesHub) removeGreen(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.greenPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// wantDerivation checks that g printed the diff scope derivation line for base and tiers.
func (f *scopesHub) wantDerivation(t *testing.T, g *runningGate, base, tiers string) {
	t.Helper()
	if line := "gate test: diff scope from " + base + " for " + tiers; !strings.Contains(g.stdout.String(), line) {
		t.Errorf("stdout = %q; want it to hold %q", g.stdout.String(), line)
	}
}

// noCarryTier1 drops tier1's covers and sets its carry to false.
func noCarryTier1(cfg map[string]any) {
	tier1 := cfg["tiers"].(map[string]any)["tier1"].(map[string]any)
	delete(tier1, "covers")
	tier1["carry"] = false
}

// wantInertEmpty checks that the gate named the inert diff and ran no command of the tiers that carry.
func wantInertEmpty(t *testing.T, g *runningGate) {
	t.Helper()
	if !strings.Contains(g.stdout.String(), "every changed file is inert, so the scope is empty") {
		t.Errorf("stdout = %q; want the inert-only diff named", g.stdout.String())
	}
}

// scopesSharedBuild declares the hub's build step over the stand-in and sets shared_build on tier0.
// It also clears the covers of tier1.
func scopesSharedBuild(cfg map[string]any) {
	cfg["build"] = map[string]any{"command": "'" + os.Args[0] + "' sharedbuild", "timeout_min": 1}
	tiers := cfg["tiers"].(map[string]any)
	tiers["tier0"].(map[string]any)["shared_build"] = true
	delete(tiers["tier1"].(map[string]any), "covers")
}

// vetSummary returns the summary line of tier1's vet run in output, failing the test when there is none.
func vetSummary(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "gate test: tier1 vet ") {
			return line
		}
	}
	t.Fatalf("output %q holds no summary line of tier1's vet run", output)
	return ""
}

func TestGateTest_Scopes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	guardInvocation := []string{"./guardpkg", "-run", "^(TestGuard)$"}
	circleTestsArgs := []string{"cs/Lib.Tests/Lib.Tests.csproj", "--filter", "FullyQualifiedName~Lib.Tests.CircleTests"}
	tier1File := "//go:build tier1\n\npackage lib\n\nimport \"testing\"\n\nfunc TestUse(t *testing.T) { _ = %s() }\n"

	t.Run("a computed scope under go runs the changed package and its importer, then the guard step's group through its tier's slots", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 2 }\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
		listed, _ := runRecordOf(t, g.stdout.String(), 0)
		guard, _ := guardRunRecordOf(t, g.stdout.String(), 0)
		if listed.ScopeForm != "computed" || listed.Slug != hubforge.TemplatePairSlug || !slices.Equal(listed.TierList, []int{0}) {
			t.Errorf("tier0 run record = %+v; want the computed scope of the pair's slug and the tier list [0]", listed)
		}
		if !guard.GuardStep || guard.Tier != 0 || guard.Slots != 1 || guard.ScopeForm != "guard" {
			t.Errorf("guard step run record = %+v; want the guard group of tier0, whose guard test sits in an untagged file, holding tier0's one slot", guard)
		}
		if strings.Contains(g.stdout.String(), notRunSuffix) {
			t.Errorf("stdout = %q; want no line naming the tiers a pass did not run, which only the card scope form prints", g.stdout.String())
		}
		f.requireFree(t)
	})

	t.Run("a computed scope over a change to a nested module's file falls back to whole, naming the reason, and plans no guard group for the tier it ran whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "nested/n.go", "package nested\n\nvar Changed = true\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "falls back to whole: nested/n.go is neither a Markdown file nor under a Go package directory") {
			t.Errorf("stdout = %q; want the fallback and its reason", g.stdout.String())
		}
	})

	t.Run("a Markdown-only change runs no tier command, logs zero and runs only the guard step", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "docs/note.md", "changed note\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.Discovered != 0 || run.ExitCode != 0 || !strings.Contains(g.stdout.String(), "the scope is empty, so tier0 runs no command") {
			t.Errorf("tier0 run record = %+v, stdout %q; want an empty-scope pass with nothing discovered", run, g.stdout.String())
		}
		if trimpath := trimpathLine("tier0", []string{"GOFLAGS=-trimpath -p=3"}); strings.Count(g.stdout.String(), trimpath) != 1 {
			t.Errorf("stdout = %q; want the -trimpath line once, for the guard group alone, since the listed tier runs no command", g.stdout.String())
		}
		f.requireFree(t)
	})

	t.Run("an inert-only diff under go runs no listed tier's command and still runs the guard step, and a carry: false tier beside it runs whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", noCarryTier1)
		f.edit(t, "docs/note.md", "inert note\n")
		g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantArgs(t, g, []string{"./..."}, guardInvocation)
		wantInertEmpty(t, g)
		if !strings.Contains(g.stdout.String(), "the scope is empty, so tier0 runs no command") {
			t.Errorf("stdout = %q; want tier0 to run no command", g.stdout.String())
		}
	})

	t.Run("an inert-only diff under none runs no listed tier's command and plans no guard step, and a carry: false tier beside it runs whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "none", noCarryTier1)
		f.edit(t, "docs/note.md", "inert note\n")
		g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantArgs(t, g, []string{"./..."})
		wantInertEmpty(t, g)
	})

	t.Run("an inert-only diff under dotnet runs no listed tier's command, and a carry: false tier beside it runs whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, noCarryTier1)
		f.edit(t, "docs/note.md", "inert note\n")
		f.wantArgs(t, f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1"), []string{"./..."})
	})

	t.Run("an untracked non-inert file beside an inert change keeps the go scope as it is derived", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.edit(t, "docs/note.md", "inert note\n")
		f.edit(t, "other/added.go", "package other\n\nvar Added = 1\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, []string{"./other"}, guardInvocation)
		if strings.Contains(g.stdout.String(), "every changed file is inert") {
			t.Errorf("stdout = %q; want the diff not held inert", g.stdout.String())
		}
	})

	t.Run("a dotnet scope that selected no test class runs the build once in tier0's slots, runs no tier0 command and runs tier1 whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, scopesSharedBuild)
		f.edit(t, "cs/Lib/Unused.cs", "namespace Lib;\n\npublic class Unused { public int Edge; }\n")
		g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantArgs(t, g, []string{"./..."})
		if got := buildRecords(t, g); len(got) != 1 || filepath.Dir(got[0].Slot) != f.pool.Dir {
			t.Errorf("build invocations = %+v; want the one build, inside a slot of %s", got, f.pool.Dir)
		}
		if !strings.Contains(g.stdout.String(), "gate test: the scope selected no test class, so tier1 runs whole") {
			t.Errorf("stdout = %q; want tier1 named as run whole", g.stdout.String())
		}
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.Discovered != 0 || run.ExitCode != 0 {
			t.Errorf("tier0 run record = %+v; want an empty pass", run)
		}
		f.requireFree(t)

		failing := startGate(t, gateRun{cwd: f.pair, args: []string{"test", "--tier", "0", "--tier", "1"}, env: []string{helperBuildExitEnv + "=3"}})
		if code := failing.wait(t); code != 3 {
			t.Errorf("exit code with a build exiting 3 = %d, stdout %q; want 3", code, failing.stdout.String())
		}
	})

	t.Run("an inert-only dotnet diff and an empty one run no build", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, scopesSharedBuild)
		wantNothingRun := func(name string) {
			t.Helper()
			g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
			if got := g.records(t); g.exit != 0 || len(got) != 0 {
				t.Errorf("%s diff: exit %d, stand-in records %+v; want no build and no tier command", name, g.exit, got)
			}
		}
		wantNothingRun("empty")
		f.edit(t, "docs/note.md", "inert note\n")
		wantNothingRun("inert-only")
	})

	t.Run("an empty dotnet diff with a carry: false tier that opts into the build runs the build before that tier's whole command", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, func(cfg map[string]any) {
			scopesSharedBuild(cfg)
			cfg["tiers"].(map[string]any)["tier0"].(map[string]any)["carry"] = false
		})
		g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		records := g.records(t)
		if g.exit != 0 || len(records) != 2 || !records[0].Build || records[1].Build || !slices.Equal(records[1].Args, []string{"./..."}) {
			t.Errorf("gate = exit %d, stand-in records %+v; want the build, then tier0's whole command", g.exit, records)
		}
	})

	t.Run("--base runs the packages the diff from that revision reaches, then the guard step", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		base := gitkit.RevParse(t, f.pair, "HEAD")
		f.edit(t, "user/user.go", "package user\n\nimport \"example.com/scopes/lib\"\n\nfunc Two() int { return lib.One() + 2 }\n")
		g := f.gate(t, f.pair, nil, "--base", base)
		f.wantArgs(t, g, []string{"./user"}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "base" {
			t.Errorf("tier0 run record scope form = %q; want base", run.ScopeForm)
		}
	})

	t.Run("a computed scope skips a //lym:watch test of an impacted package when the diff touches no file its pattern names, and runs it when the diff adds one", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "watched/w.go", "package watched\n\nvar Changed = true\n")
		f.wantArgs(t, f.gate(t, f.pair, nil), []string{"./watched", "-skip", "^(TestWatched)$"}, guardInvocation)

		f.edit(t, "watched/x_linux.go", "package watched\n")
		f.wantArgs(t, f.gate(t, f.pair, nil), []string{"./watched"}, guardInvocation)
	})

	t.Run("a computed scope under dotnet runs the selected test project once with its class filter", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, nil)
		f.edit(t, "cs/Lib/Circle.cs", "namespace Lib;\n\npublic class Circle { public int Radius; }\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, circleTestsArgs)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "computed" {
			t.Errorf("tier0 run record scope form = %q; want computed", run.ScopeForm)
		}
	})

	t.Run("a project file change under dotnet falls back to whole, naming the path", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, nil)
		f.edit(t, "cs/Lib/Lib.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\" />\n")
		g := f.gate(t, f.pair, nil)
		f.wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "falls back to whole: cs/Lib/Lib.csproj is neither inert nor a .cs file") {
			t.Errorf("stdout = %q; want the fallback and its reason", g.stdout.String())
		}
	})

	t.Run("--card runs a card's packages and the packages that import them once per module, then the guard step over the guard tests outside them", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		g := f.gate(t, f.pair, nil, "--card", "1")
		f.wantArgs(t, g, []string{"./lib", "./user"}, []string{"."}, guardInvocation)
		var cwds []string
		for _, record := range g.tierRecords(t) {
			cwds = append(cwds, record.Cwd)
		}
		if want := []string{f.pair, filepath.Join(f.pair, "nested"), f.pair}; !slices.Equal(cwds, want) {
			t.Errorf("tier cwds = %q; want the root module, the nested module, then the guard step in the root module: %q", cwds, want)
		}
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" {
			t.Errorf("tier0 run record scope form = %q; want card", run.ScopeForm)
		}
	})

	t.Run("--card runs a dependent package's guard in the guard step unless a listed tier is or covers its tier", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		plankit.WriteTree(t, f.pair, map[string]string{
			"consumer/c.go":      "package consumer\n\nimport \"example.com/scopes/lib\"\n\nvar One = lib.One\n",
			"consumer/c_test.go": "//go:build tier1\n\npackage consumer\n\nimport \"testing\"\n\n//lym:guard\nfunc TestConsumerGuard(t *testing.T) {}\n",
			// The listed tier0, and tier1 which covers it, both run this one, so the guard step never names it.
			"consumer/c0_test.go": "package consumer\n\nimport \"testing\"\n\n//lym:guard\nfunc TestConsumerTierZeroGuard(t *testing.T) {}\n",
		})
		scoped := []string{"./lib", "./consumer", "./user"}

		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1"),
			scoped, []string{"."},
			append([]string{"vet", "-tags", "tier1"}, scoped...), []string{"vet", "-tags", "tier1", "."},
			guardInvocation, []string{"./consumer", "-run", "^(TestConsumerGuard)$"})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1", "--tier", "1"), scoped, []string{"."}, guardInvocation)
	})

	t.Run("--gate batch runs tier1 over the cards' and main packages alone, with their own watch skip, and its guard step takes a dependent's tier1 guard, unless tier1 covers tier0", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		// A watched test runs when its own file changed, so this file sits at the merge base.
		gitkit.CommitFile(t, f.prime, "consumer/c0_test.go",
			"package consumer\n\nimport \"testing\"\n\n//lym:guard\nfunc TestConsumerTierZeroGuard(t *testing.T) {}\n\n"+
				"//lym:watch *_linux.go\nfunc TestConsumerWatched(t *testing.T) {}\n",
			"add the consumer tests")
		gitkit.Git(t, f.pair, "merge", "--ff-only", "-q", "main")
		plankit.WriteTree(t, f.pair, map[string]string{
			"consumer/c.go":      "package consumer\n\nimport \"example.com/scopes/lib\"\n\nvar One = lib.One\n",
			"consumer/c_test.go": "//go:build tier1\n\npackage consumer\n\nimport \"testing\"\n\n//lym:guard\nfunc TestConsumerGuard(t *testing.T) {}\n",
			"tool/main.go":       "package main\n\nfunc main() {}\n",
		})
		writeCardKeys := func(tier1CoversTier0 bool) {
			t.Helper()
			config := scopesConfigWith(t, "go", func(cfg map[string]any) {
				if !tier1CoversTier0 {
					delete(cfg["tiers"].(map[string]any)["tier1"].(map[string]any), "covers")
				}
				cfg["gates"] = map[string]any{
					"batch": map[string]any{"tiers": []int{0, 1}, "scope": "card"},
					"card":  map[string]any{"tiers": []int{0, 1}, "scope": "card"},
				}
			})
			if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		scoped := []string{"./lib", "./consumer", "./user", "-skip", "^(TestConsumerWatched)$"}
		nested := []string{".", "-skip", "^(TestConsumerWatched)$"}
		scopedWithMain := []string{"./lib", "./consumer", "./user", "./tool", "-skip", "^(TestConsumerWatched)$"}

		writeCardKeys(false)
		batch := f.gate(t, f.pair, nil, "--gate", "batch", "--card", "1", "--main-packages")
		f.wantArgs(t, batch, scopedWithMain, nested, []string{"./lib", "./tool"}, []string{"."}, guardInvocation, []string{"./consumer", "-run", "^(TestConsumerGuard)$"})
		if want := "gate test: --gate batch runs tier1 over the cards' packages alone, not the packages that depend on them"; !strings.Contains(batch.stdout.String(), want) {
			t.Errorf("stdout = %q; want it to hold %q", batch.stdout.String(), want)
		}
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "card", "--card", "1"), scoped, nested, scoped, nested, guardInvocation)

		writeCardKeys(true)
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "batch", "--card", "1"), scoped, nested, guardInvocation)
	})

	t.Run("a computed scope runs an impacted package's guard in the guard step unless a listed tier is or covers its tier", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		plankit.WriteTree(t, f.pair, map[string]string{
			"consumer/c.go":      "package consumer\n\nimport \"example.com/scopes/lib\"\n\nvar One = lib.One\n",
			"consumer/c_test.go": "//go:build tier1\n\npackage consumer\n\nimport \"testing\"\n\n//lym:guard\nfunc TestConsumerGuard(t *testing.T) {}\n",
		})
		f.edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 3 }\n")
		scoped := []string{"./consumer", "./lib", "./user"}

		f.wantArgs(t, f.gate(t, f.pair, nil), scoped, guardInvocation, []string{"./consumer", "-run", "^(TestConsumerGuard)$"})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--tier", "1"), scoped, guardInvocation)
	})

	t.Run("a --gate call skips a //lym:only test its marker does not name, a marked key runs only its marked tests or is refused, and a marker naming no gate key is refused", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		plankit.WriteTree(t, f.pair, map[string]string{
			"heldpkg/h.go": "package heldpkg\n",
			"heldpkg/h_test.go": "package heldpkg\n\nimport \"testing\"\n\n//lym:only upkeep\nfunc TestHeld(t *testing.T) {}\n\n" +
				"//lym:guard\n//lym:only finalize\nfunc TestLandingGuard(t *testing.T) {}\n",
			"nested/n_test.go": "package nested\n\nimport \"testing\"\n\n//lym:only upkeep\nfunc TestNestedHeld(t *testing.T) {}\n",
		})

		finalize := f.gate(t, f.pair, nil, "--gate", "finalize")
		f.wantArgs(t, finalize, []string{"./...", "-skip", "^(TestHeld)$"})
		if want := `tier0 skips TestHeld, since their //lym:only markers do not name --gate "finalize"`; !strings.Contains(finalize.stdout.String(), want) {
			t.Errorf("stdout = %q; want the held line %q", finalize.stdout.String(), want)
		}
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), []string{"./heldpkg", "-run", "^(TestHeld)$"})
		// A test of the same name that another package holds back leaves the marked one running.
		plankit.WriteTree(t, f.pair, map[string]string{
			"samename/s_test.go": "package samename\n\nimport \"testing\"\n\n//lym:only finalize\nfunc TestHeld(t *testing.T) {}\n",
		})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), []string{"./heldpkg", "-run", "^(TestHeld)$"})
		if err := os.RemoveAll(filepath.Join(f.pair, "samename")); err != nil {
			t.Fatal(err)
		}
		f.wantArgs(t, f.gate(t, f.pair, nil, "--all"), []string{"./..."})

		// The card scope leaves heldpkg out, so only the guard step could run its held guard.
		skip := "^(TestHeld|TestLandingGuard)$"
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "card", "--card", "1"),
			[]string{"./lib", "./user", "-skip", skip}, []string{".", "-skip", skip}, guardInvocation)

		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "upkeep", "./heldpkg"), []string{"./heldpkg", "-skip", "^(TestLandingGuard)$"})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "finalize", "-C", "nested", "."), []string{".", "-skip", "^(TestNestedHeld)$"})

		f.writeScoper(t, "none")
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), "needs the go scoper", `"none"`)
		f.writeScoper(t, "go")

		// The key lists tier0 alone, and its marked tier1 test still runs, under tier1's command.
		tierOne := filepath.Join(f.pair, "heldpkg", "h1_test.go")
		plankit.WriteTree(t, f.pair, map[string]string{
			"heldpkg/h1_test.go": "//go:build tier1\n\npackage heldpkg\n\nimport \"testing\"\n\n//lym:only upkeep\nfunc TestHeldTierOne(t *testing.T) {}\n",
		})
		bothTiers := [][]string{{"./heldpkg", "-run", "^(TestHeld)$"}, {"./heldpkg", "-run", "^(TestHeldTierOne)$"}}
		upkeep := f.gate(t, f.pair, nil, "--gate", "upkeep")
		f.wantArgs(t, upkeep, bothTiers...)
		// The two tiers share one command, so the settings tell them apart:
		// tier1's slots: all records as 0, where tier0 records 1.
		if run, _ := guardRunRecordOf(t, upkeep.stdout.String(), 1); !run.GuardStep || run.Tier != 1 || run.Slots != 0 {
			t.Errorf("tier1 guard run record = %+v; want a guard step of tier1 under its slots: all", run)
		}

		// A manual tier1 runs its marked test only for a key that lists it or a tier covering it.
		writeManualTierOne := func(mutate func(cfg map[string]any)) {
			config := scopesConfigWith(t, "go", func(cfg map[string]any) {
				cfg["tiers"].(map[string]any)["tier1"].(map[string]any)["manual"] = true
				if mutate != nil {
					mutate(cfg)
				}
			})
			if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		writeManualTierOne(nil)
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), "marks TestHeldTierOne in tier1, which is manual", "list tier1 in gate.gates.upkeep")
		writeManualTierOne(func(cfg map[string]any) {
			cfg["gates"] = map[string]any{"upkeep": map[string]any{"tiers": []int{0, 1}, "scope": "marked"}}
		})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), bothTiers...)
		writeManualTierOne(func(cfg map[string]any) {
			cfg["tiers"].(map[string]any)["tier6"].(map[string]any)["covers"] = []int{1}
			cfg["gates"] = map[string]any{"upkeep": map[string]any{"tiers": []int{6}, "scope": "marked"}}
		})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), bothTiers...)
		f.writeScoper(t, "go")
		if err := os.Remove(tierOne); err != nil {
			t.Fatal(err)
		}

		if err := os.Remove(filepath.Join(f.pair, "nested", "n_test.go")); err != nil {
			t.Fatal(err)
		}
		f.edit(t, "heldpkg/h_test.go", "package heldpkg\n\nimport \"testing\"\n\n//lym:only finalize\nfunc TestHeld(t *testing.T) {}\n")
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), "is marked //lym:only upkeep", "way forward")

		f.edit(t, "heldpkg/h_test.go", "package heldpkg\n\nimport \"testing\"\n\n//lym:only nosuch\nfunc TestHeld(t *testing.T) {}\n")
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "finalize"), `TestHeld in heldpkg names the gate key "nosuch"`, "way forward")

		f.edit(t, "go.mod", "this is not a go.mod\n")
		unlistable := f.gate(t, f.pair, nil, "--gate", "finalize")
		f.wantArgs(t, unlistable, []string{"./..."})
		if want := "cannot list the //lym:only tests of"; !strings.Contains(unlistable.stdout.String(), want) {
			t.Errorf("stdout = %q; want it to hold %q", unlistable.stdout.String(), want)
		}
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "upkeep"), "cannot list the packages of", "way forward")
	})

	t.Run("--card under dotnet runs the project of the test class naming a card's member glyph target", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, nil)
		csharpPlan := f.plan
		csharpPlan.Language = "csharp"
		csharpPlan.Cards = []plankit.Card{{Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"Lib#Circle.Area"}}}}}
		plankit.Write(t, planparser.PlanDir(f.pair), csharpPlan)
		g := f.gate(t, f.pair, nil, "--card", "1")
		f.wantArgs(t, g, circleTestsArgs)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" {
			t.Errorf("tier0 run record scope form = %q; want card", run.ScopeForm)
		}
	})

	t.Run("--card under dotnet names a selected project that a tier0 command without {targets} does not reach, and still exits 0", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, func(cfg map[string]any) {
			tiers := cfg["tiers"].(map[string]any)
			tiers["tier0"].(map[string]any)["command"] = "'" + os.Args[0] + "' cs/Other/Other.csproj {filter}"
		})
		csharpPlan := f.plan
		csharpPlan.Language = "csharp"
		csharpPlan.Cards = []plankit.Card{{Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"Lib#Circle.Area"}}}}}
		plankit.Write(t, planparser.PlanDir(f.pair), csharpPlan)
		g := f.gate(t, f.pair, nil, "--card", "1")
		if g.exit != 0 {
			t.Errorf("exit code = %d, stdout %q; want 0, since the line is output only", g.exit, g.stdout.String())
		}
		want := "gate test: the scope selected tests in cs/Lib.Tests/Lib.Tests.csproj, and no tier this call runs reaches that project, so this pass is not green for it; way forward: run \"lym gate test --tier 1 cs/Lib.Tests/Lib.Tests.csproj\""
		if !strings.Contains(g.stdout.String(), want) {
			t.Errorf("stdout = %q; want it to hold %q", g.stdout.String(), want)
		}
	})

	t.Run("--card skips a //lym:watch test of a card's package that no change since the parent merge base concerns, and runs it once one does or with no parent branch", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		watchedPlan := f.plan
		watchedPlan.Cards = []plankit.Card{{Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"watched/w.go"}}}}}
		plankit.Write(t, planparser.PlanDir(f.pair), watchedPlan)
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1"), []string{"./watched", "-skip", "^(TestWatched)$"}, guardInvocation)

		f.edit(t, "watched/x_linux.go", "package watched\n")
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1"), []string{"./watched"}, guardInvocation)

		plankit.Write(t, planparser.PlanDir(f.prime), watchedPlan)
		f.wantArgs(t, f.gate(t, f.prime, nil, "--card", "1"), []string{"./watched"}, guardInvocation)
	})

	t.Run("--card vets a card's packages per module under the tag of a tier that adds a file to one, between the listed tier and the guard step", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "lib/use_test.go", fmt.Sprintf(tier1File, "One"))
		g := f.gate(t, f.pair, nil, "--card", "1")
		f.wantArgs(t, g, []string{"./lib", "./user"}, []string{"."}, []string{"vet", "-tags", "tier1", "./lib", "./user"}, []string{"vet", "-tags", "tier1", "."}, guardInvocation)
		if line := vetSummary(t, g.stdout.String()); !strings.Contains(line, " exit 0 ") {
			t.Errorf("vet summary line = %q; want exit 0", line)
		}
		if want := "gate test: tier1, tier2, tier3, tier5, tier6, tier7" + notRunSuffix + "\n"; !strings.Contains(g.stdout.String(), want) {
			t.Errorf("stdout = %q; want the line %q, which names tier1 since its vet run does not run the tier", g.stdout.String(), want)
		}
		f.requireFree(t)
	})

	t.Run("--card fails a card whose tier1 file uses a helper only a tier0 file defines, naming the vet run", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "lib/helper_test.go", "//go:build tier0\n\npackage lib\n\nfunc helper() int { return One() }\n")
		f.edit(t, "lib/use_test.go", fmt.Sprintf(tier1File, "helper"))
		g := startGate(t, gateRun{cwd: f.pair, args: []string{"test", "--card", "1"}, realGo: true})
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
		if strings.Contains(output, notRunSuffix) {
			t.Errorf("stdout = %q; want no line naming the tiers a pass did not run, since the call failed", output)
		}
		_, logPath, _ := strings.Cut(vetSummary(t, output), " log ")
		if vet, _ := runRecordAt(t, logPath, 1); !slices.Equal(vet.TierList, []int{0}) {
			t.Errorf("vet run record tier list = %v; want [0], the listed tiers alone", vet.TierList)
		}
		if guard, _ := guardRunRecordOf(t, output, 0); !guard.GuardStep {
			t.Errorf("guard run record = %+v; want the guard step to have run after the failed vet", guard)
		}
		f.requireFree(t)
	})

	t.Run("--card with a tier that outlives timeout_min across its invocations is killed with the timeout line", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		g := f.gate(t, f.pair, []string{helperHoldEnv + "=1", helperUnitEnv + "=1s"}, "--card", "1")
		records := g.tierRecords(t)
		t.Cleanup(func() {
			for _, record := range records {
				_ = (&os.Process{Pid: record.Grandchild}).Kill()
			}
		})
		// The timed-out tier stops its own invocations, but not the guard step after it.
		if got, want := g.tierArgs(t), [][]string{{"./lib", "./user"}, guardInvocation}; !reflect.DeepEqual(got, want) {
			t.Errorf("tier invocations = %q; want the first module's run alone, killed before the second starts, then the guard step: %q", got, want)
		}
		if tier, ok := gateslot.FindTierTimeout(g.stdout.String()); g.exit != 1 || !ok || tier != "tier0" {
			t.Errorf("gate = exit %d, FindTierTimeout = (%q, %v), stdout %q; want exit 1 and the tier0 timeout line", g.exit, tier, ok, g.stdout.String())
		}
		f.requireFree(t)
	})

	for _, scoper := range []string{"go", "none", "dotnet"} {
		t.Run("--card with an unknown id is refused before any slot naming the plan's ids under "+scoper, func(t *testing.T) {
			t.Parallel()
			f := newScopesHub(t)
			f.writeScoper(t, scoper)
			f.wantRefusal(t, f.gate(t, f.pair, nil, "--card", "9"), "1-span", "way forward")
		})
	}

	t.Run("--card in a worktree with no plan is refused naming --all", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.wantRefusal(t, f.gate(t, f.prime, nil, "--card", "1"), "--all", "explicit targets")
	})

	t.Run("--gate with scope card and no --card is refused naming the key and the way forward", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "card"), `"card"`, "--card <id>", "way forward")
	})

	t.Run("--gate with scope card runs the --card packages and records the key", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "docs/note.md", "a dirtying note\n")
		g := f.gate(t, f.pair, nil, "--gate", "card", "--card", "1")
		f.wantArgs(t, g, []string{"./lib", "./user"}, []string{"."}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "card" || run.GateKey != "card" {
			t.Errorf("tier0 run record = %+v; want the card scope form and gate key card", run)
		}
		if want := "gate test: tier1, tier2, tier3, tier5, tier6, tier7" + notRunSuffix + "\n"; !strings.Contains(g.stdout.String(), want) {
			t.Errorf("stdout = %q; want the line naming every declared tier the pass did not run, %q", g.stdout.String(), want)
		}
	})

	t.Run("--card with --main-packages adds the module's main packages to the card's packages", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		plankit.WriteTree(t, f.pair, map[string]string{"tool/main.go": "package main\n\nfunc main() {}\n"})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1", "--main-packages"), []string{"./lib", "./user", "./tool"}, []string{"."}, guardInvocation)
	})

	t.Run("--card with --main-packages is refused naming the module and the way forward when go list fails there", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "go.mod", "this is not a go.mod\n")
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--card", "1", "--main-packages"), "--main-packages", "module at .", "way forward")
	})

	t.Run("--card with --main-packages adds nothing under the none scoper", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeScoper(t, "none")
		plankit.WriteTree(t, f.pair, map[string]string{"tool/main.go": "package main\n\nfunc main() {}\n"})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1", "--main-packages"), []string{"./..."})
	})

	t.Run("--card with --main-packages adds nothing under dotnet", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeDotnet(t, nil)
		csharpPlan := f.plan
		csharpPlan.Language = "csharp"
		csharpPlan.Cards = []plankit.Card{{Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"Lib#Circle.Area"}}}}}
		plankit.Write(t, planparser.PlanDir(f.pair), csharpPlan)
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1", "--main-packages"), circleTestsArgs)
	})

	t.Run("a passing --card call leaves a tier a listed tier covers out of the tiers it did not run", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "docs/note.md", "a dirtying note\n")
		g := f.gate(t, f.pair, nil, "--card", "1", "--tier", "1")
		if want := "gate test: tier2, tier3, tier5, tier6, tier7" + notRunSuffix + "\n"; g.exit != 0 || !strings.Contains(g.stdout.String(), want) {
			t.Errorf("gate = exit %d, stdout %q; want exit 0 and the line %q, without tier0, which tier1 covers", g.exit, g.stdout.String(), want)
		}
	})

	t.Run("--gate with scope all runs every tier whole, ignoring --card ids", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		for i, key := range []string{"finalize", "adopt"} {
			// Each key's run is over a tree of its own, so the previous green run is not carried over it.
			f.edit(t, "lib/lib.go", fmt.Sprintf("package lib\n\nfunc One() int { return %d }\n", 4+10*i))
			g := f.gate(t, f.pair, nil, "--gate", key, "--card", "1")
			f.wantArgs(t, g, []string{"./..."})
			if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "all" || run.GateKey != key {
				t.Errorf("tier0 run record = %+v; want the all scope form and gate key %s", run, key)
			}
		}
	})

	t.Run("--gate with scope all over an inert-only diff from the parent branch runs no tier's command", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.edit(t, "docs/note.md", "inert note for finalize\n")
		g := f.gate(t, f.pair, nil, "--gate", "finalize")
		f.wantArgs(t, g)
		if out := g.stdout.String(); !strings.Contains(out, `scope all of --gate "finalize": every file changed since`) || !strings.Contains(out, "the scope is empty, so tier0 runs no command") {
			t.Errorf("stdout = %q; want the inert-only diff named and tier0 running no command", out)
		}
	})

	t.Run("--gate with scope all runs whole over an inert-only edit in the prime, which has no parent branch", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		note := filepath.Join(f.prime, "docs", "note.md")
		if err := os.WriteFile(note, []byte("inert note edited in the prime\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		g := f.gate(t, f.prime, nil, "--gate", "finalize")
		f.wantArgs(t, g, []string{"./..."})
		if strings.Contains(g.stdout.String(), "the scope is empty") {
			t.Errorf("stdout = %q; want no empty-scope line, since the prime has no parent branch to diff from", g.stdout.String())
		}
	})

	t.Run("--gate with scope all runs whole and names why when the parent branch's merge base cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		recorded, found, err := fabricengine.ReadOriginFor(f.hub.Location, hubforge.TemplatePairSlug)
		if err != nil || !found {
			t.Fatalf("read the pair's origin record = %v, found %v; want it present", err, found)
		}
		vanished := recorded
		vanished.ParentBranch = "vanished-parent-branch"
		rewrite := func(origin fabriclayout.Origin) error {
			return fabricengine.WriteOrigin(new(fabricengine.Mutations), f.hub.Location, hubforge.TemplatePairSlug, origin)
		}
		if err := rewrite(vanished); err != nil {
			t.Fatal(err)
		}
		f.edit(t, "docs/note.md", "inert note with a vanished parent\n")
		g := f.gate(t, f.pair, nil, "--gate", "finalize")
		f.wantArgs(t, g, []string{"./..."})
		if out := g.stdout.String(); !strings.Contains(out, `the scope of --gate "finalize" stays whole: cannot read the merge base`) || strings.Contains(out, "the scope is empty") {
			t.Errorf("stdout = %q; want the stays-whole line and no empty-scope line", out)
		}
	})

	t.Run("--gate with scope diff runs the computed scope", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		config := scopesConfigWith(t, "go", func(cfg map[string]any) {
			cfg["gates"] = map[string]any{"pr-review": map[string]any{"tiers": []int{0}, "scope": "diff"}}
		})
		if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
			t.Fatal(err)
		}
		f.edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 5 }\n")
		g := f.gate(t, f.pair, nil, "--gate", "pr-review")
		f.wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "computed" || run.GateKey != "pr-review" {
			t.Errorf("tier0 run record = %+v; want the computed scope form and gate key pr-review", run)
		}
	})

	t.Run("--gate with scope diff runs whole on a whole fallback, and takes all_targets under the none scoper", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "nested/n.go", "package nested\n\nvar Changed = 2\n")
		g := f.gate(t, f.pair, nil, "--gate", "darn-write")
		f.wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "falls back to whole: nested/n.go is neither a Markdown file nor under a Go package directory") {
			t.Errorf("stdout = %q; want the whole fallback", g.stdout.String())
		}

		f.writeScoper(t, "none")
		f.edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 11 }\n")
		g = f.gate(t, f.pair, nil, "--gate", "darn-write")
		f.wantArgs(t, g, []string{"./..."})
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "computed" || run.GateKey != "darn-write" {
			t.Errorf("tier run record = %+v; want the computed scope form and gate key darn-write", run)
		}
	})

	t.Run("explicit targets beside --gate replace the key's scope", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		g := f.gate(t, f.pair, nil, "--gate", "card", "./other")
		f.wantArgs(t, g, []string{"./other"})
		if run, _ := runRecordOf(t, g.stdout.String(), 0); run.ScopeForm != "targets" || run.GateKey != "card" {
			t.Errorf("tier0 run record = %+v; want the targets scope form and gate key card", run)
		}
	})

	t.Run("--gate with an unknown key is refused with the config's way forward naming lym config gate", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.wantRefusal(t, f.gate(t, f.pair, nil, "--gate", "nosuch"), `"nosuch"`, "lym config gate")
	})

	t.Run("a misplaced guard marker refuses a computed call before any slot with its file and line", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.edit(t, "other/o_test.go", "package other\n\n//lym:guard\nfunc helper() {}\n")
		f.wantRefusal(t, f.gate(t, f.pair, nil), "other/o_test.go:3", "way forward")
	})

	t.Run("the prime with no parent runs whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		g := f.gate(t, f.prime, nil)
		f.wantArgs(t, g, []string{"./..."})
		if strings.Contains(g.stdout.String(), "falls back") || strings.Contains(g.stdout.String(), "diff scope from") {
			t.Errorf("stdout = %q; want no fallback or derivation notice, since there is no parent to diff from", g.stdout.String())
		}
	})

	t.Run("under the none scoper every scope form takes all_targets", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeScoper(t, "none")
		base := gitkit.RevParse(t, f.pair, "HEAD")

		f.edit(t, "lib/lib.go", "package lib\n\nfunc One() int { return 3 }\n")
		f.wantArgs(t, f.gate(t, f.pair, nil), []string{"./..."})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--card", "1"), []string{"./..."})
		f.wantArgs(t, f.gate(t, f.pair, nil, "--base", base), []string{"./..."})
	})

	t.Run("an all run after a green diff run runs only the packages the diff run did not cover, and a third all run carries whole", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		gitkit.CommitFile(t, f.pair, "lib/lib.go", "package lib\n\nfunc One() int { return 7 }\n", "change lib")
		base := gitkit.RevParse(t, f.pair, "HEAD~1")

		diff := f.real(t, "--base", base)
		f.wantArgs(t, diff, []string{"./lib", "./user"}, guardInvocation)

		second := f.real(t, "--all")
		f.wantArgs(t, second, []string{"./guardpkg", "./other", "./watched"})

		f.wantCarried(t, f.real(t, "--all"))
	})

	t.Run("a changed tier command, a different tree and -- -count=1 each break the carry", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
		f.wantCarried(t, f.real(t, "--all"))

		if withArgs := f.real(t, "--all", "--", "-count=1"); withArgs.exit != 0 || !reflect.DeepEqual(withArgs.tierArgs(t), [][]string{{"./...", "-count=1"}}) {
			t.Errorf("--all -- -count=1 = exit %d, tier invocations %q; want the tier to run whole under the new args", withArgs.exit, withArgs.tierArgs(t))
		}
		f.wantCarried(t, f.real(t, "--all"))

		gitkit.CommitFile(t, f.pair, "docs/note.md", "a committed note\n", "change the tree")
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})

		changed := scopesConfigWith(t, "go", func(cfg map[string]any) {
			tier0 := cfg["tiers"].(map[string]any)["tier0"].(map[string]any)
			tier0["command"] = tier0["command"].(string) + " extra"
		})
		if err := os.WriteFile(f.configPath, changed, 0o644); err != nil {
			t.Fatal(err)
		}
		f.wantArgs(t, f.real(t, "--all"), []string{"./...", "extra"})
	})

	t.Run("--card carries a vet run that passed on the same clean tree", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		gitkit.CommitFile(t, f.pair, "lib/use_test.go", fmt.Sprintf(tier1File, "One"), "add a tier1 file")

		first := f.real(t, "--card", "1")
		if line := vetSummary(t, first.stdout.String()); first.exit != 0 || !strings.Contains(line, " exit 0 ") || strings.Contains(line, " carried ") {
			t.Fatalf("first call = exit %d, vet summary line %q; want a pass that ran the vet", first.exit, line)
		}

		second := f.real(t, "--card", "1")
		// A --card call never prints the all scope form, so this call counts carried lines instead of using wantCarried.
		if got := second.tierArgs(t); second.exit != 0 || len(got) != 0 || strings.Count(second.stdout.String(), " carried ") != 3 {
			t.Errorf("second call = exit %d, tier invocations %q, stdout %q; want the listed tier, the vet run and the guard step carried", second.exit, got, second.stdout.String())
		}
		if line := vetSummary(t, second.stdout.String()); !strings.Contains(line, " carried ") {
			t.Errorf("second call vet summary line = %q; want it marked carried", line)
		}
	})

	t.Run("a tier whose command edits a tracked file and commits it before passing records nothing", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		committing := scopesConfigWith(t, "go", func(cfg map[string]any) {
			tier0 := cfg["tiers"].(map[string]any)["tier0"].(map[string]any)
			tier0["command"] = tier0["command"].(string) + " && echo more >> docs/note.md && git commit -q -am more"
		})
		if err := os.WriteFile(f.configPath, committing, 0o644); err != nil {
			t.Fatal(err)
		}
		if g := f.real(t, "--all"); g.exit != 0 {
			t.Fatalf("committing tier = exit %d, stdout %q; want a pass", g.exit, g.stdout.String())
		}
		if entries := readGreen(f.greenPath).Entries; len(entries) != 0 {
			t.Errorf("green record = %+v; want nothing recorded for a tier that changed HEAD's tree", entries)
		}
	})

	t.Run("the green entry of a run that held tests back, and the one an inert carry writes, name them", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.commitChange(t, "heldpkg/h.go", "package heldpkg\n")
		held := f.commitChange(t, "heldpkg/h_test.go", "package heldpkg\n\nimport \"testing\"\n\n//lym:only upkeep\nfunc TestHeld(t *testing.T) {}\n")
		f.removeGreen(t)
		// wantHeld checks the one entry written for commit.
		wantHeld := func(commit string) {
			t.Helper()
			var got [][]string
			for _, entry := range readGreen(f.greenPath).Entries {
				if entry.Commit == commit {
					got = append(got, entry.Held)
				}
			}
			if want := [][]string{{"TestHeld"}}; !reflect.DeepEqual(got, want) {
				t.Errorf("held tests of the entries for %s = %q; want %q", commit, got, want)
			}
		}

		f.wantArgs(t, f.real(t, "--gate", "finalize"), []string{"./...", "-skip", "^(TestHeld)$"})
		wantHeld(held)

		carried := f.commitChange(t, "docs/note.md", "an inert commit over held tests\n")
		f.wantCarried(t, f.real(t, "--gate", "finalize"))
		wantHeld(carried)
	})

	t.Run("a diff run diffs from the newest whole-green ancestor, else from the parent merge base", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.commitChange(t, "other/other.go", "package other\n\nvar Seen = 1\n")
		g := f.gate(t, f.pair, nil)
		f.wantDerivation(t, g, f.mergeBase, "tier0")

		green := f.commitChange(t, "other/other.go", "package other\n\nvar Seen = 2\n")
		if whole := f.gate(t, f.pair, nil, "--all"); whole.exit != 0 {
			t.Fatalf("--all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		f.commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 8 }\n")
		g = f.gate(t, f.pair, nil)
		f.wantDerivation(t, g, green, "tier0")
		f.wantArgs(t, g, []string{"./lib", "./user"}, guardInvocation)
	})

	t.Run("a dirty worktree diffs from the parent merge base although the record holds a whole-green ancestor", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.commitChange(t, "other/other.go", "package other\n\nvar Seen = 1\n")
		if whole := f.gate(t, f.pair, nil, "--all"); whole.exit != 0 {
			t.Fatalf("--all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		f.commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 9 }\n")
		f.edit(t, "docs/note.md", "a dirtying note\n")
		f.wantDerivation(t, f.gate(t, f.pair, nil), f.mergeBase, "tier0")
	})

	t.Run("tiers on one base share one derivation line and tiers on different bases print one each", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		separate := scopesConfigWith(t, "go", func(cfg map[string]any) {
			delete(cfg["tiers"].(map[string]any)["tier1"].(map[string]any), "covers")
		})
		if err := os.WriteFile(f.configPath, separate, 0o644); err != nil {
			t.Fatal(err)
		}

		shared := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantDerivation(t, shared, f.mergeBase, "tier0, tier1")
		if count := strings.Count(shared.stdout.String(), "gate test: diff scope from"); count != 1 {
			t.Errorf("derivation lines = %d in stdout %q; want 1", count, shared.stdout.String())
		}

		f.removeGreen(t)
		green := f.commitChange(t, "other/other.go", "package other\n\nvar Seen = 3\n")
		if whole := f.gate(t, f.pair, nil, "--tier", "0", "--all"); whole.exit != 0 {
			t.Fatalf("--tier 0 --all = exit %d, stdout %q; want a pass that records tier0 whole-green", whole.exit, whole.stdout.String())
		}
		f.commitChange(t, "other/other.go", "package other\n\nvar Seen = 4\n")
		split := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantDerivation(t, split, green, "tier0")
		f.wantDerivation(t, split, f.mergeBase, "tier1")
		if count := strings.Count(split.stdout.String(), "gate test: diff scope from"); count != 2 {
			t.Errorf("derivation lines = %d in stdout %q; want 2", count, split.stdout.String())
		}
	})

	t.Run("a narrowed dotnet pass records nothing, a --all run records the tier whole, and two vstest tiers then carry", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		const project = "cs/Lib.Tests/Lib.Tests.csproj"
		f.writeDotnet(t, func(cfg map[string]any) {
			tiers := cfg["tiers"].(map[string]any)
			for _, name := range []string{"tier0", "tier1"} {
				tier := tiers[name].(map[string]any)
				tier["all_targets"] = project
				tier["counter"] = "vstest"
			}
			delete(tiers["tier1"].(map[string]any), "covers")
		})

		base := gitkit.RevParse(t, f.pair, "HEAD")
		f.commitChange(t, "cs/Lib/Circle.cs", "namespace Lib;\n\npublic class Circle { public int Edge; }\n")
		f.wantArgs(t, f.gate(t, f.pair, nil, "--base", base), circleTestsArgs)
		if entries := readGreen(f.greenPath).Entries; len(entries) != 0 {
			t.Errorf("green record = %+v; want nothing recorded for a narrowed project run", entries)
		}

		f.wantArgs(t, f.gate(t, f.pair, nil, "--all"), []string{project})
		first := f.gate(t, f.pair, nil, "--all", "--tier", "0", "--tier", "1")
		f.wantArgs(t, first, []string{project})
		f.wantCarried(t, f.gate(t, f.pair, nil, "--all", "--tier", "0", "--tier", "1"))
	})

	t.Run("an --all run carries over a committed inert-only change from a whole ancestor and writes the current tree's entry", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})

		current := f.commitChange(t, "docs/note.md", "an inert commit\n")
		f.wantCarried(t, f.real(t, "--all"))
		if !slices.ContainsFunc(readGreen(f.greenPath).Entries, func(entry greenEntry) bool { return entry.Commit == current && entry.Whole }) {
			t.Errorf("green record = %+v; want a whole entry for the current commit %s", readGreen(f.greenPath).Entries, current)
		}

		// The entry written for the current tree carries the next call directly: removing the ancestor's leaves it alone.
		record := readGreen(f.greenPath)
		record.Entries = slices.DeleteFunc(record.Entries, func(entry greenEntry) bool { return entry.Commit != current })
		f.removeGreen(t)
		for _, entry := range record.Entries {
			if err := recordGreen(f.greenPath, entry, entry.Written); err != nil {
				t.Fatal(err)
			}
		}
		f.wantCarried(t, f.real(t, "--all"))
	})

	t.Run("an --all run runs after a committed non-inert change although a whole ancestor is green", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
		f.commitChange(t, "lib/lib.go", "package lib\n\nfunc One() int { return 10 }\n")
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
	})

	t.Run("an --all run on a dirty tree runs whole although the history is inert-only", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
		f.commitChange(t, "docs/note.md", "another inert commit\n")
		f.edit(t, "docs/note.md", "a dirtying note\n")
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
	})

	t.Run("a carry: false tier runs again over inert-only history", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", func(cfg map[string]any) { cfg["tiers"].(map[string]any)["tier0"].(map[string]any)["carry"] = false })
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
		f.commitChange(t, "docs/note.md", "a third inert commit\n")
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
	})

	t.Run("a changed inert_paths list voids the entries an inert carry would use", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeInert(t, "go", nil)
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
		f.writeInert(t, "go", func(cfg map[string]any) { cfg["inert_paths"] = []string{"*.md", "*.txt"} })
		f.commitChange(t, "docs/note.md", "a fourth inert commit\n")
		f.wantArgs(t, f.real(t, "--all"), []string{"./..."})
	})

	t.Run("under the none scoper a Markdown-only diff runs the listed tiers whole, skipping a covered one", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		f.writeScoper(t, "none")
		f.edit(t, "docs/note.md", "another note\n")
		g := f.gate(t, f.pair, nil, "--tier", "0", "--tier", "1")
		f.wantArgs(t, g, []string{"./..."})
		if !strings.Contains(g.stdout.String(), "tier0 is covered by tier1") {
			t.Errorf("stdout = %q; want tier0 named as covered", g.stdout.String())
		}
	})

	t.Run("RunGate tells its observer each tier it starts and ends, a carried tier ends with no start and a blocked tier ends with its blocked record", func(t *testing.T) {
		t.Parallel()
		f := newScopesHub(t)
		recordPath := filepath.Join(t.TempDir(), "standin.json")
		standInEnv := map[string]string{helperEnv: helperRoleStandIn, helperRecordEnv: recordPath}
		// writeGate declares the finalize key over gateTiers, with tier1 exiting tier1Exit.
		// With sharedBuild, tier0 opts into a build that exits buildExit.
		// Neither tier covers the other.
		writeGate := func(t *testing.T, gateTiers []int, tier1Exit string, sharedBuild bool, buildExit string) {
			t.Helper()
			config := scopesConfigWith(t, "go", func(cfg map[string]any) {
				cfg["gates"] = map[string]any{"finalize": map[string]any{"tiers": gateTiers, "scope": "all"}}
				tiers := cfg["tiers"].(map[string]any)
				delete(tiers["tier1"].(map[string]any), "covers")
				for name, exit := range map[string]string{"tier0": "0", "tier1": tier1Exit} {
					tier := tiers[name].(map[string]any)
					env := map[string]string{helperExitEnv: exit}
					if existing, ok := tier["env"].(map[string]string); ok {
						for key, value := range existing {
							env[key] = value
						}
					}
					for key, value := range standInEnv {
						env[key] = value
					}
					tier["env"] = env
				}
				if sharedBuild {
					cfg["build"] = map[string]any{
						"command": "'" + os.Args[0] + "' sharedbuild", "timeout_min": 1,
						"env": map[string]string{helperEnv: helperRoleStandIn, helperRecordEnv: recordPath, helperBuildExitEnv: buildExit},
					}
					tiers["tier0"].(map[string]any)["shared_build"] = true
				}
			})
			if err := os.WriteFile(f.configPath, config, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		dirty := func(t *testing.T, content string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(f.pair, "docs", "note.md"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// runGate calls RunGate over the pair with finalize and returns its exit code and the observer's calls in order.
		runGate := func(t *testing.T) (int, []string, []gateslot.RunRecord) {
			t.Helper()
			var calls []string
			var ended []gateslot.RunRecord
			observer := TierObserver{
				Started: func(tier int) { calls = append(calls, fmt.Sprintf("start tier%d", tier)) },
				Ended: func(record gateslot.RunRecord) {
					calls = append(calls, fmt.Sprintf("end tier%d", record.Tier))
					ended = append(ended, record)
				},
			}
			var out, stderr bytes.Buffer
			code := RunGate(context.Background(), f.pair, "finalize", &out, &stderr, observer)
			f.requireFree(t)
			return code, calls, ended
		}

		writeGate(t, []int{0, 1}, "3", false, "")
		dirty(t, "an observed note\n")
		code, calls, ended := runGate(t)
		if want := []string{"start tier0", "end tier0", "start tier1", "end tier1"}; !slices.Equal(calls, want) {
			t.Errorf("observer calls = %q; want %q", calls, want)
		}
		if len(ended) != 2 || ended[0].ExitCode != 0 || ended[1].ExitCode != 3 || ended[1].Outcome != "failed" {
			t.Errorf("ended records = %+v; want tier0's exit 0 and tier1's failed exit 3", ended)
		}
		if cli := f.gate(t, f.pair, nil, "--wait", "--gate", "finalize"); code != 3 || cli.exit != code {
			t.Errorf("RunGate exit code = %d, CLI exit code = %d; want both 3, the first failing tier's", code, cli.exit)
		}

		writeGate(t, []int{0, 1}, "0", false, "")
		gitkit.CommitFile(t, f.pair, "docs/note.md", "a committed observed note\n", "commit the observed note")
		if code, _, _ := runGate(t); code != 0 {
			t.Fatalf("RunGate exit code over a clean tree = %d; want 0, which records both tiers green", code)
		}
		code, calls, ended = runGate(t)
		if want := []string{"end tier0", "end tier1"}; code != 0 || !slices.Equal(calls, want) {
			t.Errorf("second RunGate = exit %d, observer calls %q; want exit 0 and %q, every tier carried with no start", code, calls, want)
		}
		for _, record := range ended {
			if !record.Carried {
				t.Errorf("ended record %+v; want it carried", record)
			}
		}

		writeGate(t, []int{0}, "0", true, "1")
		dirty(t, "a note under a failing build\n")
		code, calls, ended = runGate(t)
		if want := []string{"start tier0", "end tier0"}; code != 1 || !slices.Equal(calls, want) {
			t.Errorf("RunGate over a failing build = exit %d, observer calls %q; want exit 1 and %q", code, calls, want)
		}
		if len(ended) != 1 || ended[0].Outcome != "failed" || ended[0].ExitCode != 1 || ended[0].BlockedBy == "" {
			t.Errorf("ended records = %+v; want tier0's blocked record, failed with the build's exit 1", ended)
		}
	})
}

func TestScanGuards_DropsWatchedGuardsNoReadChangeConcerns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const guardFile = "package p\n\nimport \"testing\"\n\n//lym:guard\n//lym:watch *_linux.go\nfunc TestWatchedGuard(t *testing.T) {}\n\n" +
		"//lym:guard\nfunc TestPlainGuard(t *testing.T) {}\n"
	root := t.TempDir()
	plankit.WriteTree(t, root, map[string]string{
		"go.mod":          "module example.com/root\n\ngo 1.21\n",
		"p/p_test.go":     guardFile,
		"mod/go.mod":      "module example.com/root/mod\n\ngo 1.21\n",
		"mod/p/p_test.go": guardFile,
	})

	tests := []struct {
		name        string
		guardDir    string
		changesRead bool
		changed     []string
		want        []string
	}{
		{"UnreadChangesKeepEveryGuard", ".", false, nil, []string{"TestPlainGuard", "TestWatchedGuard"}},
		{"RootModuleChangeMatchingTheWatchKeepsIt", ".", true, []string{"p/x_linux.go"}, []string{"TestPlainGuard", "TestWatchedGuard"}},
		{"RootModuleChangeOutsideTheWatchDropsIt", ".", true, []string{"docs.md"}, []string{"TestPlainGuard"}},
		{"NestedModuleChangeMatchingTheWatchKeepsIt", "mod", true, []string{"mod/p/x_linux.go"}, []string{"TestPlainGuard", "TestWatchedGuard"}},
		{"NestedModuleChangeOutsideTheWatchDropsIt", "mod", true, []string{"mod/docs.md"}, []string{"TestPlainGuard"}},
		{"ChangeOutsideTheNestedModuleDropsIt", "mod", true, []string{"p/x_linux.go"}, []string{"TestPlainGuard"}},
		{"NoChangeDropsIt", "mod", true, nil, []string{"TestPlainGuard"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := scanGuards(scope{guardDir: tt.guardDir, changesRead: tt.changesRead, changed: tt.changed}, root)
			if err != nil {
				t.Fatalf("scanGuards error = %v; want none", err)
			}
			var names []string
			for _, dirs := range got.guards {
				for _, dirNames := range dirs {
					names = append(names, dirNames...)
				}
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.want) {
				t.Errorf("scanGuards guards = %q; want %q", names, tt.want)
			}
		})
	}
}
