package reviewbudget

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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
	tiergate.Require(t, 0)
	t.Parallel()

	perWorktree := configengine.Spec{
		Name:       "loom",
		Template:   []byte("review:\n  max_bounces: 1\n  hard_max_bounces: 3\n  amend_max_bounces: 1\nplan:\n  max_bounces: ~\n  hard_max_bounces: 0\n"),
		Categories: Categories([]string{"plan"}),
	}
	hubWide := configengine.Spec{
		Name:       "darn",
		Template:   []byte("review:\n  max_bounces: 1\n  hard_max_bounces: 3\n"),
		HubWide:    true,
		Categories: Categories(nil),
	}
	tapestry := configengine.Spec{
		Name:       "tapestry",
		Template:   []byte("plan:\n  max_bounces: ~\n  hard_max_bounces: 0\n"),
		Categories: Categories([]string{"plan"}),
	}
	plan := []string{"plan"}

	const (
		resumeTail           = `then run "lym loom resume"`
		localRoute           = `run "lym config loom --layer local --set %s=<n>" in the task worktree, and correct the task's run_settings with "lym board upsert" too`
		worktreeRoute        = `run "lym config loom --set %s=<n>" in the task worktree`
		defaultRoute         = `run "lym config loom --layer default --set %s=<n>" from the hub's prime worktree`
		tapestryDefaultRoute = `run "lym config tapestry --layer default --set %s=<n>" from the hub's prime worktree`
		hubRoute             = `run "lym config darn --set %s=<n>" from the prime`
	)
	route := func(format, key string) string { return strings.Replace(format, "%s", key, 1) }

	tests := []struct {
		name   string
		spec   configengine.Spec
		layers map[configengine.Layer]string
		// runWideLayers are the layers of values.RunWideSpec, the module holding the run-wide keys.
		runWideLayers map[configengine.Layer]string
		values        Values
		wantParts     []string
		wantSoft      Caps
	}{
		{
			name:     "clean set with a null perch budget resolves to the run-wide caps",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review: {max_bounces: 2, hard_max_bounces: 3}\n"},
			values:   Values{RunSoft: 2, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": nil}},
			wantSoft: Caps{SoftKey: RunSoftKey, Soft: 2, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:     "own zero budget is zero rounds under the perch's own key",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review: {max_bounces: 2, hard_max_bounces: 3}\nplan: {max_bounces: 0}\n"},
			values:   Values{RunSoft: 2, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": intPtr(0)}},
			wantSoft: Caps{SoftKey: "plan.max_bounces", Soft: 0, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:     "hard cap equal to the soft cap passes",
			spec:     perWorktree,
			layers:   map[configengine.Layer]string{configengine.LayerDefault: "review: {max_bounces: 3, hard_max_bounces: 3}\n"},
			values:   Values{RunSoft: 3, RunHard: 3, Perches: plan},
			wantSoft: Caps{SoftKey: RunSoftKey, Soft: 3, HardKey: RunHardKey, Hard: 3},
		},
		{
			name:   "negative run-wide budget from the local layer names the local route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerLocal: "review: {max_bounces: -1}\n"},
			values: Values{RunSoft: -1, RunHard: 3, Perches: plan},
			wantParts: []string{
				`config key "loom.review.max_bounces": must not be negative, got -1; set it to 0 for no review round, or to a positive integer`,
				route(localRoute, RunSoftKey),
				resumeTail,
			},
		},
		{
			name:   "negative perch budget from the worktree layer names the worktree route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerWorktree: "plan: {max_bounces: -2}\n"},
			values: Values{RunSoft: 1, RunHard: 3, Perches: plan, PerchSoft: map[string]*int{"plan": intPtr(-2)}},
			wantParts: []string{
				`config key "loom.plan.max_bounces": must not be negative, got -2; set it to 0 for no review round, to a positive integer, or to null to take loom.review.max_bounces`,
				route(worktreeRoute, "plan.max_bounces"),
				resumeTail,
			},
		},
		{
			name:   "negative amendment allowance from the local layer names the local route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerLocal: "review: {amend_max_bounces: -1}\n"},
			values: Values{RunSoft: 1, RunHard: 3, RunAmend: -1, Perches: plan},
			wantParts: []string{
				`config key "loom.review.amend_max_bounces": must not be negative, got -1; set it to 0 to settle every amendment with no review round, or to a positive integer`,
				route(localRoute, RunAmendKey),
				resumeTail,
			},
		},
		{
			name:   "run-wide hard cap below one from the default layer names the prime route",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerDefault: "review: {hard_max_bounces: 0}\n"},
			values: Values{RunSoft: 1, RunHard: 0, Perches: plan},
			wantParts: []string{
				`config key "loom.review.hard_max_bounces": must be at least 1, got 0`,
				route(defaultRoute, RunHardKey),
				resumeTail,
			},
		},
		{
			name:   "run-wide hard cap of a hub-wide module names the prime command",
			spec:   hubWide,
			layers: map[configengine.Layer]string{configengine.LayerDefault: "review: {hard_max_bounces: 0}\n"},
			values: Values{RunSoft: 1, RunHard: 0},
			wantParts: []string{
				`config key "darn.review.hard_max_bounces": must be at least 1, got 0`,
				route(hubRoute, RunHardKey),
			},
		},
		{
			name:   "negative perch hard cap names the take-the-run-wide wording",
			spec:   perWorktree,
			layers: map[configengine.Layer]string{configengine.LayerLocal: "plan: {hard_max_bounces: -1}\n"},
			values: Values{RunSoft: 1, RunHard: 3, Perches: plan, PerchHard: map[string]int{"plan": -1}},
			wantParts: []string{
				`config key "loom.plan.hard_max_bounces": must not be negative, got -1; set it to 0 to take loom.review.hard_max_bounces, or to a positive integer`,
				route(localRoute, "plan.hard_max_bounces"),
			},
		},
		{
			name: "hard cap below a local soft cap names the local and the hub route",
			spec: perWorktree,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "review: {hard_max_bounces: 3}\n",
				configengine.LayerLocal:   "review: {max_bounces: 5}\n",
			},
			values: Values{RunSoft: 5, RunHard: 3, Perches: plan},
			wantParts: []string{
				`config key "loom.review.hard_max_bounces": resolved hard cap 3 is below "loom.review.max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(defaultRoute, RunHardKey),
				"or lower the soft cap: " + route(localRoute, RunSoftKey),
				resumeTail,
			},
		},
		{
			name: "run-wide pair of a hub-wide module is checked when no perch is listed",
			spec: hubWide,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "review: {hard_max_bounces: 3}\n",
				configengine.LayerLocal:   "review: {max_bounces: 5}\n",
			},
			values: Values{RunSoft: 5, RunHard: 3},
			wantParts: []string{
				`config key "darn.review.hard_max_bounces": resolved hard cap 3 is below "darn.review.max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(hubRoute, RunHardKey),
				"or lower the soft cap: " + route("run \"lym config darn --layer local --set %s=<n>\" in the task worktree", RunSoftKey),
			},
		},
		{
			name: "plan perch hard cap below an inherited run-wide soft cap names tapestry for the hard cap and loom for the soft cap",
			spec: tapestry,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "plan: {hard_max_bounces: 2}\n",
			},
			runWideLayers: map[configengine.Layer]string{
				configengine.LayerLocal: "review: {max_bounces: 5}\n",
			},
			values: Values{RunSoft: 5, RunHard: 4, Perches: plan, PerchHard: map[string]int{"plan": 2}, RunWideSpec: perWorktree},
			wantParts: []string{
				`config key "tapestry.plan.hard_max_bounces": resolved hard cap 2 is below "loom.review.max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(tapestryDefaultRoute, "plan.hard_max_bounces"),
				"or lower the soft cap: " + route(localRoute, RunSoftKey),
				resumeTail,
			},
		},
		{
			name: "plan perch budget above the run-wide hard cap names loom for the hard cap and tapestry for the soft cap",
			spec: tapestry,
			layers: map[configengine.Layer]string{
				configengine.LayerDefault: "plan: {max_bounces: 5}\n",
			},
			runWideLayers: map[configengine.Layer]string{
				configengine.LayerDefault: "review: {hard_max_bounces: 4}\n",
			},
			values: Values{RunSoft: 3, RunHard: 4, Perches: plan, PerchSoft: map[string]*int{"plan": intPtr(5)}, RunWideSpec: perWorktree},
			wantParts: []string{
				`config key "loom.review.hard_max_bounces": resolved hard cap 4 is below "tapestry.plan.max_bounces"'s resolved soft cap 5`,
				"raise the hard cap: " + route(defaultRoute, RunHardKey),
				"or lower the soft cap: " + route(tapestryDefaultRoute, "plan.max_bounces"),
				resumeTail,
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
			for layer, content := range tt.runWideLayers {
				writeLayer(t, dirs, tt.values.RunWideSpec, layer, content)
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
	tiergate.Require(t, 0)
	t.Parallel()

	perches := []string{"plan", "merge"}
	const upsert = `correct the task's run_settings with "lym board upsert"`
	at := func(area, key string, value any) map[string]any {
		return map[string]any{area: map[string]any{key: value}}
	}
	tests := []struct {
		name    string
		set     map[string]any
		perches []string
		want    string
	}{
		{name: "run-wide zero is accepted", set: at("review", "max_bounces", 0), perches: perches},
		{name: "null perch soft key is accepted", set: at("plan", "max_bounces", nil), perches: perches},
		{name: "perch zero soft and hard keys are accepted", set: map[string]any{"plan": map[string]any{"max_bounces": 0}, "merge": map[string]any{"hard_max_bounces": 0}}, perches: perches},
		{name: "non-budget keys are ignored", set: map[string]any{"verify": "x", "plan": map[string]any{"auto_accept": "y"}, "other": map[string]any{"max_bounces": -5}}, perches: perches},
		{name: "perch keys are ignored without perches", set: at("plan", "max_bounces", -1)},
		{name: "negative run-wide budget", set: at("review", "max_bounces", -1), perches: perches, want: `key "loom.review.max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "negative perch budget", set: at("merge", "max_bounces", -1), perches: perches, want: `key "loom.merge.max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "run-wide hard cap of zero", set: at("review", "hard_max_bounces", 0), perches: perches, want: `key "loom.review.hard_max_bounces": must be at least 1, got 0; ` + upsert},
		{name: "negative perch hard cap", set: at("plan", "hard_max_bounces", -1), perches: perches, want: `key "loom.plan.hard_max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "null run-wide budget", set: at("review", "max_bounces", nil), perches: perches, want: `key "loom.review.max_bounces": must be an integer`},
		{name: "null run-wide hard cap", set: at("review", "hard_max_bounces", nil), perches: perches, want: `key "loom.review.hard_max_bounces": must be an integer`},
		{name: "null perch hard cap", set: at("plan", "hard_max_bounces", nil), perches: perches, want: `key "loom.plan.hard_max_bounces": must be an integer`},
		{name: "string value", set: at("plan", "max_bounces", "2"), perches: perches, want: `key "loom.plan.max_bounces": must be an integer`},
		{name: "amendment allowance of zero is accepted", set: at("review", "amend_max_bounces", 0), perches: perches},
		{name: "negative amendment allowance", set: at("review", "amend_max_bounces", -1), perches: perches, want: `key "loom.review.amend_max_bounces": must be at least 0, got -1; ` + upsert},
		{name: "null amendment allowance", set: at("review", "amend_max_bounces", nil), perches: perches, want: `key "loom.review.amend_max_bounces": must be an integer`},
		{name: "string amendment allowance", set: at("review", "amend_max_bounces", "1"), perches: perches, want: `key "loom.review.amend_max_bounces": must be an integer`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckLocal("loom", tt.set, tt.perches)
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
