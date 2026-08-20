# Discussion: Extract scout into its own standalone repo

```yaml
task: Extract scout into its own standalone repo
slug: scout-extract-standalone-repo
status: discussing
parent: main
```

## Problem

`scout` — LSP-backed code intelligence, `internal/scoutengine` (7 011 lines across 35 files: 2 737 production, 4 274 test) plus `internal/scoutcli` (1 962 lines: 937 production, 1 025 test) — has grown into a tool with its own reason to exist.
It answers "where is this symbol referenced / defined / declared" across five languages, and nothing about that job is specific to lyx.
Keeping it inside Loomyard means every change to it moves through lyx's release, lyx's constraints, and lyx's module table, for a capability lyx itself barely uses.

**Why now.** Two things changed.
First, `scout-told-geometry` (Done) already removed scout's dependence on lyx-relative path derivation, so the extraction is no longer blocked on a refactor.
Second, the agent-usage benchmark ([`docs/research/scout-agent-usage-findings.md`](../docs/research/scout-agent-usage-findings.md)) found that a Claude Code agent free to pick its own tools gets comparable results from `grep`, which removes the argument for scout being a *lyx-agent* feature and leaves only the argument for it being a *standalone deterministic tool*.
Third, the intended next step — adopting Tree-sitter so the tool owns syntax trees rather than only querying language servers — is a large body of work that has no business landing inside lyx's repo.

The end state of this task is a working `quarry` binary in its own repo that does exactly what `lyx scout` does today, and a Loomyard with no scout in it.

## Scope

**In:**

- New repo `github.com/Knatte18/quarry`, already created and cloned to `/home/knatte/Code/quarry/wts/quarry` (empty, branch `main`, no initial commit).
- Move `internal/scoutengine` → `quarry/` (public core package, package name `quarry`) and `internal/scoutcli` → `internal/cli/`, plus `cmd/quarry/main.go`.
- Copy verbatim the three dependency-free shared packages scout uses (`lock`, `proc`, `output`) and inline the one `lyxdirs` constant that survives.
- Replace the five shared packages that would drag a transitive tail (`configengine`, `logger`, `clihelp`, `lyxcwd`, and the `gitkit`/`hubforge` test fixtures) with narrow local equivalents.
- New config and state path resolution, since neither a lyx hub nor `_lyx/` exists in quarry.
- Port the full test suite, including the `//go:build integration` tier separation.
- Delete `internal/scoutengine`, `internal/scoutcli`, and the `lyx scout` subcommand from Loomyard, plus every reference to them in `cmd/lyx/`, `CONSTRAINTS.md`, `docs/overview.md`, and `manifest/roadmap.md`.
- Move scout's four research/benchmark docs to quarry.

**Out:**

- **Tree-sitter.** Not in this task at all. The layout below is chosen so a Tree-sitter path can be added later without restructuring, but no Tree-sitter code, dependency, or interface is written here.
- **MCP wrapper.** Same — `cmd/quarry-mcp/` is a future sibling of `cmd/quarry/`, not part of this task.
- **The two known defects.** `assert-no-callers` false positives on interface methods and the missing `--build-tags` pass-through are filed as [quarry#1](https://github.com/Knatte18/quarry/issues/1) and [quarry#2](https://github.com/Knatte18/quarry/issues/2) and are moved unfixed. Behaviour must be byte-for-byte what it is today, defects included — that is what makes the move verifiable.
- **Any lyx-side replacement for `lyx scout`.** No shell-out, no optional binary detection, no vendored release artifact. The subcommand is deleted outright.
- **GitHub Actions.** Actions are not enabled on the quarry repo. The test-tier build tags are ported so CI is a config file away, but no workflow file is written.
- **Renaming or redesigning the CLI surface.** The four verbs (`refs`, `definition`, `symbol`, `assert-no-callers`), their flags, and their JSON envelope stay identical. Only the binary name changes: `lyx scout refs …` becomes `quarry refs …`.
- **`manifest/designs/scout-plan-symbol-fields.md`.** That is a loom design that happens to consume scout; it stays in Loomyard with its links repointed.

## Decisions

### repo-name-and-identity

- Decision: repo `github.com/Knatte18/quarry`, binary `quarry`, module path `github.com/Knatte18/quarry`. No `lyx`/`loomyard` prefix.
- Rationale: the tool must be presentable as standalone. A `lyx` prefix additionally collides with the LyX document processor and would read as "a lyx subcommand", which is the exact coupling being removed. GitHub names collide only within an account namespace, so `Knatte18/quarry` was available despite other Quarrys existing.
- Rejected: `lyxquarry`, `loomyard-quarry`, `code-quarry`.

### dependency-strategy-copy-vs-replace

- Decision: split the nine shared Loomyard packages scout imports into **copy verbatim** and **replace with a local equivalent**, on the criterion of whether the package has its own internal dependencies.
  - **Copy verbatim:** `internal/lock` (222 lines, no deps), `internal/proc` (329, no deps), `internal/output` (256, no deps).
  - **Inline:** `internal/lyxdirs` — only `DotLyxDirName` is used, and it disappears entirely once state paths move off `_lyx/` (see `config-and-state-paths`).
  - **Replace:** `internal/configengine` (1 552 lines, pulls `envsource`+`logger`+`lyxdirs`+`yamlengine`, but scout uses exactly one function, `ConfigFile`), `internal/logger` (1 939 lines, pulls `lyxcwd`+`lyxdirs`+`proc`, scout uses `Warn` and `Info`), `internal/clihelp` (923 lines, pulls `lyxcwd`+`output`, scout uses `SetExit`/`Execute`/`ExecuteIn`/`GroupRunE`), `internal/lyxcwd` (3 028 lines, CLI-only, uses `CwdFrom`/`Resolve`/`WithCwd`), and `internal/gitkit`+`internal/hubforge` (2 192 lines, test fixtures only, and `hubforge` pulls `fabricengine`).
- Rationale: the *declared* dependency is nine packages and ~11 500 lines; the *used surface* is 18 symbols. Copying naively drags `logger` → `lyxcwd` → `gitexec` and `hubforge` → `fabricengine`, which would make quarry a Loomyard clone. Copying only the leaf packages keeps genuinely non-trivial cross-platform code (`proc.DetachBreakaway` is Windows-specific; `lock` is advisory-lock plumbing) instead of rewriting it, while everything with a tail gets a purpose-built replacement measured in tens of lines.
- Rejected: copy everything including the tail (fastest green build, worst result); replace everything including the leaves (rewrites working platform code for no gain).

### exact-replacement-shapes

- Decision: each replaced package gets a named, minimal local equivalent.
  - `configengine.ConfigFile(baseDir, "servers")` — today `filepath.Join(baseDir, "_lyx", "config", "servers.yaml")`. Replaced by the config resolution in `config-and-state-paths`, roughly 30 lines, no YAML template machinery.
  - `logger.Warn` / `logger.Info` (8 call sites) — replaced by `log/slog` against a package-level handler that writes to stderr and defaults to `slog.LevelWarn`. Note the file-scoped guard in `CONSTRAINTS.md` that pins `lspclient.go` to "stdlib plus `internal/logger`": under `slog` that file becomes stdlib-only, which strengthens rather than weakens the property.
  - `clihelp.SetExit` (22 call sites), `Execute`, `ExecuteIn`, `GroupRunE` — ported into `internal/cli/` with the `lyxcwd` dependency stripped. `SetExit` is the exit-code carrier and is the bulk of the usage; the plan must read the four functions and port their semantics exactly, not guess them.
  - `lyxcwd.CwdFrom`/`WithCwd` — the context-carried cwd-injection seam, used so tests can drive the CLI without `os.Chdir`. Port the seam itself (it is ~30 lines of `context.WithValue` plus an `os.Getwd` fallback), drop everything else. `lyxcwd.Resolve` disappears: it exists to find a lyx hub, and quarry has none.
  - `gitkit`/`hubforge` test fixtures — replaced by `t.TempDir()`-based fixtures. The tests using them build a workspace to run a language server against; they do not need a hub, a fabric, or a wiki.
- Rationale: naming the replacement shape per package is what lets mill-plan write batches that can be verified independently rather than one undifferentiated "port scout" card.
- Rejected: a single compatibility shim package re-exporting Loomyard's API surface — preserves the coupling in spirit and leaves dead code behind.

### config-and-state-paths

- Decision: split the two axes that `anchorRoot` conflates today.
  - **Config** (`servers.yaml`, the optional registry overlay): precedence `--config <path>` → `$QUARRY_CONFIG` → `os.UserConfigDir()/quarry/servers.yaml` → built-in registry. An absent file at any tier is not an error, matching today's `LoadRegistry` behaviour exactly.
  - **State** (`daemon.json`, `daemon.lock`, `daemon.sock` — today `<anchorRoot>/.lyx/scout/<lang>/`): precedence `--state-dir <path>` → `$QUARRY_STATE_DIR` → `os.UserCacheDir()/quarry/<workspace-key>/<lang>/`, where `<workspace-key>` is `filepath.Base(targetDir) + "-" + first-12-hex-of-SHA256(absolute targetDir)`.
- Rationale: in `internal/scoutcli/cli.go`'s `lookupContext`, `loc.AnchorPath()` is passed as *both* the `LoadRegistry` base and the `ensureServer` `anchorRoot`. Those are different things: config is a user-global tool setting, state is per-workspace daemon bookkeeping. Outside a lyx hub there is no single root that serves both. `os.UserConfigDir()`/`os.UserCacheDir()` are stdlib and already do the right per-OS thing (XDG on Linux, `%AppData%`/`%LocalAppData%` on Windows, `~/Library/…` on macOS), so no platform code is written. The workspace key must be deterministic because two concurrent `quarry` processes targeting the same directory must find the same daemon — that is the whole point of the supervised strategy's spawn-race lock.
- Rejected: `--config` only with no implicit lookup (deterministic but poor ergonomics); `.quarry/` inside the target directory (pollutes repos quarry is pointed at, and needs a `.gitignore` entry in someone else's project); keeping a single conflated root (there is no defensible value for it outside a hub).
- **Risk the plan must handle:** Unix domain sockets are capped at 108 bytes of path on Linux. `os.UserCacheDir()` plus a workspace key plus `<lang>/daemon.sock` can approach that. Today's code has the same exposure via `anchorRoot`, so this is not a regression, but the plan should keep the key short and add a test asserting the constructed socket path length stays under the limit for a realistic deep path.

### repo-layout

- Decision:
  ```
  quarry/              package quarry   — the engine (from internal/scoutengine)
  internal/cli/        package cli      — cobra wiring (from internal/scoutcli)
  cmd/quarry/main.go   package main     — thin entry point
  docs/                                 — the four moved research/benchmark docs
  ```
  One Go module, `github.com/Knatte18/quarry`.
- Rationale: the core is public because `loom`'s deferred Plan-Sweep work consumes `scoutengine.References` and symbol lookup directly as a Go API, and that must remain possible across the repo boundary. The CLI is `internal/` because nothing should import quarry's cobra wiring. A single module keeps versioning trivial; `cmd/quarry-mcp/` can be added later as a peer of `cmd/quarry/` with no restructuring.
- Rejected: everything under `internal/` with only the binary exported (closes the door on Go-API consumers); mirroring `quarryengine`/`quarrycli` (the `engine`/`cli` suffixes only mean something inside lyx's module pattern).

### engine-cli-seam-preserved

- Decision: carry the Scout Engine-Seam Invariant across as a quarry-owned invariant. `quarry/` never imports the CLI package, cobra, or the output-envelope package; `internal/cli/` is the sole place engine results become JSON. Port `seam_enforcement_test.go` and `lspclient_guard_test.go` with their import lists retargeted.
- Rationale: the seam is why the extraction is tractable at all, and quarry will grow a second front-end (MCP) that must consume the same typed results the CLI does rather than shelling out to the CLI. Losing the seam now would force it to be rebuilt then.
- Rejected: dropping the enforcement tests as lyx-specific ceremony — they are the machine guard on the property the whole repo layout depends on.

### mechanical-move-not-hand-transcription

- Decision: the file move and import rewrite are performed by a **Go program** written for the purpose (e.g. `tools/port/main.go` in the quarry repo, deleted before the final commit, or run from a scratch directory), not by an agent reading and retyping ~8 900 lines, and not by shell `sed`.
  The program: copies the named files, rewrites `github.com/Knatte18/loomyard/internal/scoutengine` → `github.com/Knatte18/quarry/quarry`, `…/internal/scoutcli` → `…/quarry/internal/cli`, `…/internal/{lock,proc,output}` → `…/quarry/internal/{lock,proc,output}`, and the package clauses `package scoutengine` → `package quarry`, `package scoutcli` → `package cli`.
  Hand editing is confined to the five replaced packages and their call sites — the part that genuinely requires judgement.
- Rationale: the user's explicit instruction, and the right one — transcribing 8 900 lines through an LLM context is both expensive and a correctness hazard, while `go/ast`-level or line-level rewriting of a known, closed set of import paths is deterministic and reviewable as a diff. Go rather than Python because quarry is a Go repo and the tool should not add a Python dependency. `sed` is banned by the `mill:conversation` rule.
- Rejected: `git filter-repo --subdirectory-filter` per package (preserves history, but yields two unrelated roots to graft and paths that do not match the new layout, for history that stays readable in Loomyard anyway); manual transcription.

### history-not-preserved

- Decision: quarry starts from a single `initial import` commit. History stays in Loomyard.
- Rationale: the packages are being renamed and restructured, so filtered history would point at paths that no longer exist. Loomyard remains public and its log is one `git log -- internal/scoutengine` away. The initial-import commit message should name the source commit SHA in Loomyard so the link is explicit.
- Rejected: `git filter-repo` graft (see above).

### lyx-side-removal-is-total

- Decision: `lyx scout` is deleted, not replaced by a shell-out. Loomyard gains no optional dependency on the quarry binary, no detection logic, no vendored artifact.
- Rationale: the benchmark showed agent-facing scout usage does not beat `grep`, so the subcommand's value was never the ergonomics of typing `lyx scout`. The durable value is deterministic Go-API and gate usage, and a future `loom` Plan-Sweep will import quarry as an external Go module — which is a different mechanism entirely from a shelled-out binary. Adding a shell-out now would be designing for a caller that does not exist.
- Rejected: shell-out when present (dead code path from day one); keep as an external Go module dependency in `cmd/lyx` (re-links what this task is unlinking, and drags quarry's cobra tree into lyx's help tree).

### docs-split

- Decision: move `docs/research/scout-spike.md`, `docs/research/scout-multilang.md`, `docs/research/scout-agent-usage-findings.md`, and `docs/benchmarks/scout-vs-grep.md` into quarry's `docs/`. Leave `manifest/designs/scout-plan-symbol-fields.md` in Loomyard and repoint its links to the quarry URLs.
- Rationale: the first four are quarry's own design record — measurements of the tool's precision, recall, and cost. The fifth is a loom design about consuming scout, so it belongs to the consumer. The measurements were taken against the Loomyard corpus, which stays accurate as a stated fact about the benchmark target and does not make them Loomyard documents.
- Rejected: duplicating all five in both repos (guaranteed drift); leaving them all in Loomyard (quarry ships with no rationale for its own design trade-offs, and the `scoutengine` package doc — which is also moving — points at them).

### two-repo-worktree-authorization

- Decision: this task's mill-go batches are explicitly authorized to write, commit, and push in `/home/knatte/Code/quarry/wts/quarry` in addition to this task worktree. Every command against quarry uses `git -C /home/knatte/Code/quarry/wts/quarry …` — never `cd`.
- Rationale: the repo's worktree-isolation rule (`CLAUDE.md`) forbids an agent from touching any worktree but its own unless the user says so for the specific case. The user said so for this case, and cloned the repo for it. Recording the authorization here is what makes it available to a mill-go agent that has none of this conversation.
- **Consequence the plan must respect:** batch `verify:` commands run in the task worktree by default, so quarry-side verification must be spelled `go -C /home/knatte/Code/quarry/wts/quarry test ./...` or equivalent. A single `cd` into the quarry worktree corrupts cwd for the rest of the session.

### ordering-quarry-green-before-lyx-deletion

- Decision: quarry must build and pass its ported tests before any deletion lands in Loomyard. The two repos' work is sequenced, not interleaved.
- Rationale: the deletion is irreversible in practice (recovering means a revert plus a re-port), and the port is the part most likely to surface a hidden dependency. Keeping Loomyard green and scout-bearing until quarry is proven means a failed port costs nothing.
- Rejected: parallel batches across both repos.

## Technical context

### What scout actually is

Four verbs over a generic stdio JSON-RPC LSP client:

- `refs <symbol|file:line:col>` — every reference.
- `definition <symbol|file:line:col>`.
- `symbol <query>` — workspace symbol search.
- `assert-no-callers <symbol|file:line:col>` — exit-code gate for delete/move safety, with `--except` and `--within`.

Each verb also has a batch-argument mode. Output is the `output.Ok`/`output.Err` JSON envelope.

Key files in `internal/scoutengine`:

- `lspclient.go` (22 827 bytes) — the JSON-RPC client. Speaks exactly eight LSP methods. Every request phase is deadline-bounded; a timeout hard-kills the subprocess rather than attempting a graceful shutdown that could itself block.
- `ensureserver.go` (27 529 bytes) — the daemon lifecycle. Two strategies: `ensureSupervised` (quarry owns a state file, an advisory spawn-race lock, a deterministic socket path, and detached spawn/restart) and `ensureNative` (`gopls -remote=auto`). Go dispatches to supervised with native as fallback. The other four languages have `HasNativeDaemon` false and cold-spawn per call, untouched.
- `registry.go`/`load.go`/`template.go`+`template.yaml` — the language registry: pinned built-ins for Go, Python, C#, TypeScript, Rust; optional `servers.yaml` whole-entry overlay; embedded seed template.
- `detect.go` — marker-based language detection with a fixed precedence order (go, rust, csharp, typescript, python) because map iteration is unordered.
- `position.go` — 1-based byte-column ↔ LSP 0-based UTF-16 conversion. Re-reads the file because the two only coincide on pure-ASCII lines.
- `refs.go` — `resolvePosition`, the name → position resolver via `workspace/symbol`, plus the `--in-file` narrow mode via `textDocument/documentSymbol`. Zero candidates → `ErrSymbolNotFound`, more than one → `ErrAmbiguousSymbol` carrying every candidate.
- `toolchain.go` — pins and installs `gopls` into a machine-global cache independent of `$PATH`.
- `errors.go` — the typed error set the CLI maps to exit codes.
- `doc.go` (18 118 bytes) — the as-built design record. It is the module's documentation under Loomyard's Documentation Lifecycle and must move with the code, with lyx-specific references (Cwd Resolution Invariant, `internal/modelspec`, `internal/webstercli`) rewritten rather than dropped.

### The exact used surface of the shared packages

Measured, not assumed:

| Package | Symbols scout uses |
|---|---|
| `configengine` | `ConfigFile` (3 sites) |
| `clihelp` | `SetExit` (22), `Execute` (2), `ExecuteIn` (2), `GroupRunE` (1) |
| `output` | `Err` (17), `Ok` (9) |
| `lyxdirs` | `DotLyxDirName` (3) |
| `lock` | `AcquireWriteLock`, `TryAcquireWriteLock` |
| `proc` | `KillPID` (6), `IsAlive` (2), `DetachBreakaway` (2) |
| `logger` | `Warn` (7), `Info` (1) |
| `lyxcwd` | `CwdFrom` (8), `Resolve` (2), `WithCwd` (1) — CLI only |
| `gitkit`, `hubforge` | test files only |

`lyxcwd`, `gitkit`, and `hubforge` appear in no `scoutengine` non-test file. `output` and `clihelp` appear in no `scoutengine` non-test file either — that is the engine seam holding.

### `lookupContext` is the hinge

`internal/scoutcli/cli.go:453` is where lyx-specific path resolution enters, and it is the single function that must be redesigned rather than ported:

```go
registry := scoutengine.BuiltinRegistry()
loc, resolveErr := lyxcwd.Resolve(cwd)
if resolveErr == nil {
    loaded, loadErr := scoutengine.LoadRegistry(loc.AnchorPath())
    …
    return registry, loc.AnchorPath(), nil   // ← same value as BOTH config base and anchorRoot
}
abs, err := filepath.Abs(dir)
return registry, abs, nil
```

The out-of-hub branch already exists and already degrades to built-ins plus `filepath.Abs(dir)`. In quarry, the out-of-hub branch becomes the only branch, with the config lookup lifted out of it per `config-and-state-paths`.

`anchorRoot` flows only to `DaemonStateFile(anchorRoot, lang)` and `DaemonLock(anchorRoot, lang)` in `daemonstate.go:39,47`, which join `<anchorRoot>/.lyx/scout/<lang>/daemon.{json,lock}`, and the socket is `filepath.Join(filepath.Dir(statePath), "daemon.sock")` (`ensureserver.go:305`). Redirecting those three is the entire state-path change.

### Loomyard removal checklist

Every site, enumerated:

- `cmd/lyx/main.go:32` (import), `:110` (`scoutcli.Command()`), `:76` (module list in the root command's long help).
- `cmd/lyx/helptree_test.go:28` (module list), `:107-108` (the scout help-tree case).
- `cmd/lyx/seamsignature_test.go:23,41,63` (`scoutcli.RunCLI`, `scoutcli.RunCLIIn`).
- `cmd/lyx/hermeticenv_test.go:38` (the `internal/scoutengine` subprocess-justification entry).
- `cmd/lyx/notransients_test.go:7,27,82-83` and `cmd/lyx/constructoranchoring_test.go:7,25,45,106-107` (`DaemonStateFile`/`DaemonLock` anchoring assertions).
- `cmd/lyx/configstrictness_test.go:17` (comment naming `internal/scoutengine`).
- `CONSTRAINTS.md:195-208` — the whole **Scout Engine-Seam Invariant** section; `:77` — remove `internal/scoutengine` from the told-geometry review-obligation list; `:440` — the `scout-redesign.md` prose-mention example, which must be repointed at another live example rather than left dangling. Note this line is doubly stale: it cites `manifest/roadmap.md:98`, but that reference moved to `:138` in the `b01ffc3b` roadmap cleanup, and this task edits `manifest/roadmap.md` again so the number will shift once more. Pick a replacement example that is not scout-related, and prefer a citation form that does not pin a line number.
- `docs/overview.md:293` (module table entry), `:431` (package-doc list entry), `:190` (prose listing scout among `.lyx`-writing modules).
- `manifest/roadmap.md` — lines 51, 138, 148, 150, 160, 172-174, 206, 263 all reference scout. Most are Someday items about *consuming* scout (`Plan-Sweep`, `scout-backed plan symbol fields`); those stay but must be reworded to name quarry as an external dependency. Line 160 is a scout defect item that moves to quarry's issue tracker (it is the same defect as [quarry#1](https://github.com/Knatte18/quarry/issues/1) — close it out of the roadmap rather than reword).
- `contracts/specs/loom-plan-spec.md`, `internal/loomshed/loomshed.go`, `internal/websterengine/doc.go`, `internal/gitrepo/doc.go`, `internal/fabriccli/clone.go`, `internal/fabricengine/junction.go` — all mention scout in prose or comments; audit each and reword.

### Repo state

`/home/knatte/Code/quarry/wts/quarry` is cloned, empty, on `main`, with no commits and no `go.mod`. Issues [#1](https://github.com/Knatte18/quarry/issues/1) and [#2](https://github.com/Knatte18/quarry/issues/2) are filed. GitHub Actions is not enabled. No README, `.gitignore`, or LICENSE was auto-generated — all three are still to be written.

Loomyard's module path is `github.com/Knatte18/loomyard`, Go 1.26.

## Constraints

From `CONSTRAINTS.md`, the ones this task touches:

- **Scout Engine-Seam Invariant** — `scoutengine` never imports `output`, cobra, or any `*cli` package; `scoutcli` → `scoutengine` is the only allowed direction; `lspclient.go` is stdlib plus `logger` and nothing else. Enforced by `seam_enforcement_test.go` (`TestEngineSeamInvariant_BannedImports`) and `lspclient_guard_test.go` (`TestLSPClientGuard_StdlibAndLoggerOnly`). Both tests move to quarry; the section is deleted from Loomyard's `CONSTRAINTS.md`.
- **Told-Geometry Invariant** — an engine is handed absolute paths and derives none of its own, so it runs identically inside a hub and in a bare non-git directory. `scoutengine` is on the review-obligation list. This is precisely the property that makes the extraction possible, and it must hold in quarry too — the new config/state resolution belongs in `internal/cli/`, not in the `quarry/` package.
- **CLI/Cobra Invariant** — module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests. Loomyard's help-tree test must stop expecting `scout`; quarry inherits the shape for its own root command.
- **Documentation Lifecycle** — a task changing observable CLI behaviour updates docs in the same commit. Deleting `lyx scout` is observable, so `docs/overview.md` and `CONSTRAINTS.md` land in the deletion commit.
- **Cwd Resolution Invariant** — the reason `LoadRegistry` goes through `configengine.ConfigFile` instead of hand-joining. Quarry has no `lyxcwd` and therefore no such invariant; its replacement rule is that config and state paths are resolved in exactly one place each, in `internal/cli/`.
- **Test Tier Purity** — `//go:build integration` / `//go:build smoke` separation. Ported to quarry.

Discovered during discussion:

- Quarry's dependency budget after the port is stdlib + `spf13/cobra` + `gopkg.in/yaml.v3`. Any batch that would add a fourth is a design change and must stop.
- `lspclient.go` becomes stdlib-only once `logger` is replaced by `log/slog`. The ported guard test should assert stdlib-only, tightening the invariant rather than restating it.

## Testing

The test suite is the extraction's proof. 5 299 lines of tests move with the code (4 274 from `scoutengine`, 1 025 from `scoutcli`), and the acceptance criterion for the whole task is that they pass in quarry unchanged in intent.

**Ported unchanged (pure logic, no fixtures):** `definition_test.go`, `detect_test.go`, `position_test.go`, `symbol_test.go`, `registry_test.go`, `load_test.go`, `refs_test.go`, `lspclient_test.go`, `daemonstate_test.go`, `ensureserver_test.go`, `supervised_test.go`, `toolchain_test.go`, `cli_test.go`. Only import paths and package clauses change. Any test needing more than that is a signal the port drifted and must be investigated, not patched.

**Rewritten fixtures:** `cli_integration_test.go`, `ensureserver_integration_test.go`, `refs_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go`, `supervised_scout_test.go`, `scoutdaemon_test.go` — the ones reaching for `gitkit`/`hubforge`. Replace hub construction with `t.TempDir()` plus whatever minimal files the language server needs (a `go.mod` and a `.go` file, for the Go arm).

**Ported with retargeted expectations:** `seam_enforcement_test.go` and `lspclient_guard_test.go` — the banned-import lists name Loomyard paths today.

**TDD candidates — the genuinely new code, written test-first:**

1. **Config path resolution.** Table-driven over the four precedence tiers: explicit `--config`, `$QUARRY_CONFIG`, `os.UserConfigDir()` default, and nothing-set-anywhere. Absent file at every tier must fall back to built-ins with no error, matching today's `LoadRegistry`. A present-but-malformed file must still error, and an empty/comments-only file must still yield built-ins (today's `io.EOF` special case).
2. **State path resolution and workspace key.** The key must be deterministic for the same absolute target directory and distinct for different ones, including two directories sharing a basename. Assert the constructed socket path stays under 108 bytes for a realistically deep target path. Assert `--state-dir` and `$QUARRY_STATE_DIR` override in the right order.
3. **The `clihelp` replacement.** `SetExit` has 22 call sites and is the exit-code carrier; its semantics must be pinned by tests before the call sites are repointed.
4. **The cwd-injection seam.** `WithCwd`/`CwdFrom` — that an injected cwd is honoured, that absent injection falls back to the process cwd, and that the CLI never calls `os.Chdir`.

**Task-wide verification, in order:**

1. `go -C /home/knatte/Code/quarry/wts/quarry build ./...` — quarry compiles.
2. `go -C /home/knatte/Code/quarry/wts/quarry test ./...` — unit tier green.
3. `go -C /home/knatte/Code/quarry/wts/quarry test -tags integration ./...` — integration tier green (needs `gopls`; the other four language servers' arms skip when absent, as they do today).
4. **Behavioural equivalence.** Run `quarry refs`, `definition`, `symbol`, and `assert-no-callers` against the Loomyard checkout and compare the JSON envelopes to `lyx scout` output for the same queries, before the Loomyard deletion lands. Use the five benchmark symbols already recorded in `docs/research/scout-multilang.md` — they have established ground truth, which makes this a real check rather than a smoke test. Envelopes must match modulo any absolute path that legitimately changed.
5. Only then: `go test ./...` in this worktree, after the Loomyard deletion, with `cmd/lyx` help-tree and seam-signature tests updated.

Step 4 is the one that must not be skipped. Everything else proves quarry compiles and its own tests pass; only step 4 proves it does the same thing.

## Q&A log

- **Q:** Repo name? **A:** `quarry` — repo, binary and module all the same. `Knatte18/quarry` was free; GitHub names collide only within an account namespace. `lyx`-prefixed names rejected for the LyX collision and for implying a lyx subcommand.
- **Q:** Does this task include Tree-sitter and the MCP wrapper? **A:** No. Extraction only, ending in a working `quarry` binary that does what `lyx scout` does today. Both are worth keeping in mind while choosing the layout, but no code for either is written here. Core and CLI (cobra, as lyx uses) are both required.
- **Q:** Preserve git history in the move? **A:** No — clean copy, single initial-import commit. But the move must be performed mechanically by a Go program doing the import/package rewrite, not by an agent retyping the files, and not with `sed`. Writing that program is part of the plan.
- **Q:** What happens to `lyx scout`? **A:** Deleted outright. No shell-out, no optional binary, no external Go module dependency in `cmd/lyx`.
- **Q:** Fix the two known defects during the move? **A:** No — moved unfixed, filed as quarry#1 and quarry#2 immediately. Byte-for-byte behavioural equivalence is what makes the move verifiable.
- **Q:** Where does `servers.yaml` live with no lyx hub? **A:** Delegated to design. Resolved as `--config` → `$QUARRY_CONFIG` → `os.UserConfigDir()/quarry/servers.yaml` → built-ins, with daemon state split onto a separate axis under `os.UserCacheDir()` keyed by workspace, because today's `anchorRoot` serves both purposes and neither has a hub-free answer.
- **Q:** Test tiers and CI? **A:** Inherit the `//go:build integration` / `smoke` separation. GitHub Actions is not enabled on the quarry repo, so no workflow file is written — the tags exist so CI is a config file away.
- **Q:** May mill-go write in the quarry repo? **A:** Yes — the user created and had the repo cloned for this task. Authorization is recorded in the `two-repo-worktree-authorization` decision; all quarry commands use `git -C`, never `cd`.
