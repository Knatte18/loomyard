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
- `internal/scoutcli/cli.go` — `resolveWorktreeRoot` → `resolveLocation`; one new helper (`lookupContext`); `buildOptions` signature; six `buildOptions` call sites; the four per-command pre-flight blocks collapsed onto `lookupContext`; the `assert-no-callers` hand-built literal replaced by a seventh `buildOptions` call.
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

- **Rationale:** `WorktreePath()` is `filepath.Join(HubPath, WorktreeName)`, so this yields exactly `abs` — byte-identical to today's `resolveWorktreeRoot` fallback for every ordinary directory.
  No new failure modes, no new git spawns, and the two branches of `resolveWorktreeRoot` map one-to-one onto the two branches of the new `resolveLocation`.
  **Scope of the byte-identity claim:** it is asserted for non-root directories.
  A filesystem or volume root (`/`, `C:\`) is a degenerate case — `filepath.Base("/")` is `"/"`, `filepath.Base("C:\\")` is `"\\"` — where the `Dir`/`Base`-then-`Join` round trip is not obviously an identity and was not verified.
  Not worth handling: `--target-dir /` names no buildable project, and the pre-existing `resolveWorktreeRoot` fallback has the same shape.
  Do not add a special case for it;
  do not claim byte-identity there either.
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

- **Decision:** one extraction, `lookupContext` — the seam that owns the buggy derivation.

  1. **`lookupContext`, the resolution seam.**

     ```go
     func lookupContext(cwd, dir string) (scoutengine.Registry, *lyxcwd.Location, error)
     ```

     It performs both pre-flight derivations every lookup command needs — the `servers.yaml` overlay load and the `*lyxcwd.Location` resolution — deriving each independently from `(cwd, dir)`, and returns the built-in registry plus a synthesized `Location` out-of-hub.

     **`dir` is the already-defaulted directory, never the raw `--target-dir` flag.**
     The parameter is named `dir`, not `targetDir`, for exactly this reason.
     Each `RunE` keeps its existing two lines of defaulting (`dir := targetDir; if dir == "" { dir = cwd }`, `cli.go:142-145` and `:570-573`) and passes the result in — the defaulting does **not** move into `lookupContext`, because `Options.TargetDir` and `filterWithin` in the same closure need `dir` anyway, so returning it as a third value would just hand back a value the caller already has.
     Passing the raw flag instead would make the out-of-hub branch resolve `filepath.Abs("")` — the process working directory — rather than `filepath.Abs(cwd)`, silently pointing the daemon at the wrong place whenever `--target-dir` is omitted.

     **Error contract:** the returned `error` carries `LoadRegistry` failures only.
     A `lyxcwd.Resolve` failure is *never* an error — it is the out-of-hub path, and degrades to the built-in registry plus a synthesized `Location`, exactly as today.
     Callers keep the existing envelope mapping unchanged: on a non-nil error, `clihelp.SetExit(ctx, output.Err(out, err.Error()))` then `return nil` (`cli.go:162-165`, `:579-582`).
     All four commands call it, replacing the duplicated blocks at `cli.go:147-167`, `cli.go:293-313`, `cli.go:414-434`, and `cli.go:575-585`.
     `resolveLocation` becomes its private helper rather than a separately-called function.
  2. **`assert-no-callers`' hand-built literal is replaced by a plain `buildOptions` call** — no second helper.
     The command parses `query` at `cli.go:587`, *before* building the literal at `:593`, and both derived values set the identical `Query`: `defOpts.Query = query` (`:613`) and `refOpts.Query = query` (`:621`).
     One `buildOptions(registry, dir, layout, lang, query, timeout)` value is therefore byte-equivalent to today's `baseOpts`-plus-two-assignments, and `defOpts`/`refOpts` collapse to that single value.

  `lookupContext` lives at the bottom of `internal/scoutcli/cli.go`, next to `resolveLocation` and `buildOptions`.
- **Rationale:** today's empty `WorktreeRoot` out-of-hub is a latent bug — `DaemonStateFile("")` yields a relative path resolved against the process working directory, so an out-of-hub `assert-no-callers` can spawn or dial a daemon in a different place than `refs`/`definition`/`symbol` would.
  It is the "one hand-built literal" the task body already names; folding the fix in is the natural close.
  **Why `lookupContext` and not a construction-side helper:** the defect is not in how the `Options` value is *assembled*, it is in how `assert-no-callers` *resolves* its root — the resolution is nested inside the registry-load `if` at `cli.go:577-585`, so it silently yields `""` on the else path.
  A helper that takes an already-resolved `layout` as a parameter and returns it inside an `Options` cannot observe that: asserting the value comes back out is true by construction.
  `lookupContext` owns the buggy derivation itself, so a test against it exercises the real defect.
  Once the resolution is fixed and shared, the existing `buildOptions` is a sufficient construction site — there is no second extraction.
- **Rejected:** preserving the empty-`Layout` behaviour verbatim as out of scope — keeps the bug and forces nil-handling in the engine, contradicting `no-nil-layout-check`.
  Also rejected: leaving the literal in place and merely adding a `Layout` field to it — fixes the value but leaves `assert-no-callers` on its own resolution path, so the next divergence from the other three commands goes unnoticed the same way this one did.
  Also rejected: a `baseOptions(…)` helper returning every field but `Query`, with `buildOptions` wrapping it.
  Considered and dropped: the premise that `assert-no-callers` needs a `Query`-less `Options` is false — it parses `query` before building the literal and assigns the same `query` to both derived values, so a plain `buildOptions` call covers it.

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

- **Decision:** this decision governs `internal/scoutengine`'s **prose/package documentation** only.
  It does not bound the doc comments other decisions require on code this task introduces or re-signatures — those are listed separately below and are not exceptions to this list.

  Update exactly these six `scoutengine` comment sites:

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
- **Doc comments mandated by other decisions** (required, and outside the six-site list above — the list bounds prose docs, not the documentation of code this task writes):
  - `internal/scoutengine/refs.go` — a doc comment on `Options.Layout` stating it is **required** and must be non-nil, per `no-nil-layout-check`.
    `Options` has no per-field docs today (`refs.go:45-52`), so this is the first;
    document only `Layout`, not every field.
  - `internal/scoutcli/cli.go` — a doc comment on the new `lookupContext` helper, per `fix-assert-no-callers-literal`.
  - `internal/scoutcli/cli.go:489-490` — `buildOptions`' own doc comment, "ensuring all construction sites thread **WorktreeRoot** consistently".
    The function survives the change, so its comment survives too and becomes false;
    reword to `Layout`.
  - `internal/scoutcli/cli.go` — `resolveLocation`'s doc comment must carry the "only `WorktreePath()` is a true answer" limit, per `out-of-hub-synthesized-location`.
  - `internal/scoutcli/cli.go:147-150`, `:295-296`, `:416-417` — the three comment blocks referring to `resolveWorktreeRoot`'s doc comment and to "why it never leaves `WorktreeRoot` empty outside a hub".
    All three are subsumed by the `lookupContext` extraction, which deletes the blocks they annotate;
    reword or delete them accordingly, and note that the claim now holds for `assert-no-callers` too.
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
- `buildOptions(registry, targetDir, worktreeRoot, lang string, query, timeout)` → `buildOptions(registry scoutengine.Registry, targetDir string, layout *lyxcwd.Location, lang string, query scoutengine.Query, timeout time.Duration)`.
  Body and field set are otherwise unchanged.
  The six existing call sites swap the `worktreeRoot` argument for `layout`;
  `assert-no-callers` becomes a seventh.

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
The fix must resolve the `Location` via `lookupContext` — which derives the registry and the `Location` independently, the way the other three commands already do (see the comment at `cli.go:147-150`) — and then build `Options` via `buildOptions`.
The literal *looks* like it needs a `Query`-less constructor, but does not: `query` is parsed at `cli.go:587`, before the literal at `:593`, and `defOpts.Query` (`:613`) and `refOpts.Query` (`:621`) are both assigned that same `query`.
So `baseOpts`, `defOpts` and `refOpts` all collapse into a single `buildOptions(registry, dir, layout, lang, query, timeout)` value passed to both `Definition` and `References`.

The `resolveWorktreeRoot` doc comment is referenced by three comment blocks (`cli.go:147-150`, `295-296`, `416-417`) that say "why it never leaves `WorktreeRoot` empty outside a hub".
Those need rewording for the rename, and the claim becomes true of `assert-no-callers` too.

### Affected test files (nine, not five)

Each entry lists **every** site that must change, not only the accessor calls: a file that calls `DaemonStateFile` almost always also passes `worktreeRoot` into `ensureServer`/`ensureSupervised`, and those threaded call sites are just as compile-breaking.

Untagged (Tier 1, compiled by every `go test ./...`):

1. `internal/scoutengine/scoutdaemon_test.go` — the dedicated accessor tests; every case builds `worktreePath` and calls both accessors (lines 11-63).
   No threaded call sites.
2. `internal/scoutengine/supervised_test.go` — three `worktreeRoot := t.TempDir()` fixtures at lines **63, 127, 205**;
   accessor calls at 65-66, 129-130, 207-208;
   **three** `ensureSupervised(…, lang, worktreeRoot, worktreeRoot, …)` calls, at 96, 153 and 318 (318 sits under the third fixture, so that fixture has two consumers — the accessor calls at 207-208 and this one).
3. `internal/scoutengine/ensureserver_test.go` — fixture at 352, `DaemonLock` at 353, `ensureServer(ctx, "go", entry, t.TempDir(), worktreeRoot, …)` at 380.
4. `internal/scoutcli/cli_test.go` — `TestResolveWorktreeRoot_OutsideHubFallsBackToAbsoluteTargetDir` (lines 508-525) and the `buildOptions` field-threading test (lines 536-551).
5. `cmd/lyx/constructoranchoring_test.go` — lines 80-81 and 127-128.

Behind `//go:build scout`:

6. `internal/scoutengine/supervised_scout_test.go` — fixtures at 23, 84;
   accessor calls at 25, 86;
   `ensureSupervised(…, worktreeRoot, worktreeRoot, …)` at 41, 91.
7. `internal/scoutengine/supervised_integration_test.go` — fixture at 53, accessor call at 54, `ensureSupervised(…, root, worktreeRoot, …)` at 60, 91, 114.
8. `internal/scoutengine/refs_integration_test.go` — `DaemonStateFile` at 82, 197, 235 and `Options{… WorktreeRoot …}` literals at 91, 206, 244.
9. `internal/scoutengine/ensureserver_integration_test.go` — fixture at 139, accessor call at 140, `ensureServer(…, root, worktreeRoot, …)` at 145, 176.

Files 6, 7 and 9 are where the under-count would have bitten hardest: they carry seven threaded call sites between them, all invisible to `go build ./...` and `go test ./...`.

**Comment mentions in test files — reworded in this commit, not left alone.**
`docs-in-same-commit` is scoped to `scoutengine` *production* prose docs plus the mandated new code doc comments, so it does not reach test-file comments;
they are assigned here instead.
Eight comments name `WorktreeRoot`/`worktreeRoot` in prose and become false after the rename:

- `refs_integration_test.go:77`, `:178`, `:193`
- `ensureserver_integration_test.go:127`, `:136`, `:174`
- `supervised_integration_test.go:51`, `:89`

All eight are in `//go:build scout` files, so nothing compiles or reads them in CI — which is exactly why they need naming here rather than being left to be noticed.
Reword to the new vocabulary (`layout`, or "the worktree the `Location` resolves to") in the same commit.
Files 1-5 carry no such prose mentions beyond the code sites already listed.

**The `scout` build tag is compiled by no pipeline gate.**
`go build ./...` and `go test ./...` both skip files 6-9 entirely, so a missed call site in them fails nothing and is invisible until someone runs the scout suite by hand.
This is the single largest correctness risk in the task.

### Test fixture shape

Every affected test currently uses a `t.TempDir()` (or a fixed fake path) as the worktree root.
The replacement keeps **both** values — the plain string *and* a hand-built `Location` whose `WorktreePath()` equals it:

```go
dir := t.TempDir()
l := &lyxcwd.Location{HubPath: filepath.Dir(dir), WorktreeName: filepath.Base(dir), AnchorRel: "."}
```

The string is not redundant.
`ensureSupervised(ctx, command, lang, targetDir, layout, timeout)` keeps `targetDir` as a `string` — only the root becomes a `Location` — and several fixtures pass the same directory as both: `supervised_test.go:96,153,318` and `supervised_scout_test.go:41,91` all call `ensureSupervised(…, lang, worktreeRoot, worktreeRoot, …)` today.
Those become `ensureSupervised(…, lang, dir, l, …)`.
Do not delete the string variable when introducing `l`.

This spawns no git and copies no fixture tree, so files 1-3 and 5 stay Tier 1-pure per the Test Tier Purity Invariant and need no `TestMain`/`HermeticGitEnv` addition per the Hermetic Git Test Environment Invariant.

**File 4 (`internal/scoutcli/cli_test.go`) is the exception, and it is pre-existing.**
Its out-of-hub tests — the existing one and the new `lookupContext` pin — reach `lyxcwd.Resolve`, which shells out to `gitexec.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)` (`lyxcwd.go:143`).
`internal/scoutcli` has no `TestMain` and no `HermeticGitEnv` call today.
Both guards still pass — `tierpurity_test.go` matches direct tokens in the test file (there are none;
the spawn is two packages down) and `hermeticenv_test.go` keys on direct spawns or lyxtest fixture helpers (likewise none) — so this task introduces no new violation and must not add a `TestMain` as a drive-by.
What the new test *does* inherit is the existing test's unstated premise: the out-of-hub branch is only exercised if `t.TempDir()` does not itself sit inside a git repository.
That holds for an ordinary `TMPDIR` and is how the current test already passes.
Do not build the new test on a stronger assumption than that, and do not try to force the branch by other means.
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
2. **`lookupContext` resolves a usable `Location` out-of-hub.**
   No test exists today for this derivation, which is why the empty-root bug survived.
   The test calls `lookupContext` — the resolution seam decided in `fix-assert-no-callers-literal` — with a `cwd` outside any lyx hub, and asserts the returned `*lyxcwd.Location` is non-nil with `WorktreePath()` equal to `filepath.Abs(dir)`, and that the returned registry is the built-in one.
   `scoutengine.Registry` is a `map[string]Entry` (`registry.go:46`) and so is not `==`-comparable: assert the registry half with a keyed spot-check (the `"go"` entry matches `BuiltinRegistry()["go"]`) rather than `reflect.DeepEqual` over the whole map, which would couple the test to every future built-in entry.
   This is the test that observes the actual defect: today the equivalent derivation inside `assert-no-callers`' `RunE` closure (`cli.go:577-585`) yields `""` on exactly this path.
   Because all four commands now share `lookupContext`, one test covers all four.

   **Explicitly not written:** a test asserting `buildOptions(…, layout, …).Layout == layout`.
   `buildOptions` takes `layout` as a parameter and copies it into a struct field, so such an assertion is true by construction and proves nothing about the resolution that was actually broken.
   The field-mapping half is already covered by the existing field-threading test (below), which is honest about being only a field-mapping check.

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
- **Q:** What seam does the `assert-no-callers` pin test target, given the `Options` literal is closure-local inside `RunE`? **A:** [auto-pick, review r1 — **superseded twice, see r2 and r5 below**] Extract `baseOptions(registry, targetDir, layout, lang, timeout)` returning every field but `Query`. **Why it was wrong:** the seam is on the construction side, which cannot observe the defect (r2), and its premise that the literal has no `Query` is false (r5). Recorded here because two later entries only make sense against it.
- **Q:** Is the `docs-in-same-commit` list complete? **A:** [auto-pick, review r1] No — it missed four `worktreeRoot` mentions. Enumerated to six sites: `doc.go:136-142`, `doc.go:215-219`, `doc.go:236`, `ensureserver.go:1`, `ensureserver.go:301-304`, and `daemonstate.go:5-7` plus both accessor comments. **Why:** the decision said "nothing else" while Technical context separately demanded an `ensureserver.go:301-304` reword — a direct contradiction a plan writer would have had to guess at. The stale `manifest/designs/scout-redesign.md` citation at `ensureserver.go:1-2` is pre-existing rot and stays.
- **Q:** Does a nil `Options.Layout` really fail fast? **A:** [auto-pick, review r1] Only for Go. Recorded as an accepted weakness rather than mitigated. **Why:** `Layout` is dereferenced only in `ensureSupervised`, reached only when `entry.HasNativeDaemon` (`refs.go:64-65`), so a nil on Python/C#/TypeScript/Rust is silently unused. A nil check would only make a can't-happen error louder for four languages that ignore the field.
- **Q:** Does a construction-side pin test actually observe the `assert-no-callers` bug? **A:** [auto-pick, review r2] No — it is tautological. Introduced `lookupContext(cwd, dir) (Registry, *lyxcwd.Location, error)`, which owns the buggy derivation; the pin test targets that, and the identity assertion is explicitly not written. **Why:** any helper taking `layout` as a parameter returns it by construction. The real defect is the resolution nested inside the registry-load `if` at `cli.go:577-585`, which yields `""` on the else path — nothing in the round-1 test shape reached it.
- **Q:** Is a second, construction-side extraction still needed alongside `lookupContext`? **A:** [auto-pick, review r5] No — dropped. `assert-no-callers` calls the existing `buildOptions` directly, as a seventh call site. **Why:** the justification for a `Query`-less helper was that the command needs an `Options` without a `Query`. False: `query` is parsed at `cli.go:587`, before the literal at `:593`, and `defOpts.Query` (`:613`) and `refOpts.Query` (`:621`) both take that same `query` — so one `buildOptions` value serves both calls, and the helper earned nothing.
- **Q:** Does `docs-in-same-commit`'s "exactly six sites and nothing beyond them" hold? **A:** [auto-pick, review r2] No — it contradicted three other decisions that mandate doc comments in `scoutcli` and on `Options.Layout`. Rescoped the six-site list to bind `scoutengine` prose docs only, with the mandated code doc comments listed separately. **Why:** an absolute closure on one list while other decisions demand comments elsewhere forces a plan writer to guess which mandate wins.
- **Q:** Is the affected-test enumeration complete? **A:** [auto-pick, review r2] No — it listed accessor calls only and missed eleven `ensureServer`/`ensureSupervised` call sites, seven of them in the `//go:build scout` files. Every site is now enumerated per file, and the drifted line numbers for `supervised_test.go`'s fixtures (63/127/205, not 65/129/207) are corrected. **Why:** the under-count fell precisely on the files the discussion itself calls the largest correctness risk, since no pipeline gate compiles them.
- **Q:** Does the new `Location` fixture replace the plain directory string in tests? **A:** [auto-pick, review r2] No — it must keep both. `targetDir` stays a `string` parameter, and five fixtures pass the same directory as both arguments (`ensureSupervised(…, worktreeRoot, worktreeRoot, …)`). **Why:** deleting the string when introducing `l` breaks those call sites.
- **Q:** Does `lookupContext` take the raw `--target-dir` flag or the defaulted directory? **A:** [auto-pick, review r3] The already-defaulted directory; the parameter is named `dir` to say so, and the `dir == "" → cwd` defaulting stays in each `RunE`. **Why:** passing the raw flag would resolve `filepath.Abs("")` — the process cwd — instead of `filepath.Abs(cwd)` whenever `--target-dir` is omitted. The defaulting cannot move into the helper for free: `Options.TargetDir` and `filterWithin` need `dir` in the same closure anyway.
- **Q:** What populates `lookupContext`'s error return? **A:** [auto-pick, review r3] `LoadRegistry` failures only. A `lyxcwd.Resolve` failure is never an error — it is the out-of-hub path. Callers keep the existing `output.Err` + `return nil` mapping. **Why:** leaving the contract unstated invites a plan writer to turn the out-of-hub degradation into a hard failure, which would break every out-of-hub lookup.
- **Q:** Is the out-of-hub byte-identity claim unconditional? **A:** [auto-pick, review r3] No — scoped to non-root directories. The `Dir`/`Base`-then-`Join` round trip at a filesystem or volume root (`/`, `C:\`) was not verified and is deliberately not special-cased. **Why:** `--target-dir /` names no buildable project, and today's `resolveWorktreeRoot` fallback has the same shape — but an unqualified claim would read as verified when it is not.
- **Q:** Does `buildOptions`' own doc comment need rewording? **A:** [auto-pick, review r5] Yes — `cli.go:489-490` says it ensures every construction site threads `WorktreeRoot`; the function survives, so the comment survives and becomes false. Added to the mandated-comment list. **Why:** it fell between the `scoutengine`-scoped six-site list and the mandated-new-code list, so no decision claimed it.
- **Q:** How many `ensureSupervised` call sites does `supervised_test.go` have? **A:** [auto-pick, review r5] Three (96, 153, 318), not four. **Why:** the third fixture has two consumers, which I miscounted as a fourth call — an internal inconsistency in the very enumeration this discussion bills as its largest correctness risk.
- **Q:** Who owns the test-file comments that name `WorktreeRoot`/`worktreeRoot` in prose? **A:** [auto-pick, review r4] This commit — eight comments across the three `//go:build scout` integration test files, now enumerated per file. **Why:** `docs-in-same-commit` covers `scoutengine` production prose only, so nothing claimed them; and since no gate compiles those files, a false comment would sit there indefinitely.
- **Q:** How is "the registry is the built-in one" asserted, given `Registry` is a map? **A:** [auto-pick, review r4] A keyed spot-check on the `"go"` entry, not `reflect.DeepEqual` over the whole map. **Why:** maps are not `==`-comparable, and a whole-map compare would couple the test to every future built-in entry.
- **Q:** Do the new tests really spawn no git? **A:** [auto-pick, review r4] Files 1-3 and 5, yes. File 4 (`scoutcli/cli_test.go`) does, indirectly — `lyxcwd.Resolve` shells `git rev-parse --show-toplevel` — and `internal/scoutcli` has no `TestMain`. Pre-existing, both guards still pass, do not add a `TestMain` as a drive-by. **Why:** the unqualified no-git claim would have been wrong for the one file the new pin test lives in, and the out-of-hub premise silently depends on `TMPDIR` not being inside a repo.
- **Q:** Is the synthesized out-of-hub `Location` safe to treat as a real `Location`? **A:** [auto-pick, review r1] No — only `WorktreePath()` is a true answer; `HubPath`, `RepoName`, and `AnchorPath()` are fictions. Contractually consumed by `DaemonStateFile`/`DaemonLock` alone, stated in `resolveLocation`'s doc comment. **Why:** hand-building a `Location` outside `internal/lyxcwd` has only two production precedents, both inside the geometry owner (`fabricengine/clone.go:125`, `hostlayout.go:30`); an unmarked fiction invites a later caller to read a field that means nothing.
