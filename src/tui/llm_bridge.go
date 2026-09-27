package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/openai/openai-go/v3"

	"github.com/t3snake/gocode/src/chatcompletion"
	"github.com/t3snake/gocode/src/core"
)

// ----- Bridge between TUI and calls to LLM -----

// ChatResult is a wrapper that wraps the final result from
type ChatResult struct {
	prev_messages []openai.ChatCompletionMessageParamUnion
	out           string
	err           string
	is_err        bool
}

type ChatStream struct {
	llm_msg core.Llm2Tui
}

// Runs agent loop using openai chat completion API
func promptLlm(
	prompt string,
	prev_messages []openai.ChatCompletionMessageParamUnion,
	ctx context.Context,
	tui2llm chan core.Tui2Llm,
	llm2tui chan core.Llm2Tui,
) tea.Cmd {
	// tea.Cmd can only take fn with empty params so return a function with empty params and use closure
	// This function runs as a goroutine (handled by bubbletea)
	// The return is any type, we have to intercept our type in Update function
	return func() tea.Msg {
		var display_out strings.Builder
		var display_err strings.Builder

		client := chatcompletion.GetClient()

		agent_loop_params := chatcompletion.AgentLoopParams{
			Client:     client,
			Ctx:        ctx,
			UserPrompt: prompt,
			Messages:   prev_messages,
			Writers: core.Writers{
				Out: &display_out,
				Err: &display_err,
			},
			LlmToTui: llm2tui,
			TuiToLlm: tui2llm,
		}

		result := chatcompletion.RunAgentLoop(agent_loop_params)

		select {
		case <-ctx.Done():
			return ChatResult{
				prev_messages: result.Messages,
				out:           display_out.String(),
				err:           display_err.String(),
				is_err:        result.Retcode != 0,
			}

		default:
			return ChatResult{
				prev_messages: result.Messages,
				out:           display_out.String(),
				err:           display_err.String(),
				is_err:        result.Retcode != 0,
			}
		}

	}
}

func listenLlmStream(llm2tui chan core.Llm2Tui) tea.Cmd {
	return func() tea.Msg {
		stream_chunk := <-llm2tui

		return ChatStream{
			llm_msg: stream_chunk,
		}
	}
}
