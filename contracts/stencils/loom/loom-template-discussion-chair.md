<!-- This is the lead's opening prompt: the Discussion-Write row always runs as a seat table, with one advisor per entry of loom.yaml's discussion_advisors and the lead alone when it is empty.
     Two independent axes shape the table: the advisors are seats of their own, and loom.yaml's discussion_lenses, when set, has the lead alone fork its lens review.
     internal/seatengine is its only filler, through stencil.FillWith, from the lead's value map.
     It declares no marker of its own; every marker reaches it through the two blocks it includes, each rendered whole from that same map:
     loom-template-discussion, the interview prompt, and seat-directive-chair. -->

{{template "loom-template-discussion"}}
{{template "seat-directive-chair"}}

## Working with your advisors

If your seat directive names advisors, you are not a single agent in this step: you use them as the rest of this section says, and that replaces the single-agent framing at the top of this prompt.
If it names none, you have no advisors: follow the steps above as a single agent, decide each question yourself as you judge best, and skip the rest of this section.
Every step, check and fence above still binds you alone.

- After Step 2, form your question batches exactly as Step 3 says, each question with your recommended answer and its alternatives, and send every batch to every advisor.
  No advisor owns an area, since each may find a different weakness.
- Ask in rounds: a batch holds the questions that fit now, you wait for every advisor's answer to it, and you form the next batch from those answers.
  Never send every question at once.
- In autonomous mode, settle each question only after weighing every advisor's answer to it.
  An advisor that did not start, sent a failure notice or was written off counts as giving no answer, and a pick made with no usable answer stays an auto-pick.
- In interactive mode, put the batches where Step 4 says, and let the advisors supply facts.
  The operator's turns reach you alone.
- Under each turn of `## Interview`, carry every advisor answer you relied on, naming the advisor's strand name.
  In the question ledger, mark a pick that rests on such an answer as advisor-informed, naming the same strand name.
- Read each advisor's notes file once it exists.
  Write off an advisor that does not answer in reasonable time, and carry on after a failure notice.
- The step ends exactly as Step 6 says.
