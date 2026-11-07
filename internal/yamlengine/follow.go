// follow.go carries a template change into an operator-owned file, unit by unit.
// Each unit is judged against the template the file last followed, so a value the operator set is kept.

package yamlengine

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FollowReport lists, each sorted, what a follow changed in a file.
// It also lists the operator's values it kept against a template that holds them otherwise.
type FollowReport struct {
	// Added lists the dotted path of each template unit the file gained.
	Added []string
	// Updated lists, as "<path>: <old> -> <new>", each unit the file held at the base's value and now holds at the template's.
	Updated []string
	// Removed lists the dotted path of each unit the file held at the base's value and the template dropped.
	Removed []string
	// Differs lists each unit the operator set that the template holds otherwise.
	// Each is shown as "<path>: file <value>, template <value>", and the file keeps it.
	// A value absent from one side renders as "absent".
	Differs []string
}

// Changed reports whether the merge rewrote the file.
func (r FollowReport) Changed() bool {
	return len(r.Added)+len(r.Updated)+len(r.Removed) > 0
}

// ErrInvalidBase marks an error caused by a base document that does not parse or whose top level is not a mapping.
// It tells such an error apart from one caused by the template or the file.
var ErrInvalidBase = errors.New("the template base is not a YAML mapping")

// FollowTemplate carries template into existing, an operator-owned file seeded from an earlier template.
// It keeps every value the operator set.
// base is the template the file last followed.
// haveBase false says the file has no record of one, so base is not read.
// A unit is a mapping key, descended into while the file, template and base each hold a mapping or nothing there.
// Otherwise a unit is compared whole, so a scalar or a sequence is always one unit.
// With a base, a unit the file holds as the base does, absence included, takes the template's value.
// Such a unit is gained, changed or lost accordingly.
// Any other unit is the operator's and is kept, listed under Differs when the template holds a third value.
// Without a base, a unit the file lacks is added from the template.
// A unit the file holds with a value the template holds otherwise is kept and listed under Differs.
// A unit only the file holds is kept silently.
// A mapping a removal empties is removed with it,
// and a unit copied in from the template carries none of its comments.
// An unchanged file comes back byte for byte.
// A changed file is re-encoded: it keeps comments and each node's style, and a long flow mapping stays on one line.
// The blank lines between entries are dropped.
// A document whose top level is not a mapping is an error naming the document,
// and an error caused by the base wraps ErrInvalidBase.
func FollowTemplate(base, template, existing []byte, haveBase bool) ([]byte, FollowReport, error) {
	var baseRoot *yaml.Node
	if haveBase {
		var err error
		_, baseRoot, err = parseMappingRoot(base, "base")
		if err != nil {
			return nil, FollowReport{}, fmt.Errorf("%w: %w", ErrInvalidBase, err)
		}
	}
	_, templateRoot, err := parseMappingRoot(template, "template")
	if err != nil {
		return nil, FollowReport{}, err
	}
	doc, existingRoot, err := parseMappingRoot(existing, "file")
	if err != nil {
		return nil, FollowReport{}, err
	}
	if existingRoot == nil {
		existingRoot = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if doc.Kind != yaml.DocumentNode {
			doc = &yaml.Node{Kind: yaml.DocumentNode}
		}
		doc.Content = append(doc.Content, existingRoot)
	}

	f := follower{haveBase: haveBase}
	f.followMapping("", baseRoot, templateRoot, existingRoot)
	for _, list := range []*[]string{&f.report.Added, &f.report.Updated, &f.report.Removed, &f.report.Differs} {
		sort.Strings(*list)
	}
	if !f.report.Changed() {
		return existing, f.report, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, FollowReport{}, fmt.Errorf("encode the followed YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, FollowReport{}, fmt.Errorf("encode the followed YAML: %w", err)
	}
	return out.Bytes(), f.report, nil
}

// follower walks one follow of a file and gathers its report.
type follower struct {
	haveBase bool
	report   FollowReport
}

// follow returns the value the file holds at path after the merge, nil to leave the key out.
func (f *follower) follow(path string, base, template, existing *yaml.Node) *yaml.Node {
	if isMapping(existing) && isMapping(template) && (base == nil || isMapping(base)) {
		hadEntries := len(existing.Content) > 0
		f.followMapping(path, base, template, existing)
		if hadEntries && len(existing.Content) == 0 {
			return nil
		}
		return existing
	}
	return f.followUnit(path, base, template, existing)
}

// followMapping merges the entries of the template and base mappings at path into existing in place.
// The file's keys come first, then the keys only the template holds, in its order.
// A nil base or template mapping holds no entry.
func (f *follower) followMapping(path string, base, template, existing *yaml.Node) {
	keys := mappingKeys(existing)
	for _, mapping := range []*yaml.Node{template, base} {
		for _, key := range mappingKeys(mapping) {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	for _, key := range keys {
		_, existingValue := mappingEntry(existing, key)
		_, baseValue := mappingEntry(base, key)
		templateKey, templateValue := mappingEntry(template, key)
		result := f.follow(joinKeyPath(path, key), baseValue, templateValue, existingValue)
		switch {
		case result == existingValue:
		case existingValue == nil:
			existing.Content = append(existing.Content, copyWithoutComments(templateKey), result)
		case result == nil:
			at := mappingIndex(existing, key)
			existing.Content = slices.Delete(existing.Content, at, at+2)
		default:
			existing.Content[mappingIndex(existing, key)+1] = result
		}
	}
}

// followUnit decides one unit compared whole and returns the value the file holds after the merge, nil for absent.
func (f *follower) followUnit(path string, base, template, existing *yaml.Node) *yaml.Node {
	if !f.haveBase {
		switch {
		case existing == nil && template != nil:
			f.report.Added = append(f.report.Added, path)
			return copyWithoutComments(template)
		case existing != nil && template != nil && !nodesEqual(existing, template):
			f.differs(path, existing, template)
		}
		return existing
	}
	if !nodesEqual(existing, base) {
		if !nodesEqual(template, base) && !nodesEqual(template, existing) {
			f.differs(path, existing, template)
		}
		return existing
	}
	switch {
	case nodesEqual(existing, template):
		return existing
	case template == nil:
		f.report.Removed = append(f.report.Removed, path)
		return nil
	case existing == nil:
		f.report.Added = append(f.report.Added, path)
		return copyWithoutComments(template)
	default:
		f.report.Updated = append(f.report.Updated, fmt.Sprintf("%s: %s -> %s", path, inlineValue(existing), inlineValue(template)))
		updated := copyWithoutComments(template)
		updated.HeadComment, updated.LineComment, updated.FootComment = existing.HeadComment, existing.LineComment, existing.FootComment
		return updated
	}
}

// differs records a unit the operator set that the template holds otherwise.
func (f *follower) differs(path string, existing, template *yaml.Node) {
	f.report.Differs = append(f.report.Differs, fmt.Sprintf("%s: file %s, template %s", path, inlineValue(existing), inlineValue(template)))
}

// isMapping reports whether node is a mapping.
func isMapping(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.MappingNode
}

// mappingKeys returns the keys of mapping in order, none for a nil mapping.
func mappingKeys(mapping *yaml.Node) []string {
	if mapping == nil {
		return nil
	}
	keys := make([]string, 0, len(mapping.Content)/2)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i].Value)
	}
	return keys
}

// mappingEntry returns the key and value nodes of key in mapping, both nil when mapping is nil or lacks key.
func mappingEntry(mapping *yaml.Node, key string) (keyNode, value *yaml.Node) {
	if mapping == nil {
		return nil, nil
	}
	at := mappingIndex(mapping, key)
	if at < 0 {
		return nil, nil
	}
	return mapping.Content[at], mapping.Content[at+1]
}

// copyWithoutComments returns a deep copy of node with every comment cleared and every style kept.
func copyWithoutComments(node *yaml.Node) *yaml.Node {
	clone := deepCopyNode(node)
	clearComments(clone)
	return clone
}

// clearComments clears the comments of node and every node under it.
func clearComments(node *yaml.Node) {
	node.HeadComment, node.LineComment, node.FootComment = "", "", ""
	for _, child := range node.Content {
		clearComments(child)
	}
}

// inlineValue renders node as one line of flow YAML for a report, "absent" for nil.
func inlineValue(node *yaml.Node) string {
	if node == nil {
		return "absent"
	}
	flow := copyWithoutComments(node)
	setFlowStyle(flow)
	rendered, err := yaml.Marshal(flow)
	if err != nil {
		return fmt.Sprintf("<unrenderable %s>", nodeShape(node))
	}
	return strings.TrimSpace(string(rendered))
}

// setFlowStyle sets every mapping and sequence at or under node to flow style.
func setFlowStyle(node *yaml.Node) {
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style = yaml.FlowStyle
	}
	for _, child := range node.Content {
		setFlowStyle(child)
	}
}
