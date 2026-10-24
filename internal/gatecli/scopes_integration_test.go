//go:build tier1

// scopes_integration_test.go drives `lyx gate test` over a hubforge hub with a task pair, whose fabric origin record names the prime's branch as its parent, to reach the computed, --base and --card scope forms.
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

	t.Run("--gate with an unknown key is refused with the config's way forward naming lyx config gate", func(t *testing.T) {
		wantRefusal(t, gate(t, pair, nil, "--gate", "nosuch"), `"nosuch"`, "lyx config gate")
	})

	t.Run("a misplaced guard marker refuses a computed call before any slot with its file and line", func(t *testing.T) {
		edit(t, "other/o_test.go", "package other\n\n//lyx:guard\nfunc helper() {}\n")
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
	// real runs `lyx gate test args...` from the pair with the real go command, which the green record lists packages with.
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
