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

**Why now.** Three things changed.
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
- Port the full test suite, collapsing today's two opt-in tags (`scout` on five files, `integration` on one) onto a single `//go:build lsp` tier — see the `test-tier-tags` decision.
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
  - **Copy verbatim:** `internal/lock` (222 lines, one external dep — see below), `internal/proc` (329, no internal or external deps), `internal/output` (256, no deps).
    `internal/lock/lock.go:12` imports `github.com/gofrs/flock`. That is accepted as a fourth budgeted external dependency rather than a reason to move `lock` to the replace side: cross-platform advisory file locking is exactly the non-trivial platform code this decision exists to avoid rewriting, and it is the lock guarding the daemon spawn race, where a subtle bug is a hang rather than a test failure.
  - **Inline:** `internal/lyxdirs` — only `DotLyxDirName` is used, and it disappears entirely once state paths move off `_lyx/` (see `config-and-state-paths`).
  - **Replace:** `internal/configengine` (1 552 lines, pulls `envsource`+`logger`+`lyxdirs`+`yamlengine`, but scout uses exactly one function, `ConfigFile`), `internal/logger` (1 939 lines, pulls `lyxcwd`+`lyxdirs`+`proc`, scout uses `Warn` and `Info`), `internal/clihelp` (923 lines, pulls `lyxcwd`+`output`, scout uses `SetExit`/`Execute`/`ExecuteIn`/`GroupRunE`), `internal/lyxcwd` (3 028 lines, CLI-only, uses `CwdFrom`/`Resolve`/`WithCwd`), and `internal/gitkit`+`internal/hubforge` (2 192 lines, test fixtures only, and `hubforge` pulls `fabricengine`).
- Rationale: the *declared* dependency is nine packages and ~11 500 lines; the *used surface* is 18 symbols. Copying naively drags `logger` → `lyxcwd` → `gitexec` and `hubforge` → `fabricengine`, which would make quarry a Loomyard clone. Copying only the leaf packages keeps genuinely non-trivial cross-platform code (`proc.DetachBreakaway` is Windows-specific; `lock` is advisory-lock plumbing) instead of rewriting it, while everything with a tail gets a purpose-built replacement measured in tens of lines.
- Rejected: copy everything including the tail (fastest green build, worst result); replace everything including the leaves (rewrites working platform code for no gain).

### exact-replacement-shapes

- Decision: each replaced package gets a named, minimal local equivalent.
  - `configengine.ConfigFile(baseDir, "servers")` — today `filepath.Join(baseDir, "_lyx", "config", "servers.yaml")`. Replaced by the config resolution in `config-and-state-paths`, roughly 30 lines, no YAML template machinery.
  - `logger.Warn` / `logger.Info` (8 call sites) — replaced by `log/slog` against a package-level handler that writes to stderr and defaults to `slog.LevelWarn`. Note the file-scoped guard in `CONSTRAINTS.md` that pins `lspclient.go` to "stdlib plus `internal/logger`": under `slog` that file becomes stdlib-only, which strengthens rather than weakens the property.
  - `clihelp.SetExit` (22 call sites), `Execute`, `ExecuteIn`, `GroupRunE`, `NewExitContext` — ported into `internal/cli/` with the `lyxcwd` dependency stripped. `SetExit`/`NewExitContext` are two halves of one mechanism: `NewExitContext` returns a context plus an exit-state handle whose `Code()` the tests read, and `SetExit` writes into that state from a command handler. Port the pair together, along with the exit-state type and its `Code()` method; `cli_test.go` cannot compile without them. The plan must read all five functions and port their semantics exactly, not guess them.
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

### state-path-ownership-moves-to-the-caller

- Decision: the engine is **told a leaf state directory** and joins nothing but the filename. `DaemonStateFile`/`DaemonLock` change from `(anchorRoot, lang)` to `(stateDir, lang)`, returning `filepath.Join(stateDir, lang, "daemon.json")` and `…/daemon.lock`. The `.lyx` and `scout` path segments — and with them `lyxdirs.DotLyxDirName` and the private `scoutDirName` constant — are deleted from the engine, not relocated. `ensureServer`'s and `ensureSupervised`'s `anchorRoot` parameter is renamed `stateDir` throughout. All resolution of *which* directory that is happens in `internal/cli/`, per `config-and-state-paths`.
- Rationale: this is the Told-Geometry Invariant applied literally — an engine is handed the absolute paths it operates on and derives none of its own. Today's `daemonstate.go:39,47` violate that in spirit: they take a root and derive `<root>/.lyx/scout/<lang>/` from it, and the derived segments are lyx vocabulary with no meaning in quarry. The earlier phrasing that `DotLyxDirName` "disappears entirely" while resolution "belongs in `internal/cli/`" was only satisfiable with this signature change, and the earlier claim that "redirecting those three is the entire state-path change" understated it.
- Rejected: keeping `anchorRoot` plus the engine's own segments and having the CLI pass a synthetic root — preserves a hidden `.lyx/scout/` subtree inside `os.UserCacheDir()`, which is meaningless outside lyx and would confuse anyone reading the cache directory; also keeps the engine deriving path structure it was told nothing about.
- **`Options.AnchorRoot` is renamed too.** The value reaching `ensureServer` is the exported field `Options.AnchorRoot` (`refs.go:51`, threaded at `refs.go:67`) — public API of the new `quarry` package, not an internal parameter. It becomes `Options.StateDir`, with its doc comment rewritten from "required and must be a usable absolute path" to name what the directory is for. Three files set it (`refs_integration_test.go:91,206,244`) and therefore cannot be ported on a path rewrite alone; they are listed under the rewritten tests below.
- **Consequences the plan must carry:**
  - `daemonstate_test.go` and `scoutdaemon_test.go` move from "ported unchanged" to "rewritten". `scoutdaemon_test.go` drives the constructors over three fixtures — an unanchored worktree root, a subpath-anchored root, and a plain told directory — and two of those three fixture concepts cease to exist. What remains is plain join arithmetic over a told `stateDir`, which is a smaller test than the one being replaced. The rewrite must not silently drop the per-language separation assertion, which does survive.
  - `DaemonStateFile`/`DaemonLock` stay exported: quarry's own supervised-daemon tests use them, and a future Go-API consumer needs to locate a daemon it did not spawn.
  - Loomyard's `cmd/lyx/constructoranchoring_test.go:106-107` and `notransients_test.go:82-83` assert exactly the anchoring behaviour being deleted. They are removed with the module rather than migrated.

### config-path-ownership-moves-to-the-caller

- Decision: the config half gets the same treatment as the state half. `LoadRegistry` changes from `LoadRegistry(baseDir string)` to `LoadRegistry(path string)` — told a resolved absolute path to a `servers.yaml` file, joining nothing. The precedence chain (`--config` → `$QUARRY_CONFIG` → `os.UserConfigDir()/quarry/servers.yaml`) is resolved entirely in `internal/cli/`. An absent file still returns `builtins()` with no error, and an empty or comments-only file still returns `builtins()` via the existing `io.EOF` special case — both behaviours are preserved exactly.
- Rationale: `load.go:22-23` currently calls `configengine.ConfigFile(baseDir, "servers")`, deriving `<baseDir>/_lyx/config/servers.yaml`. That is the same Told-Geometry violation `state-path-ownership-moves-to-the-caller` fixes on the state axis, and leaving it in place would make the Constraints claim that config resolution "belongs in `internal/cli/`" unsatisfiable while an exported engine function takes a base directory and joins onto it. Fixing one axis and not the other would also leave the engine with two different path contracts for no reason.
- Rejected: moving the overlay load into `internal/cli/` entirely — the YAML decode, `KnownFields(true)` strictness, `validateEntry`, and the whole-entry-replacement merge are registry semantics, not CLI concerns; relocating them would force `Registry`, `Entry`, and `validateEntry` to stay exported for a consumer that no longer exists, and would leave a Go-API consumer unable to load an overlay without reimplementing the parser.
- **Consequence:** `load_test.go` is already on the rewritten list for its `configengine.ConfigDir` use; this decision is the reason it is rewritten rather than merely retargeted — the function under test changes shape.

### test-tier-tags

- Decision: quarry uses a single opt-in build tag, `//go:build lsp`, for every test requiring a real language-server binary on `$PATH`. Verification is `go test ./...` (hermetic tier) and `go test -tags lsp ./...` (live tier).
- Rationale: the tag scheme being ported is not what the earlier draft described. Five of the six tagged files carry `//go:build scout` (`ensureserver_integration_test.go`, `refs_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go`, `supervised_scout_test.go`); only `internal/scoutcli/cli_integration_test.go` carries `//go:build integration`. A verify command spelled `-tags integration` would therefore have run one file and skipped the entire live-server suite while appearing to prove it green — the exact failure this task cannot afford. Collapsing to one tag is right because both tags mean the same substrate here (a real external binary must exist), the split into two existed only because Loomyard has other `integration`-tagged tests unrelated to scout, and `scout` is a name that stops meaning anything the moment the tool is called quarry. `lsp` names the actual precondition.
- Rejected: keeping both tags (preserves a distinction with no remaining basis, and two verify commands where one suffices); keeping the name `scout` (dead vocabulary in the new repo); using `integration` alone (accurate but vaguer — it does not tell a contributor that the precondition is an installed language server).
- Note for Loomyard's side: `cmd/lyx/tierpurity_test.go:50` pins `knownTierTags = []string{"integration", "smoke", "scout"}` and `:200,202` test the `scout` tag's parsing. Removing scout makes `scout` a dead tier tag in Loomyard; the deletion must drop it from `knownTierTags`, from those two table cases, from `CONSTRAINTS.md`'s Test Tier Purity section, and from `docs/benchmarks/running-tests.md:10,28,29,51`.

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
- Rationale: the user's explicit instruction, and the right one — transcribing 8 900 lines through an LLM context is both expensive and a correctness hazard, while `go/ast`-level or line-level rewriting of a known, closed set of import paths is deterministic and reviewable as a diff. Go rather than Python because quarry is a Go repo and the tool should not add a Python dependency. `sed` is banned twice over — by the operator's global `CLAUDE.md` ("Don't use `sed`") and by the `mill:conversation` skill's Shell Commands rule — because it triggers a permission prompt that stalls unattended runs.
- Rejected: `git filter-repo --subdirectory-filter` per package (preserves history, but yields two unrelated roots to graft and paths that do not match the new layout, for history that stays readable in Loomyard anyway); manual transcription.

### error-prefixes-stay-verbatim-through-the-port

- Decision: the 59 `fmt.Errorf("scoutengine: …")` literals across nine production files (`errors.go` 12, `ensureserver.go` 18, `lspclient.go` 8, and six others) are **not** touched by the port. They keep saying `scoutengine:` in quarry's first commit. Renaming them to `quarry:` is a separate, later commit in the quarry repo, made only after the behavioural-equivalence check in Testing step 4 has passed.
- Rationale: these strings reach the user through the JSON envelope's error field, so they are observable output. Step 4 compares envelopes between `lyx scout` and `quarry` for the same queries, and that comparison is only meaningful if the error text is identical — renaming during the port would force the criterion to be relaxed to "equal modulo error-message text", which is exactly the loophole a real behavioural difference would hide in. Sequencing the rename after the proof keeps the equality strict when it matters and costs one extra commit.
- Rejected: rename during the port (weakens step 4 to the point of not proving much); keep `scoutengine:` forever (dead vocabulary — the package is not called that in quarry, and an error naming a nonexistent package is a support burden).
- **Consequence:** the port program rewrites import paths and package clauses only, and must be verified not to touch string literals. File a quarry issue for the follow-up rename as soon as the repo has its first commit, so it is not forgotten once step 4 goes green.

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
| `configengine` | `ConfigFile` (3 sites), `ConfigDir` (2 sites, both in `load_test.go:19-20`) |
| `clihelp` | `SetExit` (22), `Execute` (2), `ExecuteIn` (2), `GroupRunE` (1), `NewExitContext` (3, all in `cli_test.go:345,471,510`) — plus the unexported exit-state type `NewExitContext` returns and its `Code()` method, which `cli_test.go` calls |
| `output` | `Err` (17), `Ok` (9) |
| `lyxdirs` | `DotLyxDirName` (3) |
| `lock` | `AcquireWriteLock`, `TryAcquireWriteLock` |
| `proc` | `KillPID` (6), `IsAlive` (2), `DetachBreakaway` (2) |
| `logger` | `Warn` (7), `Info` (1) |
| `lyxcwd` | `CwdFrom` (8), `Resolve` (2), `WithCwd` (1) — CLI only |
| `gitkit` | `HermeticGitEnv`, imported by exactly one file: `internal/scoutcli/testmain_test.go` |
| `hubforge` | `NewHub`, `SeedConfig`, imported by exactly one file: `internal/scoutcli/cli_integration_test.go` |

`lyxcwd`, `gitkit`, and `hubforge` appear in no `scoutengine` file at all — neither production nor test. Three `scoutengine` integration tests (`ensureserver_integration_test.go`, `refs_integration_test.go`, `toolchain_integration_test.go`) mention `gitkit.HermeticGitEnv` only inside a comment explaining why they do *not* need it; a grep for the package name finds them and a grep for the import does not. `output` and `clihelp` appear in no `scoutengine` non-test file either — that is the engine seam holding.

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

`anchorRoot` flows only to `DaemonStateFile(anchorRoot, lang)` and `DaemonLock(anchorRoot, lang)` in `daemonstate.go:39,47`, which join `<anchorRoot>/.lyx/scout/<lang>/daemon.{json,lock}`, and the socket is `filepath.Join(filepath.Dir(statePath), "daemon.sock")` (`ensureserver.go:305`). The value originates as the exported `Options.AnchorRoot` field (`refs.go:51`) and is threaded through at `refs.go:67`. Redirecting these paths is not merely a matter of passing a different root — the engine derives the `.lyx` and `scout` segments itself, which is the violation `state-path-ownership-moves-to-the-caller` resolves by changing the signatures and deleting the segments.

### Loomyard removal checklist

**The enumeration method, not the enumeration.** The list below was hand-built and was found incomplete on review — it missed `cmd/lyx/sandbox_coverage_test.go`, `cmd/lyx/tierpurity_test.go`, `README.md`, `internal/lyxcwd/enforcement_test.go`, six files under `manifest/designs/`, `manifest/parallel-work.md`, and two files under `docs/benchmarks/`. Treat it as a set of known high-risk sites, not a complete inventory.

The plan's deletion batch must re-run the enumeration itself and work from the result:

```
grep -rli 'scout' --exclude-dir=.git --exclude-dir=_mill . | grep -v '^./internal/scout'
```

At the time of writing that returns 73 files; after `internal/scoutengine` and `internal/scoutcli` are deleted, every remaining hit is either a site to edit or a deliberate historical mention that must be justified in the commit message. The batch is not done while an unexamined hit remains.

**A token grep is not sufficient on its own.** Some facts about scout become false without containing the word "scout" — they encode a module *count* that drops from thirteen to twelve. No machine guard checks prose counts, and no grep for `scout` will ever surface them:

- `CONSTRAINTS.md:240` — "Twelve of the thirteen seam modules also carry `RunCLIIn`".
- `CONSTRAINTS.md:269` — "across all thirteen modules and … across the twelve modules that carry it".
- `docs/overview.md:270` — "Twelve of the thirteen modules also expose `RunCLIIn`".
- `cmd/lyx/seamsignature_test.go:1,30` — "the thirteen existing RunCLI … seam functions", "the thirteen-module RunCLI seam shape" (and `:48`, the `RunCLIIn` count).

So the deletion batch runs **two** sweeps. The token grep above, and a second count-oriented sweep for: module counts in prose (`thirteen`, `twelve`, and digit forms), the tier-tag list in `cmd/lyx/tierpurity_test.go:50` and `CONSTRAINTS.md`'s Test Tier Purity section, and the module lists in `cmd/lyx/main.go:76`, `helptree_test.go:28`, and `README.md`. Scout is a `RunCLI`+`RunCLIIn` module, so both counts drop by one.

Machine guards that will fail loudly if the sweep is incomplete, and which the plan should lean on rather than duplicate by hand: `TestEnforcement_MarkdownLinks` (catches the live link in `manifest/designs/review-finding-classification.md:67` to a moved doc), the Sandbox Suite Coverage invariant (catches the stale `excludedModules["scout"]` entry in `cmd/lyx/sandbox_coverage_test.go:31`), and the help-tree test (catches the module list).

Known high-risk sites:

- `cmd/lyx/main.go:32` (import), `:110` (`scoutcli.Command()`), `:76` (module list in the root command's long help).
- `cmd/lyx/helptree_test.go:28` (module list), `:107-108` (the scout help-tree case).
- `cmd/lyx/seamsignature_test.go:23,41,63` (`scoutcli.RunCLI`, `scoutcli.RunCLIIn`).
- `cmd/lyx/hermeticenv_test.go:38` (the `internal/scoutengine` subprocess-justification entry).
- `cmd/lyx/notransients_test.go:7,27,82-83` and `cmd/lyx/constructoranchoring_test.go:7,25,45,106-107` (`DaemonStateFile`/`DaemonLock` anchoring assertions).
- `cmd/lyx/configstrictness_test.go:17` (comment naming `internal/scoutengine`).
- `CONSTRAINTS.md:195-208` — the whole **Scout Engine-Seam Invariant** section; `:77` — remove `internal/scoutengine` from the told-geometry review-obligation list; `:440` — the `scout-redesign.md` prose-mention example, which must be repointed at another live example rather than left dangling. Note this line is doubly stale: it cites `manifest/roadmap.md:98`, but that reference moved to `:138` in the `b01ffc3b` roadmap cleanup, and this task edits `manifest/roadmap.md` again so the number will shift once more. Pick a replacement example that is not scout-related, and prefer a citation form that does not pin a line number.
- `docs/overview.md:293` (module table entry), `:431` (package-doc list entry), `:190` (prose listing scout among `.lyx`-writing modules).
- `manifest/roadmap.md` — lines 51, 138, 148, 150, 160, 172-174, 206, 263 all reference scout. Most are Someday items about *consuming* scout (`Plan-Sweep`, `scout-backed plan symbol fields`); those stay but must be reworded to name quarry as an external dependency. Line 160 is a scout defect item that moves to quarry's issue tracker (it is the same defect as [quarry#1](https://github.com/Knatte18/quarry/issues/1) — close it out of the roadmap rather than reword).
- `cmd/lyx/sandbox_coverage_test.go:31` — the `excludedModules["scout"]` entry, required today by the Sandbox Suite Coverage invariant and stale the moment the module leaves.
- `cmd/lyx/tierpurity_test.go:50,200,202` plus `:30-31` — `knownTierTags` includes `"scout"`, two table cases test its parsing, and two `allowedSpawners` entries name `internal/scoutengine` test files. See the `test-tier-tags` decision.
- `README.md:87` — the scout bullet in the module list.
- `internal/lyxcwd/enforcement_test.go` — mentions scout; check whether it is a geometry-literal allowlist entry that must be dropped.
- `docs/benchmarks/running-tests.md:10,28,29,51` — documents the `scout` tag as a substrate tier; `docs/benchmarks/test-suite-timing.md` — timing numbers naming scout.
- `manifest/parallel-work.md` — carries the "Not yet a wiki task" note this task resolves; remove it.
- `manifest/designs/{loom,fabric-unified-view,semantic-index,raddle,review-finding-classification,webster-parallel-execution}.md` — prose and, in `review-finding-classification.md:67`, a live markdown link to `docs/benchmarks/scout-vs-grep.md`, which is being moved to quarry. That link is machine-checked by `TestEnforcement_MarkdownLinks` and will fail the build if repointed wrongly rather than to the quarry URL.
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
- **Test Tier Purity** — Loomyard's three opt-in tiers are `integration`, `smoke`, and `scout` (`cmd/lyx/tierpurity_test.go:50`). Scout's own tests use two of them: `scout` on five files, `integration` on one. Quarry collapses these onto a single `lsp` tier (see `test-tier-tags`), and Loomyard must retire `scout` as a tier tag when the module leaves.

Discovered during discussion:

- Quarry's dependency budget after the port is stdlib + `spf13/cobra` + `gopkg.in/yaml.v3` + `github.com/gofrs/flock` (the last arriving with `internal/lock`, see `dependency-strategy-copy-vs-replace`). Any batch that would add a fifth is a design change and must stop.
- `lspclient.go` becomes stdlib-only once `logger` is replaced by `log/slog`. The ported guard test should assert stdlib-only, tightening the invariant rather than restating it.

## Testing

The test suite is the extraction's proof. 5 299 lines of tests move with the code (4 274 from `scoutengine`, 1 025 from `scoutcli`), and the acceptance criterion for the whole task is that they pass in quarry unchanged in intent.

The three lists below are derived from the actual import sets, not from filename shape. Every scout test file appears in exactly one of them.

**Ported unchanged (import paths and package clauses only):** `definition_test.go`, `detect_test.go`, `position_test.go`, `symbol_test.go`, `registry_test.go`, `refs_test.go`, `lspclient_test.go`, `ensureserver_test.go`, `supervised_test.go`, `toolchain_test.go`, `cli_test.go`, and the `//go:build lsp` engine tests whose only external need is a live server (`ensureserver_integration_test.go`, `supervised_integration_test.go`, `toolchain_integration_test.go`, `supervised_scout_test.go`) — these import neither `gitkit` nor `hubforge`; they only mention `gitkit.HermeticGitEnv` in a comment explaining why they don't need it, and that comment must be reworded rather than left citing a package quarry does not have. Any test in this list needing more than a path rewrite is a signal the port drifted and must be investigated, not patched.

**Rewritten — state-path signature change** (see `state-path-ownership-moves-to-the-caller`): `scoutdaemon_test.go` and `daemonstate_test.go`. Both exercise the `(anchorRoot, lang)` path arithmetic that is being replaced by `(stateDir, lang)`. `scoutdaemon_test.go`'s three fixtures include an unanchored worktree root and a subpath-anchored root; both concepts vanish. The per-language separation assertion survives and must not be dropped in the rewrite.

**Rewritten — lyx fixtures:** exactly two files import `gitkit`/`hubforge`.
- `internal/scoutcli/testmain_test.go` uses `gitkit.HermeticGitEnv` to isolate the test process from the operator's git config. Quarry spawns no git at all, so the `TestMain` either disappears or is reduced to whatever env hygiene the language-server spawn genuinely needs. Decide which; do not port it blind.
- `internal/scoutcli/cli_integration_test.go` uses `hubforge.NewHub`/`SeedConfig` to build a lyx hub, and what it covers is `lookupContext`'s in-hub branch — the branch being deleted. This is **not** a `t.TempDir()` swap: its subject ceases to exist. Replace it with coverage of the new config/state resolution (TDD candidates 1 and 2 below), which is what now occupies that seam.

**Rewritten — `Options` field rename:** `refs_integration_test.go` sets `AnchorRoot:` at `:91,206,244`. It is otherwise a plain live-server test, so the rewrite is mechanical (`AnchorRoot:` → `StateDir:`, and the value it passes changes from a worktree root to a told state directory), but it is not a path-rewrite-only port and must not be listed as one.

**Rewritten — configengine surface and `LoadRegistry` signature:** `load_test.go` calls `configengine.ConfigDir` (`:19-20`) as well as `ConfigFile`, and the function it exercises changes shape under `config-path-ownership-moves-to-the-caller` (`LoadRegistry(baseDir)` → `LoadRegistry(path)`). It is table-driven over `t.TempDir` fixtures; keep the table's cases — absent file, empty file, valid overlay, unknown field, invalid entry — and retarget them onto the told-path signature.

**Ported with retargeted expectations:** `seam_enforcement_test.go` and `lspclient_guard_test.go` — the banned-import lists name Loomyard paths today.

**TDD candidates — the genuinely new code, written test-first:**

1. **Config path resolution.** Table-driven over the four precedence tiers: explicit `--config`, `$QUARRY_CONFIG`, `os.UserConfigDir()` default, and nothing-set-anywhere. Absent file at every tier must fall back to built-ins with no error, matching today's `LoadRegistry`. A present-but-malformed file must still error, and an empty/comments-only file must still yield built-ins (today's `io.EOF` special case).
2. **State path resolution and workspace key.** The key must be deterministic for the same absolute target directory and distinct for different ones, including two directories sharing a basename. Assert the constructed socket path stays under 108 bytes for a realistically deep target path. Assert `--state-dir` and `$QUARRY_STATE_DIR` override in the right order.
3. **The `clihelp` replacement.** `SetExit` has 22 call sites and is the exit-code carrier; its semantics must be pinned by tests before the call sites are repointed.
4. **The cwd-injection seam.** `WithCwd`/`CwdFrom` — that an injected cwd is honoured, that absent injection falls back to the process cwd, and that the CLI never calls `os.Chdir`.

**Task-wide verification, in order:**

1. `go -C /home/knatte/Code/quarry/wts/quarry build ./...` — quarry compiles.
2. `go -C /home/knatte/Code/quarry/wts/quarry test ./...` — unit tier green.
3. `go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./...` — live tier green (needs `gopls`; the other four language servers' arms skip when absent, as they do today). The tag is `lsp`, not `integration` — see `test-tier-tags`. A verify command spelled `-tags integration` would compile and pass while running one file out of six.
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
- **Q:** Test tiers and CI? **A:** Inherit the opt-in tier separation, but not its current spelling — review found scout's tests actually split across two tags (`scout` on five files, `integration` on one), so quarry collapses them onto a single `//go:build lsp` tier; see `test-tier-tags`. GitHub Actions is not enabled on the quarry repo, so no workflow file is written — the tag exists so CI is a config file away.
- **Q:** [auto-pick, discussion review r2 BLOCKING] Does the config axis get the same told-geometry treatment as the state axis? **A:** Yes — `LoadRegistry(baseDir)` becomes `LoadRegistry(path)`, told a resolved absolute file path, with the precedence chain resolved in `internal/cli/`. **Why:** `load.go:22-23` derives `<baseDir>/_lyx/config/servers.yaml`, the same violation the state decision fixes; leaving it would make the Constraints claim that config resolution lives in `internal/cli/` unsatisfiable, and would give the engine two different path contracts for no reason.
- **Q:** [auto-pick, discussion review r2 BLOCKING] Is a token grep enough to find every Loomyard site the deletion breaks? **A:** No — a second, count-oriented sweep is required. **Why:** four sites encode a module count of thirteen that drops to twelve without containing the word "scout" (`CONSTRAINTS.md:240,269`, `docs/overview.md:270`, `cmd/lyx/seamsignature_test.go:1,30,48`), and no machine guard checks prose counts.
- **Q:** [auto-pick, discussion review r1 BLOCKING] Who owns the daemon state path once `.lyx/scout/` is gone — does the engine keep `anchorRoot` and its own segments, or is it told a leaf directory? **A:** Told a leaf directory: `DaemonStateFile`/`DaemonLock` become `(stateDir, lang)` and join only `<lang>/daemon.{json,lock}`. **Why:** the Told-Geometry Invariant says an engine derives no paths of its own, and `.lyx`/`scout` are lyx vocabulary that would otherwise survive as meaningless segments inside `os.UserCacheDir()`. The alternative was rejected for hiding a `.lyx/scout/` subtree in a cache directory nobody would expect it in.
- **Q:** May mill-go write in the quarry repo? **A:** Yes — the user created and had the repo cloned for this task. Authorization is recorded in the `two-repo-worktree-authorization` decision; all quarry commands use `git -C`, never `cd`.
