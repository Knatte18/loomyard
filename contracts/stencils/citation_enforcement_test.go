// citation_enforcement_test.go is the permanent guard behind the class of bug this task closes: a
// stencil body naming a loomyard-internal path an agent working in some other repository cannot
// open. It scans each stencil's agent-facing body -- the bytes StripLeadingComment produces, the
// same stripping Fill and FillOptional apply before a template is ever executed, so a banner-only
// reference (this package's own doc comments about where a file lives on the loomyard side) is
// never flagged: an agent never sees the banner, so it is not a citation an agent could act on.
//
// The token rule has three conjunctive parts, each earning its keep against real stencil content:
//  1. the token sits under one of four repository-relative prefixes: "contracts/", "manifest/",
//     "docs/", "internal/";
//  2. the token ends in ".md" or ".go" -- this is what keeps a bare package reference (the
//     internal/planglyph mention at the end of the implementer body) out of scope while still
//     catching a Go-file citation;
//  3. the token contains no "#" and is not "plan:"-prefixed -- applied to the FULL surrounding
//     token, not to a regex match that begins at the prefix, because a `plan:`-handle's "plan:"
//     sits to the LEFT of where a prefix-anchored match would start (see the Card-model handle
//     grammar in loom-template-plan.md), and matching only from the prefix onward would silently
//     discard the very evidence part 3 exists to see.
//
// An occurrence passes when it is "{{.specs_dir}}"-prefixed (the deployed-copy spelling every
// rewrite in this task lands on) or when it is named on citationAllowlist below.
//
// Scenario coverage: a rewritten normative citation (an {{.specs_dir}}-prefixed token) passes; a
// bare re-added one (a loomyard-relative path with no {{.specs_dir}} prefix and no allowlist entry)
// fails; an allowlisted entry passes; an allowlist entry whose token has vanished from its
// stencil's stripped body fails as stale (TestStencils_AllowlistHasNoStaleEntries).
//
// TestStencils_ConstraintsCitationsAreGuarded is a separate, narrower assertion, not a fourth part
// of the token rule above: "CONSTRAINTS.md" is a repository-ROOT token, under none of the four
// prefixes, so the prefix rule structurally cannot see it -- and it must not simply be folded into
// the prefix set, because CONSTRAINTS.md is a legitimate target-repo path a stencil is supposed to
// name (a target repository may or may not have one), unlike the loomyard-internal paths the token
// rule polices. The defect the companion assertion closes is a missing "if present" guard on that
// instruction, not the bare token itself.

package stencils

import (
	"strings"
	"testing"
	"unicode"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// citationPrefixes are the four repository-relative prefixes a bare cross-repository citation
// token must sit under to be in scope for TestStencils_NoBareCrossRepoCitations.
var citationPrefixes = []string{"contracts/", "manifest/", "docs/", "internal/"}

// citationAllowKey identifies one (stencil name, token) pair TestStencils_NoBareCrossRepoCitations
// is told to pass despite matching the bare-citation token rule.
type citationAllowKey struct {
	Stencil string
	Token   string
}

// citationAllowlist is the justification-carrying allowlist for tokens the bare-citation rule would
// otherwise flag but that are not citations at all. Each value is the entry's own justification,
// carried as data rather than as a comment beside the entry, so the justification cannot drift away
// from what it justifies -- the same (file, target)-keyed, owner-naming shape
// internal/lyxcwd's Markdown Link Integrity allowlist already uses.
//
// One entry, deliberately: a small allowlist is the design here, not a workaround. The entry names
// loom-template-plan's own glyph-grammar worked example -- `internal/boardcli/list.go` illustrates
// the plain-file-path spelling rule for a reader of the stencil, it is not a pointer the agent is
// meant to open.
var citationAllowlist = map[citationAllowKey]string{
	{Stencil: "loom-template-plan", Token: "internal/boardcli/list.go"}: "glyph-grammar worked example, not a citation",
}

// citationCandidateTokens returns every maximal non-whitespace, non-backtick run in body that
// contains one of citationPrefixes as a substring anywhere in the run -- not merely at its start --
// which is what keeps a `plan:`-prefixed token's full spelling in view for the exclusion rule in
// citationIsBare to test, rather than only the "internal/..." suffix a prefix-anchored match would
// see.
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

// citationIsBare reports whether token is a bare cross-repository citation under the token rule's
// three conjunctive parts: it ends in ".md" or ".go", it contains no "#", and it is not
// "plan:"-prefixed. citationCandidateTokens has already established that token sits under one of
// the four repository-relative prefixes, so this function checks only parts 2 and 3.
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

// TestStencils_NoBareCrossRepoCitations fails on every bare cross-repository citation left in any
// stencil's agent-facing body -- see the file comment for the full token rule and its rationale.
func TestStencils_NoBareCrossRepoCitations(t *testing.T) {
	reg := Registry()
	for _, name := range reg.Names() {
		def, ok := reg.Default(name)
		if !ok {
			t.Fatalf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
		}
		body := stencil.StripLeadingComment(string(def))

		for _, token := range citationCandidateTokens(body) {
			if !citationIsBare(token) {
				continue
			}
			if strings.HasPrefix(token, "{{.specs_dir}}") {
				continue
			}
			if _, allowed := citationAllowlist[citationAllowKey{Stencil: name, Token: token}]; allowed {
				continue
			}
			t.Errorf("stencil %q carries the bare cross-repository citation %q; rewrite it to point at the deployed {{.specs_dir}} copy, or add a justified citationAllowlist entry if it is not actually a citation", name, token)
		}
	}
}

// TestStencils_AllowlistHasNoStaleEntries fails when a citationAllowlist entry's token no longer
// appears in its named stencil's stripped body -- without this check the allowlist would silently
// accumulate dead rows that would re-permit a reintroduced bare citation carrying the same token.
func TestStencils_AllowlistHasNoStaleEntries(t *testing.T) {
	reg := Registry()
	for key := range citationAllowlist {
		def, ok := reg.Default(key.Stencil)
		if !ok {
			t.Errorf("citationAllowlist entry names unknown stencil %q", key.Stencil)
			continue
		}
		body := stencil.StripLeadingComment(string(def))
		if !strings.Contains(body, key.Token) {
			t.Errorf("citationAllowlist entry (%q, %q) is stale: the token no longer appears in the stencil's stripped body; delete the entry", key.Stencil, key.Token)
		}
	}
}

// TestStencils_ConstraintsCitationsAreGuarded fails when a stencil's stripped body mentions
// "CONSTRAINTS.md" without the phrase "if present" in the same sentence. See the file comment for
// why this is a separate assertion rather than a fourth part of the bare-citation token rule.
func TestStencils_ConstraintsCitationsAreGuarded(t *testing.T) {
	// constraintsPlaceholder stands in for the literal "CONSTRAINTS.md" while splitting a stencil
	// body into sentences, so the period inside "CONSTRAINTS.md" itself is never mistaken for a
	// sentence boundary.
	const constraintsPlaceholder = "CONSTRAINTS\x00md"

	reg := Registry()
	for _, name := range reg.Names() {
		def, ok := reg.Default(name)
		if !ok {
			t.Fatalf("Registry().Default(%q) = _, false; want true for a name Registry().Names() returned", name)
		}
		body := stencil.StripLeadingComment(string(def))
		if !strings.Contains(body, "CONSTRAINTS.md") {
			continue
		}

		guarded := strings.ReplaceAll(body, "CONSTRAINTS.md", constraintsPlaceholder)
		for _, sentence := range strings.FieldsFunc(guarded, func(r rune) bool { return r == '.' || r == '\n' }) {
			if strings.Contains(sentence, constraintsPlaceholder) && !strings.Contains(sentence, "if present") {
				t.Errorf("stencil %q mentions CONSTRAINTS.md without \"if present\" guarding it in the same sentence: %q", name, sentence)
			}
		}
	}
}
