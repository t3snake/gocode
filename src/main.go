package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/t3snake/gocode/src/logger"
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

		client := getClient()

		logger.Info("gocode started in prompt mode.")

		retcode := runAgentLoop(client, prompt, Writers{
			os.Stdout, // just print to stdout, (might also log in prompt mode)
			os.Stderr, // just print to stderr, (might also log in prompt mode)
			true,      // suppress exclusive logs, just have the ai output or err, maybe 'showLogs' flag enables logs.
		}, nil, nil)

		os.Exit(retcode)
	}

	logger.Info("gocode started in TUI mode.")

	// else start TUI
	StartTUI()
}
