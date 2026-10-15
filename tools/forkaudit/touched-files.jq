# touched-files.jq emits one TSV row per file a Webster fork edited, wrote or read with its Edit, MultiEdit, Write and Read tools.
# Paths are made relative by stripping the fork's worktree prefix; paths outside the worktree and paths under the run-record root or the logs root are skipped.
# Use: jq -n -R -r --arg rel <transcript path relative to the projects directory> --arg prefix <worktree path with a trailing slash>
#   --arg record_root <run-record root> --arg logs_root <logs root> -f touched-files.jq <transcript>
# Columns: rel, file relative to the worktree, edits, writes, reads.
def relative: if startswith($prefix) then ltrimstr($prefix) else null end;

[ inputs | fromjson? | select(type == "object" and .type == "assistant")
  | .message.content[]? | select(.type == "tool_use" and (.name | test("^(Edit|MultiEdit|Write|Read)$")))
  | {n: .name, f: ((.input.file_path // "") | relative)}
  | select(.f != null and (.f | startswith($record_root + "/") or startswith($logs_root + "/") | not)) ]
| group_by(.f)[]
| [$rel, .[0].f, (map(select(.n == "Edit" or .n == "MultiEdit")) | length), (map(select(.n == "Write")) | length), (map(select(.n == "Read")) | length)]
| @tsv
