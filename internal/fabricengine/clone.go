// clone.go implements the clone orchestration logic with strict-abort teardown.
//
// After the weft clone succeeds, the weft primary is checked out onto its WeftBranchName-suffixed
// pairing (e.g. "main-weft" for a default branch "main") so weft:main is never claimed directly —
// every fabric-managed weft branch, including the primary's, carries the uniform "-weft" suffix.
// The suffixed branch is adopted from an existing origin/<branch>-weft when the remote already
// carries one (a re-clone of a hub with weft history) and created fresh only otherwise;
// the freshly-cloned default branch itself remains, unclaimed.
//
// Against a genuinely empty weft remote the fresh branch would be UNBORN — `git checkout -b` writes
// no ref on an unborn HEAD — so bornWeftPrimaryBranch lands an initialising empty commit on it.
// Without that, the hub came out of clone with its weft primary on a branch that did not exist, and
// every pair-creating verb forked from it failed.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// staleFabricAnchorName is the pre-rename lyx-anchor marker filename, aliased from its single
// declarer in internal/lyxcwd so clone's guard and lyxcwd's own read-time guard can never drift
// apart.
// It has no compatibility fallback read (lyxcwd.AnchorFileName does not read it);
// it is named here only so CloneHub can detect an old clone's leftover marker and hard-error rather
// than silently re-anchoring at the wrong subpath.
const staleFabricAnchorName = lyxcwd.StaleAnchorFileName

// CloneOptions carries CloneHub's parameters as named fields rather than positionals.
// This is a struct and not five positionals because two adjacent optional URL strings are exactly the
// shape that produces silent argument-order bugs, and the argument order is what this change flips:
// the weft URL is now required and first, the warp URL optional and resolved from the recorded
// binding when empty.
type CloneOptions struct {
	// WeftURL is the weft repository URL. Required.
	WeftURL string
	// WarpURL is the warp repository URL. Optional: when empty, it is resolved from the warp
	// binding recorded on the weft candidate.
	WarpURL string
	// Subpath is the lyx-anchor subpath to resolve; see CloneHub's anchor resolution step.
	Subpath string
	// Reset tears down an existing hub before cloning, for an idempotent re-clone.
	Reset bool
	// ForceBootstrap bypasses the old-order guard that refuses to bootstrap a warp-shaped
	// repository as a weft. It is ignored outside the bootstrap path (the two-argument form
	// writing a fresh binding): the guard is reachable only there, so ForceBootstrap has no effect
	// anywhere else.
	ForceBootstrap bool
}

// CloneResult carries the resolved geometry CloneHub hands back to the caller once the git-level
// clone, board-worktree materialization, and anchor resolution are done.
// It is deliberately git/geometry-only — the CLI layer (internal/fabriccli) drives config
// materialization, weft:main commit, and junction wiring from these fields, because those calls
// route through internal/configsync, which fabricengine must never import (see the fabricengine →
// configsync → configreg → fabricengine cycle documented in this file's clone-does-everything batch
// scope).
// It also embeds MutationRecord, which carries the mutation record accumulated over the call.
type CloneResult struct {
	MutationRecord
	HubPath  string // HubPath is the created <name>-HUB container directory.
	Anchor   string // Anchor is the resolved lyx-anchor subpath (e.g. "backend" or ".").
	BoardDir string // BoardDir is the package-level BoardDir(HubPath) result, the weft:main checkout.
	WeftBase string // WeftBase is the weft-side directory paired with PrimeCwd.
	PrimeCwd string // PrimeCwd is the resolved prime warp worktree path at Anchor.
	// WarpURL is the effective warp URL actually cloned, whether supplied in opts or derived from
	// the recorded binding.
	WarpURL string
	// WarpBindingRecorded is true only when this clone wrote the .lyx-warp record (a fresh binding,
	// including the clone-time backfill of a pre-binding hub).
	WarpBindingRecorded bool
}

// CloneHub orchestrates the cloning of warp and weft repositories, then
// materializes <Hub>/_board as a second weft worktree, into a Hub directory.
// It clones + checks out + materializes the board worktree + resolves/records
// the lyx-anchor subpath, and returns the resolved geometry for the CLI layer
// to drive config materialization and wiring — CloneHub deliberately does NOT
// do that itself, to avoid the fabricengine → configsync import cycle.
//
// It takes cwd (current working directory) and opts (see CloneOptions).
// opts.WeftURL is required; opts.WarpURL is optional and, when empty, is resolved from the warp
// binding recorded on the weft candidate. It returns the resolved CloneResult and any error
// encountered.
//
// The two forms order their early steps differently, because the hub name is derived from the warp
// URL and the warp URL's availability differs between them:
//
//   - Two-argument form (opts.WarpURL != ""): the hub name is derivable with no network at all, so
//     the hub-exists check (and any --reset teardown) runs offline, exactly as before this change,
//     and only then does the pre-hub probe of opts.WeftURL run to resolve the effective warp
//     binding and check the old-order guard.
//   - One-argument form (opts.WarpURL == ""): the hub name is unknowable until the binding is read,
//     so the pre-hub probe of opts.WeftURL runs first, and the hub name, the --reset teardown, and
//     the hub-exists check all follow it.
//
// After that resolution, the operation proceeds exactly as before this change:
//  1. Create the Hub directory; if it fails, return the wrapped error (no teardown yet).
//  2. Clone warp repo to <Hub>/<name>; on failure, teardown and return the error.
//  3. Clone weft repo to <Hub>/<name>-weft; on failure, teardown and return the error.
//     3b. Read the weft primary's checked-out branch and check out its
//     WeftBranchName-suffixed pairing at the same HEAD, capturing the warp
//     branch name it read; on failure, teardown and return the error.
//  4. Materialize <Hub>/_board as a second weft worktree via ensureBoardWorktree,
//     adopted onto the captured warp branch if it already exists locally from
//     step 3's clone, freshly orphan-created otherwise; on failure, teardown
//     and return the error.
//  5. Resolve the lyx-anchor subpath adopt-or-create (adopt: read the marker
//     already committed on weft:main; create: validate subpath exists in the
//     warp worktree, then write the marker to the board worktree on disk —
//     the CLI commits it), write the warp-binding record when it is new, and
//     return the resolved CloneResult.
//
// Any clone OR worktree-add failure triggers teardownHub, which removes the
// entire Hub directory; if removal also fails, the error mentions both the
// original failure and the residual Hub path.
func CloneHub(cwd string, opts CloneOptions) (res CloneResult, err error) {
	var rec *Mutations
	defer func() { res.Mutations = rec.Snapshot() }()

	// Normalize cwd to an absolute path
	cwd = filepath.Clean(cwd)

	if opts.WeftURL == "" {
		return CloneResult{}, fmt.Errorf("weft URL is required")
	}

	// Validate the requested subpath before anything is created or fetched, so a structurally
	// impossible anchor never reaches the point where teardown is the only way out. An absolute or
	// escaping subpath used to pass the later "does it exist in the cloned warp" probe, because
	// filepath.Join swallows a leading separator and ".." lands on a directory that certainly
	// exists.
	requestedAnchor, err := lyxcwd.ValidateAnchorRel(opts.Subpath)
	if err != nil {
		return CloneResult{}, err
	}
	subpathRequestedExplicitly := strings.TrimSpace(opts.Subpath) != ""

	var name, hubPath, effective string
	var writeRecord, derivedFromRecord bool

	if opts.WarpURL != "" {
		// Two-argument form: the hub name is derivable with no network at all, so resolve it,
		// apply --reset, and check for an existing hub before ever touching the network.
		name = DeriveWarpName(opts.WarpURL)
		if name == "" {
			return CloneResult{}, fmt.Errorf("could not derive repo name from warp URL %s", opts.WarpURL)
		}
		hubPath = HubPath(cwd, name)
		rec = NewMutations(hubPath)
		if opts.Reset {
			if err := resetHub(rec, cwd, hubPath); err != nil {
				return CloneResult{}, err
			}
		}
		if _, err := os.Stat(hubPath); err == nil {
			return CloneResult{}, fmt.Errorf("hub already exists at %s", hubPath)
		}

		// Only now, with the offline checks passed, does the pre-hub probe touch the network.
		probe, err := probeWeftBinding(cwd, opts.WeftURL)
		if err != nil {
			return CloneResult{}, err
		}
		effective, writeRecord, err = resolveEffectiveWarpURL(probe.RecordedWarpURL, probe.Found, opts.WarpURL)
		if err != nil {
			return CloneResult{}, err
		}
		if writeRecord && !probe.WeftLooksLikeWeft && !opts.ForceBootstrap {
			// The guard fires only on the bootstrap path (writeRecord == true), which is reachable
			// only in this two-argument form, so ForceBootstrap is structurally ignored everywhere
			// else — no usage error, no warning.
			return CloneResult{}, fmt.Errorf(
				"refusing to bootstrap %s as a weft: its history carries neither %s nor an empty tree — check the argument order, clone now takes <weft-url> [<warp-url>]",
				opts.WeftURL, lyxcwd.AnchorFileName)
		}
	} else {
		// One-argument form: the hub name is unknowable until the binding is read, so the probe
		// runs first and the offline checks follow it.
		probe, err := probeWeftBinding(cwd, opts.WeftURL)
		if err != nil {
			return CloneResult{}, err
		}
		effective, writeRecord, err = resolveEffectiveWarpURL(probe.RecordedWarpURL, probe.Found, "")
		if err != nil {
			// resolveEffectiveWarpURL's own message must not attempt to name the weft URL; the
			// caller prefixes it here.
			return CloneResult{}, fmt.Errorf("weft %s has no recorded warp binding; supply the warp URL explicitly: lyx fabric clone <weft-url> <warp-url>", opts.WeftURL)
		}
		derivedFromRecord = true
		name = DeriveWarpName(effective)
		if name == "" {
			return CloneResult{}, fmt.Errorf("could not derive repo name from warp URL %s recorded in the %s binding on weft:main", effective, WarpBindingFileName)
		}
		hubPath = HubPath(cwd, name)
		rec = NewMutations(hubPath)
		if opts.Reset {
			if err := resetHub(rec, cwd, hubPath); err != nil {
				return CloneResult{}, err
			}
		}
		// The asymmetry with the two-argument form is deliberate: an offline two-argument re-clone
		// against an existing hub still fails with "hub already exists", exactly as today; an
		// offline one-argument invocation fails with "probe weft <url>:" instead, which is the
		// irreducible cost of deriving the hub name from a remote fact.
		if _, err := os.Stat(hubPath); err == nil {
			return CloneResult{}, fmt.Errorf("hub already exists at %s", hubPath)
		}
	}

	warpURL := effective

	// Create Hub directory exclusively, minting the token teardownHub needs to prove — rather than
	// merely assume — that a later teardown call is removing a directory this invocation created.
	// os.Mkdir's EEXIST is now the safety property, and the two offline "hub already exists" stat
	// guards above are UX and ordering only, not the last word on collision safety: this call staying
	// after both stat guards, rather than folding into either, is deliberate (see this function's doc
	// comment for why moving it would either leak a residual hub or break the offline-before-network
	// ordering). Being later, the exclusive create is also strictly more correct than the stat guards
	// it follows — it closes a real time-of-check-to-time-of-use window that exists between a stat and
	// a plain os.MkdirAll, in which a concurrent process can create the hub in between and have
	// MkdirAll silently accept it.
	hubTok, err := createExclusiveDir(rec, hubPath)
	if err != nil {
		return CloneResult{}, err
	}

	// <hub>/.lyx is a fabric-recognised hub-level geometry element the way <hub>/_board
	// already is. It stays a real directory and never a junction — the hub itself is not
	// a git repo, so there is nothing to exclude and no weft to point at — and it is
	// reserved (hubSlugReservedNames, junctionnames.go) so no worktree slug can claim
	// the name. Created here, right beside the hub directory itself, so it exists for
	// the lifetime of every hub this function successfully produces; a creation failure
	// here returns directly rather than through teardownHub, matching the surrounding
	// step-4 posture — the hub directory it would need to tear down was itself just
	// created and holds nothing yet worth cleaning up specially.
	dotLyxPath := filepath.Join(hubPath, lyxdirs.DotLyxDirName)
	if err := os.MkdirAll(dotLyxPath, 0o755); err != nil {
		return CloneResult{}, err
	}
	rec.Append(KindDirCreated, dotLyxPath, "")

	// Step 5: Clone warp repo
	warpWorktreePath := filepath.Join(hubPath, name)
	if err := cloneRepo(warpURL, warpWorktreePath); err != nil {
		if derivedFromRecord {
			// The derive path names its source so a failure here is traceable back to the
			// binding that produced it, not just the bare URL.
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("clone warp %s (from the %s binding on weft:main): %w", warpURL, WarpBindingFileName, err))
		}
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	// A clone genuinely brings a worktree into being, and one entry covers the whole cloned tree —
	// the coarsest-covering-root rule, mirroring cloneRepo's own single-call-site posture.
	rec.Append(KindWorktreeCreated, warpWorktreePath, "")

	// Install the post-checkout hook after the warp worktree exists so drift
	// warnings fire on every subsequent git checkout within this repo.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// the clone (the hook is belt-and-suspenders for usability, not correctness).
	//
	// hubPath and name are already in scope from steps 2 and 1; a direct
	// struct construction is a simplification, not a correctness fix, since
	// InstallPostCheckoutHook reads exactly one field (WorktreePath()) — it
	// needs a path, not a resolution. Step 3 above aborts the clone if the hub
	// already exists and step 4 creates it fresh, so at this point the hub is
	// provably empty, <hubPath>/_board cannot exist, and a Resolve call here
	// would always have succeeded — there is no failure path left to log.
	hookLocation := &lyxcwd.Location{HubPath: hubPath, WorktreeName: name}
	if hookErr := InstallPostCheckoutHook(hookLocation); hookErr != nil {
		logger.Warn("fabricengine: post-checkout hook install failed (non-fatal)", "verb", "clone", "hub", hubPath, "error", hookErr)
	}

	// Step 6: Clone weft repo
	weftPath := weftname.SiblingPath(hubPath, name)
	if err := cloneRepo(opts.WeftURL, weftPath); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(KindWorktreeCreated, weftPath, "")

	// Step 6b: Rename the weft primary's freshly-cloned branch onto its
	// WeftBranchName-suffixed pairing, so weft:<branch> is never claimed
	// directly under fabric's uniform branch scheme. Capture warpBranch (the
	// branch read before the rename) so step 7's _board worktree-add can reuse
	// it directly, rather than re-reading git branch --show-current at weftPath
	// after the rename (which would incorrectly see the suffixed branch).
	warpBranch, err := suffixWeftPrimaryBranch(weftPath)
	if err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}

	// Step 7: Materialize <Hub>/_board as a second weft worktree, checked out
	// on warpBranch — adopted if that branch already exists locally from
	// step 6's clone, freshly orphan-created otherwise (a genuinely empty
	// weft remote).
	boardDir := BoardDir(hubPath)
	if err := ensureBoardWorktree(weftPath, warpBranch, boardDir); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(KindWorktreeCreated, boardDir, "")

	// Step 8: Resolve the lyx-anchor subpath adopt-or-create, and write the
	// marker to the board worktree ON DISK. The CLI layer commits it onto
	// weft:main (config materialization and the commit both live in
	// internal/fabriccli to avoid the fabricengine → configsync import cycle).
	//
	// A leftover pre-rename .fabric-anchor with no .lyx-anchor beside it is a
	// hard error, not a silent fallback: without this check, an old clone
	// would fall through to the create path below and re-anchor at "."
	// (or the requested --subpath) even though a real anchor was already
	// recorded under the old name, silently resolving _lyx to the wrong place
	// for a subpath-anchored repo.
	if _, statErr := os.Stat(filepath.Join(boardDir, staleFabricAnchorName)); statErr == nil {
		if _, newErr := os.Stat(filepath.Join(boardDir, lyxcwd.AnchorFileName)); os.IsNotExist(newErr) {
			// The remedy names the marker rename, never "re-clone": this error is emitted BY a
			// clone, so telling the operator to clone again just reproduces it. The record lives on
			// weft:main, so migrating it is a rename plus a commit in an existing hub's board
			// worktree, after which this clone succeeds.
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf(
				"found stale %s marker with no %s beside it at %s; in an existing hub's %s worktree run `git mv %s %s` and commit, then retry this clone",
				staleFabricAnchorName, lyxcwd.AnchorFileName, boardDir,
				BoardDirName, staleFabricAnchorName, lyxcwd.AnchorFileName))
		}
	}

	markerPath := filepath.Join(boardDir, lyxcwd.AnchorFileName)
	var anchor string
	if data, statErr := os.ReadFile(markerPath); statErr == nil {
		// Adopt path: ensureBoardWorktree checked out weft:main, which already
		// carries a committed marker from a prior clone — this is a re-clone.
		// The recorded value is validated too, not trusted: an older binary could have recorded an
		// absolute or escaping anchor, and adopting one produces a hub whose every weft commit
		// fails.
		recorded, recordedErr := lyxcwd.ValidateAnchorRel(strings.TrimSpace(string(data)))
		if recordedErr != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("recorded anchor in %s on weft:main is unusable: %w", lyxcwd.AnchorFileName, recordedErr))
		}
		if subpathRequestedExplicitly && requestedAnchor != recorded {
			// An explicitly requested subpath disagrees with the recorded anchor: never silently
			// re-anchor, the record is authoritative.
			// "." is not exempted. It used to be, because the CLI's own flag default was "." and the
			// two cases were indistinguishable — so `--subpath .` against a hub recorded at a real
			// subpath was silently adopted, the one value that escaped the never-silently-re-anchor
			// rule. The flag now defaults to the empty string, which normalises to "." with
			// subpathRequestedExplicitly false, so an explicit "." can be honoured as explicit here.
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf(
				"requested --subpath %q does not match the recorded anchor %q for this hub", requestedAnchor, recorded))
		}
		anchor = recorded
	} else {
		// Create path: first-ever clone of this weft, no marker committed yet.
		// Validate the requested subpath exists in the warp worktree before
		// recording it, so a typo like "backedn" fails loudly instead of
		// silently anchoring to a directory that was never there.
		anchor = requestedAnchor
		if info, statErr := os.Stat(filepath.Join(warpWorktreePath, anchor)); statErr != nil || !info.IsDir() {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("subpath %q does not exist as a directory in the cloned warp repo", anchor))
		}
		if err := os.WriteFile(markerPath, []byte(anchor+"\n"), 0o644); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("write %s: %w", markerPath, err))
		}
		// Create branch only: the adopt branch above found a marker already committed and wrote
		// nothing, so recording there would claim a write that never happened.
		rec.Append(KindFileWritten, markerPath, "")
	}

	// Immediately after the anchor block writes .lyx-anchor (both the adopt and the create branch
	// fall through to this point), write the warp-binding record when this clone is the one that
	// determined it. This is the clone-time backfill too: a re-clone of a pre-binding hub has
	// .lyx-anchor already committed but no .lyx-warp, so found is false, writeRecord is true, and
	// the record is written here with no special casing.
	var warpBindingRecorded bool
	if writeRecord {
		if err := writeWarpBinding(boardDir, effective); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
		rec.Append(KindFileWritten, filepath.Join(boardDir, WarpBindingFileName), "")
		warpBindingRecorded = true
	}

	// Resolve the prime layout now that the marker exists on disk, so
	// RelPath — and therefore WeftBase — reflects the resolved anchor.
	primeCwd := filepath.Join(warpWorktreePath, anchor)
	l, err := lyxcwd.Resolve(primeCwd)
	if err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("resolve prime layout at %s: %w", primeCwd, err))
	}

	// Wire the operator-convenience _board junction as a named special case,
	// the same point pathspec junctions are wired at the CLI layer (see
	// internal/fabriccli/clone.go) — but here directly, since _board needs no
	// fabric.yaml load and CloneHub must not import configsync.
	if err := wireBoardLink(rec, l, filepath.Base(warpWorktreePath)); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("wire board junction: %w", err))
	}

	weftBase := filepath.Join(WeftWorktree(l), l.AnchorRel)

	return CloneResult{
		HubPath:             hubPath,
		Anchor:              anchor,
		BoardDir:            boardDir,
		WeftBase:            weftBase,
		PrimeCwd:            primeCwd,
		WarpURL:             warpURL,
		WarpBindingRecorded: warpBindingRecorded,
	}, nil
}

// suffixWeftPrimaryBranch reads the branch checked out at weftPath (the weft
// primary, immediately after clone) and checks out its WeftBranchName-suffixed
// pairing, adopt-or-create style. When origin already carries the suffixed
// branch — a re-clone (fresh machine, `clone --reset`) of a hub that has synced
// weft history — that remote branch is adopted as a tracking local branch, so
// the fresh hub inherits the accumulated weft state (its _lyx content) and its
// first push can rebase-recover through the configured upstream instead of
// diverging permanently from an untracked fork. Only when the remote has no
// suffixed branch yet (a genuinely new hub) is the branch created fresh at the
// current HEAD. Returns an error if the weft primary is on a detached HEAD (no
// branch to read) or if any git call fails.
//
// Returns the warp branch name it read (before the rename) so CloneHub's
// _board-worktree-add step can reuse it directly — re-reading
// git branch --show-current at weftPath after this function returns would
// incorrectly see the already-renamed <warpBranch>-weft, not warpBranch.
func suffixWeftPrimaryBranch(weftPath string) (warpBranch string, err error) {
	stdout, err := gitexec.Run([]string{"branch", "--show-current"}, weftPath)
	if err != nil {
		return "", fmt.Errorf("resolve weft primary branch: %w", err)
	}
	warpBranch = strings.TrimSpace(stdout)
	if warpBranch == "" {
		return "", fmt.Errorf("weft primary at %s is on a detached HEAD after clone; cannot derive its weft branch", weftPath)
	}

	suffixedBranch := WeftBranchName(warpBranch)

	// Adopt path: the remote already carries the suffixed branch (this is a
	// re-clone of a hub with existing weft history). Starting the local branch
	// from origin/<suffixed> both checks out that history and configures the
	// upstream (git's default branch.autoSetupMerge for a remote-tracking start
	// point), which the create path below deliberately leaves to the first push.
	// This is a mixed probe: the exit path answers "no remote suffixed branch
	// yet", so it is recovered via errors.As rather than merged into a single
	// message.
	remoteRef := "refs/remotes/origin/" + suffixedBranch
	_, err = gitexec.Run([]string{"rev-parse", "--verify", "--quiet", remoteRef}, weftPath)
	remoteBranchExists := err == nil
	if err != nil {
		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			return "", fmt.Errorf("check for remote weft primary branch: %w", err)
		}
	}
	checkoutArgs := []string{"checkout", "-b", suffixedBranch}
	if remoteBranchExists {
		checkoutArgs = append(checkoutArgs, "origin/"+suffixedBranch)
	}

	if _, err := gitexec.Run(checkoutArgs, weftPath); err != nil {
		return "", fmt.Errorf("checkout -b %q in weft primary: %w", suffixedBranch, err)
	}

	if err := bornWeftPrimaryBranch(weftPath, suffixedBranch); err != nil {
		return "", err
	}
	return warpBranch, nil
}

// bornWeftPrimaryBranch gives the weft primary's suffixed branch a real commit when the clone left
// it UNBORN, so refs/heads/<suffixed> resolves the moment CloneHub returns.
//
// Against a genuinely empty weft remote — the documented first-ever-setup path probeWeftBinding's
// unborn-HEAD check and ensureBoardWorktree's orphan branch both exist to serve — `git checkout -b
// <branch>` on an unborn HEAD succeeds but creates another unborn branch: no ref is written.
// Nothing later fills it in either, because the clone-time commit the CLI lands through Bolt goes to
// the _board worktree's own unsuffixed branch, not to this one.
// The hub therefore came out of clone with its weft primary sitting on a branch that does not exist,
// and every verb that forks a new pair from it died on `fatal: invalid reference: <branch>-weft` —
// `lyx fabric add` included, which is the example both the parent command and `add` itself document.
//
// A branch that already resolves is left untouched, so the ordinary non-empty-remote clone and the
// re-clone adopt path are unaffected.
func bornWeftPrimaryBranch(weftPath, branch string) error {
	// Mixed probe: the exit path answers "the branch is still unborn", the case this function
	// exists to fix, so it is recovered via errors.As rather than merged into a single message.
	_, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + branch}, weftPath)
	if err == nil {
		return nil
	}
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return fmt.Errorf("verify weft primary branch %q: %w", branch, err)
	}

	if _, err := gitexec.Run(
		[]string{"commit", "--allow-empty", "-m", "fabric clone: initialise weft primary branch " + branch},
		weftPath,
	); err != nil {
		return fmt.Errorf("initialise unborn weft primary branch %q: %w", branch, err)
	}
	return nil
}

// cloneRepo clones a repository from url to dest.
//
// The clone is executed via gitexec.Run — the checked entry point, not the raw RunGit — with the
// parent directory of dest as the cwd, and the basename of dest as the destination argument.
// Paths are cleaned and normalized.
// The checked form is right here because a non-zero git exit at a clone is unambiguously a failure
// rather than an answer, so there is nothing for this site to recover via errors.As;
// it simply wraps the resulting *gitexec.GitError, whose own message already carries git's stderr.
func cloneRepo(url, dest string) error {
	// Clean and normalize paths
	dest = filepath.Clean(dest)
	parentDir := filepath.Dir(dest)
	parentDir = filepath.Clean(parentDir)
	destName := filepath.Base(dest)

	// Verify the parent directory exists
	info, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("parent directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent directory does not exist: %s is not a directory", parentDir)
	}

	// Convert paths to use forward slashes for git compatibility on Windows.
	// dest is split into parent+basename so git resolves cleanly on Windows;
	// the plan's full-absolute-dest variant is functionally equivalent.
	gitURL := filepath.ToSlash(url)
	gitDest := filepath.ToSlash(destName)

	if _, err := gitexec.Run([]string{"clone", gitURL, gitDest}, parentDir); err != nil {
		return fmt.Errorf("clone %q to %q failed: %w", url, dest, err)
	}

	return nil
}

// resetHub implements --reset: it removes an existing hub at hubPath, but only once it has
// established that hubPath IS one.
//
// The hub name is derived, never typed — from the warp URL in the two-argument form, and from the
// warp URL recorded on the weft's own `.lyx-warp` binding in the one-argument form, where the
// operator never sees the name of the directory being deleted at all.
// An unconditional RemoveAll on a derived path therefore destroyed any directory that merely
// happened to be called `<name>-HUB`, user content and all, on a flag whose help promises to
// "remove an existing hub".
//
// cwd is the operator-named parent CloneHub normalised at its top: resetHub has no *lyxcwd.Location
// to contain against (it runs before any resolution), so cwd is the only containment anchor
// available, and it is what R4's `clone --reset` defect needed and did not have.
//
// The hub predicate is structural and cheap: a fabric hub always holds `<hub>/_board`, and always
// holds at least one `*-weft` sibling.
// Either is enough — a hub whose `_board` worktree was hand-deleted is still recognisably a hub, and
// so is one mid-clone whose board has not been materialised yet.
// An absent path stays a silent no-op, so --reset remains idempotent.
// rec is CloneHub's own recorder, already non-nil at both call sites (see this function's callers'
// own comment) — threaded through to removePath.
func resetHub(rec *Mutations, cwd, hubPath string) error {
	info, statErr := os.Stat(hubPath)
	if os.IsNotExist(statErr) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("reset: inspect %s: %w", hubPath, statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("reset: refusing to remove %s: it is not a directory, so it is not a hub", hubPath)
	}

	if !looksLikeHub(hubPath) {
		return fmt.Errorf(
			"reset: refusing to remove %s: it has no %s and no %q sibling, so it is not a fabric hub — remove it yourself if that is really what you meant",
			hubPath, BoardDirName, "*"+weftname.Suffix)
	}

	req := pathRequest{
		what:      "reset hub",
		container: cwd,
		target:    hubPath,
		slug:      nil,
		ownership: ownedFabricHub(),
		dirtiness: dirtinessNA("--reset is the operator explicitly asking for this hub to be replaced; ownership is the check that matters here"),
		force:     false,
	}
	if err := removePath(rec, req); err != nil {
		return fmt.Errorf("reset: remove hub at %s: %w", hubPath, err)
	}
	return nil
}

// looksLikeHub reports whether hubPath carries the structural marks of a fabric hub: a `_board`
// entry, or at least one weft sibling directory.
// An unreadable directory answers false, the conservative direction — a directory fabric cannot
// enumerate is exactly where an unconditional recursive removal is least defensible.
func looksLikeHub(hubPath string) bool {
	if _, err := os.Stat(filepath.Join(hubPath, BoardDirName)); err == nil {
		return true
	}

	entries, err := os.ReadDir(hubPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := WeftWarpSlug(entry.Name()); ok {
			return true
		}
	}
	return false
}

// teardownHub removes the Hub directory and returns an error combining the cause with
// information about the failed removal (if applicable).
//
// cwd is the operator-named parent CloneHub normalised at its top, and tok is the createdToken
// createExclusiveDir minted when this same invocation created hubPath. Every one of teardownHub's
// thirteen call sites runs after that creation, so tok is always in scope. tok, not ownedFabricHub,
// is deliberate: the earliest call sites run immediately after the warp clone attempt, before any
// board or weft sibling exists, so ownedFabricHub would refuse exactly the half-built hub teardown
// exists to clean up. The token is strictly stronger than the pattern-match resetHub uses, because
// pattern-matching a derived name is exactly what R4's `clone --reset` defect exploited.
//
// If removePath succeeds, teardownHub returns cause unchanged. If removePath fails — whether an
// operational failure or a gate refusal — teardownHub returns an error combining cause with a
// message about the residual Hub.
// rec is CloneHub's own recorder — every one of teardownHub's thirteen call sites sits inside
// CloneHub, whose recorder card 10 installed, so rec is passed through at each.
func teardownHub(rec *Mutations, cwd, hubPath string, tok createdToken, cause error) error {
	req := pathRequest{
		what:      "teardown hub",
		container: cwd,
		target:    hubPath,
		slug:      nil,
		ownership: ownedFreshlyCreatedPath(tok),
		dirtiness: dirtinessNA("gate-created within this invocation; nothing pre-existing to lose"),
		force:     false,
	}
	if err := removePath(rec, req); err != nil {
		return fmt.Errorf("%w; residual hub left at %s; remove it manually before retrying", cause, hubPath)
	}
	return cause
}

// DeriveWarpName extracts the warp repository basename from a raw URL or file path.
//
// It trims any trailing slash or backslash, then takes the final path segment of rawURL
// after splitting on forward slash, backslash, and colon (for HTTPS URLs, file paths,
// and SCP-form URLs like git@github.com:user/repo.git).
// A single trailing .git extension is stripped if present. Returns an empty string if no
// basename can be extracted or if the URL contains no path segments.
//
// Examples:
//
//   - "https://github.com/u/repo.git" → "repo"
//   - "https://github.com/u/repo" → "repo"
//   - "git@github.com:u/repo.git" → "repo"
//   - "https://github.com/u/repo/" → "repo"
//   - "C:\path\to\repo.git" → "repo"
//   - "" → ""
func DeriveWarpName(rawURL string) string {
	// Trim trailing slashes (both forward and back)
	rawURL = strings.TrimSuffix(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, "\\")

	// Split on forward slash, backslash, and colon to handle HTTPS, file paths, and SCP forms
	var parts []string
	for _, seg := range strings.FieldsFunc(rawURL, func(r rune) bool { return r == '/' || r == '\\' || r == ':' }) {
		if seg != "" {
			parts = append(parts, seg)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	// Take the last segment and strip .git suffix
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	return name
}
