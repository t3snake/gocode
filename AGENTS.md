# Repository notes

## Build and verification

- The module requires Go 1.26 (`go.mod`); run commands from the repository root.
- Full unit suite: `go test ./...`
- Focus one package: `go test ./src/tools` (also `./src/chatcompletion` or `./src/jev`). Focus one test with `go test ./src/tools -run '^TestToolRegistrations$'`.
- Build without contacting an LLM: `go build -o build/gocode.exe ./src` on Windows, or `mkdir -p build && go build -o build/gocode ./src` elsewhere.
- `start_gocode.bat` creates `build/`; `start_gocode.sh` does not, so a fresh Unix checkout needs `mkdir -p build` first.

## Test style

- In Go tests, use `snake_case` for local variables and test-only fixtures; do not rename API types, functions, or struct fields for this convention.
- For table-driven tests, declare a named `test_cases := []struct { ... }{ ... }` variable first, then loop with `for _, tt := range test_cases`. Do not put the slice literal directly in the `range` expression.

## Runtime wiring

- `src/main.go` is the only executable entrypoint. No `-p` flag starts the Bubble Tea TUI; `-p "..."` runs the same agent loop directly on stdout/stderr.
- Run from the repo root: runtime logs go to the relative, gitignored `sessionLog.txt`, and model-invoked file/terminal tools inherit the process working directory.
- `src/chatcompletion` owns streaming, message history conversion, and sequential tool execution. `src/tui` owns the persistent UI history and communicates with it through the channel types in `src/core/bridge_types.go`.
- Tool schemas and dispatch must stay synchronized in `src/tools/tools.go`; terminal commands use PowerShell on Windows and Bash elsewhere.
- The TUI currently auto-approves every requested tool, and prompt mode has no approval channel. A live model can therefore write files or execute commands without a confirmation prompt.
- The OpenAI-compatible base URL comes from `OPENROUTER_BASE_URL`, defaulting to `http://localhost:3434/v1`; `OPENROUTER_API_KEY` may be empty for local servers. The chat model name is hardcoded in `src/chatcompletion/llm.go`, not selected by an environment variable.
- `src/jev` is a separate TypeSafe API client. It requires `TYPESAFE_API_KEY`; its tests replace HTTP transport and should not make real API calls.

## Change constraints

- Follow the surrounding code's conventions and naming patterns in all files, not just tests. Keep new code consistent with nearby code unless a more specific repository guideline applies.
- Preserve complete assistant tool-call messages followed by matching tool-result messages; the API and TUI history rely on tool-call IDs to reconnect them.
- `build/`, `sessionLog.txt`, and `review-md-files/` are generated/ignored outputs, not source.
