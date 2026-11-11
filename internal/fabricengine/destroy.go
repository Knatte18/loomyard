// destroy.go holds every destructive primitive of package fabricengine.
// Each primitive is reached only through an executor, and every executor runs the gate defined here first.
// The gate checks the request shape, then containment, ownership and dirtiness, stopping at the first failure;
// --force satisfies dirtiness alone.
// Arbitrary-path acts run through an os.Root,
// and each executor appends its entry to rec *Mutations only after its primitive took effect.

package fabricengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// originRemoteName is the remote name deleteRemoteBranch always operates against.
// The package hardcodes "origin" throughout its geometry.
// A configurable remote at this one call site would be inconsistent with the rest of the package,
// and it would be untested against any other value.
const originRemoteName = "origin"

// Check names one of the three checks a destructive-gate refusal can be attributed to:
// containment, ownership, or dirtiness.
// It is string-backed so a refusal renders and marshals without a conversion step.
//
// There is deliberately no CheckForce member, and none may be added.
// Force only makes the dirtiness check pass, so no refusal can be attributed to it.
type Check string

const (
	// CheckContainment names the gate's containment check:
	// the target must resolve strictly below its declared container.
	CheckContainment Check = "containment"
	// CheckOwnership names the gate's ownership check:
	// the target must satisfy one of the closed set of ownership predicates declared for the request.
	CheckOwnership Check = "ownership"
	// CheckDirtiness names the gate's dirtiness check:
	// the target must be clean, or the caller must have passed --force.
	CheckDirtiness Check = "dirtiness"
)

// destructiveRefusal is the gate's one error type.
// It names the check that refused, the requested act, the target (a path or a branch name), and a human reason.
// Every refusal in this file is returned as *destructiveRefusal, never as a bare fmt.Errorf,
// so a caller can always test for one with errors.As.
type destructiveRefusal struct {
	Check  Check
	What   string
	Target string
	Reason string
}

// Error implements the error interface.
func (e *destructiveRefusal) Error() string {
	return fmt.Sprintf("refusing to %s: %s check failed for %s: %s", e.What, e.Check, e.Target, e.Reason)
}

// surfaceRefusal returns err unchanged when it carries a *destructiveRefusal, and nil otherwise.
// A best-effort call site wraps its executor call in surfaceRefusal.
// An operational failure (git exited nonzero, the filesystem said no) stays discardable.
// A gate refusal is never discardable.
func surfaceRefusal(err error) error {
	if errors.As(err, new(*destructiveRefusal)) {
		return err
	}
	return nil
}

// Refusal is the value form of a destructive-gate refusal:
// the check that refused, the requested act, the target, and a human reason.
// RefusalOf is the only producer of a Refusal.
// It is a value rather than *destructiveRefusal, so callers cannot switch on the identity of a mutable pointer.
type Refusal struct {
	Check  Check
	What   string
	Target string
	Reason string
}

// RefusalOf reports whether err carries a *destructiveRefusal anywhere in its chain.
// It unwraps via errors.As, so a refusal wrapped several layers deep is still found.
// It returns the zero Refusal and false for a nil error and for an error carrying no refusal.
func RefusalOf(err error) (Refusal, bool) {
	if err == nil {
		return Refusal{}, false
	}
	var refusal *destructiveRefusal
	if !errors.As(err, &refusal) {
		return Refusal{}, false
	}
	return Refusal{Check: refusal.Check, What: refusal.What, Target: refusal.Target, Reason: refusal.Reason}, true
}

// slugSpec holds the inputs of the slug check: the slug, and the junction names it is checked against.
// It is set on a pathRequest whose target is derived from a slug, and nil otherwise.
// A nil slugSpec skips the slug check entirely; the check never passes or fails vacuously.
type slugSpec struct {
	name          string
	junctionNames []string
}

// createdToken is the unforgeable proof that the gate itself created a path or a git worktree.
// Only the minting functions in this file produce one.
// Unexported-ness alone does not stop a same-package composite literal;
// the bypass guard in cmd/lym/destructiveguard_test.go bans the literal `createdToken{` outside this file.
type createdToken struct {
	path     string
	worktree bool
}

// createdBranchToken is the unforgeable proof that a `git worktree add -b <branch>` created the branch locally.
// Only the minting function in this file produces one, and only in the call that creates the branch.
// Unexported-ness alone does not stop a same-package composite literal;
// the bypass guard in cmd/lym/destructiveguard_test.go bans the literal `createdBranchToken{` outside this file.
type createdBranchToken struct {
	branch string
}

// pathRequest is the gate's request for every destructive primitive whose target is a filesystem path:
// os.RemoveAll, os.Remove, git worktree remove, ResetHard, and link removal or re-pointing.
// Every field is required: a zero-value ownership or dirtiness is refused, not passed,
// so an omitted check fails loudly instead of being forgotten.
type pathRequest struct {
	// what names the act being attempted, for the refusal message (e.g. "remove worktree").
	what string
	// container is the path that target must resolve strictly below;
	// symlinks in target's ancestry are resolved before the comparison.
	container string
	// target is the path the executor will act on.
	target string
	// slug is non-nil when target is derived from a caller-supplied slug;
	// it carries the inputs of the slug check.
	slug *slugSpec
	// ownership declares which closed-enum ownership kind target must satisfy.
	ownership pathOwnership
	// dirtiness declares which dirtiness probe (or N/A) the pipeline runs against target.
	dirtiness pathDirtiness
	// force, when true, satisfies the dirtiness check only — never containment, never ownership.
	force bool
}

// branchRequest is the gate's request for the one destructive primitive whose target is a ref: git branch -D.
// It has no container and no target field, because containment does not apply to a ref.
// The type omits them rather than having each call site pass an empty value.
// Each ownership predicate takes its inputs through its constructor.
type branchRequest struct {
	// what names the act being attempted, for the refusal message.
	what string
	// repoDir is the repository holding the branch.
	// It is the weft repository, or the warp repository for the pair's warp branch, and never a path being destroyed.
	repoDir string
	// branch is the branch name the executor will delete.
	branch string
	// ownership declares which closed-enum ownership kind branch must satisfy.
	ownership branchOwnership
	// dirtiness declares which dirtiness probe the pipeline runs against branch.
	dirtiness branchDirtiness
	// force is reserved: every construction in this package hardcodes it false.
	// No call site's gate consults it today.
	// Even a gate that consulted it could not let force answer the checked-out-branch dirtiness check;
	// force never waives that check.
	// It is kept for a future gate that consults it.
	force bool
}

// remoteBranchRequest is the gate's request for the primitive whose target is a ref on a remote:
// git push <remote> --delete.
// Local branch deletions have no remote, so a remote field would be an empty value at every local call site.
// Keeping it a distinct type keeps the call sites of the two executors disjoint and auditable.
// It has no container and no target field, because containment does not apply to a ref.
type remoteBranchRequest struct {
	// what names the act being attempted, for the refusal message.
	what string
	// repoDir is the weft repo the branch lives in — not a path being destroyed.
	repoDir string
	// remote is the remote name the branch is deleted from.
	remote string
	// branch is the branch name the executor will delete on remote.
	branch string
	// ownership declares which closed-enum ownership kind branch must satisfy.
	ownership branchOwnership
	// dirtiness declares which dirtiness probe the pipeline runs against branch.
	dirtiness branchDirtiness
	// leaseSHA is the remote tip the caller observed;
	// when non-empty the deletion succeeds only while the remote branch still sits there.
	// Empty keeps the unleased, idempotent path.
	leaseSHA string
	// force is reserved: every construction in this package hardcodes it false.
	// No call site's gate consults it today.
	// It is kept for a future gate that consults it.
	force bool
}

// remoteBranchUpdateRequest is the gate's request for the leased push that moves a task pair's own branch on a remote.
// It carries a pathRequest instead of a branch declaration:
// the push runs the containment, ownership, dirtiness and force checks over the pair's warp checkout.
type remoteBranchUpdateRequest struct {
	// pathReq is the pair-warp checkout request the pipeline runs before the push.
	pathReq pathRequest
	// repo is the pair's warp checkout, the repository the push runs from.
	repo *gitrepo.Repo
	// remote is the remote name whose branch is moved.
	remote string
	// branch is the pair's own task branch; it must still be the checkout's current branch at push time.
	branch string
	// parentBranch is the branch the pair is cut from, which is never moved.
	parentBranch string
	// sha is the commit the remote branch is moved to.
	sha string
	// leaseSHA is the remote tip the caller read; the push succeeds only while the remote branch still sits there.
	leaseSHA string
}

// pathOwnershipKind enumerates the closed set of ownership predicates a pathRequest may declare.
// Each kind is reached only through its ownedXxx constructor, never constructed directly.
type pathOwnershipKind int

const (
	// pathOwnershipUnset is the zero value: an omitted declaration, always refused.
	pathOwnershipUnset pathOwnershipKind = iota
	pathOwnershipRegisteredLinkedWorktree
	pathOwnershipWarpCheckout
	pathOwnershipFabricHub
	pathOwnershipUnderGeometryRoot
	pathOwnershipFreshlyCreatedPath
	pathOwnershipFreshlyCreatedWorktree
	pathOwnershipWiredJunction
	pathOwnershipDriftedWiredJunction
	pathOwnershipPairWarpCheckout
	pathOwnershipBoardWorktree
	pathOwnershipMovePathspecs
	pathOwnershipUntrackedTrackedAt
)

// pathOwnership declares which of the closed set of ownership kinds a pathRequest's target must satisfy.
// The zero value is invalid and is refused by the pipeline before any check runs.
// Construct one only through an ownedXxx constructor, each of which takes exactly the inputs its predicate needs.
type pathOwnership struct {
	kind           pathOwnershipKind
	repoDir        string
	root           string
	tok            createdToken
	wiredLinks     []string
	expectedTarget string
	// weftDir and parentBranch are used only by the pair warp checkout predicate.
	weftDir      string
	parentBranch string
	// pathspecs are used only by the move predicate, relative to root as the weft worktree.
	pathspecs []string
	// sha is used only by the untracked predicate: the commit that must track the target.
	sha string
}

// ownedRegisteredLinkedWorktree declares target as owned when repoDir's worktree list registers it as a linked worktree.
// The main worktree does not count: this is the ordinary teardown case.
func ownedRegisteredLinkedWorktree(repoDir string) pathOwnership {
	return pathOwnership{kind: pathOwnershipRegisteredLinkedWorktree, repoDir: repoDir}
}

// ownedWarpCheckout declares target as owned when it is any worktree of the warp repo at repoDir, prime included.
// A hard reset's ordinary target is the hub's prime warp worktree, so the prime checkout must count as owned.
func ownedWarpCheckout(repoDir string) pathOwnership {
	return pathOwnership{kind: pathOwnershipWarpCheckout, repoDir: repoDir}
}

// ownedPairWarpCheckout declares target as owned when it is a registered linked worktree of the warp repo at repoDir.
// It is never the main checkout.
// Its checked-out branch must not be parentBranch.
// The weft checkout at weftDir must have the branch that the weft records for this pair checked out,
// so that branch is the pair's own.
// A detached HEAD on either side fails the predicate.
func ownedPairWarpCheckout(repoDir, weftDir, parentBranch string) pathOwnership {
	return pathOwnership{kind: pathOwnershipPairWarpCheckout, repoDir: repoDir, weftDir: weftDir, parentBranch: parentBranch}
}

// ownedBoardWorktree declares target as owned when its base name is BoltDirName,
// and a weft repo of the same hub registers it as a linked worktree:
// some directory directly in target's parent, whose name WeftWarpSlug admits, lists target among its linked worktrees.
// A directory that merely shares the name, or the Bolt checkout of another hub, fails the predicate.
func ownedBoardWorktree() pathOwnership {
	return pathOwnership{kind: pathOwnershipBoardWorktree}
}

// ownedMovePathspecs declares target as owned when it lies at or under one of pathspecs.
// Each is a slash path relative to the weft worktree at weftPath.
// The pathspecs are the paths an anchor move rewrites: A/<names>, B/<names> and the root marker, never `.lym`.
func ownedMovePathspecs(weftPath string, pathspecs []string) pathOwnership {
	return pathOwnership{kind: pathOwnershipMovePathspecs, root: weftPath, pathspecs: pathspecs}
}

// ownedUntrackedTrackedAt declares target as owned when it lies in the worktree at worktree.
// Its index must not track the target, and the commit sha must.
// It serves a move to sha that git refuses while such an untracked file stands where sha puts its own.
func ownedUntrackedTrackedAt(worktree, sha string) pathOwnership {
	return pathOwnership{kind: pathOwnershipUntrackedTrackedAt, root: worktree, sha: sha}
}

// ownedFabricHub declares target as owned when it structurally looks like a fabric hub:
// the Bolt checkout entry, or at least one weft sibling, per looksLikeHub.
func ownedFabricHub() pathOwnership {
	return pathOwnership{kind: pathOwnershipFabricHub}
}

// ownedUnderGeometryRoot declares target as owned when root is a member of the closed set of fabric geometry roots.
// It admits a target that resolves at or below root, including deep descendants and non-directory targets.
// It supplies what containment cannot: containment proves target is below the container,
// but proves nothing if the caller chose the container.
func ownedUnderGeometryRoot(root string) pathOwnership {
	return pathOwnership{kind: pathOwnershipUnderGeometryRoot, root: root}
}

// ownedFreshlyCreatedPath declares target as owned when tok, minted by createExclusiveDir, covers exactly this path.
// Since createdToken has no producer outside this file's two minters (backed by the bypass guard);
// a site cannot declare this kind for a path the gate did not create.
func ownedFreshlyCreatedPath(tok createdToken) pathOwnership {
	return pathOwnership{kind: pathOwnershipFreshlyCreatedPath, tok: tok}
}

// ownedFreshlyCreatedWorktree declares target as owned when tok is the token createGitWorktree minted for this path.
// It serves the rollback, in add.go, of a worktree that the same Add call created.
func ownedFreshlyCreatedWorktree(tok createdToken) pathOwnership {
	return pathOwnership{kind: pathOwnershipFreshlyCreatedWorktree, tok: tok}
}

// ownedWiredJunction declares target as owned when it is a member of wiredLinks, is itself a link,
// and resolves to expectedTarget, the teardown-shaped link check.
// Comparing the resolved target separates a link fabric wired from a user's own tracked symlink at the same path:
// the user's symlink is a link too, but it does not resolve to what fabric wired there.
func ownedWiredJunction(wiredLinks []string, expectedTarget string) pathOwnership {
	return pathOwnership{kind: pathOwnershipWiredJunction, wiredLinks: wiredLinks, expectedTarget: expectedTarget}
}

// ownedDriftedWiredJunction declares target as owned when it is a member of wiredLinks and is itself a link.
// The resolved target is not compared.
// A drifted or dangling target is the precondition for repairing it, so it must not disqualify the link.
func ownedDriftedWiredJunction(wiredLinks []string) pathOwnership {
	return pathOwnership{kind: pathOwnershipDriftedWiredJunction, wiredLinks: wiredLinks}
}

// branchOwnershipKind enumerates the closed set of ownership predicates a branchRequest may declare.
type branchOwnershipKind int

const (
	// branchOwnershipUnset is the zero value: an omitted declaration, always refused.
	branchOwnershipUnset branchOwnershipKind = iota
	branchOwnershipManaged
	branchOwnershipPairWarp
	branchOwnershipPairWeft
	branchOwnershipCreatedBranch
)

// branchOwnership declares which of the closed set of ownership kinds a branchRequest's branch must satisfy.
// The zero value is invalid and is refused by the pipeline before any check runs.
type branchOwnership struct {
	kind     branchOwnershipKind
	location *lymcwd.Location
	// warpBranch serves ownedPairWarpBranch and ownedPairWeftBranch;
	// parentBranch serves ownedPairWarpBranch only.
	warpBranch   string
	parentBranch string
	// createdBranch serves ownedCreatedBranch only: the one branch name its proof covers.
	createdBranch string
}

// ownedCreatedBranch declares branch as owned when it is exactly the branch tok proves this call created.
// It consults no naming scheme,
// so a bare-slug warp branch under the default empty branch_prefix is as deletable as a prefixed one.
// l serves the checked-out dirtiness probe.
// An empty proof matches nothing.
func ownedCreatedBranch(l *lymcwd.Location, tok createdBranchToken) branchOwnership {
	return branchOwnership{kind: branchOwnershipCreatedBranch, location: l, createdBranch: tok.branch}
}

// ownedPairWarpBranch declares branch as owned when it is exactly warpBranch, the branch the pair stands on.
// Its warpBranch must not be parentBranch, the branch the pair lands on.
// This kind consults no weft-side predicate, because the warp branch can be the bare slug those predicates reject.
// The checked-out question belongs to the dirtiness step.
// An empty warpBranch matches nothing.
func ownedPairWarpBranch(warpBranch, parentBranch string) branchOwnership {
	return branchOwnership{kind: branchOwnershipPairWarp, warpBranch: warpBranch, parentBranch: parentBranch}
}

// ownedPairWeftBranch declares branch as owned when it is exactly RecordsBranchName(warpBranch).
// It must also be accepted by WeftWarpSlug and must not be l's primary weft branch.
// It has no checked-out test: while Add builds the pair, the same-named local branch is the replacement.
// That branch is checked out at the new weft worktree, so a checked-out test would refuse it.
// An empty warpBranch matches nothing.
func ownedPairWeftBranch(l *lymcwd.Location, warpBranch string) branchOwnership {
	return branchOwnership{kind: branchOwnershipPairWeft, location: l, warpBranch: warpBranch}
}

// ownedManagedBranch declares a records branch as owned when WeftWarpSlug accepts it.
// It is not l's primary weft branch, and no worktree has it checked out.
// It consults no branch prefix,
// so a pair made under an earlier prefix stays as owned as one made under the current prefix.
// It takes a *lymcwd.Location because primaryWeftBranch(l) needs it.
func ownedManagedBranch(l *lymcwd.Location) branchOwnership {
	return branchOwnership{kind: branchOwnershipManaged, location: l}
}

// pathDirtinessKind enumerates the closed set of dirtiness declarations a pathRequest may carry.
type pathDirtinessKind int

const (
	// pathDirtinessUnset is the zero value: an omitted declaration, always refused.
	pathDirtinessUnset pathDirtinessKind = iota
	pathDirtinessScope
	pathDirtinessNA
	pathDirtinessTrackedExcept
	pathDirtinessTrackedOn
)

// pathDirtiness declares which dirtiness probe, or N/A, the pipeline runs against a pathRequest's target.
// The zero value is invalid, and so is dirtinessNA(""): an empty reason is a refusal, not a pass.
type pathDirtiness struct {
	kind   pathDirtinessKind
	scope  dirtyScope
	reason string
	// ownPaths serves dirtyTrackedExcept only: worktree-relative, slash-separated.
	ownPaths []string
	// onPaths serves dirtyTrackedOn only: worktree-relative, slash-separated.
	onPaths []string
}

// dirtyTrackedOn declares that the pipeline's dirtiness step probes tracked files only.
// It refuses only on a dirty tracked path in paths (worktree-relative, slash-separated).
// A dirty path anywhere else is carried across by the act and never refuses.
// The refusal names each such path.
func dirtyTrackedOn(paths []string) pathDirtiness {
	return pathDirtiness{kind: pathDirtinessTrackedOn, scope: scopeTracked, onPaths: paths}
}

// dirtyTrackedExcept declares that the pipeline's dirtiness step probes tracked files only.
// It refuses only on a dirty tracked path outside ownPaths (worktree-relative, slash-separated).
// The refusal names each such path.
func dirtyTrackedExcept(ownPaths []string) pathDirtiness {
	return pathDirtiness{kind: pathDirtinessTrackedExcept, scope: scopeTracked, ownPaths: ownPaths}
}

// dirtyScopeTracked declares that the pipeline's dirtiness step probes tracked files only.
// The probe is worktreeDirty(scopeTracked, target).
// It is the right scope wherever the destructive action leaves untracked files alone, for example a reset --hard.
func dirtyScopeTracked() pathDirtiness {
	return pathDirtiness{kind: pathDirtinessScope, scope: scopeTracked}
}

// dirtyScopeAll declares that the pipeline's dirtiness step probes tracked and untracked files alike.
// The probe is worktreeDirty(scopeAll, target).
// It is the right scope wherever the destructive action would take untracked files down with it.
func dirtyScopeAll() pathDirtiness {
	return pathDirtiness{kind: pathDirtinessScope, scope: scopeAll}
}

// dirtinessNA declares that the dirtiness check does not apply to this pathRequest, for the stated reason.
// For example, a rollback site tearing down a path this same transaction created has no work to lose.
// Nothing was ever committed to that path.
// reason must be non-empty: an empty reason is refused by the pipeline as an unstated N/A, not treated as a pass.
func dirtinessNA(reason string) pathDirtiness {
	return pathDirtiness{kind: pathDirtinessNA, reason: reason}
}

// branchDirtinessKind enumerates the closed set of dirtiness declarations a branchRequest may carry.
type branchDirtinessKind int

const (
	// branchDirtinessUnset is the zero value: an omitted declaration, always refused.
	branchDirtinessUnset branchDirtinessKind = iota
	branchDirtinessCheckedOutBranch
	branchDirtinessUnlandedWork
	branchDirtinessArchivedOnRemote
	branchDirtinessUnlandedRemoteTip
	branchDirtinessArchivedTip
)

// branchDirtiness declares which dirtiness probe the pipeline runs against a branch.
// The zero value is invalid and is refused by the pipeline before any check runs.
type branchDirtiness struct {
	kind branchDirtinessKind
	// parentBranch is the branch unlanded work must have landed on; empty when the pair has no origin record.
	parentBranch string
	// archiveTag is the archive/<slug>/* tag the pre-flight proved covers the branch's tip.
	archiveTag string
}

// dirtyArchivedTip declares that the local branch's tip is covered by archiveTag.
// The tag is an archive/<slug>/* tag whose target the branch's current tip is an ancestor of or equal to.
// Deleting the branch loses no work, even when the branch is unlanded.
// It applies to local branches only, and an empty archiveTag is refused.
func dirtyArchivedTip(archiveTag string) branchDirtiness {
	return branchDirtiness{kind: branchDirtinessArchivedTip, archiveTag: archiveTag}
}

// dirtyArchivedOnRemote declares that the remote branch's tip is covered by archiveTag.
// archiveTag is an archive/<slug>/* tag the caller's pre-flight proved reaches the tip.
// Deleting the remote branch loses no work.
// It applies to remote branches only, and an empty archiveTag is refused.
func dirtyArchivedOnRemote(archiveTag string) branchDirtiness {
	return branchDirtiness{kind: branchDirtinessArchivedOnRemote, archiveTag: archiveTag}
}

// dirtyUnlandedWork declares that the dirtiness step refuses a branch whose work would be lost.
// The branch must not be checked out,
// and every commit on it must be reachable from another ref or already landed on parentBranch.
// parentBranch is empty when the pair has no origin record.
func dirtyUnlandedWork(parentBranch string) branchDirtiness {
	return branchDirtiness{kind: branchDirtinessUnlandedWork, parentBranch: parentBranch}
}

// dirtyUnlandedRemoteTip declares that the dirtiness step refuses a remote branch's leased tip.
// The tip is safe when every commit on it is reachable from a ref other than a copy of the same branch.
// It is also safe when merging it into parentBranch changes nothing.
// parentBranch is empty when the pair has no origin record.
// It applies to remote branches only, and requires a non-empty leaseSHA, the tip under test.
func dirtyUnlandedRemoteTip(parentBranch string) branchDirtiness {
	return branchDirtiness{kind: branchDirtinessUnlandedRemoteTip, parentBranch: parentBranch}
}

// dirtyCheckedOutBranch declares that the dirtiness step asks whether the branch is checked out at any worktree.
// A branch checked out somewhere counts as holding work to lose.
// git branch -D cannot delete a checked-out branch anyway.
func dirtyCheckedOutBranch() branchDirtiness {
	return branchDirtiness{kind: branchDirtinessCheckedOutBranch}
}

// resolvePathOwnership dispatches own's kind to its predicate and reports whether target satisfies it.
// Every predicate answers false when an enumeration fails, the conservative direction.
func resolvePathOwnership(own pathOwnership, target string) (ok bool, reason string) {
	switch own.kind {
	case pathOwnershipRegisteredLinkedWorktree:
		if !isRegisteredLinkedWorktreeIn(own.repoDir, target) {
			return false, fmt.Sprintf("%s is not a registered linked worktree of %s", target, own.repoDir)
		}
		return true, ""

	case pathOwnershipWarpCheckout:
		if !isAnyWorktreeOf(own.repoDir, target) {
			return false, fmt.Sprintf("%s is not a worktree of the warp repo at %s", target, own.repoDir)
		}
		return true, ""

	case pathOwnershipPairWarpCheckout:
		return resolvePairWarpCheckout(own, target)

	case pathOwnershipBoardWorktree:
		if !isHubBoardWorktree(target) {
			return false, fmt.Sprintf("%s is not the %s worktree of a weft repo in its hub", target, BoltDirName)
		}
		return true, ""

	case pathOwnershipMovePathspecs:
		rel, err := filepath.Rel(filepath.Clean(own.root), filepath.Clean(target))
		if err != nil {
			return false, fmt.Sprintf("relate %s to the weft worktree %s: %v", target, own.root, err)
		}
		rel = filepath.ToSlash(rel)
		for _, spec := range own.pathspecs {
			spec = pathpkg.Clean(spec)
			if rel == spec || strings.HasPrefix(rel, spec+"/") {
				return true, ""
			}
		}
		return false, fmt.Sprintf("%s lies under none of the paths the anchor move rewrites", target)

	case pathOwnershipUntrackedTrackedAt:
		rel, err := filepath.Rel(filepath.Clean(own.root), filepath.Clean(target))
		if err != nil {
			return false, fmt.Sprintf("relate %s to the worktree %s: %v", target, own.root, err)
		}
		rel = filepath.ToSlash(rel)
		tracked, err := PathTracked(own.root, rel)
		if err != nil {
			return false, err.Error()
		}
		if tracked {
			return false, fmt.Sprintf("%s is tracked in %s", target, own.root)
		}
		if _, err := gitrepo.New(own.root).FileAtRevision(own.sha, rel); err != nil {
			return false, fmt.Sprintf("%s is not tracked at %s: %v", rel, own.sha, err)
		}
		return true, ""

	case pathOwnershipFabricHub:
		if !looksLikeHub(target) {
			return false, fmt.Sprintf("%s does not look like a fabric hub (no %s entry and no weft sibling)", target, BoltDirName)
		}
		return true, ""

	case pathOwnershipUnderGeometryRoot:
		// A fabric geometry root is a directory whose base name is launchersDirName or portalsDirName.
		// A root outside that set is refused, so a call site cannot satisfy this check by naming a convenient parent.
		//
		// The base-name test runs against the resolved root, not the nominal one, as pathAtOrBelow does.
		// A link standing where the geometry root belongs is not the geometry root.
		// Testing the nominal spelling would let it masquerade as one.
		resolvedRoot := resolveAncestorSymlinks(own.root)
		if base := filepath.Base(resolvedRoot); base != launchersDirName && base != portalsDirName {
			return false, fmt.Sprintf("%s is not a fabric geometry root", own.root)
		}
		if !pathAtOrBelow(own.root, target) {
			return false, fmt.Sprintf("%s does not resolve at or below geometry root %s", target, own.root)
		}
		return true, ""

	case pathOwnershipFreshlyCreatedPath:
		if own.tok.worktree || filepath.Clean(target) != own.tok.path {
			return false, "target does not match a token createExclusiveDir minted for it"
		}
		return true, ""

	case pathOwnershipFreshlyCreatedWorktree:
		if !own.tok.worktree || filepath.Clean(target) != own.tok.path {
			return false, "target does not match a token createGitWorktree minted for it"
		}
		return true, ""

	case pathOwnershipWiredJunction:
		if !containsCleanPath(own.wiredLinks, target) {
			return false, fmt.Sprintf("%s is not one of fabric's wired junction paths", target)
		}
		isLink, err := fslink.IsLink(target)
		if err != nil || !isLink {
			return false, fmt.Sprintf("%s is not a link", target)
		}
		// The comparison uses RawTarget, not PointsTo.
		// It runs against the one hop a wiring call recorded.
		// PointsTo resolves the whole chain, and it fails outright when a later segment is gone.
		// That happens after a stale-pair prune removes the warp side a wired junction chains through.
		// PointsTo also walks past a target that is itself a symlink, such as a warp `_lym` that points at weft,
		// so it compares an end state a one-hop expectedTarget was never meant to match.
		rawTarget, err := fslink.RawTarget(target)
		if err != nil {
			return false, fmt.Sprintf("read link target of %s: %v", target, err)
		}
		if filepath.Clean(rawTarget) != filepath.Clean(own.expectedTarget) {
			return false, fmt.Sprintf("%s points to %s, not the expected %s", target, rawTarget, own.expectedTarget)
		}
		return true, ""

	case pathOwnershipDriftedWiredJunction:
		// The resolved target is not compared.
		// Drift, or a dangling target, is the precondition for repairing this link, not a disqualifier.
		if !containsCleanPath(own.wiredLinks, target) {
			return false, fmt.Sprintf("%s is not one of fabric's wired junction paths", target)
		}
		isLink, err := fslink.IsLink(target)
		if err != nil || !isLink {
			return false, fmt.Sprintf("%s is not a link", target)
		}
		return true, ""

	default:
		return false, "no ownership kind declared"
	}
}

// resolvePairWarpCheckout reports whether target is the pair's own warp checkout, with a reason when it is not.
// The linked-worktree test runs first, so the prime checkout refuses without a branch read.
// A branch read that fails, a detached HEAD included, refuses too, the conservative direction.
func resolvePairWarpCheckout(own pathOwnership, target string) (bool, string) {
	if !isRegisteredLinkedWorktreeIn(own.repoDir, target) {
		return false, fmt.Sprintf("%s is not a registered linked worktree of %s", target, own.repoDir)
	}
	branch, err := gitrepo.New(target).CurrentBranch()
	if err != nil {
		return false, fmt.Sprintf("cannot read the branch checked out at %s: %v", target, err)
	}
	if ok, reason := resolvePairWarpBranch(branch, own.parentBranch, branch); !ok {
		return false, reason
	}
	weftBranch, err := gitrepo.New(own.weftDir).CurrentBranch()
	if err != nil {
		return false, fmt.Sprintf("cannot read the branch checked out at the pair's weft %s: %v", own.weftDir, err)
	}
	if weftBranch != RecordsBranchName(branch) {
		return false, fmt.Sprintf("the pair's weft %s has %q checked out, not %q, so %s is not the pair's own warp branch", own.weftDir, weftBranch, RecordsBranchName(branch), branch)
	}
	return true, ""
}

// isHubBoardWorktree reports whether target is the worktree named BoltDirName of a weft repo in its hub.
// An unreadable hub directory answers false, the conservative direction.
func isHubBoardWorktree(target string) bool {
	if filepath.Base(target) != BoltDirName {
		return false
	}
	hub := filepath.Dir(target)
	entries, err := os.ReadDir(hub)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := WeftWarpSlug(entry.Name()); !ok {
			continue
		}
		if isRegisteredLinkedWorktreeIn(filepath.Join(hub, entry.Name()), target) {
			return true
		}
	}
	return false
}

// isAnyWorktreeOf reports whether target is any worktree of the repo at repoDir, the prime checkout included.
// The main entry counts too, since the hub's prime warp worktree is an ordinary reset target and must pass.
// repoDir names whichever repo the caller checks, so the test works for any repo.
// An unenumerable repo answers false, the conservative direction.
func isAnyWorktreeOf(repoDir, target string) bool {
	entries, err := List(repoDir)
	if err != nil {
		return false
	}
	cleanTarget := filepath.Clean(target)
	for _, entry := range entries {
		if filepath.Clean(filepath.FromSlash(entry.Path)) == cleanTarget {
			return true
		}
	}
	return false
}

// pathAtOrBelow reports whether target resolves at or below root, admitting deep descendants and
// non-directory targets alike.
//
// Both paths are resolved before they are compared.
// A symlink planted in an intermediate component must not let a target outside root compare as inside it.
// This ownership kind has no independent record to cross-check its paths against.
// Resolution is therefore its only guard.
func pathAtOrBelow(root, target string) bool {
	rel, err := filepath.Rel(resolveAncestorSymlinks(root), containmentPath(target))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// containsCleanPath reports whether target equals one of paths, after filepath.Clean is applied to both.
func containsCleanPath(paths []string, target string) bool {
	cleanTarget := filepath.Clean(target)
	for _, p := range paths {
		if filepath.Clean(p) == cleanTarget {
			return true
		}
	}
	return false
}

// resolveBranchOwnership dispatches own's kind to its predicate and reports whether branch satisfies it.
func resolveBranchOwnership(own branchOwnership, branch string) (ok bool, reason string) {
	switch own.kind {
	case branchOwnershipManaged:
		return resolveManagedBranch(own.location, branch)
	case branchOwnershipPairWarp:
		return resolvePairWarpBranch(own.warpBranch, own.parentBranch, branch)
	case branchOwnershipPairWeft:
		return resolvePairWeftBranch(own.location, own.warpBranch, branch)
	case branchOwnershipCreatedBranch:
		if own.createdBranch == "" || branch != own.createdBranch {
			return false, fmt.Sprintf("%s is not the branch this call created (%q)", branch, own.createdBranch)
		}
		return true, ""
	default:
		return false, "no ownership kind declared"
	}
}

// resolvePairWarpBranch reports whether branch is the pair's own warp branch.
// branch must equal warpBranch, and warpBranch must not equal parentBranch.
func resolvePairWarpBranch(warpBranch, parentBranch, branch string) (bool, string) {
	if warpBranch == "" || branch != warpBranch {
		return false, fmt.Sprintf("%s is not the pair's own warp branch %q", branch, warpBranch)
	}
	if parentBranch != "" && warpBranch == parentBranch {
		return false, fmt.Sprintf("%s is the branch the pair lands on", branch)
	}
	return true, ""
}

// resolvePairWeftBranch reports whether branch is the pair's own weft branch.
// The two pure name checks run first, so a mismatched name refuses without spawning git;
// then branch must not be the primary weft branch of l, and an unreadable primary refuses.
func resolvePairWeftBranch(l *lymcwd.Location, warpBranch, branch string) (bool, string) {
	if warpBranch == "" || branch != RecordsBranchName(warpBranch) {
		return false, fmt.Sprintf("%s is not the pair's own weft branch for %q", branch, warpBranch)
	}
	if _, ok := WeftWarpSlug(branch); !ok {
		return false, fmt.Sprintf("%s is not a name fabric's own scheme constructs", branch)
	}
	primary, err := primaryWeftBranch(l)
	if err != nil {
		return false, fmt.Sprintf("cannot determine the repo's primary weft branch: %v", err)
	}
	if branch == primary {
		return false, fmt.Sprintf("%s is the repo's primary weft branch", branch)
	}
	return true, ""
}

// resolveManagedBranch reports whether branch may be deleted as a fabric-managed weft branch.
// branch must be a name the fabric scheme constructs, must not be the primary weft branch of l,
// and no worktree may have it checked out.
// An unreadable primary refuses rather than proceeds, since every branch deletion is irreversible.
func resolveManagedBranch(l *lymcwd.Location, branch string) (bool, string) {
	if _, weftManaged := WeftWarpSlug(branch); !weftManaged {
		return false, fmt.Sprintf("%s is not a name fabric's own scheme constructs", branch)
	}

	primary, err := primaryWeftBranch(l)
	if err != nil {
		return false, fmt.Sprintf("cannot determine the repo's primary weft branch: %v", err)
	}
	if branch == primary {
		return false, fmt.Sprintf("%s is the repo's primary weft branch", branch)
	}

	branches, err := listWeftBranches(l)
	if err != nil {
		return false, fmt.Sprintf("cannot enumerate weft branches: %v", err)
	}
	for _, b := range branches {
		if b.Branch == branch && b.WorktreePath != "" {
			return false, fmt.Sprintf("%s is checked out at %s", branch, b.WorktreePath)
		}
	}
	return true, ""
}

// checkPathRequest runs the gate's four checks against req, in fixed order, stopping at the first
// failure: containment, ownership, dirtiness, force.
//
// The request's shape is validated first, ahead of the absent-target rule.
// A missing declaration is refused whether or not the target exists, and validating a struct costs no syscall.
//
// An absent target is then a no-op success before the four checks run.
// Most ownership predicates fail on a path that is not there.
// The removals that tolerate an absent path are idempotent, so refusing here would make them hard failures.
// os.Lstat, not os.Stat, decides "absent": a dangling link is present as a link even though its target is not.
// The drifted-junction ownership check must still see such a link.
func checkPathRequest(req pathRequest) error {
	if req.ownership.kind == pathOwnershipUnset {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.target, Reason: "no ownership kind declared"}
	}
	if req.dirtiness.kind == pathDirtinessUnset {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "no dirtiness declared"}
	}
	if req.dirtiness.kind == pathDirtinessNA && req.dirtiness.reason == "" {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "dirtinessNA requires a non-empty reason"}
	}

	if _, statErr := os.Lstat(req.target); os.IsNotExist(statErr) {
		return nil
	}

	if req.slug != nil {
		if err := validateWorktreeSlug(req.slug.name, req.slug.junctionNames); err != nil {
			return &destructiveRefusal{Check: CheckContainment, What: req.what, Target: req.target, Reason: err.Error()}
		}
	}

	if err := containmentFailure(req.container, req.target); err != nil {
		return &destructiveRefusal{Check: CheckContainment, What: req.what, Target: req.target, Reason: err.Error()}
	}

	if ok, reason := resolvePathOwnership(req.ownership, req.target); !ok {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.target, Reason: reason}
	}

	return checkPathDirtiness(req)
}

// checkPathDirtiness runs the pipeline's dirtiness step for a pathRequest.
//
// req.force is consulted only here, and nowhere else in the pipeline: it satisfies dirtiness and
// nothing else, never containment and never ownership, which is what keeps "remove .." — a
// containment failure — from ever being answerable by a flag.
// A dirtinessNA declaration always passes, because a declaration without a reason is refused before this step.
// A probe that cannot run at all is itself a refusal, not a pass, since a probe failure is exactly the
// state in which an unconditional destructive act is least defensible.
//
// The probe is not atomic with the act it gates.
// This function runs `git status --porcelain` and returns; the executor then performs the primitive.
// No lock spans the two, so a write landing in that window is destroyed.
// The exposure stays narrow: removeGitWorktree re-checks through git itself, and resetHardTo delegates to git.
// The only RemoveAll sites with a real dirtiness scope are two fallbacks,
// and each fires only after git has declined the removal.
// Closing the window would need a lock held across the probe and the act at every executor.
// That costs more than the residual risk warrants.
// Saying the gate executes rather than approves does not make the probe and the act one transaction.
func checkPathDirtiness(req pathRequest) error {
	if req.dirtiness.kind == pathDirtinessNA {
		return nil
	}
	if req.force {
		return nil
	}

	dirty, detail, err := worktreeDirty(req.dirtiness.scope, req.target)
	if err != nil {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: err.Error()}
	}
	if dirty && req.dirtiness.kind == pathDirtinessTrackedExcept {
		foreign := dirtyPathsOutside(detail, req.dirtiness.ownPaths)
		if len(foreign) == 0 {
			return nil
		}
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "tracked changes outside the run's own paths: " + strings.Join(foreign, ", ")}
	}
	if dirty && req.dirtiness.kind == pathDirtinessTrackedOn {
		var touched []string
		for _, path := range dirtyPathsOutside(detail, nil) {
			if slices.Contains(req.dirtiness.onPaths, pathpkg.Clean(path)) {
				touched = append(touched, path)
			}
		}
		if len(touched) == 0 {
			return nil
		}
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "tracked changes on paths the act rewrites: " + strings.Join(touched, ", ")}
	}
	if dirty {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "worktree has uncommitted changes; use --force"}
	}
	return nil
}

// dirtyPathsOutside returns the paths in a `git status --porcelain` listing that are not in ownPaths.
// The listing arrives trimmed, so its first line may have lost the leading status space.
// Each line is read as status letters, spaces, then the path; a rename reports its new path.
func dirtyPathsOutside(porcelain string, ownPaths []string) []string {
	own := make(map[string]bool, len(ownPaths))
	for _, p := range ownPaths {
		own[pathpkg.Clean(filepath.ToSlash(p))] = true
	}
	var foreign []string
	for _, line := range strings.Split(porcelain, "\n") {
		path := strings.TrimLeft(strings.TrimLeft(strings.TrimLeft(line, " "), "MTADRCU?!"), " ")
		if _, renamed, ok := strings.Cut(path, " -> "); ok {
			path = renamed
		}
		if unquoted, err := strconv.Unquote(path); err == nil {
			path = unquoted
		}
		if path == "" || own[pathpkg.Clean(path)] {
			continue
		}
		foreign = append(foreign, path)
	}
	return foreign
}

// checkBranchRequest runs the gate's checks against req for a branch.
// It skips the containment and absent-target rules, which have no meaning for a ref.
func checkBranchRequest(req branchRequest) error {
	if req.ownership.kind == branchOwnershipUnset {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: "no ownership kind declared"}
	}
	if req.dirtiness.kind == branchDirtinessUnset {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "no dirtiness declared"}
	}

	if ok, reason := resolveBranchOwnership(req.ownership, req.branch); !ok {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: reason}
	}

	return checkBranchDirtiness(req)
}

// checkBranchDirtiness runs the dirtiness step for a branchRequest.
// It refuses a branch checked out at any worktree: git branch -D cannot delete a checked-out branch,
// so this names the refusal at the gate instead of leaving git's error.
// req.force is never consulted, since --force may answer a call site's own gate but never this check.
//
// The unlanded-work kind also refuses a branch whose commits no other ref holds and its parent does not contain.
func checkBranchDirtiness(req branchRequest) error {
	if req.dirtiness.kind == branchDirtinessArchivedOnRemote {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "archived-on-remote dirtiness answers a remote question; a local branch delete has no remote tip to prove"}
	}
	if req.dirtiness.kind == branchDirtinessUnlandedRemoteTip {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "unlanded-remote-tip dirtiness answers a remote question; a local branch delete has no remote tip to probe"}
	}
	if req.dirtiness.kind == branchDirtinessArchivedTip {
		return checkArchivedTip(req)
	}
	if req.dirtiness.kind == branchDirtinessUnlandedWork {
		return checkUnlandedWork(req)
	}
	return checkedOutBranchDirtiness(req.ownership.location, req.what, req.branch)
}

// checkArchivedTip refuses a branch whose tip the archive tag does not cover.
// It runs read-only git from req.repoDir.
// The tag must be named, and the branch tip must be an ancestor of or equal to the tag's target.
// req.force is never consulted.
func checkArchivedTip(req branchRequest) error {
	refuse := func(reason string) error {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: reason}
	}

	tag := req.dirtiness.archiveTag
	if tag == "" {
		return refuse("the branch tip is covered by no archive tag")
	}
	tip, err := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + req.branch + "^{commit}"}, req.repoDir)
	if err != nil {
		return refuse(fmt.Sprintf("cannot resolve the branch tip: %v", err))
	}
	tip = strings.TrimSpace(tip)
	covered, err := isAncestorOrEqual(req.repoDir, tip, "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return refuse(fmt.Sprintf("cannot compare the branch tip %s with archive tag %q: %v", tip, tag, err))
	}
	if !covered {
		return refuse(fmt.Sprintf("the branch tip %s is not covered by archive tag %q", tip, tag))
	}
	return nil
}

// checkUnlandedRemoteTip refuses a remote tip whose commits no other ref holds and its parent does not contain.
// It runs read-only git from req.repoDir against the leased tip, which must already be fetched.
// The tip passes when every commit on it is reachable from another ref;
// otherwise, with a parent, it passes when merging it into the parent changes nothing.
// For a remote tip, "another ref" excludes every copy of the same branch, local and remote-tracking.
// A tracking ref always holds the tip, so without that exclusion every pushed branch would look held.
// git clears accumulated --exclude patterns after each ref-selecting option,
// so the exclusion is repeated ahead of each.
// req.force is never consulted.
func checkUnlandedRemoteTip(req remoteBranchRequest) error {
	refuse := func(reason string) error {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: reason}
	}

	tip := req.leaseSHA
	out, err := gitexec.Run([]string{
		"rev-list", "--count", tip, "--not",
		"--exclude=refs/heads/" + req.branch, "--glob=refs/heads/*",
		"--exclude=*/" + req.branch, "--remotes",
		"--tags",
	}, req.repoDir)
	if err != nil {
		return refuse(fmt.Sprintf("cannot count commits reachable from no other ref: %v", err))
	}
	count := strings.TrimSpace(out)
	if count == "0" {
		return nil
	}

	if parent := req.dirtiness.parentBranch; parent != "" {
		merged, mergeErr := gitexec.Run([]string{"merge-tree", "--write-tree", parent, tip}, req.repoDir)
		parentTree, treeErr := gitrepo.New(req.repoDir).RefTree(parent)
		if mergeErr == nil && treeErr == nil {
			mergedTree, _, _ := strings.Cut(strings.TrimSpace(merged), "\n")
			if mergedTree != "" && mergedTree == parentTree {
				return nil
			}
		}
	}

	return refuse(fmt.Sprintf("%s commit(s) on the remote branch are reachable from no other ref and not landed on the parent; if the work is disposable, delete it by hand with: git push %s --delete %s", count, req.remote, req.branch))
}

// checkUnlandedWork refuses a local branch with commits that neither another ref nor its parent already holds.
// It runs read-only git from req.repoDir.
// The branch must not be checked out.
// It passes when every commit on it is reachable from another local branch, a remote-tracking ref or a tag.
// Otherwise, with a parent, it passes when merging the branch into the parent changes nothing:
// git merge-tree yields the parent tip's own tree, and a conflict counts as a change.
// Otherwise it refuses, naming the count of commits held by no other ref and a manual remedy.
// req.force is never consulted.
func checkUnlandedWork(req branchRequest) error {
	refuse := func(reason string) error {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: reason}
	}

	out, err := gitexec.Run([]string{"branch", "--format=%(refname:short)\x1f%(worktreepath)"}, req.repoDir)
	if err != nil {
		return refuse(fmt.Sprintf("list branches failed: %v", err))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, worktreePath, _ := strings.Cut(strings.TrimSpace(line), "\x1f")
		if name == req.branch && strings.TrimSpace(worktreePath) != "" {
			return refuse(fmt.Sprintf("branch is checked out at %s", strings.TrimSpace(worktreePath)))
		}
	}

	ref := "refs/heads/" + req.branch
	out, err = gitexec.Run([]string{"rev-list", "--count", ref, "--not", "--exclude=" + ref, "--glob=refs/heads/*", "--remotes", "--tags"}, req.repoDir)
	if err != nil {
		return refuse(fmt.Sprintf("cannot count commits reachable from no other ref: %v", err))
	}
	count := strings.TrimSpace(out)
	if count == "0" {
		return nil
	}

	if parent := req.dirtiness.parentBranch; parent != "" {
		merged, mergeErr := gitexec.Run([]string{"merge-tree", "--write-tree", parent, ref}, req.repoDir)
		parentTree, treeErr := gitrepo.New(req.repoDir).RefTree(parent)
		if mergeErr == nil && treeErr == nil {
			mergedTree, _, _ := strings.Cut(strings.TrimSpace(merged), "\n")
			if mergedTree != "" && mergedTree == parentTree {
				return nil
			}
		}
	}

	return refuse(fmt.Sprintf("%s commit(s) on this branch are reachable from no other ref and not landed on the parent; if the work is disposable, delete it by hand with: git branch -D %s", count, req.branch))
}

// checkedOutBranchDirtiness refuses a branch that is checked out at any worktree, as a named gate refusal.
// git branch -D cannot delete a checked-out branch;
// this check turns git's own refusal into a refusal that names the check.
// The three values it takes are everything the probe needs.
func checkedOutBranchDirtiness(l *lymcwd.Location, what, branch string) error {
	branches, err := listWeftBranches(l)
	if err != nil {
		return &destructiveRefusal{Check: CheckDirtiness, What: what, Target: branch, Reason: err.Error()}
	}
	for _, b := range branches {
		if b.Branch == branch && b.WorktreePath != "" {
			return &destructiveRefusal{Check: CheckDirtiness, What: what, Target: branch, Reason: fmt.Sprintf("branch is checked out at %s", b.WorktreePath)}
		}
	}
	return nil
}

// checkRemoteBranchRequest runs the gate's checks against req for a remote branch delete.
// An unset ownership or dirtiness declaration is refused before any check runs.
// Then ownership is resolved, and the dirtiness check for the declared kind runs.
// Containment does not apply, since a remote branch has no container field to check.
func checkRemoteBranchRequest(req remoteBranchRequest) error {
	if req.ownership.kind == branchOwnershipUnset {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: "no ownership kind declared"}
	}
	if req.dirtiness.kind == branchDirtinessUnset {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "no dirtiness declared"}
	}

	if ok, reason := resolveBranchOwnership(req.ownership, req.branch); !ok {
		return &destructiveRefusal{Check: CheckOwnership, What: req.what, Target: req.branch, Reason: reason}
	}

	if req.dirtiness.kind == branchDirtinessArchivedOnRemote {
		if req.dirtiness.archiveTag == "" {
			return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "the remote tip is covered by no archive tag"}
		}
		if req.leaseSHA == "" {
			return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "no lease SHA: the archive-coverage proof holds only for the tip it was computed against"}
		}
		return nil
	}

	if req.dirtiness.kind == branchDirtinessArchivedTip {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "archived-tip dirtiness answers a local question; a remote branch delete has no local tip to prove"}
	}

	if req.dirtiness.kind == branchDirtinessUnlandedRemoteTip {
		if req.leaseSHA == "" {
			return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: "no lease SHA: the landed-work probe holds only for the tip it is computed against"}
		}
		return checkUnlandedRemoteTip(req)
	}

	return checkedOutBranchDirtiness(req.ownership.location, req.what, req.branch)
}

// removeContainedPath removes the container-relative form of target through an os.Root rooted at
// container, so path-component resolution and the unlink are one openat chain.
// That chain refuses to traverse a symlink escaping container.
// A symlink planted at an intermediate segment of target can be flipped to point outside container after a check.
// Such a link still cannot carry the unlink outside container: os.Root rejects the escaping component at removal time.
// A final-component link is still removed as a link, since os.Root.Remove/RemoveAll never follow the
// target's own last component, so a junction removal is unaffected.
//
// recursive selects RemoveAll (a directory and its subtree) over Remove (a single entry).
// The OS itself refuses Remove on a non-empty directory.
// It reports whether an entry was removed: false for an already-absent target, which makes removal idempotent.
// It also reports whether that entry was a directory, for the caller's mutation-record detail.
// container must be a strict ancestor of target.
// A target equal to or outside container is a programming error and is returned as an error, never a silent no-op.
func removeContainedPath(container, target string, recursive bool) (removed, wasDir bool, err error) {
	rel, relErr := filepath.Rel(filepath.Clean(container), filepath.Clean(target))
	if relErr != nil {
		return false, false, fmt.Errorf("relate %s to container %s: %w", target, container, relErr)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, false, fmt.Errorf("%s is not strictly below container %s", target, container)
	}

	root, err := os.OpenRoot(container)
	if err != nil {
		if os.IsNotExist(err) {
			// The container is gone, so nothing under it exists: removing it is a no-op, not a failure.
			return false, false, nil
		}
		return false, false, fmt.Errorf("open container %s: %w", container, err)
	}
	defer root.Close()

	info, statErr := root.Lstat(rel)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, false, nil
		}
		// An intermediate component of rel that resolves through a symlink pointing outside container:
		// Lstat fails with a "path escapes from parent" error.
		// That escape is what this helper refuses, so the error is returned rather than a removal.
		return false, false, statErr
	}

	wasDir = info.IsDir()
	if wasDir && recursive {
		err = root.RemoveAll(rel)
	} else {
		err = root.Remove(rel)
	}
	if err != nil && !os.IsNotExist(err) {
		return false, false, err
	}
	return true, wasDir, nil
}

// removePath removes req.target through removeContainedPath rooted at req.container, after checking req.
// It uses RemoveAll for a directory and Remove otherwise, and it tolerates an already-absent target.
// It removes a file as well as a directory, so one executor serves both.
// On success it appends KindPathRemoved to rec, with detail "recursive" for the directory branch and
// "single" for the single-entry branch; the already-absent case records nothing, since that is a
// successful no-op, not a removal.
func removePath(rec *Mutations, req pathRequest) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}

	removed, wasDir, err := removeContainedPath(req.container, req.target, true)
	if err != nil {
		return fmt.Errorf("remove %s: %w", req.target, err)
	}
	if !removed {
		return nil
	}

	detail := "single"
	if wasDir {
		detail = "recursive"
	}
	rec.Append(KindPathRemoved, req.target, detail)
	return nil
}

// leftoverRemoveAttempts bounds the removal of a leftover worktree directory: the first try and two retries.
const leftoverRemoveAttempts = 3

// leftoverSeam holds the injectable removal, sleep and refill steps of a leftover worktree removal.
// A nil remove or sleep means production: removeContainedPath over the whole subtree, and time.Sleep.
// A nil refill runs nothing.
type leftoverSeam struct {
	remove func(container, target string) (removed, wasDir bool, err error)
	sleep  func(delay time.Duration)
	// refill runs on target after a clean `git worktree remove`; tests use it to play a late writer.
	refill func(target string)
}

// removeWorktreeLeftover finishes the removal of a worktree directory still on disk after `git worktree remove` deregistered it.
// Its caller's git run is its ownership proof.
// That run passed the gate's registered-linked-worktree and dirtiness checks on this same target;
// it had already accepted discarding it, so no check runs here.
// Containment is kept by the removal's os.Root under container.
// A late writer can make a removal fail: a refilled directory, or on Windows a file its writer still holds open.
// Such a removal is retried after one second, then two, three attempts in all; any other failure is returned at once.
// A removal appends KindPathRemoved for target to rec.
func removeWorktreeLeftover(rec *Mutations, container, target string, seam leftoverSeam) error {
	remove := seam.remove
	if remove == nil {
		remove = func(container, target string) (bool, bool, error) {
			return removeContainedPath(container, target, true)
		}
	}
	sleep := seam.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	var err error
	for attempt := 0; attempt < leftoverRemoveAttempts; attempt++ {
		if attempt > 0 {
			sleep(time.Duration(attempt) * time.Second)
		}
		var removed, wasDir bool
		removed, wasDir, err = remove(container, target)
		if err == nil {
			if removed {
				detail := "single"
				if wasDir {
					detail = "recursive"
				}
				rec.Append(KindPathRemoved, target, detail)
			}
			return nil
		}
		if !isLateWriterError(err) {
			break
		}
	}
	return fmt.Errorf("remove %s: %w", target, err)
}

// finishGitWorktreeRemoval completes a `git worktree remove` of target run from repoDir.
// It takes gitErr, the error that run returned.
// A clean exit with target still on disk means a late writer refilled the directory after git deleted it;
// such a writer is a build or language server writing git-ignored output.
// A git failure that left target deregistered and on disk: git dropped the registration and failed on the directory.
// Either directory is removed under container, and a nil error then means the worktree is gone.
// Any other outcome returns gitErr unchanged: a clean exit with nothing left, or a failure before git ran;
// a failure that left target registered, or one after which the repository's worktrees cannot be listed;
// or a failure that left nothing on disk.
func finishGitWorktreeRemoval(rec *Mutations, repoDir, container, target string, gitErr error, seam leftoverSeam) error {
	if gitErr == nil {
		if seam.refill != nil {
			seam.refill(target)
		}
	} else {
		var gitRunErr *gitexec.GitError
		if !errors.As(gitErr, &gitRunErr) {
			return gitErr
		}
		// An unlistable repository proves nothing about target, so target is treated as registered.
		// The recursive delete needs proof that git dropped the registration.
		entries, listErr := List(repoDir)
		if listErr != nil || listsLinkedWorktree(entries, target) {
			return gitErr
		}
	}
	if !pathPresent(target) {
		return gitErr
	}
	if err := removeWorktreeLeftover(rec, container, target, seam); err != nil {
		if gitErr != nil {
			return fmt.Errorf("git worktree remove %s deregistered it and failed (%v), and finishing the directory's removal failed: %w", target, gitErr, err)
		}
		return fmt.Errorf("git worktree remove %s succeeded, but the directory was refilled on disk and finishing its removal failed; it is no longer a worktree, so stop whatever refills it and delete it by hand; a pair that remains is then removed by running remove again: %w", target, err)
	}
	return nil
}

// RemoveStrayTaskPath deletes the stray directory at slug's task worktree path and reports whether it deleted anything.
// A stray directory is one that no registered linked worktree owns.
// A build or language server may refill such a path with ignored output after the pair's removal.
// The caller's ownership proof is its own knowledge that a pair of slug stood at that path.
// It comes from a run that created the pair, or from a removal of the slug's pair that the operator asked for.
// It refuses a registered linked worktree, which is a pair to remove.
// It also refuses a directory holding a `.git` entry, which is a repository of its own that fabric does not delete.
// The delete is contained by an os.Root under the hub.
// No dirtiness check runs and no force applies, since what stands there is no worktree.
// A delete that fails after the late-writer retries returns an error wrapping ErrStrayPathKept.
// It leaves the path on disk.
func (t *Topology) RemoveStrayTaskPath(l *lymcwd.Location, slug string) (bool, error) {
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return false, err
	}
	if err := refusePrimeSlug(l, slug); err != nil {
		return false, err
	}
	target := WorktreePath(l, slug)
	if !pathPresent(target) {
		return false, nil
	}
	entries, err := List(l.WorktreePath())
	if err != nil {
		return false, fmt.Errorf("list the worktrees to judge %s: %w", target, err)
	}
	if listsLinkedWorktree(entries, target) {
		return false, fmt.Errorf("%s is a registered worktree, so it is a pair to remove and not a stray directory", target)
	}
	if pathPresent(filepath.Join(target, ".git")) {
		return false, fmt.Errorf("%s holds a .git entry, so it is a repository of its own and fabric leaves it alone; save anything worth keeping from it, then delete it", target)
	}
	if err := removeWorktreeLeftover(NewMutations(l.HubPath), l.HubPath, target, t.leftover); err != nil {
		return false, fmt.Errorf("%w: %w; way forward: stop whatever writes into it, such as a build or language server, then delete it", ErrStrayPathKept, err)
	}
	return true, nil
}

// ignoredTaskPathDeletable reports whether the directory at target, a task worktree path, holds only files git ignores.
// It reads and deletes nothing.
// A build or language server leaves such a directory when it writes into the path of a pair that was removed.
// The ownership proof is git's own answer:
// each file, at the same path in the repository checked out at l, is untracked and ignored there.
// It reports false when the path is no real directory or is a registered linked worktree.
// It does the same for a directory holding a `.git` entry at any depth or any file git does not ignore.
// A step that fails proves nothing, so it reports false and logs the failure.
func ignoredTaskPathDeletable(l *lymcwd.Location, target string) bool {
	keep := func(cause error) bool {
		logger.Warn("fabricengine: the files at a task worktree path could not be judged as git-ignored, so the path is kept", "path", target, "cause", cause)
		return false
	}
	info, err := os.Lstat(target)
	if err != nil {
		return keep(err)
	}
	if !info.IsDir() {
		return false
	}
	entries, err := List(l.WorktreePath())
	if err != nil {
		return keep(fmt.Errorf("list the worktrees: %w", err))
	}
	if listsLinkedWorktree(entries, target) {
		return false
	}
	files, holdsRepository, err := filesUnderTaskPath(target)
	if err != nil {
		return keep(err)
	}
	if holdsRepository {
		return false
	}
	ignored, err := ignoresEveryPath(l.WorktreePath(), files)
	if err != nil {
		return keep(err)
	}
	return ignored
}

// removeIgnoredTaskPath deletes the directory at target, a task worktree path, that ignoredTaskPathDeletable approved.
// The delete is contained by an os.Root under hubPath and retried against a late writer;
// it is appended to rec as KindPathRemoved.
// No dirtiness check runs and no force applies, since what stands there is no worktree.
// A delete that fails after the retries returns an error wrapping ErrStrayPathKept and leaves the path on disk.
func (t *Topology) removeIgnoredTaskPath(rec *Mutations, hubPath, target string) error {
	if err := removeWorktreeLeftover(rec, hubPath, target, t.leftover); err != nil {
		return fmt.Errorf("%w: %w; way forward: stop whatever writes into it, such as a build or language server, then delete it", ErrStrayPathKept, err)
	}
	return nil
}

// filesUnderTaskPath returns the slash-separated paths, relative to root, of every entry under root that is no directory.
// It also reports whether a `.git` entry stands anywhere under root, and it stops listing at the first one.
func filesUnderTaskPath(root string) (files []string, holdsRepository bool, err error) {
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == ".git" {
			holdsRepository = true
			return filepath.SkipAll
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("list the files under %s: %w", root, err)
	}
	return files, holdsRepository, nil
}

// ignoresEveryPath reports whether git ignores every path in rels.
// Each path is relative to the root of the repository checked out at worktreeDir.
// A tracked path is never ignored, and an empty rels is ignored vacuously.
func ignoresEveryPath(worktreeDir string, rels []string) (bool, error) {
	if len(rels) == 0 {
		return true, nil
	}
	stdout, err := gitexec.RunStdin([]string{"check-ignore", "--stdin", "-z"}, worktreeDir, strings.Join(rels, "\x00")+"\x00")
	if err != nil {
		// check-ignore exits 1 when it ignores none of the paths.
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return false, nil
		}
		return false, fmt.Errorf("git check-ignore in %s: %w", worktreeDir, err)
	}
	ignored := make(map[string]bool)
	for _, path := range strings.Split(stdout, "\x00") {
		ignored[path] = true
	}
	for _, rel := range rels {
		if !ignored[rel] {
			return false, nil
		}
	}
	return true, nil
}

// removeGitWorktree is the executor for the git worktree remove primitive.
// It runs the pipeline, then runs git worktree remove [--force] from repoDir.
// It returns the error from git unchanged rather than wrapping it.
// Every call site builds its own message from that error.
// Where it needs the exit code or stderr, it recovers them via errors.As(err, &gitErr).
// It appends KindWorktreeRemoved to rec only when err is nil.
// A non-nil error must never be recorded as a completed removal.
func removeGitWorktree(rec *Mutations, req pathRequest, repoDir string) error {
	if checkErr := checkPathRequest(req); checkErr != nil {
		return checkErr
	}

	args := []string{"worktree", "remove"}
	if req.force {
		args = append(args, "--force")
	}
	args = append(args, req.target)

	_, err := gitexec.Run(args, repoDir)
	if err == nil {
		rec.Append(KindWorktreeRemoved, req.target, "")
	}
	return err
}

// removeLink is the executor for the link-removal primitive.
// It runs the pipeline, then removes req.target through removeContainedPath rooted at req.container.
// Rooting the unlink at container refuses a planted symlink in the target's intermediate ancestry at act time.
// Such a symlink would otherwise carry the removal outside the container, and no path resolved earlier is trusted.
// The link's own final component is still removed as a link, since os.Root never follows the target's last component.
// It appends KindLinkRemoved to rec only when the link was there.
// An already-absent target reports removed=false from removeContainedPath, so a no-op is not recorded as a removal.
func removeLink(rec *Mutations, req pathRequest) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}

	removed, _, err := removeContainedPath(req.container, req.target, false)
	if err != nil {
		return fmt.Errorf("remove link %s: %w", req.target, err)
	}
	if removed {
		rec.Append(KindLinkRemoved, req.target, "")
	}
	return nil
}

// repointLink is the executor for a junction re-point.
// It removes a drifted or dangling link so the caller can recreate it.
// It enforces containment and ownership, and declares dirtiness not applicable.
// A re-point is repair, not teardown, so it takes no force parameter.
// It passes rec straight through to removeLink and appends nothing of its own.
// No link_repointed kind is recorded, since a re-point is a removal here plus a creation by the caller.
func repointLink(rec *Mutations, what, container, target string, own pathOwnership) error {
	req := pathRequest{
		what:      what,
		container: container,
		target:    target,
		ownership: own,
		dirtiness: dirtinessNA("re-pointing a drifted or dangling link is repair, not teardown; there is no work to lose"),
		force:     false,
	}
	return removeLink(rec, req)
}

// deleteBranch is the executor for the git branch -D primitive.
// It runs the pipeline, then runs git branch -D from req.repoDir.
// It returns the git error unchanged rather than wrapping it, so every call site builds its own message from it.
// It appends KindBranchDeleted to rec via AppendRef, not Append, only when err is nil.
// A branch name is a ref, not a path, so it carries no hub-relative conversion.
func deleteBranch(rec *Mutations, req branchRequest) error {
	if checkErr := checkBranchRequest(req); checkErr != nil {
		return checkErr
	}

	_, err := gitexec.Run([]string{"branch", "-D", req.branch}, req.repoDir)
	if err == nil {
		rec.AppendRef(KindBranchDeleted, req.branch, "")
	}
	return err
}

// deleteRemoteBranch is the executor for the git push <remote> --delete primitive.
// It runs the pipeline, then calls gitrepo.New(req.repoDir).DeleteRemoteBranch(req.remote, req.branch).
// It returns the (deleted, err) pair unchanged and wraps nothing, so every call site builds its own message from it.
// A non-empty req.leaseSHA routes through DeleteRemoteBranchLeased instead, recording only on success.
// It appends KindRemoteBranchDeleted to rec via AppendRef, not Append, only when err is nil and deleted is true.
// A remote ref is a ref, not a path, so it carries no hub-relative conversion.
// It never appends when deleted is false, which is the already-absent idempotent success, nor on an error.
func deleteRemoteBranch(rec *Mutations, req remoteBranchRequest) (deleted bool, err error) {
	if checkErr := checkRemoteBranchRequest(req); checkErr != nil {
		return false, checkErr
	}

	if req.leaseSHA != "" {
		if err = gitrepo.New(req.repoDir).DeleteRemoteBranchLeased(req.remote, req.branch, req.leaseSHA); err != nil {
			return false, err
		}
		rec.AppendRef(KindRemoteBranchDeleted, req.branch, req.remote)
		return true, nil
	}

	deleted, err = gitrepo.New(req.repoDir).DeleteRemoteBranch(req.remote, req.branch)
	if err == nil && deleted {
		rec.AppendRef(KindRemoteBranchDeleted, req.branch, req.remote)
	}
	return deleted, err
}

// updateRemoteBranch is the executor for the leased force push that moves a task pair's own branch on a remote.
// It runs the pipeline over req.pathReq, then re-reads the checkout's branch.
// That keeps the parent branch from being moved: only the pair's own branch is pushed.
// It then calls req.repo.UpdateRemoteBranchLeased(req.remote, req.branch, req.sha, req.leaseSHA).
// A lease that no longer holds returns the divergence refusal.
// The remote tip moved after the caller read it, and may now hold commits the checkout lacks.
// Any other push failure is wrapped with a way forward.
// It appends KindRemoteBranchUpdated to rec via AppendRef only once the push succeeded.
// A remote ref carries no hub-relative conversion.
func updateRemoteBranch(rec *Mutations, req remoteBranchUpdateRequest) error {
	if err := checkPathRequest(req.pathReq); err != nil {
		return err
	}

	current, err := req.repo.CurrentBranch()
	if err != nil {
		return &destructiveRefusal{Check: CheckOwnership, What: req.pathReq.what, Target: req.branch, Reason: fmt.Sprintf("cannot read the branch checked out at %s: %v", req.pathReq.target, err)}
	}
	if current != req.branch || current == req.parentBranch {
		return &destructiveRefusal{Check: CheckOwnership, What: req.pathReq.what, Target: req.branch, Reason: fmt.Sprintf("%s has %q checked out, not the pair's own branch %q", req.pathReq.target, current, req.branch)}
	}

	err = req.repo.UpdateRemoteBranchLeased(req.remote, req.branch, req.sha, req.leaseSHA)
	switch {
	case errors.Is(err, gitrepo.ErrLeaseRejected):
		return fmt.Errorf("the remote task branch %s moved while this reset ran, so it may now hold commits this checkout lacks; %s", req.branch, remoteDivergenceWayForward(req.remote, req.branch))
	case err != nil:
		return fmt.Errorf("moving the remote task branch %s to %s failed: %w; way forward: re-run this reset once the remote is reachable", req.branch, req.sha, err)
	}
	rec.AppendRef(KindRemoteBranchUpdated, req.remote+"/"+req.branch, req.sha)
	return nil
}

// remoteDivergenceWayForward returns the way forward for a remote task branch holding commits the checkout lacks.
// It offers the `ours` merge for the run's own abandoned commits, because it takes no content and cannot conflict.
// It never offers a plain merge.
func remoteDivergenceWayForward(remote, branch string) string {
	return fmt.Sprintf("way forward: if those commits are this run's own abandoned work, run `git merge --strategy ours %s/%s` in the task worktree, "+
		"which takes no content and cannot conflict, then re-run this reset, which rewinds both the checkout and the remote task branch and discards what the merge brought in; "+
		"if they may be someone else's work, do not re-run this reset, which would discard them, and leave the rewind to the operator", remote, branch)
}

// createExclusiveDir creates path as a new directory and returns the createdToken proving this call created it.
//
// It opens path's parent as an os.Root and creates only path's final component through that root.
// os.Root.Mkdir, like os.Mkdir, never follows a symlink standing at the entry it creates,
// so a symlink planted at that final component makes the call fail with EEXIST.
// The token is minted only for a directory this call brought into being.
//
// The root does not guard against a symlink planted at an intermediate ancestor of path.
// os.OpenRoot resolves the parent argument, symlinks included, before rooting it,
// and the single-component leaf leaves no intermediate segment for the root to refuse,
// so a link in path's parent ancestry is followed exactly as os.Mkdir would follow it.
// It suffices only where the parent is an operator-controlled location; an attacker cannot plant a link there.
// A caller whose parent ancestry is attacker-influenced needs a root fixed at a container.
// It must resolve the whole relative tail through that root.
//
// Being unexported does not by itself stop a same-package composite literal.
// A guard bans the `createdToken{` literal outside this file; this file is on its allowlist.
// On success it appends KindDirCreated to rec, only after the mkdir has created the directory.
func createExclusiveDir(rec *Mutations, path string) (createdToken, error) {
	cleaned := filepath.Clean(path)
	parent := filepath.Dir(cleaned)
	root, err := os.OpenRoot(parent)
	if err != nil {
		return createdToken{}, err
	}
	defer root.Close()
	if err := root.Mkdir(filepath.Base(cleaned), 0o755); err != nil {
		return createdToken{}, err
	}
	rec.Append(KindDirCreated, path, "")
	return createdToken{path: cleaned, worktree: false}, nil
}

// createGitWorktree adds a git worktree at target.
// On success it returns two tokens: the createdToken proving this call added the worktree there,
// and the createdBranchToken proving the `-b` in buildArgs created createdBranch.
//
// buildArgs returns the full `git worktree add` argument slice for the path git should write to.
// The caller chooses the argument shape, such as a `-b <branch>` add, and the containment step stays the same.
// container is the directory that target must be a direct child of.
// The collision-free staging directory is created there.
//
// On a git or placement failure it returns the zero tokens, which no ownership kind accepts, and the error unwrapped.
// A caller recovers the git exit code and stderr from that error with errors.As.
// It appends KindWorktreeCreated to rec only on the success path that mints the token.
func createGitWorktree(rec *Mutations, repoDir, container, target, createdBranch string, buildArgs func(worktreePath string) []string) (createdToken, createdBranchToken, error) {
	if err := containedWorktreeAdd(repoDir, container, target, buildArgs); err != nil {
		return createdToken{}, createdBranchToken{}, err
	}
	rec.Append(KindWorktreeCreated, target, "")
	return createdToken{path: filepath.Clean(target), worktree: true}, createdBranchToken{branch: createdBranch}, nil
}

// containedWorktreeAdd runs `git worktree add` and fails closed when git writes outside container.
// A symlink planted at the staging or target path can make it do so.
// Such a symlink could appear between a containment check and the act, so the checks here bind to the act itself.
//
// `git worktree add <path>` follows a symlink standing at its destination argument.
// It then writes a full worktree wherever that symlink points and registers it at the resolved path.
// os.Root cannot reach through the git subprocess, so rooting the git call does not close this.
// Two staging levels and two fail-closed containment checks close it instead.
//
// git writes to a two-level staging location.
// The first level is a collision-free random parent directory, created through an os.Root rooted at container.
// It has mode 0700, so another UID can neither guess nor write the leaf.
// Mkdir refuses any intermediate-symlink escape.
// The leaf is named after target's base, so git names its admin directory after the slug.
// Two levels are required: os.Root.Rename refuses a symlink at an intermediate source component.
// So a planter who swaps the random parent makes the rename fail closed rather than escape.
//
// os.Root.Rename refuses a symlink at the rename destination.
// It moves a symlink at the source's final component as a link, not its target.
// Two checks bind containment to the act.
// After git writes, stagedWorktreeContained confirms that the staging leaf is a real directory.
// It must be reached without traversing any symlink; a symlink there means git followed a planted link.
// After the rename, the same check on target catches a leaf swapped in the window before the rename.
// A failure of either check removes the escaped worktree and staging debris, and returns an error.
// So an out-of-container worktree is never reported as placed.
//
// A same-UID or root planter can still race the add.
// Git may transiently write a checkout into a directory that planter already controls and can read the repo from.
// No staging location prevents this, since such a planter can substitute any path fabric writes to.
// For such an add the function guarantees that it never returns nil.
// It also never leaves target a dangling out-of-container symlink; it fails closed.
//
// target must be a direct child of container, so the single-component rename lands it in place.
// buildArgs returns the full argument slice for the path git should write to.
// It must embed the staging path it is handed, never target, so the followed-symlink write cannot be reintroduced.
func containedWorktreeAdd(repoDir, container, target string, buildArgs func(worktreePath string) []string) error {
	rel, relErr := filepath.Rel(filepath.Clean(container), filepath.Clean(target))
	if relErr != nil {
		return fmt.Errorf("relate worktree target %s to container %s: %w", target, container, relErr)
	}
	if rel == "." || rel == ".." || strings.ContainsRune(rel, filepath.Separator) {
		return fmt.Errorf("worktree target %s is not a direct child of container %s", target, container)
	}

	root, err := os.OpenRoot(container)
	if err != nil {
		return fmt.Errorf("open worktree container %s: %w", container, err)
	}
	defer root.Close()

	stagingParent, err := mkWorktreeStagingDir(root)
	if err != nil {
		return err
	}
	// The staging leaf keeps target's base name so git's admin directory is named after the slug, not
	// the random staging token; both the leaf-relative and target-relative paths stay under root.
	stagingRel := filepath.Join(stagingParent, rel)
	stagingPath := filepath.Join(container, stagingRel)

	// cleanupStagingDebris removes whatever the staging path currently resolves to — an in-container tree
	// or a worktree that escaped through a planted symlink, both of which git may have registered — then
	// drops the staging parent and prunes any dangling registration, so no debris survives a failed or
	// tampered add.
	cleanupStagingDebris := func() {
		_, _ = gitexec.Run([]string{"worktree", "remove", "--force", stagingPath}, repoDir)
		_ = root.RemoveAll(stagingParent)
		_, _ = gitexec.Run([]string{"worktree", "prune"}, repoDir)
	}

	if _, err := gitexec.Run(buildArgs(stagingPath), repoDir); err != nil {
		// git left at most an empty (or partial) staging tree it never registered as a worktree: drop the
		// whole staging parent through the same root so no debris survives a failed add.
		_ = root.RemoveAll(stagingParent)
		return err
	}

	// Fail closed if git followed a symlink planted at the staging leaf and wrote the worktree outside
	// container: the leaf git actually wrote to must be a real directory reached without traversing a
	// symlink. This binds containment to the act — a nominal-path resolve done before git ran cannot see
	// a symlink toggled in during git's write.
	if !stagedWorktreeContained(root, stagingRel) {
		cleanupStagingDebris()
		return fmt.Errorf("place worktree at %s: staging leaf was not a contained directory after git wrote it (a symlink planted during the add carried the worktree outside %s)", target, container)
	}

	if err := root.Rename(stagingRel, rel); err != nil {
		cleanupStagingDebris()
		return fmt.Errorf("place worktree at %s: %w", target, err)
	}

	// Fail closed if the source leaf was swapped for a symlink in the window between the check above and
	// the rename: os.Root.Rename would then have renamed that link onto target. Confirm target is a real
	// directory; otherwise unlink the symlink (os.Root.Remove never follows the final component) after
	// removing the worktree it points at, so target is left absent rather than a link pointing outside.
	if !stagedWorktreeContained(root, rel) {
		if dest, readErr := root.Readlink(rel); readErr == nil {
			if !filepath.IsAbs(dest) {
				dest = filepath.Join(container, filepath.Dir(rel), dest)
			}
			_, _ = gitexec.Run([]string{"worktree", "remove", "--force", dest}, repoDir)
		}
		_ = root.Remove(rel)
		cleanupStagingDebris()
		return fmt.Errorf("place worktree at %s: target was replaced by a symlink during placement (worktree escaped %s)", target, container)
	}

	// The staging parent's real leaf has moved to target; drop the parent through the root. RemoveAll,
	// not Remove, so a planter that polluted the parent with stray entries (a symlink named after the
	// weft leaf, say) cannot leave the empty-but-non-empty staging dir behind — os.Root removes each
	// entry as a link without following it, and the worktree is already out, so nothing of value is lost.
	// A failure here still leaves only a hidden staging dir, never the worktree, so it does not fail the add.
	_ = root.RemoveAll(stagingParent)

	if _, err := gitexec.Run([]string{"worktree", "repair", target}, repoDir); err != nil {
		// The worktree is placed, but its registration still names the staging path.
		// Remove the half-registered worktree so the caller's rollback sees a clean slate.
		// Left as it is, `git worktree remove` would itself choke on the broken registration.
		_, _ = gitexec.Run([]string{"worktree", "remove", "--force", target}, repoDir)
		_, _ = gitexec.Run([]string{"worktree", "prune"}, repoDir)
		return fmt.Errorf("repair worktree registration at %s: %w", target, err)
	}
	return nil
}

// stagedWorktreeContained reports whether rel, resolved through root's own directory handle, is a real directory.
// It is false for a symlink that git followed out of the container, and for a path whose ancestry escapes root.
// os.Root.Lstat resolves every component through root's handle and fails for an escaping intermediate component.
// A non-nil error and a symlink leaf both report as not contained: the conservative, fail-closed direction.
func stagedWorktreeContained(root *os.Root, rel string) bool {
	info, err := root.Lstat(rel)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

// worktreeStagingPrefix prefixes the name of a worktree staging directory.
// A new worktree is created in such a directory before it moves to its final path.
const worktreeStagingPrefix = ".fabric-wt-staging-"

// InWorktreeStaging reports whether path lies under a worktree staging directory.
// A worktree being created sits in such a directory until it moves to its final path.
func InWorktreeStaging(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, worktreeStagingPrefix) {
			return true
		}
	}
	return false
}

// mkWorktreeStagingDir creates a fresh, collision-free staging parent directory directly under root.
// It returns the directory's base name.
// The name is unpredictable (crypto/rand),
// so a planter under a different UID with hub write access cannot pre-plant a symlink at it.
// The mode is 0700, so such a planter can neither list nor write the leaf git creates inside it.
// The create goes through root's own directory handle, so an intermediate-symlink escape is refused, not followed.
func mkWorktreeStagingDir(root *os.Root) (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		var buf [16]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", fmt.Errorf("generate worktree staging name: %w", err)
		}
		name := worktreeStagingPrefix + hex.EncodeToString(buf[:])
		err := root.Mkdir(name, 0o700)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create worktree staging dir: %w", err)
		}
	}
	return "", fmt.Errorf("create worktree staging dir under %s: exhausted attempts", root.Name())
}

// resetHardTo rewinds the checkout of repo to sha, once checkPathRequest passes for req.
// The check has proven that req.target belongs to repo, the raw gitrepo.Repo handle.
// On a nil error from repo.ResetHard it appends KindWorktreeReset to rec, with sha as the detail.
func resetHardTo(rec *Mutations, req pathRequest, repo *gitrepo.Repo, sha string) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}
	if err := repo.ResetHard(sha); err != nil {
		return err
	}
	rec.Append(KindWorktreeReset, req.target, sha)
	return nil
}

// Failure classes of dropSeedCommits that a gate refusal does not cover.
var (
	// errDropResetRefused marks a `reset --keep` that git refused over a local change, with nothing changed.
	errDropResetRefused = errors.New("reset to upstream refused over a local change")
	// errDropReplayFailed marks a replay that failed; the repo is back at the tip the drop started from.
	errDropReplayFailed = errors.New("replaying the commits that are not seed commits failed")
)

// boardDropRequest builds the gate request for dropping seed commits from the board checkout.
// touched is every path the commits ahead of upstream change.
func boardDropRequest(boardDir string, touched []string) pathRequest {
	return pathRequest{
		what:      "drop seed commits from the board",
		container: filepath.Dir(boardDir),
		target:    boardDir,
		ownership: ownedBoardWorktree(),
		dirtiness: dirtyTrackedOn(touched),
		force:     false,
	}
}

// dropSeedCommits drops seed commits from the board checkout.
// It runs checkPathRequest on req and resets repo to upstream, keeping uncommitted changes.
// It then replays every commit of ahead that IsSeedCommit does not admit, oldest first.
// ahead is the commits HEAD has over upstream, newest first.
// A replayed commit whose change upstream already carries is dropped with the rest, as `pull --rebase` would.
// A replay failure aborts the cherry-pick and restores the tip the call started from.
// It returns errDropReplayFailed, so the repo is never left mid-replay.
// A reset that git refuses over a local change returns errDropResetRefused with nothing changed.
// It appends KindCommitsDropped once HEAD observably moved, with the dropped seed SHAs as the detail.
// It appends nothing on a refusal or a failure.
func dropSeedCommits(rec *Mutations, req pathRequest, repo *gitrepo.Repo, upstream string, ahead []string) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}
	seed, other, _, err := seedCommitsAhead(repo, ahead)
	if err != nil {
		return err
	}
	startTip, err := repo.CurrentSHA()
	if err != nil {
		return err
	}
	if err := repo.ResetKeep(upstream); err != nil {
		return fmt.Errorf("%w: %w", errDropResetRefused, err)
	}
	for i := len(other) - 1; i >= 0; i-- {
		replayErr := repo.CherryPick(other[i])
		if replayErr == nil {
			continue
		}
		abortErr := repo.CherryPickAbort()
		restoreErr := repo.ResetKeep(startTip)
		if abortErr != nil || restoreErr != nil {
			return fmt.Errorf("restore %s after a failed replay of %s: %w", startTip, other[i], errors.Join(replayErr, abortErr, restoreErr))
		}
		return fmt.Errorf("%w: %w", errDropReplayFailed, replayErr)
	}
	endTip, err := repo.CurrentSHA()
	if err != nil {
		return err
	}
	if endTip != startTip {
		rec.Append(KindCommitsDropped, req.target, strings.Join(seed, " "))
	}
	return nil
}

// DropLocalResult reports a local drop: its mutation record and the archive tags it kept.
type DropLocalResult struct {
	MutationRecord
	// Tags lists the local archive tags that hold the tips the drop moved branches off, one per moved branch.
	Tags []string `json:"tags,omitempty"`
}

// dropLocalPlan describes one branch to move.
// It holds the gate request, the checkout, and the tip the branch leaves.
// It also holds the origin tip the branch moves to and the commits of that tip that origin lacks.
// untracked lists the untracked files of a records checkout that the origin tip tracks, slash paths relative to it.
type dropLocalPlan struct {
	req       pathRequest
	repo      *gitrepo.Repo
	tip       string
	onto      string
	localOnly []string
	untracked []string
}

// DropLocal moves the code and records branches of the pair slug to the tips origin last held for them.
// Before each move it keeps the branch's local tip under a local `archive/<slug>/<tip>` tag.
// Only a branch holding commits its origin tracking ref lacks is touched,
// and a branch with no tracking ref is left alone.
// The gate runs against every branch before any is tagged or moved:
// a tracked change on a path the move rewrites refuses with those paths named, and nothing is tagged or moved.
// It also refuses a records branch whose anchor marker is empty.
// It refuses one whose anchor differs from the anchor of the origin tip it would move to, ancestry ignored.
// The refusal wraps ErrAnchorChange and names the commands that settle it.
// Each move is a `reset --keep` in the branch's own worktree; nothing is fetched or pushed, and the tags stay local.
// Before a records branch moves, each untracked, unignored file of its worktree that the origin tip tracks is removed.
// Such a file is one lym wrote in the weft, and `reset --keep` would refuse to overwrite it.
// A code branch's untracked files are never removed.
// It appends a tag_created entry per tag, a path_removed entry per removed file,
// and a commits_dropped entry per moved branch.
// Each entry follows its effect, and the commits_dropped detail holds the dropped commits' SHAs.
// Entries go to rec and to the result.
func DropLocal(rec *Mutations, l *lymcwd.Location, slug string) (res DropLocalResult, err error) {
	own := NewMutations(l.HubPath)
	defer func() {
		res.Mutations = own.Snapshot()
		rec.Extend(res.Mutations)
	}()

	targets, _, _, _, _, err := originTargets(l, ReconcileScope{Run: slug})
	if err != nil {
		return res, err
	}
	var plans []dropLocalPlan
	for _, target := range targets {
		if target.prime || target.slug != slug {
			continue
		}
		repo := gitrepo.New(target.worktree)
		tip, err := repo.CurrentSHA()
		if err != nil {
			return res, err
		}
		localOnly, err := repo.CommitsNotIn(tip, target.tracking)
		if err != nil {
			return res, err
		}
		if len(localOnly) == 0 {
			continue
		}
		if target.side == failedPushSideRecords {
			if err := refuseDropLocalAnchorChange(l, slug, target.worktree, target.tracking); err != nil {
				return res, err
			}
		}
		touched, err := pathsChangedBetween(target.worktree, "HEAD", target.tracking)
		if err != nil {
			return res, err
		}
		req := pathRequest{
			what:      "drop local commits to origin's tip",
			container: filepath.Dir(target.worktree),
			target:    target.worktree,
			ownership: ownedRegisteredLinkedWorktree(target.repoDir),
			dirtiness: dirtyTrackedOn(touched),
			force:     false,
		}
		if err := checkPathRequest(req); err != nil {
			return res, err
		}
		plan := dropLocalPlan{req: req, repo: repo, tip: tip, onto: target.tracking, localOnly: localOnly}
		if target.side == failedPushSideRecords {
			if plan.untracked, err = untrackedOnPaths(target.worktree, touched); err != nil {
				return res, err
			}
		}
		plans = append(plans, plan)
	}

	for _, plan := range plans {
		tag, err := keepArchiveTag(own, plan.repo, slug, plan.tip)
		if err != nil {
			return res, err
		}
		res.Tags = append(res.Tags, tag)
		for _, rel := range plan.untracked {
			removal := pathRequest{
				what:      "remove an untracked weft file origin's tip tracks",
				container: plan.req.target,
				target:    filepath.Join(plan.req.target, filepath.FromSlash(rel)),
				ownership: ownedUntrackedTrackedAt(plan.req.target, plan.onto),
				dirtiness: dirtinessNA("the file is untracked, so git holds no change of it, and the origin tip the branch moves to carries its own copy"),
				force:     false,
			}
			if err := removePath(own, removal); err != nil {
				return res, fmt.Errorf("clear the way for %s to move to origin's tip %s: %w; the local tip is kept under the tag %s", plan.req.target, plan.onto, err, tag)
			}
		}
		if err := plan.repo.ResetKeep(plan.onto); err != nil {
			return res, fmt.Errorf("move %s to origin's tip %s: %w; the local tip is kept under the tag %s", plan.req.target, plan.onto, err, tag)
		}
		own.Append(KindCommitsDropped, plan.req.target, strings.Join(plan.localOnly, " "))
	}
	return res, nil
}

// untrackedOnPaths returns the untracked, unignored files of the checkout at worktree that paths names.
// paths and the result are slash paths relative to worktree.
// An ignored file is left out, since git overwrites one without refusing.
func untrackedOnPaths(worktree string, paths []string) ([]string, error) {
	out, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "-z"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("list the untracked files of %s: %w", worktree, err)
	}
	var found []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" && slices.Contains(paths, path) {
			found = append(found, path)
		}
	}
	return found, nil
}

// keepArchiveTag puts the local tag `archive/<slug>/<first 12 hex of sha>` on sha in repo and returns its name.
// A tag already on sha is kept, and one on another commit is an error.
// It appends KindTagCreated once the tag exists.
// Its target is the repository root and its detail is the tag name.
func keepArchiveTag(rec *Mutations, repo *gitrepo.Repo, slug, sha string) (string, error) {
	tag := ArchiveTagName(slug, sha)
	commonDir, err := repo.CommonDir()
	if err != nil {
		return "", fmt.Errorf("tag local tip %s: %w", sha, err)
	}
	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/tags/" + tag}, commonDir)
	if err != nil {
		return "", fmt.Errorf("look up archive tag %q: %w", tag, err)
	}
	switch existing = strings.TrimSpace(existing); {
	case existing == "":
		if _, err := gitexec.Run([]string{"tag", tag, sha}, commonDir); err != nil {
			return "", fmt.Errorf("create archive tag %q at %s: %w", tag, sha, err)
		}
	case existing != sha:
		return "", fmt.Errorf("archive tag %q already exists at %s, not %s", tag, existing, sha)
	}
	rec.Append(KindTagCreated, filepath.Dir(commonDir), tag)
	return tag, nil
}

// restoreReplayTip puts repo's branch back on tip with `reset --keep` after a replay failed past its pick,
// so a failure never leaves the branch on upstream.
// Git may refuse the reset over a file the failed replay already wrote;
// the returned error then names the tag that holds tip.
func restoreReplayTip(rec *Mutations, repo *gitrepo.Repo, tip string) error {
	if err := resetKeepOrTag(rec, repo, tip); err != nil {
		return fmt.Errorf("restore %s after a failed board replay: %w", tip, err)
	}
	return nil
}

// resetKeepOrTag resets repo to tip with `reset --keep`.
// When git refuses the reset, tip is tagged so the commits a replay moved the branch off stay reachable,
// and the returned error names the tag and the `git reset --keep` that restores it.
func resetKeepOrTag(rec *Mutations, repo *gitrepo.Repo, tip string) error {
	resetErr := repo.ResetKeep(tip)
	if resetErr == nil {
		return nil
	}
	tag, tagErr := setAsideTag(rec, repo, "restore-failed", tip)
	if tagErr != nil {
		return errors.Join(resetErr, tagErr)
	}
	return fmt.Errorf("%w; the starting tip is tagged %s, restore it with `git reset --keep %s`", resetErr, tag, tag)
}

// cherryPickOnto is the executor for replaying local commits onto a records branch's moved upstream:
// it runs the pipeline against req and resets repo to onto, keeping uncommitted changes;
// then it cherry-picks commits, oldest first.
// A commit that changes no path, and one whose change onto already carries, is dropped and returned in dropped.
// A refusal of the dirtiness check, and a reset git refuses over a local change, return ErrReplayDirty with nothing changed.
// A conflicting pick aborts the cherry-pick and restores the tip the call started from.
// It returns ErrReplayConflict wrapped with the conflicting paths, so the repo is never left mid-replay.
// It appends KindRepoAdvanced once HEAD observably moved, with the new tip as the detail.
// On a refusal or a failure it appends nothing.
func cherryPickOnto(rec *Mutations, req pathRequest, repo *gitrepo.Repo, onto string, commits []string) (dropped []string, err error) {
	if err := checkPathRequest(req); err != nil {
		var refusal *destructiveRefusal
		if errors.As(err, &refusal) && refusal.Check == CheckDirtiness {
			return nil, fmt.Errorf("%w: %s", ErrReplayDirty, refusal.Reason)
		}
		return nil, err
	}
	startTip, err := repo.CurrentSHA()
	if err != nil {
		return nil, err
	}
	// A missing onto is not a dirty stop, so it is checked before the reset;
	// a refusal from the reset is then the dirty one.
	if _, err := repo.ResolveSHA(onto + "^{commit}"); err != nil {
		return nil, fmt.Errorf("resolve %s to replay onto: %w", onto, err)
	}
	if err := repo.ResetKeep(onto); err != nil {
		if gitrepo.IsLocalChangeRefusal(err) {
			return nil, fmt.Errorf("%w: %w", ErrReplayDirty, err)
		}
		return nil, fmt.Errorf("reset to %s to replay onto it: %w", onto, err)
	}
	resetBack := func(abortPick bool) error {
		var abortErr error
		if abortPick {
			abortErr = repo.CherryPickAbort()
		}
		if err := errors.Join(abortErr, resetKeepOrTag(rec, repo, startTip)); err != nil {
			return fmt.Errorf("restore %s after a failed replay: %w", startTip, err)
		}
		return nil
	}
	for _, commit := range commits {
		detail, err := repo.CommitDetail(commit)
		if err != nil {
			return nil, errors.Join(err, resetBack(false))
		}
		if len(detail.Paths) == 0 {
			dropped = append(dropped, commit)
			continue
		}
		before, err := repo.CurrentSHA()
		if err != nil {
			return nil, errors.Join(err, resetBack(false))
		}
		if pickErr := repo.CherryPick(commit); pickErr != nil {
			conflicting, listErr := repo.ConflictedFiles()
			if restoreErr := resetBack(true); restoreErr != nil {
				return nil, errors.Join(pickErr, restoreErr)
			}
			if listErr != nil {
				conflicting = nil
			}
			return nil, &replayConflict{commit: commit, paths: conflicting, cause: pickErr}
		}
		after, err := repo.CurrentSHA()
		if err != nil {
			return nil, errors.Join(err, resetBack(false))
		}
		if after == before {
			dropped = append(dropped, commit)
		}
	}
	endTip, err := repo.CurrentSHA()
	if err != nil {
		return nil, errors.Join(err, resetBack(false))
	}
	if endTip != startTip {
		rec.Append(KindRepoAdvanced, req.target, endTip)
	}
	return dropped, nil
}

// resetWeftToIntegrated is the executor that moves the weft branch checked out at weftPath to onto.
// onto is a commit built from its upstream and its local commits.
// It runs the pipeline against the replay request of weftPath.
// That request owns any worktree of the weft's repository;
// its dirtiness check refuses a tracked change on a path onto rewrites against HEAD.
// It then resets the branch to onto keeping uncommitted changes.
// A refusal of the dirtiness check, and a reset git refuses over a local change, return ErrReplayDirty with nothing changed.
// It appends KindRepoAdvanced, with onto as the detail, once the branch moved, and nothing on a refusal or a failure.
func resetWeftToIntegrated(rec *Mutations, weftPath, onto string) error {
	touched, err := diffNames(weftPath, "HEAD", onto, nil)
	if err != nil {
		return fmt.Errorf("list the paths %s rewrites in %s: %w", onto, weftPath, err)
	}
	req := replayRequest(weftPath, ownedWarpCheckout(weftPath), touched)
	if err := checkPathRequest(req); err != nil {
		var refusal *destructiveRefusal
		if errors.As(err, &refusal) && refusal.Check == CheckDirtiness {
			return fmt.Errorf("%w: %s", ErrReplayDirty, refusal.Reason)
		}
		return err
	}
	repo := gitrepo.New(weftPath)
	startTip, err := repo.CurrentSHA()
	if err != nil {
		return err
	}
	if err := repo.ResetKeep(onto); err != nil {
		if gitrepo.IsLocalChangeRefusal(err) {
			return fmt.Errorf("%w: %w", ErrReplayDirty, err)
		}
		return fmt.Errorf("reset %s to %s: %w", weftPath, onto, err)
	}
	if endTip, err := repo.CurrentSHA(); err == nil && endTip != startTip {
		rec.Append(KindRepoAdvanced, weftPath, endTip)
	}
	return nil
}

// restoreMovePathsToHead is the executor that restores to HEAD the paths an unfinished anchor move rewrote.
// They lie in the weft worktree at weftPath.
// pathspecs are slash paths relative to weftPath: A/<names>, B/<names> and the root marker of the move, never `.lym`.
// It runs the pipeline against every pathspec.
// It owns what lies under the pathspecs and declares the dirtiness check not applicable.
// The move record, or the failing step that raised it, proves the changes under them are the move's own.
// It then resets the index to HEAD for the pathspecs and restores the tracked files from it;
// it removes the untracked files git does not ignore.
// Nothing HEAD does not track stays under them.
// It appends KindWorktreeReset, with HEAD as the detail, when it found a change to undo.
// On a refusal, a failure or a clean tree it appends nothing.
func restoreMovePathsToHead(rec *Mutations, weftPath string, pathspecs []string) error {
	const what = "restore the paths of an anchor move to HEAD"
	own := ownedMovePathspecs(weftPath, pathspecs)
	for _, spec := range pathspecs {
		clean := pathpkg.Clean(spec)
		if pathpkg.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return &destructiveRefusal{Check: CheckContainment, What: what, Target: spec, Reason: "the pathspec does not name a path below the weft worktree"}
		}
		req := pathRequest{
			what:      what,
			container: weftPath,
			target:    filepath.Join(weftPath, filepath.FromSlash(clean)),
			ownership: own,
			dirtiness: dirtinessNA("the move record, or the failing step that raised it, proves the changes under the move's paths are the move's own"),
			force:     false,
		}
		if err := checkPathRequest(req); err != nil {
			return err
		}
	}

	literal := []string{"GIT_LITERAL_PATHSPECS=1"}
	git := func(args ...string) (string, error) {
		return gitexec.RunWithEnv(args, weftPath, literal)
	}
	status, err := git(append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, pathspecs...)...)
	if err != nil {
		return fmt.Errorf("list the changes under the move's paths in %s: %w", weftPath, err)
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if _, err := git(append([]string{"reset", "-q", "HEAD", "--"}, pathspecs...)...); err != nil {
		return fmt.Errorf("reset the index under the move's paths in %s: %w", weftPath, err)
	}
	var tracked []string
	for _, spec := range pathspecs {
		listed, err := git("ls-files", "-z", "--", spec)
		if err != nil {
			return fmt.Errorf("list the tracked files of %s in %s: %w", spec, weftPath, err)
		}
		if listed != "" {
			tracked = append(tracked, spec)
		}
	}
	if len(tracked) > 0 {
		if _, err := git(append([]string{"checkout", "--"}, tracked...)...); err != nil {
			return fmt.Errorf("restore the tracked files under the move's paths in %s: %w", weftPath, err)
		}
	}
	if _, err := git(append([]string{"clean", "-f", "-d", "-q", "--"}, pathspecs...)...); err != nil {
		return fmt.Errorf("remove the untracked files under the move's paths in %s: %w", weftPath, err)
	}
	rec.Append(KindWorktreeReset, weftPath, "HEAD")
	return nil
}

// ResetHard resets the warp checkout's HEAD, index, and working tree to sha.
// It discards any local commits past sha and any uncommitted tracked changes.
// The request is fixed to the warp worktree, and its ownership accepts the prime checkout.
// It discards only tracked files and never forces.
// rec is the caller's recorder; resetHardTo appends the resulting worktree_reset entry to it.
func (f *Fabric) ResetHard(rec *Mutations, sha string) error {
	req := pathRequest{
		what:      "reset warp checkout",
		container: filepath.Dir(f.warpPath),
		target:    f.warpPath,
		ownership: ownedWarpCheckout(f.warpPath),
		dirtiness: dirtyScopeTracked(),
		force:     false,
	}
	return resetHardTo(rec, req, f.code, sha)
}

// ResetPairCode resets a task pair's warp checkout's HEAD, index and working tree to sha.
// It discards uncommitted tracked changes only on the paths in ownPaths.
// It leaves untracked files alone and never touches the weft.
// The request refuses the prime checkout, a pair on parentBranch, and a detached HEAD;
// it also refuses a pair whose weft is on another branch.
//
// It also moves the pair's task branch on the origin remote to sha, so a later push is not rejected as diverged.
// The order is:
// the gate checks;
// then, unless opts.SkipPush, a fetch and an ancestry read of the remote task branch against HEAD;
// then the leased remote update;
// then the checkout rewrite.
// A refusal at any step before the remote update leaves the remote and the checkout unchanged,
// and the first refusal met is the local, cheap one.
// The remote update is skipped, with no record entry, when the repository has no remote;
// when the remote has no such branch;
// or when the remote tip already equals sha or is an ancestor of it.
// opts.SkipGit does not skip the fetch or the remote update; opts.SkipPush is the only bypass.
//
// The update rewrites only the pair's own task branch, and only to sha;
// it does so only when every commit it drops is reachable from HEAD.
// It takes a lease on the remote tip that the ancestry read saw.
// A remote-only commit becomes reachable only through an `ours` merge that the operator runs.
// Reading the commits the refusal lists is the one judgment left to the operator.
// If the checkout rewrite fails after the remote moved, the error says so;
// rec then holds the remote_branch_updated entry, and re-running converges.
// rec is the caller's recorder; resetHardTo appends the resulting worktree_reset entry to it.
func (f *Fabric) ResetPairCode(rec *Mutations, sha, parentBranch string, ownPaths []string, opts SyncOptions) error {
	req := pathRequest{
		what:      "reset pair warp checkout",
		container: filepath.Dir(f.warpPath),
		target:    f.warpPath,
		ownership: ownedPairWarpCheckout(f.warpPath, f.weftPath, parentBranch),
		dirtiness: dirtyTrackedExcept(ownPaths),
		force:     false,
	}
	if err := checkPathRequest(req); err != nil {
		return err
	}

	moved, err := f.moveRemoteTaskBranch(rec, req, sha, parentBranch, opts)
	if err != nil {
		return err
	}
	if err := resetHardTo(rec, req, f.code, sha); err != nil {
		if moved {
			return fmt.Errorf("the remote task branch was already updated to %s, but rewriting the checkout failed: %w; way forward: re-run this reset, which converges", sha, err)
		}
		return err
	}
	return nil
}

// moveRemoteTaskBranch moves the pair's remote task branch to sha.
// It refuses when the remote branch holds commits that HEAD lacks.
// It reports whether it updated the remote branch.
func (f *Fabric) moveRemoteTaskBranch(rec *Mutations, req pathRequest, sha, parentBranch string, opts SyncOptions) (moved bool, err error) {
	if opts.SkipPush {
		return false, nil
	}

	tip, remoteOnly, err := f.RemoteOnlyCommits()
	if err != nil {
		return false, fmt.Errorf("cannot read the remote task branch: %w; way forward: re-run this reset once the remote is reachable", err)
	}
	if tip == "" {
		return false, nil
	}
	branch, err := f.code.CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("cannot read the task branch: %w", err)
	}
	if len(remoteOnly) > 0 {
		return false, fmt.Errorf("the remote task branch %s holds commits this checkout lacks:\n%s\n%s", branch, f.describeCommits(remoteOnly), remoteDivergenceWayForward(originRemoteName, branch))
	}

	beyondSHA, err := f.code.CommitsNotIn(tip, sha)
	if err != nil {
		return false, fmt.Errorf("cannot compare the remote task branch with %s: %w", sha, err)
	}
	if len(beyondSHA) == 0 {
		return false, nil
	}

	err = updateRemoteBranch(rec, remoteBranchUpdateRequest{
		pathReq:      req,
		repo:         f.code,
		remote:       originRemoteName,
		branch:       branch,
		parentBranch: parentBranch,
		sha:          sha,
		leaseSHA:     tip,
	})
	return err == nil, err
}

// describeCommits renders each commit as an indented `<short sha> <subject>` line, in the order given.
// When the log cannot be read or does not return one line per commit, every commit is rendered as its bare SHA.
func (f *Fabric) describeCommits(shas []string) string {
	args := append([]string{"log", "--no-walk=unsorted", "--format=%h %s"}, shas...)
	out, err := gitexec.Run(args, f.warpPath)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != len(shas) {
		lines = shas
	}
	return "  " + strings.Join(lines, "\n  ")
}

// resetMergeSides restores the warp checkout to warpSHA, the SHA it had before the merge started.
// It runs through the resetHardTo executor with a pathRequest of its own.
// It forces the reset, because an abort discards the dirt the merge left behind:
// unresolved conflict markers are tracked-file modifications.
// MergeAbort refuses to run without a fabric-written record, so the discard covers only that merge's dirt;
// the pre-merge clean-pair guard already excludes an operator's own work.
//
// The weft is never reset here, because it was never a merge participant.
// Its commits land independently of any merge, so a reset would discard pushed status history
// and leave the local weft behind its origin, breaking the next push too.
// mergeState.WeftStart stays recorded and filled, but it is not a reset target.
func (f *Fabric) resetMergeSides(rec *Mutations, warpSHA string) error {
	warpReq := pathRequest{
		what:      "reset warp checkout for merge abort",
		container: filepath.Dir(f.warpPath),
		target:    f.warpPath,
		ownership: ownedWarpCheckout(f.warpPath),
		dirtiness: dirtyScopeTracked(),
		force:     true,
	}
	return resetHardTo(rec, warpReq, f.code, warpSHA)
}
