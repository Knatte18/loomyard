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

// describe renders a run as `tier<N> <scope> dir=<dir> <targets>`, with ` guard` for the guard step.
func describe(run tierRun) string {
	line := strings.TrimSpace(fmt.Sprintf("%s %s dir=%s %s", tierName(run.number), run.scopeForm, run.invocations[0].dir, strings.Join(run.invocations[0].targets, " ")))
	if run.guardStep {
		line += " guard"
	}
	return line
}

func TestPlanTierRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*gateslot.Config)
		request testRequest
		want    []string
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
			runs, err := planTierRuns(cfg, tc.request)
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
			_, err := planTierRuns(cfg, tc.request)
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
