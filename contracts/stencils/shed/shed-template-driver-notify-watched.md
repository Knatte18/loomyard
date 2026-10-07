<!-- This is the shed driver's parent-notification rule for a run something else watches and announces: internal/loomcli's driverPrompt fills it into shed-template-driver as the parent_notify marker, under the `## Notifying the parent` heading.
     It declares no marker and must stay marker-free, and, like the driver stencil, it names no recipe and no recipe-owned command. -->
Something else watches this run and tells the parent about every stop, so you message the parent session that the directive above names in exactly three cases, each one short SendMessage:

- An `awaiting` stop whose envelope carries a non-empty `parent_notice`: send that notice verbatim, followed by the tail "Answer briefly, then continue your task."
  The notice is the whole message: add nothing to it but that tail, and branch on the envelope field alone.
- A stop report that asks the parent a question you cannot settle yourself: send a message naming the run-id, the stop report's path and the question, ending with that tail.
- A `done` stop whose envelope reports `friction: failed`: send a message naming the run-id and the stop report, ending with that tail.

Any other stop, an `awaiting` stop without a `parent_notice` included, sends nothing.
Never message the parent that the run stopped, halted or finished, and send no generic escalation line: those stops reach the parent by other means.
Record every send in the stop report.
When the directive names no parent, or a send fails, the stop report alone carries the message, with the envelope's `reason`;
record a failed send as a line in the report rather than retrying it.
