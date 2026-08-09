# Batch: docs-sweep

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "docs-sweep"
number: 6
cards: 6
verify: go test ./cmd/lyx/... ./internal/lyxcwd/...
depends-on: [5]
```

## Batch Scope

The documentation half of the task, and its judgment core.

`_mill/discussion.md` splits the doc work into a mechanically-swept set and a hand-reworded set, because a blind `host`→`warp` swap over consumer prose produces "the warp repo" precisely where the vocabulary rule demands "the Fabric repo" — it would violate the very rule this task exists to establish.
This batch keeps that split but **narrows the mechanical set further than the discussion did**, on evidence the discussion did not have:

- `manifest/designs/fabric-unified-view.md` cannot be swept.
  Its line 122 reads "a fabric-sense `host`" — it names the retired vocabulary in order to describe the rule policing it, exactly like the ban list in `CONSTRAINTS.md`, and sweeping it would produce the nonsense "a fabric-sense `warp`".
  Its line 88 ("`hubgeometry.BoardDir` … hosts more than board's own data") is verb-sense.
  The file is therefore hand-edited, with only lines 86 and 162 changing.
- `docs/shared-libs/configengine.md` cannot be swept.
  Its line 66 contains `${env:HOST:-localhost}`, a machine-sense environment-variable example.

That leaves the mechanical set as exactly six files carrying six occurrences, all pure retired-**identifier** citations or the one two-sided phrase: `docs/shared-libs/lyxcwd.md` and the five `.claude/agents/crucible-reviewer-*.md` files.
Everything else is hand-reworded, file by file, asking per occurrence: does this sentence mean *the composite repo* (→ "Fabric"), or does it genuinely need to distinguish the two sides (→ warp/weft)?

**Two exclusions bind every card in this batch.**
The four historical-record docs — `docs/benchmarks/test-suite-timing.md`, `docs/benchmarks/fixture-copy.md`, `docs/research/scout-spike.md` and `docs/research/linux-portability-survey.md` — are not touched at all;
they record what was measured at a past commit and already preserve other retired names from the same era.
And `docs/overview.md` line 80 is **not** edited here: it restates the Fabric Vocabulary Invariant's ban list and owner set, so it moves in batch 7 alongside the `CONSTRAINTS.md` rewrite it mirrors, keeping the two consistent in one commit.

## Cards

### Card 15: sweep the six identifier-citation documentation files

- **Context:**
  - `tools/wordswap/main.go`
  - `tools/wordswap/swap.go`
  - `CONSTRAINTS.md`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:**
  - `docs/shared-libs/lyxcwd.md`
  - `.claude/agents/crucible-reviewer-low.md`
  - `.claude/agents/crucible-reviewer-medium.md`
  - `.claude/agents/crucible-reviewer-high.md`
  - `.claude/agents/crucible-reviewer-max.md`
  - `.claude/agents/crucible-reviewer-xhigh.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Run the tool over exactly these six files, with no `-skip` set — every occurrence in them is unambiguous:

  ```
  go run ./tools/wordswap -from host -to warp -dry-run docs/shared-libs/lyxcwd.md .claude/agents/crucible-reviewer-*.md
  ```

  Confirm the report shows six changes, zero `MISMATCH` lines, an empty unresolved-AMBIGUOUS bucket and an empty skipped bucket, then re-run without `-dry-run`.

  The expected results are:
  - `docs/shared-libs/lyxcwd.md` line 82 — the identifier citations `HostLyxLink` and `HostJunctions` become `WarpLyxLink` and `WarpJunctions`, matching the symbols batch 3 renamed.
    This line is the doc mirror of the `CONSTRAINTS.md` Cwd Resolution Invariant bullet batch 7 renames, and the two must agree.
  - Each `.claude/agents/crucible-reviewer-*.md` line 16 — `This is a **host-repo** commit on the crucible worktree, never a weft-repo operation.` becomes `**warp-repo**`.
    This one is prose rather than an identifier, and "warp" is correct rather than "Fabric" because the sentence exists to contrast the warp side against the weft side.

  Do not add any other file to this invocation.
  In particular `manifest/designs/fabric-unified-view.md` is hand-edited in card 16 and must never be passed to the tool.
- **Commit:** `docs: rename retired Host* identifier citations to Warp*`

### Card 16: hand-edit the two lines that change in `fabric-unified-view.md`

- **Context:**
  - `internal/fabriccli/clone.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This file is fabric's own owner prose, so it keeps `warp` and `weft` freely.
  Exactly two lines change, and three others must be left alone.

  Line 86 — the retired identifier citations in `**\`Weft*\`/\`Host*Link\`/junction-construction methods** (\`WeftWorktree\`, \`WeftRepoRoot\`, \`HostLyxLink\`, \`HostJunctions\`, \`PortalLink\`, \`LauncherDir\`, etc.)` become `**\`Weft*\`/\`Warp*Link\`/junction-construction methods**` with `WarpLyxLink` and `WarpJunctions`.

  Line 162 — the quoted source `internal/fabriccli/clone.go reads \`hostURL := args[0]; weftURL := args[1]\`` becomes `\`warpURL := args[0]; weftURL := args[1]\``, so the quotation matches what batch 3 actually left in `internal/fabriccli/clone.go`.
  Read that file to confirm the post-sweep local variable name before editing, rather than assuming it.

  Leave these three untouched:
  - Line 88 — "`hubgeometry.BoardDir` … **hosts** more than board's own data" is the English verb.
  - Line 122 — "a fabric-sense `host`" names the retired vocabulary in order to describe the rule that polices it, exactly as the ban list in `CONSTRAINTS.md` does.
    Renaming it would make the sentence self-contradictory.
  - Line 203 — already repointed to `warp-visibility.md` by batch 4 card 12.
- **Commit:** `docs(manifest): update retired identifier citations in fabric-unified-view.md`

### Card 17: reword `docs/overview.md` and `README.md` to the Fabric vocabulary

- **Context:**
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite by hand, occurrence by occurrence, applying the vocabulary rule.
  These are the repo's two most-read consumer documents, so they are where "Fabric" earns its keep.

  In `docs/overview.md`, edit the fabric-sense occurrences at lines 7, 100, 106, 108, 115, 117, 126, 127, 147, 148, 159, 160, 178, 180, 184, 185, 186, 205 and 253.
  Sentences describing *the repo as a whole* — "keeps the host repo pristine", "the **host repo** is the project's source of truth", "host commits focused on project code" — take "Fabric".
  Sentences distinguishing the two sides — "host↔weft git-coordination", "each host worktree has a sibling weft worktree", the `<host>/_lyx → <hub>/<slug>-weft/_lyx` junction table, "Host: `<hub>/<slug>/` → Weft: `<hub>/<slug>-weft/`" — take "warp".

  Do NOT edit line 80.
  It restates the Fabric Vocabulary Invariant's ban list and owner set and is rewritten in batch 7 card 21 together with `CONSTRAINTS.md`, so the two cannot drift apart.

  Leave the verb-sense occurrences at lines 262 ("Hosts every managed process as a strand") and 302 ("hosts every managed process") exactly as they are — they describe reed, not fabric.

  In `README.md`, edit lines 50, 54, 56, 61 and 81 on the same rule.
  Line 50 ("LoomYard keeps the host repo pristine") is the composite → "Fabric";
  lines 54/56 (the `<prime>/` and `<slug>/` directory-tree annotations), 61 ("each host worktree uses a junction") and 81 ("the sole host↔weft git-coordination module") are two-sided → "warp".

  Follow this repo's markdown rule throughout: one sentence per line, with additional breaks at internal independent-clause boundaries, using plain newlines.
  Never hard-wrap at a fixed column, and never introduce a trailing-double-space or backslash line break.
- **Commit:** `docs: adopt the Fabric vocabulary in overview.md and README.md`

### Card 18: reword the sandbox and configengine docs

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/sandbox-hub.md`
  - `docs/sandbox-howto.md`
  - `docs/shared-libs/configengine.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the same per-occurrence vocabulary judgment as card 17 to `docs/sandbox-hub.md` (12 occurrences) and `docs/sandbox-howto.md` (8).
  Both describe the sandbox hub from an operator's point of view, so references to the checked-out repo as a whole become "Fabric" and references contrasting it with the weft sibling become "warp".

  `docs/shared-libs/configengine.md` has exactly two fabric-sense occurrences, at lines 93 and 94.
  Line 93's "from the host worktree" is two-sided → "warp worktree".
  Line 94's "the host `_lyx` is a directory junction into the weft worktree's `_lyx`, a single host `lyx config reconcile`" is two-sided in both halves → "warp".

  Do NOT touch `docs/shared-libs/configengine.md` line 66.
  It contains `url: https://${env:HOST:-localhost}:${env:PORT:-8080}`, a machine-sense environment-variable example, and is the reason this file is hand-edited rather than swept.

  Follow the repo's semantic-line-break markdown rule for every line rewritten.
- **Commit:** `docs: adopt the Fabric vocabulary in the sandbox and configengine docs`

### Card 19: reword the manifest documents

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/warp-visibility.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `manifest/roadmap.md` — reword the remaining fabric-sense prose.
  Batch 4 card 12 already repointed the four `host-visibility` name and link tokens at lines 44, 81, 84 and 240;
  what is left is the surrounding prose, including line 81's "invisible in host's git history", which becomes "invisible in the Fabric repo's git history" or equivalent.
  This is prose editing only — do not change any roadmap item's status, since `manifest/roadmap.md` moves for a completed or added planned item and this task is neither.

  `manifest/designs/loom.md` — edit the fabric-sense occurrences at lines 62 ("the host worktree is clean", "host branch == weft branch") and 191 ("host worktree clean"), both two-sided → "warp".
  Leave lines 131 ("reed never parses it, it just **hosts** the pane") and 198 ("a strand that `reed` … **hosts** and arranges") untouched — both are the English verb applied to reed.

  `manifest/designs/warp-visibility.md` — this file was renamed by batch 4 card 12 and its contents have not been touched yet.
  Rewrite its line 1 heading `# host-visibility — CLAUDE.local.md invisible in host's git history` to `# warp-visibility — CLAUDE.local.md invisible in the Fabric repo's git history` (or an equivalent phrasing that names the file's new slug and drops the retired word), and reword its remaining fabric-sense occurrences on the same Fabric-versus-two-sided rule.

  Follow the repo's semantic-line-break markdown rule for every line rewritten.
- **Commit:** `docs(manifest): adopt the Fabric vocabulary in roadmap, loom and warp-visibility`

### Card 20: reword the eight sandbox agent prompt templates

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  - `tools/sandbox/SANDBOX-PERCH-SUITE.md`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  These eight files are agent prompt templates shipped into the sandbox hub and read by a black-box agent, so they are **consumer-facing prose**, not owner prose: the composite is "the Fabric repo", and warp/weft appear only where the sandbox's two sides must be told apart.
  Occurrence counts per file, as a work list rather than a checksum: CORE 22, FABRIC 16, PERCH 9, BURLER 8, BUILDER 8, WEBSTER 7, REED 8, SHUTTLE 6.

  `SANDBOX-FABRIC-SUITE.md` is the one file where two-sided language dominates legitimately — it drives `lyx fabric` itself — so expect most of its occurrences to become "warp" while the other seven skew heavily toward "Fabric".

  Two structural things must survive untouched in every file:
  - Every `**Covers:** <module>[, <module>...]` line, which `cmd/lyx/sandbox_coverage_test.go` parses at module granularity against the live cobra root.
    Changing one silently breaks the Sandbox Suite Coverage invariant.
  - Any occurrence of `host` that means the operator's machine rather than the repo.
    Read each in context before changing it.

  Follow the repo's semantic-line-break markdown rule for every line rewritten.
- **Commit:** `docs(sandbox): adopt the Fabric vocabulary in the agent prompt templates`

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/lyxcwd/...` covers the two packages that machine-check documentation content.

`cmd/lyx` carries `sandbox_coverage_test.go`, which parses the `**Covers:**` lines in every `tools/sandbox/*SUITE.md` file card 20 rewrites and fails if a scenario's module tagging is disturbed — the one mechanical gate over the eight templates.
`internal/lyxcwd` carries `TestEnforcement_FabricVocabulary`, whose `internal/**/*.md` walk polices markdown for bare weft/warp tokens outside the owner set;
no file this batch edits lives under `internal/`, so the expected result is that it stays green untouched, confirming the batch did not stray into policed territory.

Nothing machine-checks the prose judgment itself, and this batch does not pretend otherwise.
The vocabulary rule's prose-doc split is explicitly a review obligation in `CONSTRAINTS.md`, not a token scan — "a token scan cannot express this distinction, so it is not covered by the enforcement test."
The real gate on cards 17 through 20 is the plan reviewer and the code reviewer reading the diff, which is why the mechanical and judgment work were separated into card 15 versus cards 16–20 in the first place.
