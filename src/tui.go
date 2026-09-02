package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	// bubble tea tui fwk

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/t3snake/gocode/src/logger"
)

// Starts and runs a bubbletea TUI program
func StartTUI() {
	tui2llm := make(chan Tui2Llm)
	llm2tui := make(chan Llm2Tui)

	p := tea.NewProgram(initialModel(llm2tui, tui2llm), tea.WithFPS(120))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error %v", err)
		os.Exit(1)
	}
}

// ----- Bridge between TUI and calls to LLM -----

type Llm2Tui struct {
	// for tool call permission to tui

	is_tool_call bool   // Reports whether LLM is requesting a tool call
	tool_name    string // tool name, only guaranteed if [Llm2Tui.is_tool_call] is true

	// TODO(t3snake): parse and make map[string]string
	params string // tool params, only guaranteed if [Llm2Tui.is_tool_call] is true

	// stream thinking/content

	is_chunk        bool // Reports whether a chunk was streamed
	is_last_content bool // Reports whether the last chunk was just streamed. Only valid if [Llm2Tui.is_chunk] is true. Not used currently, TODO evaluate
	chunk_content   string

	is_usage_chunk bool // in streaming, the very last chunk when usage is enabled, just sends the token_spent
	token_spent    int  // Reports how many tokens were spent so far in the agent loop.

	should_stop_listening bool // tui can safely stop listening when this is true
}

type Tui2Llm struct {
	// allow or reject?
	is_allowed        bool   // Reports whether user allowed the tool use, either through always allow or setting allow.
	adjustment_prompt string // only used to change course, if [Tui2Llm.is_allowed] is false
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
func promptLlm(prompt string, ctx context.Context, tui2llm chan Tui2Llm, llm2tui chan Llm2Tui) tea.Cmd {
	// tea.Cmd can only take fn with empty params so return a function with empty params and use closure
	// This function runs as a goroutine (handled by bubbletea)
	// The return is any type, we have to intercept our type in Update function
	return func() tea.Msg {
		var display_out strings.Builder
		var display_err strings.Builder

		client := getClient()

		retcode := runAgentLoop(client, ctx, prompt, Writers{
			out: &display_out,
			err: &display_err,
		}, llm2tui, tui2llm)

		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				display_err.WriteString(ctx.Err().Error())
			}
			return ChatResult{
				out:    display_out.String(),
				err:    display_err.String(),
				is_err: (retcode != 0),
			}

		default:
			return ChatResult{
				out:    display_out.String(),
				err:    display_err.String(),
				is_err: (retcode != 0),
			}
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

type Role uint8

const (
	USER Role = iota
	LLM  Role = iota
	TOOL Role = iota
)

// Struct representing user and chat-agent/llm messages
type Message struct {
	role         Role // 0 USER, 1 LLM, 2 TOOL
	is_err       bool
	id           uint8  // unique identifier, currently only 256 messages possible
	display_text string // message
	error_text   string // non null and non empty when is_err is true
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

	tui2llm    chan Tui2Llm
	llm2tui    chan Llm2Tui
	ctx        context.Context
	ctx_cancel context.CancelCauseFunc

	// misc

	// If mouse is pressed down, to set mouse mode, only one can happen (scroll) or (selecting text)
	// Use opencode behavior
	is_selecting bool
}

func initialModel(llm2tui chan Llm2Tui, tui2llm chan Tui2Llm) ChatState {
	theme := catpuccinMacchiatoTheme

	ta := textarea.New()
	ta.Placeholder = "Type to get started"
	ta.ShowLineNumbers = false

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

	vp := viewport.New(viewport.WithHeight(10), viewport.WithWidth(30))
	vp.SetContent("Go Code by t3snake")
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(Color(CTPC_RED))

	us := lipgloss.NewStyle().Background(theme.UserChatBackground).Padding(1)
	as := lipgloss.NewStyle().Background(theme.AgentChatBackground).Padding(1)

	return ChatState{
		app_width:  400,
		app_height: 300,

		prompt:   ta,
		viewport: vp,

		messages: []Message{},
		current_message: Message{
			role:         LLM,
			is_err:       false,
			id:           5,
			display_text: "",
			error_text:   "",
		},

		is_loading: false,
		spinner:    s,

		theme:       theme,
		user_style:  us,
		agent_style: as,

		llm2tui: llm2tui,
		tui2llm: tui2llm,

		ctx:        nil,
		ctx_cancel: nil,

		is_selecting: false,
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

		c.prompt.SetWidth(msg.Width - 3)

		c.viewport.SetWidth(msg.Width - 1)

		if c.is_loading {
			c.viewport.SetHeight(msg.Height - 3)
		} else {
			c.viewport.SetHeight(msg.Height - c.prompt.Height() - 3)
		}

		c.viewport.Style = lipgloss.NewStyle().Padding(1).Align(lipgloss.Center)

		content := renderChatMessages(c)
		c.viewport.SetContent(content)
		c.viewport.GotoBottom()

	case tea.MouseClickMsg:
		// Note: Can either "select text" or "scroll" cant do both. Terminal alternate buffer limitation.
		// Opencode and crush get around it by essentially doing this: while mouse is pressed, disable scrolling but enable text selection
		c.is_selecting = true

	case tea.MouseReleaseMsg:
		// when mouse is "un"pressed / released, enable scrolling and disable text selection
		c.is_selecting = false
		tea.SetClipboard("selected text")

	case ChatStream:
		if msg.llm_msg.is_chunk {
			c.current_message.display_text += msg.llm_msg.chunk_content
		}

		if msg.llm_msg.is_tool_call {
			// TODO(t3snake): implement tool call user interaction allow-reject
			c.tui2llm <- Tui2Llm{
				is_allowed:        true, // currently hardcoding to true, ideally have a simple button selection
				adjustment_prompt: "",   // UX?
			}
		}

		if msg.llm_msg.should_stop_listening {
			cmd = nil
		} else {
			cmd = listenLlmStream(c.llm2tui)
		}

		content := renderChatMessages(c)
		c.viewport.SetContent(content)
		c.viewport.GotoBottom()

		return c, cmd

	case ChatResult:
		c.current_message.is_err = msg.is_err
		c.current_message.error_text = msg.err

		c.messages = append(c.messages, c.current_message)

		c.current_message.display_text = ""
		c.current_message.error_text = ""
		c.current_message.is_err = false

		c.is_loading = false

		c.viewport.SetHeight(int(c.app_height) - c.prompt.Height() - 2)
		content := renderChatMessages(c)
		c.viewport.SetContent(content)

		c.ctx = nil
		c.ctx_cancel = nil

		return c, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			// TODO copy when text selected, or remove ctrl+c binding
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
					role:         USER,
					id:           uint8(len(c.messages)),
					display_text: prompt,
					is_err:       false,
					error_text:   "",
				},
			)

			c.current_message = Message{
				role:         LLM,
				id:           uint8(len(c.messages)),
				display_text: "",
				is_err:       false,
				error_text:   "",
			}

			c.viewport.SetHeight(int(c.app_height) - 3)
			content := renderChatMessages(c)
			c.viewport.SetContent(content)

			c.ctx, c.ctx_cancel = context.WithCancelCause(context.Background())

			return c, tea.Batch(
				c.spinner.Tick,
				promptLlm(prompt, c.ctx, c.tui2llm, c.llm2tui),
				listenLlmStream(c.llm2tui),
			)

		case "esc":
			// TODO double escape for cancellation
			if c.is_loading && c.ctx_cancel != nil {
				c.ctx_cancel(errors.New("user-cancel"))
			}

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
	cursor_y := 0
	cursor_x := 0

	if c.is_loading {
		spinner := fmt.Sprintf("Thinking %s", c.spinner.View())
		view += spinner
		cursor_y = lipgloss.Height(view)
		cursor_x = len(spinner)
	} else {
		chatBoxStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(c.theme.ActiveBorder).
			Width(int(c.app_width) - 1).
			Height(7).
			MarginBottom(1)

		cursor_y = lipgloss.Height(view) + 1 // accounting for newline
		cursor_x = 1
		view = view + "\n" + chatBoxStyle.Render(c.prompt.View())
	}
	v := tea.NewView(view)

	v.WindowTitle = "Go Code"
	v.BackgroundColor = c.theme.TerminalBackground
	v.ForegroundColor = c.theme.Text
	v.AltScreen = true
	if c.is_selecting {
		v.MouseMode = tea.MouseModeNone
	} else {
		v.MouseMode = tea.MouseModeCellMotion
	}

	cr := c.prompt.Cursor()
	if cr != nil {
		cr.Y += cursor_y
		cr.X += cursor_x
	}

	v.Cursor = cr

	return v
}

func renderChatMessages(c ChatState) (content string) {
	content = ""
	msg_width := c.viewport.Width() - 2 // subtract padding

	// md render lib
	style := styles.DarkStyleConfig
	glam, _ := glamour.NewTermRenderer(
		glamour.WithWordWrap(msg_width),
		glamour.WithStyles(style),
	)

	for _, msg := range c.messages {
		switch msg.role {
		case USER:
			content += c.user_style.
				Width(msg_width).
				Render(msg.display_text) + "\n"

		case LLM:
			postfix := ""
			if msg.is_err {
				postfix = lipgloss.NewStyle().
					Foreground(Color(CTPC_RED)).
					Render(fmt.Sprintf("\nError: %s", msg.error_text))
			}

			glamout, err := glam.Render(msg.display_text)
			if err != nil {
				logger.Error(err.Error())
			}
			content += glamout + postfix + "\n"

			// content += c.agent_style.
			// Width(msg_width).
			// Render(msg.display_text+postfix) + "\n"
		}
	}

	// render currently streaming message
	if len(c.current_message.display_text) != 0 {
		glamout, err := glam.Render(c.current_message.display_text)
		if err != nil {
			logger.Error(err.Error())
		}

		content += glamout + "\n"
		// content += c.agent_style.
		// 	Width(msg_width).
		// 	Render(c.current_message.value) + "\n"
	}

	return content
}
