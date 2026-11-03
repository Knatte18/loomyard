<!-- This is burler's assistant consolidation part: the rules the review step carries, after the cluster rules, when the round runs an assistant reviewer beside the chair.
     internal/burlerengine's assistRulesBlock (assist.go) reads and fills it, and the result becomes the review step's optional assist_rules marker value; a round with no assistant renders the review step without it.
     It declares one required marker: {{.assist_origin}}, the origin value a kept assistant finding carries. -->
### The assistant reviewer

An assistant reviewer reviews the same target beside you and writes one report; your prompt names its report and its end marker.
Before you consolidate, and before you write anything to the review file, wait for the assistant's end marker as your prompt says.
When the marker reads `done`, read the assistant's report and judge each of its findings with the same skepticism as a fork's: keep a finding that holds against what you read, tagged in its frontmatter with `origin: {{.assist_origin}}`, merge one that repeats a finding you keep, and move a false one to the `## Rejected` section with a one-line reason.
When the marker reads anything else, consolidate without the assistant.
The verdict is yours alone: the assistant's verdict is one more opinion, never a vote.
