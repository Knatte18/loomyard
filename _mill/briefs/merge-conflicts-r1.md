# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C C:\Code\loomyard\wts\cli-help-ergonomics rm <file>`.

### From discussion.md

# Discussion: CLI help & error ergonomics from sandbox run

```yaml
task: CLI help & error ergonomics from sandbox run
slug: cli-help-ergonomics
status: discussing
parent: main
```

## Problem

The first manual `lyx` sandbox run (`.scratch/sandbox-runs/2026-06-28.md`, in the
`loomyard` prime worktree) put an operator in front of `lyx.exe` with no source beside
them and recorded every rough edge. This task owns the **warp / weft / config** findings
plus the **cross-cutting error-format** findings (W2–W7, W9, W12, W14, W15, W16). The
board-module findings (B1, B2, W1, W10, W11, W13) are a **separate sibling task** and are
out of scope here.

Why now: lyx is being dogfooded standalone (the `lyx-sandbox` track). When the source
isn't beside you, the binary's own `--help` and error messages are the entire interface.
The run found that `warp`, `weft`, and `config` under-document themselves, that an unknown
subcommand silently shows help instead of erroring, and that error output is inconsistent
(domain errors are JSON, Cobra-level errors are plain text) — which makes programmatic use
unreliable. A first-draft fix (commit `c9d5c59`) was written and deliberately reverted
(`aff6740`) so the work could go through review rather than land unreviewed; parts of it
are reusable, parts are wrong (see Decisions).

## Scope

**In:**

- **warp**
  - `warp clone --reset` flag (idempotent re-clone: tear down an existing hub first) +
    a `Long` describing hub layout and derived board URL (W2/W3).
  - Unknown-subcommand error: `lyx warp <unknown>` errors instead of silently showing
    help (W16) — applied to **all** parent module groups, not just warp.
  - `warp add` `Long` documenting the fork point (W6).
  - Rename the `warp status` subcommand to `warp pairs`; clarify `warp list` vs the
    renamed `warp pairs` in their `Short`/`Long` (W7).
- **weft**
  - `weft commit` `Long` documenting the fixed commit message (W4/W9).
- **config**
  - `lyx config [module] --print`: non-interactive read-only mode emitting on-disk config
    (W12).
  - List the valid module names inline in `config` help (W5).
- **cross-cutting**
  - Wrap Cobra-level errors (unknown command/flag, arg-validation, and the new
    unknown-subcommand error) in the same `{ok:false,error:...}` JSON envelope as domain
    errors, centralized at the `clihelp` seam (W14).
  - Trim trailing whitespace/newlines from error messages (esp. embedded git errors),
    centralized in `output.Err` (W15).
- Update the pinned CLI tests (`cmd/lyx/helptree_test.go` for the `status`→`pairs` rename;
  the per-module unknown-command tests to also assert the JSON envelope) and add new
  mounted-group W16 tests.
- Update `docs/roadmap.md` (weft/config/warp command-surface changes) and add a note to
  the **CLI / Cobra Invariant** in `CONSTRAINTS.md` recording the JSON-wrapped-errors and
  reject-unknown-subcommand rules.

**Out:**

- All board-module findings (B1, B2, W1, W10, W11, W13) — sibling task.
- `ghissues` (032) and the sandbox launcher (031).
- W8 (no `lyx host commit`): the sandbox flagged that host changes are committed via raw
  `git`; that is by design and not addressed here.
- A `weft commit -m/--message` flag (document-only; see Decisions).
- A `warp add --base` flag (document-only; see Decisions).
- Any rename of `warp list`; no back-compat alias for the old `warp status` name.
- Changing `--json` help output, the JSON-per-line stdout contract for real command
  output, or the `RunCLI`/`clihelp.Execute` seam shape.

## Decisions

### W14 — JSON-wrap Cobra-level errors, centralized at the clihelp seam

- Decision: In `clihelp.Execute`, set `cmd.SilenceErrors = true` and, when
  `cmd.ExecuteContext` returns a non-nil error, emit it through
  `output.Err(out, strings.TrimSpace(err.Error()))` and return that exit code (1). Apply
  the identical wrapping to the root in `cmd/lyx/main.go` (`run()` and `main()`), factored
  into a shared helper so the module seam and the root don't drift. Help output (`--help`,
  bare-group listing) is a *successful* query and stays human-readable text (exit 0) — only
  actual errors are JSON-wrapped.
- Production stream: the wrapped JSON error goes to **stdout** (via `cmd.OutOrStdout()`),
  matching how domain errors already reach stdout through `output.Err`. In `cmd/lyx/main.go`
  `main()` keeps stdout/stderr split (`main.go:37-38`), so the wrapping must target stdout
  explicitly there — a programmatic caller reading stdout sees the same envelope for Cobra
  errors and domain errors. The merged-writer `run()` test seam captures it regardless.
- Root `SilenceErrors` flip (resolves the review's double-emit note): `newRoot()` currently
  sets `SilenceErrors: false` (`cmd/lyx/main.go:85`) so cobra prints its own plain-text
  error. Since `main()`/`run()` call `root.ExecuteContext` **directly** (not
  `clihelp.Execute`), `newRoot()` must set `SilenceErrors: true` as part of this change —
  otherwise the root path would double-emit (cobra plain text **and** the JSON wrapper).
  Both the seam (`clihelp.Execute`) and the root must set `SilenceErrors: true`; factor the
  "on non-nil error → `output.Err(stdout, TrimSpace(msg))`" wrapping into a shared helper
  both call so they cannot drift.
- Rationale: One change covers every module seam and the root. Cobra returns a non-nil
  error only for genuine Cobra-level failures (flag-parse, unknown command, arg
  validation, or an error returned by a `RunE`); our handlers use `clihelp.WrapRun`/`SetExit`
  and return nil, so they never collide with this path. `clihelp` may import `output`
  (no cycle: `output` depends only on stdlib).
- Low-risk detail: the original Cobra error text (e.g. `unknown command "x" for "lyx"`)
  remains embedded *inside* the JSON `error` string, so existing
  `strings.Contains(out, "unknown command")` / `"unknown flag"` assertions keep passing;
  tests only need an added `ok:false` envelope assertion.
- Rejected: per-module wrapping (drift risk); gating JSON errors behind `--json` (forces
  programmatic callers to opt in, defeats the consistency goal).

### W16 — Reject unknown subcommands on every parent group

- Decision: Give each parent module group (`board`, `warp`, `weft`, `ide`, `muxpoc`) a
  `RunE` that returns an `unknown subcommand %q for %q` error when extra args are present
  and otherwise calls `cmd.Help()`. Combined with W14 this surfaces as a JSON error. For
  the **four** groups with a layout-resolving `PersistentPreRunE` (`weft`, `board`, `ide`,
  `muxpoc` — verified: `weft/cli.go:43`, `board/cli.go`, `ide/cli.go:34`,
  `muxpoc/cli.go:81`), add a one-line guard at the top of that hook that returns early when
  `cmd` is the parent group itself (e.g. `cmd.Name() == "weft"`), so the bare-group help
  path and the unknown-subcommand error path do **not** trigger git/layout resolution.
  `warp` has no `PersistentPreRunE`, so it needs only the `RunE`.
- Rationale (verified against cobra v1.10.2 `command.go`): Cobra's `Find` only emits
  "unknown command" via `legacyArgs` when the matched command has **no parent** — that is
  why `lyx unknownmodule` (root) errors but `lyx warp foo` (mounted child) does not. In
  `execute()`, `if !c.Runnable() { return flag.ErrHelp }` (line 955) runs *before*
  `ValidateArgs` (968) and the `PersistentPreRunE` chain (985), so setting `Args: NoArgs`
  on a non-runnable group is ineffective (help short-circuits first). Making the group
  runnable via `RunE` is the idiomatic fix; the `PersistentPreRunE` guard preserves the
  existing "list subcommands without a git repo" property for weft/board/ide/muxpoc.
- Note on test asymmetry: the per-module `RunCLI(["bogus"])` tests pass today because in
  isolation the module *is* its own root (`!HasParent` → `legacyArgs` → "unknown
  command"). The silent-help bug only manifests when groups are mounted under `lyx`, so
  the new behavioural test belongs at the `cmd/lyx` level (`lyx warp bogus`,
  `lyx weft bogus`, etc.). In isolation the message stays "unknown command" (Find path);
  mounted it becomes "unknown subcommand" (our RunE) — both acceptable, both JSON-wrapped.
- Rejected: fixing warp only (inconsistent surface across modules).

### W4/W9 — Document the weft commit message; no `-m` flag

- Decision: Add a `Long` to `weft commit` stating the commit message is the fixed string
  `"weft sync"` (the `commitMessage` const in `internal/weft/weft.go`), and pointing to
  `weft push` (commit+push) and `weft sync` (async commit+push). Do **not** add a
  `-m/--message` flag.
- Rationale: weft commits are internal automation (config sync, codeguide) that nobody
  types by hand interactively, so a custom message adds plumbing through `weft.Commit`
  for negligible value (YAGNI). The reverted draft's `Long` claimed the message is
  "auto-generated from the set of changed files" — that is **factually wrong**; it must be
  rewritten to state the fixed `"weft sync"` message.
- Rejected: threading an optional message through `Commit()` and exposing `-m`.

### W12 — `config --print` emits raw on-disk YAML

- Decision: Add a `--print` boolean flag to `lyx config`. `lyx config <module> --print`
  prints that module's on-disk `_lyx/config/<module>.yaml` verbatim to stdout and exits 0;
  `lyx config --print` (no module arg) prints all known modules' files (each clearly
  delimited, e.g. a `# <module>` header line).
- Missing-file semantics (resolves the review's aggregate-print gap): config files are
  created lazily (`config.Edit` writes from the template on first edit), so absent files
  are normal. The **single-module** form (`config <module> --print`) **errors** (JSON
  envelope, exit 1) when that module's file is missing — the operator asked for a specific
  file. The **aggregate** form (`config --print`) is deterministic and never errors on
  absence: it iterates `configreg.Names()` in registry order and, for each module, emits
  the `# <module>` delimiter header followed by either the file's YAML (if present) or a
  single `# (not configured)` comment line (if absent), then exits 0. This makes the
  "dumps all modules" test deterministic regardless of which files exist. An unknown
  **module name** passed to `--print` still errors (JSON, exit 1) via the W5/W12
  harmonized error path. A hard I/O error (file exists but unreadable) errors (JSON, exit
  1) in both forms.
- Rationale: The operator in the run inspected config by reading the YAML file directly;
  `--print` reproduces that without launching the editor. Raw YAML preserves comments and
  the env-var-substitution template form (which a YAML→JSON conversion would lose) and is
  greppable. Success is raw text; errors stay machine-readable JSON for consistency with
  the rest of the CLI.
- Resolution of the config base dir mirrors the editor path:
  `filepath.Join(l.WorktreeRoot, l.RelPath)` then `paths.ConfigFile(base, module)` — never
  a hand-built `_lyx/config/<module>.yaml` literal (Path Invariant).
- Rejected: a `{ok:true,config:{...}}` JSON envelope (loses comments/template form, adds a
  parse step); requiring a module arg with no aggregate dump (couples discovery to W5).
- Error-format harmonization (resolves the review's W12/W5 consistency note): `configcli`
  currently emits its unknown-module and edit/abort errors as **plain text** via
  `fmt.Fprintf` (`configcli.go:43,56,70`), not `output.Err`. To avoid the contradiction
  where `lyx config bogus` is plain text but `lyx config bogus --print` is JSON, **harmonize
  config's existing error paths to `output.Err`** so every config error is the JSON
  envelope. Specifically: `editOne`'s "unknown config module" (line 43), the `ErrAborted`
  abort message (line 56), the generic edit-error (line 56-57), and the sync-failure
  message (line 70), plus `runConfig`'s `Getwd`/`Resolve` failures (lines 130,137), all
  route through `output.Err`. The on-success `--print` output (raw YAML) and the
  "edited and synced" / interactive-menu success text stay as-is (success is not an error).
  Note: the unknown-module error text must still surface the known-module list (it
  currently prints `(known: %v)` from `configreg.Names()`) — preserve that inside the JSON
  `error` string so W5's discoverability is not lost on the error path.

### W5 — List valid module names in config help, derived dynamically

- Decision: Build the `config` command's `Long` at `Command()` construction so it includes
  the live module list from `configreg.Names()` (e.g. "Known modules: board, warp, weft,
  worktree, …"), rather than hardcoding names in a string literal.
- Rationale: Co-located, drift-proof — the list is assembled from the registry, so adding
  a module surfaces in help automatically (matches the "help assembled from each module's
  own self-description" principle). `ValidArgs` is already set to `configreg.Names()` for
  completion; the unknown-module error already prints `(known: …)`. This closes the gap for
  plain `--help`.
- Rejected: a hardcoded module list in the `Long` literal (rots on the next module add).

### W6 — Document `warp add`'s fork point (the invoking worktree's branch)

- Decision: Add a `Long` to `warp add` stating that the new pair is forked from **the
  branch currently checked out in the worktree you run `warp add` from** (that worktree's
  `HEAD`) — not `main`, and not whatever branch prime has checked out — and that it errors
  on a detached or unborn `HEAD`. No `--base` flag.
- Rationale: The sandbox finding ("assumes main?") is based on a wrong premise.
  `internal/warp/add.go` step 6b runs `git rev-parse --abbrev-ref HEAD` in
  `l.WorktreeRoot` (the worktree resolved from cwd) and uses that as `parentBranch`,
  aborting if it resolves to `HEAD` (detached) — so the behaviour already matches the
  desired semantics; only the documentation is missing.
- Rejected: adding a `--base <branch>` override (not requested by the run; adds flag +
  plumbing + rollback considerations).

### W7 — Rename `warp status` → `warp pairs`; clarify `warp list`

- Decision: Rename the subcommand from `status` to `pairs` (it shows full host↔weft pair
  geometry). Sharpen `warp list`'s `Short`/`Long` to make clear it lists host worktrees
  only and to point at `warp pairs` for full pair geometry. No back-compat alias for the
  old `status` name.
- Rationale: "list" reads as more complete than "status", which is backwards; "pairs"
  names what the command actually reports. The rename is low-blast-radius: the only
  references to the `status` *subcommand* are `internal/warp/warp.go` (the `Use:` string
  and `runStatus`) and the pinned `cmd/lyx/helptree_test.go` set — no launchers or Go
  callers invoke `lyx warp status` (verified by grep; the many `git status` hits are
  unrelated). Internal handler `runStatus` may be renamed `runPairs` for clarity. Two
  **doc-comment** references to "warp status" rot on rename and must be updated to "warp
  pairs": `internal/weft/cli_test.go:121` and `internal/weft/status_test.go:45` (comments
  only — no code or assertion change).
- Rejected: keep the name and only clarify help (chosen against by the operator);
  keep `status` plus a `pairs` alias (operator chose a clean rename, no alias).

### W15 — Trim error messages centrally in `output.Err`

- Decision: `strings.TrimSpace` the message inside `output.Err` so every error envelope is
  clean regardless of source (this also covers the embedded git `fatal: …\n` strings).
- Rationale: One change, the natural cross-cutting home, and it pairs with W14's trimming
  of Cobra error text. Fixes all multi-line/trailing-newline error sources, not just git.
- Rejected: trimming at the `gitexec` edge (more call sites, git-only).

### Reuse of the reverted `c9d5c59` draft

- Decision: Reuse it as a starting reference for `warp clone --reset` + `Long` (W2/W3) and
  the warp unknown-subcommand `RunE` (W16). **Rewrite** the `weft commit` `Long` (its
  "auto-generated from changed files" claim is false — see W4). The board `Long` portion of
  that commit belongs to the sibling task, not here.
- Rationale: The reverted code was sound for the warp pieces and only reverted to route
  through review; the weft piece was inaccurate.

## Technical context

CLI architecture (all governed by the **CLI / Cobra Invariant**, CONSTRAINTS.md):

- Every module exposes `Command() *cobra.Command` and a thin
  `RunCLI(out, args) int = clihelp.Execute(Command(), out, args)` seam. The root
  (`cmd/lyx/main.go newRoot()`) mounts each module's `Command()` and installs the
  persistent `--json` help flag.
- `internal/clihelp/exec.go`:
  - `Execute(cmd, out, args)` merges stdout+stderr into `out`, sets `SilenceUsage=true`,
    seeds per-invocation exit state, runs `ExecuteContext`, returns the exit code (1 on a
    non-nil Cobra error). **W14 changes this**: also set `SilenceErrors=true` and wrap the
    returned error via `output.Err`.
  - `WrapRun(fn)` bridges a `func(io.Writer,[]string) int` handler into a Cobra `RunE`
    (records exit via `SetExit`, returns nil). `Abort`/`ShouldAbort` short-circuit leaf
    bodies after a failed `PersistentPreRunE`.
- `internal/output/output.go`: `Ok(w, fields)` / `Err(w, msg)` — the JSON envelope
  (`{"ok":true,...}` / `{"ok":false,"error":msg}`), one object per line. **W15** trims
  `msg` here.
- `internal/clihelp/jsonhelp.go`: `--json` help renderer (`InstallJSONHelp`), inherited by
  all descendants. Unaffected by this task (help text only flows in, not errors).

Per-module specifics:

- **warp** (`internal/warp/warp.go`): the "warp variant" — parent group with **no**
  `PersistentPreRunE`; per-subcommand local flags read via a closure over the `*cobra.Command`
  (see `removeCmd`/`pruneCmd`/`cleanupCmd`). Add `clone --reset` the same way (a
  `cloneCmd` var, `Flags().Bool("reset", …)`, read it in the `RunE`). `runClone`/
  `runCloneWithReset` and `deriveHostName`/`hubSuffix`/`removeAll` live in
  `internal/warp/clone.go`. `warp add`'s fork logic is `internal/warp/add.go` step 6b.
- **weft** (`internal/weft/cli.go`): the "board/weft variant" — parent has a
  `PersistentPreRunE` resolving cwd→layout→config→pathspec into closure vars, plus a hidden
  `--weft-path` bypass. W16 guard goes at the top of this hook. `weft.Commit` /
  `commitMessage` const are in `internal/weft/sync.go` / `weft.go`.
- **board** (`internal/board/cli.go`): also has a `PersistentPreRunE` (W16 guard needed).
  Only its **group** gets the W16 RunE here; its leaf `Long` docs are the sibling task.
- **config** (`internal/configcli/configcli.go`): leaf-ish command `config [module]`
  (`cobra.MaximumNArgs(1)`, `ValidArgs = configreg.Names()`); `runConfig` resolves the
  layout and dispatches to `editOne` (module given) or `menu` (interactive). W12 adds a
  `--print` flag handled before the edit/menu dispatch; W5 enriches the `Long`.
  `configreg.Names()` / `configreg.Template(name)` provide the module list and templates.
- **Groups needing the W16 RunE**: `board`, `warp`, `weft`, `ide` (`internal/ide/cli.go`),
  `muxpoc` (`internal/muxpoc/cli.go`). `init`, `update`, `config` are leaf/optional-arg
  commands, not groups, and are excluded.
- **Groups needing the W16 `PersistentPreRunE` early-return guard**: `weft`
  (`cli.go:43`), `board` (`cli.go`), `ide` (`cli.go:34` — resolves cwd→layout, aborts
  without a git repo), `muxpoc` (`cli.go:81` — resolves the worktree root, emits
  `not a git repository` without one). `warp` has no `PersistentPreRunE` and is exempt.
  Each `PersistentPreRunE` already has the "no-arg listing never requires a git repo"
  intent in its doc comment — making the parent runnable (W16 RunE) is exactly what would
  break that, so the guard is mandatory for all four.

Gotchas:

- Path Invariant: never build `_lyx`/config paths from literals — use `paths.ConfigFile`,
  `paths.ConfigDir`, `paths.LyxDirName`. Applies to `config --print` and any test fixtures.
- The bare-group "no git repo needed to list subcommands" property must survive W16 — that
  is the entire reason for the `PersistentPreRunE` early-return guard on weft/board.
- `output` must stay a stdlib-only leaf; `clihelp` importing `output` is the new edge
  (acceptable, acyclic).

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** (primary): `Command()`/`RunCLI` seam unchanged; every command
  keeps a non-empty `Short` (enforced by `cmd/lyx/drift_test.go`
  `TestDriftGuard_AllCommandsHaveShort`); commands on the discovery path SHOULD carry a
  `Long` with examples; help stays co-located (no central table); the help tree is pinned
  by `cmd/lyx/helptree_test.go` (update the warp `wantSubs`: `status`→`pairs`); results go
  through the `internal/output` JSON envelope. Record the new JSON-wrapped-errors and
  reject-unknown-subcommand rules as an addition to this invariant **in the same commit**.
- **Path Invariant**: all `_lyx`/config path access via `internal/paths` helpers (W12).
- **Documentation Lifecycle / Task-completion**: behaviour changes update `docs/roadmap.md`
  in the same commit. There are **no** `docs/modules/{warp,weft,config}.md` design docs, so
  no per-module doc update is required (the existing `docs/modules/` set is `loom`, `mux`,
  `review`, `shuttle`, plus README) — confirm none needs a touch when implementing.

## Testing

TDD candidates (assertion shapes left to mill-plan):

- **W14 (clihelp/exec_test.go, cmd/lyx)**: a Cobra-level error (unknown flag, unknown
  command) now produces a single-line JSON `{"ok":false,"error":...}` envelope, exit 1.
  Update existing substring tests to additionally assert `ok:false` and well-formed JSON.
  Verify `--help` / bare-group listing still emits plain text and exit 0 (not wrapped).
- **W16 (cmd/lyx — mounted)**: new tests that `lyx warp bogus`, `lyx weft bogus`,
  `lyx board bogus`, `lyx ide bogus`, `lyx muxpoc bogus` each error (JSON, exit 1) instead
  of printing help; and that bare `lyx weft` / `lyx board` / `lyx ide` / `lyx muxpoc`
  still print the subcommand listing with **no git repo present** (guards the
  PersistentPreRunE early-return on all four groups — ide and muxpoc included, since both
  resolve layout in their hook and would otherwise emit `not a git repository`).
- **W15 (output)**: `output.Err` trims leading/trailing whitespace; an input like
  `"fatal: not a git repository\n"` yields a `error` value with no trailing newline.
- **W2/W3 (warp)**: `warp clone --reset` removes a pre-existing hub then clones (use the
  `removeAll` seam to assert teardown); `warp clone --help` / `--json` lists the `--reset`
  flag and a non-empty `Long`.
- **W7 (warp + helptree)**: `warp pairs` exists and produces the former `warp status`
  output; `warp status` no longer exists (errors as unknown subcommand via W16);
  `helptree_test.go` warp `wantSubs` updated.
- **W6 (warp)**: `warp add --help` `Long` mentions forking from the invoking worktree's
  branch (string-content assertion is sufficient; the fork behaviour itself is already
  covered by existing add tests).
- **W4 (weft)**: `weft commit --help` `Long` documents the fixed `"weft sync"` message and
  does not claim a `-m` flag exists.
- **W12 (configcli)**: `lyx config <module> --print` emits the on-disk YAML verbatim and
  exits 0 without invoking the editor (inject a fake editor/seam to assert it is never
  called); single-module `--print` on a **missing** file errors (JSON, exit 1).
  `lyx config --print` (aggregate) emits a `# <module>` header for every name in
  `configreg.Names()` — YAML for present files, `# (not configured)` for absent ones — and
  exits 0 **even when some files are missing** (seed only a subset to assert determinism);
  an unknown module name still errors (JSON, exit 1).
- **W5 (configcli)**: `config --help` `Long` contains every name from `configreg.Names()`
  (assert membership dynamically, not a hardcoded list).
- **Config error harmonization (configcli)**: `lyx config bogus` (unknown module, no
  `--print`) now emits the JSON `{ok:false,error:...}` envelope (not plain `fmt.Fprintf`
  text), exit 1, and the `error` string still contains the known-module names; the abort
  and sync-failure paths likewise emit JSON. Existing `configcli_test.go` assertions that
  match the old plain-text strings must be updated to the envelope.
- **drift/helptree**: full tree still passes `TestDriftGuard_AllCommandsHaveShort`; new
  `Long`s and the `pairs` rename reflected in pinned sets.

Existing tests in `internal/{warp,weft,board,ide,muxpoc}/...` and `internal/clihelp/...`
that assert `"unknown command"`/`"unknown flag"` substrings keep passing under W14 (text
stays embedded in the JSON) but should gain envelope assertions.

## Q&A log

- **Q:** How should Cobra-level errors be JSON-wrapped (W14)? **A:** Centralize at the
  `clihelp` seam (`SilenceErrors=true` + `output.Err` on the returned error), unconditional.
- **Q:** Apply the unknown-subcommand fix (W16) to warp only or all parent groups? **A:**
  All parent groups, via a shared RunE; guard weft/board `PersistentPreRunE`.
- **Q:** `weft commit` — add `-m` or document only (W4)? **A:** Document only; the message
  is the fixed `"weft sync"` const, and the reverted draft's "auto-generated" `Long` was
  wrong and must be rewritten.
- **Q:** `config --print` output format/scope (W12)? **A:** Raw on-disk YAML; per-module,
  and all modules when no arg; errors as JSON envelope.
- **Q:** `warp add` base branch — flag or doc (W6)? **A:** Document only. Clarified: it
  forks from the branch checked out in the worktree you run `warp add` from (that
  worktree's HEAD) — not `main`, not prime's branch. Code already does this (add.go 6b).
- **Q:** `warp list` vs `warp status` naming (W7)? **A:** Rename `status`→`pairs` (clean,
  no alias); clarify `list` help. Verified no external callers of `warp status`.
- **Q:** Where to trim git error whitespace (W15)? **A:** Centralize in `output.Err`.
- **Q (review r1 GAP):** Does the W16 `PersistentPreRunE` guard cover all affected groups?
  **A:** No — extended from {weft, board} to {weft, board, ide, muxpoc}; ide
  (`cli.go:34`) and muxpoc (`cli.go:81`) also resolve layout in their hook, so they need
  the guard too. Added ide/muxpoc bare-group no-git-repo tests.
- **Q (review r1 NOTE):** Which stream gets the W14 JSON error in production? **A:**
  stdout (via `cmd.OutOrStdout()`), matching domain errors; `main()` must target stdout
  explicitly given its split streams.
- **Q (review r1 NOTE):** config errors plain-text vs JSON? **A:** Harmonize config's
  existing edit/menu/unknown-module error paths to `output.Err` so `lyx config bogus` and
  `lyx config bogus --print` are both JSON; preserve the known-module list in the message.
- **Q (review r1 NOTE):** Stale "warp status" doc comments? **A:** Update
  `weft/cli_test.go:121` and `weft/status_test.go:45` to "warp pairs" (comments only).
- **Q (review r2 GAP):** What does aggregate `config --print` do when some module files are
  absent (they're created lazily)? **A:** Never errors — emits a `# <module>` header for
  every known module, YAML when present and `# (not configured)` when absent, exit 0. The
  single-module form still errors (exit 1) on a missing file.
- **Q (review r2 NOTE):** Does W14 need a root-side change too? **A:** Yes — `newRoot()`
  must flip `SilenceErrors` from false to true (`main.go:85`), else the root path
  double-emits cobra text + JSON. Shared wrapping helper used by both seam and root.
```


### From _mill/plan/00-overview.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
slug: "cli-help-ergonomics"
approved: true
started: "20260628-145356"
parent: "main"
root: ""
verify: go build ./...
```

### From _mill/plan/01-foundation-error-envelope.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "foundation-error-envelope"
number: 1
cards: 4
verify: go test ./internal/output/... ./internal/clihelp/... ./cmd/lyx/...
depends-on: []
```



- **Edits:**
  - `internal/output/output.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/clihelp/exec.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/output/output_test.go`
  - `internal/clihelp/exec_test.go`
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-w16-reject-unknown-subcommand.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "w16-reject-unknown-subcommand"
number: 2
cards: 7
verify: go test ./internal/warp/... ./internal/weft/... ./internal/board/... ./internal/ide/... ./internal/muxpoc/... ./cmd/lyx/...
depends-on: [1]
```



- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/ide/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/muxpoc/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/warp/warp_test.go`
  - `internal/weft/cli_test.go`
  - `internal/board/cli_test.go`
  - `internal/ide/cli_test.go`
  - `internal/muxpoc/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `cmd/lyx/unknown_subcommand_test.go`
- **Deletes:** none

### From _mill/plan/03-warp-features.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "warp-features"
number: 3
cards: 3
verify: go test ./internal/warp/... ./cmd/lyx/...
depends-on: [2]
```



- **Edits:**
  - `internal/warp/warp.go`
  - `internal/warp/clone.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/warp/warp.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/warp/warp_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-weft-commit-doc.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "weft-commit-doc"
number: 4
cards: 2
verify: go test ./internal/weft/...
depends-on: [2]
```



- **Edits:**
  - `internal/weft/cli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/weft/cli_test.go`
  - `internal/weft/status_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-config-print-and-help.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "config-print-and-help"
number: 5
cards: 4
verify: go test ./internal/configcli/...
depends-on: [1]
```



- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/configcli/configcli.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/configcli/configcli.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/configcli/configcli_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/06-docs-roadmap-constraints.md


```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "docs-roadmap-constraints"
number: 6
cards: 2
verify: null
depends-on: [3, 4, 5]
```



- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `docs/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Run `git -C C:\Code\loomyard\wts\cli-help-ergonomics add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C C:\Code\loomyard\wts\cli-help-ergonomics rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C C:\Code\loomyard\wts\cli-help-ergonomics rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. The `mill-merge-in` frontend reads this field and surfaces any losses to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\cli-help-ergonomics` for any git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\cli-help-ergonomics`.
