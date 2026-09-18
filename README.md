# GoCode

Go Code is an AI coding assistant that uses Large Language Models (LLMs) to
understand code and perform actions through tool calls.

The entry point for your `gocode` implementation is in `src/main.go`.

Originally started as a solution for the challenge ["Build Your own Claude Code"](https://codecrafters.io/challenges/claude-code) by [Codecrafters](https://codecrafters.io).

```
Note: Optimized for local model. Tested with qwen3.5:9b model running through ollama Q4_K_M quantization on a `RTX 3070` with `8GB vram`
```

## Get Started

- Ensure you have `go (1.26)` installed locally.
- Add 

### MacOS and linux

- Run `./start_gocode.sh` to build and run `gocode`, which is implemented in `src/main.go`.

OR

- Run `go run ./src` in the root directory

### Windows

- On Windows, run `start_gocode.bat` to build and run `gocode`.

OR

- Run `go run .\src\` in the root directory.

- Use commandline argument `-p "<your prompt>"` to run agent loop for your prompt.
- Use without params to use the TUI


## Architecture

```mermaid
flowchart TD
    User["User"] --> Main["src/main.go<br/>Read flags and start logging"]

    Main -->|"-p prompt"| CLI["CLI mode<br/>stdout / stderr"]
    Main -->|"No -p flag"| TUI["src/tui<br/>Bubble Tea event loop<br/>Chat display and input"]

    CLI -->|"Prompt"| Agent
    Agent -->|"Streamed text and errors"| CLI
    TUI -->|"Prompt and cancellation context"| Agent
    Agent -->|"Llm2Tui channel<br/>Text, usage, tool requests"| TUI
    TUI -->|"Tui2Llm channel<br/>Tool approval"| Agent

    Agent["src/chatcompletion<br/>RunAgentLoop<br/>Message history for each run"]
    Agent -->|"Messages and tool definitions<br/>OpenAI Go SDK"| LLM["OpenAI-compatible model server<br/>Default: localhost:3434/v1"]
    LLM -->|"Streamed response and tool calls"| Agent

    Agent -->|"Execute tool calls in order"| Tools["src/tools<br/>Parse arguments and dispatch"]
    Tools -->|"Results or errors<br/>Added to next model request"| Agent

    Tools --> Read["read_file"]
    Tools --> Write["write_file"]
    Tools --> Command["run_command_on_terminal"]
    Read --> Files[("Local files")]
    Write --> Files
    Command --> Shell["OS process<br/>Windows: PowerShell<br/>Other systems: Bash"]

    Core["src/core<br/>Shared types, writers,<br/>timeouts and errors"]
    Core -.-> Agent
    Core -.-> TUI

    Main --> Logger["src/logger"]
    Agent --> Logger
    TUI --> Logger
    Logger --> Log[("sessionLog.txt")]
```
