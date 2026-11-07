// suite.go implements the sandbox suite subcommands, which run one embedded suite scheme against the Hub warp repo.
// A run writes the scheme file there and stamps a lym binary fingerprint into it.
// It also registers the file as a git exclude entry, then launches an interactive claude session to execute it.
// Every suite shares these mechanics through the suiteSpec parameterization of runSuite.
// A spec varies only its file name, embedded doc body, default instruction, and whether reed teardown runs.

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
	// repoDirName is the subdirectory under the Hub (lym-test-LYMHUB) that holds the repo clone.
	// The Hub layout is <parent>/<hubName>/<repoDirName>.
	repoDirName = "lym-test"
)

//go:embed SANDBOX-CORE-SUITE.md
var sandboxSuiteMD string

//go:embed SANDBOX-REED-SUITE.md
var reedSandboxSuiteMD string

//go:embed SANDBOX-REED-WATCH-SUITE.md
var reedWatchSandboxSuiteMD string

//go:embed SANDBOX-SHUTTLE-SUITE.md
var shuttleSandboxSuiteMD string

//go:embed SANDBOX-CREEL-SUITE.md
var creelSandboxSuiteMD string

//go:embed SANDBOX-BURLER-SUITE.md
var burlerSandboxSuiteMD string

//go:embed SANDBOX-WEBSTER-SUITE.md
var websterSandboxSuiteMD string

//go:embed SANDBOX-FABRIC-SUITE.md
var fabricSandboxSuiteMD string

// suiteSpec describes one suite run: its scheme file, its embedded doc body, and its default prompt.
// The default prompt applies when the operator supplies no -prompt override.
// reedTeardown says whether the suite boots a live reed substrate that runSuite must tear down afterwards.
// Every other mechanic (fingerprinting, git-exclude, stale-report cleanup, agent launch) is shared by all specs.
type suiteSpec struct {
	// fileName is the name of the suite scheme file written into the Hub warp repo at each suite run.
	// It is kept out of git through a .git/info/exclude entry;
	// so git never tracks it.
	fileName string
	// doc is the embedded suite body rendered into fileName, following the
	// binary fingerprint header.
	doc string
	// instruction is the literal prompt string handed to the claude binary as
	// its sole argument when no -prompt override is supplied.
	instruction string
	// reedTeardown marks suites whose scenarios boot a live tmux substrate (lym reed up).
	// runSuite then runs lym reed down after the session, since an orphaned tmux server blocks the next build -reset.
	reedTeardown bool
}

// mainSuite is the SANDBOX-CORE-SUITE spec: the general black-box scheme exercising the lym CLI end-to-end.
var mainSuite = suiteSpec{
	fileName:    "SANDBOX-CORE-SUITE.md",
	doc:         sandboxSuiteMD,
	instruction: "Read ./SANDBOX-CORE-SUITE.md and follow the instructions in it exactly.",
}

// reedSuite is the SANDBOX-REED-SUITE spec: the dedicated scheme exercising the reed/tmux lifecycle scenarios.
var reedSuite = suiteSpec{
	fileName:     "SANDBOX-REED-SUITE.md",
	doc:          reedSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-REED-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// reedWatchSuite is the SANDBOX-REED-WATCH-SUITE spec: a non-destructive scheme that runs one attached reed session.
// reedTeardown stays false here: the reed session must outlive the driving agent's turn,
// so the operator can keep exploring the still-live session by hand.
var reedWatchSuite = suiteSpec{
	fileName:    "SANDBOX-REED-WATCH-SUITE.md",
	doc:         reedWatchSandboxSuiteMD,
	instruction: "Read ./SANDBOX-REED-WATCH-SUITE.md and follow the instructions in it exactly.",
}

// shuttleSuite is the SANDBOX-SHUTTLE-SUITE spec:
// the dedicated scheme exercising the lym shuttle black-box agent scenarios.
var shuttleSuite = suiteSpec{
	fileName:     "SANDBOX-SHUTTLE-SUITE.md",
	doc:          shuttleSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-SHUTTLE-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// creelSuite is the SANDBOX-CREEL-SUITE spec:
// the dedicated scheme exercising the lym creel black-box agent scenarios.
// It sets reedTeardown: its scenarios add reed strands, whose tmux server would block the next build -reset.
var creelSuite = suiteSpec{
	fileName:     "SANDBOX-CREEL-SUITE.md",
	doc:          creelSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-CREEL-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// burlerSuite is the SANDBOX-BURLER-SUITE spec:
// the dedicated scheme exercising the lym burler round-worker black-box agent scenarios.
var burlerSuite = suiteSpec{
	fileName:     "SANDBOX-BURLER-SUITE.md",
	doc:          burlerSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-BURLER-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// websterSuite is the SANDBOX-WEBSTER-SUITE spec:
// the dedicated scheme exercising the lym webster fork-loop black-box agent scenarios.
var websterSuite = suiteSpec{
	fileName:     "SANDBOX-WEBSTER-SUITE.md",
	doc:          websterSandboxSuiteMD,
	instruction:  "Read ./SANDBOX-WEBSTER-SUITE.md and follow the instructions in it exactly.",
	reedTeardown: true,
}

// fabricSuite is the SANDBOX-FABRIC-SUITE spec:
// the dedicated scheme exercising fabric's stricter "main-weft"-suffixed branch-naming behavior.
// It shares the Hub with every other suite and runs fabric's destructive verbs.
// The verbs are clone --reset, prune --apply, and cleanup --apply; a sandbox Hub exists to absorb them.
// sandbox/build.cmd -reset is the recovery path when a run leaves the Hub in a state another suite trips on.
// No live reed/tmux substrate is involved, so reedTeardown stays false.
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

// nonInteractiveWarning is printed before launching the agent when stdin or stdout is not an attached console.
// Without a TTY, an interactive claude session cannot idle between turns: it ends as soon as the model ends a turn.
// A backgrounded command's completion notice never arrives, and the agent may abandon the remaining scenarios.
const nonInteractiveWarning = "sandbox: warning: stdin/stdout is not an attached console; " +
	"the agent session cannot idle for notifications and may end early, abandoning scenarios. " +
	"Run the suite launcher in a real interactive terminal (do not redirect or background it).\n"

// launchAgent runs an interactive claude session inside warpRepoDir.
// It passes instruction as the sole positional argument.
// It also passes --dangerously-skip-permissions to skip per-action confirmation.
// binDir, when non-empty, is prepended to PATH.
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
	// Override the inherited environment only when a dev binary is in play;
	// otherwise the child sees the operator's environment unchanged.
	if binDir != "" {
		cmd.Env = prependPath(binDir, os.Environ())
	}
	if err := cmd.Run(); err != nil {
		// cmd.Run returns *exec.ExitError unwrapped, so a direct type assertion reaches the exit code.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		// Startup failure (binary not found at runtime, permission error, etc.)
		fmt.Fprintf(os.Stderr, "sandbox: launch agent: %v\n", err)
		return 1
	}
	return 0
}

// reedDown is a testability seam that tears down the reed substrate by running `lym reed down` inside warpRepoDir.
var reedDown = func(warpRepoDir, lymPath string) error {
	cmd := exec.Command(lymPath, "reed", "down")
	cmd.Dir = warpRepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lym reed down: %w (output: %s)", err, strings.TrimSpace(string(out)))
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
	// Source records which binary resolveLym picked (sourceDev or sourceProd).
	Source string
}

// binaryFingerprint stats and hashes the file at path to produce a binaryInfo snapshot.
func binaryFingerprint(path, source string) (binaryInfo, error) {
	// Stat first to capture size and modtime before opening the file.
	// The two calls then reflect a consistent view of the inode.
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

// renderScheme combines the binary fingerprint header with doc to produce the full suite file content.
func renderScheme(info binaryInfo, doc string) string {
	return info.header() + "\n" + doc
}

// ensureGitExclude idempotently appends entry to <repoDir>/.git/info/exclude.
// It creates the directory and file if needed, and it preserves existing content.
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

// runSuite executes a sandbox suite.
// It fingerprints the deployed lym binary, writes a fresh suite file, and registers it in .git/info/exclude.
// It then starts an interactive Claude session.
// For specs flagged reedTeardown, it runs `lym reed down` after the session ends.
func runSuite(parentDir, claudeOverride, promptOverride string, spec suiteSpec) error {
	warpRepoDir := filepath.Join(parentDir, hubName, repoDirName)

	if _, err := os.Stat(warpRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub warp repo not found at %s -- run sandbox/build.cmd first", warpRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat warp repo %s: %w", warpRepoDir, err)
	}

	lymPath, source, err := resolveLym()
	if err != nil {
		return err
	}

	info, err := binaryFingerprint(lymPath, source)
	if err != nil {
		return fmt.Errorf("fingerprint lym binary: %w", err)
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
		binDir = filepath.Dir(lymPath)
	}

	code := launchAgent(warpRepoDir, claudePath, instruction, binDir)
	fmt.Fprintf(os.Stderr,
		"sandbox: agent session ended (exit code %d). Run sandbox/fetch.cmd to collect findings into .scratch.\n",
		code)

	if spec.reedTeardown {
		if err := reedDown(warpRepoDir, lymPath); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: reed teardown: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: reed substrate torn down (lym reed down).")
		}
	}
	return nil
}
