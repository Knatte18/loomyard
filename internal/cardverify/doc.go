// Package cardverify runs the `**Verify:**` lines of a plan's cards, shared by the webster re-run at record-batch and the `lyx gate card-verify` verb a fork runs.
//
// # Runner
//
// A Runner holds the worktree a line runs in, the told gate-slot pool (nil runs unslotted), the directory its wait record goes in and the per-line timeout, zero meaning DefaultTimeout.
// RunLine runs one line through verifyrun.Run and returns how it failed, empty on a pass; RunCards runs each card's line in card order and returns one `card NN-<slug> verify <command> <how>` line per failing card, stopping at the first when told to.
//
// A line IsGateCall accepts is one `lyx gate test` call holding `--wait` before any `--` and no shell control operator.
// It runs unslotted and outside the timeout, since the gate's own pre-tier ceiling and tier timeouts bound it, and it takes its own slots.
// Any other line, a gate call lacking `--wait` included, takes one slot of the pool, writes the wait record `card verify <card>` while it waits with no cap on the wait, and runs under the timeout counted from the moment the slot is held.
// Its environment is gateslot.HolderEnv over the parent's, so a gate call inside it inherits the slot instead of taking a second.
// No line sees gateslot.PrebuiltLyxEnv, because a line's own go test builds lyx once per test binary.
//
// The package runs only lines a plan card already holds, which the plan gate checked; it adds no slot rule beyond those two forms.
package cardverify
