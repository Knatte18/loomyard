// tiers.go holds the gate config's test tiers: their type, load-time decoding and validation, and use-time lookups.

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

// Tier is one test tier the hub declares in gate.yaml: its command, bounds and how its output is counted.
type Tier struct {
	// About is one line saying what the tier tests.
	About string
	// Command is the shell command run.
	// Its placeholders are {targets}, {test_targets}, {parallel}, {args}, {filter} and {test_timeout}.
	Command string
	// Dir is the run directory relative to the worktree root, empty when gate.yaml sets none.
	Dir string
	// Env is the environment the command runs with; values may hold {parallel}.
	Env map[string]string
	// AllTargets is the targets of a whole-scope run, which {targets} and {test_targets} take.
	AllTargets string
	// Parallel is the tier's parallelism, this machine's tier_parallel entry when one is set.
	Parallel int
	// TimeoutMin is the minutes the tier may run once its slots are held.
	TimeoutMin int
	// TestTimeoutMin is the per-binary bound the {test_timeout} placeholder renders, in minutes; 0 when there is none.
	TestTimeoutMin int
	// Counter names how the command's output is counted: go-json or vstest.
	Counter string
	// Covers lists lower tier numbers this tier's command runs anyway.
	Covers []int
	// Slots is how many gate slots the tier holds; 0 means every slot.
	Slots int
	// PrebuiltLym makes a run of the tier prebuild lym from the worktree first.
	PrebuiltLym bool
	// Manual keeps the tier out of a tier range, so it runs only when named by number, such as a tier that spends tokens.
	Manual bool
	// Filter is the tier's own test-filter expression, which a run renders into the command's {filter} placeholder.
	Filter string
	// SharedBuild makes the tier run after the hub's build step, which the first such tier of a call runs once for all.
	SharedBuild bool
	// NoCarry keeps a green run from being carried over an unchanged tree; the zero value carries.
	NoCarry bool
}

// tierFile is a tier as gate.yaml spells it, before defaults and validation.
type tierFile struct {
	About          string            `yaml:"about"`
	Command        string            `yaml:"command"`
	Dir            string            `yaml:"dir"`
	Env            map[string]string `yaml:"env"`
	AllTargets     string            `yaml:"all_targets"`
	Parallel       *int              `yaml:"parallel"`
	TimeoutMin     *int              `yaml:"timeout_min"`
	TestTimeoutMin *int              `yaml:"test_timeout_min"`
	Counter        string            `yaml:"counter"`
	Covers         []int             `yaml:"covers"`
	Slots          yaml.Node         `yaml:"slots"`
	PrebuiltLym    bool              `yaml:"prebuilt_lym"`
	Manual         bool              `yaml:"manual"`
	Filter         string            `yaml:"filter"`
	SharedBuild    bool              `yaml:"shared_build"`
	Carry          *bool             `yaml:"carry"`
}

// TestTimeoutPlaceholder is the tier command placeholder a run fills with the tier's TestTimeoutMin followed by `m`.
// It renders as a duration such as `20m`, the form `go test -timeout` reads.
const TestTimeoutPlaceholder = "{test_timeout}"

// TestTargetsPlaceholder is the tier command placeholder a run fills with its targets narrowed to test packages.
// They are the packages that hold a test of the tier.
// It stands where the targets reach `go test`, so a run starts no test binary that would find no test to run.
const TestTargetsPlaceholder = "{test_targets}"

// HoldsTargets reports whether command takes a run's targets, through {targets} or TestTargetsPlaceholder.
func HoldsTargets(command string) bool {
	return strings.Contains(command, "{targets}") || strings.Contains(command, TestTargetsPlaceholder)
}

// DefaultTestTimeoutMin is the TestTimeoutMin of a tier whose command holds TestTimeoutPlaceholder and that sets no test_timeout_min.
// It leaves room above the wall of the slowest package on a heavily loaded machine.
const DefaultTestTimeoutMin = 20

// GateKey is one gate.yaml key: the tiers a site runs, the scope its call runs them over, and its lints.
type GateKey struct {
	// Tiers are the tier numbers the key runs, in order.
	Tiers []int `yaml:"tiers"`
	// Scope is card, diff or all.
	Scope string `yaml:"scope"`
	// Lints are the lint steps the key's call runs after its tiers, in order, each LintComments or LintPattern; empty runs none.
	Lints []string `yaml:"lints"`
	// Fixer is the model-spec of the cold fixer that a red agent call of this key hands its verdict to; empty turns it off.
	// Only the card key may hold one.
	Fixer string `yaml:"fixer"`
}

// cardGateKey is the name of the gate.yaml key that runs a card's gate, the one key whose agent call may hold a fixer.
const cardGateKey = "card"

// Scope values a gate key may hold.
const (
	scopeCard = "card"
	scopeDiff = "diff"
	scopeAll  = "all"
)

// Lint step names a gate key's lints may hold.
const (
	// LintComments is the comment line lint.
	// It flags fixed-column wraps and over-long lines added to `//` blocks versus HEAD.
	LintComments = "comments"
	// LintPattern is the PATTERN headroom lint: it flags PATTERN.md growth past its headroom line versus HEAD.
	LintPattern = "pattern"
)

// goReadGates are the gate keys the Go verify sites read, in the order TiersValue lists them.
var goReadGates = []string{"card", "batch", "webster", "parent-align", "finalize", "adopt", "webster-fix", "darn-write", "code-burler-fix", "pr-review"}

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
		number, err := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		if err != nil {
			return nil, fmt.Errorf("gate config key %q under \"tiers\": want a tier number that fits an int, such as tier0", key)
		}
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
		PrebuiltLym: file.PrebuiltLym,
		Manual:      file.Manual,
		Filter:      file.Filter,
		SharedBuild: file.SharedBuild,
		NoCarry:     file.Carry != nil && !*file.Carry,
	}
	if tier.Manual && number == 0 {
		return Tier{}, refuse("manual", "true; want false, since tier0 is the tier a call with no tier flag runs")
	}
	if strings.TrimSpace(tier.Command) == "" {
		return Tier{}, refuse("command", "missing; want the shell command a run executes")
	}
	if HoldsTargets(tier.Command) && tier.AllTargets == "" {
		return Tier{}, refuse("all_targets", "missing; want the targets of a whole run, since the command holds {targets} or {test_targets}")
	}
	if tier.Filter != "" && !strings.Contains(tier.Command, "{filter}") {
		return Tier{}, refuse("filter", "set, but the command holds no {filter}; want {filter} in the command or no filter")
	}
	if file.TimeoutMin == nil {
		return Tier{}, refuse("timeout_min", "missing; want at least 1")
	}
	if *file.TimeoutMin < 1 {
		return Tier{}, refuse("timeout_min", fmt.Sprintf("%d; want at least 1", *file.TimeoutMin))
	}
	tier.TimeoutMin = *file.TimeoutMin
	holdsTestTimeout := strings.Contains(tier.Command, TestTimeoutPlaceholder)
	if file.TestTimeoutMin != nil {
		if !holdsTestTimeout {
			return Tier{}, refuse("test_timeout_min", "set, but the command holds no "+TestTimeoutPlaceholder+"; want "+TestTimeoutPlaceholder+" in the command or no test_timeout_min")
		}
		if *file.TestTimeoutMin < 1 {
			return Tier{}, refuse("test_timeout_min", fmt.Sprintf("%d; want at least 1", *file.TestTimeoutMin))
		}
	}
	if holdsTestTimeout {
		tier.TestTimeoutMin = DefaultTestTimeoutMin
		if file.TestTimeoutMin != nil {
			tier.TestTimeoutMin = *file.TestTimeoutMin
		}
	}
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
		if covered < 0 || covered >= number {
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
		return fmt.Sprintf("remove tier_parallel.%s with \"lym config gate\"", name)
	}
	return fmt.Sprintf("remove tier_parallel.%s with \"lym config gate\" when the hub file holds it, or in the editor session of \"lym config gate --layer machine\" when the machine file does", name)
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

// useRefusal returns a use-time refusal naming lym config gate and closing with the role clause.
func useRefusal(format string, args ...any) error {
	return fmt.Errorf("gate config: %s; fix it with \"lym config gate\" from the prime%s", fmt.Sprintf(format, args...), roleClause)
}

// WithDefaultDir returns c with dir as the run directory of every tier and of the build step that set none.
// dir is a path relative to the worktree root.
// A gate run fills it with the worktree's anchor.
// A hub whose commands only work in a subdirectory anchor need not set dir per tier.
func (c Config) WithDefaultDir(dir string) Config {
	tiers := make(map[string]Tier, len(c.Tiers))
	for key, tier := range c.Tiers {
		if tier.Dir == "" {
			tier.Dir = dir
		}
		tiers[key] = tier
	}
	c.Tiers = tiers
	if c.Build.Command != "" && c.Build.Dir == "" {
		c.Build.Dir = dir
	}
	return c
}

// Tier returns the declared tier with the given number, or refuses naming lym config gate.
func (c Config) Tier(number int) (Tier, error) {
	tier, ok := c.Tiers[tierKey(number)]
	if !ok {
		return Tier{}, useRefusal("the hub declares no %s", tierKey(number))
	}
	return tier, nil
}

// Gate returns the named gate key's tiers and scope.
// It refuses, naming lym config gate, a hub with no tier0, a missing key, an empty list, or an undeclared tier.
func (c Config) Gate(key string) (GateKey, error) {
	if _, declared := c.Tiers[tierKey(0)]; !declared {
		return GateKey{}, useRefusal("the hub declares no tier0, so no gate has a tier to run")
	}
	gate, ok := c.Gates[key]
	if !ok {
		return GateKey{}, useRefusal("gates holds no key %q", key)
	}
	if len(gate.Tiers) == 0 {
		return GateKey{}, useRefusal("gates.%s is empty, and every gate verifies at least one tier", key)
	}
	for _, number := range gate.Tiers {
		if _, declared := c.Tiers[tierKey(number)]; !declared {
			return GateKey{}, useRefusal("gates.%s lists %s, which the hub does not declare", key, tierKey(number))
		}
	}
	return GateKey{Tiers: slices.Clone(gate.Tiers), Scope: gate.Scope, Lints: slices.Clone(gate.Lints)}, nil
}

// TierList returns the tier numbers the named gate key runs, in order, with the refusals Gate gives.
func (c Config) TierList(gate string) ([]int, error) {
	key, err := c.Gate(gate)
	if err != nil {
		return nil, err
	}
	return key.Tiers, nil
}

// GateCall renders the call a site runs for the named gate key.
// It is `lym gate test --wait --gate <key>`, followed by one `--card <id>` per card id given.
// Its `--wait` form is for a call Go runs, at a verify site or through `internal/cardverify` from a Go verb.
// It refuses, with the refusals Gate gives, a key the config cannot run.
func (c Config) GateCall(key string, cards ...string) (string, error) {
	return c.renderGateCall(key, true, cards)
}

// AgentGateCall renders the call an agent runs for the named gate key.
// It is `lym gate test --gate <key>`, followed by one `--card <id>` per card id given.
// It carries no `--wait`, so an agent strand's call takes the capped form.
// It answers `done: false` with exit 75 when its keep-warm cap passes, instead of blocking one long call.
// It refuses, with the refusals Gate gives, a key the config cannot run.
func (c Config) AgentGateCall(key string, cards ...string) (string, error) {
	return c.renderGateCall(key, false, cards)
}

// renderGateCall renders the gate call for key over cards, with `--wait` when wait is set.
func (c Config) renderGateCall(key string, wait bool, cards []string) (string, error) {
	if _, err := c.Gate(key); err != nil {
		return "", err
	}
	words := []string{"lym", "gate", "test"}
	if wait {
		words = append(words, "--wait")
	}
	words = append(words, "--gate", key)
	for _, card := range cards {
		words = append(words, "--card", card)
	}
	return strings.Join(words, " "), nil
}

// TiersValue renders the value prompts take for the hub's tiers.
// It holds one line per declared tier in number order: `- tier<N>: <about>`.
// Then it holds one line per Go-read gate key the config lists: `- gate <key>: [<n>, ...] <scope>`.
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
		key, ok := c.Gates[gate]
		if !ok {
			continue
		}
		items := make([]string, len(key.Tiers))
		for i, number := range key.Tiers {
			items[i] = strconv.Itoa(number)
		}
		lines = append(lines, fmt.Sprintf("- gate %s: [%s] %s", gate, strings.Join(items, ", "), key.Scope))
	}
	return strings.Join(lines, "\n")
}
