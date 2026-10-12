// shortcut.go parses the embedded shortcut file, mounts one root command per shortcut and dispatches a shortcut's argvs on fresh roots.

package shortcutcli

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
)

// GroupID is the cobra group every shortcut command belongs to.
const GroupID = "shortcuts"

//go:embed shortcuts.yaml
var shortcutFile []byte

// Shortcut is one declared shortcut: its name and the argvs it runs in order.
type Shortcut struct {
	Name  string
	Argvs [][]string
}

// Load returns the shortcuts of the embedded file, sorted by name.
func Load() ([]Shortcut, error) {
	return parse(shortcutFile)
}

// parse decodes data into shortcuts sorted by name.
// A value that is a sequence of scalars is one argv and a sequence of sequences of scalars is a list of argvs; any other shape is an error naming the shortcut.
// A name declared twice yields two shortcuts, so a caller can refuse the duplicate.
func parse(data []byte) ([]Shortcut, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse the shortcut file: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parse the shortcut file: the document is not a mapping of shortcut names to argvs")
	}

	pairs := document.Content[0].Content
	shortcuts := make([]Shortcut, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		name := pairs[i].Value
		argvs, err := argvsOf(pairs[i+1])
		if err != nil {
			return nil, fmt.Errorf("shortcut %q: %w", name, err)
		}
		shortcuts = append(shortcuts, Shortcut{Name: name, Argvs: argvs})
	}
	sort.SliceStable(shortcuts, func(a, b int) bool { return shortcuts[a].Name < shortcuts[b].Name })
	return shortcuts, nil
}

// argvsOf reads node as one argv or as a list of argvs.
func argvsOf(node *yaml.Node) ([][]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("want an argv or a list of argvs, got a non-sequence value")
	}
	if words, ok := scalarsOf(node); ok {
		return [][]string{words}, nil
	}
	argvs := make([][]string, 0, len(node.Content))
	for _, child := range node.Content {
		words, ok := scalarsOf(child)
		if !ok {
			return nil, fmt.Errorf("want an argv or a list of argvs, got a mixed or nested sequence")
		}
		argvs = append(argvs, words)
	}
	return argvs, nil
}

// scalarsOf returns the words of node when it is a sequence of scalars.
func scalarsOf(node *yaml.Node) ([]string, bool) {
	if node.Kind != yaml.SequenceNode {
		return nil, false
	}
	words := make([]string, 0, len(node.Content))
	for _, child := range node.Content {
		if child.Kind != yaml.ScalarNode {
			return nil, false
		}
		words = append(words, child.Value)
	}
	return words, true
}

// Mount adds the embedded file's shortcuts to root, each running its argvs on a fresh root from newRoot.
func Mount(root *cobra.Command, newRoot func() *cobra.Command) {
	mount(root, newRoot, shortcutFile)
}

// mount is Mount over the file bytes data.
// A file that does not parse logs one Warn and mounts nothing.
func mount(root *cobra.Command, newRoot func() *cobra.Command, data []byte) {
	shortcuts, err := parse(data)
	if err != nil {
		logger.Warn("shortcutcli: the shortcut file does not parse, so no shortcut is mounted", "err", err)
		return
	}
	root.AddGroup(&cobra.Group{ID: GroupID, Title: "Shortcuts:"})
	for _, shortcut := range shortcuts {
		root.AddCommand(newShortcutCmd(shortcut, newRoot))
	}
}

// newShortcutCmd builds the root command of s: it takes every word unparsed, routes them and runs s's argvs in order.
func newShortcutCmd(s Shortcut, newRoot func() *cobra.Command) *cobra.Command {
	expansions := make([]string, len(s.Argvs))
	for i, argv := range s.Argvs {
		expansions[i] = "lyx " + strings.Join(argv, " ")
	}
	return &cobra.Command{
		Use:                s.Name + " [<args>...]",
		Short:              "shortcut: " + strings.Join(expansions, ", then "),
		GroupID:            GroupID,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Annotations:        map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			routed := routeWords(args)
			if routed.Help {
				if routed.JSONGiven {
					if err := cmd.Root().PersistentFlags().Set("json", routed.JSONValue); err != nil {
						return err
					}
				}
				return cmd.Help()
			}

			argvs := make([][]string, len(s.Argvs))
			for i, argv := range s.Argvs {
				argvs[i] = append(append([]string(nil), routed.Global...), argv...)
			}
			last := len(argvs) - 1
			argvs[last] = append(argvs[last], routed.Trailing...)
			clihelp.SetExit(cmd.Context(), runArgvs(cmd, newRoot, argvs))
			return nil
		},
	}
}

// routedWords is a shortcut's words sorted by where they go.
type routedWords struct {
	// Global holds the verbosity words, which go to every argv.
	Global []string
	// Help reports a word that asks for help, which prints the shortcut's help and runs nothing.
	Help bool
	// JSONGiven reports a word that named the json flag, and JSONValue is that flag's last value.
	JSONGiven bool
	JSONValue string
	// Trailing holds every other word, and everything from the first `--`, which go to the last argv.
	Trailing []string
}

// routeWords sorts words by walking them up to the first `--`.
// A word starting with `-` is parsed alone against a mirror of the root's global flags; one that parses with no positional left is global, and every other word is trailing.
func routeWords(words []string) routedWords {
	var routed routedWords
	for i, word := range words {
		if word == "--" {
			routed.Trailing = append(routed.Trailing, words[i:]...)
			break
		}
		if !strings.HasPrefix(word, "-") {
			routed.Trailing = append(routed.Trailing, word)
			continue
		}

		var verbosity int
		var help, jsonFlag bool
		mirror := pflag.NewFlagSet("global", pflag.ContinueOnError)
		mirror.SetOutput(io.Discard)
		mirror.CountVarP(&verbosity, "verbose", "v", "")
		mirror.BoolVarP(&help, "help", "h", false, "")
		mirror.BoolVar(&jsonFlag, "json", false, "")
		mirror.Lookup("json").NoOptDefVal = "true"
		if err := mirror.Parse([]string{word}); err != nil || mirror.NArg() > 0 {
			routed.Trailing = append(routed.Trailing, word)
			continue
		}

		if mirror.Changed("verbose") {
			routed.Global = append(routed.Global, word)
		}
		if mirror.Changed("json") {
			routed.JSONGiven = true
			routed.JSONValue = mirror.Lookup("json").Value.String()
		}
		if help || jsonFlag {
			routed.Help = true
		}
	}
	return routed
}

// runArgvs runs each argv in order on a fresh root from newRoot, in cmd's context and with cmd's input and output writers.
// It stops at the first non-zero exit code and returns it.
func runArgvs(cmd *cobra.Command, newRoot func() *cobra.Command, argvs [][]string) int {
	for _, argv := range argvs {
		inner := newRoot()
		inner.SetArgs(argv)
		inner.SetIn(cmd.InOrStdin())
		inner.SetOut(cmd.OutOrStdout())
		inner.SetErr(cmd.ErrOrStderr())
		if code := clihelp.RunRootCtx(cmd.Context(), inner, cmd.OutOrStdout()); code != 0 {
			return code
		}
	}
	return 0
}
