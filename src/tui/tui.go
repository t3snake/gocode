package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	// bubble tea tui fwk

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"

	"charm.land/lipgloss/v2"

	"github.com/t3snake/gocode/src/chatcompletion"
	"github.com/t3snake/gocode/src/core"
	"github.com/t3snake/gocode/src/logger"
)

// Starts and runs a bubbletea TUI program
func StartTUI() {
	tui2llm := make(chan core.Tui2Llm)
	llm2tui := make(chan core.Llm2Tui)

	p := tea.NewProgram(initialModel(llm2tui, tui2llm), tea.WithFPS(120))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error %v", err)
		os.Exit(1)
	}
}

// ----- Bridge between TUI and calls to LLM -----

type ChatResult struct {
	out    string
	err    string
	is_err bool
}

type ChatStream struct {
	llm_msg core.Llm2Tui
}

// Runs agent loop using openai chat completion API
func promptLlm(prompt string, ctx context.Context, tui2llm chan core.Tui2Llm, llm2tui chan core.Llm2Tui) tea.Cmd {
	// tea.Cmd can only take fn with empty params so return a function with empty params and use closure
	// This function runs as a goroutine (handled by bubbletea)
	// The return is any type, we have to intercept our type in Update function
	return func() tea.Msg {
		var display_out strings.Builder
		var display_err strings.Builder

		client := chatcompletion.GetClient()

		agent_loop_params := chatcompletion.AgentLoopParams{
			Client:     client,
			Ctx:        ctx,
			UserPrompt: prompt,
			Writers: core.Writers{
				Out: &display_out,
				Err: &display_err,
			},
			LlmToTui: llm2tui,
			TuiToLlm: tui2llm,
		}

		retcode := chatcompletion.RunAgentLoop(agent_loop_params)

		select {
		case <-ctx.Done():
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

func listenLlmStream(llm2tui chan core.Llm2Tui) tea.Cmd {
	return func() tea.Msg {
		stream_chunk := <-llm2tui

		return ChatStream{
			llm_msg: stream_chunk,
		}
	}
}

// ----- Main TUI Model Update View logic -----

// TUI main state
type ChatState struct {
	// window dimensions

	app_width  uint16
	app_height uint16

	// reusable bubbles

	prompt   textarea.Model
	viewport viewport.Model

	// messages (history) and currently streaming message

	messages        []core.GocodeMessage
	current_message core.GocodeMessage
	token_spend     int

	// loading state
	is_loading bool
	spinner    spinner.Model

	// Theme related

	theme       Theme
	user_style  lipgloss.Style
	agent_style lipgloss.Style

	// Channel for communication between TUI and LLM goroutines. For streaming and toolcall UX

	tui2llm    chan core.Tui2Llm
	llm2tui    chan core.Llm2Tui
	ctx        context.Context
	ctx_cancel context.CancelCauseFunc

	// misc

	// If mouse is pressed down, to set mouse mode, only one can happen (scroll) or (selecting text)
	// Use opencode behavior
	is_selecting bool
}

func initialModel(llm2tui chan core.Llm2Tui, tui2llm chan core.Tui2Llm) ChatState {
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

		messages: []core.GocodeMessage{},
		current_message: core.GocodeMessage{
			MsgRole:     core.LLM,
			IsError:     false,
			Id:          5,
			DisplayText: "",
			ErrorText:   "",
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
	// start listener immediately
	// startea.Batch(t listener imm, listenLlmStream(c.llm2tui))ediately
	return tea.Batch(textarea.Blink, listenLlmStream(c.llm2tui))
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

		// c.prompt.BeginSelection(msg.X, msg.Y)
		// c.prompt.Sel

	case tea.MouseReleaseMsg:
		// when mouse is "un"pressed / released, enable scrolling and disable text selection
		c.is_selecting = false

	case ChatStream:
		if msg.llm_msg.IsChunk {
			c.current_message.DisplayText += msg.llm_msg.ChunkContent

			// only rerender when there is a chunk content
			content := renderChatMessages(c)
			c.viewport.SetContent(content)
			c.viewport.GotoBottom()
		}

		if msg.llm_msg.IsToolCall && c.tui2llm != nil {
			// TODO(t3snake): implement tool call user interaction allow-reject
			c.tui2llm <- core.Tui2Llm{
				IsAllowed:        true, // currently hardcoding to true, ideally have a simple button selection
				AdjustmentPrompt: "",   // UX?
			}
		}

		// always have it active, by reopening listener immediately when returned
		cmd = listenLlmStream(c.llm2tui)

		return c, cmd

	case ChatResult:
		c.current_message.IsError = msg.is_err
		c.current_message.ErrorText = msg.err

		c.messages = append(c.messages, c.current_message)

		c.current_message.DisplayText = ""
		c.current_message.ErrorText = ""
		c.current_message.IsError = false

		c.is_loading = false

		// while prompt is enabled, dont let viewport scroll (j, k vim binds)
		c.viewport.KeyMap.Down.SetEnabled(false)
		c.viewport.KeyMap.Up.SetEnabled(false)

		c.viewport.SetHeight(int(c.app_height) - c.prompt.Height() - 3)
		content := renderChatMessages(c)
		c.viewport.SetContent(content)
		c.viewport.GotoBottom()

		c.ctx_cancel(nil)

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
				core.GocodeMessage{
					MsgRole:     core.USER,
					Id:          uint8(len(c.messages)),
					DisplayText: prompt,
					IsError:     false,
					ErrorText:   "",
				},
			)

			c.current_message = core.GocodeMessage{
				MsgRole:     core.LLM,
				Id:          uint8(len(c.messages)),
				DisplayText: "",
				IsError:     false,
				ErrorText:   "",
			}

			// while is loading, let viewport scroll (j, k vim binds)
			c.viewport.KeyMap.Down.SetEnabled(true)
			c.viewport.KeyMap.Up.SetEnabled(true)

			c.viewport.SetHeight(int(c.app_height) - 3)
			content := renderChatMessages(c)
			c.viewport.SetContent(content)
			c.viewport.GotoBottom()

			c.ctx, c.ctx_cancel = context.WithCancelCause(context.Background())

			return c, tea.Batch(
				c.spinner.Tick,
				promptLlm(prompt, c.ctx, c.tui2llm, c.llm2tui),
			)

		case "esc":
			// TODO double escape for cancellation
			if c.is_loading && c.ctx_cancel != nil {
				c.ctx_cancel(core.CancelSignalError)
			}

		default:
			if !c.prompt.Focused() && !c.is_loading {
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

	if !c.is_loading {
		c.prompt, cmd = c.prompt.Update(msg)
		cmds = append(cmds, cmd)
	}

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
	v.MouseMode = tea.MouseModeCellMotion

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
		switch msg.MsgRole {
		case core.USER:
			content += c.user_style.
				Width(msg_width).
				Render(msg.DisplayText) + "\n"

		case core.LLM:
			postfix := ""
			if msg.IsError {
				postfix = lipgloss.NewStyle().
					Foreground(Color(CTPC_RED)).
					Render(fmt.Sprintf("\nError: %s", msg.ErrorText))
			}

			glamout, err := glam.Render(msg.DisplayText)
			if err != nil {
				logger.Error(err.Error())
			}
			content += glamout + postfix + "\n"
		}
	}

	// render currently streaming message
	if len(c.current_message.DisplayText) != 0 {
		glamout, err := glam.Render(c.current_message.DisplayText)
		if err != nil {
			logger.Error(err.Error())
		} else {
			content += glamout + "\n"
		}
	}

	return content
}
