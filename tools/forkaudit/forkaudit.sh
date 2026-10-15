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
#   --pack-check        add the pack-correctness section; it takes no value.
#   --code <dir>        the code repository the packs are checked against (default the repo root).
#   --plan              add the plan glyph-use section; it takes no value and needs --records but not --code.
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
# Pack correctness (--pack-check): batch resolution runs for every fork, not only those with a trace file.
# Each span entry of a fork's pack, from the pack file it read or from its prompt's inline pack, is checked against the file
# at the batch's start sha in --code, with git cat-file and git show only, and classed
# correct (the signature text lies inside the span), correct-decl (a declaration of the name lies inside it, Go files only),
# stale (found elsewhere in the file, with the line shift), wrong (file present, name absent, Go files only),
# unchecked (a non-Go file whose signature text was not found), nofile (file absent at the sha)
# or notree (batch unresolved, or the sha is not in --code).
# Files a fork edited, wrote or read that its pack gave no span for are listed with whether they existed at the sha.
# A missing --code or --records directory exits 1 naming it, --code first, before the work directory is touched.
# The report header names the resolved --code path.
#
# Plan glyph use (--plan): batch resolution runs for every fork, and a missing --records directory exits 1 naming it.
# For each run with forks in the report, the plan cards are read from the records repository at the newest commit on the run's refs
# that touches the state path, and each card's references under Edit, Create, Move, Delete, Rename and Uses
# are counted by kind: glyph, plan: handle, bare file and bare directory; a bullet under any other label is skipped.
# The run's plan writer is its top-level session transcript (never a subagent) with the most plan card writes,
# ties broken by the lower session id; its quarry calls, greps and reads are counted.
# A run whose records commit or plan-writer session is not found gets one line saying which and counts in no table.
# A run whose forks span several arms counts under each of those arms.
#
# Intermediate files in --work: projects.txt (matched project directories), candidates.txt (candidate transcripts),
# webster-files.txt (candidates holding the job heading), records.jsonl (one record per kept fork),
# batches.tsv (each fork's resolved batch), traces.tsv (each cross-checked fork's trace layout),
# under --pack-check pack-entries.tsv (every pack entry), pack-check.tsv (one class per span entry) and missed-files.tsv,
# and under --plan plan-runs.tsv, plan-cards.tsv, plan-refs.tsv and plan-writers.tsv (the chosen writer of each run).
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
WORK_FILES=(projects.txt candidates.txt webster-files.txt records.jsonl batches.tsv traces.tsv pack-entries.tsv pack-check.tsv missed-files.tsv
          plan-runs.tsv plan-cards.tsv plan-refs.tsv plan-writers.tsv)
# The heading that marks a Webster implementer prompt.
JOB_HEADING='# Webster implementer job'

usage() {
  cat >&2 <<'EOF'
Usage: forkaudit.sh (--since <date> [--until <date>] | --runs <name>[,<name>...]) [options]
Options:
  --projects <dir>   Claude Code projects directory (default $HOME/.claude/projects)
  --hub <dir>        hub whose worktrees are audited (default the repo root's parent)
  --records <dir>    records repository (default the repo root's sibling named after it plus the records suffix)
  --pack-check       add the pack-correctness section (needs --code and --records)
  --code <dir>       code repository the pack is checked against (default the repo root)
  --plan             add the plan glyph-use section (needs --records)
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
since="" until_date="" runs="" projects="${HOME:-}/.claude/projects" hub="" records="" code="" out="" work=""
since_set=0 until_set=0 runs_set=0 pack_check=0 plan=0
while [ $# -gt 0 ]; do
  case "$1" in
    --pack-check)
      pack_check=1
      shift
      ;;
    --plan)
      plan=1
      shift
      ;;
    --since|--until|--runs|--projects|--hub|--records|--code|--out|--work)
      [ $# -ge 2 ] || usage_error "$1 needs a value"
      case "$1" in
        --since) since="$2"; since_set=1 ;;
        --until) until_date="$2"; until_set=1 ;;
        --runs) runs="$2"; runs_set=1 ;;
        --projects) projects="$2" ;;
        --hub) hub="$2" ;;
        --records) records="$2" ;;
        --code) code="$2" ;;
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
code="$(realpath -m "${code:-$repo_root}")"
resolve_all=0
if [ "$pack_check" = 1 ]; then
  [ -d "$code" ] || fail "--code directory does not exist: $code"
  [ -d "$records" ] || fail "--records directory does not exist: $records"
  resolve_all=1
fi
if [ "$plan" = 1 ]; then
  [ -d "$records" ] || fail "--records directory does not exist: $records"
  resolve_all=1
fi
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
# set_run_ref_globs sets ref_globs to the git log arguments that select only run $1's records refs.
set_run_ref_globs() {
  ref_globs=("--glob=refs/heads/$1$branch_glob" "--glob=refs/remotes/origin/$1$branch_glob" "--glob=refs/tags/archive/$1/*")
}

declare -A batch_start_sha
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
    set_run_ref_globs "$fork_run"
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
    batch_start_sha["$fork_rel"]="$start_sha"
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

# classify_span prints `class<TAB>detail` for one pack span entry, checked against the code repository at the batch's start sha.
# correct: the signature text lies inside the span; correct-decl: a declaration of its name lies inside it (Go files only);
# stale: found elsewhere in the file, detail is the line shift; wrong: file present, name absent (Go files only);
# unchecked: a non-Go file whose signature text was not found; nofile: the file is absent at the sha;
# notree: the batch is unresolved or the sha is not in the code repository.
classify_span() {
  local sha="$1" file="$2" first="$3" last="$4" signature="$5" is_go=0
  [[ "$file" == *.go ]] && is_go=1
  if [ -z "$sha" ] || ! git -C "$code" cat-file -e "$sha^{commit}" 2> /dev/null; then
    printf 'notree\t-\n'
    return
  fi
  if ! git -C "$code" cat-file -e "$sha:$file" 2> /dev/null; then
    printf 'nofile\t-\n'
    return
  fi
  git -C "$code" show "$sha:$file" | SIGNATURE="$signature" awk -v first="$first" -v last="$last" -v is_go="$is_go" '
    function decl(text, name) {
      return (text ~ ("^func (\\([^)]*\\) )?" name "[(\\[]") || text ~ ("^(type|const|var) " name "([^A-Za-z0-9_]|$)") || text ~ ("^[ \t]+" name "([ \t]+[^ \t(]|[ \t]*=|[ \t]*$)"))
    }
    BEGIN {
      signature = ENVIRON["SIGNATURE"]
      stripped = signature
      if (stripped ~ /^func \(/) { sub(/^func \([^)]*\) /, "", stripped) } else { sub(/^(func|type|const|var) /, "", stripped) }
      match(stripped, /^[A-Za-z0-9_]+/)
      name = substr(stripped, 1, RLENGTH)
    }
    { text[NR] = $0 }
    END {
      for (i = first; i <= last && i <= NR; i++) { if (index(text[i], signature) > 0) { print "correct\t0"; exit } }
      if (is_go) { for (i = first; i <= last && i <= NR; i++) { if (name != "" && decl(text[i], name)) { print "correct-decl\t0"; exit } } }
      for (i = 1; i <= NR; i++) { if (index(text[i], signature) > 0 || (is_go && name != "" && decl(text[i], name))) { print "stale\t" (i - first); exit } }
      if (is_go) { print "wrong\t" name } else { print "unchecked\t-" }
    }'
}

# Pack correctness: every span entry of each fork's pack is classed, and the files the fork edited, wrote or read
# that its pack gave no span for are listed with whether they existed at the batch's start.
# pack-entries.tsv holds every fork's pack entries, pack-check.tsv one row per span entry
# (columns: transcript path, run, card, glyph, file, first line, last line, class, detail) and
# missed-files.tsv one row per missed file (columns: transcript path, run, file, edits, writes, reads, existed).
: > "$work/pack-entries.tsv"
: > "$work/pack-check.tsv"
: > "$work/missed-files.tsv"
if [ "$pack_check" = 1 ]; then
  declare -A span_files
  while IFS=$'\t' read -r fork_rel fork_run; do
    transcript="$projects/$fork_rel"
    sha="${batch_start_sha[$fork_rel]:-}"
    entries="$(jq -n -R -r --arg rel "$fork_rel" -f "$script_dir/pack-entries.jq" "$transcript")" || fail "cannot read pack entries: $transcript"
    [ -z "$entries" ] || printf '%s\n' "$entries" >> "$work/pack-entries.tsv"
    while IFS=$'\t' read -r _ card kind glyph file first last signature; do
      [ "$kind" = span ] || continue
      span_files["$fork_rel|$file"]=1
      IFS=$'\t' read -r class detail <<< "$(classify_span "$sha" "$file" "$first" "$last" "$signature")"
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$fork_rel" "$fork_run" "$card" "$glyph" "$file" "$first" "$last" "$class" "$detail" >> "$work/pack-check.tsv"
    done <<< "$entries"
    while IFS=$'\t' read -r _ touched edits writes reads; do
      [ -z "${span_files["$fork_rel|$touched"]:-}" ] || continue
      existed=unknown
      if [ -n "$sha" ] && git -C "$code" cat-file -e "$sha^{commit}" 2> /dev/null; then
        if git -C "$code" cat-file -e "$sha:$touched" 2> /dev/null; then existed=yes; else existed=no; fi
      fi
      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$fork_rel" "$fork_run" "$touched" "$edits" "$writes" "$reads" "$existed" >> "$work/missed-files.tsv"
    done < <(jq -n -R -r --arg rel "$fork_rel" --arg prefix "$hub/$fork_run/" --arg record_root "$RUN_RECORD_ROOT" --arg logs_root "$LOGS_ROOT" \
      -f "$script_dir/touched-files.jq" "$transcript")
  done < <(jq -r '[.rel, .proj] | @tsv' "$work/records.jsonl")
fi

# Plan glyph use (--plan): per run with forks in the report, the plan cards are read from the records repository
# at the newest commit on the run's refs that touches the state path, and each card's references under
# Edit, Create, Move, Delete, Rename and Uses are classed glyph, plan: handle, bare file or bare directory.
# The run's plan writer is its top-level session (never a subagent) with the most plan card writes, ties broken by the lower session id.
# plan-runs.tsv: run, records commit, plan-writer session; a dash marks what was not found, and a run missing either counts in no table.
# plan-cards.tsv: run, commit, card.
# plan-refs.tsv: run, commit, card, label, kind, reference.
# plan-writers.tsv: run, session, plan card writes, quarry calls, greps, reads.
: > "$work/plan-runs.tsv"
: > "$work/plan-cards.tsv"
: > "$work/plan-refs.tsv"
: > "$work/plan-writers.tsv"
if [ "$plan" = 1 ]; then
  while IFS= read -r plan_run; do
    set_run_ref_globs "$plan_run"
    plan_commit="$(git -C "$records" log -1 --format=%H "${ref_globs[@]}" -- "$STATE_PATH" 2> /dev/null)"
    if [ -z "$plan_commit" ]; then
      printf '%s\t-\t-\n' "$plan_run" >> "$work/plan-runs.tsv"
      continue
    fi
    while IFS= read -r plan_file; do
      card_name="$(basename "$plan_file" .md)"
      printf '%s\t%s\t%s\n' "$plan_run" "$plan_commit" "$card_name" >> "$work/plan-cards.tsv"
      git -C "$records" show "$plan_commit:$plan_file" | awk -v run="$plan_run" -v commit="$plan_commit" -v card="$card_name" '
        /^\*\*[A-Za-z]+:\*\*/ { label = $0; sub(/^\*\*/, "", label); sub(/:\*\*.*/, "", label); next }
        /^- `/ {
          if (label !~ /^(Edit|Create|Move|Delete|Rename|Uses)$/) next
          reference = $0; sub(/^- `/, "", reference); sub(/`.*/, "", reference)
          if (reference ~ /^plan:/) { kind = "planhandle" }
          else if (reference ~ /#/) { kind = "glyph" }
          else { base = reference; sub(/.*\//, "", base); if (reference ~ /\/$/ || base !~ /\./) { kind = "bare_dir" } else { kind = "bare_file" } }
          print run "\t" commit "\t" card "\t" label "\t" kind "\t" reference
        }' >> "$work/plan-refs.tsv"
    done < <(git -C "$records" ls-tree -r --name-only "$plan_commit" -- "$PLAN_DIR" | grep -v '/00-overview')
    writer_rows=""
    shopt -s nullglob
    sessions=("$projects/$hub_encoded-$plan_run"/*.jsonl)
    shopt -u nullglob
    for session_file in "${sessions[@]}"; do
      session_row="$(jq -n -R -r --arg session "$(basename "$session_file" .jsonl)" --arg plan "$PLAN_DIR" -f "$script_dir/plan-writer.jq" "$session_file")" \
        || fail "cannot read session: $session_file"
      writer_rows+="$session_row"$'\n'
    done
    writer_row="$(printf '%s' "$writer_rows" | awk -F'\t' '$2 > 0' | sort -t$'\t' -k2,2nr -k1,1 | head -1)"
    if [ -z "$writer_row" ]; then
      printf '%s\t%s\t-\n' "$plan_run" "$plan_commit" >> "$work/plan-runs.tsv"
      continue
    fi
    printf '%s\t%s\t%s\n' "$plan_run" "$plan_commit" "${writer_row%%$'\t'*}" >> "$work/plan-runs.tsv"
    printf '%s\t%s\n' "$plan_run" "$writer_row" >> "$work/plan-writers.tsv"
  done < <(jq -r '.proj' "$work/records.jsonl" | sort -u)
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
  if [ "$pack_check" = 1 ]; then
    echo "- Code: \`$code\`"
  fi
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
      --argjson pack_check "$([ "$pack_check" = 1 ] && echo true || echo false)" \
      --rawfile pack_rows "$work/pack-check.tsv" --rawfile missed_rows "$work/missed-files.tsv" \
      --argjson plan_on "$([ "$plan" = 1 ] && echo true || echo false)" \
      --rawfile plan_runs "$work/plan-runs.tsv" --rawfile plan_cards "$work/plan-cards.tsv" \
      --rawfile plan_refs "$work/plan-refs.tsv" --rawfile plan_writers "$work/plan-writers.tsv" \
      -f "$script_dir/report.jq" "$work/records.jsonl" || fail "cannot render the report"
  fi
} > "$report_tmp"

mv -f -- "$report_tmp" "$out"
report_moved=1
echo "$out"
