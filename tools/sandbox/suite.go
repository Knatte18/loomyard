// suite.go implements the "sandbox suite", "sandbox mux-suite", "sandbox
// shuttle-suite", "sandbox burler-suite", "sandbox perch-suite", "sandbox
// builder-suite", and "sandbox webster-suite" subcommands: copies one of the
// embedded suite templates (main, mux, shuttle, burler, perch, builder, or
// webster) into the Hub host repo, stamps a lyx binary fingerprint, registers
// the file as a git exclude entry, and launches an interactive Claude session
// to execute it. The seven suites share every mechanic (fingerprinting,
// git-exclude, stale-report cleanup, agent launch, post-session mux teardown)
// via the suiteSpec parameterization of runSuite; only the file name,
// embedded doc body, default instruction, and mux-teardown flag differ.

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
	// hostDirName is the subdirectory under the Hub (lyx-test-HUB) that holds
	// the host repo clone. The Hub layout is <parent>/<hubName>/<hostDirName>.
	hostDirName = "lyx-test"
)

//go:embed SANDBOX-CORE-SUITE.md
var sandboxSuiteMD string

//go:embed SANDBOX-MUX-SUITE.md
var muxSandboxSuiteMD string

//go:embed SANDBOX-SHUTTLE-SUITE.md
var shuttleSandboxSuiteMD string

//go:embed SANDBOX-BURLER-SUITE.md
var burlerSandboxSuiteMD string

//go:embed SANDBOX-PERCH-SUITE.md
var perchSandboxSuiteMD string

//go:embed SANDBOX-BUILDER-SUITE.md
var builderSandboxSuiteMD string

//go:embed SANDBOX-WEBSTER-SUITE.md
var websterSandboxSuiteMD string

// suiteSpec parameterizes runSuite over the seven supported suites (main,
// mux, shuttle, burler, perch, builder, and webster): the file written into
// the Hub host repo, the embedded doc body rendered into it, the default
// prompt handed to claude when the operator supplies no -prompt override, and
// whether the suite boots a live mux substrate that must be torn down after
// the session. Every other mechanic (fingerprinting, git-exclude,
// stale-report cleanup, agent launch) is shared across specs.
type suiteSpec struct {
	// fileName is the name of the suite scheme file written into the Hub host
	// repo at each suite run. It is intentionally kept out of git via
	// .git/info/exclude (see ensureGitExclude).
	fileName string
	// doc is the embedded suite body rendered into fileName, following the
	// binary fingerprint header.
	doc string
	// instruction is the literal prompt string handed to the claude binary as
	// its sole argument when no -prompt override is supplied.
	instruction string
	// muxTeardown marks suites whose scenarios boot a live tmux substrate
	// (lyx mux up). For those, runSuite runs `lyx mux down` in the host repo
	// after the agent session ends, whatever the agent did: an orphaned tmux
	// server holds open handles inside the Hub and blocks the next
	// sandbox-build.cmd -reset.
	muxTeardown bool
}

// mainSuite is the original SANDBOX-CORE-SUITE spec: the general black-box scheme
// exercising the lyx CLI end-to-end.
var mainSuite = suiteSpec{
	fileName:    "SANDBOX-CORE-SUITE.md",
	doc:         sandboxSuiteMD,
	instruction: "Read ./SANDBOX-CORE-SUITE.md and follow the instructions in it exactly.",
}

// muxSuite is the SANDBOX-MUX-SUITE spec: the dedicated scheme exercising the
// mux/tmux lifecycle scenarios split out of the main suite.
var muxSuite = suiteSpec{
	fileName:    "SANDBOX-MUX-SUITE.md",
	doc:         muxSandboxSuiteMD,
	instruction: "Read ./SANDBOX-MUX-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// shuttleSuite is the SANDBOX-SHUTTLE-SUITE spec: the dedicated scheme
// exercising the lyx shuttle black-box agent scenarios.
var shuttleSuite = suiteSpec{
	fileName:    "SANDBOX-SHUTTLE-SUITE.md",
	doc:         shuttleSandboxSuiteMD,
	instruction: "Read ./SANDBOX-SHUTTLE-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// burlerSuite is the SANDBOX-BURLER-SUITE spec: the dedicated scheme
// exercising the lyx burler round-worker black-box agent scenarios.
var burlerSuite = suiteSpec{
	fileName:    "SANDBOX-BURLER-SUITE.md",
	doc:         burlerSandboxSuiteMD,
	instruction: "Read ./SANDBOX-BURLER-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// perchSuite is the SANDBOX-PERCH-SUITE spec: the dedicated scheme
// exercising the lyx perch gate-loop black-box agent scenarios.
var perchSuite = suiteSpec{
	fileName:    "SANDBOX-PERCH-SUITE.md",
	doc:         perchSandboxSuiteMD,
	instruction: "Read ./SANDBOX-PERCH-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// builderSuite is the SANDBOX-BUILDER-SUITE spec: the dedicated scheme
// exercising the lyx builder batch-loop black-box agent scenarios.
var builderSuite = suiteSpec{
	fileName:    "SANDBOX-BUILDER-SUITE.md",
	doc:         builderSandboxSuiteMD,
	instruction: "Read ./SANDBOX-BUILDER-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// websterSuite is the SANDBOX-WEBSTER-SUITE spec: the dedicated scheme
// exercising the lyx webster fork-loop and model-escalation black-box agent
// scenarios.
var websterSuite = suiteSpec{
	fileName:    "SANDBOX-WEBSTER-SUITE.md",
	doc:         websterSandboxSuiteMD,
	instruction: "Read ./SANDBOX-WEBSTER-SUITE.md and follow the instructions in it exactly.",
	muxTeardown: true,
}

// lookPath is a testability seam over exec.LookPath so tests can inject fake
// PATH resolution without modifying the real environment.
var lookPath = exec.LookPath

// isCharDevice reports whether f is attached to a console character device.
// A false result means f is a pipe, regular file, or closed handle -- i.e.
// the launcher was redirected, backgrounded, or detached.
func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveStdio is a testability seam reporting whether the launcher runs
// with both stdin and stdout attached to a console.
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

// launchAgent is a testability seam that runs an interactive claude session
// inside hostRepoDir. It passes instruction as the sole positional argument and
// --dangerously-skip-permissions so the agent needs no per-action confirmation.
// The function inherits the calling process's stdin/stdout/stderr and environment,
// waits for the child to exit, and returns its exit code. A non-zero exit code
// from *exec.ExitError is returned as-is; any other error returns 1.
var launchAgent = func(hostRepoDir, claudePath, instruction string) int {
	// An interactive claude session is only reliable on an attached console;
	// warn (not fail) so a knowingly-detached run can still proceed.
	if !interactiveStdio() {
		fmt.Fprint(os.Stderr, nonInteractiveWarning)
	}
	cmd := exec.Command(claudePath, "--dangerously-skip-permissions", instruction)
	cmd.Dir = hostRepoDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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

// muxDown is a testability seam that tears down the Hub-scoped mux substrate
// after an agent session: it runs `lyx mux down` inside hostRepoDir using the
// already-fingerprinted lyx binary. `mux down` is idempotent (success with no
// session up), so the call is safe regardless of what the agent left behind.
var muxDown = func(hostRepoDir, lyxPath string) error {
	cmd := exec.Command(lyxPath, "mux", "down")
	cmd.Dir = hostRepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lyx mux down: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// binaryInfo holds a snapshot of a binary file's identity at a point in time.
// It is used to stamp the copied suite file with a reproducible fingerprint so
// that the emitted sandbox-report.json (meta.fingerprint) can be traced to the
// exact binary that triggered it.
type binaryInfo struct {
	// Path is the absolute filesystem path to the binary.
	Path string
	// Size is the binary's size in bytes at the time of the snapshot.
	Size int64
	// ModTime is the binary's file-system modification time in UTC.
	ModTime time.Time
	// SHA256 holds the first 12 hex characters of the binary's SHA-256 digest,
	// sufficient to distinguish builds without ballooning the fingerprint block.
	SHA256 string
}

// binaryFingerprint stats and hashes the file at path to produce a binaryInfo
// snapshot. ModTime is normalised to UTC. SHA256 is the first 12 hex characters
// of the full digest. Any OS or IO error is wrapped with the provided path as
// context so callers can report which binary failed.
func binaryFingerprint(path string) (binaryInfo, error) {
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

	// Stream the file through sha256 to avoid loading it into memory all at once;
	// lyx.exe can be several megabytes.
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
	}, nil
}

// header returns a small markdown block that stamps the binary's identity into
// the copied suite file. The same fingerprint is later stamped into
// meta.fingerprint of the emitted sandbox-report.json so a maintainer can
// reproduce the exact build that produced a finding.
func (b binaryInfo) header() string {
	return fmt.Sprintf("## Binary under test\n\n"+
		"- Path: `%s`\n"+
		"- Size: %d bytes\n"+
		"- ModTime: %s\n"+
		"- SHA256 (first 12): `%s`\n",
		b.Path,
		b.Size,
		b.ModTime.Format(time.RFC3339),
		b.SHA256,
	)
}

// renderScheme combines the binary fingerprint header with doc (a suiteSpec's
// embedded body) to produce the full suite file content that the launcher
// writes into the Hub host repo.
func renderScheme(info binaryInfo, doc string) string {
	return info.header() + "\n" + doc
}

// ensureGitExclude idempotently appends entry to <repoDir>/.git/info/exclude.
// It creates the .git/info/ directory and the exclude file when either is
// absent. Existing content is preserved; the entry is only appended when it is
// not already present as a whole line. This keeps the Hub host repo's working
// tree clean without touching its tracked ignore files (.gitignore).
func ensureGitExclude(repoDir, entry string) error {
	infoDir := filepath.Join(repoDir, ".git", "info")
	// Ensure .git/info/ exists; MkdirAll is a no-op when the directory is already
	// there, so this is safe to call unconditionally.
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return fmt.Errorf("create .git/info dir: %w", err)
	}

	excludePath := filepath.Join(infoDir, "exclude")

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .git/info/exclude: %w", err)
	}

	// Check for an exact line match so repeated calls are no-ops.
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimRight(line, "\r") == entry {
			return nil
		}
	}

	// Append the entry on its own line. If the file did not end with a newline
	// (or is empty/new), prepend a newline to avoid merging with the last line.
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

// runSuite executes the "sandbox suite" / "sandbox mux-suite" / "sandbox
// shuttle-suite" / "sandbox burler-suite" / "sandbox perch-suite"
// subcommands. It locates the Hub host repo under parentDir, fingerprints
// the deployed lyx binary, writes a fresh spec.fileName into the host repo
// (overwriting any prior copy), registers it in .git/info/exclude, clears
// any stale sandbox-report.json from a prior run, and starts an interactive
// Claude session with the given instruction string. After the session ends,
// specs flagged muxTeardown get a best-effort `lyx mux down` in the host repo
// so no tmux server outlives the run. It does not fetch the agent's report
// -- that is the separate fetch subcommand (runFetch), run by the operator
// after the session. claudeOverride and promptOverride are optional: when
// empty the function resolves "claude" from PATH and uses spec.instruction.
// spec selects which suite (mainSuite, muxSuite, shuttleSuite, burlerSuite,
// perchSuite, builderSuite, or websterSuite) is run.
func runSuite(parentDir, claudeOverride, promptOverride string, spec suiteSpec) error {
	// Derive the host repo path from the shared hubName const (main.go) and the
	// suite-local hostDirName const; the function relies on those consts rather than
	// the raw cwd primitive or git top-level resolution.
	hostRepoDir := filepath.Join(parentDir, hubName, hostDirName)

	// Guard against a missing Hub so the operator gets a clear, actionable message
	// rather than a confusing downstream file-write failure.
	if _, err := os.Stat(hostRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub host repo not found at %s -- run sandbox-build.cmd first", hostRepoDir)
	} else if err != nil {
		return fmt.Errorf("stat host repo %s: %w", hostRepoDir, err)
	}

	// Resolve lyx via PATH so the fingerprint captures the exact binary the
	// operator has deployed; the binary must be on PATH before running the suite.
	lyxPath, err := lookPath("lyx")
	if err != nil {
		return fmt.Errorf("lyx not found on PATH -- deploy the binary before running the suite: %w", err)
	}

	info, err := binaryFingerprint(lyxPath)
	if err != nil {
		return fmt.Errorf("fingerprint lyx binary: %w", err)
	}

	// Write the rendered scheme (fingerprint header + body) into the host repo,
	// overwriting any copy left from a previous run so every session starts fresh.
	suitePath := filepath.Join(hostRepoDir, spec.fileName)
	if err := os.WriteFile(suitePath, []byte(renderScheme(info, spec.doc)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", spec.fileName, err)
	}

	// Exclude the scheme file from git tracking in the host repo so it does not
	// show up as an untracked change when the agent runs git status or similar.
	if err := ensureGitExclude(hostRepoDir, spec.fileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	// Remove any report left over from a previous session so a fetch run
	// after this session cannot pick up stale findings under a fresh fingerprint;
	// if the agent writes nothing, fetch then correctly surfaces the
	// missing-report error instead.
	reportPath := filepath.Join(hostRepoDir, reportFileName)
	if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", reportFileName, err)
	}

	// Exclude the report from git tracking in the host repo for the same
	// reason as the scheme file above.
	if err := ensureGitExclude(hostRepoDir, reportFileName); err != nil {
		return fmt.Errorf("ensure git exclude: %w", err)
	}

	// Resolve the claude binary: honour an explicit override flag, otherwise
	// search PATH -- the agent must be installed like any other tool.
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

	// Launch the interactive agent session. An interactive claude session never
	// self-terminates, so its manual exit is expected and its non-zero exit code
	// is NORMAL -- it must not be treated as a failure. Fetching the report is a
	// separate step, so print guidance and return nil regardless of the code.
	code := launchAgent(hostRepoDir, claudePath, instruction)
	fmt.Fprintf(os.Stderr,
		"sandbox: agent session ended (exit code %d). Run sandbox-fetch.cmd to collect findings into .scratch.\n",
		code)

	// For suites whose scenarios boot a live mux substrate, tear it down now,
	// regardless of how the agent session ended: an orphaned tmux server holds
	// open handles inside the Hub host repo and blocks the next
	// sandbox-build.cmd -reset. Best-effort -- a teardown failure must not turn
	// a completed session into a launcher error.
	if spec.muxTeardown {
		if err := muxDown(hostRepoDir, lyxPath); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: mux teardown: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: mux substrate torn down (lyx mux down).")
		}
	}
	return nil
}
