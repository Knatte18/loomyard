# LoomYard skills — plan & fate of the mill skill set

> **Status: Design / plan — not built.** Authored once the `lyx` spine (`shuttle → burler → perch → loom`) can consume it.

In mill the skills *were* the orchestrator (LLM-driven). In lyx, orchestration is Go (`loom`), so the `mill-*` lifecycle family becomes `lyx` verbs, not skills. **~44 mill skills → ~10 real LY skills.**

## Decision rules

| Condition | Fate |
|---|---|
| One `lyx` verb / deterministic | **`lyx` verb** (no skill) |
| Low value even in mill | **Discard** |
| Git mechanics at commit time | **loom commit path** — Go-deterministic (weft) + builder/fixer prompt template (host code). *Not* CLAUDE.md: always-on text gets forgotten |
| Session-coloring fact, unenforceable at a point-of-use | **CLAUDE.md** (loads every session; no mill skill lands here) |
| Judgment, operator-invoked | **skill** |
| Judgment, lyx-spawned / autonomous | **prompt template** (stencil + shuttle) — Go can't call a skill |
| Judgment in both | one instruction set → **skill + prompt** |

- **Name reflects scope:** `ly-*` loomyard · `craft` generic authoring · `golang-*`/`csharp-*` language · `raddle-*` nav-docs.
- **Explicit `lyx` commands by default;** `--help --json` is the self-healing fallback when the CLI drifts.

## Ecosystem & naming

Mirrors millhouse's `millhouse` / `mill` / `millpy` split — the plugin takes the clean root, the executable the suffix.

| Name | Kind | millhouse mirror |
|---|---|---|
| **LoomYard** | repo / ecosystem | `millhouse` |
| **`lyx`** | executable (`ly` + `x`, **x = executable**) | `millpy` |
| **`ly`** | core plugin (`ly-*`) | `mill` |
| **`craft`** | generic authoring plugin (language-agnostic, reusable) | *(mill:testing/linting/…)* |
| **`golang` / `csharp` / `python`** | language plugins (install per language) | same |
| **`raddle`** | standalone nav-doc plugin; `loom` is a diff-scoped consumer | `codeguide` |

## The LoomYard skill set

One table per plugin, one row per skill. Installed set for LoomYard: **`ly + craft + golang + raddle`**.

### `ly` — core plugin (loomyard-specific)

| Skill | Purpose |
|---|---|
| `ly-workflow` | Map: module catalogue + common flows + mental model + `--help --json` fallback |
| `ly-triage` | **Pure judgment, zero board relation.** Go supplies both inputs (fetched items + a board snapshot); the skill emits a decisions artifact (per item: new / fold-into-slug / skip). Go does *all* board I/O (upsert/merge) and the fetch/close. The skill never reads or writes the board |
| `ly-git-resolve` | Merge/rebase/cherry-pick conflict resolution — skill now; prompt template (`git-conflict-resolve-template.md`) later for `--auto` |
| `ly-selfreport` *(opt.)* | Reflect on a session → file bugs via `lyx selfreport` |

#### `ly-triage` contract — whole board in, delta out

File contract (same shape as burler): **Go writes the input, the skill writes the output, Go executes.** The skill is source-agnostic — `ref` is an opaque identity string Go minted (gh-issue number, report id, …); the skill echoes it back and Go maps `ref` → real action (close issue N, etc.).

**In — `triage-in.json` (Go writes, skill reads).** The board goes in *whole* — the skill needs it to judge fold-vs-new (find overlap) and avoid slug collision:

```json
{
  "items": [ { "ref": "gh#42", "title": "…", "body": "…" } ],
  "board": [ { "slug": "reviewers-cache", "title": "…", "brief": "…",
               "status": null, "deferred": false } ],
  "hint": { "group_target": 3 }
}
```

**Out — `triage-out.json` (skill writes, Go reads).** A *delta* (decisions), never a rewritten board — the skill can only add / fold / skip, so it physically cannot drop or mangle an existing task:

```json
{
  "new_tasks": [ { "slug": "…", "title": "…", "brief": "…", "item_refs": ["gh#42","gh#7"] } ],
  "fold_ins":  [ { "target_slug": "reviewers-cache", "item_refs": ["gh#13"] } ],
  "skips":     [ { "ref": "gh#99", "reason": "not actionable" } ]
}
```

**"Delta" is not a board operation** — it is the shape of the output artifact. Go is the translator; the board stays dumb CRUD (`internal/boardengine`):

| out entry | Go call (existing board primitive) |
|---|---|
| `new_tasks[]` | one `UpsertTasksBatch` (atomic, all-or-none) |
| `fold_ins[]` | per fold: `GetTask(slug)` → append to body → `UpsertTask` (bounded, additive) |
| `skips[]` | no board call — Go just closes/labels the source item |

**Go-enforced invariants (fail-loud):** (1) every input `ref` appears exactly once across `new_tasks`∪`fold_ins`∪`skips`; (2) slug matches `[a-z][a-z0-9-]*`, no board collision; (3) fold target exists *and* is unclaimed (`status==null && !deferred`) — re-checked at apply time against the snapshot race. The asymmetry mirrors burler: rich read-input, narrow validatable write.

### `craft` — generic authoring (language-agnostic)

| Skill | Writes | Purpose |
|---|---|---|
| `conversation` | prose → operator | response style |
| `markdown` | `.md` | markdown formatting |
| `code-quality` | code | clean-code guidance |
| `testing` | code | testing principles |
| `linting` | code | style rules |
| `cli` | shell | shell-command usage |

### `golang` — language plugin (LY is Go; `csharp`/`python` are siblings, per language)

| Skill | Purpose |
|---|---|
| `golang-build` | Go build / test commands |
| `golang-testing` | Go testing conventions |
| `golang-comments` | godoc / comment rules |

### `raddle` — standalone nav-doc plugin (`loom` drives it diff-scoped)

| Skill | Purpose |
|---|---|
| `raddle-generate` | Generate nav-docs for undocumented files |
| `raddle-update` | Update docs for changed files (loom: `git diff <start-SHA>..HEAD`) |
| `raddle-maintain` | Fix / repair existing docs |
| `raddle-setup` | Initialise / activate raddle in a repo |

The review-interpretation discipline (mill's `mill-receiving-review`) lives in the shared **review→fix prompt template**, not a skill — used by **both** consumers of that two-step pattern: `burler` (interprets its own A-review in B) and `hardener` (roadmap #23; shares the burler round discipline).

## Fate of every mill skill

### → an LY skill

| mill skill | Becomes |
|---|---|
| mill:workflow | `ly-workflow` |
| mill:mill-triage-to-tasks | `ly-triage` (pure judgment; Go supplies board snapshot + does all board writes) |
| mill:mill-self-report | `ly-selfreport` *(optional)* |
| mill:conversation | `craft` · `conversation` |
| mill:markdown | `craft` · `markdown` |
| mill:code-quality | `craft` · `code-quality` |
| mill:testing | `craft` · `testing` |
| mill:linting | `craft` · `linting` |
| mill:cli | `craft` · `cli` |

### Merge into an LY skill

| mill skill | Merges into |
|---|---|
| mill:mill-ghissues-to-tasks | **splits**: fetch + close + board writes → `lyx` verb / Go; analysis → `ly-triage` |
| mill:mill-report-to-tasks | `ly-triage` (source is already a JSON file — no fetch; Go still owns board writes) |
| mill:mill-fold | `ly-triage` (fold *judgment* only; Go executes the merge/upsert) |
| mill:mill-autofix | `loom --auto` + `ly-triage` (thin driver at most) |

### → a `lyx` verb (no skill)

| mill skill | Replaced by |
|---|---|
| mill:mill-setup | `lyx warp clone` / `lyx init` |
| mill:git-clone | `lyx warp clone` |
| mill:mill-spawn | `lyx warp add` (+ loom) |
| mill:mill-claim | `lyx warp add` / loom |
| mill:mill-start | loom Discussion phase (a producer prompt) |
| mill:mill-plan | loom Plan phase |
| mill:mill-go | loom Builder phase |
| mill:mill-finalize | loom Finalize |
| mill:mill-merge | loom Finalize + `lyx warp cleanup` (conflict → `ly-git-resolve`) |
| mill:mill-merge-in | loom / warp merge (conflict → `ly-git-resolve`) |
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

### → loom commit path (Go-deterministic + prompt template)

Nothing new to CLAUDE.md — always-on text is forgotten. The rules live where commits actually happen.

| mill skill | Why |
|---|---|
| mill:git-commit | Mechanics (stage-by-name, no `-A`, no force-push, no `--no-verify`, set upstream) enforced **in Go** where lyx commits weft; message format + commit-per-fix ride the **builder/fixer prompt template** for host code. Lint / raddle-sync / main-gate are loom's job, not per-commit |
| mill:git-workflow | Branch / PR / rebase / main-gate are **loom-enforced** (breaking them is clumsiness, not a rule to restate); message + staging discipline ride the builder/fixer prompt template |

### → burler's prompt template (lyx-spawned judgment)

| mill skill | Why |
|---|---|
| mill:mill-receiving-review | shared review→fix prompt discipline (rule-4 prompt-only); two consumers — `burler` (its own A-review in B) **and** `hardener` (same two-step). Value preserved, not discarded |

### Discard

| mill skill | Why |
|---|---|
| mill:mill-groom | ~never used standalone; fold-value → `ly-triage` |
| mill:mill-wiki-push | board is one-shot & daemonless — no wiki repo |
| mill:mill-skills-from-scripts | LY has few, non-script-backed skills |
| mill:mill-skills-index | trivial if ever wanted |
| mill:git-log | low value; git history suffices |
| mill:git-pr | push-to-main is OK → PR is the exception; use `gh` ad-hoc |

**Tally:** 9 → LY skill · 4 merged · 22 → `lyx` verb · 2 → loom commit path · 1 → burler prompt · 6 discarded = 44.

## Open decisions

- **`craft` name** — vs `authoring` / `core` (the language-agnostic authoring plugin).
- **`ly-selfreport` / `ly-autofix`** — thin skills, or lean on `lyx selfreport` + `loom --auto`.
- **`raddle`-native generator timing** — millhouse-codeguide is the stopgap; the plugin boundary is the same either way.
