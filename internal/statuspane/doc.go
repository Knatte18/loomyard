// Package statuspane provides the pane mechanics the runs overview and the gate checklist share:
// a redraw-in-place pane loop and a status-file reader.
// It takes every path and size it works on as values.
// It imports the standard library, internal/shedengine, internal/selfexec and golang.org/x/term.
//
// Pane.Run polls until its context ends and redraws only when the text or the terminal size changed.
// No line of an earlier, longer or wider frame survives on screen or in the scrollback that tmux pulls back.
// Every line is drawn uncut, so a size change alone redraws.
// The wait between polls starts at one second and doubles while nothing changes, up to an eight-second ceiling.
// A redraw returns it to one second, as PATTERN-daemon-wakeups requires.
// A terminal resize ends the wait at once where the platform signals one, through the channel NotifyResize returns.
// A resize is an event and not a timer, so the idle wake rate is unchanged.
//
// Each poll calls Pane.Check before reading the frame; by default it is a selfexec.Watcher's Check.
// A pane therefore re-execs onto a replaced lym binary on its own wait and adds no wake.
// The re-exec keeps the pane, its strand binding and its TMUX_PANE, since only the process image is replaced.
//
// StatusCache.Poll reads a shed run's status file without the run's status lock.
// That is safe because internal/state writes the file atomically.
// A first failure to read keeps the last good status, and a second reports it unreadable.
//
// BuildIdentity names a lym build by its executable's symlink-resolved path, size and modification time.
// A Sidecar records which strand a bootstrap launched from which build.
// StrandCurrent decides whether to keep a strand:
// a live strand is current only when the sidecar names its guid and records the running build.
// A bootstrap therefore replaces a status strand launched by an older build.
//
// The panes' time formats come from internal/durfmt.
// Minutes serve every time still growing, so a pane's text changes at most once a minute.
// Seconds serve a closed span, which never changes again and so costs no redraw.
package statuspane
