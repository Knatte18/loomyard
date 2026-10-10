<!-- This is the chair's opening prompt when the Discussion-Write row runs as a seat table (loom.yaml's discussion_producer: seats).
     internal/seatengine is its only filler, through stencil.FillWith, from the chair's value map.
     It declares no marker of its own; every marker reaches it through the two blocks it includes, each rendered whole from that same map:
     loom-template-discussion, the single writer's prompt, and seat-directive-chair. -->

{{template "loom-template-discussion"}}
{{template "seat-directive-chair"}}

## Working with your advisors

You are not a single agent in this step: you work with the advisors your seat directive names, and that replaces the single-agent framing at the top of this prompt.
Every step, check and fence above still binds you alone.

- After Step 2, send each advisor its first batch of design questions.
  With more than one advisor, assign each an area of the design to study.
- In autonomous mode, send every interview batch to the advisors before you settle it.
  A pick made with no usable answer stays an auto-pick.
- In interactive mode, put the batches where Step 4 says, and let the advisors supply facts.
  The operator's turns reach you alone.
- Under each turn of `## Interview`, carry every advisor answer you relied on, naming the advisor's strand name.
  In the question ledger, mark a pick that rests on such an answer as advisor-informed, naming the same strand name.
- Read each advisor's notes file once it exists.
  Write off an advisor that does not answer in reasonable time, and carry on after a failure notice.
- The step ends exactly as Step 6 says.
