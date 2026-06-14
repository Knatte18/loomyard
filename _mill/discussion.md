# Discussion: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep

```yaml
task: Fix stale .mhgo/ config-layer docs + cross-cutting stale-docs sweep
slug: docs-stale-sweep
status: discussing
parent: main
```

## Problem

During the worktree-module work we found `docs/` and Go doc-comments that no longer
match shipped code. The headline case is the **`.mhgo/` config layer**: board.md and
`internal/board/cli.go` still describe a three-layer config merge with a gitignored
`.mhgo/board.yaml` override "as what board does today". That layer is gone — board
migrated to `internal/config`, a **two-layer** loader (built-in defaults +
`_mhgo/<module>.yaml`) with `.env` loading and `$env:NAME` / `$env:NAME ? fallback`
expansion. There is no `.mhgo/` config layer at all; `.mhgo/` today is only the
gitignored **runtime-state** dir (e.g. `internal/muxpoc` writes
`.mhgo/muxpoc-state.json`). The board test `TestLoad_DotMhgoIgnored` proves the config
loader ignores `.mhgo/`.

**Why now:** the worktree task already corrected its own docs and `cli.go`/`config.go`
to the delegate-to-`internal/config` wording; the same correction was never applied to
board, and the proposal asks for a tree-wide sweep so the docs stop lying about shipped
behaviour before more modules are built on top. Exploration during this discussion also
found that `internal/muxpoc` — a fully wired, dispatched `mhgo muxpoc` module — is
entirely undocumented and that `docs/overview.md` still calls worktree "coming
next"/"(sketch)".

## Scope

**In:**

- **A — config-layer corrections** (the originally-flagged items):
  - `docs/modules/board.md` §Configuration: drop the `.mhgo/board.yaml` three-layer
    model and the "Target redesign (not yet implemented)" note; **slim** the section to
    board's own keys/defaults and **delegate** the resolution model + env grammar to
    `docs/shared-libs/config.md` (mirroring `docs/modules/worktree.md`).
  - `internal/board/cli.go` and `internal/board/config.go` doc-comments: replace the
    layered-`.mhgo/` wording with the worktree-mirrored "delegates to `internal/config`;
    the module never names config-file layout" phrasing.
  - `docs/shared-libs/config.md`: remove the stale "Status: target design… Milestone 2
    lifts it here" block and the residual forward-looking phrasings ("redesigns it…",
    "board's existing behaviour, preserved", "Not supported in v1").
- **B — tree-wide stale-docs sweep** (fix **all** staleness in-place):
  - `docs/overview.md` (intro, Structure, Module-dispatch switch, Modules list, Other
    docs, `internal/state` reference).
  - `docs/roadmap.md` (note the muxpoc POC exists; cross-reference it from milestones
    5–7).
  - `docs/modules/mux.md` (keep "design, nothing implemented" for `internal/mux` but
    cross-reference the working muxpoc POC).
  - `docs/shared-libs/state.md`, `docs/shared-libs/README.md`, `docs/benchmarks.md`,
    `internal/config/config.go`, `internal/muxpoc/cli.go` (borderline-minor staleness —
    all in scope per "fix ALL staleness").
- **B-new — write `docs/modules/muxpoc.md`**: a full module doc (board.md/worktree.md
  quality) for the shipped `internal/muxpoc` POC, wired into overview.md & roadmap.md.

**Out:**

- **Workstream C — golang-skills conformance sweep** (package doc comments for the 7
  packages that lack them, godoc on every exported symbol, table-driven/`got;want` test
  restructuring, gofmt). **Explicitly removed from this task** and kept entirely
  separate; it remains recorded in `proposal-docs-stale-sweep.md`'s "golang-skills
  conformance sweep" section as a follow-up. Note the boundary: fixing *factually-stale*
  `.go` doc-comments (items A2, A3, B7, B8 below) IS in scope — that is staleness (B),
  not conformance.
- **gofmt / line endings.** Every `.go` file shows `gofmt -l` "dirty", but it is 100%
  CRLF (`git config core.autocrlf = true`; the repo stores LF). The repo content is
  already gofmt-clean. **Do not touch line endings.** When editing `.go` doc-comments,
  rely on autocrlf and verify the committed diff is content-only.
- The worktree module's own docs (`worktree.md`, `worktree/cli.go`, `worktree/config.go`)
  — already corrected in the worktree task; they are the **reference pattern**, not a
  target.
- `docs/vendor/psmux_scripting.md` (upstream psmux reference, not our design),
  `docs/psmux-tui-behavior.md` and `docs/modules/mux-exploration.md` (empirical /
  exploration records — reviewed, no module-currency staleness).
- **Any code or behaviour change.** This task edits Markdown and Go doc-comments only.
  No production logic, no tests rewritten. (No finding required a behaviour change; if
  the implementer discovers one, file it as a separate task rather than fixing it here.)
- No `mhgo board` task entry is created to track C: the `mhgo` board module is fully
  functional but **not in use** — mhgo is developed using Mill's tracker.

## Decisions

### split-conformance-out
- Decision: workstream C (golang-skills conformance) is removed from this task and tracked
  separately (it stays in the proposal).
- Rationale: it is a large, mechanically-different sweep (7 packages' package docs + every
  exported symbol + ~25 test files) with no dependency on the docs work; bundling it into
  this task's squash-merge would make an unreviewable diff.
- Rejected: keeping it in-task (scoped to production packages, or full incl. muxpoc).

### fix-all-staleness
- Decision: fix **every** stale doc in-place, including borderline-minor wording; only
  file a separate task for findings that need code/behaviour changes (none found).
- Rationale: the proposal asks for a tree-wide sweep and the user wants all staleness gone.
- Rejected: fixing only the headline config items and deferring the rest.

### gofmt-already-clean
- Decision: treat gofmt as already satisfied; never modify line endings.
- Rationale: the only "dirtiness" is CRLF on checkout; the repo stores LF via autocrlf, so
  committed content is gofmt-clean. "Fixing" CRLF would fight git and add noise.
- Rejected: running `gofmt -w` (would rewrite every file to LF in the working tree).

### slim-board-config-section
- Decision: board.md §Configuration is **slimmed** — keep board's own keys/defaults table,
  delegate the resolution model + env grammar to `shared-libs/config.md`.
- Rationale: mirrors worktree.md and the proposal principle "a module never names
  config-file layout in its own doc"; avoids two copies of the config grammar drifting.
- Rejected: keeping board.md self-contained and only correcting it to two-layer in place.

### muxpoc-coexist-and-document
- Decision: treat `internal/muxpoc` as a full, shipped module on par with board/worktree;
  **write `docs/modules/muxpoc.md`** and wire it into overview.md & roadmap.md. Use the
  **coexist** framing: muxpoc is a working POC (proves the risky parts of mux milestones
  6–7 — subprocess/reviewer panes + daemon crash-recovery — ahead of the clean module);
  `mux.md` stays the forward design for the still-unbuilt `internal/mux` and gains a
  cross-reference to muxpoc.
- Rationale: muxpoc is dispatched in `cmd/mhgo/main.go` (`mhgo muxpoc`), substantial, and
  undocumented; the user confirmed it should be documented like the others.
- Rejected: (a) "supersede" — rewriting mux.md/roadmap to "implemented as muxpoc"; (b)
  correcting existing docs to merely acknowledge muxpoc without a module doc.

## Technical context

**Reference pattern to copy (already-correct):**
- `docs/modules/worktree.md` — the corrected module-doc shape (Status block, delegates
  config to `internal/config`, no config-layout in-doc).
- `internal/worktree/cli.go` lines 1–11 and `internal/worktree/config.go` lines 1–6 —
  the corrected doc-comment wording: *"resolves configuration cwd-authoritatively via
  internal/config; the module never reads config files or knows their layout itself."*
- `docs/shared-libs/config.md` (below its stale Status block) — already describes the
  correct two-layer model, env grammar, `.env` precedence. board.md should point here.

**The actual config model** (`internal/config/config.go`): `Load(baseDir, module,
defaults)` checks `<baseDir>/_mhgo/` exists (else "not initialized…"), loads `<baseDir>/.env`,
starts from `defaults`, overlays `_mhgo/<module>.yaml` (per-key), then expands `$env:NAME`
(required; unset ⇒ hard error) and `$env:NAME ? fallback` (optional) across all values; OS
env wins over `.env`. board's `LoadConfig` then resolves a relative `path` against `baseDir`.
Two layers only — **no `.mhgo/` config layer**.

**Per-finding inventory** (line numbers are approximate — verify before editing):

*A — config-layer fixes:*
- **A1** `docs/modules/board.md` ~223–272: delete the "Target redesign (not yet
  implemented)" blockquote (225–230); replace the "Layered model — three sources"
  (236–245, incl. `<cwd>/.mhgo/board.yaml`) with a two-layer statement + a pointer to
  `../shared-libs/config.md` for resolution/env grammar. Keep the "Keys and defaults"
  table (path `../_board`, home `Home.md`, sidebar `_Sidebar.md`, proposal_prefix
  `proposal-` — matches `DefaultConfig()`). The "Environment variable expansion" and
  "Path resolution" subsections (257–272) should be folded into the config.md pointer
  (they currently omit the `? fallback` optional form). Also fix the **cli.go subsection**
  (~151–158): "configuration is loaded from layered YAML files and merged with defaults"
  → "the module's `_mhgo/board.yaml` merged with built-in defaults"; keep the `_mhgo/`
  requirement, the "not initialized here; run \"mhgo init\"" error, and the `--board-path`
  bypass paragraph.
- **A2** `internal/board/cli.go` lines 7–11 and 32–35: drop `<cwd>/.mhgo/board.yaml` and
  "layered"; mirror `worktree/cli.go` wording (cwd-authoritative via `internal/config`;
  module doesn't name layout). `--board-path` bypass note stays.
- **A3** `internal/board/config.go` file comment lines 1–5: "layered configuration system…
  YAML files organized in layers" → mirror `worktree/config.go` lines 1–6 ("configuration
  for the board module… LoadConfig delegates entirely to `internal/config`; the module
  never reads config files or knows their layout itself").
- **A4** `docs/shared-libs/config.md`: delete the Status block (6–9); change forward-looking
  phrasings to present tense — line ~8 "redesigns it to the model below", ~50 "*(This is
  board's existing behaviour, preserved.)*", ~67 "Not supported in v1:". The rest of the
  file is already accurate.

*B — sweep:*
- **B1** `docs/overview.md`: (a) intro lines 7–8 — worktree is implemented, muxpoc shipped
  as a POC, only `internal/mux` is still design; (b) §Structure 36–49 — the tree shows only
  board and claims "everything else is `package board`"; update to the real packages
  `internal/{board,worktree,muxpoc,config,git,lock,output}` + `cmd/mhgo`; (c) §Module
  dispatch switch 58–66 — shows only `board`/`init` cases; add `muxpoc` and `worktree`
  (match `cmd/mhgo/main.go`); (d) §Modules 78–83 — worktree "Sketch" → implemented, add a
  **muxpoc** bullet (shipped POC), keep mux as design; (e) line 90 — lists `internal/state`
  as existing shared infra; qualify as "(planned)"; (f) §Other docs 101–103 — worktree.md
  "(sketch)" → implemented, add `modules/muxpoc.md`, keep mux.md as design.
- **B2** `docs/roadmap.md`: add a note that a working **muxpoc POC** exists, proving the
  risky parts of milestones 6 (subprocess/reviewer panes) and 7 (daemon crash-recovery)
  ahead of the clean `internal/mux`; cross-reference `modules/muxpoc.md` from milestones
  5–7. Do **not** mark mux milestones Done (the clean module is unbuilt).
- **B3** `docs/modules/mux.md`: keep Status "design, nothing implemented yet" for
  `internal/mux`; add a cross-reference noting `internal/muxpoc` is a working POC of the
  daemon/pane-recovery/reviewer-pane model (link `muxpoc.md`).
- **B4** `docs/shared-libs/state.md` line ~35: "Exact home is decided when milestone 2/3
  lands" — milestone 2 shipped (config/git/lock) without extracting `AtomicWrite`/
  `PathGuard`; change to "milestone 3". Optionally strengthen the note with the concrete
  evidence that `internal/muxpoc` already reaches into `internal/board.AtomicWrite` (a
  cross-module reach that wants a real home).
- **B5** `docs/shared-libs/README.md` line ~21: lists `state.md`/`internal/state` as a
  library; qualify it as planned (consistent with overview.md's "(planned)").
- **B6** `docs/benchmarks.md` lines ~33, 66, 102: "deep merge from YAML layers" / "YAML
  layers" → "defaults + the module's `_mhgo/board.yaml`" (two-layer, not multi-layer).
  Leave the "Pre-config baseline (historic reference)" section untouched (labelled historic).
- **B7** `internal/config/config.go` line ~42 doc: "from defaults and layered configuration
  files" → "from defaults and the module's `_mhgo/<module>.yaml` file" (single file).
- **B8** `internal/muxpoc/cli.go` line ~42: "`daemon [not yet implemented in this batch]`"
  is stale — `daemon.go` implements `cmdDaemon` (a foreground poller with a crash-loop
  guard). Fix the comment to describe the real daemon subcommand.

**`docs/modules/muxpoc.md` (B-new) — what it must cover.** Model the structure on
`worktree.md`/`board.md`. **Read the whole `internal/muxpoc` package before writing** — the
package has good file-level/godoc comments to draw on; do not rely solely on this summary.
Cover:
- *What it is:* a proof-of-concept psmux session orchestrator for driving `claude` TUIs
  across panes; `mhgo muxpoc <subcommand>`; one psmux server per repo, socket+session name
  = `muxpoc-<sanitised filepath.Base(cwd)>` (`socketName`).
- *Subcommands* (`cli.go`): `up` (cold-start a new session, or cold-recover an existing
  one whose server died), `review` (add a reviewer pane via `split-window`), `attach` (pop
  the session into a maximized Windows Terminal on Windows / interactive `psmux attach`
  elsewhere), `status` (state-exists + server-running + live panes + saved pane metadata),
  `down` (stop the server and delete state — marks an **intentional** shutdown so `up`
  won't recover), `daemon` (foreground poller; on a dead session calls `coldRecover`, up to
  `maxRecoveries=3` within a `60s` window — crash-loop guard).
- *State model* (`state.go`): `.mhgo/muxpoc-state.json` (the gitignored **runtime-state**
  dir — explicitly NOT the removed config layer; cross-ref `state.md`/`config.md`); fields
  `session`, `socket`, `stripped_env`, `panes[]` where a `Pane` = `{id, session_id, kind:
  main|review}`. Atomic write (`board.AtomicWrite`) under an exclusive `internal/lock`
  file (`.mhgo/muxpoc-state.lock`); reads under a shared lock; corrupt state ⇒ warn + treat
  as no session.
- *Recovery model* (`up.go`/`daemon.go`): state survives a server crash; psmux assigns fresh
  pane ids across a restart, so recover re-launches `claude --resume <session-id>` per saved
  pane and re-tiles. `down` deletes state to distinguish intentional shutdown from a crash.
- *Layout* (`cmd.go`): vertical column; bottom (active) pane gets ~`activePaneShare=55%` via
  a hand-built window-layout string with a tmux-compatible checksum (`buildColumnLayout` /
  `layoutChecksum`) — presets like `even-vertical` can't express "bottom dominant".
- *Env sanitization* (`state.go`): `sanitizeEnv` strips `CLAUDECODE` and `CLAUDE_CODE_*` from
  the server's environment so child claude processes don't inherit them; the stripped keys
  are recorded in state.
- *Config:* flag-driven (`psmux`/`pwsh`/`claude` paths, `launch`/`resume` templates,
  `width`/`height`, `interval`) — muxpoc does **not** use `internal/config` or an `_mhgo/`
  yaml, unlike board/worktree. Note this difference.
- *Dependencies:* external `psmux.exe`, `claude`, `pwsh`, Windows Terminal (attach);
  internal `output` (JSON), `lock` (state lock), and `board.AtomicWrite` (a cross-module
  reach worth flagging — ties to `state.md`'s "AtomicWrite needs a home").
- *Windowless spawning:* `spawn_windows.go` (detached, `CREATE_NO_WINDOW`, own process
  group) vs `spawn_other.go` (`Setsid`).
- *Relationship to planned `internal/mux`:* coexist — muxpoc is the POC, mux.md is the
  forward design.
- *Tests:* `cli_test.go`, `cmd_test.go`, `state_test.go` (unit; pure parsers like
  `parsePaneList`/`buildColumnLayout`/`socketName` are unit-tested) and
  `muxpoc_smoke_test.go` (smoke).

## Constraints

- **Docs + doc-comments only** — no behaviour change; `go build`/`go vet`/`go test ./...`
  must stay green after the edits (board's `TestLoad_DotMhgoIgnored` is the guardrail that
  the `.mhgo/` config layer really is gone).
- **Never modify line endings** (see `gofmt-already-clean`). `.go` edits are content-only;
  rely on `core.autocrlf` so the committed diff stays LF and minimal.
- **No new config grammar in module docs** — board.md (and any module doc) points to
  `shared-libs/config.md` rather than re-documenting resolution/env rules.
- `.mhgo/` references that are legitimate **runtime-state** usages must be preserved, not
  "fixed": `internal/board/init.go` (gitignore managed block contains `.mhgo/`),
  `internal/muxpoc/state.go` (`.mhgo/muxpoc-state.json`), and the explicit "now-removed
  config layer" mentions in `state.md`/`config.md`.

## Testing

This is a documentation task; there are **no TDD candidates** and no new unit tests. The
plan should verify, not test-drive:

- **Compilation/regression:** `go build ./...`, `go vet ./...`, `go test ./...` all pass
  after the doc-comment edits (doc-comment-only changes must not break anything; board
  config tests in particular stay green).
- **Markdown link integrity:** every intra-doc link resolves — especially the new
  `docs/modules/muxpoc.md`, its inbound links from `overview.md` and `roadmap.md`, and the
  `mux.md`↔`muxpoc.md` cross-references. Do a link-resolution pass over `docs/`.
- **Staleness guard (grep):** after the sweep,
  `grep -rn "\.mhgo/board\.yaml\|three-layer\|Target redesign\|not yet implemented\|layered YAML" docs/ internal/`
  returns only legitimate runtime-state-dir references and the explicit "now-removed config
  layer" mentions — no surviving config-layer description.
- **Checklist verification:** confirm each enumerated finding A1–A4, B1–B8, and the new
  muxpoc.md are addressed; confirm board.md/board cli.go/board config.go now read like the
  worktree equivalents.

## Q&A log

- **Q:** How to handle workstream C (golang-skills conformance)? **A:** Remove it entirely
  from this task — keep it completely separate (stays in the proposal as a follow-up).
- **Q:** Fix policy for the tree-wide sweep? **A:** Fix **all** staleness in-place,
  including borderline-minor wording; file a separate task only for code/behaviour-change
  findings (none found).
- **Q:** gofmt/CRLF handling? **A:** Treat gofmt as already satisfied (repo stores LF via
  autocrlf); do not touch line endings.
- **Q:** board.md §Configuration rewrite style? **A:** Slim it and delegate the resolution
  model + env grammar to `shared-libs/config.md` (mirror worktree.md).
- **Q:** Is `internal/muxpoc` in scope? **A:** Yes — it is a full-fledged module on par with
  the others and must be documented.
- **Q:** Frame muxpoc vs the planned `internal/mux`? **A:** Coexist — muxpoc is the shipped
  POC; mux.md stays the forward design with a cross-reference; write a new
  `docs/modules/muxpoc.md` on par with board.md/worktree.md.
- **Q:** Create an `mhgo board` task to track C? **A:** No — the mhgo board module is fully
  functional but not in use; mhgo is developed with Mill's tracker.
- **Q:** mux.md "design, nothing implemented yet" — stale given muxpoc? **A:** No for the
  planned `internal/mux` (genuinely unbuilt); muxpoc is a separate POC. Keep the mux.md
  status, add a muxpoc cross-reference.
