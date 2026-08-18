# Discussion: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
task: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations
slug: orchestrator-preflight
status: discussing
parent: standalone-producers
```

## Problem

`loomengine.Preflight` (`internal/loomengine/preflight.go`) is the only implementation in the repo of what `manifest/designs/producers-standalone.md` calls tiers 1 and 2 — geometry resolution via `lyxcwd.Resolve`, then `fabricengine.PrimeName`/`Clean`/`Ready`/`Healthy`.
Its check 4 is `loom`-specific: `_lyx/loom/status.json` presence, readability and coherence.
`Hardener`, and any future `Shed` product, would have to re-implement checks 1–3 verbatim to gate themselves the same way.
The three-tier model that the producers-standalone design rests on — *producers require nothing, orchestrators require tier 3* — is a convention rather than something the code enforces, precisely because tiers 1 and 2 are welded inside one orchestrator's package.

**Why now.** This is task T5 of `manifest/designs/producers-standalone.md`, wave 2.
Two later tasks in that decomposition are blocked on it in a non-obvious way.
T7 (wave 3, standalone `lyx webster run`) and T8 (wave 4, standalone `burler`/`perch`) both need a *hub-mode trigger* — a check that says "Fabric is actually wired here, so run in hub mode; otherwise run standalone" — and that check is a tier-2 check.
A `*cli` package cannot make one today without importing `internal/fabricengine`, the repo's largest and most dangerous package, directly into a CLI.
Both tasks also need two small shared leaves (`internal/buildinfo`, `internal/standalonestate`) that originally lived in T8's brief on the assumption `burlercli`/`perchcli` were the only standalone entry points.
They are not — T7 sits an entire wave earlier than T8, so anything only T8 introduced would not exist when T7 needs it.
T5 precedes both, so the design doc moved all of it here rather than making T7 depend on T8 or duplicating the logic twice.

There is also a live defect in scope.
`cmd/lyx/main.go:97` sets `cobra.EnableTraverseRunHooks`, so root's `seedStencils` (`cmd/lyx/stencilseed.go:33`) runs before *every* module pre-run, and it triggers on bare `lyxcwd.Resolve` success (`stencilseed.go:51-56`).
`lyxcwd.Resolve` succeeds in any ordinary git repository run from its root, handing back a `Location` whose `HubPath` is `filepath.Dir(worktreeRoot)` — never verified to be a hub.
Pointed at a plain downloaded repo, `seedStencilsAt(l.HubPath, …)` therefore writes `<repo-parent>/_board/_lyx/stencils/**` and tries to commit it: a fictional-hub write, happening one layer above any single CLI.
It is invisible today only because nobody points `lyx` at a non-lyx git repo on purpose yet.

## Scope

**In:**

- A new `internal/preflight` package holding the orchestrator-agnostic tier-1 + tier-2 checks lifted out of `loomengine`, plus the `CheckID`/`Failure`/`Report` result types, its `doc.go`, and its tests.
- `internal/loomengine/preflight.go` rewritten to compose `internal/preflight` with its own check-4 (`_lyx/loom/status.json`) logic; `internal/loomengine/report.go` reduced to the loom-specific check IDs plus aliases.
- A new stdlib-only `internal/buildinfo` package owning the ldflags-stamped build channel, plus its `doc.go` and tests.
- A new stdlib-only `internal/standalonestate` package owning the `hash8` + per-OS `<state>` directory derivation, plus its `doc.go` and tests.
- A new `stencilstore.ModeFor(dev bool) Mode` helper — the single mapping site from "is this a dev build" to `stencilstore.Mode` — plus its test.
- A drift guard in `tools/deploy`'s existing `main_test.go` pinning the ldflags `-X` path against `internal/buildinfo`'s symbol.
- `cmd/lyx/stencilseed.go`: drops its own `buildChannel` variable in favour of `buildinfo`, and gates `seedStencils` on **`preflight.HubPresent`**.
  Not `preflight.Wired` — the wiring predicate is the T7/T8 hub-mode trigger and is explicitly rejected for this gate, because it would stop seeding in a real hub whenever cwd is `<hub>/_board` or an unpaired worktree. See the `seed-gate-…` Decision.
- `tools/deploy/main.go:62`: the ldflags path repointed from `-X main.buildChannel=dev` to `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev`.
- `CONSTRAINTS.md`: two new leaf invariants (`Buildinfo Leaf Invariant`, `Standalonestate Leaf Invariant`), each with a mechanical enforcement test.
- Docs in the same commit: `doc.go` for each of the three new packages, three rows in `docs/overview.md`'s directory tree, and **two** bullets — `internal/buildinfo` and `internal/standalonestate` only — under `docs/shared-libs/README.md`'s `## Implementation-only libraries` section.
  Not `## Libraries`: that section's contract is one dedicated `<name>.md` doc file per entry, and both of these are documented in their own `doc.go`, exactly as `internal/modelspec` and `internal/state` already are.
  No new `.md` files under `docs/shared-libs/` are created by this task.
- **`internal/preflight` deliberately does not go in `docs/shared-libs/README.md` at all** — it gets the `docs/overview.md` tree row and its `doc.go`, and nothing else.
  That file's stated line is that a shared lib "does one mechanical thing … carries *no* domain logic" (`README.md:7-9`), and `preflight` carries orchestrator precondition *policy*: which checks constitute readiness, how a failure is classified, what blocks a downstream seed read.
  Listing it beside `fsx` and `lock` would quietly redefine what that section means.
  `buildinfo` (read a stamped string) and `standalonestate` (hash a path, pick a directory) are mechanical in exactly the intended sense and belong there.
  Correspondingly, `docs/overview.md:315`'s shared-infrastructure sentence gains `internal/buildinfo` and `internal/standalonestate`, not `internal/preflight`.

**Out:**

- **The three-tier invariant in `CONSTRAINTS.md`.**
  That belongs to T10 (`standalone-docs-and-invariants`), whose own brief states the rule is only true once every package obeys it, and that writing it before T5 ships would pin a model the code does not implement.
  T5 makes the rule *true for the preflight*; it does not write the rule down.
- **Rewording the Cwd Resolution Invariant.** Also T10.
- **Any consumer of `internal/standalonestate`.**
  This task lands the derivation and its tests; `burlercli`/`perchcli`/`webstercli` wire it up in T7 and T8.
  Nothing in `cmd/` or any `*cli` package imports `standalonestate` at the end of this task, and that is expected, not an oversight.
- **Any change to `internal/fabricengine`.**
  `internal/preflight` calls its existing exported functions and adds nothing to it.
- **Behaviour changes to `loomengine.Preflight`.**
  Same `Report`, same failure classification, same report-not-error contract, same short-circuit semantics for every case its current tests pin.
- **The `stencilstore.Reconcile` machinery itself**, the stamping mechanism, and the Dev/Prod Binary Separation invariant.
  Only the ldflags *symbol's home* moves.
- **`manifest/roadmap.md`.**
  Per `CLAUDE.md`, the roadmap moves on completing a planned item; the producers-standalone wave-2 entry moves when the wave completes, not per-task. Leave it alone.

## Decisions

### result-types-live-in-preflight-loomengine-aliases

- **Decision.** `internal/preflight` declares `CheckID`, `Failure`, `Report`, and the five tier-1/tier-2 check-ID constants (`CheckGeometry`, `CheckWorktreeClean`, `CheckFabricReady`, `CheckFabricSync`, `CheckJunction`).
  `internal/loomengine/report.go` keeps its four loom-specific constants (`CheckSeedMissing`, `CheckSeedUnreadable`, `CheckSeedIncoherent`, `CheckHalfFinished`) — now typed as `preflight.CheckID` — and adds type aliases (`type Report = preflight.Report`, `type Failure = preflight.Failure`, `type CheckID = preflight.CheckID`) plus const aliases for the five lifted IDs (`const CheckGeometry = preflight.CheckGeometry`, and so on).
- **Rationale.** Type *aliases*, not new named types, mean `loomengine.Report` and `preflight.Report` are the identical type: every existing reference in `internal/loomengine/preflight_integration_test.go` (which names `loomengine.CheckGeometry`, `loomengine.CheckWorktreeClean`, etc. across 13 test functions) keeps compiling with no edit, and any future caller can pass a `preflight.Report` where a `loomengine.Report` is expected.
  A single `Report` type also means an orchestrator composes tier-3 failures onto the tier-1/2 report by appending, never by converting between two parallel shapes.
- **Rejected.**
  *Duplicate types in both packages with a conversion function in `loomengine`* — every check-ID constant would need a translation table, and the two `CheckID` string sets would drift silently the first time one side added a value.
  *`preflight` returns a bare `[]Failure` and `loomengine` keeps `Report`* — pushes the `OK == (len(Failures) == 0)` invariant and the `addFailure` bookkeeping into every consumer, which is exactly the duplication the lift exists to remove; `Hardener` would re-implement `Report` verbatim.

### preflight-exposes-four-entry-points

- **Decision.** `internal/preflight` exports exactly four functions:
  - `Check(cwd string) (Report, *lyxcwd.Location, error)` — resolves geometry itself, runs tiers 1 and 2, and hands the resolved `Location` back so the caller never re-resolves.
    On `lyxcwd.ErrNotAGitRepo` it returns `(Report{OK:false, Failures:[{CheckGeometry, "not inside a git repository"}]}, nil, nil)`; on any other `Resolve` error it returns `(Report{}, nil, err)`.
  - `CheckResolved(l *lyxcwd.Location) (Report, error)` — the same tier-2 body against an already-resolved `Location`, for callers that hold one and for tests that synthesise one with no backing directory on disk.
  - `Wired(cwd string) (*lyxcwd.Location, bool)` — the cheap boolean hub-mode predicate.
  - `HubPresent(cwd string) (*lyxcwd.Location, bool)` — the cheap boolean hub-existence predicate the stencil seed gates on.

  The two predicates are deliberately distinct; see the next Decision for why one cannot serve both.
- **Rationale.** `Check` is what an orchestrator calls; `CheckResolved` is what `loomengine`'s existing `CheckResolvedForTest` seam becomes, preserving the ability to drive checks against a hand-built `Location`; `Wired` is what a CLI pre-run calls.
  Returning the `*lyxcwd.Location` from `Check` matters: `lyxcwd.Resolve` spawns `git rev-parse --show-toplevel`, so a shape that forced `loomengine` to resolve again to reach `LoomStatusFile(l)` would double the git spawns of every preflight.
- **Rejected.**
  *`CheckResolved` only, caller resolves* — every orchestrator would re-implement the `ErrNotAGitRepo`-is-a-verdict-not-an-error special case, which is subtle enough that duplicating it is how the classification drifts.
  *`Check(cwd) (Report, error)` with no `Location` return* — forces the double resolve above.

### seed-gate-is-tier1-plus-Ready-not-the-full-tier-2-report

- **Decision.** Two distinct predicates, because the seed gate and the hub-mode trigger are asking two different questions, and conflating them regresses a real hub.
  - `preflight.Wired(cwd) (*lyxcwd.Location, bool)` — `lyxcwd.Resolve(cwd)` succeeding **and** `fabricengine.Ready(l)` returning true.
    This is the **hub-mode trigger** T7 and T8 consume to choose hub mode over standalone: "is Fabric wired *for this worktree*".
  - `preflight.HubPresent(cwd) (*lyxcwd.Location, bool)` — `lyxcwd.Resolve(cwd)` succeeding **and** `<hub>/_board/_lyx` existing on disk (`filepath.Join(fabricengine.BoardDir(l.HubPath), lyxdirs.LyxDirName)`, one `os.Stat`).
    This is the **seed gate**: "does the hub this write targets actually exist".
    `cmd/lyx/stencilseed.go`'s `seedStencils` gates on this one, not on `Wired`.

  Both return `(nil, false)` on any error rather than surfacing one, since both are predicates for a pre-run that must never block a command.
- **Rationale for splitting them.** `fabricengine.Ready` (`internal/fabricengine/ready.go:17`) is a single `os.Stat` of the *paired sibling* of the current worktree — `weftname.SiblingPath(l.HubPath, filepath.Base(l.WorktreePath()))`.
  That is a per-worktree pairing probe, not a hub probe, and the difference bites in a real, healthy hub.
  `<hub>/_board` is itself a second Fabric worktree materialised by `fabricengine`'s clone (`clone.go:84,290`), and it has no paired sibling of its own — so `Ready` is false when cwd is `<hub>/_board`, and equally false in any unpaired sibling or in a worktree whose pair was removed.
  Today `seedStencils` seeds correctly from all of those, because `HubPath` is `filepath.Dir(worktreeRoot)` and that value is *right* in every one of them.
  Gating the seed on `Ready` would therefore silently stop stencil seeding in three real-hub situations, which is a regression this task has no mandate for — it is here to close the fictional-hub write, not to narrow a working path.
  The honest precondition for the seed is the one `HubPresent` states: the write targets `<hub>/_board/_lyx/stencils`, so the thing that must exist is `<hub>/_board/_lyx`.
  That predicate is true in all three real-hub cases above and false in a plain downloaded repo at `/x/repo` (where `HubPath` is the fiction `/x` and `/x/_board/_lyx` does not exist), so it closes the defect and narrows nothing.
  It is structurally the same cheap hub predicate `fabricengine`'s own `looksLikeHub` (`clone.go:645`) already uses internally — reused in spirit rather than by export, since widening `fabricengine`'s API is out of scope for this task.
  Both predicates cost zero process spawns, which is what makes either viable before every single command.
  The full tier-2 set is wrong for *this* gate on two counts.
  `Clean` spawns `git status` on both sides of the pair, and `seedStencils` runs before every single `lyx` command via `EnableTraverseRunHooks` — that is a per-invocation regression on every command in the CLI.
  Worse, `Clean` failing would mean a *dirty hub does not get its stencils seeded*, which is a behaviour change nobody asked for and which would surface as stencils mysteriously going stale mid-work.
  `Healthy`'s branch-mismatch case has the same problem.
  `PrimeName` spawns `git worktree list` and adds nothing either stat does not already prove.
- **Note for the reviewer.** T5's brief says "the identical tier-1-AND-tier-2 check".
  `Wired` is that check in the sense the design doc means it — the tier-2 *wiring* predicate, named in T8's own brief as "`fabricengine.Ready`-class, reached through the `internal/preflight` package T5 lifts" — not the full four-function tier-2 report.
  Both predicates living in `internal/preflight` is what satisfies the real constraint, which is that no `*cli` package imports `internal/fabricengine` to make the check.
  The brief's one sentence about the root gate assumed a single predicate served both purposes; it does not, and this discussion splits them rather than regressing the seed to make one name cover both.
- **This deviation gets a durable home, same as the `StencilMode()` one.**
  `_mill/discussion.md` is worktree-local and vanishes on merge, and this task's Scope forbids editing the design doc.
  So `internal/preflight/doc.go` must state, in the code that ships: that `Wired` and `HubPresent` both exist, what each one asks, which of the two the stencil seed gates on, and why `Ready` alone is wrong for the seed.
  Without that, the next reader of T5's brief sees "the identical tier-1-AND-tier-2 check", finds two predicates, and "fixes" the seed back onto `Wired` — reintroducing exactly the `_board` regression round 2 caught.
- **Rejected.**
  *One predicate for both purposes.* Either choice is wrong for one of the two callers: `Ready` regresses the seed in `_board` and unpaired worktrees, and a hub-presence probe is too weak for T7/T8's mode selection, since a hub-level `_board/_lyx` can exist while *this* worktree is not wired — which is exactly the `(resolved, not wired)` plain-worktree row T8's brief calls out as selecting standalone.
  *Gate on `preflight.Check(cwd).OK`* — the `Clean`/`Healthy` problems above, on top of the `Ready` narrowing.
  *Stat `<hub>/_board/_lyx` inline in `stencilseed.go`* — re-implements the gate outside `preflight`, so T7's and T8's copies would be a third and fourth implementation, and the Fabric geometry knowledge leaks into `package main`.
  *Exporting `fabricengine.looksLikeHub`* — widening `internal/fabricengine`'s API is scoped out of this task, and `BoardDir` already exports the only token `HubPresent` needs.

### report-exposes-Has-instead-of-a-blocks-seed-field

- **Decision.** `Report` gains two exported methods: `Has(CheckID) bool` and `AddFailure(CheckID, string)` (the latter replacing today's unexported `addFailure`, since `loomengine` now appends from another package).
  `loomengine` derives its two control-flow signals from the returned report rather than from bespoke fields:
  - geometry short-circuit: `report.Has(CheckGeometry)` → return the report unchanged, do not run check 4 (matching today's two early returns at `preflight.go:43` and `preflight.go:65`).
  - today's `check3BlocksSeed`: `report.Has(CheckFabricReady) || report.Has(CheckJunction)`.
- **Rationale.** Those are exactly the two conditions that set `check3BlocksSeed = true` in the current code (`preflight.go:100` on `!ready`, `preflight.go:118` on the non-branch-mismatch `Healthy` causes, which is the same branch that adds `CheckJunction`), so the derivation is equivalence-preserving rather than approximate.
  Deriving keeps `Report` a plain value type with no orchestrator-specific state riding along, which matters because `Hardener` will want its own downstream-consequence rule and it will not be this one.
- **Rejected.**
  *A `BlocksSeed bool` field on `Report`* — bakes one orchestrator's check-4 concern into the shared type.
  *A second return value from `CheckResolved`* — same coupling, plus it widens the signature for every caller that does not care.

### buildinfo-exposes-IsDev-not-StencilMode

- **Decision.** `internal/buildinfo` exports `var Channel string` and `func IsDev() bool` (`Channel == "dev"`), and imports nothing but the standard library — in practice, nothing at all.
  The mapping to a stencil mode moves into `internal/stencilstore` as a new `func ModeFor(dev bool) Mode`.
  `cmd/lyx/stencilseed.go` becomes `mode := stencilstore.ModeFor(buildinfo.IsDev())`; T7's and T8's CLIs do the same.
- **Rationale.** T5's brief names the accessor `StencilMode()`, but `stencilstore.Mode` is a named `int` type declared in `internal/stencilstore/stencilstore.go:135`, and `internal/stencilstore` imports `internal/logger` and `internal/stencil`.
  A `buildinfo.StencilMode()` returning it would make `buildinfo` a non-leaf, contradicting the stdlib-only requirement in the *same paragraph* of the brief — and that requirement is the load-bearing one, since it is what lets `cmd/lyx` and every `*cli` package import `buildinfo` with no cycle risk.
  `ModeFor` in `stencilstore` keeps a single mapping site (no duplication across three future CLIs) and puts the `Mode` knowledge in the package that owns `Mode`.
  Semantics are preserved exactly: an unstamped binary leaves `Channel` empty, and empty maps to `ModeProduction`, which is also `Mode`'s zero value.
- **Rejected.**
  *`buildinfo` declaring its own mirror `Mode` type* — two `Mode` types one conversion apart is worse than one accessor rename.
  *Relaxing the leaf to stdlib + `stencilstore`* — `stencilstore` pulls in `logger` and `stencil`, and the whole point of the leaf is that a CLI can read the build channel without inheriting anyone's dependency set.
- **Consequence for the brief.** This is a deliberate, documented deviation from T5's `StencilMode()` wording.
  Record it in `internal/buildinfo/doc.go` so the next reader of the design doc does not "fix" it back.

### standalonestate-is-pure-derivation-with-an-injectable-seam

- **Decision.** `internal/standalonestate` exports one function:

  ```go
  func Derive(target string) (stateDir string, hash8 string, err error)
  ```

  `hash8` is SHA-256 over the normalised absolute target path, hex-encoded, truncated to the first eight characters.
  Normalisation is `filepath.EvalSymlinks` → `filepath.Clean`, falling back to `Clean` alone when `EvalSymlinks` fails (the target may not exist yet), and lower-cased before hashing when `goos == "windows"`.
  `stateDir` is `%LOCALAPPDATA%\lyx\<hash8>\` on Windows, and `$XDG_STATE_HOME/lyx/<hash8>/` falling back to `~/.local/state/lyx/<hash8>/` everywhere else.
  `Derive` creates nothing on disk.

  **`target` must already be absolute; a relative path is rejected with an error.**
  `Derive` calls `filepath.IsAbs(target)` first and returns an error when it is false.
  It never calls `filepath.Abs`.
  The consuming CLI (T7/T8) absolutises its own `--path`/`target.paths` value at its argument-parsing boundary, where a process cwd is a legitimate thing to consult, and passes an absolute path in.

  `Derive` returns an error in exactly three cases, and no others: `target` is not absolute; `LOCALAPPDATA` is unset on Windows; `XDG_STATE_HOME` is unset *and* `os.UserHomeDir` fails elsewhere.
  `EvalSymlinks` failing is explicitly *not* an error — it is the documented `Clean`-only fallback for a target that does not exist on disk yet.

  **`hash8` collisions are accepted, not handled.**
  Eight hex characters is 32 bits, so two distinct targets can in principle derive the same `stateDir`, socket and session — the same failure the normalisation rules exist to prevent, arriving by a different route.
  `Derive` does not detect this and has no collision return: the truncation width is inherited from the design doc's `hash8` definition, not chosen here, and re-litigating it is out of scope for T5.
  The disposition is stated rather than left implicit so a later reader knows it was considered: a colliding pair shares one tmux server and one state directory, which is a wrong-but-not-corrupting outcome, and the fix if it ever matters is to widen `hash8` in this one package — every consumer takes the value from `Derive` rather than re-deriving it, so the widening is a one-line change with no call-site churn.
  Internally it is a thin wrapper over an unexported `derive(goos, localAppData, xdgStateHome, home, target string) (string, string, error)`, driven directly by **in-package** tests (`package standalonestate`).
  No `export_test.go`: an in-package test reaches an unexported function without one, so a shim would be the same dead code the `preflight-tests-are-an-external-test-package` decision rejects.
  In-package is available here — and was not for `preflight` — because `standalonestate` is stdlib-only and its tests need no `hubforge` fixture, so there is no import cycle to route around.

  **At the seam boundary, the empty string means "unset".**
  The three env-ish parameters are plain strings, so `derive` cannot distinguish unset from set-to-empty and deliberately does not try: an empty `localAppData` on the Windows branch is the error case, and an empty `xdgStateHome` on the POSIX branch is what selects the `home` fallback.
  That collapse is correct on its own terms — neither an empty `%LOCALAPPDATA%` nor an empty `$XDG_STATE_HOME` is a usable directory, so treating both the same as absent is the only sane reading.
  `Derive` fills the parameters from `os.Getenv("LOCALAPPDATA")`, `os.Getenv("XDG_STATE_HOME")` and `os.UserHomeDir()`, and calls `os.UserHomeDir` **only on the POSIX branch** — on Windows it passes `""` for `home`, so a `UserHomeDir` failure can never surface as an error on a branch that never reads the value.
- **Rationale on normalisation.** Two spellings of the same directory — a symlinked path, a differing-case path on Windows or macOS — must hash identically, or two standalone runs against the same target get different sockets, sessions and state directories, silently destroying the "one tmux server per target directory, resumable" property this whole derivation buys.
  The semantics deliberately mirror `internal/lyxcwd/anchor.go`'s `normalizePath`/`samePath` (`anchor.go:112-129`), which already solve this exact class of problem: `EvalSymlinks` with a `Clean` fallback, plus case-insensitive comparison on Windows.
  Note the one intentional difference — `samePath` compares case-insensitively *after* normalising, whereas hashing has no comparison step, so the case fold must happen to the string before it is hashed.
  macOS is also case-insensitive in practice, but `lyxcwd` folds only on Windows, and matching `lyxcwd`'s rule exactly is worth more than being marginally more correct on a platform where the two derivations would then disagree.
- **Rationale on rejecting a relative target.** This is the second and last deviation from `normalizePath`, and both are now stated explicitly so neither is discovered as a surprise: `normalizePath` folds case only at comparison time (in `samePath`), whereas hashing has no comparison step so the fold must happen before the hash; and `normalizePath` contains no `filepath.Abs`, which is exactly why `Derive` must not add one.
  `filepath.Abs` resolves against the process working directory via `os.Getwd`.
  Calling it would make `internal/standalonestate` a cwd resolver, which the Cwd Resolution Invariant reserves to `internal/lyxcwd` alone, and would make the supposedly pure `derive(goos, env…, target)` seam depend on the host cwd of whatever test process ran it — silently host-dependent rather than injected.
  Rejecting a relative target keeps the package a pure function of its arguments and puts the one legitimate cwd consultation at the CLI boundary that owns it.
- **Rationale on the injectable seam.** `runtime.GOOS` is a compile-time constant, so without a seam the Windows row of the `<state>` table is untestable on Linux and the POSIX row is untestable on Windows — meaning CI would only ever exercise one of the two branches this task ships.
  Passing `goos` and the three environment values as parameters makes both rows' **branch selection and case-fold behaviour** testable everywhere, and keeping `derive` unexported keeps the seam out of the public API.

  **What the seam does *not* make host-independent: the path separator.**
  `filepath.Join` and `filepath.Clean` take their separator from the compile-time host, not from the injected `goos`, so on Linux the `goos == "windows"` row yields `/localappdata/lyx/<hash8>`, not `%LOCALAPPDATA%\lyx\<hash8>`.
  This is a property of `path/filepath`, not something the seam can or should fight.
  The Windows-row assertion is therefore built with the same `filepath.Join(localAppData, "lyx", hash8)` the implementation uses — never a literal backslash string, which would pass only on Windows and fail everywhere else.
  What the test pins is that the Windows branch consults `localAppData` (and the POSIX branch does not), plus the case fold; the separator is left to `filepath`.
  (`internal/loomengine/export_test.go`'s `CheckResolvedForTest` is the in-repo precedent for *injecting a seam a test needs* — reference only. It is not replicated here: `loomengine` needs a shim because its integration tests must be an external package to avoid the `hubforge` cycle, and `standalonestate`'s do not.)
- **Rejected.**
  *`Derive` also doing `os.MkdirAll`* — a pure derivation is testable with no filesystem and no cleanup, and the consumer that actually writes there (T7/T8) is better placed to decide when the directory should exist.
  *Reading `runtime.GOOS` and `os.Getenv` directly with `t.Setenv`-driven tests* — `t.Setenv` handles the env vars but cannot change `runtime.GOOS`, so half the table stays untested.
  *Supporting a relative target by taking the base directory as a fourth seam parameter* — it works, but it adds a parameter every caller must supply correctly for a case no caller has: T7 and T8 both hold an absolute path by the time they reach this call.
  *Reusing `lyxcwd`'s unexported `normalizePath` by exporting it* — widens `internal/lyxcwd`'s API for one caller, and `standalonestate` must stay stdlib-only, so it cannot import `lyxcwd` regardless.

### two-new-leaf-invariants-now-three-tier-rule-in-T10

- **Decision.** `CONSTRAINTS.md` gains `## Buildinfo Leaf Invariant` and `## Standalonestate Leaf Invariant` in this commit, each enforced by a `leaf_enforcement_test.go` in its own package.
  The three-tier invariant and the Cwd Resolution Invariant reword are **not** touched.
- **Rationale.** Both new packages are load-bearing *because* they are leaves — T7 and T8 import them from CLI packages specifically to avoid cycles — and an unenforced leaf claim rots the first time someone adds a convenience import.
  The enforcement tests are a direct copy of `internal/tokenvocab/leaf_enforcement_test.go`'s allowlist walk (`go/parser` with `parser.ImportsOnly`, stdlib detected as "no `.` in the first path segment"), with the allowlist set to empty for both packages since neither has a permitted non-stdlib import.
  Deferring the three-tier rule is T10's explicit instruction, quoted in its brief.
- **Rejected.**
  *Landing the three-tier invariant here* — contradicts T10's brief and would state a rule that is still false for the producer packages T6–T8 have not converted yet.
  *Leaf claims as review obligations only* — the repo's own precedent (`modelspec`, `tokenvocab`, `pattern`) is a mechanical test per leaf, and the CONSTRAINTS format requires naming an enforcement basis.

### preflight-tests-are-an-external-test-package

- **Decision.** `internal/preflight`'s git-fixture tests live in `package preflight_test` (external), integration-tagged, calling the exported `CheckResolved(l)` directly.
  `internal/preflight` gets **no** `export_test.go` — every seam its tests need is already exported, so a shim would be dead code.
  (Nor does `internal/standalonestate`: its tests are in-package and reach `derive` directly. **This task adds no `export_test.go` anywhere.**)
  `internal/loomengine`'s existing `preflight_integration_test.go` stays where it is and keeps testing `loomengine.Preflight` end-to-end unchanged.
- **Rationale.** The comment at the head of `internal/loomengine/preflight_integration_test.go` records why: `internal/hubforge` (the fixture helper) imports `internal/fabriccli`, so an *in-package* test importing `hubforge` from a package inside `fabriccli`'s dependency set closes a compile cycle.
  `internal/preflight` will sit in that same dependency set the moment anything under `fabriccli` reaches it, so adopting the external-test-package shape from the start avoids discovering the cycle later.
  `loomengine` needs its `export_test.go` because `checkResolved` is unexported there; `preflight`'s equivalent is exported by the entry-point decision above, so the external test package reaches it with no shim.
  Keeping `loomengine`'s existing 13 test functions running against the composed `loomengine.Preflight` is the actual proof that the lift changed no behaviour — they are the regression net, and none of them should need editing.
- **Rejected.**
  *Moving the tier-1/tier-2 test functions out of `loomengine` into `preflight`* — that would delete the very tests that prove `loomengine.Preflight` still behaves identically after the lift.
  New `preflight` tests are additive; `loomengine`'s are untouched.

### seed-gate-tested-through-an-extracted-target-seam

- **Decision.** `cmd/lyx/stencilseed.go` extracts the gate into `func stencilSeedTarget(ctx context.Context) (hub, worktree string, ok bool)`, which does the `lyxcwd.CwdFrom` → `preflight.HubPresent` sequence and returns `ok == false` whenever seeding must be skipped.
  `seedStencils` keeps its `testing.Testing()` early return and becomes a three-line wrapper over `stencilSeedTarget` + `seedStencilsAt`.
  An integration-tagged `cmd/lyx` test drives `stencilSeedTarget` against a real plain git repository and asserts both `ok == false` and that no `_board` directory exists beside the repo afterwards.
- **Rationale.** `seedStencils` returns immediately under `testing.Testing()` (`stencilseed.go:40-42`), so a test can never observe its gate through the exported entry point — the test would pass vacuously and prove nothing.
  Extracting the decision into a value-returning function makes the gate directly assertable, and mirrors the existing rationale for `seedStencilsAt` being separate ("so a test can drive it directly against a real hub without going through `seedStencils`' `testing.Testing()` guard").
  Asserting the absence of `_board` on disk, not just `ok == false`, is what pins the actual defect rather than an implementation detail.
- **Rejected.**
  *Testing `seedStencils` directly* — vacuous under the `testing.Testing()` guard.
  *Removing the `testing.Testing()` guard* — it exists to keep dozens of untagged `cmd/lyx` tests from spawning git, which the Test Tier Purity Invariant forbids.

## Technical context

**The lift, concretely.** `internal/loomengine/preflight.go` today is one exported `Preflight(cwd string) (Report, error)` plus an unexported `checkResolved(l *lyxcwd.Location) (Report, error)`.
The split runs straight through `checkResolved`: check 1b (`fabricengine.PrimeName`, with its short-circuit), check 2 (`fabricengine.Clean`), and check 3 (`fabricengine.Ready` then `fabricengine.Healthy`, with the `CauseBranchMismatch` → `CheckFabricSync` / everything-else → `CheckJunction` classification) all move to `internal/preflight`.
Check 4 — everything from the `os.Stat(LoomStatusFile(l))` at `preflight.go:125` to the `checkCoherence` loop — stays in `loomengine`, including the `os.MkdirAll(filepath.Dir(LoomStatusLock(l)))` workaround and its long comment, and including the TOCTOU `seed vanished between stat and read` synthesised error.

**`loomengine.Preflight` has no production callers today.** The only references anywhere are `internal/loomengine/preflight_integration_test.go` and `internal/loomengine/export_test.go`.
That makes the blast radius of the signature work small, and it means the integration test file *is* the compatibility contract.

**Existing helpers to reuse, not re-derive.**

- `internal/fabricengine`: `PrimeName(l)` (`worktreelist.go:86`), `Clean(l)` (`warpclean.go:20`), `Ready(l)` (`ready.go:17`), `Healthy(l)` (`drift.go:52`, returning a typed `HealthReason` with a `Cause` field and `CauseBranchMismatch` constant), `StencilsDir(hub)` (`junctionnames.go:126`).
  `Ready` is a one-line `os.Stat` of the paired sibling directory whose name the Fabric Vocabulary Invariant forbids `internal/preflight` from spelling — call `Ready`, never that helper, and see the Constraints section.
- `internal/lyxcwd`: `Resolve(cwd)`, `ErrNotAGitRepo`, `CwdFrom(ctx)`, and — as the semantic reference for `standalonestate`, not as an import — `normalizePath`/`samePath` (`anchor.go:112-129`).
- `internal/tokenvocab/leaf_enforcement_test.go` — copy this file's structure for both new leaf enforcement tests; it is the current idiom (`go/parser`, `parser.ImportsOnly`, allowlist map, stdlib-by-first-segment heuristic).
- `internal/loomengine/export_test.go` — reference only, for the external-test-package shape it demonstrates. This task adds no `export_test.go`; `preflight`'s seams are exported and `standalonestate`'s tests are in-package.
- `internal/hubgeom/doc.go` — the told-geometry vocabulary and tone the new `doc.go` files should match.

**Gotchas.**

- `lyxcwd.Resolve` validates far less than its name suggests: `git rev-parse --show-toplevel` must succeed, an absent `_board/.lyx-anchor` marker is *not* an error (`AnchorRel` falls back to `"."`), and `HubPath` is assigned `filepath.Dir(worktreeRoot)` unconditionally with no hub check.
  Every "is this really a hub" question must be answered at tier 2, never by a successful `Resolve`.
- There is deliberately **no** at-the-anchor check in `checkResolved` — `Resolve`'s cwd gate already proves `cwd == AnchorPath()`, and re-adding one broke every subpath-anchored hub.
  The comment at `preflight.go:70-74` explaining this must move to `internal/preflight` with the code, not be dropped.
- `Preflight`'s contract is *report, not error*: `(Report{}, err)` means "could not determine", and a `PrimeName` failure is deliberately reported as a `CheckGeometry` failure rather than escalated.
  `internal/preflight` must preserve this exactly, and its `doc.go` should restate it, since `Hardener` will be reading that doc rather than `loomengine`'s.
- **The comment being lifted alongside that contract is already false — do not carry it across verbatim.**
  `internal/loomengine/preflight.go:48-50` says the non-`ErrNotAGitRepo` branch catches "e.g. the git subprocess itself failed to spawn".
  It does not: `lyxcwd.gitWorktreeRoot` (`internal/lyxcwd/lyxcwd.go:150-163`) returns the bare sentinel for a `*gitexec.GitError` and `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)` for everything else — so an exec-level failure *also* satisfies `errors.Is(err, ErrNotAGitRepo)` and lands in the short-circuit branch, not the escalation branch.
  The residual `(Report{}, nil, err)` branch is reachable only through the anchor gate (`ErrCwdOutsideAnchor`, `ErrStaleAnchorMarker`).
  Reword it when lifting, and do not repeat the false characterisation in `internal/preflight/doc.go` — a shared package shipping a wrong claim about its own error contract is worse than the same claim sitting in one orchestrator.
- `stencilstore.Mode` is `ModeProduction Mode = iota` then `ModeDev` — `ModeProduction` is the zero value, which is why an unstamped binary safely means production.
  `ModeFor(false)` must return `ModeProduction`.
- Changing the ldflags path in `tools/deploy/main.go` and the variable's home in `cmd/lyx/stencilseed.go` must land together: a stale `-X main.buildChannel=dev` against a removed `main.buildChannel` fails silently (Go's linker does not error on an unmatched `-X`), producing a dev binary that behaves as production.
  Grep confirms exactly three sites: `tools/deploy/main.go:60,62` and `cmd/lyx/stencilseed.go:24,29,74`.
  The same-commit rule alone is a review obligation with no machine check behind it, which is why this task also adds the `tools/deploy` drift guard named in the Testing section — `tools/` lies outside every existing enforcement walk, so nothing else would ever catch the mismatch.
- New `cmd/lyx` test files must respect the Test Tier Purity Invariant — the plain-repo gate test spawns git, so it needs a `//go:build integration` first line, and `cmd/lyx` already has a `TestMain` for the Hermetic Git Test Environment Invariant.
- `internal/preflight` and `internal/standalonestate` both need their package name checked against nothing existing — `ls internal/` confirms neither name is taken.

**Docs to touch (same commit, per `CLAUDE.md`).** `docs/overview.md`'s directory tree around lines 228-244 (add three rows, alongside `internal/hubgeom`, `internal/modelspec`, `internal/tokenvocab`) and its shared-infrastructure sentence at line 315 (which gains `buildinfo` and `standalonestate` only); `docs/shared-libs/README.md`'s `## Implementation-only libraries` section, two bullets, again `buildinfo` and `standalonestate` only.
See the Scope bullets, which are authoritative for both the section choice and `preflight`'s deliberate absence.
`manifest/designs/producers-standalone.md` itself is deleted by T10, not edited here.

## Constraints

From `CONSTRAINTS.md`, binding this task:

- **Cwd Resolution Invariant** — `internal/lyxcwd` alone owns cwd resolution.
  `internal/preflight` *calls* `lyxcwd.Resolve`; it must never re-derive a worktree root, a hub path, or an anchor itself, and `internal/standalonestate` must not resolve a cwd at all (it is told a target path).
- **Test Tier Purity Invariant** — untagged test files spawn nothing.
  Every new test that touches a real git repo or a `hubforge`/`gitkit` fixture needs a `//go:build integration` constraint on its first non-empty line, and is enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant** — any new git-spawning test *package* needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
  `internal/preflight` will be one; `cmd/lyx` and `internal/loomengine` already have theirs.
- **Dev/Prod Binary Separation** — unchanged in substance, but this task moves the stamped symbol.
  The `-dev` build must still land in `.dev-bin` and must still be the only thing that produces `ModeDev`.
- **Fabric Vocabulary Invariant** — this is the constraint most likely to break the build, and it is not the read-only rule.
  The bare tokens `weft` and `warp` are banned outright — in identifiers, string literals *and* comments — in every production `.go` file under `internal/` and `cmd/`, and in every `internal/**/*.md`, outside a fixed owner set.
  The owner set is `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/gitkit`, `internal/hubforge`, `internal/boardengine`, `internal/configsync`.
  **None of `internal/preflight`, `internal/buildinfo`, `internal/standalonestate`, `internal/loomengine`, or `cmd/lyx` is in it.**
  The match is a case-insensitive **substring** test (`bareVocabularyToken`, `internal/lyxcwd/enforcement_test.go:649-657`), written that way deliberately to catch the token inside a camelCase identifier — so `fabricengine.WeftWorktree` is a violation *as an identifier reference*, not merely as prose.
  Concretely, for this task: `internal/preflight` must call `fabricengine.Ready(l)` and must never name `WeftWorktree` in production code (`Ready` already encapsulates it, which is what makes the narrow gate viable at all);
  every comment and `doc.go` in the three new packages describes the check as "Fabric is wired here" or "the worktree pair", never "the weft sibling".
  `internal/loomengine/preflight.go:78-80` is the in-repo wording precedent to copy — it says "worktree pair cleanliness" for exactly this reason.
  Note this binds `internal/**/*.md` too, so any module doc placed inside a package directory obeys it; `_mill/discussion.md` is outside the walk, which is why this file may name the identifier while the code it describes may not.
  Enforced by `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`); `*_test.go` files are excluded from the rule.
- **Fabric Write-Side Containment / Mutation Record Invariant** — a separate rule, and the one the placement decision serves: `internal/preflight` is read-only, so it must not acquire a lock, write, or record a mutation.
  Keeping it out of `internal/fabricengine` is precisely so a read-only check does not inherit that blast radius.
- **Documentation Lifecycle** — new packages get a `doc.go`; `docs/overview.md` moves when the module table changes.
- **CLI / Cobra Invariant** — `cmd/lyx/stencilseed.go` changes must not alter command registration, `Short` strings, or the help tree; the existing `cmd/lyx/helptree_test.go` and `registration_test.go` should be untouched and still pass.

From the design doc, binding this task:

- Placement in `internal/preflight` is decided, not open — never a composite verb on `internal/fabricengine`.
- `internal/buildinfo` and `internal/standalonestate` are stdlib-only leaves.
- `loomengine.Preflight`'s observable behaviour must be unchanged.
- T5 is parallel-safe with T4. It owns `cmd/lyx/stencilseed.go` and `tools/deploy/main.go` for this wave, neither of which appears in T4's Files list.

**Two files this task touches that the design doc's T5 file list does not name, both stated here so neither is a surprise at merge:**

- **`CONSTRAINTS.md` is contended with T4** — it appears in T4's own Files list (`producers-standalone.md:337`), so both wave-2 tasks edit it.
  The edits are disjoint *regions*: T4 rewords the existing `## Pattern Leaf Invariant` section to tighten its allowlist, T5 appends two brand-new sections (`## Buildinfo Leaf Invariant`, `## Standalonestate Leaf Invariant`).
  Neither reads or rewrites the other's region, so whichever lands second resolves a mechanical append-vs-append rebase, never a design conflict.
  Do not attempt to coordinate ordering — just rebase.
- **`internal/stencilstore/stencilstore.go`** gains `ModeFor(dev bool) Mode` and is a third T5-owned file.
  It is absent from T5's file list in the design doc because that list predates the `StencilMode()` resolution above.
  It is **uncontended**: T4 touches `internal/pattern`'s *import* of `stencilstore`, never this file.
- The one genuine adjacency remains `internal/loomengine` — T4 edits `plan.go`, T5 edits `preflight.go`/`report.go`. Different files; T4's own discussion already flagged it as a mechanical rebase point.

## Testing

**`internal/preflight` — TDD candidate, and the main new test surface.**
Untagged, in-package tests for anything that needs no git: `Report.Has`, `Report.AddFailure`, and the `OK == (len(Failures) == 0)` invariant.
An integration-tagged `package preflight_test` file for the check bodies, calling the exported `CheckResolved` directly against `hubforge` fixtures.
Scenarios to cover: healthy wired pair; not a git repo (via `Check`, asserting the `CheckGeometry` verdict *and* a nil error); `PrimeName` failure short-circuiting with only a geometry failure; dirty warp; dirty weft; fabric not ready; branch mismatch classifying as `CheckFabricSync`; a broken junction classifying as `CheckJunction`; a subpath-anchored hub *not* being rejected for standing at its own anchor; and multiple simultaneous failures collecting rather than short-circuiting.
The last two are the ones most likely to regress silently in a lift, since both are behaviours a naive rewrite removes.

**`internal/loomengine` — the regression net, ideally edited not at all.**
`preflight_integration_test.go`'s 13 existing test functions are the compatibility contract: same `Report`, same failure classification, same report-not-error contract.
If the type aliases are done right, this file needs zero edits, and "it compiles and passes unmodified" is itself a meaningful assertion — treat any required edit to it as a signal that the alias decision was implemented as duplicate types instead.
`export_test.go`'s `CheckResolvedForTest` should now delegate to the composed path so those tests still exercise checks 1b–4 together.

**`internal/buildinfo` — trivial but worth pinning.**
Untagged: empty `Channel` → `IsDev() == false`; `Channel == "dev"` → true; any other value (`"prod"`, `"Dev"`, whitespace) → false, so the comparison stays exact rather than drifting to a prefix or case-insensitive match.
Plus `leaf_enforcement_test.go` with an empty allowlist.

**`internal/stencilstore` — one new exported symbol, one new test.**
Untagged, in-package: `ModeFor(true) == ModeDev` and `ModeFor(false) == ModeProduction`.
Two assertions, but they are the ones that keep the `buildinfo` split honest — `ModeProduction` being `iota`'s zero value (`stencilstore.go:142`) is what makes an unstamped binary safe, and nothing else in the tree pins that mapping once `stencilseed.go` stops doing the comparison itself.

**`tools/deploy` — a drift guard for the ldflags repoint.**
Untagged, in `tools/deploy`'s existing `main_test.go`: assert the `-X` string in `main.go` is exactly `-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev`, and that `internal/buildinfo`'s source declares an exported `Channel` at that path.
This exists because Go's linker silently ignores an unmatched `-X`: without a guard, a future rename of the variable or its package leaves a `-dev` build that behaves as production, with no build error, no test failure, and no visible symptom until someone notices stencils refreshing when they should not.
The same-commit rule stated in Gotchas is a review obligation, not a check — this test is what makes the pairing machine-enforced, and it is cheap because `tools/deploy` is already a package with its own test file.

**`internal/standalonestate` — TDD candidate.**
Untagged in-package tests (`package standalonestate`) calling the unexported `derive` seam directly — no `export_test.go`.
Scenarios: the Windows row produces `%LOCALAPPDATA%\lyx\<hash8>` and the POSIX row produces `$XDG_STATE_HOME/lyx/<hash8>`, both driven on any host via the seam; the `XDG_STATE_HOME`-unset fallback to `~/.local/state`; `LOCALAPPDATA` unset on Windows returning an error; both spellings of a case-differing path producing the *same* `hash8` under `goos == "windows"` and *different* hashes under `goos == "linux"`; a relative target being **rejected** with an error (and, since this is the whole point of the rejection, the same test asserting the result does not vary with the test process' cwd); `hash8` being exactly 8 lowercase hex characters; and stability — the same input yielding the same hash across calls, which is the property the whole resumability story rests on.
A symlink case needs a real filesystem, so it belongs in a small integration-tagged test (creating a symlink is a filesystem spawn, not a git one, but tagging keeps tier 1 clean).
Also assert `Derive` creates nothing on disk.
Plus `leaf_enforcement_test.go` with an empty allowlist.

**`cmd/lyx` — the defect test named in T5's verify line.**
Integration-tagged: build a plain git repository with no `_board` sibling, drive `stencilSeedTarget` against it, assert `ok == false`, then assert no `_board` directory was created beside the repo.
Then three positive cases against a `hubforge` fixture, all asserting `ok == true` with the returned `hub`/`worktree` matching the fixture's — these are the anti-regression rows for the `Wired`-vs-`HubPresent` split, and each one would fail if the gate were `Wired`:
cwd at an ordinary warp worktree; cwd at `<hub>/_board`; and cwd at a worktree whose paired sibling has been removed.
Without them the gate is trivially always-false and nothing catches the narrowing.
Separately, a `Wired`-vs-`HubPresent` divergence test in `internal/preflight` itself: at `<hub>/_board`, `HubPresent` is true and `Wired` is false — the one assertion that pins why both exist.

**Task-wide verify.**
`go test ./...` from the worktree root, plus the task's named check: `go test ./internal/loomengine/... ./internal/fabricengine/... ./internal/preflight/... ./internal/buildinfo/... ./internal/standalonestate/...`, and the integration-tagged runs for the same packages and `cmd/lyx`.
`go test ./...` is what picks up the two additions the named check omits — `./internal/stencilstore/...` and `./tools/deploy/...`.
`internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`) covers the `docs/overview.md` and `docs/shared-libs/README.md` edits, since it scans `manifest/` and `docs/` as link *sources*.
It does **not** cover links written inside the two new `CONSTRAINTS.md` invariant sections: the Markdown Link Integrity invariant states outright that `.md` files outside `manifest/`/`docs/` — `README.md`, `CLAUDE.md`, `internal/**/*.md`, and root docs such as `CONSTRAINTS.md` — "have their own outgoing links checked by nobody".
Links *pointing at* `CONSTRAINTS.md` anchors from a scanned file are checked; links *inside* the new sections are a review obligation.
The same applies to the three new `doc.go` files' own outgoing links.
`internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`) is the one that will actually fail on a vocabulary slip in the new packages — treat it as part of the task-wide verify, not an afterthought.

## Q&A log

- **Q:** Where do `CheckID`/`Failure`/`Report` live after the lift? **A:** [auto-pick] `internal/preflight` owns them; `loomengine` type-aliases and const-aliases. **Why:** aliases keep `loomengine.Report` and `preflight.Report` the identical type, so the 13 existing integration tests compile unedited and remain a real regression net.
- **Q:** What entry points does `internal/preflight` export? **A:** [auto-pick] `Check(cwd)`, `CheckResolved(l)`, and `Wired(cwd)`. **Why:** `Check` returning the resolved `*lyxcwd.Location` avoids a second `git rev-parse` per preflight; `CheckResolved` preserves the synthetic-`Location` test seam; `Wired` is the CLI-side predicate. *(Superseded by the round-2 gap below: a fourth entry point, `HubPresent(cwd)`, was added when `Wired` proved wrong for the seed gate. The three reasons above still hold unchanged.)*
- **Q:** How does `loomengine` learn "check 3 blocks the seed read" and "geometry short-circuited"? **A:** [auto-pick] derive both from the returned `Report` via a new exported `Report.Has(CheckID)`. **Why:** `Has(CheckFabricReady) || Has(CheckJunction)` is exactly today's `check3BlocksSeed` condition, so the derivation is equivalence-preserving and no orchestrator-specific field rides along on the shared type.
- **Q:** What exactly is the `seedStencils` gate — full tier 2, or something narrower? **A:** [auto-pick] narrower than full tier 2. **Why:** adding `Clean` would spawn `git status` before every `lyx` command *and* would stop seeding stencils in a dirty hub, a behaviour change nobody asked for; `Healthy` has the same problem and `PrimeName` spawns `git worktree list` for nothing. *(The "which narrow predicate" half of this answer — originally `fabricengine.Ready` — was superseded by the round-2 gap below, which found `Ready` regresses three real-hub cases. The gate is `HubPresent`.)*
- **Q:** Does `buildinfo` expose `StencilMode()` as the brief says? **A:** [auto-pick] no — `Channel` + `IsDev()`, with a new `stencilstore.ModeFor(dev bool)` holding the mapping. **Why:** `stencilstore.Mode` is non-stdlib, so returning it would break the stdlib-only leaf the same paragraph of the brief requires, and the leaf property is the load-bearing half.
- **Q:** What is `standalonestate`'s API, and does it create the directory? **A:** [auto-pick] one `Derive(target) (stateDir, hash8, err)`, pure derivation, no `MkdirAll`, over an injectable `derive(goos, env…, target)` seam. **Why:** `runtime.GOOS` is a compile-time constant, so without the seam exactly one of the two `<state>` table rows would ever be exercised in CI.
- **Q:** Which `CONSTRAINTS.md` entries land in this commit? **A:** [auto-pick] two new leaf invariants with mechanical enforcement tests; the three-tier rule is deferred. **Why:** T10's brief explicitly reserves the three-tier invariant and says writing it before T5 ships would pin a model the code does not implement; the leaf claims, by contrast, are only true if enforced.
- **Q:** Do the docs land in the same commit? **A:** [auto-pick] yes — `doc.go` per new package, `docs/overview.md` tree rows, `docs/shared-libs/README.md` entries. **Why:** the project's task-completion rule requires it for a task adding modules; `manifest/roadmap.md` is excluded because it moves per completed wave, not per task.
- **Q:** How is the plain-repo no-op tested, given `seedStencils` returns early under `testing.Testing()`? **A:** [auto-pick] extract `stencilSeedTarget(ctx) (hub, worktree, ok)` and drive that from an integration-tagged test. **Why:** a test against `seedStencils` itself would pass vacuously through the `testing.Testing()` guard and prove nothing about the gate.
- **Q:** [review r2 gap] `fabricengine.Ready` probes the *pair of this worktree*, not the hub — so gating the stencil seed on it would silently stop seeding in three real-hub cases (cwd at `<hub>/_board`, cwd at an unpaired sibling, cwd at a worktree whose pair was removed), all of which seed correctly today. Accept and document the narrowing, or gate on a hub-level probe? **A:** gate on a hub-level probe — split into `preflight.Wired` (tier 1 + `Ready`, the T7/T8 hub-mode trigger) and `preflight.HubPresent` (tier 1 + `<hub>/_board/_lyx` exists, the seed gate). **Why:** the task's mandate is to close the fictional-hub write, not to narrow a working path, and the honest precondition for a write to `<hub>/_board/_lyx/stencils` is that `<hub>/_board/_lyx` exists. One predicate cannot serve both: a hub-level probe is too weak for mode selection, since `(resolved, not wired)` in a real hub must select standalone per T8's brief.
- **Q:** [review r1 gap] `Derive`'s normalisation began with `filepath.Abs`, which resolves against the process cwd via `os.Getwd` — contradicting this discussion's own Constraints entry ("must not resolve a cwd at all") and making the "pure" `derive` seam host-dependent. Reject relative targets, or make the base directory a seam parameter? **A:** reject — `Derive` requires an absolute `target` and errors on a relative one; no `filepath.Abs` anywhere in the package. **Why:** it keeps `standalonestate` a pure function of its arguments and leaves the one legitimate cwd consultation at the CLI argument-parsing boundary that owns it; both T7 and T8 already hold an absolute path by the time they call, so the seam-parameter alternative would add a parameter for a case no caller has.
- **Q:** Do the tier-1/tier-2 test functions move from `loomengine` to `preflight`? **A:** [auto-pick] no — `loomengine`'s tests stay and are untouched; `preflight` gets additive new ones in an external `package preflight_test`. **Why:** those tests are the only proof the lift changed no behaviour, and `hubforge` imports `fabriccli`, so an in-package fixture test in `preflight` would close a compile cycle later.
