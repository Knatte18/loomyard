// source.go reads one C# source file into its identifier set and its type declarations, with the test attributes each carries.

package dotnetimpact

import (
	"slices"
	"strings"
)

// typeDecl is one type a C# source file declares.
type typeDecl struct {
	// Name is the type's name without its type parameters.
	Name string
	// Namespace is the enclosing namespaces joined by `.`, empty when there are none.
	Namespace string
	// Outer are the names of the enclosing types, outermost first.
	Outer []string
	// Kind is `class`, `struct`, `interface`, `record`, `record struct` or `enum`.
	Kind string
	// Abstract reports an `abstract` modifier.
	Abstract bool
	// Base is the first name of the base list, without generic arguments or namespace qualifier; empty when there is none, and for an enum.
	Base string
	// TestAttributed reports a `TestFixture` or `TestClass` attribute on the type.
	TestAttributed bool
	// HasTestMethod reports a member carrying a test-method attribute.
	HasTestMethod bool
}

// sourceFile is the read of one C# source file.
type sourceFile struct {
	// Rel is the file's slash path relative to the tier's `dir`.
	Rel string
	// Identifiers is the set of every identifier token in the file, keywords included.
	Identifiers map[string]bool
	// Types are the file's type declarations in source order.
	Types []typeDecl
}

var (
	typeAttributes   = []string{"TestFixture", "TestClass"}
	methodAttributes = []string{"Test", "TestCase", "TestCaseSource", "Theory", "Fact", "TestMethod", "DataTestMethod"}
	typeKeywords     = []string{"class", "struct", "interface", "enum", "record"}
)

type frameKind int

const (
	frameBlock frameKind = iota
	frameNamespace
	frameType
)

// frame is one open brace of the file: a namespace body, a type body or any other block.
type frame struct {
	kind frameKind
	name string
	// decl indexes sourceFile.Types for a type frame.
	decl int
}

// sourceParser walks the tokens of one file.
type sourceParser struct {
	tokens []token
	i      int
	file   sourceFile
	frames []frame
	// fileNamespace is the file-scoped `namespace X;`, which applies to the rest of the file.
	fileNamespace string
	// pending are the attribute names collected since the last member boundary.
	pending map[string]bool
	// attributeEnd is the token index just past the last attribute list, where a following list is still an attribute list.
	attributeEnd int
}

// parseSource reads the C# source data of the file rel into its identifier set and type declarations.
func parseSource(rel string, data []byte) sourceFile {
	p := &sourceParser{
		tokens:       lex(data),
		file:         sourceFile{Rel: rel, Identifiers: map[string]bool{}},
		pending:      map[string]bool{},
		attributeEnd: -1,
	}
	for _, t := range p.tokens {
		if t.ident {
			p.file.Identifiers[t.text] = true
		}
	}
	for p.i < len(p.tokens) {
		p.step()
	}
	return p.file
}

// step handles the token at the cursor and advances past what it consumed.
func (p *sourceParser) step() {
	t := p.tokens[p.i]
	switch {
	case p.isPunct(p.i, "{"):
		p.frames = append(p.frames, frame{kind: frameBlock})
		clear(p.pending)
		p.i++
	case p.isPunct(p.i, "}"):
		if len(p.frames) > 0 {
			p.frames = p.frames[:len(p.frames)-1]
		}
		clear(p.pending)
		p.i++
	case p.isPunct(p.i, ";"):
		clear(p.pending)
		p.i++
	case p.isPunct(p.i, "[") && p.startsAttributeList():
		p.attributeList()
	case t.ident && t.text == "namespace" && !p.after("."):
		p.namespaceDecl()
	case t.ident && slices.Contains(typeKeywords, t.text) && p.startsTypeDecl():
		p.typeDecl()
	default:
		p.i++
	}
}

// after reports whether the token before the cursor is the punctuation text.
func (p *sourceParser) after(text string) bool {
	return p.i > 0 && !p.tokens[p.i-1].ident && p.tokens[p.i-1].text == text
}

// startsAttributeList reports whether a `[` at the cursor opens an attribute list: it sits where a member starts.
func (p *sourceParser) startsAttributeList() bool {
	if p.i == 0 || p.i == p.attributeEnd {
		return true
	}
	previous := p.tokens[p.i-1]
	return !previous.ident && (previous.text == "{" || previous.text == "}" || previous.text == ";")
}

// attributeList reads the attribute list at the cursor.
// The names land in the pending set, and a test-method attribute marks the enclosing type.
// A list with a target prefix, such as `[assembly: X]`, sets nothing.
func (p *sourceParser) attributeList() {
	end := p.matching(p.i, "[", "]")
	p.attributeEnd = end + 1
	defer func() { p.i = end + 1 }()

	if p.isPunct(p.i+2, ":") && p.tokens[p.i+1].ident && !p.isPunct(p.i+3, ":") {
		return
	}
	var names []string
	for start := p.i + 1; start < end; {
		names = append(names, p.lastQualifiedName(start))
		start = p.nextComma(start, end) + 1
	}
	for _, name := range names {
		name = strings.TrimSuffix(name, "Attribute")
		p.pending[name] = true
		if slices.Contains(methodAttributes, name) {
			p.markTestMethod()
		}
	}
}

// lastQualifiedName returns the last segment of the possibly namespace-qualified name starting at start, generic arguments and call arguments excluded.
func (p *sourceParser) lastQualifiedName(start int) string {
	name := ""
	for i := start; i < len(p.tokens) && p.tokens[i].ident; {
		name = p.tokens[i].text
		switch {
		case p.isPunct(i+1, "."):
			i += 2
		case p.isPunct(i+1, ":") && p.isPunct(i+2, ":"):
			i += 3
		default:
			return name
		}
	}
	return name
}

// nextComma returns the index of the first comma at nesting depth zero between start and end, or end when there is none.
func (p *sourceParser) nextComma(start, end int) int {
	depth := 0
	for i := start; i < end; i++ {
		t := p.tokens[i]
		if t.ident {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case ",":
			if depth == 0 {
				return i
			}
		}
	}
	return end
}

// markTestMethod records a test-method attribute on the type whose body the cursor is in.
func (p *sourceParser) markTestMethod() {
	if len(p.frames) > 0 && p.frames[len(p.frames)-1].kind == frameType {
		p.file.Types[p.frames[len(p.frames)-1].decl].HasTestMethod = true
	}
}

// namespaceDecl reads a `namespace` declaration at the cursor.
func (p *sourceParser) namespaceDecl() {
	name, next := p.qualifiedName(p.i + 1)
	switch {
	case name == "":
		p.i++
	case p.isPunct(next, "{"):
		p.frames = append(p.frames, frame{kind: frameNamespace, name: name})
		p.i = next + 1
	case p.isPunct(next, ";"):
		p.fileNamespace = name
		p.i = next + 1
	default:
		p.i++
	}
}

// qualifiedName reads the dot-joined name starting at start and returns it with the index after it.
func (p *sourceParser) qualifiedName(start int) (string, int) {
	var segments []string
	i := start
	for i < len(p.tokens) && p.tokens[i].ident {
		segments = append(segments, p.tokens[i].text)
		if !p.isPunct(i+1, ".") {
			return strings.Join(segments, "."), i + 1
		}
		i += 2
	}
	return strings.Join(segments, "."), i
}

// startsTypeDecl reports whether the keyword at the cursor declares a type rather than constrains a type parameter or names a variable.
func (p *sourceParser) startsTypeDecl() bool {
	if p.after(":") || p.after(",") || p.after(".") {
		return false
	}
	name, next := p.declaredName()
	if name == "" {
		return false
	}
	if p.tokens[p.i].text != "record" {
		return true
	}
	// `record` is also an ordinary identifier, as in `foreach (var record in rs)` or `record is { X: 1 }`;
	// a record declaration's name is no operator keyword and is followed by one of these.
	switch name {
	case "in", "is", "as", "when":
		return false
	}
	for _, follower := range []string{"(", "<", ":", "{", ";"} {
		if p.isPunct(next, follower) {
			return true
		}
	}
	return false
}

// declaredName returns the declared type's name and the index of the token after it, for the type keyword at the cursor.
// It returns an empty name when no identifier follows.
func (p *sourceParser) declaredName() (string, int) {
	i := p.i + 1
	if p.tokens[p.i].text == "record" && i < len(p.tokens) && p.tokens[i].ident && (p.tokens[i].text == "struct" || p.tokens[i].text == "class") {
		i++
	}
	if i < len(p.tokens) && p.tokens[i].ident {
		return p.tokens[i].text, i + 1
	}
	return "", i
}

// typeDecl reads the type declaration whose keyword is at the cursor, then moves into its body.
func (p *sourceParser) typeDecl() {
	keyword := p.tokens[p.i].text
	kind := keyword
	if keyword == "record" && p.tokens[p.i+1].text == "struct" {
		kind = "record struct"
	}
	name, next := p.declaredName()
	decl := typeDecl{
		Name:           name,
		Namespace:      p.namespace(),
		Outer:          p.outerTypes(),
		Kind:           kind,
		Abstract:       p.hasModifier("abstract"),
		TestAttributed: p.hasPending(typeAttributes),
	}
	next = p.skipTypeParameters(next)
	if p.isPunct(next, "(") {
		next = p.matching(next, "(", ")") + 1
	}
	if p.isPunct(next, ":") && kind != "enum" {
		decl.Base = p.lastQualifiedName(next + 1)
	}
	for next < len(p.tokens) && !p.isPunct(next, "{") && !p.isPunct(next, ";") {
		next++
	}
	p.file.Types = append(p.file.Types, decl)
	clear(p.pending)
	if p.isPunct(next, "{") {
		p.frames = append(p.frames, frame{kind: frameType, name: name, decl: len(p.file.Types) - 1})
	}
	p.i = next + 1
}

// skipTypeParameters returns the index after the `<...>` at start, or start when there is none.
func (p *sourceParser) skipTypeParameters(start int) int {
	if !p.isPunct(start, "<") {
		return start
	}
	return p.matching(start, "<", ">") + 1
}

// hasModifier reports whether the identifiers directly before the cursor include modifier.
func (p *sourceParser) hasModifier(modifier string) bool {
	for i := p.i - 1; i >= 0 && p.tokens[i].ident; i-- {
		if p.tokens[i].text == modifier {
			return true
		}
	}
	return false
}

// hasPending reports whether any of names is a pending attribute.
func (p *sourceParser) hasPending(names []string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return p.pending[name] })
}

// namespace returns the file's current namespace: the file-scoped one followed by every open namespace block.
func (p *sourceParser) namespace() string {
	var segments []string
	if p.fileNamespace != "" {
		segments = append(segments, p.fileNamespace)
	}
	for _, f := range p.frames {
		if f.kind == frameNamespace {
			segments = append(segments, f.name)
		}
	}
	return strings.Join(segments, ".")
}

// outerTypes returns the names of the open type frames, outermost first.
func (p *sourceParser) outerTypes() []string {
	var names []string
	for _, f := range p.frames {
		if f.kind == frameType {
			names = append(names, f.name)
		}
	}
	return names
}

// isPunct reports whether the token at index is the punctuation text.
func (p *sourceParser) isPunct(index int, text string) bool {
	return index >= 0 && index < len(p.tokens) && !p.tokens[index].ident && p.tokens[index].text == text
}

// matching returns the index of the token closing the open at index, or the last token index when it never closes.
func (p *sourceParser) matching(index int, open, closing string) int {
	depth := 0
	for i := index; i < len(p.tokens); i++ {
		if p.tokens[i].ident {
			continue
		}
		switch p.tokens[i].text {
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(p.tokens) - 1
}
