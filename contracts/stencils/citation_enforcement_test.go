// citation_enforcement_test.go guards stencil bodies against naming a loomyard-internal path:
// an agent working in another repository cannot open it.
// It scans each stencil's agent-facing body: the bytes StripLeadingComment produces.
// Fill and FillOptional apply the same stripping before a template executes.
// A reference that lives only in the stripped banner is never flagged:
// an agent never sees the banner, so it cannot act on the citation.
//
// The token rule has three conjunctive parts:
//  1. the token sits under one of three repository-relative prefixes: "contracts/", "docs/", "internal/";
//  2. the token ends in ".md" or ".go";
//     this keeps a bare package reference out of scope while a Go-file citation is still caught;
//  3. the token contains no "#" and is not "plan:"-prefixed, both checked on the full surrounding token:
//     a `plan:` handle's "plan:" sits left of the prefix, so a prefix-anchored match would silently drop it.
//
// An occurrence passes when it is "{{.specs_dir}}"-prefixed (the deployed-copy spelling) or named in citationAllowlist.
//
// TestStencils_NameNoConstraintsFile is a separate assertion:
// "CONSTRAINTS.md" sits at the repository root, under none of the three prefixes, so the prefix rule cannot match it.
// The file is retired, so any stencil naming it fails.

package stencils

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// citationPrefixes are the three repository-relative prefixes that make a token a cross-repository citation candidate.
var citationPrefixes = []string{"contracts/", "docs/", "internal/"}

// citationAllowKey returns the allowlist key for a token in one stencil, formatted as "stencil:token".
// An allowance covers only the stencil it names.
func citationAllowKey(stencilName, token string) string {
	return stencilName + ":" + token
}

// citationAllowlist lists tokens that the bare-citation check would flag but that are not citations.
// Each entry's Why field states why its token is not a citation.
//
// The loom-block-plan-overview entry names that stencil's glyph-grammar worked example:
// `internal/boardcli/list.go` illustrates the plain-file-path spelling rule for a reader of the stencil,
// and it is not a pointer the agent is meant to open.
var citationAllowlist = []scankit.Entry{
	{
		Key: "loom-block-plan-overview:internal/boardcli/list.go",
		Why: "glyph-grammar worked example, not a citation",
	},
	{
		Key: "loom-template-parts:internal/x/newfile.go",
		Why: "parts-file skeleton placeholder, not a citation",
	},
	{
		Key: "loom-template-parts:contracts/specs/x-spec.md",
		Why: "parts-file skeleton placeholder, not a citation",
	},
}

// citationCandidateTokens returns the runs of body that contain one of the repository-relative prefixes.
// Runs are split at whitespace and backticks, and a prefix matches anywhere in a run, not only at its start.
// That keeps the "plan:" in a `plan:` handle visible to the check that excludes it.
func citationCandidateTokens(body string) []string {
	var tokens []string
	for _, run := range strings.FieldsFunc(body, func(r rune) bool {
		return unicode.IsSpace(r) || r == '`'
	}) {
		for _, prefix := range citationPrefixes {
			if strings.Contains(run, prefix) {
				tokens = append(tokens, run)
				break
			}
		}
	}
	return tokens
}

// citationIsBare reports whether token is a bare cross-repository citation:
// it ends in ".md" or ".go", it contains no "#", and it is not "plan:"-prefixed.
// It does not check for a repository prefix, so the token must already contain one.
func citationIsBare(token string) bool {
	if !strings.HasSuffix(token, ".md") && !strings.HasSuffix(token, ".go") {
		return false
	}
	if strings.Contains(token, "#") {
		return false
	}
	if strings.HasPrefix(token, "plan:") {
		return false
	}
	return true
}

// TestStencils_NoBareCrossRepoCitations fails on each bare cross-repository citation in a stencil's agent-facing body.
// It also fails when a citationAllowlist entry matches no bare citation in that stencil's agent-facing body.
// Without that check, dead entries would accumulate and re-permit a bare citation that reappears with the same token.
func TestStencils_NoBareCrossRepoCitations(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	violations, allow := scanBareCitations(t)
	for _, v := range violations {
		t.Errorf("stencil %q carries the bare cross-repository citation %q; rewrite it to point at the deployed {{.specs_dir}} copy, or add a justified citationAllowlist entry if it is not actually a citation", v.stencil, v.token)
	}
	allow.RequireNoStale(t)
}

// bareCitation is one (stencil name, token) pair the bare-citation rule flags.
type bareCitation struct {
	stencil string
	token   string
}

// scanBareCitations returns the uncovered bare citations in every stencil's agent-facing body.
// It also returns the allowlist, which records the entries that matched, so the caller can find stale ones.
func scanBareCitations(t *testing.T) ([]bareCitation, *scankit.Allowlist) {
	t.Helper()

	allow := scankit.NewAllowlist(citationAllowlist)
	texts := shippedStencils(t)
	names := slices.Sorted(maps.Keys(texts))
	scankit.RequireFloor(t, len(names), 1, "stencil citation scan")

	var violations []bareCitation
	for _, name := range names {
		body := stencil.StripLeadingComment(string(texts[name]))

		for _, token := range citationCandidateTokens(body) {
			if !citationIsBare(token) {
				continue
			}
			if strings.HasPrefix(token, "{{.specs_dir}}") {
				continue
			}
			if allow.Allowed(citationAllowKey(name, token)) {
				continue
			}
			violations = append(violations, bareCitation{stencil: name, token: token})
		}
	}
	return violations, allow
}

// namesConstraintsFile reports whether a stencil's stripped body names the retired "CONSTRAINTS.md".
func namesConstraintsFile(body string) bool {
	return strings.Contains(body, "CONSTRAINTS.md")
}

// TestStencils_NameNoConstraintsFile fails when any stencil's stripped body names "CONSTRAINTS.md".
// The file is retired in favour of the PATTERN directive a prompt carries, so no stencil points at it, guarded or not.
//
//testtiming:keep pins the ban on a stencil naming the retired CONSTRAINTS.md, a guard that fires which the repository-relative citation rule cannot see
func TestStencils_NameNoConstraintsFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if !namesConstraintsFile("Read `CONSTRAINTS.md` at the repo root if present.") {
		t.Error("namesConstraintsFile(body naming CONSTRAINTS.md) = false; want true")
	}
	if namesConstraintsFile("Follow the PATTERN entries above and existing patterns.") {
		t.Error("namesConstraintsFile(body without CONSTRAINTS.md) = true; want false")
	}

	for name, body := range registeredBodies(t) {
		if namesConstraintsFile(body) {
			t.Errorf("stencil %q names CONSTRAINTS.md; the PATTERN directive replaces it", name)
		}
	}
}
