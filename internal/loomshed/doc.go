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
// A done card is also frozen: ValidatePlan appends a blocking `done-card-edited` finding for each one whose file no longer hashes to what its batch recorded at begin, naming the follow-up card as the way forward.
// A plan with no run record or no recorded hashes reports nothing, and an unreadable card file is a returned error.
//
// The Webster-Burler round gate (NewVerifyGate) runs less than the plan's `## verify:`.
// It lints the comments added since the plan verify's last recorded pass, then runs the command impactset derives from that diff through verifytree.Verify, falling back to the plan's own command wherever impactset cannot narrow.
// The narrowing is impactset's; Webster's gate, Publish and Finalize keep the full plan verify on the tree that lands.
// The round compiles the `tmux` and `llm` tiers and never runs `llm`.
// It runs the `tmux` tier only while a checked Publish failure record is present, as the failing top-level tests the record names, one step per package and test with the subtests collapsed into it, and, for a `publish_verify` failure, the impacted-set pass, to confirm the fix.
// Publish reruns both verifies in full regardless, and stays the guard.
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
