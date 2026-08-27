// destroy.go is the only file in package fabricengine permitted to perform a destructive primitive.
// The five primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
// worktree remove), removing or re-pointing a link (fslink.Remove), deleting a branch (git branch
// -D), and resetting a warp checkout hard (ResetHard). Every one of them is reached only through one
// of this file's executors, and every executor runs the shared check pipeline before performing its
// act — the gate executes, it does not merely approve.
//
// The pipeline runs four checks, always in this fixed order, stopping at the first failure:
// containment, ownership, dirtiness, force.
// Ahead of all four sits a request-shape check that is not one of them: a pathRequest declaring no
// ownership kind, no dirtiness, or a dirtinessNA with an empty reason is refused before any of the
// four runs, since a malformed request cannot be evaluated at all. That refusal borrows
// CheckOwnership/CheckDirtiness to name which declaration is missing, so a reader must not conclude
// from its Check value that containment passed — containment had not run yet.
// --force answers dirtiness only: it never satisfies containment and never satisfies ownership, so a
// containment failure — the class of defect that once destroyed an entire hub — can never be
// overridden by a flag.
//
// Containment is resolved, not lexical: both the container and target's ancestry are put through
// filepath.EvalSymlinks before they are related (see ancestors.go's refuseUncontainedPath and
// containmentPath). It was purely lexical until R2's review drove a symlink planted at an
// intermediate segment of a gate target and watched a gated removal delete files outside the hub
// while the check passed. Target's FINAL component stays unresolved on purpose — a junction
// removal's target is itself a link.
//
// That check resolves at one instant, though, and R3's review drove the gap it left: a symlink at an
// intermediate segment, dangling when the check ran and flipped live-and-escaping before the executor
// acted, carried a gated removal outside the hub anyway — the check-then-act window a nominal path
// resolved once and unlinked later cannot close. So the two arbitrary-path executors (removePath,
// removeLink) no longer act on the nominal path: removeContainedPath removes through an os.Root rooted
// at the gate's container, resolving each component and unlinking as one openat chain that refuses any
// component escaping the container at removal time. The containment check above stays as
// defense-in-depth; the rooted act is what binds resolution to the unlink and closes the window.
//
// R5's review drove the same window on the CREATE side: createExclusiveDir's os.Mkdir followed an
// intermediate planted symlink, and createGitWorktree's `git worktree add <target>` followed a symlink
// toggled at the target during Add's check-then-act window and wrote a whole worktree outside the hub.
// createExclusiveDir now creates its leaf through an os.Root rooted at the parent (same openat refusal
// as removeContainedPath), and createGitWorktree routes through containedWorktreeAdd, which stages the
// worktree under a rooted 0700 random parent and moves it into place with os.Root.Rename. R5 stopped
// there, but os.Root.Rename renames a symlink standing at the source's own final component as a link, so
// a staging-leaf symlink planted during git's write still produced an out-of-hub worktree reported as
// success; R6 adds the fail-closed containment checks (stagedWorktreeContained, on the staging leaf and
// again on the placed target) that make containedWorktreeAdd refuse rather than report such an add — see
// its own comment.
// The Check enum below names only the three checks a refusal can ever be attributed to —
// containment, ownership, dirtiness — because force never fails: it is consulted only to make the
// dirtiness check pass, never to cause a refusal of its own.
//
// See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant (added once this slice's guard test
// lands) for the machine-enforced half of this rule.
//
// Recording contract: every one of the eight executors below takes a leading `rec *Mutations`
// parameter and appends its own primitive's entry itself, after the primitive observably changed
// state — never before, and never for a no-op. A refusal records nothing, since nothing happened;
// removeGitWorktree and deleteBranch record only when the underlying git command returned a nil
// error, since a non-nil error — whether git ran and rejected the command or could not be run at all
// — would otherwise claim a destruction that never occurred.
// The parameter is explicit, never a request-type field,
// because a missing struct field is a silent zero value the compiler accepts while a missing
// parameter does not compile — this slice exists because a record was silently dropped, so the
// mechanism that turns dropping it into a build failure is the one this file uses.
// This record-only-on-observed-effect rule is what makes the truthfulness cross-check's commission
// direction sound: every entry this file ever appends corresponds to a real, completed effect, so a
// consumer of the record can trust that its presence means the primitive actually ran.

package fabricengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// Check names one of the three checks a destructive-gate refusal can be attributed to: containment,
// ownership, or dirtiness. It is string-backed so a refusal renders and marshals without a
// conversion step, and so RefusalOf's caller can read it with no import of anything unexported.
//
// There is deliberately no CheckForce member, and one must never be added: force is consulted only
// inside checkPathDirtiness, where it makes the dirtiness check PASS rather than fail, so a refusal
// can never be attributed to it.
type Check string

const (
	// CheckContainment names the gate's containment check: the target must resolve strictly below
	// its declared container.
	CheckContainment Check = "containment"
	// CheckOwnership names the gate's ownership check: the target must satisfy one of the closed set
	// of ownership predicates declared for the request.
	CheckOwnership Check = "ownership"
	// CheckDirtiness names the gate's dirtiness check: the target must be clean, or the caller must
	// have passed --force.
	CheckDirtiness Check = "dirtiness"
)

// destructiveRefusal is the gate's one error type: it names which of the three checks refused a
// destructive request, the requested act, the target (a path or a branch name), and a human reason.
// Every refusal in this file is returned as *destructiveRefusal, never as a bare fmt.Errorf, so a
// caller can always test for one with errors.As.
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

// Refusal is the value form of a destructive-gate refusal: the check that refused, the requested
// act, the target, and a human reason.
// RefusalOf is the only producer, and it exists rather than exporting *destructiveRefusal itself
// because *destructiveRefusal is a mutable pointer type whose identity callers could start
// switching on, when the envelope only ever needs its four values.
type Refusal struct {
	Check  Check
	What   string
	Target string
	Reason string
}

// RefusalOf reports whether err carries a *destructiveRefusal anywhere in its chain, unwrapping via
// errors.As so a refusal wrapped several layers deep (fmt.Errorf("...: %w", ...)) is still found.
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
	// container is the path target must resolve strictly below, symlinks in target's ancestry
	// resolved before the comparison; see refuseUncontainedPath.
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
		if !isAnyWorktreeOf(own.repoDir, target) {
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
		//
		// The base-name test runs against the RESOLVED root, not the nominal one, for the same
		// reason pathAtOrBelow now resolves: a link standing where the geometry root belongs is
		// not the geometry root, and testing the nominal spelling would let one masquerade as it.
		resolvedRoot := resolveAncestorSymlinks(own.root)
		if filepath.Base(resolvedRoot) != launchersDirName {
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
		// RawTarget, not PointsTo: the comparison is against the ONE HOP a wiring call actually
		// recorded, not a fully-resolved chain. PointsTo fully resolves — which fails outright when
		// a later segment is gone (a stale-pair prune, where the warp side a wired junction chains
		// through no longer exists) and silently walks past a target that is itself a further
		// symlink (a portal wired at a warp `_lyx` that itself points at weft), comparing an
		// end state a one-hop expectedTarget was never meant to match.
		rawTarget, err := fslink.RawTarget(target)
		if err != nil {
			return false, fmt.Sprintf("read link target of %s: %v", target, err)
		}
		if filepath.Clean(rawTarget) != filepath.Clean(own.expectedTarget) {
			return false, fmt.Sprintf("%s points to %s, not the expected %s", target, rawTarget, own.expectedTarget)
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

// isAnyWorktreeOf reports whether target is ANY worktree of the repo at repoDir, prime included —
// List's membership test with no Main-entry exclusion. It is deliberately not
// isRegisteredLinkedWorktreeIn, which skips the main entry: the hub's prime warp worktree is
// resetHardTo's ordinary target and must pass. ownedWarpCheckout is this predicate's sole
// constructor: it stayed repo-agnostic by construction — repoDir names whichever repo the caller is
// checking — for the now-retired weft-side twin resetMergeSides once needed, and there is no reason
// to narrow it back now that that caller is gone. An unenumerable repo answers false, the
// conservative direction.
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
// It relates the two through the same resolveAncestorSymlinks/containmentPath pair
// refuseUncontainedPath uses, and for the same reason: ownedUnderGeometryRoot is the one ownership
// kind with no independent authority to cross-check against — the worktree-shaped kinds compare
// against git's own registration and the link-shaped ones against fslink.RawTarget, both of which
// already carry resolved paths — so a nominal-path comparison here left it the single predicate a
// planted intermediate symlink could satisfy.
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
// The request's SHAPE is validated first, ahead of everything including the absent-target rule: a
// declaration that is missing is missing regardless of what is on disk, and validating a struct
// costs no syscall. The order matters more than it looks. It used to run after the absent-target
// short-circuit, which meant a request declaring no ownership and no dirtiness passed the gate
// vacuously whenever its target happened not to exist — so the property this file claims, that an
// omitted check is a loud failure rather than a forgotten one, held only for targets that were
// there, which is precisely the case a new call site's first test is least likely to cover.
//
// An absent target is then a no-op success before the four checks run — most ownership predicates
// fail on a path that is not there, and removePortal, removeJunctionRecords, removeLaunchers and
// Remove's tolerance of an already-absent weft worktree are all documented as idempotent today, so
// refusing here would turn those into hard failures. os.Lstat, not os.Stat, is what decides
// "absent": a dangling link is present as a link even though its target is not, and
// ownedDriftedWiredJunction must still see it.
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
// A dirtinessNA declaration always passes (its validity was already checked in checkPathRequest). A
// probe that cannot run at all is itself a refusal, not a pass, since a probe failure is exactly the
// state in which an unconditional destructive act is least defensible.
//
// The probe is not atomic with the act it gates, and that limit is stated rather than left implicit:
// this function runs `git status --porcelain` and returns, and the executor then performs the
// primitive, with no lock spanning the two — so a write landing in that window is destroyed.
// The exposure stays narrow by construction: removeGitWorktree re-checks through git itself,
// resetHardTo delegates to git, and the only RemoveAll sites carrying a real dirtiness scope are the
// two fallbacks that fire only AFTER git has already declined the removal. Closing the window would
// need a lock held across probe and act at every executor, which is a much larger claim about every
// future call path than the residual risk warrants — but "the gate executes rather than approves"
// must not be read as "the probe and the act are one transaction".
func checkPathDirtiness(req pathRequest) error {
	if req.dirtiness.kind == pathDirtinessNA {
		return nil
	}
	if req.force {
		return nil
	}

	dirty, _, err := worktreeDirty(req.dirtiness.scope, req.target)
	if err != nil {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: err.Error()}
	}
	if dirty {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.target, Reason: "worktree has uncommitted changes; use --force"}
	}
	return nil
}

// checkBranchRequest runs the gate's checks against req: the same pipeline checkPathRequest runs,
// minus containment and minus the absent-target rule, neither of which has meaning for a ref.
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

// checkBranchDirtiness runs the pipeline's dirtiness step for a branchRequest: is branch checked out
// at any worktree. git branch -D cannot delete a checked-out branch anyway, so this converts git's
// own refusal into a named gate refusal, the same move as re-gating removeWarpWorktreeDir's fallback.
// Unlike checkPathDirtiness, req.force is never consulted here — branch-deletion-is-ref-shaped in
// _mill/discussion.md is explicit that --force may answer a call site's own gate (Cleanup's
// raddleFoldedBack) but never this check.
func checkBranchDirtiness(req branchRequest) error {
	branches, err := listWeftBranches(req.ownership.location)
	if err != nil {
		return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: err.Error()}
	}
	for _, b := range branches {
		if b.Branch == req.branch && b.WorktreePath != "" {
			return &destructiveRefusal{Check: CheckDirtiness, What: req.what, Target: req.branch, Reason: fmt.Sprintf("branch is checked out at %s", b.WorktreePath)}
		}
	}
	return nil
}

// removeContainedPath removes the container-relative form of target through an os.Root rooted at
// container, so that path-component resolution and the unlink are one openat chain that atomically
// refuses to traverse a symlink escaping container.
// This is what binds the gate's containment resolution to the act rather than to an earlier instant:
// a symlink planted at an intermediate segment of target and flipped live-and-escaping between the
// pipeline's check and this removal can no longer carry the unlink outside container, because os.Root
// rejects the escaping component at removal time — a nominal path resolved once and acted on later
// left exactly that window open.
// A final-component link is still removed as a link, since os.Root.Remove/RemoveAll never follow the
// target's own last component, so a junction removal is unaffected.
//
// recursive selects RemoveAll (a directory and its subtree) over Remove (a single entry, which the OS
// itself refuses the moment a directory is non-empty).
// It reports whether an entry was actually removed — false for an already-absent target, preserving
// the idempotence every executor here relies on — and whether that entry was a directory, for the
// caller's mutation-record detail.
// container must be a strict ancestor of target;
// a target equal to or outside container is a programming error and is returned as one, never a silent
// no-op, since a request the gate built with a mismatched container/target pair is a bug in the gate.
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
			// The container itself is gone, so anything under it is too: the same idempotent no-op an
			// already-absent target is, not a failure.
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
		// A "path escapes from parent" error lands here when an intermediate component of rel resolves
		// through a symlink pointing outside container — exactly the escape this helper exists to
		// refuse, so it surfaces as an error rather than a removal.
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

// removePath is the executor for the os.RemoveAll/os.Remove primitive: it runs the pipeline, then
// removes req.target through removeContainedPath rooted at req.container — RemoveAll for a directory
// or Remove otherwise — tolerating an already-absent target.
// It is named removePath, not removeDir, because removeLaunchers deletes script FILES as well as
// their directory, and both must route through one executor.
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

// removeGitWorktree is the executor for the git worktree remove primitive: it runs the pipeline,
// then runs git worktree remove [--force] from repoDir.
// It returns the resulting error unchanged rather than wrapping it, because every call site builds
// its own message from it, recovering the exit code and stderr via errors.As(err, &gitErr) wherever
// it needs them.
// It appends KindWorktreeRemoved to rec only when err is nil — a non-nil error must never be recorded
// as a completed removal.
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

// removeLink is the executor for the link-removal primitive: it runs the pipeline, then removes
// req.target through removeContainedPath rooted at req.container.
// It routes through removeContainedPath rather than fslink.Remove for the same reason removePath does:
// a link removal's own intermediate ancestry is exactly where a planted symlink carried a gated
// removal outside its container, and rooting the unlink at container is what refuses that escape at
// act time rather than trusting a path resolved earlier. The link's own final component is still
// removed as a link, since os.Root never follows the target's last component.
// It appends KindLinkRemoved to rec only when the link was actually there: removeContainedPath reports
// removed=false for an already-absent target, so an idempotent no-op is not recorded as a removal.
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

// repointLink is the executor for a junction re-point: it removes a drifted or dangling link so the
// caller can recreate it, enforcing containment and ownership but declaring dirtiness N/A — a
// re-point is repair, not teardown, and has no force semantics at all, so it takes no force
// parameter: a repair site can never be handed a teardown site's force flag.
// It passes rec straight through to removeLink and appends nothing of its own: there is deliberately
// no link_repointed kind, since a repoint physically is a removal here plus a creation at the
// caller's own fslink.CreateDirLink.
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

// deleteBranch is the executor for the git branch -D primitive: it runs the pipeline, then runs git
// branch -D from req.repoDir.
// It returns the resulting error unchanged rather than wrapping it, for the same reason
// removeGitWorktree does: every call site builds its own message from it.
// It appends KindBranchDeleted to rec via AppendRef, not Append, only when err is nil: a branch name
// is a ref, not a path, so it carries no hub-relative conversion.
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

// createExclusiveDir creates path as a directory the gate can later authorise the removal of, and
// returns the createdToken proving it.
//
// It roots at path's parent and creates only path's final component (filepath.Base) through the root,
// so a symlink planted AT that final component is refused as an EEXIST — os.Root.Mkdir, like os.Mkdir,
// never follows a symlink standing at the entry it creates — and the token is only ever minted for a
// directory this call actually brought into being.
// The rooting is NOT, and is not relied on to be, a defence against a symlink planted at an INTERMEDIATE
// ancestor of path: os.OpenRoot resolves the parent argument (symlinks in it included) before rooting,
// and the single-component leaf handed to root.Mkdir has no intermediate segment left for the root to
// refuse, so a link in path's parent ancestry is followed exactly as a bare os.Mkdir would follow it.
// That is safe at this function's sole call site — CloneHub mints the hub as a single new component
// under the operator-chosen clone parent, an operator-controlled location an attacker cannot plant a
// symlink inside the way it can inside a live hub — but a future caller passing a path whose parent
// ancestry is attacker-influenced must not treat createExclusiveDir as sufficient containment on its
// own; removeContainedPath and containedWorktreeAdd, which root at a FIXED hub container and resolve the
// whole relative tail through it, are the pattern for that case.
//
// createdToken is only unforgeable because the bypass guard batch 6 adds bans the token
// `createdToken{` outside this file, with destroy.go itself on the guard's allowlist — being
// unexported does not by itself stop a same-package composite literal, and a reader who believes
// otherwise will eventually write one.
// On success it appends KindDirCreated to rec, never before the mkdir observably created the directory.
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

// createGitWorktree adds a git worktree at target through containedWorktreeAdd and, on success,
// returns the createdToken proving the gate itself added it there.
//
// buildArgs returns the full `git worktree add` argument slice given the path git should write the
// worktree to; the caller supplies it so this one minter serves the warp-side add's `-b <branch>`
// shape while the containment machinery stays uniform. container is the directory target must be a
// child of (the hub for a slug worktree), and is where the collision-free staging directory is created.
//
// On a git or placement failure it returns the zero token, which no ownership kind accepts, alongside
// the resulting error unwrapped — see createExclusiveDir's doc comment for why createdToken is
// unforgeable outside this file, and errors.As(err, &gitErr) for how a call site recovers the exit
// code and stderr it needs.
// It appends KindWorktreeCreated to rec only on the success path that mints the token.
func createGitWorktree(rec *Mutations, repoDir, container, target string, buildArgs func(worktreePath string) []string) (createdToken, error) {
	if err := containedWorktreeAdd(repoDir, container, target, buildArgs); err != nil {
		return createdToken{}, err
	}
	rec.Append(KindWorktreeCreated, target, "")
	return createdToken{path: filepath.Clean(target), worktree: true}, nil
}

// containedWorktreeAdd runs `git worktree add` in a way that never REPORTS a worktree placed inside
// container while it was actually written OUTSIDE it through a symlink planted at the staging or target
// path during the caller's check-then-act window — the create-side twin of removeContainedPath's
// delete-side guarantee. Round 5 built the staging structure below but never verified, after the move,
// that the placed target was a real directory rather than a followed symlink, so a planted staging-leaf
// symlink still produced an out-of-hub worktree reported as success; round 6 closes that by binding the
// containment check to the act at both the staging write and the placement.
//
// The escape it must close: `git worktree add <path>` resolves its destination-path argument itself and
// FOLLOWS a symlink standing there, writing a full real worktree wherever the symlink points and
// registering it at the resolved path. os.Root cannot reach through the git subprocess the way it
// reaches through removeContainedPath's own os.Remove, so this cannot be closed by rooting the git
// call; it is closed by a two-level staging structure plus two fail-closed containment checks.
//
// git's WRITE targets a two-level staging location: a collision-free random PARENT directory created
// through an os.Root rooted at container (openat-atomic, mode 0700, unpredictable so a different-UID
// planter can neither guess nor write the leaf, and any intermediate-symlink escape refused at mkdir
// time), holding a leaf named after target's base so git names its admin directory
// (`<gitdir>/worktrees/<name>`) after the slug exactly as it would have unstaged. The two-level shape is
// load-bearing: os.Root.Rename refuses a symlink at an INTERMEDIATE source component, so a planter who
// swaps the random parent (writable only from container itself) makes the rename fail closed rather than
// escape; a single-level staging dir directly under container would have no such protective parent.
//
// os.Root.Rename refuses to follow a symlink at the rename DESTINATION but renames a symlink standing at
// the source's own FINAL component as a link (it moves the link, not its target), so the two checks
// below are what actually bind containment to the act: after git writes, stagedWorktreeContained
// confirms the staging leaf git wrote to is a real directory reached without traversing any symlink (a
// symlink there means git followed a planted link and wrote outside); after the rename, the same check
// on target catches a leaf swapped in the narrow pre-check→rename window, whose symlink would otherwise
// have been renamed onto target. Either failure cleans up the escaped worktree and staging debris and
// returns an error, so an out-of-hub worktree is never reported as a placed one.
//
// A same-UID (or root) planter actively racing the add can still make git transiently write a checkout
// into a directory it already controls and can read the repo from anyway; that is not preventable by any
// staging location, since such a planter can substitute any path fabric writes to. What is guaranteed is
// that this function never returns nil for such an add and never leaves target a dangling out-of-hub
// symlink — it fails closed.
//
// target must be a direct child of container (every fabric worktree target — a hub-sibling slug worktree
// or `<hub>/_board` — is), which is what lets the single-component rename land it in place. buildArgs
// returns the full argument slice given the path git should write to; every caller embeds exactly the
// staging path this function hands it, never target, so no caller can reintroduce the followed-symlink write.
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
		// The worktree is placed but its registration still names the staging path; remove the
		// half-registered worktree so the caller's rollback sees a clean slate rather than a target whose
		// broken registration `git worktree remove` would itself choke on.
		_, _ = gitexec.Run([]string{"worktree", "remove", "--force", target}, repoDir)
		_, _ = gitexec.Run([]string{"worktree", "prune"}, repoDir)
		return fmt.Errorf("repair worktree registration at %s: %w", target, err)
	}
	return nil
}

// stagedWorktreeContained reports whether rel, resolved through root's own directory handle, is a real
// directory containedWorktreeAdd can trust as a placed worktree: not a symlink git followed out of the
// container, and not a path whose ancestry escapes root. os.Root.Lstat resolves every component through
// container's handle and returns an error for an escaping intermediate component, so a non-nil error and
// a symlink leaf are both reported as not contained — the conservative, fail-closed direction.
func stagedWorktreeContained(root *os.Root, rel string) bool {
	info, err := root.Lstat(rel)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

// mkWorktreeStagingDir creates a fresh, collision-free staging parent directory directly under root and
// returns its base name. The name is unpredictable (crypto/rand) so a different-UID planter with hub
// write access cannot pre-plant a symlink at it, the mode is 0700 so such a planter can neither list nor
// write the leaf git will create inside it, and the create goes through root's own directory handle so
// an intermediate-symlink escape is refused rather than followed.
func mkWorktreeStagingDir(root *os.Root) (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		var buf [16]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", fmt.Errorf("generate worktree staging name: %w", err)
		}
		name := ".fabric-wt-staging-" + hex.EncodeToString(buf[:])
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

// resetHardTo is the executor for the ResetHard primitive: it runs the pipeline against req, and
// only once that passes does it call repo's own ResetHard(sha) — repo is the raw gitrepo.Repo handle
// the pipeline's ownership check has just proven req.target actually belongs to, and sha is the
// commit the caller wants the checkout rewound to.
// On a nil error from repo.ResetHard it appends KindWorktreeReset to rec, with sha as the detail —
// this is the primitive behind defect 1.
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

// ResetHard resets the warp checkout's HEAD, index, and working tree to sha, discarding any local
// commits or uncommitted changes past that point.
// It is the gated executor for the ResetHard primitive: there is exactly one correct check
// declaration for "reset this Fabric's warp checkout", so this exported one-argument method
// hardcodes it rather than exposing a request-taking API — container is the hub
// (filepath.Dir(f.warpPath); Fabric holds no *lyxcwd.Location at all, and the parent of the warp
// worktree path is the hub), target is the warp worktree itself, ownership is ownedWarpCheckout so
// the hub's ordinary prime-worktree target still passes, dirtiness is dirtyScopeTracked to match
// warpWorktreeDirty exactly, and force is always false — Pull exposes no force flag, and this is the
// primitive R2's defect discarded uncommitted tracked work through on every advance path.
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
	return resetHardTo(rec, req, f.warp, sha)
}

// resetMergeSides is the gated reset abort and self-abort use to restore the warp checkout of a
// merge to its pre-merge SHA, through the existing resetHardTo executor — no new executor is
// needed, only one abort-specific pathRequest value.
//
// force is true, the one deliberate divergence from Fabric.ResetHard's hardcoded force: false: an
// abort's entire purpose is to discard the intentionally dirty state accumulated since the merge
// started — unresolved conflict markers are tracked-file modifications — and forcing is safe here
// only because every caller of this method is record-gated: MergeAbort refuses to run without a
// fabric-written record, so what gets discarded is exactly the dirt the merge in progress produced,
// the state abort exists to clean up by definition, never an operator's pre-existing work (the
// pre-merge clean-pair guard already excluded that).
//
// The weft is never reset here — abort-does-not-reset-weft. The weft was never a merge participant,
// so an abort has nothing to restore there, and with the weft advancing per transition (its own
// commits landing independently of any merge) a reset would discard already-pushed status history
// and leave the local weft behind its own origin, breaking the next push too. mergeState.WeftStart
// stays recorded and stays filled; it simply stops being a reset target.
func (f *Fabric) resetMergeSides(rec *Mutations, warpSHA string) error {
	warpReq := pathRequest{
		what:      "reset warp checkout for merge abort",
		container: filepath.Dir(f.warpPath),
		target:    f.warpPath,
		ownership: ownedWarpCheckout(f.warpPath),
		dirtiness: dirtyScopeTracked(),
		force:     true,
	}
	return resetHardTo(rec, warpReq, f.warp, warpSHA)
}
