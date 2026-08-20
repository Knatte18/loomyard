Excellent! All conflicts have been successfully resolved. Let me summarize the resolution:

## Resolution Summary

I've resolved all three conflicting files by combining both sides of the merge conflicts:

### 1. docs/overview.md (lines 235-239, 320-328)
- Combined approach: Acknowledged both the BurlerProducer (THIS branch's contribution) and the Bouncer (parent's contribution)
- Updated the adapter count from the original "three adapters" mentioned in parent's code, treating them as a logical five: SingleLLMProducer, perch, Webster, the burler round producer, and Bouncer
- The description now correctly states both are shipped members of the package

### 2. internal/shedadapters/doc.go (multiple sections)
- Package header: Updated to say "five shedengine.ShedProducer adapters" instead of four, explicitly mentioning both BurlerProducer and Bouncer
- Outcome mapping: Combined both adapter behaviors - kept THIS branch's BurlerProducer behavior and parent's Bouncer behavior in sequence
- Shared cancellation rule: Merged both adapter implementations' cancellation handling (BurlerProducer's archive carve-out and Bouncer's harvested verdict exception)
- Limitations: Combined limitations for both adapters without duplication

### 3. manifest/roadmap.md (Planned and Someday sections)
- Perch -> Shed flattening section: Kept THIS branch's version describing "shedadapters: Burler-round producer" as item 1 in Planned
- loom review-producer items: Updated all three (Discussion-Review, Plan-Review, Webster-Review) to reference both "Planned shedadapters: Burler-round producer" and "shipped Bouncer producer"
- Someday Tenter+Hardener item: Updated to clarify that the Burler-round producer is "being shipped in this task" and Bouncer is already "shipped"

All three files are now staged and ready for merge completion. The resolutions preserve THIS branch's intent (implementing BurlerProducer) while properly acknowledging the parent branch's shipped Bouncer producer.

{"status":"success"}
