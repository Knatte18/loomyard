# Discussion: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
slug: dotlyx-scratch-hygiene
status: discussing
parent: main
```

## Problem

Loomyard has two lyx directories and the split is structural: `_lyx` is durable, weft-synced, **tracked** content, while `.lyx` is ephemeral, machine-bound, **never-tracked** scratch.
The rule the operator states plainly: **every file that must not survive a move between machines — that is, every file that SHALL stay on one machine — lives under `.lyx`.**

Today the rule is violated in two directions.
Several modules write never-tracked transients into `_lyx` (perch/treadle's run and state locks and its pause flag, webster's pause flag, rendered fork prompts and three lock files, builder's pause flag and three lock files, loom's `status.json.lock`), which is the entire reason `internal/fabricengine` carries the `crossModuleMachineLocalExcludes` machinery at all — a wildcard-pattern exclusion layer that exists purely to stop misplaced files from being committed.
And `.lyx` itself is kept out of the host repo by a **committed** `.gitignore` block (`gitignore.Ensure(l.AnchorPath(), ".lyx/")`, `internal/fabriccli/clone.go:81`), which is wrong on principle: a tracked `.gitignore` entry in the *user's own* repo advertises that LYX is in use, and a host→weft junction must never leave a tracked artifact behind in the user's repo.

**Why now:** slice 7 (`6a93049c`) shrank `internal/hubgeometry` out of existence and moved junction geometry into `internal/fabricengine`, and `fabric-collapse-external-surface` (`63218a66`) removed the per-call `:(exclude)` pathspec magic and `git add -f`, installing `seedWeftArtifactExcludes` as an interim guard.
Both preconditions this task waited on are now on `main`.
This task removes the *need* for that interim guard by putting the files where they belong, and is **sequenced before slice 10** (`fabric-warp-binding-in-weft`) because both edit `internal/fabriccli/clone.go`'s `runCloneWithReset` in the same ~45-line span.

## Scope

**In:**

- A new zero-dependency leaf package `internal/lyxdirs` owning both directory-name tokens: `LyxDirName` (`_lyx`, moved out of `internal/configengine`) and `DotLyxDirName` (`.lyx`, replacing five private `dotLyxDirName` consts), plus amending every leaf-invariant allowlist and CONSTRAINTS clause that must now permit importing it.
- Making both `_lyx` and `.lyx` **structural** directories injected by `internal/fabricengine` in code, so neither is read from `fabric.yaml`'s configurable `pathspec` any more (the field survives for genuinely optional directories such as `_pattern`).
- Re-anchoring `.lyx` onto `AnchorPath()` so it is a directory sibling of `_lyx`, and updating the existing `WorktreePath()`-anchored consumers.
- Relocating **every** never-tracked transient currently written under `_lyx` into the mirrored subpath under `.lyx` (full inventory in Technical context).
- An explicit scratch-dir seam on every module holding relocated transients (`treadleengine`'s engine input, plus a `ScratchDir(l)` accessor and re-keyed transient accessors on webster, builder, perch and loom) so the transients leave the durable dir while each engine stays geometry-blind.
- Removing `internal/logger`'s persistent durable-sink file handle (open-append-close per record).
- Wiring `.lyx` as a weft-backed junction, replacing the committed `.gitignore` block in `clone.go`/`unwire.go` with the warp `.git/info/exclude` seeding every other junction already uses, plus a `.lyx`-only content-adoption path for worktrees that already hold a real `.lyx` directory.
- Removing `Unwire`'s deletion of the weft-side `_lyx` content, so unwire only reverses wiring and never destroys weft content.
- Splitting `fabricengine`'s never-committed directory set from its committed one, so `.lyx` never reaches a pathspec or weft commit routing, plus `--exclude-standard` on `weftPathspecFilter`'s `git ls-files` probe.
- Replacing the three `crossModuleMachineLocalExcludes` patterns with a single `.lyx/` entry in the weft repo's `.git/info/exclude`, seeded at wiring time, and deleting the `crossModuleMachineLocalExcludes` var.
- Recognising `<hub>/.lyx` as a hub-level geometry element owned by fabric, the way `<hub>/_board` already is.
- Two enforcement tests (single-declarer AST rule for the two directory tokens; a runtime no-transients-under-`_lyx` test).
- Docs in the same commit: `CONSTRAINTS.md`, `docs/overview.md`, the affected package docs, `manifest/designs/fabric-unified-view.md` (slice 9 status **and** the as-built anchoring table at `:60-64`), `docs/shared-libs/README.md:35`, `tools/sandbox/SANDBOX-BUILDER-SUITE.md`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` and `docs/reference/builder-contract.md`.

**Out:**

- Content adoption for any junction name other than `.lyx`.
  `_lyx` and `_pattern` keep today's hard refusal on a pre-existing real directory.
- A cleanup path for the committed `.gitignore` `.lyx/` block in repos cloned by a pre-fix binary.
  Both the `Ensure` and the `Remove` call sites are deleted outright, so no code path removes a leftover block;
  the only known affected repo is the sandbox, which is re-cloned anyway.
  The manual remedy (delete the `.lyx/` line from the lyx-managed block) is documented.
- Untracking transients a pre-fix `sync` already committed into weft history.
  They stay committed; `git rm --cached <path>` is documented as the manual remedy, not automated.
- Value-level migration of `fabric.yaml` (see the `structural-dirs-are-not-config` decision's rejected alternatives).
  `lyx config reconcile` already propagates new template **keys**; changing the **value** of an existing key is deliberately out of its contract, and the structural decision removes the need.
- Splitting an over-cap trace file into several files instead of truncating it.
  The durable sink loses its persistent handle here, but keeps today's truncate-at-`sinkMaxBytes` behaviour;
  rollover to numbered parts is a wanted follow-up task, and it also requires extending `retention.go:26`'s `traceFilePattern`, since non-matching files are never swept.
- Moving the weft-side `_lyx` deletion into the merge path.
  Unwire stops deleting it here; where (and whether) a merge step should delete it is a separate task.
- Moving `.weft/` or `gitrepo.PushLockFileName` out of `seedWeftArtifactExcludes` — those are fabric's own runtime artifacts, not module transients.
- Removing `fabric.yaml`'s `pathspec` field entirely — `_pattern` is genuinely optional per repo, so the field survives with a narrower job.
- `_raddle`, `_pattern` and `_board` content sweeps — they hold no known transients today.
  `internal/fabricengine`'s own `board.push.lock` (`bolt.go:34`, under `<hub>/_board`) and the correspondence index (`index.go:75`, inside the git dir) are fabric-internal and stay put.
- Supporting downgrade to a pre-fix binary (see `structural-dirs-are-not-config`).
- This repository's own hand-written `.gitignore` `.lyx/` block — loomyard is not fabric-wired, so its `.lyx` is a plain directory and the entry is correct as it stands.

## Decisions

### dotlyx-is-a-weft-backed-junction

- Decision: `.lyx` lives physically inside the weft worktree at `<weft>/<AnchorRel>/.lyx` and is exposed in the warp/host worktree as a directory junction created by fabric through `internal/fslink`, wired and unwired by exactly the same primitives as `_lyx` and `_pattern`.
  The committed `.gitignore` `.lyx/` block is removed from both `clone.go` and `unwire.go`; the junction name is excluded through the warp's `.git/info/exclude` via the existing `seedGitExclude` path.
- Rationale: it is the design doc's stated target (slice 9, `manifest/designs/fabric-unified-view.md`) and it needs no bespoke code — `.lyx` inherits wire/unwire/reconcile/health/drift handling for free, and one mechanism covers every host→weft directory instead of two.
  The committed `.gitignore` in the user's own repo is the specific thing being eliminated: it makes LYX visible in a repo LYX does not own.
- Rejected: **plain real `.lyx` directory in the host worktree, excluded only through the warp's `.git/info/exclude`.**
  This also achieves "no tracked artifact in the user's repo" and avoids placing machine-local state inside a git worktree, but it needs a standalone exclude-seeding call site (a third shape alongside `_lyx`'s junction and `_board`'s hub link), and `.lyx` then has no lifecycle at all — nothing wires it, nothing reports it, nothing tears it down with the pair.
  Also rejected: anchoring `.lyx` at the hub instead of per worktree (a far larger relocation of every module's current path, destroying per-worktree isolation of machine-local state).
- Consequence to keep in view: machine-local scratch now sits inside a git worktree, so every operation that touches the weft worktree must be checked against it — which is why the unwire decision below exists and why the weft-side exclude must be seeded before the first write.

### structural-dirs-are-not-config

- Decision: `_lyx` and `.lyx` are **structural** directories injected by `internal/fabricengine` in code, not entries in `fabric.yaml`'s `pathspec`.
  `fabricengine` declares two sets: `structuralCommittedDirs` (today exactly `lyxdirs.LyxDirName`) and `structuralNeverCommittedDirs` (today exactly `lyxdirs.DotLyxDirName`).
  The `pathspec` field survives for genuinely optional directories only; `template.yaml`'s default shrinks from `_lyx _pattern` to `_pattern`.
- Rationale: CONSTRAINTS states *geometry is structural, never config/env-overridable*, and both directories must always exist — every lyx module fails without them.
  A `fabric.yaml` that drops `_lyx` from `pathspec` would tear away the entire durable tree, and one that omits `.lyx` would leave machine-local scratch unwired.
  Making one configurable and the other not would be incoherent.
  This also removes the rollout problem: an already-deployed `fabric.yaml` needs no migration, because neither name is read from it any more.
- Rejected: adding `.lyx` to the yaml `pathspec` and changing `template.yaml`'s default (leaves obligatory geometry switchable per repo, and every existing repo needs a value-level migration `lyx config reconcile` deliberately does not do);
  code-injecting `.lyx` into the loaded pathspec only when missing (same effect while pretending yaml owns it);
  **listing every junction — structural ones included — in one yaml list** (tidier and more uniform on its face, and the operator's own first instinct, but it makes mandatory geometry look optional: a list you can edit is a list you can empty, and `_lyx`/`.lyx` must simply be there);
  removing `pathspec` altogether (`_pattern` really is per-repo optional).
- **Deployed configs still parse.**
  An existing `fabric.yaml` in `weft:main` keeps `pathspec: _lyx _pattern`, and `Config.Dirs()` (`config.go:28`, `strings.Fields`) still parses it — so `_lyx` arrives from **two** sources.
  Every set below is therefore a **deduplicated** union, not a concatenation;
  without dedup, duplicate names reach `HostJunctions`, `ScopedPathspec` and status output.
- **Set composition, exactly** (each a dedup-preserving union in the stated order — structural names first, then config names in `Dirs()` order, first occurrence wins):
  - Wired junction name-set (`WiredNames`/`RepoWiredNames`) = `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())`.
    This is what gets junctions and warp `.git/info/exclude` entries.
  - Pathspec / commit-routing set = `structuralCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())` — **never** `structuralNeverCommittedDirs`.
    This is what `internal/fabriccli/weft_verbs.go:100` builds `ScopedPathspec` from and what `classifyPaths` routes on.
  - Slug-reservation union (`Topology.Add`, `IsReservedHubName`) = both structural sets ∪ raw `cfg.Dirs()` ∪ the hub-reserved names.
    No filtering here: a worktree slug must be refused for every one of these names.
- **Downgrade is unsupported, one-way upgrade only.**
  A pre-fix binary's `applyStaleRemoval` (`reconcile.go:391`) removes on-disk junctions absent from *its* `RepoWiredNames`, so running an older `lyx fabric reconcile` after this change unwires `.lyx` and strands scratch inside the weft worktree.
  Record this in the module doc; do not attempt to make the change downgrade-safe.

### never-committed-is-structural-not-configurable

- Decision: membership in `structuralNeverCommittedDirs` is what makes `.lyx` uncommittable.
  The filtering lives where the pathspec is **constructed** — `ScopedPathspec`'s callers — and never inside `Config.Dirs()`, `WiredNames`, or the slug-reservation union, all three of which must keep seeing every name.
  Separately, `weftPathspecFilter`'s `git ls-files --cached --others` probe gains `--exclude-standard`.
- **`classifyPaths` needs a third bucket — omission is not enough, it is actively worse.**
  `classifyPaths` (`classify.go:14-26`) is a strict two-way split: a path under a wired prefix goes to weft, **everything else falls through to warp**, and `Commit` (`commit.go:96`) hands `warpFiles` straight to a warp `StageAndCommit`, where `commitBothSides` treats a warp failure as hard.
  So merely dropping `.lyx` from the routing set would send a stray `.lyx` path into the **user's own repo** — the one place it must never go — and fail there on the same exit-1 ignored-path error, taking the whole commit with it.
  `classifyPaths` therefore gains a third return value for paths under a `structuralNeverCommittedDirs` prefix, and `Commit` turns a non-empty third bucket into a hard error naming the offending path.
  Classification stays pure and I/O-free; the policy lives in `Commit`.
  Silent dropping stays rejected — a caller passing a `.lyx` path is a bug and must be told.
- Rationale: the two halves fix different failures.
  Keeping `.lyx` out of the pathspec means it is never even *named* in a git invocation — necessary because `git add` on an ignored path **fails the entire invocation with exit 1** and stages nothing, not even the legitimate `_lyx` files named in the same call (verified empirically, see Technical context).
  `--exclude-standard` fixes a separate latent bug: `git ls-files --cached --others -- <entry>` currently *matches* ignored files, so any stray ignored file matching any pathspec entry makes `weftPathspecFilter` forward a doomed entry and topple a whole commit.
  With the flag, such an entry is filtered out and the commit is a clean no-op.
  Encoding "never committed" as a fabric.yaml field is forbidden by the same CONSTRAINTS clause as above.
- Rejected: relying on git's own refusal alone (the guarantee is absolute but the failure is a cryptic hard error that blocks unrelated content);
  silently dropping `.lyx` paths at commit time (hides caller bugs);
  filtering inside `Dirs()` or `WiredNames` (would silently undo the junction wiring and the slug reservation).

### unwire-never-deletes-weft-content

- Decision: `Unwire` stops deleting the weft-side `_lyx` content.
  It reverses wiring only — removes host junctions and their warp `.git/info/exclude` entries — and leaves every weft-side directory intact, `_lyx` and `.lyx` alike.
  The `os.RemoveAll(weftLyxDir)` call and the follow-on `"lyx fabric unwire: clear _lyx"` commit and push (`unwire.go:93-110`) are removed.
  **Result vocabulary after the removals**, both CLI-observable JSON keys (`fabriccli/unwire.go:26-31`):
  `WeftContent` loses `"cleared"` and its value set becomes `"preserved"` | `"not_present"`;
  the `Gitignore` field and its `gitignore` output key are **dropped entirely**, since `gitignore.Remove` (`unwire.go:113`) goes with them and no `.gitignore` interaction remains to report.
  Retaining the key as a constant `"unchanged"` would report on a mechanism that no longer exists.
  This is an intentional output-envelope change and is recorded in the module doc;
  the envelope invariant governs *using* `output.Ok`/`output.Err`, which is unaffected.
  The weft-side `.lyx` is never touched by unwire either; it disappears with the weft worktree when `Remove` tears the pair down.
  `Remove` gets no new contract for that case: its `git worktree remove --force` deletes whatever is under the weft worktree, and on Windows an open handle inside `.lyx` makes the removal fail with an OS error that surfaces as-is.
  The remedy is the same as adoption's — stop the daemons and re-run — and it is stated in the docs rather than special-cased in code, because `Remove` is an explicit whole-pair teardown the operator asked for.
- Rationale: "unwire" means the inverse of "wire" — remove the junction coupling — not "delete the source directory".
  The current behaviour is already self-inconsistent: `unwire.go:30` records that weft `_pattern` content is *preserved by design* while `_lyx` is deleted and the deletion committed, with no stated reason for the asymmetry, and the doc comment simultaneously promises that "a later `lyx fabric reconcile` re-wire can recreate this worktree's wiring" — true of the wiring, false of the content it destroyed.
  Once `.lyx` lives in the weft worktree the stakes rise sharply: `_lyx` deletions are at least recoverable from git history, whereas `.lyx` is never committed, so any deletion of it is final.
  Fixing it here rather than later avoids giving the new `.lyx` contract an inconsistency we have already named, and this task edits `unwire.go` regardless.
- Rejected: deleting `.lyx` symmetrically with `_lyx` (simplest to explain, but destroys live daemon state in an otherwise reversible operation);
  leaving `_lyx` deletion alone and defining only `.lyx` behaviour (the incoherence would survive at least through slice 10);
  moving the deletion into the merge path in this task (the operator's view is that weft `_lyx` should be cleared before a branch merges, but that is a new step in the merge sequence, not a removal, and belongs to its own task).
- Test consequence: existing unwire tests asserting the clear-and-commit behaviour are rewritten to assert preservation.

### dotlyx-and-lyx-are-directory-siblings

- Decision: `.lyx` is anchored at `AnchorPath()`, exactly like `_lyx` — the two are directory siblings.
  The consumers currently anchored at `l.WorktreePath()` re-anchor as part of this slice: `logger/sink.go:37`, `shuttleengine/rundir.go:51` and `run.go:185`, `scoutengine/daemonstate.go:43,50`, `burlerengine/engine.go:106`, and every `reedengine` state site (`spawn.go:143,183`, `strand.go:309,348,471`, `lifecycle.go:545,676,778,961`, `lock.go:66`).
- Rationale: the junction lands at `<worktree>/<AnchorRel>/.lyx` (`HostJunctions`, `junction.go:61-62`) and `_lyx` is `AnchorPath()`-anchored (`builderengine/state.go:33`, `perchengine/identity.go:33`).
  Leaving those sites on `WorktreePath()` gives a subpath-anchored repo (`AnchorRel != "."`) two distinct `.lyx` roots — one junctioned and excluded, one not — which is exactly the class of bug this task exists to remove.
- Rejected: moving the junction to `WorktreePath()` (makes `.lyx` the only junction not `AnchorRel`-anchored);
  leaving those sites alone (two legal `.lyx` roots).
- Note: `reedengine.HubLogsDir` (`lifecycle.go:38`) is hub-anchored and deliberately unaffected — see the hub-level decision below.
- **Scout is not a one-line edit and must not be treated as one.**
  `DaemonStateFile`/`DaemonLock` (`daemonstate.go:42,49`) take a plain `worktreePath string`; the anchor is chosen in `internal/scoutcli` by `resolveWorktreeRoot` (`cli.go:474-477`, called at `cli.go:151,297,418`) and threaded through `scoutengine.Options.WorktreeRoot` into `ensureSupervised` (`ensureserver.go:298-300`).
  Editing `daemonstate.go:43,50` alone changes nothing, and scoutengine cannot derive `AnchorPath()` itself — its leaf allowlist excludes `lyxcwd`.
  **Decision: thread a separate anchor value, do not re-purpose `WorktreeRoot`.**
  `WorktreeRoot` also keys the daemon singleton and the LSP root, so overloading it would change daemon identity as a side effect of a path fix.
  `scoutcli` (which may import `lyxcwd`) computes the anchor path alongside `resolveWorktreeRoot` and passes it in a new `Options` field; `DaemonStateFile`/`DaemonLock` take that value.
- **Logger's exported accessor is renamed:** `logger.WorktreeLogsDir` becomes `logger.LogsDir`, since the name would otherwise assert an anchor it no longer uses.
  Call sites: the definition (`sink.go:36`) and its doc comment, `sink.go:97`, `internal/logger/worktreelogs_test.go`, and `cmd/lyx/constructoranchoring_test.go`.
  `header.WorktreeRoot` (`sink.go:45,98,147`) is a separate concern — it records the worktree root as trace metadata and keeps both its name and its `WorktreePath()` value.
- **Shuttle's config branch re-anchors too.**
  `runDirRoot` (`rundir.go:49-57`) has two bases: the `.lyx/shuttle` default at `:51` and the relative-`cfg.RunDir` branch at `:56`.
  Both move to `AnchorPath()`, so one function never resolves against two different bases when `AnchorRel != "."`.
  An absolute `cfg.RunDir` stays verbatim, unchanged.

### dotlyx-content-adoption-no-other-migration

- Decision: when wiring finds a **real directory** (`fslink.IsLink(link) == false`) at the `.lyx` junction path, fabric moves its contents into the weft-side target and replaces it with the junction.
  This adoption is **`.lyx`-only**: `_lyx`, `_pattern` and every other junction name keep today's hard refusal in `seedLyxJunction:163`.
  **Collision rule: refuse.**
  If any entry being moved already exists in the weft-side target, adoption aborts with an error naming the colliding path and leaves both sides untouched — fabric never overwrites or deletes content.
  A collision means an earlier adoption already ran, and `.lyx` is disposable enough that the operator can delete the host-side copy.
  **Precondition: no live process may hold the directory.**
  On Windows — the platform junctions exist for — moving a directory with an open handle inside it fails.
  Adoption must surface that failure as an actionable error ("stop reed/scout, then re-run `lyx fabric reconcile`"), never a partial move.
  That remedy only works for *external* holders, which is why the adopting process's own holder is removed structurally instead — see `logger-sink-holds-no-persistent-handle`.
  Without that, `lyx fabric reconcile` would hold a trace file open inside the very directory it is moving, adoption would always fail on Windows, and the upgrade cliff adoption exists to remove would simply return under a different error message.
  No other migration exists — transients already committed into weft history stay committed, with `git rm --cached` documented as the manual remedy.
- Rationale: every worktree in existence today has a real `.lyx` (logger, reed, shuttle, scout and burler write it unconditionally), so without adoption the first `reconcile` after this change hard-errors everywhere, and the only remedy would be deleting live reed/scout daemon state.
  Content under `.lyx` is always lyx's own machine-local scratch, so the guard's rationale — never touch what might be the user's hand-authored content — does not apply there.
  It does apply to `_lyx` and `_pattern`, where the refusal stays.
- Rejected: hard-erroring on `.lyx` and documenting a manual stop-daemons-then-delete step (an upgrade cliff on every existing worktree, destroying live state);
  deleting the real directory silently (same destruction, without warning);
  extending adoption to every junction name (fabric would start moving user content unasked);
  letting the weft-side copy win on collision (silently discards data fabric was asked to preserve).

### weft-side-exclusion-via-git-info-exclude

- Decision: the weft repo keeps `.lyx` untracked through a single `.lyx/` line in its `.git/info/exclude`.
  `seedWeftArtifactExcludes` remains the **sole owner** of that file's content;
  `WireJunctions` calls it for the weft side at wiring time, and `ensureWeftLockDir` keeps calling it as the self-healing path.
  The three `crossModuleMachineLocalExcludes` patterns are deleted and the var removed.
  No `.gitignore` is committed in weft either.
- Rationale: the warp rule ("never a committed `.gitignore`") is about not exposing LYX in the user's repo;
  weft is LYX's own repo, so the choice there is purely practical, and `.git/info/exclude` wins — no commit, no pathspec change, no new file in the weft root, and one `.lyx/` pattern replaces three deep wildcard patterns.
  Seeding at wiring time closes the ordering hole: wiring already materialises the weft-side target (`junction.go:118`), so the exclude entry is guaranteed to exist before anything writes into `.lyx`.
  Seeding only from `ensureWeftLockDir` would leave the window between wiring and the first weft-git verb open, during which scratch shows as untracked dirt and trips `Remove`'s no-force dirty gate.
  Keeping `ensureWeftLockDir` as a second *call site* of the same single owner preserves self-healing on machines that never re-wire.
- Rejected: a committed `.gitignore` at the weft root (needs a new staging path, since the weft root sits outside every caller's pathspec);
  seeding only from `ensureWeftLockDir` (ordering hole);
  seeding only from wiring (loses self-healing);
  keeping the three deep wildcard patterns (the machinery this task exists to delete).

### lyxdirs-owns-both-directory-tokens

- Decision: a new leaf package `internal/lyxdirs` (stdlib-only, zero internal imports) declares `LyxDirName = "_lyx"` and `DotLyxDirName = ".lyx"`.
  `configengine.LyxDirName` is moved there (≈115 references across 19 non-test files plus tests — a mechanical identifier rename), and the five private `dotLyxDirName` consts (`shuttleengine/rundir.go:28`, `scoutengine/daemonstate.go:31`, `reedengine/lifecycle.go:30`, `burlerengine/engine.go:29`, `logger/sink.go:27`) are deleted in favour of it.
  **Exact-literal inventory** (`"_lyx"` / `".lyx"` as whole string literals, non-test files — 11 occurrences in 10 files):
  six are real declarations — `configengine/config.go:24` plus the five private consts above;
  two are real code that must convert to `lyxdirs.LyxDirName` — `fabricengine/status.go:181` (a `git ls-files` argument) and `status.go:207` (a tracked-path prefix check);
  the remaining three are doc-comment prose (`lyxtest/lyxtest.go:228`, `fabricengine/fabric.go:124`, `fabricengine/junction.go:45`) and change only if the comment is reworded.
- Rationale: the two names are one structural pair — tracked versus never-tracked — and splitting their ownership across two packages is worse than either extreme.
  `configengine` owns `_lyx` only because config files happen to live under it, which is inherited, not principled.
  `fabricengine` cannot own the token: `internal/logger` needs it, and `fabricengine → logger → lyxcwd` must stay acyclic.
  `internal/lyxcwd` cannot own it either — the Cwd Resolution Invariant forbids it from exposing junction paths or per-module subdirectories, and `.lyx` is now a junction name.
  A zero-dependency leaf is importable by every one of them.
  `shuttleengine/rundir.go:27` already records the promise: *"It stays unpoliced this slice; slice 9 registers a single owner."*
- Rejected: `configengine.DotLyxDirName` (small diff, but cements an ownership we just established is wrong);
  putting only `.lyx` in a new package (splits the pair);
  leaving five private declarers (leaves the promise unkept and five sites to drift).
- **Blocking consequence: leaf allowlists must be amended in the same change.**
  `internal/scoutengine/leaf_enforcement_test.go:22-28` allows exactly `{configengine, lock, proc, logger, yaml}`, so `daemonstate.go` importing `lyxdirs` fails `TestLeafInvariant_AllowlistOnly` on day one, and CONSTRAINTS' Scoutengine Leaf Invariant names the same set in prose.
  The plan must sweep every enforcement test and matching CONSTRAINTS clause and add `internal/lyxdirs` where the package now needs it: `internal/scoutengine/leaf_enforcement_test.go` (confirmed), plus `internal/githubclient/leaf_enforcement_test.go`, `internal/lyxtest/leaf_enforcement_test.go`, `internal/treadleengine/seam_enforcement_test.go`, `internal/shuttleengine/seam_enforcement_test.go`, `internal/lyxcwd/enforcement_test.go` and `cmd/lyx/constructoranchoring_test.go` — each checked, amended only where that package actually imports `lyxdirs`.

### explicit-scratch-dir-seam-per-module

- Decision: every module holding relocated transients gains an explicit scratch directory alongside its durable one, and is *told* the path rather than deriving it.
  - `internal/treadleengine`'s engine gains an explicit scratch-directory input, defaulting to the existing `runDir` when unset.
    `run.lock`, `state.json.lock` and the `pause` flag are written there; `state.json` and round artifacts stay in `runDir`.
    `internal/perchcli` supplies `<AnchorPath>/.lyx/perch/<block>` while `runDir` stays `_lyx/perch/<block>`.
  - `websterengine`, `builderengine` and `perchengine` each gain a `ScratchDir(l)` accessor as the sibling of their existing `Dir(l)`/`RunsDir(l)`, resolving to the mirrored `.lyx` subpath.
    `loomengine` has no `Dir(l)` to mirror — its `status.json`/`status.json.lock` sit at the `_lyx` root (`config.go:71-80`) — so it gets an explicit `StatusLockPath(l)` resolving to `<AnchorPath>/.lyx/status.json.lock`, stated rather than derived by analogy.
  - Accessors that name **only** a transient keep their `(dir string)` shape and simply receive the scratch dir instead: `PauseFlagPath`, `ClearPause`, `AcquireStateMutation`, `websterengine.PromptsDir`, `perchengine.PauseFlagPath`.
  - Accessors that must straddle **both** trees take a second parameter, because one directory argument cannot express the split.
    `websterengine.LoadState`/`SaveState` (`state.go:193-217`) and `builderengine.LoadState`/`SaveState` (`state.go:133-156`) each derive `state.json` *and* `state.json.lock` from a single `dir`, while this task keeps `state.json` in `_lyx` and moves the lock to `.lyx`.
    All four gain a scratch-dir parameter, with the lock path resolved as `filepath.Join(scratchDir, stateFileName+".lock")`.
  - Webster's and builder's engine-internal calls pass the durable dir today and must be re-keyed too — `websterengine/runlevel.go:347,423,616,791` and `builderengine/runlevel.go:340,445,525` call `AcquireStateMutation`/`ClearPause` with `deps.WebsterDir`/`deps.BuilderDir`, and `RunDeps` (`runlevel.go:99-111`) carries no scratch field.
    The deps structs gain one, and every deps-constructing site supplies it: `webstercli/run.go:69`, `websterengine/beginbatch.go:104`, the buildercli equivalents, and their tests.
  - Every out-of-engine call site is updated in the same change: `internal/perchcli/pause.go:88`, `internal/perchcli/run.go:291`, and the webster/builder CLI pause and state-mutation sites.
- Rationale: it keeps each engine geometry-blind — told, never deriving — which is the exact constraint `internal/perchcli/run.go:324-333` documents as the blocker.
  The seam cannot stop at treadle: these transients are reached through **exported, single-directory-keyed** accessors used from outside the engine, so a CLI pause verb still passing the durable dir would write `_lyx/<module>/pause` while the engine reads `.lyx/<module>/pause`, and pause would silently stop working with no test failing on either side alone.
  `internal/state`'s `WriteJSON(path, lockPath, …)` / `ReadJSON(path, lockPath, …)` already take the lock path separately, so a lock can move without moving the file it guards.
- Rejected: giving only treadle the seam (leaves every other module's exported accessor ambiguous about which tree it means);
  asserting that no signature changes at all (false for the four `LoadState`/`SaveState` functions, which is the largest artifact class in the sweep);
  changing every accessor to take `*lyxcwd.Location` and resolve internally (wider signature churn, and it re-hides the choice the seam exists to make explicit);
  having `perchengine` compute both directories and hand treadle two paths (lands the decision in perch, so the next treadle-based module repeats the work);
  leaving perch's locks in `_lyx` behind a narrowed exclude (blocks deleting `crossModuleMachineLocalExcludes`, which is half this task).

### logger-sink-holds-no-persistent-handle

- Decision: `internal/logger`'s durable trace sink stops holding an open `*os.File` for the process lifetime.
  `writeDurable` opens the trace file, appends the record, and closes it again, per record.
  Nothing else about the sink changes: the `sinkMaxBytes` cap keeps its current truncate-and-go-silent behaviour, `sinkTruncated` and the truncation marker stay, and `retention.go`'s `traceFilePattern` and bounds are untouched.
  Splitting an over-cap trace into several files instead of truncating it is a wanted improvement, but it is a separate task — see Scope/Out.
- Rationale: the adopting process is otherwise a holder of a file inside the directory adoption moves, and no operator remedy can fix that — the holder *is* `lyx fabric reconcile`.
  Removing the persistent handle deletes the failure mode structurally instead of coordinating around it: adoption needs no logger API, no ordering guarantee and no knowledge of the sink, and `logger` stays completely weft-blind — it only ever resolves `AnchorPath()/.lyx/logs`, which after wiring resolves through the junction without logger noticing.
  The change is bounded but not free, and two things must change with the handle:
  `writeDurable` (`sink.go:154`) already takes `sinkMu` around every write, so the extra open/close pair sits inside a lock that is held anyway, and the byte accounting (`sinkBytesWritten`, `sinkTruncated`) is already a package global independent of the handle.
  But the **trace filename is not** — it is a local inside `ensureDurableSink`'s `sync.Once` closure (`sink.go:108-114`), reachable only through `sinkWriter`, so a new package-level path global is required.
  And `ensureDurableSink() (io.Writer, bool)` returns the handle itself, so its return contract changes to hand back the resolved path (or just readiness) instead;
  `sink_test.go:59-64`, which asserts the returned writer is non-nil, changes with it.
  `SetDurableSinkDir` (`sink.go:174`) already resets `sinkOnce` and every sink global, so re-entrant sink state is an established pattern here and the new path global joins that reset.
- Rejected: a `logger.ReopenDurableSink()` that adoption closes around the move (keeps the handle, but couples adoption and logger and adds an ordering rule someone will forget);
  having the wiring verbs point their own sink at the weft-side path (makes `logger` a weft witness and breaks the illusion fabric exists to maintain);
  moving per-worktree traces up to `<hub>/.lyx/logs` (dodges the lock structurally, but mixes every worktree's traces and changes logger's location semantics for an unrelated reason);
  disabling the durable sink during wiring (loses trace exactly where geometry changes, which is where it is most needed);
  keeping today's behaviour and letting adoption fail on Windows (reinstates the upgrade cliff on the platform junctions exist for).
- Cost accepted: one open+close syscall pair per record instead of one per process.
  Negligible for a CLI writing tens of records per invocation;
  the only site worth watching is a long-lived, chatty process such as the reed server, which writes to `<hub>/.lyx/logs` — outside every worktree and never adopted, but subject to the same write pattern.

### full-sweep-mirrored-subpaths

- Decision: sweep every module-owned directory, not just the four artifacts the original brief named.
  Every never-tracked artifact moves to the **same relative subpath** under `.lyx` that it had under `_lyx` — so `_lyx/webster/state.json.lock` becomes `.lyx/webster/state.json.lock`, guarding `_lyx/webster/state.json` from a sibling tree.
  Treadle's `pause` flag moves with the locks (it is the same never-tracked class as webster's and builder's).
- Rationale: the principle is the invariant, and a half sweep leaves behind exactly the cases the exclusion machinery being deleted existed for.
  Mirroring the subpath keeps the mapping mechanical and reviewable, and keeps per-module ownership of relative paths intact per the Cwd Resolution Invariant.
- Rejected: relocating only run-dir-scoped artifacts (leaves locks that guard durable state files in `_lyx`);
  extending the sweep into `_raddle`/`_pattern`/`_board` (no known transients there today — YAGNI).

### hub-level-dotlyx-is-a-recognised-geometry-element

- Decision: `<hub>/.lyx` (today `reedengine.HubLogsDir` at `<hub>/.lyx/logs`) becomes a fabric-recognised hub-level geometry element, the way `<hub>/_board` already is: created by fabric in `CloneHub`'s hub-materialisation path (`fabricengine/clone.go:103`, where `hubPath` itself is `MkdirAll`ed), named through `lyxdirs.DotLyxDirName`, documented in the geometry invariant, and reserved so no worktree slug can claim the name.
  **Reed keeps its own idempotent `MkdirAll` (`reedengine/lifecycle.go:250-253`).**
  It is not redundant: it must still work on hubs created before this change, and reed can boot without any fabric verb having run first.
  Its documented reason — the directory must exist and be pruned before the boot loop, so a fresh server's log lands somewhere that already exists — is unaffected.
  It stays a **real directory, not a junction** — `<hub>` is not a git repo, so there is nothing to exclude and no weft to point at.
  `reedengine` stops declaring the name itself.
- Rationale: "all machine-local data in `.lyx`" holds at hub level too, and one shared reed server per hub needs a deterministic hub-anchored place for its log.
  Recognising it makes the hub layout complete rather than leaving an ad-hoc `mkdir` outside the geometry model.
- Rejected: junctioning `<hub>/.lyx` into `_board`'s `weft:main` checkout (puts machine-local state in a synced branch for no gain);
  moving reed's server logs down to a worktree `.lyx` (destroys the one-server-per-hub deterministic location).
- **Implementation gotcha, must be handled:** `HubReservedNames()` currently serves two different jobs — reserving names a worktree slug may not claim, *and*, through `filterHubReserved`, stripping names from the wired junction set.
  Adding `.lyx` to that one list would make `filterHubReserved` delete `.lyx` from the wired names and the per-worktree junction would never be created.
  The two jobs must be split: a slug-reservation set (gains `.lyx`) and a junction-wiring block set (keeps exactly today's `_board`, `_portals`, `_launchers`, `_raddle`).
  **There is a third consumer, and it must take the wiring-block set:** `scanOnDiskJunctionNames` (`reconcile.go:351`) skips every `HubReservedNames()` entry, and it drives both `Unwire`'s sweep (`unwire.go:58`) and `applyStaleRemoval` (`reconcile.go:401`).
  If it were handed the slug-reservation set, `.lyx` would be skipped by the on-disk scan and become invisible to unwire and to stale removal — wired forever, never torn down.
  The split's test must assert `.lyx` *is* enumerated by the on-disk scan.

### commit-ordering

- Decision: transients move first, then the exclusion machinery is deleted, then the geometry changes.
- Rationale: every commit leaves the tree green, and the exclusions are removed only once nothing depends on them.
  The reverse order keeps both mechanisms alive simultaneously for longer.
- Rejected: geometry first.

## Technical context

### Where junction geometry lives after slice 7

`internal/hubgeometry` **no longer exists**.
Junction geometry is now in `internal/fabricengine`:

- `junction.go` — `HostJunctions`/`HostJunctionsHere` (the `{Name, Link, Target}` records), `WireJunctions` → `seedLyxJunction` + `seedGitExclude`, `UnwireJunctions` → `unseedLyxJunction` + `unseedGitExclude`, and `wireBoardLink` (the standalone `_board` case).
  `seedLyxJunction:163` is the host-pristine guard that gains the `.lyx`-only adoption branch;
  `junction.go:118` is where the weft-side target is materialised, which is the natural home for the weft-side exclude seeding.
- `junctionnames.go` — `BoardDirName`, `HubReservedNames`, `IsReservedHubName`, `filterHubReserved`, `junctionNames`/`WiredNames`/`RepoWiredNames`.
  `WiredNames(baseDir)` today returns `filterHubReserved(cfg.Dirs())` and must gain the structural sets with dedup.
- `config.go` — `Config.Pathspec` (a single free-form space-separated string, `config.go:23`) and `Config.Dirs()` (`strings.Fields`, `config.go:28`).
  The repo-wide file is `<Hub>/_board/_lyx/config/fabric.yaml`;
  the default lives in `internal/fabricengine/template.yaml:2` (`pathspec: _lyx _pattern`, shrinking to `_pattern`).
- `commit.go:91` — `Fabric.Commit` calls `RepoWiredNames(l)` then `classifyPaths(l.AnchorRel, wiredNames, files)`.
  This call site must switch from the wired set to the pathspec/commit-routing set.
- `weftgit.go` — `crossModuleMachineLocalExcludes` (lines 56-96, to delete), `seedWeftArtifactExcludes` (98-162, gains `.lyx/`, loses the cross-module patterns), `weftPathspecFilter` (177-199, gains `--exclude-standard` at `entryMatchesWeft:205`).
- `unwire.go:93-110` — the weft-`_lyx` clear-and-commit-and-push block to remove, with `UnwireVerbResult.WeftContent` (`unwire.go:29-31`) losing `"cleared"`;
  `unwire.go:30` already documents that `_pattern` weft content is preserved by design, which is the behaviour `_lyx` converges on.
  Three comments assert the old behaviour and must be corrected together: the **package doc comment at `unwire.go:9`**, the `Unwire` doc comment at `unwire.go:44`, and `fabriccli/fabric.go:262`.
- `reconcile.go:351` — `scanOnDiskJunctionNames`, the third `HubReservedNames()` consumer, driving `Unwire`'s sweep (`unwire.go:58`) and `applyStaleRemoval` (`reconcile.go:401`).
- `reconcile.go:391` — `applyStaleRemoval`, the reason downgrade is unsupported.
- `internal/lyxcwd/enforcement_test.go` — `TestEnforcement_GeometryLiterals`' `geometryToken` set and `geometryTokenOwners` map, which already defer `".lyx"`'s owner row to this slice.

### Config reconciliation, for context

`lyx config reconcile` (`internal/configcli/configcli.go:334`, dry-run by default, `--apply` to write) → `configsync.ReconcileAll` → `yamlengine.Reconcile` (`reconcile.go:21`) merges each config file against its template at **leaf key-path** level: keys present in the template but missing from the file are added, keys absent from the template are reported and removed, and existing values are preserved.
`configsync` even has value-migration precedent (`legacyFabricConfigModules` folds pre-cutover `warp.yaml`/`weft.yaml` into `fabric.yaml`).
What it deliberately does not do is rewrite the **value** of an existing key — which is exactly what adding `.lyx` to `pathspec`'s free-form string would have required, and exactly why the structural decision avoids the problem instead of solving it.

### The committed-`.gitignore` sites to remove

- `internal/fabriccli/clone.go:81` — `gitignore.Ensure(l.AnchorPath(), ".lyx/")` inside `runCloneWithReset`, a few lines below the `WireJunctions` call at :77.
  **Slice 10 edits the same function**, hence the sequencing.
- `internal/fabricengine/unwire.go:113` — `gitignore.Remove(cwd, ".lyx/")`, plus the doc comments at `unwire.go:44` and `fabriccli/fabric.go:262`.
- `internal/fabricengine/unwire_test.go:137,156` — `TestUnwire_RevertsGitignore` asserts the behaviour being removed and must go with it.
- `internal/gitignore` itself stays — it is still used for other entries; only the `.lyx/` callers go.

### Full transient inventory (the sweep)

Under `_lyx`, to relocate to the mirrored `.lyx` subpath:

| Module | Artifact | Declared at |
| --- | --- | --- |
| webster | `pause` | `internal/websterengine/pause.go:27,31` |
| webster | `prompts/*` (rendered fork prompts) | `internal/websterengine/state.go:52-57`; consumers `beginbatch.go:201-204`, `runlevel.go:398,460-463` |
| webster | `run.lock` | `internal/websterengine/runlevel.go:38` |
| webster | `mutate.lock` | `internal/websterengine/state.go:70` |
| webster | `state.json.lock` | `internal/websterengine/state.go:195,211` |
| builder | `pause` | `internal/builderengine/pause.go:23,31` |
| builder | `run.lock` | `internal/builderengine/runlevel.go:34,304` |
| builder | `mutate.lock` | `internal/builderengine/state.go:55` |
| builder | `state.json.lock` | `internal/builderengine/state.go:135,149` |
| perch/treadle | `run.lock` | `internal/treadleengine/run.go:42,98` |
| perch/treadle | `state.json.lock` | `internal/treadleengine/state.go:101,140,156` |
| perch/treadle | `pause` | `internal/treadleengine/state.go:32,209-216` |
| loom | `status.json.lock` | `internal/loomengine/config.go:79` |

Already in `.lyx` but **`WorktreePath()`-anchored**, so they change twice — adopt `lyxdirs.DotLyxDirName` *and* re-anchor onto `AnchorPath()`: `scoutengine/daemonstate.go:43,50`, `shuttleengine/rundir.go:51` and `run.go:185`, `burlerengine/engine.go:106`, `logger/sink.go:37`, and `reedengine`'s state sites (`spawn.go:143,183`, `strand.go:309,348,471`, `lifecycle.go:545,676,778,961`, `lock.go:66`).

Hub-anchored and deliberately unchanged in anchor: `reedengine.HubLogsDir` (`lifecycle.go:38`), which only adopts the shared const.

Out of scope, confirmed not under `_lyx`: `fabricengine`'s correspondence index (`index.go:75`, inside the git dir) and `board.push.lock` (`bolt.go:34`, under `<hub>/_board`).

### Call sites that consume the pathspec

`internal/fabriccli/weft_verbs.go:100` builds `fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())` and passes it to `fab.Commit` at :155, :184 and :240.
With `_lyx` no longer in `template.yaml`'s default, this site must build from the pathspec/commit-routing set (`structuralCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())`) or a freshly-initialised repo silently stops syncing `_lyx` at all.
This is the single most breakage-prone edit in the task.

`Topology.Add`'s reserved-name union also reads raw `Dirs()`;
it must gain both structural sets so no worktree slug can be named `_lyx` or `.lyx`.

Module weft-commit callers build their own pathspec from `configengine.LyxDirName` only (`internal/perchcli/run.go:334`, `internal/fabricengine/unwire.go:104`, webster's and builder's `weft.go`), so they are unaffected beyond the const rename.

### Empirically verified git behaviour

Run in a scratch repo with `.lyx/` in `.git/info/exclude`:

- `git add _lyx/webster/state.json .lyx/webster/state.json.lock` → exit 1, `The following paths are ignored by one of your .gitignore files`, **nothing staged at all**.
- `git add .lyx` (directory pathspec) → same hard failure.
- `git ls-files --cached --others -- .lyx` → **matches** `.lyx/webster/state.json.lock` (exit 0), because `--exclude-standard` is absent.

### Existing patterns to reuse

- `internal/fslink` — `CreateDirLink`, `IsLink`, `PointsTo`, `Remove`.
  Directory-only contract; junctions on Windows, symlinks elsewhere.
- `internal/fabricengine/junction_pattern_integration_test.go` — the template for a `.lyx` junction integration test; it already resolves the exclude file the way `seedGitExclude`/`unseedGitExclude` do.
- `internal/lyxtest` — synthetic hub construction for integration tests (note `lyxtest.go:231` joins `configengine.LyxDirName`; it participates in the rename).
- `cmd/lyx/boardguard_test.go` and `cmd/lyx/rawgitmutation_test.go` — the shape to copy for the new AST enforcement test.

### Docs that assert the old behaviour

- `tools/sandbox/SANDBOX-BUILDER-SUITE.md:276,279` — an entire scenario built on machine-local artifacts living under `_lyx` and being held back by the exclude layer.
  It must be rewritten to assert the artifacts are under `.lyx` instead;
  the underlying property it checks (never committed, never materialised on another machine) survives.
  The sandbox is also the one known repo carrying a committed `.gitignore` `.lyx/` block from a pre-fix binary.
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:128` — names `_lyx/webster/prompts/02-*.md`.
- `docs/reference/builder-contract.md:166` — describes `*.lock` and `*/builder/pause` being kept out "solely at the git-exclude layer (`fabricengine.seedWeftArtifactExcludes`)", the mechanism this task deletes.
- `manifest/designs/fabric-unified-view.md:60-64` — the **as-built anchoring table**, which states the `.lyx` group (`WorktreeLogsDir`, `ScoutDaemonStateFile`, `ScoutDaemonLock`) joins onto `Location.WorktreePath()`.
  This slice inverts exactly that line, so the paragraph is corrected in the same commit as the slice-9 status update, not just marked shipped.
  It also claims the table is "mirrored verbatim in `CONSTRAINTS.md`'s Cwd Resolution Invariant" — **that mirror does not exist**: CONSTRAINTS carries the per-segment join rule but no per-symbol anchoring table, so the stale cross-reference is dropped rather than chased.
- `docs/shared-libs/README.md:35` — describes the durable sink as "worktree-anchored (`.lyx/logs`, lazily opened …)".
  Both halves change: the anchor becomes `AnchorPath()`, and the sink is no longer a lazily-opened persistent handle.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()`, `AnchorPath()`.
  It never resolves a weft path, a junction path, or any per-module subdirectory; weft-sibling and junction construction belong to `internal/fabricengine`.
  A module's own subdirectory is that module's private relative-path constant joined onto `AnchorPath()` — per-segment joins, never a fused `"_lyx/..."` literal.
  `lyxcwd`'s imports stay capped at stdlib plus `internal/gitexec`, which is what keeps `fabricengine → logger → lyxcwd` acyclic.
  **Geometry is structural, never config/env-overridable** — the direct basis for the structural-dirs and never-committed decisions.
- **Scoutengine Leaf Invariant** — names the allowlist that must gain `internal/lyxdirs`; the prose clause and `leaf_enforcement_test.go` must change together.
- **lyxtest Leaf Invariant** and **CLI/Cobra Invariant** — no new commands here, but `internal/lyxdirs` must itself stay a leaf (stdlib-only) or it defeats its own purpose.
- **Weft Git Invariant / "Cross-module exclusions"** (`CONSTRAINTS.md:144-152`) — the mechanism this task retires.
  Its "known limitation" note (a pre-fix sync's already-committed artifacts need manual `git rm --cached`) survives as the documented manual remedy.
- **Documentation Lifecycle** — a task touching a module doc, the module table, or a cross-cutting invariant updates them in the same commit.
- **Tier 1/2 substrate rule** — tests must be tiered correctly; junction wiring on a synthetic hub is substrate-backed.

New invariants this task records in `CONSTRAINTS.md`:

- Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to; `_lyx` holds tracked content only.
- `_lyx` and `.lyx` are directory siblings under `AnchorPath()`, both structural — injected by `fabricengine`, never read from `fabric.yaml`.
- Every host→weft junction is excluded through the warp's `.git/info/exclude`, never through a committed `.gitignore` in the user's repo.
- Unwiring reverses wiring only; it never deletes weft-side content.
- `internal/lyxdirs` is the single declarer of both `_lyx` and `.lyx`.

## Testing

**`internal/lyxdirs`** — no behaviour to test beyond the enforcement tests below; it is two consts.

**Enforcement test 1 — single declarer: amend the existing test, do not add a new one.**
`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals` already polices whole-token geometry literals in path-construction context against a per-token owner map, already excludes `*_test.go`, and its comment states outright that `".lyx"` is *deliberately* unpoliced because **slice 9 is where it gets an owner row**.
This task fulfils that: `.lyx` joins `geometryToken`'s set and gains the owner row `{"internal/lyxdirs"}`, the `"_lyx"` row moves from `{"internal/configengine"}` to `{"internal/lyxdirs"}`, and the deferral comment is deleted.
Nothing new needs writing — the exact-token matching that makes this implementable is already there, so `".lyx-anchor"` (`lyxcwd/anchor.go:41`) and `".lyx/"` are distinct tokens and never match, and the surviving exclude entry is built as `lyxdirs.DotLyxDirName + "/"`.
The conversion list is the exact-literal inventory in the `lyxdirs-owns-both-directory-tokens` decision.

**Enforcement test 2 — no transients under `_lyx` (TDD candidate).**
A runtime test on a `lyxtest` synthetic hub rather than a static rule: drive each module's exported path constructors (`websterengine.Dir`/`PromptsDir`, `builderengine.Dir`, `perchengine.RunsDir`, `loomengine`'s status path, and treadle's run/scratch dirs) against a synthetic `Location`, then assert no returned `_lyx`-rooted path ends in `.lock`, equals `pause`, or resolves inside a `prompts` directory — and that each corresponding transient path *does* resolve under `.lyx`.
This checks actual behaviour instead of attempting the dataflow analysis a static rule would need.

**`internal/fabricengine`:**

- Junction lifecycle on a `lyxtest` synthetic hub (mirroring `junction_pattern_integration_test.go`): wiring creates the `.lyx` junction pointing at `<weft>/<AnchorRel>/.lyx`, seeds `.lyx` into the warp's `.git/info/exclude` **and** `.lyx/` into the weft's, and unwiring removes the junction and the warp entry.
  TDD candidate.
- Seeding order: after wiring and before any weft-git verb runs, writing a file into `.lyx` leaves `git status --porcelain` in the weft worktree clean.
  Regression test for the ordering hole.
- Unwire preserves weft content: after `Unwire`, both `<weft>/<AnchorRel>/_lyx` and `<weft>/<AnchorRel>/.lyx` still exist with their content, no `"lyx fabric unwire: clear _lyx"` commit was made, and `weft_content` reports `"preserved"`.
  The CLI envelope no longer carries a `gitignore` key.
  Existing tests asserting the old clear-and-commit behaviour are rewritten.
  TDD candidate.
- `.lyx` content adoption: a pre-existing real `.lyx` directory holding files is moved into the weft target and replaced by a junction, idempotently;
  a name collision in the target aborts with both sides untouched;
  a pre-existing real `_lyx` or `_pattern` directory still produces the hard refusal.
  TDD candidate — the halves must be asserted together or the adoption branch will over-reach.
- Structural sets: `WiredNames` returns `_lyx` and `.lyx` even for a `fabric.yaml` whose `pathspec` names neither;
  a deployed `pathspec: _lyx _pattern` yields **no duplicate** `_lyx` in any of the three sets;
  the pathspec/commit-routing set contains `_lyx` but never `.lyx`.
  TDD candidate.
- `classifyPaths`' third bucket: a `.lyx` path is routed to **neither** weft nor warp, and `Commit` returns a hard error naming it.
  Asserting only "never routed to weft" is insufficient and would pass on the dangerous implementation, where the path silently falls through to the user's own repo.
  TDD candidate.
- `weftPathspecFilter` with `--exclude-standard`: an entry matching only ignored files is filtered out and reported non-positive, so the commit is a clean no-op instead of a hard `git add` failure.
  TDD candidate — this one has a verified pre-fix failure to reproduce.
- `seedWeftArtifactExcludes` seeds `.lyx/` and no longer seeds the three cross-module patterns; idempotent on re-run.
- Reconcile/drift/health still converge with `.lyx` in the wired name-set.
- The split of `HubReservedNames` into slug-reservation versus junction-wiring sets: `.lyx` is refused as a worktree slug, **survives into the wired junction set**, and **is enumerated by `scanOnDiskJunctionNames`** so unwire and stale removal can still see it.
  TDD candidate — the naive one-list implementation fails exactly this trio.

**`internal/fabriccli`** — `lyx fabric sync` still commits `_lyx` content with `_lyx` gone from the template default (the regression guard for the `weft_verbs.go:100` edit), and never names `.lyx`.
Clone no longer writes a `.gitignore` entry;
the resulting host worktree has `.lyx` in `.git/info/exclude` and a clean `git status`.
`unwire` no longer reverts a `.gitignore` block (delete `TestUnwire_RevertsGitignore`).

**`internal/treadleengine`** — the scratch-dir seam: with no scratch dir set, locks and `pause` land in `runDir` (back-compat default); with one set, `run.lock`, `state.json.lock` and `pause` land there while `state.json` and round artifacts stay in `runDir`.
Mutual exclusion (`ErrBlockBusy`) and pause-clearing still work when the two directories differ.
TDD candidate.

**`internal/perchcli`** — the run handler passes the `.lyx`-anchored scratch dir, and the weft pathspec it builds still names only `_lyx`.

**`internal/websterengine` / `internal/builderengine` / `internal/loomengine`** — per-module path tests asserting each relocated artifact resolves under `.lyx` at the mirrored subpath, and that the durable files (`state.json`, `status.json`, reports) stay under `_lyx`.
Locking still serialises correctly when the lock file sits in a sibling tree from the file it guards.

**Anchor re-parenting** — for a synthetic hub with `AnchorRel != "."`, every relocated and every pre-existing `.lyx` consumer resolves under `AnchorPath()`, and there is exactly one `.lyx` root in the worktree.
Regression test for the two-roots bug.
Existing tests assert the **old** `WorktreePath()` anchoring and must be rewritten, not merely re-run: `cmd/lyx/constructoranchoring_test.go:77-85,123-131` (its "`.lyx` group: stays WorktreePath-anchored, ignoring AnchorRel" case inverts), `internal/logger/worktreelogs_test.go:20,40`, and `internal/burlerengine/engine_test.go:461,501`.
Scout's anchoring is asserted end-to-end rather than at `daemonstate.go` alone: a lookup in a subpath-anchored worktree resolves `daemon.json`/`daemon.lock` under `AnchorPath()/.lyx/scout/<lang>/`, while the daemon singleton key derived from `WorktreeRoot` is unchanged.

**Hub-level `<hub>/.lyx`** — `CloneHub` creates it, and reed's own `MkdirAll` remains idempotent against an already-created directory (assert both, since the second is what covers pre-fix hubs).

**`internal/logger`** — the durable sink writes correctly with no persistent handle: records from the same process append to one file across many calls, the header is still written exactly once, `sinkBytesWritten` accounting and the `sinkMaxBytes` truncation marker behave as before, and no file handle survives a `writeDurable` call.
The last point is the one that matters for adoption and is the TDD candidate: assert it by moving/renaming the logs directory between two log records, which fails today and must succeed after.

**Leaf allowlists** — each amended `leaf_enforcement_test.go` / seam test still passes, and no package gains `internal/lyxdirs` in its allowlist without actually importing it.

**Cross-platform** — junction creation goes through `internal/fslink`, already covered by `fslink_test.go`; no new platform-specific assertions needed beyond keeping the new tests off hard-coded separators.
The adoption path's busy-directory behaviour is the one genuinely platform-divergent case and should be asserted through its error contract rather than by simulating open handles.

## Q&A log

- **Q:** Skal `.lyx` bli en weft-backed junction, eller forbli en vanlig host-katalog? **A:** Junction — `.lyx` skal fysisk ligge i weft og eksponeres fra warp via en junction som fabric lager.
- **Q:** Skal eksisterende ekte `.lyx`-kataloger migreres? **A:** Opprinnelig nei (hard feil). Revidert etter review-runde 1: ja, men **kun** for `.lyx`. Uten adopsjon hard-feiler første `reconcile` i hver eneste eksisterende worktree, og eneste botemiddel ville vært å slette levende daemon-state.
- **Q:** Gir det mening at `configengine` eier `.lyx`-navnet? **A:** Nei — eierskapet er arvet, ikke prinsipielt. Ny bladpakke `internal/lyxdirs` eier begge navn.
- **Q:** Hvordan flytter perch sine låser når treadle-motoren er geometri-blind? **A:** Treadle får en eksplisitt scratch-dir-inngang; perch-CLI-en oppgir `.lyx`-stien. Motoren blir fortalt, ikke utleder.
- **Q:** Skal treadles egen `pause`-flagg (ikke nevnt i briefen) også flytte? **A:** Ja — samme aldri-tracked klasse som webster og builder.
- **Q:** `.gitignore` eller `.git/info/exclude` på weft-siden? **A:** `.git/info/exclude`. Warp får aldri en committed `.gitignore` fordi den ville avslørt at LYX brukes i brukerens eget repo; weft er vår egen, så der er valget rent praktisk.
- **Q:** Hvor langt går sveipen etter feilplasserte transienter? **A:** Alle aldri-tracked artefakter under enhver modul-eid katalog, inkludert hver `*.lock` under `_lyx`, speilet til samme subpath under `.lyx`.
- **Q:** Hva med hub-nivå `.lyx` (`<hub>/.lyx/logs`)? **A:** Fabric anerkjenner den som geometri-element, slik den anerkjenner `<hub>/_board` som weft:main. Fortsatt ekte katalog, ingen junction.
- **Q:** Blir ikke en `.lyx`-fil i lista til `Fabric.Commit` bare ekskludert av `git add` selv? **A:** Filen blir aldri committet, men `git add` feiler hele invokasjonen (exit 1, ingenting staged). Derfor kreves både at `.lyx` holdes ute av pathspec-bygging og commit-ruting, **og** `--exclude-standard` på `weftPathspecFilter`.
- **Q (gap r1):** Hvilket anker bruker `.lyx`? **A:** `AnchorPath()`. `_lyx` og `.lyx` skal være mappesøsken; begge havner i anchor.
- **Q (gap r1):** Hvordan får en allerede utrullet `fabric.yaml` `.lyx` i seg? **A:** Den skal ikke. `_lyx` og `.lyx` blir strukturelle og injiseres i kode; `pathspec` beholdes kun for valgfrie kataloger.
- **Q (gap r1):** Én eier av weft-sidens exclude, og garantert rekkefølge? **A:** `seedWeftArtifactExcludes` er eneste eier; `WireJunctions` kaller den ved wiring, `ensureWeftLockDir` beholdes som selv-heling.
- **Q (gap r1):** Mekanismen for enforcement-testene? **A:** Test 1 AST-regel med eksakt literal-match på ikke-test-filer; test 2 kjøretidstest på syntetisk hub, siden en statisk regel ville krevd dataflyt-analyse.
- **Q (gap r2):** Burde ikke *alle* junctions stått i én yaml-liste, for ryddighetens skyld? **A:** Ideelt sett ja, men alt i lyx feiler om `_lyx` eller `.lyx` mangler — de må være der. Obligatorisk geometri kan ikke ligge i en liste man kan tømme. Ført opp som avvist alternativ.
- **Q (gap r2):** Skal denne tasken bygge verdi-nivå-migrering for `fabric.yaml`? **A:** Nei. `lyx config reconcile` propagerer allerede nye *nøkler*; verdi-migrering er et eget designproblem og den strukturelle beslutningen fjerner behovet.
- **Q (gap r2):** Hva skjer med weft-sidens `.lyx` ved `unwire`? **A:** Ingenting — og det avdekket at `unwire` i dag sletter weft-sidens `_lyx`, hvilket er feil. «Unwire» betyr det motsatte av «wire»: fjern junction-koblingen, ikke slett kildemappa. Rettes i denne tasken. Slettingen av weft-`_lyx` hører hjemme før en branch merges inn — et helt annet steg, og en egen task. `.lyx` trenger ikke røres, den forsvinner med worktreet.
- **Q (gap r2):** Skal repoer klonet av en eldre binær få den committede `.gitignore`-blokka ryddet? **A:** Nei, utenfor scope. Vi har omtrent ingen slike repoer — kun sandboxen, som re-klones uansett.
- **Q (gap r3):** Hvilket sett skal `scanOnDiskJunctionNames` konsumere etter splitten? **A:** Junction-wiring-blokk-settet. Med slug-reservasjonssettet ville `.lyx` blitt hoppet over av disk-sveipen og dermed usynlig for både unwire og stale removal.
- **Q (gap r3):** Er treadle-seamen nok, eller trenger de andre modulene også en? **A:** Alle trenger den. Transientene nås via eksporterte, katalog-nøklede accessorer utenfra motoren, så en pause-verb som fortsatt oppgir den durable katalogen ville skrevet til `_lyx` mens motoren leser `.lyx` — pause slutter stille å virke. Hver modul får `ScratchDir(l)` som søster til `Dir(l)`.
- **Q (gap r3):** Ny AST-test for single-declarer? **A:** Nei. `TestEnforcement_GeometryLiterals` i `lyxcwd/enforcement_test.go` har allerede eier-tabellen, og kommentaren der sier eksplisitt at `.lyx`' eier-rad er utsatt til slice 9. `_lyx`-raden flyttes til `internal/lyxdirs` og `.lyx` får sin egen.
- **Q (gap r3):** Adopsjonen låser seg selv — prosessen holder sin egen trace-fil åpen inne i mappen den flytter. Må logger ha en fil åpen hele tiden? **A:** Nei. Den vedvarende fildeskriptoren fjernes: åpne, appende, lukke per record. Da forsvinner selvlåsingen strukturelt, adopsjonen trenger ingen logger-API, og logger forblir weft-blind. Å peke sinken mot weft-siden ble avvist — ingen modul skal kunne «se» at fabric har et eget weft-repo.
- **Q (gap r3):** Skal filen rulleres i stedet for å trunkeres når den blir for stor? **A:** Ønskelig, men ikke i denne tasken — egen oppgave senere. Merk at den også må utvide `retention.go:26`s `traceFilePattern`, ellers blir rullerte deler aldri ryddet.
- **Q (gap r4):** Holder det å utelate `.lyx` fra commit-rutingen? **A:** Nei, det er verre enn å la den være. `classifyPaths` er et to-veis skille der alt som ikke treffer et weft-prefiks faller gjennom til **warp** — altså brukerens eget repo. Den trenger en tredje bøtte, og `Commit` må gi hard feil som navngir stien.
- **Q (gap r4):** Kan seamen holdes til rene `(dir string)`-signaturer? **A:** Nei. `LoadState`/`SaveState` i webster og builder utleder både `state.json` og `state.json.lock` fra ett katalogargument, og denne tasken skiller nettopp de to. Alle fire får et scratch-dir-parameter, og `RunDeps` får et scratch-felt for de motor-interne kallene.
- **Q (gap r4):** Hva blir `unwire`s resultatvokabular? **A:** `weft_content` blir `"preserved"` | `"not_present"`; `gitignore`-nøkkelen fjernes helt, siden mekanismen den rapporterer om er borte.
- **Q (gap r5):** Holder det å re-ankre `daemonstate.go` for scout? **A:** Nei — den tar en `worktreePath string`, og ankeret velges i `scoutcli.resolveWorktreeRoot` og tres gjennom `Options.WorktreeRoot`. En egen anker-verdi tres inn i stedet for å gjenbruke `WorktreeRoot`, som også nøkler daemon-singletonen.
- **Q (gap r5):** Skal `logger.WorktreeLogsDir` få nytt navn? **A:** Ja — `logger.LogsDir`. Navnet ville ellers påstått et anker den ikke lenger bruker.
- **Q (gap r5):** Hvem oppretter `<hub>/.lyx`? **A:** Fabric, i `CloneHub`s hub-materialisering (`clone.go:103`). Reed beholder sin idempotente `MkdirAll` — den må virke på hubber laget før denne endringen, og reed kan boote uten at noen fabric-verb har kjørt.
- **Q (gap r5):** Re-ankres shuttles konfigurerte `cfg.RunDir` også? **A:** Ja, den relative grenen. Ellers ville én funksjon resolvet mot to ulike baser. Absolutt `cfg.RunDir` står uendret.
