## Todo list

### NOW

- [ ] Send history of chat ie. all agent loop mini session before appending new message. (needs map between openai and tui datatype)
- [ ] Add tool call display in viewport
- [ ] Add tool call log
- [ ] Just append to streaming message instead of re-rendering whole viewport
- [ ] Add more themes

### Very soon

- [ ] Markdown rendering library (Easy picking)
- [ ] Add cancel during stream (saves tokens / money)
- [ ] Add tool call prompt for user
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

- [x] Remove Chat theme (left and right margins) and use industry standard 
- [x] Fix bug - Scrolling support disables text selection and vice versa
- [x] Fix - Auto scroll down Chat message viewport
- [x] Add logging to see what happened behind the scenes
- [x] Fix prompt mode after introducing channels and streaming
- [x] Fix duplicate messages
- [x] Add streaming messages
- [x] Requires communication flow between LLM API and TUI state
- [x] Chat flow
