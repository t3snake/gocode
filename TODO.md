## Todo list

### NOW

- [ ] Add logging to see what happened behind the scenes.
- [ ] Send history of chat ie. all agent loop mini session before appending new message. (needs map between openai and tui datatype)
- [ ] Add tool call prompt for user
- [ ] Add tool call log
- [ ] Auto scroll Chat message viewport

### Very soon

- [ ] Markdown rendering library (Easy picking)
- [ ] /new to clear context (currently always clear context)
- [ ] Save file for sessions

### Later

- [ ] Delete specific context from message history - has to be assistant + user message (2 consecutive assistant messages will fail)
- [ ] Chat navigation using arrows/vim bindings
- [ ] Add settings, dialogs ?
- [ ] Help line for all views
- [ ] Status line with - Model, token usage, context size
- [ ] Start a new process without polluting main context for some tasks like readFile

## Completed list

- [x] Chat flow
- [x] Requires communication flow between LLM API and TUI state
- [x] Fix duplicate messages
- [x] Fix prompt mode after introducing channels and streaming
