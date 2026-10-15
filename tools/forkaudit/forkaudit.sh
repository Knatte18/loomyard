#!/usr/bin/env bash
# Webster fork read audit: finds Webster implementer fork transcripts, extracts one record per fork
# and writes a per-arm markdown report of how the forks read their kick-start pack.
# Bash and jq only; the jq filters sit beside this script and are found relative to it, never the caller's cwd.
# It needs bash, jq, git, find, awk, mktemp, realpath and GNU date, and exits 1 naming a missing command.
#
# Usage:
#   forkaudit.sh --since <date> [--until <date>] [options]
#   forkaudit.sh --runs <name>[,<name>...] [options]
#
# Flags, each taking its value as the next argument:
#   --since <date>      select fork transcripts modified at or after the date (any `date -d` string).
#   --until <date>      with --since, select only transcripts not modified after the date.
#   --runs <names>      select the forks of these runs instead: a comma-separated list of run names,
#                       each the worktree's directory name, made of [a-z0-9-] only.
#   --projects <dir>    Claude Code's projects directory (default $HOME/.claude/projects).
#   --hub <dir>         the hub whose worktrees are audited (default the repo root's parent).
#   --records <dir>     the records repository holding the webster state history
#                       (default the repo root's sibling named after the repo root plus the records suffix).
#   --out <path>        the report file (default .scratch/fork-audit.md under the repo root).
#   --work <dir>        the work directory for intermediate files (default .scratch/fork-audit under the repo root).
# Exactly one of --since and --runs is required, and --until needs --since.
#
# Outputs:
#   the report at --out, written to a temporary file beside it and moved into place last,
#   so a failed run leaves an existing report intact and no temporary file behind;
#   the --out path on stdout;
#   intermediate files in --work, kept after the run for drill-down.
# A defaulted --out whose directory is absent from a fresh worktree gets that directory created with mkdir -p
# once every check has passed, beside --work's own creation; an explicit --out never creates its directory.
#
# Exit codes: 0 success, 1 environment or bounds failure (missing command, --out or --work refused), 2 usage.
#
# Work-directory bound: --work is created if missing and marked with a .forkaudit-work file.
# A run clears only the script's own named intermediate files in it, by literal name, never by glob.
# It exits 1 naming --work, deleting nothing, when --work equals or contains the repo root, the hub or --out's directory,
# when it exists non-empty without the marker, or when it cannot be created or written.
# Paths are canonicalized with realpath -m before any comparison, so a symlink or `..` cannot dodge a check.
# One run at a time per work directory is the supported use; concurrent runs need distinct --out and --work.
#
# Discovery: project directories under --projects are named after the hub path with every character
# outside [A-Za-z0-9] replaced by `-`, then `-` and the run name.
# --since matches every project directory whose name starts with the encoded hub and `-`;
# --runs matches only the directory named exactly the encoded hub, `-` and the run name.
# The report header lists the matched directories and each --runs name that matched none.
#
# Forks: the candidates are the subagents/agent-*.jsonl files under the matched project directories;
# under --since only those modified at or after it (and, with --until, not after it), under --runs every one.
# A candidate holding the Webster implementer job heading goes through extract.jq, which keeps it only when
# its first tool call reads a Webster prompt file whose result holds that heading, and writes one record per kept fork.
# A transcript is read line by line and a line that does not parse is dropped and counted, so a half-written
# last line of a fork still running costs that line, not the fork.
# A transcript that cannot be read exits 1 naming it.
# Each fork's arm is the pack layout its prompt shows, taken from the marker table at the head of extract.jq.
# A task that adds a Webster pack layout adds its marker row there in the same task.
#
# Cross-check: the prompt decides a fork's arm, and begin-batch's `webster: kickstart pack` trace line cross-checks it.
# For each fork whose worktree (the hub plus the run name) has a trace file under its logs directory,
# the fork's batch is the one in the records repository whose spawn second is 1 to 6 seconds before the fork's first transcript line,
# searched with git log and git show only, on that run's own refs: its records branch (local or under origin/) and its archive tags.
# The fork's trace layout is the pack_layout of the last trace line for that batch before the fork's first transcript line.
# A trace layout agrees with an arm when the arm's marker row lists it; any other pairing is a disagreement and the arm stays the prompt's.
# On an unknown fork a layout of [a-z0-9_-]+ that is no arm name makes the arm trace:<layout>;
# any other layout leaves the fork unknown and is listed once as unrecognized.
# A fork with no trace file, no resolved batch or no trace line before its start has no cross-check.
# A missing records repository only disables the cross-check, and the report header says so.
#
# Intermediate files in --work: projects.txt (matched project directories), candidates.txt (candidate transcripts),
# webster-files.txt (candidates holding the job heading), records.jsonl (one record per kept fork),
# batches.tsv (each fork's resolved batch) and traces.tsv (each cross-checked fork's trace layout).
# The report body is rendered from records.jsonl by report.jq:
# a samples table, a pack-use table, an arm comparison table, the quartiles of net exploration calls per card and a per-fork table.
# A fork lacking a metric is left out of that metric's median and pooled value, and a cell shows its sample size when that is
# smaller than the arm's fork count, or n/a when none is left; the rule is stated in full at the head of report.jq.
# An arm with no forks is left out of the tables and named once under the samples table.

set -u
set -o pipefail
export LC_ALL=C

# The run-record literals this script reads, each spelled once, here.
# The roots below are derived from them, and the jq filters receive all of them with --arg.
STATE_PATH="_lyx/webster/state.json"
PLAN_DIR="_lyx/plan/"
LOGS_DIR=".lyx/logs"
RECORDS_SUFFIX="-weft"
RUN_RECORD_ROOT="${PLAN_DIR%%/*}"
LOGS_ROOT="${LOGS_DIR%%/*}"

# Bash builtins only until the command check has passed.
script_source="${BASH_SOURCE[0]}"
script_dir="${script_source%/*}"
[ "$script_dir" = "$script_source" ] && script_dir=.
script_dir="$(cd "$script_dir" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

for required in jq git find awk mktemp realpath date; do
  command -v "$required" > /dev/null 2>&1 || { echo "forkaudit: missing command: $required" >&2; exit 1; }
done
date -d @0 > /dev/null 2>&1 || { echo "forkaudit: missing command: GNU date" >&2; exit 1; }

MARKER_FILE=".forkaudit-work"
# The script's own intermediate files in --work; a run clears exactly these.
WORK_FILES=(projects.txt candidates.txt webster-files.txt records.jsonl batches.tsv traces.tsv)
# The heading that marks a Webster implementer prompt.
JOB_HEADING='# Webster implementer job'

usage() {
  cat >&2 <<'EOF'
Usage: forkaudit.sh (--since <date> [--until <date>] | --runs <name>[,<name>...]) [options]
Options:
  --projects <dir>   Claude Code projects directory (default $HOME/.claude/projects)
  --hub <dir>        hub whose worktrees are audited (default the repo root's parent)
  --records <dir>    records repository (default the repo root's sibling named after it plus the records suffix)
  --out <path>       report file (default .scratch/fork-audit.md under the repo root)
  --work <dir>       work directory (default .scratch/fork-audit under the repo root)
EOF
}

usage_error() {
  echo "forkaudit: $1" >&2
  usage
  exit 2
}

fail() {
  echo "forkaudit: $1" >&2
  exit 1
}

original_args=("$@")
since="" until_date="" runs="" projects="${HOME:-}/.claude/projects" hub="" records="" out="" work=""
since_set=0 until_set=0 runs_set=0
while [ $# -gt 0 ]; do
  case "$1" in
    --since|--until|--runs|--projects|--hub|--records|--out|--work)
      [ $# -ge 2 ] || usage_error "$1 needs a value"
      case "$1" in
        --since) since="$2"; since_set=1 ;;
        --until) until_date="$2"; until_set=1 ;;
        --runs) runs="$2"; runs_set=1 ;;
        --projects) projects="$2" ;;
        --hub) hub="$2" ;;
        --records) records="$2" ;;
        --out) out="$2" ;;
        --work) work="$2" ;;
      esac
      shift 2
      ;;
    *) usage_error "unknown argument: $1" ;;
  esac
done

[ "$since_set" = 1 ] || [ "$runs_set" = 1 ] || usage_error "one of --since or --runs is required"
[ "$since_set" = 1 ] && [ "$runs_set" = 1 ] && usage_error "--since and --runs cannot be combined"
[ "$until_set" = 1 ] && [ "$since_set" = 0 ] && usage_error "--until needs --since"

if [ "$since_set" = 1 ]; then
  [ -n "$since" ] && since_epoch="$(date -d "$since" +%s 2> /dev/null)" || usage_error "--since: cannot parse date: $since"
  if [ "$until_set" = 1 ]; then
    [ -n "$until_date" ] && until_epoch="$(date -d "$until_date" +%s 2> /dev/null)" || usage_error "--until: cannot parse date: $until_date"
    [ "$until_epoch" -ge "$since_epoch" ] || usage_error "--until is earlier than --since"
  fi
else
  case ",$runs," in
    *,,*) usage_error "--runs: empty run list or empty run name" ;;
  esac
  IFS=, read -r -a run_names <<< "$runs"
  for run_name in "${run_names[@]}"; do
    [[ "$run_name" =~ ^[a-z0-9-]+$ ]] || usage_error "--runs: invalid run name: $run_name"
  done
fi

repo_root="$(realpath -m "$repo_root")"
projects="$(realpath -m "$projects")"
hub="$(realpath -m "${hub:-$repo_root/..}")"
records="$(realpath -m "${records:-$repo_root$RECORDS_SUFFIX}")"
out_explicit=1
if [ -z "$out" ]; then
  out="$repo_root/.scratch/fork-audit.md"
  out_explicit=0
fi
work="$(realpath -m "${work:-$repo_root/.scratch/fork-audit}")"
out="$(realpath -m "$out")"
out_dir="${out%/*}"

if [ -d "$out" ]; then
  fail "--out names an existing directory: $out"
fi
if [ -d "$out_dir" ]; then
  { [ -w "$out_dir" ] && [ -x "$out_dir" ]; } || fail "--out directory is not writable: $out_dir"
elif [ "$out_explicit" = 1 ]; then
  fail "--out directory does not exist: $out_dir"
fi

work_prefix="${work%/}/"
for overlap in "the repo root:$repo_root" "the hub:$hub" "--out's directory:$out_dir"; do
  case "${overlap#*:}/" in
    "$work_prefix"*) fail "--work overlaps ${overlap%%:*}: $work" ;;
  esac
done
if [ -d "$work" ] && [ ! -e "$work/$MARKER_FILE" ]; then
  shopt -s nullglob dotglob
  work_entries=("$work"/*)
  shopt -u nullglob dotglob
  [ "${#work_entries[@]}" -eq 0 ] || fail "--work is not a forkaudit work directory: $work"
fi
work_ancestor="$work"
while [ ! -e "$work_ancestor" ]; do
  work_ancestor="${work_ancestor%/*}"
  [ -n "$work_ancestor" ] || work_ancestor=/
done
{ [ -d "$work_ancestor" ] && [ -w "$work_ancestor" ] && [ -x "$work_ancestor" ]; } || fail "--work is not writable: $work"

mkdir -p "$out_dir" || fail "--out directory cannot be created: $out_dir"
mkdir -p "$work" || fail "--work is not writable: $work"
: > "$work/$MARKER_FILE" || fail "--work is not writable: $work"
for work_file in "${WORK_FILES[@]}"; do
  rm -f -- "$work/$work_file"
done

report_tmp=""
report_moved=0
cleanup() {
  if [ -n "$report_tmp" ] && [ "$report_moved" = 0 ]; then
    rm -f -- "$report_tmp"
  fi
}
trap cleanup EXIT
report_tmp="$(mktemp "$out_dir/.forkaudit-report.XXXXXX")" || fail "--out directory is not writable: $out_dir"
chmod 644 "$report_tmp"

hub_encoded="${hub//[^A-Za-z0-9]/-}"
unmatched_runs=()
if [ "$since_set" = 1 ]; then
  for project_dir in "$projects/$hub_encoded-"*; do
    [ -d "$project_dir" ] && echo "$project_dir"
  done > "$work/projects.txt"
else
  for run_name in "${run_names[@]}"; do
    if [ -d "$projects/$hub_encoded-$run_name" ]; then
      echo "$projects/$hub_encoded-$run_name" >> "$work/projects.txt"
    else
      unmatched_runs+=("$run_name")
    fi
  done
  [ -e "$work/projects.txt" ] || : > "$work/projects.txt"
fi

find_args=(-type f -path '*/subagents/agent-*.jsonl')
if [ "$since_set" = 1 ]; then
  find_args+=(-newermt "$since")
  [ "$until_set" = 1 ] && find_args+=(! -newermt "$until_date")
fi
: > "$work/candidates.txt"
while IFS= read -r project_dir; do
  find "$project_dir" "${find_args[@]}" || fail "cannot search project directory: $project_dir"
done < "$work/projects.txt" | sort > "$work/candidates.txt" || exit 1

# A candidate holding the job heading is a fork only if extract.jq also accepts its first tool call.
: > "$work/webster-files.txt"
while IFS= read -r candidate; do
  grep -q -- "$JOB_HEADING" "$candidate" 2> /dev/null
  case $? in
    0) echo "$candidate" >> "$work/webster-files.txt" ;;
    1) ;;
    *) fail "cannot read transcript: $candidate" ;;
  esac
done < "$work/candidates.txt"

# The extractor's run name is the project directory's name without the encoded hub and `-`.
: > "$work/records.jsonl"
while IFS= read -r transcript; do
  relative="${transcript#"$projects"/}"
  project_name="${relative%%/*}"
  jq -n -R -c --arg mode extract --arg run "${project_name#"$hub_encoded"-}" --arg rel "$relative" --arg plan "$PLAN_DIR" \
    -f "$script_dir/extract.jq" "$transcript" >> "$work/records.jsonl" || fail "cannot extract transcript: $transcript"
done < "$work/webster-files.txt"

candidate_count="$(grep -c . "$work/candidates.txt")"
heading_count="$(grep -c . "$work/webster-files.txt")"
fork_count="$(grep -c . "$work/records.jsonl")"
dropped_count="$(jq -s 'map(.dropped) | add // 0' "$work/records.jsonl")"

# Batch resolution and the trace join, per fork, reading the records repository with git log and git show only.
# The batch is the one in the fork's own run's state history whose spawn second is 1 to 6 seconds before the fork's first transcript line.
# Only the fork's run refs are searched, so a concurrent run's batch spawned in the same window never resolves:
# its records branch, local and under origin/, and its archive tags.
# One letter of the suffix is bracketed so --glob matches that one branch and does not append /*.
# batches.tsv columns: transcript path relative to --projects, run, fork start, batch, card count, start sha, commit, spawn second, cards.
# traces.tsv columns: transcript path relative to --projects, batch, pack_layout of the last trace line before the fork's start.
resolve_all=0
: > "$work/batches.tsv"
: > "$work/traces.tsv"
records_found=0
[ -d "$records" ] && records_found=1
branch_glob="${RECORDS_SUFFIX%?}[${RECORDS_SUFFIX: -1}]"
if [ "$records_found" = 1 ]; then
  while IFS=$'\t' read -r fork_rel fork_run fork_start; do
    worktree="$hub/$fork_run"
    shopt -s nullglob
    trace_files=("$worktree/$LOGS_DIR"/trace-*.log)
    shopt -u nullglob
    if [ "${#trace_files[@]}" -eq 0 ] && [ "$resolve_all" = 0 ]; then
      continue
    fi
    ref_globs=("--glob=refs/heads/$fork_run$branch_glob" "--glob=refs/remotes/origin/$fork_run$branch_glob" "--glob=refs/tags/archive/$fork_run/*")
    start_epoch="$(date -u -d "$fork_start" +%s)"
    resolved=""
    for offset in 1 2 3 4 5 6; do
      spawn_second="$(date -u -d "@$((start_epoch - offset))" +%Y-%m-%dT%H:%M:%SZ)"
      commit="$(git -C "$records" log "${ref_globs[@]}" -S"$spawn_second" --format=%H -- "$STATE_PATH" 2> /dev/null | head -1)"
      [ -n "$commit" ] || continue
      batch_row="$(git -C "$records" show "$commit:$STATE_PATH" 2> /dev/null | jq -r --arg second "$spawn_second" \
        '.batches | to_entries[] | select(.value.spawnedAt == $second) | [.key, (.value.cards | length), .value.startSha, .value.spawnedAt, (.value.cards | join(","))] | @tsv' 2> /dev/null | head -1)"
      if [ -n "$batch_row" ]; then
        resolved="$commit"$'\t'"$batch_row"
        break
      fi
    done
    if [ -z "$resolved" ]; then
      printf '%s\t%s\t%s\t-\t-\t-\t-\t-\t-\n' "$fork_rel" "$fork_run" "$fork_start" >> "$work/batches.tsv"
      continue
    fi
    IFS=$'\t' read -r commit batch card_count start_sha spawn_second batch_cards <<< "$resolved"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$fork_rel" "$fork_run" "$fork_start" "$batch" "$card_count" "$start_sha" "$commit" "$spawn_second" "$batch_cards" >> "$work/batches.tsv"
    [ "${#trace_files[@]}" -gt 0 ] || continue
    start_ns="$(date -u -d "$fork_start" +%s%N)"
    best_ns=0
    layout=""
    while IFS= read -r trace_line; do
      [[ "$trace_line" =~ time=([^[:space:]]+) ]] || continue
      line_ns="$(date -u -d "${BASH_REMATCH[1]}" +%s%N 2> /dev/null)" || continue
      [ "$line_ns" -lt "$start_ns" ] && [ "$line_ns" -gt "$best_ns" ] || continue
      [[ "$trace_line" =~ pack_layout=([^[:space:]]+) ]] || continue
      best_ns="$line_ns"
      layout="${BASH_REMATCH[1]}"
    done < <(grep -h -F 'msg="webster: kickstart pack"' "${trace_files[@]}" | grep -F " batch=$batch ")
    [ -z "$layout" ] || printf '%s\t%s\t%s\n' "$fork_rel" "$batch" "$layout" >> "$work/traces.tsv"
  done < <(jq -r '[.rel, .proj, .start] | @tsv' "$work/records.jsonl")
fi

write_header() {
  local project_dir
  echo "# Webster fork read audit"
  echo
  echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo
  printf -- '- Arguments: `%s`\n' "$(printf '%q ' "${original_args[@]}")"
  echo "- Projects: \`$projects\`"
  echo "- Hub: \`$hub\`"
  echo "- Work: \`$work\`"
  echo "- Records: \`$records\`"
  if [ "$records_found" = 0 ]; then
    echo "- No cross-check: records repo $records not found."
  fi
  if [ -s "$work/projects.txt" ]; then
    echo "- Project directories matched:"
    while IFS= read -r project_dir; do
      echo "  - \`${project_dir##*/}\`"
    done < "$work/projects.txt"
  else
    echo "- Project directories matched: none"
  fi
  if [ "${#unmatched_runs[@]}" -gt 0 ]; then
    echo "- Runs with no project directory: ${unmatched_runs[*]}"
  fi
  echo "- Candidates found: $candidate_count"
  echo "- Transcripts holding the job heading: $heading_count"
  echo "- Forks kept: $fork_count"
  echo "- Transcript lines dropped as unparsable: $dropped_count"
  echo
}

markers_json="$(jq -n -c --arg mode arms --arg run "" --arg rel "" --arg plan "" -f "$script_dir/extract.jq")"
{
  write_header
  if [ "$fork_count" -eq 0 ]; then
    echo "No Webster implementer forks matched."
  else
    jq -s -r --argjson markers "$markers_json" --rawfile batches "$work/batches.tsv" --rawfile traces "$work/traces.tsv" \
      -f "$script_dir/report.jq" "$work/records.jsonl" || fail "cannot render the report"
  fi
} > "$report_tmp"

mv -f -- "$report_tmp" "$out"
report_moved=1
echo "$out"
