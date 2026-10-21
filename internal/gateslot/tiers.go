// tiers.go — the gate config's test tiers: their type, the load-time decoding and validation, and the use-time lookups a verify site renders its gate call from.

package gateslot

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Tier is one test tier the hub declares in gate.yaml: the command a run executes, its bounds and how its output is counted.
type Tier struct {
	// About is one line saying what the tier tests.
	About string
	// Command is the shell command a run executes, with the placeholders {targets}, {parallel} and {args}.
	Command string
	// Dir is the run directory relative to the worktree root.
	Dir string
	// Env is the environment the command runs with; values may hold {parallel}.
	Env map[string]string
	// AllTargets is the {targets} value of a whole-scope run.
	AllTargets string
	// Parallel is the tier's parallelism, this machine's tier_parallel entry when one is set.
	Parallel int
	// TimeoutMin is the minutes the tier may run once its slots are held.
	TimeoutMin int
	// Counter names how the command's output is counted: go-json or vstest.
	Counter string
	// Covers lists lower tier numbers this tier's command runs anyway.
	Covers []int
	// Slots is how many gate slots the tier holds; 0 means every slot.
	Slots int
	// PrebuiltLyx makes a run of the tier prebuild lyx from the worktree first.
	PrebuiltLyx bool
}

// tierFile is a tier as gate.yaml spells it, before defaults and validation.
type tierFile struct {
	About       string            `yaml:"about"`
	Command     string            `yaml:"command"`
	Dir         string            `yaml:"dir"`
	Env         map[string]string `yaml:"env"`
	AllTargets  string            `yaml:"all_targets"`
	Parallel    *int              `yaml:"parallel"`
	TimeoutMin  *int              `yaml:"timeout_min"`
	Counter     string            `yaml:"counter"`
	Covers      []int             `yaml:"covers"`
	Slots       yaml.Node         `yaml:"slots"`
	PrebuiltLyx bool              `yaml:"prebuilt_lyx"`
}

// goReadGates are the gate names the Go verify sites read, in the order TiersValue lists them.
var goReadGates = []string{"card", "batch", "round", "final", "landing", "publish"}

var (
	tierKeyPattern   = regexp.MustCompile(`^tier(0|[1-9][0-9]*)$`)
	refuseRawPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+( [A-Za-z0-9._-]+)*$`)
)

// tierKey returns the gate.yaml key of tier number.
func tierKey(number int) string {
	return "tier" + strconv.Itoa(number)
}

// decodeTiers validates and converts the tiers a gate.yaml declares.
// Cross-references between tiers and gates are never checked here.
func decodeTiers(files map[string]tierFile) (map[string]Tier, error) {
	tiers := make(map[string]Tier, len(files))
	for key, file := range files {
		if !tierKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("gate config key %q under \"tiers\": want a name spelled tier<N>, such as tier0", key)
		}
		number, _ := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		tier, err := decodeTier(key, number, file)
		if err != nil {
			return nil, err
		}
		tiers[key] = tier
	}
	return tiers, nil
}

// decodeTier validates one tier and fills its defaults.
func decodeTier(key string, number int, file tierFile) (Tier, error) {
	refuse := func(field, want string) error {
		return fmt.Errorf("gate config key %q: %s", "tiers."+key+"."+field, want)
	}
	tier := Tier{
		About:       file.About,
		Command:     file.Command,
		Dir:         file.Dir,
		Env:         file.Env,
		AllTargets:  file.AllTargets,
		Parallel:    1,
		Counter:     file.Counter,
		Covers:      file.Covers,
		Slots:       1,
		PrebuiltLyx: file.PrebuiltLyx,
	}
	if tier.Dir == "" {
		tier.Dir = "."
	}
	if strings.TrimSpace(tier.Command) == "" {
		return Tier{}, refuse("command", "missing; want the shell command a run executes")
	}
	if strings.Contains(tier.Command, "{targets}") && tier.AllTargets == "" {
		return Tier{}, refuse("all_targets", "missing; want the {targets} value of a whole run, since the command holds {targets}")
	}
	if file.TimeoutMin == nil {
		return Tier{}, refuse("timeout_min", "missing; want at least 1")
	}
	if *file.TimeoutMin < 1 {
		return Tier{}, refuse("timeout_min", fmt.Sprintf("%d; want at least 1", *file.TimeoutMin))
	}
	tier.TimeoutMin = *file.TimeoutMin
	if file.Parallel != nil {
		if *file.Parallel < 1 {
			return Tier{}, refuse("parallel", fmt.Sprintf("%d; want at least 1", *file.Parallel))
		}
		tier.Parallel = *file.Parallel
	}
	switch tier.Counter {
	case "go-json", "vstest":
	case "":
		return Tier{}, refuse("counter", "missing; want go-json or vstest")
	default:
		return Tier{}, refuse("counter", fmt.Sprintf("%q; want go-json or vstest", tier.Counter))
	}
	for _, covered := range tier.Covers {
		if covered >= number {
			return Tier{}, refuse("covers", fmt.Sprintf("%d; want tier numbers lower than %d", covered, number))
		}
	}
	if slots := file.Slots.Value; file.Slots.Kind != 0 {
		count, err := strconv.Atoi(slots)
		switch {
		case slots == "all":
			tier.Slots = 0
		case err != nil || count < 1:
			return Tier{}, refuse("slots", fmt.Sprintf("%q; want all or a count of at least 1", slots))
		default:
			tier.Slots = count
		}
	}
	return tier, nil
}

// decodeRefuseRaw checks that every refuse_raw entry is one or more plain words separated by single spaces.
func decodeRefuseRaw(entries []string) error {
	for _, entry := range entries {
		if !refuseRawPattern.MatchString(entry) {
			return fmt.Errorf("gate config key %q: %q; want one or more words of letters, digits, '.', '_' and '-' separated by single spaces", "refuse_raw", entry)
		}
	}
	return nil
}

// applyTierParallel reads the tier_parallel entries, which a scalar --set writes as strings, and lays them over the declared tiers' parallel.
// An entry for a tier the config does not declare is dropped with a warning that names the steps removing it.
// It returns the entries kept.
func applyTierParallel(entries map[string]yaml.Node, tiers map[string]Tier, machineLayerApplies bool) (map[string]int, error) {
	kept := make(map[string]int, len(entries))
	for _, name := range sortedKeys(entries) {
		node := entries[name]
		count, err := strconv.Atoi(node.Value)
		if node.Kind != yaml.ScalarNode || err != nil || count < 1 {
			return nil, fmt.Errorf("gate config key %q: %q; want an integer of at least 1", "tier_parallel."+name, node.Value)
		}
		tier, declared := tiers[name]
		if !declared {
			logger.Warn("gate config: tier_parallel entry names a tier the hub does not declare, so it is dropped", "tier", name, "way_forward", undeclaredTierParallelWayForward(name, machineLayerApplies))
			continue
		}
		tier.Parallel = count
		tiers[name] = tier
		kept[name] = count
	}
	return kept, nil
}

// undeclaredTierParallelWayForward returns the steps that remove a tier_parallel entry for an undeclared tier.
// With a machine layer the stacked document cannot tell which layer holds the entry, so both steps are named.
func undeclaredTierParallelWayForward(name string, machineLayerApplies bool) string {
	if !machineLayerApplies {
		return fmt.Sprintf("remove tier_parallel.%s with \"lyx config gate\"", name)
	}
	return fmt.Sprintf("remove tier_parallel.%s with \"lyx config gate\" when the hub file holds it, or in the editor session of \"lyx config gate --layer machine\" when the machine file does", name)
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// useRefusal returns a use-time refusal naming lyx config gate and closing with the role clause.
func useRefusal(format string, args ...any) error {
	return fmt.Errorf("gate config: %s; fix it with \"lyx config gate\" from the prime%s", fmt.Sprintf(format, args...), roleClause)
}

// Tier returns the declared tier with the given number, or refuses naming lyx config gate.
func (c Config) Tier(number int) (Tier, error) {
	tier, ok := c.Tiers[tierKey(number)]
	if !ok {
		return Tier{}, useRefusal("the hub declares no %s", tierKey(number))
	}
	return tier, nil
}

// TierList returns the tier numbers the named gate runs, in order.
// It refuses, naming lyx config gate, a hub with no tier0, a gate the config holds no list for, an empty list for any gate but publish, and a listed tier the hub does not declare.
func (c Config) TierList(gate string) ([]int, error) {
	if _, declared := c.Tiers[tierKey(0)]; !declared {
		return nil, useRefusal("the hub declares no tier0, so no gate has a tier to run")
	}
	list, ok := c.Gates[gate]
	if !ok {
		return nil, useRefusal("gates holds no list for %q", gate)
	}
	if len(list) == 0 && gate != "publish" {
		return nil, useRefusal("gates.%s is empty, and only publish may verify nothing", gate)
	}
	for _, number := range list {
		if _, declared := c.Tiers[tierKey(number)]; !declared {
			return nil, useRefusal("gates.%s lists %s, which the hub does not declare", gate, tierKey(number))
		}
	}
	return slices.Clone(list), nil
}

// GateCall renders the one call a site runs for the named gate: `lyx gate test --wait` with one `--tier <n>` per listed tier, in the list's order, then the scope words.
// An empty publish list renders "".
func (c Config) GateCall(gate string, scope ...string) (string, error) {
	list, err := c.TierList(gate)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", nil
	}
	words := []string{"lyx", "gate", "test", "--wait"}
	for _, number := range list {
		words = append(words, "--tier", strconv.Itoa(number))
	}
	return strings.Join(append(words, scope...), " "), nil
}

// TiersValue renders the value prompts take for the hub's tiers:
// one line per declared tier in number order, `- tier<N>: <about>`, then one line per Go-read gate the config lists, `- gate <name>: [<n>, ...]`.
// It is `none` when no tier is declared.
func (c Config) TiersValue() string {
	if len(c.Tiers) == 0 {
		return "none"
	}
	numbers := make([]int, 0, len(c.Tiers))
	for key := range c.Tiers {
		number, _ := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	lines := make([]string, 0, len(numbers)+len(goReadGates))
	for _, number := range numbers {
		lines = append(lines, fmt.Sprintf("- %s: %s", tierKey(number), c.Tiers[tierKey(number)].About))
	}
	for _, gate := range goReadGates {
		list, ok := c.Gates[gate]
		if !ok {
			continue
		}
		items := make([]string, len(list))
		for i, number := range list {
			items[i] = strconv.Itoa(number)
		}
		lines = append(lines, fmt.Sprintf("- gate %s: [%s]", gate, strings.Join(items, ", ")))
	}
	return strings.Join(lines, "\n")
}
