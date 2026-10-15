// Package statuspane is the pane mechanics the runs overview and the gate checklist share: a redraw-in-place pane loop, a status-file reader and the panes' time format.
// It takes every path and size it works on as values and imports the standard library, internal/shedengine, internal/selfexec and golang.org/x/term.
//
// Pane.Run polls until its context ends and redraws only when the text or the terminal size changed.
// A redraw homes the cursor and erases the screen and the history before the frame, so no line of an earlier, longer or wider frame survives on screen or in the scrollback tmux pulls back onto a growing pane.
// Every line is drawn uncut: a frame taller than the terminal scrolls its first lines into the history, and a line wider than the terminal wraps, so a size change alone redraws.
// The wait between polls starts at one second, doubles while nothing changes up to an eight-second ceiling and returns to one second on a redraw, as PATTERN-daemon-wakeups requires.
// A terminal resize ends the wait at once where the platform signals one, through the channel NotifyResize returns, so the next poll redraws for the new size; a resize is an event and never a timer, so the idle wake rate is unchanged.
// TerminalSize reads the terminal's own size, so a size change is seen even when the frame text is the same.
//
// Each poll calls Pane.Check before reading the frame; by default it is a selfexec.Watcher's Check, so a pane re-execs onto a replaced lyx binary on its own wait and adds no wake.
// The re-exec keeps the pane, its strand binding and its TMUX_PANE, since only the process image is replaced.
//
// StatusCache.Poll reads a shed run's status file without the run's status lock, which internal/state's atomic write makes safe, and decodes it only when its modification time changed.
// A first failure to read keeps the last good status and a second reports it unreadable.
//
// BuildIdentity names a lyx build by its executable's symlink-resolved path, size and modification time, and a Sidecar records which strand a bootstrap launched from which build.
// StrandCurrent is the one keep test: a live strand is current only when the sidecar names its guid and records the running build, so a bootstrap replaces a status strand launched by an older build.
//
// FormatMinutes is the format of every time that is still growing, whole minutes as 12m, 1h05m or 2d03h, so a pane's text changes at most once a minute and its poll backs off between changes.
// FormatSeconds is the format of a closed span, as 45s, 12m05s or 1h05m12s, which never changes again and so costs no redraw.
package statuspane
