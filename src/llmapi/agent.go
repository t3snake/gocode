package llmapi

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/t3snake/gocode/src/core"
)

const ChatCompletionsAPI = "chat_completions"
const ResponsesAPI = "responses"

type SelectedAgentLoopParams struct {
	API               string // ChatCompletionsAPI (also the empty default) or ResponsesAPI.
	Model             string // Required only for Responses; Chat Completions uses core.ModelName.
	Ctx               context.Context
	UserPrompt        string
	Writers           core.Writers
	ChatMessages      []openai.ChatCompletionMessageParamUnion
	ResponsesMessages responses.ResponseInputParam
	LlmToTui          chan core.Llm2Tui
	TuiToLlm          chan core.Tui2Llm
}

type SelectedAgentLoopResult struct {
	Retcode           int
	ChatMessages      []openai.ChatCompletionMessageParamUnion
	ResponsesMessages responses.ResponseInputParam
}

// RunSelectedAgentLoop connects the two entry points with a direct switch.
// It is not called by main or the TUI. Keep the two histories separate and pass
// both result histories back on the next turn. Switching API resumes that API's
// own history; it does not convert the other history. Initialize logger first.
func RunSelectedAgentLoop(params SelectedAgentLoopParams) (result SelectedAgentLoopResult) {
	if params.Ctx == nil {
		params.Ctx = context.Background()
	}
	if params.Writers.Out == nil {
		params.Writers.Out = io.Discard
	}
	if params.Writers.Err == nil {
		params.Writers.Err = io.Discard
	}
	result.ChatMessages = params.ChatMessages
	result.ResponsesMessages = params.ResponsesMessages
	switch params.API {
	case "", ChatCompletionsAPI:
		chat_result := RunAgentLoop(AgentLoopParams{
			Client: GetClient(), Ctx: params.Ctx, UserPrompt: params.UserPrompt,
			Writers: params.Writers, Messages: params.ChatMessages,
			LlmToTui: params.LlmToTui, TuiToLlm: params.TuiToLlm,
		})
		result.Retcode = chat_result.Retcode
		result.ChatMessages = chat_result.Messages
	case ResponsesAPI:
		if strings.TrimSpace(params.Model) == "" {
			fmt.Fprintln(params.Writers.Err, "A Responses model slug is required.")
			result.Retcode = 1
			return
		}
		client, err := GetResponsesClient(params.Ctx)
		if err != nil {
			fmt.Fprintf(params.Writers.Err, "Responses client: %v\n", err)
			result.Retcode = 1
			return
		}
		responses_result := RunResponsesAgentLoop(ResponsesAgentLoopParams{
			Client: client, Model: params.Model, Ctx: params.Ctx, UserPrompt: params.UserPrompt,
			Writers: params.Writers, Messages: params.ResponsesMessages,
			LlmToTui: params.LlmToTui, TuiToLlm: params.TuiToLlm,
		})
		result.Retcode = responses_result.Retcode
		result.ResponsesMessages = responses_result.Messages
	default:
		fmt.Fprintf(params.Writers.Err, "Unknown API: %q\n", params.API)
		result.Retcode = 1
	}
	return
}
