// suite.go implements the "sandbox suite", "sandbox reed-suite", "sandbox shuttle-suite", "sandbox
// burler-suite", and "sandbox webster-suite"
// subcommands: copies one of the embedded suite templates (main, reed, shuttle, burler,
// or webster) into the Hub warp repo, stamps a lyx binary fingerprint, registers the file
// as a git exclude entry, and launches an interactive Claude session to execute it.
// The five suites share every mechanic (fingerprinting, git-exclude, stale-report cleanup, agent
// launch, post-session reed teardown) via the suiteSpec parameterization of runSuite;
// only the file name, embedded doc body, default instruction, and reed-teardown flag differ.

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Suite-specific constants.
const (
	// warpDirName is the subdirectory under the Hub (lyx-test-HUB) that holds
	// the warp repo clone. The Hub layout is <parent>/<hubName>/<warpDirName>.
	warpDirName = "lyx-test"
)

//go:embed SANDBOX-CORE-SUITE.md
var sandboxSuiteMD string

//go:embed SANDBOX-REED-SUITE.md
var reedSandboxSuiteMD string

//go:embed SANDBOX-SHUTTLE-SUITE.md
var shuttleSandboxSuiteMD string

//go:embed SANDBOX-BURLER-SUITE.md
var burlerSandboxSuiteMD string

//go:embed SANDBOX-WEBSTER-SUITE.md
var websterSandboxSuiteMD string

//go:embed SANDBOX-FABRIC-SUITE.md
var fabricSandboxSuiteMD string

// suiteSpec parameterizes runSuite over the six supported suites (main,
// reed, shuttle, burler, webster, and fabric): the file written into
// the Hub warp repo, the embedded doc body rendered into it, the default
// prompt handed to claude when the operator supplies no -prompt override, and
// whether the suite boots a live reed substrate that must be torn down after
// the session. Every other mechanic (fingerprinting, git-exclude,
// stale-report cleanup, agent launch) is shared across specs.
type suiteSpec struct {
	// fileName is the name of the suite scheme file written into the Hub warp
	// repo at each suite run. It is intentionally kept out of git via
	// .git/info/exclude (see ensureGitExclude).
	fileName string
	// doc is the embedded suite body rendered into fileName, following the
	// binary fingerprint header.
	doc string
	// instruction is the literal prompt string handed to the claude binary as
	// its sole argument when no -prompt override is supplied.
	instruction string
	// reedTeardown marks suites whose scenarios boot a live tmux substrate
	// (lyx reed up). For those, runSuite runs `lyx reed down` in the warp repo
	// after the agent session ends, whatever the agent did: an orphaned tmux
	// server holds open handles inside the Hub and blocks the next
	// sandbox/build.cmd -reset.
	reedTeardown bool
}

// mainSuite is the original SANDBOX-CORE-SUITE spec: the general black-box scheme
// exercising the lyx CLI end-to-end.
var mainSuite = suiteSpec{
	fileName:    "SANDBOX-CORE-SUITE.md",
	doc:         sandboxSuiteMD,
	instruction: "Read ./SANDBOX-CORE-SUITE.md and follow the instructions in it exactly.",
}

// reedSuite is the SANDBOX-REED-SUITE spec: the dedicated scheme exercising the
// reed/tmux lifecycle scenarios split out of the main suite.
var reedSuite = suiteSpec{
	fileName:     "SANDBOX-REED-SUITE.md",
	doc:          reedSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-REED-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// shuttleSuite is the SANDBOX-SHUTTLE-SUITE spec: the dedicated scheme
// exercising the lyx shuttle black-box agent scenarios.
var shuttleSuite = suiteSpec{
	fileName:     "SANDBOX-SHUTTLE-SUITE.md",
	doc:          shuttleSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-SHUTTLE-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// burlerSuite is the SANDBOX-BURLER-SUITE spec: the dedicated scheme
// exercising the lyx burler round-worker black-box agent scenarios.
var burlerSuite = suiteSpec{
	fileName:     "SANDBOX-BURLER-SUITE.md",
	doc:          burlerSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-BURLER-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// websterSuite is the SANDBOX-WEBSTER-SUITE spec: the dedicated scheme
// exercising the lyx webster fork-loop and model-escalation black-box agent
// scenarios.
var websterSuite = suiteSpec{
	fileName:     "SANDBOX-WEBSTER-SUITE.md",
	doc:          websterSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-WEBSTER-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// fabricSuite is the SANDBOX-FABRIC-SUITE spec: the dedicated scheme exercising
// fabric's stricter "main-weft"-suffixed branch-naming behavior. It runs
// against the same shared Hub every other suite uses -- fabric's own
// destructive verbs (clone --reset, prune --apply, cleanup --apply) are
// exactly what a sandbox Hub exists to absorb, and re-running sandbox/build.cmd
// -reset is the recovery path if a run leaves the Hub in a state another
// suite would trip on. No live reed/tmux substrate is involved, so no
// teardown is needed.
var fabricSuite = suiteSpec{
	fileName:    "SANDBOX-FABRIC-SUITE.md",
	doc:         fabricSandboxSuiteMD,
	instruction: "Read ./SANDBOX-FABRIC-SUITE.md and follow the instructions in it exactly.",
}

// lookPath is a testability seam over exec.LookPath.
var lookPath = exec.LookPath

// isCharDevice reports whether f is attached to a console character device.
func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveStdio is a testability seam reporting whether stdin and stdout
// are attached to a console.
var interactiveStdio = func() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

// nonInteractiveWarning is printed before launching the agent when the
// launcher's stdio is not an attached console. An interactive claude session
// without a TTY cannot idle between turns: the process ends as soon as the
// model ends a turn, so a backgrounded command's completion notification is
// never delivered and the agent may abandon the remaining scenarios.
const nonInteractiveWarning = "sandbox: warning: stdin/stdout is not an attached console; " +
	"the agent session cannot idle for notifications and may end early, abandoning scenarios. " +
	"Run the suite launcher in a real interactive terminal (do not redirect or background it).\n"

// launchAgent runs an interactive claude session inside warpRepoDir. It passes
// instruction as the sole positional argument and --dangerously-skip-permissions
// to skip per-action confirmation. binDir (when non-empty) is prepended to PATH.
var launchAgent = func(warpRepoDir, claudePath, instruction, binDir string) int {
	// An interactive claude session is only reliable on an attached console;
	// warn (not fail) so a knowingly-detached run can still proceed.
	if !interactiveStdio() {
		fmt.Fprint(os.Stderr, nonInteractiveWarning)
	}
	cmd := exec.Command(claudePath, "--dangerously-skip-permissions", instruction)
	cmd.Dir = warpRepoDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Only override the inherited environment when a dev binary is in play;
	// otherwise the child sees the operator's own environment unchanged, as before.
	if binDir != "" {
		cmd.Env = prependPath(binDir, os.Environ())
	}
	if err := cmd.Run(); err != nil {
		// exec.ExitError is returned directly by cmd.Run, never wrapped, so a
		// plain two-value type assertion is the idiomatic way to extract the code.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		// Startup failure (binary not found at runtime, permission error, etc.)
		fmt.Fprintf(os.Stderr, "sandbox: launch agent: %v\n", err)
		return 1
	}
	return 0
}

// reedDown is a testability seam that tears down the reed substrate by running
// `lyx reed down` inside warpRepoDir.
var reedDown = func(warpRepoDir, lyxPath string) error {
	cmd := exec.Command(lyxPath, "reed", "down")
	cmd.Dir = warpRepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lyx reed down: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// binaryInfo holds a snapshot of a binary file's identity at a point in time.
type binaryInfo struct {
	// Path is the absolute filesystem path to the binary.
	Path string
	// Size is the binary's size in bytes.
	Size int64
	// ModTime is the binary's modification time in UTC.
	ModTime time.Time
	// SHA256 is the first 12 hex characters of the binary's SHA-256 digest.
	SHA256 string
	// Source records which binary resolveLyx picked (sourceDev or sourceProd).
	Source string
}

// binaryFingerprint stats and hashes the file at path to produce a binaryInfo snapshot.
func binaryFingerprint(path, source string) (binaryInfo, error) {
	// Stat first to capture size and modtime before opening the file, so the
	// two calls reflect a consistent view of the inode.
	fi, err := os.Stat(path)
	if err != nil {
		return binaryInfo{}, fmt.Errorf("stat binary %s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return binaryInfo{}, fmt.Errorf("open binary %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return binaryInfo{}, fmt.Errorf("hash binary %s: %w", path, err)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	return binaryInfo{
		Path:    path,
		Size:    fi.Size(),
		ModTime: fi.ModTime().UTC(),
		SHA256:  digest[:12],
		Source:  source,
	}, nil
}

// header returns a markdown block that stamps the binary's identity.
func (b binaryInfo) header() string {
	return fmt.Sprintf("## Binary under test\n\n"+
		"- Path: `%s`\n"+
		"- Size: %d bytes\n"+
		"- ModTime: %s\n"+
		"- SHA256 (first 12): `%s`\n"+
		"- Source: %s\n",
		b.Path,
		b.Size,
		b.ModTime.Format(time.RFC3339),
		b.SHA256,
		b.Source,
	)
}

// renderScheme combines the binary fingerprint header with doc to produce
// the full suite file content.
func renderScheme(info binaryInfo, doc string) string {
	return info.header() + "\n" + doc
}

// ensureGitExclude idempotently appends entry to <repoDir>/.git/info/exclude,
// creating the directory and file if needed. Existing content is preserved.
func ensureGitExclude(repoDir, entry string) error {
	infoDir := filepath.Join(repoDir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return fmt.Errorf("create .git/info dir: %w", err)
	}

	excludePath := filepath.Join(infoDir, "exclude")

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .git/info/exclude: %w", err)
	}

	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimRight(line, "\r") == entry {
			return nil
		}
	}

	var suffix string
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		suffix = "\n"
	}
	line := suffix + entry + "\n"
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open .git/info/exclude for append: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write .git/info/exclude: %w", err)
	}
	return nil
}

// runSuite executes a sandbox suite: fingerprints the deployed lyx binary,
// writes a fresh suite file, registers it in .git/info/exclude, and starts
// an interactive Claude session. For specs flagged reedTeardown, it runs
// `lyx reed down` after the session ends.
func runSuite(parentDir, claudeOverride, promptOverride string, spec suiteSpec) error {
	warpRepoDir := filepath.Join(parentDir, hubName, warpDirName)

	if _, err := os.Stat(warpRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub warp repo not found at %s -- run sandbox/build.cmd first", warpRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat warp repo %s: %w", warpRepoDir, err)
	}

	lyxPath, source, err := resolveLyx()
	if err != nil {
		return err
	}

	info, err := binaryFingerprint(lyxPath, source)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	suitePath := filepath.Join(warpRepoDir, spec.fileName)
	if err := os.WriteFile(suitePath, []byte(renderScheme(info, spec.doc)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", spec.fileName, err)
	}

	if err := ensureGitExclude(warpRepoDir, spec.fileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	reportPath := filepath.Join(warpRepoDir, reportFileName)
	if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", reportFileName, err)
	}

	if err := ensureGitExclude(warpRepoDir, reportFileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	claudePath := claudeOverride
	if claudePath == "" {
		claudePath, err = lookPath("claude")
		if err != nil {
			return fmt.Errorf("claude not found on PATH: %w", err)
		}
	}

	instruction := promptOverride
	if instruction == "" {
		instruction = spec.instruction
	}

	binDir := ""
	if source == sourceDev {
		binDir = filepath.Dir(lyxPath)
	}

	code := launchAgent(warpRepoDir, claudePath, instruction, binDir)
	fmt.Fprintf(os.Stderr,
		"sandbox: agent session ended (exit code %d). Run sandbox/fetch.cmd to collect findings into .scratch.\n",
		code)

	if spec.reedTeardown {
		if err := reedDown(warpRepoDir, lyxPath); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: reed teardown: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: reed substrate torn down (lyx reed down).")
		}
	}
	return nil
}
