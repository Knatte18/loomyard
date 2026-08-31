#!/usr/bin/env bash
# github-code-search-selftest.sh is an offline harness (mirroring
# github-tree-selftest.sh's shape) that drives github-code-search.sh
# through a stub `gh` on PATH, with no network access at all. It asserts
# the exact stdout for every scenario, the exact call count and call
# identity `gh` was invoked with for every scenario, and a distinguishing
# stderr substring for every distinguished failure.
#
# System `jq` is a dependency of THIS HARNESS ONLY, never of
# github-code-search.sh, which parses its `--jq` expression through
# `gh`'s own embedded gojq at run time. That means this harness validates
# the jq expression under jq while production runs the identical
# expression under gojq -- an accepted seam that a future expression
# relying on a jq/gojq-divergent construct would fall through undetected
# here.
#
# Portability envelope: the stub-on-PATH mechanism needs an
# extensionless executable with the exec bit set, so Linux and macOS are
# the asserted platforms. Windows Git Bash is expected to work but is not
# claimed; cmd.exe and PowerShell cannot run this harness or the stub at
# all.
#
# NOT covered here (documented, not asserted -- manual checks per the
# batch plan's "Batch Tests" section): one live sweep across three real
# repos, confirming the record shape and that snippets arrive; one live
# run against a repo with more than 100 matches, confirming the cap note
# fires; one live run against a deliberately misspelled repo, confirming
# the preflight catches what the search API would otherwise have
# reported as zero hits; and a spot-check that the extraction expression
# behaves identically under gojq (production, via gh) and jq (the
# harness) -- the same acknowledged seam the sibling harness documents.
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="$SCRIPT_DIR/.."
GH_SEARCH_SH="$SCRIPT_DIR/github-code-search.sh"
# Resolved once, up front, before the "gh missing from PATH" scenario
# strips PATH for its own invocation: bash's temporary-assignment scoping
# (`VAR=val cmd`) applies the modified PATH to the search for `cmd`
# itself, so bare `bash` cannot be found by name once PATH has been
# emptied. Using this absolute path (which contains a slash, so execve
# needs no PATH search at all) is what lets that scenario actually
# exercise github-code-search.sh's own "gh not found" guard instead of
# failing before it even starts.
BASH_BIN="$(command -v bash)"
STUB_BIN="$SCRIPT_DIR/testdata/github-code-search/bin"
BODIES_DIR="$SCRIPT_DIR/testdata/github-code-search/bodies"

# .scratch/ is this repo's sanctioned scratch location (gitignored) --
# never a system temp directory, so a run leaves no trace outside the repo.
SCRATCH="$PLUGIN_ROOT/../../.scratch/github-code-search-selftest"

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

# JQ_BIN is read at call time inside require_jq (never captured at
# definition time), which is what lets the require_jq-guard scenario
# exercise the guard in a subshell -- overriding JQ_BIN there and calling
# require_jq directly -- with no recursion into this script to bound.
JQ_BIN="${GITHUB_CODE_SEARCH_SELFTEST_JQ:-jq}"

require_jq() {
    if ! command -v "$JQ_BIN" >/dev/null 2>&1; then
        echo "github-code-search-selftest: '$JQ_BIN' not found on PATH — install jq to run this harness" >&2
        return 1
    fi
    return 0
}

require_jq || exit 1

rm -rf "$SCRATCH"
mkdir -p "$SCRATCH"

# run_scenario writes <map-content> to $SCRATCH/<scenario-name>/map.tsv,
# truncates that scenario's call log, then runs github-code-search.sh
# with the stub gh prepended to PATH, capturing stdout, stderr, and the
# exit status into the shell variables out/err/status. Capturing stdout
# and stderr into separate variables is mandatory: a great many
# assertions below turn on stdout being byte-empty while stderr is not.
run_scenario() {
    local name="$1" map_content="$2"
    shift 2
    local dir="$SCRATCH/$name"
    mkdir -p "$dir"
    printf '%s' "$map_content" > "$dir/map.tsv"
    : > "$dir/calls.log"
    PATH="$STUB_BIN:$PATH" \
        GH_STUB_MAP="$dir/map.tsv" \
        GH_STUB_BODIES="$BODIES_DIR" \
        GH_STUB_LOG="$dir/calls.log" \
        bash "$GH_SEARCH_SH" "$@" >"$dir/stdout" 2>"$dir/stderr"
    status=$?
    out="$(cat "$dir/stdout")"
    err="$(cat "$dir/stderr")"
}

# calls prints a scenario's call log verbatim.
calls() {
    cat "$SCRATCH/$1/calls.log" 2>/dev/null
}

# call_line_count prints the number of lines in a scenario's call log.
call_line_count() {
    local logfile="$SCRATCH/$1/calls.log"
    [ -f "$logfile" ] || { echo 0; return; }
    wc -l < "$logfile" | tr -d ' '
}

# preflight_call_count <scenario> <owner/repo> counts logged calls that
# are exactly the four-token preflight shape naming <owner/repo>'s own
# endpoint. Every search call in a sweep shares one endpoint
# (search/code), so unlike the sibling tree harness's single
# call_count_for_endpoint helper, preflight and search calls need
# distinct counting helpers here.
preflight_call_count() {
    local scenario="$1" repo="$2" count=0
    local logfile="$SCRATCH/$scenario/calls.log"
    [ -f "$logfile" ] || { echo 0; return; }
    local want="api repos/$repo --jq .full_name"
    while IFS= read -r line || [ -n "$line" ]; do
        [ "$line" = "$want" ] && count=$((count + 1))
    done < "$logfile"
    echo "$count"
}

# search_call_count <scenario> <q-substring> counts logged calls that
# are search-shaped (begin with "api -X GET ") and whose logged argument
# vector contains <q-substring>. Matched by shell case/glob rather than
# grep, so a q value's own characters (e.g. a caller's search term) are
# never read as regex metacharacters.
search_call_count() {
    local scenario="$1" substr="$2" count=0
    local logfile="$SCRATCH/$scenario/calls.log"
    [ -f "$logfile" ] || { echo 0; return; }
    while IFS= read -r line || [ -n "$line" ]; do
        case "$line" in
        "api -X GET "*)
            case "$line" in
            *"$substr"*) count=$((count + 1)) ;;
            esac
            ;;
        esac
    done < "$logfile"
    echo "$count"
}

echo "=== github-code-search selftest: offline stub-gh harness ==="

# --- Test 1: single repo, several hits, fragment sanitation -----------------
run_scenario multi "$(printf 'repos/acme/multi\tpreflight-ok.json\nsearch/code?q=tree-sitter repo:acme/multi\thits-multi.json\n')" tree-sitter acme/multi
expected="$(printf 'acme/multi\tdocs/guide.md\tuse tree-sitter for parsing\nacme/multi\tsrc/parser.rs\tlet ts = tree_sitter::Parser\nacme/multi\tnotes.txt\tbump tree-sitter')"
if [ "$out" = "$expected" ] && [ "$status" -eq 0 ]; then
    pass "single repo, several hits: exact stdout, embedded newline and tab collapsed to a single space"
else
    fail "single repo, several hits: status=$status out=$out"
fi

# --- Test 2: single repo, zero hits ------------------------------------------
run_scenario zerohit "$(printf 'repos/acme/zero\tpreflight-ok.json\nsearch/code?q=widget repo:acme/zero\thits-zero.json\n')" widget acme/zero
if [ "$status" -eq 0 ] && [ -z "$out" ] \
    && [ "$(preflight_call_count zerohit acme/zero)" -eq 1 ] \
    && [ "$(search_call_count zerohit 'repo:acme/zero')" -eq 1 ]; then
    pass "zero hits: exit 0, byte-empty stdout, preflight still made"
else
    fail "zero hits: status=$status out=$out preflight=$(preflight_call_count zerohit acme/zero) search=$(search_call_count zerohit 'repo:acme/zero')"
fi

# --- Test 3: multiple repos: ordering, call count, and call identity --------
three_map="$(printf 'repos/acme/gamma\tpreflight-ok.json\nrepos/acme/alpha\tpreflight-ok.json\nrepos/acme/beta\tpreflight-ok.json\nsearch/code?q=widget repo:acme/gamma\thits-gamma.json\nsearch/code?q=widget repo:acme/alpha\thits-alpha.json\nsearch/code?q=widget repo:acme/beta\thits-beta.json\n')"
run_scenario three "$three_map" widget acme/gamma acme/alpha acme/beta
expected="$(printf 'acme/gamma\tgamma.rs\tgamma hit\nacme/alpha\tsrc/alpha1.go\talpha one\nacme/alpha\tsrc/alpha2.go\talpha two\nacme/beta\tbeta.md\tbeta hit')"
if [ "$out" = "$expected" ] && [ "$status" -eq 0 ]; then
    pass "multiple repos: exact stdout in repo-argument order, fixture item order within each block"
else
    fail "multiple repos: status=$status out=$out"
fi
if [ "$(call_line_count three)" -eq 6 ]; then
    pass "multiple repos: exactly six gh calls (three preflight, three search)"
else
    fail "multiple repos: call log has $(call_line_count three) lines, expected 6: $(calls three)"
fi
if [ "$(preflight_call_count three acme/gamma)" -eq 1 ] && [ "$(preflight_call_count three acme/alpha)" -eq 1 ] \
    && [ "$(preflight_call_count three acme/beta)" -eq 1 ] \
    && [ "$(search_call_count three 'repo:acme/gamma')" -eq 1 ] && [ "$(search_call_count three 'repo:acme/alpha')" -eq 1 ] \
    && [ "$(search_call_count three 'repo:acme/beta')" -eq 1 ]; then
    pass "multiple repos: each repo's own search call is present with its own repo: qualifier"
else
    fail "multiple repos: call identity mismatch: $(calls three)"
fi

# --- Test 4: every search invocation carries -X GET, the text-match Accept --
# --- header, and --jq --------------------------------------------------------
run_scenario three "$three_map" widget acme/gamma acme/alpha acme/beta
all_get=1
all_accept=1
all_jq=1
while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
    "api -X GET "*)
        case "$line" in
        *"-X GET"*) : ;;
        *) all_get=0 ;;
        esac
        case "$line" in
        *"Accept: application/vnd.github.text-match+json"*) : ;;
        *) all_accept=0 ;;
        esac
        case "$line" in
        *"--jq"*) : ;;
        *) all_jq=0 ;;
        esac
        ;;
    esac
done < "$SCRATCH/three/calls.log"
if [ "$all_get" -eq 1 ] && [ "$all_accept" -eq 1 ] && [ "$all_jq" -eq 1 ]; then
    pass "search invocations: every one carries -X GET, the text-match Accept header, and --jq"
else
    fail "search invocations: missing required part(s) in the call log: $(calls three)"
fi

# --- Test 5: fragment truncation and multiple text_matches -------------------
run_scenario trunc "$(printf 'repos/acme/trunc\tpreflight-ok.json\nsearch/code?q=widget repo:acme/trunc\thits-truncate.json\n')" widget acme/trunc
first_line="${out%%$'\n'*}"
second_line="${out#*$'\n'}"
IFS=$'\t' read -r f1_full f1_path f1_frag <<<"$first_line"
IFS=$'\t' read -r f2_full f2_path f2_frag <<<"$second_line"
if [ "$status" -eq 0 ] && [ "${#f1_frag}" -eq 200 ] && [ "$f2_frag" = "first fragment" ]; then
    pass "fragment truncation: first record's snippet is exactly 200 chars, second record uses only the first of two text_matches"
else
    fail "fragment truncation: status=$status len=${#f1_frag} f2_frag=$f2_frag out=$out"
fi

# --- Test 6: an item whose text_matches array is absent or empty ------------
run_scenario nomatch "$(printf 'repos/acme/nomatch\tpreflight-ok.json\nsearch/code?q=widget repo:acme/nomatch\thits-no-textmatches.json\n')" widget acme/nomatch
expected="$(printf 'acme/nomatch\tabsent.txt\t\nacme/nomatch\tempty.txt\t')"
if [ "$out" = "$expected" ] && [ "$status" -eq 0 ]; then
    pass "absent/empty text_matches: two records emitted with an empty (but present) third field"
else
    fail "absent/empty text_matches: status=$status out=$out"
fi

# --- Test 7: total_count greater than the returned item count ---------------
run_scenario capped "$(printf 'repos/acme/capped\tpreflight-ok.json\nsearch/code?q=widget repo:acme/capped\tcapped.json\n')" widget acme/capped
expected="$(printf 'acme/capped\tc1.txt\tcap one\nacme/capped\tc2.txt\tcap two')"
if [ "$out" = "$expected" ] && [ "$status" -eq 0 ] && [[ "$err" == *"acme/capped"* ]] && [[ "$err" == *"250"* ]]; then
    pass "capped total_count: full two-record stdout, one stderr note naming the repo and the true total"
else
    fail "capped total_count: status=$status out=$out err=$err"
fi

# --- Test 8: duplicate repo refs are deduped, preserving first-occurrence ---
# --- order --------------------------------------------------------------------
dup_map="$(printf 'repos/acme/alpha\tpreflight-ok.json\nrepos/acme/beta\tpreflight-ok.json\nsearch/code?q=widget repo:acme/alpha\thits-alpha.json\nsearch/code?q=widget repo:acme/beta\thits-beta.json\n')"
run_scenario dup "$dup_map" widget acme/alpha acme/beta acme/alpha
expected="$(printf 'acme/alpha\tsrc/alpha1.go\talpha one\nacme/alpha\tsrc/alpha2.go\talpha two\nacme/beta\tbeta.md\tbeta hit')"
if [ "$out" = "$expected" ] && [ "$status" -eq 0 ] \
    && [ "$(preflight_call_count dup acme/alpha)" -eq 1 ] && [ "$(preflight_call_count dup acme/beta)" -eq 1 ] \
    && [ "$(search_call_count dup 'repo:acme/alpha')" -eq 1 ] && [ "$(search_call_count dup 'repo:acme/beta')" -eq 1 ]; then
    pass "duplicate refs: deduped, first-occurrence order, exactly two preflight and two search calls"
else
    fail "duplicate refs: status=$status out=$out preflight_alpha=$(preflight_call_count dup acme/alpha) search_alpha=$(search_call_count dup 'repo:acme/alpha')"
fi

# --- Test 9: duplicate repo refs are deduped case-insensitively -------------
dupcase_map="$(printf 'repos/acme/Alpha\tpreflight-ok.json\nrepos/acme/beta\tpreflight-ok.json\nsearch/code?q=widget repo:acme/Alpha\thits-alpha.json\nsearch/code?q=widget repo:acme/beta\thits-beta.json\n')"
run_scenario dupcase "$dupcase_map" widget acme/Alpha acme/beta ACME/ALPHA
if [ "$status" -eq 0 ] \
    && [ "$(preflight_call_count dupcase acme/Alpha)" -eq 1 ] && [ "$(preflight_call_count dupcase acme/beta)" -eq 1 ] \
    && [ "$(search_call_count dupcase 'repo:acme/Alpha')" -eq 1 ] && [ "$(search_call_count dupcase 'repo:acme/beta')" -eq 1 ] \
    && [[ "$out" == *$'\n'* ]] \
    && { case "$out" in *"acme/alpha"$'\t'*) true ;; *) false ;; esac; } \
    && { case "$out" in *"acme/Alpha"$'\t'*) false ;; *) true ;; esac; } \
    && { case "$out" in *"ACME/ALPHA"$'\t'*) false ;; *) true ;; esac; }; then
    pass "case-insensitive dedup: exactly two preflight and two search calls, emitted full_name is the API's own, not either caller spelling"
else
    fail "case-insensitive dedup: status=$status out=$out preflight=$(preflight_call_count dupcase acme/Alpha) search=$(search_call_count dupcase 'repo:acme/Alpha')"
fi

# --- Test 10: dedup happens before the cap -----------------------------------
tencap_map=""
tencap_args=()
for i in 01 02 03 04 05 06 07 08 09 10; do
    repo="acme/r$i"
    tencap_map+="repos/$repo"$'\t'"preflight-ok.json"$'\n'
    tencap_map+="search/code?q=widget repo:$repo"$'\t'"hits-zero.json"$'\n'
    tencap_args+=("$repo")
done
run_scenario tencap "$tencap_map" widget "${tencap_args[@]}" acme/r01
if [ "$status" -eq 0 ] && [ -z "$out" ] && [ "$(call_line_count tencap)" -eq 20 ]; then
    pass "dedup before cap: eleven refs (one duplicate) run normally as ten distinct calls each"
else
    fail "dedup before cap: status=$status out=$out call_lines=$(call_line_count tencap)"
fi

rm -rf "$SCRATCH"

echo "==========================================================="
if [ "$failures" -eq 0 ]; then
    echo "PASS: all github-code-search selftest assertions passed"
    exit 0
fi
echo "FAIL: $failures github-code-search selftest assertion(s) failed"
exit 1
