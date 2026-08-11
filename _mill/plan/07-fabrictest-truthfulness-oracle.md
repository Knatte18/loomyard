# Batch: fabrictest-truthfulness-oracle

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'fabrictest-truthfulness-oracle'
number: 7
cards: 5
verify: go test -tags integration ./internal/fabricengine/fabrictest/
depends-on: [6]
```

## Batch Scope

This batch turns the existing verb matrix into a truthfulness oracle: every cell cross-checks the result envelope's mutation record against the manifest diff, in both directions.
The harness already captures a before/after manifest at every cell, so the oracle is available for free on the diff side;
the record side needs real plumbing, and that plumbing is what this batch delivers.
It also deletes `internal/fabricengine/fabrictest`'s duplicate check enum in favour of the exported one batch 2 introduced, since emitting `refusal.check` as a machine-readable JSON field promotes the enum's rendered values to part of fabric's public contract and two encodings of one public contract is exactly where drift starts.

It is one batch because `VerbCase.Run`'s signature change touches all fourteen `Run` closures at once and cannot be landed partially.
It depends on batch 6 rather than batch 5 for a substantive reason, not sequencing tidiness: the `CloneHubReset`/`RealHub` cell drives `fabriccli.CloneAndWire`, so until batch 6 records the CLI layer's own config writes, junction wiring and `Bolt` commits, that cell's unfiltered honesty diff carries changes no record entry covers and the omission direction fires on correct behaviour.

Batch-local decision: the oracle lives in a new `internal/fabricengine/fabrictest/mutationoracle.go` rather than inside `manifest.go`, matching the package's one-concern-per-file layout and keeping `DiffManifest`'s own surface unchanged.

## Cards

### Card 28: consume the exported check enum, delete the copy

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/fabrictest/hub.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/matrix_test.go`
  - `internal/fabricengine/fabrictest/refusal.go`
  - `internal/fabricengine/fabrictest/refusal_test.go`
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete `internal/fabricengine/fabrictest/refusal.go`'s own `type Check string` and its three constants, and repoint every consumer to `fabricengine.Check`, `fabricengine.CheckContainment`, `fabricengine.CheckOwnership` and `fabricengine.CheckDirtiness`.
  The package already imports `fabricengine` (see `internal/fabricengine/fabrictest/hub.go`), so this costs nothing structurally.
  `RefusedByGate`'s `string(check)+" check failed"` composition keeps working verbatim, because the exported constants carry the same three string values the deleted copy did.

  `Expectation.Check` in `internal/fabricengine/fabrictest/verbs.go` changes to `fabricengine.Check`, and every cell that sets it updates to the exported constants.
  `internal/fabricengine/fabrictest/matrix_test.go` is repointed in **this** card's commit, not left to card 31: it carries `for _, other := range []Check{CheckContainment, CheckOwnership, CheckDirtiness}` in its refusal assertion, which does not compile once the local type is deleted.
  Grep the package for the bare `Check` type and the three bare constants before finishing, rather than working from this list — the compiler is the authority.

  Additionally — the bonus the accessor makes available — reimplement `RefusedByGate` on top of `fabricengine.RefusalOf` instead of substring matching, comparing the returned `Refusal.Check` to the wanted check.
  That is strictly more precise than matching a rendered message, and it is what the exported accessor exists for.
  `RefusedBefore`, which matches a non-gate error's substring, stays exactly as it is — it has no refusal to read.

  Move the never-add-a-force-member rule's *fabrictest-side* statements: `internal/fabricengine/fabrictest/refusal.go`'s standing comment and `internal/fabricengine/fabrictest/doc.go`'s restatement both point at the deleted copy after this card.
  Rewrite both to cite `fabricengine.Check`'s own doc comment as the rule's home rather than restating it a second time — one declarer for the enum means one declarer for its rule.
- **Commit:** `refactor(fabrictest): consume fabricengine's exported Check enum`

### Card 29: the oracle

- **Context:**
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/mutation.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/fabrictest/mutationoracle.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/fabrictest/mutationoracle.go` behind the `integration` build tag, exporting:

  ```go
  func AssertRecordMatchesDiff(tb testing.TB, rec fabricengine.Mutations, unfiltered []Change)
  ```

  `unfiltered` is the diff computed with a `nil` permitted list.
  The function asserts both directions and fails `tb` with a message naming the offending path or entry, in the style `AssertNoUnpermittedChange` already uses.

  **Which kinds participate.** Only the manifest-observable kinds are cross-checked in the *commission* direction: `path_removed`, `worktree_removed`, `worktree_created`, `dir_created`, `link_removed`, `link_created`, and `file_written` **when its target is outside the `.git` metadata directory**.
  The git-state kinds are exempt from commission and asserted against git itself through the existing per-verb `Expectation.Effect` assertions, which is the authoritative oracle rather than an inference from a ref file: `branch_created`, `branch_deleted`, `branch_pushed`, `commit_created`, `worktree_reset`, `worktree_switched`, `push_spawned`, `repo_advanced`, and `file_written` **when its target is under the `.git` metadata directory**.
  `CaptureManifest` records `.git` itself and each `.git/worktrees/<name>` at existence granularity, excludes everything else beneath `.git`, and states outright that branch existence is deliberately not carried by the manifest at all — so cross-checking those kinds against the diff would report a lie of commission on entirely correct behaviour.
  Declare the split as a table in this file, one row per kind, so a newly added kind cannot be silently omitted from the classification.

  **Omission direction — over raw entries, all kinds.** Every change in `unfiltered` must be covered by some record entry's **coverage set**. Matching is segment-wise subtree containment, never path equality: one `worktree_created` or `path_removed` names a single root while `DiffManifest` emits one `Change` per path beneath it, so a normal `Add` yields dozens of diff entries against one record entry.
  Reuse the segment matching `pathPermitted`/`pathAtOrBelowRoot` already implement in `internal/fabricengine/fabrictest/manifest.go` rather than writing a second matcher — a root of `_portals/x` must never match `_portalsfoo/y`, and the two mechanisms must stay consistent.
  `mutationoracle.go` is in the same `package fabrictest`, so it calls the unexported `pathAtOrBelowRoot` directly;
  `manifest.go` needs no edit and must not be widened — exporting a helper only to reach it from a sibling file in the same package would grow the harness's public surface for nothing.

  **Coverage rule.** An entry whose `Target` is a worktree root — `worktree_created`, `worktree_removed`, `worktree_reset`, `worktree_switched`, `repo_advanced` — covers both (i) every path at or beneath that worktree root and (ii) the corresponding `<prime>/.git/worktrees/<name>` admin entry.

  **The admin entry's derivation discriminates warp from weft, and the harness's two helpers cannot be applied blindly.** Both `primeWorktreeAdminPermittedRoot` and `primeWeftAdminPermittedRoot` in `internal/fabricengine/fabrictest/verbs.go` take a **warp** slug: the weft one applies `weftname.SiblingPath("", slug)` to it, so feeding it a weft target already named `<slug>-weft` yields `warp-bare-weft/.git/worktrees/<slug>-weft-weft` — a key that does not exist, leaving every `Add`/`Remove` cell's weft admin change uncovered and firing a false lie of omission.
  So: test the target's basename for `weftname.Suffix` first.
  A weft-side worktree target strips the suffix and goes to `primeWeftAdminPermittedRoot`;
  a warp-side one goes to `primeWorktreeAdminPermittedRoot` unchanged.
  Naming both admin roots directly is an acceptable alternative if the implementer finds it clearer, but the discrimination itself is not optional.
  Every other kind covers its `Target` subtree alone.

  **Coverage also extends upward, to directory-shaped ancestors.** An entry's coverage set additionally includes every **ancestor directory** of its `Target`, up to but excluding the hub root, and only for a diff change whose entry is `KindDir`.
  This is what accounts for the directory chains fabric creates and prunes implicitly around the roots it records — the `_portals/<anchor>/` chain `fslink.CreateDirLink` `MkdirAll`s, the `_launchers/<anchor>/<slug>` chain `writeLaunchers` `MkdirAll`s, and the same chains `pruneEmptyAncestors` reaps on removal.
  Without it the Add, Remove and Prune cells all report a false lie of omission on correct behaviour.
  The restriction to `KindDir` is what keeps the widening honest: a *file* or *link* appearing beside a recorded target is still an uncovered change, and only the directory levels the recorded target could not exist without are admitted.
  `Change` carries `Before`/`After` `Entry` values whose `Kind` field supplies the test, so no new manifest API is needed.
  This one rule replaces two exemption lists and is honest about *why* the admin entry changed — because a worktree was created or destroyed, which the record does state.
  Without it, git-admin bookkeeping and the working-tree rewrites `Checkout`'s `git switch` and `Pull`'s reset cause would fire as false lies of omission on correct behaviour.

  **Commission direction — over the record's net effect.** A mutation performed and then undone *inside one call* nets to zero in the before/after manifest while the record correctly carries both entries;
  that is `Add`'s mint-then-`rollbackAdd`, `Checkout`'s `rollbackSwitch`, and `reconcile`'s dangling-link repoint, three named cells that would all fail under a raw-entry rule.
  So: fold the record to its net effect first, then assert that every surviving manifest-observable entry has at least one change in `unfiltered` at or beneath its `Target`.
  An entry is exempt when a **later** entry in the same record inverts it on the same `Target`. The inverse pairs are `dir_created`/`worktree_created` ↔ `path_removed`/`worktree_removed`, and `link_created` ↔ `link_removed`.
  The omission direction is unaffected and still runs over raw entries — a path the diff shows as changed must be accounted for by *something* in the record, whether or not it was later undone.

  **A `"."` target is always exempt from commission, and exempt from omission only when it is constructive.** `CaptureManifest` never emits a `"."` key (it returns early at `path == hubRoot`), so no `"."`-targeted entry can ever find a matching change — the commission exemption is unconditional and mechanical.
  The omission direction splits by what the entry says happened:

  - **`dir_created` with `Target: "."`** — `CloneHub` minting the hub via `createExclusiveDir(hubPath)` — is **exempt**. It would make every path in the diff a segment-wise descendant, trivially satisfying omission for the entire clone cell while asserting nothing, and every creation that follows it is recorded individually by batch 5 card 21 anyway. The clone cell asserts hub creation through its own existence assertion instead.
  - **`path_removed` with `Target: "."`** — `resetHub` tearing the whole hub down through `removePath` at `target = hubPath` — **counts**, and needs an explicit normalisation the reused helper does not perform: `pathAtOrBelowRoot(path, ".")` splits `"."` into a single literal segment and compares it against the diff key's first segment, so it returns false for every hub-relative path. Normalise a `"."` target to the hub root — i.e. treat it as covering every path — **before** the segment match, rather than passing `"."` into `pathAtOrBelowRoot` and expecting it to mean "everything". This is a normalisation at the oracle's own boundary, not a second matcher, so it does not conflict with the reuse rule above. Nothing survives the teardown for a finer entry to name, so admitting it loses no assertion: it is the honest and complete statement of what happened. Without this, the `CloneHubReset`/`RealHub` cell reports every path the old hub carried as an uncovered `ChangeRemoved`, because `TestCloneHubReset` captures before/after at `fixture.ResetHubPath` and the teardown's only possible entry is this one.

  State both halves and the reason for the asymmetry in the code, so the next reader does not collapse them back into one rule.
- **Commit:** `feat(fabrictest): add the mutation-record truthfulness oracle`

### Card 30: `VerbCase.Run` returns the record

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabrictest/mutationoracle.go`
  - `internal/fabricengine/fabrictest/hub.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `VerbCase.Run`'s field type in `internal/fabricengine/fabrictest/verbs.go` from
  `func(tb testing.TB, h *Hub, f VerbFixture) error`
  to
  `func(tb testing.TB, h *Hub, f VerbFixture) (fabricengine.Mutations, error)`,
  and update its doc comment to say why: the twelve verbs return twelve heterogeneous result types, so a cell needs one accessor — `Mutated()` — to read them uniformly, and discarding the typed result left nothing for a cell to cross-check against.

  Update all fourteen `Run` closures in this file to return `res.Mutated()` alongside their error instead of discarding the result with `_`.
  Most are the two-line shape `_, err := <call>; return err` and become `res, err := <call>; return res.Mutated(), err`, but do not assume that shape uniformly: `pullCase` and `unwireJunctionsCase` both do `_, err = <call>` after an earlier assignment in the same closure, so their conversion is `res, err := <call>` with a fresh local rather than a reassignment.
  Grep the file for `Run: func(tb testing.TB` and convert every hit — the count is the authority, not this list.
  The closures, by the case that owns them: `addCase`, `removeCase`, `pruneCase`, `cleanupCase`, `checkoutCase`, `reconcileCase`, `unwireJunctionsCase`, `pullCase`, `cloneHubResetNonHubCase`, `cloneHubResetRealHubCase`, and the hostile cases (`addHostileCases`, `removeHostileCases`, `checkoutHostileCases`, `unwireJunctionsHostileCase`), two of which declare more than one case.

  Two need care:

  - `unwireJunctionsCase` drives `fabricengine.UnwireJunctions` — the **inner** type `UnwireResult`, not the `Unwire` verb's `UnwireVerbResult`. Batch 3 embedded the record in both, so `res.Mutated()` compiles here; this is the case that would not compile if the record had stopped at the verb boundary.
  - `cloneHubResetRealHubCase` drives `fabriccli.CloneAndWire`, whose returned `CloneResult` carries the CLI layer's own record after batch 6 — so this cell's honesty diff includes the junctions, config writes and commits that happen above the engine.

  Where a `Run` closure fails early through `tb.Fatalf` before reaching the verb call, return the zero `fabricengine.Mutations` alongside its error — `tb.Fatalf` does not return, so the value is unreachable, but the compiler needs it.

  **`VerbCase.Run` also has a caller outside `verbs.go`, and the arity change breaks it.** `internal/fabricengine/fabrictest/verbs_test.go` calls `err := vc.Run(t, h, fixture)` inside `TestVerbCases_CleanState`;
  repoint it to `_, err := vc.Run(t, h, fixture)`, discarding the record with `_`.
  That test asserts each case's own error shape, not its record — the record's assertions belong to card 31's cells — so discarding is correct here rather than a missed opportunity.
  Grep the package for `.Run(` before finishing: the compiler is the authority, and this file is `integration`-tagged, so an untagged `go test` never sees the break.
- **Commit:** `refactor(fabrictest): have VerbCase.Run return the mutation record`

### Card 31: wire the oracle into every cell

- **Context:**
  - `internal/fabricengine/fabrictest/mutationoracle.go`
  - `internal/fabricengine/fabrictest/verbs.go`
  - `internal/fabricengine/fabrictest/manifest.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/matrix_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabricengine/fabrictest/matrix_test.go`, both `runCell` and `TestCloneHubReset`'s inline cell body now:

  1. capture `rec, err := vc.Run(t, h, fixture)` instead of `err := vc.Run(...)`;
  2. keep the existing `AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)` call exactly as it is — it takes the two manifests and diffs internally, so it needs no new argument and its signature must not be widened;
  3. add **one** new `DiffManifest(before, after, nil)` call and pass its result to `AssertRecordMatchesDiff(t, rec, <that unfiltered diff>)`.

  Permitted removal roots suppress **diff noise only** and never suppress the honesty assertion.
  This is the difference between the slice's headline case asserting something and asserting nothing: the `Remove` anomaly cell declares `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>` as permitted precisely *because* they do get destroyed before the refusal, so if permitted roots also suppressed the honesty check, the one cell that reproduces "mutated, then refused" would assert exactly nothing about the record.
  Under this rule that cell asserts both that the deletions were allowed **and** that the envelope admitted to them.

  `DiffManifest(before, after Manifest, permitted []string)` already takes the permitted list as a parameter, so the new `nil`-permitted call needs no API change at all.
  Keep the existing five-phase cell order and every existing assertion — this card **adds** an assertion, it does not replace one.
  Put the honesty assertion after the existing `AssertNoUnpermittedChange` call so a cell that fails both reports the survival failure first, which is the one that usually explains the other.
- **Commit:** `test(fabrictest): cross-check every cell's record against the manifest diff`

### Card 32: oracle unit tests and the doc.go sabotage table

- **Context:**
  - `internal/fabricengine/fabrictest/mutationoracle.go`
  - `internal/fabricengine/fabrictest/manifest.go`
  - `internal/fabricengine/fabrictest/manifest_test.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabrictest/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/fabrictest/doc.go`
- **Creates:**
  - `internal/fabricengine/fabrictest/mutationoracle_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabricengine/fabrictest/mutationoracle_test.go` behind the `integration` build tag, testing `AssertRecordMatchesDiff` against hand-built `Mutations` and `[]Change` values — no hub, no git, no verb.
  Use a recording `testing.TB` stub (or the pattern `internal/fabricengine/fabrictest/manifest_test.go` already uses to assert a helper's failure) so a case can assert that the oracle *fails* without failing the test.

  Cases, one per rule the oracle carries:

  - a diff change with no covering record entry fails (lie of omission — defect 2's shape);
  - a manifest-observable record entry with no diff change at or beneath its target fails (lie of commission);
  - a diff change beneath a `worktree_created` target passes under segment-wise containment, and a sibling path sharing a name prefix (`_portalsfoo/y` against a `_portals/x` root) does **not**;
  - a `<prime>/.git/worktrees/<slug>` admin change is covered by the worktree-rooted entry naming that worktree, and by nothing else — with **two rows, one warp-side and one weft-side**, since the weft row is the one the naive single-helper derivation gets wrong;
  - a git-state kind (`branch_created`, `commit_created`, `worktree_reset`, `push_spawned`, `repo_advanced`) with no diff change passes the commission direction;
  - `file_written` under the `.git` metadata directory is exempt from commission while `file_written` outside the `.git` metadata directory is not;
  - a `dir_created` inverted by a later `path_removed` on the same target is exempt from commission, while the same pair in the reverse order is not — the rule is *later* inverts *earlier*;
  - a `link_created`/`link_removed` pair nets to zero the same way;
  - a `"."` target is skipped in the commission direction regardless of kind;
  - a `dir_created` with target `"."` does **not** satisfy the omission direction for an unrelated diff change, while a `path_removed` with target `"."` does — the two halves of the asymmetry, asserted separately so a future simplification that collapses them fails here;
  - a `KindDir` diff change at an ancestor of a recorded target passes under the upward coverage rule, while a `KindFile` change at that same ancestor path does not;
  - an empty record against an empty diff passes, and an empty record against a non-empty diff fails.

  Then extend `internal/fabricengine/fabrictest/doc.go`'s nine-cell sabotage table with the truthfulness dimension: with a guarding check neutered, the cell must now fail on **both** the survival assertion and the honesty assertion.
  Update the deferred-work note in the same file that currently names this slice's truthfulness assertions as work it deliberately deferred — that work has now landed, and the note must say what it became rather than pointing at a future slice.
- **Commit:** `test(fabrictest): cover the truthfulness oracle and update the sabotage table`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/fabrictest/` runs the whole tagged harness package — the full cross-product matrix plus the `CloneHub{Reset}` column plus the new oracle unit tests.
The unbounded scope is justified here rather than scoped to a name prefix: card 31 adds an assertion to **every** cell, so the batch's own change surface *is* the whole matrix, and a scoped run would leave most of what this batch touches unverified.
This package is the slice's most expensive suite;
running it once per fixer round is the intended cost, and no other batch pays it.
