package main

import (
	"flag"
	"os"
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

	if p_flag_found {
		if prompt == "" {
			panic("Prompt must not be empty")
		}

		client := getClient()

		retcode := runAgentLoop(client, prompt, Writers{
			os.Stdout, // just print to stdout, (might also log in prompt mode)
			os.Stderr, // just print to stderr, (might also log in prompt mode)
			true,      // suppress exclusive logs, just have the ai output or err, maybe 'showLogs' flag enables logs.
		}, nil, nil)

		os.Exit(retcode)
	}

	// else start TUI
	StartTUI()
}
