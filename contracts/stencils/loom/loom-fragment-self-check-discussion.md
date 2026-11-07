<!-- This is the Discussion-Write prompt's self-check step body for a run whose discussion gate has no cold fixer.
     The writer runs the validator itself.
     internal/loomengine's DiscussionTable fills it whole into loom-template-discussion's self_check marker.
     It declares no marker. -->

Before ending your turn, run the mechanical gate standalone against what you just wrote:

```bash
lym loom validate-discussion
```

The verb takes no arguments.
It exits 0 on a clean gate and 1 otherwise, and puts its findings under the failure envelope's `findings` key.
Fix whatever it reports, then re-run it until it exits 0 before ending your turn.
