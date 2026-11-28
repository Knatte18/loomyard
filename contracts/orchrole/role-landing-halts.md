<!-- This is the orch role's on-demand file for a halt at Parent-Align or Finalize, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Landing halts

Parent-Align and Finalize fix their own merge and test failures with the merge fixer;
no pull request waits for an approval.
A task run's Finalize merges main into the task branch with no lock held and no gate, then waits in the parent lock's queue.
The lock's holder gates up to `landing.batch_max` queued runs together and lands them on main,
after which main is pushed and then the board task is marked done.
A failed batch is bisected, and the run it fails on gates and fixes alone.
A parent that moved since Parent-Align, a conflict or a large merge does not send a task run back:
Finalize merges the parent in place, resolves a conflict and gates the merged tree.

A landing halt reaches you with the status marked `hand_back`;
no driver, batten or arbiter routes it to another row.
Never answer one with a goto to Plan-Bouncer, since the plan does not change.

- **A landing-stop halt** has one of these causes.
  The fixer spent its cap, failed, or made an over-cap change at Parent-Align.
  On a pawl run, the fixer escalated a decision.
  The fixer's attempt record was undecodable.
  Finalize's fix reviews ran out of rounds without approving the fixer's change.
  The halt's reason says which, and names the commits the fixer kept.
  Read the commits and decide what the run needs.
  Record it with `lym loom decision add <slug> --by parent`, then run `lym loom resume` in the task worktree.
  Once the task's scope is closed, as for a Parent-Align stop after a Code-Review round has run, the record takes no decision:
  file the call as a board note for a new task with `lym board upsert` and kind note, then resume.
  A Finalize stop is outside the closed span and still records the call.
- **An undecodable attempt record:** remove that record by the path the reason names.
  That grants the entry a fresh cap.
- **Finalize's fix reviews out of rounds:** the reason names the open findings.
  Either accept the change as it stands or have the findings fixed on the task branch by the run's agents.
  Accepting is `lym loom circling accept <slug>`, which writes those findings into the change description.
  Then run `lym loom resume`, which without an acceptance runs the fix reviews afresh.
- **A fixer that escalated a decision** halts with the landing-escalate prefix instead.
  This holds on a tapestry, jacquard, darn or sampler run.
  The arbiter judges it first, so it reaches you only as the arbiter's hand-on.
  Record the deciding decision with `lym loom decision add`, then `lym loom resume`.
  Once the task's scope is closed, as for a Parent-Align stop, file the call as a board note instead,
  have the conflict settled on the task branch so the record's decisions hold, then resume.
- **A beam** has no Parent-Align, and its Finalize merges the beam's pull request on GitHub.
  It halts when GitHub refuses:
  a pending check, a dismissed review, a refused merge method or a pull request closed on GitHub.
  Read the pull request with `lym beam pr status <slug>`.
  Settle on GitHub what the halt's message names, which for a closed one is the operator reopening it.
  Then run `lym loom resume` in the beam's worktree, and never `lym batten cancel <slug>`.
