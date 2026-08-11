# Discussion: fabric: live-state integration harness (slice 13)

```yaml
task: 'fabric: live-state integration harness (slice 13)'
slug: fabric-live-state-harness
status: discussing
parent: main
```

## Problem

The fabric v2 crucible campaign (slice 11) produced 81 findings, 9 BLOCKING, **8 of them data-loss**.
Every one of those eight was found by driving real `git` against a real filesystem in hostile or dirty state.
Not one was found by the hermetic test suite, which was green before, during and after each defect existed in the tree.

That is not a gap in any individual test.
It is a gap in what the hermetic suite is *able* to express: it never builds a real fabric hub, so it can never put one into a dirty or hostile state, so it can never observe a verb destroying something.
The `//go:build integration` tier covers that ground today, but per-verb and ad hoc — only where someone thought to write a test.
That is precisely how `remove ..` escaped four consecutive review rounds before destroying an entire hub and then reporting `{"error":"failed to check worktree status","ok":false}` — claiming it had done nothing.

**Why now:** slice 12 (`fabric-destructive-chokepoint`) landed 2026-08-11 as `3184cd5a`, routing roughly 29 destructive call sites through one four-check gate in `internal/fabricengine/destroy.go`.
Slice 12's own static guard proves *no call site bypasses the gate*.
Nothing yet proves *the gate behaves correctly once reached*.
That is this task, and it is the reason the fabric chain is serial: cells asserting on refusal behaviour could not have been written before the gate that produces that behaviour existed.

The deliverable is a cross product, not a pile of cells.
A new verb appended to the table inherits every state; a new state inherits every verb.
That property is what the current per-verb integration tests do not have.

## Scope

**In:**

- A new package `internal/fabricengine/fabrictest`, mirroring `internal/boardengine/boardtest`: its own `doc.go` plus a `testmain_test.go` calling `lyxtest.HermeticGitEnv()`.
- A **hub factory** that builds a real fabric hub by *running clone*, from local bare remotes, at full CLI fidelity (junctions wired, repo-wide `fabric.yaml` present).
- A small production extraction in `internal/fabriccli`: the engine-y middle of `runCloneWithReset` becomes an exported `CloneAndWire`, called by both the cobra handler and the factory.
- A **named hostile-state matrix** (9 states), a **verb table** (9 gate-reaching verbs plus hostile inputs), and the **cross product driven** with per-cell survival assertions.
- A **whole-hub manifest capture and diff** with prefix-rooted permit lists — the mechanism that catches destruction nobody thought to assert on.
- Two refusal-expectation helpers — `RefusedByGate(err, check)` and `RefusedBefore(err, substring)` — since most tranche-1 refusals fire in a verb's own pre-flight and never reach the gate.
- Owner rows for `internal/fabricengine/fabrictest` in `fabricVocabularyOwners` and `weftnameImportOwners` (`internal/lyxcwd/enforcement_test.go`), plus the matching `CONSTRAINTS.md` owner-set update — required in the same commit or `go test ./...` fails.
- A `fabrictest` subdirectory exclusion in `cmd/lyx/destructiveguard_test.go`'s walk, plus the matching `CONSTRAINTS.md` scope clarification — likewise required in the same commit, and for the same reason.
- Every cell run on both a `.`-anchored and a `--subpath backend` hub.
- Positive expected-effect assertions on every `clean`-state cell, so an over-refusing gate cannot pass the matrix.
- The one movable duplicate `gitStatusPorcelain` (the `fabricengine_test` copy) folded into `fabrictest`.

**Out:**

- Converting the existing `CloneHub(` call sites to the factory — **100 calls across the 7 `package fabricengine_test` files, plus 2 more in the in-package `clone_test.go`** (102 in the tree overall, across 8 files).
  They stay as they are; conversion is opportunistic later work or belongs to `lyxtest-real-hubs`.
- `internal/fabricengine/clone_test.go` — in-package, keeps its own setup, explicitly not a defect to fix here.
- The in-package `gitStatusPorcelain` in `commitweftat_test.go`.
  It is stuck there permanently: `package fabricengine` importing `fabrictest` would close an import cycle.
- The deferred matrix axes named by the design doc's scope note: **concurrency between worktrees**, **the hook surface** (non-executable user hook, `core.hooksPath`), **`_portals`/`_launchers` as a state axis** (stale portal link).
  These land alongside slice 14.
- Slice 14's truthfulness assertions ("the report was truthful").
  The result envelope does not yet support them.
- Any change to production behaviour.
  `CloneAndWire` is a pure extraction: same sequence, same order, same errors.
- Read-only verbs (`List`, `Status`, `Pairs`, `Diff`).
  They destroy nothing, so every such cell would assert a tautology.
- Closing the Windows verification gap.
  See the Windows decision below — the harness is built to run there, but has not been run there.

## Decisions

### hub-factory-fidelity

- **Decision:** Extract the engine-y middle of `internal/fabriccli/clone.go`'s `runCloneWithReset` into an exported `fabriccli.CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error)`.
  The cobra handler becomes a thin wrapper that formats the result envelope; `fabrictest`'s factory calls the same function.
- **Rationale:** `fabricengine.CloneHub` alone produces a **partial** hub — warp clone, weft clone, `_board`, `.lyx-anchor`, warp binding — but **no junctions and no repo-wide `fabric.yaml`**, because those are driven by the CLI layer through `internal/configsync`, which `fabricengine` must never import.
  Three of the gate's eight path-ownership kinds (`ownedWiredJunction`, `ownedDriftedWiredJunction`, `ownedUnderGeometryRoot`) are structurally unreachable on a clone-only hub.
  R1's tracked-symlink-at-a-wired-path defect and R5's `remove ..` `_launchers` defect both live exactly there.
  One shared function means the harness can never drift from the hub the CLI actually produces.
- **Rejected:** replicating the wiring sequence inside `fabrictest` (duplicates the handler and will silently drift — a harness asserting on a hub shape the CLI no longer produces is the exact blindness this slice exists to remove);
  `CloneHub` only (three ownership kinds unvalidatable);
  driving the real `lyx` binary (~2x slower at 101-110 ms vs 60-66 ms serial, forces JSON parsing instead of Go-value assertions, and gives up `errors.As`/message inspection on the refusal entirely).

### survival-assertion-mechanism

- **Decision:** Whole-hub **manifest diff** plus **planted sentinels**.
  Capture the hub's manifest before the verb runs and again after; diff; fail on any disappearance or mutation the cell did not explicitly permit.
  Sentinels are named files planted by each state so failure messages are legible.
- **The per-entry record, stated precisely.**
  A manifest is a map from a `filepath.ToSlash`-normalised **hub-relative path** to an entry carrying exactly three things: the **kind** (`file`, `dir`, or `link`);
  for a link, its **raw one-hop target** via `fslink.RawTarget` (not `PointsTo` — the same reason `ownedWiredJunction` uses the raw target: a fully-resolved chain fails outright when a later segment is gone, and silently walks past a target that is itself a further link);
  and for a file, a **content hash**.
  Deliberately **not** recorded: permission bits (meaningless on Windows), mtime, and size-without-hash.
  This is the minimum that makes all three of "it vanished", "it was replaced by a different kind of thing", and "its content changed" observable, and nothing beyond it.
- **The `.git` rule: a narrow allowlist, not a blanket exclusion.**
  Inside any `.git` directory — or a `.git` *file*, which is what a linked worktree carries — the manifest records **only**: the `.git` entry itself, and each `.git/worktrees/<name>` directory, both at existence granularity with no content hash.
  Everything else under `.git` is excluded.
  A **blanket** `.git` exclusion was rejected because it would blind the harness to **linked-worktree deregistration**, and that is R3's shape exactly — `prune` removing a path git had just refused to remove leaves the registration behind or tears it out, and neither is visible if `.git` is invisible.
  Hashing everything under `.git` was rejected for the opposite reason: `index`, `logs/**`, `refs/**`, `objects/**`, `FETCH_HEAD`, `ORIG_HEAD` and `packed-refs` all churn on ordinary reads and writes, so every cell would drown in diff noise and the permit lists would grow until they permitted everything.
  **Git-admin paths that remain observable under this rule:** the presence of each repo's `.git`, and the presence of each linked worktree's registration directory.
  Branch existence is deliberately **not** carried by the manifest — it is asserted through git itself in the per-verb effect assertions, where the answer is authoritative rather than inferred from a ref file.
- **Naming: the harness operation is "manifest", never "snapshot".**
  `internal/fabricengine/snapshot.go` already owns Snapshot in fabric's vocabulary — the `Snapshot: <tag>` trailer recording a warp SHA on the weft branch — and reusing the word for a filesystem capture would collide with it inside the very package the harness drives.
  `doc.go` states the distinction so a reader arriving from `snapshot.go` is not misled.
- **Rationale:** All eight data-loss defects destroyed something **nobody was asserting on**.
  A permit-list diff is what catches instance nine; a sentinel-only approach only ever catches what the test author already thought of, which is the model that missed eight defects.
  Sentinels alone would report "a path list changed"; the pair reports "the operator's uncommitted file is gone".
- **Rejected:** sentinels only (that is the current per-verb integration tests' model);
  manifest diff only (failure triage degrades to reading a path diff).

### manifest-permit-granularity

- **Decision:** **Prefix-rooted permits.**
  A cell names permitted *roots*; everything at or below a permitted root may vanish or change.
  Anything outside every permitted root that disappears or changes is a failure.
- **Rationale:** `Remove <slug>` legitimately deletes a worktree, its weft sibling, its junctions, its launchers and its portal — a dozen-plus paths whose internal file list is not this task's business.
  Prefix roots stay concise, stay stable against a pair's internal layout changing, and still fail loudly on anything outside those roots — which is the shape all eight defects took without exception.
- **Refusal cells carry permit roots too — a refusal is not assumed side-effect-free.**
  Every expectation kind carries a permit-root field, including the two refusal kinds.
  The reason is concrete and verified: `Remove` runs `removePortal` and `removeLaunchers` at `remove.go:61-66`, **before** its own dirty pre-flight at `remove.go:68-76`.
  A dirty-`Remove` cell that correctly refuses has therefore **already destroyed** `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>`.
  Those two paths are declared as permitted roots on that cell, so the matrix is green against current deliberate behaviour.
- **That anomaly is recorded, not silently normalised.**
  "The verb returned an error and destroyed something anyway" is the exact shape of all eight campaign defects, so a cell permitting it must say so out loud rather than bury it in a permit list.
  It is current intended behaviour — the comment at `remove.go:58-60` is explicit that portal and launcher cleanup runs before the git removal so it still fires when the worktree directory is already gone — and this task must not unilaterally redefine it.
  `doc.go` names it as the one known case in tranche 1 where a refusal is not side-effect-free, and flags it for slice 14's truthfulness work, where "what did this call actually mutate before it failed" becomes representable in the result envelope.
- **Rejected:** exact-path permits (hundreds of entries per cell, breaks on any layout change, and would in practice be regenerated-from-actual, at which point it asserts nothing);
  per-cell predicate functions (full expressiveness, but each cell carries bespoke logic and the declarative table is forfeited);
  empty permit roots on every refusal cell (the framing "refuses, and the hub still exists" implies it, but it is factually wrong for the single most important verb in the matrix and would fail against a correct binary);
  treating the pre-refusal portal/launcher removal as instance nine and asserting against it (it is deliberate, documented behaviour, and a harness task is the wrong place to reverse a production decision unilaterally).

### cross-product-shape

- **Decision:** `[]State × []VerbCase`, one `t.Run(state.Name + "/" + verb.Name)` per cell, each cell `t.Parallel()` on its own freshly built hub.
  Roughly: `State{Name string; Apply func(testing.TB, *Hub)}`, `VerbCase{Name string; Run func(testing.TB, *Hub) error}`, and a per-cell expectation carrying the permitted-removal roots, the optionally-expected refusing check, and the clean-state expected effect.
- **Rationale:** appending a verb to the table makes it inherit every state automatically, which is the stated property of the deliverable.
  Every cell already owns its own hub for correctness (independent bare pairs so pushes never race), so `t.Parallel()` is free.
- **Rejected:** a flat explicit list of `{state, verb, expectation}` triples — more readable per cell, but a new verb does not inherit the states, forfeiting the cross-product property that is the whole point.

### where-the-suite-lives

- **Decision:** Both the factory and the matrix suite live in `internal/fabricengine/fabrictest`.
  The package is a **hybrid of two existing precedents**, and neither one alone covers it: `boardtest` supplies the untagged-`doc.go`-plus-tagged-`*_test.go` suite shape (it holds only `doc.go` and five `*_test.go` files — **no** non-test helper source), while `lyxtest` supplies the non-test-helper-package shape the factory needs in order to be importable by `fabricengine_test` at all.
- **Rationale:** the matrix drives only the **exported** verb surface.
  A cell that needs an `export_test.go` shim is not testing the verb's public contract, it is testing an internal.
  Cases genuinely needing the gate's unexported predicates stay in `internal/fabricengine/destructivegaps_integration_test.go` where they already are — an honest cost, not a hidden gap.
- **Hard constraint discovered during exploration:** `internal/fabricengine/export_test.go` is `package fabricengine`, so its shims (`IsWarpCheckoutForTest`, `DeleteBranchForTest`, `LooksLikeHubForTest`, `RemoveWarpWorktreeDirForTest`, …) are visible **only** to `package fabricengine_test` files *in that same directory*.
  `fabrictest` is a different package in a different directory and can never see them.
  This is not a workaround-able limitation; it is a Go language rule.
- **Rejected:** putting the matrix in `package fabricengine_test` and importing `fabrictest` only for the factory (gains the shims, but splits the deliverable across two locations, does not match `boardtest`, and lets cells quietly become white-box);
  a deliberate split of black-box matrix plus white-box addendum (two matrices is two things to keep in sync).

### refusal-check-assertion

- **Decision:** Assert which check refused by **substring-matching the refusal message**, via a `fabrictest.RefusedByGate(err error, check Check) bool` helper and exported `Check` constants in `fabrictest`.
- **Rationale:** `*destructiveRefusal` is unexported, so `errors.As` is unavailable from `fabrictest`.
  The message format is `refusing to <what>: <check> check failed for <target>: <reason>`, so the helper searches for `"<check> check failed"` — unambiguous, and it survives call-site wrapping because it searches the full error string.
  The message *is* slice 12's step-5 honest-reporting contract, so pinning it tests something real rather than a proxy.
  Slice 14 rewrites the result envelope one slice later, so exporting a refusal type now is dead weight that gets replaced.
- **The exported `Check` set has three members, not four: `CheckContainment`, `CheckOwnership`, `CheckDirtiness`.**
  `checkForce` is declared at `destroy.go:39` and rendered by `String()` at `destroy.go:51`, but is **never constructed into a `destructiveRefusal`** anywhere in the tree — force is consulted only inside `checkPathDirtiness`, where it causes the dirtiness check to *pass*, so a refusal is never attributed to it.
  A `CheckForce` constant could therefore never match and must not be offered;
  `doc.go` records why, so a future reader does not add it back believing the gate simply forgot to emit it.
- **Rejected:** exporting `DestructiveRefusal` with an exported `Check` field (assert-by-value, immune to rewording, but a production API change in an additive slice, superseded by slice 14);
  asserting refusal only without the check (drops a stated requirement, and would pass against a gate refusing everything for the wrong reason).

### refused-before-the-gate-vs-by-the-gate

- **Decision:** A cell declares one of two **refusal** expectation kinds, and the verb table records which kind each cell uses:
  `RefusedByGate(check)` — the error carries `"<check> check failed"`, so the gate refused;
  `RefusedBefore(substring)` — the verb's own pre-flight refused, and the cell pins that verb's own message instead.
  A third, non-refusal kind (`Proceeds`) is added by the three-expectation-kinds decision below;
  these two cover only the cells where the verb is expected to stop.
- **Why this is not optional:** most refusals in tranche 1 **never reach the gate**.
  Verified sites: `remove.go:45` calls `validateWorktreeSlug` and returns a plain `fmt.Errorf` before any `pathRequest` exists;
  `remove.go`'s own `worktreeDirty(scopeAll, target)` pre-flight returns `fmt.Errorf("worktree has uncommitted changes; use --force")` — **byte-identical to the gate's dirtiness reason at `destroy.go:564` but without the `dirtiness check failed` prefix**, so a substring assertion on the gate form silently fails there;
  `checkout.go:42-47` returns `fmt.Errorf("weft worktree has uncommitted changes; stash or commit before checkout")`.
  Without this decision the matrix's central assertion would mis-specify on the most obvious dirty-`Remove` cell in the whole table.
- **Rationale for pinning the layer rather than erasing the distinction:** which layer refused is *signal*, not noise.
  R5's defect was precisely a pre-flight check that did not exist, and R3's was a gate-side fallback with no dirtiness check.
  A cell that records the layer fails when a future refactor moves a check across that boundary in either direction, which is exactly the drift this harness exists to detect.
- **Rejected:** asserting only "refused + survived" with no check name (never wrong, but drops a stated requirement and cannot distinguish a correct refusal from an accidental one);
  normalising all nine verbs so every refusal routes through the gate (one expectation kind and the cleanest end state, but a substantial production change in a slice scoped as additive, overlapping slice 14's envelope rewrite).

### tranche-1-verb-table

- **Decision:** The nine gate-reaching verbs.
  `Topology.Remove`, `Topology.Prune`, `Topology.Cleanup`, `Topology.Checkout`, `Topology.Reconcile`, `fabricengine.UnwireJunctions`, `Topology.Add` (its rollback path), `Fabric.Pull` (its `ResetHard`), and `fabricengine.CloneHub{Reset: true}` (its hub teardown).
- **Rationale:** this is exactly the set that reaches an executor in `destroy.go`.
  Every executor gets driven.
- **Hostile inputs are not a uniform row.**
  `""`, `.`, `..`, `../x`, a `-weft`-suffixed name, a reserved hub name (`_board`, `_portals`, `_launchers`), and a leading `-` apply only to the verbs that accept a slug or branch argument: **`Add`, `Remove`, `Checkout`**.
  `Prune`, `Cleanup`, `Reconcile` and `Unwire` take no such argument, so for them the hostile-input axis is empty and the cell must not fabricate one.
  `UnwireJunctions` takes a `names []string` — the hostile input there is a junction name escaping its worktree (`../x`), which is its own shape.
- **The `CloneHub{Reset: true}` column is re-scoped to the ownership axis, not run against the nine states.**
  `resetHub` declares `dirtinessNA` (`clone.go:585`), so **no dirtiness state can ever refuse it** — total hub destruction on `--reset` is correct behaviour, deliberately so.
  That makes the permitted root the hub root itself, which would render the manifest diff vacuous across all nine states.
  Instead this column drives `Reset` against two ownership-shaped targets and asserts refusal plus full survival: a directory that is not a hub at all, and a directory *named* `<derived>-HUB` that is not a hub — the exact shape of R4's `clone --reset` defect, which destroyed any directory matching a name fabric *derives* rather than one the operator types.
  The dirtiness rows for this column are **omitted with the reason stated in the table**, never silently present-and-vacuous.
  The rebuild half is asserted through `CloneAndWire`, so the hub that comes back is full-fidelity rather than `CloneHub`-partial.
- **Hostile-input cells run in the `clean` state only, on both anchors.**
  Crossing them with the full state matrix would give 7 inputs × 3 verbs × 9 states × 2 anchors ≈ **378 cells**, nearly all vacuous: `remove ..` refuses at `remove.go:45` before any hub state is consulted, so eight of its nine state rows assert exactly what the ninth does.
  That is precisely the vacuity the `Reset` column was re-scoped to avoid, and permitting it here would contradict that decision one paragraph later.
  **Expressed without forfeiting the cross-product property:** a `VerbCase` carries an optional `States` restriction;
  when empty — the default, and what every ordinary verb uses — the case inherits **every** state, so a newly appended verb still gets the full matrix automatically.
  Only the hostile-input cases set it, to `clean`.
  The restriction is a property of the *case*, not a special path in the driver, so the driver stays one uniform loop.
- **Resulting tranche-1 cell count:** 8 ordinary verbs × 9 states × 2 anchors = **144**, plus 7 hostile inputs × 3 verbs × 1 state × 2 anchors = **42**, plus the `Reset` column's 2 ownership targets × 2 anchors = **4**.
  **190 cells total**, at roughly 24 ms of hub cost each, parallel.
- **Leading-`-` and `Checkout` hostile-input cells assert survival and no-partial-mutation, with no check-name assertion, and are written against the *safe* expectation.**
  A leading `-` passes `validateWorktreeSlug` untouched — it is not on any of that function's five rejection rules — so no gate check owns it and neither expectation kind from the refusal decision applies.
  The cell therefore asserts that the argument **does not reach git as a flag**, and that nothing outside the permitted roots is destroyed.
  If the current code does let it through as a flag, the cell fails, and that failure is instance nine — which is the point of the tranche.
  `Checkout` validates no branch at all (`checkout.go:38-85` goes straight to `git switch`), so its hostile set is **branch-shaped rather than slug-shaped**: a nonexistent branch and a flag-shaped name.
  Those cells additionally assert **no half-switched pair**, since `Checkout` is documented as all-or-nothing with rollback on both sides.
- **Rejected:** only the four verbs from the evidence table (`remove`, `prune`, `cleanup`, `pull`) — leaves `Checkout`'s `branch -D`, `Reconcile`'s link repoint and `Add`'s rollback with no cell at all;
  all exported verbs including read-only ones (every such cell asserts a tautology);
  dropping the `Reset` column entirely (honest and smaller, but leaves R4's defect with no cell in the matrix);
  keeping `Reset` in the uniform nine-state product and accepting ~18 vacuous cells (preserves table uniformity at exactly the cost the permit-list decision already rejected);
  moving the leading-`-` and `Checkout` hostile inputs out of tranche 1 (defensible, but discards the input class most likely to hide an undiscovered defect, in the tranche whose job is finding one);
  asserting whatever the current behaviour happens to be (regenerated-from-actual, asserts nothing).

### tranche-1-state-matrix

- **Decision:** Nine named states, each traceable to the defect it was born from:
  `clean`, `dirtyWarpTracked`, `dirtyWarpUntracked`, `dirtyWeftTracked`, `bothDirty`, `trackedSymlinkAtWiredPath` (R1), `foreignDirAtFabricOwnedPath` (R4), `unrelatedGitCloneAtWeftNamedPath` (R4), `staleWiredJunction` (a drifted/dangling link, R1's repair-side counterpart).
- **Rationale:** the four dirtiness states reach only the gate's dirtiness check.
  Containment and ownership — the two checks `--force` can never override, and the two that between them account for the worst defect in the campaign (`remove ..` destroying an entire hub) — need the link/foreign-path states.
  Every state citing its originating defect keeps the matrix auditable against the evidence table rather than aspirational.
- **Rejected:** only the four dirtiness states (matches R3's proven 40-cell matrix, but leaves containment and ownership unexercised);
  all eleven states the design doc lists (the extra three are the hook surface and stale portals, explicitly deferred by the scope note).

### dirty-what-per-cell

- **Decision:** A dirtiness state dirties **the checkout the verb under test actually acts on**, resolved per cell.
  `Remove <slug>` dirties that pair's worktree; `Pull` dirties the prime warp (the checkout `ResetHard` targets); `Cleanup` dirties the branch's checkout; and so on.
- **Rationale:** a tranche-1 hub has at least four checkouts (prime warp, prime weft sibling, `_board`, and each added pair's warp worktree plus weft sibling), and the gate probes a different one per verb.
  A state that dirties a checkout the verb never touches asserts nothing.
  R2 (`pull` discarding uncommitted tracked warp work) and R3 (`prune` removing a path git had just refused) were both about the verb's **own target** being dirty.
- **Rejected:** dirtying every checkout in the hub for every dirtiness cell (uniform and simple, but then no cell distinguishes "refused because my target was dirty" from "refused because something unrelated was dirty", which is most of the signal);
  a separate `dirtyElsewhere` state alongside `dirtyTarget` (a genuine assertion — a verb must *not* refuse over an unrelated dirty worktree — but it doubles the dirtiness rows, and over-refusal is already covered by the clean-state effect assertions below).

### three-expectation-kinds-and-the-scope-table

- **Decision:** There are **three** expectation kinds, not two.
  Alongside `RefusedByGate(check)` and `RefusedBefore(substring)`, a cell may declare **`Proceeds`** — the verb succeeds despite a non-clean state, its intended effect lands, and the state's planted content survives.
  All three carry a permit-root field.
- **Why a two-kind scheme is wrong:** dirtiness scope is per-verb, so a dirtiness state does **not** imply refusal.
  `Checkout` probes `worktreeDirty(scopeTracked, weftWorktree)` at `checkout.go:42`, so `dirtyWarpUntracked` — and any untracked-only weft state — must make `Checkout` **succeed**, with the untracked file still on disk afterwards.
  Under the two-kind scheme that cell had no derivable expected outcome at all and would have been written regenerated-from-actual.
- **The scope table, so every dirtiness cell's outcome is derived rather than guessed.**
  Verified against the tree:

  | verb | probe | scope | probed path |
  |---|---|---|---|
  | `Add` | own pre-flight (`add.go:43`) | `scopeTracked` | prime warp worktree |
  | `Checkout` | own pre-flight (`checkout.go:42`) | `scopeTracked` | weft worktree |
  | `Remove` | own pre-flight (`remove.go:69`) | `scopeAll` | the pair's warp worktree |
  | `Remove` | `refuseDirtyWeftWorktree` (`remove.go:79`, `:144`) | `scopeAll` | the pair's weft worktree |
  | `Remove` | gate (`remove.go:196`, `:230`) | `dirtyScopeAll` | worktree / launchers |
  | `Prune` | own pre-flight (`prune.go:214`) | `scopeTracked` | weft path |
  | `Prune` | gate (`prune.go:269`, `:292`) | `dirtyScopeTracked` | stale pair paths |
  | `Pull` | own pre-flight (`pull.go:143`) | `scopeTracked` | prime warp worktree |
  | `Pull` | gate, via `Fabric.ResetHard` (`destroy.go:762`) | `dirtyScopeTracked` | prime warp worktree |
  | `Reconcile` | own pre-flight (`reconcile.go:301`) | `scopeAll` | `_board` |
  | `Cleanup` | gate, branch-shaped | `dirtyCheckedOutBranch` | n/a — is the branch checked out anywhere |
  | `UnwireJunctions` | gate, link-shaped | `dirtinessNA` | n/a |
  | `CloneHub{Reset}` | gate (`clone.go:585`) | `dirtinessNA` | n/a |

- **The derivation rule:** a `scopeTracked` verb against an untracked-only state expects `Proceeds`;
  a `scopeAll` verb against the same state expects a refusal.
  That divergence is the one case proving dirtiness scope is a real per-request parameter rather than a constant, which is exactly what `TestWorktreeDirty_BothScopesAcrossFourStates` already asserts at the unit level and what this matrix now asserts at the verb level.
- **Rejected:** leaving non-clean success cells undefined (they would be written from observed behaviour and assert nothing);
  giving every dirtiness cell a refusal expectation (factually wrong for every `scopeTracked` verb against an untracked-only state, and would fail against a correct binary).

### clean-state-effect-assertions

- **Decision:** Every `clean`-state cell asserts the verb **succeeds and had its intended effect**, not merely that nothing was destroyed.
- **Rationale:** this is the tautology guard.
  Without it, a gate that refused every request would pass every cell in the matrix — and slice 12 just rewired 29 call sites into that gate.
  It also makes the matrix a behaviour spec rather than only a refusal spec, at the cost of one expectation field per verb.
- **Per-verb intended effect:** `Add` → worktree, weft sibling, branch, junctions, launchers and portal all exist;
  `Remove` → all of those gone, hub otherwise intact;
  `Prune --apply` → stale pairs gone, live pairs intact;
  `Cleanup --apply` → orphan managed branches gone, primary weft branch intact;
  `Checkout` → prime warp on the branch, weft on the corresponding weft branch;
  `Reconcile` → returns pairs, no error, no mutation outside repair roots;
  `UnwireJunctions` → junctions gone, worktree intact;
  `Pull` → warp advanced to upstream;
  `CloneHub{Reset:true}` → hub torn down and rebuilt, the rebuild driven through `CloneAndWire` so junctions and the repo-wide `fabric.yaml` are present in the result (a `CloneHub`-only rebuild would reproduce exactly the partial-hub shortfall the factory decision exists to avoid).
- **Rejected:** survival assertions only (matches the design doc's literal "assertions on what must SURVIVE" framing, but an over-refusing gate becomes indistinguishable from a correct one);
  additionally asserting that `--force` *does* override dirtiness (strictly more coverage and a good later addition, but it adds a force axis to the state matrix, which is growth this tranche does not need to validate the gate).

### subpath-axis-in-tranche-1

- **Decision:** The full tranche-1 matrix runs on **both** a `.`-anchored hub and a `--subpath backend` hub.
- **Rationale:** the design doc calls the anchor/subpath mechanism "the campaign's number one concern throughout, and the axis most likely to differ".
  `_portals` and `_launchers` paths are literally `AnchorRel`-interpolated (`<hub>/_launchers/<anchor>/<slug>`), which is where a containment bug hides.
  At ~24 ms per hub concurrent, doubling hub count is not a cost concern.
- **Rejected:** `.` only (matches the scope note's literal "full subpath coverage is deferred", halves runtime, but leaves the axis the campaign worried about most entirely undriven by the tranche meant to validate the gate);
  both anchors only for containment-reaching cells (targets the risk precisely, but costs a second differently-shaped matrix and forfeits the uniform cross product).

### bare-remote-template-strategy

- **Decision:** One `sync.Once`-cached template pair per test binary, mirroring `lyxtest`'s existing `buildWarpHub`/`buildWeftPrime` pattern.
  The warp template carries **both** a root `README` and a `backend/` subdirectory, so one template serves both anchors — a `.`-anchored hub simply ignores `backend/`.
  Each scenario gets its **own** copy of the bare pair, so cells push independently without racing.
- **Two gotchas encoded exactly once, in the template builder:**
  1. `git init --bare` leaves `HEAD` on `master` while the pushed branch is `main`, so the builder must run `git -C <bare> symbolic-ref HEAD refs/heads/main`.
  2. The **weft** bare must be left genuinely empty — never pushed to — or `CloneHub`'s bootstrap guard (`clone.go:172`, `!probe.WeftLooksLikeWeft`) refuses it.
     The warp bare, by contrast, must have content pushed.
- **Rationale:** copy-the-bares is ~2 ms against a ~22 ms clone; building fresh bares per scenario replaces that with a full `git init --bare` plus work-repo plus commit plus push, which at ~160 cells is real wall-clock.
  Two separate templates would duplicate both gotchas for a difference no cell asserts on — and a `.`-anchored repo that happens to contain a `backend/` directory is the *more* realistic fixture anyway.
- **Rejected:** two template pairs, one flat and one with `backend/`;
  building fresh bares per scenario with no template.
- **Important:** `lyxtest`'s own bares **cannot** be reused.
  `initBareRemote` creates them and adds them as `origin` but leaves them **empty, never pushed to** — which is exactly why the `symbolic-ref HEAD` gotcha never arises there.
  `fabrictest` needs its own pushed-to bare builder.

### runtime-and-parallelism

- **Decision:** Every cell `t.Parallel()` on its own hub, no hand-rolled worker cap, and the measured wall-clock recorded in `fabrictest`'s `doc.go`.
- **Rationale:** clone is `fork`/`fsync`-bound and scales 5.2x on 14 cores (37% of linear).
  Every cell already owns its hub for correctness.
  Go's `-parallel` flag already bounds concurrency, so a hand-rolled cap duplicates the toolchain.
  Recording the number makes a future regression visible to a reader without adding a flaky timing assertion.
- **Rejected:** an explicit worker cap;
  a failing wall-clock guard (rejected on the same grounds the repo rejects timing assertions elsewhere — it fails on a loaded CI box, not on a real regression).

### windows-gap-and-portability

- **Decision:** No `runtime.GOOS == "windows"` skips on the harness's own states or cells.
  A named section in `fabrictest`'s `doc.go` states the gap and points at `manifest/designs/fabric-windows-verification.md`.
  **In addition:** the harness is written Windows-portable *by construction*, so that a future Win11 run is a run-and-fix exercise rather than a rewrite.
- **Rationale:** the harness is built to run on Windows and would run there if invoked.
  What is unverified is that anyone *has* run it — a skip would assert the opposite, and would bake in the assumption that it never will be.
  The operator has stated the intent to run the whole suite on Win11 later and fix what breaks then.
- **Concrete portability rules the implementation must follow:**
  - **Never** `os.Symlink`, `os.Readlink` or `os.Lstat`-based link inspection directly.
    All link creation and inspection goes through `internal/fslink`: `CreateDirLink`, `IsLink`, `PointsTo`, `RawTarget`, `Remove`, `RemoveLinksIn`.
    Windows uses directory junctions, which need no special privileges; Windows *file* symlinks need admin or Developer Mode and must not be relied on.
  - **Launcher filenames are GOOS-branched** — `.cmd` on Windows, `.sh` elsewhere, with `ide-menu.cmd`/`ide-menu.sh` for the menu launcher (`internal/fabricengine/launchers.go`, `launcher_content.go`).
    A permit-list entry or effect assertion naming `ide-menu.sh` literally would fail on Windows.
    Match launcher paths by directory root or by the same GOOS branch, never by a hardcoded extension.
  - **Manifest keys** are stored as `filepath.ToSlash`-normalised paths relative to the hub root, so a permit-list literal written with `/` works on both platforms.
  - **Path comparison** in the manifest diff must be case-insensitive on Windows.
    `lyxcwd.samePath` (`internal/lyxcwd/anchor.go:112`) does exactly this via `strings.EqualFold`, but is **unexported** — `fabrictest` needs its own equivalent rather than reaching for it.
  - **Bare-remote URLs** passed to git go through `filepath.ToSlash`, as the existing `makeBareRemote` helper already does.
  - **No file-mode dependence.**
    The manifest must not hash or compare Unix permission bits, and no state may rely on `chmod` — `0o644`/`0o755` are meaningless on Windows, and the existing `RefusesUnreadableDirectory` subtest already skips there for exactly this reason.
  - **Keep template and slug names short.**
    `<hub>/_launchers/<anchor>/<slug>/<script>` nests deep, and Windows still has a 260-character default path limit.
- **Known divergence to accept, not paper over:** the `trackedSymlinkAtWiredPath` state models R1's defect, a *git-tracked symlink*.
  On Windows that requires `core.symlinks=true` plus Developer Mode.
  Build the state through `fslink.CreateDirLink` so it materialises as a junction there; the gate's `ownedWiredJunction` check compares `fslink.RawTarget`, so the assertion keeps its shape even though the on-disk artifact differs.
  Record this divergence in `doc.go` rather than hiding it behind a skip.
- **Rejected:** `runtime.GOOS` skips on the link-shaped states (honest about junction-vs-symlink, but bakes in that the suite will never run on Windows, the opposite of the stated intent);
  a `TestWindowsGap_Documented` marker test (greppable, but asserts nothing about the code).

### extraction-scope

- **Decision:** Ship the factory and move the one movable duplicate `gitStatusPorcelain`.
  Leave every existing `CloneHub(` call site alone — **100 across the 7 external test files, plus 2 in the in-package `clone_test.go`**.
- **Rationale:** the factory is the actual deliverable.
  Converting 100 calls across 7 files is a large, regression-risky mechanical diff against tests that are currently green, and most of those calls are clone-*behaviour* tests that deliberately vary clone's own arguments — a factory serves them badly.
  `warpbinding_clone_integration_test.go` alone holds 59 and `clone_adopt_test.go` 29, which is 88 of the 100 between them.
  The remaining 2 are in `clone_test.go`, which is in-package and out of scope for a different reason — it can never import `fabrictest` at all.
  Decoupling that conversion from this task decouples an unrelated risk.
- **Rejected:** converting all 7 external files now;
  classifying and converting only the pure-setup calls (the honest middle, but the classification requires reading all 7 files, which is most of the full conversion's cost anyway).

### package-file-layout-and-build-tags

- **Decision:** `fabrictest` is a **regular package with non-test `.go` files** (like `lyxtest`) that also holds its own `*_test.go` suites (like `boardtest`).
  `doc.go` and `testmain_test.go` carry **no** build tag, matching `boardtest`;
  every other file carries `//go:build integration`.
- **Rationale, and this is load-bearing in two directions:**
  - The factory **must** live in a non-test `.go` file.
    `*_test.go` files are not importable across packages, and the successor task `lyxtest-real-hubs` needs `fabrictest` to be the landing zone for `fabricengine`'s 14 in-package `lyxtest` callers.
  - Tagging the factory `//go:build integration` means an **untagged** test file physically cannot import it, so the harness opens no new blind spot in the Test Tier Purity Invariant (whose guard is substring-based on the test file itself and would not otherwise see git spawns smuggled in behind a helper call).
    Every prospective consumer is already integration-tagged: all 7 external `CloneHub` files carry `//go:build integration`; only the in-package `clone_test.go` is untagged, and it is explicitly out of scope.
  - `doc.go` stays untagged so the package always has at least one file in a default build — otherwise `go build ./...` fails with "build constraints exclude all Go files".
    This is exactly `boardtest`'s shape.
- **Suggested files** (mill-plan may adjust): `doc.go` (untagged: package doc, the measured wall-clock number, the Windows-gap section);
  `hub.go` (template-once bares, `CloneAndWire`-backed factory);
  `manifest.go` (capture, diff, prefix-rooted permits, the `.git` allowlist, Windows-safe path compare);
  `refusal.go` (the three `Check` constants, `RefusedByGate` and `RefusedBefore`);
  `states.go` (the 9 states);
  `verbs.go` (the 9 verb cases and their hostile-input sets);
  `matrix_test.go` (the cross-product driver);
  `testmain_test.go` (untagged, mirrors `boardtest`'s).

## Technical context

**The gate this harness validates** — `internal/fabricengine/destroy.go`, landed by slice 12 as `3184cd5a`.

- One error type: `destructiveRefusal{Check, What, Target, Reason}`, unexported.
  `Error()` renders `refusing to <what>: <check> check failed for <target>: <reason>`.
- Four checks in fixed order, stopping at the first failure: **containment**, **ownership**, **dirtiness**, **force**.
  `--force` answers dirtiness **only** — never containment, never ownership.
- Executors: `removePath`, `removeGitWorktree`, `removeLink`, `repointLink`, `deleteBranch`, `resetHardTo`, plus the two token minters `createExclusiveDir` and `createGitWorktree`.
- Eight path-ownership kinds: registered-linked-worktree, warp-checkout, fabric-hub, under-geometry-root, freshly-created-path, freshly-created-worktree, wired-junction, drifted-wired-junction.
  One branch-ownership kind: managed-branch.
- Two dirtiness scopes plus N/A: `dirtyScopeTracked`, `dirtyScopeAll`, `dirtinessNA(reason)`.

**Hub geometry.**
`<parent>/<name>-HUB/` contains the prime warp worktree, a `<name>-weft` sibling, `_board` (a second weft worktree holding repo-wide config), and — once a pair is added — `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>`.
Junctions carry **absolute** targets, which is why a hub can never be filesystem-copied and must always be cloned.

**Clone split.**
`fabricengine.CloneHub(cwd, CloneOptions{WeftURL, WarpURL, Subpath, Reset, ForceBootstrap}) (CloneResult, error)` returns `{HubPath, Anchor, BoardDir, WeftBase, PrimeCwd, WarpURL, WarpBindingRecorded}`.
It is deliberately git/geometry-only — the CLI layer drives config materialisation, the `weft:main` commit and junction wiring, because those route through `internal/configsync`, which `fabricengine` must never import (the `fabricengine → configsync → configreg → fabricengine` cycle).
`internal/fabriccli/clone.go`'s `runCloneWithReset` is that driver, and its post-clone sequence is: `configsync.ReconcileFabricAt(BoardDir, true)` → `NewBolt(BoardDir).Commit(...)` → `.Push(...)` → `lyxcwd.Resolve(PrimeCwd)` → `fabricengine.WiredNames(BoardDir)` → `fabricengine.WireJunctions(l, base, names)` → `configsync.ReconcileAll(WeftBase, true)`.
That sequence is what `CloneAndWire` extracts verbatim.

**Import legality, verified.**
`configsync` imports `configreg`, which imports `fabricengine`.
So `fabrictest → fabriccli → configsync → configreg → fabricengine` is a legal chain.
`fabricengine_test → fabrictest → fabricengine` is legal because Go compiles external test packages separately.
`fabrictest` may also import `lyxtest`, since `lyxtest` imports neither.
Nothing imports `fabrictest`, exactly as nothing imports `boardtest`.
The cycle bites **only** in-package (`package fabricengine`) test files.

**Verb signatures needed by the verb table.**

- `fabricengine.NewTopology(cfg Config) *Topology`
- `(*Topology).Add(l *lyxcwd.Location, slug string, opts AddOptions) (AddResult, error)`
- `(*Topology).Remove(l *lyxcwd.Location, slug string, force bool) (RemoveResult, error)`
- `(*Topology).Prune(l *lyxcwd.Location, apply, force bool) (PruneResult, error)`
- `(*Topology).Cleanup(l *lyxcwd.Location, apply, force bool) (CleanupResult, error)`
- `(*Topology).Checkout(l *lyxcwd.Location, branch string) (CheckoutResult, error)`
- `(*Topology).Reconcile(l *lyxcwd.Location) (ReconcileResult, error)`
- `fabricengine.UnwireJunctions(l *lyxcwd.Location, slug string, names []string) (UnwireResult, error)`
- `fabricengine.Open(l *lyxcwd.Location) (*Fabric, error)` then `(*Fabric).Pull(opts SyncOptions) (PullResult, error)`
- `fabricengine.CloneHub(cwd, CloneOptions{Reset: true})`

**Path helpers.**
`fabricengine.HubPath(parent, name)`, `BoardDir(hub)`, `WorktreePath(l, slug)`, `WiredNames(baseDir)`, `DeriveWarpName(rawURL)`, `BoardDirName` (`"_board"`), `HubSuffix`, `WeftBranchName(warpBranch)`, `IsReservedHubName(name, junctionNames)`.
`_portals` and `_launchers` are unexported directory names but their paths are `<hub>/_portals/<anchor>/<slug>` and `<hub>/_launchers/<anchor>/<slug>`.

**Hostile-input reference** — what `validateWorktreeSlug` (`internal/fabricengine/slug.go:30`) actually rejects, so cells assert against the real rule rather than a guess:
empty or whitespace-only;
containing `/` or `\`;
anything where `slug != filepath.Clean(slug)`, plus `.` and `..` explicitly;
anything ending in the weft suffix;
any reserved hub name (`_board`, `_portals`, `_launchers`, the structural committed/never-committed dirs, and any configured junction name).
A **leading `-`** is *not* rejected by `validateWorktreeSlug` — it is a git-argument-injection shape, so a cell driving it asserts on what the verb does with it, not on a slug refusal.

**Existing fixtures, and why they are not enough.**
`internal/fabricengine`'s `newFabricFixture` (`reconcile_stale_registration_test.go:103`) builds on `lyxtest.CopyPairedLocal` — a **hand-assembled** hub, never produced by `CloneHub`.
`lyxtest.WarpFixture.Hub` is the **warp repo**, not a fabric hub — a naming trap that produces the wrong fixture if trusted.
`lyxtest` builds real hermetic git repos (`buildWarpHub`, `buildWeftPrime`, `CopyPaired`/`CopyPairedLocal`/`CopyWeft`, `MustRun`, `SeedConfig`, `HermeticGitEnv`) and that machinery **should** be reused where it fits — but its bares are empty and its hubs have no `_board`, no `_portals`/`_launchers`, no hub-level `.lyx`, no junctions, no `.lyx-anchor`, no warp binding and no repo-wide `fabric.yaml`.
The factory **cannot** live in `lyxtest`: the lyxtest Leaf Invariant bars it from importing `fabricengine`, machine-enforced by `internal/lyxtest/leaf_enforcement_test.go`.

**Measured cost** (2026-08-10, Linux/WSL2 ext4, 155U, 14 cores, hermetic git env, two independent methods agreeing).
`CloneHub` in-process 60-66 ms serial / 15-16 ms concurrent; via CLI ~101-110 ms serial / 19 ms concurrent.
Full fixture — own bares plus hub clone — **24 ms concurrent**, of which the bare copy is ~2 ms and the clone ~22 ms.
Clone scales 5.2x on 14 cores, being `fork`/`fsync`-bound.
For comparison, `lyxtest.CopyPaired` is 13.3 ms serial / 2.3 ms concurrent.

## Constraints

From `CONSTRAINTS.md`:

- **Test Tier Purity Invariant** — untagged test files must not call `gitexec.RunGit`, `exec.Command*`, or `lyxtest.Copy*`.
  Everything this task adds except `doc.go` and `testmain_test.go` carries `//go:build integration`, and tagging the factory is what stops an untagged consumer smuggling git spawns past the substring-based guard.
  Enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** — every git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`.
  `fabrictest` gets one, mirroring `boardtest/testmain_test.go`.
  Enforced by `cmd/lyx/hermeticenv_test.go`.
- **lyxtest Leaf Invariant** — `lyxtest` must not import `fabricengine`.
  Machine-enforced by `internal/lyxtest/leaf_enforcement_test.go`.
  This is why the factory lives in `fabrictest`.
- **Fabric Destruction Chokepoint Invariant — this one also blocks the build, and the guard's reach is wider than its invariant text implies.**
  `destroy.go` is the only file in `package fabricengine` permitted to perform a destructive primitive.
  The banned bypass tokens are all eight of `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`, and `createdToken{`.
  The two `ResetHard` tokens matter here specifically: `Fabric.Pull`'s `ResetHard` is what tranche 1's `Pull` cell exercises, and it is the primitive R2's defect discarded uncommitted tracked warp work through on every advance path.
  **The guard is directory-scoped, not package-scoped.**
  `cmd/lyx/destructiveguard_test.go` sets `destructiveGuardScanPackages = []string{"internal/fabricengine"}` and runs `filepath.WalkDir` over that whole subtree, skipping only files whose name ends in `_test.go`.
  So `internal/fabricengine/fabrictest/{hub,states,manifest,verbs,refusal}.go` **are scanned**, even though they are a different package — and the harness genuinely needs `fslink.Remove(` and `os.Remove(` to plant and tear down hostile shapes such as `staleWiredJunction`.
  **Required in the same commit:** exclude the `fabrictest` subdirectory from the guard's walk with a stated reason, and update `CONSTRAINTS.md`'s invariant text to say the scan is package-scoped in intent and names the `fabrictest` subpackage exclusion explicitly.
  A directory-level exclusion is preferred over per-file allowlist rows because it restores the guard to exactly the scope its own invariant text already claims ("the only file in *package fabricengine*") rather than punching a growing set of per-file holes in it;
  the alternative — confining every destructive token in `fabrictest` to `_test.go` files — was rejected because it would strand the state builders inside the test binary, unreachable by the `fabricengine_test` consumers the package exists to serve.
  Once the exclusion lands, `fabrictest` is outside the guard, and its state builders plant and tear down hostile filesystem shapes through `fslink` and ordinary `os` calls without pretending to be gated — but that is a consequence of the exclusion, not a property it had beforehand.
  Enforced by `cmd/lyx/destructiveguard_test.go` (`TestNoDestructiveBypass_FabricengineProductionSource`).
- **Cwd Resolution Invariant** — `internal/lyxcwd` alone owns cwd resolution.
  The factory resolves a `*lyxcwd.Location` via `lyxcwd.Resolve(PrimeCwd)`, never by constructing one.
- **Fabric Git Invariant (warp + weft)** — the harness must use fabric's own vocabulary (warp, weft, prime, pair, hub, anchor) in names and messages.
- **Fabric Vocabulary Invariant — this one blocks the build and must be handled in the same commit.**
  `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) walks every non-`_test.go` `.go` file under `internal/` and `cmd/`, computes `dir := filepath.ToSlash(filepath.Dir(relPath))`, and looks that string up in two **exact-match** maps — there is no prefix or subtree matching.
  `fabricVocabularyOwners` (`enforcement_test.go:597`) holds `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine` and `internal/configsync`;
  `weftnameImportOwners` (`enforcement_test.go:609`) holds only the first, second and fourth.
  **`internal/fabricengine/fabrictest` is in neither**, and it is a directory of *non-test* `.go` files by the package-layout decision above — so every bare `warp`/`weft` identifier or comment in `hub.go`, `states.go`, `verbs.go`, `manifest.go` and `refusal.go` fails the bare-vocabulary rule, and importing `internal/weftname` (which the `-weft`-suffix hostile input needs) fails the import rule on top.
  **Required in the same commit:** add `internal/fabricengine/fabrictest` as an owner row to **both** maps, and update `CONSTRAINTS.md`'s Fabric Vocabulary Invariant owner set to match.
  This is not optional cleanup — without it the first `go test ./...` fails, and the failure surfaces in `internal/lyxcwd`, far from the code that caused it.
- **CLI / Cobra Invariant** — `CloneAndWire`'s extraction must leave the `Command()`/`RunCLI` seam and every `Short` intact, and the help-tree tests must stay green.
- **Documentation Lifecycle** — see the task-completion rule below.

From `CLAUDE.md`:

- **Documentation lands in the same commit.**
  This task introduces cross-cutting test infrastructure and changes `internal/fabriccli`, so it must update `docs/overview.md`'s Tests section (naming `fabrictest` alongside `boardtest`) and the relevant module doc in `manifest/designs/`.
  `manifest/designs/fabric-crucible-followups.md` is the durable source of truth for slices 12-15 and should record slice 13 as landed.
  `manifest/roadmap.md` moves only on completing a planned item — this is one, so it moves.
- **All cross-OS links go through `internal/fslink`.**
  `CreateDirLink` is the entry point; Windows file symlinks must not be relied on.
- **Markdown: semantic line breaks**, one sentence per line, no fixed-column hard wrap.
- **Worktree isolation** — this agent works only in `wts/fabric-live-state-harness`, never pushes to `main` directly.

## Testing

The deliverable **is** tests, so "test approach" here means how the harness proves itself, plus what covers the production change.

**`internal/fabriccli` — `CloneAndWire` extraction.**
A pure refactor with no behaviour change.
Existing coverage in `internal/fabriccli/cli_test.go` and `pushbypass_integration_test.go` already drives `fabric clone` end to end and must stay green unmodified — that is the regression proof.
No new test is needed for the extraction itself beyond the matrix now exercising it on every cell.
TDD is not the right shape here; extract, then run the existing suite.

**`internal/fabricengine/fabrictest` — the harness.**

TDD candidates, in build order, each independently verifiable before the matrix exists:

1. **The bare-pair template builder.**
   Assert the warp bare's `HEAD` resolves to `refs/heads/main` (the `symbolic-ref` gotcha), that the warp bare has a commit reachable on `main` with both a root `README` and a `backend/` entry, and that the weft bare is genuinely empty.
   This is the highest-value TDD target: both gotchas are exactly the kind rediscovered painfully later.
2. **The hub factory.**
   For each anchor (`.` and `backend`), assert the built hub has: the prime warp worktree, the `<name>-weft` sibling, `_board`, a resolved `.lyx-anchor` matching the requested anchor, a recorded warp binding, wired junctions on the prime warp, and a repo-wide `fabric.yaml` at `BoardDir`.
   Then assert that adding a pair produces `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>`.
   This is what proves "real hub, not hand-assembled".
3. **The manifest capture and diff.**
   Round-trip properties: an unchanged hub diffs empty;
   deleting a file outside every permitted root is reported;
   deleting a file *under* a permitted root is not;
   a link whose raw one-hop target changes is reported;
   a path replaced by a different kind (file where a link was) is reported.
   Two `.git`-rule assertions, one per direction: running an ordinary `git status` and a commit against the hub produces an **empty** diff (the churn the allowlist excludes — `index`, `logs/**`, `refs/**`, `objects/**`), while removing a linked worktree's `.git/worktrees/<name>` registration **is** reported (the R3-shaped case the allowlist deliberately keeps observable).
   Path keys are `ToSlash`-normalised and comparison is case-folding on Windows.
4. **`RefusedByGate` and `RefusedBefore`.**
   `RefusedByGate` against real gate refusals produced by driving a verb, one per reachable `Check`, plus a negative: a non-refusal error must not match any check.
   `RefusedBefore` against a real pre-flight refusal — `Remove`'s own dirty message is the sharpest case, since it is byte-identical to the gate's reason minus the `dirtiness check failed` prefix, so a helper that matched on the reason text alone would wrongly report a gate refusal there.
5. **The state matrix.**
   Each state gets a direct assertion that it actually established what it claims *before* any verb runs — a `dirtyWarpTracked` state that silently failed to dirty anything would make every cell using it vacuous.
   This is the single most important guard in the whole harness and must not be skipped.
6. **The verb table.**
   Each verb case is driven once in the `clean` state to prove its `Run` and its expected-effect assertion are both wired correctly, before the cross product multiplies any mistake by nine.
7. **The cross product itself.**
   `t.Run(state/verb)` over `states × verbs × anchors`, each `t.Parallel()` on its own hub.

**Scenarios that must be covered** — each traceable to the evidence table, so the matrix can be audited against it:

- `pull` with a dirty tracked file in the warp worktree: refuses, and the uncommitted line survives (R2).
- `remove ..`: refuses **at slug validation, not at the gate's containment check**, and the hub still exists (R5 — the worst defect in the campaign).
  `Remove` calls `validateWorktreeSlug` at `remove.go:45` and returns `invalid slug "..": a slug must name a directory…` before any `pathRequest` is built, so the gate's containment refusal at `destroy.go:528` is **unreachable for this input**.
  The cell uses `RefusedBefore`, not `RefusedByGate(containment)` — the latter would fail against a correct binary.
  The survival assertion (the hub is still on disk) is what carries the R5 evidence, and it is unchanged either way.
- `remove` with a slug ending in the weft suffix, a reserved hub name, `""`, `.`, `../x`, and a leading `-`.
- `prune` against a hub child whose name ends in the weft suffix but which fabric never created — an ordinary user directory, and separately a real unrelated `git init` clone parked there (R4, two distinct states).
- `prune` against a path git has just refused to remove, with untracked files present (R3).
- `cleanup` must never delete the primary weft branch, with and without `--force` (R3).
- `reconcile` against a user's own tracked symlink sitting at a wired junction path: refuses, and the symlink survives with its original target (R1).
- `clone --reset` against a directory named `<derived>-HUB` that is not a hub (R4).
- Every `clean`-state cell: the verb succeeds and its intended effect landed.

**Sabotage-proving — mechanism and artifact, both named.**

- **Scope:** the **nine evidence-table scenarios** listed above, and only those.
  Not all 190 cells — proving a cell fails on demand costs a manual edit-run-revert cycle each, which is affordable nine times and not affordable 190 times.
- **Mechanism:** during implementation, for each of the nine, temporarily neuter the specific check that scenario depends on — the pre-flight branch, or the gate check, whichever the cell's expectation kind names — run that one cell, and confirm it **fails**.
  The edit is a **local working-tree change that is reverted immediately and never committed**;
  it does not appear in any commit, and `destroy.go` is untouched in the delivered diff.
  This is why sabotage-proving is not a permanent automated gate: proving it continuously would need a per-check injection seam that slice 12 deliberately did not build, and adding one now would be production surface added by a test-only slice.
- **Durable artifact:** a table in `fabrictest`'s `doc.go` with one row per proved scenario, recording the cell, which check was neutered, and the observed failure.
  That table is the evidence the exercise happened, and it is reviewable long after the temporary edits are gone.
- **Completion gate:** all nine rows present and populated before the task is considered done.
  A row that could not be proved is recorded as such with the reason, never silently omitted.
- **Why this is not optional:** the campaign's own standard is that a test which has never been made to fail on demand is not yet proof, and this harness exists precisely because a green suite was not evidence.
  A harness whose own cells were never made to fail would repeat the mistake it was built to correct.

**What is deliberately not tested here.**
"No call site bypasses the gate" — that is slice 12's static guard (`cmd/lyx/destructiveguard_test.go`), a different mechanism.
Duplicating it here would only risk diverging from it.

## Q&A log

- **Q:** What fidelity is a "real hub" from the factory — `CloneHub` only, replicated wiring, extracted shared function, or the real binary? **A:** Extract `fabriccli.CloneAndWire` and call it from both the cobra handler and the factory — mirrors no duplication, and a clone-only hub leaves three of the gate's eight ownership kinds structurally unreachable.
- **Q:** How does a cell assert what must survive? **A:** Whole-hub manifest diff plus planted sentinels — all eight defects destroyed something nobody was asserting on, so a permit-list diff is what catches instance nine.
- **Q:** How is the cross product expressed? **A:** `[]State × []VerbCase` with one `t.Run` per cell, so a new verb inherits every state.
- **Q:** Which verbs are in tranche 1? **A:** All nine gate-reaching verbs, with hostile inputs applied only to the three that accept a slug or branch.
- **Q:** Which states are in tranche 1? **A:** The eight defects' states plus the two link/foreign-path shapes, nine total, each citing its originating defect.
- **Q:** Where does the matrix suite itself live, given `export_test.go` shims are unreachable from `fabrictest`? **A:** In `fabrictest`, mirroring `boardtest` — a cell needing an unexported shim is testing an internal, not the verb's public contract; those cases stay in `destructivegaps_integration_test.go` as an honest cost.
- **Q:** How does a cell assert *which* check refused? **A:** Substring-match the refusal message via a `RefusedByGate` helper — zero production churn, and the message is itself slice 12's stated honesty contract, which slice 14 will rewrite anyway. Round 1 later split this into `RefusedByGate` plus `RefusedBefore`, and round 2 established that the exported `Check` set has three members, since `checkForce` is never emitted.
- **Q:** How much of the call-site extraction lands here? **A:** Factory only, plus the one movable `gitStatusPorcelain`; converting the 100 calls in the 7 external files is a regression-risky diff against green tests and belongs elsewhere, and the 2 in the in-package `clone_test.go` can never import `fabrictest` at all.
- **Q:** Does tranche 1 run cells on a `--subpath` hub? **A:** Yes, both anchors for the full matrix — `_portals`/`_launchers` paths are `AnchorRel`-interpolated, which is where a containment bug hides.
- **Q:** Do clean-state cells assert the verb succeeds? **A:** Yes — without it an over-refusing gate is indistinguishable from a correct one, which defeats the matrix's purpose.
- **Q:** Bare-remote template strategy and warp content shape? **A:** One `sync.Once` template pair carrying both a root `README` and `backend/`, copied per scenario; both gotchas encoded once.
- **Q:** "Dirty warp" and "dirty weft" — dirty which checkout? **A:** The one the verb under test actually acts on, resolved per cell; R2 and R3 were both about the verb's own target being dirty.
- **Q:** How precise is the manifest permit-list? **A:** Prefix-rooted permits — concise, stable against layout change, and still catches anything destroyed outside the permitted roots, which is the shape all eight defects took.
- **Q:** Runtime budget and parallelism? **A:** Every cell parallel on its own hub, no hand-rolled cap, measured wall-clock recorded in `doc.go` rather than enforced by a flaky timing assertion.
- **Q:** How is the Windows gap expressed? **A:** No `GOOS` skips and a `doc.go` note — the harness would run on Windows, only nobody has run it.
- **Q:** Follow-up — the operator intends to run the whole suite on Win11 later and fix what breaks then. **A:** Decision unchanged, with an added build obligation: write the harness Windows-portable by construction (all links via `fslink`, GOOS-branched launcher extensions, `ToSlash` manifest keys, case-folding path compare, no file-mode dependence, short paths), and record the one genuine divergence — a git-tracked symlink materialises as a junction on Windows — in `doc.go` rather than behind a skip.
- **Q:** Round 1 gap — most refusals never reach the gate (`remove.go:45`, `remove.go`'s own dirty pre-flight, `checkout.go:42-47` all return plain errors), so a single gate-shaped assertion mis-specifies. What is the second expectation kind? **A:** [auto-pick] Two expectation kinds, `RefusedByGate(check)` and `RefusedBefore(substring)`, declared per cell with the verb table recording which. **Why:** which layer refused is signal — R5's defect was a missing pre-flight and R3's a gate-side fallback, so a cell that pins the layer fails when a refactor moves a check across that boundary.
- **Q:** Round 1 gap — `resetHub` declares `dirtinessNA`, so no dirtiness state can refuse `CloneHub{Reset:true}` and the permitted root is the whole hub, making the manifest diff vacuous. What does that column assert? **A:** [auto-pick] Re-scope the column to the ownership axis: drive `Reset` against a non-hub directory and a `<derived>-HUB`-named non-hub, assert refusal-by-ownership plus full survival, omit the dirtiness rows with the reason stated, and assert the rebuild through `CloneAndWire`. **Why:** that is exactly R4's `clone --reset` defect, and it converts ~18 vacuous cells into two that assert something.
- **Q:** Round 1 gap — a leading `-` passes `validateWorktreeSlug` and `Checkout` validates no branch at all, leaving both cells' expected outcome undefined. **A:** [auto-pick] Assert survival and no-partial-mutation with no check-name assertion, writing the cell against the *safe* expectation (the argument must not reach git as a flag); use branch-shaped hostile inputs for `Checkout` plus a no-half-switched-pair assertion. **Why:** if the current code is unsafe the cell fails, and that failure is instance nine, which is the tranche's purpose — asserting current behaviour instead would assert nothing.
- **Q:** Round 2 gap — the destructive bypass guard is directory-scoped (`filepath.WalkDir` over `internal/fabricengine`, skipping only `_test.go`), so it *does* reach `fabrictest`'s non-test files, contradicting this document's claim that it does not. **A:** [auto-pick] Exclude the `fabrictest` subdirectory from the guard's walk with a stated reason, and update `CONSTRAINTS.md`'s invariant text to name the exclusion. **Why:** a directory exclusion restores the guard to the scope its own invariant text already claims ("the only file in *package fabricengine*"), where per-file allowlist rows would punch a growing set of holes, and confining destructive tokens to `_test.go` would strand the state builders inside the test binary where `fabricengine_test` consumers cannot reach them.
- **Q:** Round 2 gap — `Remove` deletes the portal and launchers at `remove.go:61-66`, *before* its dirty pre-flight at `:68-76`, so a correctly-refusing dirty-`Remove` cell has already destroyed two paths. Do refusal cells carry permit roots? **A:** [auto-pick] Yes — every expectation kind carries a permit-root field, those two paths are permitted on that cell, and the anomaly is recorded in `doc.go` and flagged for slice 14 rather than silently buried in a permit list. **Why:** "returned an error and destroyed something anyway" is the shape of all eight campaign defects, so a cell permitting it must say so out loud; but it is deliberate documented behaviour, and a harness task is the wrong place to reverse a production decision unilaterally.
- **Q:** Round 2 gap — dirtiness scope is per-verb, so `dirtyWarpUntracked` against a `scopeTracked` verb like `Checkout` must *succeed*, an outcome the two-kind refusal scheme could not express. **A:** [auto-pick] Add a third expectation kind, `Proceeds` (verb succeeds, effect lands, planted content survives), and record the verified per-verb dirtiness-scope table so every cell's outcome is derived rather than guessed. **Why:** the scopeTracked-vs-scopeAll divergence against an untracked-only state is the one case proving dirtiness scope is a real per-request parameter, and without the table those cells would have been written from observed behaviour and asserted nothing.
- **Q:** Round 3 gap — what does a manifest entry actually record, and how is `.git` handled, given that a blanket exclusion and a per-cell permit have opposite consequences? **A:** [auto-pick] Entry is path → kind + link raw target + file content hash, with no mode/mtime/size; `.git` gets a narrow allowlist recording only the `.git` entry itself and `.git/worktrees/<name>` at existence granularity. **Why:** a blanket `.git` exclusion would blind the harness to linked-worktree deregistration, which is R3's shape exactly, while hashing everything under `.git` would drown every cell in `index`/`logs`/`refs`/`objects` churn until the permit lists permitted everything.
- **Q:** Round 3 gap — hostile inputs are scoped to three verbs but to no state set, so under `[]State × []VerbCase` they would inherit all nine states and both anchors (~378 cells), nearly all vacuous. **A:** [auto-pick] Hostile-input cells run in `clean` only, expressed as an optional `States` restriction on the `VerbCase` that is empty by default — so an ordinary newly-appended verb still inherits every state — giving 190 cells total. **Why:** `remove ..` refuses at `remove.go:45` before any hub state is consulted, so eight of its nine state rows would assert exactly what the ninth does, which is the same vacuity the `Reset` column was re-scoped to avoid.
- **Q:** Round 3 gap — sabotage-proving named no mechanism and no artifact, and neutering `destroy.go` is a production edit not in scope. **A:** [auto-pick] Name both: a temporary local working-tree edit per scenario, reverted immediately and never committed, scoped to the nine evidence-table scenarios only, with a `doc.go` table recording cell, neutered check and observed failure as the durable artifact, and all nine rows as a completion gate. **Why:** a permanent automated gate would need a per-check injection seam slice 12 deliberately did not build, and adding one would be production surface introduced by a test-only slice — but a harness whose cells were never made to fail would repeat the mistake it exists to correct.
