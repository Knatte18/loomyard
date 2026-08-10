# Batch: the-gate

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'the-gate'
number: 2
cards: 9
verify: go test ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

This batch builds `internal/fabricengine/destroy.go` — the one file allowed to destroy — and its hermetic unit tests, plus the two symbol relocations that belong to it (`RemoveAll` and `Fabric.ResetHard`) and the one primitive whose conversion is inseparable from that relocation (`pull.go`'s three `ResetHard` calls).
It is one batch because the request types, the ownership enum, the check pipeline and the executors are a single mutually-referential design that does not compile in pieces;
splitting it would produce three batches that each fail their own `verify:`.

No other call site is converted here.
Every existing destructive site keeps working exactly as it does today, because the gate is additive until batch 3 starts routing sites onto it.
The external interface batches 3, 4 and 5 consume is the executor set (`removePath`, `removeGitWorktree`, `removeLink`, `repointLink`, `deleteBranch`, `resetHardTo`), the two token minters (`createExclusiveDir`, `createGitWorktree`), the two request types with their ownership and dirtiness constructors, and `surfaceRefusal`.

Batch-local decisions beyond `## Shared Decisions`:

- Card ordering matters here — cards 4 through 8 each add to the same new file and are written in dependency order.
  The implementer may commit them separately but must not reorder them.
- `resetHardTo` derives its container as `filepath.Dir(f.warpPath)`.
  `Fabric` holds no `*lyxcwd.Location` at all (its only constructor takes one and keeps neither), and `l.WorktreePath()` is `filepath.Join(l.HubPath, l.WorktreeName)`, so the parent of the warp worktree path *is* the hub.
  This is fabric-private geometry arithmetic, not cwd resolution, so the Cwd Resolution Invariant is not engaged.
- The gate never logs.
  A refusal is a returned `*destructiveRefusal` and nothing else;
  deciding whether it is fatal belongs to the call site.

## Cards

### Card 4: refusal type and check enum

- **Context:**
  - `_mill/discussion.md`
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/destroy.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create `internal/fabricengine/destroy.go` in `package fabricengine`.
  Declare `type destructiveCheck int` with the four unexported constants `checkContainment`, `checkOwnership`, `checkDirtiness`, `checkForce`, plus a `String() string` method returning `"containment"`, `"ownership"`, `"dirtiness"`, `"force"` so refusal messages name the check in prose.
  Declare `type destructiveRefusal struct { Check destructiveCheck; What, Target, Reason string }` with `func (e *destructiveRefusal) Error() string` producing a message of the shape `refusing to <What>: <Check> check failed for <Target>: <Reason>`.
  Every refusal in this file is returned as `*destructiveRefusal`, never as a bare `fmt.Errorf`, so a caller can always test with `errors.As`.
  Declare `func surfaceRefusal(err error) error` returning `err` when `errors.As(err, new(*destructiveRefusal))` holds and `nil` otherwise — the single expression of the `a-refusal-is-never-best-effort` decision, used by every best-effort call site in batches 3 to 5.
  Write the file's header comment now and treat it as part of this card: it must state that this is the only file in the package permitted to perform a destructive primitive, name the five primitives, name the four checks and their fixed order, state that `--force` answers dirtiness and never containment or ownership, and point at the `CONSTRAINTS.md` invariant batch 6 adds.
  Keep the header a rules statement;
  the narrative rationale goes to `internal/fabricengine/doc.go` in batch 6.
- **Commit:** `feat(fabricengine): add destroy.go with the typed destructive refusal`

### Card 5: request shapes, ownership kinds and dirtiness declarations

- **Context:**
  - `internal/fabricengine/dirtiness.go`
  - `internal/fabricengine/slug.go`
  - `internal/fabricengine/launchers.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the two request shapes and the closed enums they carry, exactly as pinned in the overview's `## Shared Decisions` "exact Go type shapes for the gate" subsection.
  Declare `type pathRequest struct { what string; container string; target string; slug *slugSpec; ownership pathOwnership; dirtiness pathDirtiness; force bool }` and `type branchRequest struct { what string; repoDir string; branch string; ownership branchOwnership; dirtiness branchDirtiness; force bool }`.
  `branchRequest` carries no `container` and no `target` field — that absence is how containment is declared structurally not-applicable for a ref, and adding either field later would reopen the hole.
  It carries no `branchPrefix` field either;
  the prefix rides on `ownedManagedBranch`, per the pinned shapes in the overview's `## Shared Decisions`.
  Declare `type slugSpec struct { name string; junctionNames []string }`.
  Declare `type createdToken struct { path string; worktree bool }`.
  Declare `pathOwnership` and `branchOwnership` as structs with unexported fields (a kind discriminator plus the per-kind inputs) and give each kind exactly one constructor function, each taking exactly what its predicate needs and nothing more:
  `ownedRegisteredLinkedWorktree(repoDir string)`, `ownedWarpCheckout(repoDir string)`, `ownedFabricHub()`, `ownedUnderGeometryRoot(root string)`, `ownedFreshlyCreatedPath(tok createdToken)`, `ownedFreshlyCreatedWorktree(tok createdToken)`, `ownedWiredJunction(wiredLinks []string, expectedTarget string)` and `ownedDriftedWiredJunction(wiredLinks []string)` return `pathOwnership`;
  `ownedManagedBranch(l *lyxcwd.Location, branchPrefix string)` returns `branchOwnership`.
  No ownership constructor takes a `*lyxcwd.Location` except `ownedManagedBranch`, because `primaryWeftBranch` is the only predicate that needs one and clone's two hub-level sites have no Location in scope at all.
  `branchPrefix` rides on that same constructor for the same reason — it is an input that kind's predicate needs and no other kind does.
  Declare `pathDirtiness` with constructors `dirtyScopeTracked()`, `dirtyScopeAll()` and `dirtinessNA(reason string)`, and `branchDirtiness` with the single constructor `dirtyCheckedOutBranch()`.
  The two `pathDirtiness` scope constructors carry the matching `dirtyScope` value from `internal/fabricengine/dirtiness.go`.
  A zero-value `pathOwnership`, `branchOwnership`, `pathDirtiness` or `branchDirtiness` is invalid and must be refused by the pipeline in card 7 — that is what makes an omitted declaration a loud failure instead of a silent pass.
  `dirtinessNA("")` is likewise invalid: an empty reason is a refusal, not a pass.
  Add no exported identifier in this card.
- **Commit:** `feat(fabricengine): add the gate's request shapes and closed check enums`

### Card 6: ownership predicate resolution

- **Context:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/launchers.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement the ownership half of the gate: one unexported resolver per kind, each returning `(ok bool, reason string)`, dispatched from a single `resolvePathOwnership(own pathOwnership, target string) (bool, string)` and `resolveBranchOwnership(own branchOwnership, branch string) (bool, string)`.
  The branch dispatcher takes no `repoDir`: the one branch-shaped kind resolves its name test, its primary-weft comparison and its checked-out lookup entirely from the Location it carries, so a `repoDir` parameter would be dead on arrival.
  Add one back only when a kind needs it.
  Each kind runs exactly the predicate the discussion's `ownership-is-a-closed-enum` table names, and every one of them reuses an existing helper rather than reimplementing it:
  `ownedRegisteredLinkedWorktree` calls `isRegisteredLinkedWorktreeIn(repoDir, target)`;
  `ownedWarpCheckout` calls `List(repoDir)` and accepts membership **including** the entry whose `Main` field is true — it is deliberately not `isRegisteredLinkedWorktreeIn`, which skips the main entry, because the hub's prime warp worktree is `ResetHard`'s ordinary target and must pass;
  `ownedFabricHub` calls `looksLikeHub(target)`;
  `ownedUnderGeometryRoot(root)` requires `root` to be a member of a closed set of fabric geometry roots and `target` to resolve at or below it, admitting deep descendants and non-directory targets;
  the geometry-root set has exactly one member today, the value `launchersDir` returns, and a root outside the set is itself a refusal, so a call site cannot satisfy containment by naming a convenient parent;
  `ownedFreshlyCreatedPath(tok)` and `ownedFreshlyCreatedWorktree(tok)` require `tok.path` to equal the request's `target` after `filepath.Clean` and require `tok.worktree` to be false and true respectively;
  `ownedWiredJunction(wiredLinks, expectedTarget)` requires `target` to be a member of `wiredLinks`, to be a link per `fslink.IsLink`, and to resolve to `expectedTarget`;
  `ownedDriftedWiredJunction(wiredLinks)` requires the first two and deliberately does not compare the resolved target, because drift is the precondition for repairing it;
  `ownedManagedBranch(l, branchPrefix)` requires the branch name to be one fabric's own scheme constructs — `WeftWarpSlug` accepts it, or it carries `branchPrefix` — requires it not to equal `primaryWeftBranch(l)`, and requires it not to be checked out at any worktree per `listWeftBranches`'s `WorktreePath` field.
  `ownedManagedBranch` inherits `primaryWeftBranch`'s fail-closed direction: an unreadable primary refuses rather than proceeds.
  The prefix is the second constructor parameter rather than a `branchRequest` field: it is an input this one predicate needs, and the rule that keeps the request shapes honest is that each check's inputs travel with the check.
  Every call site passes its own configured branch prefix, and an empty prefix means "the prefix test does not apply" rather than a match-everything wildcard.
  Every predicate answers false on an enumeration failure — the conservative direction the existing helpers already take.
- **Commit:** `feat(fabricengine): resolve every gate ownership kind from an existing predicate`

### Card 7: the shared check pipeline

- **Context:**
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/slug.go`
  - `internal/fabricengine/dirtiness.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement the one check pipeline both request shapes run, as `checkPathRequest(req pathRequest) error` and `checkBranchRequest(req branchRequest) error`.
  Before any check runs, `checkPathRequest` stats `req.target`;
  when it is absent the function returns nil and the executor performs nothing, a no-op success.
  This rule comes first and is load-bearing: most ownership predicates fail on a path that is not there, and `removePortal`, `removeJunctionRecords`, `removeLaunchers` and `Remove`'s tolerance of an already-absent weft worktree are all documented as idempotent today.
  Then, in this fixed order and stopping at the first failure:
  validity of the declarations themselves (a zero-value ownership or dirtiness, or a `dirtinessNA` with an empty reason, is refused before anything else, reported against the check whose declaration is missing);
  slug validation when `req.slug` is non-nil, via `validateWorktreeSlug(req.slug.name, req.slug.junctionNames)`, reported as a containment refusal;
  containment via `refuseUncontainedPath(req.container, req.target, req.what)`;
  ownership via card 6's resolver;
  dirtiness;
  force.
  `checkBranchRequest` runs the same pipeline minus containment and minus the absent-target rule, which have no meaning for a ref, and its dirtiness step asks whether the branch is checked out at any worktree.
  Dirtiness for a `pathRequest` calls `worktreeDirty` with the scope the declaration carries, and passes when the declaration is `dirtinessNA`;
  a probe that cannot run at all is a refusal, not a pass, except where the call site's own pre-existing behaviour was the opposite — no such site exists among the gated set, so the rule here is uniform.
  `req.force` satisfies the dirtiness check and nothing else.
  It never satisfies containment and never satisfies ownership, and the pipeline must be written so that is structural — force is consulted only inside the dirtiness step, and there is no other reference to `req.force` in the pipeline.
  This matters beyond ownership: `remove ..`, the defect that destroyed an entire hub, was a containment failure, so a force-satisfies-containment reading would bring it back behind a flag.
  Every failure returns a `*destructiveRefusal` naming the check that refused, the `what`, the target (or branch) and a human reason.
- **Commit:** `feat(fabricengine): run the four gate checks in one fixed-order pipeline`

### Card 8: the executors

- **Context:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/portals.go`
  - `internal/fabricengine/clone.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add one executor per destructive primitive.
  Each runs its request through card 7's pipeline first and performs the act only if the pipeline returns nil, which is what makes the gate execute rather than approve.
  `removePath(req pathRequest) error` calls the package's `RemoveAll` seam for a directory and `os.Remove` for a non-directory target, tolerating `os.IsNotExist` on both.
  The seam is still declared in `internal/fabricengine/clone.go` when this card runs — card 10 relocates it into this batch's new file afterwards — so call it by its bare identifier and do not qualify it with a package or move it here;
  it is named `removePath` and not `removeDir` because `removeLaunchers` deletes script files as well as their directory.
  `removeGitWorktree(req pathRequest, repoDir string) (exitCode int, stderr string, err error)` runs `git worktree remove`, appending `--force` when `req.force` is true, from `repoDir`;
  it returns git's own exit code and stderr rather than swallowing them, because three of its four call sites build distinct error messages from both.
  `removeLink(req pathRequest) error` calls `fslink.Remove`.
  `repointLink(what, container, target string, own pathOwnership) error` builds a `pathRequest` internally with `dirtinessNA` carrying a fixed reason and `force` false, then calls `removeLink`;
  it takes no `force` parameter at all, so a repair site cannot be handed a teardown site's force flag.
  `deleteBranch(req branchRequest) (exitCode int, stderr string, err error)` runs `git branch -D` from `req.repoDir`, same return shape and same reason as `removeGitWorktree`.
  `resetHardTo` is card 11's subject and is not written here.
  Every executor returns the pipeline's `*destructiveRefusal` unwrapped, so `errors.As` at a best-effort call site works without the caller knowing which executor it called.
- **Commit:** `feat(fabricengine): add one gated executor per destructive primitive`

### Card 9: the two token minters

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/add.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the creation half of the gate, so the gate owns the creation whose destruction it later authorises.
  `createExclusiveDir(path string) (createdToken, error)` creates the directory with `os.Mkdir` on the final component — not `os.MkdirAll`, which succeeds on an existing directory — so an already-present path fails with `EEXIST` and the token is only ever minted for a directory this call brought into being.
  It returns a `createdToken` with `path` set to `filepath.Clean(path)` and `worktree` false.
  `createGitWorktree(repoDir string, addArgs []string, target string) (tok createdToken, exitCode int, stderr string, err error)` — spell the first return value with an explicit name.
  Writing it as an unnamed `createdToken` followed by `exitCode int` would make Go parse the two as one name-group of type `int`, shadowing the type inside the function body so the token literal cannot compile at all.
  It runs `gitexec.RunGit(addArgs, repoDir)` and returns a token with `path` set to `filepath.Clean(target)` and `worktree` true when the add succeeded, plus git's own exit code and stderr so the call site keeps building its existing error messages.
  On a nonzero exit or a spawn error it returns the zero token, which no ownership kind accepts.
  Document on both minters that `createdToken` is only unforgeable because the bypass guard batch 6 adds bans the token `createdToken{` outside this file — the type being unexported does not prevent a same-package composite literal, and a reader who believes otherwise will eventually write one.
- **Commit:** `feat(fabricengine): mint gate-owned creation tokens for teardown ownership`

### Card 10: move the RemoveAll seam into destroy.go

- **Context:** none
- **Edits:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** move the declaration `var RemoveAll = os.RemoveAll` out of `internal/fabricengine/clone.go` and into `internal/fabricengine/destroy.go`, keeping the identifier, its exported spelling and its doc comment.
  Reword the doc comment so it describes a seam over the gate's own removal primitive rather than over clone's teardown path specifically, and note that `removePath` is now its only caller once batches 3 and 4 land.
  Leave clone's two bare `RemoveAll(hubPath)` call sites untouched in this card — batch 4 converts them.
  Remove the now-unused `os` import from `internal/fabricengine/clone.go` only if nothing else in that file uses it;
  it does, so expect the import to stay.
  The identifier has no reference anywhere in the tree except its declaration and those two call sites, so this is a same-package relocation with no caller impact.
- **Commit:** `refactor(fabricengine): move the RemoveAll seam into destroy.go`

### Card 11: move Fabric.ResetHard into destroy.go and gate it

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/dirtiness.go`
- **Edits:**
  - `internal/fabricengine/warpforward.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** move the method `Fabric.ResetHard` out of `internal/fabricengine/warpforward.go` and into `internal/fabricengine/destroy.go`, preserving its exported one-argument signature `func (f *Fabric) ResetHard(sha string) error` exactly.
  The signature is deliberately preserved: `internal/fabricengine/warpforward.go`'s header describes these delegations as public API for out-of-package callers preserving the one-repo illusion, and there is exactly one correct check declaration for "reset this Fabric's warp checkout", so the wrapper hardcodes it rather than exposing it.
  Leave `CheckoutDetached`, `RestoreBranch` and `CurrentBranch` in `internal/fabricengine/warpforward.go` untouched — none is a destructive primitive.
  Add `resetHardTo(req pathRequest, repo *gitrepo.Repo) error` to `internal/fabricengine/destroy.go`, running the pipeline and then the repo's own `ResetHard`, and have the moved method build its request with all five fields named: container `filepath.Dir(f.warpPath)`, target `f.warpPath`, ownership `ownedWarpCheckout(f.warpPath)`, dirtiness `dirtyScopeTracked()`, and force always false.
  Force is false and takes no parameter because `Pull` exposes no force flag and the defect this gate closes was `ResetHard` discarding uncommitted tracked work on every advance path.
  In `internal/fabricengine/pull.go`, change all three `f.warp.ResetHard(upstreamSHA)` calls to `f.ResetHard(upstreamSHA)`, leaving their surrounding `PartialPullError` wrapping and their `Stage: "reset"` labels unchanged.
  Leave the explicit `warpWorktreeDirty` refusal and its `ErrWarpDirty` return earlier in `Fabric.Pull` exactly where they are: it produces a named error existing tests assert on, and the gate is a floor beneath it rather than a replacement.
- **Commit:** `refactor(fabricengine): make Fabric.ResetHard the gated warp-reset executor`

### Card 12: hermetic unit tests for the gate

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/ancestors.go`
  - `internal/fabricengine/slug.go`
  - `internal/fabricengine/clone_test.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/destroy_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** write the untagged, in-package (`package fabricengine`) test file covering exactly the gate logic that needs no git, following the split the discussion's `### Hermetic tier` section establishes.
  The Test Tier Purity Invariant bans `gitexec.RunGit`, `exec.Command` and `lyxtest.Copy` in an untagged test file but does not ban filesystem access, so `t.TempDir` and a hand-built `&lyxcwd.Location{...}` are both fine and neither needs a tag.
  Cover: check ordering, by submitting a request that fails two checks and asserting the reported `Check` is the earlier one — containment before ownership with a request failing both, and ownership before dirtiness using `ownedFabricHub` against a temp directory, which needs no git;
  containment semantics through the gate for `..`, `../x`, `.`, an absolute path outside the container, a path equal to the container, and both platform separators;
  slug validation reaching the gate, for a derived slug of `..`, the reserved board name, a weft-suffixed name, the empty string and a separator-containing string;
  force semantics with one test per check, asserting `force: true` satisfies dirtiness and satisfies neither containment nor ownership;
  `dirtinessNA("")` being a refusal;
  the zero-value ownership and dirtiness declarations being refusals;
  `*destructiveRefusal` carrying the correct `Check` for each of the four;
  the absent-target no-op returning nil and destroying nothing, for each ownership kind;
  the token round trip, asserting `createExclusiveDir` refuses a path that already exists and that the token it returns authorises removal of that path and no other, and that a directory token is rejected by `ownedFreshlyCreatedWorktree`;
  and the link kinds, asserting `ownedWiredJunction` refuses a real directory, refuses a link at a path outside the wired set, and refuses a link resolving somewhere other than the expected target — the last being R1's case, an operator's own symlink sitting where fabric did not wire one — while `ownedDriftedWiredJunction` accepts a dangling or mis-pointed link at a wired path and still refuses a real directory.
  Also assert the best-effort policy directly: an operational failure from an executor is not matched by `surfaceRefusal`, while a `*destructiveRefusal` is.
  Do not test `isRegisteredLinkedWorktreeIn`, `ownedWarpCheckout`, `primaryWeftBranch` or any dirtiness probe here — all four spawn git and belong to batch 7's integration tier.
  Shape separation is asserted by construction rather than by test: a `branchDirtiness` on a `pathRequest` does not compile, so state that in a comment rather than writing a test that cannot exist.
- **Commit:** `test(fabricengine): cover the gate's hermetic logic in destroy_test.go`

## Batch Tests

`verify: go test ./internal/fabricengine/...` runs the untagged tier, which after card 12 includes `internal/fabricengine/destroy_test.go` — the batch's own new coverage and the only tier that can meaningfully judge it, since every check this batch adds that needs real git is deliberately deferred to batch 7.

Scope is the one package: batch 2 touches no file outside `internal/fabricengine`, and the module-wide `go build ./...` at the batch boundary catches any accidental break in the exported surface, which matters here because `Fabric.ResetHard` and `RemoveAll` both move.

The integration tier is deliberately not run at this batch boundary.
Batch 2 converts exactly one existing call path (`pull.go`'s three `ResetHard` sites) and adds a gate in front of it;
`Pull`'s own `ErrWarpDirty` refusal still fires first, so the integration behaviour is unchanged and the expensive tier earns its run at batch 3, where the first real conversions land.
