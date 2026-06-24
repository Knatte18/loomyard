# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C C:\Code\loomyard\wts\ly-git-clone rm <file>`.

### From discussion.md

# Discussion: ly-git-clone hub-creator (host, weft, board)

```yaml
task: ly-git-clone hub-creator (host, weft, board)
slug: ly-git-clone
status: discussing
parent: main
```

## Problem

Today lyx assumes its overlay artifacts (`_lyx/` config + task-state, `_codeguide/`, the
board) live committed inside the host repo. The **weft repo** model (see
`wiki/proposal-weft-repo.md`) removes that assumption: a fresh lyx Hub is three separate
git repos — the **host** (warp, stays pristine), the **weft** (overlay companion carrying
`_lyx/`/`_codeguide`), and the **board** (the task store lyx reads every task from) —
arranged into one Hub container.

Nothing bootstraps that Hub today. `lyx` has subcommands that operate on an *already
existing* hub (`weft`, `worktree`, `board`, `init`), but there is no command to create the
Hub from scratch by cloning the three repos into the correct geometry. This task adds it.

**Why now:** the weft engine + producers landed (roadmap milestone 5), so the geometry the
Hub must satisfy (`internal/paths`) is stable and proven. The hub-creator is roadmap
milestone 6, explicitly "ready now; needs only the done weft engine." Without it there is
no first step to stand up a real weft-backed hub for any host repo.

## Scope

**In:**

- A new **`lyx git-clone`** subcommand (Go), signature
  `lyx git-clone <host-url> <weft-url> [board-url]`.
- Creates a Hub container `<cwd>/<name>-HUB/` (where `<name>` is the host repo's basename),
  then clones three repos into it as **Prime worktrees only**:
  - host → `<name>-HUB/<name>/`
  - weft → `<name>-HUB/<name>-weft/`
  - board → `<name>-HUB/_board/`
- Board URL resolution: use `[board-url]` if given; otherwise **derive the weft repo's
  GitHub wiki** by rewriting the weft URL `…<repo>.git` → `…<repo>.wiki.git`.
- Strict pre-flight + failure handling: abort if the Hub dir already exists; on **any**
  clone failure (host, weft, **or board**) tear the Hub down and tell the user to retry.
- Integration tests against local fixture remotes.
- **Durable-doc corrections** (the docs are stale; the landed decisions are canonical):
  - `docs/roadmap.md` milestone 6 (line ~90): rewrite from "`ly-*` **skill** … wiring
    host↔weft junctions … neighbors in an existing hub" to: a Go **`lyx git-clone`
    subcommand** that creates a **fresh** Hub and clones host + weft + board, with **no**
    junction wiring (activation is a separate step).
  - `docs/roadmap.md` "Explicitly out of scope" bullet (line ~191): clarify that the
    deterministic weft→wiki URL rewrite is **in** scope; only *heuristic* inference stays
    out.
  - `docs/overview.md` weft-overlay model: correct the **board location** — the Artifacts
    table currently lists `_board/` under "Weft worktree", and the Topology omits it.
    Change the table row to **Hub** and add `_board/` as a Hub child in the topology
    diagram. Board has never been meant to live in the weft; it lives in the Hub.

**Out:**

- **No junction wiring.** No `_lyx` / `_codeguide` junctions, no `.git/info/exclude`
  entries, no `files.watcherExclude` seeding. The command clones repos and nothing else.
- **No lyx activation.** No `lyx init`, no `_lyx/` scaffolding, no config writes, no
  `board.yaml`, no hub manifest. **Only lyx itself writes lyx configs**; the hub it
  produces is dormant until lyx is activated in the Prime by a *separate* step/task.
- **No extra worktrees.** Only the three Prime worktrees are cloned. Additional host+weft
  worktree pairs are added later via `lyx worktree add`.
- **No skill, no plugin.** This is not a `/ly-*` Claude Code skill and creates no
  `plugins/ly/` directory. Claude Code plugin packaging is roadmap milestone 19, deferred.
- **No board placement modes.** The earlier `--board weft-wiki|host-wiki|standalone` enum
  is dropped; board always lands at `_board`, source is weft-wiki-by-default or an explicit
  `[board-url]`.
- **No board-repo creation from scratch.** Creating/initializing a board repo that does not
  yet exist stays roadmap milestone 16. This command *clones an existing* board.

## Decisions

### deliverable-is-a-go-command-not-a-skill

- Decision: Implement as a deterministic `lyx git-clone` Go subcommand. No LLM/skill in the
  loop.
- Rationale: The operation is pure mechanical git — parse 3 URLs, derive a name, compute a
  path, abort-if-exists, three `git clone`s, report. Zero judgment or natural-language
  work. A Go command is deterministic, has no LLM cost/latency/surprise, and is properly
  testable (integration tests against fixture remotes) — which a markdown skill cannot be.
- Rejected: (a) **Skill only** — the original "skill first, Go port later" plan; rejected
  because there is nothing to prototype (the weft flow is already built and known) and a
  skill can't be tested. (b) **Both** (Go command + thin skill wrapper) — gold-plating;
  the Claude-Code entry point belongs to milestone 19, not here.

### why-skill-was-never-needed-distribution

- Decision: Assume `lyx` is on `PATH`. Do not address distribution/packaging in this task.
- Rationale: millhouse's `/git-clone` is a *skill* only because mill has no binary to
  invoke before setup. `lyx` is a standalone Go binary: once installed
  (`go install ./cmd/lyx` → `%USERPROFILE%\go\bin`, or a `lyx.cmd` shim in
  `C:\Code\tools\bin`), `lyx git-clone …` runs from any folder with no hub and no plugin.
  The bootstrap chicken-and-egg that forces a skill does not exist here.
- Rejected: Bundling the binary in a plugin and reaching it via `${CLAUDE_PLUGIN_ROOT}` —
  that is roadmap milestone 19 ("Claude Code plugin packaging … once the binary and module
  architecture are proven"), explicitly deferred.

### hub-and-prime-naming

- Decision: Hub dir = `<name>-HUB` (uppercase `HUB` suffix). Host Prime = `<name>`. Weft
  Prime = `<name>-weft`. Board = `_board`. `<name>` is the **host** repo's basename
  (last URL path segment, `.git` stripped).
- Rationale: Uppercase `-HUB` makes it maximally visible in a file tree that the container
  is a hub-variant of the repo named `<name>`, distinct from a plain clone. The `<name>`
  (host Prime) and `<name>-weft` children are exactly what `internal/paths` geometry expects
  (`PrimeName` / `WeftRepoRoot`). `_board` is **not** a geometry-resolved path — there is no
  `_board` accessor in `internal/paths`; it is a **top-level Hub convention** that
  activation's `board.yaml` `path` later points at. This command merely places it there.
- Rejected: lowercase `<name>-hub`; camel `<name>Hub` (proposal's `LyxTestHub`) — both less
  visually loud than the operator wanted.

### only-prime-worktrees-cloned

- Decision: Clone only the Prime worktree of host and weft (a plain `git clone` of each
  remote's default branch). The weft remote may already be populated — that is fine; clone
  it as-is, no empty-repo synthesis, no branch fabrication.
- Rationale: Q7 — extra worktrees are added later via `lyx worktree add` (the paired
  host+weft spawn the worktree module already implements). A plain clone fetches all remote
  branches into `refs/remotes/origin/*` while checking out the default branch, so the weft
  repo's full branch structure is available for later worktree adds without special
  mirroring.
- Rejected: Mirroring/checking-out every branch at clone time; creating an initial commit on
  an empty weft remote — both out of scope and unnecessary.

### board-is-essential-strict-abort

- Decision: The board is **mandatory**. On any clone failure — host, weft, or board — abort
  and remove the partially-created Hub, instructing the user to fix and re-run. Likewise
  abort if `<name>-HUB` already exists.
- Rationale: lyx/loomyard reads **all tasks** from the board; without it the hub is
  non-functional. A hub missing its board is worse than no hub (a half-built trap), so board
  failure must tear everything down, same as host/weft. Strict abort keeps the outcome
  binary: a complete working hub, or nothing.
- Rejected: "Warn and continue" / "board is secondary" — false; the board is load-bearing.
  Idempotent resume (skip already-cloned repos) — adds half-state complexity for a command
  run rarely.
- **Teardown-failure (nested failure):** the teardown is `os.RemoveAll(<hub>)`. On Windows a
  still-running `git clone` child or an AV/indexer lock can make the removal itself fail. If
  teardown fails, do **not** mask it: report the residual Hub path explicitly and exit
  non-zero (`output.Err`), telling the user to remove `<hub>` manually before retrying.
  Never swallow the removal error and report success.

### board-url-derivation-default-weft-wiki

- Decision: When `[board-url]` is omitted, derive the board from the **weft** repo's GitHub
  wiki. Derivation rule (defined for every URL form): **strip a single trailing `.git` from
  the weft URL if present, then append `.wiki.git`** — so both `…/<repo>.git` and
  `…/<repo>` yield `…/<repo>.wiki.git`. An explicit `[board-url]` overrides this and is used
  verbatim.
- Rationale: Operator's product requirement — "default board git repo = weft's wiki-repo."
  The derivation is **deterministic string rewriting**, not the "heuristic inference of
  home-file content shape / board-URL derivation" the roadmap's out-of-scope list guards
  against. Amend that roadmap bullet to say deterministic wiki-URL derivation is in scope.
- Rejected: (b) plain optional arg with no derivation / skip board when omitted —
  contradicts the required default and the board-is-essential rule. (c) board-url required —
  needlessly forces the operator to spell out the obvious default every time.
- **Precondition (document in help text + error):** the board repo must already **exist and
  be initialized** before running the command. A GitHub wiki repo (`…wiki.git`) only exists
  after its first page is created in the GitHub UI; against a brand-new weft repo the
  derived wiki will 404 and — per the strict-abort rule — the whole command aborts. The
  operator must create/initialize the board first.

### board-location-is-hub-top-level

- Decision: The board repo is cloned to `<hub>/_board/` (top-level Hub child, sibling of the
  host and weft Primes). It is **never** placed inside the weft worktree.
- Rationale: Operator decision (Gap A) — the board has never been meant to live in the weft;
  it lives in the Hub. A top-level `_board` is also cleaner for a no-activation hub-creator:
  it avoids nesting a second git repo inside the weft worktree (which the weft repo would
  then need a `.git/info/exclude` entry to ignore — and exclude wiring is activation, out of
  scope here). The Hub root is not a git repo, so a sibling `_board` needs no exclusion.
- Rejected: `<prime>-weft/_board/` (inside the weft worktree) — what `docs/overview.md`
  currently says, but it is **stale/wrong**; this task corrects it (see Scope → durable-doc
  corrections). The proposal's §Model diagram already shows `_board/` as a Hub child.

### slug-is-historical-deliverable-is-lyx-subcommand

- Decision: The task slug/title `ly-git-clone` is historical (it matches the original
  "skill-first" plan, `ly-*` namespace). The actual deliverable is the **`lyx git-clone`
  subcommand** — a `lyx` binary command, not a `/ly-*` Claude Code skill.
- Rationale: Avoids confusion with the deferred milestone-19 Claude Code plugin/skill work.
  Recorded here so the plan writer is not misled by the slug into producing a skill.

## Technical context

What the plan needs to know about the codebase:

- **New module:** add `internal/gitclone/` (package `gitclone`) holding the command logic,
  plus a `case "git-clone"` in `cmd/lyx/main.go`'s subcommand switch (alongside `init`,
  `board`, `config`, `update`, `ide`, `muxpoc`, `worktree`, `weft`). Follow the existing
  module shape: a `RunCLI(out io.Writer, args []string) int` entry that emits JSON via
  `internal/output` (`output.Ok` / `output.Err`) and returns an exit code.
- **Git plumbing:** use `internal/git`'s `RunGit(args []string, cwd string) (stdout,
  stderr string, exitCode int, err error)` for every git call — never raw `exec`. Pattern
  examples: `internal/board/git.go`, `internal/worktree/weft.go`. `git clone <url> <dir>`
  lets us name the target dir explicitly, so the cloned dir names (`<name>`, `<name>-weft`,
  `_board`) are independent of the remote repo names.
- **Geometry it must produce (verified against `internal/paths`):**
  `Resolve(cwd)` sets `Hub = filepath.Dir(WorktreeRoot)` and `PrimeName =
  filepath.Base(Prime)`. Cloning the host Prime to `<name>-HUB/<name>/` therefore yields,
  from inside it, `Hub = <name>-HUB`, `PrimeName = <name>`, and
  `WeftRepoRoot() = <Hub>/<PrimeName>-weft = <name>-HUB/<name>-weft` — exactly where the
  weft Prime is cloned. The Hub container is **not** a git repo (`Hub` is the *parent* of
  the worktree root), so create it with `os.MkdirAll`, never `git init`.
- **Path invariant (CONSTRAINTS.md):** all cwd/worktree-root resolution goes through
  `internal/paths` (`paths.Getwd`, `paths.Resolve`); raw `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and
  `cmd/lyx/main.go`, enforced by `internal/paths/enforcement_test.go`. The Hub root path is
  `filepath.Join(cwd, name+"-HUB")` derived from `paths.Getwd()` — this is plain path
  construction, not geometry resolution, so it does not need a `Layout` (there is no repo at
  the hub root yet to resolve).
- **Name derivation:** host repo basename = last path segment of `<host-url>` with a single
  trailing `.git` stripped. Handle `https://…/<name>.git`, `git@host:user/<name>.git`, and
  no-`.git` forms (cf. millhouse `git-clone` skill §2.1).
- **No use of weft/worktree internals:** the paired-spawn junction helpers
  (`seedLyxJunction`, `seedGitExclude`, `createWeftWorktree`) in `internal/worktree/weft.go`
  are unexported and tied to `lyx worktree add`; this command does not call them (no
  junctions, only Prime clones).
- **fslink rule (recorded for the future, not used here):** when junctions *are* created
  (activation, a later task), they must go through `internal/fslink.CreateDirLink` — never
  OS-native `mklink`/`New-Item -Junction`. This task creates no links.
- **Reference:** `docs/roadmap.md` milestones 6/16/19, `docs/overview.md` naming section,
  the millhouse `git-clone` skill (the analog: container scaffolding, name derivation,
  abort-if-exists, partial-failure cleanup). The original design write-up
  `wiki/proposal-weft-repo.md` §6-7 lives in the **daemon-owned wiki repo and is NOT present
  in this worktree** — treat it as background only. **Every decision needed to write the
  plan is captured in this file**; the proposal is not required reading.

## Constraints

- **Path Invariant (CONSTRAINTS.md):** see Technical context — all worktree/hub geometry via
  `internal/paths`; raw `os.Getwd` / `git rev-parse --show-toplevel` forbidden outside the
  two allowed files; build-time enforced.
- **Documentation lifecycle (CONSTRAINTS.md / overview.md):** mechanical per-module docs are
  deleted when a module lands; the package header comment carries the durable rationale.
  This command's design rationale lives in the `internal/gitclone` package header, not a new
  `docs/modules/*.md`.
- **Naming (overview.md):** binary `lyx`; never name a skill `lyx-*`/`loom-*`. Not directly
  exercised here (no skill), but the subcommand belongs under the `lyx` binary's namespaced
  subcommand tree.
- **Output contract:** JSON on stdout via `internal/output`, exit 0 success / 1 error,
  matching every other `lyx` subcommand.

## Testing

The command is deterministic, so it gets **real automated integration tests** (Go) — not a
manual-only checklist. TDD candidates and scenarios for `internal/gitclone`:

- **Name + board-URL derivation (pure, unit-level, TDD):** host basename extraction across
  URL forms (`https`, `scp-like git@`, no-`.git`); weft→wiki rewrite
  (`…<repo>.git` → `…<repo>.wiki.git`); explicit `[board-url]` passthrough.
- **Happy path (integration, against local fixture remotes):** create throwaway bare repos
  on disk for host, weft, and board; run the command in a temp cwd; assert the Hub layout
  (`<name>-HUB/<name>/`, `<name>-HUB/<name>-weft/`, `<name>-HUB/_board/`), that each is a
  valid clone on the expected default branch, that the Hub root is **not** a git repo, and
  that **no** `_lyx`/`_codeguide`/junction/config artifacts were created (dormant hub).
- **Geometry round-trip:** from the cloned host Prime, `paths.Resolve` yields
  `Hub=<name>-HUB`, `PrimeName=<name>`, and `WeftRepoRoot()` pointing at the cloned weft
  Prime.
- **Abort-if-exists:** pre-create `<name>-HUB/`; assert the command refuses and changes
  nothing.
- **Strict-abort on clone failure (each of host / weft / board):** point one URL at a
  non-existent remote; assert non-zero exit, a clear error, **and** that the partial Hub was
  removed (no half-state left behind). The board case doubles as the "wiki not initialized"
  scenario.
- **Teardown-failure (nested failure):** simulate `os.RemoveAll` failing during cleanup
  (e.g. a locked/undeletable file under the partial Hub); assert the command exits non-zero
  and surfaces the residual Hub path rather than reporting success.
- **Board-URL derivation forms:** weft URL with and without a trailing `.git` both yield
  `…/<repo>.wiki.git`.
- **Default-derivation integration:** omit `[board-url]`; assert it clones the derived
  weft-wiki fixture.

Use only local on-disk fixture remotes — no network. Per the conversation rules, scratch
fixtures go under `.scratch/`, never a system temp dir.

## Q&A log

- **Q:** Skill or Go command? **A:** Go command — it's pure mechanical git, deterministic
  and testable; nothing for an LLM to do.
- **Q:** How is `lyx` reached from an arbitrary folder; isn't it a Claude plugin? **A:**
  `lyx` is a standalone binary on PATH (`go install` / `tools/bin` shim); plugin packaging
  is the deferred milestone 19, not this task.
- **Q:** Subcommand name? **A:** `lyx git-clone` (explicit — it's not self-evidently a git
  clone; the command runs rarely, so verbosity is fine). Rejected `lyx clone`,
  `lyx hub create`.
- **Q:** Hub dir name? **A:** `<name>-HUB` uppercase, for maximum visibility that it's a hub.
- **Q:** Does it wire junctions / seed `_lyx` / write config? **A:** No to all — clone only;
  lyx stays dormant; only lyx activation (a separate step) wires junctions and writes
  configs. Overrides the wiki brief's "wire the overlay junctions" phrasing.
- **Q:** If junctions were ever created here, how? **A:** Via `internal/fslink` only, never
  OS-native — but this task creates none.
- **Q:** Weft setup extent? **A:** Plain `git clone` of the weft Prime; remote may be
  non-empty; extra worktrees come later via `lyx worktree add`.
- **Q:** Board placement choice? **A:** Dropped the enum; board always lands at `_board`.
- **Q:** Board URL when omitted? **A:** Derive weft's wiki (`.git`→`.wiki.git`); amend the
  roadmap's out-of-scope bullet (deterministic derivation ≠ heuristic inference).
- **Q:** Board clone failure? **A:** Strict abort + tear down the Hub — the board is
  essential (lyx reads all tasks from it); a hub without a board is non-functional.
- **Q:** Where does the skill/plugin live? **A:** Moot — no skill, no `plugins/ly/`; that's
  milestone 19.
- **Q:** (review gap A) Board location — `<hub>/_board` vs `overview.md`'s "weft worktree"?
  **A:** Hub, always — board is **never** in the weft and never was meant to be. `overview.md`
  is stale and gets corrected by this task (artifacts table + topology).
- **Q:** (review gap B) Is the Go-command/no-junction/fresh-hub stance a deviation from the
  roadmap? **A:** No — the *roadmap* (milestone 6, line 90) is stale and gets updated to match
  the landed decisions; our decisions are canonical.


### From _mill/plan/00-overview.md


```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
slug: "ly-git-clone"
approved: true
started: "20260624-162514"
parent: "main"
root: ""
verify: null
```

### From _mill/plan/01-gitclone-command.md


```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
batch: "gitclone-command"
number: 1
cards: 6
verify: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/gitclone/gitclone.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitclone/gitclone_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitclone/clone.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitclone/cli.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitclone/clone_integration_test.go`
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-durable-docs.md


```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
batch: "durable-docs"
number: 2
cards: 2
verify: null
depends-on: []
```



- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `docs/overview.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
4. Run `git -C C:\Code\loomyard\wts\ly-git-clone add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C C:\Code\loomyard\wts\ly-git-clone rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C C:\Code\loomyard\wts\ly-git-clone rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success:

{"status":"success"}

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\ly-git-clone` for any git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\ly-git-clone`.
