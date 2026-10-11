<!-- This is the parent-review keep-alive prompt, typed into the live Discussion-Write session while the parent's review is pending, rendered by ParentReviewKeepAlivePrompt (internal/loomengine/parentreview.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.ping}}, the keep-alive's number, which keeps each ping's text distinct. -->
This is keep-alive number {{.ping}} while the parent's review of your discussion is pending: change nothing, run nothing, and end your turn.
