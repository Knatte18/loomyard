// shortcut_test.go pins the shortcut file's mount and a shortcut's word routing and dispatch, over a fake root that records the commands it runs.

package shortcutcli

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// fakeRun is what one run of a fake leaf saw.
type fakeRun struct {
	Path      string
	Args      []string
	Verbosity int
	Name      string
	Cwd       string
	Stdin     string
}

// fakeScript is a fake leaf's scripted answer: a non-zero exit code writes an error envelope, else the fields are the success envelope.
type fakeScript struct {
	exit   int
	fields map[string]any
}

// fakeRecorder records the runs of the fake leaves of every root fakeRootFactory builds.
type fakeRecorder struct {
	mu      sync.Mutex
	runs    []fakeRun
	scripts map[string]fakeScript
	// readInput makes `reed attach` record what it reads from the command's input.
	readInput bool
}

func (r *fakeRecorder) record(run fakeRun) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, run)
}

// fakeRootFactory returns a factory building a root of newRoot's shape: the global flags, groups `orch` and `reed`, and the leaves `orch start` and `reed attach`.
func fakeRootFactory(rec *fakeRecorder) func() *cobra.Command {
	return func() *cobra.Command {
		var verbosity int
		root := &cobra.Command{Use: "lyx", SilenceUsage: true, SilenceErrors: true}
		clihelp.InstallJSONHelp(root)
		root.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "increase log verbosity")

		leaf := func(use, path string, name *string) *cobra.Command {
			cmd := &cobra.Command{
				Use:         use,
				Short:       "fake " + path,
				Args:        cobra.ArbitraryArgs,
				Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
			}
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				cwd, _ := lyxcwd.CwdFrom(cmd.Context())
				run := fakeRun{Path: path, Verbosity: verbosity, Cwd: cwd}
				if len(args) > 0 {
					run.Args = append([]string(nil), args...)
				}
				if name != nil {
					run.Name = *name
				}
				if path == "reed attach" && rec.readInput {
					data, _ := io.ReadAll(cmd.InOrStdin())
					run.Stdin = string(data)
				}
				rec.record(run)

				script := rec.scripts[path]
				if script.exit != 0 {
					output.Err(cmd.OutOrStdout(), "scripted failure of "+path)
					clihelp.SetExit(cmd.Context(), script.exit)
					return nil
				}
				fields := script.fields
				if fields == nil {
					fields = map[string]any{"ran": path}
				}
				output.Ok(cmd.OutOrStdout(), fields)
				return nil
			}
			return cmd
		}

		var attachName string
		attach := leaf("attach", "reed attach", &attachName)
		attach.Flags().StringVar(&attachName, "name", "", "strand name")

		orch := &cobra.Command{Use: "orch", Short: "fake orch", RunE: clihelp.GroupRunE}
		orch.AddCommand(leaf("start", "orch start", nil))
		reed := &cobra.Command{Use: "reed", Short: "fake reed", RunE: clihelp.GroupRunE}
		reed.AddCommand(attach)
		root.AddGroup(&cobra.Group{ID: "orch", Title: "Orch:"}, &cobra.Group{ID: "reed", Title: "Reed:"})
		orch.GroupID, reed.GroupID = "orch", "reed"
		root.AddCommand(orch, reed)
		return root
	}
}

const testShortcuts = "attach: [reed, attach]\nprime: [[orch, start], [reed, attach]]\n"

func TestShortcut_Run(t *testing.T) {
	t.Parallel()

	orchStart := func(verbosity int) fakeRun { return fakeRun{Path: "orch start", Verbosity: verbosity} }
	reedAttach := func(verbosity int, name string, args ...string) fakeRun {
		return fakeRun{Path: "reed attach", Verbosity: verbosity, Name: name, Args: args}
	}

	tests := []struct {
		name    string
		args    []string
		scripts map[string]fakeScript
		input   string
		// wantRuns lists the runs in order; Cwd is filled in by the test.
		wantRuns []fakeRun
		wantCode int
		// wantOut lists substrings the output holds, and wantErrEnvelopes counts its error envelopes.
		wantOut          []string
		wantErrEnvelopes int
	}{
		{name: "attach runs reed attach once", args: []string{"attach"}, wantRuns: []fakeRun{reedAttach(0, "")}, wantOut: []string{`"ran":"reed attach"`}},
		{name: "prime runs orch start then reed attach", args: []string{"prime"}, wantRuns: []fakeRun{orchStart(0), reedAttach(0, "")}},
		{
			name: "prime runs reed attach after an already-running orch start", args: []string{"prime"},
			scripts:  map[string]fakeScript{"orch start": {fields: map[string]any{"already_running": true}}},
			wantRuns: []fakeRun{orchStart(0), reedAttach(0, "")}, wantOut: []string{`"already_running":true`},
		},
		{
			name: "a failing orch start stops the sequence with its exit code", args: []string{"prime"},
			scripts:  map[string]fakeScript{"orch start": {exit: 2}},
			wantRuns: []fakeRun{orchStart(0)}, wantCode: 2, wantErrEnvelopes: 1,
		},
		{
			name: "trailing words go to the last argv only", args: []string{"prime", "--name", "x", "extra"},
			wantRuns: []fakeRun{orchStart(0), reedAttach(0, "x", "extra")},
		},
		{name: "a leading verbosity word reaches every argv", args: []string{"-v", "prime"}, wantRuns: []fakeRun{orchStart(1), reedAttach(1, "")}},
		{name: "a doubled verbosity word counts twice", args: []string{"-vv", "prime"}, wantRuns: []fakeRun{orchStart(2), reedAttach(2, "")}},
		{name: "a verbosity word after the shortcut reaches every argv", args: []string{"prime", "-v"}, wantRuns: []fakeRun{orchStart(1), reedAttach(1, "")}},
		{name: "a verbosity word with a value reaches every argv", args: []string{"prime", "--verbose=2"}, wantRuns: []fakeRun{orchStart(2), reedAttach(2, "")}},
		{name: "json prints the shortcut's help as JSON and runs nothing", args: []string{"prime", "--json"}, wantOut: []string{`"name": "lyx prime"`}},
		{name: "json with a value prints the JSON help", args: []string{"prime", "--json=true"}, wantOut: []string{`"name": "lyx prime"`}},
		{name: "a false-valued json word is dropped and the argvs run", args: []string{"prime", "--json=false"}, wantRuns: []fakeRun{orchStart(0), reedAttach(0, "")}},
		{name: "help prints the shortcut's text help and runs nothing", args: []string{"prime", "--help"}, wantOut: []string{"shortcut: lyx orch start, then lyx reed attach"}},
		{name: "a short help word prints the text help", args: []string{"prime", "-h"}, wantOut: []string{"shortcut: lyx orch start, then lyx reed attach"}},
		{name: "a shorthand run holding help prints the text help", args: []string{"prime", "-vh"}, wantOut: []string{"shortcut: lyx orch start, then lyx reed attach"}},
		{name: "words after the double dash are trailing, help included", args: []string{"prime", "--", "-h"}, wantRuns: []fakeRun{orchStart(0), reedAttach(0, "", "-h")}},
		{
			name: "a global flag with a bad value fails in the last argv's root after the earlier argvs ran", args: []string{"prime", "--json=bogus"},
			wantRuns: []fakeRun{orchStart(0)}, wantCode: 1, wantErrEnvelopes: 1,
		},
		{name: "the outer root's input reaches the argv", args: []string{"attach"}, input: "typed", wantRuns: []fakeRun{{Path: "reed attach", Stdin: "typed"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &fakeRecorder{scripts: tt.scripts, readInput: tt.input != ""}
			factory := fakeRootFactory(rec)
			root := factory()
			mount(root, factory, []byte(testShortcuts))
			if tt.input != "" {
				root.SetIn(strings.NewReader(tt.input))
			}
			cwd := t.TempDir()
			var out strings.Builder
			code := clihelp.ExecuteIn(root, cwd, &out, tt.args)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; output:\n%s", code, tt.wantCode, out.String())
			}
			wantRuns := append([]fakeRun(nil), tt.wantRuns...)
			for i := range wantRuns {
				wantRuns[i].Cwd = cwd
			}
			if diff := cmp.Diff(wantRuns, rec.runs); diff != "" {
				t.Errorf("runs mismatch (-want +got):\n%s", diff)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out.String())
				}
			}
			if got := strings.Count(out.String(), `"ok":false`); got != tt.wantErrEnvelopes {
				t.Errorf("error envelopes = %d, want %d; output:\n%s", got, tt.wantErrEnvelopes, out.String())
			}
		})
	}
}

// TestMount_File is not parallel: logcapture.Capture swaps the process-global logger output.
func TestMount_File(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		// wantMounted maps each shortcut that must be mounted to its Short.
		wantMounted map[string]string
	}{
		{
			name: "the shipped file mounts attach and prime", data: shortcutFile,
			wantMounted: map[string]string{
				"attach": "shortcut: lyx reed attach",
				"prime":  "shortcut: lyx orch start, then lyx reed attach",
			},
		},
		{name: "malformed YAML mounts nothing", data: []byte("attach: [reed, attach")},
		{name: "a scalar value mounts nothing", data: []byte("attach: reed attach\n")},
		{name: "a mapping value mounts nothing", data: []byte("attach:\n  reed: attach\n")},
		{name: "a sequence mixing a word and a list mounts nothing", data: []byte("attach: [reed, [attach]]\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			factory := fakeRootFactory(&fakeRecorder{})
			root := factory()
			mount(root, factory, tt.data)

			mounted := map[string]string{}
			for _, child := range root.Commands() {
				if child.GroupID != GroupID {
					continue
				}
				mounted[child.Name()] = child.Short
				if want := child.Name() + " [<args>...]"; child.Use != want {
					t.Errorf("%s Use = %q, want %q", child.Name(), child.Use, want)
				}
				if got := child.Annotations[clihelp.AudienceAnnotation]; got != clihelp.AudienceOperator {
					t.Errorf("%s audience = %q, want %q", child.Name(), got, clihelp.AudienceOperator)
				}
				if !child.DisableFlagParsing {
					t.Errorf("%s keeps flag parsing on, want it off", child.Name())
				}
			}
			if diff := cmp.Diff(tt.wantMounted, mounted); len(tt.wantMounted) > 0 && diff != "" {
				t.Errorf("mounted shortcuts mismatch (-want +got):\n%s", diff)
			}
			if len(tt.wantMounted) == 0 {
				if len(mounted) != 0 || root.ContainsGroup(GroupID) {
					t.Errorf("mounted %v, group present = %v, want neither", mounted, root.ContainsGroup(GroupID))
				}
				if got := strings.Count(logs.String(), "the shortcut file does not parse"); got != 1 {
					t.Errorf("parse warnings = %d, want 1; log:\n%s", got, logs.String())
				}
				return
			}
			if !root.ContainsGroup(GroupID) {
				t.Error("the shortcut group is missing")
			}
			if logs.String() != "" {
				t.Errorf("log = %q, want nothing", logs.String())
			}
		})
	}
}
