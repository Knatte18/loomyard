<!-- This is the orch role's on-demand file for a halt at Parent-Align or Finalize, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Landing halts

Parent-Align and Finalize fix their own merge and test failures with the merge fixer;
no pull request waits for an approval.
A task run's Finalize takes the parent lock first.
It then syncs the task branch with main, squashes it onto main, marks the board task done and pushes main.
A parent that moved since Parent-Align, a conflict or a large merge does not send a task run back:
Finalize merges the parent in place, resolves a conflict under the lock and gates the merged tree.

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
- **An undecodable attempt record:** remove that record by the path the reason names.
  That grants the entry a fresh cap.
- **Finalize's fix reviews out of rounds:** the reason names the open findings.
  Either accept the change as it stands or have the findings fixed on the task branch by the run's agents.
  Accepting is `lym loom circling accept <slug>`, which writes those findings into the change description.
  Then run `lym loom resume`, which without an acceptance runs the fix reviews afresh.
- **A fixer that escalated a decision** on a tapestry, darn or sampler run halts with the landing-escalate prefix instead.
  The arbiter judges it first, so it reaches you only as the arbiter's hand-on.
  Record the deciding decision with `lym loom decision add`, then `lym loom resume`.
- **A beam** has no Parent-Align, and its Finalize merges the beam's pull request on GitHub.
  It halts when GitHub refuses:
  a pending check, a dismissed review, a refused merge method or a pull request closed on GitHub.
  Read the pull request with `lym beam pr status <slug>`.
  Settle on GitHub what the halt's message names, which for a closed one is the operator reopening it.
  Then run `lym loom resume` in the beam's worktree, and never `lym batten cancel <slug>`.
