// csharphead.go reads a C# declaration head: where its declared name sits, and which components of a handle's owner chain its explicit-interface qualifier spells.

package planglyph

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// csharpKeywords are the reserved words of C#, none of which is an identifier in a declaration head.
var csharpKeywords = map[string]bool{
	"abstract": true, "as": true, "base": true, "bool": true, "break": true, "byte": true, "case": true,
	"catch": true, "char": true, "checked": true, "class": true, "const": true, "continue": true,
	"decimal": true, "default": true, "delegate": true, "do": true, "double": true, "else": true,
	"enum": true, "event": true, "explicit": true, "extern": true, "false": true, "finally": true,
	"fixed": true, "float": true, "for": true, "foreach": true, "goto": true, "if": true,
	"implicit": true, "in": true, "int": true, "interface": true, "internal": true, "is": true,
	"lock": true, "long": true, "namespace": true, "new": true, "null": true, "object": true,
	"operator": true, "out": true, "override": true, "params": true, "private": true, "protected": true,
	"public": true, "readonly": true, "ref": true, "return": true, "sbyte": true, "sealed": true,
	"short": true, "sizeof": true, "stackalloc": true, "static": true, "string": true, "struct": true,
	"switch": true, "this": true, "throw": true, "true": true, "try": true, "typeof": true,
	"uint": true, "ulong": true, "unchecked": true, "unsafe": true, "ushort": true, "using": true,
	"virtual": true, "void": true, "volatile": true, "while": true,
}

// headToken is one word or punctuation token of a declaration head, with its byte offset.
type headToken struct {
	text  string
	start int
	word  bool
}

// tokenizeHead splits a declaration head into word and punctuation tokens, dropping whitespace and every attribute list.
// An attribute list is a bracketed span at the head's start or after whitespace, so the brackets of `int[]` and `this[int index]` stay tokens.
// `=>` is one token, so its `>` never closes an angle bracket.
func tokenizeHead(head string) []headToken {
	var tokens []headToken
	for i := 0; i < len(head); {
		r, size := utf8.DecodeRuneInString(head[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
		case r == '[' && (i == 0 || unicode.IsSpace(rune(head[i-1]))):
			i = endOfAttributeList(head, i)
		case isHeadWordRune(r):
			end := i
			for end < len(head) {
				next, nextSize := utf8.DecodeRuneInString(head[end:])
				if !isHeadWordRune(next) {
					break
				}
				end += nextSize
			}
			tokens = append(tokens, headToken{text: head[i:end], start: i, word: true})
			i = end
		case r == '=' && strings.HasPrefix(head[i:], "=>"):
			tokens = append(tokens, headToken{text: "=>", start: i})
			i += 2
		default:
			tokens = append(tokens, headToken{text: head[i : i+size], start: i})
			i += size
		}
	}
	return tokens
}

// endOfAttributeList returns the index just past the bracket that closes the one opening at head[open], or len(head) when none does.
func endOfAttributeList(head string, open int) int {
	depth := 0
	for i := open; i < len(head); i++ {
		switch head[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(head)
}

// isHeadWordRune reports whether r can be part of an identifier or keyword.
func isHeadWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// nestingDelta returns how much tok opens (+1) or closes (-1) a bracket pair, and 0 for every other token.
func nestingDelta(tok headToken) int {
	switch tok.text {
	case "(", "<", "[":
		return 1
	case ")", ">", "]":
		return -1
	}
	return 0
}

// holdsDepthZeroWord reports whether tokens hold word outside every bracket pair.
func holdsDepthZeroWord(tokens []headToken, word string) bool {
	depth := 0
	for _, tok := range tokens {
		if depth == 0 && tok.word && tok.text == word {
			return true
		}
		depth = max(depth+nestingDelta(tok), 0)
	}
	return false
}

// declaredNameIndex returns the byte index of the declared name token in head.
// With a non-empty name it is the first occurrence of name, without its generic arity suffix, as a whole identifier outside attribute lists, followed by `(`, `<`, `{`, `=`, `;`, `:`, the word `where` or the end of the head.
// With an empty name, or when that name is not found, it is the declarator position: the last identifier outside every bracket pair and attribute list, before the first `{`, `=`, `=>`, `;`, `:`, `,`, the word `where`, the end of the head, or a `(` opening a parameter list.
// It reports false when no identifier qualifies, when the identifier follows `~` or a bare `static`, and when the head holds `operator` or `this` outside every bracket pair.
func declaredNameIndex(head, name string) (int, bool) {
	tokens := tokenizeHead(head)
	if holdsDepthZeroWord(tokens, "operator") || holdsDepthZeroWord(tokens, "this") {
		return 0, false
	}

	index := -1
	if name != "" {
		bare, _, _ := strings.Cut(name, "<")
		index = namedTokenIndex(tokens, bare)
	}
	if index < 0 {
		index = declaratorTokenIndex(tokens)
	}
	if index < 0 {
		return 0, false
	}
	if index > 0 {
		previous := tokens[index-1]
		if previous.text == "~" || (previous.word && previous.text == "static") {
			return 0, false
		}
	}
	return tokens[index].start, true
}

// namedTokenIndex returns the index of the first word token equal to name that a name-ending token follows, or -1.
func namedTokenIndex(tokens []headToken, name string) int {
	for i, tok := range tokens {
		if !tok.word || tok.text != name {
			continue
		}
		if i+1 == len(tokens) {
			return i
		}
		switch tokens[i+1].text {
		case "(", "<", "{", "=", ";", ":", "where":
			return i
		}
	}
	return -1
}

// declaratorTokenIndex returns the index of the last identifier outside every bracket pair before the head's first stop token, or -1.
func declaratorTokenIndex(tokens []headToken) int {
	last := -1
	depth := 0
	for i, tok := range tokens {
		if depth == 0 {
			if tok.word {
				if tok.text == "where" {
					break
				}
				if !csharpKeywords[tok.text] {
					last = i
				}
				continue
			}
			if opensParameterList(tokens, i) {
				break
			}
			switch tok.text {
			case "{", "=", "=>", ";", ":", ",":
				return last
			}
		}
		depth = max(depth+nestingDelta(tok), 0)
	}
	return last
}

// opensParameterList reports whether tokens[i] is the `(` of a parameter list: one that follows an identifier or a closing angle bracket, never one that opens a tuple type after a keyword.
func opensParameterList(tokens []headToken, i int) bool {
	if tokens[i].text != "(" || i == 0 {
		return false
	}
	previous := tokens[i-1]
	if previous.word {
		return !csharpKeywords[previous.text]
	}
	return previous.text == ">"
}

// indexerThisIndex returns the byte index of an outermost `this` followed by `[`, the declared-name position of an indexer.
func indexerThisIndex(head string) (int, bool) {
	tokens := tokenizeHead(head)
	depth := 0
	for i, tok := range tokens {
		if depth == 0 && tok.word && tok.text == "this" && i+1 < len(tokens) && tokens[i+1].text == "[" {
			return tok.start, true
		}
		depth = max(depth+nestingDelta(tok), 0)
	}
	return 0, false
}

// csharpOwner returns the glyph's owner chain joined with `.`, less the member's explicit-interface qualifier.
// The qualifier is the longest trailing run of owner components the head writes, dotted and followed by a dot, immediately before the declared name.
// A head that writes no such qualifier, or has no declared-name position, leaves the chain whole.
func csharpOwner(g glyph.Glyph, head string) string {
	run, _ := explicitQualifier(g, head)
	return strings.Join(g.Owner[:len(g.Owner)-run], ".")
}

// explicitQualifier returns how many trailing owner components of g head's explicit-interface qualifier spells, and that qualifier as the head writes it, without its trailing dot.
// Components are compared with the head's text in the canonical C# spelling, so `IPair<string, int>` matches the component `IPair<string,int>`.
// It returns zero when the head writes none.
func explicitQualifier(g glyph.Glyph, head string) (int, string) {
	nameStart, ok := declaredNameIndex(head, g.Name)
	if !ok {
		nameStart, ok = indexerThisIndex(head)
	}
	if !ok || !strings.HasSuffix(head[:nameStart], ".") {
		return 0, ""
	}
	written := strings.TrimSuffix(head[:nameStart], ".")

	for run := len(g.Owner); run >= 1; run-- {
		want := strings.Join(g.Owner[len(g.Owner)-run:], ".")
		for start := 0; start < len(written); start++ {
			if !qualifierMayStartAt(written, start) {
				continue
			}
			if glyph.CanonicalCSharpType(written[start:]) == want {
				return run, written[start:]
			}
		}
	}
	return 0, ""
}

// qualifierMayStartAt reports whether a qualifier can begin at text[start]: on a non-space byte that does not continue an identifier or a dotted name.
func qualifierMayStartAt(text string, start int) bool {
	if unicode.IsSpace(rune(text[start])) {
		return false
	}
	if start == 0 {
		return true
	}
	previous, _ := utf8.DecodeLastRuneInString(text[:start])
	return previous != '.' && !isHeadWordRune(previous)
}

// renameCSharpDecl derives the declaration head of a C# symbol renamed to identifier, and reports false when the symbol's declared name cannot be told from its signature.
// A field, const or event gets a one-declarator head, since a signature quarry gives every declarator of one declaration would declare several symbols once its name is replaced.
// The head is then the signature cut before the first declarator's name, followed by identifier, with ` = default` appended for a const.
// Every other kind has the declared name token replaced in place.
// That token is kept only when it is the symbol's own name without its generic arity suffix, so an operator, an indexer, a finalizer and a constructor, which no identifier names, report false.
func renameCSharpDecl(sym quarry.Symbol, identifier string) (string, bool) {
	switch sym.Kind {
	case quarry.KindField, quarry.KindConst, quarry.KindEvent:
		tokens := tokenizeHead(sym.Signature)
		index := declaratorTokenIndex(tokens)
		if index < 0 {
			return "", false
		}
		head := sym.Signature[:tokens[index].start] + identifier
		if sym.Kind == quarry.KindConst {
			head += " = default"
		}
		return head, true
	}

	bare, _, _ := strings.Cut(sym.Glyph.Name, "<")
	if isCSharpConstructor(sym.Glyph) {
		return "", false
	}
	start, ok := declaredNameIndex(sym.Signature, sym.Glyph.Name)
	if !ok {
		return "", false
	}
	end := start
	for end < len(sym.Signature) {
		next, size := utf8.DecodeRuneInString(sym.Signature[end:])
		if !isHeadWordRune(next) {
			break
		}
		end += size
	}
	if sym.Signature[start:end] != bare {
		return "", false
	}
	return sym.Signature[:start] + identifier + sym.Signature[end:], true
}

// isCSharpConstructor reports whether g names a constructor, whose declared name is its owner's type name.
// C# forbids any other member from bearing the name of its enclosing type.
func isCSharpConstructor(g glyph.Glyph) bool {
	if len(g.Owner) == 0 {
		return false
	}
	ownerBare, _, _ := strings.Cut(g.Owner[len(g.Owner)-1], "<")
	nameBare, _, _ := strings.Cut(g.Name, "<")
	return ownerBare == nameBare
}

// canonicalQualifierWay returns the way forward for a C# Create handle quarry refused as an invalid container because its head spells the explicit-interface qualifier in a non-canonical spelling, or "" when the head does not.
// It reads the handle's own owner chain and the head, and quotes the qualifier in canonical spelling.
func canonicalQualifierWay(plan *planparser.Plan, handle, head string) string {
	body, _ := planparser.HandleBody(handle)
	g, err := planparser.ParseRef(plan, body)
	if err != nil {
		return ""
	}
	return canonicalQualifierWayOf(g, head)
}

// canonicalQualifierWayOf returns the way forward for a C# head under the glyph g whose explicit-interface qualifier is written in a non-canonical spelling, or "" when it is not.
// It quotes the qualifier in canonical spelling.
func canonicalQualifierWayOf(g glyph.Glyph, head string) string {
	run, written := explicitQualifier(g, head)
	if run == 0 {
		return ""
	}
	canonical := glyph.CanonicalCSharpType(written)
	if canonical == written {
		return ""
	}
	return fmt.Sprintf("; write the qualifier in the head in canonical spelling, `%s.`", canonical)
}
