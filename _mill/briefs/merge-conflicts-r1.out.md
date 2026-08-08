Perfect! The conflict has been successfully resolved. 

**Resolution summary:**

The conflict involved two different sections being added to the same location:
- **HEAD**: Attempted to re-introduce Raddle as a separate, dedicated step after Builder with detailed mechanisms
- **origin/main**: Introduced the producer contract model documentation with the "pointer rule"

**Why HEAD was discarded:**
Lines 65-67 (preceding the conflict) already establish that "Raddle folds into Finalize's own contract — not a separate producer, and not a separate step after Webster." HEAD's content directly contradicted this established design decision. It appears to be stale documentation from an earlier draft phase.

**Why origin/main was kept:**
The producer contract model (the pointer rule) is orthogonal to the Raddle-in-Finalize decision and provides essential foundational documentation about how producer contracts work. It naturally follows the Raddle paragraph and precedes the testability section.

The resolved file now maintains internal consistency: Raddle is clearly positioned as folded into Finalize (not separate), and the producer contract rules are properly documented.

All conflicts fixed, file staged.

{"status":"success"}
