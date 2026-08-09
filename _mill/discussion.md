# Discussion: Rename the fabric host vocabulary to warp, and name the composite repo Fabric

```yaml
task: Rename the fabric host vocabulary to warp, and name the composite repo Fabric
slug: fabric-host-to-warp-rename
status: discussing
parent: main
```

## Problem

The two repos in a fabric pair are **warp** (the user's own repo) and **weft** (the lyx-managed sibling).
"Host" is the retired, pre-warp name for the warp side.
Slice 8 (`fabric-weft-visibility-cleanup`, commit `406624e7`) purged the retired vocabulary from prose and comments but deliberately left every **identifier** untouched, so the code still speaks the old language at every API seam:
`HostJunctions`, `HostJunctionsHere`, `HostLyxLink`/`HostLyxLinkHere`, the `HostJunction` struct, `WeftHostSlug`, `HostWorktree`, `DeriveHostName`, and a large unexported family (`hostLayout`, `hostBranch`, `hostPath`, `hostBare`, `hostDotLyx`, `detectHostPollution`, …).
Three Go files are named for it too.
Every task that touches fabric re-learns the mapping "host means warp" before it can read the code, and every plan that cites these constructors by name bakes the retired name in further.

Separately and folded into the same pass: warp and weft are **internal** vocabulary describing the two-repo mechanism.
From outside, fabric presents one illusioned repo — warp with junctions into weft inside it — and that composite has no name today.
This task gives it one: **Fabric** (capital F).
Both halves are vocabulary-precision work over the same files, done once rather than twice.

**Why now:** the blocking predecessor `dotlyx-scratch-hygiene` (028) has merged (squashed as `dcc2fd76`, "slice 9"), which was the hard sequencing gate.
The successor `fabric-warp-binding-in-weft` (034, slice 10) depends on this task and should not cite `Host*` identifiers that this task retires.

## Scope

**In:**

- Every fabric-sense `host` token in `internal/fabricengine/` and `internal/fabriccli/` — exported identifiers, unexported identifiers, locals, test-function names, doc comments, string literals, and the embedded `post-checkout.sh` shell hook.
- `internal/lyxtest`'s exported test-fixture seam: `CopyHostHub` → `CopyWarpHub`, the `HostFixture` type, `buildHostHub`/`hostHubTemplate`/`hostHubOnce`/`hostHubPath`/`hostHubBarePath`, and every caller across the 8 packages that reference them: `internal/lyxtest`, `internal/fabricengine`, `internal/fabriccli`, `internal/lyxcwd`, `internal/idecli`, `internal/buildercli`, `internal/webstercli`, and `cmd/lyx` (`tierpurity_test.go:50`, a comment-only mention).
  `internal/configcli` is **not** among them — it uses fabric fixtures but references neither symbol.
- The two other owner packages the tightened guard reaches: `internal/weftname` (`weftname.go:10`, "host-worktree slug") and `internal/boardengine` (`board.go:15`, `:17`, `:25`).
  Without these the tightening cannot compile-and-pass — see Decisions, "Enforcement guard is tightened".
- **All** of `tools/sandbox/*.go` — the scope is the glob, not a line list.
  Six files carry the name: `suite_test.go` (60 hits), `main_test.go` (56), `report_test.go` (41), `suite.go` (24), `main.go` (15), `report.go` (8);
  `resolve.go`, `resolve_test.go` and `pathresolve_guard_test.go` are clean.
  Four user-visible error strings, not two: `main.go:139`, `:141`, `report.go:57` ("hub host repo not found at %s"), `:59` ("stat host repo %s").
  `suite.go` independently declares `hostDirName` (`:27`–`:29`) and its own `hostRepoDir` (`:169`–`:380`).
- Fabric-sense `host` in the **non-owner packages** that carry residue — almost entirely test code, plus one production comment:
  `internal/loomengine` (35 hits, `TestPreflight_HostDirty`, "paired host+fabric worktree"), `internal/webstercli` (21, incl. the `newHostWeftPair`/`newHostWeftPairAt` helpers and the literal fixture repo names `"host"`/`"host-weft"`), `internal/buildercli` (24), `internal/configcli` (15, incl. production `configcli.go:269` "the host `_lyx` parent"), `internal/perchcli` (14), `internal/websterengine/audit_test.go` (4, incl. the fixture path literal `cd /hub/host`).
  Plus `internal/builderengine/gitquery_test.go:23` ("lyxtest's weft/host fixtures") — the rest of that package is machine-/verb-sense and stays.
  See Decisions, "Non-owner residue is folded in, not deferred", and Technical context, "Exhaustive repo-wide classification".
- Four file renames as `Moves:` pairs (see Technical context).
- User-visible CLI surface: `fabriccli` `Short`/`Long` strings, `post-checkout.sh`'s stderr output, and `tools/sandbox/main.go`'s two error strings.
- **All** documentation carrying fabric-sense "host": `docs/overview.md`, `docs/sandbox-hub.md`, `docs/sandbox-howto.md`, `docs/shared-libs/configengine.md`, `README.md`, `manifest/roadmap.md`, `manifest/designs/loom.md`, `manifest/designs/host-visibility.md`, the eight `tools/sandbox/SANDBOX-*-SUITE.md` agent prompt templates, and the five `.claude/agents/crucible-reviewer-{low,medium,high,max,xhigh}.md` files (each line 16, "host-repo").
- `CONSTRAINTS.md`'s Fabric Vocabulary Invariant — full rewrite (see Decisions) — plus the Cwd Resolution Invariant's bullet citing `HostLyxLink`/`HostJunctions`, and the Fabric Git Invariant's prose.
- `internal/lyxcwd/enforcement_test.go` — tighten the guard so fabric-sense `host` fails inside the owner dirs too.
- A new reusable tool `tools/wordswap/`, committed with tests.

**Out:**

- Any behaviour change. Every diff is an identifier, a filename, a comment, a help string, or prose.
- Machine-sense and verb-sense `host` anywhere: `conhost` (`internal/reedengine`, `internal/shell`), `localhost`, `Hostname`, `hostURL`/`redditHostPattern`/`redditHostReplace` (`internal/prowler`), `ghost`/`ghostFile`, `hosts`, `hosting`.
  These are not the warp repo and must not be touched.
  Concretely including `internal/configengine/config_test.go:283`–`:308`, whose `server:\n  host: localhost` YAML fixture is machine-sense test data despite matching a bare `host` grep.
- **The whole `crucible/` directory.** All five hits are machine-sense and must survive verbatim: `orchestrator-prompt.md:94` ("exhaust the host's RAM in minutes"), `review-prompt-template.md:38` ("a host process killed"), `:110` and `:119` ("exhaust the host's RAM"), `README.md:110` (same).
  These mean *this machine*, not the warp repo.
  Called out explicitly because a plain `host` grep surfaces them and the directory is otherwise adjacent to in-scope work — `.claude/agents/crucible-reviewer-*.md:16` **is** in scope, since "a host-repo commit … never a weft-repo operation" contrasts the two fabric sides.
  `crucible/` sits outside `internal/` and `cmd/`, so no guard covers it either way; this is review obligation only.
- The `host` **ban list itself**: `CONSTRAINTS.md` lines 160–161 and `enforcement_test.go`'s `hostPhrases` / `hostGeometryIdentifiers` values.
  These name the retired vocabulary in order to forbid it, so they keep the word (see Technical context — "The ban list is not a rename target").
- Renaming anything to "Merriam", or any other unrelated vocabulary work.
- `warpPath`, `warpSHA`, `WarpCommitted` and the rest of the already-correct `Warp*` family — untouched except where `hostPath` merges into `warpPath`.

## Decisions

### Rename mechanism — one generic case-preserving word swap, not an identifier table

- **Decision:** a single generic, case-preserving, whole-word `host` → `warp` substitution over the in-scope file set, applied by a script — not a hand-maintained old→new identifier table, and not per-call-site `Edit` calls.
  The substitution handles `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP` and every unambiguously-bounded embedded form in one pass: `hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`.
  All-lowercase compounds (`hostclean`, `hostlayout`, `hosthub`) are **not** swapped automatically — see "Ambiguous compounds are reported, not guessed" below.
- **Rationale:** the boundary-case survey (see Technical context) found the exclude list is **empty** after one comment reword, so there is nothing for a per-identifier table to protect against.
  A generic swap covers identifiers, comments, test names, string literals, shell variables, and markdown prose with one mechanism; a table covers only what someone remembered to list.
  `go build ./... && go test ./...` is the completeness proof.
- **Rejected:** `gofmt -r` per old→new pair — does not touch comments, string literals, or test-function names, so a second pass is needed anyway, and the table must be maintained.
  `gopls rename` per symbol — semantically correct but ~40 manual invocations, weak on locals in test files, and blind to prose.

### The tool is `tools/wordswap/`, general and committed

- **Decision:** build `tools/wordswap/` as a language-agnostic, case-preserving, whole-word token-substitution tool, committed with unit tests, modelled on the existing `tools/mdreflow/` and `tools/godocreflow/` house pattern.
  Interface: `go run ./tools/wordswap -from host -to warp [-dry-run] [-skip <regexp>] <path-or-glob>...`.
  It exits non-zero when it reported any ambiguous compound, so an unresolved ambiguity cannot pass unnoticed in a scripted run.
- **Rationale:** the user's rule is that one-off specialised migration scripts are not committed, but a tool general enough to be reused is.
  `mdreflow` and `godocreflow` are exactly this precedent: repo-wide codemod sweeps, committed, tested, with a `Last run:` note in the package doc.
  A vocabulary rename is a recurring shape in this repo (this is the third fabric-vocabulary pass), so the next one should reuse the tool rather than rewrite a throwaway.
  Language-agnostic matters concretely: the in-scope set includes Go, `post-checkout.sh`, markdown, and agent prompt templates.
- **Rejected:** `.scratch/` throwaway script — not reusable, and the diff-as-only-evidence argument is weaker than the two committed precedents.
  `tools/renamevocab/` with a baked-in `host→warp` table — barely simpler and not general.
  A Go-AST-based `tools/identswap/` — more precise on Go but blind to shell, markdown, and yaml, so a second tool would be needed regardless.

### Safety invariant — reversibility over recorded spans

- **Decision:** `wordswap` records the byte offset of every substitution it makes.
  Before writing any file, it reverts exactly those recorded spans and asserts the result is byte-identical to the input.
  A file failing the check is left untouched and reported, matching `mdreflow`'s "left untouched and reported" behaviour.
- **Rationale:** this proves the tool only substituted and never reworded, reordered, or dropped content.
  Crucially it works even though the target word already occurs in the input — `warp` has 576 pre-existing occurrences in `fabricengine` — which a naive round-trip or count-based check cannot handle.
- **Rejected:** count invariant (`count(new, out) == count(new, in) + count(old, in) - skipped`) — cheap, but blind to text moved or lost elsewhere in the file.
  No invariant at all — breaks the house pattern both existing tools follow.

### Ambiguous compounds are reported, not guessed — the LLM adjudicates

- **Decision:** `wordswap`'s token rule swaps automatically only where the boundary is unambiguous.
  A token boundary after `host` exists when the next character is an uppercase letter, a digit, an underscore, or a non-identifier character;
  the boundary before `host` is the same rule in reverse, or start-of-input.
  Everything of the form `host` + lowercase letters at a token start — `hostclean`, `hostlayout`, `hosthub`, and equally `hostname`, `localhost`, `conhost` — is classified **AMBIGUOUS**: not swapped, and printed with file and line number.
  The implementing agent resolves each reported occurrence by judgment.
- **How a verdict is recorded — otherwise the tool can never exit zero.**
  An adjudication has two possible outcomes, and both must be expressible:
  - *"This one is fabric-sense"* → edit the occurrence by hand (or rename the file), so it no longer appears in the next run's report.
  - *"Leave this one alone"* → re-run with the occurrence named in `-skip`.
  The run is complete when `wordswap` exits **zero** with an explicit `-skip` set, and that `-skip` set is the audit record of every deliberate keep — enumerated, reviewable, and reproducible.
  Without this, a correct "leave `hosting` alone" verdict has no representation and the exit code never clears.
  This supersedes the earlier framing that the fabric-package run takes no `-skip` at all: that held for the original narrow scope, but the widened scope contains verb-sense keeps (`internal/buildercli/poll_test.go:212`, "a live pane hosting an idle agent") that are neither reworded nor swapped.
- **Rationale:** `hostclean` and `hostname` are character-class-identical, so no mechanical rule can separate them — only a dictionary or a pre-enumerated list can, and both are exactly the "someone must have remembered it" failure a generic tool exists to avoid.
  Reporting rather than guessing keeps the tool correct *and* general: the next vocabulary rename discovers its own ambiguities instead of requiring the author to know them up front.
  This is the boundary between mechanical and judgment work that the whole task is organised around — script what is mechanical, let the LLM decide what is not, and make the tool surface the difference rather than hide it.
- **No occurrence count is pinned for any commit.** An earlier draft pinned five for commit (a);
  that was measured against the original narrow scope and is wrong for the widened one — `internal/buildercli/poll_test.go:212` ("hosting") alone breaks it, and the doc sweep in commits (c)/(d) adds more (`docs/overview.md:302`, `manifest/designs/loom.md:131` and `:198`, `manifest/designs/fabric-unified-view.md:88`, `docs/sandbox-hub.md`, `tools/sandbox/SANDBOX-REED-SUITE.md:225`).
  A pinned number would fire as a false tripwire on known-benign hits.
  **Treat the report as the work list, not as a checksum.**
- **Known fabric-sense ambiguities, for orientation only:** `internal/fabricengine/hostclean.go:1`, `internal/fabricengine/drift.go:3`, `internal/fabricengine/hostlayout.go:1`, `internal/fabricengine/hostjunction_test.go:1`, and `internal/lyxtest/lyxtest.go:128` (the `lyxtest-hosthub-*` temp-dir prefix) all resolve to `warp`.
  Listed so the implementer recognises them, not as a set to check the report against.
- **Rejected:** a strict rule plus a hand-edit of the five known occurrences — works this once, but depends on the survey being exhaustive and teaches the next user nothing.
  An `-also <literal,...>` flag for explicit extra matches — pushes the judgment back onto a pre-enumerated list, the thing being avoided.
  A permissive rule where `host` + lowercase always matches — would rewrite `hostname`, `localhost`, and `conhost`, destroying the tool for reuse.

### Exclude list is empty — reword the one verb-sense hit instead of skipping it

- **Decision:** `internal/fabricengine/coalesce.go:1` reads `// coalesce.go hosts the generic loop-until-clean coalescing primitive …`.
  Reword to `// coalesce.go holds the generic …` **before** running `wordswap`, rather than skipping it.
  `wordswap`'s `-skip <regexp>` flag is still built and unit-tested as a general capability, but **this rename runs with no `-skip` argument**.
- **Rationale:** verified by exhaustive grep — that line is the *only* standalone English `host`/`hosts`/`hosting`/`hosted` in either fabric package.
  Rewording it means the word "host" does not survive in the fabric packages in any sense, which makes the tightened enforcement guard trivially verifiable and removes all risk of the script skipping the wrong occurrence.
- **Rejected:** `-skip 'hosts the generic'` — leaves a retired word in the package for no benefit.
  Word-list exclusion (`-skip-words hosts,hosting,…`) — would wrongly skip the fabric-sense plural noun in a future run.
  Run and hand-revert — no declarative recipe, does not scale.

### `hostPath` merges into the existing `warpPath` — a duplicate, not a collision

- **Decision:** `hostPath` (47 hits) renames to `warpPath` (321 pre-existing hits) by plain global substitution, with no per-file judgment.
- **Rationale:** they are the same concept under two names, split historically by which half of `fabricengine` you are in.
  `hostPath` lives in the topology files (`status.go`, `prune.go`, `reconcile.go`, `cleanup.go` and their tests), always derived from `filepath.Clean(filepath.FromSlash(entry.Path))` or `WorktreePath(l, slug)`.
  `warpPath` lives in the weft-git content-sync files (`commit.go`, `pull.go`, `index.go`, `spawn.go`, `coalesce.go`) and as the `fabric` struct's `warpPath` field.
  Verified: **zero files contain both**, so there is no shadowing and no scope conflict — the rename removes a duplicate name rather than creating a clash.
- **Rejected:** rename to a distinct `warpRepoPath` — avoids the (non-existent) collision but permanently leaves two names for one concept.
  Treat every co-occurrence as a hard blocker — there are none.

### `WeftHostSlug` → `WeftWarpSlug`

- **Decision:** pure mechanical substitution, consistent with every other member of the family.
- **Rationale:** the awkwardness is honest — it *is* the warp slug derived from a weft branch name.
  A rename pass should not smuggle in design changes, and the mechanical form has zero judgment risk.
- **Rejected:** `WarpSlug` — loses that it is derived from the weft branch.
  `WarpSlugFromWeftBranch` — most descriptive but breaks the family's naming shape and is a design change disguised as a rename.

### `internal/lyxtest`'s `CopyHostHub`/`HostFixture` are in scope

- **Decision:** rename them, along with all callers in the 8 dependent packages.
- **Rationale:** `internal/lyxtest` is a Fabric Vocabulary owner dir speaking the retired name at an exported seam.
  Leaving it means "host" survives in eight packages' test code and the task's own premise fails.
  The caller sites are mechanical and covered by the same `wordswap` run.
- **Rejected:** hold the brief's stated two-package scope and file a follow-up — leaves the most widely-imported instance of the retired name in place.
  Rename `CopyHostHub` but not `HostFixture` — splits one seam across two tasks.

### `internal/weftname`, `internal/boardengine`, and `tools/sandbox` are in scope too

- **Decision:** include all three.
  `internal/weftname/weftname.go:10` ("host-worktree slug"), `internal/boardengine/board.go:15` ("host branch"), `:17` ("host's own default branch"), `:25` ("host/warp repo", "the host repo's"), and `tools/sandbox/main.go`'s `hostURL` (`:27`), `fabricHostURL` (`:35`), `fabricHostDir` (`:38`), `hostRepoDir` (`:136`–`:189`) plus its two user-visible error strings (`:139`, `:141`).
- **Rationale:** for `weftname` and `boardengine` this is forced, not optional — the guard tightening removes the host-half owner skip, so leaving those comment lines makes `go test` fail.
  Scope had to grow or the tightening had to be abandoned.
- **Precisely which lines are compile-gating:** only `weftname.go:10` ("host-worktree slug"), `board.go:15` ("host branch") and `board.go:25` ("host/warp repo", "the host repo's") match `hostPhrases`.
  `board.go:17` ("the host's own default branch") does **not** match the phrase list — it is prose polish, renamed for consistency, not because the guard demands it.
  A plan writer must not treat `:17` as gating.
  Verified further: `internal/configsync` is clean, and no owner dir contains a `.md` file, so the tightening forces no additional packages beyond these two.
  For `tools/sandbox`, `CONSTRAINTS.md`'s owner set already names `tools/` and `sandbox/`, the eight `SANDBOX-*-SUITE.md` templates beside it are in scope, and `:139`'s `fabric hub host repo not found at %s` is a user-visible string — renaming the prose while the Go beside it keeps the retired name would be incoherent.
- **Two verb-sense hits must be preserved by hand:** `board.go:23` ("whichever repo **hosts** the wiki") and `:26` ("wiki-**hosting** repo"), plus `tools/sandbox/main.go:32` ("the dedicated hub **hosts** fabric's stricter …").
  So the exclude list is **not** empty once these packages are in scope — unlike the fabric packages, where the single verb-sense hit is reworded away.
  Reword all three the same way `coalesce.go:1` is reworded, before the `wordswap` run, keeping the exclude list empty in practice.
- **Rejected:** exempt `weftname`/`boardengine` in the guard — leaves the retired name in two owner packages and weakens the invariant to buy nothing.
  Abandon the host-half tightening — reverses a decision made for good reason.
  `tools/sandbox` as a follow-up task — leaves the retired name in a user-visible error message.
  `tools/sandbox` error strings only, identifiers untouched — half a rename, and the guard does not cover `tools/` to catch the remainder.

### Non-owner residue is folded in, not deferred

- **Decision:** the fabric-sense `host` residue in `internal/loomengine`, `internal/webstercli`, `internal/buildercli`, `internal/configcli`, `internal/perchcli`, and `internal/websterengine/audit_test.go` is renamed in this task, not left for a follow-up.
- **Rationale:** the task's premise is that the code no longer speaks the retired language.
  A reader of `internal/loomengine/preflight_integration_test.go` meets "paired host+fabric worktree" and `TestPreflight_HostDirty` and has to learn the retired mapping anyway — the premise fails wherever the residue survives, regardless of which package it is in.
  It is also the cheapest it will ever be: the same `wordswap` run covers it, and much of it disappears automatically once `lyxtest.HostFixture` is renamed.
- **This residue is NOT machine-guarded, before or after the tightening.** These are non-owner packages, so the host-phrase rule already applies to them today — yet they pass, because the surviving phrasings ("paired host+fabric worktree", "the host's own default branch", `TestPreflight_HostDirty`) are not on `hostPhrases`, and because `*_test.go` is excluded from every rule.
  That is precisely why the residue accumulated, and why finding it required a grep rather than a failing test.
  After this task it stays a review obligation.
- **Critical: non-owner *production* files are never passed to `wordswap`.**
  `enforcement_test.go:883` fails any non-owner directory whose production file carries a bare `warp` token.
  Swapping `internal/configcli/configcli.go:269` to "the warp `_lyx` parent" would make `internal/configcli` fail `TestEnforcement_FabricVocabulary` on the spot.
  The residue therefore splits by file kind, and the split is exact:

  | File | Kind | Treatment |
  | --- | --- | --- |
  | `internal/configcli/configcli.go:269` ("the host `_lyx` parent") | non-owner production, fabric-sense | **Hand-reword** to a neutral phrase — drop the qualifier, or say "Fabric". Never `warp`. |
  | `internal/builderengine/spawn.go:446` ("the host commit immediately before") | non-owner production, fabric-sense | **Hand-reword** the same way. The same file's `:9`, `:178`, `:236`, `:277` are machine/verb-sense and stay, so the file is hand-edited end to end. |
  | `internal/buildercli/poll.go:321` ("a live pane hosting an idle agent") | non-owner production, verb-sense | **Untouched.** File excluded from the sweep. |
  | Every other residue file — `loomengine/preflight_integration_test.go`, `webstercli/{cli,sync_integration,verbs}_test.go`, `buildercli/{sync_integration,sync,validate}_test.go`, `configcli/configcli_integration_test.go`, `perchcli/run_integration_test.go`, `websterengine/audit_test.go`, `builderengine/gitquery_test.go` | non-owner **test**, pure fabric-sense | **Swept by `wordswap`.** `*_test.go` is excluded from all three guard rules, so `warp` is safe there. |

  Only three non-owner production files contain the token at all, and only two lines in them change.
  "Fabric" is not a policed token — the bare-token rule covers `weft` and `warp` only — so a neutral rewording using it is safe.
- **Boundary within the residue:** `internal/configengine/config_test.go`'s `server: host: localhost` YAML fixture is machine-sense and stays.
  `internal/websterengine/audit_test.go`'s `cd /hub/host` is a fixture *path* naming the warp directory, so it is fabric-sense and renames to `/hub/warp`.
  `internal/webstercli/sync_integration_test.go` creates git repos literally named `"host"` and `"host-weft"` — fixture repo names, fabric-sense, renamed.
- **Rejected:** defer to a follow-up task — leaves the premise unmet and guarantees a fourth vocabulary pass.
  Fold in only the production comment (`configcli.go:269`) and leave test code — test code is where nearly all the residue lives, so this would fix ~1 of ~110 occurrences.

### Vocabulary rule — Fabric outward, warp/weft where the two sides must be distinguished

- **Decision:** the rule is **not** "never warp/weft outside fabric". It is:
  - **Fabric** (capital F) is the name of the fully wired-up composite — warp with junctions into weft inside it.
    Any external reader meaning *the repo as a whole* says Fabric.
  - **warp** and **weft** are used — including in CLI help text and user-visible messages — at exactly those few points where the two sides genuinely must be told apart: `lyx fabric clone <warp-url> <weft-url>`, `fabric: warp/weft out of sync`.
  - **"repo"** alone is too vague to denote warp and is not used as a substitute for it.
  - **"host"** is never used in any of these senses.
- **Rationale:** the CLI user is the most external reader there is, and a `clone` verb taking two URLs cannot avoid naming which is which.
  Forcing "Fabric" there would be wrong, and forcing a vague "repo" would be worse.
  Reserving warp/weft for genuine two-sided distinctions keeps the composite name meaningful everywhere else.
- **Rejected:** mechanical `<host-url>` → `<warp-url>` with no rule — arrives at the right string but leaves the principle unstated for the next task.
  Neutral `<repo-url>`/"the main repo" — the vagueness the user explicitly rejected.
  Freeze `<host-url>` as a backwards-compatible argument name — keeps the retired name exactly where it is most visible.

### Enforcement guard is tightened, not just preserved

- **Decision:** remove the owner-dir skip for the **host half** of `TestEnforcement_FabricVocabulary`, so a fabric-sense `host` phrase or geometry identifier fails inside `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine` too.
  The host half appears in **two** walks and both are tightened: the Go walk's `!fabricVocabularyOwners[dir] && hostHit` at `:886`, and the `internal/**/*.md` walk's `!fabricVocabularyOwners[dir] && fabricSenseHostPhrase(text)` at `:903`.
  The `.md` half is free — no owner dir contains a `.md` file, as verified above.
  The bare weft/warp owner-set skip is unchanged in both walks.
  The phrase predicate stays — it is what separates fabric-sense from verb-sense and machine-sense.
- **Rationale:** today the owner dirs are skipped entirely, so nothing machine-proves the rename stays done and a future card can reintroduce `hostBranch` freely.
  After this task the fabric packages contain zero "host" in any sense, so the tightened rule has no false positives to accommodate.
  Without this, the rename is a one-time cleanup rather than an enforced invariant.
- **What the tightened guard does *not* cover — state this honestly, do not overclaim.**
  Read from `internal/lyxcwd/enforcement_test.go`, the guard's reach after tightening is still bounded three ways:
  - `hostGeometryIdentifiers` is five exact lowercased names (`hostbranch`, `hostlayoutfor`, `hostreason`, `hostjunction`, `hostclean`).
    `HostJunctions`, `hostPath`, `hostBare`, `CopyHostHub` and `HostFixture` are **not** matched by the identifier half — only by the phrase half, and only where they occur inside a policed phrase.
  - `*_test.go` files are excluded from all three rules (line ~868), so no test file is scanned.
  - The walk covers `internal/` and `cmd/` `.go` files plus `internal/**/*.md` and the embedded agent prompt templates — never `docs/`, `README.md`, `manifest/`, `tools/sandbox/*.md`, `.claude/agents/`, or `post-checkout.sh`.
  Therefore: production Go under `internal/`+`cmd/` is machine-guarded; **test files, documentation outside `internal/`, shell, and `tools/` remain a review obligation.**
  The `CONSTRAINTS.md` rewrite must say this rather than implying full coverage.
- **Rejected:** leave the guard as-is — review discipline only, and this is the third pass at the same vocabulary precisely because discipline alone did not hold.
  Tighten only the identifier half — leaves prose reintroduction unguarded.
  Broaden the guard in this task (prefix rule instead of five exact names, lift the `_test.go` exclusion, extend walk roots to `docs/`/`manifest/`/`tools/`) — a materially larger change to a shared enforcement test, with its own false-positive surface across every module's prose;
  it belongs in its own task, not bolted onto a rename.

### `CONSTRAINTS.md` — full rewrite of the Fabric Vocabulary Invariant

- **Decision:** rewrite the invariant rather than appending bullets.
  The rewritten invariant states: `host` is a **retired** name, banned in the fabric sense everywhere including the owner dirs; the phrase predicate is retained as the sense-discriminator; **Fabric** is the external name for the composite; and warp/weft carry a carve-out for the few user-visible points where the two sides must be distinguished.
- **Rationale:** the retirement, the Fabric name, and the CLI carve-out are three faces of one rule.
  Expressed as appended bullets they read as accreted patches and the next reader has to reconstruct the rule.
- **Rejected:** append two bullets — minimal diff, layered rule.
  Split into two invariants — cleaner separation but neither is comprehensible alone.

### Commit granularity — four commits

- **Decision:**
  - **(a)** Reword the three verb-sense hits (`coalesce.go:1`, `board.go:23`/`:26`, `tools/sandbox/main.go:32`); build `tools/wordswap/` with tests; run it over the Go file set — fabric packages, `internal/lyxtest`, `internal/weftname`, `internal/boardengine`, `tools/sandbox/*.go`, the non-owner residue packages, and all callers; resolve the five reported ambiguous compounds by hand. Identifiers, comments, test names, string literals.
  - **(b)** File renames as `Moves:` pairs.
  - **(c)** Non-Go surfaces: `post-checkout.sh`, `fabriccli` `Short`/`Long` help strings, `tools/sandbox/main.go`'s two error strings.
  - **(d)** Documentation sweep, `CONSTRAINTS.md` rewrite (Fabric Vocabulary + Cwd Resolution citation + Fabric Git prose), and the `enforcement_test.go` tightening.
- **Rationale:** each commit is `go build ./... && go test ./...` green and has one readable nature.
  (a)–(c) are mechanical; (d) is the only one requiring judgment, so it is the only one a reviewer must read closely.
- **Rejected:** one commit — a ~300-file diff mixing mechanical and judgment work is unreviewable.
  Two commits — better, but still merges the file renames into the token sweep, which obscures both.

### `manifest/designs/host-visibility.md` → `warp-visibility.md`

- **Decision:** rename the file as a `Moves:` pair and update its three inbound references.
- **Rationale:** the doc is named for the retired vocabulary and describes fabric's own mechanism level (owner prose), so it keeps warp/weft internally while shedding "host".
- **Rejected:** leave the filename — the retired name survives in a path that other docs link to.

## Technical context

### The boundary-case survey is already done

The brief called for a survey pass to build an exclude list before scripting.
That survey is complete and its result is recorded here, so **mill-plan does not need to schedule a separate survey step**:

- Machine-sense `host` inside `internal/fabricengine` / `internal/fabriccli`: **none**.
  No `conhost`, no `localhost`, no `Hostname`, no `Write-Host`.
- Verb-sense **in the files actually swept**: four, all reworded before the sweep — `internal/fabricengine/coalesce.go:1`, `internal/boardengine/board.go:23` and `:26`, and `tools/sandbox/main.go:32`.
  Verb-sense **kept as-is**: `internal/buildercli/poll_test.go:212` ("a live pane hosting an idle agent"), adjudicated via `-skip`;
  and `internal/buildercli/poll.go:321` (same wording), whose file is excluded from the sweep entirely as non-owner production.
  So the run does take a `-skip` argument — see Decisions, "Ambiguous compounds are reported, not guessed".
  `internal/builderengine/spawn.go`'s machine/verb-sense lines (`:9`, `:178`, `:236`, `:277`) never reach the tool either;
  that file is hand-edited because bare `host` with a clean token boundary ("the plain host filesystem") would otherwise be swapped silently rather than reported as ambiguous.
- `DeriveHostName(hostURL)` looks machine-sense but is not: it extracts the **warp repository basename** from a raw URL or file path (`clone.go:355–371`). It renames to `DeriveWarpName(warpURL)`.
- Ambiguous all-lowercase compounds are handled by the tool's report, not by this list — see Decisions, "Ambiguous compounds are reported, not guessed".

### Exhaustive repo-wide classification

Every tracked file containing `host` in any case was enumerated via `git ls-files | xargs grep -li host` and classified.
The result is closed — a plan writer does not need to re-derive it, and anything not listed below contains no `host` at all.

**Fabric-sense — in scope** (detailed elsewhere in this document): `internal/fabricengine` (62 files), `internal/fabriccli` (5), `tools/sandbox` (6 `.go` + 8 `SUITE.md`), `internal/lyxtest` (3), `internal/boardengine` (4), `internal/weftname` (1), the non-owner residue (`internal/buildercli`, `internal/webstercli`, `internal/configcli`, `internal/loomengine`, `internal/perchcli`, `internal/websterengine`), the `CopyHostHub` caller sites in `internal/idecli` and `internal/lyxcwd`, `cmd/lyx/tierpurity_test.go:50`, the swept docs, `.claude/agents/crucible-reviewer-*.md:16`, and `README.md`/`CONSTRAINTS.md`.

Plus one hit the phrase-based greps missed:

- `internal/builderengine/gitquery_test.go:23` — "lyxtest's weft/host fixtures".
  Fabric-sense, in scope.
  The rest of `internal/builderengine` is machine-sense ("the plain host filesystem", `spawn.go:9`/`:236`) or verb-sense ("a downed reed session hosts no live strand", `spawn.go:178`) and stays.

**Machine-sense, verb-sense, or unrelated — out of scope, no change:**

| Location | Sense |
| --- | --- |
| `plugins/prowler/*` | `Hostname()`, "unresponsive hosts", Chrome on the host — machine |
| `internal/reedengine` | "can host a strand" — verb; "host-testable" — machine |
| `internal/reedcli` | `Write-Host` PowerShell cmdlet |
| `internal/treadleengine`, `internal/shuttlecli` | orphaned `conhost.exe` teardown probes |
| `internal/burlerengine` | "low-core host", "slow host" — machine; `"ghost"` — test data |
| `internal/shuttleengine` | `\\host\share` UNC path, "absolute on any host" — machine |
| `internal/shell` | "host-testable", "the current host" — machine |
| `internal/stencil`, `internal/modelspec`, `internal/boardcli` | `Ghost`/`"ghost"` — test data |
| `internal/githubclient` | "not logged in to any GitHub hosts" |
| `internal/configengine/config_test.go` | `server: host: localhost` YAML fixture |
| `cmd/lyx/crosscompile_test.go`, `gitrepoboundary_test.go` | "the host's native", "it hosts three r.run calls" — machine/verb |
| `crucible/` (3 files, 5 hits) | "the host's RAM", "a host process killed" — machine |
| `docs/reference/builder-contract.md` | "a pane hosting an idle agent", "hosts no live strand" — verb |
| `docs/reference/tmux_scripting.md` | `#H` → `Hostname` |
| `docs/research/reed-exploration.md` | "reed is host-agnostic" — machine |
| `docs/research/reed-hooks-exploration.md` | "daemon hosts background sessions" — verb |
| `docs/research/scout-multilang.md` | "same host as the scout spike" — machine |
| `docs/shared-libs/yamlengine.md` | `${env:HOST:-localhost}` env-var example |
| `go.mod`, `go.sum` | `github.com/skeema/knownhosts` dependency |

### Merged names — the co-occurrence check is generalized, not spot-checked

Two `host*` names have a pre-existing `warp*` twin they merge into, so a naive swap could in principle shadow or redeclare.
The check was run over **every** `host<X>` identifier in `internal/`, `cmd/`, and `tools/`, not just the one found first:

| Merged name | Existing twin | Files containing both |
| --- | --- | --- |
| `hostPath` (47) | `warpPath` (321) | none |
| `hostBare` (29) | `warpBare` (`coalesce_integration_test.go:95`) | none |

No other `host<X>` has a `warp<X>` twin.
Zero co-occurrence in both cases, so both are safe global substitutions.
This matters because `go build` catches package-level redeclaration but **not** a local shadowing a same-named package-level symbol — the compile alone would not have proven this.

### The ban list is not a rename target

Two places name the retired vocabulary *in order to forbid it* and must keep the word `host`:

- `CONSTRAINTS.md:160–161` — the phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`) and the identifier list (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`), plus the sentence explaining that the bare word passes untouched.
- `internal/lyxcwd/enforcement_test.go` — the `hostPhrases` slice (line ~633), the `hostGeometryIdentifiers` map (line ~622), the `fabricSenseHostPhrase` predicate and its sub-tests (`host_repo_phrase_fails`, `hostBranch_identifier_fails`, `host_verb_sense_passes`, `host_machine_sense_passes`, `write_host_cmdlet_passes`).

**Neither file may be passed to `wordswap`.** Both are hand-edited in commit (d).

The rest of `CONSTRAINTS.md` does rename, and the list is wider than the Fabric Git Invariant alone:

- The **Cwd Resolution Invariant**'s bullet "Weft-sibling paths and junction construction belong to `internal/fabricengine`, never `lyxcwd`: `WeftWorktree`/`WeftRepoRoot`/`HostLyxLink`/`HostJunctions`/portal and launcher paths …" cites two identifiers this task retires.
  It renames.
  The Constraints section below records this correctly — the invariant's *semantics* are unaffected, but its identifier citations are not.
- The **Fabric Git Invariant** — lines 175, 180, 188, 200, 214.

### File renames (`Moves:` pairs)

| From | To |
| --- | --- |
| `internal/fabricengine/hostclean.go` | `internal/fabricengine/warpclean.go` |
| `internal/fabricengine/hostlayout.go` | `internal/fabricengine/warplayout.go` |
| `internal/fabricengine/hostjunction_test.go` | `internal/fabricengine/warpjunction_test.go` |
| `manifest/designs/host-visibility.md` | `manifest/designs/warp-visibility.md` |

Inbound references to `host-visibility.md` needing update: `manifest/roadmap.md:44`, `manifest/roadmap.md:81`, `manifest/roadmap.md:84`, `manifest/roadmap.md:240`, `manifest/designs/fabric-unified-view.md:203`.

### Measured identifier surface

Exported in the fabric packages: `HostJunctions` (51), `HostJunctionsHere` (34), `HostLyxLinkHere` (23), `HostLyxLink` (19), `HostJunction` (19), `WeftHostSlug` (19), `HostWorktree` (11), `DeriveHostName` (10), `HostBranch` (3).

Unexported and locals: `hostLayout` (97), `hostBranch` (81), `hostPath` (47), `hostBare` (29), `hostDotLyx` (14), `removeHostJunction` (7), `wantHostLyxLinkHere` (6), `wantHostLyxLink` (6), `originalHostBranch` (6), `hostWtName` (6), `hostLyxDir` (6), `hostLyx` (6), `childHost` (6), `removeHostLayout` (5), `hostWorktreePath` (5), `hostSlug` (5), `detectHostPollution` (5), `hostLayoutFor` (4), `makeCLICloneHostBare` (4), plus a long tail.

In `internal/lyxtest`: `CopyHostHub`, `HostFixture`, `buildHostHub`, `hostHubTemplate`, `hostHubOnce`, `hostHubPath`, `hostHubBarePath`.

Test-function names to rename: `TestOpen_MissingHostWorktree`, `TestWireJunctions_RefusesRealHostDirectory`, `TestWeftHostSlug`, `TestWeftBranchName_RoundTripsWithWeftHostSlug`, `TestUnwire_NeverWiredHostIsIdempotentNoOp`, `TestHostLyxLinkMethods`, `TestHostJunctionsHere`, `TestHostJunctions`, `TestDetectHostPollution_*` (3), `TestCoalescePushBothAt_AdvancesBothSidesAndLeavesNoHostRootLock`, `TestCleanup_DetachedHostHeadProtectsCheckedOutWeftBranch`, `TestDeriveHostName`, `TestCopyHostHub`, `TestCopyHostHub_Isolation`.

Note: `HostRootLock`, `HostBare`, `HostHub`, `HostSlug`, `HostName` and `HostLayout` from the task brief do **not** exist under those exact names — the brief's inventory was approximate. The list above is the measured one.

### Non-Go surfaces

- `internal/fabricengine/post-checkout.sh` — `HOST_BRANCH` shell variable (lines 43–58), comments (6–7, 42, 54–55), and **user-visible stderr**: `fabric: host/weft out of sync — run \`lyx fabric checkout <branch>\` …` and `host: $HOST_BRANCH (expects weft: $EXPECTED_WEFT_BRANCH)`.
  `gofmt`-based tooling would not reach this file; `wordswap` does.
- `internal/fabriccli/fabric.go` — `Short`/`Long` strings at lines 36, 37, 62, 65, 68, 69, 73, 110, 111, 130, 131, 143, 144, 164, 165, 184, 185, 200, 201, 217, 229, 230 plus the package doc at lines 3–13. Includes `<host-url>`, `<host-name>`, "host prime", "host junctions", "host↔weft".
- Verified: **no golden files or testdata** contain `host`, and `cmd/lyx/helptree_test.go` / `longlist_test.go` / `jsonhelp_test.go` contain **no** `host` assertions. The CLI text change is therefore low-risk.

### Documentation surface (fabric-sense hits per file)

`docs/overview.md` (13), `tools/sandbox/SANDBOX-FABRIC-SUITE.md` (12), `tools/sandbox/SANDBOX-CORE-SUITE.md` (12), `docs/sandbox-hub.md` (11), `docs/sandbox-howto.md` (8), `tools/sandbox/SANDBOX-PERCH-SUITE.md` (7), `tools/sandbox/SANDBOX-BURLER-SUITE.md` (7), `tools/sandbox/SANDBOX-BUILDER-SUITE.md` (7), `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` (6), `tools/sandbox/SANDBOX-REED-SUITE.md` (6), `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` (5), `README.md` (5), `manifest/roadmap.md` (4), `manifest/designs/host-visibility.md` (4), `CONSTRAINTS.md` (4 — but see "The ban list is not a rename target"), `manifest/designs/loom.md` (2), `docs/shared-libs/configengine.md` (1).

Two further docs cite retired **identifiers** rather than the phrase, so a phrase-based grep misses them:

- `docs/shared-libs/lyxcwd.md:82` — "Weft-sibling paths and junction construction (`WeftWorktree`, `HostLyxLink`, `HostJunctions`, portal and launcher paths …)".
  This is the doc mirror of the `CONSTRAINTS.md` Cwd Resolution bullet the task renames, and must move with it.
- `manifest/designs/fabric-unified-view.md:86` — "**`Weft*`/`Host*Link`/junction-construction methods** (`WeftWorktree`, `WeftRepoRoot`, `HostLyxLink`, `HostJunctions`, `PortalLink`, `LauncherDir`, etc.)".
  `fabric-unified-view.md` is owner prose, so it keeps warp/weft freely — but the retired `Host*` identifier citations still rename.

Plus `.claude/agents/crucible-reviewer-{low,medium,high,max,xhigh}.md`, each at line 16: "This is a **host-repo** commit on the crucible worktree, never a weft-repo operation."
Five files, one identical line — it names the warp side against the weft side, so it becomes "warp-repo" per the two-sided-distinction rule.

The eight `tools/sandbox/SANDBOX-*-SUITE.md` files are **agent prompt templates** shipped into the sandbox hub and read by a black-box agent, so they are consumer-facing prose: the composite is "the Fabric repo"; warp/weft appear only where the sandbox's two sides must be distinguished.

Applying the vocabulary rule per file: `manifest/designs/fabric-unified-view.md` and the `fabricengine`/`fabriccli` package docs are **owner prose** and keep warp/weft freely.
Everything else is consumer prose — "Fabric" for the composite, warp/weft only for genuine two-sided distinctions.

### Deliberate documentation exclusions — the historical-record class

Three files are **not** swept.
All three are dated records of measurements and investigations performed at specific past commits, and all three already preserve other retired names from the same era (`internal/warpengine`, `internal/worktree`, `warpcli`, `hubgeometry`).
Renaming the symbols they cite would falsify what was actually measured or observed.
Historical records keep the vocabulary that was true when they were written:

- `docs/benchmarks/test-suite-timing.md` — lines 547, 748, 754, 854, 936 (`CopyHostHub`, `TestRemoveHostJunctionRemoved`, `TestWeftRollbackOnPostHostCreateFailure`, `TestWeftHostPristineEnforced`).
- `docs/benchmarks/fixture-copy.md` — lines 214, 236, 242 (`hostLayoutFor`, `hostPath`, alongside `internal/warpengine` at `:56`, `:58`, `:83`, `:212`).
- `docs/research/scout-spike.md` — lines 111, 116, 118, 123, 133 (`hubgeometry.WeftHostSlug`, and a call chain naming `internal/warpengine/prune.go` and `internal/warpcli/warp.go`).
- `docs/research/linux-portability-survey.md` — line 46 ("the symlink model otherwise carries the weft/host topology fine on Linux").
  Fabric-sense, but the sentence records what a specific past survey observed and names `internal/warpengine` in the same breath.

This is a closed list, not a rule of thumb: any *other* doc citing a retired identifier is swept (see the two identifier-citing docs named above).
The distinction is whether the doc records a past observation or describes present behaviour.

### Existing guard mechanics

`internal/lyxcwd/enforcement_test.go` currently holds:

- `fabricVocabularyOwners` (line ~596): `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine`, `internal/configsync`.
- `shouldSkipBareVocabularyCheck(dir)` (line ~665) — skips every owner row except `configsyncOwnerDir`.
- `failsBareVocabularyCheck(dir, bareIdent, bareLiteralOrComment)` (line ~675) — `configsync`'s narrower literal-and-comment carve-out.
- `fabricVocabularyHits(f *ast.File)` (line ~688) — returns `(bareIdent, bareLiteralOrComment, hostHit)`; the host half currently shares the same owner skip as the bare-token half (per "card 26").

The tightening splits that shared skip: the bare weft/warp half keeps the owner skip; the host half no longer applies it.
`CONSTRAINTS.md:164` states the owner set includes `tools/` and `sandbox/`, while `fabricVocabularyOwners` does not list them.
**Intended end state: the doc drops them from the owner set** and instead records that `tools/` and `sandbox/` lie outside the enforcement walk entirely — the Go walk covers `internal/` and `cmd/` only, so an owner-map entry for them would be dead code that never matches.
Calling them "owners" implies a carve-out from a check that never reaches them.
Do not add them to the map.

### Tooling available

`lyx scout` provides `refs`, `definition`, `symbol`, `assert-no-callers`.
There is **no** `scout literals` — prose is a grep-and-read job, which the survey above has already completed.
`scout refs <symbol>` is available for spot-verifying an exported symbol's call sites if a reviewer wants independent confirmation, but the `wordswap` + `go build` combination is the primary completeness proof.

Go toolchain: go1.26.0.

### House pattern for `tools/wordswap/`

Model on `tools/mdreflow/main.go`:

- `package main` with a package doc comment stating purpose, the safety invariant, usage lines, and a `Last run:` note.
- `flag.Bool("dry-run", …)`, glob-expanded positional path arguments, positional passthrough when a pattern matches nothing.
- Per-file `processFile` returning a status string; counts of changed/unchanged; a `mismatch` bucket for files failing the safety invariant, printed as `MISMATCH (left untouched): <path>` and reflected in the exit code.
- Unit tests alongside (`reflow_test.go` is the shape to follow) covering the case-preserving substitution table, the whole-word boundary rule, the `-skip` regexp, and the reversibility invariant including the case where the target word pre-exists in the input.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Vocabulary Invariant** — rewritten by this task (see Decisions). Enforced by `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`).
- **Fabric Git Invariant (warp + weft)** — prose renames (lines 175, 180, 188, 200, 214), semantics unchanged.
- **Cwd Resolution Invariant** — **semantics** unaffected (no path, anchor, or cwd-resolution change), but its text is not: the "Weft-sibling paths and junction construction" bullet cites `HostLyxLink`/`HostJunctions` by name and renames with them.
  `internal/lyxcwd` code is touched only in its enforcement test and in `anchor_test.go`'s `CopyHostHub` call sites.
- **lyxtest Leaf Invariant** — unaffected; the rename adds no imports to `internal/lyxtest`.
- **CLI/Cobra Invariant** — `Short` must remain present on every command; the help-tree tests must stay green after the `Short`/`Long` rewording.
- **Documentation Lifecycle** — this task changes observable CLI text and cross-cutting infrastructure, so `docs/overview.md`, the module docs, and `CONSTRAINTS.md` land in the same commits. `manifest/roadmap.md` moves only for a completed or added planned item; here it is edited for prose and the `host-visibility.md` link, not for a roadmap status change.

From this repo's `CLAUDE.md`:

- **Markdown semantic line breaks** — every `.md` file this task rewrites must keep one sentence per line with breaks at internal independent-clause boundaries. `wordswap` is a word-level substitution and preserves line structure, so it does not disturb this; the hand-written prose in commit (d) must follow it.
- **Worktree isolation** — all work stays in `<container>/wts/fabric-host-to-warp-rename`; no push to `main` from here.

Discovered during discussion:

- `wordswap` must never be run over `CONSTRAINTS.md`, `internal/lyxcwd/enforcement_test.go`, `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/research/scout-spike.md`, `docs/research/linux-portability-survey.md`, `internal/configengine/config_test.go`, or anything under `crucible/`.
- The three verb-sense rewords (`coalesce.go:1`, `board.go:23`/`:26`, `tools/sandbox/main.go:32`) must precede the `wordswap` run.
- The `weftname`/`boardengine` fixes must land **in or before** the tightening's commit — the tightening fails `go test` without them.
  The four-commit plan satisfies this: the fixes are in (a), the tightening in (d).
- Non-owner **production** files (`internal/configcli/configcli.go`, `internal/builderengine/spawn.go`, `internal/buildercli/poll.go`) are hand-edited, never passed to `wordswap` — a bare `warp` token there fails the guard.

## Testing

**`tools/wordswap/` — TDD candidate, the one genuinely new code in this task.**
Write the tests first; this tool mutates ~300 files, so its correctness is the task's main risk.
Scenarios to cover:

- Case-preserving substitution: `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP`; embedded forms `hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`.
- Whole-word/token boundary: `ghost` and `localhost` must **not** match (lowercase precedes), while `hostBranch`, `HostJunction`, `HOST_BRANCH` and bare `host` must.
  This is the subtle one — the boundary is camel/snake-token-aware, not merely `\b`-anchored, since `hostBranch` has no word boundary after `host`.
  Pin the exact intended rule with tests before implementing.
- **Ambiguity classification** — the other subtle one, and the reason the tool is safe to reuse.
  `hostclean`, `hostlayout`, `hosthub`, `hostname`, `conhost` at a token start followed by lowercase must all be classified AMBIGUOUS: not swapped, reported with file and line.
  Assert both halves — that the file content is unchanged for them, *and* that they appear in the report.
  Assert the non-zero exit when the report is non-empty.
- Mixed forms in one line, and multiple occurrences on one line.
- `-skip <regexp>`: matching occurrences are left untouched and reported; non-matching ones still swap.
- **Reversibility invariant**: including the critical case where the target word already occurs in the input (mirroring `warp`'s 576 pre-existing occurrences), plus a deliberately-broken transform that must be caught and leave the file untouched.
- `-dry-run` writes nothing.
- Non-Go input: a shell fragment and a markdown fragment, proving language-agnosticism.

**The rename itself — verification is mechanical, not new tests.**
`go build ./... && go test ./...` green after each of the four commits.
A clean compile proves no call site was missed and no package-level identifier collided; it is cheap enough to run per commit rather than only at the end.
It does **not** prove the two merged names are safe — a local shadowing a same-named package-level symbol still compiles — which is why the generalized co-occurrence check in Technical context was run separately and must be re-run if the rename table grows.

**Existing tests that must stay green and will need name updates:** the ~16 renamed `Test*` functions listed in Technical context are renamed by `wordswap` in commit (a), so they must still be discovered and pass.
`cmd/lyx/helptree_test.go`, `longlist_test.go`, and `jsonhelp_test.go` must stay green after the CLI help rewording in commit (c) — verified to contain no `host` assertions, so this should be a no-op, but it is the CLI/Cobra Invariant's check and must be confirmed rather than assumed.

**`TestEnforcement_FabricVocabulary` — the tightening needs its own new cases.**
After removing the host half's owner skip, add cases proving:

- A fabric-sense host phrase in an owner dir now **fails** (it previously passed).
- The bare weft/warp owner skip is **unchanged** — an owner dir may still say warp/weft freely.
- `configsync`'s narrower literal-and-comment carve-out is unaffected.
- The existing sense-discrimination sub-tests still pass: `host_verb_sense_passes`, `host_machine_sense_passes`, `write_host_cmdlet_passes`.

**Repo-wide completeness check, run manually at the end of commit (d):** grep for any remaining `host` in any case across the in-scope set — the fabric packages, `internal/lyxtest`, `internal/weftname`, `internal/boardengine`, `tools/sandbox`, the non-owner residue packages, and every swept doc.
Expect zero hits outside the exclusion list, which is **exactly** the "never run over" list in the Constraints section plus the machine-/verb-sense keeps recorded in the final `-skip` set.
The two lists are the same set by construction — if they ever diverge, the Constraints list is authoritative.

This is a **manual** check, and it stays manual: as recorded in Decisions, the tightened guard covers production Go under `internal/`+`cmd/` only — not `*_test.go`, not `docs/`, not `tools/`, not shell.
Do not describe the tightened test as encoding this check permanently; it does not.

## Q&A log

- **Q:** Is `internal/lyxtest`'s `CopyHostHub`/`HostFixture` in scope, given the brief says `fabricengine`/`fabriccli` only? **A:** In scope — it is an owner dir exporting the retired name to 8 packages.
- **Q:** `WeftHostSlug` reads badly under mechanical substitution — target name? **A:** `WeftWarpSlug`. Mechanical consistency beats a design change smuggled into a rename pass.
- **Q:** `hostPath` (47) appears to collide with the pre-existing `warpPath` (321) — how is that resolved? **A:** Not a collision. They are the same concept named twice, split by module half; zero files contain both. Global substitution, no per-file judgment.
- **Q:** Should the enforcement guard be tightened so fabric-sense host fails inside the owner dirs too? **A:** Yes — otherwise nothing machine-proves the rename stays done, and this is already the third pass at the same vocabulary.
- **Q:** How far does the documentation sweep reach? **A:** All of it, including `docs/sandbox-*.md` and the eight `tools/sandbox/SANDBOX-*-SUITE.md` agent prompt templates.
- **Q:** Which tool performs the substitution? **A:** One generic case-preserving whole-word swap, not a per-identifier table — the exclude list is empty, so a table protects against nothing.
- **Q:** Should the rename script be committed? **A:** Not as a one-off specialised script. Make it general enough to be reusable and commit it — `tools/wordswap/`, following the `tools/mdreflow/` and `tools/godocreflow/` precedent.
- **Q:** What is the tool's safety invariant, analogous to `mdreflow`'s collapsed-text check? **A:** Reversibility over recorded spans — revert exactly the recorded substitution offsets and require byte-identity with the input. Works despite `warp`'s 576 pre-existing occurrences.
- **Q:** How is the verb-sense hit in `coalesce.go:1` excluded? **A:** It is not excluded — it is reworded to "holds" first, leaving an empty exclude list and no `-skip` argument for this run.
- **Q:** Should the word "host" appear in the skip pattern at all? **A:** No. Rewording removes the last occurrence, so the fabric packages end with zero "host" in any sense.
- **Q:** Are file renames done by the tool or separately? **A:** Separately, as explicit `git mv` / `Moves:` pairs — four of them, including `host-visibility.md` → `warp-visibility.md`.
- **Q:** What does a CLI user see — `<host-url>`, `<warp-url>`, or `<repo-url>`? **A:** `<warp-url>`/`<weft-url>`. warp/weft are used at exactly the few points where the two repos must be distinguished; "repo" alone is too vague for warp; "host" is never used.
- **Q:** What replaces "host repo" in non-owner docs? **A:** "Fabric" — the name of the fully wired-up composite, warp with junctions into weft inside it.
- **Q:** How is `CONSTRAINTS.md`'s Fabric Vocabulary Invariant updated? **A:** Full rewrite — retirement of `host`, the Fabric external name, and the warp/weft CLI carve-out are three faces of one rule.
- **Q:** Commit granularity? **A:** Four commits: (a) reword + tool + Go sweep, (b) file renames, (c) non-Go surfaces, (d) docs + CONSTRAINTS + enforcement tightening.

Round 1 review gaps:

- **Q:** The guard tightening reaches `internal/weftname` and `internal/boardengine`, which were out of scope — extend scope, exempt them, or drop the tightening? **A:** Extend scope. The tightening fails `go test` otherwise, so this was forced rather than optional.
- **Q:** `CONSTRAINTS.md`'s Cwd Resolution Invariant cites `HostLyxLink`/`HostJunctions`, but the discussion claimed the invariant was "unaffected" — how is that reconciled? **A:** Corrected. Semantics unaffected, identifier citations rename.
- **Q:** The token-boundary rule cannot both reject `hostname`/`localhost`/`conhost` and match `hostclean`/`hostlayout` — how is the ambiguity resolved? **A:** The tool reports rather than guesses. `host` + lowercase is classified AMBIGUOUS, printed with file and line, and adjudicated by the implementing LLM; the tool exits non-zero while any remain. Mechanical work is scripted, judgment work is not, and the tool surfaces the boundary instead of hiding it.
- **Q:** Does the tightened guard really "encode permanently" the zero-host assertion? **A:** No — it covers production Go under `internal/`+`cmd/` only. The overclaim is removed and the guard's real reach is stated; broadening it is its own task.
- **Q:** Is `tools/sandbox/*.go` in or out, given it holds `hostURL`/`hostRepoDir` and a user-visible error string? **A:** In. `tools/` and `sandbox/` are named owner dirs, and renaming the prompt templates while the Go beside them keeps the retired name would be incoherent.

Round 2 review gaps:

- **Q:** Fabric-sense `host` also lives in `loomengine`, `webstercli`, `buildercli`, `configcli`, `perchcli` and `websterengine` — deferred or folded in? **A:** Folded in. The premise fails wherever the residue survives, and the same `wordswap` run covers it. Machine-sense `configengine` YAML fixture data stays out.
- **Q:** The `tools/sandbox` line list names only `main.go`, but `suite.go`, `report.go` and three test files carry ~157 further occurrences — enumerate or glob? **A:** Glob. The scope is `tools/sandbox/*.go`; the per-line citations are illustrative, and there are four user-visible error strings, not two.
- **Q:** `docs/benchmarks/fixture-copy.md` and `docs/research/scout-spike.md` are the same historical-record class as the excluded `test-suite-timing.md` — exclude them too? **A:** Yes. Closed list of three, distinguished by whether the doc records a past observation or describes present behaviour.
- **Q:** `docs/shared-libs/lyxcwd.md:82` and `manifest/designs/fabric-unified-view.md:86` cite retired identifiers but appear in no list — in or out? **A:** In. They cite identifiers rather than phrases, so a phrase-based grep missed them.
- **Q:** Does the "five ambiguous occurrences" figure hold for the whole task? **A:** No — and after round 3 no count is pinned for any commit. The report is the work list, never a checksum.

Round 3 review gaps:

- **Q:** Word-swapping non-owner production comments to `warp` would fail `enforcement_test.go:883`'s bare-token rule — how is the residue applied? **A:** Split by file kind. Non-owner production files are hand-edited and never passed to the tool (exactly three files, two changed lines, reworded neutrally); non-owner test files are swept freely, since `*_test.go` is excluded from every guard rule. This also averts a second failure the finding did not name: bare machine-sense `host` with a clean token boundary (`spawn.go:9`, "the plain host filesystem") would have been swapped silently rather than reported.
- **Q:** "Exactly three verb-sense hits" undercounts the widened scope — restate? **A:** Restated as four swept-and-reworded plus two kept (`poll.go:321`, `poll_test.go:212`).
- **Q:** If the tool exits non-zero while any ambiguity is unresolved, how is a legitimate "leave it alone" verdict recorded? **A:** Via `-skip`. Completion is an exit-zero run with an explicit `-skip` set, and that set is the audit record of every deliberate keep. The earlier "no `-skip` at all" framing held only for the original narrow scope.
- **Q:** The completeness-grep's exclusion list omits three files excluded elsewhere in the same document. **A:** Unified — it is the Constraints "never run over" list plus the `-skip` keeps, with Constraints authoritative.
- **Q:** "Guard tightening and the fixes in the same commit" contradicts the four-commit plan. **A:** Restated as "in or before"; (a)→(d) satisfies it.
- **Q:** Is `internal/builderengine` in the residue list? **A:** Yes — `gitquery_test.go:23` and `spawn.go:446` are fabric-sense; `spawn.go:9`/`:178`/`:236`/`:277` are machine/verb-sense and stay, so that file is hand-edited end to end.
