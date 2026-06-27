# Discussion: Built-in CLI help: lyx self-documents modules & commands

```yaml
task: 'Built-in CLI help: lyx self-documents modules & commands'
slug: builtin-cli-help
status: discussing
parent: main
```

## Problem

When `lyx` is used standalone — dogfooding in `lyx-test`, or anywhere the loomyard
source is not beside you — the binary must be able to describe its own command surface.
Today it cannot. `lyx` with no args prints a single bare line `usage: lyx <module>
[args...]` to stderr and exits 1 (`cmd/lyx/main.go` `run()`); an unknown module prints
only `unknown module: <x>`; and each module dispatches subcommands via a `switch` that
carries no descriptions. So you cannot discover what `lyx` can do from the binary alone —
you must read the source. That defeats standalone use, which is the whole point of a
deployed `lyx` on PATH.

**Why now:** standalone/dogfooding use (`lyx-sandbox`, hand-driving `lyx` in `lyx-test`)
is becoming the normal way to exercise the tool, and the undiscoverable command surface
is the immediate blocker. Make `lyx` self-documenting recursively: `lyx` lists modules,
`lyx <module>` lists that module's commands, and `--help` at any level shows usage. The
hard constraint is anti-drift — help must be co-located with the command it describes and
the top-level listing must be **assembled from** each module's own self-description, never
a hand-maintained parallel table, because help kept separate from the command rots the
moment the command changes.

## Scope

**In:**

- Introduce `github.com/spf13/cobra` as the command-tree framework for `lyx`.
- Each module exposes `func Command() *cobra.Command` carrying `Use` / `Short` / `Long`
  metadata co-located with its handlers. Applies to all 8 modules: `init` (initcli),
  `board`, `config` (configcli), `update`, `ide`, `muxpoc`, `weft`, `warp`.
- `cmd/lyx/main.go` builds a single root `lyx` cobra command assembled from every
  module's `Command()`. The root prints the module listing; each verb-module prints its
  subcommand listing; `--help` works at every level.
- Migrate each command's flags from stdlib `flag` to cobra/pflag so `--help` auto-lists
  per-flag usage. Mark internal flags (`--board-path`, `--weft-path`) hidden.
- Add a persistent `--json` flag that renders any help output (module listing, subcommand
  listing, `--help`) as structured JSON instead of human text.
- Preserve a thin `RunCLI(out io.Writer, args []string) int` adapter per module (and
  `RunInit` for initcli) as the public seam over `Command()`, so existing in-process tests
  keep compiling and passing.
- New tests: help-tree completeness, `--json` schema, exit codes, and a drift-guard that
  asserts every command in the tree has a non-empty `Short`.

**Out:**

- The JSON-on-stdout contract for **real command output** is untouched — every existing
  handler keeps writing `{"ok":true,...}` / `{"ok":false,...}` via `internal/output`.
  Only the help/usage surface and the typo/unknown-command surface change.
- No change to module behaviour, business logic, config resolution, or output schemas of
  any existing subcommand.
- No new modules or subcommands; this is purely the help/dispatch refactor.
- No shell-completion authoring effort beyond what cobra generates for free (cobra's
  built-in `completion` command is left enabled but is not a deliverable to test deeply).
- No migration of `lyx init` / `lyx update` / `lyx config` away from running on no-arg
  (see Decisions → no-arg-semantics). They keep their current primary UX.

## Decisions

### framework-cobra-vs-registry

- Decision: Use `github.com/spf13/cobra` for the command tree (not a hand-rolled command
  registry).
- Rationale: cobra is purpose-built for a self-documenting multi-command CLI — `--help`/
  `-h` at every level, `lyx help <module>`, "did you mean…?" suggestions, and shell
  completion all generated for free, with `Short`/`Long` living in the command struct
  (co-location native). Crucially it makes the anti-drift guarantee a **framework
  invariant**: a new module cannot appear in `lyx --help` without being registered as a
  child command with `Use`/`Short`. The registry would give the same guarantee only via a
  completeness test we must remember to write and maintain.
- Rejected: Hand-rolled registry (slice of command structs). Lower churn and zero
  dependency, but every help affordance (`--help`, suggestions, completion, flag usage)
  would be hand-built, and anti-drift would rest on a discipline-test rather than the
  framework. The dependency cost is negligible (pinned via `go.sum`, compiled into the
  static binary).

### integration-style-c-preserve-seam

- Decision: Style C — one root cobra tree built from per-module `Command() *cobra.Command`
  constructors, with a thin `RunCLI(out, args) int` adapter preserved per module as the
  public seam.
- Rationale: This is the best of both. The single root tree gives full cobra benefits at
  the top level (`lyx --help`, completion, suggestions across the whole tree). The
  preserved `RunCLI` adapter means the ~51 existing in-process test call sites keep
  compiling and passing unchanged — signature, JSON-on-stdout, and exit-code contracts are
  all preserved. Handler logic moves from `switch` cases into `RunE` bodies essentially
  verbatim.
- The adapter shape:

  ```go
  func RunCLI(out io.Writer, args []string) int {
      c := Command()
      c.SetOut(out)
      c.SetErr(out) // merge cobra's stderr (unknown-command text) into the single seam writer
      c.SetArgs(args)
      if err := c.Execute(); err != nil { return 1 }
      return exitCodeFromHandlers // see exit-and-error-contract
  }
  ```

- **stdout/stderr split — seam merges, binary separates.** The
  `unknown-command-human-text` decision sends cobra's typo/usage text to **stderr**, but
  the `RunCLI(out, args) int` seam exposes a single writer. Resolution: the seam wires
  **both** `SetOut(out)` and `SetErr(out)`, deliberately *merging* the two streams into
  `out` so in-process tests (e.g. the named weft/ide unknown-command assertions) can
  capture cobra's message from the one buffer. Production keeps them split:
  `cmd/lyx/main.go` builds the root with `SetOut(os.Stdout)` and `SetErr(os.Stderr)` so
  human/typo text goes to stderr and JSON output to stdout. Tests **must not over-pin** the
  exact unknown-command string: via a per-module seam it reads `unknown command "x" for
  "board"`, whereas via the assembled root it reads `... for "lyx board"` — assert on a
  stable substring (`unknown command`) and the exit code, not the full parent qualifier.

- Rejected: (a) "Pure cobra" — dissolve `RunCLI`, drive tests via a shared
  `exec(t,&buf,args...)` helper. Cleaner conceptually but ~1 day of mechanical test churn
  (replace 51 call sites) for no functional gain. (b) "Adapter-per-module with no shared
  root" — each module builds an independent cobra root inside `RunCLI`; then top-level
  `lyx --help` and cross-tree completion are NOT cobra-generated and main.go must
  hand-maintain the module listing, partially defeating the anti-drift goal.

### unknown-command-human-text

- Decision: On an unknown module or unknown subcommand (a typo on the command tree), let
  cobra print its `unknown command "x" for "lyx"` message plus "did you mean…?"
  suggestions to **stderr**, and exit 1. Real command errors (a valid command failing,
  e.g. weft's `subcommand requires a worktree context`) stay JSON `{"ok":false,...}` on
  stdout as today.
- Rationale: The unknown-command surface is a human typo path with no machine consumer;
  cobra's suggestions maximize discoverability, which is the whole point of the task. The
  JSON error contract only matters for real, programmatically-consumed failures, which are
  unaffected.
- Rejected: Wrapping unknown-command as a JSON `{"ok":false,"error":"unknown command:
  x"}` envelope on stdout. Uniform with the output contract but throws away cobra's
  suggestions and human readability for the one surface that most needs them.

### no-arg-semantics

- Decision: Adopt the cobra-idiomatic no-arg split. Verb-modules with subcommands
  (`board`, `warp`, `weft`, `ide`, `muxpoc`) have **no `Run`** on the parent, so
  `lyx <module>` prints that module's subcommand listing and exits 0. Flat/interactive
  modules keep their current behaviour: `lyx init` scaffolds, `lyx update` dry-runs,
  `lyx config` opens the interactive menu (each has a `Run`/`RunE`); their help is reached
  via `lyx <module> --help`. Bare `lyx` prints the module listing and exits 0.
- Rationale: This is cobra's default behaviour with zero extra code — a parent command
  with no `Run` prints help; a leaf with a `Run` executes. Forcing help onto
  `lyx init`/`update`/`config` would break their primary UX and require inventing new
  explicit verbs (`lyx config menu`, `lyx update apply`), which is gratuitous churn.
- Rejected: Forcing `lyx <module>` with no further arg to print help uniformly for all 8
  modules. More superficially consistent, but changes the established primary UX of three
  commands for no real benefit.
- Note on exit code: bare `lyx` now exits **0** (help is a successful query), changing
  today's exit 1. This is intentional and consistent with treating help as success.

### flags-to-pflag

- Decision: Migrate every command's flags from stdlib `flag.FlagSet` to cobra/pflag
  (`cmd.Flags()` / `cmd.PersistentFlags()`), so `--help` auto-renders per-flag usage via
  cobra. Hide the internal injected flags `--board-path` (board) and `--weft-path` (weft)
  via `MarkHidden`. muxpoc's top-level tuning flags (`--psmux`, `--pwsh`, `--claude`,
  `--launch`, `--resume`, `--width`, `--height`, `--interval`) become **persistent** flags
  on the `muxpoc` parent (inherited by its subcommands). warp's per-verb flags (`remove
  --force`, `prune --apply`, `cleanup --apply/--force`) and update's `--apply` become local
  flags on their respective commands.
- Rationale: Auto-generated, co-located flag help is exactly the per-flag layer the task
  asks for ("per-flag/arg usage lives in the command's own flagset"). pflag's stricter
  parsing is safe here: an audit of every programmatic/injected call-site shows they all
  already use double-dash long flags (`--board-path`, `--weft-path`, `--force`, `--apply`)
  — no single-dash long flags exist anywhere — so pflag breaks nothing.
- Verified call-sites for the internal flags (must keep working after migration):
  - `internal/board/spawn.go:27` → `exec.Command(exe, "board", "--board-path", abs,
    "sync")` (flag before subcommand → must be a **persistent** flag on the `board`
    parent so cobra accepts it pre-subcommand).
  - `internal/weft/spawn.go:34` → `exec.Command(exe, "weft", "--weft-path", abs, "push")`
    (same — persistent flag on the `weft` parent).
- Rejected: Keep stdlib `flag` parsing inside `RunE` and hand-write flag help in `Long`.
  Less churn but flag help no longer auto-generates and stays drift-prone, undercutting a
  primary cobra benefit.

### json-help-form

- Decision: Provide a `--json` rendering of help, in addition to the human listing. A
  persistent `--json` bool flag on the root; when set on a help path (no-arg module/
  subcommand listing, or `--help`), the help output for that node is emitted as structured
  JSON to stdout instead of human text.
- Rationale: Operator opted in. Enables future tooling/completion generation off a stable
  machine-readable description of the command tree.
- Schema (per node):

  ```json
  {
    "name":  "lyx warp",
    "short": "host↔weft coordination",
    "long":  "…",
    "commands": [
      { "name": "add", "short": "create a dormant host+weft pair", "usage": "lyx warp add <slug>" }
    ],
    "flags": [
      { "name": "--force", "shorthand": "", "usage": "…", "default": "false", "type": "bool" }
    ]
  }
  ```

  At a leaf command `commands` is empty and `flags` is populated; at a parent the reverse.
  Hidden flags (`--board-path`, `--weft-path`) are omitted from the `--json` output too.
- Scope of `--json`: it **only** alters help rendering. On a real command-execution path
  (a leaf handler actually running, e.g. `lyx board list --json`) `--json` is a **no-op** —
  parsed but ignored, never an error — because real command output is already JSON. So the
  flag is meaningful exactly on the help/no-arg/`--help` paths and inert everywhere else;
  a plan writer does not need to special-case it inside any handler.
- Rejected: Human-only help (YAGNI). Overruled by the operator in favour of the structured
  form.

### exit-and-error-contract

- Decision: Configure the root with `SilenceUsage = true` and `SilenceErrors = false`.
  Handlers (`RunE`) write their JSON envelope via `internal/output` exactly as today and
  signal failure by recording exit code 1 (a holder captured by the module's `Command()`
  constructor) and returning `nil` to cobra — so cobra never prints a second "Error:" line
  on top of a handler's JSON. Cobra-level errors (unknown command, bad flag) are left
  un-silenced so cobra prints their human text + suggestions to stderr; `Execute()`
  returning such an error maps to exit 1 in the adapter/`main`.
- Rationale: This cleanly separates the two failure surfaces: handler failures → JSON +
  exit 1 with no cobra noise; tree/flag failures → cobra's human text + suggestions +
  exit 1. It preserves every existing JSON error assertion while enabling the
  unknown-command UX decision above.
- Note: `output.Ok`/`output.Err` already return the int exit code today; the holder simply
  captures that return value inside the `RunE` wrapper. The exact holder mechanism (closure
  variable vs. a small struct) is an implementation detail for mill-plan, but the contract
  above is fixed.

## Technical context

What mill-plan needs to know about the codebase:

- **Entry point** — `cmd/lyx/main.go`: `main()` calls `os.Exit(run(os.Args[1:],
  os.Stdout))`; `run()` is a `switch module { case "init": return initcli.RunInit(...) … }`
  dispatcher writing to an `io.Writer`. This becomes: build the root cobra command from
  each module's `Command()`, `rootCmd.SetArgs(os.Args[1:])`, `Execute()`, map to exit
  code. The doc-comment module table at the top of `main.go` (lines 11–20) becomes
  redundant and should be updated/removed (the listing now derives from `Short`s).
- **Module signature today** — every module exposes `func RunCLI(out io.Writer, args
  []string) int` (initcli exposes `func RunInit(...)`; it is a flat command that ignores a
  verb). All write JSON via the shared `internal/output` helpers: `output.Ok(out, map)`
  → `{"ok":true,...}` exit 0; `output.Err(out, msg)` → `{"ok":false,"error":...}` exit 1.
- **Per-module shape** (the three dispatch families the design must absorb):
  - Verb-switch modules: `internal/board/cli.go` (subcommands: upsert, upsert-batch,
    set-phase, remove, get, list, list-full, merge, set-deps, rerender, sync; internal
    `--board-path` flag), `internal/warp/warp.go` (clone, add, list, remove, checkout,
    status, reconcile, prune, cleanup; per-verb flags on remove/prune/cleanup),
    `internal/weft/cli.go` (status, commit, push, pull, sync; internal `--weft-path`
    flag), `internal/ide/cli.go` (spawn `<slug>`, menu), `internal/muxpoc/cli.go` (up,
    review, attach, status, down, daemon; top-level tuning flags).
  - Flat commands: `internal/initcli/initcli.go` (`RunInit`, no verb — `lyx init`
    scaffolds), `internal/update/update.go` (no verb — `lyx update`, with `--apply`).
  - Module-name + interactive: `internal/configcli/configcli.go` — `lyx config` opens an
    interactive menu; `lyx config <module>` edits that module's config (positional is a
    module name, not a verb). Under cobra this is one command with an optional positional
    arg and a `Run`; consider `ValidArgs`/`ValidArgsFunction` set to the known module names
    for completion + suggestions. Its existing `unknown config module: %s (known: …)`
    validation stays inside the handler (it is a real error, not a tree typo).
- **Output helpers** — `internal/output` is the single JSON sink; do not bypass it. Keep
  every handler routing success/error through it.
- **Internal flag injection** (must survive migration) — `internal/board/spawn.go:27` and
  `internal/weft/spawn.go:34` shell out to `lyx board --board-path … sync` and `lyx weft
  --weft-path … push` respectively. These flags must be **persistent** on their parent so
  cobra accepts them positioned before the subcommand.
- **No existing cobra usage** — `go.mod` (module `github.com/Knatte18/loomyard`, Go 1.26)
  has no cobra today; deps are `gofrs/flock`, `golang.org/x/sys`, `yaml.v3`. Adding cobra
  pulls in `spf13/pflag` (its only real transitive dep).
- **Gotcha** — `internal/update/update.go` deliberately blanks `fs.Usage` to suppress
  stdlib flag help; that hack is removed when flags move to pflag.
- **Gotcha (muxpoc pre-dispatch is NOT a verbatim per-case move)** — `internal/muxpoc/
  cli.go` (≈ lines 54–94) builds `cfg` from the tuning flags **and calls
  `paths.Resolve(layout)` once before the `switch`**, shared by every subcommand. This
  shared setup has no single `RunE` home and must NOT run on the new no-arg/help listing
  path — `paths.Resolve` would wrongly require a git repo just to *list* muxpoc's
  subcommands. Move it into a **`PersistentPreRunE` on the `muxpoc` parent**: cobra runs
  `PersistentPreRunE` only when a subcommand actually executes, and skips it for the
  no-arg/`--help` listing (parent has no `Run`). So the "handlers move into `RunE`
  essentially verbatim" note from `integration-style-c-preserve-seam` has this one
  exception — muxpoc's pre-`switch` block becomes `PersistentPreRunE`, not per-`RunE`
  duplication. (configcli's `paths`/config resolution sits inside its single `Run`, so it
  is unaffected; only muxpoc has shared pre-dispatch across multiple subcommands.)
- **Gotcha** — two usage-output conventions coexist today (plain-text-to-stderr in
  board/weft/muxpoc top-level dispatch; JSON via `output.Err` in initcli/ide/warp). After
  this task the usage/no-arg/typo surface is uniformly cobra (human text or `--json`), and
  the JSON envelope is reserved for real command results/errors.

## Constraints

- **Anti-drift is the core constraint**: help text co-located with the command; the
  top-level listing assembled from per-module `Short`s; no central hand-maintained table.
  Enforced structurally by cobra plus the drift-guard test (every command has a non-empty
  `Short`).
- **Preserve the real-output JSON contract**: existing subcommands' stdout JSON and exit
  codes are unchanged. Only help/usage/typo surfaces change.
- **Preserve the in-process test seam**: `RunCLI(out, args) int` / `RunInit(out, args)
  int` must remain callable so existing tests keep working.
- **pflag double-dash**: all programmatic/injected flag call-sites must use `--long`
  form (already true; keep it true for any new injection).
- **Cross-platform**: `lyx` runs on Windows (junctions, `cmd /c code`, psmux) and
  POSIX; nothing in the help refactor is platform-specific, but the muxpoc spawn/attach
  files are split by GOOS — leave that structure intact.
- No `CONSTRAINTS.md` exists at the hub root.

## Testing

Approach: keep the existing in-process `RunCLI(&buf, args)` tests working via the
preserved adapter, fix the handful that assert exact usage text, and add tree-level tests
for the new help surface. All tests stay in-process (no subprocess) per the current
pattern; cwd-dependent modules continue to use `internal/lyxtest` helpers + `t.Chdir`.

- **Existing tests (regression)** — the ~51 `RunCLI`/`RunInit`/`run()` call sites across
  ~11 files should pass unchanged because the adapter preserves signature + JSON + exit
  codes. Expect ~3–5 assertions to need updating where they assert exact usage/unknown
  text (the surface that now comes from cobra): `internal/configcli/configcli_test.go`,
  `internal/ide/cli_test.go`, `internal/weft/cli_test.go`, and `cmd/lyx/main_test.go`
  (`TestRunUnknownModule`, and the bare-`lyx` test now expecting exit 0 + a module listing
  rather than exit 1 + empty output).
- **TDD candidates (write first):**
  - **Drift-guard** (`cmd/lyx`): walk the assembled root command tree; assert every
    command (root, each module, each subcommand) has a non-empty `Short`. This is the
    structural self-documentation invariant — failing it is the signal someone added a
    command without a description. **Must tolerate cobra's auto-added commands**: cobra
    injects `help` and `completion` (the latter with `bash`/`zsh`/`fish`/`powershell`
    children) into the tree, and `completion`'s subcommands carry cobra's own non-empty
    `Short`s. Either exclude the `help`/`completion` subtrees from the walk explicitly, or
    rely on the fact that they already have descriptions — but do NOT assert an exact tree
    shape that would break when cobra changes its auto-commands.
  - **Help-tree completeness** (`cmd/lyx`): assert `lyx --help` output names every module;
    for each verb-module assert `lyx <module> --help` names every one of its subcommands.
    Use **superset** assertions (the pinned module/subcommand set is a subset of what
    appears), or explicitly exclude `help`/`completion`, so adding/removing one of OUR
    commands without updating help fails, while cobra's auto-commands don't make the test
    brittle.
  - **`--json` help schema** (`cmd/lyx`): `lyx --json`, `lyx <module> --json`, and a leaf
    `lyx <module> <cmd> --help --json` each emit valid JSON matching the schema
    (`name`/`short`/`commands`/`flags`); hidden flags absent; leaf has populated `flags`
    and empty `commands`.
  - **Exit-code contract** (`cmd/lyx` + per module): bare `lyx` → exit 0; `lyx <verb-module>`
    no-arg → exit 0 with subcommand listing; unknown module/subcommand → exit 1 with a
    stderr message; a real handler failure → exit 1 with a JSON `{"ok":false}` envelope on
    stdout (assert one representative, e.g. weft `--weft-path … status` still gives the
    JSON `subcommand requires a worktree context`).
- **Per-module scenarios to keep covered** (already exist; must still pass):
  - board: a representative success (e.g. `list`/`rerender`) and the JSON error envelope;
    `--board-path` injection path still resolves.
  - warp: `list` success, `remove --force <slug>` flag parsing + effect, unknown-subcommand
    error.
  - weft: `--weft-path` + non-push subcommand still yields the JSON
    `subcommand requires a worktree context`.
  - update: dry-run (no flag) vs `--apply`.
  - initcli: `lyx init` success and the no-pairing error path.
  - configcli/ide/muxpoc: their existing in-process tests.
- **No-arg behaviour tests**: `lyx init`/`lyx update`/`lyx config` still *run* (not print
  help) on no-arg; `lyx <verb-module>` prints the listing. Cover at least one of each.

Avoid prescribing exact assertion shapes for the new tests beyond the schema above — that
is mill-plan's job.

## Q&A log

- **Q:** Hand-rolled command registry or spf13/cobra? **A:** cobra — it is more
  comprehensive (free `--help`/completion/suggestions) and, decisively, makes anti-drift a
  framework invariant: a new module can't appear in help without a `Use`/`Short`, vs. the
  registry relying on a completeness test we must remember to write.
- **Q:** How much work is the test-suite migration under cobra? **A:** Small if we keep the
  `RunCLI`/`Command()` seam (Style C): the ~51 in-process call sites keep working; only
  ~3–5 exact-text assertions change. The bulk of the work is production-side (switch →
  cobra tree), which is the feature itself.
- **Q:** Integration style? **A:** Style C — one root cobra tree built from per-module
  `Command() *cobra.Command`, with a thin `RunCLI(out,args) int` adapter preserved for
  tests/compat.
- **Q:** Unknown module/subcommand (typo) behaviour? **A:** cobra human text + "did you
  mean…?" to stderr, exit 1. Real command errors stay JSON `{"ok":false}`.
- **Q:** No-arg semantics across verb / flat / interactive modules? **A:** cobra-idiomatic
  split — verb-modules print their subcommand listing (exit 0); `init`/`update`/`config`
  keep running on no-arg, help via `--help`. Bare `lyx` now exits 0.
- **Q:** Migrate flags to cobra/pflag? **A:** Yes — auto-generated per-flag help; hide
  internal `--board-path`/`--weft-path`. Audit confirms all injected flags already use
  double-dash, so pflag's stricter parsing breaks nothing.
- **Q:** Offer a `--json` form of help for tooling? **A:** Yes (operator opted in, over the
  YAGNI recommendation) — a persistent `--json` flag renders any help node as structured
  JSON per the documented schema.
- **Q:** How do exit codes / error printing reconcile with cobra? **A:** `SilenceUsage=true`,
  `SilenceErrors=false`; handlers write JSON and signal failure via an exit-code holder
  returning `nil` to cobra (no double-print); cobra-level errors (unknown command/bad flag)
  are printed by cobra and map to exit 1.
- **Q:** Scope of modules? **A:** All 8 — board, warp, weft, ide, muxpoc, config, init,
  update. Real-output JSON contract untouched.
