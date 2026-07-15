# Scripting & Automation

tmux supports scripting and automation commands. On Windows, tmux (the Windows port of tmux) is command-compatible.

## Window & Pane Control

```powershell
# Create a new window
tmux new-window

# Split panes
tmux split-window -v          # Split vertically (top/bottom)
tmux split-window -h          # Split horizontally (side by side)

# Navigate panes
tmux select-pane -U           # Select pane above
tmux select-pane -D           # Select pane below
tmux select-pane -L           # Select pane to the left
tmux select-pane -R           # Select pane to the right

# Navigate windows
tmux select-window -t 1       # Select window by index (default base-index is 1)
tmux next-window              # Go to next window
tmux previous-window          # Go to previous window
tmux last-window              # Go to last active window

# Kill panes and windows
tmux kill-pane
tmux kill-window
tmux kill-session
```

## Sending Keys

```powershell
# Send text directly
tmux send-keys "ls -la" Enter

# Send keys literally (no parsing)
tmux send-keys -l "literal text"

# Paste mode (legacy compatibility)
tmux send-keys -p

# Repeat a key N times
tmux send-keys -N 5 Up

# Send copy mode command
tmux send-keys -X copy-mode-up

# Special keys supported:
# Enter, Tab, Escape, Space, Backspace
# Up, Down, Left, Right, Home, End
# PageUp, PageDown, Delete, Insert
# F1-F12, C-a through C-z (Ctrl+key)
```

## Pane Information

```powershell
# List all panes in current window
tmux list-panes

# List all windows
tmux list-windows

# Capture pane content
tmux capture-pane

# Display formatted message with variables
tmux display-message "#S:#I:#W"   # Session:Window Index:Window Name
```

## Paste Buffers

```powershell
# Set paste buffer content
tmux set-buffer "text to paste"

# Paste buffer to active pane
tmux paste-buffer

# List all buffers
tmux list-buffers

# Show buffer content
tmux show-buffer

# Delete buffer
tmux delete-buffer

# Interactive buffer chooser (enter=paste, d=delete, esc=close)
tmux choose-buffer

# Named buffers (separate from anonymous stack)
tmux set-buffer -b mydata "key=value"
tmux show-buffer -b mydata
tmux paste-buffer -b mydata
tmux delete-buffer -b mydata

# Clear command prompt history
tmux clear-prompt-history
```

## Pane Layout

```powershell
# Resize panes
tmux resize-pane -U 5         # Resize up by 5
tmux resize-pane -D 5         # Resize down by 5
tmux resize-pane -L 10        # Resize left by 10
tmux resize-pane -R 10        # Resize right by 10

# Swap panes
tmux swap-pane -U             # Swap with pane above
tmux swap-pane -D             # Swap with pane below

# Rotate panes in window
tmux rotate-window

# Toggle pane zoom
tmux zoom-pane
```

## Pane Titles

Programs running inside a pane can set the title via OSC escape sequences. PowerShell 7 does this automatically with the current working directory. (Companion documentation `pane-titles.md` not available in this repository.)

```powershell
# Set a title on the active pane
tmux select-pane -T "my build pane"

# Set pane title on a specific pane
tmux select-pane -t %3 -T "logs"

# Set per-pane style (foreground/background color override)
tmux select-pane -P "bg=default,fg=blue"

# Display pane title using format variables
tmux display-message "#{pane_title}"
```

Enable `pane-border-format` and `pane-border-status` in your config to see titles on pane borders:

```tmux
set -g pane-border-status top
set -g pane-border-format " #{pane_index}: #{pane_title} "
```

## Popups

```powershell
# Open a popup running a command
tmux display-popup "Get-Process"

# Set width and height (absolute or percentage)
tmux display-popup -w 80% -h 50% "htop"

# Set the starting directory
tmux display-popup -d "C:\Projects" -w 100 -h 30

# Close popup on command exit (default behavior, -E inverts it)
tmux display-popup -E "git log --oneline -20"

# Keep popup open after command finishes
tmux display-popup -K "echo done"
```

## Menus

```powershell
# Display an interactive menu
# Format: display-menu [-x x] [-y y] [-T title] name key command ...
tmux display-menu -T "Actions" \
  "New Window" n "new-window" \
  "Split Horizontal" h "split-window -h" \
  "Split Vertical" v "split-window -v" \
  "Close Pane" x "kill-pane"

# Position the menu at specific coordinates
tmux display-menu -x 10 -y 5 -T "Quick" \
  "Zoom" z "resize-pane -Z" \
  "Rename" r "command-prompt -I '#W' 'rename-window %%'"
```

## Session Management

```powershell
# Check if session exists (exit code 0 = exists)
tmux has-session -t mysession

# Rename session
tmux rename-session newname

# Switch to another session
tmux switch-client -t other-session

# Cycle through sessions
tmux switch-client -n          # Next session
tmux switch-client -p          # Previous session
tmux switch-client -l          # Last (most recently used) session

# Create a session with environment variables
tmux new-session -s work -e "MY_VAR=value"

# Respawn pane (restart shell, or restart with a different command)
tmux respawn-pane
tmux respawn-pane -k           # Kill the current process first
tmux respawn-pane -c /tmp      # Restart in a different directory
```

## Pane Reorganization

```powershell
# Break the current pane out into a new window
tmux break-pane

# Break a specific pane, keep it in background
tmux break-pane -d -s %3

# Join a pane from another window into the current window
tmux join-pane -s :2           # Bring pane from window 2

# Join horizontally or vertically
tmux join-pane -h -s :2        # Join side by side
tmux join-pane -v -s :3        # Join top/bottom

# Move a pane (same as join-pane)
tmux move-pane -s %5 -t %3

# Find a window by name or content
tmux find-window "search term"
```

## Environment Variables

```powershell
# Set a global env var (inherited by all new panes)
tmux set-environment -g EDITOR vim

# Set a session-scoped env var
tmux set-environment MY_VAR value

# Unset a global env var
tmux set-environment -gu MY_VAR

# Show all environment variables
tmux show-environment
tmux show-environment -g
```

## Format Variables

The `display-message` command supports 140+ variables. Common ones include:

| Variable | Description |
|----------|-------------|
| `#S` | Session name |
| `#I` | Window index |
| `#W` | Window name |
| `#P` | Pane ID |
| `#T` | Pane title |
| `#H` | Hostname |
| `#{pane_current_path}` | Current working directory of the pane |
| `#{pane_current_command}` | Foreground process name |
| `#{pane_pid}` | PID of the pane's shell |
| `#{pane_width}` | Width of the pane in columns |
| `#{pane_height}` | Height of the pane in rows |
| `#{pane_active}` | `1` if this pane is the active pane |
| `#{pane_index}` | Pane index within the window |
| `#{window_zoomed_flag}` | `1` if the window has a zoomed pane |
| `#{window_panes}` | Number of panes in the window |
| `#{window_active}` | `1` if this is the active window |
| `#{session_windows}` | Number of windows in the session |
| `#{session_attached}` | Number of clients attached to the session |
| `#{client_prefix}` | `1` if the prefix key was pressed |
| `#{client_width}` | Width of the client terminal |
| `#{client_height}` | Height of the client terminal |

### Format Modifiers

```powershell
# Conditional
tmux display-message -p "#{?window_zoomed_flag,ZOOMED,normal}"

# Comparison
tmux display-message -p "#{==:#{pane_index},0}"

# Regex substitution
tmux display-message -p "#{s/old/new/:pane_title}"

# Basename and dirname
tmux display-message -p "#{b:pane_current_path}"
tmux display-message -p "#{d:pane_current_path}"

# Loop over all windows
tmux display-message -p "#{W:#{window_index}:#{window_name} }"

# Loop over all panes
tmux display-message -p "#{P:#{pane_index} }"
```

## Advanced Commands

```powershell
# Discover supported commands
tmux list-commands

# Server/session management
tmux kill-server
tmux list-clients
tmux switch-client -t other-session

# Config at runtime
tmux source-file ~/.tmux.conf
tmux show-options
tmux set-option -g status-left "[#S]"

# Layout/history/stream control
tmux next-layout
tmux previous-layout
tmux select-layout tiled         # Apply a specific layout
tmux clear-history
tmux pipe-pane -o "cat > pane.log"

# Hooks (event callbacks) - see Hooks section below for full reference
tmux set-hook -g after-new-window "display-message created"
tmux set-hook -g client-attached "run-shell 'echo attached'"
tmux set-hook -gu after-new-window     # Unset (remove) a hook
tmux show-hooks

# Run shell commands
tmux run-shell "echo hello"           # Output shown in status bar
tmux run-shell -b "long-running.ps1"  # Fire-and-forget (background)

# Conditional execution
tmux if-shell "test -f ~/.tmux.conf" "source-file ~/.tmux.conf"
tmux if-shell -F "#{window_zoomed_flag}" "" "resize-pane -Z"

# User confirmation dialogs
tmux confirm-before -p "Kill this pane? (y/n)" kill-pane

# Wait channels for cross-pane synchronization
tmux wait-for -L mychannel             # Lock a channel
tmux wait-for -S mychannel             # Signal (unlock) a channel
tmux wait-for mychannel                # Wait until channel is signaled
```

## Hooks (Event Callbacks)

Hooks let you run commands automatically when events occur. They are one of the most powerful scripting features in tmux.

### Setting Hooks

```powershell
# Global hook (applies to all sessions)
tmux set-hook -g after-new-window "display-message 'New window created'"

# Session-scoped hook
tmux set-hook after-split-window "select-layout tiled"

# Chain multiple commands in a hook
tmux set-hook -g after-new-session "set -g status-left '[#S] ' \; display-message 'Session ready'"
```

### Available Hook Events

| Hook | Fires when... |
|------|---------------|
| `after-new-session` | A new session is created |
| `after-new-window` | A new window is created |
| `after-split-window` | A pane is split |
| `client-attached` | A client attaches to a session |
| `client-detached` | A client detaches from a session |
| `after-select-window` | A different window is selected |
| `after-select-pane` | A different pane is selected |
| `after-resize-pane` | A pane is resized |
| `pane-died` | A pane's process exits |
| `alert-activity` | Activity detected in a monitored window |
| `alert-silence` | Silence detected in a monitored window |
| `alert-bell` | Bell received from a pane |
| `after-kill-pane` | A pane is killed |

### Removing Hooks

```powershell
# Remove a global hook
tmux set-hook -gu after-new-window

# View all active hooks
tmux show-hooks
```

**Important:** If you repeatedly call `set-hook -g` for the same event, tmux appends duplicate entries. Use `set-hook -gu` to clear the old hook before setting a new one, or check `show-hooks` to verify no duplicates.

## Display Panes

Show numbered overlays on all panes, then type a number to jump to that pane:

```powershell
# Show pane number overlay (also: Prefix + q)
tmux display-panes
```

The overlay shows each pane's number according to `pane-base-index`. Press a number key while the overlay is visible to switch to that pane. The overlay auto-dismisses after `display-panes-time` milliseconds.

## Run Shell

Run an external command and display the output:

```powershell
# Output appears in the status bar message area
tmux run-shell "echo hello"

# Run in background (fire-and-forget, no output displayed)
tmux run-shell -b "long-running-script.ps1"

# Use format variables in shell commands
tmux run-shell "echo 'Current pane: #{pane_index}'"
```

## Interactive Choosers

```powershell
# Interactive session/window/pane tree browser
tmux choose-tree

# Show only sessions
tmux choose-tree -s

# Show only windows
tmux choose-tree -w

# Interactive buffer picker (enter=paste, d=delete)
tmux choose-buffer

# Interactive client picker
tmux choose-client

# Interactive options editor
tmux customize-mode
```

## Target Syntax (`-t`)

tmux supports tmux-style targets:

```powershell
# Window by index in session
tmux select-window -t work:2

# Window by name in session
tmux select-window -t work:editor

# Specific pane by index
tmux send-keys -t work:2.1 "echo hi" Enter

# Pane by pane id
tmux send-keys -t %3 "pwd" Enter

# Window by window id
tmux select-window -t @4

# Target a specific session
tmux has-session -t mysession

# Session:window.pane full path
tmux send-keys -t dev:0.2 "make build" Enter
```

## Server Namespaces (`-L`)

Use `-L` to run multiple isolated tmux servers on the same machine:

```powershell
# Start a session in a named server namespace
tmux -L work new-session -s dev

# Attach to a session in that namespace
tmux -L work attach -t dev

# Each namespace gets its own server, sessions, and socket
tmux -L personal new-session -s play
```

## Key Binding Management

```powershell
# Bind a key in the default prefix table
tmux bind-key h split-window -h

# Bind with format variable expansion (-F flag)
tmux bind-key -F M-h "resize-pane -L #{pane_width}"

# Bind with repeat (successive presses within repeat-time don't need prefix)
tmux bind-key -r Left select-pane -L
tmux bind-key -r Right select-pane -R

# Bind in root table (no prefix needed)
tmux bind-key -n M-Left select-pane -L

# Bind in a specific key table
tmux bind-key -T copy-mode-vi y send-keys -X copy-selection

# Unbind a single key
tmux unbind-key h

# Unbind ALL keys (reset to clean slate)
tmux unbind-key -a

# Unbind all keys in a specific key table only
tmux unbind-key -a -T copy-mode-vi
tmux unbind-key -a -T prefix
tmux unbind-key -a -T root
tmux unbind-key -a -T copy-mode
```

## Command Chaining

Chain multiple commands with `\;` in config files:

```tmux
# Split and select in one binding
bind-key M-v split-window -v \; select-pane -U

# Create a 3-pane layout
bind-key M-d split-window -h \; split-window -v \; select-pane -t 0

# Conditional chaining
bind-key M-z if-shell -F "#{window_zoomed_flag}" "resize-pane -Z" ""
```

From the CLI, use `\;` or quote the command:

```powershell
tmux split-window -h `; select-pane -L
```

## Querying Lists with Custom Formats

```powershell
# List all sessions with custom format
tmux list-sessions -F "#{session_name}:#{session_windows}"

# List all windows with custom format
tmux list-windows -F "#{window_index}:#{window_name}:#{window_panes}"

# List all panes across the session (-s flag)
tmux list-panes -s -F "#{window_index}.#{pane_index}: #{pane_current_command} [#{pane_width}x#{pane_height}]"

# List all panes across all sessions (-a flag)
tmux list-panes -a

# Capture pane content to stdout
tmux capture-pane -p -t %0

# Capture with line range (negative = scrollback)
tmux capture-pane -p -S -100 -E -1

# Print a format variable
tmux display-message -p "#{pane_current_path}"
```

## Window and Pane Creation Options

### new-window

```powershell
# Create a window with a name
tmux new-window -n "logs"

# Create a window in the background (don't switch to it)
tmux new-window -d -n "background"

# Create a window in a specific directory
tmux new-window -c "C:\Projects\myapp"

# Create a window running a command
tmux new-window -n "build" -- cargo watch

# Create a window at a specific index
tmux new-window -t 5
```

When you set a window name with `-n`, automatic renaming is disabled for that window so the foreground process name does not overwrite your chosen name.

### split-window

```powershell
# Split with percentage size
tmux split-window -v -p 30            # Bottom pane gets 30%
tmux split-window -h -p 70            # Right pane gets 70%

# Split in the current pane's directory
tmux split-window -h -c "#{pane_current_path}"

# Split with a specific command
tmux split-window -v -- python

# Split a specific target pane
tmux split-window -v -t %3

# Split without switching focus
tmux split-window -d -v
```

### new-session

```powershell
# Create a named session
tmux new-session -s work

# Create in a specific directory
tmux new-session -s project -c "C:\Projects\myapp"

# Create with environment variables
tmux new-session -s dev -e "NODE_ENV=development"

# Create in background (detached)
tmux new-session -d -s background

# Create with an initial command
tmux new-session -s monitor -- htop

# Create a session with a named first window
tmux new-session -s work -n "editor"
```

## Target Syntax

Many commands accept a `-t` flag to specify which session, window, or pane to act on:

```powershell
# Target a session by name
tmux switch-client -t mysession

# Target a window by index (within current session)
tmux select-window -t 3

# Target a window in a specific session
tmux select-window -t mysession:2

# Target a pane by ID (absolute, shown with %)
tmux select-pane -t %5

# Target a pane within a window
tmux select-pane -t :2.1             # Window 2, pane 1

# Special targets
tmux select-pane -t +               # Next pane
tmux select-pane -t -               # Previous pane
tmux select-window -t !             # Last (previous) window
```

## Server Namespaces

Run isolated tmux instances using the `-L` flag. Each namespace gets its own server process with its own sessions:

```powershell
# Start a session in a named namespace
tmux -L work new-session -s dev

# Attach to a session in that namespace
tmux -L work attach

# List sessions in a namespace
tmux -L work list-sessions

# Default namespace is used when -L is not specified
```

This is useful for running completely separate tmux environments, for example one for development and one for monitoring.