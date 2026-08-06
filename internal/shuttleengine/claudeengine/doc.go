// Package claudeengine is the Claude adapter behind shuttleengine.Engine:
// all Claude-specific knowledge — CLI flags, the settings.json hook schema,
// TUI startup/trust markers, and pane key choreography — lives here and
// nowhere else. shuttleengine and the run loop it drives know only the
// Engine interface; a second provider engine would be added alongside this
// package without touching either.
package claudeengine
