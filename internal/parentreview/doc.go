// Package parentreview owns the on-disk exchange between a task's Discussion-Write session and its parent's reviewer.
// It is a told-geometry package: every path arrives on Store, and it imports the standard library, internal/state, internal/logger and internal/shuttleengine only.
//
// Rounds live under Store.Root as round-<N>/, each holding:
//
//	request.json   slug, round, reviewer full name, opened-at, the two discussion paths, a state (open, expired, superseded) and the reject cap the gate held (cap, absent when none)
//	brief.md       the rendered reviewer brief
//	delivery.json  delivered-at or the last failure reason, gate prompts carried (a prompt held back while the reviewer has no live session is not carried, so is not counted), last-prompt time, waiting notifies, cap-Warn flag, keep-alive pings carried and last-ping time
//	verdict.json   approve or reject, recorded-at, consumed, superseding (set only on an approve that replaced the cap's reject)
//	review.md      the copied review file
//	alignment.json the alignment agent's attempt directory, strand, spawned-at, state (running, applied, fallback, stopped), report and reason
//
// Every write goes through internal/state under a per-round lock file in Store.LockDir, so the driver and a verb in another process never interleave a read-modify-write.
// Rounds repeat after a reject until the cap, which the store decides by counting rejected rounds on disk;
// at the cap the gate halts the run `blocked` until the parent approves.
// While a delivered request waits with no verdict, the gate pings the idle writer every GateConfig.KeepAlive, inside the writer's prompt-cache lifetime.
// It pings at most three times per round, so a late reject resumes the writer on a warm cache.
// A ping spends no gate prompt and no waiting notify, and a zero KeepAlive turns pings off.
// NewExpiryProbe is the gate's read-only companion.
// It answers whether the latest round's request is open, has no verdict and is older than GateConfig.WaitBound.
// It reads the round without its lock and writes nothing, so the wait loop can call it while the writer is mid-turn.
// Only the gate and final closures mark a request expired, when they next read the round.
// With GateConfig.Aligner set, the gate spawns the alignment agent when a round opens, before the liveness hold and before any delivery, and polls it on every evaluation.
// The agent's approve or reject is recorded as the round's verdict through the store; an escalation, a timeout or any failure marks the record fallback, rewrites brief.md and delivers to the reviewer as before.
// A verdict from the parent, the wait bound or the writer's end stops a running agent first, so the parent's verdict stands, and a record that is no longer running is never polled again.
// While the agent runs, the gate carries no prompt, and the keep-alive clock runs from the request's opening.
// Verbs, the gate closure and status act on the latest round only.
package parentreview
