# Batch: dotlyx-junction-wiring-and-unwire

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: dotlyx-junction-wiring-and-unwire
number: 8
cards: 9
verify: go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [7]
```

## Batch Scope

This batch turns `.lyx` into a real weft-backed junction and removes the last mechanism this task exists to eliminate.
It folds `structuralNeverCommittedDirs` into the wired name-set, adds a `.lyx`-only content-adoption branch to `seedLyxJunction` so the first `reconcile` after this change does not hard-error in every existing worktree, seeds the weft repo's `.git/info/exclude` with `.lyx/` at wiring time, deletes the committed `.gitignore` `.lyx/` block from both `clone.go` and `unwire.go`, stops `Unwire` from deleting weft-side `_lyx` content, and recognises `<hub>/.lyx` as a fabric-owned hub-level geometry element.

It is one batch because these pieces are mutually load-bearing: wiring `.lyx` without adoption breaks every worktree, adoption without the weft-side exclude leaves scratch showing as untracked dirt that trips `Remove`'s no-force dirty gate, and removing the committed `.gitignore` without the junction leaves `.lyx` visible to the warp repo.
Its predecessor batch 4 is what makes adoption possible at all — the durable log sink no longer holds a handle inside the directory adoption moves.

**Batch-local decision — `Remove` gets no new contract for a busy weft-side `.lyx`.**
Its `git worktree remove --force` deletes whatever is under the weft worktree, and on Windows an open handle inside `.lyx` makes the removal fail with an OS error that surfaces as-is.
The remedy is the same as adoption's — stop the daemons and re-run — and it is documented rather than special-cased, because `Remove` is an explicit whole-pair teardown the operator asked for.

**Batch-local decision — no cleanup path for a pre-fix committed `.gitignore` block.**
Both `gitignore.Ensure` and `gitignore.Remove` call sites are deleted outright, so no code path removes a leftover block.
The only known affected repo is the sandbox, which is re-cloned anyway;
the manual remedy (delete the `.lyx/` line from the lyx-managed block) is documented in the sandbox suite by batch 6 and in the module doc here.

**Batch-local decision — downgrade is unsupported, one-way upgrade only.**
A pre-fix binary's `applyStaleRemoval` removes on-disk junctions absent from *its* `RepoWiredNames`, so running an older `lyx fabric reconcile` after this change unwires `.lyx` and strands scratch inside the weft worktree.
This is recorded in the module doc;
no attempt is made to make the change downgrade-safe.

## Rename mechanic

_This batch contains no `Moves:` entries;
the section is included only because the template carries it.
Every card here is an edit or a create._

## Cards

### Card 45: wire .lyx by folding the never-committed set into the wired names

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/drift.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** change `junctionNames(baseDir)` to return `dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, filterHubReserved(cfg.Dirs()))`, so `WiredNames`/`RepoWiredNames` now yield `_lyx`, `.lyx`, and the config's optional names — in that order, deduplicated.
  Update the godocs on `junctionNames`, `WiredNames` and `RepoWiredNames` to record the full composition and to remove batch 7's stated deferral.
  State in `WiredNames`'s godoc that this set is what gets junctions and warp `.git/info/exclude` entries, and that it is deliberately **wider** than the pathspec/commit-routing set `pathspecNames` returns — the difference is exactly `structuralNeverCommittedDirs`, and that difference is what makes `.lyx` junctioned-and-excluded but never named in a git invocation.
  Also note that `filterHubReserved` is applied only to the config names: a structural name is never filtered, and `.lyx` is deliberately absent from `HubReservedNames()` for exactly that reason.
  Record one more consumer in `WiredNames`'s godoc: `Healthy` (`internal/fabricengine/drift.go`) iterates `RepoWiredNames` and requires every wired junction to exist and point at its weft target, so widening the set makes fabric health require the `.lyx` junction from this commit range on — an already-wired worktree reports `CauseJunctionMissing` ("`.lyx` junction missing") until `lyx fabric reconcile` runs and card 46's adoption converts its real `.lyx` into the junction.
  That is the intended upgrade signal, not a bug;
  card 52 records the operator-facing half.
- **Commit:** `feat(fabricengine): wire .lyx as a weft-backed junction`

### Card 46: add the .lyx-only content-adoption branch

- **Context:**
  - `internal/fslink/fslink.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/fabricengine/weftwiring.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `seedLyxJunction`'s real-directory branch — reached when `os.Lstat(link)` succeeds and `fslink.IsLink(link)` reports false — add a `.lyx`-only path taken when `j.Name == lyxdirs.DotLyxDirName`, leaving the existing hard refusal in place for every other name.
  Extract the adoption logic into a private helper, `adoptDotLyxContent(link, target string) error`, so the branch is directly testable and `seedLyxJunction` stays readable.
  Adoption must: read the host-side directory's entries;
  refuse before moving anything if **any** entry name already exists in the weft-side target, returning an error naming the colliding path and leaving both sides untouched (a collision means an earlier adoption already ran, and `.lyx` is disposable enough that the operator can delete the host-side copy — fabric never overwrites or deletes content);
  otherwise `os.Rename` each entry from the host directory into the target;
  then remove the now-empty host directory and create the junction in its place.
  Wrap a rename failure in an actionable error naming the entry and instructing the operator to stop reed/scout and re-run `lyx fabric reconcile` — on Windows, moving a directory with an open handle inside it fails, and that is the expected cause.
  A partial move must never be reported as success: on failure, return the error with whatever was already moved named in the message, so the operator can see the half state rather than being told the wiring is fine.
  Idempotency: a second `reconcile` finds a link, not a real directory, and takes the existing continue-path untouched.
  Rewrite the surrounding refusal-error comment block, which today explains the guard's rationale for `_lyx` and `_pattern`, to also state why `.lyx` is the exception: content under `.lyx` is always lyx's own machine-local scratch, so "never touch what might be the user's hand-authored content" does not apply there, while it very much does for `_lyx` and `_pattern`, where the refusal stays.
  Also state the upgrade reason: every worktree in existence today has a real `.lyx` (logger, reed, shuttle, scout and burler write it unconditionally), so without adoption the first `reconcile` after this change hard-errors everywhere.
- **Commit:** `feat(fabricengine): adopt a pre-existing real .lyx into the weft target`

### Card 47: seed the weft-side .lyx exclude at wiring time

- **Context:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/gitrepo/push.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `lyxdirs.DotLyxDirName + "/"` to `seedWeftArtifactExcludes`'s entries slice, keeping it the **sole owner** of that file's content and keeping the line-exact idempotency loop unchanged.
  Update its godoc: one `.lyx/` pattern now keeps every module's machine-local scratch untracked in the weft repo, replacing the three deep wildcard patterns batch 6 deleted, and no `.gitignore` is ever committed in weft either — `.git/info/exclude` wins there because it needs no commit, no pathspec change, and no new file in the weft root.
  In `junction.go`'s `WireJunctions`, call the weft-side seeding after `seedLyxJunction` succeeds and before `seedGitExclude`, resolving the weft worktree path from the junction records rather than re-deriving it: `seedLyxJunction` already materialises each junction's weft-side target with `os.MkdirAll(target, 0o755)`, so use `HostJunctions(l, slug, names)`' first record's `Target` parent — or, more robustly, `WeftWorktreePath(l, slug)`, which is the same value `HostJunctions` computes its targets from.
  Add a comment recording why seeding happens here and not only in `ensureWeftLockDir`: wiring already materialises the weft-side target, so the exclude entry is guaranteed to exist before anything writes into `.lyx`;
  seeding only from `ensureWeftLockDir` would leave the window between wiring and the first weft-git verb open, during which scratch shows as untracked dirt and trips `Remove`'s no-force dirty gate.
  `ensureWeftLockDir` keeps calling the same single owner as the self-healing path for machines that never re-wire — do not remove that call.
  A weft-side seeding failure is a hard error from `WireJunctions`, consistent with its existing posture.
- **Commit:** `feat(fabricengine): seed the weft-side .lyx exclude at wiring time`

### Card 48: delete the committed .gitignore .lyx/ block

- **Context:**
  - `internal/gitignore/gitignore.go`
  - `internal/fabricengine/junction.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `internal/fabriccli/clone.go`, delete the `gitignore.Ensure(l.AnchorPath(), ".lyx/")` call and its error branch, and drop the now-unused `internal/gitignore` import.
  Add a short comment where it was, recording that `.lyx` is excluded through the warp's `.git/info/exclude` by `WireJunctions` on the line above — a tracked `.gitignore` entry in the user's own repo would advertise that LYX is in use, and a host→weft junction must never leave a tracked artifact behind in the user's repo.
  Update the function's own doc comment, which lists ".gitignore" among the steps of the wiring sequence it drives.
  In `internal/fabricengine/unwire.go`, delete the `gitignore.Remove(cwd, ".lyx/")` call, the `changed` handling, and the `internal/gitignore` import;
  remove the `Gitignore string` field from `UnwireVerbResult` entirely rather than retaining it as a constant `"unchanged"` — retaining it would report on a mechanism that no longer exists.
  Correct all three comments that assert the old behaviour: the package doc comment at the top of `unwire.go`, `Unwire`'s own doc comment, and the `unwire` subcommand's `Long` text in `internal/fabriccli/fabric.go` (which currently says unwire "clears the weft-side `_lyx` content, and reverts the managed .gitignore `.lyx/` entry").
  The `Long` rewrite must describe the new contract: unwire removes every host junction and its warp `.git/info/exclude` entries, and leaves every weft-side directory intact.
  Keep `internal/gitignore` itself — `internal/vscode` still uses it for `.vscode/`.
- **Commit:** `refactor(fabric): stop writing a committed .gitignore .lyx/ block`

### Card 49: stop Unwire deleting weft-side content

- **Context:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabriccli/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `Unwire`, delete the `os.RemoveAll(weftLyxDir)` call, the `WeftContent = "cleared"` assignment, and the whole follow-on block that constructs a `Fabric`, builds a `ScopedPathspec` and calls `commitWeft` with `"lyx fabric unwire: clear _lyx"` plus `pushWeftAt`.
  What remains is a pure observation: if the weft worktree is absent, `WeftContent = "not_present"`;
  if the weft-side `_lyx` is absent, `"not_present"`;
  otherwise `"preserved"`.
  Update `UnwireVerbResult.WeftContent`'s godoc: its value set is now `"preserved"` | `"not_present"`, and it still describes `_lyx` only — weft `_pattern` content was already preserved by design, and this change makes `_lyx` converge on that same behaviour rather than leaving an unexplained asymmetry.
  State in the doc that the weft-side `.lyx` is never touched by unwire either;
  it disappears with the weft worktree when `Remove` tears the pair down, and on Windows an open handle inside it makes that `git worktree remove --force` fail with an OS error that surfaces as-is — remedy: stop the daemons and re-run.
  Drop the now-unused imports (`internal/lyxdirs` if it becomes unused here, and whatever `EnvSyncOptions`/`New`/`pushWeftAt` usage was the only reason for a given import).
  In `internal/fabriccli/unwire.go`, remove the `"gitignore": res.Gitignore` key from the `output.Ok` map — this is an intentional output-envelope change, recorded in the module doc by card 52;
  the envelope invariant governs *using* `output.Ok`/`output.Err`, which is unaffected.
- **Commit:** `fix(fabricengine): unwire reverses wiring only, never deletes weft content`

### Card 50: recognise <hub>/.lyx as hub-level geometry

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `CloneHub`'s hub-materialisation step — the `os.MkdirAll(hubPath, 0o755)` at step 4 — also create `filepath.Join(hubPath, lyxdirs.DotLyxDirName)`, with a comment recording that `<hub>/.lyx` is a fabric-recognised hub-level geometry element the way `<hub>/_board` already is, that it stays a **real directory and not a junction** (the hub is not a git repo, so there is nothing to exclude and no weft to point at), and that it is reserved so no worktree slug can claim the name (card 37's slug-reservation set).
  A creation failure follows the same `teardownHub` posture the neighbouring clone steps use, or returns the error directly if it precedes the first tear-downable step — match whatever the surrounding step-4 code already does rather than inventing a new posture.
  Do **not** remove `reedengine`'s own idempotent `MkdirAll(HubLogsDir(e.layout))` in its boot path: it must still work on hubs created before this change, reed can boot without any fabric verb having run first, and its documented reason — the directory must exist and be pruned before the boot loop so a fresh server's log lands somewhere that already exists — is unaffected.
- **Commit:** `feat(fabricengine): create <hub>/.lyx during hub materialisation`

### Card 51: cover the .lyx junction lifecycle, adoption and preservation

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/fslink/fslink.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/unwire_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/clone_test.go`
- **Creates:**
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create `dotlyxjunction_integration_test.go` with `//go:build integration` first, modelled on `junction_pattern_integration_test.go` (which already resolves the exclude file the same way `seedGitExclude`/`unseedGitExclude` do) and using `lyxtest.CopyPaired`.
  Cover, as separate tests:
  (a) **lifecycle** — wiring creates the `.lyx` junction pointing at `<weft>/<AnchorRel>/.lyx`, seeds `.lyx` into the warp's `.git/info/exclude` **and** `.lyx/` into the weft's, and unwiring removes the junction and the warp entry;
  (b) **seeding order** — after wiring and before any weft-git verb runs, writing a file into the host-side `.lyx` leaves `git status --porcelain` in the weft worktree clean (the regression test for the ordering hole);
  (c) **adoption** — a pre-existing real `.lyx` directory holding files is moved into the weft target and replaced by a junction, and a second `reconcile` is a no-op;
  (d) **adoption collision** — an entry already present in the weft-side target aborts with an error naming the colliding path and leaves both sides untouched, with the host directory still a real directory;
  (e) **adoption does not over-reach** — a pre-existing real `_lyx` or `_pattern` directory still produces the hard refusal, asserted in the same test file as (c) and (d) so the two halves can never drift apart.
  In `unwire_test.go` (`//go:build integration`), rewrite `TestUnwire_ClearsWeftLyxOnlyNeverPattern` into a preservation test: after `Unwire`, both `<weft>/<AnchorRel>/_lyx` and `<weft>/<AnchorRel>/.lyx` still exist with their content, `WeftContent` reports `"preserved"`, and the weft branch carries **no** `"lyx fabric unwire: clear _lyx"` commit — assert the last by inspecting the weft log, not by counting commits.
  Delete `TestUnwire_RevertsGitignore` entirely and drop the file's `internal/gitignore` import;
  add an assertion to one surviving unwire test that the CLI envelope no longer carries a `gitignore` key.
  Update `TestUnwire_NeverWiredHostIsIdempotentNoOp`'s expectations, which already expect `"not_present"` and must keep passing.
  In `junction_pattern_integration_test.go`, update any expectation of the wired name-set's size or contents now that `.lyx` is a member.
  Check `healthreason_integration_test.go` and `cleanreason_integration_test.go` the same way: both drive `Healthy`/drift against wired fixtures, and `Healthy` now also requires the `.lyx` junction — fixtures wired through `WireJunctions` get it automatically, so expect updates only where a test enumerates junction names or counts rather than wiring through the real path;
  if none is needed, say so in the commit message rather than silently skipping the check.
  In `structuraldirs_test.go`, extend batch 7's trio: the wired set now contains `.lyx` while the routing set still does not — this is the one assertion that pins the deliberate asymmetry between the two sets.
  In `clone_test.go`, assert `CloneHub` creates `<hub>/.lyx`, and assert reed's own `MkdirAll` remains idempotent against an already-created directory (both halves, since the second is what covers pre-fix hubs) — if `clone_test.go` cannot reach `reedengine` without an import cycle or a tier violation, put that second half in the new integration file instead and say so in the commit message.
- **Commit:** `test(fabricengine): cover the .lyx junction lifecycle, adoption and unwire preservation`

### Card 52: record the junction, unwire and hub-geometry invariants

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/dotlyxjunction_integration_test.go`
  - `internal/fabricengine/structuraldirs_test.go`
  - `internal/fabricengine/unwire_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `internal/fabricengine/doc.go`
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `CONSTRAINTS.md`, add two clauses to the geometry sections this task has been building up: every host→weft junction is excluded through the warp's `.git/info/exclude`, never through a committed `.gitignore` in the user's repo;
  and unwiring reverses wiring only — it never deletes weft-side content.
  Add the hub-geometry clause too: `<hub>/.lyx` is a fabric-recognised hub-level element alongside `<hub>/_board` — created by fabric in `CloneHub`, a real directory and never a junction, and reserved so no worktree slug can claim it.
  The structural-directories and never-committed-routing clauses are **already present** — batch 7's card 44 added them when that change landed;
  extend them here only where this batch's wiring changes what they say (the wired name-set is now wider than the routing set by exactly `structuralNeverCommittedDirs`), and do not restate them.
  Place all of the above in the existing `## Durable-vs-Ephemeral State Invariant` section from batch 5 where they extend it, and in `## Fabric Git Invariant (warp + weft)` where they concern git behaviour, rather than opening a fourth geometry section.
  Name the enforcing tests (`internal/fabricengine/dotlyxjunction_integration_test.go`, `structuraldirs_test.go`, `unwire_test.go`).
  In `docs/overview.md`, update the **Junction model** section: the wired set is no longer purely the repo-wide `pathspec` list — it is `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the hub-reserved-filtered config names, deduplicated;
  the concrete junctions this repo ships with become three (`_lyx`, `.lyx`, `_pattern`);
  the sentence "A future weft-backed module is wired by appending its directory name to `pathspec`'s template default" survives but must now say that this applies to *optional* directories only;
  and the `.lyx/` bullet in the **Durable vs ephemeral state** section, which says `.lyx` is "Untracked (listed in `.git/info/exclude`, never `.gitignore`)", is now true of both repos and should name both.
  Also correct the claim that the default pathspec is `_lyx _pattern`.
  In `internal/fabricengine/doc.go`, update the passage describing the name-set as the config pathspec filtered against `HubReservedNames()`, and record the three operator-facing facts this batch establishes: downgrade is unsupported (a pre-fix `applyStaleRemoval` unwires `.lyx` and strands scratch inside the weft worktree);
  upgrade is signalled through health — an existing worktree reports the `.lyx` junction missing (`Healthy` false, `CauseJunctionMissing`) until `lyx fabric reconcile` runs adoption, which is the documented remedy;
  and the unwire output envelope changed (`weft_content` value set is now `"preserved"`|`"not_present"`, and the `gitignore` key is gone).
  Record there too that no code path removes a leftover committed `.gitignore` `.lyx/` block from a repo cloned by a pre-fix binary, with the manual remedy stated.
  In `manifest/designs/fabric-unified-view.md`, mark **Slice 9** shipped: rewrite its three bullets to describe what actually landed rather than what was planned — in particular that `.lyx` did **not** become "one more entry in the existing pathspec" but a structural, code-injected junction, since the shipped design deliberately contradicts the slice's own prediction — and keep the "Sequenced before slice 10" note, since slice 10 is still pending and still collides on `runCloneWithReset`.
  In `manifest/roadmap.md`, update the fabric item's slice status line so slice 9 reads as shipped and only slices 8 and 10 remain.
  Follow the repo's semantic-line-break markdown rule in every file.
- **Commit:** `docs: record the .lyx junction, unwire and hub-geometry contracts`

### Card 53: confirm no committed .lyx artifact and no .lyx in any pathspec

- **Context:**
  - `internal/fabriccli/clone.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/classify.go`
  - `internal/perchcli/run.go`
  - `internal/webstercli/sync.go`
  - `internal/buildercli/sync.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** verification-only gate, no edits.
  Confirm by grep across the whole repo that, in production code: no call to `gitignore.Ensure`/`gitignore.Remove` names `.lyx` (only `internal/vscode`'s `.vscode/` call remains);
  no `ScopedPathspec` call site anywhere passes `structuralNeverCommittedDirs` or `lyxdirs.DotLyxDirName` (check `internal/fabriccli/weft_verbs.go`, `internal/perchcli/run.go`, `internal/webstercli/sync.go`, `internal/buildercli/sync.go`, and `internal/fabricengine/unwire.go` if any pathspec construction survived card 49);
  no production file outside `internal/lyxdirs` contains the literals `"_lyx"` or `".lyx"` in path-construction context (this duplicates the machine check in `internal/lyxcwd/enforcement_test.go` and should already pass — a hit means a card reintroduced one);
  and `crossModuleMachineLocalExcludes` no longer appears anywhere, including in comments.
  If any check fails, fix it under the card that owns the file rather than here, then re-run this gate.
  Report the four grep results in the batch's implementer output so the reviewer can see them without re-running.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/fabricengine/... ./internal/fabriccli/... && go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` — both edited Go packages, with the tagged run mandatory here rather than merely useful: every one of this batch's behavioural claims (junction creation, exclude seeding on both sides, adoption, collision refusal, unwire preservation, hub-`.lyx` creation) requires a real paired git fixture and lives behind `//go:build integration`.

Covered files: `internal/fabricengine/dotlyxjunction_integration_test.go` (new), `unwire_test.go`, `junction_pattern_integration_test.go`, `structuraldirs_test.go`, `clone_test.go`, plus the package's reconcile/drift/health suites (`reconcile_stale_removal_test.go`, `reconcile_stale_registration_test.go`, `boardjunction_integration_test.go`, `remove_junctions_integration_test.go`, `config_driven_junctions_integration_test.go`, `healthreason_integration_test.go`, `cleanreason_integration_test.go`, `open_integration_test.go`, `ready_integration_test.go`) re-run as the regression net for "reconcile/drift/health still converge with `.lyx` in the wired name-set" — that claim is asserted by those existing suites continuing to pass with the widened set, not by a new test.

Cross-platform: junction creation goes through `internal/fslink`, already covered by `fslink_test.go`, so no new platform-specific assertion is added beyond keeping every new path assertion off hard-coded separators.
Adoption's busy-directory behaviour is the one genuinely platform-divergent case and is asserted through its **error contract** — the wrapped message names the entry and the stop-the-daemons remedy — rather than by simulating an open handle, which is not portably reproducible under `go test`.

The two assertions that carry this batch are the adoption trio and the seeding-order test.
Cards (c), (d) and (e) must live in one file and be asserted together, because an adoption branch that over-reaches passes (c) while silently breaking `_lyx`'s and `_pattern`'s refusal — the guard whose whole purpose is never touching what might be the user's hand-authored content.
And (b) is the only proof that the exclude entry exists **before** the first write: seeding it from `ensureWeftLockDir` alone would pass every other test in this file while leaving scratch as untracked dirt during the window that trips `Remove`'s no-force dirty gate.
