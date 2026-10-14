<!-- This is the guard-fix line of webster's card-gate list in a fork or recovery prompt.
     guardNotice (guardstep.go) reads it through stencilstore, fills its two markers, strips this banner and trailing newline,
     and appends the text after the guard-scan-failure line when the file the scan failed on lies in a package a card targets.
     guard_file is that file's path, and card_path is the pointer of the first card targeting its package. -->

`{{.guard_file}}` lies in a package the card `{{.card_path}}` targets, so fix that file under that card's commit.
