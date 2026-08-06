# Discussion: fabric: close the weft-visibility leak (slice 8)

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
slug: fabric-weft-visibility-cleanup
status: discussing
parent: main
```

## Problem

Fabric exists to sell one illusion: a developer, and every lyx module, sees **one repository, called fabric**.
Under the hood that repo is two git checkouts — the warp (the host/code repo a human uses normally) and the weft (the sibling `-weft` worktree holding durable `_lyx` state) — but nothing outside `internal/fabricengine` and `internal/fabriccli` is supposed to know that.

Today the illusion leaks in every direction.
Fifteen production call sites outside fabric name the weft explicitly, and the leak is not confined to path lookups: it is in the constructor (`fabricengine.New(warpPath, weftPath)` forces a caller to *supply* a weft path), in the return type (`CommitResult.WeftCommitted` narrates the two-repo split back to the caller), in consumer-owned interface names (`builderengine.WarpResetter`, `websterengine.WarpBisector`), in exported constants whose values reach user output (`loomengine.CheckWeftPairing`, `websterengine.ClassWeftReference`), in a JSON envelope key (`"weftCommitted"`), and in roughly 380 comment mentions across 55 production files.

**Why now:** slice 7 of [manifest/designs/fabric-unified-view.md](../manifest/designs/fabric-unified-view.md) has landed.
It moved the weft-sibling and junction path surface out of `internal/hubgeometry`/`internal/lyxcwd` and into `internal/fabricengine`, which is what made the remaining leak both visible and fixable — the callers no longer reach into a shared geometry module for a weft path, they reach into fabric's own exported API for it.
Slice 8 is the follow-through: make fabric's public surface incapable of telling a caller that warp and weft exist.
Slice 8 is file-disjoint from slices 9 and 10 and can run in parallel with either.

Note that the task brief describes the leak as "call sites that read Layout `Weft*`/`Host*Link` methods directly".
That wording predates slice 7 and is now inaccurate: no call site reads `lyxcwd.Location` for a weft path any more.
The leak that remains is `fabricengine`'s own exported API.

## Scope

**In:**

- Replace `fabricengine.New(warpPath, weftPath string)` with `fabricengine.Open(l *lyxcwd.Location)` as the sole constructor any package outside `fabricengine` calls.
- Add `fabricengine.Ready(l)` so `loomengine` stops stat-ing a weft path to decide whether fabric is usable.
- Add `fabricengine.NewRefScanner(l) *RefScanner` with `Matches(cmd string) bool`, absorbing both halves of `websterengine`'s weft-reference regex so `audit.go` drops its `internal/weftname` import.
- Add `CommitResult.Committed()` so callers stop reading `WeftCommitted`.
- Unexport `Fabric.Warp`/`Fabric.Weft`, `PartialCommitError`'s fields, and `fabricengine.New` (as `newPaired`).
- Rename every production identifier and string literal outside the owner set that says `weft` or `warp`, to fabric vocabulary.
- Reword every production comment outside the owner set that says `weft` or `warp`.
- New enforcement test `TestEnforcement_FabricVocabulary` making the rule machine-checked.
- Docs: `internal/fabricengine` package doc, `CONSTRAINTS.md`, `docs/overview.md`, and slice 8's section in `manifest/designs/fabric-unified-view.md`.

**Out:**

- `internal/lyxtest` is **not** a leak to fix.
  It imports `internal/weftname` only — never `fabricengine` — which slice 7's batch 1 established deliberately and which the lyxtest Leaf Invariant *requires*.
  It builds real paired fixtures and must keep the vocabulary.
- `*_test.go` files are outside the machine check, matching `TestEnforcement_GeometryLiterals`'s existing stance that test geometry is a review rule.
  Tests that break because a symbol was unexported are updated to compile (see Testing), but no test is rewritten purely for vocabulary.
- `loomengine/preflight.go:117-130`'s substring-matching of `Healthy`'s reason strings.
  The file's own comment flags it as fragile;
  it is a separate defect and touching it here would entangle a behavioural change with a vocabulary change.
- `fabricengine.List` is **not** renamed or unexported.
  A fabric worktree's path *is* its warp worktree's path, so `List` already returns fabric worktrees;
  `WorktreeEntry`'s fields (`Path`, `Head`, `Branch`, `Main`) name neither side.
  Its doc comment is corrected, nothing more.
  `ideengine/menu.go:41` is unaffected.
- Slice 9 (`.lyx` hygiene) and slice 10 (warp-URL binding in `weft:main`).
- The still-open orchestration half of slice 6.

## Decisions

### one-constructor-open

- Decision: `fabricengine.New(warpPath, weftPath string) (*Fabric, error)` becomes private `newPaired`.
  A new `func Open(l *lyxcwd.Location) (*Fabric, error)` — implemented as `newPaired(l.WorktreePath(), weftWorktree(l))` — is the only constructor any other package calls.
  `WeftWorktree` becomes private `weftWorktree`, *except* that it stays exported for `internal/fabriccli`'s two uses (see `fabriccli-is-an-owner`).
- Rationale: `New` does almost nothing — two `os.Stat` calls via `requireDir`, then two `gitrepo.New(path)` calls, and `gitrepo.New` is `return &Repo{path: path}`.
  There is no connection, no lock, no cached state, no setup.
  The only thing the ceremony accomplished was forcing every caller to name a weft path.
  `Open(l)` also matches the house style already used by `Clean(l)`, `Healthy(l)`, `Status(l)`, `Reconcile(l)`, `Checkout(l)`.
- Rejected: a package-level `CommitScoped(l, dirs, msg, opts)` that removes the handle entirely from the three CLI sites — unnecessary, because a handle named `fabric` with a `Commit` method *is* the one-repo illusion;
  and it would not have served the two engine sites, which need a handle satisfying an interface, not a commit.
- Rejected: a separate `OpenWarp(l)` for the two engine sites that only drive warp verbs — the name says "warp", which is exactly what the caller must not see.
  The cost of this rejection is accepted and recorded under `open-stats-both-sides`.

### open-stats-both-sides

- Decision: `Open(l)` stat-validates both checkouts, so `builderengine`'s restart-chain reset and `websterengine`'s bisect (which drive warp-only verbs) still require the weft sibling to exist on disk.
- Rationale: from outside, fabric is one repo;
  "the repo is present" legitimately means both of its checkouts are present.
  Preserving today's behaviour also keeps the change purely about visibility, with no behavioural delta to reason about in review.
- Rejected: a warp-only constructor (see above) — it would remove a spurious dependency, but only by naming warp in the API.

### commit-result-committed-method

- Decision: add `func (r CommitResult) Committed() bool { return r.WarpCommitted || r.WeftCommitted }`.
  The three CLI sites become `committed = res.Committed()`.
  The four raw fields stay exported for `internal/fabriccli`, and `TestEnforcement_FabricVocabulary` bans reading them outside the owner set.
- Rationale: there is only one `Commit` verb.
  `CommitResult` is its return type, and it carries four fields because one call can land two real git commits — `Commit` classifies the caller's file list against the repo-wide pathspec and routes each path to the correct side itself (`commit.go:96`).
  That classification is the illusion working as designed;
  the defect is only that the result narrates the split back.
  `fabriccli/weft_verbs.go:160` prints `res.WeftCommitted` and `res.WeftSHA` on purpose — `lyx fabric weft …` exists to show an operator the weft side — and `fabriccli` is a separate Go package, so the fields cannot simply be unexported.
- Rejected: splitting into `Commit(...) (bool, error)` plus a `CommitDetailed(...)` for fabric's own CLI — Go cannot enforce "fabric-only" on either, so it needs the identical enforcement test *and* costs a second method.
- Rejected: collapsing to `CommitResult{SHA, Committed}` — fabric's own weft verbs lose the weft SHA they print by design.

### partial-commit-error-fields-private

- Decision: `PartialCommitError`'s `WarpSHA`, `WeftSHA`, `WeftCommitted` fields become private.
  Its `Error()` string is unchanged.
- Rationale: verified zero readers outside `internal/fabricengine`.
  The type is still returned to external callers and still matched with `errors.As`;
  only the field access goes away, and nobody used it.

### ready-not-paired

- Decision: `loomengine/preflight.go:105`'s `os.Stat(fabricengine.WeftWorktree(l))` becomes `fabricengine.Ready(l) (bool, error)`.
  preflight keeps its own classification and its `check3BlocksSeed` flag unchanged.
- Rationale: "paired" announces that there are two repos, which is the exact leak.
  `Ready` answers loomengine's real question — "is fabric usable in this worktree" — without naming a side.
- Rejected: folding pairing into `Healthy(l)` as a new classified reason — preflight would then have to string-match a reason to tell "not present" (which sets `check3BlocksSeed`) from a sync failure, and that file already documents its reason-string matching as fragile.
- Rejected: `Wired(l)` — overlaps the junction-health concept `Healthy` already owns, and preflight distinguishes the two.

### refscanner-owns-both-halves

- Decision: `websterengine`'s `weftReferencePattern(layout) *regexp.Regexp` is replaced by `fabricengine.NewRefScanner(l) *RefScanner`, with a `Matches(cmd string) bool` method.
  `RefScanner` absorbs **both** halves of today's regex: the path half (the weft worktree path, `weftname.Suffix`) and the command-spelling half (`lyx(?:\.exe)?\s+(fabric|weft|warp)\b`).
  `audit.go` drops its `internal/weftname` import.
  `CheckFork` and `CheckParent` take `*fabricengine.RefScanner` where they took `weftRef *regexp.Regexp`;
  construction stays at `runlevel.go:706` and `recordbatch.go:125`, still once per audit.
- Rationale: `audit.go` is the one site that genuinely needs the weft path as a *string* and has no operation return value to route through, so the design doc's claim that "none of them need `WeftWorktree()` once Fabric's return values are complete" is false for this one.
  The scanner is how it stops needing the string.
  Moving only the path half was considered first and rejected once the vocabulary rule was settled: `websterengine` may not contain the words `weft`/`warp` at all, so the spelling alternation has to move too.
  Webster asks a question;
  fabric owns every word in the answer.
- Rejected: `fabricengine.WeftPathPattern(l) string` returning a quoted regex fragment — still hands out the path, and `Pattern` collides with `internal/patternengine`.
- Rejected: `ReferencesFabric(cmd string, l)` as a plain func — recompiles the regex per command;
  today it is built once and reused across every Bash command in a transcript.
- Consequence: fabric now owns webster's audit *policy* for command spellings.
  Accepted: the policy is about which fabric-driving commands are forbidden, and fabric is the module that knows what those are.

### fabric-vocabulary-rule

- Decision: in production Go, the tokens `weft` and `warp` may appear only in the **owner set**.
  Everywhere else, identifiers, string literals, and comments use fabric vocabulary.
  Owner set:
  - `internal/fabricengine` — implements the illusion.
  - `internal/fabriccli` — fabric's own CLI;
    `lyx fabric weft …` exposes the weft to an operator deliberately.
  - `internal/weftname` — the `-weft` suffix leaf.
  - `internal/lyxtest` — the test-fixture leaf that builds real paired worktrees (`WeftPrime`, `WeftBare`, `WeftPath`, `CopyWeft`).
  - `internal/boardengine` — the Fabric Git Invariant's existing board carve-out;
    board lives at `weft:main`, and its whole branch-naming design is defined in terms of the `-weft` suffix (`board.go:14-30`).
  - `internal/configsync` — **string literals only**, for `legacyFabricConfigModules = []string{"warp", "weft"}` (`configsync.go:24`).
- Rationale: this is the rule the task title names.
  The `configsync` exception is forced, not a preference: those strings are on-disk legacy config filenames (`warp.yaml`, `weft.yaml`) that the migration must read by name;
  renaming them would break the migration it exists to perform.
  The `boardengine` exception is likewise pre-existing policy, recorded in `CONSTRAINTS.md`'s Fabric Git Invariant.
- Rejected: banning identifiers and string literals but leaving comments alone.
  Considered and explicitly overruled — the task is a *visibility* cleanup, and a doc comment telling a maintainer "this weft-commits state.json" leaks the model just as effectively as a symbol does.

### diagnostics-say-fabric-detail-says-weft

- Decision: consumer-emitted prose says "fabric".
  `buildercli` emits `"builder: run finished (%s) but the fabric sync failed: %v"`;
  `webstercli`, `perchcli` likewise.
  The wrapped `%v` is `fabricengine`'s own error and continues to name the weft repo and path freely.
- Rationale: an operator debugging a sync failure still gets the full weft-level detail — it just arrives from the module that owns the word.
  This resolves what would otherwise be a contradiction between "errors may explain that something happened in weft" and "these modules must not reference weft".
- Rejected: leaving `"the weft sync failed"` in consumer packages as a diagnostic exemption — it puts a permanent hole in the rule at exactly the place operators read.

### json-key-renamed

- Decision: the CLI envelope key `"weftCommitted"` becomes `"fabricCommitted"` in `buildercli/run.go:98`, `webstercli/run.go:105`, and `perchcli/run.go:378`.
  `tools/sandbox/SANDBOX-PERCH-SUITE.md:119` and `buildercli/run_test.go:150` are updated in the same commit.
- Rationale: verified those are the only two consumers.
  A machine-readable contract is the worst place to leave the leak, since it is the surface orchestrator skills read.
- Rejected: freezing the key as a machine contract — the cost of changing it is two known files.

### consumer-renames

- Decision: rename, outside the owner set:
  - `builderengine.WarpResetter` → `FabricResetter`;
    `websterengine.WarpBisector` → `FabricBisector` (with doc comments).
  - `loomengine.CheckWeftPairing` → `CheckFabricReady`;
    `CheckWeftSync` → `CheckFabricSync`.
  - `websterengine.ClassWeftReference` → `ClassFabricReference`, value `"weft-reference"` → `"fabric-reference"`;
    its violation `Detail` prose reworded (`audit.go:134`, `:203`).
  - `internal/buildercli/weft.go` → `sync.go`;
    `internal/webstercli/weft.go` → `sync.go`;
    helper `weftCommit` → `fabricSync` in both.
  - locals: `weftErr` → `syncErr`, `weftWorktree` (deleted — `Open` removes the need).
- Rationale: "Fabric is the name of the repo they see", applied literally.
  `ClassWeftReference` was initially proposed for exemption on the grounds that it names a rule aimed at agents, who *are* told weft exists;
  overruled under `fabric-vocabulary-rule`, and the reworded violation text is no less actionable ("ran a command that reaches into fabric's internals").
- Rejected: `RepoResetter`/`RepoBisector` — provider-neutral, but `builderengine.Resetter` operating on "the repo" is vaguer than naming the thing.

### enforcement-test

- Decision: new `TestEnforcement_FabricVocabulary` in `internal/lyxcwd/enforcement_test.go`, beside `TestEnforcement_GeometryLiterals`, sharing its repo-walk helper.
  It walks production `.go` files (excluding `*_test.go`) and fails any file outside the owner set containing the token `weft` or `warp` — in identifiers, string literals, **or** comments — and any file outside `{fabricengine, fabriccli, lyxtest}` importing `internal/weftname`.
  Owners are expressed as a map in the same idiom as `geometryTokenOwners`, with the `configsync` row documented as string-literal-only.
- Rationale: one rule, no per-symbol allowlist to keep in sync as fabric's API evolves.
  Including comments is a deliberate divergence from the sibling `stripGoComments` guard in the same file: that guard exists to police *usage* of `os.Getwd`, where prose mentioning it is harmless, whereas here the prose is itself the leak.
  Without the comment half, the 55-file cleanup rots back within a few tasks.
- Rejected: a symbol-and-import allowlist (`fabricengine.WeftWorktree`, the four `CommitResult` fields, …) — needs updating every time fabric gains a symbol, and silently permits any new one.
- Rejected: extending `TestEnforcement_GeometryLiterals` itself — its predicate matches whole string literals in path-construction context and cannot express selector references or comment text.

### documentation

- Decision, all in the same commit as the code:
  - `internal/fabricengine`'s package doc absorbs the durable contract: `Open` is the only constructor outside the package, `Committed()` is the only result a consumer reads, `RefScanner` is how a consumer asks about fabric-driving commands, and the vocabulary rule with its owner set.
  - `CONSTRAINTS.md`: the Cwd Resolution Invariant's fabric bullet (currently "Weft-sibling paths and junction construction belong to `internal/fabricengine`") widens to state the vocabulary rule and names `TestEnforcement_FabricVocabulary` under **Enforced by**.
    The Fabric Git Invariant's heading and body keep `warp`/`weft` — that invariant is *about* the two-repo mechanism.
  - `docs/overview.md`: the Cwd Resolution Invariant section (`:63-79`) gains the vocabulary rule alongside the existing enforcement-test description.
  - `manifest/designs/fabric-unified-view.md`: slice 8's section compacts to a shipped summary;
    the "Open questions" entry "Slice 8's CLI-wording question" (`:185`) is resolved and removed.
    The file survives until slice 10 and the open half of slice 6 both land, per its own header.
- Rationale: CLAUDE.md's Task completion rule — a task changing observable CLI behaviour and introducing cross-cutting infrastructure updates docs in the same commit.
  `manifest/roadmap.md` does **not** move: slice 8 is a planned item within an existing campaign, and the roadmap tracks the campaign.

## Technical context

**The 15 production call sites**, all verified by grep:

| file:line | today | becomes |
|---|---|---|
| `internal/buildercli/weft.go:24,32,37` | `WeftWorktree` + `New` + `res.WeftCommitted` | `Open(layout)` + `res.Committed()`; file → `sync.go` |
| `internal/webstercli/weft.go:21,27,32` | same | same; file → `sync.go` |
| `internal/perchcli/run.go:322,344,353` | same, inline | `Open(c.layout)` + `res.Committed()` |
| `internal/builderengine/spawn.go:367` | `New(…, WeftWorktree(…))` for `WarpResetter` | `Open(deps.Layout)`; interface → `FabricResetter` |
| `internal/websterengine/runlevel.go:817` | `New(…, WeftWorktree(…))` for `WarpBisector` | `Open(deps.Layout)`; interface → `FabricBisector` |
| `internal/websterengine/audit.go:91` | `regexp.QuoteMeta(WeftWorktree(layout))` | `fabricengine.NewRefScanner(layout)` |
| `internal/loomengine/preflight.go:105` | `os.Stat(WeftWorktree(l))` | `fabricengine.Ready(l)` |
| `internal/fabriccli/weft_verbs.go:102` | `New(l.WorktreePath(), WeftWorktree(l))` | `Open(l)` |

`runlevel.go:817` is **not** in the task brief's list of seven — it is the same construction pattern as `spawn.go:367` and was found by grep.
`fabriccli/weft_verbs.go:244` (`spawnPush(fabricengine.WeftWorktree(l))`) keeps calling the exported `WeftWorktree`;
`fabriccli` is a registered owner.

**Verified facts that shape the plan:**

- `Fabric.Warp` / `Fabric.Weft`: 45 uses inside `fabricengine`, **zero** in `fabriccli` or anywhere else.
  Unexporting is a mechanical in-package rename.
- `PartialCommitError`: zero readers outside `fabricengine`.
- `PushWeft`, `PullWeft`, `WeftSHAForWarpSHA`, `RecordCorrespondence`, `WeftLyxDir`, `WeftWorktreePath`, `WeftRepoRoot`, `HostLyxLink`, `HostJunctions`, `PortalLink`, `LauncherDir`: **zero production callers** outside fabric.
  Only `*_test.go` files and one doc comment at `boardengine/board.go:34`.
  These need no API change — but the enforcement test will flag the comment.
- `internal/weftname` production importers outside fabric: **only** `websterengine/audit.go`, which `refscanner-owns-both-halves` removes.
  After this task the import is confined to `fabricengine`, `fabriccli`, `lyxtest`.
- `internal/fabricengine` in-package callers of the soon-private `newPaired`: `index.go:307`, `unwire.go:100` (both hold raw paths already).
- `Commit`'s `SkipGit`-before-`New` short-circuit: all three CLI sites open-code `if !opts.SkipGit { … }` around the constructor, each with a comment explaining that `New` stats eagerly while `Commit` itself short-circuits.
  The guard stays (behaviour is unchanged);
  the explanatory comments reword.

**The comment cleanup** spans 55 production files / ~380 occurrences.
Largest non-owner concentrations: `websterengine/audit.go` (27), `builderengine/spawn.go` (21), `perchcli/run.go` (20), `websterengine/runlevel.go` (13), `loomengine/preflight.go` (12), `burlerengine/doc.go` (12), `webstercli/run.go` (11), `buildercli/run.go` (10), `buildercli/weft.go` (10).
Comment-only mentions also exist in low-level packages that never touch fabric's API — `lyxcwd/anchor.go`, `lyxcwd/lyxcwd.go`, `logger/sink.go`, `gitrepo/doc.go`, `configengine/config.go`, `scoutengine/daemonstate.go` — and reword cleanly (e.g. "the durable, weft-synced `_lyx`" → "the durable, fabric-synced `_lyx`").

**Sequencing note for mill-plan:** unexporting `New` breaks `fabriccli` in the same compile unit, so the constructor change and every call site must land in one commit — the repo cannot be left non-compiling between batches.
The comment-only cleanup in packages with no call site (`burlerengine`, `treadleengine`, `perchengine`, `logger`, `gitrepo`, `configengine`, `scoutengine`, `lyxcwd`, `configsync`, `selfreportcli`, `reedcli`, `reedengine`, `shuttlecli`, `burlercli`, `configcli`, `vscode`, `ideengine`) is independent and can be its own batch, but must land before the enforcement test is enabled.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and never mentions weft.
  This task does not add anything to `lyxcwd`;
  it extends the invariant's fabric bullet and adds a second enforcing test.
  `lyxcwd`'s import cap (stdlib + `internal/gitexec`) is untouched.
- **lyxtest Leaf Invariant** — `internal/lyxtest`'s import set is stdlib plus `lyxcwd`, `weftname`, `configengine`;
  `fabricengine`/`fabriccli` are banned.
  This is why `lyxtest` is a vocabulary owner rather than a cleanup target.
- **Fabric Git Invariant (warp + weft)** — every git operation lyx performs goes through `fabricengine`.
  Its own heading and body keep the two-repo vocabulary.
  Its board carve-out is the basis for `boardengine`'s owner-set membership.
- **CLI/Cobra Invariant** — `Short` on every command, help-tree tests.
  No command is added or removed here, but the renamed JSON key touches three `RunE` bodies.
- **Documentation Lifecycle** — see the `documentation` decision.

Discovered during discussion:

- `Open` must not change `Commit`'s behaviour in any way.
  The `SkipGit` guard placement, the combined write lock, the async detached push, and the three-outcome `PartialCommitError` mapping all stay exactly as they are.
- The `configsync` string-literal exception is permanent, not transitional.

## Testing

**TDD candidates** — write these first, they define the new API:

- `fabricengine`: `Open` returns `*ErrMissingPath` naming the warp path when the host worktree is absent, and naming the weft path when the sibling is absent, with warp checked first — the same contract `New` had, reached through a `*lyxcwd.Location`.
  Use `lyxtest.CopyPaired` for the happy path and delete one side for each failure case.
- `fabricengine`: `Ready(l)` returns `false, nil` when the sibling worktree is absent and `true, nil` when present.
  It must not error on absence — `loomengine` distinguishes absence from failure.
- `fabricengine`: `CommitResult.Committed()` is true when either side committed and false for a fully degenerate no-op.
  Table-driven over the four field combinations;
  no fixture needed.
- `fabricengine`: `RefScanner.Matches` covers the three cases `websterengine/audit_test.go` exercises today — a command containing the weft worktree path, a command containing a `-weft` sibling name, and a `lyx fabric`/`lyx weft`/`lyx warp` invocation — plus a clean command that must not match.
  This is the behavioural contract that must not regress when the regex moves packages.

**Regression coverage that must keep passing unchanged:**

- `buildercli/weft_integration_test.go:132-148` and `webstercli/weft_integration_test.go:140-157` — the commit-landed-but-`RecordCorrespondence`-failed case, asserting the helper reports `(true, err)` not `(false, err)`.
  These are the only coverage of the partial-failure path;
  they move to the renamed `sync.go` helper and the `Committed()` return, and must assert the same outcome.
- `websterengine/audit_test.go` — `CheckFork`/`CheckParent` still flag a fabric-referencing Bash command.
  Its three `fabricengine.WeftWorktree(layout)` calls and its `fakeLayout` helper move onto the scanner;
  the violation-class assertion updates to `ClassFabricReference`.
- `loomengine/preflight_integration_test.go:310` removes the weft worktree to drive the not-present branch;
  it keeps doing exactly that, through `lyxtest`'s fixture field rather than `fabricengine.WeftWorktree`.
- `buildercli/run_test.go:150` asserts the envelope contains `"weftCommitted"` → `"fabricCommitted"`.
- `configcli/configcli_integration_test.go:111` uses `fabricengine.WeftWorktreePath`, which stays exported (zero production callers, so no API change) — no edit needed.

**The enforcement test itself** needs a predicate sub-test on synthetic snippets, mirroring `TestEnforcement_GeometryLiterals`'s existing `t.Run("predicate", …)`: a non-owner file with `weft` in an identifier fails, one with `warp` in a string literal fails, one with `weft` in a comment fails, an owner-set file with all three passes, and a `configsync`-row file passes on a string literal but fails on an identifier.
Without this the test can silently stop matching anything and still go green.

**Full-suite gate:** `go test ./...` must pass.
The renames touch exported symbols in five packages, so compile breakage in test files is the expected first signal.

## Q&A log

- **Q:** Should the fix add one unified `Open(l)`, or split by what each caller uses (`CommitScoped` for the CLI sites, `OpenWarp` for the engine sites)? **A:** One unified `Open(l)`. The point is that no external caller may see that warp and weft exist — a warp-named constructor breaks that as surely as a weft-named one.
- **Q:** What does `New` actually do, and why must callers ask for it? **A:** Almost nothing — two `os.Stat`s and two path-holding structs. Establishing this is what killed the `CommitScoped`/`OpenWarp` split.
- **Q:** Why should `Commit` have a result type that isn't just a standard commit's? **A:** It is the standard `Commit`'s return type; it has four fields only because one call can land two git commits. Resolved by adding `Committed()` rather than changing the verb.
- **Q:** How should `loomengine`'s pairing probe be named? **A:** `Ready(l)`, not `Paired(l)` — "paired" announces two repos.
- **Q:** Should the audit regex move into fabric, and under what name? **A:** Yes, both halves, as `RefScanner`. `Pattern` is rejected outright — it is `internal/patternengine`'s name.
- **Q:** Should CLI output ever say "weft" to the end user? (the open decision recorded in `fabric-unified-view.md:185`) **A:** No. Consumers say "fabric"; fabric's own wrapped error keeps the weft-level detail. The JSON key is renamed too.
- **Q:** Does the vocabulary ban cover comments, or only code? **A:** Comments too — the task is a *visibility* cleanup, so a doc comment narrating the weft leaks the model just as effectively as a symbol.
- **Q:** Should `fabricengine.List` be renamed or made private? **A:** Neither. A fabric worktree's path is its warp worktree's path, so `List` already lists fabric worktrees.
- **Q:** Should reopening the leak be machine-checked? **A:** Yes — a new `TestEnforcement_FabricVocabulary` alongside the existing geometry guard.
