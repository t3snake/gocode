package main

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
	tests := []struct {
		name    string
		message string
	}{
		{"text", `{"role":"assistant","content":"Hello\n世界"}`},
		{"empty", `{"role":"assistant"}`},
		{"refusal", `{"role":"assistant","refusal":"Cannot complete this request."}`},
		{"tool calls", `{"role":"assistant","tool_calls":[{"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"file_path\":\"a.txt\"}"}},{"id":"call_write","type":"function","function":{"name":"write_file","arguments":"{\"file_path\":\"b.txt\",\"content\":\"hello\"}"}}]}`},
		{"text and tool call", `{"role":"assistant","content":"Reading the file.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response openai.ChatCompletionChoice
			if err := json.Unmarshal([]byte(tt.message), &response.Message); err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal([]byte(tt.message), &want); err != nil {
				t.Fatal(err)
			}
			assertJSON(t, createAssistantMessage(response), want)
		})
	}
}
