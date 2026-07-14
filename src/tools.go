package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openai/openai-go/v3"
)

// Constants
const ReadToolName = "ReadFile"
const WriteToolName = "WriteFile"
const BashToolName = "RunBashCommand"

// Tool utils

func ExecuteToolCall(toolcall openai.ChatCompletionMessageToolCallUnion) (string, error) {
	if toolcall.Type == "custom" {
		return "", fmt.Errorf("custom tool_call type not supported.\n")
	} else if toolcall.Type != "function" {
		return "", fmt.Errorf("unknown tool_call type %s.\n", toolcall.Type)
	}

	var arg_map map[string]any
	err := json.Unmarshal([]byte(toolcall.Function.Arguments), &arg_map)
	if err != nil {
		return "", fmt.Errorf("Error while parsing arguments: %s\n", err.Error())
	}

	fnname := toolcall.Function.Name

	switch fnname {
	case ReadToolName:
		path, ok := arg_map["file_path"]
		if !ok {
			return "", fmt.Errorf("Error: file_path argument not available in %s tool.\n", fnname)
		}
		pathstr, ok := path.(string)
		if !ok {
			return "", fmt.Errorf("Error: file_path not of type string\n")
		}

		content, err := readFile(pathstr)
		if err != nil {
			return "", fmt.Errorf("Error while reading file: %s\n", err.Error())
		}

		return content, nil

	case WriteToolName:
		// parse file_path

		path, ok := arg_map["file_path"]
		if !ok {
			return "", fmt.Errorf("Error: file_path argument not available in %s tool.\n", fnname)
		}
		pathstr, ok := path.(string)
		if !ok {
			return "", fmt.Errorf("Error: file_path not of type string\n")
		}

		// parse content

		content, ok := arg_map["content"]
		if !ok {
			return "", fmt.Errorf("Error: content argument not available in %s tool.\n", fnname)
		}
		contentstr, ok := content.(string)
		if !ok {
			return "", fmt.Errorf("Error: content not of type string\n")
		}

		err := writeFile(pathstr, contentstr)
		if err != nil {
			return "", fmt.Errorf("Error while writing file: %s", err.Error())
		}

		return "write_file successful", nil

	case BashToolName:
		command, ok := arg_map["command"]
		if !ok {
			return "", fmt.Errorf("Error: command argument not available in %s tool.\n", fnname)
		}
		commandstr, ok := command.(string)
		if !ok {
			return "", fmt.Errorf("Error: command not of type string\n")
		}

		result, err := runBashCommand(commandstr)
		if err != nil {
			// in case of bash, it is not error but just stderr output
			return err.Error(), nil
		}

		return result, nil
	}

	return "", fmt.Errorf("Error: unknown tool name %s\n", fnname)
}

// Built in tool functions and registrations
func readFileRegistration() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionToolUnionParam{
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
	}
}

func readFile(path string) (content string, err error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content = string(bytes[:])
	return
}

func writeFileRegistration() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionToolUnionParam{
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
	}
}

func writeFile(path, content string) (err error) {
	return os.WriteFile(path, []byte(content), 0666)
}

func runBashRegistration() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionToolUnionParam{
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
	}
}
func runBashCommand(command string) (stdout string, stderr error) {
	cmd_and_args := strings.Split(command, " ")
	cmd := exec.Command(cmd_and_args[0], cmd_and_args[1:]...)

	var out strings.Builder
	var err_out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &err_out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	stderr = nil
	if len(err_out.String()) != 0 {
		stderr = fmt.Errorf("%s", err_out.String())
	}
	return out.String(), stderr
}
