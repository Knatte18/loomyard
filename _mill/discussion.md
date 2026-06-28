# Discussion: ghissues module — file LoomYard bugs as GitHub issues

```yaml
task: ghissues module — file LoomYard bugs as GitHub issues
slug: ghissues-module
status: discussing
parent: main
```

## Problem

The sandbox dogfood loop needs the autonomous agent to report LoomYard bugs it
finds. That agent runs inside the sandbox hub with **only `lyx.exe` on PATH** — no
loomyard source checkout, no guaranteed mill skills. So the self-report path cannot
be a Claude skill; it must live in the binary, reachable as `lyx ghissues create ...`.

This is the lyx counterpart to mill's `/millhouse-issue`: a way to file a bug or
enhancement against LoomYard's own repo from anywhere. The critical subtlety is the
**target repo**: the agent sits in the sandbox repo (`lyx-test`), but the bugs are
about LoomYard, so the issue must be filed against `Knatte18/loomyard` — never derived
from the working directory.

**Why now:** the sandbox-suite launcher (a sibling task) is the consumer; its agent
calls `lyx ghissues create` to report. Without this module the dogfood loop has no
self-report channel from inside the binary-only sandbox.

This task owns only the **producing** side (create issues). The **consuming** side
(pull issues → mill-tasks) is a separate millhouse enhancement and is out of scope.

## Scope

**In:**

- A new `internal/ghissues` cobra module exposing `Command() *cobra.Command` and a
  `RunCLI(out io.Writer, args []string) int` seam, following the **warp variant**
  (no `PersistentPreRunE`, `clihelp.WrapRun` handlers, positional `args[0]` = title).
- One subcommand: `lyx ghissues create <title> [flags]`.
  - `--title` is the single positional arg. The command uses `cobra.ExactArgs(1)`
    so zero or 2+ positional args are rejected by cobra before the RunE runs.
  - `--body` / `-b` — issue body (markdown). When the value is exactly `-`, read the
    entire body from stdin (so agents can pipe long reports). Optional.
  - `--label` — repeatable; **defaults to `bug`** when not supplied; any supplied
    `--label` flags *replace* the default (so `--label enhancement` files an
    enhancement). No title-text auto-heuristic.
- **Rich `--help` is in-scope, not optional.** The sandbox-suite (task 031)
  reporting step relies on an agent that has *only* `lyx.exe` on PATH discovering this
  command via `lyx ghissues create --help`. Therefore:
  - The `ghissues` parent command **must** have a `Short`, and the `create` command
    **must** have both a `Short` and a `Long`. (A missing `Short` anywhere in the tree
    is caught generically by `cmd/lyx/drift_test.go`'s recursive `newRoot()` walk
    (`TestDriftGuard_AllCommandsHaveShort`). The `cmd/lyx/jsonhelp_test.go` tests do
    **not** walk all modules — they hardcode `board`/`warp remove` — so `ghissues`
    JSON-help coverage is something the plan must add explicitly, see Testing.)
  - The `create` `Long` must include concrete usage examples — at minimum
    `lyx ghissues create "title" -b -` (with prose stating **`-` means read the body
    from stdin**) and `lyx ghissues create "title" --label enhancement`.
  - The help-tree guard tests must be updated to include `ghissues` (see Testing).
- Target repo is a **hardcoded constant** `Knatte18/loomyard` baked into the binary.
  No flag, no env var, no config file overrides it.
- Wraps the **`gh` CLI** (`gh issue create --repo Knatte18/loomyard ...`). Discovery
  via `exec.LookPath("gh")`.
- JSON output via `output.Ok` / `output.Err`, matching the repo-wide envelope.
  - Success: `{"ok":true,"url":"https://github.com/Knatte18/loomyard/issues/<n>","number":<n>}`.
  - Failure: `{"ok":false,"error":"<msg>"}` with exit code 1.
- Registration in `cmd/lyx/main.go` (`newRoot()` import + `AddCommand` + the `Long`
  module list).
- **CLI-framework registration enforcement (scope expansion, task 032).** Two new
  guard tests in `cmd/lyx/` that close the registration hole the current pinned
  help-tree tests miss (a module can have a `Command()` yet never be wired into
  `newRoot()`):
  - **Test A — registration guard (`cmd/lyx/registration_test.go`):** a source/AST
    scan modelled on `internal/paths/enforcement_test.go`. Walk `internal/`, parse
    each non-`_test.go` file with `go/parser`, collect the package of every top-level
    `func Command() *cobra.Command` (no receiver, no params) into a *discovered* set;
    parse `cmd/lyx/main.go` to collect the selector idents `X` from every `X.Command()`
    argument to `root.AddCommand(...)` into a *registered* set; assert
    `discovered ⊆ registered`. An explicit (initially empty) `allowlist` var carries
    documented exceptions. This makes it impossible to add a module without wiring it
    into `main.go` — including `ghissues` itself ("exists ⇒ registered").
  - **Test B — Long-list consistency (`cmd/lyx/longlist_test.go`):** from the *live*
    tree (`newRoot()`), for each child in `root.Commands()` (skipping `help` and
    `completion`), assert `strings.Contains(root.Long, child.Name())`. The set is
    derived from the live tree — no hardcoded list — so `root.Long` cannot drift from
    the actually-registered modules ("registered ⇒ in --help prose").
  - A short note documenting these as enforcement for the CLI/Cobra invariant is added
    to `CONSTRAINTS.md` in the same commit.
- Durable design documentation in the package header comment (warp-style).
- Roadmap/overview doc updates per the task-completion rule (see Constraints).

**Out:**

- `lyx ghissues list` (and any dedup-before-filing check) — create-only first cut.
- Any config file, `configreg` registration, `template.yaml`/`ConfigTemplate()` —
  not needed because the repo is a hardcoded constant. **Do not** add ghissues to
  `internal/configreg`.
- Browser fallback (opening a pre-filled new-issue URL) when `gh` is missing —
  the tool is agent-driven; it fails loudly instead.
- `GITHUB_TOKEN` / REST API handling — `gh` owns auth.
- A `lyx doctor` check for gh presence — future work, noted as a prerequisite only.
- The consuming side (pull issues → mill-tasks) — separate millhouse enhancement.
- Configurability of the target repo in any form.

## Decisions

### target-repo-hardcoded

- Decision: The destination repo is a hardcoded Go constant `Knatte18/loomyard`,
  passed to `gh issue create --repo Knatte18/loomyard`. No flag, env var, or config
  key overrides it.
- Rationale: The module's sole purpose is reporting bugs/enhancements to LoomYard,
  always — regardless of which repo the caller sits in. A constant "just works" in
  the binary-only sandbox, where reading config from cwd would reintroduce exactly
  the cwd-dependency the task warns against (the agent's cwd is `lyx-test`, not
  loomyard). Operator was explicit: "lyx.exe shall have the loomyard repo hardcoded
  inside it; this module shall only ever report to LoomYard."
- Rejected:
  - *Config key in `_lyx/config/ghissues.yaml`* (configreg-registered like
    board/warp/weft) — config is read from cwd, so the sandbox repo would have to
    seed it; fragile, and reintroduces the cwd dependency.
  - *Constant default + `--repo`/`GHISSUES_REPO` override* — unnecessary flexibility
    for a tool that by definition only ever targets one repo; operator rejected
    making it configurable at all.
  - *Pure constant with no testability seam* — addressed separately via the gh
    runner seam (see gh-runner-seam), not via making the repo configurable.

### wrap-gh-cli

- Decision: Shell out to the `gh` CLI via a `gitexec.RunGit`-style helper
  (`exec.Command` + captured stdout/stderr buffers + `proc.HideWindow(cmd)` +
  `*exec.ExitError` → exit-code extraction).
- Rationale: Matches the existing `internal/gitexec` pattern and mill's
  `/millhouse-issue`. `gh` transparently handles auth (keyring / `GH_TOKEN` /
  `GITHUB_TOKEN`), so no token plumbing is needed. `gh` 2.89.0 is installed and on
  PATH in this environment.
- Rejected: *GitHub REST API via `net/http`* — removes the gh dependency but forces
  explicit token discovery/validation and more code, for no benefit in this
  ecosystem where gh is already an accepted dependency.

### create-only-first-cut

- Decision: Ship only `lyx ghissues create`. No `list` subcommand.
- Rationale: The producing side is all the dogfood loop needs now. A `list`/dedup
  check is genuinely separable and adds scope without an active consumer.
- Rejected: *`create` + `list`* — deferred; symmetric but YAGNI for the first cut.

### fail-loudly-on-missing-gh

- Decision: If `gh` is not found on PATH, or `gh issue create` exits non-zero (e.g.
  not authenticated, network failure, bad repo), return `output.Err(...)` with a
  clear message and exit code 1. Surface gh's stderr in the error message.
- Rationale: The caller is an autonomous agent with no human at the keyboard; a
  silent fallback (browser, pre-filled URL) would be useless. Loud, structured
  failure is what an agent can detect and report.
- Rejected: *Browser fallback like `/millhouse-issue`* — pointless headless.

### labels-default-bug

- Decision: `--label` is repeatable and defaults to `["bug"]` when unset. Any
  supplied `--label` flags replace the default. No keyword/title heuristic.
  **Implementation note:** declare the flag as a pflag `StringArray` with an **empty**
  default (`[]string{}`) — **not** `StringSlice` (which CSV-splits values, so a label
  containing a comma would be mangled). Apply the `bug` default **explicitly in the
  RunE**, not as the pflag default: `if len(labels) == 0 { labels = []string{"bug"} }`.
  This is clearer and avoids depending on pflag's "first-Set-replaces-default"
  subtlety: with an empty pflag default, supplied `--label enhancement` yields exactly
  `["enhancement"]`, and an unset flag is filled with `["bug"]` by the RunE. Keep a
  test asserting `--label enhancement` produces `["enhancement"]` (the `bug` default
  is replaced, not appended).
- Rationale: Explicit and predictable for an autonomous caller; the agent decides
  the label, the tool does not guess. `bug` is the right default for a bug-reporter.
- Rejected: *Auto bug-vs-enhancement heuristic from title text* (like
  `/millhouse-issue`) — magic and unpredictable for a programmatic caller.

### body-optional-stdin

- Decision: `--body` / `-b` is optional. When its value is exactly `-`, read the
  entire body from stdin. Title-only issues are allowed.
- Rationale: Agents pipe long markdown reports via stdin (`... create "title"
  -b -`); humans/scripts may pass a short `-b "text"` or omit it. Flexible without
  forcing detail on quick filings.
- Rejected: *Body required* — blocks legitimate title-only filings; the agent
  always supplies a body anyway, so enforcement buys nothing.

### gh-runner-seam

- Decision: Introduce a package-level function variable for the gh invocation
  (e.g. `var runGH = realRunGH`) plus a package-level stdin reader seam
  (e.g. `var stdin io.Reader = os.Stdin`). Tests override both to assert the exact
  argument vector passed to `gh` and to feed stdin, with no real `gh` call or
  network. **Return contract:** `runGH` mirrors `gitexec.RunGit`'s signature —
  `func(args []string) (stdout, stderr string, exitCode int, err error)`. The
  production `realRunGH` first does `exec.LookPath("gh")`; a not-found result is
  surfaced as a non-nil `err` with `exitCode == -1` (distinct from a found-but-failed
  gh, which returns `err == nil`, `exitCode > 0`, and gh's message in `stderr`). This
  lets the handler branch into three distinct outcomes — gh-not-found, gh-non-zero-exit
  (surface `stderr`), and success — each with its own error message.
- Rationale: The codebase has no external-command mock; the established alternative
  is a `*_SKIP` env gate (board/weft), but a skip env would only no-op the call —
  it could not verify that the correct `gh issue create --repo … --title … --label …`
  args were assembled. A package-level overridable var is a clean, idiomatic Go
  testing seam that lets contract tests verify the assembled command precisely while
  the production default execs `gh` for real. This keeps the repo hardcode while
  remaining fully testable.
- Rejected: *`GHISSUES_SKIP` env gate* — cannot assert the command vector;
  *real gh against a throwaway repo in tests* — network/auth-dependent, flaky, and
  would actually file issues.

### rich-help-in-scope

- Decision: Discoverable `--help` is a first-class deliverable, not a nicety. The
  `ghissues` parent has a `Short`; `create` has a `Short` and a `Long` with concrete
  examples (`-b -` for stdin, `--label enhancement`), and the `Long` explicitly states
  that `-` means "read the body from stdin".
- Rationale: The sandbox-suite (task 031) reporting loop depends on an agent that has
  only `lyx.exe` on PATH — no source, no skills, no docs. Its *only* way to learn how
  to file a bug is `lyx ghissues create --help`. If the help text is thin, the whole
  downstream reporting step is unusable. A missing `Short` also fails the existing
  cobra-drift / JSON-help guard tests.
- Rejected: *Terse/auto-generated help only* — leaves the agent unable to discover the
  stdin and label conventions; breaks the 031 dependency.

### arg-validation-exactargs

- Decision: `create` uses `cobra.Args = cobra.ExactArgs(1)` so exactly one positional
  (the title) is required; zero or 2+ positionals are rejected by cobra before the
  RunE runs.
- Rationale: Predictable, explicit failure for a programmatic caller (e.g. an
  unquoted multi-word title surfaces as "accepts 1 arg(s)" rather than silently
  filing an issue titled with only the first word). Less hand-rolled `len(args)`
  checking in the RunE.
- Rejected: *Manual `len(args)` check* (warp-style) — works, but `ExactArgs(1)` is the
  idiomatic cobra mechanism and gives a consistent error message for free.

### cli-registration-enforcement

- Decision: Add two build-time guard tests under `cmd/lyx/` that mechanically enforce
  the CLI / Cobra Invariant's registration + self-documentation rules (which today are
  only pinned by hardcoded superset lists in `helptree_test.go` and a `Short`-only walk
  in `drift_test.go`):
  - **Test A — `cmd/lyx/registration_test.go`** (AST source scan, modelled on
    `internal/paths/enforcement_test.go`): walk `internal/`, parse every non-`_test.go`
    `.go` file with `go/parser`, and collect the package of every top-level
    `func Command() *cobra.Command` (no receiver, no params) into a *discovered* set.
    Parse `cmd/lyx/main.go` and collect the selector ident `X` from every `X.Command()`
    argument to `root.AddCommand(...)` into a *registered* set. Assert
    `discovered ⊆ registered`; any unregistered module → `t.Errorf` naming the package.
    An explicit (empty) `allowlist` var documents future exceptions. ("exists ⇒
    registered".)
  - **Test B — `cmd/lyx/longlist_test.go`** (live tree, modelled on
    `drift_test.go`'s `newRoot()` walk): for each child of `newRoot().Commands()`
    (skipping `help`/`completion`), assert `strings.Contains(root.Long, child.Name())`.
    The set is derived from the live tree — no hardcoded list. ("registered ⇒ in --help
    prose".)
- Rationale: The existing pinned `helptree_test.go` uses superset assertions, so a
  module that has a `Command()` but was never added to `newRoot()` passes silently; and
  `root.Long` can drift from the registered set. These two guards close both holes
  generically (for every present and future module, including `ghissues` itself). This
  is a deliberate scope expansion requested by the orchestrator (task 032).
- Implementation notes: assume selector-ident == package name (holds in this repo); if
  any module diverges, match on package name and note the assumption in a code comment.
  Keep `helptree_test.go` (subcommand-level pinning still has value) and update its
  pinned sets for `ghissues` as already planned. Extend (do **not** recreate) the CLI /
  Cobra Invariant section in `CONSTRAINTS.md` with a line about these two guards, in the
  same commit (per the task-completion rule).
- Rejected: *Rely on pinned superset lists only* — that is exactly the hole being
  closed; *put the scan in `internal/paths`* — wrong package; the registration scan
  belongs next to the root it guards (`cmd/lyx`).

## Technical context

What mill-plan needs to know about the codebase:

- **Module shape (warp variant).** Follow `internal/warp/warp.go`:
  `Command()` builds a parent `&cobra.Command{Use:"ghissues", Short: ...}` (the
  parent **must** carry a non-empty `Short`) with **no** `PersistentPreRunE` and
  **no** persistent flags. The `create` subcommand carries a `Short`, a `Long` (with
  examples — see rich-help-in-scope), `Args: cobra.ExactArgs(1)`, and
  `RunE: clihelp.WrapRun(runCreate)` where `runCreate` is
  `func(out io.Writer, args []string) int`. With `ExactArgs(1)`, the RunE can read the
  title as `args[0]` without a `len(args)` guard. Subcommand-local flags
  (`--body`/`-b`, `--label`) are declared on the `create` command and read inside the
  RunE via a closure over the `*cobra.Command` (see warp's `removeCmd` closure pattern
  at `internal/warp/warp.go:88-110`). Apply the label default in the RunE
  (`if len(labels) == 0 { labels = []string{"bug"} }`), and declare `--label` with
  pflag's `StringArray` (empty default), not `StringSlice`. `RunCLI` is the one-liner
  `return clihelp.Execute(Command(), out, args)`.
- **`clihelp.WrapRun`** (`internal/clihelp/exec.go:108-118`) bridges the
  `func(out,args) int` handler into a cobra `RunE`, checking `ShouldAbort` and
  calling `SetExit`. Use it instead of the manual `SetExit` (board/weft) variant.
- **Output envelope** (`internal/output/output.go`): `output.Ok(w, map[string]any{...})`
  injects `"ok":true` (mutates the map — always pass a fresh literal) and returns 0;
  `output.Err(w, msg)` writes `{"ok":false,"error":msg}` and returns 1. One JSON
  object per line.
- **External-command helper to mirror** (`internal/gitexec/gitexec.go:15-38`): copy
  the `exec.Command` + `bytes.Buffer` capture + `proc.HideWindow(cmd)` +
  `*exec.ExitError`→exit-code structure for the real gh runner. `proc.HideWindow`
  (`internal/proc/proc_windows.go`) prevents a console flash on Windows — call it
  for any subprocess.
- **Binary discovery**: use `exec.LookPath("gh")` with a graceful error. The
  LookPath-with-graceful-error idiom is established in `internal/muxpoc/review.go:47`
  and `internal/muxpoc/up.go:77` (those sites look up `"claude"`, not `"gh"` — the
  precedent is the pattern, not the binary name).
- **Registration** (`cmd/lyx/main.go`): add
  `"github.com/Knatte18/loomyard/internal/ghissues"` to the import block
  (lines 22-30), add `ghissues.Command(),` to the `root.AddCommand(...)` call
  (lines 94-103), and append `ghissues` to the `Long` module list string
  (line 83: "Available modules: init, board, config, update, ide, muxpoc, weft,
  warp.").
- **gh create output**: `gh issue create` has no `--json` mode; on success it prints
  the new issue's URL to stdout (e.g. `https://github.com/Knatte18/loomyard/issues/123`).
  Capture that line, trim whitespace, set `url` in the envelope, and parse the
  trailing path segment as `number` (int). If the trailing segment is not a parseable
  int, still return `ok:true` with `url` and omit `number` (don't fail a successful
  filing on a parse miss).
- **gh command vector**: `gh issue create --repo Knatte18/loomyard --title <title>
  [--body <body>] --label <l1> [--label <l2> ...]`. Pass title/body as discrete
  argv elements (never a shell string) — `exec.Command` does no shell interpolation,
  so this is injection-safe by construction.
- **No config files**: do **not** create `template.yaml`/`template.go`/`config.go`
  for ghissues and do **not** touch `internal/configreg`. The repo constant lives in
  Go source.
- **Suggested files**: `internal/ghissues/ghissues.go` (package-doc header + the repo
  constant + `runGH`/`stdin` seams + the core "build args, run gh, parse URL" logic)
  and `internal/ghissues/cli.go` (`Command()`, `RunCLI()`, the `create` RunE, output
  helpers). Tests in `internal/ghissues/cli_test.go` (and/or `ghissues_test.go`).

## Constraints

From `CONSTRAINTS.md`:

- **Path Invariant.** All cwd / worktree-root queries go through
  `internal/paths.Getwd()` / `paths.Resolve()`; raw `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go` (enforced by `internal/paths/enforcement_test.go`). **In
  practice ghissues needs no cwd/worktree resolution at all** (target repo is a
  constant, gh is run without a meaningful `cmd.Dir` requirement — pass `""` or omit
  `Dir`), so this invariant is trivially satisfied; just do not introduce a raw
  `os.Getwd`.
- **`_lyx` / config-file paths** must route through `paths.ConfigDir` /
  `paths.ConfigFile` — **not applicable**, since ghissues has no config file.
- **lyxtest leaf invariant.** `internal/lyxtest` must not import feature packages or
  `configreg`. ghissues does not register config, so there is nothing to wire here;
  ghissues tests do not need `lyxtest` config seeding.
- **Documentation lifecycle** (`docs/overview.md#documentation-lifecycle`): mechanical
  per-module design docs (`docs/modules/<module>.md`) are deleted when the module
  lands; durable purpose/rationale lives in the Go **package header comment**. So:
  do **not** create `docs/modules/ghissues.md`; put the design narrative in the
  package header (warp-style, `internal/warp/warp.go:1-49` is the model).

From the project `CLAUDE.md` **task-completion rule** (must be in the same commit as
the behaviour):

- Mark the milestone ✅ Done in `docs/roadmap.md` (add a milestone entry for the
  ghissues module under the appropriate section, e.g. setup & supporting milestones,
  and mark it done).
- Update `docs/overview.md` if the module table / module list / execution stack
  changes (at minimum the prose that enumerates modules, and the `--help` module
  list in `cmd/lyx/main.go`).
- No `docs/modules/ghissues.md` (see documentation-lifecycle above).

## Testing

Test framework: standard Go `testing`, table-friendly, `package ghissues_test`
(black-box), **untagged** so it runs on every `go test ./...` (mirror
`internal/board/cli_test.go`, not the `//go:build integration` weft variant). No real
`gh` call and no network in unit tests — drive everything through `RunCLI` with a
`bytes.Buffer` and override the `runGH` / `stdin` seams.

TDD candidates / scenarios to cover:

- **Happy path**: `RunCLI(buf, ["create", "My bug title"])` with a stubbed `runGH`
  that returns a URL on stdout and exit 0 → asserts envelope `ok:true`,
  `url` matches, `number` parsed correctly; and asserts the **exact argv** the seam
  received is `["issue","create","--repo","Knatte18/loomyard","--title","My bug
  title","--label","bug"]`.
- **Custom labels**: `["create","T","--label","enhancement","--label","p1"]` →
  argv contains `--label enhancement --label p1` and **not** the default `bug`
  (asserts the RunE default is replaced, not appended).
- **Body via flag**: `-b "details"` → argv contains `--body details`.
- **Body via stdin**: `-b -` with the `stdin` seam set to a reader → argv `--body`
  carries the full piped content; assert multi-line content survives intact.
- **Body omitted**: no `--body` → argv has no `--body` element; still `ok:true`.
- **Wrong arg count (ExactArgs)**: `["create"]` (zero positionals) and
  `["create","a","b"]` (two positionals) → non-zero exit, cobra's "accepts 1 arg(s)"
  error; `runGH` is **not** called.
- **gh not found**: stub the LookPath/runner seam to report gh-missing → `ok:false`,
  exit 1, message names gh/PATH; (verify the not-found path is distinct from a
  non-zero-exit path).
- **gh non-zero exit** (e.g. auth failure): stub `runGH` to return exit 1 + stderr →
  `ok:false`, exit 1, error message surfaces gh's stderr.
- **Unparseable URL on success**: stub `runGH` to return non-URL stdout, exit 0 →
  `ok:true`, `url` present, `number` omitted (success not failed on parse miss).
- **Number parsing**: URL ending `/issues/123` → `number == 123` (int, not string).
- **Registration smoke**: `lyx ghissues` (no subcommand) lists `create`; covered by
  the root help tree. **The plan must add `"ghissues"` to the `requiredModules` lists
  in both `cmd/lyx/helptree_test.go` and `cmd/lyx/jsonhelp_test.go`.** Those tests
  assert required-modules ⊆ rendered output (a *superset* check), so they pass whether
  or not `ghissues` is registered — without adding `ghissues` to `requiredModules`,
  the registration coverage is vacuous. Adding it makes the tests actually fail if
  the `cmd/lyx/main.go` wiring (import + `AddCommand` + `Long` list) is missing.
- **Help-tree subcommand guard**: add a `ghissues` case to the table in
  `cmd/lyx/helptree_test.go`'s `TestHelpTree_VerbModuleSubcommands` with
  `wantSubs: []string{"create"}`, so `lyx ghissues` (no subcommand) is asserted to
  exit 0 and list `create`. This is the cobra-drift guard the orch thread flagged.
- **Rich-help / Short guard**: the JSON-help tests in `cmd/lyx/jsonhelp_test.go`
  assert `short` is non-empty at the module level. The plan should add coverage that
  `lyx ghissues --json` reports a non-empty `short` and lists `create`, and that
  `lyx ghissues create --help --json` reports a non-empty `short` and exposes the
  `--body` and `--label` flags (mirror `TestJSONHelp_LeafWithFlag` for `warp remove`).
  This guards the in-scope requirement that the sandbox agent can discover the command
  and its stdin/label conventions via `--help`.
- **Registration guard (Test A, `cmd/lyx/registration_test.go`)**: model on
  `internal/paths/enforcement_test.go`. Assert `discovered ⊆ registered` — fails if any
  `internal/*` package with a `func Command() *cobra.Command` is not added to
  `newRoot()`. Should pass once `ghissues` is registered (card 4) and fail if that
  wiring is removed. Include a small predicate/sub-test sanity check in the style of
  enforcement_test.go's `predicate` sub-test if practical.
- **Long-list consistency (Test B, `cmd/lyx/longlist_test.go`)**: model on
  `drift_test.go`'s live-tree walk. For each `newRoot().Commands()` child (skip
  `help`/`completion`), assert `root.Long` contains the child name. Fails if a
  registered module is missing from the root `Long` string — including `ghissues` until
  card 4 appends it.

Injection safety is structural (argv passed to `exec.Command`, no shell), so it needs
an assertion that title/body are passed as single argv elements rather than a
dedicated escaping test.

## Q&A log

- **Q:** Should the target repo be a constant, a config key, or constant-plus-override?
  **A:** Hardcoded constant `Knatte18/loomyard`, no override of any kind — the module
  exists only to report to LoomYard regardless of the caller's cwd.
- **Q:** Wrap the `gh` CLI or call the GitHub REST API? **A:** Wrap `gh` (auth handled
  for free; matches gitexec / millhouse-issue; gh 2.89.0 present on PATH).
- **Q:** Ship `create` only or `create` + `list`? **A:** `create` only; `list`/dedup
  is a separable future task.
- **Q:** Behaviour when `gh` is missing or unauthenticated? **A:** Fail loudly with a
  structured `{ok:false,error:...}` JSON error and exit 1 — no browser fallback
  (caller is a headless agent).
- **Q:** Labels — default `bug`, auto-heuristic, or fixed? **A:** Default `bug`;
  `--label` repeatable and replaces the default; no title-text heuristic.
- **Q:** Is the issue body required? **A:** Optional; `--body`/`-b` sets it, `-b -`
  reads the whole body from stdin; title-only issues allowed.
- **Q (tie-breaker):** How to test a hardcoded-repo gh wrapper without a real gh/network?
  **A:** Package-level `runGH` + `stdin` seams overridden in tests to assert the exact
  gh argv; production default execs gh for real (gitexec-style).
- **Q (tie-breaker):** gh create has no `--json`; how to return number+url? **A:**
  Capture the URL gh prints to stdout; parse the trailing segment as `number`; on a
  parse miss return `ok:true` with `url` and omit `number` rather than failing.
- **Q (orch-thread, post-approval fold-in):** Is rich `--help` optional? **A:** No —
  it is in-scope. The sandbox-suite (031) agent has only `lyx.exe` and discovers this
  command via `lyx ghissues create --help`. `create` needs a `Short` + a `Long` with
  examples (`-b -` for stdin, `--label enhancement`), the parent needs a `Short`, and
  the cobra help-tree guard tests must be updated to include `ghissues`.
- **Q (orch-thread):** How is the `bug` label default applied? **A:** In the RunE
  (`if len(labels)==0 { labels = []string{"bug"} }`) with `--label` declared as a
  pflag `StringArray` (empty default) — explicit and clearer than relying on the
  pflag-default replacement behaviour. Keep the test that `--label enhancement`
  replaces `bug`.
- **Q (orch-thread):** How are positional args validated? **A:** `cobra.ExactArgs(1)`
  on `create` — reject zero or 2+ positionals explicitly.
- **Q (orch emphasis):** Capture that **lyx.exe uses Cobra.** **A:** Yes — the module
  is a Cobra command tree (warp variant): `cobra.Command` parent + `create` subcommand,
  `cobra.ExactArgs(1)`, `clihelp.WrapRun`, wired into the Cobra root in
  `cmd/lyx/main.go`, and guarded by the Cobra help-tree drift tests.
- **Q (orch scope expansion, task 032):** Should the task also close the CLI-framework
  registration hole? **A:** Yes — add Test A (`registration_test.go`, AST scan,
  exists⇒registered) and Test B (`longlist_test.go`, live tree, registered⇒in `Long`
  prose), modelled on `internal/paths/enforcement_test.go` and `drift_test.go`. Keep
  `helptree_test.go` and update its pins for `ghissues`. Extend the CLI/Cobra Invariant
  in `CONSTRAINTS.md` (do not recreate it) with a line about the two guards, same commit.
- **Q (orch correction):** Where does the `CONSTRAINTS.md` CLI/Cobra Invariant text
  come from? **A:** main's commit `6097e0b` added it (plus a "CONSTRAINTS.md is
  authoritative" note in `CLAUDE.md`); merged into this branch via `/mill-merge-in`
  before planning the guards, so the section is extended rather than reinvented.
