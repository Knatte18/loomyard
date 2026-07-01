# Discussion: CLI ergonomics from the sandbox run: config editor + warp error wrapping

```yaml
task: CLI ergonomics from the sandbox run: config editor + warp error wrapping
slug: sandbox-cli-ergonomics
status: discussing
parent: main
```

## Problem

Two WARN-verdict ergonomics findings came out of the 2026-07-01 sandbox suite run
(sources S4, S6; see `proposal-sandbox-cli-ergonomics.md`). Both are legibility/ergonomics
polish, not correctness breaks:

1. **S4 — `lyx config <module>` is interactive-only, with no documented fallback and no
   scriptable path.** With `$EDITOR`/`$VISUAL` unset, `lyx config board` silently opens
   Notepad (Windows) or `vi` (elsewhere) via `configengine.DefaultEditor` and blocks the
   process indefinitely. `lyx config --help` never mentions EDITOR/VISUAL or what the
   fallback is. There is also no way to change a single config value without physically
   driving a text editor (or faking one with a stand-in `EDITOR` script) — which is why the
   sandbox suite itself, an autonomous LLM run non-interactively, could not exercise this
   command and left orphaned Notepad windows behind.
2. **S6 — `lyx warp checkout` (and, on closer inspection, most of `internal/warpengine`,
   plus `weftengine/sync.go` and `hubgeometry/worktreelist.go`) leak raw git `fatal:`
   stderr verbatim into the JSON `error` field**, e.g.
   `{"error":"host switch failed: fatal: invalid reference: nonexistent-branch-xyz","ok":false}`.
   The envelope shape itself is fine (`ok:false` + `error` string) — the defect is that the
   `error` text is git's own unauthored text, not lyx's.

**Why now:** an identical-class bug (raw git `fatal:` leaking into `ErrNotAGitRepo`, GitHub
issue #36 point 3) was fixed today in `internal/hubgeometry` (commit `eeb539f`). That fix
discards stderr entirely and returns a bare, lyx-authored sentinel, with a test explicitly
pinning "no `fatal:` substring in the error text." This task extends that same just-established
convention to the remaining sites the audit above turned up, and fixes the config-editor
scriptability gap in the same pass since both were flagged by the same sandbox run.

## Scope

**In:**
- Document the EDITOR/VISUAL/notepad/vi fallback in `lyx config`'s `--help` (Long text).
- Add a repeatable `lyx config <module> --set key=value` flag that writes one or more
  config values directly to disk with **no editor invocation at all**, then runs the same
  post-edit weft sync `editOne` already runs on success.
- Strip raw git stderr out of the JSON-facing error text at every call site in
  `internal/warpengine`, `internal/weftengine/sync.go`, and `internal/hubgeometry/worktreelist.go`
  that currently embeds it (16 sites — full list below in Technical Context). Replace each
  with a message composed from data the Go code already has (action, branch/module/path,
  git exit code) — never git's own stderr text.

**Out:**
- Changing the sandbox suite's own test scheme/harness (that's `sandbox-suite-expand` /
  future suite-refinement tasks, not this one).
- `internal/boardengine` and `internal/selfreportengine`, which have the identical
  `"... failed: %s", stderr` pattern (`boardengine/git.go`, `boardengine/sync.go`,
  `selfreportengine/selfreport.go`) but are outside this WARN's module boundary (config +
  warp/weft only per the brief). Left for a separate task if desired.
- Adding a secondary/debug field to the JSON error envelope to preserve raw stderr
  somewhere. Considered and explicitly rejected (see Decisions) as bigger scope than this
  WARN needs — it would touch `internal/output`'s envelope shape for every command, not
  just warp/weft.
- Nested/dotted config keys for `--set` — every existing template (`board`, `warp`, `weft`)
  is flat top-level keys only; dotted-path support would be speculative.
- Type coercion of `--set` values (bool/int/float parsing) — all current template values
  are plain strings; `--set` writes the value as given, as a YAML string scalar.
- `lyx config reconcile` behavior — unchanged.

## Decisions

### Drop git stderr, author messages from known context (fix 2 approach)

- Decision: every one of the 16 sites below stops embedding raw git stderr in its error
  message. Instead the message is composed from what the calling Go code already knows —
  the action being performed, the branch/module/path involved, and the git exit code.
  Exit code is fine to include (it's a plain integer, not git's authored text).
- Rationale: matches the just-merged precedent (`eeb539f`, `internal/hubgeometry`) for the
  identical complaint class. That fix's test (`TestResolve_NotAGitRepo`) pins "no `fatal:`
  substring, exact sentinel text" — this task applies the same "never let git's own text
  into the error field" rule everywhere else it's violated in the config/warp/weft
  surface, instead of leaving 15 near-identical bugs for the next sandbox run to re-file
  one at a time.
- Rejected: (a) a generic helper that just strips a leading `"fatal: "`/`"error: "` prefix
  and keeps the rest of git's text — cheaper, but still leaks git's authored wording and
  contradicts the "zero raw git text" rule the precedent just established. (b) keep a bare
  lyx message plus raw stderr in a secondary debug field on the JSON envelope — matches the
  original WARN's own suggested fix text, but requires an `internal/output` envelope schema
  change that reaches every command, not just this task's two modules; out of scope for an
  ergonomics-polish task.

### Fix-2 scope: all 16 sites, not just `warp checkout`

- Decision: fix every site in `internal/warpengine` (`add.go`, `checkout.go`, `cleanup.go`,
  `clone.go`, `junction.go`, `prune.go`, `reconcile.go`, `weftwiring.go`), plus
  `internal/weftengine/sync.go` and `internal/hubgeometry/worktreelist.go`.
- Rationale: the WARN's repro names `warp checkout`, but the underlying pattern is
  systemic across the module — auditing turned up 16 occurrences, not the 1 originally
  repro'd (or the 10 first estimated mid-discussion). Fixing only the literal repro would
  leave the identical defect to be re-discovered piecemeal.
- Rejected: fixing `warp checkout`'s 3 sites only, deferring the rest. Rejected because the
  fix is mechanical and uniform once the convention is decided — deferring buys nothing and
  costs a future re-triage cycle.

### Fix-1 scope: both docs and `--set` (not docs-only)

- Decision: do both — document the EDITOR/VISUAL fallback in `--help`, and add
  `--set key=value` (repeatable).
- Rationale: the WARN's own suggestion was "and/or"; docs alone don't solve the actual
  automation gap (no autonomous caller can act on documentation), and `--set` alone would
  leave a still-undocumented implicit env-var contract for the interactive path that
  remains the default when no module/`--set` is given.
- Rejected: docs-only (doesn't fix the scriptability gap that caused the suite to leave
  orphaned Notepad windows); `--set`-only (leaves the EDITOR/VISUAL fallback undocumented
  for the interactive path, which still exists and is still the default).

### `--set` is repeatable and supports quoted values with spaces

- Decision: `--set` is a repeatable flag (multiple `--set key=value` occurrences in one
  invocation, applied together against a single file open/write/sync cycle). Each value is
  parsed by splitting the `key=value` string on the **first** `=` only, so values may
  contain `=` themselves (e.g. URLs). Values containing spaces work via normal shell
  quoting (`--set proposal_prefix="the new value"`) — the shell strips the quotes before
  the flag value reaches lyx, so no additional quote-parsing is needed in Go.
- Rationale: explicit user requirement — config values may legitimately contain spaces
  (e.g. `proposal_prefix`), and being able to set more than one key per invocation avoids
  forcing multiple process spawns + editor round-trips for a multi-value change.
- Rejected: single-key-per-invocation only (works, but forces N process spawns to change N
  keys, worse ergonomics than the interactive menu it replaces for multi-value edits).

### `--set` rejects unknown/misspelled keys

- Decision: `--set` validates each key against the module's template leaf-key set before
  writing anything; an unknown key (e.g. a typo) fails the whole invocation with a clear
  error listing the known keys for that module. No partial writes on a multi-`--set`
  invocation where one key is invalid.
- Rationale: matches the existing validation style elsewhere in this command (`editOne`/
  `printModule` already reject unknown module names). Silently writing a new dead key that
  nothing reads would be a worse ergonomics regression than the one being fixed.
- Rejected: allow arbitrary keys with no validation — more flexible in theory (e.g. a
  template key added but not yet reconciled to disk), but every current template's schema
  is small and fully known ahead of time, so the flexibility buys nothing but silent typos.

### `--set` requires a module positional and is mutually exclusive with `--print`

- Decision: `lyx config --set key=value` with **no module positional** (`Args` is
  `cobra.MaximumNArgs(1)`, so this is syntactically valid) is an error — `--set` always
  needs a module to know which file to write, and must fail clearly (e.g. "module required
  with --set") rather than doing nothing or guessing. Likewise, passing both `--print` and
  `--set` in one invocation is an error (mutually exclusive) rather than silently picking
  one — `--print` is read-only and `--set` writes, so combining them is always a caller
  mistake worth surfacing rather than resolving via silent precedence.
- Rationale: raised in discussion review round 1 — the dispatch-ordering description
  ("`--print` carved out first") left both the zero-module and both-flags-given cases
  unspecified. An unspecified case in a flag that writes to disk is worse than an explicit
  error.
- Rejected: silently defaulting to the interactive menu when `--set` has no module (surprising
  — a script passing `--set` never wants the menu); letting `--print` silently win when both
  flags are given (hides a caller mistake instead of surfacing it).

### `--set` scaffolds the config file from the template if missing

- Decision: if the module's `_lyx/config/<module>.yaml` doesn't exist yet, `--set` creates
  it from the template first (same as the interactive edit path), then applies the
  requested value(s).
- Rationale: mirrors `configengine.Edit`'s existing scaffold-on-missing behavior exactly;
  `--set` should work immediately after `lyx init` without requiring a separate
  `lyx config reconcile --apply` step first — that asymmetry would be a surprising gap
  between the interactive and non-interactive paths.
- Rejected: require the file to pre-exist and fail otherwise — technically simpler, but
  breaks the "no editor, no GUI, ever" promise `--set` exists to provide, since scaffolding
  is the one part of the interactive flow that has no GUI/editor dependency anyway.

## Technical context

### Fix 1 — config editor scriptability

- `internal/configengine/edit.go`: `DefaultEditor` (EDITOR/VISUAL/notepad/vi resolution)
  and `Edit` (scaffold + editor loop + abort contract) are the existing interactive
  machinery; `--set` is a **new, separate, non-interactive path** that never calls
  `DefaultEditor` — it should reuse `Edit`'s scaffold-if-missing behavior (or a shared
  helper factored out of it) but not its editor-loop/validation-loop logic.
- `internal/yamlengine/reconcile.go` (`Reconcile`, `collectLeafPathsHelper`) is the
  existing yaml.Node-based, comment-and-order-preserving mutation technique used for
  `lyx config reconcile` — the natural technique to reuse for `--set`'s value-preserving
  write (parse into a node tree, validate the key path against the template's leaf set,
  mutate the matching leaf node's `Value` in place, marshal back) rather than a
  `map[string]any` round-trip through `yaml.Unmarshal`/`Marshal`, which would lose
  comments and key order in the on-disk file.
- `internal/configcli/configcli.go`: `dispatch()` currently routes to `printModule`,
  `editOne`, or `menu()`. A `--set` flag needs a fourth path here (evaluated before the
  existing `printOnly`/module/menu branches, mirroring how `--print` is already carved out
  first). `editOne`'s post-edit sync call (`sync(&buf)` + success/failure message shape) is
  the pattern to mirror for `--set`'s own post-write sync.
  `Command()` currently registers only a `--print` bool flag; add a `--set` flag via
  `cobra`'s `StringArray` (not `StringSlice` — `StringArray` doesn't attempt comma-splitting
  a single value, which matters since values may contain commas).
- `buildConfigLong()` builds the `Long` help text embedding `configreg.Names()`; add the
  EDITOR/VISUAL/notepad/vi fallback documentation and a `--set` usage example here. Per
  CONSTRAINTS.md's CLI/Cobra Invariant, this Long text must be re-read and confirmed
  accurate against the actual new behavior as part of this change (review-blocking if
  stale).
- All three modules' templates (`internal/{board,warp,weft}engine/template.yaml`) are flat
  top-level `key: value` scalars only — confirms no nested-key support is needed.

### Fix 2 — 16 raw-git-stderr sites and their new message shape

Precedent: `internal/hubgeometry/hubgeometry.go` `Resolve()` (commit `eeb539f`) — discards
`stderr` from `gitexec.RunGit` entirely on non-zero exit, returns a bare sentinel. Pinned by
`TestResolve_NotAGitRepo` asserting no `"fatal:"` substring.

Full list (file:line → current message → known local context to compose the new message
from instead of `stderr`; git exit code may be included since it's not git-authored text):

| Site | Current | Known context available |
|---|---|---|
| `warpengine/checkout.go:88` | `host switch failed: %s` | `branch` |
| `warpengine/checkout.go:134` | `weft switch failed: %s` | `branch` |
| `warpengine/checkout.go:165` | `fork weft branch failed: %s` | `branch`, `parentWeftBranch` |
| `warpengine/add.go:147` | `worktree add failed: %s` | `branch`, `target` |
| `warpengine/add.go:172` | `weft worktree add (adopt) failed: %s` | `branch`, weft path |
| `warpengine/add.go:202` | `push failed: %s` | `branch` |
| `warpengine/cleanup.go:160` | `git branch exited %d: %s` | already has exit code; drop stderr |
| `warpengine/cleanup.go:183` | `git branch -D %s failed: %s` | `branch` (already interpolated) |
| `warpengine/clone.go:133` | `clone failed: %s` | `url`, `dest` (pre-normalization values read better than `gitURL`/`gitDest`) |
| `warpengine/junction.go:137` | `git rev-parse --git-path failed: %s` | `worktreePath` |
| `warpengine/prune.go:179` | `git worktree remove failed (%s); fallback ... also failed: %v` | `weftPath`; keep `removeErr` (that's a Go `os` error, not git stderr) |
| `warpengine/reconcile.go:225` | `git worktree add failed: %s` | `weftPath`, `branch` |
| `warpengine/weftwiring.go:75` | `weft worktree add failed: %s` | `weftPath`, `branch` |
| `warpengine/weftwiring.go:102` | `weft push failed: %s` | `branch` (weftPath available in scope) |
| `weftengine/sync.go:160` | `push failed: %s` (in `pushUnpushed`, after rebase-retry loop) | `weftPath` |
| `hubgeometry/worktreelist.go:40` | `git worktree list failed: %s` | `sourceDir` |

Each replacement should read roughly as `"<action> <target> failed (git exit %d)"` —
exact wording is an implementation call, not a re-litigated decision, as long as no
`stderr`/git-authored text reaches the message.

Note: `internal/boardengine/git.go` (2 sites), `internal/boardengine/sync.go` (1 site), and
`internal/selfreportengine/selfreport.go` (1 site) have the identical pattern but are
explicitly out of scope (see Scope → Out).

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant**: `configcli`'s `Short`/`Long` must stay accurate — this task
  changes observable `lyx config` behavior (new `--set` flag, documented fallback), so the
  Long text review is review-blocking per the invariant, not optional polish. No new
  subcommand/module is added, so `helptree_test.go`/`registration_test.go`/`longlist_test.go`
  pinned sets should not need updates — only `drift_test.go`'s existing `Short` presence
  check continues to apply unchanged. Errors must stay JSON via `internal/output`
  (`output.Ok`/`output.Err`) — this task's whole point is keeping that envelope's `error`
  field clean of git's raw text.
- **Hub Geometry Invariant**: `--set`'s file path must go through the existing
  `hubgeometry.ConfigFile`/`ConfigDir` helpers (already how `editOne`/`printModule` resolve
  paths) — no new geometry token usage.
- **lyxtest Leaf Invariant**: not implicated — no new test code is needed in
  `internal/lyxtest` itself.
- Per CLAUDE.md's Task Completion section: this task changes observable CLI behavior
  (`lyx config --set`, error message text), so `docs/overview.md`'s `config` module bullet
  (currently "interactive menu for viewing and editing module configs; `lyx config
  reconcile` reconciles...") must be updated in the same commit to mention `--set`. No new
  cross-cutting invariant is introduced, so no `CONSTRAINTS.md` update is needed. This is
  ergonomics/polish, not a planned milestone, so `docs/roadmap.md` must **not** be touched.

## Testing

- **`internal/configengine`** (or wherever the `--set` write path lands): TDD candidates —
  unknown-key rejection (single `--set` and as part of a multi-`--set` invocation, asserting
  no partial write occurred), scaffold-when-missing then set, value containing `=` and
  spaces round-trips correctly, multiple `--set` values in one invocation all land in one
  write+sync, comments/key-order in the on-disk file are preserved after a `--set` write
  (mirroring the existing `yamlengine.Reconcile` idempotency-style tests).
- **`internal/configcli`**: dispatch-level test that `--set` never invokes the editor
  (reuse the fake-`EditorFunc`-records-a-call pattern already used in
  `configengine`/`configcli` tests — assert the fake editor is never called), that sync
  runs on success and its failure is surfaced the same way `editOne`'s is, and a help-text
  test confirming the Long text mentions EDITOR/VISUAL and `--set`.
- **`internal/warpengine`, `internal/weftengine`, `internal/hubgeometry`**: each of the 16
  sites above almost certainly already has an existing non-zero-exit-code test case (the
  existing pattern throughout this codebase, e.g. `TestResolve_NotAGitRepo`'s style) —
  update each to additionally assert the new message and, mirroring
  `TestResolve_NotAGitRepo` exactly, assert the error text does **not** contain `"fatal:"`
  (or any substring only git would produce, if the test's fake/fixture stderr uses
  different wording).
- Manual/integration: `lyx warp checkout nonexistent-branch-xyz` from an initialized hub
  should now produce an `error` field with no `fatal:`/git text; `lyx config board --set
  proposal_prefix="a b"` from a fresh (or existing) hub should write the value with spaces
  correctly and print the same `edited and synced _lyx/config/board.yaml` success line
  `editOne` already prints, without any editor window appearing.

## Q&A log

- **Q:** How wide should the git-stderr-leak fix go — just `warp checkout`, or every call
  site with the same pattern? **A:** All of them (16 sites across `warpengine`,
  `weftengine/sync.go`, `hubgeometry/worktreelist.go` — grew from an initial ~10 estimate
  as the audit went deeper); `boardengine`/`selfreportengine` explicitly excluded as outside
  this WARN's module boundary.
- **Q:** Given the just-merged `eeb539f` precedent fully discards git stderr for the
  single-cause `ErrNotAGitRepo` case, should warp/weft's multi-cause failures do the same,
  strip only the `"fatal:"` noise prefix, or keep raw stderr in a secondary debug field?
  **A:** Fully discard, but compose the message from context Go already has (action,
  branch/path, exit code) rather than a bare unadorned sentinel — multi-cause failures need
  *some* differentiation, just not git's own text.
- **Q:** Should the config-editor fix be docs-only, a `--set` flag only, or both?
  **A:** Both.
- **Q:** Why does an agent/script need `--set` at all instead of just editing
  `_lyx/config/<module>.yaml` directly on disk? **A:** Clarified with a concrete
  before/after example of `board.yaml`; user confirmed `--set` is the right shape once it
  was concrete, and separately noted the sandbox suite itself hit exactly this gap (S4) —
  it could not drive the interactive editor non-interactively.
- **Q:** Does `--set` need to support values containing spaces? **A:** Yes — via normal
  shell quoting (`--set key="value with spaces"`); no custom quote-parsing needed in Go
  since the shell already strips the quotes before lyx sees the argument. Also confirmed
  `--set` should be repeatable (multiple keys in one invocation), not single-key-only.
- **Q:** Should `--set` validate the key against the template, or allow arbitrary keys?
  **A:** Reject unknown keys.
- **Q:** Should `--set` scaffold a missing config file from the template, or require it to
  pre-exist? **A:** Scaffold first, matching the interactive editor's existing behavior.
