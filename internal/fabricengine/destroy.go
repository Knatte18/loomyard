// destroy.go is the only file in package fabricengine permitted to perform a destructive primitive.
// The five primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
// worktree remove), removing or re-pointing a link (fslink.Remove), deleting a branch (git branch
// -D), and resetting a warp checkout hard (ResetHard). Every one of them is reached only through one
// of this file's executors, and every executor runs the shared check pipeline before performing its
// act — the gate executes, it does not merely approve.
//
// The pipeline runs four checks, always in this fixed order, stopping at the first failure:
// containment, ownership, dirtiness, force.
// --force answers dirtiness only: it never satisfies containment and never satisfies ownership, so a
// containment failure — the class of defect that once destroyed an entire hub — can never be
// overridden by a flag.
//
// See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant (added once this slice's guard test
// lands) for the machine-enforced half of this rule.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// destructiveCheck enumerates the four checks the gate's pipeline runs, always in this fixed order.
type destructiveCheck int

const (
	checkContainment destructiveCheck = iota
	checkOwnership
	checkDirtiness
	checkForce
)

// String reports the check's name in prose, so a refusal message names the check that failed.
func (c destructiveCheck) String() string {
	switch c {
	case checkContainment:
		return "containment"
	case checkOwnership:
		return "ownership"
	case checkDirtiness:
		return "dirtiness"
	case checkForce:
		return "force"
	default:
		return "unknown"
	}
}

// destructiveRefusal is the gate's one error type: it names which of the four checks refused a
// destructive request, the requested act, the target (a path or a branch name), and a human reason.
// Every refusal in this file is returned as *destructiveRefusal, never as a bare fmt.Errorf, so a
// caller can always test for one with errors.As.
type destructiveRefusal struct {
	Check  destructiveCheck
	What   string
	Target string
	Reason string
}

// Error implements the error interface.
func (e *destructiveRefusal) Error() string {
	return fmt.Sprintf("refusing to %s: %s check failed for %s: %s", e.What, e.Check, e.Target, e.Reason)
}

// surfaceRefusal returns err unchanged when it carries a *destructiveRefusal, and nil otherwise.
// It is the single expression of the a-refusal-is-never-best-effort decision in _mill/discussion.md:
// every best-effort call site that can return an error wraps its executor call in surfaceRefusal, so
// an operational failure (git exited nonzero, the filesystem said no) stays discardable while a gate
// refusal never does.
func surfaceRefusal(err error) error {
	if errors.As(err, new(*destructiveRefusal)) {
		return err
	}
	return nil
}

// slugSpec carries the two halves validateWorktreeSlug needs, for a pathRequest whose target is
// slug-derived. It is nil on a pathRequest whose target is not slug-derived — slug validation is
// skipped entirely in that case, never treated as vacuously passing or failing.
type slugSpec struct {
	name          string
	junctionNames []string
}

// createdToken is the unforgeable proof that the gate itself created a path or a git worktree: the
// createExclusiveDir / createGitWorktree minters below are its only producers.
// Its unexported-ness alone does not stop a same-package composite literal — createdToken{} compiles
// anywhere in this package — so the property is enforced by the bypass guard batch 6 adds, which bans
// the token `createdToken{` outside this file. A reader who believes the type system alone enforces
// this will eventually write one.
type createdToken struct {
	path     string
	worktree bool
}

// pathRequest is the gate's request shape for every destructive primitive whose target is a
// filesystem path: os.RemoveAll/os.Remove, git worktree remove, ResetHard, and link removal/re-point.
// Every field is required — a zero-value ownership or dirtiness is refused by the pipeline rather
// than silently passed, which is what makes an omitted check a loud failure instead of a forgotten
// one.
type pathRequest struct {
	// what names the act being attempted, for the refusal message (e.g. "remove worktree").
	what string
	// container is the path target must resolve strictly below; see refuseUncontainedPath.
	container string
	// target is the path the executor will act on.
	target string
	// slug is non-nil when target is derived from a caller-supplied slug, and carries the two
	// inputs validateWorktreeSlug needs.
	slug *slugSpec
	// ownership declares which closed-enum ownership kind target must satisfy.
	ownership pathOwnership
	// dirtiness declares which dirtiness probe (or N/A) the pipeline runs against target.
	dirtiness pathDirtiness
	// force, when true, satisfies the dirtiness check only — never containment, never ownership.
	force bool
}

// branchRequest is the gate's request shape for the one destructive primitive whose target is a ref:
// git branch -D. It carries no container and no target field at all — containment is structurally
// N/A for a ref, expressed by the type rather than by a per-site "" that could be forgotten. It
// carries no branchPrefix field either: the prefix is an input ownedManagedBranch's predicate needs,
// so per the "each check's inputs travel with the check" rule it rides on that constructor instead.
type branchRequest struct {
	// what names the act being attempted, for the refusal message.
	what string
	// repoDir is the weft repo the branch lives in — not a path being destroyed.
	repoDir string
	// branch is the branch name the executor will delete.
	branch string
	// ownership declares which closed-enum ownership kind branch must satisfy.
	ownership branchOwnership
	// dirtiness declares which dirtiness probe the pipeline runs against branch.
	dirtiness branchDirtiness
	// force, when true, may answer a call site's own gate (e.g. Cleanup's raddleFoldedBack) but
	// never the gate's own checked-out-branch dirtiness check — see branch-deletion-is-ref-shaped
	// in _mill/discussion.md.
	force bool
}

// pathOwnershipKind enumerates the closed set of ownership predicates a pathRequest may declare.
// It has meaning only inside this file; every kind is reached solely through its ownedXxx
// constructor below, never constructed directly.
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
)

// pathOwnership declares which of the closed set of ownership kinds a pathRequest's target must
// satisfy. The zero value is invalid and is refused by the pipeline before any check runs.
// Construct one only via the ownedXxx functions below — each takes exactly what its predicate
// needs and nothing more, per the each-check's-inputs-travel-with-the-check rule.
type pathOwnership struct {
	kind           pathOwnershipKind
	repoDir        string
	root           string
	tok            createdToken
	wiredLinks     []string
	expectedTarget string
}

// ownedRegisteredLinkedWorktree declares target as owned when it is registered in repoDir's worktree
// list as a worktree OTHER than the main one — the ordinary teardown case, resolved via
// isRegisteredLinkedWorktreeIn.
func ownedRegisteredLinkedWorktree(repoDir string) pathOwnership {
	return pathOwnership{kind: pathOwnershipRegisteredLinkedWorktree, repoDir: repoDir}
}

// ownedWarpCheckout declares target as owned when it is ANY worktree of the warp repo at repoDir,
// prime included — the membership test resetHardTo needs, since ResetHard's ordinary target is the
// hub's prime warp worktree, which isRegisteredLinkedWorktreeIn deliberately excludes.
func ownedWarpCheckout(repoDir string) pathOwnership {
	return pathOwnership{kind: pathOwnershipWarpCheckout, repoDir: repoDir}
}

// ownedFabricHub declares target as owned when it structurally looks like a fabric hub — a `_board`
// entry, or at least one weft sibling — per looksLikeHub.
func ownedFabricHub() pathOwnership {
	return pathOwnership{kind: pathOwnershipFabricHub}
}

// ownedUnderGeometryRoot declares target as owned when root is a member of the closed set of fabric
// geometry roots and target resolves at or below it, admitting deep descendants and non-directory
// targets. It supplies what containment cannot: containment proves target is below the container,
// but proves nothing if the caller chose the container.
func ownedUnderGeometryRoot(root string) pathOwnership {
	return pathOwnership{kind: pathOwnershipUnderGeometryRoot, root: root}
}

// ownedFreshlyCreatedPath declares target as owned when tok is the token createExclusiveDir minted
// for exactly this path. Since createdToken has no producer outside this file's two minters (backed
// by the bypass guard), a site cannot declare this kind for a path the gate did not create.
func ownedFreshlyCreatedPath(tok createdToken) pathOwnership {
	return pathOwnership{kind: pathOwnershipFreshlyCreatedPath, tok: tok}
}

// ownedFreshlyCreatedWorktree declares target as owned when tok is the token createGitWorktree
// minted for exactly this path, mirroring ownedFreshlyCreatedPath for the worktree-shaped case
// (add.go's rollback of a worktree that same Add call created).
func ownedFreshlyCreatedWorktree(tok createdToken) pathOwnership {
	return pathOwnership{kind: pathOwnershipFreshlyCreatedWorktree, tok: tok}
}

// ownedWiredJunction declares target as owned when it is a member of wiredLinks, is itself a link,
// and resolves to expectedTarget — the teardown-shaped link check. Comparing the resolved target is
// what R1's defect needed and did not have: a user's own tracked symlink sitting at a wired path is
// a link, but does not resolve to what fabric wired there.
func ownedWiredJunction(wiredLinks []string, expectedTarget string) pathOwnership {
	return pathOwnership{kind: pathOwnershipWiredJunction, wiredLinks: wiredLinks, expectedTarget: expectedTarget}
}

// ownedDriftedWiredJunction declares target as owned when it is a member of wiredLinks and is itself
// a link — the re-point-shaped link check. The resolved target is deliberately not compared, because
// a drifted or dangling target is the precondition for repairing it, not a disqualifier.
func ownedDriftedWiredJunction(wiredLinks []string) pathOwnership {
	return pathOwnership{kind: pathOwnershipDriftedWiredJunction, wiredLinks: wiredLinks}
}

// branchOwnershipKind enumerates the closed set of ownership predicates a branchRequest may declare.
type branchOwnershipKind int

const (
	// branchOwnershipUnset is the zero value: an omitted declaration, always refused.
	branchOwnershipUnset branchOwnershipKind = iota
	branchOwnershipManaged
)

// branchOwnership declares which of the closed set of ownership kinds a branchRequest's branch must
// satisfy. The zero value is invalid and is refused by the pipeline before any check runs.
type branchOwnership struct {
	kind         branchOwnershipKind
	location     *lyxcwd.Location
	branchPrefix string
}

// ownedManagedBranch declares branch as owned when it is one fabric's own scheme constructs (accepted
// by WeftWarpSlug, or carrying branchPrefix), is not l's primary weft branch, and is not checked out
// at any worktree. It is the only ownership constructor that takes a *lyxcwd.Location, because
// primaryWeftBranch(l) is the one predicate that genuinely needs it — clone's two hub-level path
// sites have no Location in scope at all, which is exactly why every other kind does without one.
// An empty branchPrefix means "the prefix test does not apply", never a match-everything wildcard.
func ownedManagedBranch(l *lyxcwd.Location, branchPrefix string) branchOwnership {
	return branchOwnership{kind: branchOwnershipManaged, location: l, branchPrefix: branchPrefix}
}

// pathDirtinessKind enumerates the closed set of dirtiness declarations a pathRequest may carry.
type pathDirtinessKind int

const (
	// pathDirtinessUnset is the zero value: an omitted declaration, always refused.
	pathDirtinessUnset pathDirtinessKind = iota
	pathDirtinessScope
	pathDirtinessNA
)

// pathDirtiness declares which dirtiness probe (or N/A) the pipeline runs against a pathRequest's
// target. The zero value is invalid; dirtinessNA("") is likewise invalid — an empty reason is a
// refusal, not a pass.
type pathDirtiness struct {
	kind   pathDirtinessKind
	scope  dirtyScope
	reason string
}

// dirtyScopeTracked declares that the pipeline's dirtiness step probes tracked files only, via
// worktreeDirty(scopeTracked, target) — the right scope wherever the destructive action leaves
// untracked files alone (e.g. a reset --hard).
func dirtyScopeTracked() pathDirtiness {
	return pathDirtiness{kind: pathDirtinessScope, scope: scopeTracked}
}

// dirtyScopeAll declares that the pipeline's dirtiness step probes tracked and untracked files
// alike, via worktreeDirty(scopeAll, target) — the right scope wherever the destructive action would
// take untracked files down with it.
func dirtyScopeAll() pathDirtiness {
	return pathDirtiness{kind: pathDirtinessScope, scope: scopeAll}
}

// dirtinessNA declares that the dirtiness check does not apply to this pathRequest, for the stated
// reason — e.g. a rollback site tearing down a path this same transaction created, which has no work
// to lose because nothing was ever committed to it. reason must be non-empty: an empty reason is
// refused by the pipeline as an unstated N/A, not treated as a pass.
func dirtinessNA(reason string) pathDirtiness {
	return pathDirtiness{kind: pathDirtinessNA, reason: reason}
}

// branchDirtinessKind enumerates the closed set of dirtiness declarations a branchRequest may carry.
type branchDirtinessKind int

const (
	// branchDirtinessUnset is the zero value: an omitted declaration, always refused.
	branchDirtinessUnset branchDirtinessKind = iota
	branchDirtinessCheckedOutBranch
)

// branchDirtiness declares which dirtiness probe the pipeline runs against a branchRequest's branch.
// The zero value is invalid and is refused by the pipeline before any check runs.
type branchDirtiness struct {
	kind branchDirtinessKind
}

// dirtyCheckedOutBranch declares that the pipeline's dirtiness step asks whether branch is checked
// out at any worktree — for a ref, "is there work here to lose" means "is this branch checked out
// somewhere", since git branch -D cannot delete a checked-out branch anyway.
func dirtyCheckedOutBranch() branchDirtiness {
	return branchDirtiness{kind: branchDirtinessCheckedOutBranch}
}

// resolvePathOwnership dispatches own's kind to its predicate and reports whether target satisfies
// it. Every predicate reuses an existing helper rather than reimplementing it, and every predicate
// answers false on an enumeration failure — the conservative direction the existing helpers already
// take.
func resolvePathOwnership(own pathOwnership, target string) (ok bool, reason string) {
	switch own.kind {
	case pathOwnershipRegisteredLinkedWorktree:
		if !isRegisteredLinkedWorktreeIn(own.repoDir, target) {
			return false, fmt.Sprintf("%s is not a registered linked worktree of %s", target, own.repoDir)
		}
		return true, ""

	case pathOwnershipWarpCheckout:
		if !isWarpCheckout(own.repoDir, target) {
			return false, fmt.Sprintf("%s is not a worktree of the warp repo at %s", target, own.repoDir)
		}
		return true, ""

	case pathOwnershipFabricHub:
		if !looksLikeHub(target) {
			return false, fmt.Sprintf("%s does not look like a fabric hub (no %s entry and no weft sibling)", target, BoardDirName)
		}
		return true, ""

	case pathOwnershipUnderGeometryRoot:
		// The geometry-root set has exactly one member today, the value launchersDir returns: a
		// directory whose base name is launchersDirName. A root outside that set is itself a
		// refusal, so a call site cannot satisfy this check by naming a convenient parent.
		if filepath.Base(filepath.Clean(own.root)) != launchersDirName {
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
		resolved, err := fslink.PointsTo(target)
		if err != nil {
			return false, fmt.Sprintf("resolve link target of %s: %v", target, err)
		}
		if filepath.Clean(resolved) != filepath.Clean(own.expectedTarget) {
			return false, fmt.Sprintf("%s resolves to %s, not the expected %s", target, resolved, own.expectedTarget)
		}
		return true, ""

	case pathOwnershipDriftedWiredJunction:
		// The resolved target is deliberately not compared: drift (or a dangling target) is the
		// precondition for repairing this link, not a disqualifier.
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

// isWarpCheckout reports whether target is ANY worktree of the warp repo at repoDir, prime included
// — List's membership test with no Main-entry exclusion. It is deliberately not
// isRegisteredLinkedWorktreeIn, which skips the main entry: the hub's prime warp worktree is
// resetHardTo's ordinary target and must pass. An unenumerable repo answers false, the conservative
// direction.
func isWarpCheckout(repoDir, target string) bool {
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
func pathAtOrBelow(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// containsCleanPath reports whether target, once filepath.Clean'd, matches any entry of paths (also
// cleaned) — the membership test the two link-shaped ownership kinds share.
func containsCleanPath(paths []string, target string) bool {
	cleanTarget := filepath.Clean(target)
	for _, p := range paths {
		if filepath.Clean(p) == cleanTarget {
			return true
		}
	}
	return false
}

// resolveBranchOwnership dispatches own's kind to its predicate and reports whether branch satisfies
// it.
func resolveBranchOwnership(own branchOwnership, branch string) (ok bool, reason string) {
	switch own.kind {
	case branchOwnershipManaged:
		return resolveManagedBranch(own.location, own.branchPrefix, branch)
	default:
		return false, "no ownership kind declared"
	}
}

// resolveManagedBranch implements ownedManagedBranch's predicate: branch must be one fabric's own
// scheme constructs, must not be l's primary weft branch, and must not be checked out at any
// worktree. It inherits primaryWeftBranch's fail-closed direction: an unreadable primary refuses
// rather than proceeds, since Cleanup's deletions (and every other branch -D site) are irreversible.
func resolveManagedBranch(l *lyxcwd.Location, branchPrefix, branch string) (bool, string) {
	_, weftManaged := WeftWarpSlug(branch)
	prefixManaged := branchPrefix != "" && strings.HasPrefix(branch, branchPrefix)
	if !weftManaged && !prefixManaged {
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
// An absent target is a no-op success before any check runs — most ownership predicates fail on a
// path that is not there, and removePortal, removeJunctionRecords, removeLaunchers and Remove's
// tolerance of an already-absent weft worktree are all documented as idempotent today, so refusing
// here would turn those into hard failures. os.Lstat, not os.Stat, is what decides "absent": a
// dangling link is present as a link even though its target is not, and ownedDriftedWiredJunction
// must still see it.
func checkPathRequest(req pathRequest) error {
	if _, statErr := os.Lstat(req.target); os.IsNotExist(statErr) {
		return nil
	}

	if req.ownership.kind == pathOwnershipUnset {
		return &destructiveRefusal{Check: checkOwnership, What: req.what, Target: req.target, Reason: "no ownership kind declared"}
	}
	if req.dirtiness.kind == pathDirtinessUnset {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.target, Reason: "no dirtiness declared"}
	}
	if req.dirtiness.kind == pathDirtinessNA && req.dirtiness.reason == "" {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.target, Reason: "dirtinessNA requires a non-empty reason"}
	}

	if req.slug != nil {
		if err := validateWorktreeSlug(req.slug.name, req.slug.junctionNames); err != nil {
			return &destructiveRefusal{Check: checkContainment, What: req.what, Target: req.target, Reason: err.Error()}
		}
	}

	if err := refuseUncontainedPath(req.container, req.target, req.what); err != nil {
		return &destructiveRefusal{Check: checkContainment, What: req.what, Target: req.target, Reason: err.Error()}
	}

	if ok, reason := resolvePathOwnership(req.ownership, req.target); !ok {
		return &destructiveRefusal{Check: checkOwnership, What: req.what, Target: req.target, Reason: reason}
	}

	return checkPathDirtiness(req)
}

// checkPathDirtiness runs the pipeline's dirtiness step for a pathRequest.
//
// req.force is consulted only here, and nowhere else in the pipeline: it satisfies dirtiness and
// nothing else, never containment and never ownership, which is what keeps "remove .." — a
// containment failure — from ever being answerable by a flag.
// A dirtinessNA declaration always passes (its validity was already checked in checkPathRequest). A
// probe that cannot run at all is itself a refusal, not a pass, since a probe failure is exactly the
// state in which an unconditional destructive act is least defensible.
func checkPathDirtiness(req pathRequest) error {
	if req.dirtiness.kind == pathDirtinessNA {
		return nil
	}
	if req.force {
		return nil
	}

	dirty, _, err := worktreeDirty(req.dirtiness.scope, req.target)
	if err != nil {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.target, Reason: err.Error()}
	}
	if dirty {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.target, Reason: "worktree has uncommitted changes; use --force"}
	}
	return nil
}

// checkBranchRequest runs the gate's checks against req: the same pipeline checkPathRequest runs,
// minus containment and minus the absent-target rule, neither of which has meaning for a ref.
func checkBranchRequest(req branchRequest) error {
	if req.ownership.kind == branchOwnershipUnset {
		return &destructiveRefusal{Check: checkOwnership, What: req.what, Target: req.branch, Reason: "no ownership kind declared"}
	}
	if req.dirtiness.kind == branchDirtinessUnset {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.branch, Reason: "no dirtiness declared"}
	}

	if ok, reason := resolveBranchOwnership(req.ownership, req.branch); !ok {
		return &destructiveRefusal{Check: checkOwnership, What: req.what, Target: req.branch, Reason: reason}
	}

	return checkBranchDirtiness(req)
}

// checkBranchDirtiness runs the pipeline's dirtiness step for a branchRequest: is branch checked out
// at any worktree. git branch -D cannot delete a checked-out branch anyway, so this converts git's
// own refusal into a named gate refusal, the same move as re-gating removeWarpWorktreeDir's fallback.
// Unlike checkPathDirtiness, req.force is never consulted here — branch-deletion-is-ref-shaped in
// _mill/discussion.md is explicit that --force may answer a call site's own gate (Cleanup's
// raddleFoldedBack) but never this check.
func checkBranchDirtiness(req branchRequest) error {
	branches, err := listWeftBranches(req.ownership.location)
	if err != nil {
		return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.branch, Reason: err.Error()}
	}
	for _, b := range branches {
		if b.Branch == req.branch && b.WorktreePath != "" {
			return &destructiveRefusal{Check: checkDirtiness, What: req.what, Target: req.branch, Reason: fmt.Sprintf("branch is checked out at %s", b.WorktreePath)}
		}
	}
	return nil
}

// removePath is the executor for the os.RemoveAll/os.Remove primitive: it runs the pipeline, then
// removes req.target via RemoveAll for a directory or os.Remove otherwise, tolerating an
// already-absent target on either path.
// It is named removePath, not removeDir, because removeLaunchers deletes script FILES as well as
// their directory, and both must route through one executor.
func removePath(req pathRequest) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}

	info, statErr := os.Lstat(req.target)
	if os.IsNotExist(statErr) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("stat %s: %w", req.target, statErr)
	}

	if info.IsDir() {
		if err := RemoveAll(req.target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", req.target, err)
		}
		return nil
	}

	if err := os.Remove(req.target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", req.target, err)
	}
	return nil
}

// removeGitWorktree is the executor for the git worktree remove primitive: it runs the pipeline,
// then runs git worktree remove [--force] from repoDir. It returns git's own exit code and stderr
// rather than swallowing them, because three of its four call sites build distinct error messages
// from both.
func removeGitWorktree(req pathRequest, repoDir string) (exitCode int, stderr string, err error) {
	if checkErr := checkPathRequest(req); checkErr != nil {
		return 0, "", checkErr
	}

	args := []string{"worktree", "remove"}
	if req.force {
		args = append(args, "--force")
	}
	args = append(args, req.target)

	_, stderr, exitCode, err = gitexec.RunGit(args, repoDir)
	return exitCode, stderr, err
}

// removeLink is the executor for the fslink.Remove primitive: it runs the pipeline, then removes
// req.target via fslink.Remove.
func removeLink(req pathRequest) error {
	if err := checkPathRequest(req); err != nil {
		return err
	}
	return fslink.Remove(req.target)
}

// repointLink is the executor for a junction re-point: it removes a drifted or dangling link so the
// caller can recreate it, enforcing containment and ownership but declaring dirtiness N/A — a
// re-point is repair, not teardown, and has no force semantics at all, so it takes no force
// parameter: a repair site can never be handed a teardown site's force flag.
func repointLink(what, container, target string, own pathOwnership) error {
	req := pathRequest{
		what:      what,
		container: container,
		target:    target,
		ownership: own,
		dirtiness: dirtinessNA("re-pointing a drifted or dangling link is repair, not teardown; there is no work to lose"),
		force:     false,
	}
	return removeLink(req)
}

// deleteBranch is the executor for the git branch -D primitive: it runs the pipeline, then runs git
// branch -D from req.repoDir, with the same return shape and the same reason removeGitWorktree has
// for keeping git's own exit code and stderr rather than swallowing them.
func deleteBranch(req branchRequest) (exitCode int, stderr string, err error) {
	if checkErr := checkBranchRequest(req); checkErr != nil {
		return 0, "", checkErr
	}

	_, stderr, exitCode, err = gitexec.RunGit([]string{"branch", "-D", req.branch}, req.repoDir)
	return exitCode, stderr, err
}

// createExclusiveDir creates path as a directory the gate can later authorise the removal of, and
// returns the createdToken proving it.
//
// It uses os.Mkdir on the final component, not os.MkdirAll, which succeeds on an already-existing
// directory — so an already-present path fails with EEXIST, and the token is only ever minted for a
// directory this call actually brought into being.
//
// createdToken is only unforgeable because the bypass guard batch 6 adds bans the token
// `createdToken{` outside this file, with destroy.go itself on the guard's allowlist — being
// unexported does not by itself stop a same-package composite literal, and a reader who believes
// otherwise will eventually write one.
func createExclusiveDir(path string) (createdToken, error) {
	if err := os.Mkdir(path, 0o755); err != nil {
		return createdToken{}, err
	}
	return createdToken{path: filepath.Clean(path), worktree: false}, nil
}

// createGitWorktree runs git worktree add with addArgs from repoDir and, on success, returns the
// createdToken proving the gate itself added the worktree at target, plus git's own exit code and
// stderr so the call site keeps building its existing error messages.
//
// The first return value is spelled with an explicit name (tok createdToken) rather than left
// unnamed: an unnamed `createdToken` immediately followed by `exitCode int` would parse as one
// name-group of type int, shadowing the type inside the function body so the token literal below
// could not compile at all.
//
// On a nonzero exit or a spawn error it returns the zero token, which no ownership kind accepts — see
// createExclusiveDir's doc comment for why createdToken is unforgeable outside this file.
func createGitWorktree(repoDir string, addArgs []string, target string) (tok createdToken, exitCode int, stderr string, err error) {
	_, stderr, exitCode, err = gitexec.RunGit(addArgs, repoDir)
	if err != nil || exitCode != 0 {
		return createdToken{}, exitCode, stderr, err
	}
	return createdToken{path: filepath.Clean(target), worktree: true}, exitCode, stderr, nil
}
