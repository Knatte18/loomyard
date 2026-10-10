// cli.go assembles the cobra command tree for the gate module and the argument assembly of `lyx gate test`.

package gatecli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
)

// SlotBusyExit is the exit code `lyx gate test` returns when no slot frees within the hub's `cli_wait_sec`.
// It is EX_TEMPFAIL, so a caller can tell a busy hub from a failing test run.
const SlotBusyExit = 75

// defaultGoBinary is the go command `lyx gate test` runs.
const defaultGoBinary = "go"

// Command builds the cobra command tree for the gate module.
func Command() *cobra.Command {
	return newCommand(defaultGoBinary)
}

// newCommand builds the gate command tree over goBinary, the go command the test verb spawns.
func newCommand(goBinary string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "run go tests inside the hub's gate-slot pool",
		// ArbitraryArgs lets a standalone RunCLI, where this group is the cobra root, reach GroupRunE instead of cobra's own unknown-command error.
		Args: cobra.ArbitraryArgs,
		RunE: clihelp.GroupRunE,
	}

	var dir, tags string
	testCmd := &cobra.Command{
		Use:   "test [-C <dir>] [--tags <tags>] <packages...> [-- <go test flags>]",
		Short: "run go test over packages once a hub gate slot is free",
		Long: `test runs "go test" over the named packages inside one of the hub's gate slots,
so the hub never runs more gate builds and tests at once than its configured slot count allows.
It waits for a free slot for up to the hub's cli_wait_sec, then exits with 75 and names the holders.
Inside a slot already held by an enclosing gate run it runs without acquiring another.
Outside every hub it runs unslotted under the template's -p cap.
The output and exit code are go test's own.

Examples:

  lyx gate test ./internal/foo
  lyx gate test -C backend --tags integration ./internal/foo ./internal/bar
  lyx gate test ./... -- -run TestSomething -count=1`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			request := testRequest{goBinary: goBinary, dir: dir, tags: tags, packages: args}
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				request.packages, request.flags = args[:dash], args[dash:]
			}
			clihelp.SetExit(cmd.Context(), runTest(cmd.Context(), cmd.OutOrStdout(), request))
			return nil
		},
	}
	testCmd.Flags().StringVarP(&dir, "dir", "C", "", "directory to run go test in; defaults to the current directory")
	testCmd.Flags().StringVar(&tags, "tags", "", "comma-separated build tags passed to go test")
	cmd.AddCommand(testCmd)

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
	goBinary string
	// dir is the -C value as typed; empty means the seam cwd.
	dir  string
	tags string
	// packages are the arguments before `--`.
	packages []string
	// flags are the arguments after `--`, passed to go test unchanged.
	flags []string
}

// goTestArgs returns the go arguments that run packages in dir under the -p cap parallel.
func goTestArgs(dir string, parallel int, tags string, packages, flags []string) []string {
	args := []string{"test", "-C", dir, "-p", strconv.Itoa(parallel)}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, packages...)
	return append(args, flags...)
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

// refusePackages is the refusal for a request that names no package.
func refusePackages(out io.Writer) int {
	return output.Err(out, `gate test: name at least one package; way forward: run "lyx gate test <packages...>", for example "lyx gate test ./internal/foo"`)
}
