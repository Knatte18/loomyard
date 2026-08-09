// swap.go implements the matching, case classification, and substitution core that
// tools/wordswap/main.go drives one file at a time: token-boundary detection, case-form
// recognition, and the single left-to-right scan that produces a Result.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Occurrence records one place in a file where a candidate match for `from` was found but not
// substituted -- either because it was classified AMBIGUOUS or because a -skip regexp claimed it.
type Occurrence struct {
	Line int
	Text string
}

// Result is the outcome of running swapText over one file's contents: the (possibly unchanged)
// output text, the two report buckets, and whether the reversibility safety check failed.
type Result struct {
	Out       string
	Ambiguous []Occurrence
	Skipped   []Occurrence
	Mismatch  bool
}

// caseForm reports which of the three recognized case forms -- "lower", "title", or "upper" --
// a candidate slice takes when compared against from under strings.EqualFold.
// It returns false for any mixed form (such as "hOst"), which is not a match at all.
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

// titleCase upper-cases the first byte of from and lower-cases the rest, producing the
// camelCase/PascalCase start form (e.g. "host" -> "Host").
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

// isIdentByte reports whether b is an ASCII letter or digit -- the character class that
// continues a token for boundary purposes.
func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// boundaryBefore reports whether position i in s marks a valid token start for a match: true at
// the very start of the string, when the preceding byte is not an identifier byte, or when the
// matched form itself is "title" or "upper" -- the camelCase/SCREAMING_CASE start case that is
// valid even with a lowercase letter immediately before it.
// This is what rejects "ghost", "localhost", and "conhost": a lowercase letter immediately
// before a lowercase-form match means no match at all.
func boundaryBefore(s string, i int, form string) bool {
	if i == 0 {
		return true
	}
	if !isIdentByte(s[i-1]) {
		return true
	}
	return form == "title" || form == "upper"
}

// boundaryAfter reports whether position i+n in s marks a valid token end for a match: true at
// the end of the string, or when the following byte is an ASCII uppercase letter, a digit, an
// underscore, or any non-identifier character.
// False when the following byte is an ASCII lowercase letter -- that is the AMBIGUOUS case.
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

// swapText scans in once, left to right, for case-insensitive occurrences of from, and returns
// the substitution outcome as a Result.
// Each occurrence is classified in turn: a mixed case form or an invalid boundary-before rejects
// the occurrence entirely (no match, no report); a line matching a skip regexp is left unchanged
// and reported in Result.Skipped; an invalid boundary-after is left unchanged and reported in
// Result.Ambiguous; otherwise the occurrence is substituted with the matched case form applied to
// to, and the substitution span is recorded for the reversibility check.
// Substitution is single-pass over the input -- the output is never re-scanned, so a to produced
// by this run can never be re-matched.
// swapText returns an error only for a malformed argument (empty from or to); a failed
// reversibility check is reported through Result.Mismatch, not an error.
func swapText(in, from, to string, skips []*regexp.Regexp) (Result, error) {
	if from == "" || to == "" {
		return Result{}, fmt.Errorf("swapText: from and to must both be non-empty")
	}

	lowerIn := strings.ToLower(in)
	lowerFrom := strings.ToLower(from)
	n := len(from)

	var out strings.Builder
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

		if matchesAnySkip(lineText(in, i), skips) {
			out.WriteString(candidate)
			result.Skipped = append(result.Skipped, Occurrence{
				Line: lineNumber(in, i),
				Text: candidate,
			})
			pos = i + n
			continue
		}

		if !boundaryAfter(in, i, n) {
			out.WriteString(candidate)
			result.Ambiguous = append(result.Ambiguous, Occurrence{
				Line: lineNumber(in, i),
				Text: candidate,
			})
			pos = i + n
			continue
		}

		replacement := applyCase(form, to)
		out.WriteString(replacement)
		pos = i + n
	}
	out.WriteString(in[pos:])

	result.Out = out.String()
	return result, nil
}

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
// A skip claims the occurrence regardless of whether it would otherwise have been swapped or
// classified AMBIGUOUS -- this is what makes a deliberate keep expressible.
func matchesAnySkip(line string, skips []*regexp.Regexp) bool {
	for _, re := range skips {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
