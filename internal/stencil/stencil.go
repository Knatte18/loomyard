// stencil.go implements Fill, the leaf that substitutes marker fields in a markdown
// template with caller-supplied values. It refuses to render a template that would
// leave a required top-level marker unfilled, turning a silently-blank prompt field
// into a loud, early error instead.

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
// Every top-level marker absent from values or empty is collected and reported
// in one error; the template is never executed with unfilled top-level markers.
func Fill(template []byte, values map[string]string) ([]byte, error) {
	return FillOptional(template, values, nil)
}

// FillOptional renders a template like Fill, except names in optional are
// exempt from the unfilled-marker guarantee and render as nothing if absent or empty.
func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error) {
	stripped := stripLeadingComment(string(template))

	t, err := tmpl.New("stencil").Option("missingkey=error").Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	optionalNames := make(map[string]bool, len(optional))
	for _, name := range optional {
		optionalNames[name] = true
	}

	offenders := unfilledTopLevelMarkers(t, values, optionalNames)
	if len(offenders) > 0 {
		sort.Strings(offenders)
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

// unfilledTopLevelMarkers returns the deduplicated names of every top-level
// marker absent or empty in values, skipping names in optional.
func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string, optional map[string]bool) []string {
	if t.Tree == nil || t.Tree.Root == nil {
		return nil
	}

	var offenders []string
	seen := make(map[string]bool)
	for _, node := range t.Tree.Root.Nodes {
		actionNode, ok := node.(*parse.ActionNode)
		if !ok {
			continue
		}
		if actionNode.Pipe == nil || len(actionNode.Pipe.Cmds) != 1 {
			continue
		}
		cmd := actionNode.Pipe.Cmds[0]
		if len(cmd.Args) != 1 {
			continue
		}
		fieldNode, ok := cmd.Args[0].(*parse.FieldNode)
		if !ok || len(fieldNode.Ident) < 1 {
			continue
		}

		name := fieldNode.Ident[0]
		if optional[name] {
			continue
		}
		if strings.TrimSpace(values[name]) != "" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		offenders = append(offenders, name)
	}
	return offenders
}
