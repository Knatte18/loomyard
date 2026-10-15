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
WORK_FILES=(projects.txt candidates.txt)

usage() {
  cat >&2 <<'EOF'
Usage: forkaudit.sh (--since <date> [--until <date>] | --runs <name>[,<name>...]) [options]
Options:
  --projects <dir>   Claude Code projects directory (default $HOME/.claude/projects)
  --hub <dir>        hub whose worktrees are audited (default the repo root's parent)
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
since="" until_date="" runs="" projects="${HOME:-}/.claude/projects" hub="" out="" work=""
since_set=0 until_set=0 runs_set=0
while [ $# -gt 0 ]; do
  case "$1" in
    --since|--until|--runs|--projects|--hub|--out|--work)
      [ $# -ge 2 ] || usage_error "$1 needs a value"
      case "$1" in
        --since) since="$2"; since_set=1 ;;
        --until) until_date="$2"; until_set=1 ;;
        --runs) runs="$2"; runs_set=1 ;;
        --projects) projects="$2" ;;
        --hub) hub="$2" ;;
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
  echo
}

{
  write_header
  echo "No Webster implementer forks matched."
} > "$report_tmp"

mv -f -- "$report_tmp" "$out"
report_moved=1
echo "$out"
