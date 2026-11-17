#!/bin/sh
exec env CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1 /home/knatte/.local/bin/claude "$@"
