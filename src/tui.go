package main

import (
	"fmt"
	"os"

	// bubble tea tui fwk

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
	role   uint8 // 0 user, 1 llm
	is_err bool
	id     uint8  // unique identifier, currently only 256 messages possible
	value  string // message
}

// TUI main state
type ChatState struct {
	prompt   textarea.Model
	messages []Message
	viewport viewport.Model

	is_loading bool
	spinner    spinner.Model

	theme       Theme
	user_style  lipgloss.Style
	agent_style lipgloss.Style

	app_width  uint16
	app_height uint16
}

func initialModel() ChatState {
	theme := catpuccinMacchiatoTheme

	ta := textarea.New()
	ta.Placeholder = "Type to get started"
	ta.SetVirtualCursor(false)
	ta.Focus()

	ta.SetWidth(30)
	ta.SetHeight(5)

	ta.SetStyles(textarea.DefaultDarkStyles())
	st := ta.Styles()

	st.Cursor.Color = theme.Cursor
	st.Focused.CursorLine = lipgloss.NewStyle()
	st.Focused.Placeholder = lipgloss.NewStyle().
		Foreground(Color("#c6a0f6"))

	ta.SetStyles(st)

	ta.ShowLineNumbers = false

	vp := viewport.New(viewport.WithHeight(10), viewport.WithWidth(30))
	vp.SetContent("Go Code by t3snake")
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(Color(CTPC_RED))

	us := lipgloss.NewStyle().Background(theme.UserChatBackground).MarginLeft(5).MarginBottom(1)
	as := lipgloss.NewStyle().Background(theme.AgentChatBackground).MarginRight(5).MarginBottom(1)

	return ChatState{
		prompt:   ta,
		viewport: vp,
		messages: []Message{},

		is_loading: false,
		spinner:    s,

		theme:       theme,
		user_style:  us,
		agent_style: as,

		app_width:  400,
		app_height: 300,
	}
}

func (c ChatState) Init() tea.Cmd {
	return textarea.Blink
}

func (c ChatState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.app_height = uint16(msg.Height)
		c.app_width = uint16(msg.Width)

		c.prompt.SetWidth(msg.Width - 1)

		c.viewport.SetWidth(msg.Width - 1)
		c.viewport.SetHeight(msg.Height - c.prompt.Height() - 5)
		c.viewport.Style = lipgloss.NewStyle().Align(lipgloss.Center)
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
			id:     uint8(len(c.messages)),
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
					id:    uint8(len(c.messages)),
					value: prompt,
				},
			)

			return c, tea.Batch(c.spinner.Tick, promptLlm(prompt))

		default:
			if !c.prompt.Focused() {
				cmd = c.prompt.Focus()
				cmds = append(cmds, cmd)
			}
		}

	case spinner.TickMsg:
		c.spinner, cmd = c.spinner.Update(msg)
		cmds = append(cmds, cmd)

	}

	c.prompt, cmd = c.prompt.Update(msg)
	cmds = append(cmds, cmd)
	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return c, tea.Batch(cmds...)
}

func (c ChatState) View() tea.View {
	content := renderChatMessages(c)
	c.viewport.SetContent(content)
	view := c.viewport.View() + "\n"

	if c.is_loading {
		view += c.spinner.View()
	}

	chatBoxStyle := lipgloss.NewStyle().
		Width(int(c.app_width)).
		Height(7).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(c.theme.ActiveBorder).
		MarginTop(1).MarginBottom(1)

	v := tea.NewView(view + "\n" + chatBoxStyle.Render(c.prompt.View()))

	v.WindowTitle = "Go Code"
	v.BackgroundColor = c.theme.TerminalBackground
	v.ForegroundColor = c.theme.Text
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	cr := c.prompt.Cursor()
	if cr != nil {
		cr.Y += lipgloss.Height(view) + 2
		cr.X += 1
	}

	v.Cursor = cr

	return v
}

func renderChatMessages(c ChatState) (content string) {
	content = ""
	msg_width := c.viewport.Width() - 5
	for _, msg := range c.messages {
		if msg.role == 0 {
			prefix := ""
			if msg.is_err {
				prefix = lipgloss.NewStyle().
					Foreground(Color(CTPC_RED)).
					Render("Error: ")
			}
			content += c.user_style.
				Width(msg_width).
				Render(prefix+msg.value) + "\n"
		} else if msg.role == 1 {
			content += c.agent_style.
				Width(msg_width).
				Render(msg.value) + "\n"
		}
	}

	return content
}
