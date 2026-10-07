// Package batcher groups a plan's flat card list into the execution units webster forks each run: a
// library of batchifier implementations behind the Batcher interface, a registry mapping each
// batchifier kind to its constructor, and Active, the config entry point callers reach for.
// Batching — how many cards land in one fork,
// and in what grouping — is a standalone step webster consumes today, and one Shed will drive as
// producer #8 once built.
// It is never the plan's decision (a plan-format card carries no batch-membership field of its
// own) and never an LLM's decision (no batchifier consults a fork's judgment; grouping is pure
// orchestrator-side logic over the parsed Card list).
//
// batcher.yaml holds named profiles under profiles:, each a batchifier kind (identity or cost) with
// that kind's parameters; its active: key names the profile in force (see
// contracts/specs/loom-plan-spec.md).
// profiles: is an open map, keyed by names the operator chooses, so lyx config reconcile carries
// the operator's profiles whole; the template's identity and cautious profiles fill in only when
// the file holds no profiles: key.
// Active resolves the active profile at config-load time and builds it through the registry.
// An empty active: and active: identity resolve to the identity batcher even when no profile of
// that name is configured, so a batcher.yaml without profiles: keeps working; a configured profile
// named identity wins over that default.
// A load error names batcher.yaml and the offending profile or key: an active: naming no profile,
// an unknown batchifier kind, or a cost profile with a missing or non-positive alone_above or
// budget, a max_cards below 2, or a missing, negative or unknown weights coefficient.
// The template ships active: empty, so no run groups cards until the operator names the cautious
// profile.
//
// The identity batcher (identity.go) — one card, one batch — is one library entry among future
// grouping batchers, not a "v0" or interim implementation: it ships production-ready from day one,
// and the Batcher interface exists precisely so that later grouping batchifiers (e.g.
// one batch per dependency-free card cluster) drop into the registry without any change to
// webster's call sites.
// No type, file, or identifier in this package carries a version suffix.
//
// The estimator (estimate.go) prices a run of cards without running them, so a cost-model
// batchifier can compare groupings.
// A SizeSource supplies the worktree facts it weighs: a file's line count and a directory's
// test files; DiskSizes reads them from a told worktree root.
// A card's read set holds one entry per target and Uses ref that planparser.RefFile maps to an
// existing file, weighing its lines times context_per_line, plus a package entry and a tests entry
// for each directory the card's targets live in, each weighing package_context.
// A card's messages are target_messages per target (a Rename pair counts once), test_file_messages
// per test file in its target directories and uses_messages per Uses entry.
// SegmentCost of cards 1..n is fork_messages x startup_context, plus for each card k its messages
// times startup_context plus the weight of the distinct read-set entries of cards 1..k, so cards
// sharing files cost less together while a card that adds a large file makes every later message
// in the fork dearer.
// A card's own estimate is the SegmentCost of its one-card segment.
// The coefficients are a Weights value, read by ProfileWeights from a profile's weights: map in
// batcher.yaml.
//
// The cost-model batchifier (cost.go), built by NewCost from CostParams, splits the card sequence
// into contiguous batches of the lowest total SegmentCost.
// Cards share a fork only inside a contiguous segment of the given order, so card order and every
// forward dependency are kept.
// A one-card segment is always feasible; a segment of two or more cards is feasible only when no
// member's own cost exceeds AloneAbove, it holds at most MaxCards cards and its SegmentCost stays
// within Budget.
// A card over AloneAbove or Budget therefore runs alone, as under identity, so no card costs more
// than today, and no threshold value makes the split infeasible.
// An exact dynamic program over the segments ending at each card finds the minimum, in
// O(cards x MaxCards) segment evaluations; a tie between splits goes to the one with more batches.
// Each batch carries the profile name and its SegmentCost as its estimate.
package batcher
