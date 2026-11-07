// Package durfmt formats durations for the status panes and the timing report.
// FormatMinutes formats a time still growing in whole minutes, as 12m, 1h05m or 2d03h.
// A pane's text therefore changes at most once a minute.
// FormatSeconds formats a closed span, which never changes again.
// It renders 45s, 12m05s or 1h05m12s below a day and 2d03h from a day on.
// It imports the standard library only.
package durfmt
