# Discussion: Rename hub container suffix from -HUB to -LYXHUB

```yaml
task: Rename hub container suffix from -HUB to -LYXHUB
slug: hub-suffix-lyxhub
status: discussing
parent: main
```

## Problem

A lyx hub container directory is currently named `<warp-name>-HUB` — e.g. `loomyard-HUB`, `lyx-test-HUB`.
The bare `-HUB` suffix says nothing about *which* tool owns the directory:
a sibling listing in `~/Code` cannot distinguish a lyx-initialized hub from any other directory someone chose to call `something-HUB`,
and `lyx fabric clone --reset` has already had one real defect (R4) rooted in a derived `<name>-HUB` path colliding with a directory that was never a hub.
Renaming the suffix to `-LYXHUB` makes a hub self-identifying at a glance.

Why now: the literal is still cheap to move.
It is declared in exactly two production constants and is never used to *discover* a hub (discovery is structural), so the blast radius is a value swap plus a large but mechanical sweep of test fixtures and docs.
The longer the sandbox suites, docs, and ~40 test files keep teaching `-HUB` as the hub vocabulary, the more expensive the sweep gets.

The existing `Hub` vocabulary — `HubPath`, `HubSuffix`, `HubReservedNames`, `HubScratchDir`, `docs/sandbox-hub.md`, the "Hub" noun in prose — is **not** being replaced.
Only the on-disk directory-name suffix changes.

## Scope

**In:**

- Swap the value of both sanctioned declarers of the literal, in the same commit:
  - `internal/lyxcwd/lyxcwd.go` — private `hubSuffix` const (line ~30).
  - `internal/fabricengine/junctionnames.go` — exported `HubSuffix` const (line ~110).
  - Update both constants' doc comments (`"loomyard" → "loomyard-HUB"` examples).
- `internal/lyxcwd/enforcement_test.go` — the policed-geometry-token registry:
  - `geometryToken`'s switch case `"-HUB"` → `"-LYXHUB"`.
  - `geometryTokenOwners` key `"-HUB"` → `"-LYXHUB"` (owner set `{internal/lyxcwd, internal/fabricengine}` unchanged), plus the explanatory comment above it that names the token.
  - The `const_HUB` AST-predicate positive fixture (`package p; const s = "-HUB"`).
- Test-fixture sweep across the ~40 files listed under **Technical context → File inventory**, applying the three-class rule in Decision *Test-literal classification*.
- Sandbox tooling — the fixture hub is a real cloned directory:
  - `tools/sandbox/main.go` — `hubName = "lyx-test-HUB"` → `"lyx-test-LYXHUB"`.
  - `tools/sandbox/suite.go` — the `warpDirName` comment naming `lyx-test-HUB`.
  - All seven `tools/sandbox/SANDBOX-*-SUITE.md` files (`lyx-test-HUB/lyx-test` paths, the `<name>-HUB` example in `SANDBOX-FABRIC-SUITE.md:305`, the build.cmd line in `SANDBOX-CORE-SUITE.md:19`).
- Prose/doc updates:
  - `docs/sandbox-hub.md` (7 hits — the canonical Hub-path doc).
  - `docs/sandbox-howto.md:70`, `docs/overview.md:452`.
  - `docs/shared-libs/lyxcwd.md:110` — the policed-token list in the `TestEnforcement_GeometryLiterals` description.
  - `manifest/designs/reed-fabric-standalone-api.md:320` — the measured public-surface identifier list showing `HubSuffix ("-HUB")`.
  - `internal/fabriccli/fabric.go:68,93` — the `lyx fabric clone` Cobra `Long` help text (`<parent>/<warp-name>-HUB`).
  - `internal/fabricengine/clone.go:70,594` and `internal/hubforge/hub.go:141` — doc comments describing the `<name>-HUB` container.
- `CONSTRAINTS.md` — add a short migration note under **Hub Containment Invariant** (or a new line) recording that `-LYXHUB` is the sole hub suffix and that pre-existing `-HUB` hubs are not migrated. See Decision *Clean break*.
- Verify the full suite passes (`go test ./...` with `CGO_ENABLED=1`) and the enforcement test still policies the new token.

**Out:**

- Renaming any `Hub*` identifier, type, field, file, or doc title. `HubSuffix` stays `HubSuffix`; `docs/sandbox-hub.md` keeps its name.
- Any dual-suffix parsing, compatibility shim, deprecation window, or migration command. See Decision *Clean break*.
- Physically renaming any hub directory on disk, in tooling or in a documented operator procedure. See Decision *No physical rename*.
- Changing `looksLikeHub`, `DeriveWarpName`, or any hub-discovery logic — none of them reads the suffix (see Technical context).
- The two sentinel string constants that merely *contain* `-HUB` as a substring, and the two recorded historical captures. See Decision *Test-literal classification*, classes (b) and (c).
- Collapsing the sanctioned `lyxcwd`/`fabricengine` duplication into one declarer — impossible (`lyxcwd` cannot import `fabricengine`) and out of scope.
- `manifest/roadmap.md` — this is a rename, not the completion or addition of a planned roadmap item.
- `_mill/status.md`'s own `-HUB` occurrences — mill task-state, not product source.

## Decisions

### Clean break — `-LYXHUB` is the only suffix, no dual-parse, no migration

- **Decision:** `-LYXHUB` fully replaces `-HUB`. No code anywhere parses, trims, or recognises `-HUB` after this task.
  A hub already on disk named `<name>-HUB` keeps working; only `lyxcwd.Location.RepoName` degrades (it becomes `"loomyard-HUB"` instead of `"loomyard"`).
- **Rationale:** hub discovery is entirely name-independent, verified during exploration:
  - `lyxcwd.resolveCore` derives `hubPath` as `filepath.Dir(workTreeRoot)` where `workTreeRoot` comes from `git rev-parse --show-toplevel` — the suffix is never matched.
  - `fabricengine.looksLikeHub` is structural: it checks for a `<hub>/_board` entry or at least one `*-weft` sibling directory. It never inspects the hub's own name.
  - The suffix is therefore used in exactly two directions: **construction** (`fabricengine.HubPath(parent, name)` for a *new* hub) and **`RepoName` derivation** (`strings.TrimSuffix(filepath.Base(hubPath), hubSuffix)` in `buildLocation`).
  `RepoName` is never used to construct a path — `internal/hubgeom/hubgeom.go:26` and `internal/fabricengine/warplayout.go:34` only carry it forward, and its sole consumer is `internal/tokenvocab`'s `"repo"` display token (reed header via `internal/reedengine/header.go`, and stencil token expansion).
  The degradation on an un-migrated hub is therefore cosmetic: a stale `{repo}` string in a header pane or a rendered stencil.
- **Rejected:** *dual-suffix parse* — trimming `-LYXHUB` then `-HUB` in `buildLocation` is one extra line and would keep `RepoName` correct for old hubs, but it permanently enshrines the exact literal this task exists to retire, including keeping a `"-HUB"` row in `geometryToken` and `geometryTokenOwners` forever. The benefit bought is a display string.
- **Rejected:** *migration command* — see next Decision.

### No physical rename — pre-existing hubs are left alone, never `mv`-ed

- **Decision:** neither the tooling nor the docs offer or recommend renaming an existing `<name>-HUB` directory to `<name>-LYXHUB`. The documented path for an operator who wants the new name is to re-create the hub (`lyx fabric clone`, or `--reset` for the sandbox), not to rename it.
- **Rationale:** a hub's wiring embeds its absolute path at creation time.
  `fabricengine.PortalLink`/`portalTarget` (`internal/fabricengine/portals.go`) and `LauncherDir` (`internal/fabricengine/launchers.go`) are both built from `l.HubPath`, and `fslink.CreateDirLink` materialises junctions/symlinks against those absolute paths.
  Renaming the hub directory invalidates every portal junction and launcher script inside it — the rename would *break* a working hub rather than migrate it.
  Additionally `reedengine.ServerName` derives the tmux `-L` socket key from `filepath.Base(hubPath)` plus a sha256 of the absolute path, so a rename silently orphans any running reed server for that hub.
- **Rejected:** a `lyx fabric migrate-hub` verb — it would have to rewrite every junction and launcher and reconcile the reed socket identity, which is a substantially larger task than the rename itself, for a cosmetic gain.

### Test-literal classification — three classes, only one changes

- **Decision:** every `-HUB` occurrence in a test file falls into exactly one of three classes:
  - **(a) Real-suffix literal → change to `-LYXHUB`.** The string stands for the hub suffix, whether it is a synthetic path fixture (`filepath.Join("home", "user", "repo-HUB")`), a table-test want value, or a socket-key expectation.
  - **(b) Coincidental substring → leave untouched.** The string contains `-HUB` but does not mean the suffix:
    - `internal/fabricengine/destructivegaps_integration_test.go:120` — `const sentinel = "OUTSIDE-PARENT-HUB-CONTENT"`.
    - `internal/fabricengine/clone_reset_guard_test.go:32` — `const sentinel = "NOT-A-HUB-USER-DATA"`.
    - `internal/reedcli/smoke_teardown_test.go:270` — the prose phrase "per-HUB" in a comment. Judgement call: this *does* reference the hub concept; rewrite the phrase to "per-hub" (lowercase prose) rather than "per-LYXHUB", since it names the concept, not the directory suffix.
  - **(c) Recorded historical capture → leave untouched.** Verbatim text captured from a real past run; editing it would falsify the record:
    - `internal/shuttleengine/claudeengine/startup_test.go:210` — a captured Claude-startup log line containing `/home/hanf/Code/r5sandbox/lyx-test-HUB/r5-crash`.
    - `docs/research/session-fork-spike.md:14` — a dated research note describing where a past live-session spike actually ran.
- **Rationale:** the brief flags this sorting explicitly as work the task owes ("some assert the real suffix value, some just use it as arbitrary test data — needs sorting out which").
  A mechanical global replace corrupts (b) and falsifies (c).
- **Rejected:** *replace every occurrence mechanically* — corrupts the two sentinels, whose whole point is to be an arbitrary recognisable string, and rewrites history in (c).
- **Rejected:** *change only the two production declarers and leave all fixtures* — it compiles and passes (class (a) fixtures are synthetic paths that never touch disk), but leaves ~38 files teaching the retired vocabulary to every future reader, which is exactly the rot this task exists to prevent.

### Sandbox fixture hub is renamed

- **Decision:** `tools/sandbox/main.go`'s `hubName` becomes `"lyx-test-LYXHUB"`, and all seven `SANDBOX-*-SUITE.md` files plus `docs/sandbox-hub.md`, `docs/sandbox-howto.md`, `docs/overview.md:452` are updated to match.
- **Rationale:** the sandbox exists to exercise the real `lyx fabric clone` path end to end against a real on-disk hub. A fixture whose name no longer matches what `clone` produces silently reduces the suite's fidelity, and `CONSTRAINTS.md`'s **Sandbox Suite Coverage** invariant makes the suite's accuracy a standing obligation.
  Sandbox hubs are disposable and explicitly re-cloned (`sandbox/build.cmd -reset`), so this costs the operator one re-clone and nothing else. Per Decision *No physical rename*, the operator re-clones; they do not `mv` the old sandbox hub.
- **Rejected:** leaving the sandbox hub at `-HUB` — the suite would then be the one place in the tree still producing the retired layout.

### Enforcement registry retires the `-HUB` row

- **Decision:** in `internal/lyxcwd/enforcement_test.go`, `-HUB` is replaced by `-LYXHUB` in `geometryToken`, in `geometryTokenOwners`, and in the `const_HUB` predicate positive fixture. No `-HUB` row survives.
- **Rationale:** follows directly from *Clean break* — with no code parsing `-HUB` anywhere, policing it would guard a token that no longer exists.
  Note the file's own standing warning: retired tokens (`_pattern`, `_raddle`) are documented as *deliberately absent* rather than left as dead rows, so retiring `-HUB` outright matches the file's established convention.
- **Rejected:** keeping both tokens policed — only justified under a dual-parse world, which *Clean break* rejected.

### Sanctioned duplication is preserved

- **Decision:** both declarers stay. `internal/lyxcwd` keeps its private `hubSuffix`; `internal/fabricengine` keeps the exported `HubSuffix`. Only the value and the doc-comment examples change, in one commit.
- **Rationale:** the duplication exists because `internal/lyxcwd` cannot import `internal/fabricengine` (`lyxcwd` is an entry-gate leaf below it), and it is registered as sanctioned in `geometryTokenOwners["-HUB"] = {"internal/lyxcwd", "internal/fabricengine"}` with an explanatory comment. Nothing about a value rename changes that relationship.
- **Rejected:** collapsing to one declarer — introduces an import cycle.

## Technical context

### The two declarers and the only two uses of the literal

- `internal/lyxcwd/lyxcwd.go:30` — `hubSuffix = "-HUB"`, private. Single use at `internal/lyxcwd/lyxcwd.go:179`, inside `buildLocation`: `RepoName: strings.TrimSuffix(filepath.Base(hubPath), hubSuffix)`.
- `internal/fabricengine/junctionnames.go:110` — `const HubSuffix = "-HUB"`, exported. Single production use at `internal/fabricengine/junctionnames.go:150`, inside `HubPath(parent, name)`: `filepath.Join(parent, name+HubSuffix)`.

There is no third production use of either constant. Everything else in the repo is a doc comment, a test fixture, or prose.

### Why hub discovery is unaffected

- `internal/lyxcwd/lyxcwd.go:142` — `hubPath := filepath.Dir(workTreeRoot)`. The hub is the parent of the git worktree root; its name is read, never matched.
- `internal/fabricengine/clone.go:645` — `looksLikeHub(hubPath)` checks `<hub>/_board` via `os.Stat`, then scans directory entries for one matching `WeftWarpSlug` (i.e. a `*-weft` sibling). Purely structural.
- `internal/fabricengine/clone.go:713` — `DeriveWarpName(rawURL)` extracts a repo basename from a URL and strips `.git`. It never appends or strips the hub suffix; the caller composes `HubPath(parent, DeriveWarpName(url))`.

### `RepoName` consumer map (established by grep, non-test)

| Site | Use |
|---|---|
| `internal/hubgeom/hubgeom.go:26` | copies `l.RepoName` into a geometry struct |
| `internal/fabricengine/warplayout.go:34` | copies `l.RepoName` into the warp layout |
| `internal/standalonegeom/reedgeom.go:57` | sets `RepoName` from `filepath.Base(target)` — a separate, raw derivation that never involved the suffix |
| `internal/tokenvocab/tokenvocab.go:25` | `{Name: "repo", Resolve: func(c Ctx) string { return c.RepoName }}` — the display token |
| `internal/reedengine/header.go:16` | passes `RepoName` into `tokenvocab.Ctx` for the reed header pane |

No path is ever constructed from `RepoName`. This is what makes *Clean break* safe.

### Secondary effect worth noting (not a blocker)

`internal/reedengine/server.go:55` — `ServerName(hubPath)` builds the tmux `-L` key as `"lyx-" + truncateAtRuneBoundary(socketSafeBase(filepath.Base(abs)), maxSocketSafeBaseBytes) + "-" + shortHash`, with `maxSocketSafeBaseBytes = 48`.
`-LYXHUB` is four bytes longer than `-HUB`, so a hub whose basename now lands between 45 and 48 bytes will truncate four characters earlier in the human-readable half of the socket key.
This is by design (the cap exists precisely to absorb long names, and the sha256 half preserves uniqueness) and needs no code change — but `internal/reedengine/server_test.go` has a length-boundary case at line 75 (`longBase := strings.Repeat("h", 200) + "-HUB"`) that should be reviewed for whether its intent survives the swap.

### File inventory (52 files; `_mill/status.md` excluded as mill task-state)

**Production source — value/comment change (class a):**
`internal/lyxcwd/lyxcwd.go`, `internal/fabricengine/junctionnames.go`, `internal/fabricengine/clone.go` (comments only), `internal/fabriccli/fabric.go` (Cobra help text), `internal/hubforge/hub.go` (comment only), `tools/sandbox/main.go`, `tools/sandbox/suite.go` (comment only).

**Test files — class (a), change:**
`cmd/lyx/constructoranchoring_test.go`, `cmd/lyx/notransients_test.go`,
`internal/fabricengine/{clone_reset_guard,fabric,hubscratch,junction,junctionnames,livestate_doc,livestate_verbs,origin,portallauncher,warpbinding_clone_integration,warplayout_fastpath,destructivegaps_integration}_test.go`,
`internal/hubgeom/{hubgeom,webstergeom}_test.go`,
`internal/logger/logsdir_test.go`,
`internal/loomengine/{config,discussionpath,friction,loomstatus,review}_test.go`,
`internal/lyxcwd/{enforcement,geometry,lyxcwd}_test.go`,
`internal/reedengine/{server,state}_test.go`,
`internal/tokenvocab/tokenvocab_test.go`,
`internal/weftname/weftname_test.go`.

Note: several of these reference the constant rather than the literal (`internal/fabricengine/clone_reset_guard_test.go` uses `"warp"+HubSuffix`; `internal/lyxcwd/lyxcwd_test.go:56` uses `fabricengine.HubSuffix`; `internal/fabricengine/junctionnames_test.go:145,151` use `+HubSuffix`).
Those update themselves and need only a comment/prose review — but the same files also carry hard literals (`junctionnames_test.go:116-117` has `"/repos/loomyard-HUB"`), so each file still needs individual inspection rather than a blanket skip.

**Test files — class (b) or (c), do NOT change the literal:**
`internal/fabricengine/destructivegaps_integration_test.go:120` (sentinel — but line 119 and 455 use `+fabricengine.HubSuffix` and are class (a), self-updating),
`internal/fabricengine/clone_reset_guard_test.go:32` (sentinel — other lines in the file are class (a)),
`internal/shuttleengine/claudeengine/startup_test.go:210` (recorded capture),
`internal/reedcli/smoke_teardown_test.go:270` (prose — lowercase to "per-hub").

**Docs/prose:**
`docs/overview.md:452`, `docs/sandbox-hub.md` (7 hits), `docs/sandbox-howto.md:70`, `docs/shared-libs/lyxcwd.md:110`, `manifest/designs/reed-fabric-standalone-api.md:320`,
`tools/sandbox/SANDBOX-{BURLER,CORE,FABRIC,REED,REED-WATCH,SHUTTLE,WEBSTER}-SUITE.md`.
`docs/research/session-fork-spike.md:14` is class (c) — leave.

## Constraints

From `CONSTRAINTS.md`:

- **Hub Containment Invariant** — no hub-level container is junctioned into a worktree. Unchanged by this task; the new constraint note about hub-suffix migration lands adjacent to it.
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`; no hub is hand-assembled. Any integration/smoke test that needs a real hub must keep going through `hubforge`, not construct a `-LYXHUB` directory by hand.
- **Sandbox Suite Coverage** — every registered lyx module is exercised by the sandbox suite or explicitly excluded with a reason. Motivates renaming the sandbox fixture hub rather than leaving it stale.
- **Test Tier Purity Invariant** — untagged tests perform no expensive spawns (no `gitexec.Run`, `exec.Command`, `gitkit.Copy*`, `hubforge.NewHub` outside `integration`/`smoke`-tagged files). The class-(a) sweep must not convert a synthetic-path fixture into one that touches disk.
- **Hermetic Git Test Environment Invariant** — any test package spawning git calls `gitkit.HermeticGitEnv()` in `TestMain`. No change expected, but relevant if a test is touched in an integration-tagged file.
- **Documentation Lifecycle** — a change to observable CLI behaviour updates its module doc, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for any new cross-cutting invariant, all in the same commit. The `lyx fabric clone` help text and the hub-path docs change here, so the doc updates are not optional follow-up.
- **Markdown Link Integrity** — the doc sweep must not break existing links.

From `CLAUDE.md`:

- **Markdown: semantic line breaks** — one sentence per line, no fixed-column hard-wrap, in every `.md` file touched.
- **Build prerequisite: cgo** — verification runs need `CGO_ENABLED=1` and a C compiler on `PATH`.
- **`manifest/roadmap.md` moves only on completing or adding a planned item** — this rename does not qualify.

Discovered during discussion:

- The `lyxcwd`/`fabricengine` duplication is sanctioned *by the enforcement test's owner map*, not merely by convention — changing one declarer without the other, or changing either without the map, fails `TestEnforcement_GeometryLiterals`. All three must move together.

## Testing

`TestEnforcement_GeometryLiterals` in `internal/lyxcwd/enforcement_test.go` is the natural **TDD candidate**, and the only one:
update `geometryToken`, `geometryTokenOwners`, and the `const_HUB` positive fixture to `-LYXHUB` *first*, observe the test fail because the two production constants still declare `-HUB` outside any registered owner for the new token (and because the old literal now sits unpoliced), then swap both constants and watch it pass.
That ordering makes the enforcement registry — not a grep — the thing that proves both declarers moved.

Scenarios that must be covered:

- **Enforcement registry.** `-LYXHUB` is policed with owner set `{internal/lyxcwd, internal/fabricengine}`; `-HUB` is no longer policed and no longer appears in the AST predicate fixtures.
- **`HubPath` construction.** `internal/fabricengine/junctionnames_test.go`'s `TestHubPath` — verify `HubPath(parent, name)` yields `<parent>/<name>-LYXHUB`. Keep at least one assertion pinned to the hard literal rather than `+HubSuffix`, so the test can actually fail if the constant changes again; the `+HubSuffix` cases alone are tautological.
- **`RepoName` derivation.** `internal/lyxcwd/lyxcwd_test.go` — `Location.RepoName` trims `-LYXHUB`.
  Add an explicit negative case asserting that a hub named `<name>-HUB` now yields `RepoName == "<name>-HUB"` (the documented, accepted degradation from Decision *Clean break*) — this pins the clean-break behaviour so a future reader cannot mistake it for a bug.
- **Structural discovery is name-blind.** Cover that `looksLikeHub` still accepts a directory named with the old suffix (it has `_board` / a `*-weft` sibling), so an existing hub is not refused. Check whether an existing `internal/fabricengine` test already asserts this; if so, extend rather than duplicate.
- **Socket-key boundary.** `internal/reedengine/server_test.go` — re-verify the truncation boundary cases (notably line 75's 200-char base, line 122's `base := "loomyard-HUB"`, and line 136's `want := "lyx-loomyard-HUB-"` prefix) still test what they were written to test with a four-byte-longer suffix.
- **Sentinel integrity.** The two sentinel constants and the recorded capture are unchanged after the sweep — worth a final explicit grep for `-HUB` at the end of implementation, whose only surviving hits should be the four class-(b)/(c) sites plus `_mill/status.md`.
- **Full suite green.** `go test ./...` with `CGO_ENABLED=1`. Sandbox suites are manual/operator-driven and are not part of the automated gate; their `.md` updates are doc changes, not test executions.

Per-module notes:

- `internal/loomengine`, `internal/logger`, `internal/hubgeom`, `internal/tokenvocab`, `internal/weftname`, `cmd/lyx` — these carry synthetic `HubPath` fixtures only. The sweep is mechanical and the tests should pass unchanged in behaviour; no new coverage is owed.
- `internal/fabricengine` — carries both real assertions and fixtures; needs the most careful per-file pass.
- `tools/sandbox` — no Go tests; the change is a constant plus documentation.

## Q&A log

- **Q:** Existing on-disk `-HUB` hubs — migrate, dual-parse, or clean break? **A:** [auto-pick] Clean break — `-LYXHUB` only, no migration and no dual-parse. **Why:** hub discovery is name-independent (`filepath.Dir(worktreeRoot)` + structural `looksLikeHub`), so old hubs keep working; the only degradation is `RepoName`, which is never used in path construction and only feeds the `{repo}` display token. Dual-parse would permanently preserve the literal this task exists to retire.
- **Q:** Which of the ~40 test files change? **A:** [auto-pick] Change only literals that stand for the real hub suffix; leave coincidental substrings and recorded historical captures alone. **Why:** the brief names this sorting as owed work, and a mechanical global replace would corrupt the two arbitrary sentinel constants and falsify two verbatim historical records.
- **Q:** Rename the sandbox fixture hub (`lyx-test-HUB`, a real cloned directory)? **A:** [auto-pick] Yes — rename in `tools/sandbox/main.go` and every SANDBOX-*-SUITE.md and doc. **Why:** the sandbox exercises the real clone path; a fixture name that no longer matches what `clone` produces silently reduces fidelity, and sandbox hubs are disposable (`build.cmd -reset`).
- **Q:** Keep the sanctioned `lyxcwd.hubSuffix` / `fabricengine.HubSuffix` duplication? **A:** [auto-pick] Yes — keep both declarers, swap the value in both in the same commit. **Why:** `lyxcwd` cannot import `fabricengine`; the duplication is registered in `geometryTokenOwners` and is orthogonal to a value rename.
- **Q:** What happens to the enforcement test's policed-token registry? **A:** [auto-pick] Retire the `-HUB` row entirely, replace with `-LYXHUB`. **Why:** follows from the clean break — with nothing parsing `-HUB`, policing it would guard a token that no longer exists; the file's own convention is to drop retired tokens (`_pattern`, `_raddle`) rather than leave dead rows.
- **Q:** Should the task offer a physical-rename migration path for existing hubs? **A:** [auto-pick] No — never `mv`, re-create instead. **Why:** portal junctions and launcher scripts embed the hub's absolute path at creation (`portals.go`, `launchers.go`, via `fslink.CreateDirLink`), and `reedengine.ServerName` derives the tmux socket key from the hub basename plus a hash of its absolute path — a rename breaks a working hub rather than migrating it.
- **Q:** Does the four-byte-longer suffix affect the tmux socket-key cap? **A:** [auto-pick] No code change; review the affected tests. **Why:** `maxSocketSafeBaseBytes = 48` exists precisely to absorb long hub names and the sha256 half preserves uniqueness, but `internal/reedengine/server_test.go`'s boundary cases were written against a four-byte-shorter suffix and must be re-checked for intent.

### Open — scope extension

The brief states: *"The operator may extend this task's scope with a few more small polish items during mill-start — do not assume this rename is the whole task once discussion starts."*
This run is `--orch`, so no operator was present in Phase: Discuss to supply those items.
Any additional polish items should be raised in the orchestrator-authored discussion-review round 1 and folded into this file as BLOCKING findings.
As written, this file covers the rename and nothing beyond it.
