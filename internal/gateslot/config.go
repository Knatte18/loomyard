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

// Config is the resolved gate config of the hub file and this machine's override.
type Config struct {
	// Slots is how many gate runs the hub executes at once.
	Slots int `yaml:"slots"`
	// GoParallel is the `-p` cap every slotted run gets.
	GoParallel int `yaml:"go_parallel"`
	// BusyQuitSec is the continuous seconds a Go test binary may keep a core busy before the watchdog quits it; 0 turns the watchdog off.
	BusyQuitSec int `yaml:"busy_quit_sec"`
}

// ErrConfigAbsent is LoadConfig's error for a baseDir with no gate.yaml; its way forward is `lyx fabric reconcile`, which writes the file from the template.
var ErrConfigAbsent = errors.New("gate config absent; run \"lyx fabric reconcile\"; a session lyx refuses the verb from reports status: FAILED and the orch runs it")

// ConfigSpec returns gate's layered-load spec.
// `slots` and `go_parallel` take a machine override, and every key stays closed to the local layer.
// `cli_wait_sec` is retired, so a hub file that still sets it loads and reconcile prunes it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "gate",
		Template: []byte(ConfigTemplate()),
		HubWide:  true,
		Categories: map[string]configengine.Category{
			"slots":         configengine.CategoryMachine,
			"go_parallel":   configengine.CategoryMachine,
			"busy_quit_sec": configengine.CategoryCrossPair,
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
// Every other failure names each file that can hold the value and its way forward, so each gate site that wraps it carries the clause.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	spec := ConfigSpec()
	files := ConfigFiles(dirs)
	if _, err := os.Stat(files[0]); errors.Is(err, os.ErrNotExist) {
		return Config{}, ErrConfigAbsent
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
	cfg, err := decodeConfig(resolved)
	if err != nil {
		return Config{}, withConfigWayForward(err, files, local)
	}
	return cfg, nil
}

// holdsUnits reports whether the layer file at path is readable and holds at least one config unit.
// An empty or comment-only file is one the load drops, so removing it fixes nothing.
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
	const roleClause = "; a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them"
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
	return decodeConfig([]byte(ConfigTemplate()))
}

func decodeConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal gate config: %w", err)
	}
	for _, key := range []struct {
		name  string
		value int
	}{{"slots", cfg.Slots}, {"go_parallel", cfg.GoParallel}} {
		if key.value < 1 {
			return Config{}, fmt.Errorf("gate config key %q: %d; want at least 1", key.name, key.value)
		}
	}
	if cfg.BusyQuitSec < 0 {
		return Config{}, fmt.Errorf("gate config key %q: %d; want at least 0", "busy_quit_sec", cfg.BusyQuitSec)
	}
	return cfg, nil
}

// Limits converts the config to the pool limits, busy_quit_sec as a duration.
func (c Config) Limits() Limits {
	return Limits{
		Slots:      c.Slots,
		GoParallel: c.GoParallel,
		BusyQuit:   time.Duration(c.BusyQuitSec) * time.Second,
	}
}
