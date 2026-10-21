// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// roleClause closes every gate config refusal's way forward: the steps need the prime, so any other session reports and the orch takes them.
const roleClause = "; a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them"

// Config is the resolved gate config of the hub file and this machine's override.
// The keys named in the tags are the gate.yaml keys; decodeConfig reads them, so a Config is never unmarshalled directly.
type Config struct {
	// Slots is how many gate runs the hub executes at once.
	Slots int `yaml:"slots"`
	// GoParallel is the `-p` cap every slotted run gets.
	GoParallel int `yaml:"go_parallel"`
	// BusyQuitSec is the continuous seconds a Go test binary may keep a core busy before the watchdog quits it; 0 turns the watchdog off.
	BusyQuitSec int `yaml:"busy_quit_sec"`
	// Tiers are the declared test tiers keyed tier0, tier1, and so on.
	Tiers map[string]Tier `yaml:"tiers"`
	// Gates maps a gate name to the tier numbers that site runs.
	Gates map[string][]int `yaml:"gates"`
	// Scoper is how a computed, base or card scope maps to targets: go or none.
	Scoper string `yaml:"scoper"`
	// GuardTier is the tier whose command runs the guard tests outside a narrowed scope.
	GuardTier int `yaml:"guard_tier"`
	// RefuseRaw lists the command prefixes the shell guard refuses.
	RefuseRaw []string `yaml:"refuse_raw"`
	// TierParallel holds this machine's parallel for each declared tier it overrides.
	TierParallel map[string]int `yaml:"tier_parallel"`
}

// configFile is gate.yaml as it is spelled, before defaults and validation.
// A tier_parallel entry is a node because a scalar --set writes it as a string.
type configFile struct {
	Slots        int                  `yaml:"slots"`
	GoParallel   int                  `yaml:"go_parallel"`
	BusyQuitSec  int                  `yaml:"busy_quit_sec"`
	Tiers        map[string]tierFile  `yaml:"tiers"`
	Gates        map[string][]int     `yaml:"gates"`
	Scoper       string               `yaml:"scoper"`
	GuardTier    int                  `yaml:"guard_tier"`
	RefuseRaw    []string             `yaml:"refuse_raw"`
	TierParallel map[string]yaml.Node `yaml:"tier_parallel"`
}

// ErrConfigAbsent is LoadConfig's error for a baseDir with no gate.yaml.
// Its way forward is `lyx fabric reconcile` in a hub, which writes the file from the template, and `lyx config gate` outside every hub, which writes the worktree's own file.
var ErrConfigAbsent = errors.New("gate config absent")

// ConfigOpenMaps returns the gate.yaml keys whose entries belong to the operator, which configengine carries whole through reconcile and the layer stack.
func ConfigOpenMaps() []string {
	return []string{"tiers", "gates", "tier_parallel"}
}

// ConfigSpec returns gate's layered-load spec.
// `slots`, `go_parallel` and `tier_parallel` take a machine override, and every key stays closed to the local layer.
// `tiers` runs any shell command in a task worktree, so it is executable; the gate lists, the scoper, the guard tier and the refused commands switch off or hollow a guard.
// `cli_wait_sec` is retired, so a hub file that still sets it loads and reconcile prunes it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "gate",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"slots":         configengine.CategoryMachine,
			"go_parallel":   configengine.CategoryMachine,
			"tier_parallel": configengine.CategoryMachine,
			"busy_quit_sec": configengine.CategoryCrossPair,
			"tiers":         configengine.CategoryExecutable,
			"gates":         configengine.CategoryGuard,
			"scoper":        configengine.CategoryGuard,
			"guard_tier":    configengine.CategoryGuard,
			"refuse_raw":    configengine.CategoryGuard,
		},
		RetiredKeys: map[string]configengine.Category{
			"cli_wait_sec": configengine.CategoryCrossPair,
		},
	}
}

// ConfigFiles returns the gate config files a load under dirs stacks and that exist:
// the hub file, followed by the machine file when that layer applies and the file is present.
// A machine file that cannot be statted for another reason is left out, since the load then fails reading it with an error naming its path.
func ConfigFiles(dirs configengine.Dirs) []string {
	spec := ConfigSpec()
	hubFile, _ := configengine.LayerFile(dirs, spec, configengine.BaseLayer(dirs, spec))
	files := []string{hubFile}
	if machineFile, applies := configengine.LayerFile(dirs, spec, configengine.LayerMachine); applies {
		if _, err := os.Stat(machineFile); err == nil {
			files = append(files, machineFile)
		}
	}
	return files
}

// LoadConfig loads the gate config strictly through the hub-wide layer stack under dirs, this machine's override included:
// an absent hub file is ErrConfigAbsent, and a value below 1 fails naming its key.
// Outside every hub, where dirs tell no board, the file is the worktree's own and its absence names `lyx config gate` as the way to write it.
// Every other failure names each file that can hold the value and its way forward, so each gate site that wraps it carries the clause.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	spec := ConfigSpec()
	files := ConfigFiles(dirs)
	if _, err := os.Stat(files[0]); errors.Is(err, os.ErrNotExist) {
		if dirs.Board == "" {
			return Config{}, fmt.Errorf("%w; run \"lyx config gate\" to write this worktree's own gate.yaml, creating the worktree's _lyx directory first when it is absent, since the write refuses without it; a session lyx refuses the verb from reports status: FAILED and the orch runs it", ErrConfigAbsent)
		}
		return Config{}, fmt.Errorf("%w; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it", ErrConfigAbsent)
	}
	local := ""
	if localFile, applies := configengine.LayerFile(dirs, spec, configengine.LayerLocal); applies {
		if _, err := configengine.FindBaseDir(dirs.Anchor); err == nil {
			if holdsUnits(localFile, spec) {
				local = localFile
			}
		}
	}
	resolved, err := configengine.Load(dirs, spec)
	if err != nil {
		return Config{}, withConfigWayForward(err, files, local)
	}
	_, machineLayerApplies := configengine.LayerFile(dirs, spec, configengine.LayerMachine)
	cfg, err := decodeConfig(resolved, machineLayerApplies)
	if err != nil {
		return Config{}, withConfigWayForward(err, files, local)
	}
	return cfg, nil
}

// holdsUnits reports whether the layer file at path is readable and holds at least one config unit.
// An empty or comment-only file is one the load drops, so removing it fixes nothing; an unparsable file counts as holding units, since the load fails naming it.
func holdsUnits(path string, spec configengine.Spec) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	units, err := yamlengine.LeafPaths(data, spec.OpenMaps, spec.WholeEntries)
	return err != nil || len(units) > 0
}

// withConfigWayForward appends to err the way forward for a gate config that is present but unusable.
// files are the hub file and, when present, the machine file; local is the local override file, or "" when none takes part.
// The clause for a session that is not the prime closes every form.
func withConfigWayForward(err error, files []string, local string) error {
	removeLocal := ""
	if local != "" {
		removeLocal = fmt.Sprintf("; remove %s, which takes no gate key", local)
	}
	if len(files) < 2 {
		return fmt.Errorf("%w; fix it with \"lyx config gate\" from the prime%s%s", err, removeLocal, roleClause)
	}
	hub, machine := files[0], files[1]
	return fmt.Errorf("%w; the hub file %s, the machine file %s or the file the error names holds the value: "+
		"fix the hub file with \"lyx config gate\" from the prime, "+
		"and the machine file with \"lyx config gate --layer machine\" from the prime, "+
		"or, when that refuses a key the machine file holds, by removing %s; "+
		"\"lyx config gate --resolved\" names the layer holding each value%s%s",
		err, hub, machine, machine, removeLocal, roleClause)
}

// TemplateConfig decodes the embedded template alone, the config of a run outside every hub.
func TemplateConfig() (Config, error) {
	return decodeConfig([]byte(ConfigTemplate()), false)
}

// decodeConfig decodes and validates a stacked gate document.
// machineLayerApplies tells whether the load stacked a machine layer, which decides the steps the dropped-tier_parallel warning names.
// Cross-references between gate lists and tiers are never checked here.
func decodeConfig(data []byte, machineLayerApplies bool) (Config, error) {
	var file configFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Config{}, fmt.Errorf("unmarshal gate config: %w", err)
	}
	for _, key := range []struct {
		name  string
		value int
	}{{"slots", file.Slots}, {"go_parallel", file.GoParallel}} {
		if key.value < 1 {
			return Config{}, fmt.Errorf("gate config key %q: %d; want at least 1", key.name, key.value)
		}
	}
	if file.BusyQuitSec < 0 {
		return Config{}, fmt.Errorf("gate config key %q: %d; want at least 0", "busy_quit_sec", file.BusyQuitSec)
	}
	tiers, err := decodeTiers(file.Tiers)
	if err != nil {
		return Config{}, err
	}
	if file.Scoper != "go" && file.Scoper != "none" {
		return Config{}, fmt.Errorf("gate config key %q: %q; want go or none", "scoper", file.Scoper)
	}
	if err := decodeRefuseRaw(file.RefuseRaw); err != nil {
		return Config{}, err
	}
	tierParallel, err := applyTierParallel(file.TierParallel, tiers, machineLayerApplies)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Slots:        file.Slots,
		GoParallel:   file.GoParallel,
		BusyQuitSec:  file.BusyQuitSec,
		Tiers:        tiers,
		Gates:        file.Gates,
		Scoper:       file.Scoper,
		GuardTier:    file.GuardTier,
		RefuseRaw:    file.RefuseRaw,
		TierParallel: tierParallel,
	}, nil
}

// Limits converts the config to the pool limits, busy_quit_sec as a duration.
func (c Config) Limits() Limits {
	return Limits{
		Slots:      c.Slots,
		GoParallel: c.GoParallel,
		BusyQuit:   time.Duration(c.BusyQuitSec) * time.Second,
	}
}
