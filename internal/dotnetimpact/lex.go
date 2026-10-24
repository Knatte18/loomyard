// lex.go turns C# source into identifier and punctuation tokens, dropping comments, preprocessor lines, character literals and the literal text of every string.

package dotnetimpact

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// token is one identifier or one punctuation character of a C# source file.
type token struct {
	// text is the identifier without a verbatim `@` prefix, or the one punctuation character.
	text string
	// ident reports whether the token is an identifier, keywords included.
	ident bool
	// pos is the byte offset of the token in the source.
	pos int
}

// lexer holds the scan position over one source string.
type lexer struct {
	src    string
	pos    int
	tokens []token
}

// lex returns the identifier and punctuation tokens of data.
// Comments, `#` preprocessor lines, character literals and the literal text of regular, verbatim and raw strings are dropped.
// The code of every interpolation hole is kept as tokens, nested strings and braces included, without the hole's own braces.
func lex(data []byte) []token {
	l := &lexer{src: string(data)}
	l.code(false)
	return l.tokens
}

// code scans tokens until the end of the source, or, when inHole is set, until the brace that closes the hole, which it leaves unconsumed.
func (l *lexer) code(inHole bool) {
	depth := 0
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.pos++
		case c == '/' && l.peek(1) == '/':
			l.skipTo("\n")
		case c == '/' && l.peek(1) == '*':
			l.skipPast("*/")
		case c == '#' && l.atLineStart():
			l.skipTo("\n")
		case c == '\'':
			l.skipCharLiteral()
		case c == '"' || c == '$' || c == '@':
			l.stringOrOther()
		case c == '{':
			depth++
			l.emitPunct()
		case c == '}':
			if inHole && depth == 0 {
				return
			}
			depth--
			l.emitPunct()
		case c >= '0' && c <= '9':
			l.skipWord()
		default:
			r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
			if r == '_' || unicode.IsLetter(r) {
				start := l.pos
				l.skipWord()
				l.tokens = append(l.tokens, token{text: l.src[start:l.pos], ident: true, pos: start})
			} else {
				l.emitPunct()
			}
		}
	}
}

// peek returns the byte offset ahead of the scan position, or 0 past the end.
func (l *lexer) peek(offset int) byte {
	if l.pos+offset < len(l.src) {
		return l.src[l.pos+offset]
	}
	return 0
}

// atLineStart reports whether only blanks precede the scan position on its line.
func (l *lexer) atLineStart() bool {
	for i := l.pos - 1; i >= 0; i-- {
		switch l.src[i] {
		case '\n':
			return true
		case ' ', '\t', '\r':
		default:
			return false
		}
	}
	return true
}

// emitPunct emits the character at the scan position as a punctuation token.
func (l *lexer) emitPunct() {
	_, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.tokens = append(l.tokens, token{text: l.src[l.pos : l.pos+size], pos: l.pos})
	l.pos += size
}

// skipWord advances past a run of letters, digits and underscores.
func (l *lexer) skipWord() {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return
		}
		l.pos += size
	}
}

// skipTo advances to the next occurrence of marker, or the end of the source.
func (l *lexer) skipTo(marker string) {
	if index := strings.Index(l.src[l.pos:], marker); index >= 0 {
		l.pos += index
		return
	}
	l.pos = len(l.src)
}

// skipPast advances past the next occurrence of marker, or to the end of the source.
func (l *lexer) skipPast(marker string) {
	l.skipTo(marker)
	l.pos = min(l.pos+len(marker), len(l.src))
}

// skipCharLiteral advances past a character literal, honouring a backslash escape.
func (l *lexer) skipCharLiteral() {
	i := l.pos + 1
	for i < len(l.src) && l.src[i] != '\'' && l.src[i] != '\n' {
		if l.src[i] == '\\' {
			i++
		}
		i++
	}
	l.pos = min(i+1, len(l.src))
}

// stringOrOther handles a `"`, `$` or `@` at the scan position:
// a string literal of any form is consumed, a verbatim identifier is emitted without its `@`, and any other use is punctuation.
func (l *lexer) stringOrOther() {
	i := l.pos
	dollars, verbatim := 0, false
	for i < len(l.src) && (l.src[i] == '$' || l.src[i] == '@') {
		if l.src[i] == '$' {
			dollars++
		} else {
			verbatim = true
		}
		i++
	}
	if i < len(l.src) && l.src[i] == '"' {
		l.pos = i
		l.stringBody(dollars, verbatim)
		return
	}
	if l.src[l.pos] == '@' && i == l.pos+1 {
		r, _ := utf8.DecodeRuneInString(l.src[i:])
		if r == '_' || unicode.IsLetter(r) {
			l.pos = i
			start := l.pos
			l.skipWord()
			l.tokens = append(l.tokens, token{text: l.src[start:l.pos], ident: true, pos: start})
			return
		}
	}
	l.emitPunct()
}

// stringBody consumes the string whose opening quote is at the scan position.
// dollars is the count of `$` prefixes, which is the brace count that opens a hole, and verbatim reports an `@` prefix.
func (l *lexer) stringBody(dollars int, verbatim bool) {
	quotes := 0
	for l.pos+quotes < len(l.src) && l.src[l.pos+quotes] == '"' {
		quotes++
	}
	if quotes == 2 {
		l.pos += 2
		return
	}
	if quotes >= 3 && !verbatim {
		l.pos += quotes
		l.rawBody(quotes, dollars)
		return
	}
	l.pos++
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case c == '"' && verbatim && l.peek(1) == '"':
			l.pos += 2
		case c == '"':
			l.pos++
			return
		case c == '\\' && !verbatim:
			l.pos += 2
		case c == '{' && dollars > 0 && l.peek(1) == '{':
			l.pos += 2
		case c == '{' && dollars > 0:
			l.pos++
			l.hole(1)
		default:
			l.pos++
		}
	}
}

// rawBody consumes a raw string's text up to its closing run of quotes.
// dollars is the brace count that opens a hole in an interpolated raw string, 0 when it is not interpolated.
func (l *lexer) rawBody(quotes, dollars int) {
	closing := strings.Repeat("\"", quotes)
	for l.pos < len(l.src) {
		switch {
		case strings.HasPrefix(l.src[l.pos:], closing):
			l.pos += quotes
			return
		case l.src[l.pos] == '{' && dollars > 0:
			run := 0
			for l.pos+run < len(l.src) && l.src[l.pos+run] == '{' {
				run++
			}
			l.pos += run
			if run >= dollars {
				l.hole(dollars)
			}
		default:
			l.pos++
		}
	}
}

// hole scans the code of one interpolation hole, whose opening braces are consumed, and consumes the closing braces.
// closing is the brace count that ends the hole.
func (l *lexer) hole(closing int) {
	l.code(true)
	for ; closing > 0 && l.pos < len(l.src) && l.src[l.pos] == '}'; closing-- {
		l.pos++
	}
}
