// stencil.go implements Fill, the leaf that substitutes marker fields in a markdown template with
// caller-supplied values.
// It refuses to render a template that would leave a required top-level marker unfilled, turning a
// silently-blank prompt field into a loud, early error instead.

package stencil

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	tmpl "text/template"
	"text/template/parse"
)

// Fill renders a markdown template by substituting {{.X}} markers from values.
// Every top-level marker absent from values or empty is collected and reported in one error;
// the template is never executed with unfilled top-level markers.
func Fill(template []byte, values map[string]string) ([]byte, error) {
	return FillOptional(template, values, nil)
}

// FillOptional renders a template like Fill, except names in optional are exempt from the
// unfilled-marker guarantee and render as nothing if absent or empty.
func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error) {
	return FillWith(template, nil, values, optional)
}

// FillWith renders a template like FillOptional, together with the named included templates its bare {{template "name"}} actions pull in.
// Each include renders from the same values as the main template, even inside a {{with}} or {{range}} body.
// An include action carrying a pipeline of its own, an action naming a block absent from includes, and an included block that itself declares an include are refused, each naming the block;
// includes resolve one level deep.
// The unfilled-marker guarantee covers the top-level markers of the main template and of every included block.
func FillWith(template []byte, includes map[string][]byte, values map[string]string, optional []string) ([]byte, error) {
	stripped := stripLeadingComment(string(template))

	t, err := tmpl.New("stencil").Option("missingkey=error").Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	attached, err := attachIncludes(t, includes)
	if err != nil {
		return nil, err
	}

	optionalNames := make(map[string]bool, len(optional))
	for _, name := range optional {
		optionalNames[name] = true
	}

	offenders := unfilledTopLevelMarkers(t, values, optionalNames)
	if t.Tree != nil {
		offenders = append(offenders, presentButEmptyBranchMarkers(t.Tree.Root, values, optionalNames)...)
	}
	for _, included := range attached {
		offenders = append(offenders, unfilledTopLevelMarkers(included, values, optionalNames)...)
	}
	if len(offenders) > 0 {
		offenders = dedupSorted(offenders)
		return nil, fmt.Errorf("stencil: unfilled top-level marker(s): %s", strings.Join(offenders, ", "))
	}

	execValues := values
	if len(optionalNames) > 0 {
		execValues = make(map[string]string, len(values))
		for k, v := range values {
			execValues[k] = v
		}
		for name := range optionalNames {
			if strings.TrimSpace(execValues[name]) == "" {
				execValues[name] = ""
			}
		}
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, execValues); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// attachIncludes parses every include that t's include actions name into t's template set under its name, and points each bare include action at the root variable $ so the block renders from the values Execute is given.
// It returns the attached include templates, in name order.
func attachIncludes(t *tmpl.Template, includes map[string][]byte) ([]*tmpl.Template, error) {
	if t.Tree == nil {
		return nil, nil
	}
	actions := includeActions(t.Tree.Root)
	rootPipe, err := rootVariablePipe()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, action := range actions {
		if action.Pipe != nil {
			return nil, fmt.Errorf("stencil: include %q carries a pipeline of its own; write it as the bare {{template %q}}", action.Name, action.Name)
		}
		names = append(names, action.Name)
	}
	names = dedupSorted(names)

	attached := make([]*tmpl.Template, 0, len(names))
	for _, name := range names {
		body, ok := includes[name]
		if !ok {
			return nil, fmt.Errorf("stencil: include %q has no entry in the includes", name)
		}
		included, err := t.New(name).Parse(stripLeadingComment(string(body)))
		if err != nil {
			return nil, fmt.Errorf("parse include %q: %w", name, err)
		}
		if included.Tree != nil && len(includeNames(included.Tree.Root)) > 0 {
			return nil, fmt.Errorf("stencil: include %q declares an include of its own; includes resolve one level deep", name)
		}
		attached = append(attached, included)
	}
	for _, action := range actions {
		action.Pipe = rootPipe
	}
	return attached, nil
}

// rootVariablePipe returns the parsed pipeline of {{$}}, which evaluates to the data a template is executed with.
func rootVariablePipe() (*parse.PipeNode, error) {
	t, err := tmpl.New("root").Parse("{{$}}")
	if err != nil {
		return nil, fmt.Errorf("parse root pipeline: %w", err)
	}
	return t.Tree.Root.Nodes[0].(*parse.ActionNode).Pipe, nil
}

// includeActions returns every {{template "name"}} action under root, walking {{if}}, {{with}} and {{range}} bodies and their else branches.
func includeActions(root *parse.ListNode) []*parse.TemplateNode {
	if root == nil {
		return nil
	}
	var actions []*parse.TemplateNode
	var walk func(list *parse.ListNode)
	walk = func(list *parse.ListNode) {
		if list == nil {
			return
		}
		for _, node := range list.Nodes {
			switch n := node.(type) {
			case *parse.TemplateNode:
				actions = append(actions, n)
			case *parse.IfNode:
				walk(n.List)
				walk(n.ElseList)
			case *parse.WithNode:
				walk(n.List)
				walk(n.ElseList)
			case *parse.RangeNode:
				walk(n.List)
				walk(n.ElseList)
			}
		}
	}
	walk(root)
	return actions
}

// includeNames returns the sorted, deduplicated names of the include actions under root.
func includeNames(root *parse.ListNode) []string {
	var names []string
	for _, action := range includeActions(root) {
		names = append(names, action.Name)
	}
	return dedupSorted(names)
}

// IncludeNames parses template and returns the sorted, deduplicated names of the {{template "name"}} actions it declares,
// so a caller knows which blocks to read for FillWith.
// A leading comment is stripped first, so a header spelling an include action reports none.
func IncludeNames(template []byte) ([]string, error) {
	stripped := stripLeadingComment(string(template))

	t, err := tmpl.New("stencil").Option("missingkey=error").Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if t.Tree == nil {
		return nil, nil
	}
	return includeNames(t.Tree.Root), nil
}

// dedupSorted returns names deduplicated and sorted -- used to merge unfilledTopLevelMarkers' and
// presentButEmptyBranchMarkers' offender lists into one stable report, since the same name can
// appear in both (e.g. a top-level {{.X}} and a branch-internal {{.X}} referencing the same marker).
func dedupSorted(names []string) []string {
	sort.Strings(names)
	deduped := names[:0]
	var prev string
	for i, name := range names {
		if i > 0 && name == prev {
			continue
		}
		deduped = append(deduped, name)
		prev = name
	}
	return deduped
}

// stripLeadingComment drops a leading `<!-- ... -->` block from text.
// Returns text unchanged if no leading block is found.
func stripLeadingComment(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(trimmed, "<!--") {
		return text
	}
	closeIdx := strings.Index(trimmed, "-->")
	if closeIdx == -1 {
		return text
	}
	rest := trimmed[closeIdx+len("-->"):]
	return strings.TrimLeft(rest, "\r\n")
}

// StripLeadingComment drops a leading `<!-- ... -->` block from text and returns text unchanged
// when there is none.
// It is the same stripper Fill and FillOptional apply to a template before parsing it.
func StripLeadingComment(text string) string {
	return stripLeadingComment(text)
}

// TopLevelMarkers parses template and returns the deduplicated, sorted names of every top-level
// {{.X}} marker it declares.
// It is the marker-set accessor `lyx stencil validate` uses to compare two templates,
// not a validity check: a marker is listed here regardless of whether any value would fill it.
func TopLevelMarkers(template []byte) ([]string, error) {
	stripped := stripLeadingComment(string(template))

	t, err := tmpl.New("stencil").Option("missingkey=error").Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	names := topLevelMarkerNames(t)
	sort.Strings(names)
	return names, nil
}

// topLevelMarkerNames returns the deduplicated names of every top-level {{.X}} marker declared in
// t's parsed tree, with no filtering by value or optionality.
func topLevelMarkerNames(t *tmpl.Template) []string {
	if t.Tree == nil || t.Tree.Root == nil {
		return nil
	}

	var names []string
	seen := make(map[string]bool)
	for _, node := range t.Tree.Root.Nodes {
		actionNode, ok := node.(*parse.ActionNode)
		if !ok {
			continue
		}
		name, ok := actionFieldName(actionNode)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// actionFieldName reports the single field name a bare {{.X}} ActionNode substitutes, or false for
// anything else (a pipeline with functions/args, more than one command).
// Shared by topLevelMarkerNames and presentButEmptyBranchMarkers so the two walks agree on what
// counts as a plain marker.
func actionFieldName(n *parse.ActionNode) (string, bool) {
	if n.Pipe == nil || len(n.Pipe.Cmds) != 1 {
		return "", false
	}
	cmd := n.Pipe.Cmds[0]
	if len(cmd.Args) != 1 {
		return "", false
	}
	fieldNode, ok := cmd.Args[0].(*parse.FieldNode)
	if !ok || len(fieldNode.Ident) < 1 {
		return "", false
	}
	return fieldNode.Ident[0], true
}

// unfilledTopLevelMarkers returns the deduplicated names of every top-level
// marker absent or empty in values, skipping names in optional.
func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string, optional map[string]bool) []string {
	var offenders []string
	for _, name := range topLevelMarkerNames(t) {
		if optional[name] {
			continue
		}
		if strings.TrimSpace(values[name]) != "" {
			continue
		}
		offenders = append(offenders, name)
	}
	return offenders
}

// presentButEmptyBranchMarkers returns branch-internal marker names, deduplicated, that are present
// in values as an empty or whitespace-only string within an {{if}} branch confidently evaluated
// true, skipping names in optional.
// This is the one class of unfilled marker neither existing guard catches: an absent branch-internal
// marker still errors at execution via missingkey=error (left entirely alone by this function -- see
// the confidentlyTrue doc comment), but a key present as "" renders silently, which is exactly what
// every stencil asset's own header comment warns "{{if}}/{{range}} conditionals" risk ("a required
// marker inside a conditional branch would render silently blank when present-but-empty").
// Only {{if}}: neither {{range}} nor {{with}} nor {{template}} is walked, so a marker reachable only
// through one of those remains outside this guarantee exactly as it was before this function existed.
func presentButEmptyBranchMarkers(root *parse.ListNode, values map[string]string, optional map[string]bool) []string {
	if root == nil {
		return nil
	}
	var offenders []string
	seen := make(map[string]bool)
	var walk func(nodes []parse.Node)
	walk = func(nodes []parse.Node) {
		for _, node := range nodes {
			switch n := node.(type) {
			case *parse.ActionNode:
				name, ok := actionFieldName(n)
				if !ok || optional[name] || seen[name] {
					continue
				}
				value, present := values[name]
				if !present || strings.TrimSpace(value) != "" {
					continue
				}
				seen[name] = true
				offenders = append(offenders, name)
			case *parse.IfNode:
				if confidentlyTrue(n.Pipe, values) {
					walk(n.List.Nodes)
				}
				// A false or unresolvable condition is left entirely to the existing lazy missingkey=error path at execution time (see the forgotten_discriminator and branch_internal_absent rows of TestFill).
				// This function only adds coverage for a branch confidently known to run.
			}
		}
	}
	walk(root.Nodes)
	return offenders
}

// confidentlyTrue reports whether a simple {{if .X}} or {{if eq .X "literal"}} condition evaluates
// true given values.
// An absent condition key, or any condition shape other than these two, is never reported true --
// presentButEmptyBranchMarkers's only job is to add coverage for a branch confidently known to
// execute; anything uncertain is left to the existing execution-time missingkey=error path exactly
// as it behaved before this function existed, never newly flagged and never newly suppressed.
func confidentlyTrue(pipe *parse.PipeNode, values map[string]string) bool {
	if pipe == nil || len(pipe.Cmds) != 1 {
		return false
	}
	cmd := pipe.Cmds[0]
	switch len(cmd.Args) {
	case 1:
		// {{if .X}}
		field, ok := cmd.Args[0].(*parse.FieldNode)
		if !ok || len(field.Ident) < 1 {
			return false
		}
		value, present := values[field.Ident[0]]
		return present && strings.TrimSpace(value) != ""
	case 3:
		// {{if eq .X "literal"}}
		ident, ok := cmd.Args[0].(*parse.IdentifierNode)
		if !ok || ident.Ident != "eq" {
			return false
		}
		field, ok := cmd.Args[1].(*parse.FieldNode)
		if !ok || len(field.Ident) < 1 {
			return false
		}
		lit, ok := cmd.Args[2].(*parse.StringNode)
		if !ok {
			return false
		}
		value, present := values[field.Ident[0]]
		return present && value == lit.Text
	default:
		return false
	}
}
