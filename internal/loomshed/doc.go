// Package loomshed owns loom's own producer constructors, its durable row names, its
// status seeder, and its own cancellation helpers. Loom's ordered producer list itself moved to
// contracts/recipes/loom-recipe.yaml; internal/loomrecipe is what assembles a *shedengine.Shed from
// it. It takes told absolute paths and has no direct production import of internal/lyxcwd -- see
// PATTERN-told-geometry.
//
// The stops its gate rows halt on each name a way forward, tabulated in contracts/specs/refusal-spec.md.
// Loom-Preflight's half-finished-run stop names `lyx loom goto` as that way forward.
//
// Loom-Preflight also loads `batcher.yaml` from the told config base directory after a passing seed check, as the Batchifier row does, so a stale file stops the run before any LLM row.
// A retired key names `lyx config reconcile --apply` as the way forward, any other load fault the fix-and-re-step one.
// The check reads and never writes, and validates no other module's config.
//
// Its plan gates resolve plan refs through a told planindex.Index, so the package links no tree-sitter grammar.
// The plan gate and `lyx loom validate-plan` share ValidatePlan, which reads webster's run record under the anchor and tells the index the cards of every batch it holds done.
// Those cards are history, not targets: a run moved back to the plan review after Webster executed batches has their work in the tree, so their tree-dependent checks are skipped.
// Every other card, and every card of a plan with no run record, is checked against the tree.
//
// A plan gate that passes on informational findings alone records them as a count and a file path in its log line, and writes the findings, one per line, to `<gate>-informational-findings.txt` under the run's told ephemeral scratch directory.
// A gate told no scratch directory, such as a standalone validate, and a gate whose file cannot be written keep the findings inline in the log line.
// The blocking path is unchanged: the findings go to the writer and the log line in full.
//
// The verify gate (NewVerifyGate) is told its verify command by the recipe's wiring and reads it at each arrival; it never parses the plan itself.
// Told no merge base, it takes the round form the Webster-Burler row runs, which runs less than the full command.
// It lints the comments added since the told command's last recorded pass, then runs the command impactset derives from that diff through verifytree.Verify, falling back to the told command wherever impactset cannot narrow.
// The narrowing is impactset's; Webster's gate, Publish and Finalize keep the full plan verify on the tree that lands.
// The round compiles the `tmux` and `llm` tiers and never runs `llm`.
// It runs the `tmux` tier only while a checked Publish failure record is present, as the failing tests the record names and, for a `publish_verify` failure, the impacted-set pass, to confirm the fix.
// Publish reruns both verifies in full regardless, and stays the guard.
// Told a merge base reader, it takes the whole-diff form the `Darn` row runs:
// the comment lint runs from the task branch's merge base with the parent to HEAD, and the told command runs in full as the site's base command, so Publish skips on the same tree and command.
// That form has no skip path: an empty told command, a command read error and a merge base read error are each the gate's returned error.
// With a checked Publish failure record present it appends the failing tests and, for a `publish_verify` failure, `./...` under `tmux`.
//
// The Plan-Write rotation archives the prior plan and appends a prior-plan block naming that archive to the respawned session's prompt.
// It first archives webster's run record into the archive's `webster` subdirectory through a told seam, so the next Webster run starts a new run over the new plan instead of refusing the old record's plan drift.
// A run holding webster's run lock refuses the archive before any plan file moves, and ArchivedPlanWebsterDirs lists the subdirectories a rotation writes the record into.
//
// It declares its own unexported cancellation helpers (entryErr/cancelErr in ctx.go) rather than
// reusing internal/shedadapters' identically-shaped, unexported ones: shedadapters' versions are
// unexported and Scope forbids changing that package, so every real producer written here honours
// the two obligations Shed cannot enforce -- return exactly Done, Stuck or Awaiting, and surface
// context cancellation as a non-nil error, never as Stuck -- with its own copies. The duplication is
// deliberate, not an oversight.
package loomshed
