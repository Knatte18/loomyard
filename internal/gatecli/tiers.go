// tiers.go plans the tier runs of one `lym gate test` call from the loaded gate config and the request, and renders a tier's command.

package gatecli

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// The scope forms a tier run is labelled with in its summary line and run record.
const (
	scopeTargets = "targets"
	scopeAll     = "all"
	scopeGuard   = "guard"
	scopeVet     = "vet"
)

// roleClause closes every refusal that sends the caller to `lym config gate`: the steps need the prime, so any other session reports and the orch takes them.
const roleClause = "; a session that is not the prime takes none of these steps; it reports status: FAILED and the orch runs them"

// invocation is one run of a tier's command: the directory it runs in and the words its {targets} takes.
type invocation struct {
	// dir is the run directory, relative to the worktree root unless absolute.
	dir     string
	targets []string
	// terms are the class filter terms of an invocation the dotnet scoper narrowed to one selected project; nil for every other invocation.
	terms []string
	// partial marks an invocation narrowed to one selected project of a dotnet share, which runs part of its tier and so is never recorded as a whole pass.
	partial bool
}

// tierRun is one tier a call runs: the tier, the label of its scope and the invocations of its command.
// The guard step is the last tierRun of a plan, and its targets are filled when the guard scan has run.
type tierRun struct {
	number int
	tier   gateslot.Tier
	// scopeForm labels the run in its summary line and record.
	scopeForm   string
	invocations []invocation
	// args are the words that fill the command's {args}.
	args []string
	// ignoresTargets is set when explicit targets were given and the command holds no {targets}, so it runs whole.
	ignoresTargets bool
	// guardStep marks the run of a tier's command over the guard tests of the files that belong to that tier.
	guardStep bool
	// vetStep marks the run of `go vet` under a tier's build tag over a --card scope's packages.
	vetStep bool
	// buildOnly marks a shared_build tier whose share selected no test class and so has no invocation: it takes its slots to run the build step and runs no test command.
	buildOnly bool
}

// tierName returns the gate.yaml key and the label of tier number.
func tierName(number int) string {
	return "tier" + strconv.Itoa(number)
}

// planTierRuns builds the run list of request over cfg and the scope sc without touching the disk:
// the listed tiers ascending with the tiers another listed tier covers dropped, each over its own share of the scope, which a diff scope derives per base, then, for a --card scope under the go scoper, one vet run per tier of sc.tagTiers that no listed tier is or covers, ascending, then one guard run per tier that sc.guards holds guard tests for, when --guards or the scope's form asks for the guard step, ascending and leaving out a tier the listed tiers already ran whole.
// A guard run is a placeholder whose packages and names the run fills from sc.guards.
// A covered tier is named on request.stderr when it has one.
// A tier a no-test-class scope runs whole is named on request.stderr when it has one.
// It refuses, naming `lym config gate`, a hub with no tier0, an undeclared tier, a dangling covers entry, words after `--` for a tier whose command lacks {args}, a guard group whose tier is undeclared or whose command lacks {targets} or {args}, and --guards under the none scoper.
// It refuses --guards under the dotnet scoper too, naming only dropping --guards, since no config change gives a C# hub the guard tests.
func planTierRuns(cfg gateslot.Config, request testRequest, sc scope) ([]tierRun, error) {
	if _, err := cfg.Tier(0); err != nil {
		return nil, fmt.Errorf("gate test: %w", err)
	}
	numbers := listedTiers(cfg, request)
	tiers := make(map[int]gateslot.Tier, len(numbers))
	for _, number := range numbers {
		tier, err := cfg.Tier(number)
		if err != nil {
			return nil, fmt.Errorf("gate test: %w", err)
		}
		tiers[number] = tier
		for _, covered := range tier.Covers {
			if _, err := cfg.Tier(covered); err != nil {
				return nil, fmt.Errorf("gate test: %s covers %s: %w", tierName(number), tierName(covered), err)
			}
		}
	}

	kept := dropCovered(numbers, tiers, request)

	runs := make([]tierRun, 0, len(kept)+1)
	for _, number := range kept {
		tier := tiers[number]
		if len(request.args) > 0 && !strings.Contains(tier.Command, "{args}") {
			return nil, configRefusal("words after \"--\" were given, but the command of %s holds no {args}", tierName(number))
		}
		share := sc.forTier(number)
		if share.noTests && !tier.SharedBuild {
			printLine(request.stderr, fmt.Sprintf("gate test: the scope selected no test class, so %s runs whole", tierName(number)))
		}
		invocations, ignoresTargets := scopeInvocations(tier, request, share)
		runs = append(runs, tierRun{
			number:         number,
			tier:           tier,
			scopeForm:      sc.form,
			invocations:    invocations,
			args:           request.args,
			ignoresTargets: ignoresTargets,
			buildOnly:      tier.SharedBuild && share.noTests && len(invocations) == 0,
		})
	}

	if sc.form == scopeCard && cfg.Scoper == "go" {
		for _, number := range sc.tagTiers {
			if slices.Contains(numbers, number) || slices.ContainsFunc(kept, func(listed int) bool { return slices.Contains(tiers[listed].Covers, number) }) {
				continue
			}
			run, err := vetStepRun(cfg, number, request.goBinary, sc.modules)
			if err != nil {
				return nil, err
			}
			runs = append(runs, run)
		}
	}

	if !request.guards && !sc.guardStep {
		return runs, nil
	}
	if cfg.Scoper == "dotnet" {
		return nil, errors.New("gate test: --guards needs the go scoper to find the guard tests, but gate.yaml's scoper is \"dotnet\"; way forward: drop --guards, then re-run the same command")
	}
	if cfg.Scoper != "go" {
		return nil, configRefusal("--guards needs the go scoper to find the guard tests, but gate.yaml's scoper is %q; drop --guards, or set scoper to go", cfg.Scoper)
	}

	groupTiers := make([]int, 0, len(sc.guards))
	for number, dirs := range sc.guards {
		if len(dirs) > 0 {
			groupTiers = append(groupTiers, number)
		}
	}
	sort.Ints(groupTiers)
	for _, number := range groupTiers {
		if ranWhole(runs, number, sc) {
			continue
		}
		tier, declared := cfg.Tiers[tierName(number)]
		if !declared {
			return nil, configRefusal("the guard tests of %s run under its command, but the hub declares no %s", tierName(number), tierName(number))
		}
		if !strings.Contains(tier.Command, "{targets}") || !strings.Contains(tier.Command, "{args}") {
			return nil, configRefusal("the guard step runs the command of %s, which must hold {targets} and {args}", tierName(number))
		}
		dir := tier.Dir
		switch {
		case sc.guardDir != "":
			dir = sc.guardDir
		case request.dir != "":
			dir = request.dir
		}
		runs = append(runs, tierRun{
			number:      number,
			tier:        tier,
			scopeForm:   scopeGuard,
			invocations: []invocation{{dir: dir}},
			guardStep:   true,
		})
	}
	return runs, nil
}

// scopeInvocations returns the invocations one tier makes over sc, and whether its command ignores the scope's narrowing.
// Explicit targets run in the -C directory, else the tier's dir; a whole scope runs the tier's all_targets, split at spaces, in the tier's dir.
// A narrower scope runs the tier once per module in that module's directory, and the empty scope makes none; a dotnet share's module is a selected project, whose invocation carries its class filter terms and is partial.
// A scope that selected no test class makes no invocation for a tier with shared_build, and runs any other tier whole.
// A tier with carry: false runs whole under every scope form but explicit targets, the empty scope included.
// A command without {targets} runs once, whole, under explicit targets and under a non-empty narrower scope.
func scopeInvocations(tier gateslot.Tier, request testRequest, sc scope) (invocations []invocation, ignoresTargets bool) {
	runsWhole := []invocation{{dir: tier.Dir, targets: strings.Fields(tier.AllTargets)}}
	switch {
	case sc.form == scopeTargets:
		dir := tier.Dir
		if request.dir != "" {
			dir = request.dir
		}
		return []invocation{{dir: dir, targets: request.targets}}, !strings.Contains(tier.Command, "{targets}")
	case sc.whole, tier.NoCarry:
		return runsWhole, false
	case sc.noTests && tier.SharedBuild:
		return nil, false
	case sc.noTests:
		return runsWhole, false
	case len(sc.modules) == 0:
		return nil, false
	case !strings.Contains(tier.Command, "{targets}"):
		return runsWhole, true
	}
	for _, module := range sc.modules {
		invocations = append(invocations, invocation{dir: module.dir, targets: module.targets, terms: module.terms, partial: len(module.terms) > 0})
	}
	return invocations, false
}

// vetStepRun returns the vet run of the tier numbered number over modules: `<goBinary> vet -tags tierN` over each module's packages in that module's directory, under the tier's env, parallel and timeout_min, holding one slot.
// A goBinary the platform shell cannot quote is refused.
func vetStepRun(cfg gateslot.Config, number int, goBinary string, modules []moduleTargets) (tierRun, error) {
	tier, err := cfg.Tier(number)
	if err != nil {
		return tierRun{}, fmt.Errorf("gate test: %w", err)
	}
	quoted, err := quoteAll([]string{goBinary})
	if err != nil {
		return tierRun{}, err
	}
	invocations := make([]invocation, len(modules))
	for i, module := range modules {
		invocations[i] = invocation{dir: module.dir, targets: module.targets}
	}
	return tierRun{
		number: number,
		tier: gateslot.Tier{
			Command:    quoted[0] + " vet -tags " + tierName(number) + " {targets}",
			Env:        tier.Env,
			Parallel:   tier.Parallel,
			TimeoutMin: tier.TimeoutMin,
			Counter:    counterGoJSON,
			Slots:      1,
		},
		scopeForm:   scopeVet,
		invocations: invocations,
		vetStep:     true,
	}, nil
}

// configRefusal returns a refusal that sends the caller to `lym config gate` and closes with the role clause.
func configRefusal(format string, args ...any) error {
	return fmt.Errorf("gate test: %s; way forward: fix it with \"lym config gate\" from the prime%s", fmt.Sprintf(format, args...), roleClause)
}

// listedTiers returns the tier numbers request names, ascending and without repeats: the --tier values and, with --upto N, every tier cfg declares numbered 0 through N that is not manual.
// Each manual tier --upto skips and --tier does not name is named on request.stderr when it has one.
// With --gate it returns the tiers of that key, ascending and without repeats, and with neither flag it returns tier0.
func listedTiers(cfg gateslot.Config, request testRequest) []int {
	if request.gate != "" {
		numbers := slices.Clone(cfg.Gates[request.gate].Tiers)
		sort.Ints(numbers)
		return slices.Compact(numbers)
	}
	if len(request.tiers) == 0 && !request.uptoGiven {
		return []int{0}
	}
	numbers := slices.Clone(request.tiers)
	if request.uptoGiven {
		var declared []int
		for key := range cfg.Tiers {
			if number, _ := strconv.Atoi(strings.TrimPrefix(key, "tier")); number <= request.upto {
				declared = append(declared, number)
			}
		}
		sort.Ints(declared)
		for _, number := range declared {
			switch {
			case !cfg.Tiers[tierName(number)].Manual:
				numbers = append(numbers, number)
			case !slices.Contains(request.tiers, number) && request.stderr != nil:
				fmt.Fprintf(request.stderr, "gate test: %s is manual and runs only when --tier names it\n", tierName(number))
			}
		}
	}
	sort.Ints(numbers)
	return slices.Compact(numbers)
}

// dropCovered returns the tier numbers of numbers that no higher kept tier covers, ascending, naming each dropped tier on request.stderr.
func dropCovered(numbers []int, tiers map[int]gateslot.Tier, request testRequest) []int {
	var kept []int
	for i := len(numbers) - 1; i >= 0; i-- {
		number := numbers[i]
		coveredBy := slices.IndexFunc(kept, func(higher int) bool { return slices.Contains(tiers[higher].Covers, number) })
		if coveredBy < 0 {
			kept = append(kept, number)
			continue
		}
		if request.stderr != nil {
			fmt.Fprintf(request.stderr, "gate test: %s is covered by %s and not run separately\n", tierName(number), tierName(kept[coveredBy]))
		}
	}
	slices.Reverse(kept)
	return kept
}

// ranWhole reports whether a run of runs is the tier number or a tier that covers it, over a whole share of sc.
func ranWhole(runs []tierRun, number int, sc scope) bool {
	return slices.ContainsFunc(runs, func(run tierRun) bool {
		return (run.number == number || slices.Contains(run.tier.Covers, number)) && sc.forTier(run.number).whole
	})
}

// renderTierCommand fills {targets}, {args}, {parallel} and {filter} in the tier's command, each target and each arg quoted for the platform shell.
// number is the tier's number and terms are the class filter terms of the invocation; {filter} renders as filterArgument says.
// A placeholder the command does not hold is not added, so terms given to a command without {filter} filter nothing.
func renderTierCommand(number int, tier gateslot.Tier, targets, terms, args []string) (string, error) {
	quotedTargets, err := quoteAll(targets)
	if err != nil {
		return "", err
	}
	quotedArgs, err := quoteAll(args)
	if err != nil {
		return "", err
	}
	filter := ""
	if strings.Contains(tier.Command, "{filter}") {
		if filter, err = filterArgument(runtime.GOOS, number, tier, terms); err != nil {
			return "", err
		}
	}
	return strings.NewReplacer(
		"{targets}", strings.Join(quotedTargets, " "),
		"{args}", strings.Join(quotedArgs, " "),
		"{parallel}", strconv.Itoa(tier.Parallel),
		"{filter}", filter,
	).Replace(tier.Command), nil
}

// filterArgument returns the `--filter` argument for the shell of goos that a tier's {filter} placeholder renders to, or "" when there is no expression.
// With no terms the expression is the tier's filter.
// With terms it is the tier's filter, when it has one, ANDed with the terms ORed together.
// Terms that push the expression past greenCommandBudget are dropped, and the tier's filter stands alone, so the project runs whole within itself.
func filterArgument(goos string, number int, tier gateslot.Tier, terms []string) (string, error) {
	expression := tier.Filter
	if len(terms) > 0 {
		alternatives := strings.Join(terms, "|")
		composed := alternatives
		if tier.Filter != "" {
			composed = "(" + tier.Filter + ")&(" + alternatives + ")"
		}
		if len(composed) > greenCommandBudget {
			logger.Info("gate test: the class filter exceeds the command budget; the project runs without it", "tier", tierName(number), "terms", len(terms))
		} else {
			expression = composed
		}
	}
	if expression == "" {
		return "", nil
	}
	quoted, err := verifyrun.QuoteArg(goos, expression)
	if err != nil {
		return "", fmt.Errorf("gate test: the filter of %s cannot be quoted for the shell: %w; way forward: fix the tier's filter with \"lym config gate\" from the prime%s", tierName(number), err, roleClause)
	}
	return "--filter " + quoted, nil
}

// quoteAll quotes each word as one literal shell argument.
func quoteAll(words []string) ([]string, error) {
	quoted := make([]string, len(words))
	for i, word := range words {
		var err error
		if quoted[i], err = verifyrun.QuoteArg(runtime.GOOS, word); err != nil {
			return nil, fmt.Errorf("gate test: quote %q for the shell: %w; way forward: pass a value without a percent sign", word, err)
		}
	}
	return quoted, nil
}

// renderTierEnv returns the tier's env as KEY=VALUE entries in key order, {parallel} filled.
func renderTierEnv(tier gateslot.Tier) []string {
	keys := make([]string, 0, len(tier.Env))
	for key := range tier.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parallel := strconv.Itoa(tier.Parallel)
	entries := make([]string, len(keys))
	for i, key := range keys {
		entries[i] = key + "=" + strings.ReplaceAll(tier.Env[key], "{parallel}", parallel)
	}
	return entries
}

// trimpathLine returns the line that tells a test author the tier builds with `-trimpath`, or "" when the GOFLAGS that env gives a child carries no enabled `-trimpath`.
// A child sees the last GOFLAGS entry of env, so that entry alone decides.
// Under `-trimpath` `runtime.Caller` reports module-relative file paths, and the line names the way forward of locating the repository from the working directory or `go.mod`.
func trimpathLine(tier string, env []string) string {
	goFlags := ""
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "GOFLAGS="); ok {
			goFlags = value
		}
	}
	// go keeps the last occurrence of a repeated flag, so the last trimpath field decides.
	enabled := false
	for _, field := range strings.Fields(goFlags) {
		name, value, hasValue := strings.Cut(strings.TrimLeft(field, "-"), "=")
		if name != "trimpath" {
			continue
		}
		if !hasValue {
			enabled = true
			continue
		}
		parsed, err := strconv.ParseBool(value)
		enabled = err == nil && parsed
	}
	if !enabled {
		return ""
	}
	return fmt.Sprintf("gate test: %s builds with GOFLAGS=%q, and -trimpath makes runtime.Caller report module-relative file paths; a test locates its repository from its working directory or go.mod, never from a runtime.Caller path alone", tier, goFlags)
}

// notRunTiers returns the tiers cfg declares that no listed run of plan is or covers, ascending.
// A guard or vet run does not count, since it runs a tier's guard tests or compiles its files without running the tier.
func notRunTiers(cfg gateslot.Config, plan []tierRun) []int {
	var numbers []int
	for key := range cfg.Tiers {
		number, _ := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		ran := slices.ContainsFunc(plan, func(run tierRun) bool {
			return !run.guardStep && !run.vetStep && (run.number == number || slices.Contains(run.tier.Covers, number))
		})
		if !ran {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	return numbers
}
