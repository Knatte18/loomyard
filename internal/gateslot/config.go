// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
	"bytes"
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
	return []string{"tiers", "tier_parallel"}
}

// ConfigPerNameMaps returns the gate.yaml keys whose entries are named by the operator and merged name by name:
// a hub file that tunes one gate keeps the template's list for every other gate.
func ConfigPerNameMaps() []string {
	return []string{"gates"}
}

// ConfigSpec returns gate's layered-load spec.
// `slots` and `tier_parallel` take a machine override, and every key stays closed to the local layer.
// `tiers` runs any shell command in a task worktree, so it is executable; the gate lists, the scoper, the guard tier and the refused commands switch off or hollow a guard.
// `cli_wait_sec` and `go_parallel` are retired, so a hub or machine file that still sets one loads and reconcile prunes it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:        "gate",
		Template:    []byte(ConfigTemplate()),
		OpenMaps:    ConfigOpenMaps(),
		PerNameMaps: ConfigPerNameMaps(),
		HubWide:     true,
		Categories: map[string]configengine.Category{
			"slots":         configengine.CategoryMachine,
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
			"go_parallel":  configengine.CategoryMachine,
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
	if file.Slots < 1 {
		return Config{}, fmt.Errorf("gate config key %q: %d; want at least 1", "slots", file.Slots)
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
// Tier0Parallel is tier0's effective parallel, or 0 when the config declares no tier0.
func (c Config) Limits() Limits {
	return Limits{
		Slots:         c.Slots,
		Tier0Parallel: c.Tiers[tierKey(0)].Parallel,
		BusyQuit:      time.Duration(c.BusyQuitSec) * time.Second,
	}
}

// MigrateConfig prunes the retired gates.publish list from a gate.yaml document and returns one line, "gates.publish: retired, removed", when it did.
// Every other gate, the round list included, and every comment is carried through yaml node editing.
// A document without gates.publish is returned unchanged, so a migrated file is unchanged on a second pass.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: yaml root is not a mapping"))
	}
	gates := mappingValue(root, "gates")
	if gates == nil || gates.Kind != yaml.MappingNode {
		return existing, nil, nil
	}
	at := mappingKeyIndex(gates, "publish")
	if at < 0 {
		return existing, nil, nil
	}
	gates.Content = append(gates.Content[:at], gates.Content[at+2:]...)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: encode yaml: %w", err))
	}
	return out.Bytes(), []string{"gates.publish: retired, removed"}, nil
}

// mappingKeyIndex returns the index of key in mapping's Content, or -1 when the mapping holds no such key.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// mappingValue returns the value node of key in mapping, or nil when the mapping holds no such key.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	at := mappingKeyIndex(mapping, key)
	if at < 0 {
		return nil
	}
	return mapping.Content[at+1]
}
