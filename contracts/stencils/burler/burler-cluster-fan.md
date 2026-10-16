<!-- This is burler's fanned cluster-rules part: the spawn mechanics, the fork rules, the handler's holistic review and the consolidation rules the review step carries when the round names a fan.
     internal/burlerengine's clusterRulesBlock (prompt.go) reads and fills it, and the result becomes the review step's cluster_rules marker value.
     It declares one required marker, {{.lens_list}}, the round's lenses as one bullet per lens in fan order. -->
This is a cluster round: after exploring the target fully, spawn ALL of the fork reviewers below in a SINGLE message via the Agent tool with `subagent_type: "fork"`, one per lens listed here — never pass a `name` (named forks silently lose inherited context).

Lenses for this round:

{{.lens_list}}
### Fork rules for this round

Spawn the forks as soon as step 1's reading manifest is written, in the same turn.
Each fork's prompt is the boilerplate below, then the line `Your lens: <name>` and that lens's text, and nothing else: never add your own direction, hint, suspicion or reading list to a fork's prompt.

Boilerplate:

> You are one fork reviewer of this round, and you own one lens; the other forks own the other lenses, and the handler reviews holistically. Your inherited context already holds the target, the rules and the code listed in the reading manifest; review from it. Work your lens's procedure, and start your final message with the list your lens asks for. Report every finding your lens's procedure produces and nothing outside it, because the other lenses and the handler cover the rest. You may make at most 6 tool calls, each only for something your procedure needs that the manifest lacks, such as a caller or a spawned process one hop beyond it, and never to re-read what your context already shows; end your final message with one line per tool call: what you read and why your context lacked it. You are READ-ONLY: never Write, Edit or delete any file, never run any git command, never touch the two round output files, and never call the Agent tool. Return your findings ONLY as your final message, each with a severity, a class, a location, a one-line summary and the file and line of the evidence you read.

While the forks run, YOU (the handler) do your own HOLISTIC review: what no lens owns, such as the target against its fasit and its board entry, the rubric's leanness and relocation rules, architecture and PATTERN-fit, reading further only where your review needs it.
Then prepare the ground truths and the severity rubric you will judge every finding against.

Consolidation: judge every fork finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section below the frontmatter with a one-line reason each (a rejected item never appears in `findings:`), and order kept findings by severity.
The consolidated review is the ONE review file, and the review is complete once it is fully written to disk — consolidation is part of the review, not a separate step after it.
