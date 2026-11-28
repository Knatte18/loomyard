<!-- This is the orch role's on-demand file for making and landing a beam,
     and for moving a running task to another parent.
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

- `lym beam describe <slug>` records the beam's change description, which you write.
  `lym beam describe <slug> --draft` first writes a starting point, `summary.md` in the beam's `_lym/landing` directory,
  from the entry's title and brief and every member pull request.
  Edit that file, then run `lym beam describe <slug>`;
  with a pull request recorded, it also updates the pull request.
- `lym beam pr open <slug>`, `update`, `status` and `close` for the beam's pull request into its parent.
  `status` reads whether it can merge now and what stops it, and changes nothing.
  `open` refuses while the beam conflicts with its parent or its merge task is not done:
  run `merge-parent` and let its task land first.
- `lym beam merge-parent <slug>`, which brings the parent in.

Write the description for a reader of the pull request:

- Its heading line is the pull request's title, at most 72 characters.
- State the end result, what the beam ended up with, never the way there.
- List every member pull request, so a reader can follow each step to the result on its own.
- Never list the decisions made along the way, from the project record or elsewhere.
- Close it with the `## Merge danger` section the final-summary spec, `loom-final-summary-spec.md`, defines,
  its door and blast radius called for the beam's whole landing; the draft leaves both as placeholders.

Change any part of it whenever you or the operator want, then run `lym beam describe <slug>` again.

While the pull request is recorded, the beam's dobby rewrites the description after each member landing,
and it updates the pull request itself, whatever the member's `beam.land_notice` reads.
A rewrite that fails leaves the recorded description standing, and dobby posts you why;
fix the description by hand and run `lym beam describe <slug>`, or leave it to the next member landing.

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
a task run's is a tapestry, jacquard, motif, darn or sampler row, a beam's is Beam-Up or a later pawl row.
The way forward is `lym batten cancel <slug> --remove` on the operator's word, then `lym beam create <slug>`.
Never repair a run with `lym fabric add`, which gives the wrong run a worktree back.
Never delete a worktree directory by hand.
When lym refuses a step, stop and tell the operator what it refused.

## Moving a running task to another parent

Move a running task only on the operator's word, and only by this procedure.
It uses verbs that exist: no batten verb or recipe row moves a task, and a move is never a recipe step.
It moves a task only through a cancel that completes.
Never move a beam project, a grouping project or a front task this way.
The moved work stays on the same entry and slug, so file no new task for it.

The move note is what the cancelled run learned, written for the fresh run that replaces it.
It holds four parts:

- what is done, naming the cards or decisions it covers;
- the decisions taken, each with its reason;
- what is left;
- where the commits are: the task branch, its tip SHA and its merge base with the old parent branch.

1. Write the move note and hold the run.
   - The live writer is the session of the row the run is in:
     the Discussion writer, the plan writer, or the Webster master during Webster.
     While no pause covers the slug, ask it with `lym creel post --expect-reply <strand> "<text>"`
     for the note's first three parts in its reply text, writing no file.
     Ask before the hold, since a pause that covers a strand holds every delivery into its pane.
     Write the note in your own `.scratch/` from the reply, then hold the run with `lym tender pause <slug>`.
   - When no writer is live, because the run is halted, sits in a review, gate or landing row, or its strand is gone,
     hold the run with `lym tender pause <slug>` and write the whole note yourself in your own `.scratch/`.
     Read the run's decision record, plan and status, and the task branch's log against the old parent.
     A writer that refuses the ask, or replies without the three parts, counts as no live writer.
   - Never hold the run with `lym batten pause`:
     the inner run reads only the pause `lym tender pause` writes, and under a batten pause it keeps making rows.
   - After the hold, add the fourth part yourself.
     The tip is the worktree's tip of the task branch,
     or origin's tip when `git -C <prime> ls-remote origin` shows it strictly containing the worktree's,
     since the cancel then archives origin's tip alone.
     Read the merge base of that tip with the old parent branch.
2. Check with `lym board get` that the new parent can take a member.
   - The root always qualifies.
   - A project qualifies when it is not done and no run holds it:
     its status is no run status (`<state> · <producer>`),
     or, for a beam, its producer is a hold-free row: Worktree-Create, Preflight, Record-Write, Beam-Up or Beam-Await.
   - A beam on which `lym beam ready` has run never qualifies, since it never starts a member filed after it.
     That flag is the ready record `_lym/landing/beam-ready.json` under the beam's anchor,
     committed on the beam's records branch (`<beam records branch>`, its branch name with the records suffix) with no push,
     and it counts only when its `slug` is the beam's slug.
     When the beam's worktree is on your machine, the beam qualifies only when that worktree holds no such record.
     Otherwise run `git -C <records repo> fetch origin`,
     then `git -C <records repo> show origin/<beam records branch>:<anchor>_lym/landing/beam-ready.json`,
     where `<anchor>` is the anchor's subpath with a trailing slash, empty for an anchor at the repository root.
     The same read serves a one-repository hub.
     A record found there that names the beam means the beam does not qualify.
     Any other outcome proves nothing for a beam on another machine, since the record may be committed there and not pushed:
     ask that beam's orch with `lym creel post --expect-reply` whether `lym beam ready` ran,
     and ask the operator when no reply comes within ten minutes.
     The beam qualifies only on a no.
   - When the check fails, the move stops before the cancel:
     lift the hold with `lym tender resume <slug>` and tell the operator.
3. Run `lym batten cancel <slug> --remove` from the prime, on the entry's owner machine.
   Its output does not show whether it ended anything, and each of its steps is a no-op once done,
   so tell a partial finish from a refusal by the way forward the output names.
   - A partial finish is an ok envelope with `end_only: true` or a non-empty `way_forward`,
     or an error whose way forward is only to fix the cause it names, or to wait until origin answers, then cancel again.
     Take the fix it names and run the cancel again until it completes.
     When a run returns the same error again, stop with your hold in place and tell the operator.
   - A refusal is every other error:
     the hub-pause refusal, the owned-elsewhere refusal, "the work … reached its parent", "landed by another machine"
     and the nothing-left refusal.
     The move stops there.
     On the hub-pause refusal, keep your slug hold and tell the operator;
     once the operator has lifted the hub pause, run the cancel again.
     On the owned-elsewhere refusal, `lym tender resume <slug>` is refused too,
     so tell the operator, who chooses between the adopt and the cancel on the owner the refusal names.
     On any other refusal, lift the hold with `lym tender resume <slug>` and take the way forward the message names;
     for work that already reached its parent, that is letting the landing finish, after which nothing is left to move.
   - Never run `lym tender resume` without a slug:
     it lifts every pause entry of the hub, the operator's and your own slug hold alike.

   The code archive tag is `archive/<slug>/<first 12 hex of the tip>`, named from the tip of step 1,
   never read from the output's `archive_tags`, which names at most a local tag not yet pushed.
   A teardown child pushes the tags and deletes the branches on origin after the cancel returns.
   Before step 4, confirm both sides on origin, and repeat the check until both hold:
   - the code tag, with `git -C <prime> ls-remote origin 'refs/tags/archive/<slug>/*'`;
   - no records branch of the slug, with `git -C <records repo> ls-remote origin`.
     The records branch is the task branch's name with the records suffix appended,
     the one `git -C <records repo> branch -r` shows beside the task branch's name,
     listed under `refs/heads/`, or under `refs/lym/heads/` in a one-repository hub.
     The board refuses a `--record-file` upsert while it reads live on origin, and a fresh run would adopt it.

   On a remote-teardown failure notice for the slug, take the way forward it names, then check again.
   When ten minutes pass with neither both checks holding nor a notice,
   push the code tag by hand from the prime, `git -C <prime> push origin refs/tags/archive/<slug>/<12 hex>`,
   since the task worktree a notice names is removed, then check again.
   When the records branch is still listed after that, stop and tell the operator,
   naming the branch and the move note's path in your `.scratch/`.
   The entry stays a draft under its old parent, and the move goes on at step 4 once the branch is gone from origin.
4. Write the move note into what the fresh run reads, in one upsert of the same entry:
   - An entry with no stored decision record gets a `## Move note` section in its body.
   - An entry with a stored decision record (`discussed`) runs from that record and never reads its body.
     Add an H3 `### Move note` under the record's `## Notes for the plan writer`,
     adding that section as the record's last when it lacks it,
     and store the record with `lym board upsert --record-file`.
     Write the note's four parts as bullets or H4 headings, never as H2.

   The section holds the note, the code archive tag and the fetch that brings it,
   `git fetch origin refs/tags/archive/<slug>/<12 hex>:refs/tags/archive/<slug>/<12 hex>`,
   the records archive tag's pattern `archive/<slug>/` in the records repository,
   and the rule below on reading the old commits.
   The same upsert sets `project` to the new beam's slug, or empties it for the root.
   When the board refuses the `project` value, because the new parent moved past its hold-free rows after step 2,
   run the upsert again without the `project` change and tell the operator:
   the entry stays a draft under its old parent with the move note written in.
5. Release the entry.
   While the entry `stale-records-ref-blocks-rerun` has not landed,
   the releasing orch first runs `git -C <records repo> fetch --prune origin`:
   a stale tracking ref of the deleted records branch makes dobby refuse the re-run
   with `the code tip ... is not on origin yet`, and the prune clears it.
   The orch whose worktree is the new parent then releases the entry with `lym board set-status` and a null status.
   When that is another orch, leave the entry `draft` and tell the operator which orch releases it,
   and that the prune comes first.

The fresh run forks from its new parent's head like any run, and reads the old commits without re-using them.
It first fetches the code archive tag with the fetch the note names,
since a plain fetch never follows an archive tag whose branch was deleted.
It then reads the diff from the note's merge base to the tag.
No agent cherry-picks, merges or checks out the archive tag into the new task branch,
since the old tip can carry unlanded commits of the old parent into a parent that never took them.
Work the fresh run keeps is written again through its own plan and gates.
A move before any code commit leaves an empty diff, and the note alone carries the old run's work.
