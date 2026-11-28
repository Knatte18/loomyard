// recordsprobe.go implements the pre-hub weft probe.
// The probe is a shallow, throwaway clone of the weft candidate;
// it reads the recorded warp binding and tells a real weft from an accidental warp before any hub directory exists.
//
// The hub is named after the warp repo.
// In the one-argument clone form there is therefore no hub path, and no board worktree, until the warp URL is known.
// Git offers no porcelain that reads a single file from a remote without first materializing a local repo directory.
// This file does that: it makes a temporary clone, reads it, and removes it before CloneHub commits to a hub layout.
// It also holds the weft-branch fetch helper that the --reset subpath check shares with the probe.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// warpProbeDirPrefix prefixes the throwaway clone directories of the pre-hub probe and of the --reset subpath check.
// It is not a path token, so the geometry-literal checks do not match it.
const warpProbeDirPrefix = ".lym-clone-probe-"

// warpProbeResult reports what a throwaway clone of the weft candidate shows, before any hub directory exists.
type warpProbeResult struct {
	// RecordedWarpURL is the warp URL read from the weft candidate's committed binding.
	// It is valid only when Found is true.
	RecordedWarpURL string
	// Found is true only when WarpBindingFileName is present in the probe's HEAD commit.
	Found bool
	// WeftLooksLikeWeft is true when the candidate has an unborn HEAD, a genuinely empty weft remote.
	// It is also true when its HEAD commit or main-weft branch carries lymcwd.AnchorFileName at the root,
	// or its HEAD commit carries the superseded marker staleFabricAnchorName at the root.
	WeftLooksLikeWeft bool
	// RecordedShortname is the repo shortname read from the probe's HEAD commit.
	// It is valid only when ShortnameFound is true; a blank or grammar-failing value reports ShortnameFound false.
	RecordedShortname string
	// ShortnameFound is true only when ShortnameFileName is present at the probe's HEAD with a usable value.
	ShortnameFound bool
	// AnchorFound is true when lymcwd.AnchorFileName is present at the probe's HEAD or at the root of its main-weft branch.
	AnchorFound bool
}

// freshBind reports whether the probed weft is one lym has never touched:
// neither the warp binding nor the anchor marker is present, which an unborn or empty weft is.
func (p warpProbeResult) freshBind() bool { return !p.Found && !p.AnchorFound }

// probeWeftBinding shallow-clones weftURL into a throwaway directory under cwd.
// It reads the warp-binding and anchor evidence in the clone's HEAD commit, then removes the clone before returning.
// It checks, in order, for an unborn HEAD (a genuinely empty weft remote) and for the recorded warp binding.
// Only when the binding is absent does it check for the anchor marker and the superseded marker.
//
// The probe lives in cwd, never os.TempDir(): cwd is writable because the hub is about to be created there,
// and this avoids system-temp variance across platforms.
func probeWeftBinding(cwd, weftURL string) (warpProbeResult, error) {
	probeDir, err := os.MkdirTemp(cwd, warpProbeDirPrefix)
	if err != nil {
		return warpProbeResult{}, fmt.Errorf("create probe directory in %s: %w", cwd, err)
	}
	defer func() { _ = os.RemoveAll(probeDir) }()

	// --depth 1 --filter=tree:0 --no-checkout --single-branch asks for a minimal clone.
	// Against a local bare repo path, git ignores --filter and --depth;
	// it performs an ordinary hardlinked clone and emits warnings on stderr.
	// Those warnings are not a failure; only a nonzero exit or a failure to run git at all is.
	_, err = gitexec.Run([]string{
		"clone", "--depth", "1", "--filter=tree:0", "--no-checkout", "--single-branch",
		filepath.ToSlash(weftURL), filepath.ToSlash(filepath.Base(probeDir)),
	}, cwd)
	if err != nil {
		return warpProbeResult{}, wrapProbeError(weftURL, "clone", err)
	}

	// Unborn-HEAD check: ErrNoCommits means the weft candidate has no commits at all, a genuinely empty weft remote.
	// Any other error is a real failure that must be wrapped and propagated.
	_, err = gitrepo.New(probeDir).CurrentSHA()
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return warpProbeResult{Found: false, WeftLooksLikeWeft: true}, nil
	}
	if err != nil {
		return warpProbeResult{}, wrapProbeError(weftURL, "rev-parse HEAD", err)
	}

	shortnamePresent, err := probeTreeHasPath(weftURL, probeDir, ShortnameFileName)
	if err != nil {
		return warpProbeResult{}, err
	}
	var recordedShortname string
	var shortnameFound bool
	if shortnamePresent {
		stdout, err := gitexec.Run([]string{"show", "HEAD:" + ShortnameFileName}, probeDir)
		if err != nil {
			return warpProbeResult{}, wrapProbeError(weftURL, "show", err)
		}
		recordedShortname, shortnameFound = usableShortname(stdout, weftURL)
	}

	anchorPresent, err := probeTreeHasPath(weftURL, probeDir, lymcwd.AnchorFileName)
	if err != nil {
		return warpProbeResult{}, err
	}
	if !anchorPresent {
		anchorPresent, err = probeMainWeftHasPath(weftURL, probeDir, lymcwd.AnchorFileName)
		if err != nil {
			return warpProbeResult{}, err
		}
	}

	bindingPresent, err := probeTreeHasPath(weftURL, probeDir, WarpBindingFileName)
	if err != nil {
		return warpProbeResult{}, err
	}

	if bindingPresent {
		stdout, err := gitexec.Run([]string{"show", "HEAD:" + WarpBindingFileName}, probeDir)
		if err != nil {
			return warpProbeResult{}, wrapProbeError(weftURL, "show", err)
		}
		recorded := strings.TrimSpace(stdout)
		if recorded != "" {
			return warpProbeResult{
				RecordedWarpURL: recorded, Found: true,
				RecordedShortname: recordedShortname, ShortnameFound: shortnameFound, AnchorFound: anchorPresent,
			}, nil
		}
		// A value that is empty after trimming counts as absent, so the probe falls through to the anchor checks.
	}

	if anchorPresent {
		return warpProbeResult{WeftLooksLikeWeft: true, RecordedShortname: recordedShortname, ShortnameFound: shortnameFound, AnchorFound: true}, nil
	}

	// A weft carrying only the superseded anchor marker is unambiguously a weft.
	// Recognising it here keeps the stale-marker error in clone.go reachable.
	// That error names re-cloning as the migration remedy.
	// Without this probe, a generic refusal would fire first and replace that specific, actionable message.
	stalePresent, err := probeTreeHasPath(weftURL, probeDir, staleFabricAnchorName)
	if err != nil {
		return warpProbeResult{}, err
	}
	// The superseded marker also counts as a bound weft, so its migration error is reached, not a fresh-bind refusal.
	return warpProbeResult{WeftLooksLikeWeft: stalePresent, RecordedShortname: recordedShortname, ShortnameFound: shortnameFound, AnchorFound: stalePresent}, nil
}

// probeTreeHasPath reports whether path is present in the HEAD commit of the clone at probeDir.
// It runs `git ls-tree HEAD --name-only -- <path>`: empty output means absent, and a path echoed back means present.
// A nonzero exit or a failure to run git is an error.
func probeTreeHasPath(weftURL, probeDir, path string) (bool, error) {
	stdout, err := gitexec.Run([]string{"ls-tree", "HEAD", "--name-only", "--", path}, probeDir)
	if err != nil {
		return false, wrapProbeError(weftURL, "ls-tree", err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// probeMainWeftHasPath reports whether path is present at the root of the candidate's main-weft branch.
// A hub's prime weft carries its anchor marker on that branch, while the candidate's HEAD may name another.
// It fetches the branch into the probe clone.
// A candidate without the branch answers false, and any other git failure is an error.
func probeMainWeftHasPath(weftURL, probeDir, path string) (bool, error) {
	present, err := fetchWeftBranchTip(weftURL, probeDir, RecordsBranchName("main"))
	if err != nil || !present {
		return false, err
	}
	stdout, err := gitexec.Run([]string{"ls-tree", "FETCH_HEAD", "--name-only", "--", path}, probeDir)
	if err != nil {
		return false, wrapProbeError(weftURL, "ls-tree", err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// fetchWeftBranchTip reports whether origin carries branch.
// When it does, it fetches the tip of branch into FETCH_HEAD of the clone at dir, one commit deep.
// A remote without the branch answers false with nothing fetched, and any git failure is an error.
func fetchWeftBranchTip(weftURL, dir, branch string) (bool, error) {
	branchRef := "refs/heads/" + branch
	listed, err := gitexec.Run([]string{"ls-remote", "--heads", "origin", branchRef}, dir)
	if err != nil {
		return false, wrapProbeError(weftURL, "ls-remote", err)
	}
	if strings.TrimSpace(listed) == "" {
		return false, nil
	}
	if _, err := gitexec.Run([]string{"fetch", "--depth", "1", "origin", branchRef}, dir); err != nil {
		return false, wrapProbeError(weftURL, "fetch", err)
	}
	return true, nil
}

// wrapProbeError formats a probe-phase failure as "probe weft <url>: <detail>".
// The detail is cause's message when cause is non-nil.
// Callers pass a non-nil cause, so the nil arm is unreachable today.
// It is kept for a future caller that reaches this helper without an error.
// Without the arm, op would be a parameter with no reader.
func wrapProbeError(weftURL, op string, cause error) error {
	if cause != nil {
		return fmt.Errorf("probe weft %s: %w", weftURL, cause)
	}
	return fmt.Errorf("probe weft %s: git %s failed", weftURL, op)
}
