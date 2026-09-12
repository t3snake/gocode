package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestToolRegistrations(t *testing.T) {
	tests := []struct {
		name     string
		register func() openai.ChatCompletionToolUnionParam
		required []string
	}{
		{"read_file", readFileRegistration, []string{"file_path"}},
		{"write_file", writeFileRegistration, []string{"file_path", "content"}},
		{"run_command_on_terminal", runBashRegistration, []string{"command"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := tt.register()
			if tool.OfFunction == nil {
				t.Fatal("registration has no function")
			}
			fn := tool.OfFunction.Function
			if fn.Name != tt.name {
				t.Errorf("name = %q, want %q", fn.Name, tt.name)
			}
			if fn.Description.Value == "" {
				t.Error("description is empty")
			}
			if !fn.Strict.Value {
				t.Error("strict mode is disabled")
			}
			if fn.Parameters["type"] != "object" {
				t.Errorf("parameter type = %v, want object", fn.Parameters["type"])
			}
			if !reflect.DeepEqual(fn.Parameters["required"], tt.required) {
				t.Errorf("required = %v, want %v", fn.Parameters["required"], tt.required)
			}
			properties, ok := fn.Parameters["properties"].(map[string]any)
			if !ok {
				t.Fatal("properties is not an object")
			}
			if len(properties) != len(tt.required) {
				t.Errorf("property count = %d, want %d", len(properties), len(tt.required))
			}
			for _, name := range tt.required {
				property, ok := properties[name].(map[string]any)
				if !ok || property["type"] != "string" {
					t.Errorf("property %q = %v, want a string parameter", name, properties[name])
				}
			}
		})
	}
}

func TestRegisterTools(t *testing.T) {
	want := map[string]bool{"read_file": true, "write_file": true, "run_command_on_terminal": true}
	for _, tool := range registerTools() {
		if tool.OfFunction == nil {
			t.Fatal("registered tool has no function")
		}
		name := tool.OfFunction.Function.Name
		if !want[name] {
			t.Errorf("unexpected or duplicate tool %q", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing tools: %v", want)
	}
}

func TestExecuteToolCallInvalidArguments(t *testing.T) {
	// Every case must return before file access or command execution.
	tests := []struct {
		name      string
		kind      string
		tool      string
		arguments string
		errorText string
	}{
		{"custom type", "custom", "", "", "custom tool_call type not supported"},
		{"unknown type", "other", "", "", "unknown tool_call type other"},
		{"empty type", "", "", "", "unknown tool_call type"},
		{"invalid JSON", "function", ReadToolName, "{", "Error while parsing arguments"},
		{"array arguments", "function", ReadToolName, "[]", "Error while parsing arguments"},
		{"unknown tool", "function", "missing_tool", "{}", "unknown tool name missing_tool"},
		{"read missing path", "function", ReadToolName, "{}", "file_path argument not available"},
		{"read null arguments", "function", ReadToolName, "null", "file_path argument not available"},
		{"read numeric path", "function", ReadToolName, `{"file_path":42}`, "file_path not of type string"},
		{"read null path", "function", ReadToolName, `{"file_path":null}`, "file_path not of type string"},
		{"write missing path", "function", WriteToolName, "{}", "file_path argument not available"},
		{"write invalid path", "function", WriteToolName, `{"file_path":false}`, "file_path not of type string"},
		{"write missing content", "function", WriteToolName, `{"file_path":"unused.txt"}`, "content argument not available"},
		{"write invalid content", "function", WriteToolName, `{"file_path":"unused.txt","content":[]}`, "content not of type string"},
		{"write null content", "function", WriteToolName, `{"file_path":"unused.txt","content":null}`, "content not of type string"},
		{"command missing", "function", RunCommandToolName, "{}", "command argument not available"},
		{"command invalid", "function", RunCommandToolName, `{"command":123}`, "command not of type string"},
		{"command null", "function", RunCommandToolName, `{"command":null}`, "command not of type string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := openai.ChatCompletionMessageToolCallUnion{Type: tt.kind}
			call.Function.Name = tt.tool
			call.Function.Arguments = tt.arguments
			got, err := ExecuteToolCall(call, context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.errorText) {
				t.Errorf("error = %v, want %q", err, tt.errorText)
			}
			if got != "" {
				t.Errorf("result = %q, want empty", got)
			}
		})
	}
}
