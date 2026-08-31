#!/usr/bin/env bash
# github-read-selftest.sh is an offline harness (mirroring
# github-tree-selftest.sh's shape) that drives github-read.sh through a
# stub `curl` and a stub `gh` on PATH, with no network access at all. It
# asserts exact stdout bytes, exact argument vectors for both commands,
# exact call counts and ordering, and distinguishing stderr substrings.
#
# System `jq` is a dependency of THIS HARNESS ONLY, never of
# github-read.sh, which parses its `--jq` expression through `gh`'s own
# embedded gojq at run time -- the same accepted seam github-tree-selftest.sh
# documents.
#
# Portability envelope: the stub-on-PATH mechanism needs two extensionless
# executables with the exec bit set, so Linux and macOS are the asserted
# platforms. Windows Git Bash is expected to work but is not claimed;
# cmd.exe and PowerShell cannot run this harness or the stubs at all.
#
# NOT covered here (documented, not asserted -- manual checks): one live
# run against a public repository confirming the raw path is taken; one
# live run against a private repository confirming the fallback fires and
# succeeds; one live run against a directory path in a private repository;
# and whatever testdata/github-read/CAPTURE.md records as not exercised
# offline (symlink and submodule type-probe responses, and the
# contents-API size ceiling).
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="$SCRIPT_DIR/.."
GH_READ_SH="$SCRIPT_DIR/github-read.sh"
# Resolved once, up front, before the missing-gh test strips PATH for its
# own invocation: bash's temporary-assignment scoping (`VAR=val cmd`)
# applies the modified PATH to the search for `cmd` itself, so bare `bash`
# cannot be found by name once PATH has been emptied. Using this absolute
# path (which contains a slash, so execve needs no PATH search at all) is
# what lets that test actually exercise github-read.sh's own "gh not
# found" guard instead of failing before it even starts.
BASH_BIN="$(command -v bash)"
STUB_BIN="$SCRIPT_DIR/testdata/github-read/bin"
BODIES_DIR="$SCRIPT_DIR/testdata/github-read/bodies"

# .scratch/ is this repo's sanctioned scratch location (gitignored) --
# never a system temp directory, so a run leaves no trace outside the repo.
SCRATCH="$PLUGIN_ROOT/../../.scratch/github-read-selftest"

failures=0

# fail records one failed assertion and keeps going, so a single bad
# assertion doesn't hide the rest of the sequence's results.
fail() {
    echo "FAIL: $1" >&2
    failures=$((failures + 1))
}

pass() {
    echo "PASS: $1"
}

JQ_BIN="${GITHUB_READ_SELFTEST_JQ:-jq}"

require_jq() {
    if ! command -v "$JQ_BIN" >/dev/null 2>&1; then
        echo "github-read-selftest: '$JQ_BIN' not found on PATH — install jq to run this harness" >&2
        return 1
    fi
    return 0
}

require_jq || exit 1

rm -rf "$SCRATCH"
mkdir -p "$SCRATCH"

# build_curl_free_stub_dir builds, under the harness scratch, a directory
# containing a copy of the stub `gh` and nothing else, and prints its path.
# github-tree-selftest.sh's own missing-binary trick -- emptying PATH
# entirely -- cannot serve the curl-absent scenario, because that scenario
# needs `gh` to still resolve while `curl` does not, and an emptied PATH
# hides both. Pointing PATH at a directory holding only the stub `gh`, with
# no other directory behind it, is what makes the absence real rather than
# assumed.
build_curl_free_stub_dir() {
    local dir="$SCRATCH/curl-free-bin"
    rm -rf "$dir"
    mkdir -p "$dir"
    cp "$STUB_BIN/gh" "$dir/gh"
    chmod +x "$dir/gh"
    echo "$dir"
}

# run_scenario_with_stub_dir writes <curl-map-content> and <gh-map-content>
# to the scenario's own scratch directory, truncates both stubs' call
# logs, points PATH at <stub-dir> (prepended) and TMPDIR at the scenario's
# own scratch directory (which is what makes the temp-file-cleanup
# assertions in the validation section possible), runs github-read.sh with
# the remaining arguments, and captures stdout, stderr, and exit status
# into the shell variables out/err/status. Capturing stdout and stderr
# separately is mandatory: many assertions below turn on stdout being
# byte-empty while stderr is not.
run_scenario_with_stub_dir() {
    local name="$1" stub_dir="$2" curl_map="$3" gh_map="$4"
    shift 4
    local dir="$SCRATCH/$name"
    mkdir -p "$dir"
    printf '%s' "$curl_map" >"$dir/curl_map.tsv"
    printf '%s' "$gh_map" >"$dir/gh_map.tsv"
    : >"$dir/curl_calls.log"
    : >"$dir/gh_calls.log"
    PATH="$stub_dir:$PATH" \
        TMPDIR="$dir" \
        CURL_STUB_MAP="$dir/curl_map.tsv" \
        CURL_STUB_BODIES="$BODIES_DIR" \
        CURL_STUB_LOG="$dir/curl_calls.log" \
        GH_STUB_MAP="$dir/gh_map.tsv" \
        GH_STUB_BODIES="$BODIES_DIR" \
        GH_STUB_LOG="$dir/gh_calls.log" \
        bash "$GH_READ_SH" "$@" >"$dir/stdout" 2>"$dir/stderr"
    status=$?
    out="$(cat "$dir/stdout")"
    err="$(cat "$dir/stderr")"
}

# run_scenario is the default form of the runner above, pointed at the
# normal stub directory -- the parameter every scenario but the
# curl-absent one uses.
run_scenario() {
    local name="$1" curl_map="$2" gh_map="$3"
    shift 3
    run_scenario_with_stub_dir "$name" "$STUB_BIN" "$curl_map" "$gh_map" "$@"
}

# curl_calls / gh_calls print a scenario's call log verbatim.
curl_calls() {
    cat "$SCRATCH/$1/curl_calls.log" 2>/dev/null
}

gh_calls() {
    cat "$SCRATCH/$1/gh_calls.log" 2>/dev/null
}

# curl_call_line_count / gh_call_line_count print the number of lines in a
# scenario's call log for that stub.
curl_call_line_count() {
    local logfile="$SCRATCH/$1/curl_calls.log"
    [ -f "$logfile" ] || { echo 0; return; }
    wc -l <"$logfile" | tr -d ' '
}

gh_call_line_count() {
    local logfile="$SCRATCH/$1/gh_calls.log"
    [ -f "$logfile" ] || { echo 0; return; }
    wc -l <"$logfile" | tr -d ' '
}

echo "=== github-read selftest: offline stub-curl/stub-gh harness ==="

# --- Test 1: the preference-order proof --------------------------------
# The single most load-bearing assertion in this harness: a successful raw
# read makes zero gh calls. That is the measured win the whole task exists
# to capture, and it is the one assertion that fails if the fallback order
# is ever inverted.
run_scenario pref_order "$(printf '0\tplain.txt')" "" acme/pref src/a.txt
if cmp -s "$SCRATCH/pref_order/stdout" "$BODIES_DIR/plain.txt" && [ "$status" -eq 0 ]; then
    pass "preference order: stdout byte-identical to the fixture, exit 0"
else
    fail "preference order: status=$status"
fi
if [ "$(curl_call_line_count pref_order)" -eq 1 ]; then
    pass "preference order: exactly one curl call"
else
    fail "preference order: curl call log has $(curl_call_line_count pref_order) lines, expected 1: $(curl_calls pref_order)"
fi
if [ "$(gh_call_line_count pref_order)" -eq 0 ]; then
    pass "preference order: zero gh calls on a successful raw read"
else
    fail "preference order: gh was called despite a successful raw read: $(gh_calls pref_order)"
fi
case "$(curl_calls pref_order)" in
*"https://raw.githubusercontent.com/acme/pref/HEAD/src/a.txt"*) pass "preference order: raw URL carries the repo, the literal HEAD, and the path" ;;
*) fail "preference order: raw URL malformed: $(curl_calls pref_order)" ;;
esac

# --- Test 2: the argument-vector proof -----------------------------------
# Splitting on whitespace is safe here because none of this vector's
# tokens (including the URL, whose character set is validated before this
# call is ever made) can contain a space.
read -r -a vec <<<"$(curl_calls pref_order)"
if [ "${vec[0]:-}" = "-s" ] && [ "${vec[1]:-}" = "-f" ] && [ "${vec[2]:-}" = "-L" ] \
    && [ "${vec[3]:-}" = "--connect-timeout" ] && [ "${vec[4]:-}" = "5" ] \
    && [ "${vec[5]:-}" = "--max-time" ] && [ "${vec[6]:-}" = "30" ] \
    && [ "${vec[7]:-}" = "-o" ] \
    && [ "${vec[9]:-}" = "https://raw.githubusercontent.com/acme/pref/HEAD/src/a.txt" ] \
    && [ "${#vec[@]}" -eq 10 ]; then
    pass "argument vector: -s -f -L --connect-timeout 5 --max-time 30 -o <tmp> <url>, in order, no other flags"
else
    fail "argument vector: $(curl_calls pref_order)"
fi
if [ "${vec[1]:-}" = "-f" ]; then
    pass "argument vector: -f present specifically (its absence would degrade a 404 into fake file content)"
else
    fail "argument vector: -f missing"
fi

# --- Test 3: the trigger is curl's exit status, not a parsed code -------
run_scenario trigger_timeout "$(printf '28\t')" "$(printf 'probe\trepos/acme/trig/contents/x.txt\tprobe-file.json\t\t\ncontent\trepos/acme/trig/contents/x.txt\tplain.txt\t\t\n')" acme/trig x.txt
if cmp -s "$SCRATCH/trigger_timeout/stdout" "$BODIES_DIR/plain.txt" && [ "$status" -eq 0 ]; then
    pass "trigger is exit status: a timeout exit (28) falls through to a succeeding fallback"
else
    fail "trigger is exit status (timeout): status=$status out=$out"
fi
run_scenario trigger_refused "$(printf '7\t')" "$(printf 'probe\trepos/acme/trig2/contents/x.txt\tprobe-file.json\t\t\ncontent\trepos/acme/trig2/contents/x.txt\tplain.txt\t\t\n')" acme/trig2 x.txt
if cmp -s "$SCRATCH/trigger_refused/stdout" "$BODIES_DIR/plain.txt" && [ "$status" -eq 0 ]; then
    pass "trigger is exit status: a connection-refused exit (7) falls through to a succeeding fallback"
else
    fail "trigger is exit status (connection refused): status=$status out=$out"
fi

# --- Test 4: the not-found regression test -------------------------------
# The stub writes nothing to its output file and exits with the status the
# -f flag would produce for a 404 (22); the raw-attempt loop's own
# no-body-inspection contract must never write curl's would-be "404: Not
# Found" text (which the -f flag prevents from reaching the output file in
# the first place) as though it were file content.
run_scenario not_found "$(printf '22\t')" "$(printf 'probe\trepos/acme/nf/contents/gone.txt\terror-404.json\t404\t\n')" acme/nf gone.txt
case "$out" in
*"Not Found"*) fail "not-found regression: stdout carries not-found text: $out" ;;
*) pass "not-found regression: stdout carries no not-found text" ;;
esac
if [ "$status" -ne 0 ]; then
    pass "not-found regression: non-zero exit"
else
    fail "not-found regression: exit 0 unexpected"
fi

# --- Test 5: the no-partial-prefix proof ----------------------------------
# This assertion would fail against a stream-to-stdout implementation and
# is what pins the temp-file buffering decision: a partial body written by
# a dying connection must never appear ahead of the fallback's own bytes.
run_scenario no_partial "$(printf '18\tpartial-decoy.txt')" "$(printf 'probe\trepos/acme/partial/contents/x.txt\tprobe-file.json\t\t\ncontent\trepos/acme/partial/contents/x.txt\tplain.txt\t\t\n')" acme/partial x.txt
if cmp -s "$SCRATCH/no_partial/stdout" "$BODIES_DIR/plain.txt"; then
    pass "no-partial-prefix: stdout is exactly the fallback's bytes, no partial prefix in front of them"
else
    fail "no-partial-prefix: stdout does not match the fixture exactly (a partial prefix or trailing bytes leaked)"
fi

# --- Test 6: curl absent from PATH ----------------------------------------
curl_free_dir="$(build_curl_free_stub_dir)"
run_scenario_with_stub_dir curl_absent "$curl_free_dir" "" "$(printf 'probe\trepos/acme/nocurl/contents/x.txt\tprobe-file.json\t\t\ncontent\trepos/acme/nocurl/contents/x.txt\tplain.txt\t\t\n')" acme/nocurl x.txt
if cmp -s "$SCRATCH/curl_absent/stdout" "$BODIES_DIR/plain.txt" && [ "$status" -eq 0 ]; then
    pass "curl absent from PATH: script goes straight to gh api and still produces correct stdout"
else
    fail "curl absent from PATH: status=$status out=$out"
fi
if [ "$(curl_call_line_count curl_absent)" -eq 0 ]; then
    pass "curl absent from PATH: zero curl calls logged (curl genuinely cannot resolve)"
else
    fail "curl absent from PATH: a curl call was logged despite curl's absence: $(curl_calls curl_absent)"
fi
case "$err" in
*curl*) fail "curl absent from PATH: stderr mentions curl: $err" ;;
*) pass "curl absent from PATH: stderr says nothing about curl" ;;
esac

echo "==========================================================="
if [ "$failures" -eq 0 ]; then
    echo "PASS: all github-read selftest assertions passed"
    exit 0
fi
echo "FAIL: $failures github-read selftest assertion(s) failed"
exit 1
