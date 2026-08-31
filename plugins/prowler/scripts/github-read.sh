#!/usr/bin/env bash
# github-read.sh reads exactly one file from a GitHub repository and writes
# its content verbatim to stdout and nothing else. It prefers a direct
# https://raw.githubusercontent.com/ fetch and falls back to `gh api` only
# when the raw fetch fails -- the raw host is measurably faster (no
# authenticated-API round trip) and the fallback exists purely to cover
# what raw cannot: private repositories, and the rare host-level hiccup.
#
# There are deliberately no retries and no backoff anywhere in this script,
# the same policy github-tree.sh documents. The `--connect-timeout` and
# `--max-time` bounds given to curl are not retries either -- they exist
# only to turn a hung raw request into one clean non-zero exit within a
# bounded time, so the single `gh api` fallback attempt actually gets a
# turn instead of the script hanging forever on a dead connection.
#
# Like github-tree.sh, this script reads no file inside the plugin -- it
# takes an owner/repo and a path and calls `curl`/`gh`, nothing else -- so
# it does not self-locate a SCRIPT_DIR/PLUGIN_ROOT.
#
# This script reads exactly one file per invocation: stdout carries that
# file's content verbatim and nothing else, every diagnostic goes to
# stderr, and a read is pinned to `HEAD` -- there is no ref argument, and
# no way to read anything but the default branch's current content.
set -u

# die prints one message to stderr and exits 1. It is not used for the
# usage error, which is the one case that exits 2 instead.
die() {
    echo "$1" >&2
    exit 1
}

# usage prints the usage line to stderr and exits 2. It is deliberately not
# routed through die, which exits 1 -- exit 2 is reserved for malformed
# invocations, exit 1 for every operational failure.
usage() {
    echo "github-read: usage: github-read.sh <owner/repo> <path>" >&2
    exit 2
}

# --- Prerequisite and argument handling, in this exact order, so that a --
# --- rejection never reaches the network. --------------------------------

if ! command -v gh >/dev/null 2>&1; then
    die "github-read: gh not found on PATH — install the GitHub CLI and authenticate it (gh auth login)"
fi

# `curl` is not checked here: its absence is never an error, only a loss of
# the fast path (see the raw-attempt section below).

# Parse arguments with the same loop shape github-tree.sh uses, minus the
# two flags this script does not have: a `--` terminator is honoured at any
# position and consumed, every token after it is a positional unexamined,
# and before it any token beginning with two dashes is a usage error while
# every other token (including a single-dash token, which is never treated
# as a flag) is a positional.
args=()
terminated=0
while [ "$#" -gt 0 ]; do
    if [ "$terminated" -eq 1 ]; then
        args+=("$1")
        shift
        continue
    fi
    case "$1" in
    --)
        terminated=1
        shift
        ;;
    --*)
        usage
        ;;
    *)
        args+=("$1")
        shift
        ;;
    esac
done

# The count is checked on the post-terminator positional list, so a
# terminator plus a doubly-dashed path is a legitimate two-positional call
# rather than a count error.
[ "${#args[@]}" -eq 2 ] || usage

REPO="${args[0]}"
RAW_PATH="${args[1]}"

# Copied verbatim from github-tree.sh, including its bracket-range regex
# form: the collation looseness this form carries under some locales is an
# accepted property there, and divergence between the two scripts' slug
# checks would be the worse outcome.
if ! [[ "$REPO" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
    die "github-read: '$REPO' is not a valid <owner>/<repo> reference"
fi

# Normalize the path: strip every leading and trailing '/'.
path="$RAW_PATH"
while [[ "$path" == /* ]]; do path="${path#/}"; done
while [[ "$path" == */ ]]; do path="${path%/}"; done

# Unlike github-tree.sh, an empty path after normalisation is a usage error
# rather than a whole-repo listing: there is no such thing as reading an
# entire repository as one file.
if [ -z "$path" ]; then
    usage
fi

# Validate by deleting every accepted character from a copy of the path and
# checking what remains, rather than testing the whole string against
# `[[ =~ ^[A-Za-z0-9._/-]+$ ]]` or indexing it byte by byte to find "the
# first offender".
#
# Regex bracket ranges like A-Z are collation-ordered, not codepoint-
# ordered: under a UTF-8 locale (this box's default), glibc's regex engine
# treats accented letters such as 'ï' as falling inside that collation
# range, so `[[ "naïve" =~ ^[A-Za-z0-9._/-]+$ ]]` wrongly matches -- a real,
# reproduced failure mode, not a hypothetical one. Bash's glob-pattern
# matching (which parameter-expansion substitution uses) does not have this
# problem, so it is used for both detection and the offending-character
# report.
#
# Byte indexing has a separate, independent problem: it is byte-oriented
# under a C/POSIX locale, which minimal shells and CI images routinely run
# with and which this script deliberately does not pin, so indexing would
# slice a multi-byte UTF-8 character into single invalid bytes and report
# one of them as "the character". The glob substitution below is immune to
# that too: the accepted set is pure ASCII, so a UTF-8 continuation byte is
# never in it and no multi-byte sequence can be split -- the remainder
# always holds each offending character whole. Reporting the full remaining
# set rather than just the first offender is the same choice made for the
# same reason: there is no first-offender extraction that is byte-safe
# without re-introducing the indexing problem.
#
# The accepted character set is a subset of URL-safe characters, which is
# what makes URL-encoding unnecessary when the path is interpolated
# directly into the raw URL below.
offending="${path//[A-Za-z0-9._\/-]/}"
if [ -n "$offending" ]; then
    die "github-read: path '$path' contains unsupported character(s) '$offending' — only [A-Za-z0-9._/-] is accepted"
fi

# Two temp files, created unconditionally so neither is unset under `set
# -u`, and a single EXIT trap removing both, armed immediately after both
# exist so no exit path -- including a `die` before either is used -- ever
# leaks either one.
BODY_FILE="$(mktemp)"
STDERR_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE" "$STDERR_FILE"' EXIT

# --- Raw attempt: the fast path ------------------------------------------
#
# A missing `curl` costs speed, not capability: fall straight through to
# the `gh api` fallback with no error and nothing written to stderr.
# Warning on every read would be noise the caller cannot act on, since the
# fallback covers this case completely.
if command -v curl >/dev/null 2>&1; then
    raw_url="https://raw.githubusercontent.com/$REPO/HEAD/$path"

    # Exactly this argument vector, no other flags:
    #   -f  turns every response at or above 400 into a non-zero exit with
    #       an empty output file, so curl's exit status alone is a
    #       sufficient failure signal and no HTTP status needs capturing
    #       or parsing. Without it a plain request answers a 404 with exit
    #       0 and a "404: Not Found" body that would be emitted as if it
    #       were the file's content.
    #   -s  (rather than -sS) is required because this script never
    #       reports the raw attempt's failure -- curl must write no
    #       progress or error text to stderr either.
    #   -L  follows a redirect the raw host may answer with; an unfollowed
    #       301 would be a spurious fallback.
    #   --connect-timeout 5 / --max-time 30 bound a hung request so it
    #       becomes one clean non-zero exit that hands off to the single
    #       `gh api` attempt exactly once. These are bounds, not retries.
    #   -o "$BODY_FILE" writes the body to the temp file rather than
    #       streaming it to stdout: streaming would leave a truncated
    #       prefix on stdout if the connection died mid-body after a 200,
    #       and the fallback would then append a second copy behind it.
    #       Command substitution is avoided for the same call for a
    #       different reason -- it strips every trailing newline and
    #       silently drops NUL bytes, corrupting byte fidelity on every
    #       read rather than only on failure.
    if curl -s -f -L --connect-timeout 5 --max-time 30 -o "$BODY_FILE" "$raw_url"; then
        cat "$BODY_FILE"
        exit 0
    fi
    # Failure is defined as curl's exit status being non-zero and nothing
    # else: no HTTP status is captured, parsed, or branched on, and body
    # emptiness is never the signal, because an empty file that read
    # successfully (the zero-byte-file case) is a valid outcome the
    # harness asserts explicitly. Fall through to the gh api fallback.
fi
