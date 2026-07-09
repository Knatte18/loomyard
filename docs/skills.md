# LoomYard skills — plan & fate of the mill skill set

> **Status: Design / plan — not built.** The `ly-*` skill layer is authored once the `lyx` spine
> (`shuttle → burler → perch → loom`) is far enough along to consume it. This doc is the reference
> for *which* skills LoomYard needs and why — derived by walking millhouse's `mill` skill set and
> assigning each a fate.

## Why LoomYard needs *far fewer* skills than mill

In mill, **the skills *were* the orchestrator.** Orchestration was LLM-driven, so every lifecycle
action (spawn, plan, go, merge, cleanup, status) had to be a skill. In lyx, orchestration is **Go**
(`loom`). So the entire `mill-*` lifecycle family does not get *ported* — it is *replaced by a
binary*. What survives as a skill is only (a) behavioural/style guidance a binary cannot encode, and
(b) a thin layer of judgment-heavy entry points. Net: **~44 mill skills → ~10 real LY skills**, none
of them CLI wrappers or orchestration.

## Decision rules

Applied in order to each mill skill:

1. **One `lyx` verb / deterministic?** → **`lyx` verb, no skill.** The command is self-documenting;
   the caller writes it explicitly.
2. **Low-value even in mill?** → **Discard.** (e.g. `mill-groom` was ~never used standalone.)
3. **Always-on rule** (git hygiene, invariants)? → **CLAUDE.md**, not a skill — CLAUDE.md loads every
   session, a skill only on demand, so guarantees belong there.
4. **Substantial judgment, not CLI-discoverable?** → **skill**, and the surface depends on the
   call-site:
   - **operator-invoked** (a human in a Claude session) → a **skill**.
   - **lyx-spawned / autonomous** → a **prompt template** (stencil-filled, spawned via shuttle),
     because Go cannot invoke a skill.
   - **both** → author the instructions **once**, expose them as *both* a skill and a prompt template.

### Two cross-cutting conventions

- **Name reflects scope.** `ly-*` = loomyard/lyx-specific. Language plugins (`golang-*`, `csharp-*`)
  = language-specific, reusable, unchanged. `raddle-*` = the standalone nav-doc generator. Truly
  generic behavioural guidance is *not* loomyard-specific — see [plugin taxonomy](#plugin-taxonomy).
- **Explicit commands by default; `--help --json` as self-healing fallback.** A skill that needs a
  specific `lyx` command writes it **explicitly** (zero discovery latency). Only if that command
  *fails* (the CLI drifted — a flag renamed) does the LLM fall back to
  `lyx <module> <cmd> --help --json` and self-heal. `ly-workflow` carries only the general mechanism
  + module map, not a per-command catalogue.

## Plugin taxonomy

| Layer | Scope | Naming | Contents |
|---|---|---|---|
| **`ly`** | loomyard/lyx-specific | `ly-*` | `ly-workflow`, `ly-triage`, `ly-git-resolve`, `ly-selfreport` (optional) |
| **Language** | language-specific, reusable | `golang-*`, `csharp-*`, `python-*` | build / test / comments per language (unchanged from millhouse; LY uses `golang-*`) |
| **Generic behavioural** | project-agnostic | *(placement open — see below)* | conversation, code-quality, testing, linting, markdown, cli |
| **raddle** | standalone tool | `raddle-*` | nav-doc generation; **`loom` is a consumer** that drives it diff-scoped (`git diff <start-SHA>..HEAD`). One-way coupling: raddle knows nothing about lyx. |

**Open decision — generic behavioural placement.** testing / linting / markdown / code-quality /
conversation / cli are *not* loomyard-specific; naming them `ly-testing` overstates scope. Two options:
1. **A separate generic plugin** (neutral name — `craft` / `dev` / `core`?): keeps `ly-*` tight and
   honest, reusable across projects. *(Recommended — scope-honest.)*
2. **Fold into `ly`** (as mill folds them into `mill:*`): simpler, more self-contained, but the name
   overstates loomyard-tie.

Nuance: testing/linting have a *language* dimension (generic principles + `golang-testing` specifics);
conversation/markdown/code-quality are purely generic.

## `ly-workflow` — the map, not the manual

`ly-workflow` is the one orientation skill: a **general wayfinding catalogue** of how to use the
loomyard repo — which modules exist, the common flows (`lyx warp add` to spawn, `lyx loom run` to
drive, `lyx board upsert` to add a task), the mental model (host + weft + board), and the
`--help --json` fallback rule. It is an *overview*, never a per-flag catalogue and never a replacement
for the explicit command an individual skill writes.

## `ly-git-resolve` — the one genuinely-new git skill

Mechanical git is `lyx` (warp/weft) or the agent's normal host git. The single exception needing
judgment is **merge/rebase/cherry-pick conflict resolution**. Named `ly-git-resolve` (git explicit;
"resolve" is git's verb for conflicts; *not* `merge-resolve` since conflicts also arise from rebase /
cherry-pick / stash). It is the rule-4-"both" case: a **skill** now (operator resolving a manual
merge), and later a **prompt template** (`git-conflict-resolve-template.md`, stencil + shuttle) for
autonomous `--auto` runs — same instructions behind both. loom's default on conflict is to **escalate
to the operator** (stuck/pause), not silently auto-resolve, which can corrupt intent.

## Fate of every mill skill

### Survives as an `ly-*` skill

| mill skill | Becomes | Why |
|---|---|---|
| mill:workflow | **`ly-workflow`** | Entry/catalogue: mental model + module map + `--help --json` fallback. |
| mill:mill-triage-to-tasks | **`ly-triage`** | The one judgment-heavy board skill: intake → compare-vs-board → fold/new/skip → `lyx board`. |
| mill:mill-self-report | **`ly-selfreport`** (optional) | Reflection judgment (detect bugs from a session); files via `lyx selfreport`. |
| *(new)* | **`ly-git-resolve`** | Conflict resolution — judgment, not mechanical, not CLI. mill had no standalone equivalent. |

### Generic behavioural — kept, placement pending

| mill skill | Becomes | Why |
|---|---|---|
| mill:conversation | Keep (generic) | Response-style guidance; not CLI-discoverable. |
| mill:code-quality | Keep (generic) | Clean-code guidance. |
| mill:testing | Keep (generic) | Testing principles (+ `golang-testing` for Go specifics). |
| mill:linting | Keep (generic) | Style rules (+ language specifics). |
| mill:markdown | Keep (generic) | Markdown formatting for generated files. |
| mill:cli | Keep (generic) | Shell-command guidance (could fold into `ly-workflow`). |

### Folds into another skill (Merge)

| mill skill | Merges into | Why |
|---|---|---|
| mill:mill-ghissues-to-tasks | `ly-triage` | GitHub-issue source adapter. |
| mill:mill-report-to-tasks | `ly-triage` | JSON-report source adapter. |
| mill:mill-fold | `ly-triage` | Fold-in *is* ly-triage's core judgment; execution via `lyx board merge/upsert`. |
| mill:mill-autofix | `loom` (autonomous) + `ly-triage` | "loop loom over open bugs" = loom `--auto` + ly-triage; a thin driver at most. |

### Absorbed by a `lyx` verb (→ CLI, no skill)

| mill skill | Replaced by |
|---|---|
| mill:mill-setup | `lyx warp clone` / `lyx init` |
| mill:git-clone | `lyx warp clone` |
| mill:mill-spawn | `lyx warp add` (+ loom) |
| mill:mill-claim | `lyx warp add` / loom |
| mill:mill-start | loom Discussion phase (a producer prompt, not a skill) |
| mill:mill-plan | loom Plan phase |
| mill:mill-go | loom Builder phase |
| mill:mill-finalize | loom Finalize |
| mill:mill-merge | loom Finalize + `lyx warp cleanup` (conflict → `ly-git-resolve`) |
| mill:mill-merge-in | loom / warp merge logic (conflict → `ly-git-resolve`) |
| mill:mill-cleanup | `lyx warp cleanup` |
| mill:mill-abandon | `lyx loom abandon` |
| mill:mill-pause | `lyx loom pause` |
| mill:mill-resume | loom resume + weft-sync + session-sync |
| mill:mill-status | `lyx loom status` |
| mill:mill-inspect | `lyx board` / `lyx loom status` |
| mill:mill-add | `lyx board upsert` |
| mill:mill-color | `lyx ide` |
| mill:mill-terminal | `lyx ide` |
| mill:mill-vscode | `lyx ide` |
| mill:millhouse-issue | `lyx selfreport` |
| mill:git-issue | `lyx selfreport` / `gh` |

### Folds into CLAUDE.md (always-on rules, not a skill)

| mill skill | Folds to |
|---|---|
| mill:git-commit | CLAUDE.md hygiene (stage individually, never `--no-verify`/force-push) + burler prompts own commit-per-fix |
| mill:git-workflow | CLAUDE.md — always-on git rules belong in the per-session instructions, not an on-demand skill |

### Discard (low value or obsolete)

| mill skill | Why |
|---|---|
| mill:mill-groom | ~never used standalone; its fold-value lives in `ly-triage`. |
| mill:mill-wiki-push | Board is one-shot & daemonless — no wiki repo to push. |
| mill:mill-skills-from-scripts | LY has few, non-script-backed skills. |
| mill:mill-skills-index | Same; a skills index is trivial if ever wanted. |
| mill:mill-receiving-review | `perch` automates evaluation of review findings. |
| mill:git-log | Work-journal-from-commits; low value, git history suffices. |
| mill:git-pr | Push-to-main is OK in LY → a PR is the exception; use `gh` ad-hoc. |

## Tally

44 mill skills →

- **~4 loomyard-specific** `ly-*`: `ly-workflow`, `ly-triage`, `ly-git-resolve`, `ly-selfreport` (optional)
- **~6 generic behavioural** (placement pending): conversation, code-quality, testing, linting, markdown, cli
- **4 merged** into the above · **22 → CLI** · **2 → CLAUDE.md** · **7 discarded**
- Language plugins (`golang-*`) and the `raddle` plugin sit alongside, unchanged / standalone.

**~10 real skills, none a CLI wrapper or orchestration.** The `lyx` binary + a self-documenting CLI +
`ly-workflow`'s map is the rest of the "manual".

## Open decisions

- **Generic-behavioural placement** — separate generic plugin (and its name) vs. fold into `ly`.
- **`ly-selfreport` / `ly-autofix`** — keep as thin skills, or lean entirely on `lyx selfreport` +
  loom `--auto`.
- **raddle-native generator** — timing of a lyx-native replacement for the millhouse-codeguide
  stopgap (the plugin boundary — standalone, diff-driven by loom — is the same either way).
