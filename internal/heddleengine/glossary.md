## lym's glossary

Each line names a lym term, its meaning, and the words never to use for it.
Use these terms with the operator, on the board and in records.
When someone uses a word a line rules out, or a term in another sense than its line gives, name the term and its line.
A term settled that this list lacks is added here, in lym's repository at `internal/heddleengine/glossary.md`.

- **board**: the hub's list of entries. Never: backlog, tracker.
- **entry**: one item on the board, a task, a note or a project. Never: card, kort, ticket, issue.
- **task**: an entry a run works; only the operator makes or promotes one. Never: card, job.
- **note**: an entry that is not work yet, such as an idea, a finding or a Sketch. Never: card, draft.
- **project**: an entry that groups tasks under one aim. Never: epic.
- **decision record**: what a run's change must do, from Discussion or the entry. Never: log, logg.
- **support log**: Discussion's interview, rejected alternatives and review rounds, beside the decision record.
- **Sketch**: a decision record kept on the board as a note, for an entry discussed before it runs.
- **recipe**: the rows a run walks in order, such as tapestry, darn or sampler. Never: profile, profil, pipeline.
- **jacquard**: the recipe that builds a task from its stored decision record: tapestry's rows after Discussion.
- **row**: one step of a recipe, run by one producer and checked by its gates. Never: stage.
- **run**: one recipe walked for one task in the task's worktree. Never: job, build.
- **plan**: the cards the Plan row writes for Webster to implement.
- **card**: one unit of a plan, which Webster implements in a batch. Never for an entry.
- **gate**: a mechanical check a row's output passes before the run moves on. Never: review.
- **change description**: the title and body a run lands under. Never: summary, log.
- **landing**: the merge of a run's task branch into its parent branch, in Finalize. Never: deploy, release.
- **hub**: one repository's worktrees, Bolt checkout, board and agents on a machine.
- **orch**: the operator's delegate session, which talks with the operator, writes the board and parents runs.
- **parent**: the session a run's agents ask and report to, the orch or a run above them.
- **autonomy**: a worktree's or a task's mode in which the orch settles every halt of the runs it covers and reaches the operator only for a severe halt, a merge-ready beam or a raddle split's approval. Never: autopilot, full auto.
- **severe halt**: a halt in one of autonomy's closed classes, which the orch relays to the operator. Never: critical halt, blocker.
- **strand**: one agent's pane, holding one session. Never: tab, window.
- **beam**: a long-lived branch that groups board work; its tasks fork from it and land back into it.
- **pair**: a task's code worktree together with its records worktree.
- **pusher**: the background process that brings one worktree's run commits to origin in order.
- **move note**: what a cancelled run learned, written for the fresh run that replaces it. Never: hand-back, handoff.
