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
  generic behavioural guidance is *not* loomyard-specific — see [Ecosystem & naming](#ecosystem--naming).
- **Explicit commands by default; `--help --json` as self-healing fallback.** A skill that needs a
  specific `lyx` command writes it **explicitly** (zero discovery latency). Only if that command
  *fails* (the CLI drifted — a flag renamed) does the LLM fall back to
  `lyx <module> <cmd> --help --json` and self-heal. `ly-workflow` carries only the general mechanism
  + module map, not a per-command catalogue.

## Ecosystem & naming

LoomYard is an *ecosystem of several plugins*, not one — so the names split three ways, mirroring
millhouse's `millhouse` / `mill` / `millpy` exactly:

| Name | Kind | millhouse mirror | Notes |
|---|---|---|---|
| **LoomYard** | the repo / ecosystem | `millhouse` | the whole |
| **`lyx`** | the executable | `millpy` | `ly` + `x` — **the `x` *is* "executable"**; reserved for the Go binary |
| **`ly`** | the core plugin | `mill` | the operator skill layer (`ly-*`) — the clean root |
| **`raddle`** | standalone plugin | `codeguide` | nav-doc generator; `loom` is a diff-scoped consumer (`git diff <start-SHA>..HEAD`); raddle knows nothing about lyx |
| **`golang` / `csharp`** | language plugins | same | per-language build / test / comments; LY uses `golang-*` |
| *(generic behavioural)* | plugin, name open | *(mill:\*)* | conversation / code-quality / testing / linting / markdown / cli |

**Why `ly` for the plugin (not `lyx`, not `weaver`):** `lyx` is reserved for the executable (the `x`
*is* "executable"), so the plugin cannot take it. `ly`/`lyx` is *exactly* the `mill`/`millpy` split —
the plugin takes the clean root, the impl takes a suffix. `ly` being LoomYard's short form is no more
a conflation than `mill` being millhouse's root: sibling plugins (`raddle`, `csharp`) carry their own
domain names and never contest `ly`-ness. Short is deliberate — it reads clean as a prefix
(`ly-workflow`), and a longer weaving name (`weaver-*`) loses exactly the brevity that made `ly` right.

**Open decision — generic-behavioural placement.** testing / linting / markdown / code-quality /
conversation / cli are *not* loomyard-specific; naming them `ly-testing` would overstate scope. Two
options: (1) a **separate generic plugin** (neutral name — `craft` / `dev` / `core`?), scope-honest
and reusable *(recommended)*; or (2) **fold into `ly`** (as mill folds them into `mill:*`), simpler
but the name overstates the loomyard tie. Nuance: testing/linting have a *language* dimension (generic
principles + `golang-testing` specifics); conversation/markdown/code-quality are purely generic.

## The LoomYard skill set — what we will build

The concrete target, by plugin. Everything not listed here is a `lyx` verb, a CLAUDE.md rule, or
dropped (see the [fate table](#fate-of-every-mill-skill)).

| Plugin | Skill | Purpose |
|---|---|---|
| **`ly`** (core) | `ly-workflow` | The map: module catalogue + common flows + mental model + `--help --json` fallback |
| **`ly`** | `ly-triage` | Board intake: GH issues / JSON reports → judge vs board → fold / new / skip → `lyx board` |
| **`ly`** | `ly-git-resolve` | Merge / rebase / cherry-pick conflict resolution (skill now; prompt template later for `--auto`) |
| **`ly`** | `ly-selfreport` *(optional)* | Reflect on a session, file bugs via `lyx selfreport` |
| **generic** *(name pending)* | `conversation` | Response style |
| **generic** | `code-quality` | Clean-code guidance |
| **generic** | `testing` | Testing principles (+ `golang-testing` for Go specifics) |
| **generic** | `linting` | Style rules (+ language specifics) |
| **generic** | `markdown` | Markdown formatting for generated files |
| **generic** | `cli` | Shell-command guidance |
| **`golang`** | `golang-build` / `-test` / `-comments` | Go build / test / comment conventions (LY is Go) |
| **`raddle`** | `raddle-generate` / `-update` / `-maintain` | Nav-docs for any repo; `loom` drives it diff-scoped |

**~10 skills total** (a couple optional), none a CLI wrapper or orchestration. The rest of the
"manual" is the `lyx` binary + its self-documenting CLI + `ly-workflow`'s map. The review-interpretation
discipline (from mill's `mill-receiving-review`) lives in **burler's prompt**, not a skill.

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

### Folds into a prompt template (lyx-spawned judgment, not a skill)

| mill skill | Folds to |
|---|---|
| mill:mill-receiving-review | **burler's prompt template** (`review-prompt-template.md`). burler interprets its own A-review and acts on it in the B-step (and reconciles cluster findings in A) — that review-interpretation discipline was genuinely valuable in mill and is **preserved**, not discarded. It lives in burler's prompt because burler is lyx-spawned / autonomous (rule 4, the *prompt-only* case; contrast `ly-git-resolve`, which is skill *and* prompt). |

### Discard (low value or obsolete)

| mill skill | Why |
|---|---|
| mill:mill-groom | ~never used standalone; its fold-value lives in `ly-triage`. |
| mill:mill-wiki-push | Board is one-shot & daemonless — no wiki repo to push. |
| mill:mill-skills-from-scripts | LY has few, non-script-backed skills. |
| mill:mill-skills-index | Same; a skills index is trivial if ever wanted. |
| mill:git-log | Work-journal-from-commits; low value, git history suffices. |
| mill:git-pr | Push-to-main is OK in LY → a PR is the exception; use `gh` ad-hoc. |

## Tally

44 mill skills →

- **~4 loomyard-specific** `ly-*`: `ly-workflow`, `ly-triage`, `ly-git-resolve`, `ly-selfreport` (optional)
- **~6 generic behavioural** (placement pending): conversation, code-quality, testing, linting, markdown, cli
- **4 merged** · **22 → CLI** · **2 → CLAUDE.md** · **1 → burler prompt** · **6 discarded**
- Language plugins (`golang-*`) and the `raddle` plugin sit alongside, unchanged / standalone.

**~10 real skills, none a CLI wrapper or orchestration.** The `lyx` binary + a self-documenting CLI +
`ly-workflow`'s map is the rest of the "manual".

## Open decisions

- **Generic-behavioural placement** — separate generic plugin (and its name) vs. fold into `ly`.
- **`ly-selfreport` / `ly-autofix`** — keep as thin skills, or lean entirely on `lyx selfreport` +
  loom `--auto`.
- **raddle-native generator** — timing of a lyx-native replacement for the millhouse-codeguide
  stopgap (the plugin boundary — standalone, diff-driven by loom — is the same either way).
