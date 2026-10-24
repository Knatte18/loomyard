// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
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
	// Gates maps a gate key to the tiers and scope that site runs.
	Gates map[string]GateKey `yaml:"gates"`
	// Scoper is how a computed, base or card scope maps to targets: go or none.
	Scoper string `yaml:"scoper"`
	// RefuseRaw lists the command prefixes the shell guard refuses.
	RefuseRaw []string `yaml:"refuse_raw"`
	// TierParallel holds this machine's parallel for each declared tier it overrides.
	TierParallel map[string]int `yaml:"tier_parallel"`
}

// configFile is gate.yaml as it is spelled, before defaults and validation.
// A tier_parallel entry is a node because a scalar --set writes it as a string, and a gates entry is a node so decodeGates names the key a bad entry sits under.
type configFile struct {
	Slots        int                  `yaml:"slots"`
	BusyQuitSec  int                  `yaml:"busy_quit_sec"`
	Tiers        map[string]tierFile  `yaml:"tiers"`
	Gates        map[string]yaml.Node `yaml:"gates"`
	Scoper       string               `yaml:"scoper"`
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
// `tiers` runs any shell command in a task worktree, so it is executable; the gate lists, the scoper and the refused commands switch off or hollow a guard.
// `cli_wait_sec`, `go_parallel` and `guard_tier` are retired, so a hub or machine file that still sets one loads and reconcile prunes it.
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
			"refuse_raw":    configengine.CategoryGuard,
		},
		RetiredKeys: map[string]configengine.Category{
			"cli_wait_sec": configengine.CategoryCrossPair,
			"go_parallel":  configengine.CategoryMachine,
			"guard_tier":   configengine.CategoryGuard,
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
	if err := refuseRetiredGateLists(files[0]); err != nil {
		return Config{}, err
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
		var guided guidedError
		if errors.As(err, &guided) {
			return Config{}, err
		}
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
	gates, err := decodeGates(file.Gates)
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
		Gates:        gates,
		Scoper:       file.Scoper,
		RefuseRaw:    file.RefuseRaw,
		TierParallel: tierParallel,
	}, nil
}

// guidedError marks a decode refusal that already carries its own way forward, so LoadConfig returns it without the generic clause.
type guidedError struct{ error }

func (e guidedError) Unwrap() error { return e.error }

// refuseRetiredGateLists refuses a hub file whose gates entry is still in the retired list shape, which a deploy leaves until reconcile runs MigrateConfig.
// The layered load would refuse it first as a shape mismatch against the template, naming no way forward that migrates it.
func refuseRetiredGateLists(hubFile string) error {
	data, err := os.ReadFile(hubFile)
	if err != nil {
		return nil
	}
	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil || document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	gates := mappingValue(document.Content[0], "gates")
	if gates == nil || gates.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(gates.Content); i += 2 {
		if gates.Content[i+1].Kind == yaml.SequenceNode {
			return fmt.Errorf("gate config key %q: the retired list shape; fix it with \"lyx fabric reconcile\" (or \"lyx config reconcile --layer default --apply\") from the prime, which migrates it%s", "gates."+gates.Content[i].Value, roleClause)
		}
	}
	return nil
}

// decodeGates decodes the gates entries into keys and validates each key's scope.
func decodeGates(entries map[string]yaml.Node) (map[string]GateKey, error) {
	gates := make(map[string]GateKey, len(entries))
	for _, name := range sortedKeys(entries) {
		node := entries[name]
		path := "gates." + name
		var key GateKey
		if err := node.Decode(&key); err != nil {
			return nil, fmt.Errorf("gate config key %q: %w", path, err)
		}
		switch key.Scope {
		case scopeCard:
			if name != "card" && name != "batch" {
				return nil, guidedError{fmt.Errorf("gate config key %q: scope card is only for the card and batch keys; fix it with \"lyx config gate\" from the prime%s", path, roleClause)}
			}
		case scopeDiff, scopeAll:
		default:
			return nil, guidedError{fmt.Errorf("gate config key %q: scope %q; want card, diff or all; fix it with \"lyx config gate\" from the prime%s", path, key.Scope, roleClause)}
		}
		gates[name] = key
	}
	return gates, nil
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

// gateTarget is one key a retired gate list migrates to, with the scope its mapping takes.
type gateTarget struct {
	key   string
	scope string
}

// retiredGateLists maps each retired gate list to the keys it becomes.
var retiredGateLists = map[string][]gateTarget{
	"landing": {{"merge-in", scopeAll}, {"finalize", scopeAll}, {"darn-write", scopeDiff}, {"code-burler-fix", scopeDiff}},
	"final":   {{"webster", scopeAll}},
	"round":   {{"webster-fix", scopeDiff}},
}

// MigrateConfig migrates a gate.yaml document's retired gates and keys and returns one line per rewrite.
// The retired gates.publish list and the retired guard_tier key are pruned, a landing, final or round list becomes the keys retiredGateLists names, and a card or batch list becomes a mapping with scope card.
// A key the file already holds in the new shape keeps its value, and every comment and other key is carried through yaml node editing.
// A document with nothing to migrate is returned unchanged, so a migrated file is unchanged on a second pass.
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
	var lines []string
	if gates := mappingValue(root, "gates"); gates != nil && gates.Kind == yaml.MappingNode {
		if at := mappingKeyIndex(gates, "publish"); at >= 0 {
			gates.Content = append(gates.Content[:at], gates.Content[at+2:]...)
			lines = append(lines, "gates.publish: retired, removed")
		}
		lines = append(lines, migrateGateLists(gates)...)
	}
	if at := mappingKeyIndex(root, "guard_tier"); at >= 0 {
		root.Content = append(root.Content[:at], root.Content[at+2:]...)
		lines = append(lines, "guard_tier: retired, removed")
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("migrate gate config: encode yaml: %w", err))
	}
	return out.Bytes(), lines, nil
}

// migrateGateLists rewrites every list-shaped entry of the gates mapping into the keyed shape and returns one line per rewrite.
// A retired key's list is dropped from its place and its targets, minus those the mapping already holds, take that place.
func migrateGateLists(gates *yaml.Node) []string {
	held := make(map[string]bool)
	for i := 0; i+1 < len(gates.Content); i += 2 {
		held[gates.Content[i].Value] = true
	}
	var lines []string
	var content []*yaml.Node
	for i := 0; i+1 < len(gates.Content); i += 2 {
		name, value := gates.Content[i], gates.Content[i+1]
		if value.Kind != yaml.SequenceNode {
			content = append(content, name, value)
			continue
		}
		if name.Value == "card" || name.Value == "batch" {
			content = append(content, name, gateMapping(value, scopeCard))
			lines = append(lines, fmt.Sprintf("gates.%s: list shape retired, migrated to tiers and scope %s", name.Value, scopeCard))
			continue
		}
		targets, retired := retiredGateLists[name.Value]
		if !retired {
			content = append(content, name, value)
			continue
		}
		var created []string
		for _, target := range targets {
			if held[target.key] {
				continue
			}
			held[target.key] = true
			targetName := *name
			targetName.Value = target.key
			if len(created) > 0 {
				targetName.HeadComment, targetName.LineComment, targetName.FootComment = "", "", ""
			}
			content = append(content, &targetName, gateMapping(value, target.scope))
			created = append(created, target.key)
		}
		if len(created) == 0 {
			lines = append(lines, fmt.Sprintf("gates.%s: retired, removed", name.Value))
			continue
		}
		lines = append(lines, fmt.Sprintf("gates.%s: retired, migrated to %s", name.Value, joinWithAnd(created)))
	}
	gates.Content = content
	return lines
}

// gateMapping returns a flow mapping node holding a copy of the tiers list and the scope.
func gateMapping(tiers *yaml.Node, scope string) *yaml.Node {
	list := *tiers
	list.Content = slices.Clone(tiers.Content)
	list.Style = yaml.FlowStyle
	scalar := func(value string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	}
	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Style:   yaml.FlowStyle,
		Content: []*yaml.Node{scalar("tiers"), &list, scalar("scope"), scalar(scope)},
	}
}

// joinWithAnd joins names with commas and a final "and".
func joinWithAnd(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
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
