package reviewbudget

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

func intPtr(v int) *int { return &v }

func writeLayer(t *testing.T, dirs configengine.Dirs, spec configengine.Spec, layer configengine.Layer, content string) {
	t.Helper()
	path, ok := configengine.LayerFile(dirs, spec, layer)
	if !ok {
		t.Fatalf("layer %s does not apply", layer)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckResolved(t *testing.T) {
	t.Parallel()

	perWorktree := configengine.Spec{
		Name:       "loom",
		Template:   []byte("review_max_bounces: 1\nreview_hard_max_bounces: 3\nplan_max_bounces: ~\nplan_hard_max_bounces: 0\n"),
		Categories: Categories([]string{"plan"}),
	}
	hubWide := configengine.Spec{
		Name:       "darn",
		Template:   []byte("review_max_bounces: 1\nreview_hard_max_bounces: 3\n"),
		HubWide:    true,
		Categories: Categories(nil),
	}
	plan := []string{"plan"}

	const (
		resumeTail    = `then run "lyx loom resume"`
		localRoute    = `run "lyx config loom --layer local --set %s=<n>" in the task worktree, and correct the task's run_settings with "lyx board upsert" too`
		worktreeRoute = `run "lyx config loom --set %s=<n>" in the task worktree`
		defaultRoute  = `run "lyx config loom --layer default --set %s=<n>" from the hub's prime worktree`
		hubRoute      = `run "lyx config darn --set %s=<n>" from the prime`
	)
	route := func(format, key string) string { return strings.Replace(format, "%s", key, 1) }

	tests := []struct {
		name      string
		spec      configengine.Spec
		layers    map[configengine.Layer]string
		values    Values
		wantParts []string
		wantSoft  Caps
	}{
		{
			name:     "clean set with a null perch budget resolves to the run-wide caps",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review_max_bounces: 2\nreview_hard_max_bounces: 3\n"},
			values:   Values{RunSoft: 2, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": nil}},
			wantSoft: Caps{SoftKey: RunSoftKey, Soft: 2, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:     "own zero budget is zero rounds under the perch's own key",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review_max_bounces: 2\nreview_hard_max_bounces: 3\nplan_max_bounces: 0\n"},
			values:   Values{RunSoft: 2, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": intPtr(0)}},
			wantSoft: Caps{SoftKey: "plan_max_bounces", Soft: 0, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:     "hard cap equal to the soft cap passes",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review_max_bounces: 3\nreview_hard_max_bounces: 3\n"},
			values:   Values{RunSoft: 3, RunHard: 3, Perches: plan},
			wantSoft: Caps{SoftKey: RunSoftKey, Soft: 3, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:   "negative run-wide budget from the local layer names the local route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerLocal: "review_max_bounces: -1\n"},
			values: Values{RunSoft: -1, RunHard: 3, Perches: plan},
			wantParts: []string{
				`loom config key "review_max_bounces": must not be negative, got -1; set it to 0 for no review round, or to a positive integer`,
				route(localRoute, RunSoftKey),
				resumeTail,
			},
		},
		{
			name:   "negative perch budget from the worktree layer names the worktree route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerWorktree: "plan_max_bounces: -2\n"},
			values: Values{RunSoft: 1, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": intPtr(-2)}},
			wantParts: []string{
				`loom config key "plan_max_bounces": must not be negative, got -2; set it to 0 for no review round, to a positive integer, or to null to take review_max_bounces`,
				route(worktreeRoute, "plan_max_bounces"),
				resumeTail,
			},
		},
		{
			name:   "run-wide hard cap below one from the default layer names the prime route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerDefault: "review_hard_max_bounces: 0\n"},
			values: Values{RunSoft: 1, RunHard: 0, Perches: plan},
			wantParts: []string{
				`loom config key "review_hard_max_bounces": must be at least 1, got 0`,
				route(defaultRoute, RunHardKey),
				resumeTail,
			},
		},
		{
			name:   "run-wide hard cap of a hub-wide module names the prime command",
			spec:   hubWide,
			layers: map[configengine.Layer]string{configengine.LayerDefault: "review_hard_max_bounces: 0\n"},
			values: Values{RunSoft: 1, RunHard: 0},
			wantParts: []string{
				`darn config key "review_hard_max_bounces": must be at least 1, got 0`,
				route(hubRoute, RunHardKey),
			},
		},
		{
			name:   "negative perch hard cap names the take-the-run-wide wording",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerLocal: "plan_hard_max_bounces: -1\n"},
			values: Values{RunSoft: 1, RunHard: 3, Perches: plan, PerchHard: map[string]int{"plan": -1}},
			wantParts: []string{
				`loom config key "plan_hard_max_bounces": must not be negative, got -1; set it to 0 to take review_hard_max_bounces, or to a positive integer`,
				route(localRoute, "plan_hard_max_bounces"),
			},
		},
		{
			name: "hard cap below a local soft cap names the local and the hub route",
			spec: perWorktree,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "review_hard_max_bounces: 3\n",
				configengine.LayerLocal:   "review_max_bounces: 5\n",
			},
			values: Values{RunSoft: 5, RunHard: 3, Perches: plan},
			wantParts: []string{
				`loom config key "review_hard_max_bounces": resolved hard cap 3 is below "review_max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(defaultRoute, RunHardKey),
				"or lower the soft cap: " + route(localRoute, RunSoftKey),
				resumeTail,
			},
		},
		{
			name: "run-wide pair of a hub-wide module is checked when no perch is listed",
			spec: hubWide,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "review_hard_max_bounces: 3\n",
				configengine.LayerLocal:   "review_max_bounces: 5\n",
			},
			values: Values{RunSoft: 5, RunHard: 3},
			wantParts: []string{
				`darn config key "review_hard_max_bounces": resolved hard cap 3 is below "review_max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(hubRoute, RunHardKey),
				"or lower the soft cap: " + route("run \"lyx config darn --layer local --set %s=<n>\" in the task worktree", RunSoftKey),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Anchor: t.TempDir(), Board: t.TempDir()}
			for layer, content := range tt.layers {
				writeLayer(t, dirs, tt.spec, layer, content)
			}

			err := CheckResolved(dirs, tt.spec, tt.values)
			if len(tt.wantParts) == 0 {
				if err != nil {
					t.Fatalf("CheckResolved() = %v, want nil", err)
				}
				perch := ""
				if len(tt.values.Perches) > 0 {
					perch = tt.values.Perches[0]
				}
				got := Resolve(perch, tt.values.RunSoft, tt.values.RunHard, tt.values.PerchSoft[perch], tt.values.PerchHard[perch])
				if got != tt.wantSoft {
					t.Fatalf("Resolve() = %+v, want %+v", got, tt.wantSoft)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckResolved() = nil, want a refusal")
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("refusal %q does not contain %q", err.Error(), part)
				}
			}
		})
	}
}

func TestCheckLocal(t *testing.T) {
	t.Parallel()

	perches := []string{"plan", "merge"}
	const upsert = `correct the task's run_settings with "lyx board upsert"`
	tests := []struct {
		name    string
		set     map[string]any
		perches []string
		want    string
	}{
		{name: "run-wide zero is accepted", set: map[string]any{"review_max_bounces": 0}, perches: perches},
		{name: "null perch soft key is accepted", set: map[string]any{"plan_max_bounces": nil}, perches: perches},
		{name: "perch zero soft and hard keys are accepted", set: map[string]any{"plan_max_bounces": 0, "merge_hard_max_bounces": 0}, perches: perches},
		{name: "non-budget keys are ignored", set: map[string]any{"verify": "x", "plan_auto_accept": "y", "other_max_bounces": -5}, perches: perches},
		{name: "perch keys are ignored without perches", set: map[string]any{"plan_max_bounces": -1}},
		{name: "negative run-wide budget", set: map[string]any{"review_max_bounces": -1}, perches: perches, want: `key "review_max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "negative perch budget", set: map[string]any{"merge_max_bounces": -1}, perches: perches, want: `key "merge_max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "run-wide hard cap of zero", set: map[string]any{"review_hard_max_bounces": 0}, perches: perches, want: `key "review_hard_max_bounces": must be at least 1, got 0; ` + upsert},
		{name: "negative perch hard cap", set: map[string]any{"plan_hard_max_bounces": -1}, perches: perches, want: `key "plan_hard_max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "null run-wide budget", set: map[string]any{"review_max_bounces": nil}, perches: perches, want: `key "review_max_bounces": must be an integer`},
		{name: "null run-wide hard cap", set: map[string]any{"review_hard_max_bounces": nil}, perches: perches, want: `key "review_hard_max_bounces": must be an integer`},
		{name: "null perch hard cap", set: map[string]any{"plan_hard_max_bounces": nil}, perches: perches, want: `key "plan_hard_max_bounces": must be an integer`},
		{name: "string value", set: map[string]any{"plan_max_bounces": "2"}, perches: perches, want: `key "plan_max_bounces": must be an integer`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckLocal(tt.set, tt.perches)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("CheckLocal() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Fatalf("CheckLocal() = %v, want %q", err, tt.want)
			}
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Errorf("CheckLocal() = %v, want it to match configengine.ErrInvalid", err)
			}
		})
	}
}
