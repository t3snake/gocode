# Decisions Log

This is meant to be a decision log that tries to log all the different decisions made during development and why for reference of future me and other people when they read the code. (maybe even agents?)

## Decisions

### TUI - LLM Communications

- Started with a complete call (without streaming) in which case everything could be done in a single thread/goroutine, but TUI is non responsive and it is nice to see progress while the messages are loading.
- Moved to Channel communications with 2 channels `tui2llm` and `llm2tui` that allows bidirectional communication.
- Initially the channel listener on the TUI side would start listening when user pressed enter and end when that agent loop was done and the user could type the next message. There was complication here with different places the control flow could end and I would start a new goroutine while not closing the old one.
- Moved to an always on listener that only closes when there is a message from Agent loop to TUI and then immediately start back up. Thus I never need to start the listener manually and can only have a single thread all the time dedicated to listening to the reverse stream, mostly blocked waiting for the channel to receive.
- The first version of message history maintained across conversation, I created a wrapper on top of the openai messages, which I used to render TUI and also to recreate the history when sending the user message for next agent loop iteration. This caused many effects:
	- The tool requests happen in the `assistant` message but the actual result is in a seperate `toolcall` message. There is some unnecessary plumbing or pointers to render this in TUI.
	- Recreation of the actual messages is most likely not one to one, thus KV Cache hits wont happen and for local models this means more processing but for frontier models it means way more cost.
	- Generic representations of these openai like messages for eg: having padding means empty content and display are just gaps and lot of specific coding needs to be done to accomodate this message type
- The new idea is to store the message array as is separately. This should be passed back in the ChatResult from Agent loop to TUI. The actual TUI representation will be separate and just accumulate on top. This is optimized for the TUI view.

### TUI

- Initially selection did not work while mouse mode was on.
- I added a listener to disable mouse mode when clicked. This fixed selections in most terminals mostly due to their native handling. But terminals like zed it did not work, then I realized that once mouse mode is off, I dont have a hook to disable it which was the problem.
- I reverted to original but since selection does not work anywhere then, I revert to the second behavior. But ideally need to move to emulating select by natively handling selection and changing the backgrounds for all the text that is selected.

### General

- Settings.json, started from a simple map for JSON marshal and unmarshal, but immediately realized that the settings are finite so there is not much benefit from dynamic and there is too much boiler plate to convert from `any` to specific types.
- Initially did a map so I could easily add dynamic fields without changing types, but cant avoid coupling so a typed struct is just better and faster for now.
- I add this in a `gocode` folder within the user config folder which resolves to `~/.config/gocode/*` in linux, `%APPDATA%/gocode/*` in Windows and `$HOME/Library/Application Support/gocode/*` in Mac. Using `os.UserConfigDir()` to get the base.
- If `gocode` folder is not there it will be created, when trying to store the settings (will happen atleast when the user tries to logon to ChatGPT sub, might change to always at first start up)

### OpenAI - Sign In With ChatGPT (SIWC)

- Implementing SIWC, early and for now made a simple settings.json where I will persist the OAuth tokens for reconnecting on next run.
- **Will** move to a credentials file that has permissions set so that only this app will have access (not sure about read?). Same as private key is saved in `.ssh` directory.
