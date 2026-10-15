// Package durfmt is the duration formats the status panes and the timing report share.
// FormatMinutes is the format of a time still growing, whole minutes as 12m, 1h05m or 2d03h, so a pane's text changes at most once a minute.
// FormatSeconds is the format of a closed span, 45s, 12m05s or 1h05m12s below a day and 2d03h from a day on, which never changes again.
// It imports the standard library only.
package durfmt
