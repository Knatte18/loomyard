// Package battenshed owns the four task-worktree batten producers: creating the task worktree,
// seeding its own inner run, running that run inside it to a terminal state, and tearing it down.
// Any producer list may name them by reference, the same way internal/landingshed frames its own
// two producers.
//
// Told-geometry tier: this package takes every absolute path it operates on from its caller and
// has no direct production import of internal/lyxcwd, per the Told-Geometry Invariant
// (CONSTRAINTS.md). Its seam_enforcement_test.go enforces that membership mechanically.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary
// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
// by name.
//
// A driver strand that dies mid-run, that is alive but parked -- a provider waiting on an
// interactive prompt its launcher never answered -- or that stops of its own accord with the run
// still non-terminal -- an llm driver handing back on a refusal it may not retry, exhausting its
// step cap, or finding its skill unavailable, each leaving its report under the task worktree's
// .lyx/shed/self/ -- is not detected here, by design: the InnerRun row watches the child's
// persisted status file, never the driver's own liveness or progress. A long-quiet Run-Shed
// therefore means "possibly dead, parked or stopped", not "working", until the row's bounce budget
// runs out; an operator tells the cases apart by attaching to the child's session.
//
// Its counterpart is equally by design: a driver that finishes NORMALLY leaves its strand and its
// run directory behind. Nothing here tears either down as part of a clean finish -- only the
// whole-worktree teardown row cleans up, at the very end, by removing the worktree they live in.
//
// It declares its own unexported entryErr/cancelErr helpers (ctx.go) and its own reportStuck
// carrier (stuck.go) for the same deliberate-duplication reason internal/preflightshed/doc.go and
// internal/landingshed/stuck.go already record: each producer-owning package carries its own copy
// rather than sharing one across packages it otherwise has no reason to depend on.
package battenshed
