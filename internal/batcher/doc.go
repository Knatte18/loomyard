// Package batcher groups a plan's flat card list into the execution units webster forks each run: a library of batchifier implementations behind the Batcher interface, a registry mapping each batchifier kind to its constructor, and Active, the config entry point callers reach for.
// Batching — how many cards land in one fork,
// and in what grouping — is a standalone step webster consumes today, and one Shed will drive as
// producer #8 once built.
// It is never the plan's decision (a plan-format card carries no batch-membership field of its
// own) and never an LLM's decision (no batchifier consults a fork's judgment; grouping is pure
// orchestrator-side logic over the parsed Card list).
//
// batcher.yaml holds named profiles under profiles:, each a batchifier kind (identity or cost) with that kind's parameters;
// its active: key names the profile in force (see contracts/specs/loom-plan-spec.md).
// profiles: is an open map, keyed by names the operator chooses, so lyx config reconcile carries the operator's profiles whole;
// the template's identity and cautious profiles fill in only when the file holds no profiles: key.
// Active resolves the active profile at config-load time and builds it through the registry.
// An empty active: and active: identity resolve to the identity batcher even when no profile of that name is configured, so a batcher.yaml without profiles: keeps working;
// a configured profile named identity wins over that default.
// A load error names batcher.yaml and the offending profile or key: an active: naming no profile, an unknown batchifier kind, or a cost profile with a missing or non-positive budget, a max_cards below 2, the retired alone_above, a weights: map still carrying the retired startup_context, or a missing, negative or unknown weights coefficient.
// The template ships active: empty, so no run groups cards until the operator names the cautious profile.
//
// The identity batcher (identity.go) — one card, one batch — is one library entry among future
// grouping batchers, not a "v0" or interim implementation: it ships production-ready from day one,
// and the Batcher interface exists precisely so that later grouping batchifiers (e.g.
// one batch per dependency-free card cluster) drop into the registry without any change to
// webster's call sites.
// No type, file, or identifier in this package carries a version suffix.
//
// The estimator (estimate.go) predicts how full a fork's context gets without running it, so a cost-model batchifier can keep every fork well inside the model's window.
// A SizeSource supplies the worktree facts it weighs: a file's line count and a directory's test files;
// DiskSizes reads them from a told worktree root.
// A card's read set holds one entry per target and Uses ref that planparser.RefFile maps to an existing file, weighing its lines times context_per_line, a package entry and a tests entry for each directory the card's targets live in, each weighing package_context, and the card's own text, its lines times context_per_line.
// A card's messages are target_messages per target (a Rename pair counts once), test_file_messages per test file in its target directories and uses_messages per Uses entry;
// its write allowance is its messages times message_context, the tool calls and output they add, plus its card-text lines times write_per_card_line, the code the card carries that the fork writes back out.
// PeakContext of a segment at a 1-based batch position is master_base plus (position - 1) times batch_growth, plus fork_messages times message_context, plus the weight of the distinct read-set entries of all its cards, plus every card's write allowance.
// A fork inherits the orchestrating session's context, which is master_base when the first fork spawns and grows by batch_growth for every batch that finished before it, so a later batch starts deeper.
// Context only grows within a fork, so that is the context after its last card, its largest;
// a file two cards share counts once, and adding a card never lowers the peak.
// A card's own estimate is the PeakContext of its one-card segment.
// The coefficients are a Weights value, read by ProfileWeights from a profile's weights: map in batcher.yaml.
//
// The template's cautious start coefficients are measured, the in-fork coefficients are not.
// master_base is 52000, the mean measured first-fork start (50.3K to 53.5K), and batch_growth is 7000, the mean growth per batch (2.5K to 13.0K), over three cautious runs and twelve forks with each start measured, one of them in a resumed Merriam session.
// Growth counts batches, not cards: Merriam gains context per fork it spawns and records, whatever the fork's size.
// The in-fork coefficients were fitted under the old constant start of 60000, and await a re-fit against the position model.
// That fit came from tools/tokencount's calibration, which set each one-card estimate beside the measured peak context of the fork that ran the card (the largest input plus cache tokens of any of its messages) over 173 one-card forks of past runs;
// the card text is not in a run's base tree, so write_per_card_line and the card-text read are unfitted.
// Each batch records its estimate's components (Breakdown), which webster keeps in state.json, so later runs can fit them all.
//
// The cost-model batchifier (cost.go), built by NewCost from CostParams, splits the card sequence into the fewest contiguous batches that fit.
// Cards share a fork only inside a contiguous segment of the given order, so card order and every forward dependency are kept.
// A one-card segment is always feasible;
// a segment of two or more cards is feasible only when it holds at most MaxCards cards and its PeakContext at its own batch position stays within Budget.
// A card over Budget at its position therefore runs alone, as under identity, with nothing halting or warning, and no threshold value makes the split infeasible;
// the recorded Estimate and Position show it.
// Among the splits with the fewest batches it takes the one whose largest batch peaks lowest, so batches come out balanced;
// a remaining tie leaves the last batch shortest.
// The k-th batch of the result is at position k, so a segment's peak depends on the batch it forms.
// An exact dynamic program over (batches so far, last card placed) finds the split in O(cards² x MaxCards) segment evaluations, accumulating each segment's position-independent peak one card at a time and adding the position term per batch count.
// Each batch carries the profile name, its PeakContext at its own position as its estimate and the estimate's Breakdown, which records that position.
package batcher
