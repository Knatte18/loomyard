// Package cardverify runs the `**Verify:**` lines of a plan's cards.
// The webster re-run at record-batch and the `lym gate card-verify` verb a fork runs share it.
//
// # Runner
//
// A Runner holds the worktree a line runs in and the told gate-slot pool, nil meaning unslotted.
// RunLine runs one line through verifyrun and returns how it failed, empty on a pass.
// RunCards runs each card's line in card order, stopping at the first failure when told to.
// It returns one `card NN-<slug> verify <command> <how>` line per failing card.
//
// Both take the caller's context.
// It bounds the slot wait, so a cancel there fails the line with `could not acquire a gate slot`;
// it bounds the line too, so a cancelled line ends its process group, SIGTERM then SIGKILL.
//
// A line IsGateCall accepts is one `lym gate test` call holding `--wait` before any `--` and no shell control operator.
// It runs unslotted and outside the timeout, since the gate's own pre-tier ceiling and tier timeouts bound it;
// it takes its own slots.
// Any other line, a gate call lacking `--wait` included, takes one slot of the pool.
// It has no cap on the wait and runs under the timeout counted from the moment the slot is held.
// Its environment is gateslot.HolderEnv over the parent's,
// so a gate call inside it inherits the slot instead of taking a second.
// No line sees gateslot.PrebuiltLymEnv, because a line's own go test builds lym once per test binary.
// No line sees agentname.StrandNameEnv,
// so a gate call inside a line runs in-process and never takes the capped form of a strand.
//
// The package runs only lines a plan card already holds, which the plan gate checked.
// It adds no slot rule beyond the gate-call form and the slotted form.
package cardverify
