package chatcompletion

import (
	// openai api to communicate with LLM
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/t3snake/gocode/src/core"
	"github.com/t3snake/gocode/src/jev"
	"github.com/t3snake/gocode/src/logger"
	"github.com/t3snake/gocode/src/tools"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type AgentLoopParams struct {
	Client     openai.Client                            // Use GetClient() to get client
	Ctx        context.Context                          // Context used for passing user cancellations
	UserPrompt string                                   // The new prompt from the user
	Writers    core.Writers                             // Writers with output and error streams, used differently in prompt and TUI modes
	Messages   []openai.ChatCompletionMessageParamUnion // Previous messages in the session if any
	LlmToTui   chan core.Llm2Tui                        // Channel for communication from this goroutine to the TUI, nil in prompt mode
	TuiToLlm   chan core.Tui2Llm                        // Channel for communication from TUI to this goroutine, nil in prompt mode
}

type AgentLoopResult struct {
	Retcode  int
	Messages []openai.ChatCompletionMessageParamUnion
}

func RunAgentLoop(params AgentLoopParams) (result AgentLoopResult) {
	var err error

	// messages array that maintains chat history
	// TODO add developer prompt, customizable?
	messages := make([]openai.ChatCompletionMessageParamUnion, core.MessageSizeLimit)

	is_new_session := false
	i := 0
	if len(params.Messages) == 0 {
		is_new_session = true
		messages[0] = createDeveloperMessage(getSystemPrompt())
		i++
	} else {
		for ; i < len(params.Messages); i++ {
			messages[i] = params.Messages[i]
		}
	}

	// initialize or append message with given prompt
	messages[i] = createUserMessage(params.UserPrompt)
	msg_len := i + 1

	enable_edit := true
	if is_new_session {
		enable_edit = askJevEditNeeded(params.UserPrompt)
	}

	tool_list := registerTools(enable_edit)

	logger.Info("Starting new LLM agent loop.")
	logger.Infof("Prompt: '%s'", params.UserPrompt)

	for {
		if msg_len >= 250 {
			message := "Message count reached >= 250. Time to increase array size."
			logger.Error(message)
			fmt.Fprintln(params.Writers.Err, message)
			result.Retcode = 1
			result.Messages = messages[:msg_len]
			return
		}

		// Timeout is only for single stream in the agent loop
		// Dont defer cancellation, cancel explicitly to mark it done
		ctx, cancel := context.WithTimeout(params.Ctx, core.StreamTimeout)

		stream := params.Client.Chat.Completions.NewStreaming(ctx,
			openai.ChatCompletionNewParams{
				Model:    core.ModelName,
				Messages: messages[:msg_len],
				Tools:    tool_list,
				StreamOptions: openai.ChatCompletionStreamOptionsParam{
					IncludeObfuscation: openai.Bool(true),
					IncludeUsage:       openai.Bool(true),
				},
			},
			option.WithMaxRetries(2),
		)

		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()

			acc.AddChunk(chunk)

			if len(chunk.Choices) == 0 {
				// NOTE last usage chunk that comes with stream option "include usage". Add to accumulator.
				if params.LlmToTui != nil {
					l2t := initLlm2Tui()
					l2t.IsUsageChunk = true
					l2t.TokenSpent = int(acc.Usage.TotalTokens)

					params.LlmToTui <- l2t
				}
				continue
			}

			// check if streaming just finished with this chunk
			if _, ok := acc.JustFinishedContent(); ok {
				// NOTE seems this is not the last chunk sent, there is one last chunk sent without choices and just Usage data
				if params.LlmToTui != nil {
					l2t := initLlm2Tui()
					l2t.IsChunk = true
					l2t.IsLastChunk = true
					l2t.ChunkContent = chunk.Choices[0].Delta.Content

					params.LlmToTui <- l2t
				}
				continue
			}

			if tool, ok := acc.JustFinishedToolCall(); ok {
				tool_call := fmt.Sprintf("Tool call requested - %s (%s)", tool.Name, tool.Arguments)
				logger.Info(tool_call)
				fmt.Fprintln(params.Writers.Out, tool_call)
			}

			if refusal, ok := acc.JustFinishedRefusal(); ok {
				refusal_out := fmt.Sprintf("Refusal (LLM): %s", refusal)
				fmt.Fprintln(params.Writers.Err, refusal_out)
			}

			if params.LlmToTui != nil {
				l2t := initLlm2Tui()
				l2t.IsChunk = true
				l2t.ChunkContent = chunk.Choices[0].Delta.Content

				params.LlmToTui <- l2t

			} else {
				// print chunk (helpful for prompt mode streaming directly to stdout)
				fmt.Fprintf(params.Writers.Out, "%s", chunk.Choices[0].Delta.Content)
			}
		}

		ctxErr := ctx.Err()
		cancel()

		switch {
		case errors.Is(ctxErr, context.DeadlineExceeded):
			timeout := "stream timed out (> 3 minutes)"
			logger.Error(timeout)
			fmt.Fprintf(params.Writers.Err, "%s\n", timeout)

			result.Messages = messages[:msg_len]
			result.Retcode = 1
			return

		case errors.Is(ctxErr, core.CancelSignalError):
			logger.Errorf("%v", ctxErr)
			fmt.Fprintf(params.Writers.Err, "%v\n", ctxErr)

		}

		if err = stream.Err(); err != nil {
			logger.Errorf("%v", err)
			fmt.Fprintf(params.Writers.Err, "%v\n", err)

			result.Messages = messages[:msg_len]
			result.Retcode = 1
			return
		}

		if len(acc.Choices) == 0 {
			logger.Error("No choices in LLM response.")
			fmt.Fprintln(params.Writers.Err, "Error: No choices in LLM response")

			result.Messages = messages[:msg_len]
			result.Retcode = 1
			return
		}

		choice := acc.Choices[0]

		// always add response to message array with assistant role
		messages[msg_len] = createAssistantMessageFromResponse(choice)
		msg_len++

		results := make([]string, len(choice.Message.ToolCalls))
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) != 0 {
			tool_calls := choice.Message.ToolCalls
			for idx, tool_call := range tool_calls {
				// TODO should be blocked until user gives permission
				if params.LlmToTui != nil {
					l2t := initLlm2Tui()
					l2t.IsToolCall = true
					l2t.ToolName = tool_call.Function.Name
					l2t.ToolParams = tool_call.Function.Arguments
					l2t.ToolId = tool_call.ID

					params.LlmToTui <- l2t
				}

				if params.TuiToLlm != nil {
					user_action := <-params.TuiToLlm

					if !user_action.IsAllowed {
						// TODO send back to llm or return ?
						logger.Info("User did not allow tool call")
						fmt.Fprintf(params.Writers.Err, "User did not allow tool call")

						result.Messages = messages[:msg_len]
						result.Retcode = 1
						return
					}
				}

				tool_ctx, cancel_tool := context.WithTimeout(params.Ctx, core.ToolExecutionTimeout)

				results[idx], err = tools.ExecuteToolCall(tool_call, tool_ctx)

				ctxErr = tool_ctx.Err()
				cancel_tool()

				switch {
				case errors.Is(ctxErr, context.DeadlineExceeded):
					timeout := "tool execution timed out (> 5 minutes)"
					logger.Error(timeout)
					fmt.Fprintf(params.Writers.Err, "%s\n", timeout)

					result.Messages = messages[:msg_len]
					result.Retcode = 1
					return

				case errors.Is(ctxErr, core.CancelSignalError):
					logger.Warningf("Note: execution of tool %s aborted due to interruption", tool_call.Function.Name)
					fmt.Fprintf(params.Writers.Err, "Note: execution of tool %s aborted due to interruption", tool_call.Function.Name)

					result.Messages = messages[:msg_len]
					result.Retcode = 1
					return

				}

				if err != nil {
					logger.Errorf("%v", err)
					fmt.Fprintf(params.Writers.Err, "%s\n", err.Error())

					messages[msg_len] = createToolMessage(tool_call.ID, err.Error())
					msg_len++

					continue
				}

				var tool_result string

				// TODO currently hardcoded in core/config.go - Need setting for "on/off" and "when to truncate"
				trunc_limit := core.ToolResultLogTruncLimit
				if len(results[idx]) < trunc_limit {
					tool_result = results[idx]
				} else {
					tool_result = results[idx][:trunc_limit] + "...(truncated)"
				}

				tool_log := fmt.Sprintf("Tool info\nname: %s\nparams: %s\nresult: %s\n", tool_call.Function.Name, tool_call.Function.Arguments, tool_result)

				logger.Info(tool_log)
				fmt.Fprintf(params.Writers.Err, "===== debug info =====\n%s===== END =====\n", tool_log)

				if params.LlmToTui != nil {
					l2t := initLlm2Tui()
					l2t.IsToolResult = true
					l2t.ToolId = tool_call.ID
					l2t.ToolResult = results[idx]

					params.LlmToTui <- l2t
				}

				messages[msg_len] = createToolMessage(tool_call.ID, results[idx])
				msg_len++
			}

			if params.LlmToTui != nil {
				// signal current loop end, the agent loop is still running
				l2t := initLlm2Tui()
				l2t.IsLoopDone = true

				params.LlmToTui <- l2t
			}

		} else {
			// stream already wrote everything. Agent loop has ended.
			// Note: dont send IsLoopDone, let chatstream handle finishing of the agentLoop
			fmt.Fprintln(params.Writers.Out, "")
			break
		}
	}

	result.Messages = messages[:msg_len]
	result.Retcode = 0
	return
}

func GetClient() openai.Client {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = core.LocalLlmEndpoint
	}

	if apiKey == "" {
		logger.Warning("Env variable OPENROUTER_API_KEY not found, defaulting to empty")
	}
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseUrl),
		option.WithHeader("User-Agent", "GoCode/"+core.Version),
	)

	return client
}

// Register list of tools to be advertised to the LLM
func registerTools(enable_edit bool) []openai.ChatCompletionToolUnionParam {
	if enable_edit {
		return []openai.ChatCompletionToolUnionParam{
			tools.ReadFileRegistration(),
			tools.WriteFileRegistration(),
			tools.RunTerminalCommandRegistration(),
		}
	} else {
		return []openai.ChatCompletionToolUnionParam{
			tools.ReadFileRegistration(),
			tools.RunTerminalCommandRegistration(),
		}
	}
}

// Creates a ChatCompletion message with role "user" and prompt as content
func createUserMessage(prompt string) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfString: openai.String(prompt),
			},
		},
	}
}

// Creates a ChatCompletion message with role "developer" and prompt as content
func createDeveloperMessage(prompt string) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{
			Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
				OfString: openai.String(prompt),
			},
		},
	}
}

// Creates a ChatCompletion message with role "assistant" and prompt_response as content
func createAssistantMessageFromResponse(response openai.ChatCompletionChoice) openai.ChatCompletionMessageParamUnion {
	asst_msg := response.Message.ToAssistantMessageParam()
	return openai.ChatCompletionMessageParamUnion{
		OfAssistant: &asst_msg,
	}
}

// Creates a ChatCompletion message with role "tool" and tool_result as content
func createToolMessage(tool_id, tool_result string) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfTool: &openai.ChatCompletionToolMessageParam{
			ToolCallID: tool_id,
			Content: openai.ChatCompletionToolMessageParamContentUnion{
				OfString: openai.String(tool_result),
			},
		},
	}
}

func getSystemPrompt() string {
	cmd := exec.Command("pwd")

	var out strings.Builder
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "You are a coding assistant"
	}

	agent_md_content := getAgentsMdContent()
	if len(agent_md_content) != 0 {
		agent_md_content = fmt.Sprintf("AGENTS.md contents\n%s", agent_md_content)
	}

	files_in_dir := filesInPwd()
	if len(files_in_dir) != 0 {
		files_in_dir = fmt.Sprintf("Files in current directory\n%s\n", files_in_dir)
	}

	return fmt.Sprintf("%s\nPresent Working Directory: %s\n%s%s",
		"You are a coding assistant/agent",
		out.String(),
		files_in_dir,
		agent_md_content,
	)
}

func getAgentsMdContent() string {
	content, err := os.ReadFile("AGENTS.md")
	if err != nil {
		// assume the file is not there
		return ""
	}

	return string(content[:])
}

func filesInPwd() string {
	cmd := exec.Command("ls")

	var out strings.Builder
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return ""
	}

	return out.String()
}

func initLlm2Tui() core.Llm2Tui {
	return core.Llm2Tui{
		IsToolCall: false,
		ToolName:   "",
		ToolParams: "",

		IsToolResult: false,
		ToolResult:   "",

		ToolId: "",

		IsChunk:      false,
		IsLastChunk:  false,
		ChunkContent: "",

		IsUsageChunk: false,
		TokenSpent:   0,

		IsLoopDone: false,
	}
}

func askJevEditNeeded(prompt string) bool {
	client, err := jev.NewClient()
	if err != nil {
		logger.Errorf("%v", err)
		return true
	}

	questions := make(map[string]jev.Question)

	questions["isEditNeeded"] = jev.Question{
		Type:         jev.Noul,
		Instructions: "Does this prompt in the agent require edit tool capabilities?",
	}

	response, err := client.Evaluate(context.TODO(), prompt, questions)
	if err != nil {
		logger.Errorf("%v", err)
		return true
	}

	ans, ok := response.Answers["isEditNeeded"]
	if !ok {
		logger.Error("isEditNeeded key not present in the answer")
		return true
	}

	if ans.Type != jev.Noul {
		logger.Error("isEditNeeded keyed answer not of type noul (unexpected)")
		return true
	}

	if ans.Noul > 0.6 {
		return true
	} else {
		return false
	}

}
