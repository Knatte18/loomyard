# Discussion: Rename internal/ghissues → selfreport

```yaml
task: Rename internal/ghissues → selfreport
slug: rename-ghissues-to-selfreport
status: discussing
parent: main
```

## Problem

The `ghissues` module names the *mechanism* (GitHub issues) instead of the
*responsibility*. The module is lyx's **self-report channel** — the direct analog of
millhouse's `millhouse-issue` skill: "lyx has a bug → file it against lyx's own repo",
regardless of which host repo lyx is driving. The target is hardcoded
(`targetRepo = "Knatte18/loomyard"` in `ghissuesengine`), the only verb is `create`, and
every help string already says "file LoomYard bugs". So the module is, and is meant to
stay, a fixed self-report channel — `ghissues` undersells that and reads like a generic
GitHub-issues utility.

Renaming to `selfreport` names the responsibility, matches the `millhouse-issue`
precedent, and follows this repo's standing "name the responsibility, not the bucket"
sweeps (`config→configengine`, `paths→hubgeometry`). **Why now:** part of the active
naming-clarity sweep across lyx modules; this rename runs in parallel with
`rename-paths-to-hubgeometry`.

This is a **behaviour-preserving rename**, not a generalization. No flags, output shape,
target repo, label defaults, stdin handling, JSON envelope, or exit codes change. The
only observable change is the command verb itself: `lyx ghissues create` → `lyx
selfreport create`.

## Scope

**In:**

- Rename `internal/ghissuescli/` → `internal/selfreportcli/` (package `selfreportcli`,
  files `cli.go`, `cli_test.go`). Use `git mv` for the directory, then surgical edits.
- Rename `internal/ghissuesengine/` → `internal/selfreportengine/` (package
  `selfreportengine`). The implementation file is currently `ghissues.go`; rename it to
  `selfreport.go` to match the package and avoid a stale filename.
- Update the CLI command tree in `selfreportcli/cli.go`:
  - `Use: "ghissues"` → `"selfreport"`, so the verb becomes `lyx selfreport create`.
  - **Reframe** the parent `Short` and the `create` subcommand `Short`/`Long` to lead
    with the self-report responsibility (file a LoomYard bug/enhancement to lyx's own
    repo), while still naming `gh`/GitHub as the mechanism (the command genuinely files a
    GitHub issue, so the prose must stay mechanism-accurate per the CLI/Cobra Invariant).
  - Update all `Long` examples `lyx ghissues create ...` → `lyx selfreport create ...`.
  - Keep `targetRepo = "Knatte18/loomyard"` unchanged.
  - Rename internal identifiers/comments that say `ghissues`/`ghissuesengine` to the new
    package names.
- `cmd/lyx/main.go`: import path (`internal/ghissuescli` → `internal/selfreportcli`),
  the `selfreportcli.Command()` registration in `newRoot()`, and the `Available modules:
  ... ghissues.` line in the root `Long` (rename the trailing `ghissues` token to
  `selfreport`, per the CLI/Cobra Invariant "registered ⇒ in --help prose").
- `cmd/lyx/helptree_test.go`: the pinned `requiredModules` set (`ghissues` →
  `selfreport`) and the module's row (`name`/`module` = `ghissues` → `selfreport`; its
  `wantSubs` already contains `create`, unchanged).
- `cmd/lyx/jsonhelp_test.go`: the pinned module token in the names list, the two test
  functions `TestJSONHelp_GhissuesSchema` / `TestJSONHelp_GhissuesCreateLeaf` (rename to
  `...Selfreport...`), their `run([]string{"ghissues", ...})` argv, and all message/
  comment references.
- `internal/lyxtest/leaf_enforcement_test.go`: the two banned-import entries
  (`.../ghissuesengine`, `.../ghissuescli` → `.../selfreportengine`, `.../selfreportcli`)
  plus the header comments listing the engine/cli pairs.
- `internal/lyxtest/doc.go`: the comment listing `ghissuesengine/ghissuescli` →
  `selfreportengine/selfreportcli`.
- **Test identifiers and comments:** rename **all** `Ghissues`/`ghissues` occurrences in
  test function names, doc comments, and string literals to `Selfreport`/`selfreport`
  (no stale identifiers left behind).
- Docs:
  - `docs/overview.md`: the module-tree lines (`internal/ghissuescli/`,
    `internal/ghissuesengine/`) and the **ghissues** bullet in the module list
    (`lyx ghissues create` example → `selfreport`; rename the bullet heading).
  - `docs/roadmap.md`: the "✅ Done" ghissues milestone entry — update the
    `lyx ghissues create` example and the `internal/ghissuesengine` package reference;
    keep the historical milestone framing.
  - `tools/sandbox/test-scheme.md`: all `lyx ghissues create` → `lyx selfreport create`.
  - `docs/sandbox-howto.md` and `docs/sandbox-hub.md`: rename `lyx ghissues create` →
    `lyx selfreport create`, **and remove every `mill-ghissues-to-tasks` reference**
    (see Decision: drop-mill-pipeline-refs).

**Out:**

- Any behaviour change: no `--repo` flag, no host-repo / RepoA targeting, no change to
  the hardcoded loomyard target, label defaults (`bug`), stdin handling, or the JSON
  envelope.
- Any change to the engine's logic beyond the package/identifier rename.
- Renaming the millhouse `mill-ghissues-to-tasks` skill itself — it lives in a separate
  repo (millhouse) and is not lyx's to rename. This task only *removes* its mentions
  from Loomyard docs (see Decision below); it does not invent a renamed mill skill.
- `_mill/` internal files (status.md etc.) — not product code.

## Decisions

### module-name-selfreport

- Decision: Rename the module to `selfreport` (`selfreportcli` / `selfreportengine`).
- Rationale: Names the responsibility (lyx self-reporting to its own repo), matches the
  `millhouse-issue` precedent, and follows the repo's "name the responsibility, not the
  bucket" convention. `selfreport` over `bugreport`/`selfissue` because the channel files
  both bugs *and* enhancements (the `--label` flag overrides the default `bug`), so "bug"
  undersells it; "self" captures that the target is lyx's own repo. The
  `<module>cli`/`<module>engine` convention gives `selfreportcli` / `selfreportengine`.
- Rejected: `bugreport` (undersells enhancements), `selfissue` (still mechanism-leaning),
  keeping `ghissues` (the problem being fixed).

### reframe-help-prose

- Decision: Reframe the parent `Short` and the `create` `Short`/`Long` to lead with the
  self-report responsibility, while still naming `gh`/GitHub as the mechanism.
- Rationale: The CLI/Cobra Invariant requires help prose to match behaviour — the command
  genuinely files a GitHub issue via `gh`, so the mechanism must remain visible. But the
  module's *name* now advertises the responsibility, so the help text should lead with it
  too. Example targets (mill-plan may refine exact wording): parent `Short` ≈ "self-report
  a LoomYard bug or enhancement to lyx's own repo (via gh/GitHub issues)"; `create` `Short`
  ≈ "file a self-report issue on the LoomYard repository via gh". The `Long` keeps the gh
  prerequisite note and the three worked examples, with the verb updated to `selfreport`.
- Rejected: Keeping the verbatim mechanism-only wording ("file LoomYard bugs … as GitHub
  issues") — accurate but misses the chance to align help with the new responsibility
  framing the user explicitly chose.

### drop-mill-pipeline-refs

- Decision: **Remove** every `mill-ghissues-to-tasks` reference from Loomyard docs
  (`docs/sandbox-howto.md`, `docs/sandbox-hub.md`), rewording the surrounding prose so it
  no longer names that mill pipeline. Do **not** rename it to `mill-selfreport-to-tasks`.
- Rationale: `mill-ghissues-to-tasks` is a millhouse skill living in a separate repo;
  lyx has no authority to rename it and there is no reason to even mention it from
  Loomyard docs. The docs should describe what lyx does (file a self-report GitHub issue)
  and stop there — the downstream mill ingestion is out of Loomyard's concern.
- Rejected: Renaming to `mill-selfreport-to-tasks` (would invent a rename that does not
  exist on the mill side); leaving the references (the user explicitly wants them gone).
- Concrete edits:
  - `docs/sandbox-howto.md:17-18`: drop "which feed the `GitHub issue → mill-ghissues-to-tasks`
    pipeline" — reword to end at "via `lyx selfreport create`." (or "filed as GitHub issues.").
  - `docs/sandbox-howto.md:103`: drop the "with the mill pipeline (`/mill-ghissues-to-tasks`)"
    clause — reword so the sentence stands without naming the mill skill.
  - `docs/sandbox-hub.md:111-112`: drop "which feeds the `GitHub issue -> mill-ghissues-to-tasks`
    pipeline" — reword to end at "via `lyx selfreport create`."
  - mill-plan must read each of these passages in full and reword for grammar, not just
    delete the token, so the surrounding sentences remain coherent.

### git-mv-renames

- Decision: Perform directory/file renames with `git mv`, then make surgical edits to
  package clauses, import paths, identifiers, and comments. No full-file rewrites.
- Rationale: Preserves git history/blame across the rename and keeps the diff reviewable
  as a rename + small edits rather than delete+add. (Standing repo convention for
  renames.)
- Rejected: Delete-and-recreate (loses blame, noisier diff).

## Technical context

The module is two packages plus a handful of registration/test/doc touch-points. Exact
inventory (confirmed by exploration, 14 grep hits; `_mill/status.md` excluded as
non-product):

**Source (renamed dirs):**
- `internal/ghissuescli/cli.go` — cobra command tree. `Use: "ghissues"`, parent `Short`,
  named `createCmd` var (warp pattern: reads `Changed("body")` inside `RunE`), `--body`/
  `--label` flags, `RunCLI` seam (`return clihelp.Execute(Command(), out, args)`),
  `runCreate` handler delegating to `ghissuesengine.CreateIssue`. Imports
  `internal/ghissuesengine`, `internal/clihelp`, `internal/output`.
- `internal/ghissuescli/cli_test.go` — white-box tests in `package ghissuescli`; swaps
  the `ghissuesengine.RunGH` seam with a fake. Test funcs are `TestRunCreate_*` (no
  `Ghissues` token in those names, but the file header/comments reference `ghissues`).
- `internal/ghissuesengine/ghissues.go` → `selfreport.go` — domain kernel: `targetRepo`
  const, `RunGH`/`realRunGH` seam, `buildCreateArgs`, `CreateIssue`, `lastNonEmptyLine`.
  Imports `internal/proc`. No `cli`/cobra imports (engine purity holds — keep it that way).

**Registration / pinned guards:**
- `cmd/lyx/main.go` — import (`:25`), `Available modules:` Long line (`:77`, trailing
  `ghissues.`), `ghissuescli.Command()` in `newRoot()` (`:96`).
- `cmd/lyx/helptree_test.go` — `requiredModules` set (`:28`), the `ghissues` table row
  (`:89-90`).
- `cmd/lyx/jsonhelp_test.go` — names list (`:94`), `TestJSONHelp_GhissuesSchema` (`:145-164`),
  `TestJSONHelp_GhissuesCreateLeaf` (`:211-245`), all argv/comments referencing `ghissues`.
- `internal/lyxtest/leaf_enforcement_test.go` — banned-import entries (`:46-47`) + header
  comments (`:3`, `:21`).
- `internal/lyxtest/doc.go` — comment (`:11`).

**Docs:** `docs/overview.md` (`:175-176` tree, `:218-219` bullet), `docs/roadmap.md`
(`:198-202` Done entry), `docs/sandbox-howto.md` (`:17-18`, `:103`),
`docs/sandbox-hub.md` (`:87`, `:111-112`), `tools/sandbox/test-scheme.md`
(`:22`, `:63`, `:65`, `:67`, `:174`).

Note: `docs/sandbox-hub.md:87` and `tools/sandbox/test-scheme.md:22` reference
`lyx ghissues create` as the command — these are plain command-name updates (no mill
pipeline involved).

**Reuse / no new code:** This is purely renaming. No new helpers, no new tests, no
signature changes. `RunGH`/`RunCLI`/`CreateIssue` keep their exact signatures so callers
(only `cmd/lyx` and the package's own tests) need nothing beyond the import-path swap.

## Constraints

From `CONSTRAINTS.md` (hub root):

- **CLI / Cobra Invariant** — the load-bearing one here:
  - Module seam preserved: `Command() *cobra.Command` + `RunCLI` = `return
    clihelp.Execute(Command(), out, args)`. Unchanged by the rename.
  - **Registration**: rename must update all three of (1) import, (2)
    `root.AddCommand(selfreportcli.Command())`, (3) the root `Long` module-list token.
    Enforced by `cmd/lyx/registration_test.go` ("exists ⇒ registered") and
    `cmd/lyx/longlist_test.go` ("registered ⇒ in --help prose").
  - **Every command has a `Short`** — keep both the parent and `create` `Short` non-empty
    after rewording (enforced by `cmd/lyx/drift_test.go`).
  - **Help prose is review-checked against behaviour** — the reframed `Short`/`Long` must
    stay mechanism-accurate (still files a GitHub issue via gh). This is an explicit
    review obligation: the reviewer reads every reworded `Short`/`Long` and confirms it
    matches the (unchanged) behaviour.
  - **Help tree pinned by test** — update `helptree_test.go` and `jsonhelp_test.go` in
    the same commit.
  - **Package naming** — `<module>cli`/`<module>engine` convention: `selfreportcli` /
    `selfreportengine`. Engine must never import cli/cobra (already true; preserve).
- **lyxtest Leaf Invariant** — `internal/lyxtest` stays a leaf; we only edit its banned-
  import string list and comments (no new imports). `leaf_enforcement_test.go` keeps
  guarding it. Banned-import entries must exactly match the new import paths.
- **Path Invariant** — not touched (selfreport packages do not resolve cwd/geometry).
- **Documentation Lifecycle / Task completion (CLAUDE.md)** — docs are updated in the
  **same commit(s)** as the code: `docs/overview.md` module table, plus the sandbox/
  roadmap doc edits listed in Scope. Roadmap entry is a historical "Done" item, updated in
  place (not a new milestone).

Independence: this task is **independent of `rename-paths-to-hubgeometry`** — no code
dependency (selfreport packages don't import `internal/paths`; the paths importer-sweep
doesn't touch them). The only shared files (`docs/overview.md`,
`internal/lyxtest/leaf_enforcement_test.go`, `internal/lyxtest/doc.go`) are edited in
disjoint regions, so whichever merges second rebases cleanly or hits at most a trivial
textual nit. Safe to run in parallel.

## Testing

No new tests — the change is mechanical and behaviour-preserving; the existing suites are
the guardrail. The plan must ensure these all pass after the rename:

- `go build ./...` — catches any missed import path or identifier.
- `go test ./...` — full suite.
- `internal/selfreportcli` (renamed `cli_test.go`) — the white-box `TestRunCreate_*`
  tests still pass against the renamed `selfreportengine.RunGH` seam.
- `cmd/lyx` guards: `helptree_test.go`, `jsonhelp_test.go`
  (`TestJSONHelp_SelfreportSchema` / `TestJSONHelp_SelfreportCreateLeaf`),
  `registration_test.go`, `longlist_test.go`, `drift_test.go` — all pass with the new
  module name and reworded help.
- `internal/lyxtest/leaf_enforcement_test.go` — passes with the renamed banned-import
  entries (and would fail loudly if an entry's path didn't match a real package).

Verification beyond `go test`: grep the whole tree for `ghissues` (case-insensitive) after
the rename and confirm zero hits remain outside `_mill/` — every product occurrence
(code, test, doc) must be gone, and no `mill-ghissues-to-tasks` token survives.

## Q&A log

- **Q:** How to treat `mill-ghissues-to-tasks` references (a millhouse skill, not the lyx
  module)? **A:** Remove them entirely from Loomyard docs — the mill skill lives in a
  separate repo, won't be renamed, and there's no reason to mention it from Loomyard.
  Reword the surrounding prose; do not invent `mill-selfreport-to-tasks`.
- **Q:** Keep the mechanism-only help wording or reframe? **A:** Reframe `Short`/`Long`
  to lead with the self-report responsibility, still naming gh/GitHub as the mechanism
  (must stay behaviour-accurate per the CLI/Cobra Invariant).
- **Q:** Rename test identifiers/comments (`Ghissues`/`ghissues`) too? **A:** Yes — rename
  **all** of them to `Selfreport`/`selfreport`; leave no stale identifiers (incl. the
  `TestJSONHelp_Ghissues*` functions and file-header comments).
- **Q:** Rename mechanics? **A:** `git mv` the dirs/files (incl. `ghissues.go` →
  `selfreport.go`), then surgical edits — preserve blame, no full-file rewrites.
- **Q:** Relationship to `rename-paths-to-hubgeometry`? **A:** Independent; shared files
  edited in disjoint regions; safe to run in parallel.
