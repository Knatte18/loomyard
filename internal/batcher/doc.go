// Package batcher groups a plan's flat card list into the execution units webster
// forks each run: a library of batchifier implementations behind the Batcher
// interface, a name-keyed registry those implementations self-register into, and
// Select, which resolves the config-chosen active batcher by name. Batching — how
// many cards land in one fork, and in what grouping — is 100% webster's own
// execution-policy decision. It is never the plan's decision (a plan-format-v3 card
// carries no batch-membership field of its own) and never an LLM's decision (no
// batchifier consults a fork's judgment; grouping is pure host-side logic over the
// parsed Card list).
//
// The active batcher is chosen via webster.yaml's batcher: config key (see
// docs/reference/plan-format-v3.md and websterengine's config loading), which
// Select resolves against the registry at config-load time. An empty key resolves
// to DefaultName, the identity batcher.
//
// The identity batcher (identity.go) — one card, one batch — is one library entry
// among future grouping batchers, not a "v0" or interim implementation: it ships
// production-ready from day one, and the Batcher interface exists precisely so that
// later grouping batchifiers (e.g. one batch per dependency-free card cluster) drop
// into the registry without any change to webster's call sites. No type, file, or
// identifier in this package carries a version suffix.
package batcher
