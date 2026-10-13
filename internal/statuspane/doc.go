// Package statuspane is the pane mechanics the runs overview and the gate checklist share: a redraw-in-place pane loop, a status-file reader and the panes' time format.
// It takes every path and size it works on as values and imports the standard library, internal/shedengine, internal/agentname, internal/logger and golang.org/x/term.
//
// Pane.Run polls until its context ends.
// Each poll it reports the frame's line count through Pane.Report when it differs from the count last reported successfully, so reed can size the strand to its content, and redraws only when the text or the terminal size changed.
// A redraw homes the cursor and erases the screen and the history before the frame, so no line of an earlier, longer or wider frame survives on screen or in the scrollback tmux pulls back onto a growing pane; each line is cut to the terminal's columns so none wraps.
// A frame taller than the terminal prints the rows its own Fit returns, the last a summary line naming how many are not shown.
// The wait between polls starts at one second, doubles while nothing changes up to an eight-second ceiling and returns to one second on a redraw, as PATTERN-daemon-wakeups requires.
// A terminal resize ends the wait at once where the platform signals one, through the channel NotifyResize returns, so the next poll redraws for the new size; a resize is an event and never a timer, so the idle wake rate is unchanged.
// ReportFromEnv builds the report func from the strand name reed exported to the pane and its own TMUX_PANE, so reed's name and pane checks bound a report to the strand that runs the pane.
// TerminalSize reads the terminal's own size, so a summary line is right before reed has applied a reported count.
//
// StatusCache.Poll reads a shed run's status file without the run's status lock, which internal/state's atomic write makes safe, and decodes it only when its modification time changed.
// A first failure to read keeps the last good status and a second reports it unreadable.
//
// BuildIdentity names a lyx build by its executable's symlink-resolved path, size and modification time, and a Sidecar records which strand a bootstrap launched from which build.
// StrandCurrent is the one keep test: a live strand is current only when the sidecar names its guid and records the running build, so a bootstrap replaces a status strand launched by an older build.
//
// FormatMinutes is the one time format both panes use, whole minutes as 12m, 1h05m or 2d03h, so a pane's text changes at most once a minute and its poll backs off between changes.
package statuspane
