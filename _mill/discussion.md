# Discussion: Relocate producer prompt files into a stencils/ directory

```yaml
task: Relocate producer prompt files into a stencils/ directory
slug: stencils-directory-reorg
status: discussing
parent: main
```

## Problem

Every LLM producer's prompt lives as a `//go:embed`'d `.md` file at the root of its owning package — `internal/loomengine/discussion-template.md`, `internal/burlerengine/round-orchestrator-template.md`, and 13 more — mixed in with Go source.
They are hard to find as a set, and the files that most need frequent human reading and tuning are buried in directories otherwise full of deterministic Go code.

The deeper problem surfaced during this discussion, and it displaced the original one.
Because the prompts are embedded, the running `lyx` binary reads a frozen copy of each file compiled into itself, never the file on disk.
Editing a prompt has no effect until you rebuild, and the instructions that actually drive every LLM in the system are invisible from outside the binary.
The operator's position is that hiding LLM instructions behind a binary is not acceptable, and that editing a prompt must take effect immediately.

**Why now:** the original plan was to move the files into a top-level `stencils/` tree and link each back into its package with an `internal/fslink` directory junction.
An empirical spike killed that outright (see the `embed-through-junction-spike` decision), which forced the read mechanism itself onto the table.
The task therefore changed shape mid-discussion: it is no longer a file move, it is a new read-and-ownership mechanism of which the file move is one part.

## Scope

**In:**

- A new top-level `stencils/` directory holding the 15 producer prompts, one subfolder per family, renamed to the `<family>-<type>-<role>.md` convention.
- A new `internal/stencilstore` package owning the whole stencil lifecycle: seeding, hash-stamping, edit detection, reading, and validation.
- Runtime reading of every producer prompt from `<hub>/_board/_lyx/stencils/`, replacing direct use of the embedded bytes across four packages and five files (see the embed-site list under Technical context).
- Retention of `//go:embed` for the shipped default bytes only — as the seed source, never as a live read path.
- A hash stamp written into each seeded file's existing leading `<!-- ... -->` banner, and the edit-detection rule built on it.
- A new `lyx stencil` cobra module with `list`, `validate`, `diff`, `sync`, and `promote` subcommands, where `diff` supports `--all` and `--exit-code`.
- A pre-commit hook in loomyard only, running `lyx stencil diff --all --exit-code`, guarding against an unported board-copy edit.
- Drift notification via `logger.Warn` when an operator-edited file falls behind a newer shipped default.
- Amending the treadle import allowlist and its enforcement test to admit `internal/stencilstore`.
- Extending the Fabric Vocabulary enforcement walk to cover the new `stencils/` root.
- Renaming `internal/reedengine/header-template.md` to `console-header.md` and fixing that file's stale doc comment.
- A new `fabricengine.StencilsDir(hub)` resolver beside `BoardDir`, and the signature changes that thread the resolved directory into each engine.
- A new CONSTRAINTS.md invariant recording stencil ownership, the amended treadle allowlist bullet, and the CLI/Cobra seam counts going from eleven/ten to twelve/eleven.
- Rewriting the wiki task's body so it describes the mechanism actually built rather than the junction layout that was disproved.

**Out:**

- Junctions, symlinks, and `internal/fslink` — the spike proved `//go:embed` cannot traverse them, and the operator separately ruled out linking prompt directories into packages as a design.
- Moving `internal/reedengine/console-header.md` into `stencils/`.
  It is a tmux pane display banner rendered through `internal/tokenvocab`, not a producer prompt, and it stays embedded in `internal/reedengine` exactly as today.
  Only its filename and doc comment change.
- `crucible/orchestrator-prompt.md` and `crucible/review-prompt-template.md` — a separate, manually-operated paste-ready system never rendered by Go code.
- `internal/planparser/testdata/goodplan/*.md` — test fixtures, a different category.
- The `template.yaml` config templates.
  They already have their own materialize-and-read lifecycle through `configreg`/`configsync`, and this task neither changes nor merges with it.
- Moving `<hub>/.lyx` into `<hub>/_board`.
  Raised by the operator here and deliberately deferred to its own task, `hub-dotlyx-into-board`, already filed in the wiki backlog.
- Removing the requirement that lyx run inside a lyx-initialised repo.
  The operator raised this as a real limitation but explicitly parked it.
- Any automatic three-way merge of an operator-edited stencil against a newer default.

## Decisions

### embed-through-junction-spike

- Decision: the junction-based layout in the original proposal is not implementable and is abandoned.
  `//go:embed` cannot resolve through a directory symlink or junction, in any form.
- Rationale: run empirically before any file was moved, in a throwaway module at `.scratch/embedspike/` on Go 1.26/linux, exactly as the proposal demanded.
  Results: a relative symlink with a file pattern fails with `cannot embed file …: in non-directory stencils`;
  an absolute symlink fails identically;
  a whole-directory pattern (`//go:embed stencils`) fails with `cannot embed irregular file stencils`;
  a real directory in the same position builds cleanly;
  and a `.md` in a subdirectory of the embedding package's own directory builds cleanly.
  The control case is what makes this conclusive — identical setup, differing only in whether the path component is a link.
  The errors come from the go command's own embed resolution, not from the OS, so a Windows junction cannot behave better, and the design required the link to work on every supported platform.
- Rejected: proceeding on the assumption it might work on Windows even though it fails on linux — one negative platform settles a design that needs all of them.

### runtime-read-not-embed

- Decision: producers read their prompt from a file on disk at call time, via `os.ReadFile`.
  `//go:embed` is retained solely to carry the shipped default bytes used to seed that file, and is never the path a live read takes.
- Rationale: this is the operator's primary requirement — an edited `.md` must take effect with no rebuild, and the instructions driving every LLM must be readable as ordinary files rather than hidden inside a binary.
- Rejected: keeping embedding as the read path (the status quo — fails both requirements).
  Also rejected: dropping `//go:embed` entirely and having `tools/deploy` install a stencils tree beside the binary.
  `tools/deploy` installs only the binary today, `go install` would produce a binary with no stencils at all, and "the binary cannot find its stencils" would be a new failure class.

### one-tree-with-hash-stamp

- Decision: one directory tree, not two.
  There is no separate `default/` tree and no separate override tree, and no `eject` command.
  Ownership of each individual file is decided by a hash stamp written into the file's leading banner comment.
- Rationale: an earlier two-tree proposal (lyx-owned `default/` always rewritten, plus an operator-owned override folder) was rejected in favour of this because the stamp answers the ownership question per file with strictly less machinery.
  It also dissolves the question of whether V1 should support overrides at all: an override is not a feature to build, it is simply the state of having edited the file.
- Rejected: two trees plus an `eject` verb — more code, two places to look, and an extra concept for the operator to hold.

### stamp-format-and-edit-detection

- Decision: each seeded file carries a stamp line inside the leading `<!-- ... -->` banner it already has, of the form `<!-- lyx-stencil: sha256=<hex> -->` (folded into the existing banner block rather than added as a second one).
  The hash is computed over the file's body **after** the leading comment block is stripped — that is, over exactly the text `internal/stencil` parses and the LLM ultimately sees.
  Per file, on every run:

  | state on disk | lyx does |
  |---|---|
  | file absent | write the shipped default, stamp it |
  | `hash(body) == stamp` | untouched by a human → overwrite with the new default if it changed, restamp, silently |
  | `hash(body) == hash(shipped default)` | already equals what we would write → restamp to that hash, silently, and treat as untouched from now on |
  | `hash(body) != stamp` and `!= hash(shipped default)` | human-edited → never touched, the on-disk version is used |
  | stamp missing or unparseable | treated as human-edited → never touched |

  The rows are evaluated in that order, and the third is a reconciliation rule that is load-bearing rather than an optimisation.
  Without it a file whose body has legitimately caught up with the shipped default — after a `promote` and the deploy that follows it, or after an operator reverts an edit by hand — keeps a stamp naming the *old* default forever, is classified edited forever, is skipped by every future refresh forever, and never returns to a clean state.
  With it, the stamp self-heals the moment body and shipped default agree.

- Rationale: hashing the stripped body is not merely convenient, it is required — a hash stored inside the file cannot cover itself, and stripping the leading comment is what removes the self-reference.
  It also has the right semantics: editing banner prose is not editing the instructions, while editing the instructions always changes the hash.
  A content hash is preferred over a hand-maintained version number because nobody remembers to bump a number and a hash cannot be wrong.
  The stamp records provenance — which default this file came from — never a version of the operator's own file, which is why manual editing correctly never touches it.
  Every ambiguous state resolves to "leave it alone", so the mechanism always fails toward not destroying the operator's work.
- Rationale for the normal case: the operator expects to rarely edit these files and to change the lyx-side source instead, so "untouched → silently updated" is the common path and must require no interaction whatsoever.
- Rejected: a hand-maintained integer or date version in the file.
  Also rejected: storing hashes in a sidecar file — a third file per stencil, and it can desynchronise from the file it describes.

### no-automatic-merge

- Decision: lyx never merges a newer default into an operator-edited file.
  When an edited file falls behind, lyx emits one non-blocking `logger.Warn` and provides `lyx stencil diff <name>`;
  porting changes across is a human act.
- Rationale: a merged LLM instruction carrying conflict markers that nobody read is a producer that misbehaves inexplicably.
  The diff carries almost all the value at a fraction of the risk.
- Rationale for where the base text comes from: `_lyx` is tracked content and lyx commits its own `_lyx` writes, so every default-refresh lands as a commit in the board repo.
  The board repo's own git history is therefore the archive of every default version that hub has ever seen, which is what makes `diff` possible with no historical versions embedded in the binary and no base copies on disk.
- The two diff modes have **different base texts**, and conflating them would make the pre-commit guard unusable:

  | mode | base | target | purpose |
  |---|---|---|---|
  | `lyx stencil diff <name>` | the default this file was forked from, recovered from the board repo's git history of that file | the currently shipped default | upstream changes the operator has not yet taken |
  | `lyx stencil diff --all --exit-code` | the worktree's own `stencils/<family>/<name>.md` source body | the live board copy's body | an edit made in the board copy that was never ported back |

  The port-back guard must compare against the **source tree**, not the shipped default.
  Comparing against the shipped default would make the guard fire on exactly the commit that fixes it: the developer edits the board copy, runs `promote`, and commits — but the shipped default is still the old embedded one until the next deploy, so a shipped-default base would block that commit and every later one until a rebuild.
  Against the source tree, `promote` brings the two into agreement immediately and the hook passes.
- Rejected: `git merge-file` three-way rebase of an override (technically feasible since all three texts are recoverable, but it writes into the file the operator owns).
  Also rejected: auto-merging only when conflict-free — silent writes into the operator's file precisely when they are not watching.

### hub-wide-placement

- Decision: the read path is `<hub>/_board/_lyx/stencils/<family>/<name>.md` — one copy per hub, shared by every worktree.
- Rationale: a stencil edit should apply to the whole repo and all its worktrees, not to one task worktree.
  `_board` is what means "shared across every worktree in this hub".
  The precedent is exact and already shipping: `fabric.yaml` is hub-wide and materialised once at `configengine.ConfigDir(fabricengine.BoardDir(hub))` via `configsync.ReconcileFabricAt`, in deliberate contrast to per-worktree `ReconcileAll`.
- Consequence to be documented for the operator: an edit takes effect for every worktree in the hub, including sessions running right now, at their next render.
  A stencil edit is not an isolated act.
- Rejected: `<worktree>/_lyx/stencils/` — 15 files duplicated per worktree, and an edit would apply only to that one task.

### deployment-versus-production

- Decision: loomyard carries two copies of each stencil by design — the source at top-level `stencils/`, and the live copy at `<hub>/_board/_lyx/stencils/`.
  lyx treats its own repo exactly like any other consumer repo, with no self-recognition special case.
- Rationale: the operator's framing is that files in the repo working directories are not in production.
  A change becomes production only by deploying, after which lyx writes it into `_board/_lyx/stencils` on the next run.
  The two copies are the deployment boundary made visible, not accidental duplication.
- Consequence: the loomyard development loop is edit the board copy, test live, then port back into `stencils/` and deploy to make it permanent.
- Rejected: a special case where lyx detects it is standing in its own source repo and reads top-level `stencils/` directly.
  It would save one copy step but removes the very place where "worked locally, broke after install" bugs are caught.

### port-back-is-mechanical-not-remembered

- Decision: the port-back step in the loop above is never a manual copy.
  `lyx stencil promote <name>` copies the live board copy into the source `stencils/<family>/` tree of the current worktree, stripping the stamp on the way in (the source tree is the seed, so it carries no stamp).
  Additionally, `lyx stencil diff` grows a `--exit-code` flag with git-diff semantics, and loomyard alone wires `lyx stencil diff --all --exit-code` as a pre-commit hook so an unported board edit cannot land silently.
- Rationale: the `deployment-versus-production` loop otherwise ends in a hand-copy, and this codebase does not trust hand-steps — the Fabric Destruction Chokepoint Invariant, the Mutation Record Invariant, and this task's own allowlist and enforcement-walk amendments all exist because review discipline alone was judged insufficient.
  The failure is specifically nasty here.
  An edited board copy is permanently in the "never touched" state by design, its content lives only in `weft:main`'s commit stream rather than in the `stencils/` tree anyone reviewing this feature would read, and every later default refresh skips it forever.
  The drift is therefore silent, permanent, and not self-healing, and it is worst in the one repo that exercises the mechanism most.
- Rationale for the shape: `promote` removes the manual step rather than guarding it, which is the stronger of the two fixes;
  the pre-commit `--exit-code` check catches the case where someone edits the board copy and forgets `promote` entirely.
  The two are complementary, not alternatives.
- Note on why CI cannot be the guard: a CI runner has no access to the operator's hub, so it cannot compare `stencils/` against a `_board/_lyx/stencils/` that only exists on the developer's machine.
  The check has to run where the hub is, which is the pre-commit hook, not CI.
- Hook installation and preconditions, since a guard nobody installs is not a guard.
  The script is **tracked** in the repo under `tools/hooks/pre-commit`, and `tools/deploy` sets `core.hooksPath` to `tools/hooks` when it runs in loomyard.
  `.git/hooks` is deliberately not used: it is untracked, and it lives in the common gitdir, so it is shared repo-wide across every warp worktree rather than being per-worktree.
  Preconditions: when `lyx` is not on PATH, or no hub can be resolved, the hook prints a warning and exits 0.
  It never blocks a commit for missing tooling — making the repo uncommittable whenever the build is broken would be a worse failure than the drift it guards against, and that is precisely the state a developer is in while fixing a broken build.
- Rejected: documenting the port-back as a discipline step and leaving it to memory — that is exactly the discipline-dependent failure mode the hash stamp was introduced to eliminate for the general operator.
  Also rejected: a CI-side assertion, for the reason above.

### stencilstore-ownership

- Decision: a new `internal/stencilstore` package owns the entire lifecycle — seed, hash, edit detection, read, and validate.
  Its API takes an explicit base directory from the caller, e.g. `stencilstore.Read(baseDir, "loom-template-discussion")`.
- Rationale: one package is the single truth about stencil lifecycle, and an explicit `baseDir` keeps every engine *told* its geometry rather than deriving it.
  That distinction is what makes the treadle allowlist amendment defensible rather than a hole in the invariant.
  It also means tests pass a `t.TempDir()` and need no hub, no git, and no fixture — which keeps the affected tests Tier 1 pure.
- Decision on what `baseDir` actually is and how it reaches each engine.
  `baseDir` is the **fully resolved absolute stencils directory**, not a hub path — `stencilstore` never joins `_board` itself.
  Resolution lives in `internal/fabricengine` beside `BoardDir`, as a new `fabricengine.StencilsDir(hub string) string` returning `<hub>/_board/_lyx/stencils`, because `fabricengine` already owns the `_board` token (`BoardDirName`) and `internal/lyxdirs` owns `_lyx`.
  Duplicating either literal inside `stencilstore` would trip `TestEnforcement_GeometryLiterals`.
  Per engine:

  | engine | how it is told |
  |---|---|
  | `loomengine`, `burlerengine` | already carry a `*lyxcwd.Location`; the calling `*cli` package passes `fabricengine.StencilsDir(loc.HubPath)` in |
  | `treadleengine` | a new caller-supplied field alongside the existing `runDir` / `Profile.GateDir`, set by the round runner that adapts onto treadle's vocabulary — treadle stays told, never deriving, so the Runner-Seam Invariant's actual rule holds |
  | `websterengine` | the no-arg accessors `MasterTemplate()`, `IntegrationTemplate()`, `ImplementerBodyTemplate()`, `ForkTemplate()`, `RecoveryTemplate()` take the directory and gain an `error` return, since a read can now fail |

- Rationale: without this the design is unimplementable for treadle specifically.
  `internal/treadleengine` is barred from `internal/lyxcwd` and is told only `runDir` and `Profile.GateDir`, neither of which is the hub, and its embedded vars are read deep inside `runJudgeCall` in `judge.go`/`targeting.go`.
  Webster's accessors are no-arg today and cannot stay that way once reading can fail.
- Rejected: reading in the composition root and threading prompt bytes into every engine (changes signatures across all five engines and pushes an I/O dependency up into the CLI layer for every producer).
  Also rejected: `stencilstore` taking a hub path and joining `_board`/`_lyx` itself — it would restate geometry tokens two other packages own.
  Also rejected: a package-level root injected once at startup (global mutable state).
  Also rejected: putting seeding in `configsync` beside config materialisation — it splits stencil logic across two packages and drags `configsync` into treadle's import path.

### seeding-trigger

- Decision: seeding and refresh happen automatically on every lyx run that needs a stencil, and the resulting board write is committed through `Bolt` like any other board write.
  `lyx stencil sync` exists to force the same operation on demand, but is never the only way it happens.
- Rationale: self-healing and always current — a deleted file reappears, and the first run after a deploy carries the changed defaults across in one commit.
  That commit is also what builds the git history `stencil diff` depends on.
  Writing only on an explicit command would leave the tree stale until the operator remembered, which would make "always readable on disk" untrue.
- Rejected: explicit-sync-only, and writing without committing (which would leave the files untracked in board and destroy the history `diff` needs).

### read-cadence

- Decision: read the file on every call, with no caching.
- Rationale: that is what immediate effect means, and one `os.ReadFile` per prompt render is negligible beside the LLM call that follows it.
- Rejected: read once per process — would require a restart to observe an edit.

### missing-board-is-a-hard-error

- Decision: if the board is unavailable, the producer path fails loudly.
  There is no silent fallback to the embedded default at runtime.
- Rationale: the board is required for lyx to operate at all, so a missing board is a real error rather than a condition to paper over.
  A silent in-memory fallback would also mean a producer could quietly run against different instructions than the ones on disk, which is exactly the invisibility this task exists to remove.
- Note: this costs nothing in tests, because `stencilstore` takes an explicit base directory — tests seed a `t.TempDir()` and never need a hub.
- Rejected: falling back to embedded defaults when the board is missing.

### invalid-stencil-handling

- Decision: an unfillable stencil fails loudly at the point of use, and `lyx stencil validate` exists to catch it up front.
- Rationale: `stencil.Fill` requires every top-level marker to be present, so an edit that deletes a marker breaks a producer mid-run.
  The up-front check mirrors `reedengine`'s existing `ValidateHeader`, which exists for exactly this reason.
- Rejected: falling back to the default when an override is invalid — that silently ignores the operator's edit, which is worse than failing.

### cli-surface

- Decision: a new `lyx stencil` cobra module with `list`, `validate`, `diff`, `sync`, and `promote`.
  `diff` takes `--all` and `--exit-code`.
- Rationale: `validate` and `diff` were decided independently, `diff` is the entire migration story, and `list` is what makes the stencil set discoverable.
  Building the mechanism without them leaves it unoperatable.
  The CLI is additive: seeding is automatic, and `sync` only forces what already happens.
  `promote` and the `--exit-code` flag exist for the `port-back-is-mechanical-not-remembered` decision.
- Rejected: no CLI in V1 (leaves drift undiagnosable), and `validate`-only (omits the verb that matters the day a default changes).

### drift-notification-channel

- Decision: `logger.Warn`, one line, never blocking.
- Rationale: it reaches the durable Info+ trace sink and respects existing verbosity configuration, inventing no new channel.
  The CLI/Cobra Invariant reserves the JSON envelope for errors, and a drift notice is not an error.
- Rejected: a new envelope field (widens the key set for every command) and raw stderr (bypasses the logger layer everything else uses).

### file-layout-and-naming

- Decision: the original proposal's layout and naming survive unchanged — `stencils/{loom,burler,treadle,webster}/`, files named `<family>-<type>-<role>.md`, with the type token one of `template`, `step`, `prefix`, `body`.
  The `<hub>/_board/_lyx/stencils/` tree mirrors the same family subfolders and filenames.
  A stencil's identity is its filename without the extension;
  its family subfolder is derived from the name's first token.
- Rationale: nothing in the spike or the mechanism change touched the naming question, which was settled on its own merits before this task.
  There is no `reed/` subfolder.
- Full mapping, all 15 files:

  | current path | new path |
  |---|---|
  | `internal/loomengine/discussion-template.md` | `stencils/loom/loom-template-discussion.md` |
  | `internal/loomengine/plan-template.md` | `stencils/loom/loom-template-plan.md` |
  | `internal/burlerengine/round-orchestrator-template.md` | `stencils/burler/burler-template-round-orchestrator.md` |
  | `internal/burlerengine/instruction-1-explore-template.md` | `stencils/burler/burler-step-1-explore.md` |
  | `internal/burlerengine/instruction-2-review-template.md` | `stencils/burler/burler-step-2-review.md` |
  | `internal/burlerengine/instruction-3-fix-template.md` | `stencils/burler/burler-step-3-fix.md` |
  | `internal/treadleengine/judge-circling-template.md` | `stencils/treadle/treadle-template-judge-circling.md` |
  | `internal/treadleengine/judge-milestone-template.md` | `stencils/treadle/treadle-template-judge-milestone.md` |
  | `internal/treadleengine/triage-template.md` | `stencils/treadle/treadle-template-triage.md` |
  | `internal/treadleengine/targeting-template.md` | `stencils/treadle/treadle-template-targeting.md` |
  | `internal/websterengine/master-template.md` | `stencils/webster/webster-template-master.md` |
  | `internal/websterengine/integration-template.md` | `stencils/webster/webster-template-integration.md` |
  | `internal/websterengine/fork-prefix.md` | `stencils/webster/webster-prefix-fork.md` |
  | `internal/websterengine/recovery-prefix.md` | `stencils/webster/webster-prefix-recovery.md` |
  | `internal/websterengine/implementer-body.md` | `stencils/webster/webster-body-implementer.md` |

### stencils-is-a-go-package

- Decision: top-level `stencils/` contains exactly one `.go` file at its root, holding every `//go:embed` directive and exporting one default per stencil.
  The family subfolders contain only `.md`.
- Rationale: `//go:embed` reaches only files at or below the embedding package's own directory, so a top-level prompt directory forces exactly one Go file at that directory's root — there is no arrangement with zero.
  Verified building in the spike.
  Exporting one typed default per stencil rather than an `embed.FS` keeps a renamed or missing file a build error instead of a runtime one.
- Note on naming: the package sits alongside the existing `internal/stencil` (the rendering mechanism).
  Plural versus singular is the only distinguisher, accepted because call sites read `stencils.LoomTemplateDiscussion` against `stencil.Fill`.
- Rejected: one Go package per family (four packages where one suffices), and `internal/stencils/` (keeps the prompts under `internal/`, which is the thing the task set out to undo).

### reed-rename

- Decision: `internal/reedengine/header-template.md` becomes `internal/reedengine/console-header.md`, staying in `internal/reedengine`, staying embedded, and staying entirely outside the stencil mechanism.
- Rationale: it is a tmux pane display banner rendered through `internal/tokenvocab` by `internal/reedengine/header.go`, not a producer prompt.
  Dropping "template" from its name stops the word denoting three unrelated things.
  `console-header.md` says what it is;
  bare `header.md` would collide visually with `header.go` beside it.
- Additional fix in the same commit: `internal/reedengine/headertemplate.go`'s doc comment claims the asset is rendered "via internal/stencil", which is false — `header.go` uses `tokenvocab`.

### task-stays-whole

- Decision: this remains one task, and the wiki task body is rewritten to describe the mechanism actually being built.
- Rationale: the mechanism and the file move are one coherent change;
  splitting would land half a mechanism in main, and a mechanism-only first task delivers nothing observable.
  The original proposal instructed a stop-and-rescope if the spike failed, and the operator, having taken every decision that rescoping would have covered, elected to continue.
- Rejected: splitting into mechanism-then-move, and returning to the board for a formal rescope.

## Technical context

**The 16-versus-15 count.** A repo-wide sweep confirms exactly 16 `.md` files under `internal/` outside `internal/planparser/testdata/`: 4 in `burlerengine`, 2 in `loomengine`, 1 in `reedengine`, 4 in `treadleengine`, 5 in `websterengine`.
15 are producer prompts;
reed's is the false positive.
The proposal's "16 files" prose and its 15-row table are consistent.

**Current embed sites**, all of which change:

- `internal/loomengine/prompttemplate.go` and `plantemplate.go` — one `[]byte` var each.
- `internal/burlerengine/template.go` — four `[]byte` vars.
- `internal/treadleengine/template.go` — four `[]byte` vars.
- `internal/websterengine/render.go` — five `[]byte` vars, plus the accessors `MasterTemplate()`, `IntegrationTemplate()`, `ImplementerBodyTemplate()`, `ForkTemplate()`, `RecoveryTemplate()`.

**Webster composes rather than reads directly.** `render.go`'s `joinTemplateAssets` concatenates a prefix and a body with a blank line before `stencil.Fill` runs, because `internal/stencil` has no include mechanism.
`composeForkTemplate` joins `fork-prefix` ahead of `implementer-body`;
`composeRecoveryTemplate` joins `recovery-prefix` ahead of the same body.
Three files therefore participate in two composed prompts, and both must read through `stencilstore` for an edit to any of the three to take effect.
Note that the composed result contains two leading banner comments, only the first of which `stripLeadingComment` removes — this is existing behaviour, not something this task introduces, but the plan must not make it worse.

**The banner comment already exists and is already stripped.** `internal/stencil/stencil.go:27` calls `stripLeadingComment` before parsing, and `stencil.go:67` implements it: a leading `<!--` … `-->` block is dropped, otherwise the text is returned unchanged.
All 15 files open with such a banner today.
This is what makes the hash stamp free — it never reaches the LLM.

**Config precedent to model against, not to merge with.** `configreg.Modules()` pairs each module with a `Template func() string` from an embedded `template.yaml`;
`configsync.ReconcileAll` materialises the template when the file is absent, and for `SeedOnly` modules never rewrites a file that is present.
`configengine.ConfigDir(baseDir)` is `<baseDir>/_lyx/config`.
`internal/fabriccli/fabric.go:608-615` calls `configsync.ReconcileFabricAt(fabricengine.BoardDir(l.HubPath), true)`, which is the hub-wide counterpart and the exact shape `stencilstore` should follow for placement.
`fabricengine.BoardDir(hub)` is `filepath.Join(hub, BoardDirName)` (`internal/fabricengine/junctionnames.go:100`).

**Reed already does two-tier resolution at value level.** `internal/reedengine/header.go` uses `e.cfg.Header.Template` when non-empty and falls back to the embedded `HeaderTemplate()` otherwise.
Same idea, different layer.

**Board writes go through `Bolt`.** Per the Fabric Git Invariant, no package other than `internal/fabricengine` runs raw git, and board writes flow through `fabricengine.NewBolt(fabricengine.BoardDir(hub))` (see `internal/fabriccli/fabric.go:635`).
Seeding is therefore a commit on `weft:main`, not a bare file write.

**The enforcement walk will silently stop covering these files.** `internal/lyxcwd/enforcement_test.go:936` runs `walkEnforcementRoots(t, repoRoot, []string{"internal"}, []string{".md"}, …)` to police the Fabric Vocabulary Invariant across `internal/**/*.md`.
Moving the 15 files out of `internal/` un-guards every one of them unless `"stencils"` is added to that root list.
This must land in the same commit as the move.

**Treadle's import allowlist is machine-enforced.** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`) pins stdlib plus `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/shuttleengine`, and `gopkg.in/yaml.v3`.
Adding `internal/stencilstore` requires editing both that test and the CONSTRAINTS bullet in the same commit.

**Install layout.** `tools/deploy` builds and installs the binary only, into `-dest` or GOBIN (`tools/deploy/main.go`).
There is no installed asset directory, which is why shipping defaults as files beside the binary was rejected.

**Spike artefacts** live at `.scratch/embedspike/` and are disposable — `.scratch/` is gitignored.
The plan should not preserve them.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Vocabulary Invariant** — the `internal/**/*.md` walk in `internal/lyxcwd/enforcement_test.go` must gain the `stencils` root, or 15 files silently leave coverage.
  The new `stencils/*.go` file is production Go outside `internal/` and `cmd/`, so it falls outside the Go half of that walk;
  the plan should note this honestly rather than imply coverage it does not have.
- **Treadle Runner-Seam Invariant** — the allowlist must be amended to admit `internal/stencilstore`, with the justification (treadle is still *told* its base directory, never deriving it) recorded in the CONSTRAINTS bullet.
- **CLI / Cobra Invariant** — `lyx stencil` needs `Command()`/`RunCLI` (and `RunCLIIn`, since it reads geometry), a non-empty `Short` on parent and every subcommand, registration in `newRoot()` plus the root `Long` module list, `RunE = clihelp.GroupRunE` on the parent, and JSON errors through the `internal/output` envelope.
  `cmd/lyx/helptree_test.go`, `registration_test.go`, `longlist_test.go`, `drift_test.go`, and `seamsignature_test.go` all react to a new module.
  The invariant's own text hardcodes the seam counts — "eleven seam modules" and "ten of the eleven" carrying `RunCLIIn` — so adding `stencil` makes those twelve and eleven, and that edit belongs in the same commit as the rest.
  `stencil` carries `RunCLIIn`, since it reads geometry.
- **Sandbox Suite Coverage** — a newly registered module needs either a `**Covers:** stencil` scenario in a `tools/sandbox/*SUITE.md` file or an `excludedModules` entry with a reason;
  `cmd/lyx/sandbox_coverage_test.go` fails otherwise.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` holds tracked content only, which is correct for stencils.
  Nothing in this task writes under `.lyx`.
- **Fabric Git Invariant** — the board write is a `Bolt` operation, never raw git.
  Note what `Bolt` actually does, since an earlier draft of this document got it wrong: `Bolt.Commit` (`internal/fabricengine/bolt.go:23`) delegates to `commitWeftAt` (`internal/fabricengine/weftgit.go:336-341`), which takes **no pathspec** and calls `gitrepo.StageAllAndCommit`, and whose own doc states it "does not acquire the weft write lock;
  the caller is responsible for synchronization".
  There is no `ScopedPathspec` on this path.
  Two consequences the plan must handle rather than inherit: the commit stages everything in the board repo, so seeding must not run while unrelated board edits are in flight;
  and seeding fires on every run from every worktree and session in a hub, so concurrent seeding writes need explicit synchronisation.
  Required rule: seeding writes only when content actually changes (the common case writes nothing at all), and the write plus commit runs under `Bolt.Sync`, which is the existing absorbing-push-lock seam on this same handle.
- **Test Tier Purity Invariant** — untagged tests must not spawn git or build hub fixtures.
  Satisfied by `stencilstore` taking an explicit base directory, so tests use `t.TempDir()`.
- **Documentation Lifecycle / task-completion rule** — `manifest/designs/` for any module doc touched, `docs/overview.md` for the module table and execution stack (a new `stencil` module changes both), and CONSTRAINTS.md for the new invariant, all in the same commit.
  `docs/overview.md:288-289` names `discussion-template.md` and `plan-template.md` by their current paths and must be updated.
- **Markdown Link Integrity** — `manifest/` and `docs/` are the scan sources;
  any new link from those files into `stencils/` will be resolved, so paths must be correct.

New invariant to record in CONSTRAINTS.md in the same commit: **Stencil Ownership Invariant** — every producer prompt is read at call time from `<hub>/_board/_lyx/stencils/`, never from embedded bytes;
`//go:embed` in `stencils/` carries seed defaults only;
`internal/stencilstore` is the sole owner of seeding, hash-stamping, edit detection, reading, and validation;
and a file whose body hash does not match its stamp is never overwritten.

## Testing

**`internal/stencilstore` — the TDD candidate, and the only one.**
Every state in the edit-detection table is a unit test against a `t.TempDir()`, with no git and no hub, keeping the package Tier 1 pure:

- absent file seeds the default and writes a stamp
- untouched file (`hash(body) == stamp`) with an unchanged default is left byte-identical
- untouched file with a changed default is overwritten and restamped
- edited file (`hash(body) != stamp`) is never modified, and its content is what a read returns
- file with a missing or malformed stamp is treated as edited and left alone
- a file edited and then reverted to the exact default body is treated as untouched
- hashing ignores changes confined to the leading banner comment, and reacts to any change below it
- a stencil whose body no longer fills (a deleted top-level marker) fails validation with the offending name

**Reading and rendering.** One test per producer family asserting that an edited on-disk file — not the embedded default — is what reaches `stencil.Fill`.
Webster needs its own case covering the composed prompts: editing `webster-prefix-fork.md` or `webster-body-implementer.md` must change `ForkTemplate()` output, and editing the body must change both composed prompts.

**Missing board.** Assert the hard error, and assert its message names what is missing.

**Existing tests that assert on prompt content stay pointed at the embedded defaults.**
`internal/burlerengine/template_test.go` enforces the Review Round Invariant (`TestTemplate_StatesRoundDiscipline`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`) and is testing the *shipped* prompt, which remains the right subject.
These should keep working with at most a rename of the variables they read.
The plan must verify this rather than assume it.

**Enforcement tests to re-run and extend.** `internal/lyxcwd/enforcement_test.go` with the added `stencils` root — confirm all 15 relocated files are actually visited, ideally by asserting a non-zero visit count rather than trusting a silent walk.
`internal/treadleengine/seam_enforcement_test.go` with the amended allowlist.
`cmd/lyx/sandbox_coverage_test.go`, `helptree_test.go`, `registration_test.go`, `longlist_test.go`, `seamsignature_test.go` for the new CLI module.

**CLI.** `list` names all 15;
`validate` reports a deliberately broken stencil;
`diff` produces output against a seeded-then-changed default;
`sync` is idempotent on a second run.

**Port-back guard.** `promote` copies an edited board copy into the source tree and strips the stamp on the way in.
Assert the full round trip explicitly, since it is what closes the loop: promote, then re-seed from the new default, and assert the board copy ends up restamped and back in the untouched state via the reconciliation row of the edit-detection table.
A test that stops at `promote` would pass while leaving the file permanently classified edited.
`diff --all --exit-code` exits non-zero when a board copy differs from the worktree's `stencils/` source and zero when they agree — including zero immediately after a `promote`, which is the case the pre-commit hook depends on.
Both directions need a test, because an `--exit-code` that never fires is a hook that silently passes forever.

**Seeding concurrency.** Assert that a run whose defaults are unchanged writes nothing and produces no board commit, since that is what keeps `Bolt`'s wildcard `StageAllAndCommit` from sweeping up unrelated in-flight board edits on every single run.

**Full-suite gate.** `go build ./...` and the full `go test ./...` must pass, since this change touches five engines, one enforcement walk, one import allowlist, and the cobra root.

## Q&A log

- **Q:** The spike shows `//go:embed` cannot traverse a junction. Continue, or stop and rescope as the proposal instructed? **A:** Continue — junctions are out entirely, and not acceptable as a design regardless of whether embedding could see through them.
- **Q:** Keep `//go:embed` at all? **A:** Only as the seed default. Prompts must be readable as files and editable with immediate effect; hiding LLM instructions behind a binary is not acceptable.
- **Q:** Where does the shipped default come from at runtime? **A:** Embedded in the binary, written out to disk on first run, and read from disk thereafter.
- **Q:** How does an improved default reach a repo that already seeded? **A:** Hash stamp in the file. If the operator has not edited it, overwrite silently with no migration prompt at all — that is the common case, since the operator expects to change the lyx-side source rather than the deployed file.
- **Q:** Should the stamp be a version number the operator maintains? **A:** No. A content hash, and manual editing must never require touching it — it records provenance, not the operator's own version.
- **Q:** One tree with per-file stamps, or two trees plus an eject command? **A:** One tree. The stamp settles ownership per file, and "override" stops being a feature that needs building.
- **Q:** Per-worktree or hub-wide? **A:** Hub-wide. A stencil change should apply to the whole repo and every worktree.
- **Q:** Does `_lyx` exist inside `_board` already? **A:** Yes — `fabric.yaml` lives at `<hub>/_board/_lyx/config/`, so the placement is established mechanism rather than a new one.
- **Q:** Should `<hub>/.lyx` move into `_board` too, since `_board` is what means hub-wide? **A:** Yes, but as its own task — CONSTRAINTS changes only on explicit instruction. Filed as `hub-dotlyx-into-board`.
- **Q:** How does loomyard handle being both the source of defaults and a consumer? **A:** Two copies, no special case. Files in the working directories are not in production; a change reaches production by deploying, after which lyx writes it into `_board/_lyx/stencils`.
- **Q:** Read cadence? **A:** Every call, no caching — fast on Linux and negligible beside the LLM call.
- **Q:** Missing board — hard error or fall back to embedded? **A:** Hard error. The board must exist. (The operator noted that requiring a lyx-initialised repo at all is a real limitation, but parked it as out of scope.)
- **Q:** Automatic three-way merge for an edited file? **A:** No. Notify and provide a diff;
  the human ports the change.
- **Q:** Does reed's file stay in `internal/reedengine`? **A:** Yes — renamed to `console-header.md`, still embedded, never part of the stencil mechanism.
- **Q:** Does V1 need a `lyx stencil` command? **A:** Yes, and seeding must additionally happen automatically on demand rather than only through it.
- **Q:** Drift notification channel? **A:** `logger.Warn`.
- **Q:** Does `stencilstore` own writing and hashing too, or only reading? **A:** All of it.
- **Q:** Still one task? **A:** Yes, with the wiki task body rewritten to match what is actually being built.
- **Q:** After `promote`, the board copy's stamp still names the old default, so it stays classified edited forever and never returns to clean. What restores it? **A:** A reconciliation row in the edit-detection table — body hash equal to the shipped default's hash means restamp silently and treat as untouched, whatever the stamp said. It also covers a hand-reverted edit.
- **Q:** Does `diff` compare against the shipped default or against the source tree? **A:** Both, in different modes, and conflating them breaks the guard. `diff <name>` is forked-from-default versus shipped default; `diff --all --exit-code` is board copy versus the worktree's `stencils/` source. A shipped-default base would block the very commit that ports a change back.
- **Q:** How does the stencils directory reach `treadleengine`, which is barred from `lyxcwd` and told only `runDir`/`GateDir`? **A:** As a new caller-supplied field, resolved by a new `fabricengine.StencilsDir(hub)` and passed in by the round runner. Webster's five no-arg accessors take the directory and gain an error return.
- **Q:** The loomyard loop ends in a hand-copy from the board copy back into `stencils/`. What stops a real edit becoming permanently invisible to the source tree? **A:** Nothing, as originally written — raised by the orchestrator review. Resolved by making the port-back mechanical (`lyx stencil promote`) and adding a loomyard-only pre-commit `lyx stencil diff --all --exit-code`. CI cannot be the guard, since a CI runner has no access to the operator's hub.
