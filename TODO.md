## Todo list

### NOW

- [ ] Finish SIWC (Sign In With Chatgpt) flow
- [ ] Implement Responses API for ChatGPT new model communication
- [ ] Polish tool call display in viewport
- [ ] Add tool call prompt for user

### Very soon

- [ ] Add tokens, context window info
- [ ] /new to clear context (currently always clear context)
- [ ] Add startup settings as json config initially, then move to settings menu
- [ ] TODO tool
- [ ] Web search tool using DDG
- [ ] Add subagent tool
- [ ] Use jev to decide if tool call should be in subagent or not
- [ ] Fix selections - move to native selections (figure out text, figure out background and mouse position etc), cant turn back mouse mode once disabled
- [ ] Add either cancel recovery -> recovers prompt in promptbox on cancellation or better up for history of prompts
- [ ] Save file for sessions
- [ ] Add more themes
- [ ] Start a new process (subagent) without polluting main context for some tasks like readFile

### Later

- [ ] MCP Support
- [ ] Just append to streaming message instead of re-rendering whole viewport
- [ ] Delete specific context from message history - has to be assistant + user message (2 consecutive assistant messages will fail)
- [ ] Chat navigation using arrows/vim bindings
- [ ] Add settings, dialogs ?
- [ ] Add changing log levels during runtime (hide/show tool output option in settings)
- [ ] Help line for all views
- [ ] Status line with - Model, token usage, context size

## Completed list

- [x] Many bug fixes and `AGENTS.md` support and upfront `ls` and `pwd` data.
- [x] **Use tui specific Message types**
- [x] **Add jev mini library**
- [x] Polish some rough edges in UI
- [x] **Add tool call display in TUI (Unpolished)**
- [x] Fix missed bug `tool_call.AsFunction().x` works on the llm raw json, instead use `tool_call.Function.x`
- [x] Fix duplicate response rendering, double user message since history also includes current prompt
- [x] Fix auto scroll even when streaming but empty chunk
- [x] Fix auto scroll down even when not streaming
- [x] Fix display going off on the upcoming prompt after the history change.
- [x] **Send history of chat ie. all agent loop mini session before appending new message.**
- [x] Fix some bugs with keybindings processed by both prompt viewport when typing, only one should process keypresses
- [x] Refactor tui and llm into separate packages
- [x] Refactor tools into separate package (map per toolname to get everything?)
- [x] Fixed bash/pwsh tool errors just printing `exit status 1` now give back stdout, std err and error if happened to the LLM
- [x] Make stream listener to always listen and run new go routine whenever it returns
- [x] Pass user cancel signal to bash tool, stream listener in TUI
- [x] Fix - Deadline of 2/3 minutes is not just single stream but it seems the whole agent loop
- [x] Pass context to bash tool so it can also be cancelled when parents get cancelled
- [x] Fix bash tool calls (simple space splitting) now run bash or pwsh and pass the whole cmd as arg
- [x] Markdown rendering library (Easy picking)
- [x] Fix stuck at thinking ui when token context cutoff happens
- [x] Add tool call logging
- [x] **Add cancel during stream (saves tokens / money)**
- [x] Remove Chat theme (left and right margins) and use industry standard 
- [x] Fix bug - Scrolling support disables text selection and vice versa
- [x] Fix - Auto scroll down Chat message viewport
- [x] Add logging to see what happened behind the scenes
- [x] Fix prompt mode after introducing channels and streaming
- [x] Fix duplicate messages
- [x] **Add streaming messages**
- [x] **Chat flow - Requires communication flow between LLM API and TUI state**
