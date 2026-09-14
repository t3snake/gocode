package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/t3snake/gocode/src/chatcompletion"
	"github.com/t3snake/gocode/src/core"
	"github.com/t3snake/gocode/src/logger"
	"github.com/t3snake/gocode/src/tui"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM in CLI mode")
	flag.Parse()

	// check if flag is found, ie. if run in CLI mode
	p_flag_found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "p" {
			p_flag_found = true
		}
	})

	f, err := os.OpenFile("sessionLog.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cant create log file: %v", err)
		os.Exit(1)
	}
	defer f.Close()

	logger.Init(f)

	if p_flag_found {
		if prompt == "" {
			panic("Prompt must not be empty")
		}

		client := chatcompletion.GetClient()

		logger.Info("gocode started in prompt mode.")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		agent_loop_params := chatcompletion.AgentLoopParams{
			Client:     client,
			Ctx:        ctx,
			UserPrompt: prompt,
			Writers: core.Writers{
				Out: os.Stdout,
				Err: os.Stderr,
			},
			LlmToTui: nil,
			TuiToLlm: nil,
		}

		retcode := chatcompletion.RunAgentLoop(agent_loop_params)

		os.Exit(retcode)
	}

	logger.Info("gocode started in TUI mode.")

	// else start TUI
	tui.StartTUI()
}
