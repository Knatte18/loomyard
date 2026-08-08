# Discussion: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
task: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)
slug: scout-lyxcwd-accessors
status: discussing
parent: main
```

## Problem

`scoutengine.DaemonStateFile` and `scoutengine.DaemonLock` take a plain `string` worktree path because, until recently, `scoutengine` could not import `internal/lyxcwd`.
The consequence is split ownership of one path: `scoutcli` decides *which* directory the scout daemon's runtime state lives under (`resolveWorktreeRoot`, `internal/scoutcli/cli.go:477`), threads that string through `scoutengine.Options.WorktreeRoot` → `acquireConnection` → `ensureServer` → `ensureSupervised`, and only there does `scoutengine` join its own `.lyx/scout/<lang>/` segments onto it.
The Cwd Resolution Invariant says a module's own subdirectory is that module's own private relative-path constant joined onto a `*lyxcwd.Location` accessor directly — the resolving and the joining belong to one owner, and today they don't.

**Why now:** the `scout-seam-conversion` task (commit `62e17d13`) rewrote the scoutengine constraint from a leaf-package import allowlist into a banned-list seam rule.
`internal/lyxcwd` is no longer a banned import for `scoutengine`, which removes the sole reason the accessors took a `string`.
This task collects the debt while the seam change is fresh.

A second, smaller defect is folded in: the `assert-no-callers` subcommand builds its `scoutengine.Options` literal by hand (`internal/scoutcli/cli.go:593-599`) instead of going through `buildOptions`, and leaves `WorktreeRoot` empty when `lyxcwd.Resolve` fails.
An empty worktree root makes `DaemonStateFile("", lang)` return a *relative* `.lyx/scout/<lang>/daemon.json`, resolved against whatever the process working directory happens to be — a latent bug the six `buildOptions` call sites do not have.

### Three corrections to the wiki task body

The task body names three symbols that do not exist in the tree.
mill-plan must plan against the real names, not the body's.

1. **`lyxdirs.DotLyxDirName` does not exist.**
   There is no `internal/lyxdirs` package.
   `.lyx` is declared privately per module today — `scoutengine/daemonstate.go:31` and `logger/sink.go:27` each declare their own `dotLyxDirName` const.
   `daemonstate.go:26-31` states outright that this token stays unpoliced until "slice 9 is where `.lyx` gets a single owner".
   Scout keeps its own `dotLyxDirName` and `scoutDirName` consts unchanged.
2. **`websterengine.ScratchDir` does not exist.**
   The named precedent is wrong in two ways: there is no such function, and `websterengine`'s actual constructors (`Dir`/`ReportsDir`/`PromptsDir`, `internal/websterengine/state.go:41-57`) are `_lyx`-**durable** and `AnchorPath()`-anchored — the opposite anchoring group from scout's.
   The correct precedent is `logger.WorktreeLogsDir(l *lyxcwd.Location)` (`internal/logger/sink.go:36`): same `.lyx`-ephemeral group, takes a `*lyxcwd.Location`, joins onto `WorktreePath()`.
3. **`Options.AnchorRoot` and `resolveAnchorRoot` do not exist.**
   They are named `Options.WorktreeRoot` (`internal/scoutengine/refs.go:48`) and `resolveWorktreeRoot` (`internal/scoutcli/cli.go:477`).
   Pure naming drift in the body — no design impact.

The body also says five test files are affected.
The real count is nine (enumerated under Technical context), one of which — `cmd/lyx/constructoranchoring_test.go` — lies outside the scope paragraph the body states.

## Scope

**In:**

- `internal/scoutengine/daemonstate.go` — re-signature `DaemonStateFile`/`DaemonLock` to take `*lyxcwd.Location`, join onto `l.WorktreePath()`.
- `internal/scoutengine/refs.go` — replace `Options.WorktreeRoot string` with `Options.Layout *lyxcwd.Location`; update `acquireConnection`'s `ensureServer` call.
- `internal/scoutengine/ensureserver.go` — re-signature `ensureServer` and `ensureSupervised` to carry `*lyxcwd.Location` instead of `worktreeRoot string`.
- `internal/scoutcli/cli.go` — `resolveWorktreeRoot` → `resolveLocation`; `buildOptions` signature; six `buildOptions` call sites; the `assert-no-callers` hand-built literal.
- `internal/scoutengine/doc.go` — the "Daemon state and concurrency" section's description of the accessors.
- The nine affected test files (enumerated under Technical context), including `cmd/lyx/constructoranchoring_test.go`.
- Two new pin tests (out-of-hub Location, `assert-no-callers` routing) — see Testing.
- `// TODO(dotlyx):` markers on both accessors, so the dotlyx-hygiene task finds them.

**Out:**

- **Changing the anchor from `WorktreePath()` to `AnchorPath()`.**
  Deliberately not done — see the `anchor-stays-worktreepath` decision.
- Creating an `internal/lyxdirs` package or otherwise giving `.lyx` a single owner.
  That is the dotlyx-hygiene task's own scoped mandate.
- `CONSTRAINTS.md`.
  This adds no new cross-cutting invariant — it is an application of the existing Cwd Resolution Invariant.
  See the `no-constraints-note` decision for why the `.lyx`-anchoring rule specifically must *not* be written down here.
- `manifest/roadmap.md`.
  A refactor, not a planned-item completion.
- `docs/overview.md`.
  No module added, no execution-stack change, no CLI behaviour change.
- `internal/scoutengine/lspclient.go`.
  It is under a file-scoped import guard (stdlib + `internal/logger` only, `lspclient_guard_test.go`) and has nothing to do with daemon geometry.
  Never add a `lyxcwd` import there.
- Any observable CLI behaviour change **other than the one intended delta** named in `fix-assert-no-callers-literal`: out-of-hub `assert-no-callers` stops resolving its daemon-state path relative to the process working directory and starts resolving it under `filepath.Abs(targetDir)`, matching the other three commands.
  Every *other* resolved path must be byte-identical before and after, in-hub and out-of-hub alike.

## Decisions

### anchor-stays-worktreepath

- **Decision:** `DaemonStateFile`/`DaemonLock` join onto `l.WorktreePath()`, not `l.AnchorPath()`, contradicting the task body's literal instruction.
  Leave a `// TODO(dotlyx):` marker on both accessors noting they are candidates for the `WorktreePath` → `AnchorPath` migration when `.lyx` gets a single owner.
- **Rationale:** `cmd/lyx/constructoranchoring_test.go` pins a deliberate three-group anchoring table recorded as a Shared Decision, and asserts (lines 123-128) that `DaemonStateFile`/`DaemonLock` stay **byte-identical at a nested `AnchorRel`** — grouped with `logger.WorktreeLogsDir` as `.lyx`-ephemeral, WorktreePath-anchored, explicitly ignoring `AnchorRel`.
  Switching to `AnchorPath()` would break that test and reverse the recorded decision.
  The `WorktreePath` → `AnchorPath` migration for `.lyx` is the dotlyx-hygiene task's own scoped mandate (`daemonstate.go:29-30` names it: "slice 9 is where `.lyx` gets a single owner"); doing it piecemeal for one module leaves scout and `logger` inconsistent with each other for however long slice 9 takes.
  The CONSTRAINTS bullet mandating `AnchorPath()` is scoped to a module's *durable-storage* subdirectory; `.lyx` is ephemeral by construction, so the bullet does not bind here.
  It also keeps the daemon a worktree-wide singleton per language, which is what `DaemonStateFile`'s own doc comment promises.
- **Rejected:** `l.AnchorPath()` as the body says — breaks `TestConstructorAnchoring_SubpathAnchored`, makes `scoutengine` inconsistent with `logger.WorktreeLogsDir`, and pre-empts a decision another task owns.

### options-carries-layout

- **Decision:** `scoutengine.Options.WorktreeRoot string` becomes `Options.Layout *lyxcwd.Location`.
  `TargetDir` stays a separate `string` field.
- **Rationale:** this is the task's whole point — `scoutengine` holds the typed `Location` and derives its own path from it, rather than receiving a path a different package already chose.
  `Layout` mirrors `websterengine.RunDeps.Layout`.
  The rename also fixes a Cwd Resolution Invariant naming violation: `WorktreeRoot` is a `root`-named field, and the invariant reserves `root` for the git worktree root — which is what it holds in-hub but *not* out-of-hub, where it holds `filepath.Abs(targetDir)`.
- **Rejected:** keeping `WorktreeRoot string` and re-signaturing only the two accessors — the engine still receives a pre-chosen path string, so split ownership survives and the task achieves nothing.
  Also rejected: dropping `TargetDir` and deriving it from the `Location` — `--target-dir` is an independent user-facing flag that legitimately differs from the worktree root.

### field-named-layout

- **Decision:** the field is `Layout`, not `Location`.
- **Rationale:** one vocabulary across the repo — `websterengine.RunDeps.Layout`, `buildercli`/`webstercli`'s `c.layout`, `perchengine`'s `layout` field all name a `*lyxcwd.Location` this way.
- **Rejected:** `Location` — truer to the type name, but the repo already settled.

### out-of-hub-synthesized-location

- **Decision:** when `lyxcwd.Resolve(cwd)` fails, `scoutcli` synthesizes the `Location` by hand from the absolute target directory:

  ```go
  abs, err := filepath.Abs(targetDir)  // on error, fall back to targetDir as today
  &lyxcwd.Location{HubPath: filepath.Dir(abs), WorktreeName: filepath.Base(abs), AnchorRel: "."}
  ```

- **Rationale:** `WorktreePath()` is `filepath.Join(HubPath, WorktreeName)`, so this yields exactly `abs` — byte-identical to today's `resolveWorktreeRoot` fallback.
  No new failure modes, no new git spawns, and the two branches of `resolveWorktreeRoot` map one-to-one onto the two branches of the new `resolveLocation`.
- **Constraint on the synthesized value — it is a fiction outside `WorktreePath()`.**
  Hand-building a `Location` outside `internal/lyxcwd` has only two production precedents, both in the geometry owner itself (`fabricengine/clone.go:125`, `fabricengine/hostlayout.go:30`).
  The value synthesized here is deliberately narrower: `HubPath` is merely the parent of the target directory and names no real hub, `RepoName` is left zero, and `AnchorPath()` is meaningless because `AnchorRel` was assumed rather than read from a `.fabric-anchor` marker.
  Only `WorktreePath()` is a true answer.
  It is therefore contractually consumed by `DaemonStateFile`/`DaemonLock` alone — the two accessors that read nothing else — and must never be widened to feed a caller that reads `AnchorPath()`, `HubPath`, or `RepoName`.
  `resolveLocation`'s doc comment must say this explicitly, so the next person to reach for the out-of-hub branch sees the limit before reusing the value.
- **Rejected:** `lyxcwd.ResolveWorktree(targetDir)` — spawns git, can still fail outside any repo, and would need this same fallback anyway; strictly more moving parts for the same answer.
  Also rejected: leaving `Options.Layout` nil out-of-hub and degrading to the legacy path — a real behaviour change (the supervised daemon stops working outside a hub) plus a nil-deref hazard.

### no-nil-layout-check

- **Decision:** `Options.Layout` is documented as required.
  No nil check anywhere in `scoutengine`.
- **Rationale:** `websterengine` never nil-checks `deps.Layout` either (`recordbatch.go:103`, `runlevel.go:472`, `beginbatch.go:196`).
  A nil deref is the correct, immediate signal for a caller that skipped a required field, and `scoutcli` — the only production caller — now always supplies one.
- **Accepted weakness — the fail-fast is language-conditional, not universal.**
  `Layout` is dereferenced only inside `ensureSupervised`, which is reached only when `entry.HasNativeDaemon` is true (`refs.go:64-65`) — in V1 that is Go alone.
  A nil `Layout` on a Python, C#, TypeScript, or Rust lookup therefore never panics and never surfaces;
  it is silently unused.
  So "fails fast" is true for Go and false for every other language.
  Accepted rather than mitigated: the alternative is a nil check whose only job is to make a can't-happen programmer error louder for four languages that ignore the field anyway, and `scoutcli` supplying a non-nil `Layout` at all four construction sites is what actually closes the hole.
  Flagged here so a future reader does not mistake the fail-fast claim for a universal guarantee.
- **Rejected:** returning a typed error from `References`/`Definition`/`Symbol` when nil — a new error path for a programmer error that cannot occur in-tree.
  Also rejected: falling back to a cwd-derived `Location` inside the engine — that is the engine resolving geometry, the exact thing being deleted.

### location-threaded-to-ensuresupervised

- **Decision:** the `*lyxcwd.Location` is threaded down the existing call chain, typed: `acquireConnection` passes `opts.Layout` → `ensureServer` → `ensureSupervised`, which calls `DaemonStateFile(l, lang)` and `DaemonLock(l, lang)` itself.
- **Rationale:** `ensureSupervised` is the sole production caller of both accessors, so *something* must reach it — the task body's "removes the parameter threading" means removing the split *resolution* ownership, not the parameter.
  Threading the typed `Location` keeps the accessors' only caller the owner of its own paths, with a minimal diff.
- **Rejected:** resolving `statePath`/`lockPath` in `acquireConnection` and threading those two strings down — reintroduces exactly the split ownership being removed, one layer up.
  Also rejected: stashing the `Location` on a package-level or client struct — hidden state for no gain.

### fix-assert-no-callers-literal

- **Decision:** `assert-no-callers` routes through the same `resolveLocation` helper as the other three commands, and its hand-built `Options` literal is replaced by a shared constructor so `Layout` is never nil.
  The seam is decided here, not left to mill-plan: extract

  ```go
  func baseOptions(registry scoutengine.Registry, targetDir string, layout *lyxcwd.Location, lang string, timeout time.Duration) scoutengine.Options
  ```

  returning every field except `Query`.
  `buildOptions` becomes a thin wrapper that calls `baseOptions` and sets `Query`;
  `assert-no-callers` calls `baseOptions` directly, since it resolves its query separately and reuses one `baseOpts` value across its Definition and References calls.
  Both helpers live next to each other at the bottom of `internal/scoutcli/cli.go`, where `resolveWorktreeRoot`/`buildOptions` sit today.
- **Rationale:** today's empty `WorktreeRoot` out-of-hub is a latent bug — `DaemonStateFile("")` yields a relative path resolved against the process working directory, so an out-of-hub `assert-no-callers` can spawn or dial a daemon in a different place than `refs`/`definition`/`symbol` would.
  It is the "one hand-built literal" the task body already names; folding the fix in is the natural close.
  The `baseOptions` extraction is what makes the fix testable: the literal is a closure-local value inside `RunE` today, reachable by no unit test, which is precisely why the empty-root bug survived.
- **Rejected:** preserving the empty-`Layout` behaviour verbatim as out of scope — keeps the bug and forces nil-handling in the engine, contradicting `no-nil-layout-check`.
  Also rejected: leaving the literal in place and merely adding a `Layout` field to it — fixes the value but keeps the construction site untestable, so the next divergence between `assert-no-callers` and the other three commands goes unnoticed the same way this one did.

### constructoranchoring-test-in-scope

- **Decision:** `cmd/lyx/constructoranchoring_test.go` changes in the same commit, re-signaturing its four accessor calls (lines 80-81, 127-128) to pass a `*lyxcwd.Location`.
  Its expected values and its `.lyx`-group comments stay unchanged.
- **Rationale:** the tree does not compile otherwise — that is the atomicity the task body already argues for.
  Because `anchor-stays-worktreepath` keeps `WorktreePath()` anchoring, this is a pure signature change: both tests already have `l` in scope (`l := anchoringFixture(...)`), so `scoutengine.DaemonStateFile(worktree, "go")` becomes `scoutengine.DaemonStateFile(l, "go")` with the same expected path.
  The test then also becomes the place the anchoring decision is machine-pinned.
- **Rejected:** nothing viable — the file cannot be left uncompiled.

### no-constraints-note

- **Decision:** do **not** add a `CONSTRAINTS.md` note stating that `.lyx`-ephemeral constructors anchor on `WorktreePath()`.
- **Rationale:** that rule is real but temporary — the dotlyx-hygiene task is chartered to reverse it.
  Writing it into `CONSTRAINTS.md` now creates a second, contradicting source that the dotlyx task must find and clean up, on top of `constructoranchoring_test.go` which already encodes it.
  The `// TODO(dotlyx):` markers from `anchor-stays-worktreepath` carry the same information to the one reader who needs it, and disappear with the code they annotate.
- **Rejected:** adding the note — the rule currently lives only in `constructoranchoring_test.go`'s comments, which is a legitimate argument for writing it down, but it belongs to the task that will change it, not this one.

### docs-in-same-commit

- **Decision:** update exactly these six comment sites in the same commit, and nothing beyond them:

  1. `internal/scoutengine/doc.go:136-142` — "# The EnsureServer seam" spells the signature out as `ensureServer(ctx, lang, entry, targetDir, worktreeRoot, timeout)`.
  2. `internal/scoutengine/doc.go:215-219` — "# Daemon state and concurrency" describes the state as keyed "per `(worktreeRoot, lang)`", resolved "via this package's own `DaemonStateFile`/`DaemonLock`".
  3. `internal/scoutengine/doc.go:236` — "The daemon's socket path is a deterministic function of `(worktreeRoot, lang)`".
  4. `internal/scoutengine/ensureserver.go:1` — the file header opens "implements the `EnsureServer(lang, worktreeRoot) -> LSPConn` seam".
  5. `internal/scoutengine/ensureserver.go:301-304` — the inline comment repeating "a deterministic function of `(worktreeRoot, lang)`".
  6. `internal/scoutengine/daemonstate.go:5-7` (file header) plus the two accessor doc comments at `:38-41` and `:46-48`, all of which name `worktreePath` explicitly.

- **Rationale:** the `(worktreeRoot, lang)` *keying* prose survives the change on the merits — the key is still worktree-and-language — but every site spelling out a parameter named `worktreeRoot` becomes wrong, and sites 1, 3, 4 and 5 were missed by an earlier, narrower reading of this decision.
  Sites 3 and 5 are the same sentence duplicated between `doc.go` and `ensureserver.go`;
  they must stay consistent with each other.
  No `manifest/designs/` doc covers the scoutengine module (`scout-plan-symbol-fields.md` is a different subject), so there is nothing outside the package to update.
  Known stale reference, deliberately **not** fixed here: `ensureserver.go:1-2` cites `manifest/designs/scout-redesign.md`, which does not exist in the tree.
  That is pre-existing rot unrelated to this refactor;
  leave it, so this commit stays a pure signature change.
- **Rejected:** touching `CONSTRAINTS.md` (see `no-constraints-note`), `manifest/roadmap.md`, or `docs/overview.md` — none of the Documentation Lifecycle triggers fire for a signature refactor with no observable behaviour change.

## Technical context

### The two accessors today

`internal/scoutengine/daemonstate.go:38-51`:

```go
func DaemonStateFile(worktreePath, lang string) string {
	return filepath.Join(worktreePath, dotLyxDirName, scoutDirName, lang, "daemon.json")
}

func DaemonLock(worktreePath, lang string) string {
	return filepath.Join(worktreePath, dotLyxDirName, scoutDirName, lang, "daemon.lock")
}
```

`dotLyxDirName = ".lyx"` (line 31) and `scoutDirName = "scout"` (line 36) are package-private consts and stay exactly as they are.
Only the first parameter changes: `worktreePath string` → `l *lyxcwd.Location`, with `l.WorktreePath()` substituted for `worktreePath` in the `filepath.Join`.

The precedent to mirror is `internal/logger/sink.go:31-38` (`WorktreeLogsDir`), including its doc-comment shape — it states both the anchoring choice and the `.lyx`-vs-`_lyx` ephemerality reason.

### The threading chain

| Site | Today | After |
| --- | --- | --- |
| `internal/scoutengine/refs.go:45-52` | `Options{… WorktreeRoot string …}` | `Options{… Layout *lyxcwd.Location …}` |
| `internal/scoutengine/refs.go:65` | `ensureServer(ctx, lang, entry, opts.TargetDir, opts.WorktreeRoot, opts.Timeout)` | `…, opts.TargetDir, opts.Layout, opts.Timeout)` |
| `internal/scoutengine/ensureserver.go:54` | `ensureServer(…, targetDir, worktreeRoot string, timeout …)` | `ensureServer(…, targetDir string, layout *lyxcwd.Location, timeout …)` |
| `internal/scoutengine/ensureserver.go:66` | `ensureSupervised(ctx, command, lang, targetDir, worktreeRoot, timeout)` | `…, targetDir, layout, timeout)` |
| `internal/scoutengine/ensureserver.go:298` | `ensureSupervised(…, lang, targetDir, worktreeRoot string, timeout …)` | `…, lang, targetDir string, layout *lyxcwd.Location, timeout …)` |
| `internal/scoutengine/ensureserver.go:299-300` | `DaemonStateFile(worktreeRoot, lang)` / `DaemonLock(worktreeRoot, lang)` | `DaemonStateFile(layout, lang)` / `DaemonLock(layout, lang)` |

`ensureNative` (`ensureserver.go:69`) takes no worktree root and is untouched.
`socketPath` stays derived as `filepath.Join(filepath.Dir(statePath), "daemon.sock")` (`ensureserver.go:305`) — unchanged, since `statePath` is unchanged in value.
The comment at `ensureserver.go:301-304` says the socket path is "a deterministic function of `(worktreeRoot, lang)`"; that stays true, but the wording should follow the parameter rename.

### The CLI side

`internal/scoutcli/cli.go:474-500` holds both helpers:

- `resolveWorktreeRoot(cwd, targetDir string) string` → becomes `resolveLocation(cwd, targetDir string) *lyxcwd.Location`.
  In-hub branch returns the resolved `layout` directly instead of `layout.WorktreePath()`;
  out-of-hub branch returns the synthesized `Location` per the `out-of-hub-synthesized-location` decision.
  Note the existing inner error path: when `filepath.Abs(targetDir)` itself fails, today's code returns the bare `targetDir`.
  Preserve that (synthesize from `targetDir` instead of `abs`) rather than silently changing it.
  Its doc comment must carry the consumed-by-`WorktreePath()`-only limit from the `out-of-hub-synthesized-location` decision.
- `buildOptions(registry, targetDir, worktreeRoot, lang string, query, timeout)` → split in two, per `fix-assert-no-callers-literal`:
  `baseOptions(registry scoutengine.Registry, targetDir string, layout *lyxcwd.Location, lang string, timeout time.Duration) scoutengine.Options` sets every field except `Query`;
  `buildOptions(…, query scoutengine.Query, …)` calls `baseOptions` and sets `Query`.
  The six existing `buildOptions` call sites keep calling `buildOptions` with the `worktreeRoot` argument swapped for `layout`.

Three commands each resolve once and call `buildOptions` twice (single-arg path and batch path):

| Command | Resolve site | `buildOptions` sites |
| --- | --- | --- |
| `refs` | `cli.go:151` | `cli.go:189`, `cli.go:204` |
| `definition` | `cli.go:297` | `cli.go:335`, `cli.go:350` |
| `symbol` | `cli.go:418` | `cli.go:437`, `cli.go:460` |

`assert-no-callers` is the fourth, and the odd one out.
At `cli.go:575-585` it inlines its own resolution *inside* the registry-loading `if layout, resolveErr := lyxcwd.Resolve(cwd); resolveErr == nil` block, setting `worktreeRoot = layout.WorktreePath()` there and leaving it empty otherwise;
then it builds `scoutengine.Options` by hand at `cli.go:593-599`.
Note the coupling: the same `if` both loads the registry overlay and resolves the root.
The fix must resolve the `Location` via `resolveLocation` independently of the registry-load branch — the other three commands already keep these two derivations separate (see the comment at `cli.go:147-150`) — and then build `Options` via `baseOptions`.
A direct `buildOptions` swap is not possible: the literal carries no `Query`, because the command resolves its query separately and reuses one `baseOpts` value across both its Definition and References calls.
That is exactly why `fix-assert-no-callers-literal` splits `baseOptions` out — `assert-no-callers` calls it directly, `buildOptions` calls it and adds `Query`.

The `resolveWorktreeRoot` doc comment is referenced by three comment blocks (`cli.go:147-150`, `295-296`, `416-417`) that say "why it never leaves `WorktreeRoot` empty outside a hub".
Those need rewording for the rename, and the claim becomes true of `assert-no-callers` too.

### Affected test files (nine, not five)

Untagged (Tier 1, compiled by every `go test ./...`):

1. `internal/scoutengine/scoutdaemon_test.go` — the dedicated accessor tests; every case builds `worktreePath` and calls both accessors (lines 11-63).
2. `internal/scoutengine/supervised_test.go` — three `worktreeRoot := t.TempDir()` fixtures at lines 65-66, 129-130, 207-208.
3. `internal/scoutengine/ensureserver_test.go` — `DaemonLock(worktreeRoot, "go")` at line 353.
4. `internal/scoutcli/cli_test.go` — `TestResolveWorktreeRoot_OutsideHubFallsBackToAbsoluteTargetDir` (lines 508-525) and the `buildOptions` field-threading test (lines 536-551).
5. `cmd/lyx/constructoranchoring_test.go` — lines 80-81 and 127-128.

Behind `//go:build scout`:

6. `internal/scoutengine/supervised_scout_test.go` — lines 25, 86.
7. `internal/scoutengine/supervised_integration_test.go` — line 54.
8. `internal/scoutengine/refs_integration_test.go` — `DaemonStateFile` at lines 82, 197, 235 and `Options{… WorktreeRoot …}` literals at lines 91, 206, 244.
9. `internal/scoutengine/ensureserver_integration_test.go` — line 140.

**The `scout` build tag is compiled by no pipeline gate.**
`go build ./...` and `go test ./...` both skip files 6-9 entirely, so a missed call site in them fails nothing and is invisible until someone runs the scout suite by hand.
This is the single largest correctness risk in the task.

### Test fixture shape

Every affected test currently uses a `t.TempDir()` (or a fixed fake path) as the worktree root.
The replacement is a hand-built `Location` whose `WorktreePath()` equals that same directory:

```go
dir := t.TempDir()
l := &lyxcwd.Location{HubPath: filepath.Dir(dir), WorktreeName: filepath.Base(dir), AnchorRel: "."}
```

This spawns no git and copies no fixture tree, so the untagged files stay Tier 1-pure per the Test Tier Purity Invariant and need no `TestMain`/`HermeticGitEnv` addition per the Hermetic Git Test Environment Invariant.
`cmd/lyx/constructoranchoring_test.go` already has its own `anchoringFixture` helper and an `l` in scope in both tests — reuse those, do not add a second helper.
`internal/websterengine/audit_test.go:24` and `template_test.go:42` show the same hand-built-`Location` idiom if a shape reference is wanted.

### Import-graph safety

`internal/lyxcwd` imports only stdlib plus `internal/gitexec`, so adding it to `scoutengine` creates no cycle.
The Scout Engine-Seam Invariant polices a *banned* list (`internal/output`, `cobra`, any `internal/*cli`, `internal/clihelp`), not an allowlist — `internal/lyxcwd` is not on it, and `scoutengine` already imports `internal/logger`, `internal/lock`, and `internal/proc` from the shared-infrastructure layer.
The one hard constraint is the file-scoped guard: `internal/scoutengine/lspclient.go` must keep importing stdlib plus `internal/logger` and nothing else (`lspclient_guard_test.go`).

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant.**
  The invariant this task exists to satisfy.
  Relevant bullets: `root` always means the git worktree/repo root, never `cwd` (which is why `Options.WorktreeRoot` is a misnomer out-of-hub and gets renamed);
  a module's own subdirectory is that module's own private relative-path constant joined onto a `Location` accessor directly, never a `lyxcwd` function call (so `dotLyxDirName`/`scoutDirName` stay in `scoutengine`);
  `lyxcwd.Resolve` exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()`, `AnchorPath()`.
  Enforced by `internal/lyxcwd/enforcement_test.go`.
- **Scout Engine-Seam Invariant.**
  Banned-list, direct imports only: `internal/output`, `cobra`, any `internal/*cli`, `internal/clihelp`.
  Plus the narrower file-scoped guard on `lspclient.go` (stdlib + `internal/logger` only).
  Enforced by `internal/scoutengine/seam_enforcement_test.go` and `internal/scoutengine/lspclient_guard_test.go`.
- **Test Tier Purity Invariant.**
  Untagged test files spawn nothing — no `gitexec.RunGit`, no `exec.Command`, no `lyxtest.Copy*`.
  The hand-built-`Location` fixture shape above is what keeps files 1-5 compliant.
  Enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant.**
  Only binds packages whose tests spawn git.
  No new `TestMain` is needed as long as the fixture shape above is used.
  Enforced by `cmd/lyx/hermeticenv_test.go`.
- **CLI/Cobra Invariant.**
  No command surface changes here — no new flags, no changed `Short`/`Long`, no envelope changes.
  Confirm the help tree is untouched;
  `cmd/lyx/helptree_test.go` and `drift_test.go` will catch a slip.
- **Documentation Lifecycle.**
  Satisfied by the `docs-in-same-commit` decision.

Discovered during discussion:

- The three-group anchoring table pinned by `cmd/lyx/constructoranchoring_test.go` (`_lyx`-durable → `AnchorPath`;
  `.lyx`-ephemeral → `WorktreePath`;
  `HubLogsDir` → `HubPath`) is a recorded Shared Decision.
  It is not in `CONSTRAINTS.md` and, per `no-constraints-note`, must not be added there by this task.
- Every resolved path must be byte-identical before and after the change, in-hub and out-of-hub, **with exactly one intended exception**: out-of-hub `assert-no-callers`, whose daemon-state path moves from process-cwd-relative to `filepath.Abs(targetDir)`-rooted per `fix-assert-no-callers-literal`.
  That single delta is the point of the fix, not a regression — do not read the byte-identical rule as grounds to back it out.
  If any expected value in any of the nine *existing* test files has to change, something is wrong with the implementation — treat it as a signal, not a test to update.
  No existing test covers the out-of-hub `assert-no-callers` path, which is why the delta touches no existing expected value.

## Testing

**TDD candidates** — the two new pin tests, both untagged, both in `internal/scoutcli/cli_test.go`:

1. **Out-of-hub `resolveLocation`.**
   Replaces `TestResolveWorktreeRoot_OutsideHubFallsBackToAbsoluteTargetDir` (`cli_test.go:508`).
   Assert the returned `Location`'s `WorktreePath()` equals `filepath.Abs(targetDir)` — the same expected value the current test asserts, so it proves byte-identical behaviour across the refactor rather than merely testing the new code.
   Worth also asserting `AnchorRel == "."`, since a synthesized non-`.` anchor would silently move the daemon state.
2. **`assert-no-callers` builds a non-nil `Layout`.**
   No such test exists today, which is why the empty-root bug survived.
   The test calls `baseOptions` — the seam decided in `fix-assert-no-callers-literal` — and asserts that for a given out-of-hub `(cwd, targetDir)` the returned `Options.Layout.WorktreePath()` equals what `buildOptions` yields for the same inputs, not merely that `Layout` is non-nil.
   Equality with the other three commands is the property that matters: a non-nil-but-different `Layout` would still point `assert-no-callers` at a different daemon than `refs`/`definition`/`symbol`.
   The `baseOptions` extraction is part of the fix, not test-only scaffolding.

**Adapted existing tests** — signature changes only, expected values untouched:

- `internal/scoutengine/scoutdaemon_test.go`.
  The per-language distinctness cases (`TestDaemonStateFile_DistinctPerLanguage`, `TestDaemonLock_DistinctPerLanguage`) are the ones that would catch a wrong `filepath.Join` argument order.
- `internal/scoutengine/supervised_test.go`, `ensureserver_test.go`.
- `internal/scoutcli/cli_test.go`'s `buildOptions` field-threading test — retarget the `WorktreeRoot` assertion at line 541 onto `Layout`, comparing `got.Layout.WorktreePath()` rather than the struct, so the test survives any future `Location` field addition.
- `cmd/lyx/constructoranchoring_test.go` — pass `l`;
  both the unanchored and subpath-anchored cases must keep asserting the same `dotLyxBase`-derived paths.
  `TestConstructorAnchoring_SubpathAnchored` is the machine pin on the `anchor-stays-worktreepath` decision: if it starts failing, the implementation drifted to `AnchorPath()`.
- The four `//go:build scout` files — signature changes only.

**Verification sequence:**

```
go build ./...
go test ./...
go vet -tags scout ./internal/scoutengine/...
go test ./cmd/lyx/ -run TestConstructorAnchoring
```

The `go vet -tags scout` run is not optional and not redundant: it is the only thing that proves the four `scout`-tagged test files still compile, since no pipeline gate builds that tag.

**Deliberately not run:** the `scout`-tagged tests themselves.
They spawn a real gopls, are slow and environment-dependent, and this refactor changes no runtime behaviour they would catch beyond compilation — which `go vet -tags scout` already covers.

## Q&A log

- **Q:** Which anchor do `DaemonStateFile`/`DaemonLock` join onto — `WorktreePath()` or the body's `AnchorPath()`? **A:** `WorktreePath()`, keeping today's anchoring and the `logger.WorktreeLogsDir` precedent. Not because it is right forever, but because the `.lyx` `WorktreePath` → `AnchorPath` migration is the dotlyx-hygiene task's own scoped mandate — doing it piecemeal for one module is worse than not doing it. Leave `// TODO(dotlyx):` markers so that task finds these two functions.
- **Q:** Does `Options` carry `*lyxcwd.Location` or keep `WorktreeRoot string`? **A:** `Layout *lyxcwd.Location` — the whole point of the task. `TargetDir` stays a separate field; `--target-dir` is an independent user flag.
- **Q:** Out of a hub, how is `Options.Layout` built? **A:** Synthesize `&lyxcwd.Location{HubPath: filepath.Dir(abs), WorktreeName: filepath.Base(abs), AnchorRel: "."}` from `filepath.Abs(targetDir)`. Byte-identical to today's fallback, no new failure modes.
- **Q:** `assert-no-callers` leaves `WorktreeRoot` empty out-of-hub. Fix or preserve? **A:** Fix — route it through the same resolver. The empty root is a latent bug (`DaemonStateFile("")` yields a path relative to the process cwd); fold the fix in rather than preserving it.
- **Q:** Is `cmd/lyx/constructoranchoring_test.go` in scope, given the body's scope paragraph excludes it? **A:** In scope, same commit — the tree does not compile otherwise. Because the anchor stays `WorktreePath()`, only the accessor calls are re-signatured; no assertion or anchoring logic changes.
- **Q:** How far does the `Location` travel inside `scoutengine`? **A:** All the way down to `ensureSupervised`, typed, which calls both accessors itself. Resolving the paths higher up (in `acquireConnection`) would reintroduce the same split ownership one layer up.
- **Q:** What happens if `Options.Layout` is nil? **A:** Nothing defensive — document it as required, no nil check, matching the `websterengine.RunDeps.Layout` precedent. `scoutcli` always supplies one.
- **Q:** Field named `Layout` or `Location`? **A:** `Layout` — one vocabulary with `websterengine`, `buildercli`, `webstercli`, `perchengine`.
- **Q:** Should `CONSTRAINTS.md` record that `.lyx`-ephemeral constructors anchor on `WorktreePath()`? **A:** No. That rule is owned by the dotlyx task that will reverse it; writing it down now just creates a contradicting source that task has to clean up.
- **Q:** How is this verified, given four test files are behind an ungated build tag? **A:** `go build ./...`, `go test ./...`, an explicit `go vet -tags scout ./internal/scoutengine/...`, and `go test ./cmd/lyx/ -run TestConstructorAnchoring`, plus the two new pin tests. Do not run the `scout`-tagged tests — slow, environment-dependent, and they catch nothing beyond compilation for this change.
- **Q:** The byte-identical rule forbids observable behaviour changes, but the `assert-no-callers` fix *is* one. Which wins? **A:** [auto-pick, review r1] The fix wins, and the rule is restated with an explicit carve-out naming out-of-hub `assert-no-callers` as the single intended delta. **Why:** an unqualified byte-identical rule reads as grounds to back the fix out, which would preserve the bug the task set out to close; no existing test covers that path, so the delta touches no expected value.
- **Q:** What seam does the `assert-no-callers` pin test target, given the `Options` literal is closure-local inside `RunE`? **A:** [auto-pick, review r1] Extract `baseOptions(registry, targetDir, layout, lang, timeout)` returning every field but `Query`; `buildOptions` wraps it, `assert-no-callers` calls it directly, and the test calls `baseOptions`. **Why:** leaving the seam to mill-plan left the pin test's target undefined; the literal has no `Query`, so a plain `buildOptions` swap was never possible.
- **Q:** Is the `docs-in-same-commit` list complete? **A:** [auto-pick, review r1] No — it missed four `worktreeRoot` mentions. Enumerated to six sites: `doc.go:136-142`, `doc.go:215-219`, `doc.go:236`, `ensureserver.go:1`, `ensureserver.go:301-304`, and `daemonstate.go:5-7` plus both accessor comments. **Why:** the decision said "nothing else" while Technical context separately demanded an `ensureserver.go:301-304` reword — a direct contradiction a plan writer would have had to guess at. The stale `manifest/designs/scout-redesign.md` citation at `ensureserver.go:1-2` is pre-existing rot and stays.
- **Q:** Does a nil `Options.Layout` really fail fast? **A:** [auto-pick, review r1] Only for Go. Recorded as an accepted weakness rather than mitigated. **Why:** `Layout` is dereferenced only in `ensureSupervised`, reached only when `entry.HasNativeDaemon` (`refs.go:64-65`), so a nil on Python/C#/TypeScript/Rust is silently unused. A nil check would only make a can't-happen error louder for four languages that ignore the field.
- **Q:** Is the synthesized out-of-hub `Location` safe to treat as a real `Location`? **A:** [auto-pick, review r1] No — only `WorktreePath()` is a true answer; `HubPath`, `RepoName`, and `AnchorPath()` are fictions. Contractually consumed by `DaemonStateFile`/`DaemonLock` alone, stated in `resolveLocation`'s doc comment. **Why:** hand-building a `Location` outside `internal/lyxcwd` has only two production precedents, both inside the geometry owner (`fabricengine/clone.go:125`, `hostlayout.go:30`); an unmarked fiction invites a later caller to read a field that means nothing.
