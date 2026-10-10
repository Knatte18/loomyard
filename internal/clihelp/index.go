// index.go renders the command index: a nested markdown list of one line per command, filtered by the audience annotation, read from a live cobra tree.

package clihelp

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Invocable reports whether cmd takes an audience: a leaf, or a runnable group whose Use carries a token after its name or that has a visible flag of its own.
// A group's persistent flags never qualify it, since they configure its children.
func Invocable(cmd *cobra.Command) bool {
	if !cmd.HasSubCommands() {
		return true
	}
	if !cmd.Runnable() {
		return false
	}
	return len(strings.Fields(cmd.Use)) > 1 || len(visibleLocalFlags(cmd)) > 0
}

// RenderIndex renders the nested markdown list of every command under root whose audience annotation equals audience.
// It indents two spaces per level and implies `lyx` plus the parent path by nesting.
// A group prints in group form when only a descendant prints.
// A runnable group prints in leaf form when its own annotation matches.
// A command without an annotation never prints.
// It walks root in memory and reads no file.
func RenderIndex(root *cobra.Command, audience string) string {
	var builder strings.Builder
	for _, line := range indexLines(root, audience, -1) {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	return builder.String()
}

// indexLines returns the index lines for cmd and everything below it; the root, at depth -1, has no line of its own.
func indexLines(cmd *cobra.Command, audience string, depth int) []string {
	var childLines []string
	for _, child := range cmd.Commands() {
		if child.Name() == "completion" {
			continue
		}
		childLines = append(childLines, indexLines(child, audience, depth+1)...)
	}
	if depth < 0 {
		return childLines
	}
	indent := strings.Repeat("  ", depth)
	switch {
	case Invocable(cmd) && cmd.Annotations[AudienceAnnotation] == audience:
		return append([]string{indent + leafLine(cmd)}, childLines...)
	case len(childLines) > 0:
		return append([]string{indent + groupLine(cmd)}, childLines...)
	}
	return nil
}

// groupLine formats cmd as the name in backticks, a colon and its Short.
func groupLine(cmd *cobra.Command) string {
	return withNote(cmd, "- `"+cmd.Name()+"`: "+cmd.Short)
}

// leafLine formats cmd as its Use in backticks, then the flags Use does not spell, a colon and its Short.
func leafLine(cmd *cobra.Command) string {
	line := "- `" + cmd.Use + "`"
	var flagNames []string
	for _, flag := range visibleLocalFlags(cmd) {
		spelled := "--" + flag.Name
		if !strings.Contains(cmd.Use, spelled) {
			flagNames = append(flagNames, spelled)
		}
	}
	if len(flagNames) > 0 {
		line += " `" + strings.Join(flagNames, " ") + "`"
	}
	return withNote(cmd, line+": "+cmd.Short)
}

// withNote appends cmd's index note, when it carries one, to line.
func withNote(cmd *cobra.Command, line string) string {
	if note := cmd.Annotations[IndexNoteAnnotation]; note != "" {
		return line + " " + note
	}
	return line
}

// visibleLocalFlags returns cmd's own non-persistent flags that are neither hidden nor one of --json, --verbose and --help.
func visibleLocalFlags(cmd *cobra.Command) []*pflag.Flag {
	var flags []*pflag.Flag
	cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Hidden && !metaFlags[flag.Name] && flag.Name != "verbose" {
			flags = append(flags, flag)
		}
	})
	return flags
}
