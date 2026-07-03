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

func runAgentLoop(client openai.Client, prompt string, writers Writers) (exitcode int) {
	// messages array that maintains chat history
	messages := make([]openai.ChatCompletionMessageParamUnion, 100)

	msg_len := 1

	// initial message with given prompt
	messages[0] = createUserMessage(prompt)

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Fprintln(writers.err, "Logs will appear here!")

	for {
		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    "qwen3.5:9b",
				Messages: messages[:msg_len],
				Tools:    registerTools(),
			},
		)

		if err != nil {
			fmt.Fprintf(writers.err, "error: %v\n", err)
			error_msg := fmt.Sprintf("error: %v\n", err)

			messages[msg_len] = createUserMessage(error_msg)
			msg_len++
			continue
		}

		if len(resp.Choices) == 0 {
			panic("No choices in response")
		}

		choice := resp.Choices[0] //.Message.Content
		response_message := fmt.Sprint(choice.Message.Content)

		// always add response to message array with assistant role
		messages[msg_len] = createAssistantMessage(choice)
		msg_len++

		results := make([]string, len(choice.Message.ToolCalls))
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) != 0 {
			tool_calls := choice.Message.ToolCalls
			for idx, tool_call := range tool_calls {
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
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
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
