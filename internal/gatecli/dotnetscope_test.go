// dotnetscope_test.go covers naming the selected test projects that no run of a dotnet call reaches.

package gatecli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestUnrunProjectLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	lineFor := func(project, wayForward string) string {
		return "gate test: the scope selected tests in " + project + ", and no tier this call runs reaches that project, so this pass is not green for it; way forward: " + wayForward
	}
	const configStep = `no declared tier's command reaches it, so the prime adds {targets} to a tier's command with "lym config gate", and the same command runs again`
	fastOnly := gateslot.Tier{Command: "dotnet test Fast/Fast.csproj {filter}"}
	wholeRun := func(tier gateslot.Tier) tierRun {
		return tierRun{number: 0, tier: tier, invocations: []invocation{{dir: "."}}}
	}
	selected := scope{form: scopeCard, tiers: map[int]tierScope{0: {modules: []moduleTargets{
		{dir: ".", targets: []string{"A/A.csproj"}, terms: []string{"T1"}},
		{dir: ".", targets: []string{"Fast/Fast.csproj"}, terms: []string{"T2"}},
	}}}}
	// selectedIn returns a scope that selects only the project at projectPath, in each of tiers.
	selectedIn := func(projectPath string, tiers ...int) scope {
		shares := map[int]tierScope{}
		for _, number := range tiers {
			shares[number] = tierScope{modules: []moduleTargets{{dir: ".", targets: []string{projectPath}}}}
		}
		return scope{form: scopeCard, tiers: shares}
	}
	tests := []struct {
		name  string
		tier1 gateslot.Tier
		// tier0 is the command of the listed run, which runs whole; it defaults to fastOnly.
		tier0 gateslot.Tier
		// tier1Run lists tier1 in the call with its own invocations when set.
		tier1Run []invocation
		// sc is the call's scope; it defaults to selected.
		sc   scope
		want []string
	}{
		{
			name:  "a project the whole run's command does not name is reported with the lowest tier that takes targets",
			tier1: gateslot.Tier{Command: "dotnet test {targets} {filter}"},
			want:  []string{lineFor("A/A.csproj", `run "lym gate test --tier 1 A/A.csproj"`)},
		},
		{
			name:  "a tier whose command names the project in quotes and backslashes is the way forward without targets",
			tier1: gateslot.Tier{Command: `dotnet test "A\A.csproj"`},
			want:  []string{lineFor("A/A.csproj", `run "lym gate test --tier 1"`)},
		},
		{
			name:  "with only a manual tier reaching it the line names the config step",
			tier1: gateslot.Tier{Command: "llm {targets} {args}", Manual: true},
			want:  []string{lineFor("A/A.csproj", configStep)},
		},
		{
			name:     "a project another run of the call names among its targets is not reported",
			tier1:    gateslot.Tier{Command: "dotnet test {targets} {filter}"},
			tier1Run: []invocation{{dir: ".", targets: []string{"A/A.csproj"}}},
		},
		{
			name:  "a command naming a solution file reaches every project",
			tier0: gateslot.Tier{Command: "dotnet test All.sln {filter}"},
			tier1: gateslot.Tier{Command: "dotnet test Fast/Fast.csproj"},
		},
		{
			name:  "a command naming no project file reaches every project",
			tier0: gateslot.Tier{Command: "dotnet test {filter}"},
			tier1: gateslot.Tier{Command: "dotnet test Fast/Fast.csproj"},
		},
		{
			name:  "a command naming only a built assembly reaches no project",
			tier0: gateslot.Tier{Command: "dotnet test Fast/bin/Debug/Fast.dll"},
			tier1: gateslot.Tier{Command: "dotnet test Fast/Fast.csproj"},
			sc:    selectedIn("A/A.csproj", 0),
			want:  []string{lineFor("A/A.csproj", configStep)},
		},
		{
			name:  "a command naming the built assembly of a project reaches that project",
			tier0: gateslot.Tier{Command: "dotnet test A/bin/Debug/A.dll"},
			tier1: gateslot.Tier{Command: "dotnet test Fast/Fast.csproj"},
			sc:    selectedIn("A/A.csproj", 0),
		},
		{
			name:     "a project of the same file name in another directory is reported",
			tier1:    gateslot.Tier{Command: "dotnet test B/Tests.csproj"},
			tier1Run: []invocation{{dir: "."}},
			sc:       selectedIn("A/Tests.csproj", 0),
			want:     []string{lineFor("A/Tests.csproj", configStep)},
		},
		{
			name:     "a command naming the project with a leading dot slash reaches it",
			tier1:    gateslot.Tier{Command: "dotnet test ./A/Tests.csproj"},
			tier1Run: []invocation{{dir: "."}},
			sc:       selectedIn("A/Tests.csproj", 0),
		},
		{
			name:     "a run with no invocation reaches nothing",
			tier1:    gateslot.Tier{Command: "dotnet test {targets} {filter}"},
			tier1Run: []invocation{},
			want:     []string{lineFor("A/A.csproj", `run "lym gate test --tier 1 A/A.csproj"`)},
		},
		{
			name:     "a project selected by two tier shares is reported once",
			tier1:    gateslot.Tier{Command: "dotnet test Fast/Fast.csproj"},
			tier1Run: []invocation{{dir: "."}},
			sc:       selectedIn("A/A.csproj", 0, 1),
			want:     []string{lineFor("A/A.csproj", configStep)},
		},
		{
			name:     "a project path written with backslashes is reached by a command naming it with slashes",
			tier1:    gateslot.Tier{Command: "dotnet test A/A.csproj"},
			tier1Run: []invocation{{dir: "."}},
			sc:       selectedIn(`A\A.csproj`, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tier0 := tt.tier0
			if tier0.Command == "" {
				tier0 = fastOnly
			}
			sc := tt.sc
			if sc.tiers == nil {
				sc = selected
			}
			cfg := gateslot.Config{Scoper: "dotnet", Tiers: map[string]gateslot.Tier{"tier0": tier0, "tier1": tt.tier1}}
			runs := []tierRun{wholeRun(tier0)}
			if tt.tier1Run != nil {
				runs = append(runs, tierRun{number: 1, tier: tt.tier1, invocations: tt.tier1Run})
			}
			if got := unrunProjectLines(cfg, runs, sc); !slices.Equal(got, tt.want) {
				t.Errorf("unrunProjectLines() = %q; want %q", got, tt.want)
			}
		})
	}
}
