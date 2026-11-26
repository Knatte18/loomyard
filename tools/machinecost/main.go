// Command machinecost measures the costs a test run is made of on this machine and writes one comparable report.
// With it, two machines, or two folders on one machine, can be compared on one footing.
//
//	go build -o <bin>/machinecost ./tools/machinecost
//	machinecost probe -dir lym=/tmp/lym-probe -dir zz=/tmp/zz-probe -out laptop.json
//	machinecost compare laptop.json desktop.json
//
// Build it with `go build -o` and run the binary: it measures new-binary holds, which a `go run` would add to.
//
// probe measures, in order:
//   - process starts from Go: /bin/true, `git --version` and `lym --help`,
//     each as the median wall per start and the children's user and system time per start;
//   - in each -dir folder, the first and second run of a freshly linked program whose build ID is random,
//     so an endpoint agent has never seen its hash;
//   - in each -dir folder, file operations: create, write, close and delete a small file,
//     stat, mkdir and rmdir, and a 4 KiB write with and without fsync;
//   - a git workload shaped like a fabric test fixture, repeated each round:
//     init a bare repository, clone it, add and commit a file, and push.
//     Each step records its median wall, user and system time, and the git processes it starts.
//
// The report holds a machine descriptor and a load snapshot taken before and after the measurements.
// A figure measured under unknown load compares with nothing.
// -dir takes name=path, with a name that holds no /.
// The name keys that folder's measurements, so two machines compare when their folders share names.
// -lym defaults to the lym on PATH; with neither, the lym starts are skipped and the report says so.
// The git workload runs under a config file of the probe's own.
// It ignores the machine's global and system git config, so each machine runs the same git commands.
//
// probe writes the JSON report to -out and prints the same figures as text.
// The default report file is machinecost-<host>-<UTC time>.json in the current directory.
// compare reads two reports and prints each measurement of both with the ratio of the second to the first.
//
// probe creates folders only inside each -dir folder and in its own temp folder under the lym temp root.
// It deletes every one of them before it exits, also when an interrupt, SIGTERM or hangup ends the run early.
// A process ended any other way, such as by SIGKILL, leaves the folders inside the -dir folders behind.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// Exit codes of the command.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Default counts for the probe's repeated measurements.
const (
	defaultStarts    = 200
	defaultFileOps   = 200
	defaultBinaries  = 3
	defaultGitRounds = 20
)

// lymBinaryName is the command looked up on PATH when -lym is not given.
const lymBinaryName = "lym"

// The default report file name, from the host name and the start time in reportTimeLayout.
const (
	reportNameFormat = "machinecost-%s-%s.json"
	reportTimeLayout = "20060102T150405Z"
)

const usageText = `usage:
  machinecost probe [-dir name=path]... [-lym path] [-starts n] [-fileops n] [-binaries n] [-git-rounds n] [-out file]
  machinecost compare <base.json> <other.json>
`

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	if err := lymtemp.Remove(); err != nil {
		fmt.Fprintf(os.Stderr, "machinecost: %v\n", err)
		code = exitError
	}
	os.Exit(code)
}

// run dispatches a subcommand and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	switch args[0] {
	case "probe":
		return runProbeCommand(args[1:], stdout, stderr)
	case "compare":
		return runCompareCommand(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "machinecost: unknown subcommand %q\n%s", args[0], usageText)
		return exitUsage
	}
}

// runProbeCommand parses probe's flags, runs the probe, writes the JSON report and prints it as text.
func runProbeCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var dirs dirList
	flags.Var(&dirs, "dir", "folder to measure, as name=path; repeatable")
	lymBinary := flags.String("lym", "", "lym binary to time; defaults to the lym on PATH")
	starts := flags.Int("starts", defaultStarts, "process starts per command")
	fileOps := flags.Int("fileops", defaultFileOps, "repetitions of each file operation per folder")
	binaries := flags.Int("binaries", defaultBinaries, "freshly linked programs per folder")
	gitRounds := flags.Int("git-rounds", defaultGitRounds, "rounds of the git workload")
	out := flags.String("out", "", "report file; defaults to "+fmt.Sprintf(reportNameFormat, "<host>", "<UTC time>"))
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "machinecost probe: unexpected argument %q\n", flags.Arg(0))
		return exitUsage
	}
	for _, count := range []int{*starts, *fileOps, *binaries, *gitRounds} {
		if count < 1 {
			fmt.Fprintln(stderr, "machinecost probe: every count must be at least 1")
			return exitUsage
		}
	}
	if *lymBinary == "" {
		if found, err := exec.LookPath(lymBinaryName); err == nil {
			*lymBinary = found
		}
	}

	startedAt := time.Now()
	config := ProbeConfig{
		Dirs:      dirs,
		LymBinary: *lymBinary,
		Starts:    *starts,
		FileOps:   *fileOps,
		Binaries:  *binaries,
		GitRounds: *gitRounds,
	}
	// Catching the signals lets an interrupted probe return and delete its folders.
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	// Catching a hangup that nohup ignores would end a run meant to outlive its terminal.
	if !signal.Ignored(syscall.SIGHUP) {
		signals = append(signals, syscall.SIGHUP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()
	report, err := runProbe(ctx, config, startedAt, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "machinecost probe: %v\n", err)
		return exitError
	}
	reportPath := *out
	if reportPath == "" {
		reportPath = fmt.Sprintf(reportNameFormat, report.Machine.Hostname, startedAt.UTC().Format(reportTimeLayout))
	}
	if err := writeReport(reportPath, report); err != nil {
		fmt.Fprintf(stderr, "machinecost probe: %v\n", err)
		return exitError
	}
	renderReport(stdout, report)
	fmt.Fprintf(stdout, "\nReport: %s\n", reportPath)
	return exitOK
}

// runCompareCommand reads two reports and prints their comparison.
func runCompareCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	base, err := readReport(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "machinecost compare: %v\n", err)
		return exitError
	}
	other, err := readReport(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "machinecost compare: %v\n", err)
		return exitError
	}
	renderComparison(stdout, base, other)
	return exitOK
}

// dirList collects the repeatable -dir flag.
// A folder's name keys its measurements, so a name given twice is refused.
type dirList []NamedDir

func (d *dirList) String() string {
	names := make([]string, 0, len(*d))
	for _, dir := range *d {
		names = append(names, dir.Name+"="+dir.Path)
	}
	return strings.Join(names, ",")
}

func (d *dirList) Set(value string) error {
	name, path, named := strings.Cut(value, "=")
	// A / in the name is a bare path holding =, and would also split the measurement keys.
	if !named || name == "" || path == "" || strings.Contains(name, "/") {
		return errors.New("want name=path, with a name holding no /")
	}
	for _, dir := range *d {
		if dir.Name == name {
			return fmt.Errorf("folder name %q given twice", name)
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	*d = append(*d, NamedDir{Name: name, Path: absolute})
	return nil
}
