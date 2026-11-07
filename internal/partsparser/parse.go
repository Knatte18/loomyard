// parse.go declares the parts model and Parse, which reads the parts file's markdown into it.

package partsparser

import (
	"fmt"
	"slices"
	"strings"
)

// Parts is a parsed parts file: its title, its split note, its parts in file order and its shared spots.
type Parts struct {
	Title     string
	SplitNote string
	Parts     []Part
	Spots     []SharedSpot
}

// Part is one part of the split: the contract its writer works to.
// Exports are the refs the part's cards create for other parts to use.
// Area is the packages and files the part's cards touch.
type Part struct {
	Name      string
	Decisions []string
	TakesIn   string
	Delivers  string
	Exports   []string
	DependsOn []string
	Area      []string
}

// SharedSpot is a file several parts need a change in, which the chair's cards write rather than any part's.
// Exports are the refs the spot's change provides to the parts that need it.
// Uses are the refs of parts it relies on.
type SharedSpot struct {
	Path      string
	Decisions []string
	NeededBy  []string
	Needs     string
	Exports   []string
	Uses      []string
}

// Part returns the part named name, and whether the file holds one.
func (p *Parts) Part(name string) (Part, bool) {
	for _, part := range p.Parts {
		if part.Name == name {
			return part, true
		}
	}
	return Part{}, false
}

// External returns the refs another part's cards may use from the part named name.
// The refs come from each part the named part depends on, then from each shared spot whose `Needed by` lists it.
// Each ref appears once.
func (p *Parts) External(name string) []string {
	var refs []string
	add := func(exports []string) {
		for _, ref := range exports {
			if !slices.Contains(refs, ref) {
				refs = append(refs, ref)
			}
		}
	}
	if part, ok := p.Part(name); ok {
		for _, dependency := range part.DependsOn {
			if upstream, ok := p.Part(dependency); ok {
				add(upstream.Exports)
			}
		}
	}
	for _, spot := range p.Spots {
		if slices.Contains(spot.NeededBy, name) {
			add(spot.Exports)
		}
	}
	return refs
}

// SpotPaths returns every shared spot's path.
func (p *Parts) SpotPaths() []string {
	paths := make([]string, len(p.Spots))
	for i, spot := range p.Spots {
		paths[i] = spot.Path
	}
	return paths
}

const (
	h1Prefix        = "# Parts:"
	splitNoteHeader = "## Split note"
	partPrefix      = "## Part:"
	spotPrefix      = "## Shared spot:"
	specPointer     = "see contracts/specs/loom-parts-spec.md for the format"
)

// fieldKind is how a field's value is written: prose, a comma-separated list, or sub-bullets of one backticked ref each.
type fieldKind int

const (
	prose fieldKind = iota
	commaList
	refList
)

// fieldRule is one legal field label of a section kind.
type fieldRule struct {
	label    string
	kind     fieldKind
	required bool
}

var partFields = []fieldRule{
	{"Decisions", commaList, true},
	{"Takes in", prose, true},
	{"Delivers", prose, true},
	{"Exports", refList, false},
	{"Depends on", commaList, false},
	{"Area", refList, true},
}

var spotFields = []fieldRule{
	{"Decisions", commaList, false},
	{"Needed by", commaList, true},
	{"Needs", prose, true},
	{"Exports", refList, false},
	{"Uses", refList, false},
}

// rawLine is one body line of a section with its 1-based line number in the file.
type rawLine struct {
	number int
	text   string
	fenced bool
}

// section is one H2 section of the file: its heading, the heading's line number and the lines under it.
type section struct {
	heading string
	number  int
	body    []rawLine
}

// rawField is a field as written: its value text, or its refs for a refList.
type rawField struct {
	rule  fieldRule
	line  int
	value string
	refs  []string
}

// lineError builds the error for a wrong parts file line, naming the line, the problem and the way forward.
func lineError(line int, problem, wayForward string) error {
	return fmt.Errorf("parts file line %d: %s; way forward: %s; %s", line, problem, wayForward, specPointer)
}

// Parse reads a parts file's markdown into a Parts, checking it against the parts format.
// A file that breaks the format fails with an error that names the line, the fault and the way forward.
// The fault may be a missing H1 or split note, an unknown label, or a required field left empty.
func Parse(data []byte) (*Parts, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	first := 0
	for first < len(lines) && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	if first == len(lines) || !strings.HasPrefix(lines[first], h1Prefix) {
		return nil, lineError(first+1, "the file does not open with `# Parts: <task title>`", "start the file with the H1 `# Parts: <task title>`")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[first], h1Prefix))
	if title == "" {
		return nil, lineError(first+1, "the H1 has no task title", "write the task title after `# Parts:`")
	}

	sections, err := splitSections(lines, first+1)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 || sections[0].heading != splitNoteHeader {
		return nil, lineError(firstSectionLine(sections, len(lines)), "the first section is not `## Split note`", "write `## Split note` right after the H1, saying how the task splits or why it does not")
	}
	note := strings.TrimSpace(joinText(sections[0].body))
	if note == "" {
		return nil, lineError(sections[0].number, "the split note is empty", "say how the task splits, or why it does not")
	}

	parts := &Parts{Title: title, SplitNote: note}
	for _, sec := range sections[1:] {
		switch {
		case strings.HasPrefix(sec.heading, partPrefix):
			if len(parts.Spots) > 0 {
				return nil, lineError(sec.number, "a `## Part:` section follows a shared spot", "move every `## Part:` section above the first `## Shared spot:` section")
			}
			part, err := buildPart(sec)
			if err != nil {
				return nil, err
			}
			parts.Parts = append(parts.Parts, part)
		case strings.HasPrefix(sec.heading, spotPrefix):
			spot, err := buildSpot(sec)
			if err != nil {
				return nil, err
			}
			parts.Spots = append(parts.Spots, spot)
		default:
			return nil, lineError(sec.number, fmt.Sprintf("unknown or misplaced section heading %q", sec.heading), "use `## Part: <name>` or ## Shared spot: `<path>`, after the one `## Split note`")
		}
	}
	return parts, nil
}

// firstSectionLine returns the line of the first section, or the file's end when it has none.
func firstSectionLine(sections []section, fileLines int) int {
	if len(sections) == 0 {
		return fileLines
	}
	return sections[0].number
}

// splitSections cuts the lines from index start into H2 sections.
// A heading line outside a fenced block that opens with `#` but is not an H2 is refused, as is text before the first H2.
func splitSections(lines []string, start int) ([]section, error) {
	var sections []section
	fenced := false
	for i := start; i < len(lines); i++ {
		text := strings.TrimRight(lines[i], " \t")
		if strings.HasPrefix(strings.TrimLeft(text, " \t"), "```") {
			fenced = !fenced
			if len(sections) > 0 {
				sections[len(sections)-1].body = append(sections[len(sections)-1].body, rawLine{i + 1, text, true})
			}
			continue
		}
		if fenced {
			if len(sections) > 0 {
				sections[len(sections)-1].body = append(sections[len(sections)-1].body, rawLine{i + 1, text, true})
			}
			continue
		}
		switch {
		case strings.HasPrefix(text, "## "):
			sections = append(sections, section{heading: text, number: i + 1})
		case strings.HasPrefix(text, "#"):
			return nil, lineError(i+1, fmt.Sprintf("unknown heading %q", text), "use only the H1, `## Split note`, `## Part: <name>` and ## Shared spot: `<path>` headings")
		case len(sections) == 0:
			if text != "" {
				return nil, lineError(i+1, "text before the first section", "move it under `## Split note`")
			}
		default:
			sections[len(sections)-1].body = append(sections[len(sections)-1].body, rawLine{i + 1, text, false})
		}
	}
	return sections, nil
}

// joinText joins a body's lines, blank and fenced lines included, with newlines.
func joinText(body []rawLine) string {
	texts := make([]string, len(body))
	for i, line := range body {
		texts[i] = line.text
	}
	return strings.Join(texts, "\n")
}

// parseFields reads a section's body as fields under the given rules;
// it checks the required ones are present and non-empty.
func parseFields(sec section, rules []fieldRule) (map[string]*rawField, error) {
	labels := make([]string, len(rules))
	for i, rule := range rules {
		labels[i] = rule.label
	}
	fields := map[string]*rawField{}
	var current *rawField
	var proseLines []string
	flushProse := func() {
		if current != nil && current.rule.kind == prose {
			current.value = strings.TrimSpace(current.value + "\n" + strings.Join(proseLines, "\n"))
		}
		proseLines = nil
	}
	for _, line := range sec.body {
		trimmed := strings.TrimSpace(line.text)
		if line.fenced || trimmed == "" {
			if current != nil && current.rule.kind == prose {
				proseLines = append(proseLines, line.text)
			}
			continue
		}
		if strings.HasPrefix(line.text, "- ") {
			flushProse()
			label, value, ok := strings.Cut(strings.TrimPrefix(line.text, "- "), ":")
			label = strings.TrimSpace(label)
			if !ok {
				return nil, lineError(line.number, fmt.Sprintf("%q is not a field `- <Label>: <value>`", line.text), fmt.Sprintf("write the line as `- <Label>: <value>` with a label of %s", strings.Join(labels, ", ")))
			}
			idx := slices.IndexFunc(rules, func(r fieldRule) bool { return r.label == label })
			if idx < 0 {
				return nil, lineError(line.number, fmt.Sprintf("unknown field label %q", label), fmt.Sprintf("use one of %s", strings.Join(labels, ", ")))
			}
			if earlier, repeated := fields[label]; repeated {
				return nil, lineError(line.number, fmt.Sprintf("field %q is repeated; it is first written on line %d", label, earlier.line), "keep the field once and merge the values")
			}
			current = &rawField{rule: rules[idx], line: line.number, value: strings.TrimSpace(value)}
			fields[label] = current
			if current.rule.kind == refList && current.value != "" {
				return nil, lineError(line.number, fmt.Sprintf("field %q holds sub-bullets, not an inline value", label), fmt.Sprintf("leave the rest of the line empty and write one indented sub-bullet of one backticked ref per entry under %q", label))
			}
			continue
		}
		if !strings.HasPrefix(line.text, " ") && !strings.HasPrefix(line.text, "\t") {
			return nil, lineError(line.number, fmt.Sprintf("%q is not a field `- <Label>: <value>`", line.text), "start the line with `- <Label>: ` or indent it under the field it continues")
		}
		if current == nil {
			return nil, lineError(line.number, "an indented line before any field", "put it under a `- <Label>: <value>` field")
		}
		switch current.rule.kind {
		case prose:
			proseLines = append(proseLines, trimmed)
		case commaList:
			current.value += " " + trimmed
		case refList:
			entry, isBullet := strings.CutPrefix(trimmed, "- ")
			ref, isRef := strings.CutPrefix(strings.TrimSpace(entry), "`")
			ref, closed := strings.CutSuffix(ref, "`")
			if !isBullet || !isRef || !closed || ref == "" || strings.Contains(ref, "`") {
				return nil, lineError(line.number, fmt.Sprintf("%q is not one backticked ref sub-bullet", trimmed), fmt.Sprintf("write each entry of %q as an indented bullet holding one ref in backticks, such as `- `internal/x`", current.rule.label))
			}
			current.refs = append(current.refs, ref)
		}
	}
	flushProse()

	for _, rule := range rules {
		field, present := fields[rule.label]
		switch {
		case !present && rule.required:
			return nil, lineError(sec.number, fmt.Sprintf("%s lacks the required field %q", sec.heading, rule.label), fmt.Sprintf("add `- %s: ...` to the section", rule.label))
		case present && field.isEmpty():
			return nil, lineError(field.line, fmt.Sprintf("field %q is empty", rule.label), "give it a value, or leave an optional field out instead of writing it empty or as none")
		}
	}
	return fields, nil
}

// isEmpty reports whether the field holds no value and no ref.
func (f *rawField) isEmpty() bool {
	if f.rule.kind == refList {
		return len(f.refs) == 0
	}
	return f.value == ""
}

// list splits a comma-list field's value into its entries; an absent field gives none.
func (f *rawField) list() []string {
	if f == nil {
		return nil
	}
	var entries []string
	for _, entry := range strings.Split(f.value, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

// text returns a prose field's value; an absent field gives "".
func (f *rawField) text() string {
	if f == nil {
		return ""
	}
	return f.value
}

// refsOf returns a refList field's refs; an absent field gives none.
func (f *rawField) refsOf() []string {
	if f == nil {
		return nil
	}
	return f.refs
}

// buildPart builds a Part from its `## Part: <name>` section.
func buildPart(sec section) (Part, error) {
	name := strings.TrimSpace(strings.TrimPrefix(sec.heading, partPrefix))
	if name == "" {
		return Part{}, lineError(sec.number, "the part heading has no name", "write `## Part: <name>`")
	}
	fields, err := parseFields(sec, partFields)
	if err != nil {
		return Part{}, err
	}
	return Part{
		Name:      name,
		Decisions: fields["Decisions"].list(),
		TakesIn:   fields["Takes in"].text(),
		Delivers:  fields["Delivers"].text(),
		Exports:   fields["Exports"].refsOf(),
		DependsOn: fields["Depends on"].list(),
		Area:      fields["Area"].refsOf(),
	}, nil
}

// buildSpot builds a SharedSpot from its ## Shared spot: `<path>` section.
func buildSpot(sec section) (SharedSpot, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(sec.heading, spotPrefix))
	path, isPath := strings.CutPrefix(rest, "`")
	path, closed := strings.CutSuffix(path, "`")
	if !isPath || !closed || path == "" || strings.Contains(path, "`") {
		return SharedSpot{}, lineError(sec.number, fmt.Sprintf("%q does not name the spot's path in backticks", sec.heading), "write ## Shared spot: `<path>`")
	}
	fields, err := parseFields(sec, spotFields)
	if err != nil {
		return SharedSpot{}, err
	}
	return SharedSpot{
		Path:      path,
		Decisions: fields["Decisions"].list(),
		NeededBy:  fields["Needed by"].list(),
		Needs:     fields["Needs"].text(),
		Exports:   fields["Exports"].refsOf(),
		Uses:      fields["Uses"].refsOf(),
	}, nil
}
