package main

import (
	"fmt"
	"os"

	// bubble tea tui fwk
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const USER_YELLOW = "#FFEBCC"
const AGENT_BLUE = "#BFDDF0"
const TERM_BG = "#B1D3B9"
const CURSOR = "#C5B3D3"

// Starts and runs a bubbletea TUI program
func StartTUI() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error %v", err)
		os.Exit(1)
	}
}

// ----- Bridge between TUI and calls to LLM -----

// Simple string value that takes in out and err messages from request to LLM
type ChatIoStream struct {
	value string
}

// Implement io.Writer interface to be compatible with fmt.Fprintf
func (co *ChatIoStream) Write(p []byte) (n int, err error) {
	co.value += string(p)
	return len(p), nil
}

type ChatResult struct {
	out    string
	err    string
	is_err bool
}

// tea.Cmd can only take empty params so return a function with empty params
func promptLlm(prompt string) tea.Cmd {
	// This function runs as a goroutine (handled by bubbletea)
	// The return is any type, we have to intercept our type in Update function
	return func() tea.Msg {
		var llm_out ChatIoStream
		var llm_err ChatIoStream

		client := getClient()

		retcode := runAgentLoop(client, prompt, Writers{
			out: &llm_out,
			err: &llm_err,
		})

		return ChatResult{
			out:    llm_out.value,
			err:    llm_err.value,
			is_err: (retcode != 0),
		}
	}
}

// ----- Main TUI Model Update View logic -----

// Struct representing user and chat-agent/llm messages
type Message struct {
	role   int8 // 0 user, 1 llm
	is_err bool
	id     int    // unique identifier, currently only 256 messages possible
	value  string // message
}

// TUI main state
type ChatState struct {
	prompt   textarea.Model
	messages []Message
	viewport viewport.Model

	is_loading bool
	spinner    spinner.Model

	user_style  lipgloss.Style
	agent_style lipgloss.Style
}

func initialModel() ChatState {
	ta := textarea.New()
	ta.Placeholder = "Type to get started"
	ta.SetVirtualCursor(false)
	ta.Focus()

	ta.Prompt = "| "
	ta.SetWidth(30)
	ta.SetHeight(5)

	st := ta.Styles()
	st.Focused.CursorLine = lipgloss.NewStyle()
	st.Cursor.Color = lipgloss.Color(CURSOR)
	// st.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	st.Focused.Base = lipgloss.NewStyle().
		Background(lipgloss.Color(USER_YELLOW)).
		Foreground(lipgloss.Color("#FFFFFF"))
	ta.SetStyles(st)

	ta.ShowLineNumbers = false

	vp := viewport.New(viewport.WithHeight(10), viewport.WithWidth(30))
	vp.SetContent("Go Code by t3snake")
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	us := lipgloss.NewStyle().Foreground(lipgloss.Color(USER_YELLOW))
	as := lipgloss.NewStyle().Foreground(lipgloss.Color(AGENT_BLUE))

	return ChatState{
		prompt:   ta,
		viewport: vp,
		messages: []Message{},

		is_loading: false,
		spinner:    s,

		user_style:  us,
		agent_style: as,
	}
}

func (c ChatState) Init() tea.Cmd {
	return nil
}

func (c ChatState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.prompt.SetWidth(msg.Width)
		st := c.prompt.Styles()
		st.Focused.Base = st.Focused.Base.Width(msg.Width)
		c.prompt.SetStyles(st)

		c.viewport.SetWidth(msg.Width)
		c.viewport.SetHeight(msg.Height - c.prompt.Height())
	case ChatResult:
		var output string
		if msg.is_err {
			output = msg.err
		} else {
			output = msg.out
		}

		c.is_loading = false
		c.messages = append(c.messages, Message{
			role:   1,
			id:     len(c.messages),
			value:  output,
			is_err: msg.is_err,
		})

		return c, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return c, tea.Quit

		case "enter":
			prompt := c.prompt.Value()
			if len(prompt) == 0 {
				return c, nil
			}

			if prompt == "quit" || prompt == "exit" {
				return c, tea.Quit
			}

			c.is_loading = true
			c.prompt.Reset()
			c.messages = append(c.messages,
				Message{
					role:  0,
					id:    len(c.messages),
					value: prompt,
				},
			)

			return c, promptLlm(prompt)

		default:
			var cmd tea.Cmd
			c.prompt, cmd = c.prompt.Update(msg)
			c.spinner, cmd = c.spinner.Update(msg)
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
	view := c.viewport.View()

	if c.is_loading {
		view += "\n" + c.spinner.View()
	}

	v := tea.NewView(view + "\n" + c.prompt.View())

	cr := c.prompt.Cursor()
	if cr != nil {
		cr.Y += lipgloss.Height(view)
	}

	v.Cursor = cr
	v.AltScreen = true
	v.WindowTitle = "Go Code"
	v.BackgroundColor = lipgloss.Color(TERM_BG)

	return v
}
