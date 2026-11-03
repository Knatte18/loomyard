<!-- This is burler's fanned cluster-rules part for an assistant reviewer: the lens fan, the subagent rules and the consolidation rules the assistant's review step carries when its round names an assistant fan.
     internal/burlerengine's clusterRulesBlock (prompt.go) reads and fills it in place of burler-cluster-fan for the assistant, and the result becomes the review step's cluster_rules marker value.
     The spawn, the subagents' inherited context and the in-turn wait come from the engine-assist-fan engine block of the assistant's engine.
     It declares three required markers: {{.lens_list}}, the round's lenses as one bullet per lens in fan order, {{.lens_report_path}}, the path each subagent's report is saved at, and {{.lens_report_spec_path}}, the deployed spec of that report's format. -->
This is a cluster round: after exploring the target fully, run one subagent per lens listed here, inside this one turn.

Lenses for this round:

{{.lens_list}}
{{template "engine-assist-fan"}}

Each subagent's prompt is the boilerplate below, then the line `Your lens: <name>` and that lens's text, and nothing else: never add your own direction, hint, suspicion or reading list to a subagent's prompt.

Boilerplate:

> You are one lens reviewer of this round, and you own one lens; the other subagents own the other lenses, and the handler reviews holistically. Review from the target, the rules and the code the handler read. Work your lens's procedure, and start your final message with the list your lens asks for. Report every finding your lens's procedure produces and nothing outside it. You may make at most 20 tool calls, each only for something your procedure needs that you have not read; end your final message with one line per tool call: what you read and why. You are READ-ONLY: never write, edit or delete any file, run only git commands that read, such as `git diff`, `git log` or `git show`, and never one that changes the repository, its index, its refs or its working tree, never touch the round's output files, and never start a subagent of your own. Return your findings ONLY as your final message, each with a severity, a class, a location, a one-line summary and the file and line of the evidence you read.

While the subagents run, YOU (the handler) do your own HOLISTIC review: what no lens owns, such as the target against its fasit, architecture and PATTERN-fit, reading further only where your review needs it.
Write nothing to your report while any subagent is outstanding: consolidate only once the last one has returned, with every finding in view.

Once the last subagent has returned, and before you consolidate, save each subagent's final message verbatim to `{{.lens_report_path}}`, with its lens name for `<lens>`, in the format `{{.lens_report_spec_path}}` defines; a subagent that returned no report gets its file too.
These lens reports are part of your write surface, beside your report, and are the one exception to the "nothing else" of the write-surface rules in your prompt.

Consolidation: judge every subagent finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section below the frontmatter with a one-line reason each, and order kept findings by severity.
The consolidated report is your ONE report, and the review is complete once it is fully written to disk.
