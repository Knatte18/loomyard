// cli.go builds the creel cobra command with its operator verbs and the hidden daemon verb, and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package creelcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// Exit codes beyond 0 for success and 1 for a refusal: the ways a waiting verb ends without the outcome the caller asked for.
const (
	// ExitInjectUnconfirmed is the exit of an inject --wait whose text was typed but not confirmed.
	ExitInjectUnconfirmed = 2
	// ExitInjectExpired is the exit of an inject --wait that expired before it could be typed.
	ExitInjectExpired = 3
	// ExitWaitTimeout is the exit of a waiting verb whose --timeout passed with no outcome.
	ExitWaitTimeout = 4
	// ExitReturnedUndelivered is the exit of a post --wait-reply whose message returned undelivered.
	ExitReturnedUndelivered = 5
)

// realClock is the production creelengine.Clock.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// clientSeam is the clock and daemon spawn every verb's client is built with.
// spawn starts the daemon for a hub with its stderr appended to the log file at logPath and answers the daemon's pid with the signal of its exit.
// A test replaces it, and so does not run in parallel.
type clientSeam struct {
	clock creelengine.Clock
	spawn func(hubPath, logPath string) (creelengine.Spawn, error)
}

var seam = clientSeam{clock: realClock{}, spawn: spawnDaemon}

// Command returns the cobra command for `lyx creel`: the bare group, its operator verbs and the hidden daemon.
func Command() *cobra.Command {
	creel := &cobra.Command{
		Use:   "creel",
		Short: "send messages and injects between the agents of the hub through its message daemon",
		Long: `creel is the hub's message daemon and its verbs. post leaves a message for a
strand, which gets a one-line reminder typed when it is idle, and read hands
the message over. inject types a line into an idle strand at once, interrupt
stops a running turn, forward passes a message on, hold keeps a receiver to
one sender for a while, and inbox and log show the pool. The daemon starts
by itself on the first verb.`,
		RunE: clihelp.GroupRunE,
	}
	creel.AddCommand(
		postCmd(), injectCmd(), interruptCmd(), forwardCmd(), readCmd(), inboxCmd(), logCmd(), holdCmd(), releaseCmd(), daemonCmd(),
	)
	// Creel reads no stencils, and the seed pass's log lines would mix into the stderr its verbs use for refusals and stall progress.
	// The long-lived daemon must also never commit seed writes into the hub.
	for _, cmd := range append([]*cobra.Command{creel}, creel.Commands()...) {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[clihelp.SkipStencilSeedAnnotation] = clihelp.AnnotationEnabled
	}
	return creel
}

// result is what a verb's body hands back: the fields to print, the exit code and, for a non-zero code, the message that goes with it.
type result struct {
	fields map[string]any
	code   int
	note   string
	// printed says the body already wrote its output.
	printed bool
}

// refuse prints err as a refusal and records exit 1.
func refuse(cmd *cobra.Command, err error) error {
	writeRefusal(cmd, err.Error(), nil, 1)
	return nil
}

// writeRefusal prints the JSON error envelope, with ok false, the trimmed msg and fields, to the command's stdout and records code as the exit.
// When that write fails it writes one line "creel: <msg>" to the command's stderr instead, so a refusal is never lost; the exit code stays code.
func writeRefusal(cmd *cobra.Command, msg string, fields map[string]any, code int) {
	clihelp.SetExit(cmd.Context(), code)
	msg = strings.TrimSpace(msg)
	if fields == nil {
		fields = map[string]any{}
	}
	fields["ok"] = false
	fields["error"] = msg
	data, err := json.Marshal(fields)
	if err == nil {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "creel: "+msg)
	}
}

// writeOk prints fields as one JSON line with ok set to true, as output.Ok does, and returns the write's error.
func writeOk(w io.Writer, fields map[string]any) error {
	fields["ok"] = true
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// runVerb resolves the hub from the command's cwd, builds the verb's client and runs body, then prints what it returns.
// A standalone directory is refused: creel needs a hub.
func runVerb(cmd *cobra.Command, timeout time.Duration, body func(client *creelengine.Client, out io.Writer) (result, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return refuse(cmd, err)
	}
	location, mode, err := preflight.ResolveMode(cwd)
	if err != nil {
		return refuse(cmd, err)
	}
	if mode != preflight.ModeHub {
		return refuse(cmd, errors.New("creel needs a hub, and this directory is not in one; way forward: run the verb from a worktree of a lyx hub"))
	}
	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		return refuse(cmd, fmt.Errorf("open the lyxtemp root: %w", err))
	}
	geometry, err := hubgeom.CreelGeometry(location.HubPath, root.Path())
	if err != nil {
		return refuse(cmd, err)
	}
	hubPath := location.HubPath
	client := creelengine.NewClient(geometry, claimFor(cwd), func() (creelengine.Spawn, error) { return seam.spawn(hubPath, geometry.DaemonLogPath) }, seam.clock)
	res, err := body(client, out)
	if errors.Is(err, creelengine.ErrWaitTimeout) {
		writeRefusal(cmd, "no outcome arrived before the timeout of "+timeout.String(), res.fields, ExitWaitTimeout)
		return nil
	}
	if err != nil {
		return refuse(cmd, err)
	}
	if res.printed {
		return nil
	}
	if res.code != 0 {
		writeRefusal(cmd, res.note, res.fields, res.code)
		return nil
	}
	output.Ok(out, res.fields)
	return nil
}

// claimFor states who the caller says it is: its strand name, and the envelope an EXTERNAL caller is shown with.
// A claim names no program; a program sender comes only through the Go client.
func claimFor(cwd string) creelengine.Claim {
	return creelengine.Claim{
		StrandName:    os.Getenv(agentname.StrandNameEnv),
		Terminal:      controllingTerminal(),
		ParentProcess: parentProcessName(),
		WorkingDir:    cwd,
	}
}

// controllingTerminal returns the terminal standard input is attached to without its /dev/ prefix, empty when it is none or the platform cannot tell.
func controllingTerminal() string {
	target, err := os.Readlink("/proc/self/fd/0")
	if err != nil || !strings.HasPrefix(target, "/dev/") {
		return ""
	}
	return strings.TrimPrefix(target, "/dev/")
}

// parentProcessName returns the name of this process's parent, empty when the platform cannot tell.
func parentProcessName() string {
	parent, ok := proc.ParentPID(os.Getpid())
	if !ok {
		return ""
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(parent) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// stallFields renders the stall a response carries as the stall, its reason text with the way forward and the time it was read; nil when it carries none.
func stallFields(resp creelengine.Response) map[string]any {
	if resp.Stall == "" {
		return nil
	}
	return map[string]any{"stall": string(resp.Stall), "stall_reason": resp.Stall.Reason(), "stall_at": resp.StallAt.Format(time.RFC3339)}
}

// messageFields renders a message with its envelope and body.
func messageFields(message creelengine.Message) map[string]any {
	fields := map[string]any{"id": message.ID, "from": message.From, "kind": message.Kind, "body": message.Body, "posted": message.Posted}
	if message.Envelope != "" {
		fields["envelope"] = message.Envelope
	}
	if len(message.Path) > 0 {
		fields["path"] = message.Path
	}
	if message.ReplyTo != 0 {
		fields["reply_to"] = message.ReplyTo
	}
	if message.EndedUnreadBy != "" {
		fields["ended_unread_by"] = message.EndedUnreadBy
		fields["ended_unread_at"] = message.EndedUnreadAt
	}
	return fields
}

// messagesFields renders messages, never nil so an empty read prints an empty list.
func messagesFields(messages []creelengine.Message) []map[string]any {
	rendered := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		rendered = append(rendered, messageFields(message))
	}
	return rendered
}

// postCmd returns the `creel post` verb.
func postCmd() *cobra.Command {
	var (
		replyID   int64
		waitReply bool
		timeout   time.Duration
	)
	cmd := &cobra.Command{
		Use:         "post [<receiver>] <text>",
		Short:       "leave a message for a strand, or reply to one",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `post leaves the text as a message for the receiver strand and prints its id.
The receiver gets a one-line reminder typed when it is idle, and reads the
message with "lyx creel read".

With --reply <id> the text answers message <id> and goes to its sender, so no
receiver is given. With --wait-reply the call waits for the reply, up to
--timeout, and exits 5 if the message returns undelivered.

Example:
  lyx creel post ly:impl "the schema changed, rebase before you go on"
  lyx creel post --reply 12 "done"`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			replying := cmd.Flags().Changed("reply")
			switch {
			case replying && len(args) != 1:
				return refuse(cmd, errors.New("post --reply takes the text alone; the forms are: post <receiver> <text> and post --reply <id> <text>"))
			case !replying && len(args) != 2:
				return refuse(cmd, errors.New("post takes a receiver and a text; the forms are: post <receiver> <text> and post --reply <id> <text>"))
			case replying && waitReply:
				return refuse(cmd, errors.New("--wait-reply applies to a post to a receiver, not to a --reply"))
			}
			return runVerb(cmd, timeout, func(client *creelengine.Client, _ io.Writer) (result, error) {
				if replying {
					resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbReply, ID: replyID, Text: args[0]}, 0, nil)
					if err != nil {
						return result{}, err
					}
					return result{fields: map[string]any{"id": resp.ID, "sender": resp.Sender}}, nil
				}
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbPost, Receiver: args[0], Text: args[1], Wait: waitReply}, timeout, nil)
				if err != nil {
					return result{}, err
				}
				fields := map[string]any{"id": resp.ID, "sender": resp.Sender}
				if !waitReply {
					return result{fields: fields}, nil
				}
				fields["outcome"] = resp.Outcome
				if resp.Message == nil {
					return result{fields: fields}, nil
				}
				if resp.Outcome == creelengine.OutcomeReturned {
					fields["returned"] = messageFields(*resp.Message)
					return result{fields: fields, code: ExitReturnedUndelivered, note: "the message returned undelivered"}, nil
				}
				fields["reply"] = messageFields(*resp.Message)
				return result{fields: fields}, nil
			})
		},
	}
	cmd.Flags().Int64Var(&replyID, "reply", 0, "answer the message with this id instead of posting to a receiver")
	cmd.Flags().BoolVar(&waitReply, "wait-reply", false, "wait for the reply to the message")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "give up waiting after this long, 0 for never")
	return cmd
}

// injectCmd returns the `creel inject` verb.
func injectCmd() *cobra.Command {
	var (
		interrupt bool
		ttl       time.Duration
		wait      bool
		timeout   time.Duration
	)
	cmd := &cobra.Command{
		Use:         "inject <receiver> <text>",
		Short:       "type a line into an idle strand's pane",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `inject queues the text to be typed into the receiver's pane the moment it is
idle, and prints the inject's id. EXTERNAL callers, programs and orch strands
may inject, interrupt and hold on any strand, and any other strand on its own
strand only.

--interrupt stops the receiver's running turn first. --ttl sets how long the
inject waits for an idle receiver before it expires. With --wait the call
waits for the outcome, up to --timeout: exit 0 when injected, 2 when typed
but unconfirmed, 3 when expired.

Example:
  lyx creel inject ly:impl "stop and run the tests" --interrupt --wait`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, timeout, func(client *creelengine.Client, _ io.Writer) (result, error) {
				req := creelengine.Request{
					Verb: creelengine.VerbInject, Receiver: args[0], Text: args[1],
					Interrupt: interrupt, TTLSeconds: int64(ttl / time.Second), Wait: wait,
				}
				var last creelengine.Response
				resp, err := client.CallWatching(req, timeout, nil, func(note creelengine.Response) {
					last = note
					fmt.Fprintf(cmd.ErrOrStderr(), "creel: %s is stalled: %s (at %s)\n", args[0], note.Stall.Reason(), note.StallAt.Format(time.RFC3339))
				})
				if err != nil {
					return result{fields: stallFields(last)}, err
				}
				fields := map[string]any{"id": resp.ID, "sender": resp.Sender}
				if !wait {
					fields["interrupted"] = resp.Interrupted
					return result{fields: fields}, nil
				}
				fields["outcome"] = resp.Outcome
				switch resp.Outcome {
				case creelengine.OutcomeUnconfirmed:
					return result{fields: fields, code: ExitInjectUnconfirmed, note: "the text was typed but its landing was not confirmed"}, nil
				case creelengine.OutcomeExpired:
					maps.Copy(fields, stallFields(resp))
					return result{fields: fields, code: ExitInjectExpired, note: "the inject expired before it could be typed"}, nil
				}
				return result{fields: fields}, nil
			})
		},
	}
	cmd.Flags().BoolVar(&interrupt, "interrupt", false, "stop the receiver's running turn before typing")
	cmd.Flags().DurationVar(&ttl, "ttl", 0, "how long the inject waits for an idle receiver, 0 for the default")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the inject's outcome")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "give up waiting after this long, 0 for never")
	return cmd
}

// interruptCmd returns the `creel interrupt` verb.
func interruptCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "interrupt <receiver>",
		Short:       "stop the receiver's running turn",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `interrupt sends the guarded interrupt to the receiver's running turn and
prints whether it stopped one. A receiver with no running turn is sent
nothing. EXTERNAL callers, programs and orch strands may inject, interrupt and
hold on any strand, and any other strand on its own strand only.

Example:
  lyx creel interrupt ly:impl`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbInterrupt, Receiver: args[0]}, 0, nil)
				if err != nil {
					return result{}, err
				}
				return result{fields: map[string]any{"sent": resp.Sent}}, nil
			})
		},
	}
}

// forwardCmd returns the `creel forward` verb.
func forwardCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "forward <id> <receiver>",
		Short:       "pass a message on to another strand",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `forward hands the message with the id to the receiver, which then holds it
and gets a reminder; the message keeps its original sender and records who
passed it on.

Example:
  lyx creel forward 12 ly:reviewer`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return refuse(cmd, fmt.Errorf("forward: %q is not a message id; way forward: pass the number \"lyx creel inbox\" or \"lyx creel read\" shows", args[0]))
			}
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				if _, err := client.Call(creelengine.Request{Verb: creelengine.VerbForward, ID: id, Receiver: args[1]}, 0, nil); err != nil {
					return result{}, err
				}
				return result{fields: map[string]any{"forwarded": id, "to": args[1]}}, nil
			})
		},
	}
}

// readCmd returns the `creel read` verb.
// It acknowledges the messages only once its envelope was written; a write that fails leaves them unread and the verb refuses with the write's error.
// A write that fails after part of the envelope went out leaves them unread too, so a consumer may hold a truncated envelope of messages it reads again later.
func readCmd() *cobra.Command {
	var (
		from    string
		all     bool
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:         "read",
		Short:       "print the messages left for the caller and mark them read",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `read prints the oldest unread message left for the caller and every other
unread message from its sender, each with its sender, envelope and body, and
then marks them read. With nothing unread it says so and exits 0.

--from prints every unread message from one sender, --all prints every unread
message, and --wait waits for unread mail, up to --timeout, before it reads.

Example:
  lyx creel read
  lyx creel read --from ly:orch --wait`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, timeout, func(client *creelengine.Client, out io.Writer) (result, error) {
				printed := false
				printMessages := func(resp creelengine.Response) error {
					printed = true
					fields := map[string]any{"messages": messagesFields(resp.Messages)}
					if len(resp.Messages) == 0 {
						fields["note"] = "nothing unread"
					}
					if err := writeOk(out, fields); err != nil {
						return fmt.Errorf("creel read: could not write the messages, so they stay unread: %w", err)
					}
					return nil
				}
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbRead, From: from, All: all, Wait: wait}, timeout, printMessages)
				if err != nil {
					return result{}, err
				}
				if !printed {
					if err := printMessages(resp); err != nil {
						return result{}, err
					}
				}
				return result{printed: true}, nil
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "read only the messages from this sender")
	cmd.Flags().BoolVar(&all, "all", false, "read every unread message, not only the oldest sender's")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for unread mail before reading")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "give up waiting after this long, 0 for never")
	return cmd
}

// inboxCmd returns the `creel inbox` verb.
func inboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "inbox",
		Short:       "count the caller's unread messages by sender",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `inbox prints how many unread messages each sender has left for the caller,
without marking anything read.

Example:
  lyx creel inbox`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbInbox}, 0, nil)
				if err != nil {
					return result{}, err
				}
				senders := make([]map[string]any, 0, len(resp.Inbox.Senders))
				for _, entry := range resp.Inbox.Senders {
					senders = append(senders, map[string]any{"from": entry.From, "unread": entry.Unread})
				}
				inbox := map[string]any{"senders": senders}
				if resp.Inbox.Withheld {
					inbox["withheld"] = true
					inbox["notice"] = resp.Inbox.Notice
				}
				return result{fields: map[string]any{"inbox": inbox}}, nil
			})
		},
	}
}

// logCmd returns the `creel log` verb.
func logCmd() *cobra.Command {
	var (
		id    int64
		from  string
		to    string
		since string
	)
	cmd := &cobra.Command{
		Use:         "log",
		Short:       "show the events of the message journal, without bodies",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `log prints the journal's events, oldest first: who posted, read, forwarded,
injected or held, and when. It never prints a message body.

--id limits it to one message, --from and --to to a sender and a receiver,
and --since to events after a time: an RFC 3339 timestamp, or a duration
such as 2h meaning that long ago.

Example:
  lyx creel log --id 12
  lyx creel log --since 1h`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var sinceTime time.Time
			if since != "" {
				parsed, err := parseSince(since, time.Now())
				if err != nil {
					return refuse(cmd, err)
				}
				sinceTime = parsed
			}
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbLog, ID: id, From: from, Receiver: to, Since: sinceTime}, 0, nil)
				if err != nil {
					return result{}, err
				}
				entries := make([]map[string]any, 0, len(resp.Log))
				for _, entry := range resp.Log {
					entries = append(entries, logEntryFields(entry))
				}
				return result{fields: map[string]any{"log": entries}}, nil
			})
		},
	}
	cmd.Flags().Int64Var(&id, "id", 0, "show only the events of the message with this id")
	cmd.Flags().StringVar(&from, "from", "", "show only the events from this sender")
	cmd.Flags().StringVar(&to, "to", "", "show only the events to this receiver")
	cmd.Flags().StringVar(&since, "since", "", "show only the events after this RFC 3339 time, or this long ago")
	return cmd
}

// parseSince reads an RFC 3339 timestamp, or a duration meaning that long before now.
func parseSince(value string, now time.Time) (time.Time, error) {
	if at, err := time.Parse(time.RFC3339, value); err == nil {
		return at, nil
	}
	if ago, err := time.ParseDuration(value); err == nil {
		return now.Add(-ago), nil
	}
	return time.Time{}, fmt.Errorf("log: --since %q is neither an RFC 3339 time nor a duration; way forward: pass one such as 2026-01-02T15:04:05Z or 2h", value)
}

// logEntryFields renders a journal event, leaving out the fields it does not carry.
func logEntryFields(entry creelengine.LogEntry) map[string]any {
	fields := map[string]any{"event": entry.Event, "at": entry.At}
	if entry.ID != 0 {
		fields["id"] = entry.ID
	}
	for key, value := range map[string]string{
		"from": entry.From, "to": entry.To, "by": entry.By, "receiver": entry.Receiver, "subject": entry.Subject, "notice": entry.Notice, "reason": entry.Reason, "kind": entry.Kind,
	} {
		if value != "" {
			fields[key] = value
		}
	}
	if len(entry.Unread) > 0 {
		fields["unread"] = entry.Unread
	}
	if entry.Step != 0 {
		fields["step"] = entry.Step
	}
	return fields
}

// holdCmd returns the `creel hold` verb.
func holdCmd() *cobra.Command {
	var duration time.Duration
	cmd := &cobra.Command{
		Use:         "hold <receiver> --for <duration>",
		Short:       "hold a receiver's deliveries to the caller's own for a while",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `hold keeps the receiver's deliveries to the caller's: only the caller's injects
pass and only its mail is reminded, until the hold lapses after --for or
"lyx creel release". Holding again renews it; another sender's hold on a held
receiver is refused. EXTERNAL callers, programs and orch strands may inject,
interrupt and hold on any strand, and any other strand on its own strand only.

Example:
  lyx creel hold ly:impl --for 10m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbHold, Receiver: args[0], ForSeconds: int64(duration / time.Second)}, 0, nil)
				if err != nil {
					return result{}, err
				}
				return result{fields: map[string]any{"hold": map[string]any{
					"owner": resp.Hold.Owner, "expires": resp.Hold.Expires, "renewed": resp.Hold.Renewed,
				}}}, nil
			})
		},
	}
	cmd.Flags().DurationVar(&duration, "for", 0, "how long the hold lasts")
	_ = cmd.MarkFlagRequired("for")
	return cmd
}

// releaseCmd returns the `creel release` verb.
func releaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "release <receiver>",
		Short:       "end the caller's hold on a receiver",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `release ends the caller's hold on the receiver and prints whether there was
one to end. An unknown name is refused with the names that exist. EXTERNAL
callers, programs and orch strands may inject, interrupt and hold on any
strand, and any other strand on its own strand only.

Example:
  lyx creel release ly:impl`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, 0, func(client *creelengine.Client, _ io.Writer) (result, error) {
				resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbRelease, Receiver: args[0]}, 0, nil)
				if err != nil {
					return result{}, err
				}
				return result{fields: map[string]any{"released": resp.Released}}, nil
			})
		},
	}
}

// daemonCmd returns the hidden `creel daemon` verb, the per-hub daemon that the first verb starts.
func daemonCmd() *cobra.Command {
	var hubPath string
	cmd := &cobra.Command{
		Use:         "daemon --hub-path <path>",
		Short:       "run the message daemon of one hub",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `daemon serves the hub at --hub-path until it is stopped or its binary is
replaced, and is what the first creel verb starts. Run beside a live daemon of
the same hub it says so and exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if !filepath.IsAbs(hubPath) {
				return refuse(cmd, fmt.Errorf("daemon: --hub-path %q is not an absolute path; way forward: pass the hub's absolute path", hubPath))
			}
			err := runDaemon(cmd.Context(), hubPath, cmd.ErrOrStderr())
			if errors.Is(err, creelengine.ErrDaemonRunning) {
				output.Ok(cmd.OutOrStdout(), map[string]any{"daemon": "another daemon already serves this hub"})
				return nil
			}
			if err != nil {
				return refuse(cmd, err)
			}
			output.Ok(cmd.OutOrStdout(), map[string]any{"daemon": "stopped"})
			return nil
		},
	}
	cmd.Flags().StringVar(&hubPath, "hub-path", "", "absolute path of the hub to serve")
	_ = cmd.MarkFlagRequired("hub-path")
	return cmd
}

// pidLineWriter writes the lines it is handed to dst, each opened with the creel daemon log's pid prefix of this process.
// It buffers a line until its newline arrives and writes each line to dst in one call, so two daemons appending to one file never interleave inside a line.
type pidLineWriter struct {
	dst     io.Writer
	mu      sync.Mutex
	pending []byte
}

// Write splits p into lines and writes each completed line, prefixed, to dst; a trailing partial line waits for its newline.
// It reports len(p) written unless dst fails.
func (w *pidLineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	prefix := creelengine.DaemonLogPrefix(os.Getpid())
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		line := prefix + string(w.pending[:end+1])
		w.pending = w.pending[end+1:]
		if _, err := io.WriteString(w.dst, line); err != nil {
			return len(p), err
		}
	}
}

// runDaemon serves the hub at hubPath until a signal or a replaced binary ends it.
// It first writes the daemon start header to stderr, then sends the logger's warnings there, each line tagged with this process's pid.
// A start that fails for any reason but another daemon already serving is logged at Warn before it is returned.
func runDaemon(ctx context.Context, hubPath string, stderr io.Writer) error {
	if _, err := fmt.Fprintln(stderr, creelengine.DaemonStartHeader(os.Getpid(), time.Now())); err != nil {
		return fmt.Errorf("write the daemon start header: %w", err)
	}
	logger.SetOutput(&pidLineWriter{dst: stderr})
	logger.SetVerbosity(0)
	err := serveDaemon(ctx, hubPath)
	if err != nil && !errors.Is(err, creelengine.ErrDaemonRunning) {
		logger.Warn("creel: the daemon did not start or ended with an error", "error", err)
	}
	return err
}

// serveDaemon resolves the hub's geometry and runs the engine's daemon over it.
func serveDaemon(ctx context.Context, hubPath string) error {
	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		return fmt.Errorf("open the lyxtemp root: %w", err)
	}
	geometry, err := hubgeom.CreelGeometry(hubPath, root.Path())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	delivery := newHubDelivery(hubPath, shuttleengines.New)
	return creelengine.Run(ctx, creelengine.Options{
		Geometry:  geometry,
		Delivery:  delivery,
		Clock:     realClock{},
		ParentPID: proc.ParentPID,
		GOOS:      runtime.GOOS,
		Replaced:  selfexec.New().Replaced,
	})
}

// RunCLI is the public seam for the creel module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
