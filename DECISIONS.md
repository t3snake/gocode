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
