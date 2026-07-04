package main

import (
	"fmt"
	"os"

	// bubble tea tui fwk
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func StartTUI() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error %v", err)
		os.Exit(1)
	}
}

type ChatOutput struct {
	value string
}

func (co *ChatOutput) Write(p []byte) (n int, err error) {
	co.value += string(p)
	return len(p), nil
}

type ChatState struct {
	title    string
	prompt   textarea.Model
	viewport viewport.Model
	llm_out  *ChatOutput
	llm_err  *ChatOutput
	user_out *ChatOutput
}

func initialModel() ChatState {
	ta := textarea.New()
	ta.Placeholder = "Type to get started"
	ta.SetVirtualCursor(false)
	ta.Focus()

	ta.Prompt = "| "
	ta.SetWidth(30)
	ta.SetHeight(5)

	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(s)

	ta.ShowLineNumbers = false

	vp := viewport.New(viewport.WithHeight(5), viewport.WithWidth(30))
	vp.SetContent("Go Code by t3snake")
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	return ChatState{
		title:    "Go Code by t3snake",
		prompt:   ta,
		viewport: vp,
		llm_out: &ChatOutput{
			value: "",
		},
		llm_err: &ChatOutput{
			value: "",
		},
		user_out: &ChatOutput{
			value: "",
		},
	}
}

func (c ChatState) Init() tea.Cmd {
	return nil
}

func (c ChatState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return c, tea.Quit

		case "enter":
			if len(c.prompt.Value()) == 0 {
				return c, nil
			}

			if c.prompt.Value() == "quit" {
				return c, tea.Quit
			}

			client := getClient()

			runAgentLoop(client, c.prompt.Value(), Writers{
				out: c.llm_out,
				err: c.llm_err,
			})

		default:
			var cmd tea.Cmd
			c.prompt, cmd = c.prompt.Update(msg)
			return c, cmd
		}

	case cursor.BlinkMsg:
		var cmd tea.Cmd
		c.prompt, cmd = c.prompt.Update(msg)
		return c, cmd

	}

	return c, nil
}

func (c ChatState) View() tea.View {
	viewportView := c.viewport.View()
	v := tea.NewView(viewportView + "\n" + c.prompt.View())
	cr := c.prompt.Cursor()
	if cr != nil {
		cr.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = cr
	v.AltScreen = true

	return v
}
