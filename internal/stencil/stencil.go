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

// Fill renders a markdown template by substituting {{.X}} marker fields from values
// and returns the rendered bytes. It never HTML-escapes output (it uses text/template,
// not html/template), so values containing markdown or code fences pass through verbatim.
//
// A leading `<!-- ... -->` comment is stripped before parsing (a documentation banner on
// the template asset); comments elsewhere in the template are left untouched as ordinary
// template syntax.
//
// Fill's one load-bearing guarantee: every top-level marker (a `{{.X}}` substitution that
// is a direct child of the template, not nested inside a conditional) that is absent from
// values or maps to an empty-or-whitespace-only string is collected — all of them, not
// just the first — and reported together in one sorted, deterministic error, and the
// template is never executed. A marker reached only inside a taken `{{if}}`/`{{with}}`/
// `{{range}}` branch is instead caught incrementally during execution (via
// `missingkey=error`): only an absent branch-internal key is an error; a present-but-empty
// branch-internal value renders as a silent blank. Because that empty check only applies at
// the top level, a caller-required marker (such as `fasit`/`target`) MUST be placed at the
// template's top level, never inside a conditional branch, or an empty value for it will
// pass through unnoticed.
func Fill(template []byte, values map[string]string) ([]byte, error) {
	// Strip a leading banner comment before parsing; mid-template comments are ordinary
	// template syntax and must reach the parser untouched.
	stripped := stripLeadingComment(string(template))

	// missingkey=error turns a branch-internal reference to an absent key into an
	// execution-time error instead of silently rendering "<no value>".
	t, err := tmpl.New("stencil").Option("missingkey=error").Parse(stripped)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	// Batch-check every top-level marker before executing anything: this is what lets us
	// report every unfilled top-level marker in one error instead of failing on the first.
	offenders := unfilledTopLevelMarkers(t, values)
	if len(offenders) > 0 {
		sort.Strings(offenders)
		return nil, fmt.Errorf("stencil: unfilled top-level marker(s): %s", strings.Join(offenders, ", "))
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		// A branch-internal reached-but-absent marker surfaces here, one at a time,
		// because missingkey=error halts execution at the first miss.
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// stripLeadingComment drops a leading `<!-- ... -->` block from text, returning
// everything after the closing `-->` with leading `\r`/`\n` trimmed. The block is
// dropped only when text, after trimming leading whitespace, begins with `<!--`; if
// there is no leading `<!--` or the comment is never closed, text is returned
// unchanged so a mid-template comment is never mistaken for the leading banner.
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

// unfilledTopLevelMarkers walks the parsed template's top-level (depth-0) nodes only —
// it does not descend into if/with/range bodies, since those are checked incrementally
// at execution time instead — and returns the deduplicated names of every bare `{{.X}}`
// substitution whose value in values is absent or empty-or-whitespace-only.
func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string) []string {
	// A comment-only or empty template parses to a tree with no root, or a root with
	// no nodes; either way there is nothing to check.
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
		// Only a single-command pipe whose sole argument is a bare field reference is a
		// plain `{{.X}}` substitution; anything else (pipelines, functions, multiple
		// commands) is not the marker idiom this guard targets.
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
		// An absent key reads as the zero value "" from a map[string]string, so this
		// single TrimSpace check covers both the absent-key and empty/whitespace cases.
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
