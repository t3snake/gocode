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

	fncall := toolcall.AsFunction()
	var arg_map map[string]any
	err := json.Unmarshal([]byte(fncall.Function.Arguments), &arg_map)
	if err != nil {
		return "", fmt.Errorf("Error while parsing arguments: %s\n", err.Error())
	}

	fnname := fncall.Function.Name

	if fnname == ReadToolName {
		path, ok := arg_map["file_path"]
		if !ok {
			return "", fmt.Errorf("Error: file_path argument not available in %s tool.\n", fnname)
		}
		pathstr, ok := path.(string)
		if !ok {
			return "", fmt.Errorf("Error: file_path not of type string\n")
		}

		content, err := read_file(pathstr)
		if err != nil {
			return "", fmt.Errorf("Error while reading file: %s\n", err.Error())
		}

		return content, nil
	} else if fnname == WriteToolName {
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

		err := write_file(pathstr, contentstr)
		if err != nil {
			return "", fmt.Errorf("Error while writinf file: ", err.Error())
		}

		return "write_file successful", nil
	} else if fnname == BashToolName {
		command, ok := arg_map["command"]
		if !ok {
			return "", fmt.Errorf("Error: command argument not available in %s tool.\n", fnname)
		}
		commandstr, ok := command.(string)
		if !ok {
			return "", fmt.Errorf("Error: command not of type string\n")
		}

		result, err := run_bash_cmd(commandstr)
		if err != nil {
			// in case of bash, it is not error but just stderr output
			return err.Error(), nil
		}

		return result, nil
	}

	return "", fmt.Errorf("Error: unknown tool name %s\n", fnname)
}

// Built in tools

func read_file(path string) (content string, err error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content = string(bytes[:])
	return
}

func write_file(path, content string) (err error) {
	return os.WriteFile(path, []byte(content), 0666)
}

func run_bash_cmd(command string) (stdout string, stderr error) {
	cmd_and_args := strings.Split(command, " ")
	cmd := exec.Command(cmd_and_args[0], cmd_and_args[1:]...)

	var out strings.Builder
	var err_out strings.Builder

	_ = cmd.Run()

	return out.String(), fmt.Errorf(err_out.String())
}
