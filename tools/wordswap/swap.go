// swap.go implements the matching, case classification, and substitution core of tools/wordswap.
// main.go drives it one file at a time.
// It holds token-boundary detection, case-form recognition, and the single left-to-right scan that produces a Result.

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Occurrence records one place in a file where a candidate match for `from` was found but not substituted.
// It was either classified AMBIGUOUS or claimed by a -skip regexp.
// Text is the full text of the line the occurrence sits on.
// main.go prints it in the "<path>:<line>: <line text>" report format for each bucket.
type Occurrence struct {
	Line int
	Text string
}

// Result is the outcome of one substitution pass over a file's contents.
// Out is the output text, unchanged when the reversibility check fails.
// Ambiguous and Skipped are the two report buckets.
// Mismatch reports whether the reversibility check failed.
type Result struct {
	Out       string
	Ambiguous []Occurrence
	Skipped   []Occurrence
	Mismatch  bool
}

// caseForm reports which case form candidate takes relative to from: "lower", "title", or "upper".
// It reports false for a mixed form such as "hOst", which is not a match at all.
func caseForm(candidate, from string) (string, bool) {
	if candidate == strings.ToLower(from) {
		return "lower", true
	}
	if candidate == titleCase(from) {
		return "title", true
	}
	if candidate == strings.ToUpper(from) {
		return "upper", true
	}
	return "", false
}

// titleCase upper-cases the first byte of from and lower-cases the rest.
// The result is the camelCase/PascalCase start form, so "host" becomes "Host".
func titleCase(from string) string {
	lower := strings.ToLower(from)
	if lower == "" {
		return lower
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

// applyCase renders to in the given case form ("lower", "title", or "upper"), matching the case
// form the matched occurrence of from took.
func applyCase(form, to string) string {
	switch form {
	case "title":
		return titleCase(to)
	case "upper":
		return strings.ToUpper(to)
	default:
		return strings.ToLower(to)
	}
}

// isIdentByte reports whether b is an ASCII letter or digit.
// Such a byte continues a token for boundary purposes.
func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// boundaryBefore reports whether position i in s is a valid token start for a match in the given form.
// It is true at the start of s, and when the preceding byte is not an identifier byte.
// The "title" and "upper" forms are also valid after an identifier byte, as in camelCase and SCREAMING_CASE.
// A lowercase match after an identifier byte is rejected, so "ghost", "localhost", and "conhost" do not match.
func boundaryBefore(s string, i int, form string) bool {
	if i == 0 {
		return true
	}
	if !isIdentByte(s[i-1]) {
		return true
	}
	return form == "title" || form == "upper"
}

// boundaryAfter reports whether position i+n in s ends a token for a match.
// It is true at the end of s, and when the following byte is not an ASCII lowercase letter.
// A following lowercase letter means the match is classified AMBIGUOUS.
func boundaryAfter(s string, i, n int) bool {
	if i+n == len(s) {
		return true
	}
	next := s[i+n]
	if next >= 'a' && next <= 'z' {
		return false
	}
	return true
}

// swapText scans in once, left to right, for case-insensitive occurrences of from.
// It returns the substitution outcome as a Result.
// Each occurrence is classified in turn.
// A mixed case form, or an invalid boundary-before, rejects the occurrence entirely: no match, no report.
// A line matching a skip regexp leaves the occurrence unchanged and records it in Result.Skipped.
// An invalid boundary-after leaves it unchanged and records it in Result.Ambiguous.
// Any other occurrence is substituted with the matched case form applied to to.
// Its span is recorded for the reversibility check.
// Substitution is single-pass over the input;
// the output is never re-scanned, so a to produced by this run can never be re-matched.
// swapText returns an error only for a malformed argument, an empty from or to.
// A failed reversibility check is reported through Result.Mismatch, not as an error.
func swapText(in, from, to string, skips []*regexp.Regexp) (Result, error) {
	if from == "" || to == "" {
		return Result{}, fmt.Errorf("swapText: from and to must both be non-empty")
	}

	lowerIn := strings.ToLower(in)
	lowerFrom := strings.ToLower(from)
	n := len(from)

	var out strings.Builder
	var spans []span
	var result Result

	pos := 0
	for pos <= len(in)-n {
		idx := strings.Index(lowerIn[pos:], lowerFrom)
		if idx < 0 {
			break
		}
		i := pos + idx
		candidate := in[i : i+n]
		form, ok := caseForm(candidate, from)
		if !ok || !boundaryBefore(in, i, form) {
			out.WriteString(in[pos : i+1])
			pos = i + 1
			continue
		}

		out.WriteString(in[pos:i])

		line := lineText(in, i)
		if matchesAnySkip(line, skips) {
			out.WriteString(candidate)
			result.Skipped = append(result.Skipped, Occurrence{
				Line: lineNumber(in, i),
				Text: line,
			})
			pos = i + n
			continue
		}

		if !boundaryAfter(in, i, n) {
			out.WriteString(candidate)
			result.Ambiguous = append(result.Ambiguous, Occurrence{
				Line: lineNumber(in, i),
				Text: line,
			})
			pos = i + n
			continue
		}

		replacement := applyCase(form, to)
		start := out.Len()
		out.WriteString(replacement)
		spans = append(spans, span{start: start, end: start + len(replacement), orig: candidate})
		pos = i + n
	}
	out.WriteString(in[pos:])

	result.Out = out.String()
	if revertSpans(result.Out, spans) != in {
		result.Mismatch = true
		result.Out = in
	}
	return result, nil
}

// span records one substitution: the output offsets of its replacement, and the original text it displaced.
// The reversibility check uses it to undo the substitution.
type span struct {
	start, end int
	orig       string
}

// revertSpansImpl rebuilds the input from out by replacing each recorded output span with its orig text.
// It walks spans in ascending start order.
// The reversibility check depends on this.
// Reverting exactly the recorded offsets must reproduce the input byte-for-byte.
// The check still holds when to already occurs in the input.
// It compares the rebuilt bytes, not a count of occurrences.
func revertSpansImpl(out string, spans []span) string {
	var b strings.Builder
	pos := 0
	for _, sp := range spans {
		b.WriteString(out[pos:sp.start])
		b.WriteString(sp.orig)
		pos = sp.end
	}
	b.WriteString(out[pos:])
	return b.String()
}

// revertSpans is a package-level indirection over revertSpansImpl, so tests can replace it with a broken transform.
// That exercises the Result.Mismatch path.
var revertSpans = revertSpansImpl

// lineNumber returns the 1-based line number containing byte offset i in s, counted by "\n".
func lineNumber(s string, i int) int {
	return 1 + strings.Count(s[:i], "\n")
}

// lineText returns the full text of the line containing byte offset i in s, excluding the
// terminating newline.
func lineText(s string, i int) string {
	start := strings.LastIndexByte(s[:i], '\n') + 1
	end := strings.IndexByte(s[i:], '\n')
	if end < 0 {
		return s[start:]
	}
	return s[start : i+end]
}

// matchesAnySkip reports whether line matches any regexp in skips.
// A skip claims the occurrence whether it would otherwise be swapped or classified AMBIGUOUS.
// That is what makes a deliberate keep expressible.
func matchesAnySkip(line string, skips []*regexp.Regexp) bool {
	for _, re := range skips {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
