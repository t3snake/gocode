package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	// openai api to communicate with LLM
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	// bubble tea tui fwk
	tea "charm.land/bubbletea/v2"
)

type chat struct {
	title        string
	prompt       string
	ghost_prompt string
	output       string
}

func initialModel() chat {
	return chat{
		title:        "Go Code by t3snake",
		prompt:       "",
		ghost_prompt: "Type to get started",
		output:       "",
	}
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

func main() {
	var cli_mode bool
	flag.BoolVar(&cli_mode, "cli", false, "Run in cli mode with -p flag")

	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	if cli_mode {
		if prompt == "" {
			panic("Prompt must not be empty")
		}

		client := getClient()

		retcode := runAgentLoop(client, prompt)
		os.Exit(retcode)
	}

	// else TUI mode
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error %v", err)
		os.Exit(1)
	}
}

func (c chat) Init() tea.Cmd {
	return nil
}

func (c chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return c, tea.Quit
		case "enter", "space":
			if len(c.prompt) == 0 {
				return c, nil
			}

			client := getClient()
			runAgentLoop(client, c.prompt)
		}
	}
	return c, nil
}

func (c chat) View() tea.View {
	s := fmt.Sprintf("%s\n\n", c.title)
	if len(c.prompt) == 0 {
		s += fmt.Sprintf("> %s\n\n", c.ghost_prompt)
	} else {
		s += fmt.Sprintf("> %s\n\n", c.prompt)
	}

	s += fmt.Sprintf("Output:\n%s\n\n", c.output)

	return tea.NewView(s)
}

// get tools registered to be advertised to the LLM
func getToolList() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        ReadToolName,
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
		{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        WriteToolName,
					Description: openai.String("Write content to a file"),
					Parameters: openai.FunctionParameters{
						"type": "object",
						"properties": map[string]any{
							"file_path": map[string]any{
								"type":        "string",
								"description": "path of the file to read",
							},
							"content": map[string]any{
								"type":        "string",
								"description": "The content to write to the file",
							},
						},
						"required": []string{"file_path", "content"},
					},
					Strict: openai.Bool(true),
				},
			},
		},
		{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        BashToolName,
					Description: openai.String("Execute a shell command"),
					Parameters: openai.FunctionParameters{
						"type": "object",
						"properties": map[string]any{
							"command": map[string]any{
								"type":        "string",
								"description": "The command to execute",
							},
						},
						"required": []string{"command"},
					},
					Strict: openai.Bool(true),
				},
			},
		},
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

func runAgentLoop(client openai.Client, prompt string) (exitcode int) {
	// messages array that maintains chat history
	messages := make([]openai.ChatCompletionMessageParamUnion, 100)

	msg_len := 1

	// initial message with given prompt
	messages[0] = createUserMessage(prompt)

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Fprintln(os.Stderr, "Logs will appear here!")

	for {
		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    "qwen3.5:9b",
				Messages: messages[:msg_len],
				Tools:    getToolList(),
			},
		)

		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
					fmt.Fprintf(os.Stderr, "%s\n", err.Error())
					continue
				}

				fmt.Fprintf(os.Stderr, "===== debug info: tool info =====\nname: %s\nparams: %s\nresult: %s\n===== END =====\n",
					tool_call.Function.Name, tool_call.Function.Arguments, results[idx])

				messages[msg_len] = createToolMessage(tool_call.ID, results[idx])
				msg_len++
			}
		} else {
			fmt.Println(response_message)
			break
		}
	}

	return 0
}
