package llmapi

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/t3snake/gocode/src/core"
	"github.com/t3snake/gocode/src/logger"
	"github.com/t3snake/gocode/src/openaisiwc"
	"github.com/t3snake/gocode/src/tools"
)

type ResponsesAgentLoopParams struct {
	Client     openai.Client // Use GetResponsesClient before each user turn.
	Model      string        // Required: a model slug available to the signed-in account.
	Ctx        context.Context
	UserPrompt string
	Writers    core.Writers
	Messages   responses.ResponseInputParam
	LlmToTui   chan core.Llm2Tui
	TuiToLlm   chan core.Tui2Llm
}

type ResponsesAgentLoopResult struct {
	Retcode  int
	Messages responses.ResponseInputParam
}

// GetResponsesClient uses settings.json credentials. It does not run inference.
// See https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference.
func GetResponsesClient(ctx context.Context) (openai.Client, error) {
	access_token, err := openaisiwc.GetAccessToken(ctx)
	if err != nil {
		return openai.Client{}, err
	}
	return openai.NewClient(
		option.WithAPIKey(access_token),
		option.WithBaseURL("https://api.openai.com/v1"),
		option.WithHeaderDel("OpenAI-Organization"),
		option.WithHeaderDel("OpenAI-Project"),
		option.WithHeader("User-Agent", "GoCode/"+core.Version),
		option.WithMaxRetries(0),
	), nil
}

// RunResponsesAgentLoop is separate from RunAgentLoop and is not used by main or
// the TUI. Pass result.Messages back on the next turn, including reasoning items.
// As with RunAgentLoop, initialize logger before calling this entry point.
func RunResponsesAgentLoop(params ResponsesAgentLoopParams) (result ResponsesAgentLoopResult) {
	if params.Ctx == nil {
		params.Ctx = context.Background()
	}
	if params.Writers.Out == nil {
		params.Writers.Out = io.Discard
	}
	if params.Writers.Err == nil {
		params.Writers.Err = io.Discard
	}
	messages := slices.Clone(params.Messages)
	result.Retcode = 1
	var loop_err error
	defer func() {
		result.Messages = messages
		if loop_err != nil {
			logger.Errorf("Responses API: %v", loop_err)
			fmt.Fprintf(params.Writers.Err, "Responses API: %v\n", loop_err)
		}
	}()
	if strings.TrimSpace(params.Model) == "" {
		loop_err = fmt.Errorf("a Responses model slug is required")
		return
	}
	if loop_err = params.Ctx.Err(); loop_err != nil {
		return
	}
	if len(messages) >= core.MessageSizeLimit {
		loop_err = fmt.Errorf("message count reached %d", core.MessageSizeLimit)
		return
	}

	is_new_session := len(messages) == 0
	if is_new_session {
		messages = append(messages, responses.ResponseInputItemParamOfMessage(getSystemPrompt(), responses.EasyInputMessageRoleDeveloper))
	}
	messages = append(messages, responses.ResponseInputItemParamOfMessage(params.UserPrompt, responses.EasyInputMessageRoleUser))
	enable_edit := true
	if is_new_session {
		enable_edit = askJevEditNeeded(params.UserPrompt)
	}
	tool_list := registerResponsesTools(enable_edit)

	logger.Info("Starting new Responses agent loop.")
	for {
		if loop_err = params.Ctx.Err(); loop_err != nil {
			return
		}
		if len(messages) >= core.MessageSizeLimit {
			loop_err = fmt.Errorf("message count reached %d", core.MessageSizeLimit)
			return
		}
		stream_ctx, cancel := context.WithTimeout(params.Ctx, core.StreamTimeout)
		stream := params.Client.Responses.NewStreaming(stream_ctx, responses.ResponseNewParams{
			Model:   shared.ResponsesModel(params.Model),
			Input:   responses.ResponseNewParamsInputUnion{OfInputItemList: messages},
			Tools:   tool_list,
			Store:   openai.Bool(false),
			Include: []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
		})
		var response responses.Response
		completed := false
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.output_text.delta", "response.refusal.delta":
				if params.LlmToTui != nil {
					loop_err = sendResponsesEvent(stream_ctx, params.LlmToTui, core.Llm2Tui{IsChunk: true, ChunkContent: event.Delta})
				} else if event.Type == "response.refusal.delta" {
					_, loop_err = fmt.Fprint(params.Writers.Err, event.Delta)
				} else {
					_, loop_err = fmt.Fprint(params.Writers.Out, event.Delta)
				}
			case "response.completed":
				response = event.Response
				completed = true
			case "response.failed":
				loop_err = fmt.Errorf("response failed (%s): %s", event.Response.Error.Code, event.Response.Error.Message)
			case "response.incomplete":
				loop_err = fmt.Errorf("response incomplete: %s", event.Response.IncompleteDetails.Reason)
			case "error":
				loop_err = fmt.Errorf("stream error (%s): %s", event.Code, event.Message)
			}
			if completed || loop_err != nil {
				break
			}
		}
		if loop_err == nil {
			loop_err = stream.Err()
		}
		if loop_err == nil {
			loop_err = stream_ctx.Err()
		}
		stream.Close()
		cancel()
		if loop_err != nil {
			return
		}
		if !completed || len(response.Output) == 0 {
			loop_err = fmt.Errorf("stream ended without a completed response with output")
			return
		}

		// Keep all output, including encrypted reasoning, before the tool results.
		output_items := make(responses.ResponseInputParam, 0, len(response.Output))
		var tool_calls []responses.ResponseFunctionToolCall
		for _, output := range response.Output {
			var input_item responses.ResponseInputItemUnionParam
			if loop_err = input_item.UnmarshalJSON([]byte(output.RawJSON())); loop_err != nil {
				return
			}
			output_items = append(output_items, input_item)
			if output.Type == "function_call" {
				tool_calls = append(tool_calls, output.AsFunctionCall())
			}
		}
		if len(messages)+len(output_items)+len(tool_calls) > core.MessageSizeLimit {
			loop_err = fmt.Errorf("response would exceed the message limit of %d", core.MessageSizeLimit)
			return
		}
		messages = append(messages, output_items...)
		tool_result_start := len(messages)
		// Keep every call paired with a result even if approval or execution stops.
		for _, tool_call := range tool_calls {
			messages = append(messages, createResponsesToolMessage(tool_call, "Tool call was not executed because the turn stopped."))
		}
		if loop_err = sendResponsesEvent(params.Ctx, params.LlmToTui, core.Llm2Tui{IsChunk: true, IsLastChunk: true}); loop_err != nil {
			return
		}
		if loop_err = sendResponsesEvent(params.Ctx, params.LlmToTui, core.Llm2Tui{IsUsageChunk: true, TokenSpent: int(response.Usage.TotalTokens)}); loop_err != nil {
			return
		}
		if len(tool_calls) == 0 {
			fmt.Fprintln(params.Writers.Out)
			result.Retcode = 0
			return
		}

		for idx, tool_call := range tool_calls {
			logger.Infof("Tool call requested - %s (%s)", tool_call.Name, tool_call.Arguments)
			fmt.Fprintf(params.Writers.Out, "Tool call requested - %s (%s)\n", tool_call.Name, tool_call.Arguments)
			if loop_err = sendResponsesEvent(params.Ctx, params.LlmToTui, core.Llm2Tui{
				IsToolCall: true, ToolName: tool_call.Name, ToolParams: tool_call.Arguments, ToolId: tool_call.CallID,
			}); loop_err != nil {
				return
			}
			if params.TuiToLlm != nil {
				select {
				case <-params.Ctx.Done():
					loop_err = params.Ctx.Err()
					return
				case user_action, ok := <-params.TuiToLlm:
					if !ok || !user_action.IsAllowed {
						loop_err = fmt.Errorf("user did not allow tool call")
						messages[tool_result_start+idx] = createResponsesToolMessage(tool_call, loop_err.Error())
						return
					}
				}
			}
			if loop_err = params.Ctx.Err(); loop_err != nil {
				return
			}

			// The existing executor accepts Chat Completions calls. Only its input
			// shape changes here; the tool code and registration stay in tools.go.
			tool_ctx, cancel_tool := context.WithTimeout(params.Ctx, core.ToolExecutionTimeout)
			var tool_result string
			var tool_err error
			if tool_call.Namespace != "gocode" {
				tool_err = fmt.Errorf("unknown tool namespace %q", tool_call.Namespace)
			} else {
				tool_result, tool_err = tools.ExecuteToolCall(openai.ChatCompletionMessageToolCallUnion{
					ID: tool_call.CallID, Type: "function",
					Function: openai.ChatCompletionMessageFunctionToolCallFunction{Name: tool_call.Name, Arguments: tool_call.Arguments},
				}, tool_ctx)
			}
			tool_ctx_err := tool_ctx.Err()
			cancel_tool()
			if tool_ctx_err != nil {
				tool_err = tool_ctx_err
			}
			if tool_err != nil {
				tool_result = fmt.Sprintf("Tool error: %v", tool_err)
				fmt.Fprintln(params.Writers.Err, tool_result)
			}
			messages[tool_result_start+idx] = createResponsesToolMessage(tool_call, tool_result)
			tool_log_result := tool_result
			if len(tool_log_result) > core.ToolResultLogTruncLimit {
				tool_log_result = tool_log_result[:core.ToolResultLogTruncLimit] + "...(truncated)"
			}
			logger.Infof("Tool info\nname: %s\nparams: %s\nresult: %s\n", tool_call.Name, tool_call.Arguments, tool_log_result)
			if loop_err = sendResponsesEvent(params.Ctx, params.LlmToTui, core.Llm2Tui{
				IsToolResult: true, ToolId: tool_call.CallID, ToolResult: tool_result,
			}); loop_err != nil {
				return
			}
			if tool_ctx_err != nil {
				loop_err = tool_ctx_err
				return
			}
		}
		if loop_err = sendResponsesEvent(params.Ctx, params.LlmToTui, core.Llm2Tui{IsLoopDone: true}); loop_err != nil {
			return
		}
	}
}

func registerResponsesTools(enable_edit bool) []responses.ToolUnionParam {
	var tool_list []responses.NamespaceToolToolUnionParam
	for _, registration := range registerTools(enable_edit) {
		function := registration.OfFunction.Function
		parameters := maps.Clone(function.Parameters)
		// Strict schemas require this; leave the Chat Completions schema unchanged.
		parameters["additionalProperties"] = false
		tool_list = append(tool_list, responses.NamespaceToolToolUnionParam{
			OfFunction: &responses.NamespaceToolToolFunctionParam{
				Name: function.Name, Description: function.Description,
				Parameters: parameters, Strict: function.Strict,
			},
		})
	}
	return []responses.ToolUnionParam{{OfNamespace: &responses.NamespaceToolParam{
		Name: "gocode", Description: "Read files, write files, and run terminal commands.", Tools: tool_list,
	}}}
}

func createResponsesToolMessage(tool_call responses.ResponseFunctionToolCall, tool_result string) responses.ResponseInputItemUnionParam {
	message := responses.ResponseInputItemParamOfFunctionCallOutput(tool_result)
	message.OfFunctionCallOutput.CallID = openai.String(tool_call.CallID)
	message.OfFunctionCallOutput.Name = openai.String(tool_call.Name)
	message.OfFunctionCallOutput.Namespace = openai.String(tool_call.Namespace)
	return message
}

func sendResponsesEvent(ctx context.Context, channel chan core.Llm2Tui, message core.Llm2Tui) error {
	if channel == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case channel <- message:
		return nil
	}
}
