package main

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

const (
	CLR_PASTEL_YELLOW = "#FAEDCB"
	CLR_PASTEL_BLUE   = "#C6DEF1"
	CLR_PASTEL_GREEN  = "#C9E4DE"
	CLR_PASTEL_PURPLE = "#DBCDF0"
	CLR_PASTEL_ORANGE = "#F7D9C4"
	CLR_BLACK         = "#000000"

	// catpuccin macchiato theme
	// https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md
	CTPC_BG        = "#494d64"
	CTPC_ROSEWATER = "#f4dbd6"
	CTPC_CRUST     = "#181926"
	CTPC_LAVENDER  = "#b7bdf8"
	CTPC_OVERLAY_0 = "#6e738d"
	CTPC_SUBTEXT   = "#a5adcb"
	CTPC_RED       = "#ed8796"
	CTPC_BG_2      = "#5b6078"
	CTPC_BLUE      = "#8aadf4"
)

type Theme struct {
	Cursor              color.Color
	CursorText          color.Color
	ActiveBorder        color.Color
	InactiveBorder      color.Color
	Text                color.Color
	TerminalBackground  color.Color
	UserChatBackground  color.Color
	AgentChatBackground color.Color
}

var initialTheme = Theme{
	Cursor:              Color(CLR_PASTEL_PURPLE),
	CursorText:          Color(CLR_BLACK),
	ActiveBorder:        Color(CLR_BLACK),
	InactiveBorder:      Color(CLR_PASTEL_ORANGE),
	Text:                Color(CLR_BLACK),
	TerminalBackground:  Color(CLR_PASTEL_GREEN),
	UserChatBackground:  Color(CLR_PASTEL_YELLOW),
	AgentChatBackground: Color(CLR_PASTEL_BLUE),
}

var catpuccinMacchiatoTheme = Theme{
	Cursor:              Color(CTPC_ROSEWATER),
	CursorText:          Color(CTPC_CRUST),
	ActiveBorder:        Color(CTPC_LAVENDER),
	InactiveBorder:      Color(CTPC_OVERLAY_0),
	Text:                Color(CTPC_SUBTEXT),
	TerminalBackground:  Color(CTPC_BG),
	UserChatBackground:  Color(CTPC_BG_2),
	AgentChatBackground: Color(CTPC_BG_2),
}

func Color(hex string) color.Color {
	return lipgloss.Color(hex)
}
