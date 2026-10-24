// cli.go assembles the cobra command tree for the gate module, the flags of `lyx gate test` and the refusal for a failed guard scan.

package gatecli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// SlotBusyExit is the exit code `lyx gate test` returns when no slot frees within the keep-cache-warm cap.
// It is EX_TEMPFAIL and no failure: the caller runs the same command again, and can tell a busy hub from a failing test run.
const SlotBusyExit = 75

// defaultGoBinary is the go command `lyx gate test` builds `lyx` with.
const defaultGoBinary = "go"

// defaultPreTierCeiling bounds the work before a call's first tier.
const defaultPreTierCeiling = 10 * time.Minute

// Command builds the cobra command tree for the gate module.
func Command() *cobra.Command {
	return newCommand(defaultGoBinary, defaultSamplePeriod, shuttleengine.DefaultAwaitCap, defaultPreTierCeiling, time.Minute)
}

// newCommand builds the gate command tree over goBinary, the go command a prebuilding tier builds `lyx` with, samplePeriod, how often the CPU watchdog samples a tier's process tree, slotWait, the longest the test verb waits for a free slot without --wait, preTierCeiling, the bound on the work before the first tier, and timeoutUnit, the length of one `timeout_min` minute.
func newCommand(goBinary string, samplePeriod, slotWait, preTierCeiling, timeoutUnit time.Duration) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "run the hub's test tiers inside the hub's gate-slot pool",
		// ArbitraryArgs lets a standalone RunCLI, where this group is the cobra root, reach GroupRunE instead of cobra's own unknown-command error.
		Args: cobra.ArbitraryArgs,
		RunE: clihelp.GroupRunE,
	}

	var dir, tags, base, gate string
	var tiers []int
	var upto int
	var cards []string
	var all, guards, wait bool
	testCmd := &cobra.Command{
		Use:         "test [<targets>...]",
		Short:       "run the hub's test tiers once their gate slots are free",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: fmt.Sprintf(`test runs the tiers the hub declares in gate.yaml, in ascending tier number, going on past a failed one.
--gate K runs the tiers and the scope the gate key K of gate.yaml names, and goes with none of --tier, --upto, --all and --base.
A key of scope card needs --card <id> per card, scope all runs every tier whole, and scope diff runs the diff from the merge base with the pair's parent branch.
Explicit <targets> beside --gate replace the key's scope.
--tier N names a tier and may repeat; the default is tier0 without --upto or --gate.
--upto N adds every declared tier numbered 0 through N but a manual one, which runs only when --tier names it.
Each tier takes the gate slots it asks for and runs its command through the platform shell, bounded by its timeout_min.
Without --wait it waits for the slots for up to %s, under the prompt-cache lifetime.
When they do not free, it prints ok with slot_free false naming the holders and exits %d, which is no failure:
run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls.
With --wait it blocks until the slots free.
Inside a slot already held by an enclosing gate run it runs without acquiring another.
Each tier prints a summary line to stderr and writes its record and log under the gate directory.
The exit code is the first failing run's own, or 0.
Without --gate the scope is the explicit <targets>, --all, --card <id>, --base <rev>, or the diff from the merge base with the pair's parent branch.
Arguments after "--" fill each tier's {args}.
--guards, and a narrower scope under the go scoper, also run the //lyx:guard tests outside the scope.
-C names the run directory, with explicit targets only.

Examples:

  lyx gate test --wait ./internal/foo
  lyx gate test --tier 0 --tier 1 --all
  lyx gate test --wait --gate finalize
  lyx gate test --wait --gate card --card 3
  lyx gate test --upto 2
  lyx gate test --base main
  lyx gate test ./... -- -run TestSomething -count=1`, slotWait, SlotBusyExit),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			request := testRequest{
				goBinary: goBinary, samplePeriod: samplePeriod, slotWait: slotWait, preTierCeiling: preTierCeiling, timeoutUnit: timeoutUnit,
				stderr: cmd.ErrOrStderr(), dir: dir, tags: tags, gate: gate, tiers: tiers, cards: cards, base: base, all: all, guards: guards, wait: wait, targets: args,
				upto: upto, uptoGiven: cmd.Flags().Changed("upto"),
			}
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				request.targets, request.args = args[:dash], args[dash:]
			}
			clihelp.SetExit(cmd.Context(), runTest(cmd.Context(), cmd.OutOrStdout(), request))
			return nil
		},
	}
	testCmd.Flags().StringVarP(&dir, "dir", "C", "", "directory the tiers run in, with explicit targets only; defaults to each tier's own dir")
	testCmd.Flags().StringVar(&tags, "tags", "", "refused: name the tier that compiles the tags with --tier")
	testCmd.Flags().StringVar(&gate, "gate", "", "gate.yaml key whose tiers and scope the call runs; goes with none of --tier, --upto, --all and --base")
	testCmd.Flags().IntSliceVar(&tiers, "tier", nil, "tier number to run; repeatable, default 0")
	testCmd.Flags().IntVar(&upto, "upto", 0, "run every declared tier numbered 0 through N but the manual ones")
	testCmd.Flags().StringArrayVar(&cards, "card", nil, "plan card id whose packages are the scope; repeatable")
	testCmd.Flags().StringVar(&base, "base", "", "revision whose diff to the working tree is the scope")
	testCmd.Flags().BoolVar(&all, "all", false, "run every tier over its all_targets")
	testCmd.Flags().BoolVar(&guards, "guards", false, "after the listed tiers, run every //lyx:guard test of the module under the tier its file belongs to")
	testCmd.Flags().BoolVar(&wait, "wait", false, "block until the tiers' slots free instead of answering slot_free false after the keep-cache-warm cap")
	cmd.AddCommand(testCmd)
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(reportCmd())
	cmd.AddCommand(cardVerifyCmd())

	return cmd
}

// RunCLI is the public seam for the gate module, delegating to clihelp.Execute for in-process test capture.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// RunCLIIn is RunCLI with cwd as the working directory the verb resolves against instead of the process's.
// An empty cwd delegates to RunCLI, since lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return RunCLI(out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// testRequest is one `lyx gate test` invocation after flag parsing.
type testRequest struct {
	// goBinary is the go command a prebuilding tier builds `lyx` with.
	goBinary string
	// samplePeriod is how often the CPU watchdog samples a tier's process tree.
	samplePeriod time.Duration
	// slotWait is the longest a run without wait waits for free slots.
	slotWait time.Duration
	// preTierCeiling bounds the work before the first tier.
	preTierCeiling time.Duration
	// timeoutUnit is the length of one `timeout_min` minute.
	timeoutUnit time.Duration
	// stderr receives the lines the verb prints beside its output: the CPU watchdog's quit, a tier's timeout and each tier's summary.
	stderr io.Writer
	// dir is the -C value as typed; empty means the tier's own dir.
	dir  string
	tags string
	// gate is the --gate key: its tiers are the listed tiers and its scope picks the scope form; empty for a call with manual flags.
	gate string
	// tiers are the --tier values as typed; empty means tier0 unless uptoGiven.
	tiers []int
	// upto is the --upto value, read only when uptoGiven.
	upto      int
	uptoGiven bool
	// cards are the --card values.
	cards []string
	// base is the --base revision.
	base string
	// all is the --all flag: every listed tier runs over its all_targets.
	all bool
	// guards is the --guards flag: run the guard step after the listed tiers.
	guards bool
	// wait is the --wait flag: block until the slots free.
	wait bool
	// targets are the arguments before `--`.
	targets []string
	// args are the arguments after `--`, which fill each tier's {args}.
	args []string

	// The fields below are filled by runTest once the hub is resolved and the config loaded, and read by the tier runs.

	// root is the worktree root the tiers' dirs and a prebuild are relative to.
	root string
	// prebuiltStore is the hub's store of prebuilt `lyx` binaries; empty outside a hub.
	prebuiltStore string
	// busyQuit is the CPU watchdog's busy window; 0 turns it off.
	busyQuit time.Duration
	// watching is whether the CPU watchdog applies to this run.
	watching bool
}

// absoluteDir returns the directory a request runs in: dir made absolute against the seam cwd, or the seam cwd itself when dir is empty.
// It fails when a named directory does not exist.
func absoluteDir(cwd, dir string) (string, error) {
	if dir == "" {
		return cwd, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	dir = filepath.Clean(dir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("gate test: -C directory %q does not exist; way forward: pass an existing directory to -C", dir)
	}
	return dir, nil
}

// guardScanRefusal is the refusal text for a failed module guard scan of moduleDir.
// A guard-scan error names its file, and its line when it has one, with the way forward its kind gives; any other error names the module directory.
func guardScanRefusal(moduleDir string, err error) string {
	var scanErr *impactset.GuardScanError
	if !errors.As(err, &scanErr) {
		return fmt.Sprintf("gate test: the guard step cannot list the packages of %s: %v; way forward: make the directory readable or fix the module until \"go list ./...\" succeeds there, then re-run the same command", moduleDir, err)
	}
	location := scanErr.File
	if scanErr.Line != 0 {
		location += ":" + strconv.Itoa(scanErr.Line)
	}
	switch scanErr.Kind {
	case impactset.GuardMisplaced:
		return fmt.Sprintf("gate test: the guard step found a //lyx:guard marker at %s that is not directly above a top-level func Test line; way forward: move the marker directly above its func Test line, then re-run the same command", location)
	case impactset.GuardWrongTier:
		return fmt.Sprintf("gate test: the guard step found a //lyx:guard marker at %s in a tier2 or tier3 file; way forward: move the marked test into a tier0 or tier1 file, then re-run the same command", location)
	default:
		return fmt.Sprintf("gate test: the guard step cannot parse %s: %v; way forward: make the file readable or fix it until it parses, then re-run the same command", location, err)
	}
}
