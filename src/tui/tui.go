package tui

import (
	"context"
	"fmt"
	"os"

	// bubble tea tui fwk

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"

	"charm.land/lipgloss/v2"

	// openai sdk for chatcompletions

	"github.com/openai/openai-go/v3"

	// internal packages

	"github.com/t3snake/gocode/src/core"
	"github.com/t3snake/gocode/src/logger"
)

// StartTUI Starts and runs a bubbletea TUI program
func StartTUI() {
	tui2llm := make(chan core.Tui2Llm)
	llm2tui := make(chan core.Llm2Tui)

	p := tea.NewProgram(initialModel(llm2tui, tui2llm), tea.WithFPS(120))
	if _, err := p.Run(); err != nil {
		_, err2 := fmt.Fprintf(os.Stderr, "Error %v", err)
		if err2 != nil {
			panic(err2)
		}
		os.Exit(1)
	}
}

// ----- Main TUI Model Update View logic -----
type TuiMsgType uint8

const (
	USER TuiMsgType = iota
	LLM
	REASONING
	TOOLCALL
)

// TuiMessage represents the structure to be represented in TUI.
type TuiMessage struct {
	msg_type      TuiMsgType
	is_empty      bool   // Initially this is true, if any change happens will become false
	is_error      bool   // Initially false, true if there is an issue
	content       string // Relevant when [TuiMessage.msg_type] is [USER], [LLM] or [REASONING]
	error_content string // Relevant when [TuiMessage.is_error] is true
	tool_name     string // Relevant when [TuiMessage.msg_type] is [TOOLCALL]
	tool_args     string // Relevant when [TuiMessage.msg_type] is [TOOLCALL]
	tool_result   string // Relevant when [TuiMessage.msg_type] is [TOOLCALL]
}

// ChatState TUI main state
type ChatState struct {
	// window dimensions

	app_width  uint16
	app_height uint16

	// reusable bubbles

	prompt   textarea.Model
	viewport viewport.Model

	// messages (history) and currently streaming message

	message_history []openai.ChatCompletionMessageParamUnion

	tui_messages []TuiMessage

	// Represents LLM Message that acculumates with streaming and is later appended to [ChatState.tui_messages]
	current_message TuiMessage

	token_spend int

	// loading state
	is_loading bool
	spinner    spinner.Model

	// Theme related

	theme       Theme
	user_style  lipgloss.Style
	agent_style lipgloss.Style
	tool_style  lipgloss.Style

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

	us := lipgloss.NewStyle().Background(theme.UserChatBackground).Padding(1).MarginBottom(1)
	as := lipgloss.NewStyle().Background(theme.AgentChatBackground).Padding(1).MarginBottom(1)
	ts := lipgloss.NewStyle().Background(theme.ToolCallBackground).
		Foreground(Color(CTPC_SUBTEXT_0)).PaddingLeft(5).PaddingRight(5).MarginBottom(1)

	return ChatState{
		app_width:  400,
		app_height: 300,

		prompt:   ta,
		viewport: vp,

		message_history: []openai.ChatCompletionMessageParamUnion{},

		tui_messages: []TuiMessage{},
		current_message: TuiMessage{
			msg_type:      LLM,
			is_empty:      true,
			is_error:      false,
			content:       "",
			error_content: "",
			tool_name:     "",
			tool_args:     "",
			tool_result:   "",
		},

		is_loading: false,
		spinner:    s,

		theme:       theme,
		user_style:  us,
		agent_style: as,
		tool_style:  ts,

		llm2tui: llm2tui,
		tui2llm: tui2llm,

		ctx:        nil,
		ctx_cancel: nil,

		is_selecting: false,
	}
}

func (c ChatState) Init() tea.Cmd {
	// start listener immediately
	return tea.Batch(textarea.Blink, listenLlmStream(c.llm2tui))
}

func (c ChatState) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		c.app_height = uint16(msg.Height)
		c.app_width = uint16(msg.Width)

		c.prompt.SetWidth(msg.Width)

		c.viewport.SetWidth(msg.Width)

		if c.is_loading {
			c.viewport.SetHeight(msg.Height - 3)
		} else {
			c.viewport.SetHeight(msg.Height - c.prompt.Height() - 3)
		}

		c.viewport.Style = lipgloss.NewStyle().Align(lipgloss.Center)

		content := renderChatMessages(c)
		c.viewport.SetContent(content)
		// c.viewport.GotoBottom() // probably dont need to go to bottom when resizing, there will be some desync based on width resize

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
		if msg.llm_msg.IsChunk && len(msg.llm_msg.ChunkContent) != 0 {
			c.current_message.content += msg.llm_msg.ChunkContent
			c.current_message.is_empty = false

			// only rerender when there is a chunk content
			content := renderChatMessages(c)
			c.viewport.SetContent(content)
			c.viewport.GotoBottom()
		}

		if msg.llm_msg.IsToolCall {
			if c.current_message.msg_type == LLM && !c.current_message.is_empty {
				// append previous message if non empty and msg_type is LLM
				c.tui_messages = append(c.tui_messages, c.current_message)
			}

			// Note: TOOLCALL msg should have been appended already, IF the result came, else just ignore it

			// Tool Call requested case, reset current_message as TOOLCALL and store name and params
			c.current_message = resetCurrentMessage(TOOLCALL)
			c.current_message.tool_name = msg.llm_msg.ToolName
			c.current_message.tool_args = msg.llm_msg.ToolParams
			c.current_message.tool_result = ""
			c.current_message.is_empty = false

			if c.tui2llm != nil {
				// TODO(t3snake): implement tool call user interaction allow-reject
				c.tui2llm <- core.Tui2Llm{
					IsAllowed:        true, // currently hardcoding to true, ideally have a simple button selection
					AdjustmentPrompt: "",   // UX?
				}
			}

			content := renderChatMessages(c)
			c.viewport.SetContent(content)
			c.viewport.GotoBottom()
		}

		if msg.llm_msg.IsToolResult {
			if c.current_message.msg_type != TOOLCALL {
				logger.Error("ToolResult path called and current message is not of type TOOLCALL")
				c.current_message.error_content = "Erronous state with tool result, check logs"
			} else {
				c.current_message.tool_result = msg.llm_msg.ToolResult
			}

			c.tui_messages = append(c.tui_messages, c.current_message)
			c.current_message = resetCurrentMessage(LLM)
		}

		if msg.llm_msg.IsLoopDone {
			// append the assistant role message
			if !c.current_message.is_empty {
				c.tui_messages = append(c.tui_messages, c.current_message)
				c.current_message = resetCurrentMessage(LLM)
			}
		}

		// always have it active, by reopening listener immediately when returned
		cmd = listenLlmStream(c.llm2tui)

		return c, cmd

	case ChatResult:
		if msg.is_err {
			c.current_message.is_error = true
			c.current_message.error_content = msg.err
			c.current_message.is_empty = false
		}

		// get the exact message array from AgentLoop that can be passed back
		c.message_history = msg.prev_messages

		if !c.current_message.is_empty {
			c.tui_messages = append(c.tui_messages, c.current_message)
		}

		c.current_message = resetCurrentMessage(LLM)

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

			user_msg := resetCurrentMessage(USER)
			user_msg.content = prompt

			c.tui_messages = append(c.tui_messages, user_msg)

			c.current_message = resetCurrentMessage(LLM)

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
				promptLlm(prompt, c.message_history, c.ctx, c.tui2llm, c.llm2tui),
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

			if !c.is_loading {
				// Note: will stop from keypress going to viewport update (vimbindings jumping around)
				c.prompt, cmd = c.prompt.Update(msg)
				cmds = append(cmds, cmd)

				return c, tea.Batch(cmds...)
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
		spinnr := fmt.Sprintf("Thinking %s", c.spinner.View())
		view += spinnr
		cursor_y = lipgloss.Height(view)
		cursor_x = len(spinnr)
	} else {
		chatBoxStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(c.theme.ActiveBorder).
			Width(int(c.app_width)).
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

	// TODO move to fully programmed selection handling for the TUI
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
	msg_width := c.viewport.Width()
	txt_width := msg_width - 2 // subtract padding

	// md render lib
	style := styles.DarkStyleConfig
	glam, _ := glamour.NewTermRenderer(
		glamour.WithWordWrap(txt_width),
		glamour.WithStyles(style),
	)
	defer glam.Close()

	for _, msg := range c.tui_messages {
		switch msg.msg_type {
		case USER:
			content += c.user_style.
				Width(msg_width).
				Render(msg.content) + "\n"

		case LLM:
			if msg.is_empty {
				break // Note: does not break for, only skips rest of the code in this case
			}

			postfix := ""
			if msg.is_error {
				postfix = lipgloss.NewStyle().
					Foreground(Color(CTPC_RED)).
					Render(fmt.Sprintf("\nError: %s", msg.error_content))
			}

			glamout, err := glam.Render(msg.content)
			if err != nil {
				logger.Error(err.Error())
			}
			content += glamout + postfix + "\n"

		case REASONING: // TODO

		case TOOLCALL:
			content += c.tool_style.
				AlignHorizontal(lipgloss.Position(lipgloss.Center)).
				Render(fmt.Sprintf("✔ %s %s", msg.tool_name, msg.tool_args))

		default:
			panic("unhandled default case")
		}
	}

	// render currently streaming message
	if !c.current_message.is_empty && len(c.current_message.content) != 0 {
		if c.current_message.msg_type == LLM {
			glamout, err := glam.Render(c.current_message.content)
			if err != nil {
				logger.Error(err.Error())
			} else {
				content += glamout + "\n"
			}
		}

		if c.current_message.msg_type == TOOLCALL {
			content += c.tool_style.
				AlignHorizontal(lipgloss.Position(lipgloss.Center)).
				Render(fmt.Sprintf("☯ %s %s", c.current_message.tool_name, c.current_message.tool_args))
		}
	}

	return content
}

func resetCurrentMessage(msg_type TuiMsgType) TuiMessage {
	return TuiMessage{
		msg_type:      msg_type,
		is_empty:      true,
		is_error:      false,
		content:       "",
		error_content: "",
		tool_name:     "",
		tool_args:     "",
		tool_result:   "",
	}
}
