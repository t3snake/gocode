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

type Llm2Tui struct {
	// for tool call permission to tui

	is_tool_call bool   // Reports whether LLM is requesting a tool call
	tool_name    string // tool name, only guaranteed if [Llm2Tui.is_tool_call] is true

	// TODO(t3snake): parse and make map[string]string
	params string // tool params, only guaranteed if [Llm2Tui.is_tool_call] is true

	// stream thinking/content

	is_chunk      bool // Reports whether a chunk was streamed
	is_last       bool // Reports whether the last chunk was just streamed. Only valid if [Llm2Tui.is_chunk] is true.
	chunk_content string

	token_spent int // Reports how many tokens were spent so far in the agent loop.

}

type Tui2Llm struct {
	// allow or reject?
	is_allowed        bool   // Reports whether user allowed the tool use, either through always allow or setting allow.
	adjustment_prompt string // only used to change course, if [Tui2Llm.is_allowed] is false
}

// Starts and runs a bubbletea TUI program
func StartTUI() {
	tui2llm := make(chan Tui2Llm)
	llm2tui := make(chan Llm2Tui)

	p := tea.NewProgram(initialModel(llm2tui, tui2llm))
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

type ChatStream struct {
	llm_msg Llm2Tui
}

// Runs agent loop using openai chat completion API
func promptLlm(prompt string, tui2llm chan Tui2Llm, llm2tui chan Llm2Tui) tea.Cmd {
	// tea.Cmd can only take fn with empty params so return a function with empty params and use closure
	// This function runs as a goroutine (handled by bubbletea)
	// The return is any type, we have to intercept our type in Update function
	return func() tea.Msg {
		var llm_out ChatIoStream
		var llm_err ChatIoStream

		client := getClient()

		retcode := runAgentLoop(client, prompt, Writers{
			out: &llm_out,
			err: &llm_err,
		}, llm2tui, tui2llm)

		return ChatResult{
			out:    llm_out.value,
			err:    llm_err.value,
			is_err: (retcode != 0),
		}
	}
}

func listenLlmStream(llm2tui chan Llm2Tui) tea.Cmd {
	return func() tea.Msg {
		stream_chunk := <-llm2tui

		return ChatStream{llm_msg: stream_chunk}
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
	// window dimensions

	app_width  uint16
	app_height uint16

	// reusable bubbles

	prompt   textarea.Model
	viewport viewport.Model

	// messages (history) and currently streaming message

	messages        []Message
	current_message Message
	token_spend     int

	// loading state
	is_loading bool
	spinner    spinner.Model

	// Theme related

	theme       Theme
	user_style  lipgloss.Style
	agent_style lipgloss.Style

	// Channel for communication between TUI and LLM goroutines. For streaming and toolcall UX
	tui2llm chan Tui2Llm
	llm2tui chan Llm2Tui
}

func initialModel(llm2tui chan Llm2Tui, tui2llm chan Tui2Llm) ChatState {
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

	us := lipgloss.NewStyle().Background(theme.UserChatBackground).MarginLeft(5).MarginBottom(1).Padding(1)
	as := lipgloss.NewStyle().Background(theme.AgentChatBackground).MarginRight(5).MarginBottom(1).Padding(1)

	return ChatState{
		app_width:  400,
		app_height: 300,

		prompt:   ta,
		viewport: vp,

		messages: []Message{},
		current_message: Message{
			role:   1,
			is_err: false,
			id:     5,
			value:  "",
		},

		is_loading: false,
		spinner:    s,

		theme:       theme,
		user_style:  us,
		agent_style: as,

		llm2tui: llm2tui,
		tui2llm: tui2llm,
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
		c.viewport.Style = lipgloss.NewStyle().Padding(1).Align(lipgloss.Center)

	case ChatStream:
		is_last_chunk := false

		if msg.llm_msg.is_chunk {
			c.current_message = Message{
				role:  1,
				id:    uint8(len(c.messages)),
				value: c.current_message.value + msg.llm_msg.chunk_content,
			}

			if msg.llm_msg.is_last {
				is_last_chunk = true
				c.messages = append(c.messages, c.current_message)
				c.current_message.value = ""
			}
		}

		if msg.llm_msg.is_tool_call {
			// TODO(t3snake): implement tool call user interaction allow-reject
			c.tui2llm <- Tui2Llm{
				is_allowed:        true, // currently hardcoding to true, ideally have a simple button selection
				adjustment_prompt: "",   // UX?
			}
		}

		if is_last_chunk {
			cmd = nil
		} else {
			cmd = listenLlmStream(c.llm2tui)
		}

		content := renderChatMessages(c)
		c.viewport.SetContent(content)

		return c, cmd

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

		content := renderChatMessages(c)
		c.viewport.SetContent(content)

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

			content := renderChatMessages(c)
			c.viewport.SetContent(content)

			return c, tea.Batch(
				c.spinner.Tick,
				promptLlm(prompt, c.tui2llm, c.llm2tui),
				listenLlmStream(c.llm2tui),
			)

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

	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)

	c.prompt, cmd = c.prompt.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

func (c ChatState) View() tea.View {
	view := c.viewport.View() + "\n"

	if c.is_loading {
		view += fmt.Sprintf("Thinking %s", c.spinner.View())
	}

	chatBoxStyle := lipgloss.NewStyle().
		Width(int(c.app_width)).
		Height(7).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(c.theme.ActiveBorder).
		MarginBottom(1)

	v := tea.NewView(view + "\n" + chatBoxStyle.Render(c.prompt.View()))

	v.WindowTitle = "Go Code"
	v.BackgroundColor = c.theme.TerminalBackground
	v.ForegroundColor = c.theme.Text
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	cr := c.prompt.Cursor()
	if cr != nil {
		cr.Y += lipgloss.Height(view) + 1
		cr.X += 1
	}

	v.Cursor = cr

	return v
}

func renderChatMessages(c ChatState) (content string) {
	content = ""
	msg_width := c.viewport.Width() - 5
	for _, msg := range c.messages {
		switch msg.role {
		case 0: // user message
			prefix := ""
			if msg.is_err {
				prefix = lipgloss.NewStyle().
					Foreground(Color(CTPC_RED)).
					Render("Error: ")
			}
			content += c.user_style.
				Width(msg_width).
				Render(prefix+msg.value) + "\n"
		case 1: // agent message
			content += c.agent_style.
				Width(msg_width).
				Render(msg.value) + "\n"
		}
	}

	// render currently streaming message
	if len(c.current_message.value) != 0 {
		content += c.agent_style.
			Width(msg_width).
			Render(c.current_message.value) + "\n"
	}

	return content
}
