# Discussion: fabric: close the weft-visibility leak (slice 8)

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
slug: fabric-weft-visibility-cleanup
status: discussing
parent: main
```

## Problem

Fabric exists to sell one illusion: a developer, an agent, and every lyx module see **one repository, called fabric**.
Under the hood that repo is two git checkouts — the warp (the host/code repo a human uses normally) and the weft (the sibling `-weft` worktree holding durable `_lyx` state) — but nothing outside fabric's own packages is supposed to know that.
`_lyx` is an ordinary directory in the fabric repo.
That is the whole contract.

Today the illusion leaks in every direction, and the leak is not confined to path lookups:

- **The constructor** — `fabricengine.New(warpPath, weftPath)` forces a caller to *supply* a weft path.
- **The return type** — `CommitResult.WeftCommitted` narrates the two-repo split back to the caller.
- **A reason string that crosses the boundary** — `Healthy` returns `"host on %s, weft on %s (want %s)"` (`drift.go:58`), which `loomengine` both substring-matches to classify a failure and prints in loom's report.
- **Consumer-owned interface names** — `builderengine.WarpResetter`, `websterengine.WarpBisector`.
- **Exported constants whose values reach user output** — `loomengine.CheckWeftPairing` = `"weft-pairing"`, `CheckWeftSync` = `"weft-sync"`, `websterengine.ClassWeftReference` = `"weft-reference"`.
- **A JSON envelope key** — `"weftCommitted"` in three CLI verbs.
- **Operator-facing CLI strings and help text** — `configcli`'s `"…but weft sync failed"` and its cobra `Long`.
- **The agent prompt templates themselves** — `websterengine/master-template.md` actively *teaches* every Builder and Webster agent that `_lyx` is a link into a separate weft worktree with a `-weft` sibling name, then forbids them from touching it.
- **~380 comment mentions across 55 production files.**

**Why now:** slice 7 of [manifest/designs/fabric-unified-view.md](../manifest/designs/fabric-unified-view.md) has landed.
It moved the weft-sibling and junction path surface out of `internal/hubgeometry`/`internal/lyxcwd` into `internal/fabricengine`, which is what made the remaining leak both visible and fixable — callers no longer reach into a shared geometry module for a weft path, they reach into fabric's own exported API for it.
Slice 8 is the follow-through: make fabric's public surface incapable of telling anyone that warp and weft exist.
It is file-disjoint from slices 9 and 10 and can run in parallel with either.

The task brief describes the leak as "call sites that read Layout `Weft*`/`Host*Link` methods directly".
That wording predates slice 7 and is now inaccurate: no call site reads `lyxcwd.Location` for a weft path any more.
The leak that remains is fabric's own exported API, and everything downstream of it.

## Scope

This task is a **cleanup**.
Nothing is excluded on scope grounds.
The Out list below contains only items excluded by a structural invariant, a physical fact about an external resource, or because they are a different task entirely.

**In:**

- **API** — `Open(l)` replaces `New(warpPath, weftPath)` as the sole constructor outside fabric;
  `Ready(l)` replaces the weft `os.Stat` in loom preflight;
  `NewRefScanner(l)`/`RefScanner.Matches` replaces webster's weft-reference regex;
  `CommitResult.Committed()` replaces `WeftCommitted` reads;
  `Healthy` gains a typed reason;
  `Fabric.Warp`/`Fabric.Weft`, `PartialCommitError`'s fields, and `New` all go private.
- **Identifiers and string literals** — every production `weft`/`warp` token outside the owner set, including `loomengine`'s `CheckID` values, `websterengine`'s violation-class value, `configcli`'s error strings and cobra `Long`, and the `"weftCommitted"` JSON key.
- **Comments** — every production `weft`/`warp` comment mention outside the owner set (~380 across 55 files).
- **Agent prompt templates** — the five `go:embed`-ed `.md` templates are rewritten to describe one repo.
- **Test files** — hand-cleaned of vocabulary that is not a reference to owner-package API, including `cmd/lyx/boardguard_test.go` (which calls the invariant "Weft Git Invariant" where `CONSTRAINTS.md` says "Fabric Git Invariant (warp + weft)") and `cmd/lyx/rawgitmutation_test.go:10,45` (which names `WarpBisector`/`WarpResetter` in comments this task renames).
  **Carve-out:** the retained env-var names `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` stay verbatim wherever they appear, including in `webstercli`, `buildercli`, `perchcli`, and `configcli` tests — they are the literal names of variables this task deliberately does not rename (see Out), so a test that sets one must spell it correctly.
- **Enforcement** — new `TestEnforcement_FabricVocabulary`, covering production `.go` files and the embedded templates.
- **Docs** — `internal/fabricengine`'s package doc, `CONSTRAINTS.md`, `docs/overview.md`, slice 8's section in `manifest/designs/fabric-unified-view.md`, plus the repo-prose docs named in `doc-vocabulary-split`.

**Out, and why each is structural rather than scope:**

- `internal/lyxtest` keeps the vocabulary.
  The lyxtest Leaf Invariant *bans* it from importing `fabricengine`;
  it imports `internal/weftname` and builds real paired worktrees, so `WeftPrime`/`WeftBare`/`WeftPath`/`CopyWeft` are honest names for what it constructs.
  It is a vocabulary owner, not an unfixed leak.
- The two GitHub URLs in `tools/sandbox/main.go:28,36` (`lyx-test-weft`, `lyx-fabric-test-weft`) and the identifiers naming them (`weftURL`, `fabricWeftURL`).
  Those repos exist on GitHub;
  the strings are the address of an external resource, and the identifiers must match what they point at.
  Renaming them in Go alone breaks every sandbox run.
- `WEFT_SKIP_GIT` / `WEFT_SKIP_PUSH`.
  Read only by `internal/fabricengine` (`fabric.go:100-104`, `spawn.go:33`) — an owner — so the vocabulary rule permits them.
  They configure fabric's own git behaviour, no consumer module reads them, and renaming breaks every existing CI invocation for zero visibility gain.
- Slices 9 and 10, and the still-open orchestration half of slice 6 — separate tasks with their own discussion phases.

## Decisions

### one-constructor-open

- Decision: `fabricengine.New(warpPath, weftPath string) (*Fabric, error)` becomes private `newPaired`.
  A new `func Open(l *lyxcwd.Location) (*Fabric, error)` — implemented as `newPaired(l.WorktreePath(), weftWorktree(l))` — is the only constructor any other package calls.
  `WeftWorktree` becomes private `weftWorktree`, except that it stays exported for `internal/fabriccli`'s two remaining uses (`weft_verbs.go:244`'s `spawnPush`, and its own `Open` migration).
- Rationale: `New` does almost nothing — two `os.Stat` calls via `requireDir`, then two `gitrepo.New(path)` calls, and `gitrepo.New` is `return &Repo{path: path}`.
  No connection, no lock, no cached state, no setup.
  The only thing the ceremony accomplished was forcing every caller to name a weft path.
  `Open(l)` also matches the house style already used by `Clean(l)`, `Healthy(l)`, `Status(l)`, `Reconcile(l)`, `Checkout(l)`.
- Rejected: a package-level `CommitScoped(l, dirs, msg, opts)` removing the handle from the three CLI sites — unnecessary, because a handle named `fabric` with a `Commit` method *is* the one-repo illusion;
  and it would not serve the two engine sites, which need a handle satisfying an interface, not a commit.
- Rejected: a separate `OpenWarp(l)` for the two engine sites that drive only warp verbs — the name says "warp", which is exactly what the caller must not see.

### export-test-shim

- Decision: add `internal/fabricengine/export_test.go` (in `package fabricengine`) re-exporting `newPaired` and the two now-private fields for the four `package fabricengine_test` files that need them.
  Separately, `fabric_test.go`'s missing-path contract is restated through `Open(l)`, since that is the constructor the contract now belongs to.
- Rationale: unexporting `New`, `Fabric.Warp`, and `Fabric.Weft` is **not** a purely in-package change — four files under `internal/fabricengine/` are `package fabricengine_test` and call them from outside: `fabric_test.go` (7 uses, including the `:25,44` missing-path contract tests), `weftgit_exclude_test.go` (4), `warpforward_integration_test.go` (4), `checkout_index_refresh_test.go` (2).
  The `export_test.go` shim is the standard Go idiom for exactly this, and it relocates no test file.
- Rejected: converting all four to `package fabricengine` — gives up external-package testing for files that currently prove the API works from outside, which is the more valuable signal for a task about API surface.
- Rejected: restating all four through `Open` alone — `warpforward_integration_test.go` and `weftgit_exclude_test.go` build fixtures from raw scratch paths that no `lyxcwd.Location` describes.

### open-does-not-wire

- Decision: `Open` performs no setup.
  It derives both paths, stat-validates them, and returns the handle.
  Creating the dual worktree and wiring junctions stays where it is: `Topology.Add`/`Checkout`/`Reconcile`/`Remove`/`Prune`/`Cleanup`, driving `createWeftWorktree` (`weftwiring.go:101`), `WireJunctions`/`seedLyxJunction`/`wireBoardLink`/`seedGitExclude` (`junction.go:91-237`), and the six `git worktree add` call sites (`add.go:134,157`, `weftwiring.go:108`, `reconcile.go:226`, `boardweft.go:33,46`).
- Rationale: folding wiring into `Open` would make every `builder run` and `webster begin-batch` silently repair geometry as a side effect of asking for a handle, and would let the restart-chain reset and the bisect path rewrite geometry mid-run.
  `Reconcile` exists to be that verb, called deliberately.

### open-stats-both-sides

- Decision: `Open(l)` stat-validates both checkouts, so `builderengine`'s restart-chain reset and `websterengine`'s bisect (which drive warp-only verbs) still require the sibling to exist on disk.
- Rationale: from outside, fabric is one repo;
  "the repo is present" legitimately means both of its checkouts are present.
  Preserving today's behaviour keeps the change purely about visibility, with no behavioural delta to reason about in review.

### commit-result-committed-method

- Decision: add `func (r CommitResult) Committed() bool { return r.WarpCommitted || r.WeftCommitted }`.
  The three CLI sites become `committed = res.Committed()`.
  The four raw fields stay exported for `internal/fabriccli`, and `TestEnforcement_FabricVocabulary` bans reading them outside the owner set.
- Rationale: there is only one `Commit` verb.
  `CommitResult` is its return type, and it carries four fields because one call can land two real git commits — `Commit` classifies the caller's file list against the repo-wide pathspec and routes each path to the correct side itself (`commit.go:96`).
  That classification is the illusion working as designed;
  the defect is only that the result narrates the split back.
  `fabriccli/weft_verbs.go:160` prints `res.WeftCommitted` and `res.WeftSHA` on purpose — `lyx fabric weft …` exists to show an operator the weft side — and `fabriccli` is a separate Go package, so the fields cannot simply be unexported.
- Rejected: splitting into `Commit(...) (bool, error)` plus a `CommitDetailed(...)` for fabric's own CLI — Go cannot enforce "fabric-only" on either, so it needs the identical enforcement test *and* costs a second method.
- Rejected: collapsing to `CommitResult{SHA, Committed}` — fabric's own weft verbs lose the weft SHA they print by design.

### partial-commit-error-fields-private

- Decision: `PartialCommitError`'s `WarpSHA`, `WeftSHA`, `WeftCommitted` fields become private.
  Its `Error()` string is unchanged.
- Rationale: verified zero readers outside `internal/fabricengine`.
  The type is still returned to external callers and still matched with `errors.As`;
  only the field access goes away, and nobody used it.

### ready-not-paired

- Decision: `loomengine/preflight.go:105`'s `os.Stat(fabricengine.WeftWorktree(l))` becomes `fabricengine.Ready(l) (bool, error)`.
  Contract: `(false, nil)` when the sibling worktree is absent;
  `(true, nil)` when present;
  `(false, err)` for any other stat failure (permissions, I/O).
  preflight keeps today's behaviour exactly: absence records the check failure and sets `check3BlocksSeed`, any other error hard-returns `Report{}, err` as `preflight.go:106-108` does today.
- Rationale: "paired" announces that there are two repos, which is the exact leak.
  `Ready` answers loomengine's real question — "is fabric usable in this worktree" — without naming a side.
  Splitting absence from failure at the API boundary is what lets preflight keep its classification unchanged;
  collapsing both into `(false, nil)` would turn a permissions fault into a silent "fabric not ready", a behavioural change disguised as a cleanup.
- Rejected: folding pairing into `Healthy(l)` as a new reason — preflight would have to string-match to tell absence from a sync failure, which is the very fragility `healthy-typed-reason` removes.
- Rejected: `Wired(l)` — overlaps the junction-health concept `Healthy` owns, and preflight distinguishes the two.

### healthy-typed-reason

- Decision: `Healthy(l) (ok bool, reason string, err error)` becomes `Healthy(l) (ok bool, reason HealthReason, err error)`, where `HealthReason` is a small struct or enum-plus-detail carrying a typed cause plus an already-fabric-worded display string.
  `loomengine/preflight.go:117-141` switches on the typed cause instead of `strings.HasPrefix(reason, "host on ")` / `strings.Contains(reason, "junction")`.
  There are **five** causes, not three — one per reason shape `drift.go` returns:

  | cause | source | today's `CheckID` |
  |---|---|---|
  | branch mismatch | `drift.go:58` `"host on %s, weft on %s (want %s)"` | `CheckWeftSync` |
  | config load failed | `:69` `"host junction check unavailable: cannot load fabric.yaml: %v"` | `CheckJunction` |
  | junction missing | `:83` `"host %s junction missing"` | `CheckJunction` |
  | not a junction | `:94` `"host %s is not a junction"` | `CheckJunction` |
  | junction points elsewhere | `:110` `"host %s junction points elsewhere"` | `CheckJunction` |

  The four non-branch shapes classify as `CheckJunction` + `check3BlocksSeed` today only because each happens to contain the substring `"junction"`.
  The equivalence test covers all five individually.
  The config-load failure stays a **cause**, not a promoted `error` return, even though it is an error surfaced as a reason: promoting it would change preflight's outcome for that case from "check failed" to "preflight aborted".
  Recorded here as a latent oddity for a later task, deliberately not fixed in a cleanup.
- Rationale: this is a vocabulary leak *and* a fragility, and one change fixes both.
  `drift.go:58` returns `"host on %s, weft on %s (want %s)"` — loomengine substring-matches it to pick a `CheckID` *and* prints it in loom's report, so the word crosses the boundary into operator output.
  `preflight.go:117-130`'s own comment already documents that any future reword of those reasons silently reverts the classification to `CheckWeftSync`;
  rewording them for vocabulary would trip exactly that trap.
  Typing the reason makes the reword safe and removes the trap permanently.
- Note for the plan: this is the one behavioural-surface change in the task.
  The classification outcomes must stay identical — same `CheckID` for the same underlying condition — and that equivalence needs a test.

### refscanner-owns-both-halves

- Decision: `websterengine`'s `weftReferencePattern(layout) *regexp.Regexp` is replaced by `fabricengine.NewRefScanner(l) *RefScanner`, with a `Matches(cmd string) bool` method.
  `RefScanner` absorbs **both** halves of today's regex: the path half (the weft worktree path, `weftname.Suffix`) and the command-spelling half (`lyx(?:\.exe)?\s+(fabric|weft|warp)\b`).
  `audit.go` drops its `internal/weftname` import.
  `CheckFork` and `CheckParent` take `*fabricengine.RefScanner` where they took `weftRef *regexp.Regexp`;
  construction stays at `runlevel.go:706` and `recordbatch.go:125`, still once per audit.
- Rationale: `audit.go` is the one site that genuinely needs the weft path as a *string* and has no operation return value to route through, so the design doc's claim that "none of them need `WeftWorktree()` once Fabric's return values are complete" is false for this one.
  The scanner is how it stops needing the string.
  Moving only the path half was considered and rejected once the vocabulary rule was settled: `websterengine` may not contain the words at all, so the spelling alternation has to move too.
  Webster asks a question;
  fabric owns every word in the answer.
- Rejected: `fabricengine.WeftPathPattern(l) string` returning a quoted regex fragment — still hands out the path, and `Pattern` collides with `internal/patternengine`.
- Rejected: `ReferencesFabric(cmd string, l)` as a plain func — recompiles the regex per command;
  today it is built once and reused across every Bash command in a transcript.
- Consequence: fabric now owns webster's audit policy for command spellings.
  Accepted — the policy is about which fabric-driving commands are forbidden, and fabric is the module that knows what those are.

### templates-describe-one-repo

- Decision: the five `go:embed`-ed prompt templates are rewritten so that an agent is never told warp or weft exist.
  `_lyx` is presented as an ordinary directory in the fabric repo, with no mention of links, siblings, `-weft` names, or separate worktrees.
  Concretely: `master-template.md:29`'s "`_lyx` is a link into a separate weft worktree (a sibling directory whose name ends in `-weft`): NEVER reference that physical weft path…" collapses to a **positive** rule that names no geometry — "`_lyx` holds plan and state files;
  read and write them as ordinary files through `_lyx/...` paths.
  You never run git against `_lyx`;
  it is committed for you." — and `:20`'s "you never run git against the weft" folds into the same rule plus a ban on driving fabric's git (`lyx fabric`) at all;
  `:136`'s "a weft-reference" and `:140-142`'s "A weft-sync error" / "weft sync failed" follow the renamed violation class and the renamed CLI error strings.
  `websterengine/template_test.go:246,257,318` and `builderengine`/`burlerengine` template tests update with them.
- Rationale: this is the largest leak in the system and the one that most directly contradicts the illusion — the templates do not merely mention weft, they *teach* every Builder and Webster agent that it exists, what it is called, and where it sits, and then forbid touching it.
  An agent that is never told cannot reference what it does not know;
  the `RefScanner` still catches a reference discovered some other way, so the enforcement does not weaken.
  The instruction also gets stronger, not vaguer: "only ever use `_lyx/...` paths" is a rule an agent can follow without understanding the geometry, where "never reference the physical path of the sibling" required understanding it first.
- Rejected: leaving templates alone as deliberately weft-aware — considered and overruled;
  it would also desync `master-template.md:136`'s violation wording from the renamed class value the code emits.
- Rejected: a "never reference a path outside your worktree root" ban.
  An agent has `_lyx` in its own worktree and no motive to look one level up — the current template manufactures the curiosity it then forbids.
  The discovery path that actually matters is not `ls ..` but **`_lyx` itself being the link**: `realpath _lyx`, `readlink`, `find -L`, or `git -C _lyx status` yields the sibling path from inside the agent's own worktree.
  The positive rule above forbids precisely that class of command without hinting that `_lyx` resolves to anything.

### scanner-keeps-hard-fail

- Decision: `RefScanner` keeps the run-failing penalty on **both** halves — the `lyx fabric`/`lyx weft`/`lyx warp` spellings and the sibling-path/`-weft`-suffix half — even though the rewritten templates no longer warn about the second.
- Rationale: the penalty is a backstop for a discovery path no instruction anticipated, and the Fabric Git Invariant's agent half depends on a deliberate `cd ../<slug>-weft && git commit` being caught, not merely noted.
  The asymmetry is real and accepted: an agent that stumbles onto the physical path can fail a run on a rule it was never given.
  The mitigation is `templates-describe-one-repo`'s positive `_lyx` rule, which removes every ordinary reason to run such a command.
- Rejected: downgrading the path half to a `ForkWarnings` warning — proportionate to whether the agent was told, but a deliberate weft commit would then only warn.
- Rejected: adding a template line naming the penalty without the geometry ("any command that resolves or reaches outside `_lyx/...` for state fails the run") — makes the penalty foreseeable, but hints that `_lyx` resolves to something.

### doc-vocabulary-split

- Decision: repo-level prose docs split by **what the doc describes**.
  A doc explaining *fabric's own mechanism* keeps the vocabulary;
  a doc describing *a consumer module's behaviour* rewords, because that module does not know weft exists.
  - **Keep:** `README.md:50,55,57,58,61,81` (the architecture section — a maintainer is entitled to learn the two-repo design from the README), `CONSTRAINTS.md`'s Fabric Git Invariant, `manifest/designs/fabric-unified-view.md`.
  - **Reword:** `README.md:62` ("`_lyx/` is durable and weft-synced" — that is state semantics a consumer sees), `docs/skills.md:14,167,184`, `docs/reference/builder-contract.md:22,24` ("Performs the loop's exit-time backstop weft commit"), `docs/benchmarks/test-suite-timing.md`.
- Rationale: the rule is about what modules and agents see, not about hiding the design from a human reading the repo.
  `builder-contract.md` is builder's *contract*, and builder does not know weft exists — its contract must not say so.
- The machine check does not cover these files: it walks `internal/` and `cmd/` plus embedded templates, because a repo-prose ban would have to encode this mechanism-vs-behaviour distinction, which no token scan can express.
  Prose-doc discipline is a review obligation, recorded in `CONSTRAINTS.md`.
- Rejected: rewording all repo prose including README's architecture section — the repo would then document the two-repo design nowhere outside `CONSTRAINTS.md` and the design doc.
- Rejected: leaving all repo prose alone — leaves the leak in the most operator-facing surface there is.

### comment-fidelity

- Decision: each non-owner comment is classified before rewording.
  - **Sync semantics** — mechanical substitution ("the durable, weft-synced `_lyx`" → "the durable, fabric-synced `_lyx`").
    This is the large majority.
  - **Two-repo mechanics** — case by case, because substituting "fabric" would erase *which* checkout physically holds something.
    Named cases: `lyxcwd/anchor.go:2,4,32,39`, `gitrepo/doc.go:132,252`, `configengine/config.go:5`.
- Decision for `anchor.go` specifically: reword "the weft:main root" to "the board root".
  This is lossless — the marker lives at `<boardDir(hub)>/.lyx-anchor`, `_board` is a geometry token `lyxcwd` already co-owns (`geometryTokenOwners`), and "the board root" is *more* useful than "the weft:main root" to a reader who has to find the file.
- Rationale: my earlier claim that these comments "reword cleanly" was wrong for this class;
  the `_lyx` sync-semantics example does not generalise to a comment documenting where a marker physically sits.
- Rejected: relocating the mechanics detail into `fabricengine`'s package doc and dropping it from `lyxcwd` — cleaner ownership, but a reader of `anchor.go` loses the pointer to where the write side lives.

### never-told-has-a-bound

- Decision: record explicitly that the "an agent is never told weft exists" property holds **only until the first sync failure**.
  `diagnostics-say-fabric-detail-says-weft` keeps fabric's weft-naming detail in the wrapped `%v`, and `master-template.md:143` instructs Master to quote a sync failure verbatim into `stuck_reason` — so on the first failure an agent learns the word and writes it into `_lyx`.
- Rationale: the alternative is scoping what detail fabric exposes through the consumer envelope, which would cost the operator the diagnostic that motivated keeping it.
  The bound is acceptable because it is reached only on a failure path, and an agent that has already failed its run is not going on to make decisions from the leaked word.
  Recorded so a future reader does not mistake the property for absolute.

### fabric-vocabulary-rule

- Decision: in production code, the tokens `weft` and `warp` may appear only in the **owner set**.
  Everywhere else — identifiers, string literals, comments, and embedded prompt templates — uses fabric vocabulary.
  Owner set:
  - `internal/fabricengine` — implements the illusion.
  - `internal/fabriccli` — fabric's own CLI;
    `lyx fabric weft …` exposes the weft to an operator deliberately.
  - `internal/weftname` — the `-weft` suffix leaf.
  - `internal/lyxtest` — the test-fixture leaf that builds real paired worktrees.
  - `internal/boardengine` — the Fabric Git Invariant's existing board carve-out;
    board lives at `weft:main` and its whole branch-naming design is defined by the `-weft` suffix (`board.go:14-30`).
  - `internal/configsync` — **string literals only**, for `legacyFabricConfigModules = []string{"warp", "weft"}` (`configsync.go:24`).
  - `tools/` and `sandbox/` — the black-box harness that drives `lyx fabric clone <hostURL> <weftURL>`, fabric's own deliberately two-sided CLI, and that names the real `lyx-test-weft`/`lyx-fabric-test-weft` GitHub repos.
- Rationale: this is the rule the task title names.
  The `configsync` exception is forced: those strings are on-disk legacy config filenames (`warp.yaml`, `weft.yaml`) the migration must read by name.
  The `tools/`/`sandbox/` exception is likewise forced: `weftURL = "https://github.com/Knatte18/lyx-test-weft"` names an external resource, and the identifier must match what it points at.
  The `boardengine` exception is pre-existing policy recorded in `CONSTRAINTS.md`.
- Rejected: banning identifiers and string literals but leaving comments and templates alone.
  Explicitly overruled — a doc comment or a prompt template that narrates the weft leaks the model at least as effectively as a symbol does.

### diagnostics-say-fabric-detail-says-weft

- Decision: consumer-emitted prose says "fabric".
  `buildercli` emits `"builder: run finished (%s) but the fabric sync failed: %v"`;
  `webstercli`, `perchcli`, and `configcli` likewise.
  The wrapped `%v` is `fabricengine`'s own error and continues to name the weft repo and path freely.
- Rationale: an operator debugging a sync failure still gets the full weft-level detail — it arrives from the module that owns the word.
  This resolves what would otherwise be a contradiction between "errors may explain that something happened in weft" and "these modules must not reference weft".
- Rejected: leaving `"the weft sync failed"` in consumer packages as a diagnostic exemption — a permanent hole in the rule at exactly the place operators read.

### json-key-renamed

- Decision: the CLI envelope key `"weftCommitted"` becomes `"fabricCommitted"` in `buildercli/run.go:98`, `webstercli/run.go:105`, `perchcli/run.go:378`.
  `tools/sandbox/SANDBOX-PERCH-SUITE.md:119` and `buildercli/run_test.go:150` update in the same commit.
- Rationale: verified those are the only two consumers.
  A machine-readable contract is the worst place to leave the leak — it is the surface orchestrator skills read.

### consumer-renames

- Decision: rename, outside the owner set:
  - `builderengine.WarpResetter` → `FabricResetter`;
    `websterengine.WarpBisector` → `FabricBisector` (with doc comments).
  - `loomengine.CheckWeftPairing` → `CheckFabricReady`, value `"weft-pairing"` → `"fabric-ready"`;
    `CheckWeftSync` → `CheckFabricSync`, value `"weft-sync"` → `"fabric-sync"` (`report.go:23-26`);
    `preflight.go:109`'s reason string `"weft not paired"` → `"fabric not ready"`.
  - `websterengine.ClassWeftReference` → `ClassFabricReference`, value `"weft-reference"` → `"fabric-reference"`;
    its violation `Detail` prose reworded (`audit.go:134`, `:203`).
  - `configcli`: `configcli.go:125,181`'s `"edited _lyx/config/%s.yaml but weft sync failed: %s"` → `"…but fabric sync failed: %s"`;
    `:233`'s cobra `Long` `"…and syncs weft on\n"` → `"…and syncs fabric on\n"`;
    `configcli_test.go:187`'s assertion updates.
  - `internal/buildercli/weft.go` → `sync.go`;
    `internal/webstercli/weft.go` → `sync.go`;
    helper `weftCommit` → `fabricSync` in both.
  - locals: `weftErr` → `syncErr`;
    `weftWorktree` deleted (`Open` removes the need).
- Rationale: "Fabric is the name of the repo they see", applied literally.
  Pinning the `CheckID` and violation-class *values*, not just the identifiers, matters because both are emitted into operator-facing output.
  The `configcli` `Long` change is recorded against the CLI/Cobra Invariant's help-accuracy obligation.
- Rejected: `RepoResetter`/`RepoBisector` — provider-neutral, but `builderengine.Resetter` operating on "the repo" is vaguer than naming the thing.
- Plan obligation: grep for consumers of the old `CheckID` and violation-class *values* before landing — they are strings, so the compiler will not find them.

### enforcement-test

- Decision: new `TestEnforcement_FabricVocabulary` in `internal/lyxcwd/enforcement_test.go`.
  It fails any file outside the owner set containing the token `weft` or `warp` — in identifiers, string literals, **or** comments — and any file outside `{fabricengine, fabriccli, lyxtest}` importing `internal/weftname`.
  Coverage: production `.go` files under `internal/` and `cmd/`, plus every `//go:embed`-ed `.md` prompt template under `internal/`.
  `*_test.go` files are outside the machine check.
  Owners are expressed as a map in the same idiom as `geometryTokenOwners`, with `configsync` documented as string-literal-only.
  The three walks (`TestEnforcement`, `TestEnforcement_GeometryLiterals`, and this one) get a single extracted `filepath.WalkDir` helper as part of the work.
- Rationale: one rule, no per-symbol allowlist to keep in sync as fabric's API evolves.
  Including comments is a deliberate divergence from the sibling `stripGoComments` guard in the same file: that guard polices *usage* of `os.Getwd`, where prose mentioning it is harmless, whereas here the prose is itself the leak.
  Including the embedded templates is what stops `templates-describe-one-repo` from rotting back, since nothing else checks them.
  Scoping the walk to `internal/` and `cmd/` is what keeps `tools/`'s external-resource URLs out of the check without a per-file exception;
  it is also why repo-prose docs are not covered — see `doc-vocabulary-split` for why no token scan can express that rule.
- `*_test.go` exclusion is technical, not scope: any test needing a real paired fixture must reference `internal/lyxtest`'s owner-defined API (`WeftPrime`, `CopyWeft`, …), which a token scan cannot distinguish from a leak.
  Test files are hand-cleaned in this task for everything that is not such a reference;
  keeping them out of the machine check also matches `TestEnforcement_GeometryLiterals`'s existing stance.
- Rejected: a symbol-and-import allowlist — needs updating every time fabric gains a symbol, and silently permits any new one.
- Rejected: extending `TestEnforcement_GeometryLiterals` itself — its predicate matches whole string literals in path-construction context and cannot express selector references, comment text, or `.md` files.

### documentation

- Decision, all in the same commit as the code:
  - `internal/fabricengine`'s package doc absorbs the durable contract: `Open` is the only constructor outside the package, `Committed()` the only result a consumer reads, `RefScanner` how a consumer asks about fabric-driving commands, `Healthy`'s typed reason, and the vocabulary rule with its owner set.
  - `CONSTRAINTS.md` — the Cwd Resolution Invariant's fabric bullet (`:25-26`) widens to state the vocabulary rule and names `TestEnforcement_FabricVocabulary` under **Enforced by**.
    The Fabric Git Invariant's heading and body keep `warp`/`weft` (that invariant is *about* the two-repo mechanism), but its **Enforced by** bullet (`:148-151`) is corrected: it currently names `websterengine`'s `weftReferencePattern` as the machine check for the agent half, a symbol this task deletes — it becomes `fabricengine.RefScanner`.
    Its "agent prompt templates never instruct a weft git op" clause is restated to match `templates-describe-one-repo`: templates never *mention* the two-repo structure at all, which is a stronger rule than the one it replaces.
    Any clause left invalid by these changes is removed rather than left stale.
  - `docs/overview.md` — the Cwd Resolution Invariant section (`:63-79`) gains the vocabulary rule alongside the existing enforcement-test description.
  - `manifest/designs/fabric-unified-view.md` — slice 8's section compacts to a shipped summary;
    the "Open questions" entry "Slice 8's CLI-wording question" (`:185`) is resolved and removed.
    The file survives until slice 10 and the open half of slice 6 both land, per its own header.
- Rationale: CLAUDE.md's Task completion rule — a task changing observable CLI behaviour and introducing cross-cutting infrastructure updates docs in the same commit.
  `manifest/roadmap.md` does not move: slice 8 is a planned item within an existing campaign.

## Technical context

**The 15 production call sites**, all verified by grep:

| file:line | today | becomes |
|---|---|---|
| `internal/buildercli/weft.go:24,32,37` | `WeftWorktree` + `New` + `res.WeftCommitted` | `Open(layout)` + `res.Committed()`; file → `sync.go` |
| `internal/webstercli/weft.go:21,27,32` | same | same; file → `sync.go` |
| `internal/perchcli/run.go:322,344,353` | same, inline | `Open(c.layout)` + `res.Committed()` |
| `internal/builderengine/spawn.go:367` | `New(…, WeftWorktree(…))` for `WarpResetter` | `Open(deps.Layout)`; interface → `FabricResetter` |
| `internal/websterengine/runlevel.go:817` | `New(…, WeftWorktree(…))` for `WarpBisector` | `Open(deps.Layout)`; interface → `FabricBisector` |
| `internal/websterengine/audit.go:91` | `regexp.QuoteMeta(WeftWorktree(layout))` | `fabricengine.NewRefScanner(layout)` |
| `internal/loomengine/preflight.go:105` | `os.Stat(WeftWorktree(l))` | `fabricengine.Ready(l)` |
| `internal/loomengine/preflight.go:112-141` | `Healthy` + substring-matched reason | `Healthy` + typed reason switch |
| `internal/fabriccli/weft_verbs.go:102` | `New(l.WorktreePath(), WeftWorktree(l))` | `Open(l)` |

`runlevel.go:817` is **not** in the task brief's list of seven — it is the same construction pattern as `spawn.go:367` and was found by grep.
`fabriccli/weft_verbs.go:244` (`spawnPush(fabricengine.WeftWorktree(l))`) keeps calling the exported `WeftWorktree`;
`fabriccli` is a registered owner.

**Verified facts that shape the plan:**

- `Fabric.Warp` / `Fabric.Weft`: 45 uses inside `fabricengine`, **zero** in `fabriccli` or any other production package — but **not** a purely in-package rename.
  Four files under `internal/fabricengine/` are `package fabricengine_test` and reach `New`/`Warp`/`Weft` from outside: `fabric_test.go` (7), `weftgit_exclude_test.go` (4), `warpforward_integration_test.go` (4), `checkout_index_refresh_test.go` (2).
  See `export-test-shim`.
- `PartialCommitError`: zero readers outside `fabricengine`.
- `PushWeft`, `PullWeft`, `WeftSHAForWarpSHA`, `RecordCorrespondence`, `WeftLyxDir`, `WeftWorktreePath`, `WeftRepoRoot`, `HostLyxLink`, `HostJunctions`, `PortalLink`, `LauncherDir`: **zero production callers** outside fabric.
  Only `*_test.go` files and one doc comment at `boardengine/board.go:34`.
  No API change needed;
  the comment is inside an owner package.
- `internal/weftname` production importers outside fabric: **only** `websterengine/audit.go`, which `refscanner-owns-both-halves` removes.
  After this task the import is confined to `fabricengine`, `fabriccli`, `lyxtest`.
- In-package callers of the soon-private `newPaired`: `index.go:307`, `unwire.go:100` (both already hold raw paths).
- `Commit`'s `SkipGit`-before-`New` short-circuit: all three CLI sites open-code `if !opts.SkipGit { … }` around the constructor, each with a comment explaining that `New` stats eagerly while `Commit` itself short-circuits.
  The guard stays (behaviour unchanged);
  the explanatory comments reword.
- Embedded templates are reached via `//go:embed` in `websterengine/render.go:42-96` and `builderengine/template.go:19-35`;
  `burlerengine` has four more.

**The comment cleanup** spans 55 production files / ~380 occurrences.
Largest non-owner concentrations: `websterengine/audit.go` (27), `builderengine/spawn.go` (21), `perchcli/run.go` (20), `websterengine/runlevel.go` (13), `loomengine/preflight.go` (12), `burlerengine/doc.go` (12), `webstercli/run.go` (11), `buildercli/run.go` (10), `buildercli/weft.go` (10).
Comment-only mentions also exist in low-level packages that never touch fabric's API — `lyxcwd/anchor.go`, `lyxcwd/lyxcwd.go`, `logger/sink.go`, `gitrepo/doc.go`, `configengine/config.go`, `scoutengine/daemonstate.go` — and reword cleanly (e.g. "the durable, weft-synced `_lyx`" → "the durable, fabric-synced `_lyx`").

**Sequencing note for mill-plan:** unexporting `New` breaks `fabriccli` in the same compile unit, so the constructor change and every call site must land in one commit — the repo cannot be left non-compiling between batches.
`healthy-typed-reason` is likewise atomic with `loomengine/preflight.go`.
The comment-only cleanup in packages with no call site (`burlerengine`, `treadleengine`, `perchengine`, `logger`, `gitrepo`, `configengine`, `scoutengine`, `lyxcwd`, `configsync`, `selfreportcli`, `reedcli`, `reedengine`, `shuttlecli`, `burlercli`, `vscode`, `ideengine`) is independent and can be its own batch, but every batch must land before the enforcement test is enabled.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and never mentions weft.
  Nothing is added to `lyxcwd`;
  the invariant's fabric bullet widens and gains a second enforcing test.
  `lyxcwd`'s import cap (stdlib + `internal/gitexec`) is untouched.
- **lyxtest Leaf Invariant** — `internal/lyxtest`'s import set is stdlib plus `lyxcwd`, `weftname`, `configengine`;
  `fabricengine`/`fabriccli` are banned.
  This is why `lyxtest` is a vocabulary owner rather than a cleanup target.
- **Fabric Git Invariant (warp + weft)** — every git operation lyx performs goes through `fabricengine`.
  Its own heading and body keep the two-repo vocabulary;
  its **Enforced by** bullet is corrected (see `documentation`).
- **CLI/Cobra Invariant** — `Short` on every command, help-tree tests.
  No command is added or removed, but `configcli`'s `Long` changes and three `RunE` bodies emit a renamed JSON key.
- **Documentation Lifecycle** — see `documentation`.

Discovered during discussion:

- `Open` must not change `Commit`'s behaviour in any way: the `SkipGit` guard placement, the combined write lock, the async detached push, and the three-outcome `PartialCommitError` mapping all stay exactly as they are.
- `healthy-typed-reason` is the only change with a behavioural surface;
  its classification outcomes must be provably identical.
- The `configsync` and `tools/`/`sandbox/` exceptions are permanent, not transitional.

## Testing

**TDD candidates** — write these first, they define the new API:

- `fabricengine.Open`: returns `*ErrMissingPath` naming the host worktree when it is absent, and naming the sibling when *it* is absent, with the host checked first — the same contract `New` had, reached through a `*lyxcwd.Location`.
  `lyxtest.CopyPaired` for the happy path;
  delete one side per failure case.
- `fabricengine.Ready`: `(false, nil)` on absence, `(true, nil)` when present, `(false, err)` on a non-absence stat failure.
  The third case is what pins `ready-not-paired`'s contract and must not be omitted.
- `fabricengine.CommitResult.Committed()`: true when either side committed, false for a fully degenerate no-op.
  Table-driven over the four field combinations;
  no fixture needed.
- `fabricengine.RefScanner.Matches`: covers the three cases `websterengine/audit_test.go` exercises today — a command containing the sibling worktree path, a command containing a `-weft` sibling name, and a `lyx fabric`/`lyx weft`/`lyx warp` invocation — plus a clean command that must not match.
  This is the behavioural contract that must not regress when the regex moves packages.
- `fabricengine.Healthy`'s typed reason: one case per cause, asserting the typed value — all **five** from `healthy-typed-reason`'s table, config-load failure included.
  Paired with a `loomengine` test asserting that each of the five maps to the same `CheckID` it maps to today: branch mismatch → `CheckFabricSync`;
  the other four → `CheckJunction` with `check3BlocksSeed` set.
  This equivalence is the safety net for the task's only behavioural-surface change, and it must enumerate the causes individually — a single "junction-ish" case would re-encode the substring coincidence the change exists to remove.

**Regression coverage that must keep passing:**

- `buildercli/weft_integration_test.go:132-148` and `webstercli/weft_integration_test.go:140-157` — the commit-landed-but-`RecordCorrespondence`-failed case, asserting the helper reports `(true, err)` not `(false, err)`.
  These are the only coverage of the partial-failure path;
  they move to the renamed `sync.go` helper and the `Committed()` return, and must assert the same outcome.
- `websterengine/audit_test.go` — `CheckFork`/`CheckParent` still flag a fabric-referencing Bash command.
  Its three `fabricengine.WeftWorktree(layout)` calls and its `fakeLayout` helper move onto the scanner;
  the violation-class assertion updates to `ClassFabricReference`.
- `websterengine/template_test.go:246,257,318` and the `builderengine`/`burlerengine` template tests — pinned template literals update to the rewritten one-repo wording.
  These are what prove `templates-describe-one-repo` actually landed in the embedded files.
- `loomengine/preflight_integration_test.go:310` removes the sibling worktree to drive the not-present branch;
  it keeps doing exactly that, through `lyxtest`'s fixture field rather than `fabricengine.WeftWorktree`.
- `buildercli/run_test.go:150` — `"weftCommitted"` → `"fabricCommitted"`.
- `configcli_test.go:187` — `"weft sync failed"` → `"fabric sync failed"`.
- `webstercli/verbs_test.go:633` — a **negative** assertion (`if strings.Contains(out.String(), "weft sync failed")`).
  After the reword that string can never appear, so leaving it makes the test pass forever while checking nothing.
  It must be updated to assert against `"fabric sync failed"`, not deleted.
- `internal/fabricengine/fabric_test.go:25,44` — the missing-path contract, restated through `Open(l)` per `export-test-shim`;
  the other three external-package test files move onto the `export_test.go` shim.
- `configcli/configcli_integration_test.go:111` uses `fabricengine.WeftWorktreePath`, which stays exported (zero production callers, no API change) — no edit needed.

**The enforcement test itself** needs a predicate sub-test on synthetic snippets, mirroring `TestEnforcement_GeometryLiterals`'s existing `t.Run("predicate", …)`: a non-owner file with `weft` in an identifier fails;
one with `warp` in a string literal fails;
one with `weft` in a comment fails;
an embedded `.md` template with `weft` fails;
an owner-set file with all of the above passes;
a `configsync`-row file passes on a string literal but fails on an identifier.
Without this the test can silently stop matching anything and still go green.

**Full-suite gate:** `go test ./...` must pass.
The renames touch exported symbols in six packages, so compile breakage in test files is the expected first signal.

## Q&A log

- **Q:** Should the fix add one unified `Open(l)`, or split by what each caller uses (`CommitScoped` for the CLI sites, `OpenWarp` for the engine sites)? **A:** One unified `Open(l)`. No external caller may see that warp and weft exist — a warp-named constructor breaks that as surely as a weft-named one.
- **Q:** What does `New` actually do, and why must callers ask for it? **A:** Almost nothing — two `os.Stat`s and two path-holding structs. Establishing this killed the `CommitScoped`/`OpenWarp` split.
- **Q:** Does `Open` set up the dual worktree and wire junctions? **A:** No. It is a handle constructor only; wiring stays on `Topology` (`Add`/`Reconcile`/…), reached through `lyx fabric clone`/`add`/`reconcile`.
- **Q:** Why should `Commit` have a result type that isn't just a standard commit's? **A:** It is the standard `Commit`'s return type; four fields only because one call can land two git commits. Resolved by adding `Committed()` rather than changing the verb.
- **Q:** How should `loomengine`'s pairing probe be named? **A:** `Ready(l)`, not `Paired(l)` — "paired" announces two repos.
- **Q:** Should the audit regex move into fabric, and under what name? **A:** Yes, both halves, as `RefScanner`. `Pattern` is rejected outright — it is `internal/patternengine`'s name.
- **Q:** Should CLI output ever say "weft" to the end user? (the open decision recorded in `fabric-unified-view.md:185`) **A:** No. Consumers say "fabric"; fabric's own wrapped error keeps the weft-level detail. The JSON key is renamed too.
- **Q:** Does the vocabulary ban cover comments, or only code? **A:** Comments too — the task is a *visibility* cleanup, so a doc comment narrating the weft leaks the model just as effectively as a symbol.
- **Q:** Should `fabricengine.List` be renamed or made private? **A:** Neither. A fabric worktree's path is its warp worktree's path, so `List` already lists fabric worktrees.
- **Q:** Should reopening the leak be machine-checked? **A:** Yes — a new `TestEnforcement_FabricVocabulary` alongside the existing geometry guard.
- **Q:** `tools/` holds `weftURL` and two real GitHub repo URLs — exclude, or clean? **A:** Clean the rest of `tools/`, but the identifiers naming the warp and weft test repos keep their weft names; do not rename them to something like `fabricStateRepo`. They point at real repos.
- **Q:** Are the agent prompt templates in scope? **A:** Yes, emphatically. Builder and Webster see only the fabric repo, in which `_lyx` is an ordinary folder. The templates say nothing about links, siblings, or separate worktrees.
- **Q:** Should anything be excluded on scope grounds? **A:** No. This task is a cleanup; the only exclusions are structural invariants, external resources, and separate slices.
- **Q:** How do the four `package fabricengine_test` files survive unexporting `New`/`Warp`/`Weft`? **A:** An `export_test.go` shim, plus restating `fabric_test.go`'s missing-path contract through `Open`.
- **Q:** Should `Healthy`'s config-load failure become a real `error` instead of a cause? **A:** No — that would change preflight's outcome for that case. Keep it a cause; record the oddity.
- **Q:** Are repo-level prose docs (README, docs/) in scope? **A:** Split by what the doc describes — mechanism docs keep the vocabulary, consumer-behaviour docs reword.
- **Q:** Why must a template warn an agent away from a sibling directory it has no reason to look for? **A:** It must not, and the earlier "never leave your worktree root" wording was wrong. The real discovery path is `_lyx` itself being the link (`realpath`/`readlink`/`find -L`/`git -C _lyx`), reachable from inside the agent's own worktree. The positive `_lyx` rule forbids exactly that without hinting the link exists.
- **Q:** Does the scanner keep failing runs on the path half once nothing warns the agent? **A:** Yes, both halves stay hard-fail. The asymmetry is accepted and recorded.
