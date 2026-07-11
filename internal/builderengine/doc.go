// Package builderengine is the domain kernel behind Loom's Builder phase: the
// batch-implementation loop that drives a pinned plan-format v1 plan (see
// docs/modules/plan-format.md) through implementer sessions, batch by batch,
// until the plan is built. builderengine holds no loop itself — the loop is
// an LLM orchestrator session driving fat `lyx builder` verbs
// (`internal/buildercli`), and this package provides only those verbs' logic
// plus the distillation behind them: plan parsing and validation, run state,
// spawn/poll classification, digest computation, chain rollback, and the
// pause flag.
//
// # LLM orchestrator over fat Go verbs
//
// A long-lived orchestrator session (model config-chosen, Sonnet default)
// holds the batch loop; Go never iterates batches or makes orchestration
// decisions itself. This mirrors mill-go's lesson the hard way: mill-go's
// context bloat came from an LLM orchestrator swallowing verbose sub-agent
// output, not from the loop being LLM-held. So the orchestrator here reads
// only a distilled digest per batch (see the `poll` verb) — it never ingests
// raw implementer session prose. Recovery from a stuck batch is the
// orchestrator's judgment call, never a Go branch; a fresh escalated
// recovery session is spawned to retry a stuck batch, never a `/model`
// switch inside the polluted session.
//
// # No DAG — a strictly ordered batch list
//
// A plan is an ordered sequence of batches with no dependency graph and no
// intra-plan parallelism: batch N may assume batches 1..N-1 are already
// committed. mill's DAG existed only to support parallelism that was never
// exercised; parallelism belongs one level up, as separate tasks in separate
// worktrees.
//
// # advance vs. converge — builder's sibling relation to perch
//
// builder is the ADVANCE half of a pair whose CONVERGE half is
// internal/perchengine: builder drives implementers through a plan until the
// last batch is green (or the run reports stuck/paused), then stops — it
// performs no terminal holistic review itself. The holistic Builder-review
// is a separate perch gate, driven by loom or the operator running
// `lyx perch run` after `builder run` finishes. Keeping the two split lets an
// LLM orchestrator drive the batch loop without ever touching perch's
// block-exit weft-committing discipline.
//
// # engine/cli split, and the weft-ownership asymmetry
//
// builderengine is geometry-AWARE: it resolves `_lyx/plan` and
// `_lyx/builder` paths itself via the internal/hubgeometry helpers
// (PlanDir/BuilderDir/BuilderReportsDir), because those paths are part of
// the pinned plan-format contract, not caller-supplied. This is the one
// documented difference from perchengine's pattern (which is fully
// geometry-blind). builderengine is nonetheless weft-BLIND: every weft
// commit of a builder artifact (a batch report, state.json, outcome.yaml)
// happens in internal/buildercli, never here — mirroring perchcli's
// block-exit weft Commit+Push discipline. The orchestrator and implementer
// agents never run weft git themselves (the Weft Git Invariant); an
// implementer DOES commit its own code to the host repo, once per card — the
// documented asymmetry.
package builderengine
