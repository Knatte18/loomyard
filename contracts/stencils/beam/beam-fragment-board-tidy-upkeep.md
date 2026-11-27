<!-- This is the board-tidy prompt's upkeep step for the prime's agent, whose worktree is on the main branch.
     internal/dobbyengine's TidyPrompt fills upkeep_call, the agent's call of the upkeep gate key rendered from the hub's gate config,
     and puts the step in the board-tidy stencil's upkeep marker. -->

3. Run the upkeep checks, the tests that need not stop a task, on this worktree:
   `{{.upkeep_call}}`.
   When the call answers `done: false`, run the same command again in a turn that does nothing else.
   File a board note for each failure it reports, through `lym board upsert` with `kind` `note`, after you search the board with `lym board find`:
   a failure that an open note already names gets no second note.
   - A test that names dead code, such as a production function no `main` reaches, gets one note per function it names.
     The note names the function, its file and the test, and asks for the function's removal or its first caller.
   - Any other failing test gets one `bug` note naming the test and the first lines of its failure.
   - A call refused because no test is marked for it has nothing to check and gets no note.
     Any other refused call ran no check, and gets one `bug` note holding the refusal.
   Pick each note's labels with `lym board labels`.
