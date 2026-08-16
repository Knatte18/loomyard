// Package batcher groups a plan's flat card list into the execution units webster forks each run: a
// library of batchifier implementations behind the Batcher interface, a name-keyed registry those
// implementations self-register into, Select, which resolves a batcher by name, and Active, the
// config entry point callers reach for.
// Batching — how many cards land in one fork,
// and in what grouping — is a standalone step webster consumes today, and one Shed will drive as
// producer #8 once built.
// It is never the plan's decision (a plan-format card carries no batch-membership field of its
// own) and never an LLM's decision (no batchifier consults a fork's judgment; grouping is pure
// orchestrator-side logic over the parsed Card list).
//
// The active batcher is chosen via batcher.yaml's active: config key (see
// contracts/specs/loom-plan-spec.md), which Active resolves against the registry at config-load time.
// An empty key resolves to DefaultName, the identity batcher.
//
// The identity batcher (identity.go) — one card, one batch — is one library entry among future
// grouping batchers, not a "v0" or interim implementation: it ships production-ready from day one,
// and the Batcher interface exists precisely so that later grouping batchifiers (e.g.
// one batch per dependency-free card cluster) drop into the registry without any change to
// webster's call sites.
// No type, file, or identifier in this package carries a version suffix.
package batcher
