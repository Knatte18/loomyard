// hardenwiring_test.go covers the pure parts of a hardener run's wiring.
// Those are the live gate call's arguments, environment and result reading.
// They also include the Harden-Review budget the routing projection reports.

package loomcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedHardenerConfig writes contents as the hub-wide hardener.yaml in hub's board directory.
func seedHardenerConfig(t *testing.T, hub, contents string) {
	t.Helper()
	path := configengine.ConfigFile(fabricengine.BoardDir(hub), hardenshed.ConfigModule)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

func TestHardenLiveArgs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	packages := []string{"internal/a", "internal/b"}
	t.Run("the live call names its key and the packages, and no copies and no tier", func(t *testing.T) {
		t.Parallel()
		got := hardenLiveArgs(hardenshed.LiveGateKey, 0, packages)
		want := []string{"gate", "test", "--wait", "--gate", "harden-live", "internal/a", "internal/b"}
		if !slices.Equal(got, want) {
			t.Errorf("hardenLiveArgs(%q, 0, packages) = %v; want %v", hardenshed.LiveGateKey, got, want)
		}
	})
	t.Run("the copies call carries --copies before the packages", func(t *testing.T) {
		t.Parallel()
		got := hardenLiveArgs(hardenshed.LiveCopiesGateKey, 3, packages)
		want := []string{"gate", "test", "--wait", "--gate", "harden-live-copies", "--copies", "3", "internal/a", "internal/b"}
		if !slices.Equal(got, want) {
			t.Errorf("hardenLiveArgs(%q, 3, packages) = %v; want %v", hardenshed.LiveCopiesGateKey, got, want)
		}
	})
	t.Run("the environment drops the held slot and keeps every other variable", func(t *testing.T) {
		t.Parallel()
		got := hardenLiveEnv([]string{"A=1", gateslot.InheritEnv + "=/slots/one", "B=2"})
		if want := []string{"A=1", "B=2"}; !slices.Equal(got, want) {
			t.Errorf("hardenLiveEnv() = %v; want %v", got, want)
		}
	})

	const startedLine = "gate test: started 2 test binaries in tier2\n"
	tests := []struct {
		name        string
		exitCode    int
		stdout      string
		stderr      string
		wantPassed  bool
		wantRefused string
	}{
		{name: "exit 0 with a summary line on stderr is passed", exitCode: 0, stderr: startedLine, wantPassed: true},
		{name: "exit 1 with a failure report on stderr is failed and not refused", exitCode: 1, stderr: startedLine + "--- FAIL: TestX\n"},
		{
			name: "exit 1 whose last stdout line is a refusal and whose stderr holds no started line is refused", exitCode: 1,
			stdout:      "build output\n{\"ok\":false,\"error\":\"gates.harden-live is not set\"}\n",
			wantRefused: "gates.harden-live is not set",
		},
		{name: "an ok false envelope after a started line is a failed run", exitCode: 1, stdout: "{\"ok\":false,\"error\":\"tier failed\"}\n", stderr: startedLine},
		{name: "a JSON line without ok false and an error is a failed run", exitCode: 1, stdout: "{\"ok\":true,\"error\":\"x\"}\n"},
		{name: "a JSON line with ok false and no error is a failed run", exitCode: 1, stdout: "{\"ok\":false}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hardenLiveRun(tt.exitCode, []byte(tt.stdout), []byte(tt.stderr))
			if got.Passed != tt.wantPassed || got.Refused != tt.wantRefused {
				t.Errorf("hardenLiveRun(%d, ...) = Passed %v, Refused %q; want %v, %q", tt.exitCode, got.Passed, got.Refused, tt.wantPassed, tt.wantRefused)
			}
			if want := tt.stderr + tt.stdout; got.Output != want {
				t.Errorf("hardenLiveRun(%d, ...).Output = %q; want stderr then stdout %q", tt.exitCode, got.Output, want)
			}
		})
	}
}

func TestHardenLiveKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		gateCfg    gateslot.Config
		wantTiers  []int
		wantTested []int
	}{
		{
			name: "EachTiersCoveredTiersJoinTheTestedOnes",
			gateCfg: gateslot.Config{
				Tiers: map[string]gateslot.Tier{"tier1": {}, "tier2": {Covers: []int{1, 0}}},
				Gates: map[string]gateslot.GateKey{hardenshed.LiveGateKey: {Tiers: []int{2}}},
			},
			wantTiers:  []int{2},
			wantTested: []int{0, 1, 2},
		},
		{name: "AbsentGateConfigTestsNoTier", gateCfg: gateslot.Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")

			key, err := hardenLiveKey(fabricengine.ConfigDirs(loc), tt.gateCfg, hardenshed.LiveGateKey)

			if err != nil {
				t.Fatalf("hardenLiveKey() error = %v; want nil", err)
			}
			if key.Set || !slices.Equal(key.Tiers, tt.wantTiers) || !slices.Equal(key.Tested, tt.wantTested) {
				t.Errorf("hardenLiveKey() = %+v; want unset, tiers %v and tested %v", key, tt.wantTiers, tt.wantTested)
			}
		})
	}
}

func TestHardenSegmentSettings_RefusesAnUnresolvableModelSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		cfg  hardenshed.Config
		key  string
	}{
		{"Reviewer", hardenshed.Config{Reviewer: []string{"opus["}, HardMaxBounces: 3}, "reviewer"},
		{"Fixer", hardenshed.Config{Fixer: []string{"opus["}, HardMaxBounces: 3}, "fixer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := hardenSegmentSettings(tt.cfg, loomengine.Config{}, modelspec.Registry{})
			if err == nil {
				t.Fatal("hardenSegmentSettings() error = nil; want a refusal")
			}
			for _, want := range []string{"hardener.yaml", `key "` + tt.key + `" entry "opus["`, "way forward", hardenResumeClause} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("hardenSegmentSettings() error = %q; want it to hold %q", err, want)
				}
			}
		})
	}
}

func TestReviewBudget_HardenReview(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var template hardenshed.Config
	if err := yaml.Unmarshal([]byte(hardenshed.ConfigTemplate()), &template); err != nil {
		t.Fatalf("unmarshal the hardener template = %v; want nil", err)
	}
	tests := []struct {
		name   string
		recipe string
		config string
		want   map[string]int
	}{
		{name: "a hardener run reports hardener.yaml's max_bounces", recipe: shedrun.RecipeHardener, config: "max_bounces: 4\n", want: map[string]int{"Code-Review": 1, hardenReviewSegment: 4}},
		{name: "a hardener.yaml that does not load reports the template's max_bounces", recipe: shedrun.RecipeHardener, config: "max_bounces: [\n", want: map[string]int{"Code-Review": 1, hardenReviewSegment: template.MaxBounces}},
		{name: "a recipe with no HardenFix row reports no Harden-Review budget", recipe: shedrun.RecipeDarn, config: "max_bounces: 4\n", want: map[string]int{"Code-Review": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			seedHardenerConfig(t, loc.HubPath, tt.config)
			got, err := withHardenBudget(map[string]int{"Code-Review": 1}, tt.recipe, fabricengine.ConfigDirs(loc))
			if err != nil {
				t.Fatalf("withHardenBudget() = %v, %v; want nil error", got, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("withHardenBudget() = %v; want %v", got, tt.want)
			}
			for segment, want := range tt.want {
				if got[segment] != want {
					t.Errorf("withHardenBudget()[%q] = %d; want %d", segment, got[segment], want)
				}
			}
		})
	}
}
