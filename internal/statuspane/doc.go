// Package statuspane provides the pane mechanics the runs overview and the gate checklist share:
// a redraw-in-place pane loop and a status-file reader.
// It takes every path and size it works on as values.
// It imports the standard library, internal/shedengine, internal/selfexec and golang.org/x/term.
//
// Pane.Run polls until its context ends and redraws only when the text or the terminal size changed.
// A redraw homes the cursor and erases the screen and the history before the frame.
// No line of an earlier, longer or wider frame survives on screen.
// No such line survives in the scrollback that tmux pulls back onto a growing pane.
// Every line is drawn uncut: a frame taller than the terminal scrolls its first lines into the history,
// and a line wider than the terminal wraps, so a size change alone redraws.
// The wait between polls starts at one second and doubles while nothing changes, up to an eight-second ceiling.
// A redraw returns it to one second, as PATTERN-daemon-wakeups requires.
// A terminal resize ends the wait at once where the platform signals one, through the channel NotifyResize returns,
// so the next poll redraws for the new size.
// A resize is an event and not a timer, so the idle wake rate is unchanged.
// TerminalSize reads the terminal's own size, so a size change is seen even when the frame text is the same.
//
// Each poll calls Pane.Check before reading the frame.
// By default it is a selfexec.Watcher's Check.
// A pane therefore re-execs onto a replaced lym binary on its own wait and adds no wake.
// The re-exec keeps the pane, its strand binding and its TMUX_PANE, since only the process image is replaced.
//
// StatusCache.Poll reads a shed run's status file without the run's status lock.
// That is safe because internal/state writes the file atomically.
// It decodes the file only when its modification time changed.
// A first failure to read keeps the last good status, and a second reports it unreadable.
//
// BuildIdentity names a lym build by its executable's symlink-resolved path, size and modification time.
// A Sidecar records which strand a bootstrap launched from which build.
// StrandCurrent decides whether to keep a strand:
// a live strand is current only when the sidecar names its guid and records the running build.
// A bootstrap therefore replaces a status strand launched by an older build.
//
// The panes' time formats come from internal/durfmt.
// Minutes serve every time still growing.
// A pane's text then changes at most once a minute, and its poll backs off between changes.
// Seconds serve a closed span, which never changes again and so costs no redraw.
package statuspane
