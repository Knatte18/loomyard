// clone_onerepository.go implements the one-repository form of CloneHub.
// It also holds the push that completes a --single-repo clone.
// The one URL is cloned as the hub's prime.
// The board sits on the orphan branch BoltBranchName in a second worktree of it.
// The layout marker is committed at the root of the prime's branch, which is how every later read tells the layout.
// Nothing here pushes during CloneHub: PushOneRepositoryClone does it once the clone's seed records are committed.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// PrimeBranchConfigKey is the git config key that holds the prime's branch.
// It lives in the prime repository of a one-repository hub.
const PrimeBranchConfigKey = "lym.primeBranch"

// cloneOrigin is what the clone of the one URL shows about origin, read once the prime exists.
type cloneOrigin struct {
	// Branch is the branch checked out in the prime, origin's default branch.
	Branch string
	// HasBolt is true when origin carries the BoltBranchName branch.
	HasBolt bool
	// HasBinding is true when the prime's branch holds the records binding file at its root.
	HasBinding bool
	// HasAnchor is true when the prime's branch holds the anchor marker at its root.
	HasAnchor bool
	// hubToken proves that this clone created the hub directory whose prime was read.
	hubToken createdToken
}

// readCloneOrigin reads which branches and root files origin holds from the clone at primePath.
// It reads the clone's remote-tracking refs and the checked-out branch's tree, and makes no network call.
// A prime with no commit holds no root file.
func readCloneOrigin(primePath string) (cloneOrigin, error) {
	branch, err := checkedOutBranch(primePath, "prime")
	if err != nil {
		return cloneOrigin{}, err
	}
	origin := cloneOrigin{Branch: branch}

	repo := gitrepo.New(primePath)
	_, err = repo.RefSHA("refs/remotes/" + originRemoteName + "/" + BoltBranchName)
	switch {
	case err == nil:
		origin.HasBolt = true
	case !errors.Is(err, gitrepo.ErrRefNotFound):
		return cloneOrigin{}, fmt.Errorf("check origin for the %s branch: %w", BoltBranchName, err)
	}

	if _, err := repo.CurrentSHA(); errors.Is(err, gitrepo.ErrNoCommits) {
		return origin, nil
	} else if err != nil {
		return cloneOrigin{}, fmt.Errorf("read the prime's HEAD: %w", err)
	}
	holdsRootFile := func(file string) (bool, error) {
		data, err := showFile(primePath, "HEAD", file)
		if err != nil {
			return false, fmt.Errorf("read %s at the root of %s: %w", file, branch, err)
		}
		return data != nil, nil
	}
	if origin.HasBinding, err = holdsRootFile(WarpBindingFileName); err != nil {
		return cloneOrigin{}, err
	}
	if origin.HasAnchor, err = holdsRootFile(lymcwd.AnchorFileName); err != nil {
		return cloneOrigin{}, err
	}
	return origin, nil
}

// originHoldsBolt reports whether the repository at url carries the BoltBranchName branch.
// A failed read answers false, so an unreachable URL is reported by the step that clones or probes it.
func originHoldsBolt(cwd, url string) bool {
	listed, err := gitexec.Run([]string{"ls-remote", "--heads", filepath.ToSlash(url), "refs/heads/" + BoltBranchName}, cwd)
	return err == nil && strings.TrimSpace(listed) != ""
}

// cloneOneRepositoryHub clones opts.WeftURL as the prime of a one-repository hub at hubPath.
// The hub directory is created exclusively, and every failure after that tears it down.
// It then reads what origin holds and hands the rest of the clone to cloneOneRepository.
func cloneOneRepositoryHub(rec *Mutations, cwd, hubPath, name string, opts CloneOptions) (CloneResult, error) {
	// A fresh one-repository hub needs its shortname, so the refusal comes before anything is created.
	if opts.SingleRepo {
		if _, _, _, err := resolveEffectiveShortname("", false, opts.Shortname, true); err != nil {
			return CloneResult{}, err
		}
	}

	if opts.Reset {
		if err := resetHub(rec, cwd, hubPath); err != nil {
			return CloneResult{}, err
		}
	} else if _, err := os.Stat(hubPath); err == nil {
		return CloneResult{}, fmt.Errorf("hub already exists at %s", hubPath)
	}

	hubTok, err := createExclusiveDir(rec, hubPath)
	if err != nil {
		return CloneResult{}, err
	}

	primePath := filepath.Join(hubPath, name)
	if err := cloneRepo(opts.WeftURL, primePath); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(KindWorktreeCreated, primePath, "")
	if err := refuseUncheckedOutWarpClone(primePath, opts.WeftURL); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	installPostCheckoutHookNonFatal(hubPath, name)

	origin, err := readCloneOrigin(primePath)
	if err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	origin.hubToken = hubTok
	return cloneOneRepository(rec, cwd, hubPath, opts, origin)
}

// cloneOneRepository completes a one-repository clone whose prime is already cloned into hubPath.
// The Bolt checkout is added as a worktree of the prime on BoltBranchName.
// It adopts origin's branch when origin holds it, and is created as an orphan otherwise.
// Over an origin holding the Bolt branch, the prime's branch must hold the anchor marker already.
// With opts.SingleRepo it refuses an origin that holds the Bolt branch or a records binding.
// It then seeds the board with the marker and the shortname, and commits the marker at the root of the prime's branch.
// Both paths record the prime's branch in the prime repository's config under PrimeBranchConfigKey.
// It pushes nothing; PushOneRepositoryClone pushes what a --single-repo clone created.
// Every failure goes through teardownHub, so a refused clone leaves no hub directory.
func cloneOneRepository(rec *Mutations, cwd, hubPath string, opts CloneOptions, origin cloneOrigin) (CloneResult, error) {
	fail := func(err error) (CloneResult, error) {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, origin.hubToken, err)
	}

	const singleRepoWayForward = "run the clone without --single-repo, or with it against a repository holding neither the " + BoltBranchName + " branch nor a records binding"
	switch {
	case opts.SingleRepo && origin.HasBolt:
		return fail(fmt.Errorf("--single-repo refused: origin holds the %s branch, so it already holds a one-repository hub; way forward: %s", BoltBranchName, singleRepoWayForward))
	case opts.SingleRepo && origin.HasBinding:
		return fail(fmt.Errorf("--single-repo refused: origin's %s holds the records binding %s, so it is a records repository; way forward: %s", origin.Branch, WarpBindingFileName, singleRepoWayForward))
	case !opts.SingleRepo && !origin.HasBolt:
		return fail(fmt.Errorf("origin no longer holds the %s branch; way forward: run the clone again", BoltBranchName))
	}

	requestedAnchor, err := lymcwd.ValidateAnchorRel(opts.Subpath)
	if err != nil {
		return fail(err)
	}
	requestedExplicitly := strings.TrimSpace(opts.Subpath) != ""
	primePath := filepath.Join(hubPath, DeriveWarpName(opts.WeftURL))
	boardDir := BoltDir(hubPath)

	if err := ensureBoardWorktree(primePath, BoltBranchName, boardDir); err != nil {
		return fail(err)
	}
	rec.Append(KindWorktreeCreated, boardDir, "")

	// The board's `.lym` is the hub-wide never-tracked tree.
	// It is excluded before the board is ever committed, so a stage-all there never sweeps it up.
	if err := seedDotLymExclude(boardDir); err != nil {
		return fail(err)
	}
	scratchDir := HubScratchDir(hubPath)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fail(err)
	}
	rec.Append(KindDirCreated, scratchDir, "")

	var recordedShortname string
	var shortnameFound bool
	if !opts.SingleRepo {
		raw, readErr := os.ReadFile(filepath.Join(boardDir, ShortnameFileName))
		switch {
		case readErr == nil:
			recordedShortname, shortnameFound = usableShortname(string(raw), "on "+BoltBranchName)
		case !errors.Is(readErr, fs.ErrNotExist):
			return fail(fmt.Errorf("read %s on %s: %w", ShortnameFileName, BoltBranchName, readErr))
		}
	}
	effectiveShortname, writeShortname, shortnameWarning, err := resolveEffectiveShortname(recordedShortname, shortnameFound, opts.Shortname, opts.SingleRepo)
	if err != nil {
		return fail(err)
	}

	var anchor string
	switch {
	case origin.HasAnchor:
		primeMarker, readErr := os.ReadFile(filepath.Join(primePath, lymcwd.AnchorFileName))
		if readErr != nil {
			return fail(fmt.Errorf("read %s at the root of %s: %w", lymcwd.AnchorFileName, origin.Branch, readErr))
		}
		anchor, err = adoptRecordedAnchor(primeMarker, "on "+origin.Branch, requestedAnchor, requestedExplicitly)
		if err != nil {
			return fail(err)
		}
	case !opts.SingleRepo:
		return fail(fmt.Errorf(
			"origin's %s branch holds no %s at its root, so this clone cannot tell the prime is part of a one-repository hub; way forward: commit at the root of origin's %s a %s holding %q, the anchor the %s branch names, then run the clone again",
			origin.Branch, lymcwd.AnchorFileName, origin.Branch, lymcwd.AnchorFileName, boardBranchAnchor(boardDir), BoltBranchName))
	default:
		// Validate that the requested subpath exists before recording it:
		// a typo such as "backedn" fails loudly instead of silently anchoring to a directory that was never there.
		anchor = requestedAnchor
		if info, statErr := os.Stat(filepath.Join(primePath, anchor)); statErr != nil || !info.IsDir() {
			return fail(fmt.Errorf("subpath %q does not exist as a directory in the cloned repository", anchor))
		}
	}

	shortnameRecorded := false
	if writeShortname {
		if err := WriteShortname(boardDir, effectiveShortname); err != nil {
			return fail(err)
		}
		rec.Append(KindFileWritten, filepath.Join(boardDir, ShortnameFileName), "")
		shortnameRecorded = true
	}

	if opts.SingleRepo {
		// The board branch carries the marker before the prime's branch does,
		// so the prime already reads as a one-repository worktree when its own marker commit takes its lock.
		if err := seedOneRepositoryBoard(rec, boardDir, anchor); err != nil {
			return fail(err)
		}
		if _, err := commitAnchorMarker(rec, primePath, anchor, "fabric clone: record anchor"); err != nil {
			return fail(err)
		}
	}

	if _, err := gitexec.Run([]string{"config", PrimeBranchConfigKey, origin.Branch}, primePath); err != nil {
		return fail(fmt.Errorf("record the prime's branch in %s: %w", PrimeBranchConfigKey, err))
	}

	primeCwd := filepath.Join(primePath, anchor)
	l, err := lymcwd.Resolve(primeCwd)
	if err != nil {
		return fail(fmt.Errorf("resolve prime layout at %s: %w", primeCwd, err))
	}
	weftBase := filepath.Join(RecordsWorktree(l), l.AnchorRel)

	regenerateAfterVerb(rec, l, "clone")

	return CloneResult{
		HubPath:           hubPath,
		Anchor:            anchor,
		BoardDir:          boardDir,
		WeftBase:          weftBase,
		PrimeCwd:          primeCwd,
		WarpURL:           opts.WeftURL,
		Shortname:         effectiveShortname,
		ShortnameRecorded: shortnameRecorded,
		Warning:           shortnameWarning,
		OneRepository:     true,
		hubToken:          origin.hubToken,
		primePath:         primePath,
		pushOwed:          opts.SingleRepo,
	}, nil
}

// boardBranchAnchor returns the anchor the marker on the Bolt checkout at boardDir names, or "." when it names none.
func boardBranchAnchor(boardDir string) string {
	data, err := os.ReadFile(filepath.Join(boardDir, lymcwd.AnchorFileName))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "."
	}
	return strings.TrimSpace(string(data))
}

// seedOneRepositoryBoard writes the anchor marker into the orphan board checkout at boardDir.
// It commits the marker with the shortname.
// The commit is the root of the board branch, and it makes the checkout read as a one-repository hub's.
func seedOneRepositoryBoard(rec *Mutations, boardDir, anchor string) error {
	markerPath := filepath.Join(boardDir, lymcwd.AnchorFileName)
	if err := os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", markerPath, err)
	}
	rec.Append(KindFileWritten, markerPath, "")

	sha, committed, err := gitrepo.New(boardDir).StageAndCommit("fabric clone: seed the board", []string{lymcwd.AnchorFileName, ShortnameFileName})
	if err != nil {
		return fmt.Errorf("commit the board's seed in %s: %w", boardDir, err)
	}
	if committed {
		rec.Append(KindCommitCreated, boardDir, sha)
	}
	return nil
}

// PushOneRepositoryClone pushes what a --single-repo clone created, so origin holds the marker and the board branch.
// It pushes the prime's branch with its upstream, carrying the marker and the seed records.
// Only after that lands does it push the board branch with its upstream.
// A clone that created nothing on origin pushes nothing.
// A refused push pushes nothing further and tears the hub down, so running the clone again starts clean.
// The returned error names git's push error and a way forward;
// a branch the remote protects is named as protected, with the way forward of allowing direct pushes to it.
func PushOneRepositoryClone(res CloneResult) error {
	if !res.pushOwed {
		return nil
	}
	rec := NewMutations(res.HubPath)
	cwd := filepath.Dir(res.HubPath)

	primeBranch, err := checkedOutBranch(res.primePath, "prime")
	if err != nil {
		return teardownHub(rec, cwd, res.HubPath, res.hubToken, err)
	}
	if err := gitrepo.New(res.primePath).PushRebaseFree(); err != nil {
		return teardownHub(rec, cwd, res.HubPath, res.hubToken, oneRepositoryPushRefusal(primeBranch, err))
	}
	if err := gitrepo.New(res.BoardDir).PushRebaseFree(); err != nil {
		return teardownHub(rec, cwd, res.HubPath, res.hubToken, oneRepositoryPushRefusal(BoltBranchName, err))
	}
	return nil
}

// oneRepositoryPushRefusal words a refused clone push of branch with its way forward.
func oneRepositoryPushRefusal(branch string, pushErr error) error {
	if ProtectedBranchRefusal(pushErr) {
		return fmt.Errorf("origin refused the push of %s because the branch is protected: %w; way forward: allow direct pushes to %s for the account lym pushes as, then run the clone again", branch, pushErr, branch)
	}
	return fmt.Errorf("origin refused the push of %s: %w; way forward: settle what the error names, then run the clone again", branch, pushErr)
}
