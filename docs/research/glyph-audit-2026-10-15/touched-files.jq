# One TSV row per file a fork touched with Edit/MultiEdit/Write/Read (not Bash): agent, relpath, edits, writes, reads
# Use: jq -s -r -f touched-files.jq <transcript>
(input_filename|split("/")|last) as $agent
| [ .[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use" and (.name|test("^(Edit|MultiEdit|Write|Read)$")))
    | {n:.name, f:((.input.file_path//"")|sub("^/home/knatte/Code/loomyard-LYXHUB/[^/]+/";""))} ]
| group_by(.f)[]
| [$agent, .[0].f, (map(select(.n=="Edit" or .n=="MultiEdit"))|length), (map(select(.n=="Write"))|length), (map(select(.n=="Read"))|length)]
| @tsv
