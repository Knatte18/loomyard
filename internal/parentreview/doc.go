// Package parentreview owns the on-disk exchange between a task's Discussion-Write session and its parent's reviewer.
// It is a told-geometry package: every path arrives on Store, and it imports the standard library, internal/state, internal/logger and internal/shuttleengine only.
//
// Rounds live under Store.Root as round-<N>/, each holding:
//
//	request.json   slug, round, reviewer full name, opened-at, the two discussion paths, a state (open, expired, superseded) and the reject cap the gate held (cap, absent when none)
//	brief.md       the rendered reviewer brief
//	delivery.json  delivered-at or the last failure reason, gate prompts carried (a prompt held back while the reviewer has no live session is not carried, so is not counted), last-prompt time, waiting notifies, cap-Warn flag
//	verdict.json   approve or reject, recorded-at, consumed, superseding (set only on an approve that replaced the cap's reject)
//	review.md      the copied review file
//
// Every write goes through internal/state under a per-round lock file in Store.LockDir, so the driver and a verb in another process never interleave a read-modify-write.
// Rounds repeat after a reject until the cap, which the store decides by counting rejected rounds on disk.
// Verbs, the gate closure and status act on the latest round only.
package parentreview
