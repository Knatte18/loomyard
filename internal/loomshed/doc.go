// Package loomshed owns loom's own producer constructors, its durable row names, its status seeder, and its own cancellation helpers.
// Loom's ordered producer list itself moved to contracts/recipes/loom-recipe.yaml; internal/loomrecipe is what assembles a *shedengine.Shed from it.
// It takes told absolute paths and has no direct production import of internal/lyxcwd -- see PATTERN-cwd-resolution.
//
// The stops its gate rows halt on each name a way forward, as contracts/specs/refusal-spec.md defines one.
// Loom-Preflight's half-finished-run stop names `lyx loom goto` as that way forward.
//
// Loom-Preflight also loads `batcher.yaml` through the layer stack under the told `configengine.Dirs` after a passing seed check, as the Webster row does when it partitions the plan, so a stale file in any layer stops the run before any LLM row.
// A retired key names the way forward its layer file calls for, `lyx config reconcile --apply` for a worktree or local file and `lyx fabric reconcile` for the repo default, which a session lyx refuses the verb from reports status: FAILED and the orch runs;
// any other load fault names the fix-and-re-step one.
// The check reads and never writes, and validates no other module's config.
//
// Its plan gates resolve plan refs through a told planindex.Index, so the package links no tree-sitter grammar.
// The plan gate and `lyx loom validate-plan` share ValidatePlan, which reads webster's run record under the anchor and tells the index the cards of every batch it holds done.
// A twill Plan-Write run's gate and `lyx loom validate-plan --lens-review` share ValidatePlanLensReview, which adds the presence check on `lens-review.md`.
// Those cards are history, not targets: a run moved back to the plan review after Webster executed batches has their work in the tree, so their tree-dependent checks are skipped.
// Every other card, and every card of a plan with no run record, is checked against the tree.
// A done card is also frozen: ValidatePlan appends a blocking `done-card-edited` finding for each one whose file no longer hashes to what its batch recorded at begin, naming the follow-up card as the way forward.
// A plan with no run record or no recorded hashes reports nothing, and an unreadable card file is a returned error.
//
// Every plan gate, and every `validate-plan` mode, also takes a PlanContext and appends blocking checks outside planparser.
// `rename-tool-missing` is one finding per symbol `Rename` pair of a card not recorded done when the context's rename tools are empty, with `lyx config language` as the way forward.
// `pattern-changes-mismatch` is one finding per `## PATTERN changes` bullet that the overview and the context's decision record do not both declare.
// An absent record adds nothing, and a malformed record section is a returned error naming `lyx loom validate-discussion`.
// The zero PlanContext runs neither of those two.
//
// Four PATTERN checks follow them, in this order, each reading `PATTERN.md` in the worktree and the overview PlanContext.PatternBase returns for the task branch's merge base, or for HEAD with no parent recorded.
// `pattern-removes-prescribed` is one finding per removed entry that a card not recorded done fences a line for.
// `pattern-headroom` is one finding when the overview projected from every card's fenced entry lines and the removed entries grows past the headroom line.
// `pattern-card-headroom` is one finding per card not recorded done whose own step grows past the line, the bound its card gate's `lint-pattern` step applies:
// the projection starts at the worktree overview and takes those cards in plan order, each applying its fenced entry lines and dropping the removed entries it is the first to name in backticks, and a removed entry no such card names is dropped before the first.
// `pattern-line-unprescribed` runs only with a decision record path, and is one finding per added or changed entry that no card fences a line for and whose bullet equals the base's.
// A worktree with no `PATTERN.md`, or a nil PatternBase, runs none of the four and reads no base.
//
// A plan gate that passes on informational findings alone records them as a count and a file path in its log line, and writes the findings, one per line, to `<gate>-informational-findings.txt` under the run's told ephemeral scratch directory.
// A gate told no scratch directory, such as a standalone validate, and a gate whose file cannot be written keep the findings inline in the log line.
// The blocking path is unchanged: the findings go to the writer and the log line in full.
//
// The verify gate (NewVerifyGate) is told its verify command by the recipe's wiring and reads it at each arrival; it never parses the plan itself.
// The command is the row's own `lyx gate test --wait --gate <key>` call, the `code-burler-fix` key at Code-Burler-Fix and the `darn-write` key at Darn-Write, so each fix row and writer runs its own key and shares no pass with Webster's end gate.
// Told a tree scope reader, it takes the darn form; otherwise it takes the round form.
// The form is a told reader, never inferred from the merge base reader's presence.
// Told a merge base, the round form first runs the headroom lint from it to HEAD, before the comment lint and any test; told none, it skips that lint.
// The round form lints the comments added since the told command's latest recorded pass when that commit is an ancestor of HEAD, else since the merge base, so a first round lints rather than skips; with neither base it runs no lint.
// It then runs the told command as it is through verifytree.Verify: the gate derives its own diff base, narrowing, fallback to whole and guard step.
// Webster's gate, Publish and Finalize keep the full verify on the tree that lands.
// A failed run's findings carry the exit code, the log path and the log's tail, and when the whole log holds the CPU watchdog's line that `lyx gate test` prints after it quits a busy test binary, a paragraph naming the window.
// A watchdog quit of a healthy package has no code fix: the way forward is raising `busy_quit_sec` through `lyx config gate` from the prime, then re-stepping the row.
// The gate runs no flake rerun.
// The told command runs the tiers its key lists and nothing else, so `tmux` and `llm` tiers run only if the hub lists them in that key.
// Publish reruns both verifies in full regardless, and stays the guard.
// Told a tree scope reader, it takes the darn form the darn recipe's `Darn-Write` and `Code-Burler-Fix` rows run, and requires the merge base reader:
// the headroom lint and then the comment lint run from the task branch's merge base with the parent to HEAD, so every attempt diffs from the merge base and is never narrowed to a stale pass.
// The tree scope reader is then read at each arrival.
// A tree-scoped diff, wholly under the hub's configured non-production trees, runs only the tree-scoped command the reader answers; an empty command, as for a docs-only diff whose trees name no target, runs nothing, logs so and passes, so an empty tree scope never falls back to the computed scope.
// Any other diff runs the told command.
// The told command is the key's own call in every case, so a pass keeps its record entry and a later arrival on the same tree and command skips.
// That form has no skip path: an empty told command, a command read error, a merge base read error and a tree scope read error are each the gate's returned error.
// An impact-scoped pass lets through a break only the whole module shows;
// Merge-In's whole run catches it for every diff touching production code.
//
// The Plan-Write rotation archives the prior plan and appends a prior-plan block naming that archive to the respawned session's prompt.
// It first archives webster's run record into the archive's `webster` subdirectory through a told seam, so the next Webster run starts a new run over the new plan instead of refusing the old record's plan drift.
// A run holding webster's run lock refuses the archive before any plan file moves, and ArchivedPlanWebsterDirs lists the subdirectories a rotation writes the record into.
// The Plan-Review and Code-Review run directories stay out of that rotation.
//
// `_lyx/plan/`'s top-level files hold exactly one plan generation.
// A retired generation lives only under the rework round that retired it (`round-<N>/prior-generation/`), or in the Plan-Write rotation's archive above.
// Only PR-Rework archives a generation into a round, and this package declares the round layout once, building every path from the existing accessors and telling webster its archive destination.
// Webster's run record and the Plan-Review and Code-Review run directories are generation-scoped: they move into the round with the plan, so the next generation starts with none.
//
// The Darn-Write row's producer (NewDarnWrite) wraps the gated darn writer session and tells it a prior-work note, empty when absent.
// The note, written in Go, says the change is already under way on the task branch and names the latest Darn-Write history entry's halt reason.
// On a non-empty output pointer, Done or a gate-failed Stuck, it commits the landing directory.
// A gate-failed Stuck keeps its record and gains the resume way forward.
// A failed commit is a returned error, since re-running the session cannot fix a git or filesystem fault.
//
// A rework round a run recorded before PR-Rework went stays readable.
// PlanReviewSkippable classifies the live plan generation from the committed round records, and answers false while the plan amendment record exists, so the amended plan is judged and its settle records a round start.
// ArchivedWebsterDirs lists each round's archived webster run record.
//
// It declares its own unexported cancellation helpers (entryErr/cancelErr in ctx.go) rather than
// reusing internal/shedadapters' identically-shaped, unexported ones: shedadapters' versions are
// unexported and Scope forbids changing that package, so every real producer written here honours
// the two obligations Shed cannot enforce -- return exactly Done, Stuck or Awaiting, and surface
// context cancellation as a non-nil error, never as Stuck -- with its own copies. The duplication is
// deliberate, not an oversight.
package loomshed
