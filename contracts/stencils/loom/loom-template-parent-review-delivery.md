<!-- This is the parent-review delivery prompt, typed into the live Discussion-Write session once its discussion passes the mechanical gate, rendered by ParentReviewDeliveryPrompt (internal/loomengine/parentreview.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its markers are {{.slug}}, {{.request_path}} and {{.reviewer}}, the reviewer's full agent name.
     The SendMessage wording lives here only: shuttleengine and reedengine carry no provider words. -->
Your discussion for {{.slug}} passed the mechanical check and now needs the parent's review: send one message through Claude Code's SendMessage to {{.reviewer}}, naming the task {{.slug}}, giving the request path {{.request_path}}, and asking them to review it with a one-shot fork; then run `lyx loom review delivered {{.slug}}`, or `lyx loom review delivered {{.slug}} --failed "<reason>"` when the send failed, and end your turn without changing the discussion.
