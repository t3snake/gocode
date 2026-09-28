package chatcompletion

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/openai/openai-go/v3"
)

// Compare the JSON sent to the API, including role defaults and empty content.
func assertJSON(t *testing.T, value any, want any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JSON = %s, want %#v", data, want)
	}
}

func TestCreatePromptMessages(t *testing.T) {
	builders := []struct {
		role  string
		build func(string) openai.ChatCompletionMessageParamUnion
	}{
		{"user", createUserMessage},
		{"developer", createDeveloperMessage},
	}
	inputs := []struct {
		name string
		text string
	}{
		{"plain", "Explain this code."},
		{"empty", ""},
		{"whitespace", " \t\n "},
		{"unicode and escapes", "Hello 世界 👋\n\"quoted\" \\path\tend"},
	}
	for _, builder := range builders {
		for _, input := range inputs {
			t.Run(builder.role+"/"+input.name, func(t *testing.T) {
				assertJSON(t, builder.build(input.text), map[string]any{
					"role": builder.role, "content": input.text,
				})
			})
		}
	}
}

func TestCreateToolMessage(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		result string
	}{
		{"result", "call_123", "file contents\n世界"},
		{"empty result", "call_empty", ""},
		{"error result", "call_failed", "Error: file not found\n"},
		{"literal arguments", "call_\"\\", " \t{\"ok\":true}\n "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSON(t, createToolMessage(tt.id, tt.result), map[string]any{
				"role": "tool", "tool_call_id": tt.id, "content": tt.result,
			})
		})
	}
}

func TestCreateAssistantMessage(t *testing.T) {
	read_call := openai.ChatCompletionMessageToolCallUnion{
		ID:   "call_read",
		Type: "function",
		Function: openai.ChatCompletionMessageFunctionToolCallFunction{
			Name:      "read_file",
			Arguments: `{"file_path":"a.txt"}`,
		},
	}
	write_call := openai.ChatCompletionMessageToolCallUnion{
		ID:   "call_write",
		Type: "function",
		Function: openai.ChatCompletionMessageFunctionToolCallFunction{
			Name:      "write_file",
			Arguments: `{"file_path":"b.txt","content":"hello"}`,
		},
	}

	test_cases := []struct {
		name     string
		response openai.ChatCompletionMessage
		expected map[string]any
	}{
		{
			name:     "text",
			response: openai.ChatCompletionMessage{Content: "Hello\n世界"},
			expected: map[string]any{"role": "assistant", "content": "Hello\n世界"},
		},
		{
			name:     "empty",
			response: openai.ChatCompletionMessage{},
			expected: map[string]any{"role": "assistant"},
		},
		{
			name:     "refusal",
			response: openai.ChatCompletionMessage{Refusal: "Cannot complete this request."},
			expected: map[string]any{
				"role": "assistant", "refusal": "Cannot complete this request.",
			},
		},
		{
			name:     "tool calls",
			response: openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{read_call, write_call}},
			expected: map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id": "call_read", "type": "function",
						"function": map[string]any{"name": "read_file", "arguments": `{"file_path":"a.txt"}`},
					},
					map[string]any{
						"id": "call_write", "type": "function",
						"function": map[string]any{"name": "write_file", "arguments": `{"file_path":"b.txt","content":"hello"}`},
					},
				},
			},
		},
		{
			name: "text and tool call",
			response: openai.ChatCompletionMessage{
				Content: "Reading the file.",
				ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
					ID: "call_1", Type: "function",
					Function: openai.ChatCompletionMessageFunctionToolCallFunction{
						Name: "read_file", Arguments: "{}",
					},
				}},
			},
			expected: map[string]any{
				"role": "assistant", "content": "Reading the file.",
				"tool_calls": []any{map[string]any{
					"id": "call_1", "type": "function",
					"function": map[string]any{"name": "read_file", "arguments": "{}"},
				}},
			},
		},
	}
	for _, tt := range test_cases {
		t.Run(tt.name, func(t *testing.T) {
			response := openai.ChatCompletionChoice{Message: tt.response}
			assertJSON(t, createAssistantMessageFromResponse(response), tt.expected)
		})
	}
}
