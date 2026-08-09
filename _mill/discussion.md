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
- `internal/lyxtest`'s exported test-fixture seam: `CopyHostHub` → `CopyWarpHub`, the `HostFixture` type, `buildHostHub`/`hostHubTemplate`/`hostHubOnce`/`hostHubPath`/`hostHubBarePath`, and every caller across the 8 packages that use them (`cmd/lyx`, `internal/buildercli`, `internal/idecli`, `internal/lyxcwd`, `internal/webstercli`, `internal/configcli`, `internal/fabricengine`, `internal/fabriccli`).
- Four file renames as `Moves:` pairs (see Technical context).
- User-visible CLI surface: `fabriccli` `Short`/`Long` strings, and `post-checkout.sh`'s stderr output.
- **All** documentation carrying fabric-sense "host": `docs/overview.md`, `docs/sandbox-hub.md`, `docs/sandbox-howto.md`, `docs/shared-libs/configengine.md`, `README.md`, `manifest/roadmap.md`, `manifest/designs/loom.md`, `manifest/designs/host-visibility.md`, and the eight `tools/sandbox/SANDBOX-*-SUITE.md` agent prompt templates.
- `CONSTRAINTS.md`'s Fabric Vocabulary Invariant — full rewrite (see Decisions).
- `internal/lyxcwd/enforcement_test.go` — tighten the guard so fabric-sense `host` fails inside the owner dirs too.
- A new reusable tool `tools/wordswap/`, committed with tests.

**Out:**

- Any behaviour change. Every diff is an identifier, a filename, a comment, a help string, or prose.
- Machine-sense and verb-sense `host` **outside** the fabric packages: `conhost` (`internal/reedengine`, `internal/shell`), `localhost`, `Hostname`, `hostURL`/`redditHostPattern`/`redditHostReplace` (`internal/prowler`), `ghost`/`ghostFile`, `hosts`, `hosting`.
  These are not the warp repo and must not be touched.
- The `host` **ban list itself**: `CONSTRAINTS.md` lines 160–161 and `enforcement_test.go`'s `hostPhrases` / `hostGeometryIdentifiers` values.
  These name the retired vocabulary in order to forbid it, so they keep the word (see Technical context — "The ban list is not a rename target").
- Renaming anything to "Merriam", or any other unrelated vocabulary work.
- `warpPath`, `warpSHA`, `WarpCommitted` and the rest of the already-correct `Warp*` family — untouched except where `hostPath` merges into `warpPath`.

## Decisions

### Rename mechanism — one generic case-preserving word swap, not an identifier table

- **Decision:** a single generic, case-preserving, whole-word `host` → `warp` substitution over the in-scope file set, applied by a script — not a hand-maintained old→new identifier table, and not per-call-site `Edit` calls.
  The substitution handles `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP` and every embedded form in one pass: `hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`, `hostclean.go`→`warpclean.go`.
- **Rationale:** the boundary-case survey (see Technical context) found the exclude list is **empty** after one comment reword, so there is nothing for a per-identifier table to protect against.
  A generic swap covers identifiers, comments, test names, string literals, shell variables, and markdown prose with one mechanism; a table covers only what someone remembered to list.
  `go build ./... && go test ./...` is the completeness proof.
- **Rejected:** `gofmt -r` per old→new pair — does not touch comments, string literals, or test-function names, so a second pass is needed anyway, and the table must be maintained.
  `gopls rename` per symbol — semantically correct but ~40 manual invocations, weak on locals in test files, and blind to prose.

### The tool is `tools/wordswap/`, general and committed

- **Decision:** build `tools/wordswap/` as a language-agnostic, case-preserving, whole-word token-substitution tool, committed with unit tests, modelled on the existing `tools/mdreflow/` and `tools/godocreflow/` house pattern.
  Interface: `go run ./tools/wordswap -from host -to warp [-dry-run] [-skip <regexp>] <path-or-glob>...`.
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
  The bare weft/warp owner-set skip is unchanged.
  The phrase predicate stays — it is what separates fabric-sense from verb-sense and machine-sense.
- **Rationale:** today the owner dirs are skipped entirely, so nothing machine-proves the rename stays done and a future card can reintroduce `hostBranch` freely.
  After this task the fabric packages contain zero "host" in any sense, so the tightened rule has no false positives to accommodate.
  Without this, the rename is a one-time cleanup rather than an enforced invariant.
- **Rejected:** leave the guard as-is — review discipline only, and this is the third pass at the same vocabulary precisely because discipline alone did not hold.
  Tighten only the identifier half — leaves prose reintroduction unguarded.

### `CONSTRAINTS.md` — full rewrite of the Fabric Vocabulary Invariant

- **Decision:** rewrite the invariant rather than appending bullets.
  The rewritten invariant states: `host` is a **retired** name, banned in the fabric sense everywhere including the owner dirs; the phrase predicate is retained as the sense-discriminator; **Fabric** is the external name for the composite; and warp/weft carry a carve-out for the few user-visible points where the two sides must be distinguished.
- **Rationale:** the retirement, the Fabric name, and the CLI carve-out are three faces of one rule.
  Expressed as appended bullets they read as accreted patches and the next reader has to reconstruct the rule.
- **Rejected:** append two bullets — minimal diff, layered rule.
  Split into two invariants — cleaner separation but neither is comprehensible alone.

### Commit granularity — four commits

- **Decision:**
  - **(a)** Reword `coalesce.go:1`; build `tools/wordswap/` with tests; run it over the Go file set (fabric packages, `internal/lyxtest`, and all callers). Identifiers, comments, test names, string literals.
  - **(b)** File renames as `Moves:` pairs.
  - **(c)** Non-Go surfaces: `post-checkout.sh`, `fabriccli` `Short`/`Long` help strings.
  - **(d)** Documentation sweep, `CONSTRAINTS.md` rewrite, and the `enforcement_test.go` tightening.
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
- Verb-sense: exactly one — `internal/fabricengine/coalesce.go:1`, handled by reword.
- `DeriveHostName(hostURL)` looks machine-sense but is not: it extracts the **warp repository basename** from a raw URL or file path (`clone.go:355–371`). It renames to `DeriveWarpName(warpURL)`.
- Therefore the `wordswap` run over the fabric packages takes **no `-skip` argument**.

### The ban list is not a rename target

Two places name the retired vocabulary *in order to forbid it* and must keep the word `host`:

- `CONSTRAINTS.md:160–161` — the phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`) and the identifier list (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`), plus the sentence explaining that the bare word passes untouched.
- `internal/lyxcwd/enforcement_test.go` — the `hostPhrases` slice (line ~633), the `hostGeometryIdentifiers` map (line ~622), the `fabricSenseHostPhrase` predicate and its sub-tests (`host_repo_phrase_fails`, `hostBranch_identifier_fails`, `host_verb_sense_passes`, `host_machine_sense_passes`, `write_host_cmdlet_passes`).

**Neither file may be passed to `wordswap`.** Both are hand-edited in commit (d).
The rest of `CONSTRAINTS.md` — lines 175, 180, 188, 200, 214 in the Fabric Git Invariant and elsewhere — does rename.

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

The eight `tools/sandbox/SANDBOX-*-SUITE.md` files are **agent prompt templates** shipped into the sandbox hub and read by a black-box agent, so they are consumer-facing prose: the composite is "the Fabric repo"; warp/weft appear only where the sandbox's two sides must be distinguished.

Applying the vocabulary rule per file: `manifest/designs/fabric-unified-view.md` and the `fabricengine`/`fabriccli` package docs are **owner prose** and keep warp/weft freely.
Everything else is consumer prose — "Fabric" for the composite, warp/weft only for genuine two-sided distinctions.

### Existing guard mechanics

`internal/lyxcwd/enforcement_test.go` currently holds:

- `fabricVocabularyOwners` (line ~596): `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine`, `internal/configsync`.
- `shouldSkipBareVocabularyCheck(dir)` (line ~665) — skips every owner row except `configsyncOwnerDir`.
- `failsBareVocabularyCheck(dir, bareIdent, bareLiteralOrComment)` (line ~675) — `configsync`'s narrower literal-and-comment carve-out.
- `fabricVocabularyHits(f *ast.File)` (line ~688) — returns `(bareIdent, bareLiteralOrComment, hostHit)`; the host half currently shares the same owner skip as the bare-token half (per "card 26").

The tightening splits that shared skip: the bare weft/warp half keeps the owner skip; the host half no longer applies it.
`CONSTRAINTS.md` states the owner set includes `tools/` and `sandbox/` — reconcile the doc against the actual map, which does not list them, while rewriting the invariant.

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
- **Cwd Resolution Invariant** — unaffected; no path, anchor, or cwd-resolution change. `internal/lyxcwd` is touched only in its enforcement test and in `anchor_test.go`'s `CopyHostHub` call sites.
- **lyxtest Leaf Invariant** — unaffected; the rename adds no imports to `internal/lyxtest`.
- **CLI/Cobra Invariant** — `Short` must remain present on every command; the help-tree tests must stay green after the `Short`/`Long` rewording.
- **Documentation Lifecycle** — this task changes observable CLI text and cross-cutting infrastructure, so `docs/overview.md`, the module docs, and `CONSTRAINTS.md` land in the same commits. `manifest/roadmap.md` moves only for a completed or added planned item; here it is edited for prose and the `host-visibility.md` link, not for a roadmap status change.

From this repo's `CLAUDE.md`:

- **Markdown semantic line breaks** — every `.md` file this task rewrites must keep one sentence per line with breaks at internal independent-clause boundaries. `wordswap` is a word-level substitution and preserves line structure, so it does not disturb this; the hand-written prose in commit (d) must follow it.
- **Worktree isolation** — all work stays in `<container>/wts/fabric-host-to-warp-rename`; no push to `main` from here.

Discovered during discussion:

- `wordswap` must never be run over `CONSTRAINTS.md` or `internal/lyxcwd/enforcement_test.go`.
- The reword of `coalesce.go:1` must precede the `wordswap` run.

## Testing

**`tools/wordswap/` — TDD candidate, the one genuinely new code in this task.**
Write the tests first; this tool mutates ~300 files, so its correctness is the task's main risk.
Scenarios to cover:

- Case-preserving substitution: `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP`; embedded forms `hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`.
- Whole-word/token boundary: `ghost`, `hostname`, `localhost`, `conhost` must **not** match, while `hostBranch` and `HostJunction` must.
  This is the subtle one — the boundary is camel/snake-token-aware, not merely `\b`-anchored, since `hostBranch` has no word boundary after `host`.
  Pin the exact intended rule with tests before implementing.
- Mixed forms in one line, and multiple occurrences on one line.
- `-skip <regexp>`: matching occurrences are left untouched and reported; non-matching ones still swap.
- **Reversibility invariant**: including the critical case where the target word already occurs in the input (mirroring `warp`'s 576 pre-existing occurrences), plus a deliberately-broken transform that must be caught and leave the file untouched.
- `-dry-run` writes nothing.
- Non-Go input: a shell fragment and a markdown fragment, proving language-agnosticism.

**The rename itself — verification is mechanical, not new tests.**
`go build ./... && go test ./...` green after each of the four commits.
A clean compile is the actual proof that no call site was missed and no identifier collided; it is cheap enough to run per commit rather than only at the end.

**Existing tests that must stay green and will need name updates:** the ~16 renamed `Test*` functions listed in Technical context are renamed by `wordswap` in commit (a), so they must still be discovered and pass.
`cmd/lyx/helptree_test.go`, `longlist_test.go`, and `jsonhelp_test.go` must stay green after the CLI help rewording in commit (c) — verified to contain no `host` assertions, so this should be a no-op, but it is the CLI/Cobra Invariant's check and must be confirmed rather than assumed.

**`TestEnforcement_FabricVocabulary` — the tightening needs its own new cases.**
After removing the host half's owner skip, add cases proving:

- A fabric-sense host phrase in an owner dir now **fails** (it previously passed).
- The bare weft/warp owner skip is **unchanged** — an owner dir may still say warp/weft freely.
- `configsync`'s narrower literal-and-comment carve-out is unaffected.
- The existing sense-discrimination sub-tests still pass: `host_verb_sense_passes`, `host_machine_sense_passes`, `write_host_cmdlet_passes`.

**Repo-wide completeness check, run manually at the end of commit (d):** grep the fabric packages for any remaining `host` in any case.
Expect zero hits.
This is the assertion the tightened enforcement test now encodes permanently.

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
