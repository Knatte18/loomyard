// warpprobe.go implements the pre-hub weft probe: a shallow, throwaway clone of the weft candidate
// used to read the recorded warp binding and discriminate a real weft from an accidental warp before
// any hub directory is created.
//
// The hub is named after the warp repo, so in the one-argument clone form there is no hub path yet —
// and therefore no board worktree — until the warp URL is known, and git offers no porcelain that
// reads a single file from a remote without first materializing a local repo directory.
// This file is that materialization: a temporary clone, read, then removed, entirely before CloneHub
// commits to a hub layout.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// warpProbeDirPrefix is the os.MkdirTemp prefix for the probe's throwaway clone directory.
// This is a MkdirTemp prefix, not a _lyx/.lyx path token, so it does not fall under the Lyxdirs
// Single-Declarer Invariant, and TestEnforcement_GeometryLiterals (exact-equality matching) does not
// trip on it.
const warpProbeDirPrefix = ".lyx-clone-probe-"

// warpProbeResult carries what probeWeftBinding learned from the throwaway clone of the weft
// candidate, before any hub directory exists.
type warpProbeResult struct {
	// RecordedWarpURL is the warp URL read from the weft candidate's committed binding, valid only
	// when Found is true.
	RecordedWarpURL string
	// Found is true only when WarpBindingFileName is present in the probe's HEAD commit.
	Found bool
	// WeftLooksLikeWeft is true when the candidate has an unborn HEAD (a genuinely empty weft
	// remote) or its HEAD commit carries lyxcwd.AnchorFileName (or the stale pre-rename marker) at
	// the root.
	WeftLooksLikeWeft bool
}

// probeWeftBinding shallow-clones weftURL into a throwaway directory under cwd, reads whatever
// warp-binding and anchor evidence its HEAD commit carries, then removes the clone before returning.
// It performs, in order: an unborn-HEAD check (a genuinely empty weft remote), a presence check for
// the recorded warp binding, and — only when the binding is absent — a presence check for the anchor
// marker (both the current and the stale pre-rename name) used by CloneHub's old-order bootstrap
// guard.
//
// The probe lives in cwd, never os.TempDir(): cwd is provably writable (the hub is about to be
// created there) and this avoids system-temp variance across platforms.
func probeWeftBinding(cwd, weftURL string) (warpProbeResult, error) {
	probeDir, err := os.MkdirTemp(cwd, warpProbeDirPrefix)
	if err != nil {
		return warpProbeResult{}, fmt.Errorf("create probe directory in %s: %w", cwd, err)
	}
	defer func() { _ = os.RemoveAll(probeDir) }()

	// --depth 1 --filter=tree:0 --no-checkout --single-branch: minimal by intent, but against a
	// local bare repo path (every fixture in this repo's test suite) git ignores --filter and
	// --depth and performs an ordinary hardlinked clone, emitting warnings on stderr. Those
	// warnings are not a failure; only a nonzero exit or a failure to run git at all is.
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{
		"clone", "--depth", "1", "--filter=tree:0", "--no-checkout", "--single-branch",
		filepath.ToSlash(weftURL), filepath.ToSlash(filepath.Base(probeDir)),
	}, cwd)
	_ = stdout
	if err != nil {
		return warpProbeResult{}, wrapProbeError(weftURL, "clone", "", err)
	}
	if exitCode != 0 {
		return warpProbeResult{}, wrapProbeError(weftURL, "clone", stderr, nil)
	}

	// Unborn-HEAD check: a nonzero exit here means the weft candidate has no commits at all, the
	// genuinely empty weft remote that ensureBoardWorktree's orphan-create path already supports.
	_, stderr, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "HEAD"}, probeDir)
	if err != nil {
		return warpProbeResult{}, wrapProbeError(weftURL, "rev-parse HEAD", "", err)
	}
	if exitCode != 0 {
		return warpProbeResult{Found: false, WeftLooksLikeWeft: true}, nil
	}

	bindingPresent, err := probeTreeHasPath(weftURL, probeDir, WarpBindingFileName)
	if err != nil {
		return warpProbeResult{}, err
	}

	if bindingPresent {
		stdout, stderr, exitCode, err = gitexec.RunGit([]string{"show", "HEAD:" + WarpBindingFileName}, probeDir)
		if err != nil {
			return warpProbeResult{}, wrapProbeError(weftURL, "show", "", err)
		}
		if exitCode != 0 {
			return warpProbeResult{}, wrapProbeError(weftURL, "show", stderr, nil)
		}
		recorded := strings.TrimSpace(stdout)
		if recorded != "" {
			return warpProbeResult{RecordedWarpURL: recorded, Found: true}, nil
		}
		// An empty-after-trim value is treated as absent, matching readWarpBinding's own
		// empty-is-absent rule.
	}

	anchorPresent, err := probeTreeHasPath(weftURL, probeDir, lyxcwd.AnchorFileName)
	if err != nil {
		return warpProbeResult{}, err
	}
	if anchorPresent {
		return warpProbeResult{WeftLooksLikeWeft: true}, nil
	}

	// A legacy hub predating the anchor rename carries only the old marker, and it is unambiguously
	// a weft; recognising it here is what keeps clone.go's existing stale-marker hard error — the
	// one that names re-cloning as the migration remedy — reachable. Without this second probe the
	// new guard would fire first and replace that specific, actionable message with a generic
	// argument-order complaint, which would be a real regression for exactly the operator the
	// migration error exists for.
	stalePresent, err := probeTreeHasPath(weftURL, probeDir, staleFabricAnchorName)
	if err != nil {
		return warpProbeResult{}, err
	}
	return warpProbeResult{WeftLooksLikeWeft: stalePresent}, nil
}

// probeTreeHasPath reports whether path is present in probeDir's HEAD commit, using
// `git ls-tree HEAD --name-only -- <path>` per the plan's absence-is-proved-by-ls-tree decision: exit
// 0 with empty stdout means absent, exit 0 with the path echoed means present, and a nonzero exit or a
// failure to run git at all is a hard error.
func probeTreeHasPath(weftURL, probeDir, path string) (bool, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"ls-tree", "HEAD", "--name-only", "--", path}, probeDir)
	if err != nil {
		return false, wrapProbeError(weftURL, "ls-tree", "", err)
	}
	if exitCode != 0 {
		return false, wrapProbeError(weftURL, "ls-tree", stderr, nil)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// wrapProbeError formats a probe-phase failure as "probe weft <url>: <detail>", using git's trimmed
// stderr as the detail when available and falling back to a message naming the failing git
// subcommand (op, e.g. "clone", "ls-tree", "show") otherwise, so an empty-stderr failure still tells
// the operator which git invocation produced it.
func wrapProbeError(weftURL, op, stderr string, cause error) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		if cause != nil {
			detail = cause.Error()
		} else {
			detail = fmt.Sprintf("git %s failed", op)
		}
	}
	return fmt.Errorf("probe weft %s: %s", weftURL, detail)
}
