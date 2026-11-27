// clone.go implements CloneHub, the clone orchestration that tears the hub down when any step fails.
// It also holds the refusal of a hub a killed clone left without its Bolt checkout, which names the re-clone.
//
// After the weft clone succeeds, the weft primary is checked out onto a "-weft"-suffixed branch.
// For the default branch "main", that branch is "main-weft".
// Every fabric-managed weft branch carries that suffix, so weft:main is never claimed directly.
// The suffixed branch is adopted from origin/<branch>-weft when the remote has one, and created fresh otherwise.
// The default branch itself stays, unclaimed.
//
// Against an empty weft remote, the fresh branch is unborn, because git checkout -b writes no ref on an unborn HEAD.
// bornWeftPrimaryBranch therefore commits an initial empty commit onto it.
// Without that commit, the weft primary would name a branch that does not exist,
// and every pair-creating verb forked from the hub would fail.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// staleFabricAnchorName is the pre-rename lym-anchor marker filename, aliased from lymcwd.StaleAnchorFileName.
// CloneHub looks for it only to refuse a hub that carries it, rather than silently re-anchoring at the wrong subpath.
// The refusal applies only when the prime's weft root carries no marker.
const staleFabricAnchorName = lymcwd.StaleAnchorFileName

// existingHubError refuses a clone into hubPath, which already exists.
// A clone killed partway leaves such a hub, which no other verb finishes,
// so the way forward is the same clone with --reset, which replaces the hub.
func existingHubError(cwd, hubPath string, opts fabriclayout.CloneOptions) error {
	return fmt.Errorf("hub already exists at %s; way forward: replace it with `%s`", hubPath, cloneResetCommand(cwd, opts))
}

// cloneResetCommand spells the clone opts asks for into the directory cwd, with --reset added.
func cloneResetCommand(cwd string, opts fabriclayout.CloneOptions) string {
	args := []string{"lym", "fabric", "clone"}
	if opts.SingleRepo {
		args = append(args, "--single-repo")
	}
	if opts.Shortname != "" {
		args = append(args, "--shortname", opts.Shortname)
	}
	if opts.Subpath != "" {
		args = append(args, "--subpath", opts.Subpath)
	}
	if opts.ForceBootstrap {
		args = append(args, "--force-bootstrap")
	}
	args = append(args, opts.WeftURL)
	if opts.WarpURL != "" {
		args = append(args, opts.WarpURL)
	}
	return strings.Join(append(args, "--into", cwd, "--reset"), " ")
}

// RefuseHubWithoutBolt refuses the hub of l when its Bolt checkout holds no repository.
// A clone killed after it made the records store and before the Bolt checkout leaves such a hub,
// and every pull or write of the board fails on it with a raw git error.
// The refusal names the clone that replaces the hub: its own origin URLs, re-cloned into its parent with --reset.
// A one-repository hub's clone takes the code store's URL alone.
// Its refusal names the join, for an origin that holds the hub's board, and the seed, for an origin that holds none.
// A records store whose layout is lost is named as a one-repository hub.
// Any layout read failure is carried on the refusal.
// A URL that cannot be read is named by a placeholder, and so is the seed's shortname.
func RefuseHubWithoutBolt(l *lymcwd.Location) error {
	boltDir := fabriclayout.BoltDir(l.HubPath)
	if _, err := os.Lstat(filepath.Join(boltDir, ".git")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("fabricengine: look for the repository of the %s checkout at %s: %w", fabriclayout.BoltDirName, boltDir, err)
	}

	codeURL := "<code-url>"
	if url, err := gitrepo.New(l.WorktreePath()).RemoteURL(originRemoteName); err == nil {
		codeURL = url
	}
	recordsURL := "<records-url>"
	oneRepository := false
	var layoutErr error
	if recordsRoot, err := RecordsRepoRoot(l); err == nil {
		ns, nsErr := originNamespaceAt(recordsRoot)
		layoutErr = nsErr
		oneRepository = errors.Is(nsErr, ErrRecordsLayout) || ns != (gitrepo.RefNamespace{})
		if !oneRepository {
			if url, err := gitrepo.New(recordsRoot).RemoteURL(originRemoteName); err == nil {
				recordsURL = url
			}
		}
	}

	parent := filepath.Dir(l.HubPath)
	replacement := "`" + cloneResetCommand(parent, fabriclayout.CloneOptions{WeftURL: recordsURL, WarpURL: codeURL}) + "`"
	if oneRepository {
		join := cloneResetCommand(parent, fabriclayout.CloneOptions{WeftURL: codeURL})
		seed := cloneResetCommand(parent, fabriclayout.CloneOptions{SingleRepo: true, Shortname: "<shortname>", WeftURL: codeURL})
		replacement = fmt.Sprintf("`%s` when origin holds the hub's board, or `%s` when it holds none", join, seed)
	}
	refusal := fmt.Sprintf("the hub at %s has no repository at its %s checkout %s, as a clone that stopped before making it leaves; "+
		"way forward: replace the hub with %s",
		l.HubPath, fabriclayout.BoltDirName, boltDir, replacement)
	if layoutErr != nil {
		return fmt.Errorf("%s; the records store's layout also cannot be read: %w", refusal, layoutErr)
	}
	return errors.New(refusal)
}

// CloneResult carries the geometry that CloneHub returns once the clone, board worktree and anchor are set up.
// It holds only git and geometry values.
// The CLI layer drives config materialization, the weft:main commit and junction wiring from these fields.
// Those steps go through internal/configsync, which fabricengine cannot import without an import cycle.
// It embeds MutationRecord, which records the mutations made during the call.
type CloneResult struct {
	fabriclayout.MutationRecord
	HubPath  string // HubPath is the created <name>-LYMHUB container directory.
	Anchor   string // Anchor is the resolved lym-anchor subpath (e.g. "backend" or ".").
	BoardDir string // BoardDir is the package-level BoltDir(HubPath) result, the weft:main checkout.
	WeftBase string // WeftBase is the weft-side directory paired with PrimeCwd.
	PrimeCwd string // PrimeCwd is the resolved prime warp worktree path at Anchor.
	// WarpURL is the effective warp URL actually cloned, whether supplied in opts or derived from
	// the recorded binding.
	WarpURL string
	// WarpBindingRecorded is true only when this clone wrote the .lym-warp record (a fresh binding,
	// including the clone-time backfill of a pre-binding hub).
	WarpBindingRecorded bool
	// Shortname is the effective repo shortname, empty when the weft is bound but has no shortname.
	Shortname string
	// ShortnameRecorded is true only when this clone wrote the .lym-shortname record.
	ShortnameRecorded bool
	// Warning is non-empty when the clone succeeded but left something the operator must do.
	Warning string
	// SeamWarning is the warning text of a failed worktree event for the prime, naming the render verb that repairs it.
	// CloneHub leaves it empty; the wiring that follows the clone fires the event and sets it.
	// A clone that fails after the event returns no result, so the warning then travels in the error.
	SeamWarning string
	// seed is set only by a SingleRepo clone.
	// It marks the result for PushCloneBoard and AbandonSeededClone, so no caller reads the layout.
	seed *cloneSeed
}

// cloneSeed holds what a SingleRepo clone's seed push and the teardown of a failed one need.
type cloneSeed struct {
	// container is the directory the hub was created in, and hubPath the hub directory.
	container string
	hubPath   string
	// hubTok proves this clone created hubPath.
	hubTok createdToken
	// recordsPath is the records store, and boardDir the board worktree of it.
	recordsPath string
	boardDir    string
	// recordsBranch is the prime's records branch.
	recordsBranch string
	// observedTip is the tip of recordsBranch under the records namespace on origin at the precondition check.
	// It is empty when origin held no such branch.
	observedTip string
}

// CloneHub clones the warp and weft repositories into a new hub directory and returns the resolved geometry.
// It checks out the weft primary's pairing branch and materializes the board worktree.
// It resolves the lym-anchor subpath and records the warp binding when it is new.
// It does not materialize config or wire junctions.
// The CLI layer does that from the returned CloneResult.
// Those steps import internal/configsync, which fabricengine cannot import.
//
// opts.WeftURL is required.
// opts.WarpURL is optional; when empty, it is resolved from the warp binding recorded on the weft candidate.
//
// When opts.WarpURL is set, the hub name is known without the network.
// The hub-exists check, and under Reset the check that the target is a hub, run before the weft probe.
// When it is empty, the weft probe runs first, because the recorded binding supplies the warp URL and so the hub name.
// Either way, Reset removes an existing hub only after the probe has checked the warp binding.
// The probe also checks the shortname and the old-order guard.
// It checks an explicit subpath against staged clones of both remotes.
// A refused argument therefore leaves the existing hub untouched.
//
// After that resolution, the operation proceeds as follows:
//  1. Create the hub directory; on failure, return the wrapped error (no teardown yet).
//  2. Clone the warp repo to <Hub>/<name>; on failure, tear down and return the error.
//  3. Clone the weft repo to <Hub>/<name>-weft; on failure, tear down and return the error.
//     3b. Read the weft primary's checked-out branch and check out its "-weft"-suffixed pairing at
//     the same HEAD, capturing the warp branch name it read; on failure, tear down and return the error.
//  4. Materialize the Bolt checkout as a second weft worktree via ensureBoardWorktree, adopting the
//     captured warp branch if it already exists locally from step 3, or freshly orphan-creating it otherwise;
//     on failure, tear down and return the error.
//  5. Resolve the lym-anchor subpath, adopt or create. Adopt reads the marker committed at the root of the
//     prime's weft worktree and writes nothing. Create validates
//     that the subpath exists in the warp worktree, then commits the marker on the prime's weft branch.
//     Then write the warp-binding record when it is new, and return the resolved CloneResult.
//
// A one-URL clone first reads origin's layout with one ls-remote.
// That read comes before the hub-exists check, --reset and anything created.
// An origin left by the earlier one-repository layout is refused.
// Without opts.SingleRepo, so is an origin holding records branches and no board, which a seed that stopped leaves.
// With opts.SingleRepo, an origin that already holds a board is refused too.
// So is one whose default branch is a records repository.
// So is one whose default branch tracks the earlier layout's directories.
// Otherwise the URL is the code repository of a one-repository hub:
// an origin holding the board is joined, and one holding none is seeded under opts.SingleRepo.
// The records store of such a hub is a git init of its own, fetching only the records namespace.
// A seeded clone sets CloneResult's seed mark, and PushCloneBoard then pushes it.
//
// Any clone or worktree-add failure triggers teardownHub, which removes the entire hub directory.
// If removal also fails, the error names both the original failure and the residual hub path.
//
// A cwd that is not an existing directory is refused by name before any git runs.
func CloneHub(cwd string, opts fabriclayout.CloneOptions) (res CloneResult, err error) {
	var rec *Mutations
	defer func() { res.Mutations = rec.Snapshot() }()

	cwd = filepath.Clean(cwd)
	// git started in a directory that is not there fails as if git itself were missing, so the directory is named here.
	if info, statErr := os.Stat(cwd); statErr != nil {
		return CloneResult{}, fmt.Errorf("the directory the hub is created in cannot be read: %w; way forward: create it, or pass --into an existing directory", statErr)
	} else if !info.IsDir() {
		return CloneResult{}, fmt.Errorf("%s, where the hub is created, is not a directory; way forward: pass --into an existing directory", cwd)
	}

	if opts.WeftURL == "" {
		return CloneResult{}, fmt.Errorf("weft URL is required")
	}

	// Validate the requested subpath before anything is created or fetched.
	// This refuses a structurally impossible anchor before teardown is the only way out.
	// An absolute or escaping subpath would pass the later existence probe in the cloned warp.
	// filepath.Join drops a leading separator, and ".." lands on a directory that always exists.
	requestedAnchor, err := lymcwd.ValidateAnchorRel(opts.Subpath)
	if err != nil {
		return CloneResult{}, err
	}
	subpathRequestedExplicitly := strings.TrimSpace(opts.Subpath) != ""

	if opts.SingleRepo && opts.WarpURL != "" {
		return CloneResult{}, fmt.Errorf("--single-repo takes the code repository's URL alone; way forward: give --single-repo one URL, the code repository")
	}
	if opts.SingleRepo && opts.Shortname == "" {
		return CloneResult{}, fmt.Errorf("a fresh --single-repo clone needs the repo's shortname: pass --shortname <shortname>, 2-6 characters matching [a-z][a-z0-9]{1,5}, e.g. lym fabric clone --single-repo --shortname lx <code-url>")
	}

	var layout originLayout
	oneRepository := false
	if opts.WarpURL == "" {
		layout, err = readOriginLayout(cwd, opts.WeftURL)
		if err != nil {
			return CloneResult{}, err
		}
		if err := refuseOneURLClone(cwd, layout, opts); err != nil {
			return CloneResult{}, err
		}
		oneRepository = opts.SingleRepo || layout.boardTip != ""
	}

	var name, hubPath, effective string
	var writeRecord, derivedFromRecord bool
	var effectiveShortname, shortnameWarning string
	var writeShortname bool

	if opts.WarpURL != "" {
		// Two-argument form: the hub name needs no network.
		// Derive it and run the offline checks before touching the network.
		name = DeriveWarpName(opts.WarpURL)
		if name == "" {
			return CloneResult{}, fmt.Errorf("could not derive repo name from warp URL %s", opts.WarpURL)
		}
		hubPath = fabriclayout.HubPath(cwd, name)
		rec = NewMutations(hubPath)
		if opts.Reset {
			if _, err := checkResetTarget(hubPath); err != nil {
				return CloneResult{}, err
			}
		} else if _, err := os.Stat(hubPath); err == nil {
			return CloneResult{}, existingHubError(cwd, hubPath, opts)
		}

		// The pre-hub probe touches the network only after the offline checks have passed.
		probe, err := probeWeftBinding(cwd, opts.WeftURL)
		if err != nil {
			return CloneResult{}, err
		}
		effective, writeRecord, err = resolveEffectiveWarpURL(probe.RecordedWarpURL, probe.Found, opts.WarpURL)
		if err != nil {
			return CloneResult{}, err
		}
		effectiveShortname, writeShortname, shortnameWarning, err = resolveEffectiveShortname(probe.RecordedShortname, probe.ShortnameFound, opts.Shortname, probe.freshBind())
		if err != nil {
			return CloneResult{}, err
		}
		if writeRecord && !probe.WeftLooksLikeWeft && !opts.ForceBootstrap {
			// The guard applies only on the bootstrap path, where writeRecord is true.
			// Only the two-argument form reaches that path.
			// ForceBootstrap is therefore ignored everywhere else, with no usage error and no warning.
			return CloneResult{}, fmt.Errorf(
				"refusing to bootstrap %s as a weft: its history carries neither %s on weft:main or main-weft nor an empty tree — check the argument order, clone now takes <weft-url> [<warp-url>]",
				opts.WeftURL, lymcwd.AnchorFileName)
		}
	} else if oneRepository {
		// One-repository form: the one URL is the code repository, and the hub is named after it.
		// The name needs no network, so the offline checks run before anything is created.
		name = DeriveWarpName(opts.WeftURL)
		if name == "" {
			return CloneResult{}, fmt.Errorf("could not derive repo name from URL %s", opts.WeftURL)
		}
		hubPath = fabriclayout.HubPath(cwd, name)
		rec = NewMutations(hubPath)
		effective = opts.WeftURL
		if opts.Reset {
			if _, err := checkResetTarget(hubPath); err != nil {
				return CloneResult{}, err
			}
		} else if _, err := os.Stat(hubPath); err == nil {
			return CloneResult{}, existingHubError(cwd, hubPath, opts)
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
			// The resolver's message does not name the weft URL, so this error names it.
			return CloneResult{}, &sentinelError{
				sentinel: ErrNoWarpBinding,
				msg:      fmt.Sprintf("weft %s has no recorded warp binding; supply the warp URL explicitly: lym fabric clone <weft-url> <warp-url>", opts.WeftURL),
			}
		}
		effectiveShortname, writeShortname, shortnameWarning, err = resolveEffectiveShortname(probe.RecordedShortname, probe.ShortnameFound, opts.Shortname, probe.freshBind())
		if err != nil {
			return CloneResult{}, err
		}
		derivedFromRecord = true
		name = DeriveWarpName(effective)
		if name == "" {
			return CloneResult{}, fmt.Errorf("could not derive repo name from warp URL %s recorded in the %s binding on weft:main", effective, fabriclayout.WarpBindingFileName)
		}
		hubPath = fabriclayout.HubPath(cwd, name)
		rec = NewMutations(hubPath)
	}

	// --subpath can only be checked against the cloned trees.
	// A --reset clone therefore checks it against staged shallow clones while the existing hub is still intact.
	// A seeded clone creates its anchor fresh and adopts none, so only a two-repository clone and a join are checked.
	if opts.Reset && subpathRequestedExplicitly && !opts.SingleRepo {
		if _, statErr := os.Stat(hubPath); statErr == nil {
			namespace := gitrepo.RefNamespace{}
			if oneRepository {
				namespace = oneRepositoryNamespace()
			}
			if err := checkSubpathBeforeReset(cwd, opts.WeftURL, effective, requestedAnchor, namespace); err != nil {
				return CloneResult{}, err
			}
		}
	}

	// Every argument has now been checked against the remotes, so --reset may remove the existing hub.
	// Offline, a two-argument clone fails here: with --reset as a non-hub refusal, without it as "hub already exists".
	// An offline one-argument clone fails earlier, at "probe weft <url>:", because the hub name comes from a remote fact.
	if opts.Reset {
		if err := resetHub(rec, cwd, hubPath); err != nil {
			return CloneResult{}, err
		}
	} else if _, err := os.Stat(hubPath); err == nil {
		return CloneResult{}, existingHubError(cwd, hubPath, opts)
	}

	warpURL := effective

	// Create the hub directory exclusively: os.Mkdir's EEXIST is the collision guarantee.
	// The token it returns lets teardownHub prove that a later teardown removes a directory this call created.
	// The stat guards above only order the checks and give a clear error; they do not prevent a collision.
	// Moving this create earlier would either leak a residual hub or break the offline-before-network ordering.
	// A plain os.MkdirAll after the stat would race:
	// a concurrent process could create the hub in between, and MkdirAll would accept it silently.
	hubTok, err := createExclusiveDir(rec, hubPath)
	if err != nil {
		return CloneResult{}, err
	}

	// Clone the warp repo into the hub.
	warpWorktreePath := filepath.Join(hubPath, name)
	if err := cloneRepo(warpURL, warpWorktreePath); err != nil {
		if derivedFromRecord {
			// The derive path names its source so a failure here is traceable back to the
			// binding that produced it, not just the bare URL.
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("clone warp %s (from the %s binding on weft:main): %w", warpURL, fabriclayout.WarpBindingFileName, err))
		}
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	// The clone brings a worktree into being.
	// One entry at the coarsest covering root covers the whole cloned tree.
	rec.Append(fabriclayout.KindWorktreeCreated, warpWorktreePath, "")

	if err := refuseUncheckedOutWarpClone(warpWorktreePath, warpURL); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}

	// The post-checkout hook fires drift warnings on every later git checkout in this repo.
	// It is installed after the warp worktree exists.
	// Hook installation is non-fatal: a failure is logged and does not abort the clone.
	// The hook serves usability, not correctness.
	//
	// InstallPostCheckoutHook reads one field, WorktreePath(), so it needs a path, not a resolution.
	// The exclusive create above leaves no marker yet, so the Location answers "." for its anchor.
	hookLocation, hookErr := lymcwd.NamedWorktree(hubPath, name)
	if hookErr == nil {
		hookErr = InstallPostCheckoutHook(hookLocation)
	}
	if hookErr != nil {
		logger.Warn("fabricengine: post-checkout hook install failed (non-fatal)", "verb", "clone", "hub", hubPath, "error", hookErr)
	}

	// Clone the weft repo into the hub.
	weftPath := weftname.SiblingPath(hubPath, name)
	if oneRepository {
		if err := initRecordsStore(opts.WeftURL, weftPath, !opts.SingleRepo); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
	} else if err := cloneRepo(opts.WeftURL, weftPath); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(fabriclayout.KindWorktreeCreated, weftPath, "")

	// The weft primary is checked out on RecordsBranchName(warpBranch);
	// this keeps weft:<branch> from being claimed directly under fabric's uniform branch scheme.
	// warpBranch is read from the warp clone, never from the weft clone.
	// `lym fabric add` forks every new pair's weft branch from RecordsBranchName(<warp branch>).
	// The weft remote's HEAD may be an empty bare created with a different default branch.
	// A weft primary named after that HEAD leaves a hub on which no pair can ever be created.
	// The board worktree below reuses the same value.
	warpBranch, err := checkedOutBranch(warpWorktreePath, "warp prime")
	if err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	if err := suffixWeftPrimaryBranch(weftPath, warpBranch); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}

	// Materialize the Bolt checkout as a second weft worktree on the board branch.
	// In a two-repository hub that is warpBranch, and in a one-repository hub the fixed board branch of the namespace.
	// It adopts that branch if it already exists locally from the weft clone;
	// otherwise it creates a fresh orphan, which is the case for an empty weft remote.
	boardBranch := warpBranch
	if oneRepository {
		boardBranch = fabriclayout.OneRepositoryBoardBranch
		primeRecordsBranch := fabriclayout.RecordsBranchName(warpBranch)
		if _, err := gitexec.Run([]string{"config", fabriclayout.PrimeRecordsBranchKey, primeRecordsBranch}, weftPath); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("record the prime's records branch %s: %w", primeRecordsBranch, err))
		}
	}
	boardDir := fabriclayout.BoltDir(hubPath)
	if err := ensureBoardWorktree(weftPath, boardBranch, boardDir); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(fabriclayout.KindWorktreeCreated, boardDir, "")

	// The hub-wide never-tracked tree is the Bolt checkout's .lym, the ephemeral sibling of its _lym.
	// It is created here because the Bolt checkout exists only once ensureBoardWorktree above has run.
	//
	// The excludes are seeded first so the Bolt checkout's .lym never shows as untracked dirt.
	// The board worktree stays clean from the hub's birth on.
	// The board's stage-all commit seeds the excludes again before staging.
	//
	// A seed failure is fatal and goes through teardownHub like the other failures in this step:
	// a hub whose weft common gitdir cannot take an exclude cannot keep its scratch untracked.
	if err := seedWeftArtifactExcludes(boardDir); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("seed weft artifact excludes in board worktree: %w", err))
	}
	scratchDir := fabriclayout.HubScratchDir(hubPath)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
	}
	rec.Append(fabriclayout.KindDirCreated, scratchDir, "")

	// A one-repository clone reads the board's records only now, from the board origin's tip gave it.
	// A seeded clone reads none, so its binding is the one URL and its shortname the supplied one.
	if oneRepository {
		var recordedWarp, recordedShortname string
		var warpFound, shortnameFound bool
		if !opts.SingleRepo {
			recordedWarp, warpFound, recordedShortname, shortnameFound, err = readJoinedBoardRecords(weftPath, opts.WeftURL)
			if err != nil {
				return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
			}
		}
		effective, writeRecord, err = resolveEffectiveWarpURL(recordedWarp, warpFound, opts.WeftURL)
		if err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
		effectiveShortname, writeShortname, shortnameWarning, err = resolveEffectiveShortname(recordedShortname, shortnameFound, opts.Shortname, !warpFound)
		if err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
	}

	// Resolve the lym-anchor subpath as adopt-or-create.
	// A fresh bind commits the marker on the prime's weft branch here;
	// the Bolt commit the CLI layer makes later carries config and no anchor.
	primeData, primeReadErr := os.ReadFile(filepath.Join(weftPath, lymcwd.AnchorFileName))
	var anchor string
	if primeReadErr == nil {
		// Adopt path: the prime's weft worktree carries a committed marker at its root,
		// so this is a re-clone of a hub that already recorded one, and nothing is written.
		anchor, err = adoptRecordedAnchor(primeData, "on "+fabriclayout.RecordsBranchName(warpBranch), requestedAnchor, subpathRequestedExplicitly)
		if err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
	} else {
		// A `.fabric-anchor` marker on the board with no `.lym-anchor` marker on the prime is refused, not ignored:
		// without this check the clone would fall through to the create path and re-anchor at "." or at the requested --subpath.
		// The marker records a real anchor, so `_lym` would silently resolve to the wrong place for a subpath-anchored repo.
		if _, statErr := os.Stat(filepath.Join(boardDir, staleFabricAnchorName)); statErr == nil {
			// The remedy names the marker on the prime's records branch, never a re-clone:
			// this error comes from a clone, so telling the operator to clone again reproduces it.
			// A clone adopts only the marker committed on that branch.
			// Renaming the marker in the board worktree alone would let the retry fall through to the create path.
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf(
				"found stale %s marker with no %s beside it at %s; in an existing hub, commit %s holding the old marker's content at the root of the weft remote's records branch %s "+
					"from the prime's weft worktree and push it, remove %s from the %s worktree, commit and push that, then retry this clone",
				staleFabricAnchorName, lymcwd.AnchorFileName, boardDir,
				lymcwd.AnchorFileName, fabriclayout.RecordsBranchName(warpBranch),
				staleFabricAnchorName, fabriclayout.BoltDirName))
		}

		// Create path: first-ever clone of this weft, no marker committed yet.
		// Validate that the requested subpath exists in the warp worktree before recording it:
		// a typo such as "backedn" fails loudly instead of silently anchoring to a directory that was never there.
		anchor = requestedAnchor
		if info, statErr := os.Stat(filepath.Join(warpWorktreePath, anchor)); statErr != nil || !info.IsDir() {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("subpath %q does not exist as a directory in the cloned warp repo", anchor))
		}
		if _, err := commitAnchorMarker(rec, weftPath, anchor, "fabric clone: record anchor"); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
	}

	// Write the warp-binding record when this clone determined it; both branches above fall through to this point.
	var warpBindingRecorded bool
	if writeRecord {
		if err := writeWarpBinding(boardDir, effective); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
		rec.Append(fabriclayout.KindFileWritten, filepath.Join(boardDir, fabriclayout.WarpBindingFileName), "")
		warpBindingRecorded = true
	}

	// The shortname record sits beside the warp binding, committed by the same CLI-layer Bolt commit.
	var shortnameRecorded bool
	if writeShortname {
		if err := WriteShortname(boardDir, effectiveShortname); err != nil {
			return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, err)
		}
		rec.Append(fabriclayout.KindFileWritten, filepath.Join(boardDir, fabriclayout.ShortnameFileName), "")
		shortnameRecorded = true
	}

	// Resolve the prime layout now that the marker is recorded, so
	// RelPath — and therefore WeftBase — reflects the resolved anchor.
	primeCwd := filepath.Join(warpWorktreePath, anchor)
	l, err := lymcwd.Resolve(primeCwd)
	if err != nil {
		return CloneResult{}, teardownHub(rec, cwd, hubPath, hubTok, fmt.Errorf("resolve prime layout at %s: %w", primeCwd, err))
	}

	weftBase := filepath.Join(RecordsWorktree(l), l.AnchorRel)

	regenerateAfterVerb(rec, l, "clone")

	var seed *cloneSeed
	if opts.SingleRepo {
		primeRecordsBranch := fabriclayout.RecordsBranchName(warpBranch)
		seed = &cloneSeed{
			container:     cwd,
			hubPath:       hubPath,
			hubTok:        hubTok,
			recordsPath:   weftPath,
			boardDir:      boardDir,
			recordsBranch: primeRecordsBranch,
			observedTip:   layout.heads[primeRecordsBranch],
		}
	}

	return CloneResult{
		seed:                seed,
		HubPath:             hubPath,
		Anchor:              anchor,
		BoardDir:            boardDir,
		WeftBase:            weftBase,
		PrimeCwd:            primeCwd,
		WarpURL:             warpURL,
		WarpBindingRecorded: warpBindingRecorded,
		Shortname:           effectiveShortname,
		ShortnameRecorded:   shortnameRecorded,
		Warning:             shortnameWarning,
	}, nil
}

// adoptRecordedAnchor validates the anchor a marker records and checks it against the requested subpath.
// source names where the marker was read, for the error text.
// The recorded value is validated, not trusted:
// adopting an absolute or escaping anchor yields a hub whose every weft commit fails.
// An explicitly requested subpath that disagrees with the recorded anchor is refused, never silently re-anchored;
// "." is not exempt: the flag defaults to the empty string, so an explicit "." is distinguishable from no request.
func adoptRecordedAnchor(markerData []byte, source, requestedAnchor string, requestedExplicitly bool) (string, error) {
	recorded, err := lymcwd.ValidateAnchorRel(strings.TrimSpace(string(markerData)))
	if err != nil {
		return "", fmt.Errorf("recorded anchor in %s %s is unusable: %w", lymcwd.AnchorFileName, source, err)
	}
	if requestedExplicitly && requestedAnchor != recorded {
		return "", fmt.Errorf("requested --subpath %q does not match the recorded anchor %q for this hub", requestedAnchor, recorded)
	}
	return recorded, nil
}

// checkedOutBranch returns the branch checked out at dir, born or unborn.
// The role argument names dir in the error.
// It refuses a detached HEAD, which names no branch to pair with.
func checkedOutBranch(dir, role string) (string, error) {
	stdout, err := gitexec.Run([]string{"branch", "--show-current"}, dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s branch: %w", role, err)
	}
	branch := strings.TrimSpace(stdout)
	if branch == "" {
		return "", fmt.Errorf("%s at %s is on a detached HEAD after clone; cannot derive its weft branch", role, dir)
	}
	return branch, nil
}

// suffixWeftPrimaryBranch checks the freshly cloned weft primary at weftPath out onto RecordsBranchName(warpBranch).
// On a re-clone of a hub with synced weft history, origin already carries the branch;
// it is adopted as a tracking local branch.
// The fresh hub inherits the accumulated weft state, including its _lym content.
// Its first push rebase-recovers through the configured upstream;
// without that upstream, it would diverge permanently from an untracked fork.
// Only when origin has no such branch yet is it created fresh at the current HEAD.
// It returns an error if any git call fails.
func suffixWeftPrimaryBranch(weftPath, warpBranch string) error {
	suffixedBranch := fabriclayout.RecordsBranchName(warpBranch)

	// Adopt path: origin already carries the suffixed branch, as on a re-clone of a hub with existing weft history.
	// Starting the local branch from origin/<suffixed> checks out that history.
	// It sets the upstream, as git's default branch.autoSetupMerge does for a remote-tracking start point.
	// The create path below leaves the upstream to the first push.
	// ErrRefNotFound answers "no remote suffixed branch yet"; any other error is a real failure.
	remoteRef := "refs/remotes/origin/" + suffixedBranch
	_, err := gitrepo.New(weftPath).RefSHA(remoteRef)
	remoteBranchExists := err == nil
	if err != nil && !errors.Is(err, gitrepo.ErrRefNotFound) {
		return fmt.Errorf("check for remote weft primary branch: %w", err)
	}
	checkoutArgs := []string{"checkout", "-b", suffixedBranch}
	if remoteBranchExists {
		checkoutArgs = append(checkoutArgs, "origin/"+suffixedBranch)
	}

	if _, err := gitexec.Run(checkoutArgs, weftPath); err != nil {
		return fmt.Errorf("checkout -b %q in weft primary: %w", suffixedBranch, err)
	}

	return bornWeftPrimaryBranch(weftPath, suffixedBranch)
}

// bornWeftPrimaryBranch gives the weft primary's suffixed branch a real commit when the clone leaves it UNBORN.
// So refs/heads/<suffixed> resolves by the time the clone returns.
//
// On a genuinely empty weft remote, the first-ever setup path leaves HEAD unborn.
// `git checkout -b <branch>` on an unborn HEAD succeeds but creates another unborn branch, and no ref is written.
// Nothing later fills it in either:
// the clone-time commit the CLI lands through Bolt goes to the Bolt checkout's own unsuffixed branch.
// Left that way, the hub's weft primary sits on a branch that does not exist, and forking a new pair from it fails.
//
// A branch that already resolves is left untouched, so a clone of a non-empty remote is unaffected.
func bornWeftPrimaryBranch(weftPath, branch string) error {
	// A missing branch is an answer, the still-unborn case this function exists to fix; a failed read is an error.
	exists, err := gitrepo.New(weftPath).BranchExists(branch)
	if err != nil {
		return fmt.Errorf("verify weft primary branch %q: %w", branch, err)
	}
	if exists {
		return nil
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
// The clone runs via gitexec.Run:
// the parent directory of dest is the cwd, and the basename of dest is the destination argument.
// Paths are cleaned and normalized.
// A non-zero git exit at a clone is always a failure, not an answer, so there is nothing to recover with errors.As.
// The error wraps the resulting *gitexec.GitError, whose message already carries git's stderr.
func cloneRepo(url, dest string) error {
	dest = filepath.Clean(dest)
	parentDir := filepath.Dir(dest)
	parentDir = filepath.Clean(parentDir)
	destName := filepath.Base(dest)

	info, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("parent directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent directory does not exist: %s is not a directory", parentDir)
	}

	// Convert paths to forward slashes for git compatibility on Windows.
	// dest is split into parent and basename so git resolves the target cleanly on Windows.
	gitURL := filepath.ToSlash(url)
	gitDest := filepath.ToSlash(destName)

	if _, err := gitexec.Run([]string{"clone", gitURL, gitDest}, parentDir); err != nil {
		return fmt.Errorf("clone %q to %q failed: %w", url, dest, err)
	}

	return nil
}

// refuseUncheckedOutWarpClone returns an error when the freshly cloned warp worktree sits on an unborn branch.
// That is the case when the remote carries branches but its HEAD names a nonexistent ref.
//
// git clone treats such a remote as a success.
// It warns on stderr, exits 0, and leaves a worktree with no files on a branch that has no commits.
// Without this check the caller would wire every junction and record the anchor and binding.
// It would then report success for a hub that can never work.
//
// The refusal is narrow.
// An unborn warp HEAD is legitimate elsewhere.
// A warp remote with no branches at all is the empty-remote bootstrap, and the check must not touch it.
// Only the conjunction of unborn locally and at least one remote branch is refused.
//
// An enumeration failure answers no refusal, so the check never adds a new way for a clone to fail.
func refuseUncheckedOutWarpClone(warpWorktreePath, warpURL string) error {
	if _, headErr := gitrepo.New(warpWorktreePath).CurrentSHA(); headErr == nil {
		return nil
	}

	remoteBranches, listErr := gitexec.Run([]string{"branch", "--remotes", "--format=%(refname:short)"}, warpWorktreePath)
	if listErr != nil || strings.TrimSpace(remoteBranches) == "" {
		return nil
	}

	return fmt.Errorf(
		"warp repo %s cloned with nothing checked out: its default branch (remote HEAD) names a ref that does not exist, while the remote does carry branches (%s) — point the remote's HEAD at a real branch (`git remote set-head origin --auto` against a working clone, or the repo host's default-branch setting) and re-run `lym fabric clone`",
		warpURL, strings.Join(strings.Fields(remoteBranches), ", "),
	)
}

// resetHub implements --reset: it removes an existing hub at hubPath, but only once it has
// established that hubPath is one.
//
// The hub name is derived, never typed: from the warp URL in the two-argument form, and from the
// warp URL recorded in the weft's own `.lym-warp` binding in the one-argument form.
// In that form the operator never sees the name of the directory being deleted.
// A derived name says nothing about the directory it points at.
// An unconditional removal on that path could delete any directory that merely shares the name, user content and all.
//
// cwd is the parent directory the removal must stay inside.
// No location is resolved this early, so cwd is the only containment anchor available.
//
// The hub predicate is structural and cheap.
// A fabric hub always holds the Bolt checkout and at least one `*-weft` sibling.
// Either is enough.
// A hub whose Bolt checkout was hand-deleted is still recognisably a hub.
// So is one whose board is not yet materialised mid-clone.
// An absent path is a silent no-op, so --reset stays idempotent.
// rec records the mutations of the enclosing clone and is passed through to removePath.
func resetHub(rec *Mutations, cwd, hubPath string) error {
	present, err := checkResetTarget(hubPath)
	if err != nil || !present {
		return err
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

// checkResetTarget refuses a --reset target that exists but is not a fabric hub.
// It reports whether the target is present.
// It reads the filesystem only and removes nothing, so a clone can run it before touching the network.
// The hub is removed only once every argument has been checked against the remotes.
func checkResetTarget(hubPath string) (present bool, err error) {
	info, statErr := os.Stat(hubPath)
	if os.IsNotExist(statErr) {
		return false, nil
	}
	if statErr != nil {
		return false, fmt.Errorf("reset: inspect %s: %w", hubPath, statErr)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("reset: refusing to remove %s: it is not a directory, so it is not a hub", hubPath)
	}
	if !looksLikeHub(hubPath) {
		return false, fmt.Errorf(
			"reset: refusing to remove %s: it has no %s and no %q sibling, so it is not a fabric hub — remove it yourself if that is really what you meant",
			hubPath, fabriclayout.BoltDirName, "*"+weftname.Suffix)
	}
	return true, nil
}

// looksLikeHub reports whether hubPath carries the structural marks of a fabric hub:
// the Bolt checkout entry, or at least one weft sibling directory.
// An unreadable directory is not reported as a hub.
// That is the conservative direction: recursive removal is least defensible where contents cannot be enumerated.
func looksLikeHub(hubPath string) bool {
	if _, err := os.Stat(filepath.Join(hubPath, fabriclayout.BoltDirName)); err == nil {
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
		if _, ok := fabriclayout.WeftWarpSlug(entry.Name()); ok {
			return true
		}
	}
	return false
}

// teardownHub removes the hub directory and returns an error combining the cause with
// information about the failed removal (if applicable).
//
// cwd is the parent directory the removal must stay inside.
// tok is the createdToken minted when this invocation created hubPath.
// Ownership is proved by tok, not by the hub pattern.
// The earliest failures come right after the warp clone, before any board or weft sibling exists,
// so a pattern check would refuse exactly the half-built hub this teardown exists to remove.
// A token is stronger evidence than a name pattern, which a derived name can satisfy by accident.
//
// If removePath succeeds, teardownHub returns cause unchanged.
// If removePath fails, whether on an operational failure or a refusal, teardownHub returns an error.
// That error combines cause with a message about the residual hub.
// rec records the mutations of the enclosing clone and is passed through to removePath.
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

// initRecordsStore makes the one-repository records store at dir from the origin at url.
// It runs git init and adds origin with tag auto-following off.
// Before any fetch it replaces the fetch refspecs with the records namespace's branches and tags.
// It also sets the push refspecs, so the store never fetches or holds a code ref.
// With fetch it then fetches the namespaced branches and tags under the remote-refs lock.
func initRecordsStore(url, dir string, fetch bool) error {
	ns := oneRepositoryNamespace()
	remoteKey := "remote." + originRemoteName
	steps := [][]string{
		{"init"},
		{"remote", "add", "--no-tags", originRemoteName, filepath.ToSlash(url)},
		{"config", "--replace-all", remoteKey + ".fetch", "+" + ns.Heads + "*:refs/remotes/" + originRemoteName + "/*"},
		{"config", "--add", remoteKey + ".fetch", "+" + ns.Tags + "*:refs/tags/*"},
		{"config", "--replace-all", remoteKey + ".push", "refs/heads/*:" + ns.Heads + "*"},
		{"config", "--add", remoteKey + ".push", "refs/tags/*:" + ns.Tags + "*"},
	}

	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("create the records store at %s: %w", dir, err)
	}
	for _, args := range steps {
		if _, err := gitexec.Run(args, dir); err != nil {
			return fmt.Errorf("make the records store at %s: git %s: %w", dir, args[0], err)
		}
	}
	if !fetch {
		return nil
	}
	if err := gitrepo.New(dir).Fetch(); err != nil {
		return fmt.Errorf("fetch the records of %s into %s: %w", url, dir, err)
	}
	return nil
}

// readJoinedBoardRecords reads the warp binding and the shortname a joined clone's board holds at origin's board tip.
// It reads them from the records store's remote-tracking ref of the board branch.
// It reports each as found only when the record is present and usable.
// url names the origin in a warning about an unusable shortname.
func readJoinedBoardRecords(recordsPath, url string) (warp string, warpFound bool, shortname string, shortnameFound bool, err error) {
	boardRef := "refs/remotes/" + originRemoteName + "/" + fabriclayout.OneRepositoryBoardBranch
	read := func(file string) (string, bool, error) {
		listed, err := gitexec.Run([]string{"ls-tree", boardRef, "--name-only", "--", file}, recordsPath)
		if err != nil {
			return "", false, fmt.Errorf("look for %s on the board of %s: %w", file, url, err)
		}
		if strings.TrimSpace(listed) == "" {
			return "", false, nil
		}
		content, err := gitexec.Run([]string{"show", boardRef + ":" + file}, recordsPath)
		if err != nil {
			return "", false, fmt.Errorf("read %s on the board of %s: %w", file, url, err)
		}
		return content, true, nil
	}

	rawWarp, warpPresent, err := read(fabriclayout.WarpBindingFileName)
	if err != nil {
		return "", false, "", false, err
	}
	if trimmed := strings.TrimSpace(rawWarp); warpPresent && trimmed != "" {
		warp, warpFound = trimmed, true
	}
	rawShortname, shortnamePresent, err := read(fabriclayout.ShortnameFileName)
	if err != nil {
		return "", false, "", false, err
	}
	if shortnamePresent {
		shortname, shortnameFound = usableShortname(rawShortname, url)
	}
	return warp, warpFound, shortname, shortnameFound, nil
}

// PushCloneBoard pushes a clone's board to origin once its first commit is made.
// A two-repository clone and a join push it as any board is pushed.
// A SingleRepo clone's result carries the seed mark.
// Its push is the leased, ordered seed of origin's records namespace.
// A failed seed push has already removed the hub, or kept it while origin may still take a pushed ref.
// Its error names the way forward.
func PushCloneBoard(res CloneResult, rec *Mutations) error {
	if res.seed == nil {
		return NewBolt(res.BoardDir).PushRecorded(SyncOptions{}, rec)
	}
	return pushCloneSeed(rec, res.seed)
}

// AbandonSeededClone is the recovery for a SingleRepo clone that fails after CloneHub and before PushCloneBoard.
// With the seed mark it removes the hub, so no hub is left for a reconcile to push unleased.
// It returns cause with the same clone as its way forward; origin holds nothing of the seed at that point.
// Without the mark it returns cause unchanged.
func AbandonSeededClone(res CloneResult, rec *Mutations, cause error) error {
	if res.seed == nil {
		return cause
	}
	rerun := fmt.Errorf("%w; way forward: run the same `lym fabric clone --single-repo` again", cause)
	return teardownHub(rec, res.seed.container, res.seed.hubPath, res.seed.hubTok, rerun)
}

// DeriveWarpName extracts the warp repository basename from a raw URL or file path.
//
// It trims any trailing slash or backslash.
// It then takes the final segment after splitting on forward slash, backslash, and colon.
// Those separators cover HTTPS URLs, file paths, and SCP-form URLs like git@github.com:user/repo.git.
// A single trailing .git extension is stripped if present.
// It returns an empty string if no basename can be extracted or if the URL contains no path segments.
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
	rawURL = strings.TrimSuffix(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, "\\")

	// The same split handles HTTPS URLs, file paths, and SCP-form URLs.
	var parts []string
	for _, seg := range strings.FieldsFunc(rawURL, func(r rune) bool { return r == '/' || r == '\\' || r == ':' }) {
		if seg != "" {
			parts = append(parts, seg)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	return name
}
