package main

import (
	// openai api to communicate with LLM
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/t3snake/gocode/src/logger"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Can be writing to stdout/stderr or files as logs
// All printfs are written to logs, but specific logging is only written to log files
// This is helpful in prompt mode on terminal, which usually would not show logs on the terminal
type Writers struct {
	out          io.Writer
	err          io.Writer
	suppressLogs bool
}

type Messages struct {
}

func runAgentLoop(client openai.Client, parent_ctx context.Context, prompt string, writers Writers, llm2tui chan Llm2Tui, tui2llm chan Tui2Llm) (exitcode int) {
	var err error

	// TODO does stream take 2 minute for overall stream or for each stream event? could be less
	ctx, cancel := context.WithTimeout(parent_ctx, 2*time.Minute)
	defer cancel()

	// messages array that maintains chat history
	// TODO add developer prompt, customizable?
	messages := make([]openai.ChatCompletionMessageParamUnion, 100)
	msg_len := 1

	// initial message with given prompt
	messages[0] = createUserMessage(prompt)

	logger.Info("Starting new LLM agent loop.")
	logger.Info(fmt.Sprintf("Prompt: '%s'", prompt))

	for {
		if msg_len >= 100 {
			message := "Message count reached >= 100. Time to increase array size."
			logger.Error(message)
			fmt.Println(writers.err, message)
			return 1
		}

		stream := client.Chat.Completions.NewStreaming(ctx,
			openai.ChatCompletionNewParams{
				Model:    "Qwen3.6-35B-A3B-UD-IQ4_XS.gguf",
				Messages: messages[:msg_len],
				Tools:    registerTools(),
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
				if llm2tui != nil {
					llm2tui <- Llm2Tui{
						is_tool_call: false,
						tool_name:    "",
						params:       "",

						is_chunk:        false,
						is_last_content: false,
						chunk_content:   "",

						is_usage_chunk: true,
						token_spent:    int(acc.Usage.TotalTokens),
					}

				}
				continue
			}

			// check if streaming just finished with this chunk
			if _, ok := acc.JustFinishedContent(); ok {
				// NOTE seems this is not the last chunk sent, there is one last chunk sent without choices and just Usage data
				if llm2tui != nil {
					llm2tui <- Llm2Tui{
						is_tool_call: false,
						tool_name:    "",
						params:       "",

						is_chunk:        true,
						is_last_content: true,
						chunk_content:   chunk.Choices[0].Delta.Content,

						is_usage_chunk: false,
						token_spent:    0,
					}
				}
				continue
			}

			if tool, ok := acc.JustFinishedToolCall(); ok {
				tool_call := fmt.Sprintf("Tool call requested - %s (%s)", tool.Name, tool.Arguments)
				logger.Info(tool_call)
				fmt.Fprintln(writers.out, tool_call)
			}

			if refusal, ok := acc.JustFinishedRefusal(); ok {
				refusal_out := fmt.Sprintf("Refusal (LLM): %s", refusal)
				fmt.Fprintln(writers.err, refusal_out)
				return 1
			}

			if llm2tui != nil {
				llm2tui <- Llm2Tui{
					is_tool_call: false,
					tool_name:    "",
					params:       "",

					is_chunk:        true,
					is_last_content: false,
					chunk_content:   chunk.Choices[0].Delta.Content,

					is_usage_chunk: false,
					token_spent:    0,
				}
			} else {
				// print chunk (helpful for non tui streaming)
				fmt.Fprintf(writers.out, "%s", chunk.Choices[0].Delta.Content)
			}
		}

		if err := stream.Err(); err != nil {
			logger.Error(err.Error())
			fmt.Fprintf(writers.err, "error: %v\n", err)
			return 1
		}

		if len(acc.Choices) == 0 {
			logger.Error("No choices in LLM response.")
			fmt.Fprintln(writers.err, "Error: No choices in LLM response")
			return 1
		}

		choice := acc.Choices[0]

		// always add response to message array with assistant role
		messages[msg_len] = createAssistantMessage(choice)
		msg_len++

		results := make([]string, len(choice.Message.ToolCalls))
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) != 0 {
			tool_calls := choice.Message.ToolCalls
			for idx, tool_call := range tool_calls {
				// TODO should be blocked until user gives permission
				if llm2tui != nil {
					llm2tui <- Llm2Tui{
						is_tool_call: true,
						tool_name:    tool_call.AsFunction().Function.Name,
						params:       tool_call.AsFunction().Function.Arguments,

						is_chunk:        false,
						is_last_content: false,
						chunk_content:   "",

						token_spent: int(acc.Usage.TotalTokens),
					}
				}

				if tui2llm != nil {
					user_action := <-tui2llm

					if !user_action.is_allowed {
						// TODO send back to llm or return ?
						logger.Info("User did not allow tool call")
						fmt.Fprintf(writers.err, "User did not allow tool call")
						return 1
					}
				}

				results[idx], err = ExecuteToolCall(tool_call)
				if err != nil {
					err_msg := fmt.Sprintf("Error during tool call: %s", err.Error())
					logger.Error(err_msg)
					fmt.Fprintf(writers.err, "%s\n", err_msg)

					messages[msg_len] = createToolMessage(tool_call.ID, err_msg)
					msg_len++

					continue
				}

				var tool_result string

				// TODO currently hardcoded truncation of tool result to 300 characters? Need setting for "on/off" and "when to truncate"
				trunc_limit := 300
				if len(results[idx]) < trunc_limit {
					tool_result = results[idx]
				} else {
					tool_result = results[idx][:trunc_limit] + "...(truncated)"
				}

				tool_log := fmt.Sprintf("Tool info\nname: %s\nparams: %s\nresult: %s\n", tool_call.Function.Name, tool_call.Function.Arguments, tool_result)

				logger.Info(tool_log)
				fmt.Fprintf(writers.err, "===== debug info =====\n%s===== END =====\n", tool_log)

				messages[msg_len] = createToolMessage(tool_call.ID, results[idx])
				msg_len++
			}
		} else {
			// stream already wrote everything.
			if llm2tui != nil {
				// send stop listening signal
				llm2tui <- Llm2Tui{
					is_tool_call: false,
					tool_name:    "",
					params:       "",

					is_chunk:        false,
					is_last_content: false,
					chunk_content:   "",

					is_usage_chunk: false,
					token_spent:    0,

					should_stop_listening: true,
				}

			}
			fmt.Fprintln(writers.out, "")
			break
		}
	}

	return 0
}

func getClient() openai.Client {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "http://localhost:3434/v1"
	}

	if apiKey == "" {
		apiKey = ""
		// panic("Env variable OPENROUTER_API_KEY not found")
	}
	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))

	return client
}

// Register list of tools to be advertised to the LLM
func registerTools() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		readFileRegistration(),
		writeFileRegistration(),
		runBashRegistration(),
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
func createAssistantMessage(response openai.ChatCompletionChoice) openai.ChatCompletionMessageParamUnion {
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
