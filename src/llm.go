package main

import (
	// openai api to communicate with LLM
	"context"
	"fmt"
	"io"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Writers struct {
	out io.Writer
	err io.Writer
}

func runAgentLoop(client openai.Client, prompt string, writers Writers, llm2tui chan Llm2Tui, tui2llm chan Tui2Llm) (exitcode int) {
	var err error

	// messages array that maintains chat history
	messages := make([]openai.ChatCompletionMessageParamUnion, 100)
	msg_len := 1

	// initial message with given prompt
	messages[0] = createUserMessage(prompt)

	// fmt.Fprintln(writers.err, "Logs will appear here!")
	for {
		stream := client.Chat.Completions.NewStreaming(context.Background(),
			openai.ChatCompletionNewParams{
				Model:         "Qwen3.6-35B-A3B-UD-IQ4_XS.gguf",
				Messages:      messages[:msg_len],
				Tools:         registerTools(),
				StreamOptions: openai.ChatCompletionStreamOptionsParam{},
			},
			option.WithMaxRetries(2),
		)

		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()

			acc.AddChunk(chunk)

			// check if streaming just finished with this chunk
			if content, ok := acc.JustFinishedContent(); ok {
				fmt.Fprintf(writers.out, "%s", content)
				llm2tui <- Llm2Tui{
					is_tool_call: false,
					tool_name:    "",
					params:       "",

					is_chunk:      true,
					is_last:       true,
					chunk_content: chunk.Choices[0].Delta.Content,

					token_spent: int(acc.Usage.TotalTokens),
				}
			}

			if tool, ok := acc.JustFinishedToolCall(); ok {
				fmt.Fprintf(writers.out, "%s - (%s)", tool.Name, tool.Arguments)
			}

			if refusal, ok := acc.JustFinishedRefusal(); ok {
				fmt.Fprintf(writers.err, "Refusal (LLM): %s", refusal)
				return 1
			}

			llm2tui <- Llm2Tui{
				is_tool_call: false,
				tool_name:    "",
				params:       "",

				is_chunk:      true,
				is_last:       false,
				chunk_content: chunk.Choices[0].Delta.Content,

				token_spent: int(acc.Usage.TotalTokens),
			}
		}

		if err := stream.Err(); err != nil {
			fmt.Fprintf(writers.err, "error: %v\n", err)
			return 1
		}

		if len(acc.Choices) == 0 {
			panic("No choices in response")
		}

		choice := acc.Choices[0] //.Message.Content
		response_message := fmt.Sprint(choice.Message.Content)

		// always add response to message array with assistant role
		messages[msg_len] = createAssistantMessage(choice)
		msg_len++

		results := make([]string, len(choice.Message.ToolCalls))
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) != 0 {
			tool_calls := choice.Message.ToolCalls
			for idx, tool_call := range tool_calls {
				// TODO should be blocked until user gives permission
				llm2tui <- Llm2Tui{
					is_tool_call: true,
					tool_name:    tool_call.AsFunction().Function.Name,
					params:       tool_call.AsFunction().Function.Arguments,

					is_chunk:      false,
					is_last:       false,
					chunk_content: "",

					token_spent: int(acc.Usage.TotalTokens),
				}

				user_action := <-tui2llm

				if !user_action.is_allowed {
					// TODO send back to llm or return ?
					fmt.Fprintf(writers.err, "User cancelled tool call.")
					return 1
				}

				results[idx], err = ExecuteToolCall(tool_call)
				if err != nil {
					fmt.Fprintf(writers.err, "%s\n", err.Error())
					continue
				}

				fmt.Fprintf(writers.err, "===== debug info: tool info =====\nname: %s\nparams: %s\nresult: %s\n===== END =====\n",
					tool_call.Function.Name, tool_call.Function.Arguments, results[idx])

				messages[msg_len] = createToolMessage(tool_call.ID, results[idx])
				msg_len++
			}
		} else {
			fmt.Fprintln(writers.out, response_message)
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
