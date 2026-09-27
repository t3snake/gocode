package core

import "io"

type Llm2Tui struct {
	// for tool call permission to tui

	IsToolCall bool   // Reports whether LLM is requesting a tool call
	ToolName   string // tool name, only guaranteed if [Llm2Tui.IsToolCall] is true
	ToolParams string // tool params, only guaranteed if [Llm2Tui.IsToolCall] is true
	// TODO(t3snake): parse and make map[string]string

	IsToolResult bool   // Reports result back to TUI for display, storage so next chat can recreate the history
	ToolResult   string // tool result, only guaranteed if [Llm2Tui.IsToolResult] is true

	ToolId string // Identifier from the LLM that links tool call request and result. Non null when either [Llm2Tui.IsToolCall] or [Llm2Tui.IsToolResult] is true

	// stream thinking/content

	IsChunk      bool // Reports whether a chunk was streamed
	IsLastChunk  bool // Reports whether the last chunk was just streamed. Only valid if [Llm2Tui.is_chunk] is true. Not used currently, TODO evaluate
	ChunkContent string

	IsUsageChunk bool // in streaming, the very last chunk when usage is enabled, just sends the token_spent
	TokenSpent   int  // Reports how many tokens were spent so far in the agent loop.

	// Signals to TUI so it can append the current_message to messages and start fresh. Required to maintain Message history as is.
	// This is only sent in the middle of the agent loop and not at the end of the loop
	IsLoopDone bool
}

type Tui2Llm struct {
	// allow or reject?
	IsAllowed        bool   // Reports whether user allowed the tool use, either through always allow or setting allow.
	AdjustmentPrompt string // only used to change course, if [Tui2Llm.is_allowed] is false
}

// Writers Can be writing to stdout/stderr or files as logs
// All printfs are written to logs, but specific logging is only written to log files
// This is helpful in prompt mode on terminal, which usually would not show logs on the terminal
type Writers struct {
	Out io.Writer
	Err io.Writer
}

type Role uint8

const (
	USER Role = iota
	ASSISTANT
	TOOL
	DEVELOPER
)
