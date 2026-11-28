// config.go — the hub-wide gate config module: its keys, strict loading and conversion to pool limits.

package gateslot

import (
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
	"github.com/Knatte18/loomyard/internal/logger"
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
	// MemoryQuitMB is the megabytes a Go test binary, with every process it starts, may hold;
	// past it the memory guard kills them all, and 0 turns the guard off.
	MemoryQuitMB int `yaml:"memory_quit_mb"`
	// FixerTimeoutMin is the minutes a cold gate fixer may run before its caller stops waiting for it.
	FixerTimeoutMin int `yaml:"fixer_timeout_min"`
	// PlanTier0BudgetSec is the seconds a plan's new and changed tier0 tests may add to one tier0 run; 0 sets no budget.
	// The planner and the plan review read it through TiersValue.
	PlanTier0BudgetSec int `yaml:"plan_tier0_budget_sec"`
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

// RedFirstMode is how `lym gate red-first` runs the changed tests of a scoper on the diff's base.
type RedFirstMode int

// The ways a scoper's changed tests run on the diff's base.
const (
	// RedFirstUnsupported is a scoper the check cannot judge tests for.
	RedFirstUnsupported RedFirstMode = iota
	// RedFirstOverlay runs the tests in the worktree, under a build overlay of the base's production files.
	RedFirstOverlay
	// RedFirstCheckout runs the tests in a checkout of the base, where the changed test files keep their working-tree text.
	RedFirstCheckout
)

// scoperRedFirstMode says, per scoper, how the red-first check runs its changed tests.
var scoperRedFirstMode = map[string]RedFirstMode{"go": RedFirstOverlay, "none": RedFirstUnsupported, "dotnet": RedFirstCheckout}

// RedFirstMode returns how the red-first check runs the config's scoper's changed tests on the diff's base.
func (c Config) RedFirstMode() RedFirstMode {
	return scoperRedFirstMode[c.Scoper]
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
// A gate.tier_parallel entry is a node because a scalar --set writes it as a string.
// A gate.gates entry is a node so decodeGates names the key a bad entry sits under.
type configFile struct {
	Slots              int                  `yaml:"slots"`
	BusyQuitSec        int                  `yaml:"busy_quit_sec"`
	MemoryQuitMB       int                  `yaml:"memory_quit_mb"`
	FixerTimeoutMin    int                  `yaml:"fixer_timeout_min"`
	PlanTier0BudgetSec int                  `yaml:"plan_tier0_budget_sec"`
	Tiers              map[string]tierFile  `yaml:"tiers"`
	Gates              map[string]yaml.Node `yaml:"gates"`
	Scoper             string               `yaml:"scoper"`
	RefuseRaw          []string             `yaml:"refuse_raw"`
	TierParallel       map[string]yaml.Node `yaml:"tier_parallel"`
	Build              buildFile            `yaml:"build"`
	InertPaths         []string             `yaml:"inert_paths"`
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
// `gate.slots` and `gate.tier_parallel` take a machine override, and every key stays closed to the local layer.
// `gate.fixer_timeout_min` and `gate.memory_quit_mb` are cross-pair like `gate.busy_quit_sec`;
// a gate key's `fixer` sits inside `gate.gates`, a guard key.
// `gate.tiers` and `gate.build` run any shell command in a task worktree, so they are executable.
// The gate lists, the scoper, the refused commands and the inert paths switch off or hollow a guard.
// `cli_wait_sec`, `go_parallel`, `guard_tier` and `test_map_max_age_days` are retired;
// a file that still sets one is refused, naming the key.
// `gate.tiers` and `gate.build` carry the shape their decode gives them.
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
			"slots":                 configengine.CategoryMachine,
			"tier_parallel":         configengine.CategoryMachine,
			"busy_quit_sec":         configengine.CategoryCrossPair,
			"memory_quit_mb":        configengine.CategoryCrossPair,
			"fixer_timeout_min":     configengine.CategoryCrossPair,
			"plan_tier0_budget_sec": configengine.CategoryGuard,
			"tiers":                 configengine.CategoryExecutable,
			"gates":                 configengine.CategoryGuard,
			"scoper":                configengine.CategoryGuard,
			"refuse_raw":            configengine.CategoryGuard,
			"build":                 configengine.CategoryExecutable,
			"inert_paths":           configengine.CategoryGuard,
		},
		RetiredKeys: map[string]configengine.Category{
			"cli_wait_sec":          configengine.CategoryCrossPair,
			"go_parallel":           configengine.CategoryMachine,
			"guard_tier":            configengine.CategoryGuard,
			"test_map_max_age_days": configengine.CategoryCrossPair,
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

// LoadRefuseRaw returns the gate.refuse_raw list of the gate config under dirs, for a session launch's shell guard.
// A launch runs no gate, so a key it does not read never refuses it:
// when the full load fails past the layer merge, the list is read from the merged stack alone,
// and the failure is logged as a warning naming its way forward.
// An absent hub file yields an empty list.
// A stack that does not merge returns the full load's error.
// A gate.refuse_raw list that is itself invalid returns that list's error with the gate config's way forward.
func LoadRefuseRaw(dirs configengine.Dirs) ([]string, error) {
	cfg, loadErr := LoadConfig(dirs)
	if loadErr == nil {
		return cfg.RefuseRaw, nil
	}
	if errors.Is(loadErr, ErrConfigAbsent) {
		return nil, nil
	}
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		return nil, loadErr
	}
	var file struct {
		RefuseRaw []string `yaml:"refuse_raw"`
	}
	if err := yaml.Unmarshal(resolved, &file); err != nil {
		return nil, loadErr
	}
	if err := decodeRefuseRaw(file.RefuseRaw); err != nil {
		return nil, withConfigWayForward(err, ConfigFiles(dirs), "")
	}
	logger.Warn("gate config: the config does not load, so the launch reads its "+keyName("refuse_raw")+" list alone; every gate call refuses until it is fixed", "error", loadErr)
	return file.RefuseRaw, nil
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

// decodeConfig decodes and validates a stacked gate document.
// machineLayerApplies tells whether the load stacked a machine layer;
// it decides the steps the dropped-gate.tier_parallel warning names.
// Cross-references between gate lists and tiers are never checked here.
func decodeConfig(data []byte, machineLayerApplies bool) (Config, error) {
	var file configFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Config{}, fmt.Errorf("unmarshal gate config: %w", err)
	}
	if file.Slots < 1 {
		return Config{}, fmt.Errorf("config key %q: %d; want at least 1", keyName("slots"), file.Slots)
	}
	if file.BusyQuitSec < 0 {
		return Config{}, fmt.Errorf("config key %q: %d; want at least 0", keyName(busyQuitSecKey), file.BusyQuitSec)
	}
	if file.MemoryQuitMB < 0 {
		return Config{}, fmt.Errorf("config key %q: %d; want at least 0", keyName("memory_quit_mb"), file.MemoryQuitMB)
	}
	if file.PlanTier0BudgetSec < 0 {
		return Config{}, fmt.Errorf("config key %q: %d; want at least 0", keyName("plan_tier0_budget_sec"), file.PlanTier0BudgetSec)
	}
	if file.FixerTimeoutMin < 1 {
		return Config{}, fmt.Errorf("config key %q: %d; want at least 1", keyName("fixer_timeout_min"), file.FixerTimeoutMin)
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
		return Config{}, fmt.Errorf("config key %q: %q; want go, none or dotnet", keyName("scoper"), file.Scoper)
	}
	build, err := decodeBuild(file.Build)
	if err != nil {
		return Config{}, err
	}
	for _, key := range sortedKeys(tiers) {
		if tiers[key].SharedBuild && build.Command == "" {
			return Config{}, fmt.Errorf("config key %q: true, but the hub declares no build step; want a build step or shared_build false", keyName("tiers."+key+".shared_build"))
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
		Slots:              file.Slots,
		BusyQuitSec:        file.BusyQuitSec,
		MemoryQuitMB:       file.MemoryQuitMB,
		FixerTimeoutMin:    file.FixerTimeoutMin,
		PlanTier0BudgetSec: file.PlanTier0BudgetSec,
		Tiers:              tiers,
		Gates:              gates,
		Scoper:             file.Scoper,
		RefuseRaw:          file.RefuseRaw,
		TierParallel:       tierParallel,
		Build:              build,
		InertPaths:         file.InertPaths,
	}, nil
}

// decodeBuild validates the build step and fills its defaults;
// a build with no key set is no step and decodes to the zero BuildStep.
func decodeBuild(file buildFile) (BuildStep, error) {
	if file.About == "" && file.Command == "" && file.Dir == "" && len(file.Env) == 0 && file.TimeoutMin == nil {
		return BuildStep{}, nil
	}
	refuse := func(field, want string) error {
		return fmt.Errorf("config key %q: %s", keyName("build."+field), want)
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
			return useRefusal("key %q: %v", keyName("tiers."+key+".dir"), err)
		}
	}
	if cfg.Build.Command != "" {
		if _, err := StatCommandDir(worktreeRoot, anchorDir, cfg.Build.Dir); err != nil {
			return useRefusal("key %q: %v", keyName("build.dir"), err)
		}
	}
	return nil
}

// Inert reports whether the worktree-relative slash path rel matches a gate.inert_paths pattern:
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

// refuseRetiredGateLists refuses a hub file whose gates entry is still in the retired list shape.
// The layered load would refuse it first as a shape mismatch against the template;
// the refusal names the mapping shape that replaces the list.
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
			return fmt.Errorf("config key %q: the retired list shape; write it as a mapping with tiers and scope, such as {tiers: [0], scope: diff}, with \"lym config gate\" from the prime%s", keyName("gates."+gates.Content[i].Value), roleClause)
		}
	}
	return nil
}

// FixerTimeout returns the bound on a cold gate fixer's run as a duration.
func (c Config) FixerTimeout() time.Duration {
	return time.Duration(c.FixerTimeoutMin) * time.Minute
}

// MemoryQuit returns the memory a test binary may hold before the memory guard kills it, `gate.memory_quit_mb` in bytes.
// It is 0 when the guard is off.
func (c Config) MemoryQuit() int64 {
	return int64(c.MemoryQuitMB) << 20
}

// decodeGates decodes the gates entries into keys and validates each key's scope, lints and fixer.
func decodeGates(entries map[string]yaml.Node) (map[string]GateKey, error) {
	gates := make(map[string]GateKey, len(entries))
	for _, name := range sortedKeys(entries) {
		node := entries[name]
		path := keyName("gates." + name)
		var key GateKey
		if err := node.Decode(&key); err != nil {
			return nil, fmt.Errorf("config key %q: %w", path, err)
		}
		switch key.Scope {
		case scopeCard:
			if name != CardGateKey && name != BatchGateKey {
				return nil, guidedError{fmt.Errorf("config key %q: scope card is only for the card and batch keys; fix it with \"lym config gate\" from the prime%s", path, roleClause)}
			}
		case scopeDiff, scopeAll, ScopeMarked:
		case retiredScopeCoverage:
			return nil, guidedError{fmt.Errorf("config key %q: scope coverage is retired, since the gate no longer selects tests from the coverage maps; set %s to diff with \"lym config gate\" from the prime%s", path, path+".scope", roleClause)}
		default:
			return nil, guidedError{fmt.Errorf("config key %q: scope %q; want card, diff, all or marked; fix it with \"lym config gate\" from the prime%s", path, key.Scope, roleClause)}
		}
		for _, lint := range key.Lints {
			if lint != LintComments && lint != LintPattern {
				return nil, guidedError{fmt.Errorf("config key %q: lint %q; want %s or %s; fix it with \"lym config gate\" from the prime%s", path, lint, LintComments, LintPattern, roleClause)}
			}
		}
		if key.Fixer != "" {
			if name != CardGateKey {
				return nil, guidedError{fmt.Errorf("config key %q: a fixer is only for the card key, since only its agent call hands a red verdict to a fixer; remove fixer from that key with \"lym config gate\" from the prime%s", path+".fixer", roleClause)}
			}
			if _, err := modelspec.Parse(key.Fixer); err != nil {
				return nil, guidedError{fmt.Errorf("config key %q: %q is no model-spec: %v; set a model-spec such as sonnet[medium], or \"\" to turn the fixer off, with \"lym config gate\" from the prime%s", path+".fixer", key.Fixer, err, roleClause)}
			}
		}
		gates[name] = key
	}
	return gates, nil
}

// Limits converts the config to the pool limits, gate.busy_quit_sec as a duration.
// Tier0Parallel is tier0's effective parallel, or 0 when the config declares no tier0.
func (c Config) Limits() Limits {
	return Limits{
		Slots:         c.Slots,
		Tier0Parallel: c.Tiers[tierKey(0)].Parallel,
		BusyQuit:      time.Duration(c.BusyQuitSec) * time.Second,
	}
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
