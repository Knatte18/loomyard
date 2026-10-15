<!-- This is the pack read line of webster's fork prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack in the file layout, fills its one marker, strips this banner and trailing newline;
     the text fills the optional kickstart_pack marker of webster-body-implementer, which ends the prompt, so the read line is the prompt's last line.
     A per-card pack fills that marker from webster-fragment-pack-start-card instead.
     The one marker is pack_path, the batch's pack file, which holds the one-turn read instruction and the pack rendered from webster-fragment-pack-section. -->

## Start here

Now read `{{.pack_path}}` and do what it says.
