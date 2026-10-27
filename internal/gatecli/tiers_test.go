//go:build tier0

// tiers_test.go covers the pure planning of a call's tier runs and the rendering of a tier's command.

package gatecli

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// planConfig returns a gate config with tier0 over the root module and tier1, which covers tier0, over the directory sub.
func planConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{
			"tier0": {Command: "go test {targets} {args}", Dir: ".", AllTargets: "./...", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1},
			"tier1": {Command: "go test -tags integration {targets} {args}", Dir: "sub", AllTargets: "./... ./x/...", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1, Covers: []int{0}},
		},
		Scoper: "go",
	}
}

// withManualTier3 makes tier1 cover nothing and declares a manual tier3, leaving tier2 undeclared.
func withManualTier3(cfg *gateslot.Config) {
	t1 := cfg.Tiers["tier1"]
	t1.Covers = nil
	cfg.Tiers["tier1"] = t1
	cfg.Tiers["tier3"] = gateslot.Tier{Command: "llm {args}", Dir: ".", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1, Manual: true}
}

// describe renders a run as `tier<N> <scope> dir=<dir> <targets>`, one `dir=… …` per invocation joined by ` | ` and `none` for no invocation, with ` terms=<terms>` and ` partial` after an invocation that carries them, ` ignores` for a command that ignores the scope and ` guard` for the guard step.
func describe(run tierRun) string {
	parts := make([]string, len(run.invocations))
	for i, invocation := range run.invocations {
		parts[i] = strings.TrimSpace(fmt.Sprintf("dir=%s %s", invocation.dir, strings.Join(invocation.targets, " ")))
		if len(invocation.terms) > 0 {
			parts[i] += " terms=" + strings.Join(invocation.terms, ",")
		}
		if invocation.partial {
			parts[i] += " partial"
		}
	}
	if len(parts) == 0 {
		parts = []string{"none"}
	}
	line := fmt.Sprintf("%s %s %s", tierName(run.number), run.scopeForm, strings.Join(parts, " | "))
	if run.ignoresTargets {
		line += " ignores"
	}
	if run.guardStep {
		line += " guard"
	}
	if run.buildOnly {
		line += " buildonly"
	}
	return line
}

// requestScope returns the scope of a request that names explicit targets or none, the two forms no computation changes.
func requestScope(request testRequest) scope {
	if len(request.targets) > 0 {
		return scope{form: scopeTargets}
	}
	return scope{form: scopeAll, whole: true}
}

func TestPlanTierRuns(t *testing.T) {
	t.Parallel()

	tier1Guards := map[int]map[string][]string{1: {"g": {"TestG"}}}
	bothTierGuards := map[int]map[string][]string{0: {"f": {"TestF"}}, 1: {"g": {"TestG"}}}
	narrower := scope{
		form:      scopeComputed,
		modules:   []moduleTargets{{dir: ".", targets: []string{"./a", "./b"}}, {dir: "nested", targets: []string{"."}}},
		guardStep: true,
		guardDir:  ".",
		guards:    tier1Guards,
	}
	narrowerWithBothTiers := narrower
	narrowerWithBothTiers.guards = bothTierGuards
	card := narrower
	card.form = scopeCard
	card.tagTiers = []int{0, 1, 3}
	cardOfTier0And1 := card
	cardOfTier0And1.tagTiers = []int{0, 1}
	dotnet := func(cfg *gateslot.Config) {
		cfg.Scoper = "dotnet"
		t1 := cfg.Tiers["tier1"]
		t1.Covers = nil
		cfg.Tiers["tier1"] = t1
	}
	noCarry := func(cfg *gateslot.Config) {
		t0 := cfg.Tiers["tier0"]
		t0.NoCarry = true
		cfg.Tiers["tier0"] = t0
	}
	projects := tierScope{modules: []moduleTargets{
		{dir: ".", targets: []string{"A/A.csproj"}, terms: []string{"T1", "T2"}},
		{dir: ".", targets: []string{"B/B.csproj"}, terms: []string{"T3"}},
	}}
	tests := []struct {
		name    string
		mutate  func(*gateslot.Config)
		request testRequest
		// scope defaults to the scope of the request's explicit targets, or --all.
		scope scope
		want  []string
		// wantStderr is text the plan must print to stderr.
		wantStderr string
	}{
		{name: "the default is tier0 over its all_targets in its dir", want: []string{"tier0 all dir=. ./..."}},
		{
			name:    "tiers run ascending and without repeats",
			mutate:  func(cfg *gateslot.Config) { t1 := cfg.Tiers["tier1"]; t1.Covers = nil; cfg.Tiers["tier1"] = t1 },
			request: testRequest{tiers: []int{1, 0, 1}},
			want:    []string{"tier0 all dir=. ./...", "tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:       "a covered tier is dropped and named on stderr",
			request:    testRequest{tiers: []int{0, 1}},
			want:       []string{"tier1 all dir=sub ./... ./x/..."},
			wantStderr: "tier0 is covered by tier1",
		},
		{
			name:       "--upto runs the declared tiers through N and skips a manual one, naming it",
			mutate:     withManualTier3,
			request:    testRequest{upto: 5, uptoGiven: true},
			want:       []string{"tier0 all dir=. ./...", "tier1 all dir=sub ./... ./x/..."},
			wantStderr: "tier3 is manual and runs only when --tier names it",
		},
		{
			name:    "--upto 0 runs tier0 alone",
			mutate:  withManualTier3,
			request: testRequest{upto: 0, uptoGiven: true},
			want:    []string{"tier0 all dir=. ./..."},
		},
		{
			name:    "--tier adds a manual tier beside --upto",
			mutate:  withManualTier3,
			request: testRequest{tiers: []int{3}, upto: 3, uptoGiven: true},
			want:    []string{"tier0 all dir=. ./...", "tier1 all dir=sub ./... ./x/...", "tier3 all dir=."},
		},
		{
			name:       "--upto drops a tier another tier in its range covers",
			request:    testRequest{upto: 1, uptoGiven: true},
			want:       []string{"tier1 all dir=sub ./... ./x/..."},
			wantStderr: "tier0 is covered by tier1",
		},
		{
			name: "a gate key's tiers are the listed tiers, ascending, a manual one included",
			mutate: func(cfg *gateslot.Config) {
				withManualTier3(cfg)
				cfg.Gates = map[string]gateslot.GateKey{"both": {Tiers: []int{3, 0}, Scope: "all"}}
			},
			request: testRequest{gate: "both"},
			want:    []string{"tier0 all dir=. ./...", "tier3 all dir=."},
		},
		{
			name: "a gate key's tier another listed tier covers is dropped and named on stderr",
			mutate: func(cfg *gateslot.Config) {
				cfg.Gates = map[string]gateslot.GateKey{"both": {Tiers: []int{0, 1}, Scope: "all"}}
			},
			request:    testRequest{gate: "both"},
			want:       []string{"tier1 all dir=sub ./... ./x/..."},
			wantStderr: "tier0 is covered by tier1",
		},
		{
			name:    "explicit targets run verbatim in the -C directory",
			request: testRequest{tiers: []int{0}, targets: []string{"./a", "./b"}, dir: "/work/nested"},
			want:    []string{"tier0 targets dir=/work/nested ./a ./b"},
		},
		{
			name:    "explicit targets run in the tier's dir without -C",
			request: testRequest{tiers: []int{1}, targets: []string{"./a"}},
			want:    []string{"tier1 targets dir=sub ./a"},
		},
		{
			name:    "--all takes all_targets",
			request: testRequest{tiers: []int{1}, all: true},
			want:    []string{"tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:    "a guard group follows the listed tiers under the dir of its own tier",
			request: testRequest{tiers: []int{0}, guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: tier1Guards},
			want:    []string{"tier0 all dir=. ./...", "tier1 guard dir=sub guard"},
		},
		{
			name:  "guard tests in tier0 and tier1 files plan two groups under their own tiers, ascending",
			scope: narrowerWithBothTiers,
			want:  []string{"tier0 computed dir=. ./a ./b | dir=nested .", "tier0 guard dir=. guard", "tier1 guard dir=. guard"},
		},
		{
			name:    "a group is skipped when the call ran its tier whole",
			request: testRequest{tiers: []int{0, 1}, guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: tier1Guards},
			want:    []string{"tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:    "a group is skipped when a tier covering its tier ran whole",
			request: testRequest{tiers: []int{1}, guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: map[int]map[string][]string{0: {"f": {"TestF"}}}},
			want:    []string{"tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:    "a group still runs after its tier ran over explicit targets",
			request: testRequest{tiers: []int{1}, guards: true, targets: []string{"./a"}},
			scope:   scope{form: scopeTargets, guards: tier1Guards},
			want:    []string{"tier1 targets dir=sub ./a", "tier1 guard dir=sub guard"},
		},
		{
			name:    "no guard group is planned when the scan found no guard test",
			request: testRequest{guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: map[int]map[string][]string{}},
			want:    []string{"tier0 all dir=. ./..."},
		},
		{
			name:  "a per-module scope runs the tier once per module, then the guard group in the scope's guard dir",
			scope: narrower,
			want:  []string{"tier0 computed dir=. ./a ./b | dir=nested .", "tier1 guard dir=. guard"},
		},
		{
			name: "a command without {targets} runs once, whole, under a narrower scope",
			mutate: func(cfg *gateslot.Config) {
				t0 := cfg.Tiers["tier0"]
				t0.Command = "make test {args}"
				cfg.Tiers["tier0"] = t0
			},
			scope: narrower,
			want:  []string{"tier0 computed dir=. ./... ignores", "tier1 guard dir=. guard"},
		},
		{
			name:  "an empty scope runs no listed tier's command, and the guard group is still planned",
			scope: scope{form: scopeComputed, guardStep: true, guardDir: ".", guards: tier1Guards},
			want:  []string{"tier0 computed none", "tier1 guard dir=. guard"},
		},
		{
			name:  "a whole fallback runs the tier whole and still plans the guard group",
			scope: scope{form: scopeComputed, whole: true, guardStep: true, guardDir: ".", guards: tier1Guards},
			want:  []string{"tier0 computed dir=. ./...", "tier1 guard dir=. guard"},
		},
		{
			name:    "a whole scope that ran the group's tier whole plans no guard group",
			request: testRequest{tiers: []int{1}},
			scope:   scope{form: scopeComputed, whole: true, guardStep: true, guardDir: ".", guards: tier1Guards},
			want:    []string{"tier1 computed dir=sub ./... ./x/..."},
		},
		{
			name:    "a narrower scope plans the guard group although the listed tier is the group's tier",
			request: testRequest{tiers: []int{1}},
			scope:   narrower,
			want:    []string{"tier1 computed dir=. ./a ./b | dir=nested .", "tier1 guard dir=. guard"},
		},
		{
			name:   "a --card scope vets each tag tier no listed tier runs, after the listed tiers and before the guard group",
			mutate: withManualTier3,
			scope:  card,
			want:   []string{"tier0 card dir=. ./a ./b | dir=nested .", "tier1 vet dir=. ./a ./b | dir=nested .", "tier3 vet dir=. ./a ./b | dir=nested .", "tier1 guard dir=. guard"},
		},
		{
			name:    "a --card scope vets no tier a listed tier covers",
			request: testRequest{tiers: []int{1}},
			scope:   cardOfTier0And1,
			want:    []string{"tier1 card dir=. ./a ./b | dir=nested .", "tier1 guard dir=. guard"},
		},
		{
			name:   "a --card scope under the none scoper plans no vet run",
			mutate: func(cfg *gateslot.Config) { cfg.Scoper = "none" },
			scope:  scope{form: scopeCard, whole: true, tagTiers: []int{1}},
			want:   []string{"tier0 card dir=. ./..."},
		},
		{
			name:   "a narrowed dotnet share runs one partial invocation per project with its terms",
			mutate: dotnet,
			scope:  scope{form: scopeComputed, tiers: map[int]tierScope{0: projects}},
			want:   []string{"tier0 computed dir=. A/A.csproj terms=T1,T2 partial | dir=. B/B.csproj terms=T3 partial"},
		},
		{
			name: "a narrowed dotnet share for a command without {targets} runs once, whole, with no terms",
			mutate: func(cfg *gateslot.Config) {
				dotnet(cfg)
				t0 := cfg.Tiers["tier0"]
				t0.Command = "dotnet test {filter}"
				cfg.Tiers["tier0"] = t0
			},
			scope: scope{form: scopeComputed, tiers: map[int]tierScope{0: projects}},
			want:  []string{"tier0 computed dir=. ./... ignores"},
		},
		{
			name:    "dotnet shares split by base: one tier narrowed, another whole with no terms",
			mutate:  dotnet,
			request: testRequest{tiers: []int{0, 1}},
			scope:   scope{form: scopeComputed, tiers: map[int]tierScope{0: {whole: true}, 1: projects}},
			want:    []string{"tier0 computed dir=. ./...", "tier1 computed dir=. A/A.csproj terms=T1,T2 partial | dir=. B/B.csproj terms=T3 partial"},
		},
		{
			name: "a dotnet scope that selected no test class gives a shared_build tier no invocation and runs another tier whole",
			mutate: func(cfg *gateslot.Config) {
				dotnet(cfg)
				t0 := cfg.Tiers["tier0"]
				t0.SharedBuild = true
				cfg.Tiers["tier0"] = t0
			},
			request:    testRequest{tiers: []int{0, 1}},
			scope:      scope{form: scopeComputed, tiers: map[int]tierScope{0: {noTests: true}, 1: {noTests: true}}},
			want:       []string{"tier0 computed none buildonly", "tier1 computed dir=sub ./... ./x/..."},
			wantStderr: "gate test: the scope selected no test class, so tier1 runs whole",
		},
		{
			name: "a no-test-class share gives a shared_build tier a build-only run and a shared_build carry: false tier its whole invocation",
			mutate: func(cfg *gateslot.Config) {
				dotnet(cfg)
				for _, key := range []string{"tier0", "tier1"} {
					tier := cfg.Tiers[key]
					tier.SharedBuild = true
					cfg.Tiers[key] = tier
				}
				t1 := cfg.Tiers["tier1"]
				t1.NoCarry = true
				cfg.Tiers["tier1"] = t1
			},
			request: testRequest{tiers: []int{0, 1}},
			scope:   scope{form: scopeComputed, tiers: map[int]tierScope{0: {noTests: true}, 1: {noTests: true}}},
			want:    []string{"tier0 computed none buildonly", "tier1 computed dir=sub ./... ./x/..."},
		},
		{
			name:   "a carry: false tier runs whole under a narrowed computed scope",
			mutate: noCarry,
			scope:  narrower,
			want:   []string{"tier0 computed dir=. ./...", "tier1 guard dir=. guard"},
		},
		{
			name:   "a carry: false tier runs whole under a --card scope",
			mutate: noCarry,
			scope:  scope{form: scopeCard, modules: narrower.modules, tagTiers: []int{0}},
			want:   []string{"tier0 card dir=. ./..."},
		},
		{
			name:   "a carry: false tier runs whole under the empty scope",
			mutate: noCarry,
			scope:  scope{form: scopeComputed},
			want:   []string{"tier0 computed dir=. ./..."},
		},
		{
			name: "a carry: false shared_build tier runs whole under a no-test-class share",
			mutate: func(cfg *gateslot.Config) {
				dotnet(cfg)
				noCarry(cfg)
				t0 := cfg.Tiers["tier0"]
				t0.SharedBuild = true
				cfg.Tiers["tier0"] = t0
			},
			scope: scope{form: scopeComputed, tiers: map[int]tierScope{0: {noTests: true}}},
			want:  []string{"tier0 computed dir=. ./..."},
		},
		{
			name: "a carry: false tier runs whole, with no terms, under a narrowed dotnet share",
			mutate: func(cfg *gateslot.Config) {
				dotnet(cfg)
				noCarry(cfg)
			},
			scope: scope{form: scopeComputed, tiers: map[int]tierScope{0: projects}},
			want:  []string{"tier0 computed dir=. ./..."},
		},
		{
			name:    "a carry: false tier narrows to explicit targets like any tier",
			mutate:  noCarry,
			request: testRequest{tiers: []int{0}, targets: []string{"./a"}},
			want:    []string{"tier0 targets dir=. ./a"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := planConfig()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			var stderr bytes.Buffer
			tc.request.stderr = &stderr
			sc := tc.scope
			if sc.form == "" {
				sc = requestScope(tc.request)
			}
			runs, err := planTierRuns(cfg, tc.request, sc)
			if err != nil {
				t.Fatalf("planTierRuns() error = %v", err)
			}
			got := make([]string, len(runs))
			for i, run := range runs {
				got[i] = describe(run)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("planTierRuns() = %q; want %q", got, tc.want)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q; want it to hold %q", stderr.String(), tc.wantStderr)
			}
		})
	}

	refusals := []struct {
		name    string
		mutate  func(*gateslot.Config)
		request testRequest
		// scope defaults to the scope of the request's explicit targets, or --all.
		scope scope
		want  string
		// exact is the whole refusal, for one that names no `lym config gate` way forward.
		exact string
	}{
		{
			name: "words after -- for a tier without {args}",
			mutate: func(cfg *gateslot.Config) {
				t0 := cfg.Tiers["tier0"]
				t0.Command = "dotnet test {targets}"
				cfg.Tiers["tier0"] = t0
			},
			request: testRequest{args: []string{"-v"}},
			want:    "command of tier0 holds no {args}",
		},
		{name: "an undeclared tier", request: testRequest{tiers: []int{5}}, want: "declares no tier5"},
		{
			name:   "a hub with no tier0",
			mutate: func(cfg *gateslot.Config) { delete(cfg.Tiers, "tier0") },
			want:   "declares no tier0",
		},
		{
			name:    "a covers entry naming an undeclared tier",
			mutate:  func(cfg *gateslot.Config) { t1 := cfg.Tiers["tier1"]; t1.Covers = []int{7}; cfg.Tiers["tier1"] = t1 },
			request: testRequest{tiers: []int{1}},
			want:    "tier1 covers tier7",
		},
		{
			name:    "a guard group whose tier is undeclared",
			request: testRequest{guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: map[int]map[string][]string{9: {"g": {"TestG"}}}},
			want:    "declares no tier9",
		},
		{
			name: "a guard group whose tier command lacks {targets}",
			mutate: func(cfg *gateslot.Config) {
				t1 := cfg.Tiers["tier1"]
				t1.Command = "go test {args}"
				cfg.Tiers["tier1"] = t1
			},
			request: testRequest{guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: map[int]map[string][]string{1: {"g": {"TestG"}}}},
			want:    "must hold {targets} and {args}",
		},
		{
			name: "a guard group whose tier command lacks {args}",
			mutate: func(cfg *gateslot.Config) {
				t1 := cfg.Tiers["tier1"]
				t1.Command = "go test {targets}"
				cfg.Tiers["tier1"] = t1
			},
			request: testRequest{guards: true},
			scope:   scope{form: scopeAll, whole: true, guards: map[int]map[string][]string{1: {"g": {"TestG"}}}},
			want:    "must hold {targets} and {args}",
		},
		{
			name:    "--guards under the none scoper",
			mutate:  func(cfg *gateslot.Config) { cfg.Scoper = "none" },
			request: testRequest{guards: true},
			want:    `scoper is "none"`,
		},
		{
			name:    "--guards under the dotnet scoper",
			mutate:  func(cfg *gateslot.Config) { cfg.Scoper = "dotnet" },
			request: testRequest{guards: true},
			exact:   `gate test: --guards needs the go scoper to find the guard tests, but gate.yaml's scoper is "dotnet"; way forward: drop --guards, then re-run the same command`,
		},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := planConfig()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			sc := tc.scope
			if sc.form == "" {
				sc = requestScope(tc.request)
			}
			_, err := planTierRuns(cfg, tc.request, sc)
			if err == nil {
				t.Fatal("planTierRuns() error = nil; want a refusal")
			}
			if tc.exact != "" {
				if err.Error() != tc.exact {
					t.Errorf("planTierRuns() error = %q; want %q", err, tc.exact)
				}
				return
			}
			for _, want := range []string{tc.want, "lym config gate", "status: FAILED"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("planTierRuns() error = %q; want it to hold %q", err, want)
				}
			}
		})
	}
}

func TestTrimpathLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		env       []string
		wantFlags string
	}{
		{name: "a -trimpath field fires", env: []string{"GOFLAGS=-trimpath -p=4"}, wantFlags: "-trimpath -p=4"},
		{name: "a double-dash field fires", env: []string{"GOFLAGS=--trimpath"}, wantFlags: "--trimpath"},
		{name: "an explicit true fires", env: []string{"GOFLAGS=-p=2 -trimpath=true"}, wantFlags: "-p=2 -trimpath=true"},
		{name: "an explicit 1 fires", env: []string{"GOFLAGS=-trimpath=1"}, wantFlags: "-trimpath=1"},
		{name: "an explicit false is silent", env: []string{"GOFLAGS=-trimpath=false"}},
		{name: "a later false overrides an earlier enable", env: []string{"GOFLAGS=-trimpath -trimpath=false"}},
		{name: "a later enable overrides an earlier false", env: []string{"GOFLAGS=-trimpath=false -trimpath"}, wantFlags: "-trimpath=false -trimpath"},
		{name: "a GOFLAGS without it is silent", env: []string{"GOFLAGS=-p=4 -mod=readonly"}},
		{name: "no GOFLAGS is silent", env: []string{"A=1"}},
		{name: "the last GOFLAGS entry wins over an inherited one", env: []string{"GOFLAGS=-trimpath", "GOFLAGS=-p=4"}},
		{name: "a tier GOFLAGS after an inherited one fires", env: []string{"GOFLAGS=-p=4", "GOFLAGS=-trimpath -p=3"}, wantFlags: "-trimpath -p=3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			want := ""
			if tt.wantFlags != "" {
				want = fmt.Sprintf("gate test: tier0 builds with GOFLAGS=%q, and -trimpath makes runtime.Caller report module-relative file paths; a test locates its repository from its working directory or go.mod, never from a runtime.Caller path alone", tt.wantFlags)
			}
			if got := trimpathLine("tier0", tt.env); got != want {
				t.Errorf("trimpathLine(tier0, %q) = %q; want %q", tt.env, got, want)
			}
		})
	}
}

func TestRenderTierCommand(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the expected quoting is the sh form")
	}
	tests := []struct {
		name    string
		command string
		filter  string
		targets []string
		terms   []string
		args    []string
		want    string
	}{
		{name: "each target and arg is quoted", command: "run {targets} -- {args}", targets: []string{"./a b", "it's"}, args: []string{"-run", "^(X|Y)$"}, want: `run './a b' 'it'\''s' -- '-run' '^(X|Y)$'`},
		{name: "parallel is filled", command: "make -j{parallel}", targets: []string{"x"}, want: "make -j3"},
		{name: "a placeholder the command lacks is not added", command: "build", targets: []string{"x"}, args: []string{"y"}, want: "build"},
		{name: "a target holding a placeholder is not expanded again", command: "run {targets} {args}", targets: []string{"{args}"}, args: []string{"z"}, want: "run '{args}' 'z'"},
		{name: "a tier filter alone", command: "t {targets} {filter}", filter: "Category!=Slow", targets: []string{"p"}, want: "t 'p' --filter 'Category!=Slow'"},
		{name: "no filter and no terms renders nothing", command: "t {targets} {filter}", targets: []string{"p"}, want: "t 'p' "},
		{name: "terms alone", command: "t {filter}", terms: []string{"FullyQualifiedName~A", "FullyQualifiedName~B"}, want: "t --filter 'FullyQualifiedName~A|FullyQualifiedName~B'"},
		{name: "terms compose with the tier filter", command: "t {filter}", filter: "Category!=Slow", terms: []string{"FullyQualifiedName~A"}, want: "t --filter '(Category!=Slow)&(FullyQualifiedName~A)'"},
		{name: "terms past the budget render the tier filter alone", command: "t {filter}", filter: "Category!=Slow", terms: []string{strings.Repeat("A", greenCommandBudget)}, want: "t --filter 'Category!=Slow'"},
		{name: "terms given to a command without a filter placeholder filter nothing", command: "t {targets}", targets: []string{"p"}, terms: []string{"FullyQualifiedName~A"}, want: "t 'p'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := renderTierCommand(2, gateslot.Tier{Command: tc.command, Filter: tc.filter, Parallel: 3}, tc.targets, tc.terms, tc.args)
			if err != nil {
				t.Fatalf("renderTierCommand() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("renderTierCommand() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestFilterArgument(t *testing.T) {
	t.Parallel()
	refusal := "gate test: the filter of tier3 cannot be quoted for the shell: " + verifyrun.ErrPercentUnderCmd.Error() +
		"; way forward: fix the tier's filter with \"lym config gate\" from the prime" + roleClause
	tests := []struct {
		name    string
		goos    string
		filter  string
		want    string
		wantErr string
	}{
		{name: "a percent sign is quoted under sh", goos: "linux", filter: "Name!=100%", want: "--filter 'Name!=100%'"},
		{name: "a plain filter is quoted under cmd", goos: "windows", filter: "Category!=Slow", want: `--filter "Category!=Slow"`},
		{name: "a percent sign is refused under cmd", goos: "windows", filter: "Name!=100%", wantErr: refusal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := filterArgument(tt.goos, 3, gateslot.Tier{Filter: tt.filter}, nil)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("filterArgument() error = %v; want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("filterArgument() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
