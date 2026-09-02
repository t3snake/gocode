## Todo list

### NOW

- [ ] Send history of chat ie. all agent loop mini session before appending new message. (needs map between openai and tui datatype)
- [ ] Add tool call display in viewport
- [ ] Just append to streaming message instead of re-rendering whole viewport

### Very soon

- [ ] Add either cancel recovery -> recovers prompt in promptbox on cancellation or better up for history of prompts
- [ ] Add tool call prompt for user
- [ ] Add tokens, context window info
- [ ] /new to clear context (currently always clear context)
- [ ] Save file for sessions
- [ ] Refactor tui and llm into separate packages
- [ ] Refactor tools into separate package (map per toolname to get everything?)
- [ ] Add more themes

### Later

- [ ] Delete specific context from message history - has to be assistant + user message (2 consecutive assistant messages will fail)
- [ ] Chat navigation using arrows/vim bindings
- [ ] Add settings, dialogs ?
- [ ] Help line for all views
- [ ] Status line with - Model, token usage, context size
- [ ] Start a new process without polluting main context for some tasks like readFile

## Completed list

- [x] Markdown rendering library (Easy picking)
- [x] Fix stuck at thinking ui when token context cutoff happens
- [x] Add tool call logging
- [x] Add cancel during stream (saves tokens / money)
- [x] Remove Chat theme (left and right margins) and use industry standard 
- [x] Fix bug - Scrolling support disables text selection and vice versa
- [x] Fix - Auto scroll down Chat message viewport
- [x] Add logging to see what happened behind the scenes
- [x] Fix prompt mode after introducing channels and streaming
- [x] Fix duplicate messages
- [x] Add streaming messages
- [x] Requires communication flow between LLM API and TUI state
- [x] Chat flow
