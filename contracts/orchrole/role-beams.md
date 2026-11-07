<!-- This is the orch role's on-demand file for making and landing a beam.
     RenderRoleFile (internal/orchengine/prompt.go) renders it beside the role; it carries no markers. -->
# Beams

A beam is a board project entry made into a worktree of its own, which runs its own dobby and orch.

## Making a beam

Make one on the operator's go-ahead, from the prime or, for a nested beam, from the parent beam.
Never make one from a task worktree:

1. Write the entry with `lym board upsert` and `"kind":"project"`.
2. Run `lym beam create <slug>`.
   `--anchor <dir>` gives the beam its own anchor.
   The anchor is a path relative to the code repo root, tracked in the commit the beam forks from;
   without the flag the beam inherits its parent's anchor.

Dobby starts no project entry, so a release never makes a beam.
The verb starts a batten run for the project.
Its Worktree-Create row makes the worktree, unmarked, and its Seed-Child row seeds the pawl run inside it.
The pawl run's Beam-Up row marks the worktree a beam and starts the beam's dobby and orch,
so the beam's orch appears only after Beam-Up, and the operator attaches to it later than the create.
A task run has the same batten rows, but its inner run has no Beam-Up, so its worktree never becomes a beam.
`lym beam start <slug>` brings an existing beam's dobby and orch back up;
it refuses a task worktree and never makes a beam.

## Landing a beam

The orch of a beam works the beam's landing at Beam-Await with the operator.
When the operator says so, run:

- `lym beam describe <slug>` writes the beam's change description.
  It draws on the entry's title and brief, the beam's landings and the decisions of its project record.
  Change the description only through the entry's title and brief with `lym board upsert`.
  Then run `lym beam describe <slug>` again; never edit `summary.md` by hand.
- `lym beam pr open <slug>`, `update`, `status` and `close` for the beam's pull request into its parent.
  `status` reads whether it can merge now and what stops it, and changes nothing.
- `lym beam merge-parent <slug>`, which brings the parent in.

On a dobby notice naming landings into the beam, run `lym beam describe <slug>`, then `lym beam pr update <slug>`.

Never merge a pull request, and never close one to add work.
Work lands into the beam while the pull request stays open.
Close one only when the operator chooses to land without it.

Run `lym beam ready <slug>` only when the operator is satisfied.
It is final: no verb takes the beam back to Beam-Await, and a member filed after it never starts.
It refuses what you can still fix, each refusal naming its way forward.
Follow a halted Finalize with `lym beam start <slug>`, which brings your orch back and leaves the dobby down.
Then run `lym loom resume` in the beam's worktree after the fix the halt names.

## A project that ran as a task run

A project that runs as a task run is no beam, and no repair of its worktree makes it one.
`lym batten status <slug>` reports the same rows, Worktree-Create through Run-Shed, for a task run and a beam,
so it cannot tell them apart.
The entry's `kind` does (`lym board get <slug>`, project versus task),
and so does the inner run's current producer, read with `lym loom status` in the worktree:
a task run's is a tapestry, motif, darn or sampler row, a beam's is Beam-Up or a later pawl row.
The way forward is `lym batten cancel <slug> --remove` on the operator's word, then `lym beam create <slug>`.
Never repair a run with `lym fabric add`, which gives the wrong run a worktree back.
Never delete a worktree directory by hand.
When lym refuses a step, stop and tell the operator what it refused.
