// suite.go implements the "sandbox suite" subcommand: copies the embedded
// SANDBOX-SUITE template into the Hub host repo, stamps a lyx binary fingerprint,
// registers the file as a git exclude entry, and launches an interactive Claude
// session to execute the scheme.

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

	// suiteFileName is the name of the suite scheme file written into the Hub
	// host repo at each suite run. It is intentionally kept out of git via
	// .git/info/exclude (see ensureGitExclude).
	suiteFileName = "SANDBOX-SUITE.md"

	// defaultInstruction is the literal prompt string handed to the claude
	// binary as its sole argument when no -prompt override is supplied.
	defaultInstruction = "Read ./SANDBOX-SUITE.md and follow the instructions in it exactly."
)

//go:embed SANDBOX-SUITE.md
var sandboxSuiteMD string

// lookPath is a testability seam over exec.LookPath so tests can inject fake
// PATH resolution without modifying the real environment.
var lookPath = exec.LookPath

// launchAgent is a testability seam that runs an interactive claude session
// inside hostRepoDir. It passes instruction as the sole positional argument and
// --dangerously-skip-permissions so the agent needs no per-action confirmation.
// The function inherits the calling process's stdin/stdout/stderr and environment,
// waits for the child to exit, and returns its exit code. A non-zero exit code
// from *exec.ExitError is returned as-is; any other error returns 1.
var launchAgent = func(hostRepoDir, claudePath, instruction string) int {
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

// binaryInfo holds a snapshot of a binary file's identity at a point in time.
// It is used to stamp the SANDBOX-SUITE with a reproducible fingerprint so that
// filed issues can be traced to the exact binary that triggered them.
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
// the copied SANDBOX-SUITE. The agent is instructed to include this block in every
// issue it files so that a maintainer can reproduce the exact build.
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

// renderScheme combines the binary fingerprint header with the embedded
// SANDBOX-SUITE body to produce the full SANDBOX-SUITE.md content that the
// launcher writes into the Hub host repo.
func renderScheme(info binaryInfo) string {
	return info.header() + "\n" + sandboxSuiteMD
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

// runSuite executes the "sandbox suite" subcommand. It locates the Hub host
// repo under parentDir, fingerprints the deployed lyx binary, writes a fresh
// SANDBOX-SUITE.md into the host repo (overwriting any prior copy), registers
// it in .git/info/exclude, and starts an interactive Claude session with the
// given instruction string. claudeOverride and promptOverride are optional: when
// empty the function resolves "claude" from PATH and uses defaultInstruction.
func runSuite(parentDir, claudeOverride, promptOverride string) error {
	// Derive the host repo path from the shared hubName const (main.go) and the
	// suite-local hostDirName const; the function relies on those consts rather than
	// the raw cwd primitive or git top-level resolution.
	hostRepoDir := filepath.Join(parentDir, hubName, hostDirName)

	// Guard against a missing Hub so the operator gets a clear, actionable message
	// rather than a confusing downstream file-write failure.
	if _, err := os.Stat(hostRepoDir); os.IsNotExist(err) {
		return fmt.Errorf("hub host repo not found at %s -- run `sandbox build` first", hostRepoDir)
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
	suitePath := filepath.Join(hostRepoDir, suiteFileName)
	if err := os.WriteFile(suitePath, []byte(renderScheme(info)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", suiteFileName, err)
	}

	// Exclude the scheme file from git tracking in the host repo so it does not
	// show up as an untracked change when the agent runs git status or similar.
	if err := ensureGitExclude(hostRepoDir, suiteFileName); err != nil {
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
		instruction = defaultInstruction
	}

	// Launch the interactive agent session. A non-zero exit code is propagated
	// as an error so `go run` callers observe a failure even though the actual
	// exit code cannot be forwarded through `go run` itself.
	if code := launchAgent(hostRepoDir, claudePath, instruction); code != 0 {
		return fmt.Errorf("claude exited with code %d", code)
	}
	return nil
}
