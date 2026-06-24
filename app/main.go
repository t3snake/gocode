package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))

	retcode := runAgentLoop(client, prompt)
	os.Exit(retcode)
}

func getToolList() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        "Read",
					Description: openai.String("Read and return contents of a file"),
					Parameters: openai.FunctionParameters{
						"type": "object",
						"properties": map[string]any{
							"file_path": map[string]any{
								"type":        "string",
								"description": "path of the file to read",
							},
						},
						"required": []string{"file_path"},
					},
					Strict: openai.Bool(true),
				},
			},
		},
	}
}

func getUserMessageForPrompt(prompt string) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfString: openai.String(prompt),
			},
		},
	}
}

func getToolMessageForResult(tool_name, tool_result string) openai.ChatCompletionMessageParamUnion {
	return openai.ChatCompletionMessageParamUnion{
		OfTool: &openai.ChatCompletionToolMessageParam{
			ToolCallID: tool_name,
			Content: openai.ChatCompletionToolMessageParamContentUnion{
				OfString: openai.String(tool_result),
			},
		},
	}
}

func runAgentLoop(client openai.Client, prompt string) (exitcode int) {
	// messages array that maintains chat history
	messages := make([]openai.ChatCompletionMessageParamUnion, 100)

	msg_len := 1

	// initial message with given prompt
	messages[0] = getUserMessageForPrompt(prompt)

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Fprintln(os.Stderr, "Logs will appear here!")

	for {
		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    "anthropic/claude-haiku-4.5",
				Messages: messages[:msg_len],
				Tools:    getToolList(),
			},
		)

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			error_msg := fmt.Sprintf("error: %v\n", err)

			messages[msg_len] = getUserMessageForPrompt(error_msg)
			msg_len++
			continue
		}

		if len(resp.Choices) == 0 {
			panic("No choices in response")
		}

		choice := resp.Choices[0] //.Message.Content
		response_message := fmt.Sprint(choice.Message.Content)

		results := make([]string, len(choice.Message.ToolCalls))
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) != 0 {
			tool_calls := choice.Message.ToolCalls
			for idx, tool_call := range tool_calls {
				results[idx], err = ExecuteToolCall(tool_call)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err.Error())
					continue
				}

				fmt.Fprintf(os.Stderr, "debug info: tool result\n", results[idx])

				messages[msg_len] = getToolMessageForResult(tool_call.ID, results[idx])
				msg_len++
			}
		} else {
			fmt.Println(response_message)
			break
		}
	}

	return 0
}
