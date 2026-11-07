// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// roleClause ends every gate config refusal's way forward.
// The steps need the prime, so any other session reports and the orch takes them.
const roleClause = "; a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them"

// Config is the resolved gate config of the hub file and this machine's override.
// The yaml tags name the gate.yaml keys.
// A Config is never unmarshalled directly; the load validates the file first.
type Config struct {
	// Slots is how many gate runs the hub executes at once.
	Slots int `yaml:"slots"`
	// BusyQuitSec is the seconds a Go test binary may keep a core busy before the watchdog quits it; 0 turns it off.
	BusyQuitSec int `yaml:"busy_quit_sec"`
	// FixerTimeoutMin is the minutes a cold gate fixer may run before its caller stops waiting for it.
	FixerTimeoutMin int `yaml:"fixer_timeout_min"`
	// Tiers are the declared test tiers keyed tier0, tier1, and so on.
	Tiers map[string]Tier `yaml:"tiers"`
	// Gates maps a gate key to the tiers and scope that site runs.
	Gates map[string]GateKey `yaml:"gates"`
	// Scoper is how a computed, base or card scope maps to targets: go, none or dotnet.
	Scoper string `yaml:"scoper"`
	// RefuseRaw lists the command prefixes the shell guard refuses.
	RefuseRaw []string `yaml:"refuse_raw"`
	// TierParallel holds this machine's parallel for each declared tier it overrides.
	TierParallel map[string]int `yaml:"tier_parallel"`
	// Build is the hub's shared build step; its zero value declares none, and a declared step has a non-empty Command.
	Build BuildStep `yaml:"build"`
	// InertPaths are the path patterns of files no test reads, matched by Inert.
	InertPaths []string `yaml:"inert_paths"`
}

// BuildStep is the build a gate call runs once for every tier that sets shared_build.
type BuildStep struct {
	// About is one line saying what the build produces.
	About string
	// Command is the shell command the build runs.
	Command string
	// Dir is the run directory relative to the worktree root, empty when gate.yaml sets none.
	Dir string
	// Env is the environment the command runs with.
	Env map[string]string
	// TimeoutMin is the minutes the build may run once its slots are held.
	TimeoutMin int
}

// buildFile is the build step as gate.yaml spells it, before defaults and validation.
type buildFile struct {
	About      string            `yaml:"about"`
	Command    string            `yaml:"command"`
	Dir        string            `yaml:"dir"`
	Env        map[string]string `yaml:"env"`
	TimeoutMin *int              `yaml:"timeout_min"`
}

// configFile is gate.yaml as it is spelled, before defaults and validation.
// A tier_parallel entry is a node because a scalar --set writes it as a string.
// A gates entry is a node so decodeGates names the key a bad entry sits under.
type configFile struct {
	Slots           int                  `yaml:"slots"`
	BusyQuitSec     int                  `yaml:"busy_quit_sec"`
	FixerTimeoutMin int                  `yaml:"fixer_timeout_min"`
	Tiers           map[string]tierFile  `yaml:"tiers"`
	Gates           map[string]yaml.Node `yaml:"gates"`
	Scoper          string               `yaml:"scoper"`
	RefuseRaw       []string             `yaml:"refuse_raw"`
	TierParallel    map[string]yaml.Node `yaml:"tier_parallel"`
	Build           buildFile            `yaml:"build"`
	InertPaths      []string             `yaml:"inert_paths"`
}

// ErrConfigAbsent is LoadConfig's error for a layer stack whose hub gate.yaml is missing.
// The way forward is `lym fabric reconcile` in a hub, which writes the file from the template.
// Outside every hub, `lym config gate` writes the worktree's own file.
var ErrConfigAbsent = errors.New("gate config absent")

// ConfigOpenMaps returns the gate.yaml keys whose entries the operator owns.
// configengine carries those entries whole through reconcile and the layer stack.
func ConfigOpenMaps() []string {
	return []string{"tiers", "tier_parallel", "build"}
}

// ConfigPerNameMaps returns the gate.yaml keys whose entries are named by the operator and merged name by name:
// a hub file that tunes one gate keeps the template's list for every other gate.
func ConfigPerNameMaps() []string {
	return []string{"gates"}
}

// ConfigSpec returns gate's layered-load spec.
// `slots` and `tier_parallel` take a machine override, and every key stays closed to the local layer.
// `fixer_timeout_min` is cross-pair like `busy_quit_sec`; a gate key's `fixer` sits inside `gates`, a guard key.
// `tiers` and `build` run any shell command in a task worktree, so they are executable.
// The gate lists, the scoper, the refused commands and the inert paths switch off or hollow a guard.
// `cli_wait_sec`, `go_parallel` and `guard_tier` are retired;
// a hub or machine file that still sets one loads, and reconcile prunes it.
// `tiers` and `build` carry the shape their decode gives them.
// So `--set` reaches one key inside a tier or the build step, such as `tiers.tier0.env.LYM_TEST_TIER`;
// it refuses a key neither holds.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     "gate",
		Template: []byte(ConfigTemplate()),
		OpenMaps: ConfigOpenMaps(),
		OpenMapShapes: map[string]*yamlengine.Shape{
			"tiers": yamlengine.ShapeOf(map[string]tierFile{}),
			"build": yamlengine.ShapeOf(buildFile{}),
		},
		PerNameMaps: ConfigPerNameMaps(),
		HubWide:     true,
		Categories: map[string]configengine.Category{
			"slots":             configengine.CategoryMachine,
			"tier_parallel":     configengine.CategoryMachine,
			"busy_quit_sec":     configengine.CategoryCrossPair,
			"fixer_timeout_min": configengine.CategoryCrossPair,
			"tiers":             configengine.CategoryExecutable,
			"gates":             configengine.CategoryGuard,
			"scoper":            configengine.CategoryGuard,
			"refuse_raw":        configengine.CategoryGuard,
			"build":             configengine.CategoryExecutable,
			"inert_paths":       configengine.CategoryGuard,
		},
		RetiredKeys: map[string]configengine.Category{
			"cli_wait_sec": configengine.CategoryCrossPair,
			"go_parallel":  configengine.CategoryMachine,
			"guard_tier":   configengine.CategoryGuard,
		},
	}
}

// ConfigFiles returns the gate config files a load under dirs stacks:
// the hub file, followed by the machine file when that layer applies and the file is present.
// A machine file that cannot be statted is left out;
// the load then fails reading it with an error naming its path.
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

// LoadConfig loads the gate config strictly through the hub-wide layer stack under dirs, machine override included:
// an absent hub file is ErrConfigAbsent, and a value out of range fails naming its key.
// Outside every hub, where dirs name no board, the file is the worktree's own.
// Its absence names `lym config gate` as the way to write it.
// Every other failure names each file that can hold the value and its way forward;
// each gate site that wraps it carries the clause.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	spec := ConfigSpec()
	files := ConfigFiles(dirs)
	if _, err := os.Stat(files[0]); errors.Is(err, os.ErrNotExist) {
		if dirs.Board == "" {
			return Config{}, fmt.Errorf("%w; run \"lym config gate\" to write this worktree's own gate.yaml, creating the worktree's _lym directory first when it is absent, since the write refuses without it; a session lym refuses the verb from reports status: FAILED and the orch runs it", ErrConfigAbsent)
		}
		return Config{}, fmt.Errorf("%w; run \"lym fabric reconcile\"; a session lym refuses the verb from reports status: FAILED and the orch runs it", ErrConfigAbsent)
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
// An empty or comment-only file is one the load drops, so removing it fixes nothing;
// an unparsable file counts as holding units, since the load fails naming it.
func holdsUnits(path string, spec configengine.Spec) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	units, err := yamlengine.LeafPaths(data, spec.OpenMaps, spec.WholeEntries)
	return err != nil || len(units) > 0
}

// withConfigWayForward appends to err the way forward for a gate config that is present but unusable.
// files are the hub file and, when present, the machine file;
// local is the local override file, or "" when none takes part.
// The clause for a session that is not the prime closes every form.
func withConfigWayForward(err error, files []string, local string) error {
	removeLocal := ""
	if local != "" {
		removeLocal = fmt.Sprintf("; remove %s, which takes no gate key", local)
	}
	if len(files) < 2 {
		return fmt.Errorf("%w; fix it with \"lym config gate\" from the prime%s%s", err, removeLocal, roleClause)
	}
	hub, machine := files[0], files[1]
	return fmt.Errorf("%w; the hub file %s, the machine file %s or the file the error names holds the value: "+
		"fix the hub file with \"lym config gate\" from the prime, "+
		"and the machine file with \"lym config gate --layer machine\" from the prime, "+
		"or, when that refuses a key the machine file holds, by removing %s; "+
		"\"lym config gate --resolved\" names the layer holding each value%s%s",
		err, hub, machine, machine, removeLocal, roleClause)
}

// TemplateConfig decodes the embedded template alone, the config of a run outside every hub.
func TemplateConfig() (Config, error) {
	return decodeConfig([]byte(ConfigTemplate()), false)
}

// decodeConfig decodes and validates a stacked gate document.
// machineLayerApplies tells whether the load stacked a machine layer;
// it decides the steps the dropped-tier_parallel warning names.
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
	if file.FixerTimeoutMin < 1 {
		return Config{}, fmt.Errorf("gate config key %q: %d; want at least 1", "fixer_timeout_min", file.FixerTimeoutMin)
	}
	tiers, err := decodeTiers(file.Tiers)
	if err != nil {
		return Config{}, err
	}
	gates, err := decodeGates(file.Gates)
	if err != nil {
		return Config{}, err
	}
	if file.Scoper != "go" && file.Scoper != "none" && file.Scoper != "dotnet" {
		return Config{}, fmt.Errorf("gate config key %q: %q; want go, none or dotnet", "scoper", file.Scoper)
	}
	build, err := decodeBuild(file.Build)
	if err != nil {
		return Config{}, err
	}
	for _, key := range sortedKeys(tiers) {
		if tiers[key].SharedBuild && build.Command == "" {
			return Config{}, fmt.Errorf("gate config key %q: true, but the hub declares no build step; want a build step or shared_build false", "tiers."+key+".shared_build")
		}
	}
	if err := decodeRefuseRaw(file.RefuseRaw); err != nil {
		return Config{}, err
	}
	tierParallel, err := applyTierParallel(file.TierParallel, tiers, machineLayerApplies)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Slots:           file.Slots,
		BusyQuitSec:     file.BusyQuitSec,
		FixerTimeoutMin: file.FixerTimeoutMin,
		Tiers:           tiers,
		Gates:           gates,
		Scoper:          file.Scoper,
		RefuseRaw:       file.RefuseRaw,
		TierParallel:    tierParallel,
		Build:           build,
		InertPaths:      file.InertPaths,
	}, nil
}

// decodeBuild validates the build step and fills its defaults;
// a build with no key set is no step and decodes to the zero BuildStep.
func decodeBuild(file buildFile) (BuildStep, error) {
	if file.About == "" && file.Command == "" && file.Dir == "" && len(file.Env) == 0 && file.TimeoutMin == nil {
		return BuildStep{}, nil
	}
	refuse := func(field, want string) error {
		return fmt.Errorf("gate config key %q: %s", "build."+field, want)
	}
	if strings.TrimSpace(file.Command) == "" {
		return BuildStep{}, refuse("command", "missing; want the shell command the build runs")
	}
	if file.TimeoutMin == nil {
		return BuildStep{}, refuse("timeout_min", "missing; want at least 1")
	}
	if *file.TimeoutMin < 1 {
		return BuildStep{}, refuse("timeout_min", fmt.Sprintf("%d; want at least 1", *file.TimeoutMin))
	}
	return BuildStep{About: file.About, Command: file.Command, Dir: file.Dir, Env: file.Env, TimeoutMin: *file.TimeoutMin}, nil
}

// ResolveCommandDir returns the directory a hub command whose configured dir is dir runs in:
// the anchored directory anchorDir when dir is empty; dir itself when absolute;
// otherwise dir joined onto the worktree root worktreeRoot.
// It is the rule the drift steps and the hub-wide dir checks resolve a dir by.
// The gate's own run path reaches the same directory by filling an unset dir with the anchor relative to the root.
// Joining the result onto the root, one value means one directory in every config file.
func ResolveCommandDir(worktreeRoot, anchorDir, dir string) string {
	switch {
	case dir == "":
		return anchorDir
	case filepath.IsAbs(dir):
		return dir
	default:
		return filepath.Join(worktreeRoot, dir)
	}
}

// StatCommandDir returns the directory a hub command whose configured dir is dir runs in.
// It resolves dir against worktreeRoot and anchorDir.
// It refuses one that is no existing directory with an error naming dir and the resolved path.
func StatCommandDir(worktreeRoot, anchorDir, dir string) (string, error) {
	resolved := ResolveCommandDir(worktreeRoot, anchorDir, dir)
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return resolved, nil
	}
	return resolved, fmt.Errorf("dir %q resolves to %s, which is no existing directory; want an existing directory, absolute or relative to the worktree root, or no dir to run in the worktree's anchor", dir, resolved)
}

// CheckDirs loads the gate config under dirs and refuses a tier or build dir that is no existing directory.
// Relative dirs resolve against worktreeRoot, and an unset dir resolves to anchorDir.
// A hub-wide gate config write runs it against the prime,
// so a dir that would fail every gate run is refused when it is written.
func CheckDirs(dirs configengine.Dirs, worktreeRoot, anchorDir string) error {
	cfg, err := LoadConfig(dirs)
	if err != nil {
		return err
	}
	for _, key := range sortedKeys(cfg.Tiers) {
		if _, err := StatCommandDir(worktreeRoot, anchorDir, cfg.Tiers[key].Dir); err != nil {
			return useRefusal("key %q: %v", "tiers."+key+".dir", err)
		}
	}
	if cfg.Build.Command != "" {
		if _, err := StatCommandDir(worktreeRoot, anchorDir, cfg.Build.Dir); err != nil {
			return useRefusal("key %q: %v", "build.dir", err)
		}
	}
	return nil
}

// Inert reports whether the worktree-relative slash path rel matches an inert_paths pattern:
// a pattern without a slash matches the path's base name, a pattern with one matches the whole path.
// A pattern path.Match rejects matches nothing.
func (c Config) Inert(rel string) bool {
	for _, pattern := range c.InertPaths {
		subject := rel
		if !strings.Contains(pattern, "/") {
			subject = path.Base(rel)
		}
		if matched, err := path.Match(pattern, subject); err == nil && matched {
			return true
		}
	}
	return false
}

// AllInert reports whether paths is non-empty, InertPaths is non-empty and every path is inert.
func (c Config) AllInert(paths []string) bool {
	if len(c.InertPaths) == 0 || len(paths) == 0 {
		return false
	}
	return !slices.ContainsFunc(paths, func(rel string) bool { return !c.Inert(rel) })
}

// guidedError marks a decode refusal that already carries its own way forward;
// LoadConfig returns it without the generic clause.
type guidedError struct{ error }

func (e guidedError) Unwrap() error { return e.error }

// refuseRetiredGateLists refuses a hub file whose gates entry is still in the retired list shape;
// a deploy leaves such a file until reconcile runs MigrateConfig.
// The layered load would refuse it first as a shape mismatch against the template;
// it names no way forward that migrates it.
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
			return fmt.Errorf("gate config key %q: the retired list shape; fix it with \"lym fabric reconcile\" (or \"lym config reconcile --layer default --apply\") from the prime, which migrates it%s", "gates."+gates.Content[i].Value, roleClause)
		}
	}
	return nil
}

// FixerTimeout returns the bound on a cold gate fixer's run as a duration.
func (c Config) FixerTimeout() time.Duration {
	return time.Duration(c.FixerTimeoutMin) * time.Minute
}

// decodeGates decodes the gates entries into keys and validates each key's scope, lints and fixer.
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
			if name != cardGateKey && name != "batch" {
				return nil, guidedError{fmt.Errorf("gate config key %q: scope card is only for the card and batch keys; fix it with \"lym config gate\" from the prime%s", path, roleClause)}
			}
		case scopeDiff, scopeAll:
		default:
			return nil, guidedError{fmt.Errorf("gate config key %q: scope %q; want card, diff or all; fix it with \"lym config gate\" from the prime%s", path, key.Scope, roleClause)}
		}
		for _, lint := range key.Lints {
			if lint != LintComments && lint != LintPattern {
				return nil, guidedError{fmt.Errorf("gate config key %q: lint %q; want %s or %s; fix it with \"lym config gate\" from the prime%s", path, lint, LintComments, LintPattern, roleClause)}
			}
		}
		if key.Fixer != "" {
			if name != cardGateKey {
				return nil, guidedError{fmt.Errorf("gate config key %q: a fixer is only for the card key, since only its agent call hands a red verdict to a fixer; remove fixer from that key with \"lym config gate\" from the prime%s", path+".fixer", roleClause)}
			}
			if _, err := modelspec.Parse(key.Fixer); err != nil {
				return nil, guidedError{fmt.Errorf("gate config key %q: %q is no model-spec: %v; set a model-spec such as sonnet[medium], or \"\" to turn the fixer off, with \"lym config gate\" from the prime%s", path+".fixer", key.Fixer, err, roleClause)}
			}
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

// The gate key of the parent-align check, and the retired merge-in key that migrates to it.
const (
	parentAlignGateKey    = "parent-align"
	retiredMergeInGateKey = "merge-in"
)

// The gate keys the adopt seed reads and writes.
const (
	finalizeGateKey = "finalize"
	adoptGateKey    = "adopt"
)

// retiredGateLists maps each retired gate list to the keys it becomes.
var retiredGateLists = map[string][]gateTarget{
	"landing": {{parentAlignGateKey, scopeAll}, {"finalize", scopeAll}, {"darn-write", scopeDiff}, {"code-burler-fix", scopeDiff}},
	"final":   {{"webster", scopeAll}},
	"round":   {{"webster-fix", scopeDiff}},
}

// MigrateConfig migrates a gate.yaml document's retired gates and keys and returns one line per rewrite.
// The retired gates.publish list and the retired guard_tier key are pruned.
// A landing, final or round list becomes the keys that `retiredGateLists` maps it to,
// and a card or batch list becomes a mapping with scope card.
// A gates.merge-in key is renamed to gates.parent-align, keeping its value.
// A key the file already holds in the new shape keeps its value,
// and every comment and other key is carried through yaml node editing.
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
		lines = append(lines, renameGateKeys(gates)...)
	}
	if at := mappingKeyIndex(root, "guard_tier"); at >= 0 {
		root.Content = append(root.Content[:at], root.Content[at+2:]...)
		lines = append(lines, "guard_tier: retired, removed")
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}

	encoded, err := encodeGateDocument(&document, "migrate gate config")
	if err != nil {
		return nil, nil, err
	}
	return encoded, lines, nil
}

// encodeGateDocument encodes a parsed gate document with two-space indentation.
// It returns an error marked configengine.ErrInvalid whose message opens with what.
func encodeGateDocument(document *yaml.Node, what string) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("%s: encode yaml: %w", what, err))
	}
	if err := encoder.Close(); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("%s: encode yaml: %w", what, err))
	}
	return out.Bytes(), nil
}

// SeedAdoptGate adds the adopt gate to a hub's base gate document when the document tunes finalize;
// it returns the line that says so.
// The new gates.adopt carries a copy of gates.finalize's value,
// so the adopt check runs the tiers the hub tuned for its landing.
// A document with no gates.finalize, or already holding gates.adopt, is returned unchanged with no line,
// so a second pass changes nothing.
// A hub that tunes finalize cannot pin adopt to the template's own value:
// the reconcile drops a template-equal entry, and the next pass seeds adopt again from finalize.
// It runs over the hub base file alone: a local-layer file, a seed and a run-settings gate object are never seeded.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func SeedAdoptGate(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("seed adopt gate: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("seed adopt gate: yaml root is not a mapping"))
	}
	gates := mappingValue(root, "gates")
	if gates == nil || gates.Kind != yaml.MappingNode {
		return existing, nil, nil
	}
	lines := seedAdoptGate(gates)
	if len(lines) == 0 {
		return existing, nil, nil
	}
	encoded, err := encodeGateDocument(&document, "seed adopt gate")
	if err != nil {
		return nil, nil, err
	}
	return encoded, lines, nil
}

// seedAdoptGate writes adopt into the gates mapping right after finalize:
// a flow mapping holding a copy of finalize's value; it returns the line that says so.
// A mapping without a finalize mapping, or already holding adopt, is left as it is.
func seedAdoptGate(gates *yaml.Node) []string {
	at := mappingKeyIndex(gates, finalizeGateKey)
	if at < 0 || gates.Content[at+1].Kind != yaml.MappingNode || mappingKeyIndex(gates, adoptGateKey) >= 0 {
		return nil
	}
	value := cloneGateNode(gates.Content[at+1])
	value.Style = yaml.FlowStyle
	name := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: adoptGateKey}
	gates.Content = slices.Insert(gates.Content, at+2, name, value)
	return []string{fmt.Sprintf("gates.%s: added with the value of gates.%s", adoptGateKey, finalizeGateKey)}
}

// cloneGateNode returns a deep copy of node without its comments.
func cloneGateNode(node *yaml.Node) *yaml.Node {
	clone := *node
	clone.HeadComment, clone.LineComment, clone.FootComment = "", "", ""
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = cloneGateNode(child)
	}
	return &clone
}

// migrateGateLists rewrites every list-shaped gate entry into the keyed shape and returns one line per rewrite.
// A retired key's list is dropped from its place;
// its targets, minus those the mapping already holds, take that place.
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
		if name.Value == cardGateKey || name.Value == "batch" {
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

// renameGateKeys renames the gates mapping's retired merge-in key to parent-align and returns one line per rewrite.
// The renamed key carries the value and comments of the retired one.
// When the mapping already holds parent-align, that key keeps its value and the retired key is dropped.
func renameGateKeys(gates *yaml.Node) []string {
	at := mappingKeyIndex(gates, retiredMergeInGateKey)
	if at < 0 {
		return nil
	}
	if mappingKeyIndex(gates, parentAlignGateKey) >= 0 {
		gates.Content = append(gates.Content[:at], gates.Content[at+2:]...)
		return []string{fmt.Sprintf("gates.%s: retired, removed", retiredMergeInGateKey)}
	}
	gates.Content[at].Value = parentAlignGateKey
	return []string{fmt.Sprintf("gates.%s: renamed to gates.%s", retiredMergeInGateKey, parentAlignGateKey)}
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
