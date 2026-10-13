// bar.go declares the tmux format strings of reed's two-line status bar and the strand pane-border title.
// The strings are built from tmux 3.6's own default status-format shape (list mode, the #{W:}, #{P:} and #{S:} loops, the < and > markers).
// They read strands through the pane user options @strand and @strand_color and the window user option @lyx_strands, never through the current window.
// They read own windows through the window user options @lyx_own, @strand_color and @lyx_unseen.
// A window carrying the window user option @lyx_hidden still gets no button.
// Every option read here is display only: no Go decision reads @strand_color or @lyx_unseen.

package reedengine

const (
	// strandColorFormat expands to a strand pane's @strand_color, or white when the pane has none.
	strandColorFormat = "#{?#{@strand_color},#{@strand_color},white}"

	// strandWaitFormat expands to " ⏳<label> <minutes>m" while the pane's @lyx_wait is set, and to nothing otherwise.
	// The minutes are the status refresh's own strftime epoch (%s) minus @lyx_wait_start, computed by tmux at each refresh.
	strandWaitFormat = "#{?#{@lyx_wait}, ⏳#{@lyx_wait} #{e|/:#{e|-:%s,#{@lyx_wait_start}},60}m,}"

	// listPrologueFormat opens a left-aligned scrolling list with the < and > markers.
	listPrologueFormat = "#[list=on align=left]#[list=left-marker]<#[list=right-marker]>#[list=on]"

	// statusBarStyle is the base style of the status bar, filling every gap between the buttons: white text on a grey a shade darker than unlitButtonStyle.
	statusBarStyle = "bg=colour235,fg=white"

	// unlitButtonStyle is the style of a button that is not the selected one: white text on dark grey.
	unlitButtonStyle = "bg=colour236 fg=white"

	// viewButtonFormat is the VIEW button, a user range lit while the current window carries @lyx_strands and is not zoomed.
	viewButtonFormat = "#[range=user|view #{?#{&&:#{@lyx_strands},#{!=:#{window_zoomed_flag},1}},list=focus bg=white fg=black bold," + unlitButtonStyle + "}] VIEW #[norange list=on default] "

	// strandButtonFormat is one strand button, a pane range labelled with @strand.
	// It is lit in the strand's color exactly when its pane is the zoomed active pane of its window, and unlit (the color on dark grey) otherwise.
	strandButtonFormat = "#[range=pane|#{pane_id} #{?#{&&:#{window_zoomed_flag},#{pane_active}},list=focus bg=" + strandColorFormat + " fg=black bold,bg=colour236 fg=" + strandColorFormat + "}] #{@strand}" + strandWaitFormat + " #[norange list=on default] "

	// windowButtonFormat is one window button, a window range labelled with the window name and lit while that window is current.
	windowButtonFormat = "#[range=window|#{window_index} #{?window_active,list=focus bg=white fg=black bold," + unlitButtonStyle + "}] #{window_name} #[norange list=on default] "

	// windowWaitFormat expands, for the window the #{W:} loop is on, each of its panes' strandWaitFormat, so every waiting strand of an own window shows on its button.
	windowWaitFormat = "#{P:" + strandWaitFormat + "}"

	// ownWindowButtonFormat is one own-window button, a window range labelled with the window name and its panes' wait marks.
	// It is lit in the window's @strand_color while the window is current; otherwise it is that color on dark grey, underscored while @lyx_unseen is set.
	ownWindowButtonFormat = "#[range=window|#{window_index} #{?window_active,list=focus bg=" + strandColorFormat + " fg=black bold,bg=colour236 fg=" + strandColorFormat + "#{?#{@lyx_unseen}, underscore,}}] #{window_name}" + windowWaitFormat + " #[norange list=on default] "

	// statusFormatButtons is status-format[0]: the VIEW button, then every strand of every window carrying @lyx_strands, in pane order.
	// Then it holds one own-window button per window carrying @lyx_own and not @lyx_hidden, whether or not the session has another window.
	// Then, when the session has more than one window, it holds one button per window carrying none of @lyx_strands, @lyx_own and @lyx_hidden.
	statusFormatButtons = listPrologueFormat + viewButtonFormat +
		"#{W:#{?#{@lyx_strands},#{P:#{?#{@strand}," + strandButtonFormat + ",}},#{?#{&&:#{@lyx_own},#{!=:#{@lyx_hidden},1}}," + ownWindowButtonFormat + ",#{?#{&&:#{>:#{session_windows},1},#{!=:#{@lyx_hidden},1}}," + windowButtonFormat + ",}}}}"

	// unseenClearHookName is the session hook that fires when the client moves to another window.
	unseenClearHookName = "session-window-changed"

	// unseenClearHookCommand clears @lyx_unseen on the window the client moved to, which is the hook's context window.
	unseenClearHookCommand = "set-option -w -u @lyx_unseen"

	// sessionButtonFormat and currentSessionButtonFormat are one session button each, a session range.
	// The current session shows its full name lit, every other session its name cut to 12 characters.
	sessionButtonFormat        = "#[range=session|#{session_id} " + unlitButtonStyle + "] #{=12:session_name} #[norange list=on default] "
	currentSessionButtonFormat = "#[range=session|#{session_id} list=focus bg=yellow fg=black bold] #{session_name} #[norange list=on default] "

	// statusFormatSessions is status-format[1]: one button per session in the #{S:} loop's own session-id order, then the time right-aligned.
	// Each loop body is wrapped in reedSessionFilter, so a session that does not count as reed's gets no button.
	statusFormatSessions = listPrologueFormat + "#{S:#{?" + reedSessionFilter + "," + sessionButtonFormat + ",}," + "#{?" + reedSessionFilter + "," + currentSessionButtonFormat + ",}}#[nolist align=right]%H:%M "

	// paneBorderFormat is the pane-border title: " <@strand> " in the strand's color, bold on the active pane, and " selvage " for a pane without @strand.
	paneBorderFormat = "#{?#{@strand},#[fg=" + strandColorFormat + "]#{?pane_active,#[bold],} #{@strand} #[default],#[default] selvage }"
)
