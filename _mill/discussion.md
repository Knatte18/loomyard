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
- A run-time `logger.Warn` when a board copy has drifted from the worktree's `stencils/` source, folded into the pass lyx already makes over the files.
- Drift notification via `logger.Warn` when an operator-edited file falls behind a newer shipped default.
- Amending the treadle import allowlist and its enforcement test to admit `internal/stencilstore`.
- Extending the Fabric Vocabulary enforcement walk to cover the new `stencils/` root.
- Renaming `internal/reedengine/header-template.md` to `console-header.md`, updating that asset's doc comment and its own leading banner for the new filename.
- A new `fabricengine.StencilsDir(hub)` resolver beside `BoardDir`, and the signature changes that thread the resolved directory into each engine.
- Exporting two things from `internal/stencil`: the leading-comment stripper (used by webster's `joinTemplateAssets`, which now strips every asset's banner) and a top-level-marker lister, today unexported inside `unfilledTopLevelMarkers`, which `validate` needs.
- `.gitattributes` changes: 15 new `stencils/**` LF pins, removal of the 8 stale `internal/*` rows, and a seeded `.gitattributes` in the board's stencils tree.
- A `**Covers:** stencil` scenario in `tools/sandbox/SANDBOX-CORE-SUITE.md`.
- Updating every prose reference to the fifteen old filenames across `docs/`, `manifest/designs/`, `CLAUDE.md`, and the relocated prompts' own banners.
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

- Decision: each seeded file carries a stamp line of the form `<!-- lyx-stencil: sha256=<hex> -->` inside its leading `<!-- ... -->` banner — folded into the existing banner where one exists, or written as a new leading banner for `implementer-body.md`, the one default that ships without one.
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
- **The hash is taken over an LF-normalised body**, and every comparison normalises both sides first.
  Without this the mechanism breaks completely on a machine with `core.autocrlf=true`: the board copy is a git checkout, so an LF file seeded by lyx comes back as CRLF, whose hash matches neither the stamp nor the shipped default — so *every* stencil is classified human-edited, forever, and never refreshed again.
  The `diff <name>` base lookup would diverge on the same platform for a related but distinct reason: that base is read through go-git, which performs no CRLF conversion at all (`internal/gitrepo/doc.go:218`), while the working-tree copy it is compared against was written by CLI git, which does.
  The two sides can therefore differ by line ending alone, so both must be LF-normalised before hashing or diffing.
  Note that `doc.go`'s own conclusion from this is the opposite of "gitrepo is conversion-free": it is why `StageAndCommit`/`StageAllAndCommit` stay CLI-bound.
- **The board's stencils tree is seeded with its own `.gitattributes`** pinning `*.md` to `text eol=lf`, since the generated board repo has none and inherits nothing from loomyard's.
  Its lifecycle is seed-if-absent only, mirroring `configsync`'s `SeedOnly`: written when missing, never rewritten when present, never stamped, never in the registry, invisible to `list`/`validate`/`diff`, and always inside the seeding commit's positive pathspec.
  Seed-if-absent is right because LF-normalised hashing already keeps the mechanism correct on its own, so a second edit-detection scheme for a non-markdown file buys nothing.
- Loomyard's own `.gitattributes` changes too: the 15 new `stencils/**` paths are pinned, and the 8 now-stale `internal/*` rows (four burler, four treadle) are removed.
  Note that loom's two and webster's five are unpinned today, so the move also closes a gap rather than only relocating rows.

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
- The two diff modes have **different base texts**, and conflating them would make the port-back guard unusable:

  | mode | base | target | purpose |
  |---|---|---|---|
  | `lyx stencil diff <name>` | the default this file was forked from, recovered from the board repo's git history of that file (see below) | the currently shipped default | upstream changes the operator has not yet taken |
  | `lyx stencil diff --all --exit-code` | the worktree's own `stencils/<family>/<name>.md` source body | the live board copy's body | an edit made in the board copy that was never ported back |

  The port-back guard must compare against the **source tree**, not the shipped default.
  Comparing against the shipped default would leave the warning firing right through the fix: the developer edits the board copy, runs `promote`, and is still told the copy has drifted, because the shipped default stays the old embedded one until the next deploy.
  Against the source tree, `promote` brings the two into agreement immediately and the warning stops.
- **How the `diff <name>` base is actually recovered**, since "from git history" names no owner and no API.
  `internal/gitrepo` gains a read-blob-at-revision verb, built on go-git — the gitrepo Client Boundary Invariant assigns commit/tree/blob lookups and ref reads to go-git, so this adds no `gitexec` call site and does not touch the Checked-Call Invariant.
  `internal/fabricengine` wraps it as the board-scoped accessor, because the Fabric Git Invariant routes every git operation on either repo through that package and its read-only carve-out is scoped to warp alone.
  Lookup key: walk that path's history in the board repo newest-first, strip each revision's leading comment, hash the body, and take the first revision whose hash equals the file's current stamp — that revision is by definition the default this file was forked from.
  When no revision matches (history pruned, the stamp hand-written, or the file predates its own history), `diff` says so explicitly and falls back to showing the shipped default against the on-disk body.
  It never silently reports an empty diff, which would read as "no upstream changes" when the truth is "base not found".
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
  Additionally, `lyx stencil diff` grows a `--exit-code` flag with git-diff semantics, and lyx itself emits a `logger.Warn` at run time when a board copy has drifted from the worktree's `stencils/` source — the same `logger.Warn` channel as the upstream-drift notice, and the same `Reconcile` pass, which receives the worktree `stencils/` path as its `sourceDir` argument.
  One pass, not two: the 0.15 ms figure this design is justified on is the cost of reading the files once.
- Rationale: the `deployment-versus-production` loop otherwise ends in a hand-copy, and this codebase does not trust hand-steps — the Fabric Destruction Chokepoint Invariant, the Mutation Record Invariant, and this task's own allowlist and enforcement-walk amendments all exist because review discipline alone was judged insufficient.
  The failure is specifically nasty here.
  An edited board copy is permanently in the "never touched" state by design, its content lives only in `weft:main`'s commit stream rather than in the `stencils/` tree anyone reviewing this feature would read, and every later default refresh skips it forever.
  The drift is therefore silent, permanent, and not self-healing, and it is worst in the one repo that exercises the mechanism most.
- Rationale for the shape: `promote` removes the manual step rather than guarding it, and the run-time warning catches the case where someone edits the board copy and forgets `promote` entirely.
  The two are complementary, not alternatives, and the guarantee comes from having removed the manual copy — the warning is a backstop, not the mechanism.
- **The warning never blocks anything**, and cannot, because the comparison is inherently cross-worktree: the board copy is one per hub while `stencils/` is per warp worktree.
  The moment worktree A promotes an edit and deploys, every other task worktree's older source differs from the shared board copy through no fault of its own — and concurrent task worktrees are the normal mode of work in this repo, not an edge case.
  Distinguishing "unported edit" from "another worktree moved ahead" would require asking whether the board body appears anywhere in the source's reachable history, which is far more work than the signal is worth.
  Accepted blast radius, stated plainly: the drift warning can fire in a worktree that changed nothing, and that is tolerable precisely because it only prints.
  In a repo with no `stencils/` source tree — every consumer repo — the warning is skipped silently, since there is nothing to port back to.
  It is the one member of this trio that stays quiet rather than erroring: `promote` and `diff --all` are explicit operator requests and must say why they cannot run, while the warning is unsolicited and would otherwise fire on every run in every consumer repo forever.
- **No git hook.**
  An earlier draft put this check in a loomyard-only `pre-commit` hook.
  Rejected on measurement and on entanglement.
  Measurement: the whole per-run pass — reading all 15 files, normalising line endings, and hashing — costs about 0.15 ms, so folding the check into the run lyx already performs is free, while a hook pays a process spawn on every commit in every worktree.
  Entanglement: installing it meant a tracked script, an install step in `tools/deploy`, and a coexistence contract with `internal/fabricengine`'s own hook installer, which resolves its directory via `git rev-parse --git-path hooks` and would have been affected by any `core.hooksPath` we set.
  `fabricengine.InstallPostCheckoutHook` stays the only hook installer in the repo.
  The hook could only warn anyway, so nothing was lost but the commit-time timing of the message.
- Note on why CI cannot be the guard either: a CI runner has no access to the operator's hub, so it cannot compare `stencils/` against a `_board/_lyx/stencils/` that only exists on the developer's machine.
  The check has to run where the hub is, which is inside lyx itself.
- Rejected: documenting the port-back as a discipline step and leaving it to memory — that is exactly the discipline-dependent failure mode the hash stamp was introduced to eliminate for the general operator.
  Also rejected: a CI-side assertion, and the pre-commit hook, both for the reasons above.

### stencilstore-ownership

- Decision: a new `internal/stencilstore` package owns the entire lifecycle — seed, hash, edit detection, read, and validate.
  Its API takes an explicit base directory from the caller, e.g. `stencilstore.Read(baseDir, "loom-template-discussion")`;
  `Reconcile` additionally takes the registry, the build mode, and the optional worktree source directory — see `seeding-trigger` for the full signature.
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
  | `treadleengine` | a new caller-supplied field alongside the existing `runDir` / `Profile.GateDir`, set by the round runner that adapts onto treadle's vocabulary, and threaded from `run.go` into the four template-reading functions named below — treadle stays told, never deriving, so the Runner-Seam Invariant's actual rule holds |
  | `websterengine` | the no-arg accessors `MasterTemplate()`, `IntegrationTemplate()`, `ImplementerBodyTemplate()`, `ForkTemplate()`, `RecoveryTemplate()` take the directory and gain an `error` return, since a read can now fail |

- Rationale: without this the design is unimplementable for treadle specifically.
  `internal/treadleengine` is barred from `internal/lyxcwd` and is told only `runDir` and `Profile.GateDir`, neither of which is the hub.
  Its embedded vars are read at four separate package-level functions, none of them methods on `Engine`: `runCircling` (`internal/treadleengine/judge.go:58`), `runMilestone` (`judge.go:73`), `runTriage` (`judge.go:147`), and `runTargeting` (`internal/treadleengine/targeting.go:31`).
  `runJudgeCall` (`judge.go:93`) takes the already-selected template as a `template []byte` parameter and reads nothing itself — an earlier draft of this document named it as the read site, which the signatures refute.
  All four take loose scalars rather than a struct, so a new field alone reaches none of them: the directory arrives as a new field **and** as a parameter threaded through those four functions from their callers in `run.go`.
  Webster's accessors are no-arg today and cannot stay that way once reading can fail.
- Rejected: reading in the composition root and threading prompt bytes into every engine (changes signatures across all five engines and pushes an I/O dependency up into the CLI layer for every producer).
  Also rejected: `stencilstore` taking a hub path and joining `_board`/`_lyx` itself — it would restate geometry tokens two other packages own.
  Also rejected: a package-level root injected once at startup (global mutable state).
  Also rejected: putting seeding in `configsync` beside config materialisation — it splits stencil logic across two packages and drags `configsync` into treadle's import path.

### seeding-trigger

- Decision: seeding and refresh run **once per process, at a named composition point** — `cmd/lyx`'s root pre-run — never lazily inside `stencilstore.Read`.
  `lyx stencil sync` forces the same pass on demand, but is never the only way it happens.
- **The split that makes this work, and the import direction it fixes.**
  `stencilstore` writes files and nothing else: a `Reconcile(baseDir string, registry Registry, mode Mode, sourceDir string) ([]string, error)` pass applies the edit-detection table and returns the list of paths it actually wrote.
  `mode` is the caller-supplied dev/production build channel (see the `-dev` bullet below);
  `sourceDir` is the worktree's `stencils/` tree used for the drift comparison, and the empty string means "no source tree here", which is what makes the drift warning silent in a consumer repo.
  Both are arguments, never values `stencilstore` derives — that is what keeps its tests hermetic against a bare `t.TempDir()`.
  The composition root hands that list to the `board.lock`-taking `fabricengine` commit verb.
  `stencilstore` therefore never imports `fabricengine`, and `stencilstore.Read` stays a pure file read.
  This is load-bearing, not tidiness: if the pass ran lazily inside `Read`, treadle's reads — which happen four levels down in `runCircling`/`runMilestone`/`runTriage`/`runTargeting` — would drag `fabricengine` onto treadle's stack, which is exactly what the Runner-Seam allowlist amendment is being justified against.
  Running it at the root instead means treadle's dependency really is one file-reading package.
- Root pre-run resolves no hub for commands that do not have one (`lyx fabric clone` and friends), so the pass is skipped there rather than failing.
  That is not in tension with `missing-board-is-a-hard-error`: the hard error belongs to the producer read path, where a stencil is genuinely required.
- **A `-dev` build seeds absent files but never refreshes an untouched one.**
  The repo deliberately keeps two binaries with different embedded defaults (Dev/Prod Binary Separation;
  `tools/deploy -dev` builds into `.dev-bin`).
  With the plain rule, alternating dev and prod runs against the same hub would rewrite and re-commit the same untouched file in opposite directions on every single run — and that alternation *is* the prescribed test-live-then-deploy loop, so it would be the normal case rather than a corner.
  Rule: a `-dev` build performs row 1 of the edit-detection table (seed when absent) and skips row 2 (refresh when untouched), warning once when its embedded default differs from what is on disk.
  A production build performs the full table.
  **Only row 2 is skipped.**
  A `-dev` build performs rows 1, 3, 4 and 5 unchanged — in particular the reconciliation restamp (row 3), which is what returns a board copy to the untouched state after a `promote`, the one loop the dev binary exists for.
  Row 3 cannot reintroduce the thrash it is grouped with: it writes only the stamp line inside the leading banner, which the hash excludes by construction, and it fires only when the body already equals *that* binary's shipped default, so two binaries can never restamp the same file in opposite directions on alternating runs.
  This requires the binary to know which it is.
  Mechanism: `tools/deploy -dev` sets `var buildChannel string` in `package main` (`cmd/lyx/main.go`) via `-ldflags "-X main.buildChannel=dev"`;
  `tools/deploy/main.go` passes no `-ldflags` today, so this is a new flag on that build path.
  The composition root threads the resulting mode into `stencilstore.Reconcile` as an explicit argument — `stencilstore` never reads build identity itself, which is what keeps its dev/prod tests hermetic.
  `package main` is the right home because the root pre-run is the only consumer.
  An **unstamped** binary — a plain `go build`/`go install`, or a `go test` binary — classifies as **production** and performs the full table.
  Production is the conservative default because it keeps the shipped defaults converging;
  dev is the exception and must opt in explicitly.
  An explicit `lyx stencil sync` **does** perform the refresh row even from a `-dev` build.
  Skipping the refresh is about not thrashing on every incidental run;
  an operator who types `sync` is asking for exactly that write, and the dev binary is the one used in the prescribed test-live loop, so refusing there would make the verb useless where it is most needed.
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
- Rationale: `stencil.Fill` fails when the **template** carries a top-level marker the producer's values do not fill (`internal/stencil/stencil.go:39-43,82`), so an edit that adds or renames a marker breaks a producer mid-run — while an edit that *deletes* a marker fills cleanly and silently drops that content from the prompt, which is the invisibility class this task exists to remove.
  An earlier draft of this document had that direction backwards.
- `validate` therefore compares each body's top-level marker set against its shipped default's, both recoverable through the registry.
  A marker present in the body but absent from the default is an **error** — it will break `Fill`.
  A default marker missing from the body is a **warning** — legal customisation, but content-dropping.
  Comparing against the default's marker set is the only workable basis, because the values each producer supplies live inside the engines and are unreachable from `stencilcli`.
  It also matches the direction `reedengine`'s existing `ValidateHeader` already takes, erroring on an unknown top-level token (`internal/reedengine/header_test.go:51-55`).
- Rejected: falling back to the default when an override is invalid — that silently ignores the operator's edit, which is worse than failing.

### cli-surface

- Decision: a new `lyx stencil` cobra module with `list`, `validate`, `diff`, `sync`, and `promote`.
  `diff` takes `--all` and `--exit-code`.
- Rationale: `validate` and `diff` were decided independently, `diff` is the entire migration story, and `list` is what makes the stencil set discoverable.
  Building the mechanism without them leaves it unoperatable.
  The CLI is additive: seeding is automatic, and `sync` only forces what already happens.
  `promote` and the `--exit-code` flag exist for the `port-back-is-mechanical-not-remembered` decision.
- Behaviour outside loomyard: `promote` and `diff --all` are both defined against a `stencils/` source tree that exists only in this repo, while the module is registered globally.
  In a consumer repo, or for a board copy whose name no longer matches any source file, both exit with an error naming the missing source tree or file.
  Neither ever creates a `stencils/` directory, and neither silently no-ops — a stray source tree in a consumer repo would be read by nothing, and a silent success would misreport the guard as having run.
  `list`, `validate`, `sync`, and `diff <name>` work everywhere, since they need only the board copy.
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
- The same file also holds the **name → default registry**, an exported ordered map from stencil name to its default text, built beside the typed vars.
  `internal/stencilstore` and the composition roots that hand it the registry — `cmd/lyx`'s root pre-run and `stencilcli` — are its only consumers.
  No engine imports it, and treadle calls only `stencilstore.Read(baseDir, name)`, which needs no registry, so treadle's allowlist needs the one `internal/stencilstore` entry and no second one.
  The registry stays a `Reconcile` parameter rather than a package-level import inside `stencilstore`, which is what lets the edit-detection tests run against a fake registry and a bare `t.TempDir()`.
  A test in the `stencils` package walks the family subfolders and asserts the registry and the `.md` tree name exactly the same set in both directions.
  Without that test a hand-maintained map reintroduces the silent-omission failure the typed-var choice exists to prevent — a `.md` added but never registered would be invisible to `list`, never seeded, and never validated.
- Note on naming: the package sits alongside the existing `internal/stencil` (the rendering mechanism).
  Plural versus singular is the only distinguisher, accepted because call sites read `stencils.LoomTemplateDiscussion` against `stencil.Fill`.
- Rejected: one Go package per family (four packages where one suffices), and `internal/stencils/` (keeps the prompts under `internal/`, which is the thing the task set out to undo).

### compose-strips-every-banner

- Decision: `internal/stencil` exports its leading-comment stripper (today the unexported `stripLeadingComment`, `internal/stencil/stencil.go:67`), and `websterengine`'s `joinTemplateAssets` strips **every** asset's banner before concatenating, not just the first.
- Rationale: `render.go:60-77` joins prefix and body, and `stencil.Fill` strips only the leading banner of the joined result, so a banner on the second file would reach the LLM verbatim.
  Today `implementer-body.md` has no banner, so nothing leaks;
  this task creates the hazard, because once that file carries `<!-- lyx-stencil: sha256=… -->` the stamp line is delivered into both the fork prompt and the recovery prompt as if it were instruction text.
  Leaking an internal bookkeeping hash into a producer's prompt is not acceptable, and the general stripper also hardens any future banner-carrying asset.
- Rejected: stripping only the body's banner at compose time (works, but leaves the general case wrong for any future third asset).
  Also rejected: keeping the stamp out of files that are composed — it would exempt exactly three of the fifteen from edit detection.

### reed-rename

- Decision: `internal/reedengine/header-template.md` becomes `internal/reedengine/console-header.md`, staying in `internal/reedengine`, staying embedded, and staying entirely outside the stencil mechanism.
- Rationale: it is a tmux pane display banner rendered through `internal/tokenvocab` by `internal/reedengine/header.go`, not a producer prompt.
  Dropping "template" from its name stops the word denoting three unrelated things.
  `console-header.md` says what it is;
  bare `header.md` would collide visually with `header.go` beside it.
- Additional fix in the same commit: `internal/reedengine/headertemplate.go`'s doc comment (`headertemplate.go:2-4`) names the asset by its old filename and describes the `*-template.md` convention this rename retires for it.
  Rewrite it to name `console-header.md` and to state the render path precisely — `tokenvocab.Render` (`internal/tokenvocab/render.go:12`), which is itself a thin wrapper over `stencil.Fill`.
  The existing "rendered via internal/stencil" wording is **true** and must not be "corrected" to say otherwise;
  an earlier draft of this document asserted it was false, which `internal/tokenvocab/render.go:12` refutes.
  The asset's own leading banner (`header-template.md:1`) names the old filename too and moves with it.

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
Today the composed result carries only the prefix's banner, which `Fill` strips, so nothing leaks.
Once seeding stamps `implementer-body.md` it would carry two, and only the first is stripped — this task creates that hazard and must fix it, see the `compose-strips-every-banner` decision.

**The banner comment already exists and is already stripped.** `internal/stencil/stencil.go:27` calls `stripLeadingComment` before parsing, and `stencil.go:67` implements it: a leading `<!--` … `-->` block is dropped, otherwise the text is returned unchanged.
14 of the 15 open with such a banner today;
`internal/websterengine/implementer-body.md` does not — it opens on its `# Webster implementer job` heading — so seeding creates the banner there.
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
  **Named deviation from the package-naming rule**, to be recorded in the same CONSTRAINTS bullet: the invariant pairs `<module>cli` with a `<module>engine` kernel, but `stencilcli`'s kernel is `internal/stencilstore`, not `stencilengine`.
  Reason: `internal/stencil` already holds the singular name and top-level `stencils` holds the plural, so a third near-homograph would make three packages one character apart, and `stencilstore` says what the package actually is.
  This is a deviation with a reason, not an oversight, and the CONSTRAINTS bullet must say so rather than leaving the next reader to find a rule apparently broken.
- **Sandbox Suite Coverage** — resolved rather than restated: a `**Covers:** stencil` scenario is added to `tools/sandbox/SANDBOX-CORE-SUITE.md`, not an `excludedModules` row.
  `list` and `validate` are read-only and trivially black-box exercisable, so none of the three existing exclusion reasons (interactive stdin, real GitHub writes, external binary on `$PATH`) applies here.
  `cmd/lyx/sandbox_coverage_test.go` fails without one or the other.
- **Durable-vs-Ephemeral State Invariant** — `_lyx` holds tracked content only, which is correct for stencils.
  Nothing in this task writes under `.lyx`.
- **Fabric Git Invariant** — the board write is a `Bolt` operation, never raw git.
  Note what `Bolt` actually does, since an earlier draft of this document got it wrong: `Bolt.Commit` (`internal/fabricengine/bolt.go:23`) delegates to `commitWeftAt` (`internal/fabricengine/weftgit.go:336-341`), which takes **no pathspec** and calls `gitrepo.StageAllAndCommit`, and whose own doc states it "does not acquire the weft write lock;
  the caller is responsible for synchronization".
  There is no `ScopedPathspec` on this path.
  Two consequences the plan must handle rather than inherit: the commit stages everything in the board repo, so seeding must not run while unrelated board edits are in flight;
  and seeding fires on every run from every worktree and session in a hub, so concurrent seeding writes need explicit synchronisation.
  Required rule, in two parts.
  **Write only on change** — the common case writes nothing at all, so the stage-all commit never fires on an ordinary run.
  **Do not use `Bolt` for the seeding commit.**
  `Bolt.Sync` takes `board.push.lock` (`internal/fabricengine/bolt.go:33` → `coalescePush`, `internal/fabricengine/coalesce.go:24`), but board's own file writes are serialised by a *different* lock, `board.lock` (`internal/boardengine/sync.go:24,63`, `internal/boardengine/board.go:109`).
  Seeding under `Bolt.Sync` would therefore not exclude a concurrent `boardCriticalSection` mid-render, and `Bolt.Commit`'s stage-all could capture a half-written board.
  An earlier draft of this document named `Bolt.Sync` and was wrong.
  Instead: a new verb in `internal/fabricengine` acquires `board.lock` and commits the stencils subtree with an explicit positive pathspec via `gitrepo.StageAndCommit`, never stage-all.
  **It commits only and does not push.**
  Pushing per run would fire a push on nearly every lyx invocation;
  the commit rides board's next push through the existing coalescing path instead.
  Residual risk the plan must confirm rather than assume: two hubs seeding independently produce identical *content* (the bytes are deterministic from the binary) but distinct commits, so the plan must verify board's existing push path tolerates that the same way it already tolerates any other independently-made board commit.
  The `board.lock` filename becomes single-declarer in `internal/fabricengine` (which already owns the board directory) with `internal/boardengine` aliasing it rather than re-declaring the literal — the same shape fabric's clone-time guard already uses for the anchor-marker names.
  It is unexported inside `boardengine` today, so it is not reachable from a new package without this move.
- **Mutation Record Invariant** — the new `fabricengine` seeding verb is a mutating fabric verb, so it takes a `rec *Mutations` parameter, appends after each primitive observably changes state, and its result type embeds `MutationRecord`.
  No new `Kind` member is needed: `KindFileWritten` and `KindCommitCreated` already exist (`internal/fabricengine/mutation.go:45,50`), which also keeps the same-commit `Kind`-plus-recording-site-plus-guard-entry rule from applying.
  `lyx stencil sync`'s envelope therefore carries the fixed `mutations` array and `partial` bool like every other mutating verb outcome, while a pre-flight failure emits a bare `output.Err` with neither key, per that invariant's pre-flight carve-out.
- **Test Tier Purity Invariant** — untagged tests must not spawn git or build hub fixtures.
  This is satisfied **for `internal/stencilstore` only**, because it takes an explicit base directory and its tests use `t.TempDir()`.
  It is not a claim about the whole task: the promote round trip, the diff-base history walk, the seeding-commit pathspec, and the mutation-record assertions all need a real board repo, so those files carry an `integration` build tag.
- **Hermetic Git Test Environment Invariant** — every one of those git-spawning test packages needs a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, including the new `stencilcli` test files and any new `fabricengine` test file.
  `cmd/lyx/hermeticenv_test.go` fails otherwise.
- **Documentation Lifecycle / task-completion rule** — `manifest/designs/` for any module doc touched, `docs/overview.md` for the module table and execution stack (a new `stencil` module changes both), and CONSTRAINTS.md for the new invariant, all in the same commit.
  The stale-reference sweep is `grep -rn` over the fifteen **current** filenames across `docs/`, `manifest/`, `CLAUDE.md`, `README.md`, and `internal/**/*.md` — not `docs/` alone, since none of these references is a markdown link and `TestEnforcement_MarkdownLinks` therefore catches none of them.
  Known hits at discussion time: `docs/overview.md:288-289`, `manifest/designs/loom.md:193`, `manifest/designs/scout-plan-symbol-fields.md` (seven mentions of `plan-template.md`, in a design whose whole subject is editing that file), `manifest/designs/shed-followups.md:180`, `CLAUDE.md:67` (`master-template.md`), and the prompts' own cross-referencing banners: `internal/burlerengine/instruction-{1,2,3}-*-template.md:3` naming `round-orchestrator-template.md`, and `internal/websterengine/fork-prefix.md:1,7` / `recovery-prefix.md:1` naming `implementer-body.md`.
  The plan re-runs the grep rather than trusting this list.
  The banners' cross-references are rewritten to the new names too — free, since the hash is taken over the stripped body, and a banner naming a file that no longer exists is read by a human constantly even though `stripLeadingComment` hides it from the producer.
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
- a stencil whose body adds a top-level marker unknown to its shipped default fails validation with the offending name, while one that deletes a default marker is reported as a warning

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
`diff --all --exit-code` exits non-zero when a board copy differs from the worktree's `stencils/` source and zero when they agree — including zero immediately after a `promote`, which is what makes the run-time warning stop.
Both directions need a test, because a drift check that never fires is a guard that silently passes forever.
Assert too that the warning is emitted via `logger.Warn` and never affects an exit code.

**Seeding concurrency.** Assert that a run whose defaults are unchanged writes nothing and produces no board commit.
Assert the seeding commit carries a positive pathspec covering only the stencils subtree — including the seeded `.gitattributes` — so an unrelated dirty file elsewhere in the board is not swept into it.
This is the regression guard against reverting to a stage-all commit.

**Dev/prod seeding.** The assertions drive `Reconcile`'s explicit mode argument, not a stamped binary, so they stay hermetic.
Assert that dev mode leaves an untouched file whose content differs from the embedded default byte-identical on disk, and that production mode overwrites the same file.
Assert that dev mode still performs the reconciliation row: a board copy whose body equals the dev binary's shipped default but whose stamp names an older one is restamped and reclassified untouched.
Without both directions the thrash reappears silently.
Assert separately that an explicit `lyx stencil sync` from a `-dev`-stamped build *does* perform the refresh — the decided exception, and the one a naive reading of the skip rule would implement backwards.

**Trigger site.** Assert the reconcile pass runs once at the root pre-run rather than per read, that a command with no resolvable hub skips it instead of failing, and that an empty `sourceDir` skips the drift comparison silently rather than erroring.
The import direction is worth a guard of its own: `internal/stencilstore` must not import `internal/fabricengine`, since a lazy-read implementation would satisfy every behavioural test above while quietly putting `fabricengine` on treadle's stack.

**Drift warning in a consumer repo.** Assert it is silent when no `stencils/` source tree exists, in contrast to `promote`/`diff --all`, which error.

**Non-loomyard CLI.** Assert `promote` and `diff --all` error, rather than no-op or create a directory, when no `stencils/` source tree is present.

**Diff base recovery.** Assert the history walk finds the forked-from revision for a file stamped from an older default, and that an unrecoverable base (no matching revision) reports itself explicitly rather than rendering an empty diff.
The empty-diff case is the dangerous one, since it reads as "you are up to date".

**Mutation record.** Assert the seeding verb's record is empty on a no-op run and carries `file_written` plus `commit_created` on a run that actually seeded.

**Banner stripping.** Assert a composed webster prompt contains no `lyx-stencil:` line and no `<!--` at all — the regression guard for the stamp leaking into a live prompt.

**Hash normalisation.** Assert a body written with CRLF line endings hashes identically to the same body with LF, in both the untouched-detection and base-recovery paths.
This is the one that silently disables the entire mechanism on a Windows checkout if it regresses.

**Registry completeness.** Assert the `stencils` package's registry and its `.md` tree name the same set in both directions, so a file added without registration fails the build rather than going invisible.

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
- **Q:** Does `Bolt.Sync` serialise the seeding write against board's own writes? **A:** No — `Bolt.Sync` takes `board.push.lock` while board writes take `board.lock`, and `Bolt.Commit` stages everything. Seeding gets its own `fabricengine` verb taking `board.lock` and committing a positive pathspec, and `board.lock`'s name becomes single-declarer in `fabricengine`.
- **Q:** The board copy is hub-wide but `stencils/` is per worktree. Doesn't the guard then fire in worktrees that changed nothing? **A:** Yes, and that is accepted, because it only ever prints. `promote` is the real mechanism; the warning is a backstop.
- **Q:** Dev and prod binaries carry different embedded defaults against the same hub. What stops them rewriting the same untouched file in opposite directions every run? **A:** A `-dev` build seeds absent files but never refreshes untouched ones, warning instead. Only the refresh row is skipped — the reconciliation restamp still runs, since it is what closes the promote round trip the dev binary exists for.
- **Q:** Where does `diff <name>`'s base text actually come from, mechanically? **A:** A new go-git blob-read verb in `internal/gitrepo`, wrapped by a board-scoped `fabricengine` accessor. The key is the file's stamp: walk the path's history newest-first and take the first revision whose stripped-body hash matches. No match reports itself rather than rendering an empty diff.
- **Q:** Doesn't setting `core.hooksPath` collide with fabric's own hook installer? **A:** It would — fabric resolves its hooks dir with `git rev-parse --git-path hooks`, which honours `core.hooksPath`. Moot now: there is no hook at all.
- **Q:** Isn't all of this a lot of overhead to fire on every run? **A:** Measured on the real files: 69 KB across 15 stencils, and one full read + LF-normalise + hash pass costs about 0.15 ms, against an LLM call taking seconds. The only thing that cost real time was the pre-commit hook's process spawn per commit, which is why it was dropped in favour of a run-time warning.
- **Q:** Does an explicit `lyx stencil sync` refresh from a `-dev` build? **A:** Yes. The dev skip exists to stop incidental thrash, not to refuse an explicit request — and the dev binary is the one used in the test-live loop.
- **Q:** How does the binary know it is a dev build, and what is an unstamped binary? **A:** `-ldflags -X` set by `tools/deploy -dev`. Unstamped — plain `go build`/`go install`, or a test binary — counts as production, since converging on shipped defaults is the safe default and dev must opt in.
- **Q:** Once `implementer-body.md` carries a stamp, does it leak into webster's composed prompts? **A:** Yes — `stripLeadingComment` drops only the first banner of a joined pair, so the stamp would be delivered as instruction text. `internal/stencil` exports its stripper and `joinTemplateAssets` strips every asset, which is what makes the stamp safe to add. Note that file has no banner today, so this task creates the hazard rather than inheriting it.
- **Q:** What happens to the hashes on a machine with `core.autocrlf=true`? **A:** Without a rule, every stencil is classified human-edited forever and never refreshed. Hashing is over an LF-normalised body, the board's stencils tree is seeded with its own `.gitattributes`, and loomyard's `.gitattributes` gains the 15 new paths and loses the 8 stale ones. The base-recovery path needs the same normalisation for the mirror-image reason: go-git returns stored blob bytes untouched while the on-disk copy went through CLI git's conversion.
- **Q:** Who owns the name → default registry, given typed vars rather than an `embed.FS`? **A:** The `stencils` package itself, beside the vars, consumed only by `stencilstore`. A test asserts registry and `.md` tree match in both directions, so a hand-maintained map cannot silently omit a file.
- **Q:** Sandbox coverage — scenario or exclusion? **A:** A `**Covers:** stencil` scenario in `SANDBOX-CORE-SUITE.md`. None of the three existing exclusion reasons applies to a read-only `list`/`validate`.
- **Q:** Does a deleted top-level marker break `Fill`? **A:** No, the opposite — an *added* or renamed marker breaks it; a deleted one fills cleanly and silently drops that content. An earlier draft had this backwards. `validate` compares the body's marker set against the shipped default's: extra marker is an error, missing marker a warning.
- **Q:** Where does the seed/refresh pass actually run? **A:** Once per process at `cmd/lyx`'s root pre-run, never lazily inside `Read`. A lazy pass would put `fabricengine` on treadle's stack via `runTriage`/`runTargeting` and their siblings, defeating the very allowlist amendment it is justified against. `stencilstore` writes files and returns the list; the composition root hands that to the `fabricengine` commit verb.
- **Q:** Does the seeding verb push? **A:** No — it commits only and rides board's next push. Pushing per run would fire on nearly every invocation.
- **Q:** `lyx stencil`'s kernel is `stencilstore`, not `stencilengine`. Doesn't that break the CLI/Cobra naming rule? **A:** Yes, and it is recorded as a named deviation in the same CONSTRAINTS bullet. `stencilengine` would be a third package one character from `internal/stencil` and top-level `stencils`.
- **Q:** The loomyard loop ends in a hand-copy from the board copy back into `stencils/`. What stops a real edit becoming permanently invisible to the source tree? **A:** Nothing, as originally written — raised by the orchestrator review. Resolved by making the port-back mechanical (`lyx stencil promote`) plus a run-time `logger.Warn` on drift. CI cannot be the guard, since a CI runner has no access to the operator's hub.
