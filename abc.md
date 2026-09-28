# ChatCompletion Module Review

## Summary
The `chatcompletion` module is the core of the agentic loop. It handles the streaming communication with the LLM, manages conversation history, dispatches tool calls, and facilitates real-time interaction between the LLM and the TUI (via channels).

## Detailed Findings

### 1. Strengths
* **Robust Agent Loop**: `RunAgentLoop` correctly manages the complexities of an interactive agent, including handling tool call/result cycles, streaming responses, and context timeouts.
* **Concurrency & Communication**: The use of channels (`LlmToTui`, `TuiToLlm`) to bridge the LLM loop with the TUI allows for a responsive user interface without blocking the core logic.
* **Error Handling**: The implementation is proactive about handling timeouts (both for the stream and the tools) and context cancellations, ensuring the application doesn't hang on failed network calls or long-running tools.
* **Tool Integration**: The integration with the `tools` package is clean. It supports both "standard" and "edit" modes (via `askJevEditNeeded`), giving the user control over the agent's capabilities.
* **Testing**: `llm_test.go` provides good coverage for message construction functions, ensuring that the JSON structure sent to the API is correct.

### 2. Areas for Improvement & Potential Issues

#### A. Implementation of `getSystemPrompt`
Currently, `getSystemPrompt` uses `os/exec` to run shell commands:
```go
cmd := exec.Command("pwd")
// ...
cmd := exec.Command("ls")
```
* **Issue**: This is less efficient and less portable than using Go's native `os` package.
* **Recommendation**: Use `os.Getwd()` instead of `pwd` and `os.ReadDir(".")` instead of `ls`.

#### B. Hardcoded Constraints
* **Message Limit**: The limit of 250 messages is hardcoded: `if msg_len >= 250`. 
* **Tool Truncation**: The logic for truncating tool results is present but relies on a hardcoded limit from `core/config.go`.
* **Recommendation**: These should ideally be part of a configuration struct or environment variables to allow for tuning without recompilation.

#### C. TODOs in Code
There are several outstanding items in the code:
* `// TODO should be blocked until user gives permission` (in tool call loop).
* `// TODO currently hardcoded in core/config.go - Need setting for "on/off" and "when to truncate"`.
* `// TODO add developer prompt, customizable?`.

#### D. Naming Conventions
The module uses `snake_case` for many local variables (e.g., `is_new_session`, `msg_len`, `tool_list`). While the `AGENTS.md` notes that this is acceptable in tests, it deviates from the standard Go `camelCase` convention in the main logic. Since the instruction is to follow the "surrounding code's conventions," and the current code uses `snake_case`, I did not flag this as a bug, but it is worth noting for consistency.

### 3. Security Considerations
* **Command Execution**: The agent has the capability to run terminal commands. While the TUI provides an approval mechanism (via `params.TuiToLlm`), the module allows for "auto-approval" if the TUI is not present (prompt mode). Users should be aware that the LLM can execute arbitrary shell commands.
