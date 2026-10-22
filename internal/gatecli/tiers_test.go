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
)

// planConfig returns a gate config with tier0 over the root module and tier1, which covers tier0, over the directory sub.
func planConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{
			"tier0": {Command: "go test {targets} {args}", Dir: ".", AllTargets: "./...", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1},
			"tier1": {Command: "go test -tags integration {targets} {args}", Dir: "sub", AllTargets: "./... ./x/...", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1, Covers: []int{0}},
		},
		Scoper:    "go",
		GuardTier: 1,
	}
}

// withManualTier3 makes tier1 cover nothing and declares a manual tier3, leaving tier2 undeclared.
func withManualTier3(cfg *gateslot.Config) {
	t1 := cfg.Tiers["tier1"]
	t1.Covers = nil
	cfg.Tiers["tier1"] = t1
	cfg.Tiers["tier3"] = gateslot.Tier{Command: "llm {args}", Dir: ".", TimeoutMin: 1, Counter: "go-json", Parallel: 1, Slots: 1, Manual: true}
}

// describe renders a run as `tier<N> <scope> dir=<dir> <targets>`, one `dir=… …` per invocation joined by ` | ` and `none` for no invocation, with ` ignores` for a command that ignores the scope and ` guard` for the guard step.
func describe(run tierRun) string {
	parts := make([]string, len(run.invocations))
	for i, invocation := range run.invocations {
		parts[i] = strings.TrimSpace(fmt.Sprintf("dir=%s %s", invocation.dir, strings.Join(invocation.targets, " ")))
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

	narrower := scope{
		form:      scopeComputed,
		modules:   []moduleTargets{{dir: ".", targets: []string{"./a", "./b"}}, {dir: "nested", targets: []string{"."}}},
		guardStep: true,
		guardDir:  ".",
		guards:    map[string][]string{},
	}
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
			name:    "the guard step follows the listed tiers",
			request: testRequest{tiers: []int{0}, guards: true},
			want:    []string{"tier0 all dir=. ./...", "tier1 guard dir=sub guard"},
		},
		{
			name:    "the guard step is skipped when the call ran guard_tier whole",
			request: testRequest{tiers: []int{1}, guards: true},
			want:    []string{"tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:    "the guard step is skipped when a tier covering guard_tier ran whole",
			mutate:  func(cfg *gateslot.Config) { cfg.GuardTier = 0 },
			request: testRequest{tiers: []int{1}, guards: true},
			want:    []string{"tier1 all dir=sub ./... ./x/..."},
		},
		{
			name:    "the guard step still runs after guard_tier ran over explicit targets",
			request: testRequest{tiers: []int{1}, guards: true, targets: []string{"./a"}},
			want:    []string{"tier1 targets dir=sub ./a", "tier1 guard dir=sub guard"},
		},
		{
			name:  "a per-module scope runs the tier once per module, then the guard step in the scope's guard dir",
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
			name:  "an empty scope runs no listed tier's command, and the guard step is still planned",
			scope: scope{form: scopeComputed, guardStep: true, guardDir: ".", guards: map[string][]string{}},
			want:  []string{"tier0 computed none", "tier1 guard dir=. guard"},
		},
		{
			name:  "a whole fallback runs the tier whole and still plans the guard step",
			scope: scope{form: scopeComputed, whole: true, guardStep: true, guardDir: "."},
			want:  []string{"tier0 computed dir=. ./...", "tier1 guard dir=. guard"},
		},
		{
			name:    "a whole scope that ran guard_tier whole plans no guard step",
			request: testRequest{tiers: []int{1}},
			scope:   scope{form: scopeComputed, whole: true, guardStep: true, guardDir: "."},
			want:    []string{"tier1 computed dir=sub ./... ./x/..."},
		},
		{
			name:    "a narrower scope plans the guard step although the listed tier is guard_tier",
			request: testRequest{tiers: []int{1}},
			scope:   narrower,
			want:    []string{"tier1 computed dir=. ./a ./b | dir=nested .", "tier1 guard dir=. guard"},
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
		want    string
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
			name:    "a guard_tier naming an undeclared tier",
			mutate:  func(cfg *gateslot.Config) { cfg.GuardTier = 9 },
			request: testRequest{guards: true},
			want:    "guard_tier",
		},
		{
			name: "a guard tier command without {targets}",
			mutate: func(cfg *gateslot.Config) {
				t1 := cfg.Tiers["tier1"]
				t1.Command = "go test {args}"
				cfg.Tiers["tier1"] = t1
			},
			request: testRequest{guards: true},
			want:    "must hold {targets} and {args}",
		},
		{
			name: "a guard tier command without {args}",
			mutate: func(cfg *gateslot.Config) {
				t1 := cfg.Tiers["tier1"]
				t1.Command = "go test {targets}"
				cfg.Tiers["tier1"] = t1
			},
			request: testRequest{guards: true},
			want:    "must hold {targets} and {args}",
		},
		{
			name:    "--guards under the none scoper",
			mutate:  func(cfg *gateslot.Config) { cfg.Scoper = "none" },
			request: testRequest{guards: true},
			want:    `scoper is "none"`,
		},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := planConfig()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			_, err := planTierRuns(cfg, tc.request, requestScope(tc.request))
			if err == nil {
				t.Fatal("planTierRuns() error = nil; want a refusal")
			}
			for _, want := range []string{tc.want, "lyx config gate", "status: FAILED"} {
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
		targets []string
		args    []string
		want    string
	}{
		{name: "each target and arg is quoted", command: "run {targets} -- {args}", targets: []string{"./a b", "it's"}, args: []string{"-run", "^(X|Y)$"}, want: `run './a b' 'it'\''s' -- '-run' '^(X|Y)$'`},
		{name: "parallel is filled", command: "make -j{parallel}", targets: []string{"x"}, want: "make -j3"},
		{name: "a placeholder the command lacks is not added", command: "build", targets: []string{"x"}, args: []string{"y"}, want: "build"},
		{name: "a target holding a placeholder is not expanded again", command: "run {targets} {args}", targets: []string{"{args}"}, args: []string{"z"}, want: "run '{args}' 'z'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := renderTierCommand(gateslot.Tier{Command: tc.command, Parallel: 3}, tc.targets, tc.args)
			if err != nil {
				t.Fatalf("renderTierCommand() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("renderTierCommand() = %q; want %q", got, tc.want)
			}
		})
	}
}
